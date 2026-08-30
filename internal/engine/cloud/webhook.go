package cloud

import (
	"context"
	"encoding/json"
	"strconv"
	"time"
)

// VerifyToken / AppSecret expõem o que o handler HTTP precisa pra validar o
// GET de verificação e a assinatura X-Hub-Signature-256 da Meta.
func (e *Engine) VerifyToken() string { return e.cfg.VerifyToken }
func (e *Engine) AppSecret() string   { return e.cfg.AppSecret }

/* estrutura do payload de webhook da Meta (só os campos que usamos) */

type waWebhook struct {
	Entry []struct {
		Changes []struct {
			Value struct {
				Metadata struct {
					PhoneNumberID string `json:"phone_number_id"`
				} `json:"metadata"`
				Contacts []struct {
					WaID    string `json:"wa_id"`
					Profile struct {
						Name string `json:"name"`
					} `json:"profile"`
				} `json:"contacts"`
				Messages []waInMessage     `json:"messages"`
				Statuses []waInStatus      `json:"statuses"`
				Errors   []json.RawMessage `json:"errors"`
			} `json:"value"`
			Field string `json:"field"`
		} `json:"changes"`
	} `json:"entry"`
}

type waInMessage struct {
	From      string `json:"from"`
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Text      struct {
		Body string `json:"body"`
	} `json:"text"`
	Image    *waInMedia `json:"image"`
	Video    *waInMedia `json:"video"`
	Audio    *waInMedia `json:"audio"`
	Document *waInMedia `json:"document"`
	Sticker  *waInMedia `json:"sticker"`
	Location *struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
		Name      string  `json:"name"`
		Address   string  `json:"address"`
	} `json:"location"`
	Interactive *struct {
		Type        string `json:"type"`
		ButtonReply *struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"button_reply"`
		ListReply *struct {
			ID          string `json:"id"`
			Title       string `json:"title"`
			Description string `json:"description"`
		} `json:"list_reply"`
	} `json:"interactive"`
	Button *struct {
		Text    string `json:"text"`
		Payload string `json:"payload"`
	} `json:"button"`
	Context *struct {
		From string `json:"from"`
		ID   string `json:"id"`
	} `json:"context"`
}

type waInMedia struct {
	ID       string `json:"id"`
	MimeType string `json:"mime_type"`
	Caption  string `json:"caption"`
	Filename string `json:"filename"`
	Sha256   string `json:"sha256"`
}

type waInStatus struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	Timestamp   string `json:"timestamp"`
	RecipientID string `json:"recipient_id"`
}

// Ingest processa um payload de webhook da Meta e emite os eventos canônicos
// (message / message.any / message.ack), iguais aos do whatsmeow.
func (e *Engine) Ingest(payload []byte) {
	var wh waWebhook
	if err := json.Unmarshal(payload, &wh); err != nil {
		e.deps.Logger.Warn("cloud webhook: json inválido", "err", err)
		return
	}
	for _, entry := range wh.Entry {
		for _, ch := range entry.Changes {
			v := ch.Value
			names := map[string]string{}
			for _, c := range v.Contacts {
				names[c.WaID] = c.Profile.Name
			}
			for i := range v.Messages {
				e.handleInbound(&v.Messages[i], names[v.Messages[i].From])
			}
			for _, st := range v.Statuses {
				e.emitAck(st)
			}
		}
	}
}

func atoiTs(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	if n == 0 {
		return time.Now().Unix()
	}
	return n
}

func (e *Engine) handleInbound(m *waInMessage, pushName string) {
	chat := m.From + "@s.whatsapp.net"
	p := map[string]any{
		"id":        m.ID,
		"chatId":    chat,
		"from":      chat,
		"fromMe":    false,
		"isGroup":   false,
		"pushName":  pushName,
		"type":      m.Type,
		"timestamp": atoiTs(m.Timestamp),
		"body":      "",
	}
	if m.Context != nil && m.Context.ID != "" {
		p["quotedId"] = m.Context.ID
	}

	var media *waInMedia
	switch m.Type {
	case "text":
		p["body"] = m.Text.Body
	case "image":
		media = m.Image
	case "video":
		media = m.Video
	case "audio":
		media = m.Audio
	case "document":
		media = m.Document
	case "sticker":
		media = m.Sticker
	case "location":
		if m.Location != nil {
			p["location"] = map[string]any{
				"latitude": m.Location.Latitude, "longitude": m.Location.Longitude,
				"name": m.Location.Name, "address": m.Location.Address,
			}
		}
	case "interactive":
		if m.Interactive != nil {
			p["type"] = "button" // normaliza: resposta de botão/lista
			var id, title string
			if m.Interactive.ButtonReply != nil {
				id, title = m.Interactive.ButtonReply.ID, m.Interactive.ButtonReply.Title
			} else if m.Interactive.ListReply != nil {
				id, title = m.Interactive.ListReply.ID, m.Interactive.ListReply.Title
			}
			p["body"] = title
			p["reply"] = map[string]any{"id": id, "title": title}
		}
	case "button":
		if m.Button != nil {
			p["type"] = "button"
			p["body"] = m.Button.Text
			p["reply"] = map[string]any{"id": m.Button.Payload, "title": m.Button.Text}
		}
	}

	if media != nil && media.ID != "" {
		p["hasMedia"] = true
		p["mediaType"] = m.Type
		p["mediaMeta"] = map[string]any{
			"cloudId": media.ID, "mimetype": media.MimeType, "filename": media.Filename,
		}
		if media.Caption != "" {
			p["body"] = media.Caption
		}
		e.mu.RLock()
		jobs := e.mediaJobs
		e.mu.RUnlock()
		select {
		case jobs <- mediaJob{p: p, mediaID: media.ID, mime: media.MimeType, msgID: m.ID}:
			return // worker faz o fetch + emit
		default:
			// pool cheio/parado: emite já, sem a mídia baixada (o mediaMeta
			// permite baixar depois via /media/download).
		}
	}
	e.emitMessage(p)
}

// attachMedia baixa a mídia e guarda no sink; anexa "media" no payload.
func (e *Engine) attachMedia(p map[string]any, mediaID, mime, msgID string) {
	if e.deps.Media == nil || !e.deps.Media.Enabled() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	data, m, err := e.fetchMedia(ctx, mediaID)
	cancel()
	if err != nil {
		p["media"] = map[string]any{"error": err.Error()}
		return
	}
	if mime == "" {
		mime = m
	}
	url, size, serr := e.deps.Media.Store(context.Background(), e.deps.Session, msgID, mime, data)
	if serr != nil {
		p["media"] = map[string]any{"error": serr.Error()}
		return
	}
	p["media"] = map[string]any{"id": msgID, "url": url, "mimetype": mime, "size": size}
}

// emitMessage usa o wamid como ID do evento — assim uma re-entrega da Meta
// (se o 200 demorou) produz o MESMO event id e o dispatcher dedup por TaskID.
func (e *Engine) emitMessage(p map[string]any) {
	id, _ := p["id"].(string)
	e.emitID(id, "message.any", p)
	if fm, _ := p["fromMe"].(bool); !fm {
		e.emitID(id, "message", p)
	}
}

func (e *Engine) emitAck(st waInStatus) {
	e.emitID(st.ID+":"+st.Status, "message.ack", map[string]any{
		"ids":       []string{st.ID},
		"type":      st.Status, // sent|delivered|read|failed
		"chatId":    st.RecipientID + "@s.whatsapp.net",
		"timestamp": atoiTs(st.Timestamp),
	})
}

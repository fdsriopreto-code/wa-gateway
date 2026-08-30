// Package leads mantém, por (sessão, contato), uma visão de conversa pronta pra
// um CRM: últimas mensagens (recebida / enviada / lida por eles), tempo sem
// resposta, contagens, tempo médio de resposta e a ORIGEM do lead (anúncio
// Click-to-WhatsApp, UTMs, click ids). Consome o log durável de eventos, então
// vale pros dois motores e sobrevive a restart.
package leads

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"wa-gateway/internal/events"
	"wa-gateway/internal/store"
)

// SessionCfg é o mínimo que precisamos do Manager (evita ciclo de import).
type SessionCfg interface {
	LeadsEnabled(session string) bool
}

type Ingestor struct {
	store  *store.Store
	cfg    SessionCfg
	bus    *events.Bus
	stream *events.Stream
	log    *slog.Logger
	nodeID string
	stale  time.Duration
}

func New(st *store.Store, cfg SessionCfg, bus *events.Bus, stream *events.Stream, log *slog.Logger, nodeID string, stale time.Duration) *Ingestor {
	if stale <= 0 {
		stale = 2 * time.Hour
	}
	return &Ingestor{store: st, cfg: cfg, bus: bus, stream: stream, log: log.With("comp", "leads"), nodeID: nodeID, stale: stale}
}

// Run liga o consumidor (message.any p/ métricas + message.ack p/ "lida por eles").
func (i *Ingestor) Run(ctx context.Context, stream *events.Stream) {
	if stream == nil {
		return
	}
	stream.Consume(ctx, "leads", i.nodeID, []string{events.MessageAny, events.MessageAck}, 4, i.handle)
}

func (i *Ingestor) handle(ctx context.Context, e events.Event) error {
	if e.Session == "" || (i.cfg != nil && !i.cfg.LeadsEnabled(e.Session)) {
		return nil
	}
	p, _ := e.Payload.(map[string]any)
	if p == nil {
		return nil
	}
	chat, _ := p["chatId"].(string)
	if !is1to1(chat) {
		return nil // grupos, broadcast, status, newsletter: fora
	}
	ts := tsOf(p["timestamp"])
	if ts.IsZero() {
		ts = e.Timestamp
	}
	phone := digits(chat)

	switch e.Name {
	case events.MessageAny:
		preview := preview(p)
		if fromMe, _ := p["fromMe"].(bool); fromMe {
			return i.store.RecordOutbound(ctx, e.Session, chat, phone, preview, ts)
		}
		src := ExtractSource(p)
		raw, _ := json.Marshal(src)
		isNew, err := i.store.RecordInbound(ctx, e.Session, chat, phone, str(p["pushName"]), preview, ts, raw)
		if err != nil {
			return err
		}
		if isNew {
			i.emit(events.LeadNew, e.Session, map[string]any{
				"chatId": chat, "phone": phone, "pushName": str(p["pushName"]),
				"source": src, "firstMessage": src.FirstMessage,
			})
		}
		return nil

	case events.MessageAck:
		switch strings.ToLower(str(p["type"])) {
		case "read", "played":
			return i.store.RecordRead(ctx, e.Session, chat, ts)
		}
	}
	return nil
}

// SweepStale roda em loop: leads em waiting_us há mais que `stale` viram
// evento lead.stale (uma vez). Bloqueia até ctx acabar.
func (i *Ingestor) SweepStale(ctx context.Context) {
	t := time.NewTicker(60 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			rows, err := i.store.ClaimStaleLeads(ctx, i.stale)
			if err != nil {
				i.log.Warn("sweep stale", "err", err)
				continue
			}
			for _, r := range rows {
				i.emit(events.LeadStale, r.Session, map[string]any{
					"chatId": r.ChatID, "phone": r.Phone, "pushName": r.PushName,
					"waitingSince":   r.WaitingSince.UTC().Format(time.RFC3339),
					"waitingSeconds": r.WaitingSeconds,
				})
			}
		}
	}
}

func (i *Ingestor) emit(name, session string, payload map[string]any) {
	ev := events.Event{
		ID: events.NewID(), Session: session, Name: name,
		Timestamp: time.Now().UTC(), Engine: "wa-gateway", Payload: payload,
	}
	if i.bus != nil {
		i.bus.Publish(ev)
	}
	i.stream.Append(ev) // nil-safe
}

/* ---- helpers ---- */

func str(v any) string { s, _ := v.(string); return s }

func is1to1(chat string) bool {
	return strings.HasSuffix(chat, "@s.whatsapp.net") || strings.HasSuffix(chat, "@c.us")
}

func digits(s string) string {
	if i := strings.IndexByte(s, '@'); i >= 0 {
		s = s[:i]
	}
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func tsOf(v any) time.Time {
	switch n := v.(type) {
	case float64:
		return time.Unix(int64(n), 0).UTC()
	case int64:
		return time.Unix(n, 0).UTC()
	case int:
		return time.Unix(int64(n), 0).UTC()
	case string:
		if t, err := time.Parse(time.RFC3339, n); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

// preview monta um resumo curto da última mensagem pro CRM.
func preview(p map[string]any) string {
	if b := strings.TrimSpace(str(p["body"])); b != "" {
		return trunc(b, 140)
	}
	if t := str(p["type"]); t != "" && t != "text" {
		switch t {
		case "image":
			return "[imagem]"
		case "audio", "ptt":
			return "[áudio]"
		case "video":
			return "[vídeo]"
		case "document":
			return "[documento]"
		case "sticker":
			return "[figurinha]"
		case "location":
			return "[localização]"
		case "contact", "contacts":
			return "[contato]"
		default:
			return "[" + t + "]"
		}
	}
	return ""
}

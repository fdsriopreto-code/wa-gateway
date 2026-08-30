package cloud

import (
	"context"
	"encoding/json"
	"fmt"

	"wa-gateway/internal/engine"
)

func (e *Engine) msg(to string, extra map[string]any) map[string]any {
	m := map[string]any{
		"messaging_product": "whatsapp",
		"recipient_type":    "individual",
		"to":                toNumber(to),
	}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func (e *Engine) send(ctx context.Context, body map[string]any) (engine.SendResult, error) {
	var raw json.RawMessage
	if err := e.graphPOST(ctx, "", body, &raw); err != nil {
		return engine.SendResult{}, err
	}
	return sendResultFromResp(raw), nil
}

func withContext(body map[string]any, opts engine.MessageOpts) map[string]any {
	if opts.QuotedID != "" {
		body["context"] = map[string]any{"message_id": opts.QuotedID}
	}
	return body
}

func (e *Engine) SendText(ctx context.Context, chatID, text string, opts engine.MessageOpts) (engine.SendResult, error) {
	body := e.msg(chatID, map[string]any{
		"type": "text",
		"text": map[string]any{"body": text, "preview_url": opts.LinkPreview},
	})
	return e.send(ctx, withContext(body, opts))
}

// mediaPayload sobe a mídia (bytes) e devolve {id} ou, se veio URL, {link}.
func (e *Engine) mediaObj(ctx context.Context, m engine.Media, data []byte, mimetype string, withCaption, withFilename bool) (map[string]any, error) {
	obj := map[string]any{}
	if len(data) > 0 {
		id, err := e.uploadMedia(ctx, data, mimetype)
		if err != nil {
			return nil, err
		}
		obj["id"] = id
	} else {
		return nil, fmt.Errorf("cloud: sem bytes de mídia (envie o binário; link externo não é suportado aqui)")
	}
	if withCaption && m.Caption != "" {
		obj["caption"] = m.Caption
	}
	if withFilename && m.Filename != "" {
		obj["filename"] = m.Filename
	}
	return obj, nil
}

func (e *Engine) sendMedia(ctx context.Context, chatID, kind string, m engine.Media, data []byte, mimetype string, caption, filename bool) (engine.SendResult, error) {
	obj, err := e.mediaObj(ctx, m, data, mimetype, caption, filename)
	if err != nil {
		return engine.SendResult{}, err
	}
	body := e.msg(chatID, map[string]any{"type": kind, kind: obj})
	return e.send(ctx, withContext(body, m.Opts))
}

func (e *Engine) SendImage(ctx context.Context, chatID string, data []byte, mimetype, caption string) (engine.SendResult, error) {
	return e.sendMedia(ctx, chatID, "image", engine.Media{Caption: caption}, data, mimetype, true, false)
}
func (e *Engine) SendFile(ctx context.Context, chatID string, m engine.Media) (engine.SendResult, error) {
	return e.sendMedia(ctx, chatID, "document", m, m.Data, m.Mimetype, true, true)
}
func (e *Engine) SendVideo(ctx context.Context, chatID string, m engine.Media) (engine.SendResult, error) {
	return e.sendMedia(ctx, chatID, "video", m, m.Data, m.Mimetype, true, false)
}
func (e *Engine) SendAudio(ctx context.Context, chatID string, m engine.Media) (engine.SendResult, error) {
	return e.sendMedia(ctx, chatID, "audio", m, m.Data, m.Mimetype, false, false)
}
func (e *Engine) SendSticker(ctx context.Context, chatID string, data []byte, opts engine.MessageOpts) (engine.SendResult, error) {
	return e.sendMedia(ctx, chatID, "sticker", engine.Media{Opts: opts}, data, "image/webp", false, false)
}

func (e *Engine) SendLocation(ctx context.Context, chatID string, loc engine.Location) (engine.SendResult, error) {
	body := e.msg(chatID, map[string]any{
		"type": "location",
		"location": map[string]any{
			"latitude": loc.Latitude, "longitude": loc.Longitude,
			"name": loc.Name, "address": loc.Address,
		},
	})
	return e.send(ctx, body)
}

func (e *Engine) SendContact(ctx context.Context, chatID string, cs []engine.Contact) (engine.SendResult, error) {
	contacts := make([]map[string]any, 0, len(cs))
	for _, c := range cs {
		contacts = append(contacts, map[string]any{
			"name":   map[string]any{"formatted_name": c.Name, "first_name": c.Name},
			"phones": []map[string]any{{"phone": c.Phone, "type": "CELL"}},
		})
	}
	return e.send(ctx, e.msg(chatID, map[string]any{"type": "contacts", "contacts": contacts}))
}

func (e *Engine) SendReaction(ctx context.Context, ref engine.MessageRef, emoji string) (engine.SendResult, error) {
	body := e.msg(ref.ChatID, map[string]any{
		"type":     "reaction",
		"reaction": map[string]any{"message_id": ref.ID, "emoji": emoji},
	})
	return e.send(ctx, body)
}

func (e *Engine) MarkRead(ctx context.Context, ref engine.MessageRef) error {
	body := map[string]any{
		"messaging_product": "whatsapp",
		"status":            "read",
		"message_id":        ref.ID,
	}
	return e.graphPOST(ctx, "/"+e.cfg.PhoneNumberID+"/messages", body, nil)
}

/* -------- interativas + template (o motivo de existir a Cloud API) -------- */

func (e *Engine) SendInteractive(ctx context.Context, chatID string, i engine.Interactive) (engine.SendResult, error) {
	inter := map[string]any{"body": map[string]any{"text": i.Body}}
	if i.Header != "" {
		inter["header"] = map[string]any{"type": "text", "text": i.Header}
	}
	if i.Footer != "" {
		inter["footer"] = map[string]any{"text": i.Footer}
	}
	switch i.Type {
	case "button":
		btns := make([]map[string]any, 0, len(i.Buttons))
		for _, b := range i.Buttons {
			btns = append(btns, map[string]any{"type": "reply", "reply": map[string]any{"id": b.ID, "title": b.Title}})
		}
		inter["type"] = "button"
		inter["action"] = map[string]any{"buttons": btns}
	case "list":
		secs := make([]map[string]any, 0, len(i.Sections))
		for _, s := range i.Sections {
			rows := make([]map[string]any, 0, len(s.Rows))
			for _, r := range s.Rows {
				rows = append(rows, map[string]any{"id": r.ID, "title": r.Title, "description": r.Description})
			}
			secs = append(secs, map[string]any{"title": s.Title, "rows": rows})
		}
		inter["type"] = "list"
		inter["action"] = map[string]any{"button": i.ButtonText, "sections": secs}
	case "cta_url":
		inter["type"] = "cta_url"
		inter["action"] = map[string]any{
			"name":       "cta_url",
			"parameters": map[string]any{"display_text": i.DisplayURL, "url": i.URL},
		}
	default:
		return engine.SendResult{}, fmt.Errorf("cloud: tipo interativo desconhecido %q (use button|list|cta_url)", i.Type)
	}
	return e.send(ctx, e.msg(chatID, map[string]any{"type": "interactive", "interactive": inter}))
}

func (e *Engine) SendTemplate(ctx context.Context, chatID, name, lang string, components json.RawMessage) (engine.SendResult, error) {
	tpl := map[string]any{
		"name":     name,
		"language": map[string]any{"code": lang},
	}
	if len(components) > 0 {
		var comp any
		if err := json.Unmarshal(components, &comp); err != nil {
			return engine.SendResult{}, fmt.Errorf("components inválido: %w", err)
		}
		tpl["components"] = comp
	}
	return e.send(ctx, e.msg(chatID, map[string]any{"type": "template", "template": tpl}))
}

/* ---------------- não suportado na Cloud API ---------------- */

func (e *Engine) SendPoll(context.Context, string, string, []string, int, engine.MessageOpts) (engine.SendResult, error) {
	return engine.SendResult{}, engine.ErrNotSupported
}
func (e *Engine) Forward(context.Context, string, engine.ForwardSource) (engine.SendResult, error) {
	return engine.SendResult{}, engine.ErrNotSupported
}
func (e *Engine) DeleteMessage(context.Context, engine.MessageRef) (engine.SendResult, error) {
	return engine.SendResult{}, engine.ErrNotSupported
}
func (e *Engine) EditMessage(context.Context, engine.MessageRef, string) (engine.SendResult, error) {
	return engine.SendResult{}, engine.ErrNotSupported
}
func (e *Engine) SendChatPresence(context.Context, string, engine.ChatState) error {
	return engine.ErrNotSupported
}
func (e *Engine) CheckOnWhatsApp(context.Context, []string) ([]engine.OnWhatsApp, error) {
	return nil, engine.ErrNotSupported
}
func (e *Engine) GetUserInfo(context.Context, []string) ([]engine.UserInfo, error) {
	return nil, engine.ErrNotSupported
}
func (e *Engine) GetProfilePicture(context.Context, string, bool) (string, error) {
	return "", engine.ErrNotSupported
}
func (e *Engine) Contacts(context.Context) ([]engine.ContactEntry, error) {
	return nil, engine.ErrNotSupported
}
func (e *Engine) Labels(context.Context) ([]engine.Label, error) { return nil, engine.ErrNotSupported }
func (e *Engine) EditLabel(context.Context, string, string, int32, bool) error {
	return engine.ErrNotSupported
}
func (e *Engine) SetChatLabel(context.Context, string, string, bool) error {
	return engine.ErrNotSupported
}
func (e *Engine) SetMessageLabel(context.Context, string, string, string, bool) error {
	return engine.ErrNotSupported
}
func (e *Engine) PairPhone(context.Context, string) (string, error) {
	return "", engine.ErrNotSupported
}
func (e *Engine) SetStatusMessage(context.Context, string) error { return engine.ErrNotSupported }
func (e *Engine) SetPresence(context.Context, bool) error        { return engine.ErrNotSupported }
func (e *Engine) SetBlocked(context.Context, string, bool) ([]string, error) {
	return nil, engine.ErrNotSupported
}
func (e *Engine) Blocklist(context.Context) ([]string, error) { return nil, engine.ErrNotSupported }
func (e *Engine) ListGroups(context.Context) ([]engine.Group, error) {
	return nil, engine.ErrNotSupported
}
func (e *Engine) GroupInfo(context.Context, string) (engine.Group, error) {
	return engine.Group{}, engine.ErrNotSupported
}
func (e *Engine) CreateGroup(context.Context, string, []string) (engine.Group, error) {
	return engine.Group{}, engine.ErrNotSupported
}
func (e *Engine) LeaveGroup(context.Context, string) error { return engine.ErrNotSupported }
func (e *Engine) UpdateParticipants(context.Context, string, engine.ParticipantAction, []string) ([]engine.GroupParticipant, error) {
	return nil, engine.ErrNotSupported
}
func (e *Engine) SetGroupName(context.Context, string, string) error  { return engine.ErrNotSupported }
func (e *Engine) SetGroupTopic(context.Context, string, string) error { return engine.ErrNotSupported }
func (e *Engine) SetGroupPhoto(context.Context, string, []byte) (string, error) {
	return "", engine.ErrNotSupported
}
func (e *Engine) SetGroupAnnounce(context.Context, string, bool) error { return engine.ErrNotSupported }
func (e *Engine) SetGroupLocked(context.Context, string, bool) error   { return engine.ErrNotSupported }
func (e *Engine) GroupInviteLink(context.Context, string, bool) (string, error) {
	return "", engine.ErrNotSupported
}
func (e *Engine) JoinGroupWithLink(context.Context, string) (string, error) {
	return "", engine.ErrNotSupported
}

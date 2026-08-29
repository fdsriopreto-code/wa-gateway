package whatsmeow

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"

	"wa-gateway/internal/engine"
)

// result normaliza a resposta de SendMessage.
func result(resp whatsmeow.SendResponse) engine.SendResult {
	return engine.SendResult{MessageID: resp.ID, Timestamp: resp.Timestamp.Unix()}
}

// send resolve o client, parseia o chatId e despacha a mensagem ja montada.
func (e *Engine) send(ctx context.Context, chatID string, msg *waProto.Message) (engine.SendResult, error) {
	client, err := e.currentClient()
	if err != nil {
		return engine.SendResult{}, err
	}
	jid, err := types.ParseJID(chatID)
	if err != nil {
		return engine.SendResult{}, fmt.Errorf("chatId invalido: %w", err)
	}
	resp, err := client.SendMessage(ctx, jid, msg)
	if err != nil {
		return engine.SendResult{}, err
	}
	return result(resp), nil
}

// uploadMedia sobe o binario para o CDN do WhatsApp.
func (e *Engine) uploadMedia(ctx context.Context, data []byte, mt whatsmeow.MediaType) (whatsmeow.UploadResponse, error) {
	client, err := e.currentClient()
	if err != nil {
		return whatsmeow.UploadResponse{}, err
	}
	up, err := client.Upload(ctx, data, mt)
	if err != nil {
		return whatsmeow.UploadResponse{}, fmt.Errorf("upload: %w", err)
	}
	return up, nil
}

func (e *Engine) SendFile(ctx context.Context, chatID string, m engine.Media) (engine.SendResult, error) {
	up, err := e.uploadMedia(ctx, m.Data, whatsmeow.MediaDocument)
	if err != nil {
		return engine.SendResult{}, err
	}
	mime := m.Mimetype
	if mime == "" {
		mime = "application/octet-stream"
	}
	name := m.Filename
	if name == "" {
		name = "file"
	}
	msg := &waProto.Message{DocumentMessage: &waProto.DocumentMessage{
		URL:           proto.String(up.URL),
		DirectPath:    proto.String(up.DirectPath),
		Mimetype:      proto.String(mime),
		Title:         proto.String(name),
		FileName:      proto.String(name),
		Caption:       strPtrOrNil(m.Caption),
		MediaKey:      up.MediaKey,
		FileEncSHA256: up.FileEncSHA256,
		FileSHA256:    up.FileSHA256,
		FileLength:    proto.Uint64(up.FileLength),
	}}
	return e.send(ctx, chatID, msg)
}

func (e *Engine) SendVideo(ctx context.Context, chatID string, m engine.Media) (engine.SendResult, error) {
	up, err := e.uploadMedia(ctx, m.Data, whatsmeow.MediaVideo)
	if err != nil {
		return engine.SendResult{}, err
	}
	mime := m.Mimetype
	if mime == "" {
		mime = "video/mp4"
	}
	vm := &waProto.VideoMessage{
		URL:           proto.String(up.URL),
		DirectPath:    proto.String(up.DirectPath),
		Mimetype:      proto.String(mime),
		Caption:       strPtrOrNil(m.Caption),
		MediaKey:      up.MediaKey,
		FileEncSHA256: up.FileEncSHA256,
		FileSHA256:    up.FileSHA256,
		FileLength:    proto.Uint64(up.FileLength),
	}
	if m.Seconds > 0 {
		vm.Seconds = proto.Uint32(m.Seconds)
	}
	if m.GIF {
		vm.GifPlayback = proto.Bool(true)
	}
	return e.send(ctx, chatID, &waProto.Message{VideoMessage: vm})
}

func (e *Engine) SendAudio(ctx context.Context, chatID string, m engine.Media) (engine.SendResult, error) {
	up, err := e.uploadMedia(ctx, m.Data, whatsmeow.MediaAudio)
	if err != nil {
		return engine.SendResult{}, err
	}
	mime := m.Mimetype
	if mime == "" {
		mime = "audio/ogg; codecs=opus"
	}
	am := &waProto.AudioMessage{
		URL:           proto.String(up.URL),
		DirectPath:    proto.String(up.DirectPath),
		Mimetype:      proto.String(mime),
		MediaKey:      up.MediaKey,
		FileEncSHA256: up.FileEncSHA256,
		FileSHA256:    up.FileSHA256,
		FileLength:    proto.Uint64(up.FileLength),
		PTT:           proto.Bool(m.Voice),
	}
	if m.Seconds > 0 {
		am.Seconds = proto.Uint32(m.Seconds)
	}
	return e.send(ctx, chatID, &waProto.Message{AudioMessage: am})
}

func (e *Engine) SendLocation(ctx context.Context, chatID string, loc engine.Location) (engine.SendResult, error) {
	lm := &waProto.LocationMessage{
		DegreesLatitude:  proto.Float64(loc.Latitude),
		DegreesLongitude: proto.Float64(loc.Longitude),
	}
	if loc.Name != "" {
		lm.Name = proto.String(loc.Name)
	}
	if loc.Address != "" {
		lm.Address = proto.String(loc.Address)
	}
	return e.send(ctx, chatID, &waProto.Message{LocationMessage: lm})
}

func (e *Engine) SendContact(ctx context.Context, chatID string, cs []engine.Contact) (engine.SendResult, error) {
	if len(cs) == 0 {
		return engine.SendResult{}, fmt.Errorf("nenhum contato informado")
	}
	if len(cs) == 1 {
		c := cs[0]
		return e.send(ctx, chatID, &waProto.Message{ContactMessage: &waProto.ContactMessage{
			DisplayName: proto.String(c.Name),
			Vcard:       proto.String(vcard(c)),
		}})
	}
	arr := &waProto.ContactsArrayMessage{DisplayName: proto.String(fmt.Sprintf("%d contatos", len(cs)))}
	for _, c := range cs {
		arr.Contacts = append(arr.Contacts, &waProto.ContactMessage{
			DisplayName: proto.String(c.Name),
			Vcard:       proto.String(vcard(c)),
		})
	}
	return e.send(ctx, chatID, &waProto.Message{ContactsArrayMessage: arr})
}

// vcard devolve o VCard do contato: usa o fornecido, ou monta um minimo.
func vcard(c engine.Contact) string {
	if strings.Contains(c.VCard, "BEGIN:VCARD") {
		return c.VCard
	}
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, c.Phone)
	var b strings.Builder
	b.WriteString("BEGIN:VCARD\nVERSION:3.0\n")
	fmt.Fprintf(&b, "FN:%s\n", c.Name)
	if digits != "" {
		fmt.Fprintf(&b, "TEL;type=CELL;type=VOICE;waid=%s:+%s\n", digits, digits)
	}
	b.WriteString("END:VCARD")
	return b.String()
}

// --- operacoes sobre mensagens existentes ---

func (e *Engine) refJIDs(ref engine.MessageRef) (chat, sender types.JID, err error) {
	chat, err = types.ParseJID(ref.ChatID)
	if err != nil {
		return chat, sender, fmt.Errorf("chatId invalido: %w", err)
	}
	switch {
	case ref.FromMe:
		if c := e.clientJID(); !c.IsEmpty() {
			sender = c.ToNonAD()
		}
	case ref.SenderID != "":
		sender, err = types.ParseJID(ref.SenderID)
		if err != nil {
			return chat, sender, fmt.Errorf("senderId invalido: %w", err)
		}
	default:
		sender = chat // chat direto: o remetente e o proprio chat
	}
	return chat, sender, nil
}

func (e *Engine) clientJID() types.JID {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.client == nil || e.client.Store == nil || e.client.Store.ID == nil {
		return types.EmptyJID
	}
	return *e.client.Store.ID
}

func (e *Engine) SendReaction(ctx context.Context, ref engine.MessageRef, emoji string) (engine.SendResult, error) {
	client, err := e.currentClient()
	if err != nil {
		return engine.SendResult{}, err
	}
	chat, sender, err := e.refJIDs(ref)
	if err != nil {
		return engine.SendResult{}, err
	}
	msg := client.BuildReaction(chat, sender, ref.ID, emoji)
	resp, err := client.SendMessage(ctx, chat, msg)
	if err != nil {
		return engine.SendResult{}, err
	}
	return result(resp), nil
}

func (e *Engine) DeleteMessage(ctx context.Context, ref engine.MessageRef) (engine.SendResult, error) {
	client, err := e.currentClient()
	if err != nil {
		return engine.SendResult{}, err
	}
	chat, sender, err := e.refJIDs(ref)
	if err != nil {
		return engine.SendResult{}, err
	}
	msg := client.BuildRevoke(chat, sender, ref.ID)
	resp, err := client.SendMessage(ctx, chat, msg)
	if err != nil {
		return engine.SendResult{}, err
	}
	return result(resp), nil
}

func (e *Engine) EditMessage(ctx context.Context, ref engine.MessageRef, newText string) (engine.SendResult, error) {
	client, err := e.currentClient()
	if err != nil {
		return engine.SendResult{}, err
	}
	chat, _, err := e.refJIDs(ref)
	if err != nil {
		return engine.SendResult{}, err
	}
	newContent := &waProto.Message{Conversation: proto.String(newText)}
	msg := client.BuildEdit(chat, ref.ID, newContent)
	resp, err := client.SendMessage(ctx, chat, msg)
	if err != nil {
		return engine.SendResult{}, err
	}
	return result(resp), nil
}

func (e *Engine) MarkRead(ctx context.Context, ref engine.MessageRef) error {
	client, err := e.currentClient()
	if err != nil {
		return err
	}
	chat, sender, err := e.refJIDs(ref)
	if err != nil {
		return err
	}
	return client.MarkRead(ctx, []types.MessageID{ref.ID}, time.Now(), chat, sender)
}

func (e *Engine) SendChatPresence(ctx context.Context, chatID string, state engine.ChatState) error {
	client, err := e.currentClient()
	if err != nil {
		return err
	}
	jid, err := types.ParseJID(chatID)
	if err != nil {
		return fmt.Errorf("chatId invalido: %w", err)
	}
	var (
		cp    types.ChatPresence
		media types.ChatPresenceMedia
	)
	switch state {
	case engine.ChatStateTyping:
		cp, media = types.ChatPresenceComposing, types.ChatPresenceMediaText
	case engine.ChatStateRecording:
		cp, media = types.ChatPresenceComposing, types.ChatPresenceMediaAudio
	case engine.ChatStatePaused:
		cp = types.ChatPresencePaused
	default:
		return fmt.Errorf("estado invalido: %s", state)
	}
	// presenca de chat exige que a sessao esteja "available" primeiro.
	_ = client.SendPresence(ctx, types.PresenceAvailable)
	return client.SendChatPresence(ctx, jid, cp, media)
}

func strPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return proto.String(s)
}

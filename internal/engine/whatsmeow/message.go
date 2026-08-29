package whatsmeow

import (
	waProto "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	waEvents "go.mau.fi/whatsmeow/types/events"
)

// normalizeMessage achata um *events.Message num payload estavel e enxuto,
// no estilo do WAHA/Evolution, mas com nomes proprios. Com includeRaw, o
// struct cru do whatsmeow vai em "raw".
func normalizeMessage(evt *waEvents.Message, includeRaw bool) map[string]any {
	info := evt.Info
	msg := evt.Message

	from := preferPN(info.Sender, info.SenderAlt)
	chat := info.Chat
	if !info.IsGroup {
		// em DM o "chat" e a propria pessoa; resolve @lid -> telefone.
		chat = preferPN(info.Chat, pickAlt(info))
	}

	kind, body := classifyMessage(msg)

	out := map[string]any{
		"id":        info.ID,
		"chatId":    jidStr(chat),
		"from":      jidStr(from),
		"fromMe":    info.IsFromMe,
		"isGroup":   info.IsGroup,
		"pushName":  info.PushName,
		"type":      kind,
		"timestamp": info.Timestamp.Unix(),
		"body":      body,
	}
	if includeRaw {
		out["raw"] = evt
	}
	if info.Chat.Server == types.HiddenUserServer {
		out["chatLid"] = info.Chat.String()
	}
	if info.IsGroup {
		out["author"] = jidStr(preferPN(info.Sender, info.SenderAlt))
	}
	if info.MediaType != "" {
		out["hasMedia"] = true
		out["mediaType"] = info.MediaType
	}
	if mm := mediaMetaOf(msg); mm != nil {
		out["mediaMeta"] = mm // p/ baixar depois (bytes viram base64 no webhook)
	}
	if ci := contextInfo(msg); ci != nil {
		if ci.GetStanzaID() != "" {
			out["quotedId"] = ci.GetStanzaID()
		}
		if len(ci.GetMentionedJID()) > 0 {
			out["mentions"] = ci.GetMentionedJID()
		}
	}
	if r := msg.GetReactionMessage(); r != nil {
		out["reaction"] = r.GetText()
		if k := r.GetKey(); k != nil {
			out["reactionTargetId"] = k.GetID()
		}
	}
	return out
}

// normalizeReceipt achata um recibo de entrega/leitura.
func normalizeReceipt(evt *waEvents.Receipt, includeRaw bool) map[string]any {
	kind := "delivered"
	switch evt.Type {
	case types.ReceiptTypeRead, types.ReceiptTypeReadSelf:
		kind = "read"
	case types.ReceiptTypePlayed:
		kind = "played"
	case types.ReceiptTypeRetry:
		kind = "retry"
	case types.ReceiptTypeSender:
		kind = "sender"
	}
	out := map[string]any{
		"ids":       evt.MessageIDs,
		"chatId":    jidStr(preferPN(evt.Chat, evt.SenderAlt)),
		"from":      jidStr(preferPN(evt.Sender, evt.SenderAlt)),
		"type":      kind,
		"timestamp": evt.Timestamp.Unix(),
	}
	if includeRaw {
		out["raw"] = evt
	}
	return out
}

// dl e o subconjunto de campos que qualquer *Message de midia expoe.
type dl interface {
	GetDirectPath() string
	GetMediaKey() []byte
	GetFileSHA256() []byte
	GetFileEncSHA256() []byte
	GetFileLength() uint64
	GetMimetype() string
}

func mediaPart(m *waProto.Message) dl {
	switch {
	case m == nil:
		return nil
	case m.GetImageMessage() != nil:
		return m.GetImageMessage()
	case m.GetVideoMessage() != nil:
		return m.GetVideoMessage()
	case m.GetAudioMessage() != nil:
		return m.GetAudioMessage()
	case m.GetDocumentMessage() != nil:
		return m.GetDocumentMessage()
	case m.GetStickerMessage() != nil:
		return m.GetStickerMessage()
	default:
		return nil
	}
}

// mediaMetaOf devolve os campos p/ baixar a midia depois (nil se nao ha midia).
func mediaMetaOf(m *waProto.Message) map[string]any {
	p := mediaPart(m)
	if p == nil {
		return nil
	}
	out := map[string]any{
		"directPath":    p.GetDirectPath(),
		"mimetype":      p.GetMimetype(),
		"mediaKey":      p.GetMediaKey(),
		"fileSha256":    p.GetFileSHA256(),
		"fileEncSha256": p.GetFileEncSHA256(),
		"fileLength":    p.GetFileLength(),
	}
	if d := m.GetDocumentMessage(); d != nil && d.GetFileName() != "" {
		out["filename"] = d.GetFileName()
	}
	return out
}

// mediaMime extrai o mimetype da parte de midia da mensagem.
func mediaMime(m *waProto.Message) string {
	switch {
	case m == nil:
		return ""
	case m.GetImageMessage() != nil:
		return m.GetImageMessage().GetMimetype()
	case m.GetVideoMessage() != nil:
		return m.GetVideoMessage().GetMimetype()
	case m.GetAudioMessage() != nil:
		return m.GetAudioMessage().GetMimetype()
	case m.GetDocumentMessage() != nil:
		return m.GetDocumentMessage().GetMimetype()
	case m.GetStickerMessage() != nil:
		return m.GetStickerMessage().GetMimetype()
	default:
		return ""
	}
}

func classifyMessage(m *waProto.Message) (kind, body string) {
	switch {
	case m == nil:
		return "unknown", ""
	case m.GetConversation() != "":
		return "text", m.GetConversation()
	case m.GetExtendedTextMessage() != nil:
		return "text", m.GetExtendedTextMessage().GetText()
	case m.GetImageMessage() != nil:
		return "image", m.GetImageMessage().GetCaption()
	case m.GetVideoMessage() != nil:
		return "video", m.GetVideoMessage().GetCaption()
	case m.GetAudioMessage() != nil:
		return "audio", ""
	case m.GetDocumentMessage() != nil:
		return "document", m.GetDocumentMessage().GetCaption()
	case m.GetStickerMessage() != nil:
		return "sticker", ""
	case m.GetLocationMessage() != nil:
		return "location", m.GetLocationMessage().GetName()
	case m.GetLiveLocationMessage() != nil:
		return "live_location", ""
	case m.GetContactMessage() != nil:
		return "contact", m.GetContactMessage().GetDisplayName()
	case m.GetContactsArrayMessage() != nil:
		return "contacts", ""
	case m.GetReactionMessage() != nil:
		return "reaction", m.GetReactionMessage().GetText()
	case m.GetPollCreationMessage() != nil:
		return "poll", m.GetPollCreationMessage().GetName()
	case m.GetPollUpdateMessage() != nil:
		return "poll_vote", ""
	case m.GetButtonsResponseMessage() != nil:
		return "buttons_response", m.GetButtonsResponseMessage().GetSelectedDisplayText()
	case m.GetListResponseMessage() != nil:
		return "list_response", m.GetListResponseMessage().GetTitle()
	case m.GetTemplateButtonReplyMessage() != nil:
		return "template_reply", m.GetTemplateButtonReplyMessage().GetSelectedDisplayText()
	case m.GetProtocolMessage() != nil:
		return "protocol", ""
	default:
		return "unknown", ""
	}
}

func contextInfo(m *waProto.Message) *waProto.ContextInfo {
	if m == nil {
		return nil
	}
	if et := m.GetExtendedTextMessage(); et != nil {
		return et.GetContextInfo()
	}
	if im := m.GetImageMessage(); im != nil {
		return im.GetContextInfo()
	}
	if vm := m.GetVideoMessage(); vm != nil {
		return vm.GetContextInfo()
	}
	if dm := m.GetDocumentMessage(); dm != nil {
		return dm.GetContextInfo()
	}
	if am := m.GetAudioMessage(); am != nil {
		return am.GetContextInfo()
	}
	return nil
}

func jidStr(j types.JID) string {
	if j.IsEmpty() {
		return ""
	}
	return j.ToNonAD().String()
}

// preferPN devolve o JID de telefone (@s.whatsapp.net) quando o principal
// veio como @lid e existe uma alternativa.
func preferPN(main, alt types.JID) types.JID {
	if main.Server == types.HiddenUserServer && !alt.IsEmpty() {
		return alt
	}
	return main
}

func pickAlt(info types.MessageInfo) types.JID {
	if !info.SenderAlt.IsEmpty() {
		return info.SenderAlt
	}
	return info.RecipientAlt
}

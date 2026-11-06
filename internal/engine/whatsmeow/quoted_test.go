package whatsmeow

import (
	"testing"

	waProto "go.mau.fi/whatsmeow/proto/waE2E"
	waEvents "go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

func TestMensagemMarcada(t *testing.T) {
	audio := &waProto.Message{AudioMessage: &waProto.AudioMessage{
		DirectPath: proto.String("/caminho"), MediaKey: []byte("chave"), Mimetype: proto.String("audio/ogg"),
	}}
	evt := &waEvents.Message{Message: &waProto.Message{ExtendedTextMessage: &waProto.ExtendedTextMessage{
		Text: proto.String("escuta esse"),
		ContextInfo: &waProto.ContextInfo{
			StanzaID:      proto.String("ABC123"),
			Participant:   proto.String("5517999999999@s.whatsapp.net"),
			QuotedMessage: audio,
		},
	}}}

	out := normalizeMessage(evt, false)
	if out["quotedId"] != "ABC123" {
		t.Fatalf("quotedId = %v", out["quotedId"])
	}
	q, ok := out["quoted"].(map[string]any)
	if !ok {
		t.Fatalf("sem quoted: %v", out)
	}
	if q["id"] != "ABC123" || q["type"] != "audio" || q["participant"] != "5517999999999@s.whatsapp.net" {
		t.Errorf("quoted = %v", q)
	}
	mm, ok := q["mediaMeta"].(map[string]any)
	if !ok || mm["directPath"] != "/caminho" {
		t.Errorf("mediaMeta = %v", q["mediaMeta"])
	}
}

func TestMensagemMarcadaDeTexto(t *testing.T) {
	ci := &waProto.ContextInfo{
		StanzaID:      proto.String("XYZ"),
		QuotedMessage: &waProto.Message{Conversation: proto.String("Confirma a sexta?")},
	}
	q := quotedOf(ci)
	if q["type"] != "text" || q["body"] != "Confirma a sexta?" {
		t.Errorf("quoted = %v", q)
	}
	if _, tem := q["mediaMeta"]; tem {
		t.Errorf("texto nao tem mediaMeta: %v", q)
	}
}

func TestSemConteudoMarcado(t *testing.T) {
	if q := quotedOf(&waProto.ContextInfo{StanzaID: proto.String("XYZ")}); q != nil {
		t.Errorf("sem conteudo deveria ser nil: %v", q)
	}
}

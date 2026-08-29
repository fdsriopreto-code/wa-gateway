package whatsmeow

import (
	"testing"

	waProto "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

func TestClassifyMessage(t *testing.T) {
	cases := []struct {
		name     string
		msg      *waProto.Message
		wantKind string
		wantBody string
	}{
		{"nil", nil, "unknown", ""},
		{"conversation", &waProto.Message{Conversation: proto.String("Oi")}, "text", "Oi"},
		{"extended text", &waProto.Message{ExtendedTextMessage: &waProto.ExtendedTextMessage{Text: proto.String("ola")}}, "text", "ola"},
		{"image com caption", &waProto.Message{ImageMessage: &waProto.ImageMessage{Caption: proto.String("foto")}}, "image", "foto"},
		{"audio", &waProto.Message{AudioMessage: &waProto.AudioMessage{}}, "audio", ""},
		{"reaction", &waProto.Message{ReactionMessage: &waProto.ReactionMessage{Text: proto.String("👍")}}, "reaction", "👍"},
		{"poll", &waProto.Message{PollCreationMessage: &waProto.PollCreationMessage{Name: proto.String("Qual?")}}, "poll", "Qual?"},
		{"desconhecido", &waProto.Message{}, "unknown", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			k, b := classifyMessage(c.msg)
			if k != c.wantKind || b != c.wantBody {
				t.Errorf("got (%q,%q), want (%q,%q)", k, b, c.wantKind, c.wantBody)
			}
		})
	}
}

func TestPreferPN(t *testing.T) {
	lid := types.JID{User: "161761386868832", Server: types.HiddenUserServer}
	pn := types.JID{User: "5517996778746", Server: types.DefaultUserServer}

	if got := preferPN(lid, pn); got != pn {
		t.Errorf("lid+alt: got %s, want %s", got, pn)
	}
	if got := preferPN(pn, types.EmptyJID); got != pn {
		t.Errorf("pn sem alt: got %s, want %s", got, pn)
	}
	if got := preferPN(lid, types.EmptyJID); got != lid {
		t.Errorf("lid sem alt: deve manter lid, got %s", got)
	}
}

func TestJidStrEmpty(t *testing.T) {
	if jidStr(types.EmptyJID) != "" {
		t.Error("JID vazio deveria virar string vazia")
	}
}

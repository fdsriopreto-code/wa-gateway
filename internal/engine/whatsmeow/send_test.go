package whatsmeow

import (
	"strings"
	"testing"

	"wa-gateway/internal/engine"
)

func TestVCard(t *testing.T) {
	t.Run("monta a partir de nome/telefone", func(t *testing.T) {
		got := vcard(engine.Contact{Name: "Fulano", Phone: "+55 (11) 99999-9999"})
		for _, want := range []string{"BEGIN:VCARD", "FN:Fulano", "waid=5511999999999", "+5511999999999", "END:VCARD"} {
			if !strings.Contains(got, want) {
				t.Errorf("vcard sem %q:\n%s", want, got)
			}
		}
	})

	t.Run("usa o vcard fornecido", func(t *testing.T) {
		raw := "BEGIN:VCARD\nVERSION:3.0\nFN:Custom\nEND:VCARD"
		if got := vcard(engine.Contact{Name: "x", VCard: raw}); got != raw {
			t.Errorf("esperava passthrough, veio:\n%s", got)
		}
	})

	t.Run("sem telefone omite TEL", func(t *testing.T) {
		if got := vcard(engine.Contact{Name: "SoNome"}); strings.Contains(got, "TEL") {
			t.Errorf("nao esperava TEL:\n%s", got)
		}
	})
}

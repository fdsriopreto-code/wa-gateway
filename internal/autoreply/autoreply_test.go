package autoreply

import (
	"testing"
	"time"

	"wa-gateway/internal/session"
)

func TestIs1to1(t *testing.T) {
	yes := []string{"5517999@s.whatsapp.net", "111@c.us"}
	no := []string{"123-456@g.us", "status@broadcast", "", "abc"}
	for _, c := range yes {
		if !is1to1(c) {
			t.Errorf("%q devia ser 1:1", c)
		}
	}
	for _, c := range no {
		if is1to1(c) {
			t.Errorf("%q não devia ser 1:1", c)
		}
	}
}

func TestMessageText(t *testing.T) {
	if got := messageText(map[string]any{"body": "  oi  "}); got != "  oi  " {
		t.Errorf("body: %q", got)
	}
	if got := messageText(map[string]any{"body": "", "transcript": "áudio aqui"}); got != "áudio aqui" {
		t.Errorf("fallback transcript: %q", got)
	}
	if got := messageText(map[string]any{"type": "image"}); got != "" {
		t.Errorf("sem texto: %q", got)
	}
}

func TestParseHM(t *testing.T) {
	ok := map[string]int{"09:00": 540, "18:30": 1110, " 8:05 ": 485, "00:00": 0, "23:59": 1439}
	for in, want := range ok {
		if got, valid := parseHM(in); !valid || got != want {
			t.Errorf("parseHM(%q) = %d,%v want %d", in, got, valid, want)
		}
	}
	for _, bad := range []string{"", "9", "9h00", "25:00", "10:70", "abc:def"} {
		if _, valid := parseHM(bad); valid {
			t.Errorf("parseHM(%q) devia falhar", bad)
		}
	}
}

func TestWithinHours(t *testing.T) {
	h := &session.OfficeHours{TZ: "UTC", Start: "09:00", End: "18:00", Days: []int{1, 2, 3, 4, 5}}

	// Quarta-feira 12:00 UTC -> dentro
	wed := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	if !withinHours(h, wed) {
		t.Error("quarta 12:00 devia ser horário comercial")
	}
	// Quarta 20:00 -> fora
	if withinHours(h, time.Date(2026, 8, 26, 20, 0, 0, 0, time.UTC)) {
		t.Error("quarta 20:00 devia ser fora")
	}
	// Domingo 12:00 -> fora (dia não útil)
	if withinHours(h, time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)) {
		t.Error("domingo devia ser fora")
	}

	// Janela cruzando meia-noite: 22:00-06:00
	night := &session.OfficeHours{TZ: "UTC", Start: "22:00", End: "06:00", Days: []int{0, 1, 2, 3, 4, 5, 6}}
	if !withinHours(night, time.Date(2026, 8, 26, 23, 30, 0, 0, time.UTC)) {
		t.Error("23:30 devia cair na janela noturna")
	}
	if !withinHours(night, time.Date(2026, 8, 26, 3, 0, 0, 0, time.UTC)) {
		t.Error("03:00 devia cair na janela noturna")
	}
	if withinHours(night, time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)) {
		t.Error("12:00 não devia cair na janela noturna")
	}

	// Horário malformado -> conservador (dentro = não responde por "fora")
	bad := &session.OfficeHours{Start: "xx", End: "yy"}
	if !withinHours(bad, wed) {
		t.Error("horário inválido devia ser tratado como 'dentro'")
	}
}

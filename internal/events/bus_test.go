package events

import "testing"

func TestMatch(t *testing.T) {
	cases := []struct {
		pattern, name string
		want          bool
	}{
		{"*", "message", true},
		{"*", "anything.here", true},
		{"message", "message", true},
		{"message", "message.any", false},
		{"message.*", "message.any", true},
		{"message.*", "message.ack", true},
		{"message.*", "message", true}, // o namespace casa o nome base
		{"message.*", "session.status", false},
		{"session.*", "session.qr", true},
		{"group.*", "message.any", false},
	}
	for _, c := range cases {
		if got := Match(c.pattern, c.name); got != c.want {
			t.Errorf("Match(%q,%q) = %v, want %v", c.pattern, c.name, got, c.want)
		}
	}
}

func TestMatchAny(t *testing.T) {
	pats := []string{"session.*", "message.ack"}
	if !MatchAny(pats, "session.qr") {
		t.Error("session.qr deveria casar session.*")
	}
	if !MatchAny(pats, "message.ack") {
		t.Error("message.ack deveria casar exato")
	}
	if MatchAny(pats, "message.any") {
		t.Error("message.any NAO deveria casar")
	}
	if MatchAny(nil, "qualquer") {
		t.Error("lista vazia nao casa nada")
	}
}

func TestBusDeliversAndDrops(t *testing.T) {
	b := NewBus(nil)
	ch, cancel := b.Subscribe("t", "message.*", 2)
	defer cancel()

	b.Publish(Event{Name: "message.any"})
	b.Publish(Event{Name: "session.status"}) // filtrado
	b.Publish(Event{Name: "message.ack"})
	b.Publish(Event{Name: "message.any"}) // estoura o buffer (2) -> drop

	got := 0
	for len(ch) > 0 {
		<-ch
		got++
	}
	if got != 2 {
		t.Fatalf("recebeu %d eventos, esperava 2 (buffer cheio derruba o resto)", got)
	}
}

package events

import (
	"context"
	"errors"
	"testing"
	"time"
)

func pending(t *testing.T, s *Stream, group string) int64 {
	t.Helper()
	p, err := s.rdb.XPending(context.Background(), streamKey, group).Result()
	if err != nil {
		t.Fatal(err)
	}
	return p.Count
}

func TestPermanentErrorAcksImmediately(t *testing.T) {
	s, _ := newTestStream(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Consume(ctx, "leads", "n1", []string{"*"}, 1, func(_ context.Context, e Event) error {
		return Permanent(errors.New("FK: sessão apagada"))
	})
	waitForGroup(t, s, "leads")
	s.Append(Event{ID: "p", Name: "message.any", Timestamp: time.Now()})
	waitFor(t, 2*time.Second, func() bool {
		n, _ := s.rdb.XLen(context.Background(), streamKey).Result()
		return n == 1 && pending(t, s, "leads") == 0
	})
}

func TestRepeatedFailureGivesUp(t *testing.T) {
	s, _ := newTestStream(t)
	ctx := context.Background()
	if err := s.rdb.XGroupCreateMkStream(ctx, streamKey, "g", "$").Err(); err != nil {
		t.Fatal(err)
	}
	transient := errors.New("postgres fora")
	for i := 1; i < maxFailures; i++ {
		if s.giveUp(ctx, "g", "1-0", transient) {
			t.Fatalf("desistiu cedo demais (tentativa %d)", i)
		}
	}
	if !s.giveUp(ctx, "g", "1-0", transient) {
		t.Fatalf("deveria desistir na tentativa %d", maxFailures)
	}
	if s.giveUp(ctx, "g", "2-0", transient) {
		t.Fatal("contador é por evento: outro evento não herda as falhas")
	}
	if !s.giveUp(ctx, "g", "3-0", Permanent(transient)) {
		t.Fatal("falha permanente desiste na primeira")
	}
	if !errors.Is(Permanent(transient), transient) {
		t.Fatal("Permanent deve preservar o erro original")
	}
}

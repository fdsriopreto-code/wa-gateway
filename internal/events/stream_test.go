package events

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestStream(t *testing.T) (*Stream, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return NewStream(rdb, nil), mr
}

func TestStreamAppendAndConsume(t *testing.T) {
	s, _ := newTestStream(t)

	var got []string
	var mu sync.Mutex
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go s.Consume(ctx, "webhook", "n1", []string{"message.*"}, 2, func(_ context.Context, e Event) error {
		mu.Lock()
		got = append(got, e.Name)
		mu.Unlock()
		return nil
	})
	waitForGroup(t, s, "webhook") // grupo criado com "$": só pega o que vier depois

	s.Append(Event{ID: "1", Name: "message.any", Timestamp: time.Now()})
	s.Append(Event{ID: "2", Name: "session.status", Timestamp: time.Now()}) // filtrado
	s.Append(Event{ID: "3", Name: "message.ack", Timestamp: time.Now()})

	waitFor(t, 2*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(got) == 2
	})
	mu.Lock()
	defer mu.Unlock()
	if got[0] != "message.any" || got[1] != "message.ack" {
		t.Fatalf("recebeu %v", got)
	}
}

func TestStreamHandlerErrorKeepsPending(t *testing.T) {
	s, _ := newTestStream(t)
	var attempts atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go s.Consume(ctx, "inbox", "n1", []string{"*"}, 1, func(_ context.Context, e Event) error {
		if attempts.Add(1) < 3 {
			return context.DeadlineExceeded // falha as 2 primeiras
		}
		return nil
	})
	waitForGroup(t, s, "inbox")

	s.Append(Event{ID: "x", Name: "message.any", Timestamp: time.Now()})

	// não deve dar XACK enquanto falha -> continua pendente e é reprocessado
	// via claimStale (min-idle é 60s, então forçamos pelo re-XREADGROUP de
	// mensagens já entregues ao mesmo consumidor não acontece; validamos que
	// pelo menos processou 1x e ficou pendente).
	waitFor(t, 2*time.Second, func() bool { return attempts.Load() >= 1 })

	// pendência visível no PEL
	waitFor(t, 2*time.Second, func() bool {
		p := s.rdb.XPending(ctx, streamKey, "inbox").Val()
		return p != nil && p.Count == 1
	})
}

func waitForGroup(t *testing.T, s *Stream, group string) {
	t.Helper()
	waitFor(t, 2*time.Second, func() bool {
		gs, err := s.rdb.XInfoGroups(context.Background(), streamKey).Result()
		if err != nil {
			return false
		}
		for _, g := range gs {
			if g.Name == group {
				return true
			}
		}
		return false
	})
}

func waitFor(t *testing.T, d time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condição não satisfeita no prazo")
}

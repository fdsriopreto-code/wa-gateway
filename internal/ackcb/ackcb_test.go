package ackcb

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"wa-gateway/internal/cache"
	"wa-gateway/internal/events"
)

func newStore(t *testing.T) (*Store, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mr.Close)
	rc, err := cache.New(context.Background(), "redis://"+mr.Addr())
	if err != nil {
		t.Fatal(err)
	}
	return New(rc, slog.New(slog.NewTextHandler(io.Discard, nil)), "n1"), mr
}

func TestArmAndFire(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()

	var (
		mu   sync.Mutex
		got  map[string]any
		hits int
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		hits++
		_ = json.Unmarshal(b, &got)
		mu.Unlock()
		w.WriteHeader(200)
	}))
	defer srv.Close()

	s.Arm(ctx, "sess", "MID1", srv.URL, json.RawMessage(`{"row":42}`), time.Minute)

	// ack "delivered" pra MID1 -> dispara
	err := s.handle(ctx, events.Event{
		Session: "sess", Name: events.MessageAck,
		Payload: map[string]any{"ids": []any{"MID1"}, "chatId": "551199@s.whatsapp.net", "type": "delivered", "timestamp": float64(123)},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { mu.Lock(); defer mu.Unlock(); return hits == 1 })

	mu.Lock()
	if got["messageId"] != "MID1" || got["status"] != "delivered" || got["to"] != "551199@s.whatsapp.net" {
		t.Fatalf("payload errado: %v", got)
	}
	if d, _ := got["data"].(map[string]any); d["row"] != float64(42) {
		t.Fatalf("data não ecoou: %v", got["data"])
	}
	mu.Unlock()

	// segundo ack ("read") não deve disparar de novo — já desarmou
	_ = s.handle(ctx, events.Event{
		Session: "sess", Name: events.MessageAck,
		Payload: map[string]any{"ids": []any{"MID1"}, "type": "read"},
	})
	time.Sleep(150 * time.Millisecond)
	mu.Lock()
	if hits != 1 {
		t.Fatalf("disparou %d vezes, esperava 1", hits)
	}
	mu.Unlock()
}

func TestIgnoresNonTerminalAndUnarmed(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true }))
	defer srv.Close()

	s.Arm(ctx, "sess", "MID1", srv.URL, nil, time.Minute)
	// "sender" não é terminal
	_ = s.handle(ctx, events.Event{Session: "sess", Name: events.MessageAck,
		Payload: map[string]any{"ids": []any{"MID1"}, "type": "sender"}})
	// mensagem sem callback armado
	_ = s.handle(ctx, events.Event{Session: "sess", Name: events.MessageAck,
		Payload: map[string]any{"ids": []any{"OUTRA"}, "type": "delivered"}})
	time.Sleep(120 * time.Millisecond)
	if hit {
		t.Fatal("não devia ter chamado o callback")
	}
}

func TestCloudStyleSingleID(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()
	done := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { done <- struct{}{} }))
	defer srv.Close()

	s.Arm(ctx, "cloud", "wamid.X", srv.URL, nil, time.Minute)
	_ = s.handle(ctx, events.Event{Session: "cloud", Name: events.MessageAck,
		Payload: map[string]any{"id": "wamid.X", "type": "failed"}})
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("callback não chegou (formato cloud id:)")
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	for i := 0; i < 50; i++ {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timeout esperando condição")
}

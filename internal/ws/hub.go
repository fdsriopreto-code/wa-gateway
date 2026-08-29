// Package ws entrega eventos em tempo real por WebSocket. Best-effort:
// sem fila, sem retry; cliente lento perde mensagens (contador em /metrics).
package ws

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"wa-gateway/internal/events"
	"wa-gateway/internal/observability"
)

type client struct {
	session string
	filters []string
	send    chan []byte
}

type Hub struct {
	log *slog.Logger
	mu  sync.RWMutex
	set map[*client]struct{}
}

func NewHub(log *slog.Logger) *Hub {
	return &Hub{log: log, set: make(map[*client]struct{})}
}

// Run consome o barramento e faz fan-out para os clientes conectados.
func (h *Hub) Run(ctx context.Context, bus *events.Bus) {
	ch, cancel := bus.Subscribe("ws", "*", 8192)
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-ch:
			if !ok {
				return
			}
			h.broadcast(e)
		}
	}
}

func (h *Hub) broadcast(e events.Event) {
	msg, err := json.Marshal(map[string]any{
		"id":        e.ID,
		"session":   e.Session,
		"event":     e.Name,
		"engine":    e.Engine,
		"timestamp": e.Timestamp.UnixMilli(),
		"payload":   e.Payload,
	})
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.set {
		if c.session != "*" && c.session != e.Session {
			continue
		}
		if len(c.filters) > 0 && !events.MatchAny(c.filters, e.Name) {
			continue
		}
		select {
		case c.send <- msg:
		default:
			observability.BusDropped.WithLabelValues("ws-client").Inc()
		}
	}
}

// Handler e o http.HandlerFunc de GET /ws?session=<s>&events=<a,b>.
func (h *Hub) Handler(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	sess := r.URL.Query().Get("session")
	if sess == "" {
		sess = "*"
	}
	var filters []string
	if raw := r.URL.Query().Get("events"); raw != "" {
		for _, p := range strings.Split(raw, ",") {
			if p = strings.TrimSpace(p); p != "" {
				filters = append(filters, p)
			}
		}
	}

	c := &client{session: sess, filters: filters, send: make(chan []byte, 256)}
	h.mu.Lock()
	h.set[c] = struct{}{}
	h.mu.Unlock()
	observability.WSClients.Inc()

	defer func() {
		h.mu.Lock()
		delete(h.set, c)
		h.mu.Unlock()
		observability.WSClients.Dec()
	}()

	ctx := conn.CloseRead(r.Context()) // descarta frames do cliente, detecta close

	ping := time.NewTicker(45 * time.Second)
	defer ping.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ping.C:
			if err := conn.Ping(ctx); err != nil {
				return
			}
		case msg := <-c.send:
			wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := conn.Write(wctx, websocket.MessageText, msg)
			cancel()
			if err != nil {
				return
			}
		}
	}
}

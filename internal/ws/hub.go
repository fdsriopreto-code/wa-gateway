// Package ws entrega eventos em tempo real por WebSocket. Best-effort:
// sem fila, sem retry; cliente lento perde mensagens (contador em /metrics).
//
// Multi-no: cada no publica seus eventos locais num canal Redis; os outros
// nos reentregam para os clientes deles. Como uma sessao roda em exatamente
// um no (lock de posse), nao ha duplicidade.
package ws

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"wa-gateway/internal/cache"
	"wa-gateway/internal/events"
	"wa-gateway/internal/observability"
)

const (
	redisChannel = "wa:ws"
	nodesKey     = "wa:ws:nodes"
)

type client struct {
	session string
	filters []string
	send    chan []byte
}

type Hub struct {
	log      *slog.Logger
	redis    *cache.Redis
	nodeID   string
	mu       sync.RWMutex
	set      map[*client]struct{}
	hasPeers atomic.Bool // true quando há >1 nó vivo (aí vale publicar no Redis)
}

func NewHub(log *slog.Logger, redis *cache.Redis, nodeID string) *Hub {
	return &Hub{log: log, redis: redis, nodeID: nodeID, set: make(map[*client]struct{})}
}

// Run consome o barramento local (fan-out + publish no Redis) e assina o
// canal Redis (reentrega eventos de outros nos).
func (h *Hub) Run(ctx context.Context, bus *events.Bus) {
	if h.redis != nil {
		go h.consumeRedis(ctx)
		go h.heartbeat(ctx)
	}
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
			h.onLocal(e)
		}
	}
}

func (h *Hub) onLocal(e events.Event) {
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
	h.deliver(e.Session, e.Name, msg)
	// só propaga entre nós se houver outro nó vivo — num deploy de 1 réplica
	// (o caso comum) isso zera o tráfego pub/sub à toa.
	if h.redis != nil && h.hasPeers.Load() {
		framed := append([]byte(h.nodeID), '\x00')
		framed = append(framed, msg...)
		_ = h.redis.Publish(context.Background(), redisChannel, framed)
	}
}

// heartbeat marca este nó como vivo e atualiza hasPeers (a cada 8s).
func (h *Hub) heartbeat(ctx context.Context) {
	t := time.NewTicker(8 * time.Second)
	defer t.Stop()
	tick := func() {
		n, err := h.redis.Heartbeat(ctx, nodesKey, h.nodeID, 25*time.Second)
		if err == nil {
			h.hasPeers.Store(n > 1)
		}
	}
	tick()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			tick()
		}
	}
}

func (h *Hub) consumeRedis(ctx context.Context) {
	sub := h.redis.Subscribe(ctx, redisChannel)
	defer sub.Close()
	for {
		m, err := sub.ReceiveMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			time.Sleep(time.Second)
			continue
		}
		raw := []byte(m.Payload)
		i := bytes.IndexByte(raw, '\x00')
		if i < 0 {
			continue
		}
		if string(raw[:i]) == h.nodeID {
			continue // meu proprio evento, ja entregue localmente
		}
		body := raw[i+1:]
		var meta struct {
			Session string `json:"session"`
			Event   string `json:"event"`
		}
		if json.Unmarshal(body, &meta) != nil {
			continue
		}
		h.deliver(meta.Session, meta.Event, append([]byte(nil), body...))
	}
}

// deliver aplica os filtros de cada cliente e envia a mensagem ja pronta.
func (h *Hub) deliver(session, name string, msg []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.set {
		if c.session != "*" && c.session != session {
			continue
		}
		if len(c.filters) > 0 && !events.MatchAny(c.filters, name) {
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

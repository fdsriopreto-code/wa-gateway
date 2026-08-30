// Package ackcb entrega um "callback de status" por mensagem: quem envia passa
// um callbackUrl e, quando o WhatsApp confirma a entrega (ou falha), o gateway
// faz um POST nessa URL — igual ao StatusCallback do Twilio. Bom pra fluxos
// transacionais (OTP, recibo) onde você quer saber de UMA mensagem sem assinar
// a torrente de message.ack.
package ackcb

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"wa-gateway/internal/cache"
	"wa-gateway/internal/events"
)

const armTTL = 15 * time.Minute

// terminais: o 1º desses que chegar dispara o callback (e desarma).
var terminal = map[string]bool{"delivered": true, "read": true, "played": true, "failed": true, "error": true}

type Store struct {
	rc     *cache.Redis
	http   *http.Client
	log    *slog.Logger
	nodeID string
}

func New(rc *cache.Redis, log *slog.Logger, nodeID string) *Store {
	return &Store{
		rc:     rc,
		http:   &http.Client{Timeout: 10 * time.Second},
		log:    log.With("comp", "ackcb"),
		nodeID: nodeID,
	}
}

func key(session, msgID string) string { return "wa:ackcb:" + session + ":" + msgID }

// Arm registra o callback pra um messageId recém-enviado. url vazio = no-op.
func (s *Store) Arm(ctx context.Context, session, msgID, url string, data json.RawMessage, ttl time.Duration) {
	if s == nil || url == "" || msgID == "" {
		return
	}
	if ttl <= 0 {
		ttl = armTTL
	}
	if len(data) == 0 {
		data = json.RawMessage("null")
	}
	pipe := s.rc.Raw().TxPipeline()
	pipe.HSet(ctx, key(session, msgID), map[string]any{"url": url, "data": string(data)})
	pipe.Expire(ctx, key(session, msgID), ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		s.log.Warn("arm falhou", "session", session, "msg", msgID, "err", err)
	}
}

// Run liga o consumidor. Bloqueia até ctx acabar.
func (s *Store) Run(ctx context.Context, stream *events.Stream) {
	if stream == nil {
		return
	}
	stream.Consume(ctx, "ackcb", s.nodeID, []string{events.MessageAck}, 4, s.handle)
}

func (s *Store) handle(ctx context.Context, e events.Event) error {
	if e.Name != events.MessageAck {
		return nil
	}
	p, _ := e.Payload.(map[string]any)
	if p == nil {
		return nil
	}
	status := strings.ToLower(str(p["type"]))
	if !terminal[status] {
		return nil
	}
	to := str(p["chatId"])
	ts := p["timestamp"]

	for _, id := range messageIDs(p) {
		k := key(e.Session, id)
		h, err := s.rc.Raw().HGetAll(ctx, k).Result()
		if err != nil || h["url"] == "" {
			continue
		}
		// claim atômico: só quem consegue apagar dispara (evita 2x entre nós /
		// entre delivered+read).
		if n, _ := s.rc.Raw().Del(ctx, k).Result(); n == 0 {
			continue
		}
		body, _ := json.Marshal(map[string]any{
			"messageId": id,
			"session":   e.Session,
			"to":        to,
			"status":    status,
			"timestamp": ts,
			"data":      json.RawMessage(orNull(h["data"])),
		})
		go s.post(h["url"], body, e.Session, id)
	}
	return nil
}

func (s *Store) post(url string, body []byte, session, id string) {
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "wa-gateway-ackcb/1")
		resp, err := s.http.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode < 300 {
				return
			}
			lastErr = &httpStatusErr{resp.StatusCode}
		} else {
			lastErr = err
		}
		time.Sleep(time.Duration(attempt) * 2 * time.Second)
	}
	s.log.Warn("callback de status falhou", "session", session, "msg", id, "url", redact(url), "err", lastErr)
}

type httpStatusErr struct{ code int }

func (e *httpStatusErr) Error() string { return "http " + strconv.Itoa(e.code) }

/* ---- helpers ---- */

func str(v any) string { s, _ := v.(string); return s }

func orNull(s string) string {
	if s == "" {
		return "null"
	}
	return s
}

// messageIDs cobre whatsmeow (ids: []) e cloud (id: "").
func messageIDs(p map[string]any) []string {
	var out []string
	switch v := p["ids"].(type) {
	case []any:
		for _, x := range v {
			if s, ok := x.(string); ok && s != "" {
				out = append(out, s)
			}
		}
	case []string:
		out = append(out, v...)
	}
	if s := str(p["id"]); s != "" {
		out = append(out, s)
	}
	return out
}

func redact(u string) string {
	if i := strings.IndexByte(u, '?'); i >= 0 {
		return u[:i] + "?…"
	}
	return u
}

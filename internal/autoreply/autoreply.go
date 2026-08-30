// Package autoreply responde automaticamente mensagens 1:1 recebidas, segundo
// config.autoReply da sessão (saudação 1x por contato, regras por palavra-chave,
// fallback, e opção de só responder fora do horário comercial). Consome o log
// durável de eventos (Redis Stream) via consumer group, então vale para os dois
// motores e sobrevive a restart.
package autoreply

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/hibiken/asynq"

	"wa-gateway/internal/cache"
	"wa-gateway/internal/engine"
	"wa-gateway/internal/events"
	"wa-gateway/internal/outbox"
	"wa-gateway/internal/session"
)

type Responder struct {
	mgr    *session.Manager
	queue  *outbox.Queue // preferido (paced + cluster-safe); pode ser nil
	rc     *cache.Redis
	log    *slog.Logger
	nodeID string
}

func New(mgr *session.Manager, q *outbox.Queue, rc *cache.Redis, log *slog.Logger, nodeID string) *Responder {
	return &Responder{mgr: mgr, queue: q, rc: rc, log: log.With("comp", "autoreply"), nodeID: nodeID}
}

// Run liga o consumidor. Bloqueia até ctx acabar.
func (r *Responder) Run(ctx context.Context, stream *events.Stream) {
	if stream == nil {
		return
	}
	stream.Consume(ctx, "autoreply", r.nodeID, []string{events.Message}, 4, r.handle)
}

func (r *Responder) handle(ctx context.Context, e events.Event) error {
	if e.Name != events.Message || e.Session == "" {
		return nil
	}
	cfg := r.mgr.AutoReplyConfig(e.Session)
	if cfg == nil || !cfg.Enabled {
		return nil
	}
	p, _ := e.Payload.(map[string]any)
	if p == nil {
		return nil
	}
	if fm, _ := p["fromMe"].(bool); fm {
		return nil
	}
	chat, _ := p["chatId"].(string)
	if chat == "" || !is1to1(chat) {
		return nil // grupos, broadcast, status: fora do escopo
	}

	// anti ping-pong: no máximo 1 auto-resposta por contato a cada 30s.
	if ok, _ := r.rc.Raw().SetNX(ctx, "wa:autoreply:rl:"+e.Session+":"+chat, "1", 30*time.Second).Result(); !ok {
		return nil
	}

	if cfg.OnlyOutsideHours && cfg.Hours != nil && withinHours(cfg.Hours, time.Now()) {
		return nil
	}

	text := messageText(p)
	reply := r.pick(ctx, cfg, e.Session, chat, text)
	if reply == "" {
		return nil
	}
	if err := r.send(ctx, e.Session, chat, reply); err != nil {
		r.log.Warn("falha ao enviar auto-resposta", "session", e.Session, "chat", chat, "err", err)
	}
	return nil
}

// pick decide o texto da resposta: saudação (1x/contato) > regra por
// palavra-chave > fallback.
func (r *Responder) pick(ctx context.Context, cfg *session.AutoReplyConfig, sess, chat, text string) string {
	if cfg.Greeting != "" {
		cd := time.Duration(cfg.GreetingCooldownH) * time.Hour
		if cd <= 0 {
			cd = 24 * time.Hour
		}
		if ok, _ := r.rc.Raw().SetNX(ctx, "wa:autoreply:greeted:"+sess+":"+chat, "1", cd).Result(); ok {
			return cfg.Greeting
		}
	}
	low := strings.ToLower(text)
	if low != "" {
		for _, rule := range cfg.Rules {
			for _, kw := range rule.Contains {
				kw = strings.ToLower(strings.TrimSpace(kw))
				if kw != "" && strings.Contains(low, kw) {
					return rule.Reply
				}
			}
		}
	}
	return cfg.Fallback
}

func (r *Responder) send(ctx context.Context, sess, chat, text string) error {
	if r.queue != nil {
		_, err := r.queue.Enqueue(ctx, outbox.Job{
			ID:      "autoreply:" + sess + ":" + chat + ":" + time.Now().UTC().Format("20060102T150405"),
			Session: sess,
			Kind:    outbox.KindText,
			Args:    outbox.Args{ChatID: chat, Text: text},
		}, nil, 0)
		if err == asynq.ErrDuplicateTask || err == asynq.ErrTaskIDConflict {
			return nil
		}
		return err
	}
	eng, ok := r.mgr.Engine(sess)
	if !ok {
		return nil // sessão não vive aqui e não há fila; deixa passar
	}
	_, err := eng.SendText(ctx, chat, text, engine.MessageOpts{})
	return err
}

func is1to1(chat string) bool {
	return strings.HasSuffix(chat, "@s.whatsapp.net") || strings.HasSuffix(chat, "@c.us")
}

func messageText(p map[string]any) string {
	for _, k := range []string{"body", "text", "caption", "transcript", "imageCaption"} {
		if v, _ := p[k].(string); strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// withinHours diz se `now` cai dentro do horário comercial (TZ, start-end,
// dias). Regras malformadas => trata como "dentro" (conservador: não responde
// achando que é fora).
func withinHours(h *session.OfficeHours, now time.Time) bool {
	loc := time.UTC
	if h.TZ != "" {
		if l, err := time.LoadLocation(h.TZ); err == nil {
			loc = l
		}
	}
	now = now.In(loc)

	days := h.Days
	if len(days) == 0 {
		days = []int{1, 2, 3, 4, 5} // seg-sex
	}
	today := int(now.Weekday())
	work := false
	for _, d := range days {
		if d == today {
			work = true
			break
		}
	}
	if !work {
		return false
	}

	start, ok1 := parseHM(h.Start)
	end, ok2 := parseHM(h.End)
	if !ok1 || !ok2 {
		return true
	}
	mins := now.Hour()*60 + now.Minute()
	if start <= end {
		return mins >= start && mins < end
	}
	// janela que cruza a meia-noite (ex.: 22:00-06:00)
	return mins >= start || mins < end
}

func parseHM(s string) (int, bool) {
	s = strings.TrimSpace(s)
	i := strings.IndexByte(s, ':')
	if i < 0 {
		return 0, false
	}
	h, e1 := strconv.Atoi(strings.TrimSpace(s[:i]))
	m, e2 := strconv.Atoi(strings.TrimSpace(s[i+1:]))
	if e1 != nil || e2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

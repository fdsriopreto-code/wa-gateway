// Package webhook consome o barramento interno, monta o envelope e entrega
// via fila duravel (asynq/Redis) com HMAC e retry exponencial.
package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/hibiken/asynq"

	"wa-gateway/internal/events"
	"wa-gateway/internal/observability"
	"wa-gateway/internal/session"
	"wa-gateway/internal/store"
)

const TaskDeliver = "webhook:deliver"

type deliverPayload struct {
	DeliveryID string            `json:"delivery_id"`
	Session    string            `json:"session"`
	Event      string            `json:"event"`
	URL        string            `json:"url"`
	Secret     string            `json:"secret"`
	Headers    map[string]string `json:"headers"`
	Body       json.RawMessage   `json:"body"`
}

type Dispatcher struct {
	store       *store.Store
	client      *asynq.Client
	log         *slog.Logger
	http        *http.Client
	maxAttempts int
}

func NewDispatcher(st *store.Store, client *asynq.Client, log *slog.Logger, timeout time.Duration, maxAttempts int) *Dispatcher {
	if maxAttempts <= 0 {
		maxAttempts = 15
	}
	tr := &http.Transport{
		MaxIdleConns:        200,
		MaxIdleConnsPerHost: 32,
		IdleConnTimeout:     90 * time.Second,
	}
	return &Dispatcher{
		store: st, client: client, log: log,
		http:        &http.Client{Timeout: timeout, Transport: tr},
		maxAttempts: maxAttempts,
	}
}

// Run consome o barramento ate o contexto ser cancelado.
func (d *Dispatcher) Run(ctx context.Context, bus *events.Bus) {
	ch, cancel := bus.Subscribe("webhook", "*", 16384)
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-ch:
			if !ok {
				return
			}
			d.handle(ctx, e)
		}
	}
}

func (d *Dispatcher) handle(ctx context.Context, e events.Event) {
	rec, err := d.store.GetSession(ctx, e.Session)
	if err != nil {
		return
	}
	cfg, err := session.ParseConfig(rec.Config)
	if err != nil {
		d.log.Warn("config de sessao invalida", "session", e.Session, "err", err)
		return
	}
	for _, wh := range cfg.Webhooks {
		if wh.URL == "" || !events.MatchAny(wh.Events, e.Name) {
			continue
		}
		d.enqueue(ctx, e, cfg.Metadata, wh)
	}
}

func (d *Dispatcher) enqueue(ctx context.Context, e events.Event, metadata map[string]string, wh session.WebhookConfig) {
	body, err := json.Marshal(NewEnvelope(e, metadata))
	if err != nil {
		d.log.Error("marshal envelope", "err", err)
		return
	}
	secret := ""
	if wh.HMAC != nil {
		secret = wh.HMAC.Secret
	}
	attempts := d.maxAttempts
	backoff := 5 * time.Second
	if wh.Retries != nil {
		if wh.Retries.Attempts > 0 {
			attempts = wh.Retries.Attempts
		}
		if wh.Retries.Backoff != "" {
			if b, err := time.ParseDuration(wh.Retries.Backoff); err == nil {
				backoff = b
			}
		}
	}

	p := deliverPayload{
		DeliveryID: e.ID, Session: e.Session, Event: e.Name,
		URL: wh.URL, Secret: secret, Headers: wh.Headers, Body: body,
	}
	raw, _ := json.Marshal(p)

	_ = d.store.CreateDelivery(ctx, e.ID, e.Session, wh.URL, e.Name)

	task := asynq.NewTask(TaskDeliver, raw,
		asynq.MaxRetry(attempts),
		asynq.Timeout(d.http.Timeout+5*time.Second),
		asynq.Queue("webhook"),
		asynq.Retention(24*time.Hour),
	)
	if _, err := d.client.EnqueueContext(ctx, task, asynq.TaskID(e.ID+"|"+shortURL(wh.URL))); err != nil {
		// TaskID duplicado = evento ja enfileirado; ignora.
		if err != asynq.ErrDuplicateTask && err != asynq.ErrTaskIDConflict {
			d.log.Error("enqueue webhook", "err", err)
		}
	}
	_ = backoff // reservado: politica custom de backoff por webhook
}

// Handler processa a task de entrega (roda nos workers asynq).
func (d *Dispatcher) Handler() asynq.HandlerFunc {
	return func(ctx context.Context, t *asynq.Task) error {
		var p deliverPayload
		if err := json.Unmarshal(t.Payload(), &p); err != nil {
			return fmt.Errorf("payload invalido: %w", asynq.SkipRetry)
		}
		attempt, _ := asynq.GetRetryCount(ctx)
		attempt++

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.URL, bytes.NewReader(p.Body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "wa-gateway/webhook")
		req.Header.Set("X-Webhook-Id", p.DeliveryID)
		req.Header.Set("X-Webhook-Timestamp", fmt.Sprintf("%d", time.Now().UnixMilli()))
		if p.Secret != "" {
			req.Header.Set("X-Webhook-Signature", Sign(p.Secret, p.Body))
		}
		for k, v := range p.Headers {
			req.Header.Set(k, v)
		}

		resp, err := d.http.Do(req)
		if err != nil {
			_ = d.store.MarkFailed(ctx, p.DeliveryID, attempt, 0, err.Error())
			observability.WebhookDeliveries.WithLabelValues("error").Inc()
			return err
		}
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			_ = d.store.MarkDelivered(ctx, p.DeliveryID, attempt, resp.StatusCode)
			observability.WebhookDeliveries.WithLabelValues("delivered").Inc()
			return nil
		}
		_ = d.store.MarkFailed(ctx, p.DeliveryID, attempt, resp.StatusCode, fmt.Sprintf("status %d", resp.StatusCode))
		observability.WebhookDeliveries.WithLabelValues("retry").Inc()
		return fmt.Errorf("webhook respondeu %d", resp.StatusCode)
	}
}

func shortURL(u string) string {
	if len(u) > 64 {
		return u[:64]
	}
	return u
}

// Package webhook consome o barramento interno, monta o envelope e entrega
// via fila duravel (asynq/Redis) com HMAC e retry exponencial.
package webhook

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/hibiken/asynq"

	"wa-gateway/internal/events"
	"wa-gateway/internal/observability"
	"wa-gateway/internal/session"
	"wa-gateway/internal/store"
)

const TaskDeliver = "webhook:deliver"

type deliverPayload struct {
	DeliveryID string            `json:"delivery_id"` // <eventID>|<urlKey> — PK único por (evento, URL)
	EventID    string            `json:"event_id"`
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
	nodeID      string
	cfgCache    sync.Map // session -> cachedCfg
}

func NewDispatcher(st *store.Store, client *asynq.Client, log *slog.Logger, timeout time.Duration, maxAttempts int, nodeID string) *Dispatcher {
	if maxAttempts <= 0 {
		maxAttempts = 15
	}
	tr := &http.Transport{
		MaxIdleConns:        200,
		MaxIdleConnsPerHost: 32,
		IdleConnTimeout:     90 * time.Second,
	}
	return &Dispatcher{
		store: st, client: client, log: log, nodeID: nodeID,
		http:        &http.Client{Timeout: timeout, Transport: tr},
		maxAttempts: maxAttempts,
	}
}

// Run consome o log duravel de eventos (Redis Stream, consumer group
// "webhook") ate o contexto ser cancelado. Cada evento e processado uma vez
// pelo cluster; XACK so no sucesso, entao crash no meio nao perde entrega.
func (d *Dispatcher) Run(ctx context.Context, stream *events.Stream) {
	stream.Consume(ctx, "webhook", d.nodeID, []string{"*"}, 8, d.handle)
}

type cachedCfg struct {
	webhooks []session.WebhookConfig
	metadata map[string]string
	exp      time.Time
}

// sessionCfg le a config da sessao com cache curto (5s) — evita um SELECT
// no Postgres por evento.
func (d *Dispatcher) sessionCfg(ctx context.Context, name string) ([]session.WebhookConfig, map[string]string, bool) {
	if v, ok := d.cfgCache.Load(name); ok {
		c := v.(cachedCfg)
		if time.Now().Before(c.exp) {
			return c.webhooks, c.metadata, true
		}
	}
	rec, err := d.store.GetSession(ctx, name)
	if err != nil {
		return nil, nil, false
	}
	cfg, err := session.ParseConfig(rec.Config)
	if err != nil {
		d.log.Warn("config de sessao invalida", "session", name, "err", err)
		return nil, nil, false
	}
	d.cfgCache.Store(name, cachedCfg{webhooks: cfg.Webhooks, metadata: cfg.Metadata, exp: time.Now().Add(5 * time.Second)})
	return cfg.Webhooks, cfg.Metadata, true
}

func (d *Dispatcher) handle(ctx context.Context, e events.Event) error {
	webhooks, metadata, ok := d.sessionCfg(ctx, e.Session)
	if !ok {
		return nil // sessao sumiu / config invalida: nada a entregar, ack
	}
	// dedupe por destino: se a sessao tiver duas entradas apontando pro mesmo
	// URL (ex.: URL de teste e de producao do n8n, ou entrada duplicada), o
	// evento sai uma vez so por URL.
	seen := make(map[string]struct{}, len(webhooks))
	var firstErr error
	for _, wh := range webhooks {
		if wh.URL == "" || !events.MatchAny(wh.Events, e.Name) {
			continue
		}
		norm := strings.TrimRight(wh.URL, "/")
		if _, dup := seen[norm]; dup {
			continue
		}
		seen[norm] = struct{}{}
		if err := d.enqueue(ctx, e, metadata, wh); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr // != nil -> evento fica pendente no stream -> reprocessa
}

func (d *Dispatcher) enqueue(ctx context.Context, e events.Event, metadata map[string]string, wh session.WebhookConfig) error {
	body, err := json.Marshal(NewEnvelope(e, metadata))
	if err != nil {
		d.log.Error("marshal envelope", "err", err)
		return nil // payload que nao serializa nao melhora com retry
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

	// PK único por (evento, URL) — o mesmo id vira o TaskID do asynq, então
	// reprocessar o evento no stream não duplica entrega.
	deliveryID := e.ID + "|" + urlKey(wh.URL)
	p := deliverPayload{
		DeliveryID: deliveryID, EventID: e.ID, Session: e.Session, Event: e.Name,
		URL: wh.URL, Secret: secret, Headers: wh.Headers, Body: body,
	}
	raw, _ := json.Marshal(p)

	_ = d.store.CreateDelivery(ctx, deliveryID, e.Session, wh.URL, e.Name, raw)

	task := asynq.NewTask(TaskDeliver, raw,
		asynq.MaxRetry(attempts),
		asynq.Timeout(d.http.Timeout+5*time.Second),
		asynq.Queue("webhook"),
		asynq.Retention(24*time.Hour),
	)
	_ = backoff // reservado: politica custom de backoff por webhook
	if _, err := d.client.EnqueueContext(ctx, task, asynq.TaskID(deliveryID)); err != nil {
		if err == asynq.ErrDuplicateTask || err == asynq.ErrTaskIDConflict {
			return nil // ja enfileirado numa passada anterior
		}
		d.log.Error("enqueue webhook", "err", err)
		return err // transitorio -> reprocessa o evento
	}
	return nil
}

// Retry reenfileira uma entrega já registrada (usa o payload guardado).
// Enfileira SEM TaskID fixo pra não colidir com o dedupe do dispatch normal.
func (d *Dispatcher) Retry(ctx context.Context, deliveryID string) error {
	raw, err := d.store.GetDeliveryPayload(ctx, deliveryID)
	if err != nil {
		return fmt.Errorf("entrega %s nao encontrada", deliveryID)
	}
	if len(raw) == 0 {
		return fmt.Errorf("entrega %s sem payload guardado (anterior ao reenvio manual)", deliveryID)
	}
	task := asynq.NewTask(TaskDeliver, raw,
		asynq.MaxRetry(d.maxAttempts),
		asynq.Timeout(d.http.Timeout+5*time.Second),
		asynq.Queue("webhook"),
		asynq.Retention(24*time.Hour),
	)
	if _, err := d.client.EnqueueContext(ctx, task); err != nil {
		return err
	}
	_ = d.store.MarkPending(ctx, deliveryID)
	return nil
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
		webhookID := p.EventID
		if webhookID == "" {
			webhookID = p.DeliveryID
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "wa-gateway/webhook")
		req.Header.Set("X-Webhook-Id", webhookID)
		req.Header.Set("X-Delivery-Id", p.DeliveryID)
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

// urlKey e um identificador curto e estavel do URL, usado no TaskID de
// deduplicacao do asynq. Hash (nao truncamento) pra dois URLs distintos
// nunca colidirem e derrubarem uma entrega legitima.
func urlKey(u string) string {
	sum := sha1.Sum([]byte(u))
	return hex.EncodeToString(sum[:8])
}

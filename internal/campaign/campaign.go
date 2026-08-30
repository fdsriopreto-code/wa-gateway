// Package campaign faz envio em massa reaproveitando a fila de saída: cada
// destinatário vira um job da outbox (id "camp:<id>:<n>"), então o pacing
// anti-ban, o retry e o registro são os mesmos de um envio avulso. O
// progresso é um agregado sobre campaign_targets, alimentado pelo worker da
// outbox via OutboxRecorder.
package campaign

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"wa-gateway/internal/cache"
	"wa-gateway/internal/outbox"
	"wa-gateway/internal/store"
)

const (
	lockTTL   = 2 * time.Minute
	chunk     = 100 // reavalia o status da campanha a cada N enfileiramentos
	enqRetry  = 3
	retryWait = 2 * time.Second
)

// Runner orquestra as campanhas deste nó.
type Runner struct {
	store  *store.Store
	queue  *outbox.Queue
	cache  *cache.Redis
	log    *slog.Logger
	nodeID string
}

func NewRunner(st *store.Store, q *outbox.Queue, rc *cache.Redis, log *slog.Logger, nodeID string) *Runner {
	return &Runner{store: st, queue: q, cache: rc, log: log.With("comp", "campaign"), nodeID: nodeID}
}

func lockKey(id string) string { return "wa:campaign:" + id }

// Start dispara (em goroutine) o enfileiramento dos alvos pendentes de uma
// campanha. Idempotente: alvos já enfileirados são ignorados pelo TaskID.
func (r *Runner) Start(c store.Campaign) {
	go r.run(c)
}

// Stop marca a campanha como parada. Os jobs já enfileirados que ainda não
// saíram são barrados pelo Gate do worker; os pendentes não são enfileirados.
func (r *Runner) Stop(ctx context.Context, id string) error {
	return r.store.SetCampaignStatus(ctx, id, "stopped")
}

// Resume reenfileira as campanhas que ficaram "running" (reinício do processo).
func (r *Runner) Resume(ctx context.Context) {
	cs, err := r.store.RunningCampaigns(ctx)
	if err != nil {
		r.log.Error("resume: listar campanhas", "err", err)
		return
	}
	for _, c := range cs {
		r.log.Info("resume: retomando campanha", "id", c.ID, "session", c.Session)
		r.Start(c)
	}
}

func (r *Runner) run(c store.Campaign) {
	ctx := context.Background()

	ok, err := r.cache.AcquireLock(ctx, lockKey(c.ID), r.nodeID, lockTTL)
	if err != nil || !ok {
		r.log.Info("campanha já sob execução noutro nó", "id", c.ID, "err", err)
		return
	}
	defer func() { _ = r.cache.ReleaseLock(ctx, lockKey(c.ID), r.nodeID) }()

	var pace *outbox.Pace
	if c.Pace.MinIntervalMs > 0 || c.Pace.JitterMs > 0 {
		pace = &outbox.Pace{
			MinInterval: time.Duration(c.Pace.MinIntervalMs) * time.Millisecond,
			Jitter:      time.Duration(c.Pace.JitterMs) * time.Millisecond,
		}
	}

	var tmpl outbox.Args
	if len(c.Args) > 0 {
		if err := json.Unmarshal(c.Args, &tmpl); err != nil {
			r.log.Error("campanha: args inválidos", "id", c.ID, "err", err)
			_ = r.store.SetCampaignStatus(ctx, c.ID, "stopped")
			return
		}
	}

	sent := 0
	for {
		if st := r.store.CampaignStatus(ctx, c.ID); st != "running" {
			r.log.Info("campanha não está mais running, parando", "id", c.ID, "status", st)
			return
		}
		targets, err := r.store.PendingTargets(ctx, c.ID, chunk)
		if err != nil {
			r.log.Error("campanha: ler alvos", "id", c.ID, "err", err)
			return
		}
		if len(targets) == 0 {
			break
		}
		for _, t := range targets {
			job := outbox.Job{
				ID:      store.CampaignJobID(c.ID, t.N),
				Session: c.Session,
				Kind:    outbox.Kind(c.Kind),
				Args:    argsFor(tmpl, t.ChatID),
			}
			if err := r.enqueue(ctx, job, pace); err != nil {
				// limite diário ou fila indisponível: para por aqui e deixa o
				// resto pendente. O operador recria/roda de novo depois.
				r.log.Warn("campanha: enfileiramento parou", "id", c.ID, "n", t.N, "err", err)
				_ = r.store.SetCampaignStatus(ctx, c.ID, "stopped")
				return
			}
			_ = r.store.MarkTargetQueued(ctx, c.ID, t.N)
			sent++
		}
		_, _ = r.cache.RenewLock(ctx, lockKey(c.ID), r.nodeID, lockTTL)
	}

	if _, err := r.store.FinalizeCampaign(ctx, c.ID); err != nil {
		r.log.Warn("campanha: finalize", "id", c.ID, "err", err)
	}
	r.log.Info("campanha: alvos enfileirados", "id", c.ID, "n", sent)
}

func (r *Runner) enqueue(ctx context.Context, j outbox.Job, pace *outbox.Pace) error {
	var err error
	for i := 0; i < enqRetry; i++ {
		if _, err = r.queue.Enqueue(ctx, j, pace, 0); err == nil {
			return nil
		}
		if _, ok := err.(outbox.ErrDailyLimit); ok {
			return err // não adianta re-tentar
		}
		time.Sleep(retryWait)
	}
	return err
}

// argsFor injeta o destinatário no Args-template da campanha.
func argsFor(tmpl outbox.Args, chatID string) outbox.Args {
	a := tmpl
	a.ChatID = chatID
	return a
}

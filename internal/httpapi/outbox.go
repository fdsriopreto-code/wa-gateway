package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"wa-gateway/internal/events"
	"wa-gateway/internal/outbox"
	"wa-gateway/internal/session"
)

// enqueue coloca um envio na fila de saida, aplicando o pacing da sessao
// (ou o global) e devolve 202 com o id do job e o horario previsto.
func (d Deps) enqueue(w http.ResponseWriter, r *http.Request, sess string, kind outbox.Kind, args outbox.Args, delayStr string) {
	if sess == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "session e obrigatorio")
		return
	}
	if d.Queue == nil {
		writeErr(w, http.StatusServiceUnavailable, "queue_disabled", "fila de saida indisponivel")
		return
	}

	var extra time.Duration
	if delayStr != "" {
		dur, err := time.ParseDuration(delayStr)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "bad_request", "delay invalido: "+err.Error())
			return
		}
		extra = dur
	}

	override := d.paceOverride(r, sess)
	job := outbox.Job{ID: events.NewID(), Session: sess, Kind: kind, Args: args}

	runAt, err := d.Queue.Enqueue(r.Context(), job, override, extra)
	if err != nil {
		var dl outbox.ErrDailyLimit
		if errors.As(err, &dl) {
			writeErr(w, http.StatusTooManyRequests, "daily_limit", err.Error())
			return
		}
		writeErr(w, http.StatusInternalServerError, "enqueue_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"queued": true,
		"jobId":  job.ID,
		"runAt":  runAt.UTC().Format(time.RFC3339Nano),
	})
}

// paceOverride le config.outbox da sessao e converte para *outbox.Pace.
func (d Deps) paceOverride(r *http.Request, sess string) *outbox.Pace {
	rec, err := d.Manager.Get(r.Context(), sess)
	if err != nil {
		return nil
	}
	cfg, err := session.ParseConfig(rec.Config)
	if err != nil || cfg.Outbox == nil {
		return nil
	}
	return &outbox.Pace{
		MinInterval: time.Duration(cfg.Outbox.MinIntervalMs) * time.Millisecond,
		Jitter:      time.Duration(cfg.Outbox.JitterMs) * time.Millisecond,
		DailyLimit:  cfg.Outbox.DailyLimit,
	}
}

// GET /api/outbox?session=&limit=
func (d Deps) listOutbox(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	sess := q.Get("session")
	if sess == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "session e obrigatorio")
		return
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	jobs, err := d.Store.ListOutboxJobs(r.Context(), sess, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if jobs == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	writeJSON(w, http.StatusOK, jobs)
}

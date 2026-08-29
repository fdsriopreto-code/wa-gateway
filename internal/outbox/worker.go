package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"

	"github.com/hibiken/asynq"

	"wa-gateway/internal/engine"
)

// EngineResolver devolve a engine viva de uma sessao neste no.
type EngineResolver interface {
	Engine(session string) (engine.Engine, bool)
}

// Worker consome a fila "outbox" e executa cada job contra a engine.
type Worker struct {
	resolver EngineResolver
	rec      Recorder
	log      *slog.Logger
}

func NewWorker(r EngineResolver, rec Recorder, log *slog.Logger) *Worker {
	return &Worker{resolver: r, rec: rec, log: log}
}

// errSessionInactive faz o asynq re-tentar: a sessao pode estar reconectando.
var errSessionInactive = errors.New("sessao inativa neste no")

func (w *Worker) Handler() asynq.HandlerFunc {
	return func(ctx context.Context, t *asynq.Task) error {
		var j Job
		if err := json.Unmarshal(t.Payload(), &j); err != nil {
			return errors.Join(asynq.SkipRetry, err)
		}
		attempt, _ := asynq.GetRetryCount(ctx)
		attempt++

		eng, ok := w.resolver.Engine(j.Session)
		if !ok {
			w.log.Warn("outbox: sessao inativa, re-tentando", "session", j.Session, "job", j.ID, "attempt", attempt)
			return errSessionInactive
		}

		res, err := Dispatch(ctx, eng, j.Kind, j.Args)
		if err != nil {
			if w.rec != nil {
				_ = w.rec.Failed(ctx, j.ID, attempt, err.Error())
			}
			// erro de validacao (chatId/ref/kind) nao melhora com retry.
			if isPermanent(err) {
				w.log.Error("outbox: job invalido, descartando", "job", j.ID, "err", err)
				return errors.Join(asynq.SkipRetry, err)
			}
			w.log.Warn("outbox: falha no envio, re-tentando", "job", j.ID, "attempt", attempt, "err", err)
			return err
		}

		w.log.Info("outbox: enviado", "job", j.ID, "session", j.Session, "kind", j.Kind, "messageId", res.MessageID)
		if w.rec != nil {
			_ = w.rec.Sent(ctx, j.ID, attempt, res.MessageID)
		}
		return nil
	}
}

// isPermanent classifica erros de validacao (que nao melhoram com retry).
func isPermanent(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	for _, frag := range []string{"invalido", "ausente", "desconhecido", "obrigatori"} {
		if strings.Contains(s, frag) {
			return true
		}
	}
	return false
}

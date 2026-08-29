// Package outbox e a fila de saida de mensagens com "pacing" por sessao.
//
// Motivacao: disparar varias mensagens seguidas do mesmo numero e o caminho
// mais rapido para um ban. Em vez de BullMQ (que e Node), usamos o asynq
// (mesma stack do webhook, Redis por baixo). Cada envio enfileirado reserva
// um "slot" de tempo no Redis: os jobs de uma sessao saem espacados por um
// intervalo minimo + jitter aleatorio, preservando a ordem de chegada.
package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/hibiken/asynq"

	"wa-gateway/internal/cache"
)

const (
	TaskSend  = "outbox:send"
	QueueName = "outbox"
)

// Pace e a politica de espacamento aplicada a uma sessao.
type Pace struct {
	MinInterval time.Duration // intervalo minimo entre dois envios da mesma sessao
	Jitter      time.Duration // atraso aleatorio adicional (0..Jitter)
	DailyLimit  int           // teto de envios por dia (0 = ilimitado)
}

// Defaults e o pacing global, usado quando a sessao nao define o seu.
type Defaults struct {
	Pace Pace
}

// Job e a unidade enfileirada: uma acao de envio para uma sessao.
type Job struct {
	ID      string `json:"id"`
	Session string `json:"session"`
	Kind    Kind   `json:"kind"`
	Args    Args   `json:"args"`
}

// Recorder persiste o ciclo de vida de um job (opcional; pode ser nil).
type Recorder interface {
	Scheduled(ctx context.Context, j Job, runAt time.Time) error
	Sent(ctx context.Context, id string, attempt int, messageID string) error
	Failed(ctx context.Context, id string, attempt int, errMsg string) error
}

// Queue enfileira jobs respeitando o pacing por sessao.
type Queue struct {
	client   *asynq.Client
	redis    *cache.Redis
	defaults Defaults
	rec      Recorder
}

func NewQueue(client *asynq.Client, rc *cache.Redis, defaults Defaults, rec Recorder) *Queue {
	if defaults.Pace.MinInterval <= 0 {
		defaults.Pace.MinInterval = 3 * time.Second
	}
	if defaults.Pace.Jitter < 0 {
		defaults.Pace.Jitter = 0
	}
	return &Queue{client: client, redis: rc, defaults: defaults, rec: rec}
}

// ErrDailyLimit e devolvido quando a sessao ja bateu o teto diario.
type ErrDailyLimit struct {
	Session string
	Limit   int
}

func (e ErrDailyLimit) Error() string {
	return fmt.Sprintf("sessao %q atingiu o limite diario de %d envios", e.Session, e.Limit)
}

// slotScript reserva atomicamente o proximo horario livre da sessao.
// KEYS[1] = chave do relogio da sessao
// ARGV[1] = agora (ms)  ARGV[2] = intervalo (ms)  ARGV[3] = ttl da chave (ms)
var slotScript = `
local cur = tonumber(redis.call('GET', KEYS[1]) or '0')
local base = math.max(tonumber(ARGV[1]), cur)
local slot = base + tonumber(ARGV[2])
redis.call('SET', KEYS[1], slot, 'PX', tonumber(ARGV[3]))
return slot`

func clockKey(session string) string { return "wa:outbox:clock:" + session }
func dailyKey(session string) string {
	return "wa:outbox:daily:" + session + ":" + time.Now().UTC().Format("20060102")
}

// paceFor mescla o pacing da sessao com os defaults globais.
func (q *Queue) paceFor(override *Pace) Pace {
	p := q.defaults.Pace
	if override == nil {
		return p
	}
	if override.MinInterval > 0 {
		p.MinInterval = override.MinInterval
	}
	if override.Jitter > 0 {
		p.Jitter = override.Jitter
	}
	if override.DailyLimit > 0 {
		p.DailyLimit = override.DailyLimit
	}
	return p
}

// Enqueue reserva o slot de tempo da sessao e enfileira o job. Devolve o
// horario previsto de envio.
func (q *Queue) Enqueue(ctx context.Context, j Job, override *Pace, extraDelay time.Duration) (time.Time, error) {
	pace := q.paceFor(override)

	if pace.DailyLimit > 0 {
		n, err := q.redis.Raw().Incr(ctx, dailyKey(j.Session)).Result()
		if err != nil {
			return time.Time{}, fmt.Errorf("daily incr: %w", err)
		}
		if n == 1 {
			_ = q.redis.Raw().Expire(ctx, dailyKey(j.Session), 26*time.Hour).Err()
		}
		if n > int64(pace.DailyLimit) {
			return time.Time{}, ErrDailyLimit{Session: j.Session, Limit: pace.DailyLimit}
		}
	}

	interval := pace.MinInterval + extraDelay
	ttl := interval + 10*time.Minute
	slotMs, err := q.redis.Raw().Eval(ctx, slotScript, []string{clockKey(j.Session)},
		time.Now().UnixMilli(), interval.Milliseconds(), ttl.Milliseconds()).Int64()
	if err != nil {
		return time.Time{}, fmt.Errorf("reserva de slot: %w", err)
	}
	runAt := time.UnixMilli(slotMs)
	if pace.Jitter > 0 {
		runAt = runAt.Add(time.Duration(rand.Int63n(int64(pace.Jitter))))
	}

	payload, err := json.Marshal(j)
	if err != nil {
		return time.Time{}, err
	}
	task := asynq.NewTask(TaskSend, payload,
		asynq.Queue(QueueName),
		asynq.TaskID(j.ID),
		asynq.MaxRetry(8),
		asynq.ProcessAt(runAt),
		asynq.Retention(24*time.Hour),
	)
	if _, err := q.client.EnqueueContext(ctx, task); err != nil {
		if err == asynq.ErrDuplicateTask || err == asynq.ErrTaskIDConflict {
			return runAt, nil
		}
		return time.Time{}, err
	}
	if q.rec != nil {
		_ = q.rec.Scheduled(ctx, j, runAt)
	}
	return runAt, nil
}

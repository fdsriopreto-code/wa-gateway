package outbox

import (
	"errors"
	"testing"
	"time"

	"github.com/hibiken/asynq"
)

func TestPaceForMerge(t *testing.T) {
	q := &Queue{defaults: Defaults{Pace: Pace{MinInterval: 3 * time.Second, Jitter: 2 * time.Second, DailyLimit: 0}}}

	if got := q.paceFor(nil); got != q.defaults.Pace {
		t.Fatalf("nil override deveria devolver o default, veio %+v", got)
	}

	got := q.paceFor(&Pace{MinInterval: 10 * time.Second})
	if got.MinInterval != 10*time.Second {
		t.Errorf("MinInterval = %v, want 10s", got.MinInterval)
	}
	if got.Jitter != 2*time.Second {
		t.Errorf("Jitter = %v, want default 2s", got.Jitter)
	}

	got = q.paceFor(&Pace{DailyLimit: 500})
	if got.DailyLimit != 500 || got.MinInterval != 3*time.Second {
		t.Errorf("merge parcial errado: %+v", got)
	}
}

func TestIsPermanent(t *testing.T) {
	perm := []error{
		errors.New("chatId invalido: x"),
		errors.New("ref ausente"),
		errors.New(`kind desconhecido: "foo"`),
	}
	for _, e := range perm {
		if !isPermanent(e) {
			t.Errorf("%q deveria ser permanente", e)
		}
	}
	if isPermanent(errors.New("connection reset by peer")) {
		t.Error("erro de rede nao deveria ser permanente")
	}
	if isPermanent(nil) {
		t.Error("nil nao e permanente")
	}
}

func TestNewQueueDefaults(t *testing.T) {
	q := NewQueue(&asynq.Client{}, nil, Defaults{}, nil)
	if q.defaults.Pace.MinInterval != 3*time.Second {
		t.Errorf("MinInterval default = %v, want 3s", q.defaults.Pace.MinInterval)
	}
}

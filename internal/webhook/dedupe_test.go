package webhook

import (
	"errors"
	"fmt"
	"testing"

	"github.com/hibiken/asynq"
)

// O asynq devolve o conflito de TaskID embrulhado (fmt.Errorf("%w", ...)):
// comparar com == nunca casava e o evento ficava em reprocessamento infinito.
func TestTaskIDConflictIsWrapped(t *testing.T) {
	err := fmt.Errorf("%w", asynq.ErrTaskIDConflict)
	if err == asynq.ErrTaskIDConflict {
		t.Fatal("premissa mudou: o erro não vem mais embrulhado")
	}
	if !errors.Is(err, asynq.ErrTaskIDConflict) {
		t.Fatal("errors.Is deveria reconhecer o conflito embrulhado")
	}
}

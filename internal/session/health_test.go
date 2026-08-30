package session

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"wa-gateway/internal/engine"
	"wa-gateway/internal/events"
)

func newHealthMgr(threshold time.Duration) (*Manager, *events.Bus, <-chan events.Event) {
	bus := events.NewBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	ch, _ := bus.Subscribe("test", "session.*", 16)
	m := &Manager{
		log:            slog.New(slog.NewTextHandler(io.Discard, nil)),
		bus:            bus,
		health:         make(map[string]*healthState),
		unhealthyAfter: threshold,
	}
	return m, bus, ch
}

func drain(ch <-chan events.Event) []string {
	var out []string
	for {
		select {
		case e := <-ch:
			out = append(out, e.Name)
		case <-time.After(50 * time.Millisecond):
			return out
		}
	}
}

func TestHealthDebounceAndRecover(t *testing.T) {
	m, _, ch := newHealthMgr(time.Minute)
	base := time.Now()

	// WORKING => healthy, sem evento
	m.evalHealth("s", engine.StatusWorking, base)
	if got := m.Health("s"); got != HealthOK {
		t.Fatalf("health = %q", got)
	}

	// FALHOU agora — ainda dentro do limiar => waiting, sem evento
	m.evalHealth("s", engine.StatusFailed, base.Add(10*time.Second))
	if got := m.Health("s"); got != HealthWaiting {
		t.Fatalf("health logo após falha = %q, queria waiting", got)
	}
	if evs := drain(ch); len(evs) != 0 {
		t.Fatalf("não devia emitir ainda: %v", evs)
	}

	// Continua falho passado o limiar => unhealthy + evento
	m.evalHealth("s", engine.StatusFailed, base.Add(2*time.Minute))
	if got := m.Health("s"); got != HealthDown {
		t.Fatalf("health = %q, queria unhealthy", got)
	}
	if evs := drain(ch); len(evs) != 1 || evs[0] != events.SessionUnhealthy {
		t.Fatalf("eventos = %v, queria [session.unhealthy]", evs)
	}

	// Ainda falho => não repete o evento
	m.evalHealth("s", engine.StatusFailed, base.Add(3*time.Minute))
	if evs := drain(ch); len(evs) != 0 {
		t.Fatalf("não devia repetir: %v", evs)
	}

	// Voltou pra WORKING => healthy + evento de recuperação
	m.evalHealth("s", engine.StatusWorking, base.Add(4*time.Minute))
	if got := m.Health("s"); got != HealthOK {
		t.Fatalf("health = %q", got)
	}
	if evs := drain(ch); len(evs) != 1 || evs[0] != events.SessionHealthy {
		t.Fatalf("eventos = %v, queria [session.healthy]", evs)
	}
}

func TestHealthScanQRNeverAlerts(t *testing.T) {
	m, _, ch := newHealthMgr(time.Minute)
	base := time.Now()
	for i := 0; i < 10; i++ {
		m.evalHealth("s", engine.StatusScanQR, base.Add(time.Duration(i)*time.Minute))
	}
	if got := m.Health("s"); got != HealthWaiting {
		t.Fatalf("health = %q, queria waiting", got)
	}
	if evs := drain(ch); len(evs) != 0 {
		t.Fatalf("SCAN_QR não devia alertar: %v", evs)
	}
}

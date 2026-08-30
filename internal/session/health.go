package session

import (
	"context"
	"time"

	"wa-gateway/internal/engine"
	"wa-gateway/internal/events"
	"wa-gateway/internal/observability"
)

// Health é o estado de saúde derivado de uma sessão viva neste nó.
type Health string

const (
	HealthOK      Health = "healthy"
	HealthWaiting Health = "waiting"   // SCAN_QR / iniciando há pouco — esperado
	HealthDown    Health = "unhealthy" // devia estar WORKING e não está há um tempo
)

const healthSweep = 20 * time.Second

type healthState struct {
	last     Health    // valor vivo (p/ exibir no console)
	reported Health    // último estado que gerou evento (detecção de borda)
	badSince time.Time // desde quando o status != WORKING; zero = ok agora
}

// SetHealthThreshold define quanto tempo o status precisa ficar ruim antes de
// emitir session.unhealthy (SESSION_UNHEALTHY_AFTER). <=0 mantém o padrão.
func (m *Manager) SetHealthThreshold(d time.Duration) {
	if d > 0 {
		m.unhealthyAfter = d
	}
}

// Health devolve o estado de saúde atual de uma sessão viva neste nó.
// "" quando a sessão não está rodando aqui.
func (m *Manager) Health(name string) Health {
	m.healthMu.RLock()
	defer m.healthMu.RUnlock()
	if hs := m.health[name]; hs != nil {
		return hs.last
	}
	return ""
}

// MonitorHealth avalia periodicamente as sessões vivas e emite
// session.unhealthy / session.healthy nas transições (com debounce). Bloqueia
// até ctx acabar — rodar em goroutine.
func (m *Manager) MonitorHealth(ctx context.Context) {
	if m.unhealthyAfter <= 0 {
		m.unhealthyAfter = 2 * time.Minute
	}
	t := time.NewTicker(healthSweep)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.sweepHealth()
		}
	}
}

func (m *Manager) sweepHealth() {
	m.mu.RLock()
	names := make([]string, 0, len(m.running))
	engs := make([]engine.Engine, 0, len(m.running))
	for n, h := range m.running {
		names = append(names, n)
		engs = append(engs, h.eng)
	}
	m.mu.RUnlock()

	now := time.Now()
	alive := make(map[string]bool, len(names))
	for i, name := range names {
		alive[name] = true
		m.evalHealth(name, engs[i].Status(), now)
	}

	m.healthMu.Lock()
	for n := range m.health {
		if !alive[n] {
			delete(m.health, n) // sessão saiu deste nó
		}
	}
	m.healthMu.Unlock()
}

func (m *Manager) evalHealth(name string, st engine.Status, now time.Time) {
	m.healthMu.Lock()
	hs := m.health[name]
	if hs == nil {
		hs = &healthState{last: HealthOK, reported: HealthOK}
		m.health[name] = hs
	}

	var cur Health
	switch {
	case st == engine.StatusWorking:
		hs.badSince = time.Time{}
		cur = HealthOK
	case st == engine.StatusScanQR:
		cur = HealthWaiting // aguardando o QR ser lido — ação do usuário
	default: // STARTING, FAILED, LOGGED_OUT, STOPPED
		if hs.badSince.IsZero() {
			hs.badSince = now
		}
		if now.Sub(hs.badSince) >= m.unhealthyAfter {
			cur = HealthDown
		} else {
			cur = HealthWaiting
		}
	}
	hs.last = cur

	var (
		emitName string
		since    time.Time
	)
	switch {
	case cur == HealthDown && hs.reported != HealthDown:
		hs.reported = HealthDown
		emitName = events.SessionUnhealthy
		since = hs.badSince
	case cur == HealthOK && hs.reported == HealthDown:
		hs.reported = HealthOK
		emitName = events.SessionHealthy
	}
	m.healthMu.Unlock()

	if emitName == "" {
		return
	}
	payload := map[string]any{"status": string(st), "health": string(cur)}
	if !since.IsZero() {
		payload["since"] = since.UTC().Format(time.RFC3339)
		payload["forSeconds"] = int(now.Sub(since).Seconds())
	}
	if emitName == events.SessionUnhealthy {
		m.log.Warn("sessão não saudável", "session", name, "status", st, "há", now.Sub(since).Truncate(time.Second))
	} else {
		m.log.Info("sessão recuperou", "session", name, "status", st)
	}
	m.raise(name, emitName, payload)
}

// raise publica um evento de sistema da sessão (bus + stream durável).
func (m *Manager) raise(name, evName string, payload map[string]any) {
	e := events.Event{
		ID:        events.NewID(),
		Session:   name,
		Name:      evName,
		Timestamp: time.Now().UTC(),
		Engine:    "wa-gateway",
		Payload:   payload,
	}
	observability.EventsPublished.WithLabelValues(evName).Inc()
	m.bus.Publish(e)
	m.evStream.Append(e)
}

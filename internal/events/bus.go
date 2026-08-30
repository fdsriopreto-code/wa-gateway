package events

import (
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Matcher aceita "*" (tudo), "prefixo.*" (namespace) ou nome exato.
func Match(pattern, name string) bool {
	if pattern == "*" || pattern == name {
		return true
	}
	if strings.HasSuffix(pattern, ".*") {
		prefix := pattern[:len(pattern)-1] // mantem o ponto: "message."
		return strings.HasPrefix(name, prefix) || name == pattern[:len(pattern)-2]
	}
	return false
}

// MatchAny retorna true se algum dos patterns casar com name.
func MatchAny(patterns []string, name string) bool {
	for _, p := range patterns {
		if Match(p, name) {
			return true
		}
	}
	return false
}

type subscription struct {
	name    string
	pattern string
	ch      chan Event
	maxWait time.Duration // >0: Publish espera até isso antes de descartar
	dropped atomic.Uint64
}

// Bus e um pub/sub in-process. Publish nao bloqueia por assinante best-effort
// (perde evento + incrementa contador); para assinantes "confiaveis"
// (SubscribeReliable) espera ate maxWait antes de descartar.
type Bus struct {
	logger *slog.Logger
	mu     sync.RWMutex
	subs   map[*subscription]struct{}
	snap   atomic.Pointer[[]*subscription] // cópia imutável p/ Publish sem lock

	// OnDrop, se definido, e chamado quando um assinante perde um evento.
	OnDrop func(subscriber string)
	// OnPublish, se definido, e chamado a cada Publish (para metricas).
	OnPublish func(event string)
}

func NewBus(log *slog.Logger) *Bus {
	b := &Bus{logger: log, subs: make(map[*subscription]struct{})}
	empty := make([]*subscription, 0)
	b.snap.Store(&empty)
	return b
}

func (b *Bus) rebuildSnapshot() {
	list := make([]*subscription, 0, len(b.subs))
	for s := range b.subs {
		list = append(list, s)
	}
	b.snap.Store(&list)
}

// Subscribe registra um assinante best-effort (evento descartado na hora se o
// canal estiver cheio). name so serve para logs/metricas.
func (b *Bus) Subscribe(name, pattern string, buffer int) (<-chan Event, func()) {
	return b.subscribe(name, pattern, buffer, 0)
}

// SubscribeReliable e como Subscribe, mas Publish espera ate maxWait o canal
// abrir espaço antes de descartar — para consumidores onde perder evento é
// grave (webhook, persistencia). O custo é uma pausa curta no publicador sob
// pico sustentado.
func (b *Bus) SubscribeReliable(name, pattern string, buffer int, maxWait time.Duration) (<-chan Event, func()) {
	if maxWait <= 0 {
		maxWait = 250 * time.Millisecond
	}
	return b.subscribe(name, pattern, buffer, maxWait)
}

func (b *Bus) subscribe(name, pattern string, buffer int, maxWait time.Duration) (<-chan Event, func()) {
	if buffer <= 0 {
		buffer = 1024
	}
	s := &subscription{name: name, pattern: pattern, ch: make(chan Event, buffer), maxWait: maxWait}
	b.mu.Lock()
	b.subs[s] = struct{}{}
	b.rebuildSnapshot()
	b.mu.Unlock()

	return s.ch, func() {
		b.mu.Lock()
		if _, ok := b.subs[s]; ok {
			delete(b.subs, s)
			b.rebuildSnapshot()
			close(s.ch)
		}
		b.mu.Unlock()
	}
}

func (b *Bus) Publish(e Event) {
	if b.OnPublish != nil {
		b.OnPublish(e.Name)
	}
	for _, s := range *b.snap.Load() {
		if !Match(s.pattern, e.Name) {
			continue
		}
		select {
		case s.ch <- e:
			continue
		default:
		}
		if s.maxWait > 0 {
			t := time.NewTimer(s.maxWait)
			select {
			case s.ch <- e:
				t.Stop()
				continue
			case <-t.C:
			}
		}
		n := s.dropped.Add(1)
		if b.OnDrop != nil {
			b.OnDrop(s.name)
		}
		if n%100 == 1 && b.logger != nil {
			b.logger.Warn("assinante lento, evento descartado",
				"subscriber", s.name, "dropped_total", n)
		}
	}
}

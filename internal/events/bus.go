package events

import (
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
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
	dropped atomic.Uint64
}

// Bus e um pub/sub in-process. Publish nunca bloqueia: assinante lento
// perde eventos e incrementa um contador.
type Bus struct {
	log  *slog.Logger
	mu   sync.RWMutex
	subs map[*subscription]struct{}

	// OnDrop, se definido, e chamado quando um assinante perde um evento.
	OnDrop func(subscriber string)
	// OnPublish, se definido, e chamado a cada Publish (para metricas).
	OnPublish func(event string)
}

func NewBus(log *slog.Logger) *Bus {
	return &Bus{log: log, subs: make(map[*subscription]struct{})}
}

// Subscribe registra um assinante. name so serve para logs/metricas.
// Retorna o canal de leitura e uma funcao de cancelamento.
func (b *Bus) Subscribe(name, pattern string, buffer int) (<-chan Event, func()) {
	if buffer <= 0 {
		buffer = 1024
	}
	s := &subscription{name: name, pattern: pattern, ch: make(chan Event, buffer)}
	b.mu.Lock()
	b.subs[s] = struct{}{}
	b.mu.Unlock()

	return s.ch, func() {
		b.mu.Lock()
		if _, ok := b.subs[s]; ok {
			delete(b.subs, s)
			close(s.ch)
		}
		b.mu.Unlock()
	}
}

func (b *Bus) Publish(e Event) {
	if b.OnPublish != nil {
		b.OnPublish(e.Name)
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for s := range b.subs {
		if !Match(s.pattern, e.Name) {
			continue
		}
		select {
		case s.ch <- e:
		default:
			n := s.dropped.Add(1)
			if b.OnDrop != nil {
				b.OnDrop(s.name)
			}
			if n%100 == 1 {
				b.log.Warn("assinante lento, evento descartado",
					"subscriber", s.name, "dropped_total", n)
			}
		}
	}
}

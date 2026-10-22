package session

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestStartRespectsNodeCap(t *testing.T) {
	m := &Manager{running: map[string]*handle{"a": {}, "b": {}}}
	if m.full() {
		t.Fatal("sem teto nunca está cheio")
	}
	m.SetMaxSessions(2)
	if !m.full() {
		t.Fatal("2 de 2 deveria estar cheio")
	}
	// store nil: se o teto não barrar antes, isto entra em pânico
	if err := m.start(context.Background(), "c", true); !errors.Is(err, ErrNodeFull) {
		t.Fatalf("start com nó cheio: err = %v, quero ErrNodeFull", err)
	}
	m.SetMaxSessions(3)
	if m.full() {
		t.Fatal("2 de 3 não está cheio")
	}
}

func TestSessionBehaviorTypingDefaults(t *testing.T) {
	on := Config{Typing: &TypingConfig{Enabled: true}}
	m := &Manager{}
	m.rawCache.Store("s", cfgFlag{cfg: on, exp: farFuture()})
	b := m.sessionBehavior("s")
	if b.TypingMsPerChar != 40 || b.TypingMaxMs != 6000 {
		t.Fatalf("defaults de digitação errados: %+v", b)
	}
	m.rawCache.Store("off", cfgFlag{cfg: Config{Typing: &TypingConfig{Enabled: false, MsPerChar: 10}}, exp: farFuture()})
	if b := m.sessionBehavior("off"); b.TypingMsPerChar != 0 {
		t.Fatalf("typing desligado não deveria simular: %+v", b)
	}
}

func farFuture() time.Time { return time.Now().Add(time.Hour) }

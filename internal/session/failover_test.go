package session

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"wa-gateway/internal/cache"
	"wa-gateway/internal/engine"
)

// engine falsa: so o Stop importa aqui (o resto da interface fica nil e nao e chamado)
type engFalsa struct {
	engine.Engine
	parou atomic.Bool
}

func (e *engFalsa) Stop() error  { e.parou.Store(true); return nil }
func (e *engFalsa) Name() string { return "falsa" }

func novoMgrComLock(t *testing.T) (*Manager, *miniredis.Miniredis, *engFalsa, chan struct{}) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mr.Close)
	rc, err := cache.New(context.Background(), "redis://"+mr.Addr())
	if err != nil {
		t.Fatal(err)
	}
	// store fica nil de proposito: se o stop tentar gravar "parada" no banco, o teste quebra
	m := &Manager{log: slog.New(slog.NewTextHandler(io.Discard, nil)), cache: rc, nodeID: "no-a", running: map[string]*handle{}}
	if ok, err := rc.AcquireLock(context.Background(), lockKey("s1"), "no-a", lockTTL); err != nil || !ok {
		t.Fatalf("lock inicial: %v %v", ok, err)
	}
	eng := &engFalsa{}
	h := &handle{eng: eng, stopCh: make(chan struct{})}
	m.running["s1"] = h
	return m, mr, eng, h.stopCh
}

func esperar(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	fim := time.Now().Add(2 * time.Second)
	for time.Now().Before(fim) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal(msg)
}

// Outro no ficou com o lock: este para a engine, sai de "running" e NAO grava parada
// (o estado desejado continua WORKING para o reconcile religar).
func TestPerdeuLockParaSemGravarParada(t *testing.T) {
	antes := lockRefresh
	lockRefresh = 20 * time.Millisecond
	defer func() { lockRefresh = antes }()

	m, mr, eng, stop := novoMgrComLock(t)
	go m.refreshLock("s1", stop)
	mr.Set(lockKey("s1"), "no-b") // outro no assumiu

	esperar(t, func() bool { return eng.parou.Load() }, "engine devia parar ao perder o lock")
	esperar(t, func() bool { m.mu.RLock(); defer m.mu.RUnlock(); return m.running["s1"] == nil }, "sessao devia sair de running")
	if dono, _ := m.cache.LockOwner(context.Background(), lockKey("s1")); dono != "no-b" {
		t.Fatalf("nao podia mexer no lock do outro no, dono=%q", dono)
	}
}

// Redis fora do ar: tambem para aqui sem gravar parada (antes virava STOPPED para sempre).
func TestRedisCaiuParaSemGravarParada(t *testing.T) {
	antes := lockRefresh
	lockRefresh = 20 * time.Millisecond
	defer func() { lockRefresh = antes }()

	m, mr, eng, stop := novoMgrComLock(t)
	go m.refreshLock("s1", stop)
	mr.Close()

	esperar(t, func() bool { return eng.parou.Load() }, "engine devia parar sem Redis")
	esperar(t, func() bool { m.mu.RLock(); defer m.mu.RUnlock(); return m.running["s1"] == nil }, "sessao devia sair de running")
}

// Com o lock em dia, nada acontece.
func TestLockEmDiaContinuaRodando(t *testing.T) {
	antes := lockRefresh
	lockRefresh = 20 * time.Millisecond
	defer func() { lockRefresh = antes }()

	m, _, eng, stop := novoMgrComLock(t)
	go m.refreshLock("s1", stop)
	time.Sleep(120 * time.Millisecond)
	close(stop)
	if eng.parou.Load() {
		t.Fatal("nao devia parar com o lock renovado")
	}
	if dono, _ := m.cache.LockOwner(context.Background(), lockKey("s1")); dono != "no-a" {
		t.Fatalf("lock devia seguir com este no, dono=%q", dono)
	}
}

// Package session orquestra o ciclo de vida das sessoes: 1 engine viva por
// sessao, posse garantida por lock no Redis (base para escala horizontal).
package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"wa-gateway/internal/cache"
	"wa-gateway/internal/engine"
	"wa-gateway/internal/events"
	"wa-gateway/internal/observability"
	"wa-gateway/internal/store"
)

const (
	lockTTL     = 30 * time.Second
	lockRefresh = 10 * time.Second
)

var (
	ErrNotFound    = store.ErrNotFound
	ErrLocked      = errors.New("sessao pertence a outro no")
	ErrNotActive   = errors.New("sessao nao esta ativa neste no")
	ErrJIDConflict = errors.New("conflito de JID entre sessoes")
)

type handle struct {
	eng    engine.Engine
	stopCh chan struct{}
	jid    string // JID pareado, quando conhecido (para detectar colisao)
}

type Manager struct {
	store         *store.Store
	cache         *cache.Redis
	bus           *events.Bus
	log           *slog.Logger
	dsn           string
	nodeID        string
	advertiseURL  string // URL HTTP deste nó p/ roteamento entre nós ("" = off)
	defaultEngine string
	media         engine.MediaSink

	mu      sync.RWMutex
	running map[string]*handle

	rawCache sync.Map // name -> cfgFlag
}

type cfgFlag struct {
	cfg Config
	exp time.Time
}

func NewManager(st *store.Store, rc *cache.Redis, bus *events.Bus, log *slog.Logger, dsn, nodeID, defaultEngine string, media engine.MediaSink) *Manager {
	return &Manager{
		store: st, cache: rc, bus: bus, log: log,
		dsn: dsn, nodeID: nodeID, defaultEngine: defaultEngine, media: media,
		running: make(map[string]*handle),
	}
}

// SetAdvertiseURL liga o roteamento entre nós: outros nós encaminham requests
// de sessões deste nó para esta URL. Vazio = roteamento desligado.
func (m *Manager) SetAdvertiseURL(u string) { m.advertiseURL = strings.TrimRight(u, "/") }

// ClusterEnabled diz se o roteamento entre nós está ligado (advertiseURL set).
func (m *Manager) ClusterEnabled() bool { return m.advertiseURL != "" }

// NodeID / AdvertiseURL — usados pelo endpoint /api/cluster.
func (m *Manager) NodeID() string       { return m.nodeID }
func (m *Manager) AdvertiseURL() string { return m.advertiseURL }

// LocalSessions lista os nomes das sessões vivas neste nó.
func (m *Manager) LocalSessions() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, 0, len(m.running))
	for n := range m.running {
		out = append(out, n)
	}
	return out
}

// ClusterHeartbeat publica periodicamente o endereço deste nó no Redis
// (nodeID → advertiseURL) para o roteamento entre nós. No-op sem advertiseURL.
func (m *Manager) ClusterHeartbeat(ctx context.Context) {
	if m.advertiseURL == "" || m.cache == nil {
		return
	}
	pub := func() {
		c, cancel := context.WithTimeout(ctx, 3*time.Second)
		_ = m.cache.SetNodeAddr(c, m.nodeID, m.advertiseURL, 30*time.Second)
		cancel()
	}
	pub()
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			pub()
		}
	}
}

// OwnerBaseURL devolve a URL do nó que hoje é dono da sessão `name`, e se ele
// é OUTRO nó (ok=true → vale encaminhar). "" quando ninguém é dono, é este
// nó, ou o dono não publicou endereço.
func (m *Manager) OwnerBaseURL(ctx context.Context, name string) (url string, remote bool) {
	if m.cache == nil {
		return "", false
	}
	owner, err := m.cache.LockOwner(ctx, lockKey(name))
	if err != nil || owner == "" || owner == m.nodeID {
		return "", false
	}
	addr, err := m.cache.GetNodeAddr(ctx, owner)
	if err != nil || addr == "" {
		return "", false
	}
	return strings.TrimRight(addr, "/"), true
}

// sessionConfig le sessions.config com cache curto (10s).
func (m *Manager) sessionConfig(name string) Config {
	if v, ok := m.rawCache.Load(name); ok {
		f := v.(cfgFlag)
		if time.Now().Before(f.exp) {
			return f.cfg
		}
	}
	var cfg Config
	if rec, err := m.store.GetSession(context.Background(), name); err == nil {
		if c, err := ParseConfig(rec.Config); err == nil {
			cfg = c
		}
	}
	m.rawCache.Store(name, cfgFlag{cfg: cfg, exp: time.Now().Add(10 * time.Second)})
	return cfg
}

func (m *Manager) sessionWantsRaw(name string) bool { return m.sessionConfig(name).RawEvents }

func (m *Manager) sessionBehavior(name string) engine.AutoBehavior {
	c := m.sessionConfig(name)
	return engine.AutoBehavior{AutoRead: c.AutoRead, AutoOnline: c.AutoOnline}
}

func lockKey(name string) string { return "wa:lock:" + name }

// Upsert cria ou atualiza o registro da sessao (sem inicia-la).
func (m *Manager) Upsert(ctx context.Context, name string, cfg json.RawMessage) (store.SessionRecord, error) {
	eng := m.defaultEngine
	m.rawCache.Delete(name)
	return m.store.UpsertSession(ctx, name, eng, cfg)
}

func (m *Manager) Get(ctx context.Context, name string) (store.SessionRecord, error) {
	return m.store.GetSession(ctx, name)
}

func (m *Manager) List(ctx context.Context) ([]store.SessionRecord, error) {
	return m.store.ListSessions(ctx)
}

func (m *Manager) Delete(ctx context.Context, name string) error {
	_ = m.Stop(ctx, name, false)
	m.rawCache.Delete(name)
	return m.store.DeleteSession(ctx, name)
}

// Engine devolve a engine viva desta sessao neste no.
func (m *Manager) Engine(name string) (engine.Engine, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	h, ok := m.running[name]
	if !ok {
		return nil, false
	}
	return h.eng, true
}

// Start adquire o lock de posse e inicia a engine. ctx e usado apenas para
// as chamadas de setup; o ciclo de vida da engine e independente.
func (m *Manager) Start(ctx context.Context, name string) error {
	return m.start(ctx, name, false)
}

// start com recovering=true e usado so pelo RestoreOwned (boot): permite a
// engine adotar um device orfao do store quando a sessao ja estava ativa mas
// perdeu o vinculo do JID. Numa partida interativa (recovering=false) isso
// nao acontece, pra uma sessao nova nao roubar o device de outra.
func (m *Manager) start(ctx context.Context, name string, recovering bool) error {
	m.mu.Lock()
	if _, ok := m.running[name]; ok {
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()

	rec, err := m.store.GetSession(ctx, name)
	if err != nil {
		return err
	}

	// Guarda contra colisao de device: duas sessoes nao podem apontar para o
	// mesmo JID pareado (WhatsApp derruba uma). Acontece se um registro ficou
	// com o jid errado.
	if rec.JID != "" {
		m.mu.RLock()
		for other, hd := range m.running {
			if other != name && hd.jid == rec.JID {
				m.mu.RUnlock()
				return fmt.Errorf("%w: JID %s ja em uso pela sessao %q — apague e recrie esta sessao", ErrJIDConflict, rec.JID, other)
			}
		}
		m.mu.RUnlock()
	}

	ok, err := m.cache.AcquireLock(ctx, lockKey(name), m.nodeID, lockTTL)
	if err != nil {
		return fmt.Errorf("lock: %w", err)
	}
	if !ok {
		return ErrLocked
	}

	engName := rec.Engine
	if engName == "" {
		engName = m.defaultEngine
	}
	factory, ok := engine.Get(engName)
	if !ok {
		_ = m.cache.ReleaseLock(ctx, lockKey(name), m.nodeID)
		return fmt.Errorf("engine desconhecida: %s", engName)
	}

	eng, err := factory(engine.Deps{
		Session:    name,
		DSN:        m.dsn,
		Logger:     m.log.With("session", name),
		Emit:       m.emit(name, engName),
		Media:      m.media,
		RawEvents:  func() bool { return m.sessionWantsRaw(name) },
		Behavior:   func() engine.AutoBehavior { return m.sessionBehavior(name) },
		StoredJID:  rec.JID,
		Recovering: recovering,
	})
	if err != nil {
		_ = m.cache.ReleaseLock(ctx, lockKey(name), m.nodeID)
		return err
	}

	h := &handle{eng: eng, stopCh: make(chan struct{})}
	m.mu.Lock()
	m.running[name] = h
	m.mu.Unlock()

	go m.refreshLock(name, h.stopCh)

	if err := eng.Start(context.Background()); err != nil {
		_ = m.Stop(ctx, name, false)
		return err
	}
	m.mu.Lock()
	if hh := m.running[name]; hh != nil {
		hh.jid = eng.JID()
	}
	m.mu.Unlock()
	_ = m.store.SetSessionStatus(ctx, name, string(eng.Status()), eng.JID())
	m.log.Info("sessao iniciada", "session", name, "engine", engName)
	return nil
}

func (m *Manager) Stop(ctx context.Context, name string, logout bool) error {
	return m.stop(ctx, name, logout, true)
}

// stop encerra a engine da sessao. persist=false (usado no shutdown) NAO
// altera o status no banco: o estado desejado continua o que era (ex.:
// WORKING) para que RestoreOwned reconecte no proximo boot.
func (m *Manager) stop(ctx context.Context, name string, logout, persist bool) error {
	m.mu.Lock()
	h, ok := m.running[name]
	if ok {
		delete(m.running, name)
	}
	m.mu.Unlock()
	if !ok {
		return ErrNotActive
	}

	close(h.stopCh)
	if logout {
		_ = h.eng.Logout(context.Background())
	} else {
		_ = h.eng.Stop()
	}
	_ = m.cache.ReleaseLock(ctx, lockKey(name), m.nodeID)

	if persist {
		status := string(engine.StatusStopped)
		if logout {
			status = string(engine.StatusLoggedOut)
		}
		_ = m.store.SetSessionStatus(context.Background(), name, status, "")
	}
	m.log.Info("sessao parada", "session", name, "logout", logout, "persist", persist)
	return nil
}

// StopAll e chamado no shutdown: solta locks e desconecta tudo, SEM marcar as
// sessoes como paradas — assim RestoreOwned as reconecta no proximo boot.
func (m *Manager) StopAll() {
	m.mu.RLock()
	names := make([]string, 0, len(m.running))
	for n := range m.running {
		names = append(names, n)
	}
	m.mu.RUnlock()
	for _, n := range names {
		_ = m.stop(context.Background(), n, false, false)
	}
}

// RestoreOwned reinicia, na subida, as sessoes que este no consegue travar
// e que estavam em execucao.
func (m *Manager) RestoreOwned(ctx context.Context) {
	recs, err := m.store.ListSessions(ctx)
	if err != nil {
		m.log.Error("restore: list sessions", "err", err)
		return
	}
	restorable := map[string]bool{
		string(engine.StatusWorking):  true,
		string(engine.StatusStarting): true,
		string(engine.StatusScanQR):   true, // estava pareando: retoma o fluxo
	}
	// reconecta em paralelo (limitado) — com muitas sessoes, sequencial
	// levaria minutos.
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for _, r := range recs {
		if !restorable[r.Status] {
			continue
		}
		r := r
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if err := m.start(ctx, r.Name, true); err != nil && !errors.Is(err, ErrLocked) {
				m.log.Warn("restore: falha ao iniciar", "session", r.Name, "err", err)
			} else {
				m.log.Info("restore: sessao reconectando", "session", r.Name, "era", r.Status)
			}
		}()
	}
	wg.Wait()
}

func (m *Manager) refreshLock(name string, stop <-chan struct{}) {
	t := time.NewTicker(lockRefresh)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			ok, err := m.cache.RenewLock(context.Background(), lockKey(name), m.nodeID, lockTTL)
			if err != nil || !ok {
				m.log.Error("perdi o lock da sessao, parando", "session", name, "err", err)
				go m.Stop(context.Background(), name, false)
				return
			}
		}
	}
}

// emit devolve o callback que a engine usa para publicar eventos, ja
// enriquecendo status/JID no store quando relevante.
func (m *Manager) emit(name, engName string) func(events.Event) {
	return func(e events.Event) {
		if e.Session == "" {
			e.Session = name
		}
		if e.Engine == "" {
			e.Engine = engName
		}
		observability.EventsPublished.WithLabelValues(e.Name).Inc()
		m.bus.Publish(e)

		switch e.Name {
		case "message":
			observability.MessagesReceived.WithLabelValues(name).Inc()
		case "message.any":
			if mp, ok := e.Payload.(map[string]any); ok {
				if fm, _ := mp["fromMe"].(bool); fm {
					observability.MessagesSent.WithLabelValues(name).Inc()
				}
			}
		}

		if e.Name == events.SessionStatus {
			if eng, ok := m.Engine(name); ok {
				jid := eng.JID()
				_ = m.store.SetSessionStatus(context.Background(), name, string(eng.Status()), jid)
				if jid != "" {
					m.mu.Lock()
					if hh := m.running[name]; hh != nil {
						hh.jid = jid
					}
					m.mu.Unlock()
				}
			}
		}
	}
}

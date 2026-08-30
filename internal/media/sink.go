package media

import (
	"bytes"
	"context"
	"log/slog"
	"time"

	"wa-gateway/internal/store"
)

// Policy diz, para uma sessão, se a mídia deve ser guardada e por quanto
// tempo. ttl<=0 => sem expiração automática.
type Policy struct {
	Disabled bool
	TTL      time.Duration
}

// PolicyFn resolve a política de uma sessão. Nil => guarda tudo, pra sempre.
type PolicyFn func(session string) Policy

// Sink adapta um Store + o banco para o contrato engine.MediaSink: guarda o
// binario no backend, registra em `media` e devolve a URL de download.
type Sink struct {
	store  Store
	db     *store.Store
	log    *slog.Logger
	policy PolicyFn
}

func NewSink(s Store, db *store.Store) *Sink { return &Sink{store: s, db: db} }

// SetPolicy liga o resolvedor de política por sessão (opt-out + TTL).
func (s *Sink) SetPolicy(fn PolicyFn) { s.policy = fn }

// SetLogger é opcional (usado pelo coletor).
func (s *Sink) SetLogger(l *slog.Logger) { s.log = l }

func (s *Sink) Enabled() bool { return s.store != nil && s.store.Enabled() }

func (s *Sink) WantStore(session string) bool { return !s.resolve(session).Disabled }

func (s *Sink) resolve(session string) Policy {
	if s.policy == nil {
		return Policy{}
	}
	return s.policy(session)
}

func (s *Sink) Store(ctx context.Context, session, msgID, mimetype string, data []byte) (string, int, error) {
	pol := s.resolve(session)
	if pol.Disabled {
		return "", 0, nil // sessão optou por não guardar mídia
	}
	obj, err := s.store.Put(ctx, session+"/"+msgID, mimetype, bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", 0, err
	}
	var exp *time.Time
	if pol.TTL > 0 {
		t := time.Now().Add(pol.TTL)
		exp = &t
	}
	_ = s.db.SaveMedia(ctx, store.MediaRecord{
		ID: msgID, Session: session, Mimetype: mimetype,
		Size: int64(len(data)), Backend: "s3", Ref: obj.Ref, ExpiresAt: exp,
	})
	return "/api/media/" + msgID, len(data), nil
}

// RunGC apaga em loop as mídias vencidas (expires_at < agora) do storage e do
// banco. No-op se o storage estiver desligado.
func (s *Sink) RunGC(ctx context.Context, every time.Duration) {
	if !s.Enabled() {
		return
	}
	if every <= 0 {
		every = 5 * time.Minute
	}
	t := time.NewTicker(every)
	defer t.Stop()
	s.sweep(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.sweep(ctx)
		}
	}
}

func (s *Sink) sweep(ctx context.Context) {
	rows, err := s.db.ExpiredMedia(ctx, 500)
	if err != nil {
		if s.log != nil {
			s.log.Warn("media gc: listar vencidas", "err", err)
		}
		return
	}
	if len(rows) == 0 {
		return
	}
	done := make([]string, 0, len(rows))
	for _, r := range rows {
		dctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		err := s.store.Delete(dctx, r.Ref)
		cancel()
		if err != nil {
			if s.log != nil {
				s.log.Warn("media gc: delete no storage", "ref", r.Ref, "err", err)
			}
			continue // tenta de novo na próxima rodada
		}
		done = append(done, r.ID)
	}
	if len(done) > 0 {
		_ = s.db.DeleteMedia(ctx, done)
		if s.log != nil {
			s.log.Info("media gc: apagadas", "n", len(done))
		}
	}
}

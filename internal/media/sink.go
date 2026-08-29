package media

import (
	"bytes"
	"context"

	"wa-gateway/internal/store"
)

// Sink adapta um Store + o banco para o contrato engine.MediaSink: guarda o
// binario no backend, registra em `media` e devolve a URL de download.
type Sink struct {
	store Store
	db    *store.Store
}

func NewSink(s Store, db *store.Store) *Sink { return &Sink{store: s, db: db} }

func (s *Sink) Enabled() bool { return s.store != nil && s.store.Enabled() }

func (s *Sink) Store(ctx context.Context, session, msgID, mimetype string, data []byte) (string, int, error) {
	obj, err := s.store.Put(ctx, session+"/"+msgID, mimetype, bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", 0, err
	}
	_ = s.db.SaveMedia(ctx, store.MediaRecord{
		ID: msgID, Session: session, Mimetype: mimetype,
		Size: int64(len(data)), Backend: "s3", Ref: obj.Ref,
	})
	return "/api/media/" + msgID, len(data), nil
}

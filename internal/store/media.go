package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type MediaRecord struct {
	ID        string    `json:"id"`
	Session   string    `json:"session"`
	Mimetype  string    `json:"mimetype"`
	Size      int64     `json:"size"`
	Backend   string    `json:"backend"`
	Ref       string    `json:"ref"`
	CreatedAt time.Time `json:"createdAt"`
}

func (s *Store) SaveMedia(ctx context.Context, m MediaRecord) error {
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO media (id, session, mimetype, size, backend, ref)
		 VALUES ($1,$2,$3,$4,$5,$6)
		 ON CONFLICT (id) DO NOTHING`,
		m.ID, m.Session, m.Mimetype, m.Size, m.Backend, m.Ref)
	return err
}

func (s *Store) GetMedia(ctx context.Context, id string) (MediaRecord, error) {
	var m MediaRecord
	var size *int64
	err := s.Pool.QueryRow(ctx,
		`SELECT id, session, mimetype, coalesce(size,0), backend, ref, created_at
		 FROM media WHERE id = $1`, id).
		Scan(&m.ID, &m.Session, &m.Mimetype, &size, &m.Backend, &m.Ref, &m.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return m, ErrNotFound
	}
	if size != nil {
		m.Size = *size
	}
	return m, err
}

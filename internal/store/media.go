package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type MediaRecord struct {
	ID        string     `json:"id"`
	Session   string     `json:"session"`
	Mimetype  string     `json:"mimetype"`
	Size      int64      `json:"size"`
	Backend   string     `json:"backend"`
	Ref       string     `json:"ref"`
	CreatedAt time.Time  `json:"createdAt"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}

func (s *Store) SaveMedia(ctx context.Context, m MediaRecord) error {
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO media (id, session, mimetype, size, backend, ref, expires_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)
		 ON CONFLICT (id) DO NOTHING`,
		m.ID, m.Session, m.Mimetype, m.Size, m.Backend, m.Ref, m.ExpiresAt)
	return err
}

// MediaRef e o mínimo pra apagar do storage + do banco.
type MediaRef struct{ ID, Ref, Session string }

// ExpiredMedia devolve mídias com expires_at já vencido (limitado).
func (s *Store) ExpiredMedia(ctx context.Context, limit int) ([]MediaRef, error) {
	if limit <= 0 || limit > 2000 {
		limit = 500
	}
	rows, err := s.Pool.Query(ctx,
		`SELECT id, ref, session FROM media
		 WHERE expires_at IS NOT NULL AND expires_at < now()
		 ORDER BY expires_at LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MediaRef
	for rows.Next() {
		var m MediaRef
		if err := rows.Scan(&m.ID, &m.Ref, &m.Session); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// DeleteMedia remove as linhas dadas (só o registro; o storage é apagado por quem chama).
func (s *Store) DeleteMedia(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := s.Pool.Exec(ctx, `DELETE FROM media WHERE id = ANY($1)`, ids)
	return err
}

// SessionMedia lista as mídias de uma sessão (ref + id), opcionalmente só as
// mais velhas que `olderThan`. Para o purge manual.
func (s *Store) SessionMedia(ctx context.Context, session string, olderThan time.Duration, limit int) ([]MediaRef, error) {
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	q := `SELECT id, ref, session FROM media WHERE session = $1`
	args := []any{session}
	if olderThan > 0 {
		q += ` AND created_at < now() - $2::interval`
		args = append(args, olderThan.String())
	}
	q += ` ORDER BY created_at LIMIT ` + itoa(limit)
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MediaRef
	for rows.Next() {
		var m MediaRef
		if err := rows.Scan(&m.ID, &m.Ref, &m.Session); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) GetMedia(ctx context.Context, id string) (MediaRecord, error) {
	var m MediaRecord
	var size *int64
	err := s.Pool.QueryRow(ctx,
		`SELECT id, session, mimetype, coalesce(size,0), backend, ref, created_at, expires_at
		 FROM media WHERE id = $1`, id).
		Scan(&m.ID, &m.Session, &m.Mimetype, &size, &m.Backend, &m.Ref, &m.CreatedAt, &m.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return m, ErrNotFound
	}
	if size != nil {
		m.Size = *size
	}
	return m, err
}

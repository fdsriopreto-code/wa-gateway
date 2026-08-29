package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrNotFound = errors.New("nao encontrado")

type SessionRecord struct {
	Name      string          `json:"name"`
	Engine    string          `json:"engine"`
	Status    string          `json:"status"`
	JID       string          `json:"jid,omitempty"`
	PushName  string          `json:"pushName,omitempty"`
	Config    json.RawMessage `json:"config"`
	Me        json.RawMessage `json:"me,omitempty"`
	CreatedAt time.Time       `json:"createdAt"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

func (s *Store) UpsertSession(ctx context.Context, name, engine string, config json.RawMessage) (SessionRecord, error) {
	if len(config) == 0 {
		config = json.RawMessage(`{}`)
	}
	const q = `
INSERT INTO sessions (name, engine, config)
VALUES ($1, $2, $3)
ON CONFLICT (name) DO UPDATE SET engine = EXCLUDED.engine, config = EXCLUDED.config, updated_at = now()
RETURNING name, engine, status, coalesce(jid,''), coalesce(push_name,''), config, me, created_at, updated_at`
	return scanSession(s.Pool.QueryRow(ctx, q, name, engine, config))
}

func (s *Store) GetSession(ctx context.Context, name string) (SessionRecord, error) {
	const q = `
SELECT name, engine, status, coalesce(jid,''), coalesce(push_name,''), config, me, created_at, updated_at
FROM sessions WHERE name = $1`
	rec, err := scanSession(s.Pool.QueryRow(ctx, q, name))
	if errors.Is(err, pgx.ErrNoRows) {
		return rec, ErrNotFound
	}
	return rec, err
}

func (s *Store) ListSessions(ctx context.Context) ([]SessionRecord, error) {
	const q = `
SELECT name, engine, status, coalesce(jid,''), coalesce(push_name,''), config, me, created_at, updated_at
FROM sessions ORDER BY name`
	rows, err := s.Pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SessionRecord
	for rows.Next() {
		rec, err := scanSessionRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *Store) DeleteSession(ctx context.Context, name string) error {
	ct, err := s.Pool.Exec(ctx, `DELETE FROM sessions WHERE name = $1`, name)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetSessionStatus(ctx context.Context, name, status, jid string) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE sessions SET status = $2, jid = NULLIF($3,''), updated_at = now() WHERE name = $1`,
		name, status, jid)
	return err
}

func (s *Store) SetSessionMe(ctx context.Context, name, pushName string, me json.RawMessage) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE sessions SET push_name = NULLIF($2,''), me = $3, updated_at = now() WHERE name = $1`,
		name, pushName, me)
	return err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanSession(r pgx.Row) (SessionRecord, error) { return scanSessionRows(r) }

func scanSessionRows(r rowScanner) (SessionRecord, error) {
	var rec SessionRecord
	err := r.Scan(&rec.Name, &rec.Engine, &rec.Status, &rec.JID, &rec.PushName,
		&rec.Config, &rec.Me, &rec.CreatedAt, &rec.UpdatedAt)
	return rec, err
}

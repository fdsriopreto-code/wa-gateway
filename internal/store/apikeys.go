package store

import (
	"context"
	"time"
)

type APIKeyRecord struct {
	ID         string     `json:"id"`
	Label      string     `json:"label,omitempty"`
	Scopes     []string   `json:"scopes"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	RevokedAt  *time.Time `json:"revokedAt,omitempty"`
}

func (s *Store) CreateAPIKey(ctx context.Context, id, hash, label string, scopes []string) error {
	if scopes == nil {
		scopes = []string{}
	}
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO api_keys (id, hash, label, scopes) VALUES ($1,$2,NULLIF($3,''),$4)`,
		id, hash, label, scopes)
	return err
}

func (s *Store) ListAPIKeys(ctx context.Context) ([]APIKeyRecord, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT id, coalesce(label,''), scopes, last_used_at, created_at, revoked_at
		 FROM api_keys ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []APIKeyRecord
	for rows.Next() {
		var k APIKeyRecord
		if err := rows.Scan(&k.ID, &k.Label, &k.Scopes, &k.LastUsedAt, &k.CreatedAt, &k.RevokedAt); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// RevokeAPIKey marca a chave como revogada (nao apaga: preserva auditoria).
func (s *Store) RevokeAPIKey(ctx context.Context, id string) error {
	ct, err := s.Pool.Exec(ctx,
		`UPDATE api_keys SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL`, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// CountSessionsByStatus agrupa as sessoes por status (para o dashboard).
func (s *Store) CountSessionsByStatus(ctx context.Context) (map[string]int, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT status, count(*) FROM sessions GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]int{}
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			return nil, err
		}
		out[status] = n
	}
	return out, rows.Err()
}

package store

import (
	"context"
	"time"
)

func (s *Store) CreateDelivery(ctx context.Context, id, session, url, event string) error {
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO webhook_deliveries (id, session, url, event) VALUES ($1,$2,$3,$4)
		 ON CONFLICT (id) DO NOTHING`,
		id, session, url, event)
	return err
}

func (s *Store) MarkDelivered(ctx context.Context, id string, attempts, code int) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE webhook_deliveries
		 SET status='delivered', attempts=$2, response_code=$3, delivered_at=now(), last_error=NULL
		 WHERE id=$1`,
		id, attempts, code)
	return err
}

func (s *Store) MarkFailed(ctx context.Context, id string, attempts, code int, errMsg string) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE webhook_deliveries
		 SET status='failed', attempts=$2, response_code=NULLIF($3,0), last_error=$4
		 WHERE id=$1`,
		id, attempts, code, errMsg)
	return err
}

type DeliveryRecord struct {
	ID           string    `json:"id"`
	Session      string    `json:"session"`
	URL          string    `json:"url"`
	Event        string    `json:"event"`
	Status       string    `json:"status"`
	Attempts     int       `json:"attempts"`
	ResponseCode int       `json:"responseCode,omitempty"`
	LastError    string    `json:"lastError,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	DeliveredAt  time.Time `json:"deliveredAt,omitempty"`
}

// ListDeliveries devolve as entregas de webhook mais recentes de uma sessao.
func (s *Store) ListDeliveries(ctx context.Context, session string, limit int) ([]DeliveryRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.Pool.Query(ctx,
		`SELECT id, session, url, event, status, attempts,
		        coalesce(response_code,0), coalesce(last_error,''), created_at, delivered_at
		 FROM webhook_deliveries WHERE session=$1 ORDER BY created_at DESC LIMIT $2`,
		session, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DeliveryRecord
	for rows.Next() {
		var d DeliveryRecord
		var deliveredAt *time.Time
		if err := rows.Scan(&d.ID, &d.Session, &d.URL, &d.Event, &d.Status, &d.Attempts,
			&d.ResponseCode, &d.LastError, &d.CreatedAt, &deliveredAt); err != nil {
			return nil, err
		}
		if deliveredAt != nil {
			d.DeliveredAt = *deliveredAt
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

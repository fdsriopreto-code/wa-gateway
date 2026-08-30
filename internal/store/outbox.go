package store

import (
	"context"
	"time"

	"wa-gateway/internal/outbox"
)

// OutboxRecorder implementa outbox.Recorder sobre a tabela outbox_jobs.
type OutboxRecorder struct{ s *Store }

func (s *Store) OutboxRecorder() *OutboxRecorder { return &OutboxRecorder{s: s} }

func (r *OutboxRecorder) Scheduled(ctx context.Context, j outbox.Job, runAt time.Time) error {
	_, err := r.s.Pool.Exec(ctx,
		`INSERT INTO outbox_jobs (id, session, kind, status, run_at)
		 VALUES ($1,$2,$3,'scheduled',$4)
		 ON CONFLICT (id) DO NOTHING`,
		j.ID, j.Session, string(j.Kind), runAt)
	return err
}

func (r *OutboxRecorder) Sent(ctx context.Context, id string, attempt int, messageID string) error {
	_, err := r.s.Pool.Exec(ctx,
		`UPDATE outbox_jobs
		 SET status='sent', attempts=$2, message_id=NULLIF($3,''), sent_at=now(), last_error=NULL
		 WHERE id=$1`,
		id, attempt, messageID)
	if cid, n, ok := ParseCampaignJobID(id); ok {
		r.s.campaignTargetSent(ctx, cid, n, messageID)
	}
	return err
}

func (r *OutboxRecorder) Failed(ctx context.Context, id string, attempt int, errMsg string) error {
	_, err := r.s.Pool.Exec(ctx,
		`UPDATE outbox_jobs SET status='failed', attempts=$2, last_error=$3 WHERE id=$1`,
		id, attempt, errMsg)
	if cid, n, ok := ParseCampaignJobID(id); ok {
		r.s.campaignTargetFailed(ctx, cid, n, errMsg)
	}
	return err
}

// ListOutboxJobs devolve os jobs mais recentes de uma sessao.
func (s *Store) ListOutboxJobs(ctx context.Context, session string, limit int) ([]OutboxJob, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.Pool.Query(ctx,
		`SELECT id, session, kind, status, run_at, attempts, coalesce(message_id,''), coalesce(last_error,''), created_at, sent_at
		 FROM outbox_jobs WHERE session=$1 ORDER BY created_at DESC LIMIT $2`,
		session, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []OutboxJob
	for rows.Next() {
		var j OutboxJob
		var sentAt *time.Time
		if err := rows.Scan(&j.ID, &j.Session, &j.Kind, &j.Status, &j.RunAt, &j.Attempts,
			&j.MessageID, &j.LastError, &j.CreatedAt, &sentAt); err != nil {
			return nil, err
		}
		if sentAt != nil {
			j.SentAt = *sentAt
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

type OutboxJob struct {
	ID        string    `json:"id"`
	Session   string    `json:"session"`
	Kind      string    `json:"kind"`
	Status    string    `json:"status"`
	RunAt     time.Time `json:"runAt"`
	Attempts  int       `json:"attempts"`
	MessageID string    `json:"messageId,omitempty"`
	LastError string    `json:"lastError,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	SentAt    time.Time `json:"sentAt,omitempty"`
}

package store

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// CampaignPace é o espaçamento pedido para a campanha (0 = usa o global).
type CampaignPace struct {
	MinIntervalMs int `json:"minIntervalMs,omitempty"`
	JitterMs      int `json:"jitterMs,omitempty"`
}

type Campaign struct {
	ID        string          `json:"id"`
	Session   string          `json:"session"`
	Name      string          `json:"name"`
	Status    string          `json:"status"` // running | done | stopped
	Kind      string          `json:"kind"`
	Args      json.RawMessage `json:"args"`
	Pace      CampaignPace    `json:"pace"`
	Total     int             `json:"total"`
	CreatedAt time.Time       `json:"createdAt"`
	UpdatedAt time.Time       `json:"updatedAt"`
	Counts    map[string]int  `json:"counts,omitempty"` // status do alvo -> qtd (no read)
}

type CampaignTarget struct {
	N         int       `json:"n"`
	ChatID    string    `json:"chatId"`
	Status    string    `json:"status"`
	MessageID string    `json:"messageId,omitempty"`
	Error     string    `json:"error,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// CampaignJobID monta o id do job da outbox para um alvo.
func CampaignJobID(campaignID string, n int) string {
	return "camp:" + campaignID + ":" + strconv.Itoa(n)
}

// ParseCampaignJobID extrai (campaignID, n) de um id "camp:<id>:<n>".
func ParseCampaignJobID(jobID string) (string, int, bool) {
	if !strings.HasPrefix(jobID, "camp:") {
		return "", 0, false
	}
	rest := jobID[len("camp:"):]
	i := strings.LastIndexByte(rest, ':')
	if i < 0 {
		return "", 0, false
	}
	n, err := strconv.Atoi(rest[i+1:])
	if err != nil {
		return "", 0, false
	}
	return rest[:i], n, true
}

// CreateCampaign grava a campanha e seus alvos numa transação.
func (s *Store) CreateCampaign(ctx context.Context, c Campaign, recipients []string) error {
	pace, _ := json.Marshal(c.Pace)
	args := c.Args
	if len(args) == 0 {
		args = json.RawMessage("{}")
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`INSERT INTO campaigns (id, session, name, status, kind, args, pace, total)
		 VALUES ($1,$2,$3,'running',$4,$5,$6,$7)`,
		c.ID, c.Session, c.Name, c.Kind, args, pace, len(recipients)); err != nil {
		return err
	}

	b := &pgx.Batch{}
	for i, to := range recipients {
		b.Queue(
			`INSERT INTO campaign_targets (campaign_id, n, chat_id) VALUES ($1,$2,$3)`,
			c.ID, i, to)
	}
	br := tx.SendBatch(ctx, b)
	for range recipients {
		if _, err := br.Exec(); err != nil {
			_ = br.Close()
			return err
		}
	}
	if err := br.Close(); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func scanCampaign(row pgx.Row) (Campaign, error) {
	var c Campaign
	var pace []byte
	err := row.Scan(&c.ID, &c.Session, &c.Name, &c.Status, &c.Kind, &c.Args, &pace,
		&c.Total, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return c, err
	}
	_ = json.Unmarshal(pace, &c.Pace)
	return c, nil
}

const campaignCols = `id, session, name, status, kind, args, pace, total, created_at, updated_at`

func (s *Store) GetCampaign(ctx context.Context, id string) (Campaign, error) {
	c, err := scanCampaign(s.Pool.QueryRow(ctx,
		`SELECT `+campaignCols+` FROM campaigns WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	if err != nil {
		return c, err
	}
	c.Counts, err = s.campaignCounts(ctx, id)
	return c, err
}

func (s *Store) campaignCounts(ctx context.Context, id string) (map[string]int, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT status, count(*) FROM campaign_targets WHERE campaign_id=$1 GROUP BY status`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var st string
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			return nil, err
		}
		out[st] = n
	}
	return out, rows.Err()
}

// ListCampaigns lista campanhas (session=="" => todas), mais recentes primeiro.
func (s *Store) ListCampaigns(ctx context.Context, session string, limit int) ([]Campaign, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var (
		rows pgx.Rows
		err  error
	)
	if session == "" {
		rows, err = s.Pool.Query(ctx,
			`SELECT `+campaignCols+` FROM campaigns ORDER BY created_at DESC LIMIT $1`, limit)
	} else {
		rows, err = s.Pool.Query(ctx,
			`SELECT `+campaignCols+` FROM campaigns WHERE session=$1 ORDER BY created_at DESC LIMIT $2`,
			session, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Campaign
	for rows.Next() {
		c, err := scanCampaign(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].Counts, err = s.campaignCounts(ctx, out[i].ID); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// RunningCampaigns devolve as campanhas ainda em andamento (para retomar no boot).
func (s *Store) RunningCampaigns(ctx context.Context) ([]Campaign, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT `+campaignCols+` FROM campaigns WHERE status='running' ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Campaign
	for rows.Next() {
		c, err := scanCampaign(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) SetCampaignStatus(ctx context.Context, id, status string) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE campaigns SET status=$2, updated_at=now() WHERE id=$1`, id, status)
	return err
}

// CampaignStatus devolve só o status ("" se não existe).
func (s *Store) CampaignStatus(ctx context.Context, id string) string {
	var st string
	_ = s.Pool.QueryRow(ctx, `SELECT status FROM campaigns WHERE id=$1`, id).Scan(&st)
	return st
}

// PendingTargets devolve os próximos alvos ainda não enfileirados, em ordem.
func (s *Store) PendingTargets(ctx context.Context, campaignID string, limit int) ([]CampaignTarget, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := s.Pool.Query(ctx,
		`SELECT n, chat_id FROM campaign_targets
		 WHERE campaign_id=$1 AND status='pending' ORDER BY n LIMIT $2`,
		campaignID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CampaignTarget
	for rows.Next() {
		var t CampaignTarget
		if err := rows.Scan(&t.N, &t.ChatID); err != nil {
			return nil, err
		}
		t.Status = "pending"
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) MarkTargetQueued(ctx context.Context, campaignID string, n int) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE campaign_targets SET status='queued', updated_at=now()
		 WHERE campaign_id=$1 AND n=$2 AND status='pending'`,
		campaignID, n)
	return err
}

// FinalizeCampaign marca 'done' se não há mais alvo pendente nem 'queued'
// (todos já viraram sent/failed). Devolve o status final.
func (s *Store) FinalizeCampaign(ctx context.Context, id string) (string, error) {
	var open int
	if err := s.Pool.QueryRow(ctx,
		`SELECT count(*) FROM campaign_targets
		 WHERE campaign_id=$1 AND status IN ('pending','queued')`, id).Scan(&open); err != nil {
		return "", err
	}
	if open > 0 {
		return "running", nil
	}
	if _, err := s.Pool.Exec(ctx,
		`UPDATE campaigns SET status='done', updated_at=now()
		 WHERE id=$1 AND status='running'`, id); err != nil {
		return "", err
	}
	return "done", nil
}

// campaignTargetSent/Failed são chamados pelo OutboxRecorder quando o job é de
// campanha (id "camp:<id>:<n>").
func (s *Store) campaignTargetSent(ctx context.Context, id string, n int, messageID string) {
	_, _ = s.Pool.Exec(ctx,
		`UPDATE campaign_targets SET status='sent', message_id=NULLIF($3,''), error=NULL, updated_at=now()
		 WHERE campaign_id=$1 AND n=$2`, id, n, messageID)
	_, _ = s.FinalizeCampaign(ctx, id)
}

func (s *Store) campaignTargetFailed(ctx context.Context, id string, n int, errMsg string) {
	_, _ = s.Pool.Exec(ctx,
		`UPDATE campaign_targets SET status='failed', error=$3, updated_at=now()
		 WHERE campaign_id=$1 AND n=$2`, id, n, errMsg)
	_, _ = s.FinalizeCampaign(ctx, id)
}

// FailedTargets devolve alvos que falharam (amostra, para o detalhe da campanha).
func (s *Store) FailedTargets(ctx context.Context, campaignID string, limit int) ([]CampaignTarget, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.Pool.Query(ctx,
		`SELECT n, chat_id, coalesce(error,'') FROM campaign_targets
		 WHERE campaign_id=$1 AND status='failed' ORDER BY n LIMIT $2`, campaignID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CampaignTarget
	for rows.Next() {
		t := CampaignTarget{Status: "failed"}
		if err := rows.Scan(&t.N, &t.ChatID, &t.Error); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

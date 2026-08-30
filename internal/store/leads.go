package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Lead é a visão de conversa de um contato, pronta pra um CRM.
type Lead struct {
	Session           string          `json:"session"`
	ChatID            string          `json:"chatId"`
	Phone             string          `json:"phone"`
	PushName          string          `json:"pushName,omitempty"`
	FirstContactAt    time.Time       `json:"firstContactAt"`
	LastInboundAt     *time.Time      `json:"lastInboundAt,omitempty"`
	LastOutboundAt    *time.Time      `json:"lastOutboundAt,omitempty"`
	LastReadByThemAt  *time.Time      `json:"lastReadByThemAt,omitempty"`
	InboundCount      int             `json:"inboundCount"`
	OutboundCount     int             `json:"outboundCount"`
	ResponseCount     int             `json:"responseCount"`
	Status            string          `json:"status"` // new | waiting_us | waiting_them | closed
	WaitingSince      *time.Time      `json:"waitingSince,omitempty"`
	Stage             string          `json:"stage,omitempty"`
	Owner             string          `json:"owner,omitempty"`
	Tags              []string        `json:"tags"`
	Notes             string          `json:"notes,omitempty"`
	Source            json.RawMessage `json:"source"`
	LastMessage       string          `json:"lastMessage,omitempty"`
	LastMessageFromMe bool            `json:"lastMessageFromMe"`
	UpdatedAt         time.Time       `json:"updatedAt"`

	// derivados (preenchidos no read):
	AvgResponseSeconds      int64 `json:"avgResponseSeconds"`
	WaitingSeconds          int64 `json:"waitingSeconds"`
	SecondsSinceLastInbound int64 `json:"secondsSinceLastInbound"`
}

const leadCols = `session, chat_id, phone, push_name, first_contact_at, last_inbound_at,
	last_outbound_at, last_read_by_them_at, inbound_count, outbound_count, response_count,
	response_seconds_total, status, waiting_since, stage, owner, tags, notes, source,
	last_message, last_message_from_me, updated_at`

func scanLead(row pgx.Row) (Lead, error) {
	var l Lead
	var respSecsTotal int64
	err := row.Scan(&l.Session, &l.ChatID, &l.Phone, &l.PushName, &l.FirstContactAt,
		&l.LastInboundAt, &l.LastOutboundAt, &l.LastReadByThemAt, &l.InboundCount,
		&l.OutboundCount, &l.ResponseCount, &respSecsTotal, &l.Status, &l.WaitingSince,
		&l.Stage, &l.Owner, &l.Tags, &l.Notes, &l.Source, &l.LastMessage,
		&l.LastMessageFromMe, &l.UpdatedAt)
	if err != nil {
		return l, err
	}
	if l.Tags == nil {
		l.Tags = []string{}
	}
	if len(l.Source) == 0 {
		l.Source = json.RawMessage("{}")
	}
	now := time.Now()
	if l.ResponseCount > 0 {
		l.AvgResponseSeconds = respSecsTotal / int64(l.ResponseCount)
	}
	if l.WaitingSince != nil {
		l.WaitingSeconds = int64(now.Sub(*l.WaitingSince).Seconds())
	}
	if l.LastInboundAt != nil {
		l.SecondsSinceLastInbound = int64(now.Sub(*l.LastInboundAt).Seconds())
	}
	return l, nil
}

// RecordInbound registra uma mensagem RECEBIDA. Devolve isNew=true se criou o
// lead agora. `source` é o JSON de atribuição (só é gravado no 1º contato).
func (s *Store) RecordInbound(ctx context.Context, session, chatID, phone, pushName, preview string, ts time.Time, source []byte) (bool, error) {
	if len(source) == 0 {
		source = []byte("{}")
	}
	var isNew bool
	err := s.Pool.QueryRow(ctx, `
INSERT INTO leads (session, chat_id, phone, push_name, first_contact_at, last_inbound_at,
	inbound_count, status, waiting_since, last_message, last_message_from_me, source, updated_at)
VALUES ($1,$2,$3,$4,$5,$5, 1,'waiting_us',$5,$6,false,$7, now())
ON CONFLICT (session, chat_id) DO UPDATE SET
	push_name = CASE WHEN EXCLUDED.push_name <> '' THEN EXCLUDED.push_name ELSE leads.push_name END,
	phone = CASE WHEN leads.phone = '' THEN EXCLUDED.phone ELSE leads.phone END,
	last_inbound_at = EXCLUDED.last_inbound_at,
	inbound_count = leads.inbound_count + 1,
	status = 'waiting_us',
	waiting_since = COALESCE(leads.waiting_since, EXCLUDED.waiting_since),
	stale_notified = CASE WHEN leads.status = 'waiting_us' THEN leads.stale_notified ELSE false END,
	last_message = EXCLUDED.last_message,
	last_message_from_me = false,
	source = CASE WHEN leads.source = '{}'::jsonb THEN EXCLUDED.source ELSE leads.source END,
	updated_at = now()
RETURNING (xmax = 0)`,
		session, chatID, phone, pushName, ts.UTC(), preview, source).Scan(&isNew)
	return isNew, err
}

// RecordOutbound registra uma mensagem ENVIADA por nós.
func (s *Store) RecordOutbound(ctx context.Context, session, chatID, phone, preview string, ts time.Time) error {
	_, err := s.Pool.Exec(ctx, `
INSERT INTO leads (session, chat_id, phone, first_contact_at, last_outbound_at,
	outbound_count, status, last_message, last_message_from_me, updated_at)
VALUES ($1,$2,$3,$4,$4, 1,'waiting_them',$5,true, now())
ON CONFLICT (session, chat_id) DO UPDATE SET
	last_outbound_at = EXCLUDED.last_outbound_at,
	outbound_count = leads.outbound_count + 1,
	response_count = leads.response_count + (CASE WHEN leads.status = 'waiting_us' THEN 1 ELSE 0 END),
	response_seconds_total = leads.response_seconds_total + (CASE
		WHEN leads.status = 'waiting_us' AND leads.waiting_since IS NOT NULL
		THEN GREATEST(0, EXTRACT(EPOCH FROM (EXCLUDED.last_outbound_at - leads.waiting_since))::bigint)
		ELSE 0 END),
	status = CASE WHEN leads.status = 'closed' THEN 'closed' ELSE 'waiting_them' END,
	waiting_since = NULL,
	stale_notified = false,
	last_message = EXCLUDED.last_message,
	last_message_from_me = true,
	updated_at = now()`,
		session, chatID, phone, ts.UTC(), preview)
	return err
}

// RecordRead: eles leram nossa mensagem (recibo read/played).
func (s *Store) RecordRead(ctx context.Context, session, chatID string, ts time.Time) error {
	_, err := s.Pool.Exec(ctx, `
UPDATE leads SET last_read_by_them_at = $3, updated_at = now()
WHERE session = $1 AND chat_id = $2
  AND (last_read_by_them_at IS NULL OR last_read_by_them_at < $3)`,
		session, chatID, ts.UTC())
	return err
}

// StaleLead é um lead que passou do limiar sem resposta.
type StaleLead struct {
	Session        string
	ChatID         string
	Phone          string
	PushName       string
	WaitingSince   time.Time
	WaitingSeconds int64
}

// ClaimStaleLeads marca como notificados e devolve os leads em waiting_us há
// mais que `after`. Atômico (RETURNING), seguro entre nós.
func (s *Store) ClaimStaleLeads(ctx context.Context, after time.Duration) ([]StaleLead, error) {
	secs := int64(after.Seconds())
	if secs < 30 {
		secs = 30
	}
	rows, err := s.Pool.Query(ctx, `
UPDATE leads SET stale_notified = true, updated_at = now()
WHERE status = 'waiting_us' AND stale_notified = false
  AND waiting_since IS NOT NULL AND waiting_since < now() - ($1 * interval '1 second')
RETURNING session, chat_id, phone, push_name, waiting_since,
  EXTRACT(EPOCH FROM (now() - waiting_since))::bigint`, secs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StaleLead
	for rows.Next() {
		var l StaleLead
		if err := rows.Scan(&l.Session, &l.ChatID, &l.Phone, &l.PushName, &l.WaitingSince, &l.WaitingSeconds); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// LeadFilter parametriza a listagem.
type LeadFilter struct {
	Status string
	Stage  string
	Tag    string
	Q      string
	Source string // "ad" | "utm" | "any"
	Sort   string // updated (padrão) | waiting | recent
	Limit  int
	Offset int
}

func (s *Store) ListLeads(ctx context.Context, session string, f LeadFilter) ([]Lead, error) {
	args := []any{session}
	where := []string{"session = $1"}
	add := func(cond string, val any) {
		args = append(args, val)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if f.Status != "" {
		add("status = $%d", f.Status)
	}
	if f.Stage != "" {
		add("stage = $%d", f.Stage)
	}
	if f.Tag != "" {
		add("tags @> ARRAY[$%d]::text[]", f.Tag)
	}
	if f.Q != "" {
		args = append(args, f.Q)
		n := len(args)
		where = append(where, fmt.Sprintf(
			"(phone ILIKE '%%'||$%d||'%%' OR push_name ILIKE '%%'||$%d||'%%' OR last_message ILIKE '%%'||$%d||'%%')",
			n, n, n))
	}
	switch f.Source {
	case "ad":
		where = append(where, "source ? 'adReferral'")
	case "utm":
		where = append(where, "source ? 'utm'")
	case "any":
		where = append(where, "(source ? 'adReferral' OR source ? 'utm' OR source ? 'clickIds')")
	}
	order := "updated_at DESC"
	switch f.Sort {
	case "waiting":
		order = "waiting_since ASC NULLS LAST, updated_at DESC"
	case "recent":
		order = "last_inbound_at DESC NULLS LAST"
	}
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	args = append(args, limit)
	q := fmt.Sprintf(`SELECT %s FROM leads WHERE %s ORDER BY %s LIMIT $%d`,
		leadCols, strings.Join(where, " AND "), order, len(args))
	if f.Offset > 0 {
		args = append(args, f.Offset)
		q += fmt.Sprintf(" OFFSET $%d", len(args))
	}
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Lead
	for rows.Next() {
		l, err := scanLead(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Store) GetLead(ctx context.Context, session, chatID string) (Lead, error) {
	l, err := scanLead(s.Pool.QueryRow(ctx,
		`SELECT `+leadCols+` FROM leads WHERE session = $1 AND chat_id = $2`, session, chatID))
	if errors.Is(err, pgx.ErrNoRows) {
		return l, ErrNotFound
	}
	return l, err
}

// LeadPatch são os campos de CRM que o operador controla.
type LeadPatch struct {
	Stage  *string   `json:"stage"`
	Owner  *string   `json:"owner"`
	Tags   *[]string `json:"tags"`
	Notes  *string   `json:"notes"`
	Status *string   `json:"status"` // só permite closed | waiting_us | waiting_them
}

func (s *Store) UpdateLeadCRM(ctx context.Context, session, chatID string, p LeadPatch) (Lead, error) {
	set := []string{"updated_at = now()"}
	args := []any{session, chatID}
	add := func(col string, val any) {
		args = append(args, val)
		set = append(set, fmt.Sprintf("%s = $%d", col, len(args)))
	}
	if p.Stage != nil {
		add("stage", strings.TrimSpace(*p.Stage))
	}
	if p.Owner != nil {
		add("owner", strings.TrimSpace(*p.Owner))
	}
	if p.Tags != nil {
		add("tags", *p.Tags)
	}
	if p.Notes != nil {
		add("notes", *p.Notes)
	}
	if p.Status != nil {
		st := strings.TrimSpace(*p.Status)
		if st == "closed" || st == "waiting_us" || st == "waiting_them" || st == "new" {
			add("status", st)
			if st == "closed" || st == "waiting_them" {
				set = append(set, "waiting_since = NULL")
			}
		}
	}
	q := fmt.Sprintf(`UPDATE leads SET %s WHERE session = $1 AND chat_id = $2 RETURNING %s`,
		strings.Join(set, ", "), leadCols)
	l, err := scanLead(s.Pool.QueryRow(ctx, q, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return l, ErrNotFound
	}
	return l, err
}

// LeadStats é o funil pronto pro dashboard do CRM.
type LeadStats struct {
	Total           int            `json:"total"`
	New             int            `json:"new"`
	WaitingUs       int            `json:"waitingUs"`
	WaitingThem     int            `json:"waitingThem"`
	Closed          int            `json:"closed"`
	Stale           int            `json:"stale"`
	FromAds         int            `json:"fromAds"`
	FromUTM         int            `json:"fromUtm"`
	AvgResponseSecs int64          `json:"avgResponseSeconds"`
	ByStage         map[string]int `json:"byStage"`
}

func (s *Store) LeadStats(ctx context.Context, session string, staleAfter time.Duration) (LeadStats, error) {
	secs := int64(staleAfter.Seconds())
	if secs < 30 {
		secs = 7200
	}
	var st LeadStats
	var respSecs, respCount int64
	err := s.Pool.QueryRow(ctx, `
SELECT
  count(*),
  count(*) FILTER (WHERE status='new'),
  count(*) FILTER (WHERE status='waiting_us'),
  count(*) FILTER (WHERE status='waiting_them'),
  count(*) FILTER (WHERE status='closed'),
  count(*) FILTER (WHERE status='waiting_us' AND waiting_since < now() - ($2 * interval '1 second')),
  count(*) FILTER (WHERE source ? 'adReferral'),
  count(*) FILTER (WHERE source ? 'utm'),
  coalesce(sum(response_seconds_total),0),
  coalesce(sum(response_count),0)
FROM leads WHERE session = $1`, session, secs).Scan(
		&st.Total, &st.New, &st.WaitingUs, &st.WaitingThem, &st.Closed,
		&st.Stale, &st.FromAds, &st.FromUTM, &respSecs, &respCount)
	if err != nil {
		return st, err
	}
	if respCount > 0 {
		st.AvgResponseSecs = respSecs / respCount
	}
	st.ByStage = map[string]int{}
	rows, err := s.Pool.Query(ctx,
		`SELECT stage, count(*) FROM leads WHERE session = $1 AND stage <> '' GROUP BY stage ORDER BY 2 DESC`, session)
	if err != nil {
		return st, err
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		var n int
		if err := rows.Scan(&k, &n); err != nil {
			return st, err
		}
		st.ByStage[k] = n
	}
	return st, rows.Err()
}

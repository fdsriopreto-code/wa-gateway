package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"wa-gateway/internal/engine"
)

type MessageRecord struct {
	Session   string          `json:"session"`
	ID        string          `json:"id"`
	ChatJID   string          `json:"chatId"`
	SenderJID string          `json:"senderJid,omitempty"`
	FromMe    bool            `json:"fromMe"`
	Type      string          `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	Body      string          `json:"body,omitempty"`
	PushName  string          `json:"pushName,omitempty"`
	Ack       int             `json:"ack"`
	Payload   json.RawMessage `json:"payload,omitempty"`

	// midia (para baixar depois)
	Media *engine.StoredMedia `json:"-"`
}

// SaveMessage grava/atualiza uma mensagem recebida ou enviada.
func (s *Store) SaveMessage(ctx context.Context, m MessageRecord) error {
	var mm engine.StoredMedia
	if m.Media != nil {
		mm = *m.Media
	}
	_, err := s.Pool.Exec(ctx, `
INSERT INTO messages (session, id, chat_jid, sender_jid, from_me, type, timestamp, body, push_name, payload, ack,
                      media_mimetype, media_filename, media_direct_path, media_key, media_file_sha256, media_file_enc_sha256, media_file_length)
VALUES ($1,$2,$3,NULLIF($4,''),$5,$6,$7,NULLIF($8,''),NULLIF($9,''),$10,$11,
        NULLIF($12,''),NULLIF($13,''),NULLIF($14,''),$15,$16,$17,NULLIF($18,0))
ON CONFLICT (session, id) DO UPDATE SET
  body = COALESCE(EXCLUDED.body, messages.body),
  ack  = GREATEST(messages.ack, EXCLUDED.ack),
  push_name = COALESCE(EXCLUDED.push_name, messages.push_name)`,
		m.Session, m.ID, m.ChatJID, m.SenderJID, m.FromMe, m.Type, m.Timestamp, m.Body, m.PushName, jsonOr(m.Payload), m.Ack,
		mm.Mimetype, mm.Filename, mm.DirectPath, nilBytes(mm.MediaKey), nilBytes(mm.FileSHA256), nilBytes(mm.FileEncSHA256), int64(mm.FileLength))
	return err
}

// SaveChat mantem a lista de conversas atualizada.
func (s *Store) SaveChat(ctx context.Context, session, jid, name string, isGroup bool, lastActivity time.Time) error {
	_, err := s.Pool.Exec(ctx, `
INSERT INTO chats (session, jid, name, is_group, updated_at)
VALUES ($1,$2,NULLIF($3,''),$4,$5)
ON CONFLICT (session, jid) DO UPDATE SET
  name = COALESCE(NULLIF(EXCLUDED.name,''), chats.name),
  is_group = EXCLUDED.is_group,
  updated_at = GREATEST(chats.updated_at, EXCLUDED.updated_at)`,
		session, jid, name, isGroup, lastActivity)
	return err
}

// UpdateAck aplica um recibo (2=entregue, 3=lido, 4=ouvido) as mensagens dadas.
func (s *Store) UpdateAck(ctx context.Context, session string, ids []string, ack int) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := s.Pool.Exec(ctx,
		`UPDATE messages SET ack = GREATEST(ack,$3) WHERE session=$1 AND id = ANY($2)`,
		session, ids, ack)
	return err
}

type ChatRow struct {
	JID      string    `json:"jid"`
	Name     string    `json:"name,omitempty"`
	IsGroup  bool      `json:"isGroup"`
	Unread   int       `json:"unread"`
	Archived bool      `json:"archived"`
	Updated  time.Time `json:"updatedAt"`
}

func (s *Store) ListChats(ctx context.Context, session string, limit int) ([]ChatRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.Pool.Query(ctx,
		`SELECT jid, coalesce(name,''), is_group, unread, archived, updated_at
		 FROM chats WHERE session=$1 ORDER BY updated_at DESC LIMIT $2`, session, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ChatRow
	for rows.Next() {
		var c ChatRow
		if err := rows.Scan(&c.JID, &c.Name, &c.IsGroup, &c.Unread, &c.Archived, &c.Updated); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) ListMessages(ctx context.Context, session, chatJID string, limit int, before time.Time) ([]MessageRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	q := `SELECT id, chat_jid, coalesce(sender_jid,''), from_me, type, timestamp, coalesce(body,''), coalesce(push_name,''), ack
	      FROM messages WHERE session=$1 AND chat_jid=$2`
	args := []any{session, chatJID}
	if !before.IsZero() {
		q += ` AND timestamp < $3`
		args = append(args, before)
	}
	args = append(args, limit)
	q += ` ORDER BY timestamp DESC LIMIT $` + itoa(len(args))
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MessageRecord
	for rows.Next() {
		var m MessageRecord
		m.Session = session
		if err := rows.Scan(&m.ID, &m.ChatJID, &m.SenderJID, &m.FromMe, &m.Type, &m.Timestamp, &m.Body, &m.PushName, &m.Ack); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// GetMessage devolve uma mensagem com os campos de midia (para download).
func (s *Store) GetMessage(ctx context.Context, session, id string) (MessageRecord, error) {
	var m MessageRecord
	m.Session, m.ID = session, id
	var mm engine.StoredMedia
	var flen *int64
	var dp, mime, fname *string
	err := s.Pool.QueryRow(ctx, `
SELECT chat_jid, coalesce(sender_jid,''), from_me, type, timestamp, coalesce(body,''), coalesce(push_name,''), ack,
       media_direct_path, media_mimetype, media_filename, media_key, media_file_sha256, media_file_enc_sha256, media_file_length
FROM messages WHERE session=$1 AND id=$2`, session, id).
		Scan(&m.ChatJID, &m.SenderJID, &m.FromMe, &m.Type, &m.Timestamp, &m.Body, &m.PushName, &m.Ack,
			&dp, &mime, &fname, &mm.MediaKey, &mm.FileSHA256, &mm.FileEncSHA256, &flen)
	if errors.Is(err, pgx.ErrNoRows) {
		return m, ErrNotFound
	}
	if err != nil {
		return m, err
	}
	if dp != nil {
		mm.DirectPath = *dp
	}
	if mime != nil {
		mm.Mimetype = *mime
	}
	if fname != nil {
		mm.Filename = *fname
	}
	if flen != nil {
		mm.FileLength = uint64(*flen)
	}
	mm.Type = m.Type
	if mm.DirectPath != "" {
		m.Media = &mm
	}
	return m, nil
}

func jsonOr(r json.RawMessage) []byte {
	if len(r) == 0 {
		return []byte("{}")
	}
	return r
}
func nilBytes(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	return b
}
func itoa(n int) string {
	if n <= 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

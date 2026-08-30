// Package inbox consome o barramento e persiste mensagens/chats recebidos e
// enviados no Postgres — destrava historico, download de midia antiga e
// (futuro) forward. Ligado por env MESSAGE_STORE (default on).
package inbox

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"time"

	"wa-gateway/internal/engine"
	"wa-gateway/internal/events"
	"wa-gateway/internal/store"
)

type Consumer struct {
	db     *store.Store
	log    *slog.Logger
	nodeID string
}

func New(db *store.Store, log *slog.Logger, nodeID string) *Consumer {
	return &Consumer{db: db, log: log, nodeID: nodeID}
}

// Run consome message.* do log duravel (Redis Stream, consumer group "inbox")
// ate o contexto ser cancelado. XACK so no sucesso: falha do Postgres deixa
// o evento pendente e ele e reprocessado.
func (c *Consumer) Run(ctx context.Context, stream *events.Stream) {
	stream.Consume(ctx, "inbox", c.nodeID, []string{"message.*"}, 4, c.handle)
}

func (c *Consumer) handle(ctx context.Context, e events.Event) error {
	p, ok := e.Payload.(map[string]any)
	if !ok {
		return nil
	}
	switch e.Name {
	case "message.any":
		return c.saveMessage(ctx, e.Session, p)
	case "message.ack":
		ids := strSlice(p["ids"])
		if len(ids) > 0 {
			return c.db.UpdateAck(ctx, e.Session, ids, ackNum(str(p["type"])))
		}
	}
	return nil
}

func (c *Consumer) saveMessage(ctx context.Context, session string, p map[string]any) error {
	id := str(p["id"])
	if id == "" {
		return nil
	}
	rec := store.MessageRecord{
		Session:   session,
		ID:        id,
		ChatJID:   str(p["chatId"]),
		SenderJID: str(p["from"]),
		FromMe:    boolOf(p["fromMe"]),
		Type:      str(p["type"]),
		Timestamp: time.Unix(i64(p["timestamp"]), 0),
		Body:      str(p["body"]),
		PushName:  str(p["pushName"]),
	}
	if mm, ok := p["mediaMeta"].(map[string]any); ok {
		rec.Media = &engine.StoredMedia{
			Type:          rec.Type,
			DirectPath:    str(mm["directPath"]),
			Mimetype:      str(mm["mimetype"]),
			Filename:      str(mm["filename"]),
			MediaKey:      bytesOf(mm["mediaKey"]),
			FileSHA256:    bytesOf(mm["fileSha256"]),
			FileEncSHA256: bytesOf(mm["fileEncSha256"]),
			FileLength:    u64(mm["fileLength"]),
		}
	}
	rec.Payload, _ = json.Marshal(p)

	if err := c.db.SaveMessage(ctx, rec); err != nil {
		c.log.Warn("inbox: save message", "err", err, "id", id)
		return err // deixa pendente no stream -> reprocessa
	}
	name := ""
	if !rec.FromMe {
		name = rec.PushName
	}
	if err := c.db.SaveChat(ctx, session, rec.ChatJID, name, boolOf(p["isGroup"]), rec.Timestamp); err != nil {
		c.log.Warn("inbox: save chat", "err", err) // secundário: não bloqueia o ack
	}
	return nil
}

func ackNum(t string) int {
	switch t {
	case "read", "read-self":
		return 3
	case "played":
		return 4
	default:
		return 2 // delivered
	}
}

/* ---- coercao de map[string]any ---- */

func str(v any) string {
	s, _ := v.(string)
	return s
}
func boolOf(v any) bool {
	b, _ := v.(bool)
	return b
}
func i64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	}
	return 0
}
func u64(v any) uint64 {
	switch n := v.(type) {
	case uint64:
		return n
	case int64:
		return uint64(n)
	case int:
		return uint64(n)
	case float64:
		return uint64(n)
	}
	return 0
}
func bytesOf(v any) []byte {
	switch b := v.(type) {
	case []byte:
		return b
	case string: // fallback: base64 (payload vindo de JSON)
		if dec, err := base64.StdEncoding.DecodeString(b); err == nil {
			return dec
		}
	}
	return nil
}
func strSlice(v any) []string {
	switch a := v.(type) {
	case []string:
		return a
	case []any:
		out := make([]string, 0, len(a))
		for _, x := range a {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

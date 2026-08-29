package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"

	"wa-gateway/internal/events"
)

// Envelope e o corpo POST enviado ao webhook do cliente (formato estilo WAHA).
type Envelope struct {
	ID        string            `json:"id"`
	Timestamp int64             `json:"timestamp"` // unix ms
	Session   string            `json:"session"`
	Engine    string            `json:"engine"`
	Event     string            `json:"event"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Payload   any               `json:"payload"`
}

func NewEnvelope(e events.Event, metadata map[string]string) Envelope {
	return Envelope{
		ID:        e.ID,
		Timestamp: e.Timestamp.UnixMilli(),
		Session:   e.Session,
		Engine:    e.Engine,
		Event:     e.Name,
		Metadata:  metadata,
		Payload:   e.Payload,
	}
}

// Sign devolve "sha256=<hex>" do HMAC-SHA256 do corpo cru.
func Sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

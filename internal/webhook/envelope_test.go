package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"wa-gateway/internal/events"
)

func TestSign(t *testing.T) {
	got := Sign("s3cr3t", []byte(`{"a":1}`))
	if !strings.HasPrefix(got, "sha256=") {
		t.Fatalf("prefixo errado: %s", got)
	}
	// confere contra um HMAC calculado a parte
	mac := hmac.New(sha256.New, []byte("s3cr3t"))
	mac.Write([]byte(`{"a":1}`))
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if got != want {
		t.Fatalf("assinatura = %s, want %s", got, want)
	}
	// segredo diferente -> assinatura diferente
	if Sign("outro", []byte(`{"a":1}`)) == got {
		t.Error("segredos diferentes produziram a mesma assinatura")
	}
}

func TestNewEnvelope(t *testing.T) {
	e := events.Event{
		ID:        "abc",
		Session:   "s1",
		Name:      "message",
		Engine:    "wa-gateway",
		Timestamp: time.Unix(1700000000, 0).UTC(),
		Payload:   map[string]any{"body": "oi"},
	}
	env := NewEnvelope(e, map[string]string{"env": "prod"})
	b, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if m["event"] != "message" || m["session"] != "s1" {
		t.Fatalf("envelope sem event/session: %s", b)
	}
	if _, ok := m["payload"]; !ok {
		t.Errorf("envelope sem payload: %s", b)
	}
}

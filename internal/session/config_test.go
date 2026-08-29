package session

import "testing"

func TestParseConfig(t *testing.T) {
	t.Run("vazio", func(t *testing.T) {
		c, err := ParseConfig(nil)
		if err != nil || c.Webhooks != nil || c.Outbox != nil || c.RawEvents {
			t.Fatalf("config vazia deveria zerar tudo: %+v err=%v", c, err)
		}
	})

	t.Run("completo", func(t *testing.T) {
		raw := []byte(`{
			"rawEvents": true,
			"outbox": { "minIntervalMs": 8000, "jitterMs": 3000, "dailyLimit": 500 },
			"webhooks": [
				{ "url": "https://x/hook", "events": ["message","session.status"], "hmac": { "secret": "s" } }
			]
		}`)
		c, err := ParseConfig(raw)
		if err != nil {
			t.Fatal(err)
		}
		if !c.RawEvents {
			t.Error("rawEvents deveria ser true")
		}
		if c.Outbox == nil || c.Outbox.MinIntervalMs != 8000 || c.Outbox.DailyLimit != 500 {
			t.Errorf("outbox errado: %+v", c.Outbox)
		}
		if len(c.Webhooks) != 1 || c.Webhooks[0].URL != "https://x/hook" ||
			len(c.Webhooks[0].Events) != 2 || c.Webhooks[0].HMAC == nil || c.Webhooks[0].HMAC.Secret != "s" {
			t.Errorf("webhook errado: %+v", c.Webhooks)
		}
	})

	t.Run("json invalido", func(t *testing.T) {
		if _, err := ParseConfig([]byte(`{bad`)); err == nil {
			t.Error("esperava erro de parse")
		}
	})
}

package session

import "encoding/json"

// Config e a configuracao de runtime de uma sessao, persistida em
// sessions.config (JSONB) e editavel por API.
type Config struct {
	Metadata map[string]string `json:"metadata,omitempty"`
	Webhooks []WebhookConfig   `json:"webhooks,omitempty"`
	Outbox   *OutboxConfig     `json:"outbox,omitempty"`
}

// OutboxConfig sobrescreve, para esta sessao, o pacing global da fila de
// saida. Campos zerados caem no default global.
type OutboxConfig struct {
	MinIntervalMs int `json:"minIntervalMs,omitempty"` // intervalo minimo entre envios
	JitterMs      int `json:"jitterMs,omitempty"`      // atraso aleatorio adicional
	DailyLimit    int `json:"dailyLimit,omitempty"`    // teto de envios por dia (0 = ilimitado)
}

type WebhookConfig struct {
	URL     string            `json:"url"`
	Events  []string          `json:"events"` // aceita wildcard: "message.*", "*"
	HMAC    *HMACConfig       `json:"hmac,omitempty"`
	Retries *RetryConfig      `json:"retries,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

type HMACConfig struct {
	Secret string `json:"secret"`
}

type RetryConfig struct {
	Attempts int    `json:"attempts,omitempty"`
	Backoff  string `json:"backoff,omitempty"` // ex.: "5s"
}

func ParseConfig(raw json.RawMessage) (Config, error) {
	var c Config
	if len(raw) == 0 {
		return c, nil
	}
	err := json.Unmarshal(raw, &c)
	return c, err
}

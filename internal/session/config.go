package session

import "encoding/json"

// Config e a configuracao de runtime de uma sessao, persistida em
// sessions.config (JSONB) e editavel por API.
type Config struct {
	Metadata map[string]string `json:"metadata,omitempty"`
	Webhooks []WebhookConfig   `json:"webhooks,omitempty"`
	Outbox   *OutboxConfig     `json:"outbox,omitempty"`
	// RawEvents inclui o struct cru da engine em payload.raw nos eventos de
	// mensagem/recibo. Default false (payload enxuto).
	RawEvents bool `json:"rawEvents,omitempty"`
	// AutoRead marca como lida (recibo azul) toda mensagem recebida.
	AutoRead bool `json:"autoRead,omitempty"`
	// AutoOnline mantem a sessao com presenca "available" apos conectar.
	AutoOnline bool `json:"autoOnline,omitempty"`
	// Cloud: credenciais da WhatsApp Cloud API. Preenchido = a sessao usa o
	// motor "cloud" (oficial da Meta) em vez do whatsmeow.
	Cloud *CloudConfig `json:"cloud,omitempty"`
}

// CloudConfig espelha engine.CloudConfig (o pacote session nao pode importar
// engine sem ciclo). O Manager converte um no outro.
type CloudConfig struct {
	PhoneNumberID string `json:"phoneNumberId"`
	AccessToken   string `json:"accessToken"`
	WABAID        string `json:"wabaId,omitempty"`
	GraphVersion  string `json:"graphVersion,omitempty"`
	VerifyToken   string `json:"verifyToken,omitempty"`
	AppSecret     string `json:"appSecret,omitempty"`
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

// MapWebhookSecrets aplica fn a cada webhooks[].hmac.secret do JSON de config
// e devolve o JSON reescrito. Usado para cifrar/decifrar em repouso. Config
// vazia ou sem secret volta igual.
func MapWebhookSecrets(raw json.RawMessage, fn func(string) string) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}
	var c Config
	if json.Unmarshal(raw, &c) != nil {
		return raw // não mexe no que não parseia
	}
	touched := false
	for i := range c.Webhooks {
		if c.Webhooks[i].HMAC != nil && c.Webhooks[i].HMAC.Secret != "" {
			c.Webhooks[i].HMAC.Secret = fn(c.Webhooks[i].HMAC.Secret)
			touched = true
		}
	}
	if !touched {
		return raw
	}
	out, err := json.Marshal(c)
	if err != nil {
		return raw
	}
	return out
}

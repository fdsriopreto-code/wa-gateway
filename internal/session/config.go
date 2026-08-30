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
	// Media: politica de armazenamento de midia recebida desta sessao.
	Media *MediaConfig `json:"media,omitempty"`
	// AutoReply: resposta automatica (fora de horario, saudacao, palavras-chave).
	AutoReply *AutoReplyConfig `json:"autoReply,omitempty"`
	// OTP: padroes do sistema de codigo de verificacao (/otp/send|verify).
	OTP *OTPConfig `json:"otp,omitempty"`
	// Leads: liga/desliga a coleta de metricas de CRM desta sessao (default on).
	Leads *LeadsConfig `json:"leads,omitempty"`
}

// LeadsConfig controla a coleta de leads/CRM por sessao.
type LeadsConfig struct {
	// Enabled: nil/true = coleta (default). false = nao mantem lead desta sessao.
	Enabled *bool `json:"enabled,omitempty"`
}

// OTPConfig sao os padroes por sessao do sistema de OTP. Cada campo 0/"" cai
// no default do servidor. O corpo da request pode sobrescrever template/brand.
type OTPConfig struct {
	Template           string `json:"template,omitempty"`           // usa {{code}} {{brand}} {{minutes}} {{ttl}}
	Brand              string `json:"brand,omitempty"`              // nome que aparece na mensagem
	CodeLength         int    `json:"codeLength,omitempty"`         // default 6 (4..10)
	TTLSeconds         int    `json:"ttlSeconds,omitempty"`         // default 300 (30..1800)
	MaxAttempts        int    `json:"maxAttempts,omitempty"`        // default 5 (1..10)
	ResendAfterSeconds int    `json:"resendAfterSeconds,omitempty"` // default 60
	HourlyCap          int    `json:"hourlyCap,omitempty"`          // default 5 envios/hora/numero (0 = ilimitado)
}

// AutoReplyConfig liga a resposta automatica para mensagens recebidas em
// conversas 1:1 (grupos ignorados). O consumidor le do log de eventos, entao
// funciona nos dois motores.
type AutoReplyConfig struct {
	Enabled bool `json:"enabled"`
	// OnlyOutsideHours: so responde quando esta FORA do horario comercial
	// definido em Hours. Sem Hours, isso nao tem efeito (responde sempre).
	OnlyOutsideHours bool         `json:"onlyOutsideHours,omitempty"`
	Hours            *OfficeHours `json:"hours,omitempty"`
	// Greeting: mandada uma vez por contato (re-manda depois de GreetingCooldownH).
	Greeting          string `json:"greeting,omitempty"`
	GreetingCooldownH int    `json:"greetingCooldownH,omitempty"` // default 24
	// Rules: primeira regra cujo "contains" casa (case-insensitive) responde.
	Rules []AutoReplyRule `json:"rules,omitempty"`
	// Fallback: resposta quando nada mais casou (opcional).
	Fallback string `json:"fallback,omitempty"`
}

type OfficeHours struct {
	TZ    string `json:"tz,omitempty"`    // ex.: "America/Sao_Paulo"; vazio = UTC
	Start string `json:"start,omitempty"` // "HH:MM"
	End   string `json:"end,omitempty"`   // "HH:MM"
	Days  []int  `json:"days,omitempty"`  // 0=Dom .. 6=Sab; vazio = seg-sex
}

type AutoReplyRule struct {
	Contains []string `json:"contains"`
	Reply    string   `json:"reply"`
}

// MediaConfig controla o que fazer com a midia recebida (quando MEDIA_BACKEND
// esta ligado).
type MediaConfig struct {
	// Store: guardar a midia no storage? nil/true = sim; false = descarta
	// (nao sobe pro S3, o evento vem so com mediaMeta pra baixar sob demanda).
	Store *bool `json:"store,omitempty"`
	// TTL: apaga a midia automaticamente depois desse tempo ("5m", "24h",
	// "720h"). Vazio/"0" = usa o padrao global (MEDIA_TTL) ou guarda pra
	// sempre.
	TTL string `json:"ttl,omitempty"`
	// Enrich: transcrever audio / descrever imagem recebidos e por o texto no
	// payload (transcript / imageCaption). nil = usa o padrao global
	// (MEDIA_ENRICH); precisa de AI_API_KEY no servidor.
	Enrich *bool `json:"enrich,omitempty"`
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

// MapSecrets aplica fn a todo campo sensível do JSON de config (HMAC secret
// de webhook, accessToken e appSecret da Cloud API) e devolve o JSON
// reescrito. Usado para cifrar/decifrar em repouso. Config vazia ou sem
// segredo volta igual (mesmo slice).
func MapSecrets(raw json.RawMessage, fn func(string) string) json.RawMessage {
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
	if c.Cloud != nil {
		if c.Cloud.AccessToken != "" {
			c.Cloud.AccessToken = fn(c.Cloud.AccessToken)
			touched = true
		}
		if c.Cloud.AppSecret != "" {
			c.Cloud.AppSecret = fn(c.Cloud.AppSecret)
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

// MapWebhookSecrets: compat. Use MapSecrets.
func MapWebhookSecrets(raw json.RawMessage, fn func(string) string) json.RawMessage {
	return MapSecrets(raw, fn)
}

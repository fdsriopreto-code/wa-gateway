// Package config carrega a configuracao de bootstrap a partir do ambiente.
// Config de runtime por sessao (webhooks, hmac, retries) vive no Postgres,
// ver internal/session.Config.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddr  string
	PublicURL string

	DatabaseURL string
	RedisURL    string

	APIKey        string // chave-mestra opcional (escopo "*")
	DefaultEngine string

	WebhookTimeout     time.Duration
	WebhookMaxAttempts int

	// Pacing global da fila de saida (anti-ban). Sobrescrevivel por sessao
	// via config.outbox.
	OutboxMinInterval time.Duration
	OutboxJitter      time.Duration
	OutboxDailyLimit  int

	// Persistencia de mensagens/chats recebidos no Postgres (historico,
	// download de midia antiga). "on" (padrao) | "off".
	MessageStore bool

	// Armazenamento de midia (opcional). MediaBackend: "none" (padrao) | "s3".
	MediaBackend    string
	S3Endpoint      string
	S3Region        string
	S3Bucket        string
	S3AccessKey     string
	S3SecretKey     string
	S3UseSSL        bool
	S3PathStyle     bool
	S3PublicBaseURL string

	LogLevel  string
	LogFormat string
	NodeID    string
}

func Load() (Config, error) {
	c := Config{
		HTTPAddr:           env("HTTP_ADDR", ":3000"),
		PublicURL:          env("PUBLIC_URL", "http://localhost:3000"),
		DatabaseURL:        env("DATABASE_URL", ""),
		RedisURL:           env("REDIS_URL", "redis://localhost:6379/0"),
		APIKey:             env("API_KEY", ""),
		DefaultEngine:      env("DEFAULT_ENGINE", "whatsmeow"),
		WebhookTimeout:     envDuration("WEBHOOK_TIMEOUT", 15*time.Second),
		WebhookMaxAttempts: envInt("WEBHOOK_MAX_ATTEMPTS", 15),
		OutboxMinInterval:  envDuration("OUTBOX_MIN_INTERVAL", 3*time.Second),
		OutboxJitter:       envDuration("OUTBOX_JITTER", 2*time.Second),
		OutboxDailyLimit:   envInt("OUTBOX_DAILY_LIMIT", 0),
		MessageStore:       env("MESSAGE_STORE", "on") != "off",
		MediaBackend:       env("MEDIA_BACKEND", "none"),
		S3Endpoint:         env("S3_ENDPOINT", ""),
		S3Region:           env("S3_REGION", "us-east-1"),
		S3Bucket:           env("S3_BUCKET", ""),
		S3AccessKey:        env("S3_ACCESS_KEY", ""),
		S3SecretKey:        env("S3_SECRET_KEY", ""),
		S3UseSSL:           envBool("S3_USE_SSL", false),
		S3PathStyle:        envBool("S3_PATH_STYLE", true),
		S3PublicBaseURL:    env("S3_PUBLIC_BASE_URL", ""),
		LogLevel:           env("LOG_LEVEL", "info"),
		LogFormat:          env("LOG_FORMAT", "text"),
		NodeID:             env("NODE_ID", ""),
	}

	if c.DatabaseURL == "" {
		return c, fmt.Errorf("DATABASE_URL e obrigatorio")
	}
	if c.NodeID == "" {
		host, _ := os.Hostname()
		c.NodeID = fmt.Sprintf("%s-%d", host, os.Getpid())
	}
	return c, nil
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envBool(k string, def bool) bool {
	switch os.Getenv(k) {
	case "1", "true", "TRUE", "yes", "on":
		return true
	case "0", "false", "FALSE", "no", "off":
		return false
	}
	return def
}

func envDuration(k string, def time.Duration) time.Duration {
	if v := os.Getenv(k); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

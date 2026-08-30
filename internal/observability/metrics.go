package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	HTTPRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "wa_http_requests_total",
		Help: "Total de requisicoes HTTP por rota e status.",
	}, []string{"method", "route", "status"})

	HTTPDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "wa_http_request_duration_seconds",
		Help:    "Latencia das requisicoes HTTP.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "route"})

	EventsPublished = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "wa_events_published_total",
		Help: "Eventos publicados no barramento interno por nome.",
	}, []string{"event"})

	BusDropped = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "wa_bus_dropped_total",
		Help: "Eventos descartados por assinante lento.",
	}, []string{"subscriber"})

	WebhookDeliveries = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "wa_webhook_deliveries_total",
		Help: "Resultado das entregas de webhook.",
	}, []string{"result"})

	SessionsActive = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "wa_sessions_active",
		Help: "Sessoes ativas neste no, por status.",
	}, []string{"status"})

	WSClients = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "wa_ws_clients",
		Help: "Clientes WebSocket conectados neste no.",
	})

	RateLimited = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "wa_rate_limited_total",
		Help: "Requisicoes rejeitadas por rate limit, por chave de API.",
	}, []string{"key"})

	MessagesSent = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "wa_messages_sent_total",
		Help: "Mensagens enviadas pela conta (fromMe), por sessao.",
	}, []string{"session"})

	MessagesReceived = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "wa_messages_received_total",
		Help: "Mensagens recebidas (nao fromMe), por sessao.",
	}, []string{"session"})
)

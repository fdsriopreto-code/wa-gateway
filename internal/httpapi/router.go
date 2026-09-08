package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"wa-gateway/internal/auth"
)

func NewRouter(d Deps, authn *auth.Authenticator) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	if len(d.CORSOrigins) > 0 {
		r.Use(corsMW(d.CORSOrigins))
	}
	if d.AccessLog && d.Log != nil {
		r.Use(accessLogMW(d.Log))
	}
	r.Use(metricsMW)

	// publico
	r.Get("/health", d.health)
	r.Get("/ready", d.ready)
	r.Get("/metrics", promhttp.Handler().ServeHTTP)
	r.Get("/api/version", d.version)
	r.Get("/openapi.json", d.openapiJSON)
	r.Get("/docs", d.docsPage)
	// webhook da WhatsApp Cloud API (a Meta chama; sem X-Api-Key)
	r.HandleFunc("/api/{session}/cloud/webhook", d.cloudWebhook)

	// autenticado
	r.Group(func(r chi.Router) {
		r.Use(authn.Middleware)
		r.Use(clusterProxyMW(d.Manager, d.Log))
		r.Use(sessionScopeMW)
		if d.Cache != nil && d.RateRPS > 0 {
			burst := d.RateBurst
			if burst <= 0 {
				burst = d.RateRPS * 2
			}
			if burst < 10 {
				burst = 10
			}
			r.Use(rateLimitMW(d.Cache, d.RateRPS, burst))
		}
		if d.Cache != nil {
			r.Use(idempotencyMW(d.Cache))
		}

		r.Route("/api/sessions", func(r chi.Router) {
			r.Get("/", d.listSessions)
			r.Post("/", d.createSession)
			r.Route("/{session}", func(r chi.Router) {
				r.Get("/", d.getSession)
				r.Put("/", d.updateSession)
				r.Delete("/", d.deleteSession)
				r.Post("/start", d.startSession)
				r.Post("/stop", d.stopSession)
				r.Post("/logout", d.logoutSession)
				r.Post("/restart", d.restartSession)
				r.Get("/me", d.sessionMe)
			})
		})

		r.Get("/api/{session}/auth/qr", d.sessionQR)
		r.Get("/api/{session}/auth/qr.png", d.sessionQRImage)
		r.Post("/api/sessions/{session}/auth/pair-code", d.pairCode)
		r.Post("/api/sessions/{session}/webhook/test", d.webhookTest)

		// --- perfil / conta ---
		r.Get("/api/{session}/me", d.me)
		r.Put("/api/{session}/profile/status", d.setProfileStatus)
		r.Post("/api/{session}/presence", d.globalPresence)
		r.Get("/api/{session}/blocklist", d.getBlocklist)
		r.Post("/api/{session}/block", d.setBlocked)

		// --- labels (WhatsApp Business) ---
		r.Get("/api/{session}/labels", d.listLabels)
		r.Post("/api/{session}/labels", d.editLabel)
		r.Post("/api/{session}/labels/chat", d.labelChat)
		r.Post("/api/{session}/labels/message", d.labelMessage)

		// --- dashboard / auditoria ---
		// --- leads / CRM ---
		r.Get("/api/{session}/leads", d.listLeads)
		r.Get("/api/{session}/leads/stats", d.leadStats)
		r.Get("/api/{session}/leads/{chatId}", d.getLead)
		r.Patch("/api/{session}/leads/{chatId}", d.patchLead)
		r.Post("/api/{session}/leads/{chatId}", d.patchLead)

		r.Get("/api/{session}/polls/{messageId}", d.getPollResult)

		// --- OTP (código de verificação) ---
		r.Post("/api/{session}/otp/send", d.otpSend)
		r.Post("/api/{session}/otp/verify", d.otpVerify)
		r.Post("/api/{session}/otp/cancel", d.otpCancel)

		// --- campanhas (envio em massa) ---
		r.Post("/api/{session}/campaign", d.createCampaign)
		r.Route("/api/campaigns", func(r chi.Router) {
			r.Get("/", d.listCampaigns)
			r.Get("/{id}", d.getCampaign)
			r.Post("/{id}/stop", d.stopCampaign)
		})

		r.Get("/api/stats", d.stats)
		r.Get("/api/cluster", d.cluster)
		r.Get("/api/deliveries", d.listDeliveries)
		r.Post("/api/deliveries/{id}/retry", d.retryDelivery)
		r.Route("/api/keys", func(r chi.Router) {
			r.Get("/", d.listKeys)
			r.Post("/", d.createKey)
			r.Delete("/{id}", d.revokeKey)
		})

		// --- envio ---
		r.Post("/api/sendText", d.sendText)
		r.Post("/api/sendImage", d.sendImage)
		r.Post("/api/sendFile", d.sendFile)
		r.Post("/api/sendVideo", d.sendVideo)
		r.Post("/api/sendAudio", d.sendAudio)
		r.Post("/api/sendSticker", d.sendSticker)
		r.Post("/api/sendLocation", d.sendLocation)
		r.Post("/api/sendContact", d.sendContact)
		r.Post("/api/sendPoll", d.sendPoll)
		r.Post("/api/sendInteractive", d.sendInteractive)
		r.Post("/api/sendTemplate", d.sendTemplate)

		// --- operacoes sobre mensagens ---
		r.Post("/api/reaction", d.reaction)
		r.Post("/api/deleteMessage", d.deleteMessage)
		r.Post("/api/editMessage", d.editMessage)
		r.Post("/api/sendSeen", d.sendSeen)
		r.Post("/api/presence", d.chatPresence)

		// --- fila de saida ---
		r.Get("/api/outbox", d.listOutbox)

		// --- midia ---
		r.Get("/api/media/{id}", d.getMedia)
		r.Delete("/api/media/{id}", d.deleteMedia)
		r.Post("/api/{session}/media/purge", d.purgeSessionMedia)

		// --- historico (store de mensagens) ---
		r.Get("/api/chats", d.listChats)
		r.Get("/api/chats/{chatId}/messages", d.chatMessages)
		r.Get("/api/messages/{id}/download", d.messageDownload)
		r.Post("/api/{session}/media/download", d.mediaDownloadDirect)
		r.Post("/api/forwardMessage", d.forwardMessage)

		// --- contatos ---
		r.Route("/api/contacts", func(r chi.Router) {
			r.Get("/", d.contactsList)
			r.Get("/check", d.contactsCheck)
			r.Get("/info", d.contactsInfo)
			r.Get("/profile-picture", d.contactsPicture)
		})

		// --- grupos ---
		r.Route("/api/groups", func(r chi.Router) {
			r.Get("/", d.listGroups)
			r.Post("/", d.createGroup)
			r.Post("/join", d.joinGroup)
			r.Route("/{jid}", func(r chi.Router) {
				r.Get("/", d.getGroup)
				r.Post("/leave", d.leaveGroup)
				r.Post("/participants", d.groupParticipants)
				r.Put("/name", d.setGroupName)
				r.Put("/topic", d.setGroupTopic)
				r.Put("/photo", d.setGroupPhoto)
				r.Put("/announce", d.setGroupAnnounce)
				r.Put("/locked", d.setGroupLocked)
				r.Get("/invite-link", d.groupInviteLink)
			})
		})

		r.Get("/ws", d.Hub.Handler)

		// --- MCP (agentes de IA) ---
		r.Post("/mcp", d.mcpHandler)
		r.Get("/mcp", d.mcpHandler)
	})

	// console web embarcado (estatico, sem auth — a chave vai do navegador)
	r.Handle("/*", spaHandler())

	return r
}

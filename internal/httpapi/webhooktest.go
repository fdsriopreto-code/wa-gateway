package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"wa-gateway/internal/session"
	"wa-gateway/internal/webhook"
)

// POST /api/sessions/{session}/webhook/test
// Envia um evento sintetico para cada webhook configurado e devolve o status
// de cada entrega — util pra validar a URL no n8n/backend antes de usar.
func (d Deps) webhookTest(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "session")
	rec, err := d.Manager.Get(r.Context(), name)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "sessao nao encontrada")
		return
	}
	cfg, err := session.ParseConfig(rec.Config)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_config", err.Error())
		return
	}
	if len(cfg.Webhooks) == 0 {
		writeErr(w, http.StatusBadRequest, "no_webhooks", "nenhum webhook configurado nesta sessao")
		return
	}

	body, _ := json.Marshal(map[string]any{
		"id":        "test-" + time.Now().Format("150405"),
		"timestamp": time.Now().UnixMilli(),
		"session":   name,
		"engine":    rec.Engine,
		"event":     "webhook.test",
		"metadata":  cfg.Metadata,
		"payload":   map[string]any{"message": "wa-gateway: teste de webhook", "ok": true},
	})

	client := &http.Client{Timeout: 8 * time.Second}
	type res struct {
		URL    string `json:"url"`
		Status int    `json:"status,omitempty"`
		OK     bool   `json:"ok"`
		Error  string `json:"error,omitempty"`
		MS     int64  `json:"ms"`
	}
	out := make([]res, 0, len(cfg.Webhooks))
	for _, wh := range cfg.Webhooks {
		if wh.URL == "" {
			continue
		}
		start := time.Now()
		req, _ := http.NewRequestWithContext(r.Context(), http.MethodPost, wh.URL, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "wa-gateway/webhook-test")
		req.Header.Set("X-Webhook-Id", "test")
		if wh.HMAC != nil && wh.HMAC.Secret != "" {
			req.Header.Set("X-Webhook-Signature", webhook.Sign(wh.HMAC.Secret, body))
		}
		for k, v := range wh.Headers {
			req.Header.Set(k, v)
		}
		item := res{URL: wh.URL}
		resp, err := client.Do(req)
		item.MS = time.Since(start).Milliseconds()
		if err != nil {
			item.Error = err.Error()
		} else {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			item.Status = resp.StatusCode
			item.OK = resp.StatusCode >= 200 && resp.StatusCode < 300
		}
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, out)
}

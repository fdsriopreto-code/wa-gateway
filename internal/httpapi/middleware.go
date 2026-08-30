package httpapi

import (
	"bytes"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"wa-gateway/internal/auth"
	"wa-gateway/internal/cache"
	"wa-gateway/internal/observability"
)

// idempotencyMW: POST com header `Idempotency-Key` e cacheado no Redis por
// 10 min. Retentativas (ex.: do n8n) devolvem a mesma resposta sem re-executar.
func idempotencyMW(rc *cache.Redis) func(http.Handler) http.Handler {
	type cached struct {
		Status int    `json:"s"`
		Body   string `json:"b"`
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get("Idempotency-Key")
			if key == "" || r.Method != http.MethodPost || rc == nil {
				next.ServeHTTP(w, r)
				return
			}
			p, _ := auth.FromContext(r.Context())
			ck := "idem:" + p.KeyID + ":" + key

			var hit cached
			if ok, _ := rc.GetJSON(r.Context(), ck, &hit); ok {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Idempotency-Replayed", "true")
				w.WriteHeader(hit.Status)
				_, _ = w.Write([]byte(hit.Body))
				return
			}
			cw := &captureWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(cw, r)
			if cw.status >= 200 && cw.status < 300 {
				_ = rc.SetJSON(r.Context(), ck, cached{Status: cw.status, Body: cw.buf.String()}, 10*time.Minute)
			}
		})
	}
}

type captureWriter struct {
	http.ResponseWriter
	status int
	buf    bytes.Buffer
	wrote  bool
}

func (c *captureWriter) WriteHeader(code int) {
	c.status = code
	c.wrote = true
	c.ResponseWriter.WriteHeader(code)
}
func (c *captureWriter) Write(b []byte) (int, error) {
	if !c.wrote {
		c.wrote = true
	}
	c.buf.Write(b)
	return c.ResponseWriter.Write(b)
}

// rateLimitMW aplica um token bucket por chave de API (Principal.KeyID) via
// Redis. rps<=0 desliga. A chave-mestra ("master") fica isenta. Fail-open:
// se o Redis nao responder, deixa passar.
func rateLimitMW(rc *cache.Redis, rps, burst float64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if rc == nil || rps <= 0 {
				next.ServeHTTP(w, r)
				return
			}
			p, ok := auth.FromContext(r.Context())
			if !ok || p.KeyID == "" || p.KeyID == "master" {
				next.ServeHTTP(w, r)
				return
			}
			allowed, remaining, retry, _ := rc.RateAllow(r.Context(), p.KeyID, rps, burst)
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(int(burst)))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
			if !allowed {
				secs := int(retry/time.Second) + 1
				w.Header().Set("Retry-After", strconv.Itoa(secs))
				observability.RateLimited.WithLabelValues(p.KeyID).Inc()
				writeJSON(w, http.StatusTooManyRequests, map[string]any{
					"error": "rate_limited", "message": "limite de requisicoes excedido para esta chave",
					"retryAfterMs": retry.Milliseconds(),
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// corsMW responde preflight e ecoa a origem quando ela esta na lista (ou "*").
func corsMW(origins []string) func(http.Handler) http.Handler {
	allowAll := false
	set := map[string]bool{}
	for _, o := range origins {
		if o == "*" {
			allowAll = true
		}
		set[o] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && (allowAll || set[origin]) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Api-Key, Authorization")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Max-Age", "600")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// accessLogMW loga uma linha estruturada por request.
func accessLogMW(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()
			next.ServeHTTP(ww, r)
			if strings.HasPrefix(r.URL.Path, "/health") || strings.HasPrefix(r.URL.Path, "/metrics") {
				return
			}
			log.Info("http",
				"method", r.Method, "path", r.URL.Path, "status", ww.Status(),
				"bytes", ww.BytesWritten(), "dur_ms", time.Since(start).Milliseconds(),
				"ip", r.Header.Get("X-Forwarded-For"))
		})
	}
}

// metricsMW registra contagem e latencia por rota (padrao chi).
func metricsMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()
		next.ServeHTTP(ww, r)

		route := "unknown"
		if rc := chi.RouteContext(r.Context()); rc != nil && rc.RoutePattern() != "" {
			route = rc.RoutePattern()
		}
		observability.HTTPRequests.WithLabelValues(r.Method, route, http.StatusText(ww.Status())).Inc()
		observability.HTTPDuration.WithLabelValues(r.Method, route).Observe(time.Since(start).Seconds())
	})
}

package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"wa-gateway/internal/auth"
	"wa-gateway/internal/cache"
	"wa-gateway/internal/netguard"
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

// sessionScopeMW barra chaves com escopo por sessao (`session:<name>`) de
// tocar sessoes fora do escopo. Chave irrestrita (`*` / `session:*`) e a
// chave-mestra passam sem custo (nem lê o corpo).
//
// Para chave escopada, confere TODA referência a sessão que um handler possa
// usar — path {session}, todos os ?session= e todas as chaves do corpo JSON
// cujo nome casa com "session" sem diferenciar maiúsculas (o encoding/json
// casa campo assim e fica com o ÚLTIMO valor repetido). Ler só o primeiro
// valor, ou só os primeiros KB do corpo, deixava uma chave usar a sessão de
// outro cliente.
//
// Chave sem "*" também liga o modo estrito anti-SSRF no contexto: URL que um
// tenant manda (mídia, link preview) não alcança a rede interna.
func sessionScopeMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := auth.FromContext(r.Context())
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		if !p.Can("*") {
			r = r.WithContext(netguard.WithStrict(r.Context()))
		}
		if p.Can("*") || p.Can("session:*") {
			next.ServeHTTP(w, r)
			return
		}
		names, status, err := requestSessions(r)
		if err != nil {
			writeJSON(w, status, map[string]any{"error": "bad_request", "message": err.Error()})
			return
		}
		if len(names) == 0 {
			if r.URL.Path == "/ws" { // stream de todas as sessões: barrado p/ chave escopada
				writeJSON(w, http.StatusForbidden, map[string]any{
					"error": "forbidden_session", "message": "chave com escopo por sessão precisa de ?session=<nome> no /ws",
				})
				return
			}
			next.ServeHTTP(w, r) // endpoint sem sessão (stats, keys, list…)
			return
		}
		for _, sess := range names {
			if sess == "*" || !p.CanSession(sess) {
				writeJSON(w, http.StatusForbidden, map[string]any{
					"error":   "forbidden_session",
					"message": "esta chave de API não tem acesso à sessão " + sess,
				})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// maxScopedBody é o teto do corpo que o sessionScopeMW lê inteiro (envio de
// mídia de 64MB em base64 cabe com folga).
const maxScopedBody = 128 << 20

// requestSessions junta todas as sessões citadas no request (path, query e
// chaves de topo do corpo JSON). O corpo é lido inteiro e restaurado.
func requestSessions(r *http.Request) ([]string, int, error) {
	var out []string
	if rc := chi.RouteContext(r.Context()); rc != nil {
		if s := rc.URLParam("session"); s != "" {
			out = append(out, s)
		}
	}
	for k, vs := range r.URL.Query() {
		if strings.EqualFold(k, "session") {
			out = append(out, vs...)
		}
	}
	if r.Body == nil || r.Body == http.NoBody {
		return out, 0, nil
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxScopedBody+1))
	_ = r.Body.Close()
	if err != nil {
		return nil, http.StatusBadRequest, fmt.Errorf("falha lendo o corpo: %w", err)
	}
	if len(body) > maxScopedBody {
		return nil, http.StatusRequestEntityTooLarge, fmt.Errorf("corpo maior que %d MB", maxScopedBody>>20)
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	trimmed := bytes.TrimLeft(body, " \t\r\n")
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return out, 0, nil // não é objeto JSON: nenhum handler tira sessão dele
	}
	fromBody, err := topLevelStrings(trimmed, "session")
	if err != nil {
		return nil, http.StatusBadRequest, fmt.Errorf("JSON inválido: %w", err)
	}
	return append(out, fromBody...), 0, nil
}

// topLevelStrings devolve todos os valores string das chaves de topo cujo
// nome casa com key ignorando maiúsculas (inclui repetidas). Valor não-string
// em chave que casa é recusado: não dá pra saber como o handler o leria.
func topLevelStrings(b []byte, key string) ([]string, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	var out []string
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return nil, err
		}
		name, _ := kt.(string)
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, err
		}
		if !strings.EqualFold(name, key) {
			continue
		}
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			if string(v) == "null" {
				continue
			}
			return nil, fmt.Errorf("campo %q precisa ser texto", name)
		}
		out = append(out, s)
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	return out, nil
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

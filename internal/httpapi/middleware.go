package httpapi

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"wa-gateway/internal/observability"
)

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

package httpapi

import "net/http"

func (d Deps) health(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	dbOK := d.Store.Pool.Ping(ctx) == nil
	status := "ok"
	code := http.StatusOK
	if !dbOK {
		status, code = "degraded", http.StatusServiceUnavailable
	}
	writeJSON(w, code, map[string]any{
		"status":   status,
		"database": dbOK,
		"version":  d.Version,
		"started":  d.StartedAt,
	})
}

func (d Deps) version(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"name":    "wa-gateway",
		"version": d.Version,
	})
}

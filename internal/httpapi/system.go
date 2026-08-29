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
		"commit":   d.Commit,
		"started":  d.StartedAt,
	})
}

// ready e o probe de readiness: 200 só quando o Postgres responde.
func (d Deps) ready(w http.ResponseWriter, r *http.Request) {
	if d.Store.Pool.Ping(r.Context()) != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ready": false, "database": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ready": true, "database": true})
}

func (d Deps) version(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"name":    "wa-gateway",
		"version": d.Version,
		"commit":  d.Commit,
	})
}

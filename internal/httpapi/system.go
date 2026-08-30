package httpapi

import "net/http"

func (d Deps) redisOK(r *http.Request) bool {
	if d.Cache == nil {
		return true // Redis não configurado neste processo: não bloqueia o probe
	}
	return d.Cache.Raw().Ping(r.Context()).Err() == nil
}

func (d Deps) health(w http.ResponseWriter, r *http.Request) {
	dbOK := d.Store.Pool.Ping(r.Context()) == nil
	redisOK := d.redisOK(r)
	status := "ok"
	code := http.StatusOK
	if !dbOK || !redisOK {
		status, code = "degraded", http.StatusServiceUnavailable
	}
	writeJSON(w, code, map[string]any{
		"status":   status,
		"database": dbOK,
		"redis":    redisOK,
		"version":  d.Version,
		"commit":   d.Commit,
		"started":  d.StartedAt,
	})
}

// ready e o probe de readiness: 200 só quando Postgres E Redis respondem.
func (d Deps) ready(w http.ResponseWriter, r *http.Request) {
	dbOK := d.Store.Pool.Ping(r.Context()) == nil
	redisOK := d.redisOK(r)
	if !dbOK || !redisOK {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ready": false, "database": dbOK, "redis": redisOK})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ready": true, "database": true, "redis": true})
}

func (d Deps) version(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"name":    "wa-gateway",
		"version": d.Version,
		"commit":  d.Commit,
	})
}

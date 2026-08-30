package httpapi

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"wa-gateway/internal/auth"
	"wa-gateway/internal/store"
)

// GET /api/media/{id}
//
// Faz stream do binario guardado. Com ?redirect=true devolve 302 para uma
// URL temporaria (ou publica) do backend, evitando passar pelo gateway.
func (d Deps) getMedia(w http.ResponseWriter, r *http.Request) {
	if d.Media == nil || !d.Media.Enabled() {
		writeErr(w, http.StatusServiceUnavailable, "media_disabled", "armazenamento de midia desligado")
		return
	}
	id := chi.URLParam(r, "id")
	rec, err := d.Store.GetMedia(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not_found", "midia nao encontrada")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	if r.URL.Query().Get("redirect") == "true" {
		url, err := d.Media.PresignedURL(r.Context(), rec.Ref, 15*time.Minute)
		if err != nil {
			writeErr(w, http.StatusBadGateway, "media_failed", err.Error())
			return
		}
		http.Redirect(w, r, url, http.StatusFound)
		return
	}

	body, mimetype, err := d.Media.Get(r.Context(), rec.Ref)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "media_failed", err.Error())
		return
	}
	defer body.Close()

	if mimetype == "" {
		mimetype = rec.Mimetype
	}
	w.Header().Set("Content-Type", mimetype)
	if rec.Size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(rec.Size, 10))
	}
	w.Header().Set("Cache-Control", "private, max-age=86400")
	_, _ = io.Copy(w, body)
}

// DELETE /api/media/{id} — apaga uma mídia do storage e do registro.
func (d Deps) deleteMedia(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rec, err := d.Store.GetMedia(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not_found", "midia nao encontrada")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if p, _ := auth.FromContext(r.Context()); !p.CanSession(rec.Session) {
		writeErr(w, http.StatusForbidden, "forbidden_session", "sem acesso à sessão "+rec.Session)
		return
	}
	if d.Media != nil {
		_ = d.Media.Delete(r.Context(), rec.Ref)
	}
	_ = d.Store.DeleteMedia(r.Context(), []string{id})
	writeJSON(w, http.StatusOK, map[string]any{"deleted": id})
}

// POST /api/{session}/media/purge — apaga TODAS as mídias da sessão (ou só as
// mais velhas que "olderThan", ex.: "168h").
func (d Deps) purgeSessionMedia(w http.ResponseWriter, r *http.Request) {
	session := chi.URLParam(r, "session")
	var req struct {
		OlderThan string `json:"olderThan"`
	}
	_ = decode(r, &req)
	var older time.Duration
	if req.OlderThan != "" {
		older, _ = time.ParseDuration(req.OlderThan)
	}
	rows, err := d.Store.SessionMedia(r.Context(), session, older, 5000)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	done := make([]string, 0, len(rows))
	for _, m := range rows {
		if d.Media != nil {
			if e := d.Media.Delete(r.Context(), m.Ref); e != nil {
				continue
			}
		}
		done = append(done, m.ID)
	}
	_ = d.Store.DeleteMedia(r.Context(), done)
	writeJSON(w, http.StatusOK, map[string]any{"deleted": len(done), "matched": len(rows)})
}

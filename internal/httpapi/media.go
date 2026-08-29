package httpapi

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

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

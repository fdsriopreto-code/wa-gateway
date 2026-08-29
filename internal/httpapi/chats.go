package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"wa-gateway/internal/store"
)

// GET /api/chats?session=&limit=
func (d Deps) listChats(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	sess := q.Get("session")
	if sess == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "session e obrigatorio")
		return
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	rows, err := d.Store.ListChats(r.Context(), sess, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if rows == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

// GET /api/chats/{chatId}/messages?session=&limit=&before=<rfc3339>
func (d Deps) chatMessages(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	sess := q.Get("session")
	if sess == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "session e obrigatorio")
		return
	}
	chatID := chi.URLParam(r, "chatId")
	limit, _ := strconv.Atoi(q.Get("limit"))
	var before time.Time
	if b := q.Get("before"); b != "" {
		if t, err := time.Parse(time.RFC3339, b); err == nil {
			before = t
		}
	}
	rows, err := d.Store.ListMessages(r.Context(), sess, chatID, limit, before)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if rows == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

// GET /api/messages/{id}/download?session=
// Baixa+descriptografa a midia de uma mensagem guardada.
func (d Deps) messageDownload(w http.ResponseWriter, r *http.Request) {
	sess := r.URL.Query().Get("session")
	if sess == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "session e obrigatorio")
		return
	}
	rec, err := d.Store.GetMessage(r.Context(), sess, chi.URLParam(r, "id"))
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not_found", "mensagem nao encontrada no store")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if rec.Media == nil {
		writeErr(w, http.StatusBadRequest, "no_media", "mensagem nao tem midia guardada")
		return
	}
	eng, ok := d.engineFor(w, sess)
	if !ok {
		return
	}
	data, mime, err := eng.DownloadMedia(r.Context(), *rec.Media)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "download_failed", err.Error())
		return
	}
	if mime == "" {
		mime = "application/octet-stream"
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	if rec.Media.Filename != "" {
		w.Header().Set("Content-Disposition", "inline; filename=\""+rec.Media.Filename+"\"")
	}
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, _ = w.Write(data)
}

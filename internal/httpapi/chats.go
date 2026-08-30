package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"wa-gateway/internal/engine"
	"wa-gateway/internal/outbox"
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

// POST /api/{session}/media/download
//
// Baixa+descriptografa a mídia direto dos campos `mediaMeta` que vêm no
// evento — não depende do store de mensagens (sem race com a persistência).
// Os campos de bytes chegam em base64 (JSON) e o Go decodifica sozinho.
func (d Deps) mediaDownloadDirect(w http.ResponseWriter, r *http.Request) {
	eng, ok := d.engineFor(w, chi.URLParam(r, "session"))
	if !ok {
		return
	}
	var req struct {
		Type          string `json:"type"`
		DirectPath    string `json:"directPath"`
		Mimetype      string `json:"mimetype"`
		Filename      string `json:"filename"`
		MediaKey      []byte `json:"mediaKey"`
		FileSHA256    []byte `json:"fileSha256"`
		FileEncSHA256 []byte `json:"fileEncSha256"`
		FileLength    uint64 `json:"fileLength"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if req.DirectPath == "" || len(req.MediaKey) == 0 {
		writeErr(w, http.StatusBadRequest, "bad_request", "directPath e mediaKey sao obrigatorios (use o mediaMeta do evento)")
		return
	}
	data, mime, err := eng.DownloadMedia(r.Context(), engine.StoredMedia{
		Type: req.Type, DirectPath: req.DirectPath, Mimetype: req.Mimetype, Filename: req.Filename,
		MediaKey: req.MediaKey, FileSHA256: req.FileSHA256, FileEncSHA256: req.FileEncSHA256, FileLength: req.FileLength,
	})
	if err != nil {
		writeErr(w, http.StatusBadGateway, "download_failed", err.Error())
		return
	}
	if mime == "" {
		mime = "application/octet-stream"
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	if req.Filename != "" {
		w.Header().Set("Content-Disposition", "inline; filename=\""+req.Filename+"\"")
	}
	_, _ = w.Write(data)
}

type forwardReq struct {
	queueOpts
	Session   string `json:"session"`
	ToChatID  string `json:"toChatId"`
	MessageID string `json:"messageId"`
}

// POST /api/forwardMessage  {session, toChatId, messageId}
// Encaminha uma mensagem guardada (texto ou mídia) para outro chat.
func (d Deps) forwardMessage(w http.ResponseWriter, r *http.Request) {
	var req forwardReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if req.ToChatID == "" || req.MessageID == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "toChatId e messageId sao obrigatorios")
		return
	}
	if req.Session == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "session e obrigatorio")
		return
	}
	rec, err := d.Store.GetMessage(r.Context(), req.Session, req.MessageID)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not_found", "mensagem nao encontrada no store (MESSAGE_STORE ligado?)")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	src := engine.ForwardSource{Type: rec.Type, Body: rec.Body, Media: rec.Media}
	if req.Enqueue {
		d.enqueue(w, r, req.Session, outbox.KindForward,
			outbox.Args{ChatID: req.ToChatID, Forward: &src}, req.Delay)
		return
	}
	eng, ok := d.engineFor(w, req.Session)
	if !ok {
		return
	}
	res, err := eng.Forward(r.Context(), req.ToChatID, src)
	sendResp(w, res, err)
}

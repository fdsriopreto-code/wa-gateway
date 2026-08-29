package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	qrcode "github.com/skip2/go-qrcode"

	"wa-gateway/internal/auth"
	"wa-gateway/internal/store"
)

// GET /api/stats — resumo para o dashboard.
func (d Deps) stats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	byStatus, err := d.Store.CountSessionsByStatus(ctx)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	total := 0
	for _, n := range byStatus {
		total += n
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"sessions": map[string]any{"total": total, "byStatus": byStatus},
		"database": d.Store.Pool.Ping(ctx) == nil,
		"version":  d.Version,
		"commit":   d.Commit,
		"started":  d.StartedAt,
	})
}

// GET /api/deliveries?session=&limit=
func (d Deps) listDeliveries(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	sess := q.Get("session")
	if sess == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "session e obrigatorio")
		return
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	recs, err := d.Store.ListDeliveries(r.Context(), sess, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if recs == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	writeJSON(w, http.StatusOK, recs)
}

// GET /api/{session}/auth/qr.png — QR atual como imagem.
func (d Deps) sessionQRImage(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "session")
	eng, ok := d.Manager.Engine(name)
	if !ok {
		writeErr(w, http.StatusConflict, "not_active", "sessao nao esta ativa neste no")
		return
	}
	code := eng.QR()
	if code == "" {
		writeErr(w, http.StatusNotFound, "no_qr", "nenhum QR disponivel (status="+string(eng.Status())+")")
		return
	}
	png, err := qrcode.Encode(code, qrcode.Medium, 320)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(png)
}

// ---- API keys ----

type createKeyReq struct {
	Label  string   `json:"label"`
	Scopes []string `json:"scopes"`
}

func (d Deps) listKeys(w http.ResponseWriter, r *http.Request) {
	if !canAdmin(r) {
		writeErr(w, http.StatusForbidden, "forbidden", "requer escopo *")
		return
	}
	keys, err := d.Store.ListAPIKeys(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if keys == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	writeJSON(w, http.StatusOK, keys)
}

func (d Deps) createKey(w http.ResponseWriter, r *http.Request) {
	if !canAdmin(r) {
		writeErr(w, http.StatusForbidden, "forbidden", "requer escopo *")
		return
	}
	var req createKeyReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if len(req.Scopes) == 0 {
		req.Scopes = []string{"*"}
	}
	keyID, secret, hash, err := auth.GenerateKey()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if err := d.Store.CreateAPIKey(r.Context(), keyID, hash, req.Label, req.Scopes); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	// o token completo so aparece aqui, uma vez.
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":     keyID,
		"token":  keyID + "." + secret,
		"scopes": req.Scopes,
		"label":  req.Label,
	})
}

func (d Deps) revokeKey(w http.ResponseWriter, r *http.Request) {
	if !canAdmin(r) {
		writeErr(w, http.StatusForbidden, "forbidden", "requer escopo *")
		return
	}
	err := d.Store.RevokeAPIKey(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not_found", "chave nao encontrada ou ja revogada")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func canAdmin(r *http.Request) bool {
	p, ok := auth.FromContext(r.Context())
	return ok && p.Can("*")
}

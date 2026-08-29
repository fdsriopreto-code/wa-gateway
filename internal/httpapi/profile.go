package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// POST /api/sessions/{session}/auth/pair-code   {phone}
// Pareamento por codigo (sem QR). A sessao precisa estar iniciada e nao pareada.
func (d Deps) pairCode(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "session")
	var req struct {
		Phone string `json:"phone"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if req.Phone == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "phone e obrigatorio (formato internacional, so digitos)")
		return
	}
	eng, ok := d.engineFor(w, name)
	if !ok {
		return
	}
	code, err := eng.PairPhone(r.Context(), req.Phone)
	if err != nil {
		writeErr(w, http.StatusConflict, "pair_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"code": code,
		"hint": "no celular: Aparelhos conectados > Conectar > Conectar com número > digite o código",
	})
}

// GET /api/{session}/me
func (d Deps) me(w http.ResponseWriter, r *http.Request) {
	eng, ok := d.engineFor(w, chi.URLParam(r, "session"))
	if !ok {
		return
	}
	m, err := eng.Me(r.Context())
	if err != nil {
		writeErr(w, http.StatusConflict, "not_connected", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// PUT /api/{session}/profile/status   {status}
func (d Deps) setProfileStatus(w http.ResponseWriter, r *http.Request) {
	eng, ok := d.engineFor(w, chi.URLParam(r, "session"))
	if !ok {
		return
	}
	var req struct {
		Status string `json:"status"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if err := eng.SetStatusMessage(r.Context(), req.Status); err != nil {
		writeErr(w, http.StatusBadGateway, "failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// POST /api/{session}/presence   {available}
func (d Deps) globalPresence(w http.ResponseWriter, r *http.Request) {
	eng, ok := d.engineFor(w, chi.URLParam(r, "session"))
	if !ok {
		return
	}
	var req struct {
		Available bool `json:"available"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if err := eng.SetPresence(r.Context(), req.Available); err != nil {
		writeErr(w, http.StatusBadGateway, "failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// GET /api/{session}/blocklist
func (d Deps) getBlocklist(w http.ResponseWriter, r *http.Request) {
	eng, ok := d.engineFor(w, chi.URLParam(r, "session"))
	if !ok {
		return
	}
	list, err := eng.Blocklist(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, "failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// POST /api/{session}/block   {jid, block}
func (d Deps) setBlocked(w http.ResponseWriter, r *http.Request) {
	eng, ok := d.engineFor(w, chi.URLParam(r, "session"))
	if !ok {
		return
	}
	var req struct {
		JID   string `json:"jid"`
		Block bool   `json:"block"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if req.JID == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "jid e obrigatorio")
		return
	}
	list, err := eng.SetBlocked(r.Context(), req.JID, req.Block)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

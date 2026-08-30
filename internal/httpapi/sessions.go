package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"wa-gateway/internal/auth"
	"wa-gateway/internal/session"
	"wa-gateway/internal/store"
)

type sessionView struct {
	Name     string          `json:"name"`
	Engine   string          `json:"engine"`
	Status   string          `json:"status"`
	JID      string          `json:"jid,omitempty"`
	PushName string          `json:"pushName,omitempty"`
	Config   json.RawMessage `json:"config"`
	QR       string          `json:"qr,omitempty"`
}

func (d Deps) view(rec store.SessionRecord) sessionView {
	v := sessionView{
		Name: rec.Name, Engine: rec.Engine, Status: rec.Status,
		JID: rec.JID, PushName: rec.PushName, Config: rec.Config,
	}
	if eng, ok := d.Manager.Engine(rec.Name); ok {
		v.Status = string(eng.Status())
		if q := eng.QR(); q != "" {
			v.QR = q
		}
		if j := eng.JID(); j != "" {
			v.JID = j
		}
	}
	return v
}

type upsertSessionReq struct {
	Name   string          `json:"name"`
	Config json.RawMessage `json:"config"`
	Start  bool            `json:"start"`
}

func (d Deps) listSessions(w http.ResponseWriter, r *http.Request) {
	recs, err := d.Manager.List(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	p, _ := auth.FromContext(r.Context())
	scoped := p.SessionScoped()
	out := make([]sessionView, 0, len(recs))
	for _, rec := range recs {
		if scoped && !p.CanSession(rec.Name) {
			continue // chave escopada só enxerga as sessões dela
		}
		out = append(out, d.view(rec))
	}
	writeJSON(w, http.StatusOK, out)
}

func (d Deps) createSession(w http.ResponseWriter, r *http.Request) {
	var req upsertSessionReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "name e obrigatorio")
		return
	}
	if p, _ := auth.FromContext(r.Context()); p.SessionScoped() {
		writeErr(w, http.StatusForbidden, "forbidden_session", "chave com escopo por sessão não pode criar sessão nova")
		return
	}
	if _, err := session.ParseConfig(req.Config); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "config invalida: "+err.Error())
		return
	}
	rec, err := d.Manager.Upsert(r.Context(), req.Name, req.Config)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if req.Start {
		if err := d.Manager.Start(r.Context(), req.Name); err != nil {
			writeErr(w, http.StatusConflict, "start_failed", err.Error())
			return
		}
		rec, _ = d.Manager.Get(r.Context(), req.Name)
	}
	writeJSON(w, http.StatusCreated, d.view(rec))
}

func (d Deps) getSession(w http.ResponseWriter, r *http.Request) {
	rec, err := d.Manager.Get(r.Context(), chi.URLParam(r, "session"))
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not_found", "sessao nao encontrada")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, d.view(rec))
}

func (d Deps) updateSession(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "session")
	var req upsertSessionReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if _, err := session.ParseConfig(req.Config); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "config invalida: "+err.Error())
		return
	}
	rec, err := d.Manager.Upsert(r.Context(), name, req.Config)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, d.view(rec))
}

func (d Deps) deleteSession(w http.ResponseWriter, r *http.Request) {
	err := d.Manager.Delete(r.Context(), chi.URLParam(r, "session"))
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not_found", "sessao nao encontrada")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d Deps) startSession(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "session")
	if err := d.Manager.Start(r.Context(), name); err != nil {
		status := http.StatusConflict
		if errors.Is(err, store.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeErr(w, status, "start_failed", err.Error())
		return
	}
	rec, _ := d.Manager.Get(r.Context(), name)
	writeJSON(w, http.StatusOK, d.view(rec))
}

func (d Deps) stopSession(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "session")
	if err := d.Manager.Stop(r.Context(), name, false); err != nil && !errors.Is(err, session.ErrNotActive) {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	rec, _ := d.Manager.Get(r.Context(), name)
	writeJSON(w, http.StatusOK, d.view(rec))
}

func (d Deps) logoutSession(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "session")
	if err := d.Manager.Stop(r.Context(), name, true); err != nil && !errors.Is(err, session.ErrNotActive) {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	rec, _ := d.Manager.Get(r.Context(), name)
	writeJSON(w, http.StatusOK, d.view(rec))
}

func (d Deps) restartSession(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "session")
	_ = d.Manager.Stop(r.Context(), name, false)
	if err := d.Manager.Start(r.Context(), name); err != nil {
		writeErr(w, http.StatusConflict, "start_failed", err.Error())
		return
	}
	rec, _ := d.Manager.Get(r.Context(), name)
	writeJSON(w, http.StatusOK, d.view(rec))
}

func (d Deps) sessionMe(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "session")
	eng, ok := d.Manager.Engine(name)
	if !ok {
		writeErr(w, http.StatusConflict, "not_active", "sessao nao esta ativa neste no")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"jid":    eng.JID(),
		"status": eng.Status(),
	})
}

func (d Deps) sessionQR(w http.ResponseWriter, r *http.Request) {
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
	writeJSON(w, http.StatusOK, map[string]string{"code": code})
}

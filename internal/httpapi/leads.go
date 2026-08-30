package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"wa-gateway/internal/store"
)

// chatArg normaliza o {chatId} da rota: aceita número puro ou JID completo.
func chatArg(r *http.Request) string {
	c := chi.URLParam(r, "chatId")
	if c == "" || strings.Contains(c, "@") {
		return c
	}
	d := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, c)
	if d == "" {
		return c
	}
	return d + "@s.whatsapp.net"
}

func (d Deps) listLeads(w http.ResponseWriter, r *http.Request) {
	session := chi.URLParam(r, "session")
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	f := store.LeadFilter{
		Status: q.Get("status"),
		Stage:  q.Get("stage"),
		Tag:    q.Get("tag"),
		Q:      strings.TrimSpace(q.Get("q")),
		Source: q.Get("source"),
		Sort:   q.Get("sort"),
		Limit:  limit,
		Offset: offset,
	}
	rows, err := d.Store.ListLeads(r.Context(), session, f)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (d Deps) getLead(w http.ResponseWriter, r *http.Request) {
	l, err := d.Store.GetLead(r.Context(), chi.URLParam(r, "session"), chatArg(r))
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not_found", "lead não encontrado")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, l)
}

func (d Deps) patchLead(w http.ResponseWriter, r *http.Request) {
	var p store.LeadPatch
	if err := decode(r, &p); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	l, err := d.Store.UpdateLeadCRM(r.Context(), chi.URLParam(r, "session"), chatArg(r), p)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not_found", "lead não encontrado")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, l)
}

func (d Deps) leadStats(w http.ResponseWriter, r *http.Request) {
	stale := d.LeadStale
	if stale <= 0 {
		stale = 2 * time.Hour
	}
	st, err := d.Store.LeadStats(r.Context(), chi.URLParam(r, "session"), stale)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, st)
}

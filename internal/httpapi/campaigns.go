package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"wa-gateway/internal/auth"
	"wa-gateway/internal/engine"
	"wa-gateway/internal/outbox"
	"wa-gateway/internal/store"
)

type campaignReq struct {
	Name          string   `json:"name"`
	Recipients    []string `json:"recipients"`
	Kind          string   `json:"kind"` // text|image|file|video|audio
	Text          string   `json:"text"`
	Caption       string   `json:"caption"`
	Mimetype      string   `json:"mimetype"`
	Filename      string   `json:"filename"`
	Data          string   `json:"data"` // base64 (aceita data URI) — p/ campanhas de mídia
	MinIntervalMs int      `json:"minIntervalMs"`
	JitterMs      int      `json:"jitterMs"`
}

const (
	maxRecipients   = 50000
	maxCampaignBlob = 5 << 20 // 5 MiB: a mídia de campanha é replicada por alvo na fila
)

var campaignKinds = map[string]bool{
	"text": true, "image": true, "file": true, "video": true, "audio": true,
}

func newCampaignID() string {
	var b [9]byte
	_, _ = rand.Read(b[:])
	return "cmp_" + hex.EncodeToString(b[:])
}

// normalizeRecipient limpa e converte um número/JID em chatId.
func normalizeRecipient(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if strings.Contains(s, "@") {
		return s
	}
	d := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
	if d == "" {
		return ""
	}
	return d + "@s.whatsapp.net"
}

func (d Deps) createCampaign(w http.ResponseWriter, r *http.Request) {
	if d.Campaigns == nil {
		writeErr(w, http.StatusNotImplemented, "not_supported", "campanhas desligadas (sem fila de saída)")
		return
	}
	sess := chi.URLParam(r, "session")
	var req campaignReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if _, err := d.Manager.Get(r.Context(), sess); err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "sessão não encontrada")
		return
	}
	kind := strings.ToLower(strings.TrimSpace(req.Kind))
	if kind == "" {
		kind = "text"
	}
	if !campaignKinds[kind] {
		writeErr(w, http.StatusBadRequest, "bad_request", "kind inválido para campanha: "+kind)
		return
	}

	seen := map[string]bool{}
	recips := make([]string, 0, len(req.Recipients))
	for _, raw := range req.Recipients {
		c := normalizeRecipient(raw)
		if c == "" || seen[c] {
			continue
		}
		seen[c] = true
		recips = append(recips, c)
	}
	if len(recips) == 0 {
		writeErr(w, http.StatusBadRequest, "bad_request", "recipients vazio (ou só números inválidos)")
		return
	}
	if len(recips) > maxRecipients {
		writeErr(w, http.StatusBadRequest, "bad_request", "recipients acima do limite ("+strconv.Itoa(maxRecipients)+")")
		return
	}

	args := outbox.Args{}
	if kind == "text" {
		if strings.TrimSpace(req.Text) == "" {
			writeErr(w, http.StatusBadRequest, "bad_request", "text obrigatório para campanha de texto")
			return
		}
		args.Text = req.Text
	} else {
		if req.Data == "" {
			writeErr(w, http.StatusBadRequest, "bad_request", "data (base64) obrigatório para campanha de mídia")
			return
		}
		data, detected, err := decodeB64(req.Data)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "bad_request", "data não é base64 válido")
			return
		}
		if len(data) > maxCampaignBlob {
			writeErr(w, http.StatusRequestEntityTooLarge, "too_large", "mídia de campanha limitada a 5 MiB")
			return
		}
		mime := req.Mimetype
		if mime == "" {
			mime = detected
		}
		args.Media = &engine.Media{Data: data, Mimetype: mime, Filename: req.Filename, Caption: req.Caption}
	}
	rawArgs, _ := json.Marshal(args)

	c := store.Campaign{
		ID:      newCampaignID(),
		Session: sess,
		Name:    strings.TrimSpace(req.Name),
		Kind:    kind,
		Args:    rawArgs,
		Pace:    store.CampaignPace{MinIntervalMs: req.MinIntervalMs, JitterMs: req.JitterMs},
	}
	if err := d.Store.CreateCampaign(r.Context(), c, recips); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	c.Total = len(recips)
	c.Status = "running"
	d.Campaigns.Start(c)
	writeJSON(w, http.StatusCreated, map[string]any{
		"id": c.ID, "session": sess, "total": c.Total, "status": "running",
	})
}

func (d Deps) listCampaigns(w http.ResponseWriter, r *http.Request) {
	sess := strings.TrimSpace(r.URL.Query().Get("session"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	cs, err := d.Store.ListCampaigns(r.Context(), sess, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	p, _ := auth.FromContext(r.Context())
	out := make([]store.Campaign, 0, len(cs))
	for _, c := range cs {
		if p.SessionScoped() && !p.CanSession(c.Session) {
			continue
		}
		out = append(out, c)
	}
	writeJSON(w, http.StatusOK, out)
}

func (d Deps) getCampaign(w http.ResponseWriter, r *http.Request) {
	c, err := d.Store.GetCampaign(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not_found", "campanha não encontrada")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if p, _ := auth.FromContext(r.Context()); p.SessionScoped() && !p.CanSession(c.Session) {
		writeErr(w, http.StatusForbidden, "forbidden_session", "sem acesso à sessão desta campanha")
		return
	}
	failed, _ := d.Store.FailedTargets(r.Context(), c.ID, 50)
	writeJSON(w, http.StatusOK, map[string]any{"campaign": c, "failed": failed})
}

func (d Deps) stopCampaign(w http.ResponseWriter, r *http.Request) {
	if d.Campaigns == nil {
		writeErr(w, http.StatusNotImplemented, "not_supported", "campanhas desligadas")
		return
	}
	id := chi.URLParam(r, "id")
	c, err := d.Store.GetCampaign(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not_found", "campanha não encontrada")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if p, _ := auth.FromContext(r.Context()); p.SessionScoped() && !p.CanSession(c.Session) {
		writeErr(w, http.StatusForbidden, "forbidden_session", "sem acesso à sessão desta campanha")
		return
	}
	if err := d.Campaigns.Stop(r.Context(), id); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "status": "stopped"})
}

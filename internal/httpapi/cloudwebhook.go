package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// cloudIngester é o que uma engine "cloud" expõe para o webhook da Meta.
type cloudIngester interface {
	VerifyToken() string
	AppSecret() string
	Ingest([]byte)
}

// GET/POST /api/{session}/cloud/webhook — endpoint PÚBLICO que a Meta chama.
// GET: responde hub.challenge se hub.verify_token bate.
// POST: valida X-Hub-Signature-256 (se appSecret setado) e ingere o payload.
func (d Deps) cloudWebhook(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "session")
	eng, ok := d.Manager.Engine(name)
	if !ok {
		http.Error(w, "sessao nao ativa", http.StatusNotFound)
		return
	}
	ci, ok := eng.(cloudIngester)
	if !ok {
		http.Error(w, "sessao nao e cloud api", http.StatusBadRequest)
		return
	}

	if r.Method == http.MethodGet {
		q := r.URL.Query()
		if q.Get("hub.mode") == "subscribe" && q.Get("hub.verify_token") == ci.VerifyToken() && ci.VerifyToken() != "" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(q.Get("hub.challenge")))
			return
		}
		http.Error(w, "verify token invalido", http.StatusForbidden)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		http.Error(w, "read", http.StatusBadRequest)
		return
	}
	if sec := ci.AppSecret(); sec != "" {
		got := r.Header.Get("X-Hub-Signature-256")
		mac := hmac.New(sha256.New, []byte(sec))
		mac.Write(body)
		want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		if !hmac.Equal([]byte(strings.TrimSpace(got)), []byte(want)) {
			http.Error(w, "assinatura invalida", http.StatusUnauthorized)
			return
		}
	}
	// responde 200 imediatamente; o processamento (incl. download de mídia)
	// é assíncrono dentro do Ingest.
	w.WriteHeader(http.StatusOK)
	go ci.Ingest(body)
}

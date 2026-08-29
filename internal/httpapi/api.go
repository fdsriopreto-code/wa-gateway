// Package httpapi expoe a REST + WebSocket. Rotas espelham a superficie do
// WAHA onde faz sentido, para facilitar migracao de clientes.
package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"wa-gateway/internal/cache"
	"wa-gateway/internal/engine"
	"wa-gateway/internal/media"
	"wa-gateway/internal/outbox"
	"wa-gateway/internal/session"
	"wa-gateway/internal/store"
	"wa-gateway/internal/ws"
)

type Deps struct {
	Manager     *session.Manager
	Store       *store.Store
	Hub         *ws.Hub
	Queue       *outbox.Queue // fila de saida com pacing; pode ser nil
	Media       media.Store   // armazenamento de midia; pode ser nil/Disabled
	Cache       *cache.Redis  // p/ idempotencia; pode ser nil
	Log         *slog.Logger
	Version     string
	Commit      string
	StartedAt   string
	CORSOrigins []string
	AccessLog   bool
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]string{"error": code, "message": msg})
}

func decode(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

// engineFor resolve a engine viva da sessao neste no, ja escrevendo o erro
// HTTP apropriado se ela nao existir.
func (d Deps) engineFor(w http.ResponseWriter, session string) (engine.Engine, bool) {
	if session == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "session e obrigatorio")
		return nil, false
	}
	eng, ok := d.Manager.Engine(session)
	if !ok {
		writeErr(w, http.StatusConflict, "not_active", "sessao nao esta ativa neste no")
		return nil, false
	}
	return eng, true
}

// decodeB64 aceita base64 puro ou data URI ("data:<mime>;base64,<...>") e
// devolve os bytes + o mimetype detectado (vazio se nao houver no data URI).
func decodeB64(raw string) (data []byte, mimetype string, err error) {
	if strings.HasPrefix(raw, "data:") {
		if i := strings.Index(raw, ","); i > 0 {
			mimetype = strings.TrimSuffix(strings.TrimPrefix(raw[:i], "data:"), ";base64")
			raw = raw[i+1:]
		}
	}
	data, err = base64.StdEncoding.DecodeString(strings.TrimSpace(raw))
	return data, mimetype, err
}

// sendResp e a resposta padrao de qualquer envio: 201 com o SendResult, ou
// 502 se a engine falhou.
func sendResp(w http.ResponseWriter, res engine.SendResult, err error) {
	if err != nil {
		writeErr(w, http.StatusBadGateway, "send_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

// Package cloud implementa engine.Engine sobre a WhatsApp Cloud API oficial
// da Meta (Graph API). Ao contrário do whatsmeow, aqui não há QR nem
// websocket: a sessão é um par (phoneNumberID, accessToken) e os eventos
// chegam por um webhook que a Meta chama — ver webhook.go.
//
// Só um subconjunto do contrato faz sentido na Cloud API: envio (inclusive
// botões/lista/template), recibos, download de mídia e perfil de negócio. O
// resto devolve engine.ErrNotSupported.
package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"wa-gateway/internal/engine"
	"wa-gateway/internal/events"
)

func init() { engine.Register("cloud", New) }

const defaultGraphVersion = "v21.0"

type Engine struct {
	deps engine.Deps
	cfg  engine.CloudConfig
	http *http.Client

	mu     sync.RWMutex
	status engine.Status

	// pool de download de mídia recebida: tira o fetch do caminho da resposta
	// do webhook (a Meta espera 200 rápido).
	mediaJobs chan mediaJob
	mediaWG   sync.WaitGroup
}

type mediaJob struct {
	p       map[string]any
	mediaID string
	mime    string
	msgID   string
}

const mediaWorkers = 6

func New(deps engine.Deps) (engine.Engine, error) {
	if deps.Cloud == nil {
		return nil, fmt.Errorf("cloud: faltam credenciais (config.cloud)")
	}
	c := *deps.Cloud
	if c.GraphVersion == "" {
		c.GraphVersion = defaultGraphVersion
	}
	return &Engine{
		deps:   deps,
		cfg:    c,
		http:   &http.Client{Timeout: 30 * time.Second},
		status: engine.StatusStopped,
	}, nil
}

func (e *Engine) Name() string { return "cloud" }

func (e *Engine) Status() engine.Status {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.status
}

func (e *Engine) setStatus(s engine.Status) {
	e.mu.Lock()
	changed := e.status != s
	e.status = s
	e.mu.Unlock()
	if changed {
		e.emit(events.SessionStatus, map[string]any{"status": string(s)})
	}
}

func (e *Engine) QR() string { return "" }

func (e *Engine) JID() string {
	// não há JID; devolvemos o phone number id pra rastreabilidade.
	return e.cfg.PhoneNumberID
}

// Start valida o token consultando o próprio número. Sucesso => WORKING.
func (e *Engine) Start(ctx context.Context) error {
	e.setStatus(engine.StatusStarting)
	var out struct {
		VerifiedName    string `json:"verified_name"`
		DisplayNumber   string `json:"display_phone_number"`
		QualityRating   string `json:"quality_rating"`
		PlatformType    string `json:"platform_type"`
		CodeVerifStatus string `json:"code_verification_status"`
	}
	if err := e.graphGET(ctx, "/"+e.cfg.PhoneNumberID, nil, &out); err != nil {
		e.setStatus(engine.StatusFailed)
		return fmt.Errorf("cloud: token inválido ou número inacessível: %w", err)
	}
	e.mu.Lock()
	e.mediaJobs = make(chan mediaJob, 256)
	e.mu.Unlock()
	for i := 0; i < mediaWorkers; i++ {
		e.mediaWG.Add(1)
		go e.mediaWorker()
	}

	e.setStatus(engine.StatusWorking)
	e.deps.Logger.Info("cloud api conectada", "number", out.DisplayNumber, "name", out.VerifiedName)
	return nil
}

func (e *Engine) mediaWorker() {
	defer e.mediaWG.Done()
	e.mu.RLock()
	jobs := e.mediaJobs
	e.mu.RUnlock()
	for j := range jobs {
		e.attachMedia(j.p, j.mediaID, j.mime, j.msgID)
		e.emitMessage(j.p)
	}
}

func (e *Engine) Stop() error {
	e.mu.Lock()
	jobs := e.mediaJobs
	e.mediaJobs = nil
	e.mu.Unlock()
	if jobs != nil {
		close(jobs)
		e.mediaWG.Wait()
	}
	e.setStatus(engine.StatusStopped)
	return nil
}

func (e *Engine) Logout(context.Context) error { return e.Stop() }

func (e *Engine) emit(name string, payload any) { e.emitID("", name, payload) }

// emitID publica um evento com ID explícito (idempotência em re-entregas da
// Meta). id vazio => aleatório.
func (e *Engine) emitID(id, name string, payload any) {
	if e.deps.Emit == nil {
		return
	}
	if id == "" {
		id = events.NewID()
	}
	e.deps.Emit(events.Event{
		ID:        id,
		Session:   e.deps.Session,
		Name:      name,
		Timestamp: time.Now().UTC(),
		Engine:    "cloud",
		Payload:   payload,
	})
}

/* ---------------- cliente Graph ---------------- */

func (e *Engine) base() string {
	return "https://graph.facebook.com/" + e.cfg.GraphVersion
}

func (e *Engine) graphGET(ctx context.Context, path string, q map[string]string, out any) error {
	u := e.base() + path
	if len(q) > 0 {
		parts := make([]string, 0, len(q))
		for k, v := range q {
			parts = append(parts, k+"="+v)
		}
		u += "?" + strings.Join(parts, "&")
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	req.Header.Set("Authorization", "Bearer "+e.cfg.AccessToken)
	return e.do(req, out)
}

// graphPOST envia JSON para /{phoneNumberID}<path> (path vazio = /messages).
func (e *Engine) graphPOST(ctx context.Context, path string, body, out any) error {
	if path == "" {
		path = "/" + e.cfg.PhoneNumberID + "/messages"
	}
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, e.base()+path, bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+e.cfg.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	return e.do(req, out)
}

func (e *Engine) do(req *http.Request, out any) error {
	resp, err := e.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var ge struct {
			Error struct {
				Message string `json:"message"`
				Type    string `json:"type"`
				Code    int    `json:"code"`
			} `json:"error"`
		}
		_ = json.Unmarshal(raw, &ge)
		if ge.Error.Message != "" {
			return fmt.Errorf("graph %d: %s (code %d)", resp.StatusCode, ge.Error.Message, ge.Error.Code)
		}
		return fmt.Errorf("graph %d: %s", resp.StatusCode, string(raw))
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}

// sendResultFromResp extrai o id da mensagem da resposta padrão de /messages.
func sendResultFromResp(raw json.RawMessage) engine.SendResult {
	var r struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
	}
	_ = json.Unmarshal(raw, &r)
	id := ""
	if len(r.Messages) > 0 {
		id = r.Messages[0].ID
	}
	return engine.SendResult{MessageID: id, Timestamp: time.Now().Unix()}
}

// toNumber normaliza um chatID/jid pra o formato que a Cloud API quer: só
// dígitos do número, sem "@s.whatsapp.net".
func toNumber(chatID string) string {
	s := chatID
	if i := strings.IndexByte(s, '@'); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimPrefix(s, "+")
	return s
}

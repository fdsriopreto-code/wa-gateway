// Package whatsmeow implementa engine.Engine sobre go.mau.fi/whatsmeow.
//
// NOTE: o whatsmeow evolui a API com frequencia. Este arquivo mira a versao
// resolvida por `go mod tidy` no momento do build; se algo nao compilar, o
// ajuste fica contido aqui.
package whatsmeow

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/proto/waE2E"
	waStore "go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	waEvents "go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"

	"wa-gateway/internal/engine"
	"wa-gateway/internal/events"
)

func init() {
	engine.Register("whatsmeow", New)
}

type Engine struct {
	deps engine.Deps

	mu     sync.RWMutex
	status engine.Status
	qr     string

	container *sqlstore.Container
	client    *whatsmeow.Client

	ctx    context.Context
	cancel context.CancelFunc
}

func New(deps engine.Deps) (engine.Engine, error) {
	return &Engine{deps: deps, status: engine.StatusStopped}, nil
}

func (e *Engine) Name() string { return "whatsmeow" }

func (e *Engine) Status() engine.Status {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.status
}

func (e *Engine) QR() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.qr
}

func (e *Engine) JID() string {
	e.mu.RLock()
	c := e.client
	e.mu.RUnlock()
	if c == nil || c.Store == nil || c.Store.ID == nil {
		return ""
	}
	return c.Store.ID.String()
}

// Start conecta a sessao. NAO deve receber um contexto de request: o ciclo
// de vida e do proprio Engine (e.ctx).
func (e *Engine) Start(ctx context.Context) error {
	e.mu.Lock()
	if e.client != nil {
		e.mu.Unlock()
		return nil
	}
	e.ctx, e.cancel = context.WithCancel(context.Background())
	e.setStatusLocked(engine.StatusStarting)
	e.mu.Unlock()

	dbLog := waLog.Stdout("wa-sqlstore/"+e.deps.Session, "WARN", false)
	container, err := sqlstore.New(ctx, "pgx", e.deps.DSN, dbLog)
	if err != nil {
		e.fail()
		return fmt.Errorf("sqlstore: %w", err)
	}

	// IMPORTANTE: o sqlstore guarda TODOS os devices (de todas as sessoes) na
	// mesma tabela. GetFirstDevice pegaria sempre o device #1 -> multi-sessao
	// quebrada (duas sessoes logando com o mesmo numero). Selecionamos pelo
	// JID que a sessao ja pareou; sem JID = sessao nova = device novo (QR).
	var device *waStore.Device
	if e.deps.StoredJID != "" {
		jid, perr := types.ParseJID(e.deps.StoredJID)
		if perr == nil {
			device, err = container.GetDevice(ctx, jid)
			if err != nil {
				e.fail()
				return fmt.Errorf("get device %s: %w", jid, err)
			}
		}
	}
	if device == nil {
		device = container.NewDevice() // sera salvo automaticamente no pareamento
	}

	clientLog := waLog.Stdout("wa/"+e.deps.Session, "INFO", false)
	client := whatsmeow.NewClient(device, clientLog)
	client.AddEventHandler(e.handleEvent)

	e.mu.Lock()
	e.container = container
	e.client = client
	e.mu.Unlock()

	needsQR := client.Store.ID == nil
	if needsQR {
		qrChan, err := client.GetQRChannel(e.ctx)
		if err != nil {
			e.fail()
			return fmt.Errorf("qr channel: %w", err)
		}
		if err := client.Connect(); err != nil {
			e.fail()
			return fmt.Errorf("connect: %w", err)
		}
		go e.pumpQR(qrChan)
	} else {
		if err := client.Connect(); err != nil {
			e.fail()
			return fmt.Errorf("connect: %w", err)
		}
	}
	return nil
}

func (e *Engine) pumpQR(ch <-chan whatsmeow.QRChannelItem) {
	for item := range ch {
		switch item.Event {
		case "code":
			e.mu.Lock()
			e.qr = item.Code
			e.setStatusLocked(engine.StatusScanQR)
			e.mu.Unlock()
			e.emit(events.QRCode, map[string]any{"code": item.Code})
		case "success":
			e.mu.Lock()
			e.qr = ""
			e.mu.Unlock()
		case "timeout":
			e.emit("session.qr.timeout", map[string]any{})
		default:
			e.emit("session.qr.event", map[string]any{"event": item.Event})
		}
	}
}

func (e *Engine) handleEvent(raw any) {
	if st, ok := connState(raw); ok {
		e.mu.Lock()
		e.setStatusLocked(st)
		if st == engine.StatusWorking {
			e.qr = ""
		}
		e.mu.Unlock()
	}

	// mensagens e recibos saem com payload normalizado (achatado). "raw" so
	// entra se a sessao pediu (config.rawEvents).
	switch ev := raw.(type) {
	case *waEvents.Message:
		p := normalizeMessage(ev, e.wantRaw())
		e.attachMedia(p, ev)
		e.emit("message.any", p)
		if !ev.Info.IsFromMe {
			e.emit("message", p) // recebidas: o que um bot assina
		}
		return
	case *waEvents.Receipt:
		e.emit("message.ack", normalizeReceipt(ev, e.wantRaw()))
		return
	}

	name, payload := translate(raw)
	e.emit(name, payload)
}

func (e *Engine) wantRaw() bool {
	return e.deps.RawEvents != nil && e.deps.RawEvents()
}

// attachMedia baixa+descriptografa a midia recebida e guarda no backend,
// anexando um campo "media" no payload. Roda inline (com timeout) e so
// quando ha um MediaSink habilitado.
func (e *Engine) attachMedia(p map[string]any, ev *waEvents.Message) {
	if e.deps.Media == nil || !e.deps.Media.Enabled() || ev.Info.MediaType == "" || ev.Message == nil {
		return
	}
	e.mu.RLock()
	client := e.client
	e.mu.RUnlock()
	if client == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	data, err := client.DownloadAny(ctx, ev.Message)
	if err != nil {
		p["media"] = map[string]any{"error": "download: " + err.Error()}
		return
	}
	mime := mediaMime(ev.Message)
	url, size, err := e.deps.Media.Store(ctx, e.deps.Session, ev.Info.ID, mime, data)
	if err != nil {
		p["media"] = map[string]any{"error": "store: " + err.Error()}
		return
	}
	p["media"] = map[string]any{"id": ev.Info.ID, "url": url, "mimetype": mime, "size": size}
}

func (e *Engine) emit(name string, payload any) {
	if e.deps.Emit == nil {
		return
	}
	e.deps.Emit(events.Event{
		ID:        events.NewID(),
		Session:   e.deps.Session,
		Name:      name,
		Timestamp: time.Now().UTC(),
		Engine:    "whatsmeow",
		Payload:   payload,
	})
}

func (e *Engine) setStatusLocked(s engine.Status) {
	if e.status == s {
		return
	}
	e.status = s
	go e.emit(events.SessionStatus, map[string]any{"status": string(s)})
}

func (e *Engine) fail() {
	e.mu.Lock()
	e.setStatusLocked(engine.StatusFailed)
	e.mu.Unlock()
}

func (e *Engine) Stop() error {
	e.mu.Lock()
	client, cancel := e.client, e.cancel
	e.client, e.container, e.cancel = nil, nil, nil
	e.setStatusLocked(engine.StatusStopped)
	e.mu.Unlock()

	if client != nil {
		client.Disconnect()
	}
	if cancel != nil {
		cancel()
	}
	return nil
}

func (e *Engine) Logout(ctx context.Context) error {
	e.mu.RLock()
	client := e.client
	e.mu.RUnlock()
	if client == nil {
		return fmt.Errorf("sessao nao iniciada")
	}
	if err := client.Logout(ctx); err != nil {
		return err
	}
	return e.Stop()
}

func (e *Engine) currentClient() (*whatsmeow.Client, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.client == nil {
		return nil, fmt.Errorf("sessao nao iniciada")
	}
	if e.status != engine.StatusWorking {
		return nil, fmt.Errorf("sessao nao conectada (status=%s)", e.status)
	}
	return e.client, nil
}

func (e *Engine) SendText(ctx context.Context, chatID, text string) (engine.SendResult, error) {
	client, err := e.currentClient()
	if err != nil {
		return engine.SendResult{}, err
	}
	jid, err := types.ParseJID(chatID)
	if err != nil {
		return engine.SendResult{}, fmt.Errorf("chatId invalido: %w", err)
	}
	msg := &waProto.Message{Conversation: proto.String(text)}
	resp, err := client.SendMessage(ctx, jid, msg)
	if err != nil {
		return engine.SendResult{}, err
	}
	return engine.SendResult{MessageID: resp.ID, Timestamp: resp.Timestamp.Unix()}, nil
}

func (e *Engine) SendImage(ctx context.Context, chatID string, data []byte, mimetype, caption string) (engine.SendResult, error) {
	client, err := e.currentClient()
	if err != nil {
		return engine.SendResult{}, err
	}
	jid, err := types.ParseJID(chatID)
	if err != nil {
		return engine.SendResult{}, fmt.Errorf("chatId invalido: %w", err)
	}
	up, err := client.Upload(ctx, data, whatsmeow.MediaImage)
	if err != nil {
		return engine.SendResult{}, fmt.Errorf("upload: %w", err)
	}
	if mimetype == "" {
		mimetype = "image/jpeg"
	}
	msg := &waProto.Message{ImageMessage: &waProto.ImageMessage{
		Caption:       proto.String(caption),
		Mimetype:      proto.String(mimetype),
		URL:           proto.String(up.URL),
		DirectPath:    proto.String(up.DirectPath),
		MediaKey:      up.MediaKey,
		FileEncSHA256: up.FileEncSHA256,
		FileSHA256:    up.FileSHA256,
		FileLength:    proto.Uint64(up.FileLength),
	}}
	resp, err := client.SendMessage(ctx, jid, msg)
	if err != nil {
		return engine.SendResult{}, err
	}
	return engine.SendResult{MessageID: resp.ID, Timestamp: resp.Timestamp.Unix()}, nil
}

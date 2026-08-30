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

// engineName e o identificador publico da engine (aparece em sessions.engine
// e no campo "engine" dos eventos). O pacote continua chamado "whatsmeow"
// internamente, mas nao expomos o nome da lib pra fora.
const engineName = "wa-gateway"

func init() {
	engine.Register(engineName, New)
	engine.Register("whatsmeow", New) // alias: registros antigos no banco
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

	// pool de download de midia: tira o attachMedia do caminho critico do
	// handler de eventos do whatsmeow (senao uma rajada de midia atrasa os
	// textos que vem atras).
	mediaJobs chan *waEvents.Message
	mediaWG   sync.WaitGroup
}

const mediaWorkers = 4

func New(deps engine.Deps) (engine.Engine, error) {
	return &Engine{deps: deps, status: engine.StatusStopped}, nil
}

func (e *Engine) Name() string { return engineName }

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
	// quebrada. Selecionamos o device desta sessao pelo numero (User do JID)
	// que ela ja pareou. Busca por numero e tolerante a diferenca de sufixo
	// de device/agente entre o que guardamos e o que o whatsmeow guarda.
	device, err := e.pickDevice(ctx, container)
	if err != nil {
		e.fail()
		return err
	}

	clientLog := waLog.Stdout("wa/"+e.deps.Session, "INFO", false)
	client := whatsmeow.NewClient(device, clientLog)
	client.AddEventHandler(e.handleEvent)

	e.mu.Lock()
	e.container = container
	e.client = client
	e.mediaJobs = make(chan *waEvents.Message, 256)
	e.mu.Unlock()

	for i := 0; i < mediaWorkers; i++ {
		e.mediaWG.Add(1)
		go e.mediaWorker(e.mediaJobs)
	}

	needsQR := client.Store.ID == nil
	if needsQR {
		e.deps.Logger.Info("device novo — vai pedir QR", "storedJID", e.deps.StoredJID)
	} else {
		e.deps.Logger.Info("device carregado do store — reconectando sem QR", "jid", client.Store.ID.String())
	}
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

// pickDevice escolhe o device desta sessao: procura no store um device cujo
// numero (User do JID) bata com o que a sessao ja pareou. Se nao achar,
// devolve um device novo (fluxo de QR).
func (e *Engine) pickDevice(ctx context.Context, container *sqlstore.Container) (*waStore.Device, error) {
	all, err := container.GetAllDevices(ctx)
	if err != nil {
		return nil, fmt.Errorf("listar devices: %w", err)
	}

	want, perr := types.ParseJID(e.deps.StoredJID)
	if e.deps.StoredJID == "" || perr != nil || want.User == "" {
		if e.deps.StoredJID != "" {
			e.deps.Logger.Warn("StoredJID invalido, tratando como sessao nova", "storedJID", e.deps.StoredJID, "err", perr)
		}
		// Sessao sem JID registrado, sendo retomada no boot: se ha EXATAMENTE
		// um device no store, ele provavelmente e desta sessao — pareou mas o
		// processo caiu antes de persistir o JID (redeploy no meio do
		// pareamento). Adota em vez de pedir QR de novo e deixar o device
		// orfao. Nao vale pra partida interativa (uma sessao nova nao rouba o
		// device de outra).
		if e.deps.Recovering && len(all) == 1 && all[0].ID != nil {
			e.deps.Logger.Warn("StoredJID vazio — adotando o unico device do store", "jid", all[0].ID.String())
			return all[0], nil
		}
		return container.NewDevice(), nil
	}
	var exact, byNumber *waStore.Device
	have := make([]string, 0, len(all))
	for _, d := range all {
		if d.ID == nil {
			continue
		}
		have = append(have, d.ID.String())
		if d.ID.String() == e.deps.StoredJID {
			exact = d
		} else if d.ID.User == want.User && byNumber == nil {
			byNumber = d
		}
	}
	switch {
	case exact != nil:
		return exact, nil
	case byNumber != nil:
		e.deps.Logger.Info("device casado pelo numero (JID exato divergiu)", "quero", e.deps.StoredJID, "achei", byNumber.ID.String())
		return byNumber, nil
	default:
		e.deps.Logger.Warn("device do numero nao encontrado no store — vai pedir QR",
			"quero", want.User, "devicesNoStore", have)
		return container.NewDevice(), nil
	}
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
			// pareou. Sai de SCAN_QR na hora (senao o console segue mostrando
			// QR ate o evento Connected) e — o mais importante — o
			// setStatusLocked emite session.status, o que faz o Manager
			// persistir sessions.jid AGORA. Sem isso, um restart entre o pair
			// e o Connected perde o vinculo e a proxima subida pede QR de novo
			// mesmo com o device ja pareado no store.
			e.mu.Lock()
			e.qr = ""
			e.setStatusLocked(engine.StatusStarting)
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
		if st == engine.StatusWorking && e.behavior().AutoOnline {
			go e.goOnline()
		}
	}

	// mensagens e recibos saem com payload normalizado (achatado). "raw" so
	// entra se a sessao pediu (config.rawEvents).
	switch ev := raw.(type) {
	case *waEvents.Message:
		if !ev.Info.IsFromMe && e.behavior().AutoRead {
			go e.autoRead(ev)
		}
		// midia (com sink ligado) vai pro pool: o download nao pode bloquear
		// este goroutine, senao atrasa as mensagens que vem atras.
		if ev.Info.MediaType != "" && e.deps.Media != nil && e.deps.Media.Enabled() {
			e.mu.RLock()
			jobs := e.mediaJobs
			e.mu.RUnlock()
			select {
			case jobs <- ev:
				return
			default: // pool cheio (ou parado) -> cai no caminho inline
			}
		}
		p := normalizeMessage(ev, e.wantRaw())
		e.attachMedia(p, ev)
		e.emitMessage(p, ev.Info.IsFromMe)
		return
	case *waEvents.Receipt:
		e.emit("message.ack", normalizeReceipt(ev, e.wantRaw()))
		return
	}

	name, payload := translate(raw)
	e.emit(name, payload)
}

// mediaWorker processa mensagens de midia fora do caminho critico.
func (e *Engine) mediaWorker(jobs <-chan *waEvents.Message) {
	defer e.mediaWG.Done()
	for ev := range jobs {
		p := normalizeMessage(ev, e.wantRaw())
		e.attachMedia(p, ev)
		e.emitMessage(p, ev.Info.IsFromMe)
	}
}

func (e *Engine) emitMessage(p map[string]any, fromMe bool) {
	e.emit("message.any", p)
	if !fromMe {
		e.emit("message", p) // recebidas: o que um bot assina
	}
}

func (e *Engine) wantRaw() bool {
	return e.deps.RawEvents != nil && e.deps.RawEvents()
}

func (e *Engine) behavior() engine.AutoBehavior {
	if e.deps.Behavior == nil {
		return engine.AutoBehavior{}
	}
	return e.deps.Behavior()
}

// autoRead marca a mensagem recebida como lida, fora do caminho critico.
func (e *Engine) autoRead(ev *waEvents.Message) {
	e.mu.RLock()
	client := e.client
	e.mu.RUnlock()
	if client == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	sender := ev.Info.Sender
	_ = client.MarkRead(ctx, []types.MessageID{ev.Info.ID}, time.Now(), ev.Info.Chat, sender)
}

// goOnline manda presenca "available" apos conectar (mantem o "online").
func (e *Engine) goOnline() {
	e.mu.RLock()
	client := e.client
	e.mu.RUnlock()
	if client == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = client.SendPresence(ctx, types.PresenceAvailable)
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
		Engine:    engineName,
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
	jobs := e.mediaJobs
	e.client, e.container, e.cancel, e.mediaJobs = nil, nil, nil, nil
	e.setStatusLocked(engine.StatusStopped)
	e.mu.Unlock()

	// fecha o pool e espera os workers drenarem o que ja pegaram. attachMedia
	// vira no-op quando e.client == nil, entao os jobs restantes saem rapido.
	if jobs != nil {
		close(jobs)
		e.mediaWG.Wait()
	}
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

func (e *Engine) SendText(ctx context.Context, chatID, text string, opts engine.MessageOpts) (engine.SendResult, error) {
	ci := ctxInfo(opts)
	// texto puro sem extras -> Conversation; com citacao/mencao/preview -> ExtendedText.
	if ci == nil && !opts.LinkPreview {
		return e.send(ctx, chatID, &waProto.Message{Conversation: proto.String(text)})
	}
	etm := &waProto.ExtendedTextMessage{Text: proto.String(text), ContextInfo: ci}
	if opts.LinkPreview {
		e.applyLinkPreview(ctx, etm, text)
	}
	return e.send(ctx, chatID, &waProto.Message{ExtendedTextMessage: etm})
}

func (e *Engine) SendImage(ctx context.Context, chatID string, data []byte, mimetype, caption string) (engine.SendResult, error) {
	up, err := e.uploadMedia(ctx, data, whatsmeow.MediaImage)
	if err != nil {
		return engine.SendResult{}, err
	}
	if mimetype == "" {
		mimetype = "image/jpeg"
	}
	return e.send(ctx, chatID, &waProto.Message{ImageMessage: &waProto.ImageMessage{
		Caption:       strPtrOrNil(caption),
		Mimetype:      proto.String(mimetype),
		URL:           proto.String(up.URL),
		DirectPath:    proto.String(up.DirectPath),
		MediaKey:      up.MediaKey,
		FileEncSHA256: up.FileEncSHA256,
		FileSHA256:    up.FileSHA256,
		FileLength:    proto.Uint64(up.FileLength),
	}})
}

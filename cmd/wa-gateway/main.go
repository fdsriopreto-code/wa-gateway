// Command wa-gateway: API HTTP multi-sessao de WhatsApp.
//
// Fase 1 (MVP): engine whatsmeow, entrega por webhook (fila asynq/Redis) e
// WebSocket, PostgreSQL como fonte de verdade, Redis para cache/fila/locks.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"

	"wa-gateway/internal/auth"
	"wa-gateway/internal/cache"
	"wa-gateway/internal/config"
	_ "wa-gateway/internal/engine/whatsmeow" // registra a engine
	"wa-gateway/internal/events"
	"wa-gateway/internal/httpapi"
	"wa-gateway/internal/media"
	"wa-gateway/internal/observability"
	"wa-gateway/internal/outbox"
	"wa-gateway/internal/session"
	"wa-gateway/internal/store"
	"wa-gateway/internal/webhook"
	"wa-gateway/internal/ws"
)

var version = "0.1.0-dev"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := observability.NewLogger(cfg.LogLevel, cfg.LogFormat)
	slog.SetDefault(log)
	log.Info("iniciando wa-gateway", "version", version, "node", cfg.NodeID)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// ---- Postgres ----
	log.Info("aplicando migracoes")
	if err := store.Migrate(cfg.DatabaseURL); err != nil {
		return err
	}
	st, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer st.Close()

	// ---- Redis ----
	rc, err := cache.New(ctx, cfg.RedisURL)
	if err != nil {
		return err
	}
	defer rc.Close()

	redisOpt, err := asynq.ParseRedisURI(cfg.RedisURL)
	if err != nil {
		return err
	}

	// ---- barramento interno ----
	bus := events.NewBus(log)
	bus.OnPublish = func(name string) { observability.EventsPublished.WithLabelValues(name).Inc() }
	bus.OnDrop = func(sub string) { observability.BusDropped.WithLabelValues(sub).Inc() }

	// ---- fila / webhooks ----
	asynqClient := asynq.NewClient(redisOpt)
	defer asynqClient.Close()

	dispatcher := webhook.NewDispatcher(st, asynqClient, log, cfg.WebhookTimeout, cfg.WebhookMaxAttempts)

	// ---- sessoes ----
	mgr := session.NewManager(st, rc, bus, log, cfg.DatabaseURL, cfg.NodeID, cfg.DefaultEngine)

	// ---- fila de saida (pacing anti-ban) ----
	outQueue := outbox.NewQueue(asynqClient, rc, outbox.Defaults{Pace: outbox.Pace{
		MinInterval: cfg.OutboxMinInterval,
		Jitter:      cfg.OutboxJitter,
		DailyLimit:  cfg.OutboxDailyLimit,
	}}, st.OutboxRecorder())
	outWorker := outbox.NewWorker(mgr, st.OutboxRecorder(), log)

	asynqSrv := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: 20,
		Queues:      map[string]int{"webhook": 10, "outbox": 5, "default": 1},
		Logger:      asynqSlog{log},
	})
	mux := asynq.NewServeMux()
	mux.HandleFunc(webhook.TaskDeliver, dispatcher.Handler())
	mux.HandleFunc(outbox.TaskSend, outWorker.Handler())
	if err := asynqSrv.Start(mux); err != nil {
		return err
	}
	defer asynqSrv.Shutdown()

	// ---- armazenamento de midia (opcional) ----
	var mediaStore media.Store = media.Disabled{}
	if cfg.MediaBackend == "s3" {
		mediaStore, err = media.NewS3(ctx, media.S3Config{
			Endpoint:      cfg.S3Endpoint,
			Region:        cfg.S3Region,
			Bucket:        cfg.S3Bucket,
			AccessKey:     cfg.S3AccessKey,
			SecretKey:     cfg.S3SecretKey,
			UseSSL:        cfg.S3UseSSL,
			PathStyle:     cfg.S3PathStyle,
			PublicBaseURL: cfg.S3PublicBaseURL,
		})
		if err != nil {
			return err
		}
		log.Info("armazenamento de midia: s3", "bucket", cfg.S3Bucket, "endpoint", cfg.S3Endpoint)
	}

	// ---- websocket ----
	hub := ws.NewHub(log)

	// ---- consumidores do barramento ----
	go dispatcher.Run(ctx, bus)
	go hub.Run(ctx, bus)
	go mgr.RestoreOwned(ctx)

	// ---- HTTP ----
	authn := auth.New(cfg.APIKey, func(ctx context.Context, keyID string) (string, []string, error) {
		var hash string
		var scopes []string
		err := st.Pool.QueryRow(ctx,
			`SELECT hash, scopes FROM api_keys WHERE id = $1 AND revoked_at IS NULL`, keyID).
			Scan(&hash, &scopes)
		if err != nil {
			return "", nil, auth.ErrUnknownKey
		}
		return hash, scopes, nil
	})
	if cfg.APIKey == "" {
		log.Warn("API_KEY vazia: apenas chaves da tabela api_keys serao aceitas")
	}

	handler := httpapi.NewRouter(httpapi.Deps{
		Manager:   mgr,
		Store:     st,
		Hub:       hub,
		Queue:     outQueue,
		Media:     mediaStore,
		Log:       log,
		Version:   version,
		StartedAt: time.Now().UTC().Format(time.RFC3339),
	}, authn)

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Info("HTTP ouvindo", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("http server", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	log.Info("encerrando...")

	shutCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx)
	mgr.StopAll()

	log.Info("adeus")
	return nil
}

// asynqSlog adapta *slog.Logger para asynq.Logger.
type asynqSlog struct{ l *slog.Logger }

func (a asynqSlog) Debug(args ...any) { a.l.Debug("asynq", "msg", args) }
func (a asynqSlog) Info(args ...any)  { a.l.Info("asynq", "msg", args) }
func (a asynqSlog) Warn(args ...any)  { a.l.Warn("asynq", "msg", args) }
func (a asynqSlog) Error(args ...any) { a.l.Error("asynq", "msg", args) }
func (a asynqSlog) Fatal(args ...any) { a.l.Error("asynq-fatal", "msg", args) }

package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/menezmethod/inferencia/internal/auth"
	"github.com/menezmethod/inferencia/internal/backend"
	"github.com/menezmethod/inferencia/internal/config"
	"github.com/menezmethod/inferencia/internal/decision"
	"github.com/menezmethod/inferencia/internal/logging"
	"github.com/menezmethod/inferencia/internal/observability"
	"github.com/menezmethod/inferencia/internal/router"
	"github.com/menezmethod/inferencia/internal/server"
	"github.com/menezmethod/inferencia/internal/watchdog"
)

func main() {
	configPath := flag.String("config", "", "path to config.yaml (optional, env vars work without it)")
	flag.Parse()

	// Load configuration: defaults -> YAML file -> env vars.
	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}

	// Set up structured logger.
	logger := newLogger(cfg.Log)

	// Load API keys.
	ks, err := auth.NewKeyStore(cfg.Auth.KeysFile)
	if err != nil {
		logger.Error("failed to load API keys", "err", err)
		os.Exit(1)
	}
	logger.Info("api keys loaded", "count", ks.Count())

	// Register backends.
	reg := backend.NewRegistry()
	for _, b := range cfg.Backends {
		healthTimeout := b.HealthTimeout
		if healthTimeout <= 0 {
			healthTimeout = 5 * time.Second
		}
		switch b.Type {
		case "mlx":
			// MLX speaks OpenAI-compatible HTTP; the Ollama adapter covers it
			// (native Ollama endpoints are only used when a think control is sent).
			reg.Register(backend.NewOllama(b.Name, b.URL, healthTimeout, b.Timeout))
			logger.Info("backend registered", "name", b.Name, "type", b.Type, "url", b.URL)
		case "commandcode":
			key := ""
			if b.APIKeyEnv != "" {
				key = os.Getenv(b.APIKeyEnv)
			}
			if key == "" {
				key = os.Getenv("INFERENCIA_COMMANDCODE_KEY")
			}
			if key == "" {
				logger.Warn("skipping commandcode backend: no API key (set INFERENCIA_COMMANDCODE_KEY or api_key_env)", "name", b.Name)
				continue
			}
			baseURL := b.URL
			if baseURL == "" {
				baseURL = "https://api.commandcode.ai/provider/v1"
			}
			reg.Register(backend.NewCommandCode(b.Name, baseURL, key, healthTimeout, b.Timeout))
			logger.Info("backend registered", "name", b.Name, "type", b.Type, "url", baseURL)
		case "ollama":
			reg.Register(backend.NewOllama(b.Name, b.URL, healthTimeout, b.Timeout))
			logger.Info("backend registered", "name", b.Name, "type", b.Type, "url", b.URL)
		default:
			logger.Error("unknown backend type", "name", b.Name, "type", b.Type)
			os.Exit(1)
		}
	}

	ttsRouter := router.NewRegistry()
	for _, t := range cfg.TTSBackends {
		ttsBackend := backend.NewTTSHTTP(t.Name, t.URL, t.Timeout)
		ttsRouter.Register(router.BackendInfo{
			Name:         t.Name,
			TTSBackend:   ttsBackend,
			Capabilities: []router.Capability{router.CapTTS},
			Models: []router.ModelInfo{
				{ID: t.Name, Kind: router.CapTTS},
			},
		})
		logger.Info("tts backend registered", "name", t.Name, "url", t.URL)
	}

	// Decision engines (System One): Laya (local) and Jev (hosted) share one protocol.
	var decisionClients []*decision.Client
	for _, d := range cfg.Decisions {
		key := ""
		if d.APIKeyEnv != "" {
			key = os.Getenv(d.APIKeyEnv)
			if key == "" {
				logger.Warn("skipping decision backend: API key env is empty", "name", d.Name, "env", d.APIKeyEnv)
				continue
			}
		}
		decisionClients = append(decisionClients, decision.New(d.Name, d.URL, key, d.Models...))
		logger.Info("decision backend registered", "name", d.Name, "url", d.URL, "models", d.Models)
	}
	decisions := decision.NewSet(decisionClients...)

	wd := watchdog.New(watchdog.Config{
		Interval:       cfg.Watchdog.Interval,
		FailThreshold:  cfg.Watchdog.FailThreshold,
		RequestTimeout: cfg.Watchdog.RequestTimeout,
	}, reg, ttsRouter, logger)

	srv := server.New(cfg, reg, ttsRouter, decisions, ks, wd, logger)

	wd.Start()

	// Optional OpenTelemetry tracing: wrap handler so all requests are traced.
	var tp *sdktrace.TracerProvider
	if cfg.Observability.OTelEnabled {
		var errOTel error
		tp, errOTel = observability.NewTracerProvider(context.Background(), cfg.Observability.OTelEndpoint, cfg.Observability.OTelServiceName)
		if errOTel != nil {
			logger.Error("otel tracer provider failed", "err", errOTel)
			os.Exit(1)
		}
		srv.Handler = observability.HTTPHandler(srv.Handler, cfg.Observability.OTelServiceName)
		logger.Info("opentelemetry tracing enabled", "endpoint", cfg.Observability.OTelEndpoint)
	}

	// Graceful shutdown on SIGINT/SIGTERM.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		logger.Info("server starting", "addr", cfg.Server.Addr())
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	<-stop
	logger.Info("shutdown signal received")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	wd.Stop()
	if tp != nil {
		if err := tp.Shutdown(ctx); err != nil {
			logger.Error("otel shutdown failed", "err", err)
		}
	}
	server.Shutdown(ctx, srv, logger)
	logger.Info("server stopped")
}

func newLogger(cfg config.Log) *slog.Logger {
	var level slog.Level
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	return logging.NewLogger(os.Stdout, level, cfg.Format, cfg.CloudFormat)
}

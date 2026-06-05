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

	"github.com/menezmethod/inferencia/internal/auth"
	"github.com/menezmethod/inferencia/internal/backend"
	"github.com/menezmethod/inferencia/internal/config"
	"github.com/menezmethod/inferencia/internal/logging"
	"github.com/menezmethod/inferencia/internal/middleware"
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
			reg.Register(backend.NewMLX(b.Name, b.URL, healthTimeout, b.Timeout))
			logger.Info("backend registered", "name", b.Name, "type", b.Type, "url", b.URL)
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
			Name:       t.Name,
			TTSBackend: ttsBackend,
			Capabilities: []router.Capability{router.CapTTS},
			Models: []router.ModelInfo{
				{ID: t.Name, Kind: router.CapTTS},
			},
		})
		logger.Info("tts backend registered", "name", t.Name, "url", t.URL)
	}

	wd := watchdog.New(watchdog.Config{
		Interval:       cfg.Watchdog.Interval,
		FailThreshold:  cfg.Watchdog.FailThreshold,
		RequestTimeout: cfg.Watchdog.RequestTimeout,
	}, reg, ttsRouter, logger)

	srv := server.New(cfg, reg, ks, wd, logger)

	if ttsRouter.Len() > 0 {
		rl := middleware.NewRateLimiter(cfg.RateLimit.RequestsPerSecond, cfg.RateLimit.Burst)
		protected := func(h http.Handler) http.Handler {
			return middleware.Chain(h,
				middleware.RequestID(),
				middleware.Recover(logger),
				middleware.Metrics(),
				middleware.Logging(logger),
				middleware.Auth(ks),
				middleware.RateLimit(rl),
			)
		}
		server.RegisterTTSRoutes(srv, ttsRouter, wd, logger, protected)
	}

	// Register consolidated health status endpoint.
	server.RegisterHealthStatusRoute(srv, reg, ttsRouter)

	wd.Start()

	// Optional OpenTelemetry tracing: wrap handler so all requests are traced.
	var tp *observability.TracerProvider
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
		_ = tp.Shutdown(ctx)
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

	// Use cloud-friendly logger (GCP severity, optional resource) when configured.
	if cfg.CloudFormat != "" {
		return logging.NewLogger(os.Stdout, level, cfg.Format, cfg.CloudFormat)
	}

	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	if cfg.Format == "text" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}
	return slog.New(handler)
}

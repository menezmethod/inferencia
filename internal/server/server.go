// Package server configures and runs the HTTP server.
package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/menezmethod/inferencia/internal/auth"
	"github.com/menezmethod/inferencia/internal/backend"
	"github.com/menezmethod/inferencia/internal/config"
	"github.com/menezmethod/inferencia/internal/decision"
	"github.com/menezmethod/inferencia/internal/handler"
	"github.com/menezmethod/inferencia/internal/middleware"
	"github.com/menezmethod/inferencia/internal/router"
)

// New creates a configured *http.Server with all routes and middleware wired.
// hc may be nil (treats all backends as healthy) or a watchdog for degraded-backend skipping.
// ttsReg may be nil when no TTS backends are configured; decisions may be empty.
func New(cfg config.Config, reg *backend.Registry, ttsReg *router.Registry, decisions *decision.Set, ks *auth.KeyStore, hc backend.HealthChecker, logger *slog.Logger) *http.Server {
	mux := http.NewServeMux()
	rl := middleware.NewRateLimiter(cfg.RateLimit.RequestsPerSecond, cfg.RateLimit.Burst)

	// Middleware stack applied to authenticated API routes.
	// Order (outermost → innermost): RequestID → Recover → Metrics → Logging → Auth → RateLimit
	// Logging runs after Auth so the canonical log line includes the masked API key.
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

	// Local (free) backends: used by the auto:free policy to keep requests
	// off paid providers.
	localBackends := make(map[string]bool, len(cfg.Backends))
	for _, b := range cfg.Backends {
		if b.Type == "ollama" || b.Type == "mlx" {
			localBackends[b.Name] = true
		}
	}

	// Health, docs, version, and metrics — no auth required.
	mux.HandleFunc("GET /health", handler.HealthStatus(reg, ttsReg))
	mux.HandleFunc("GET /health/status", handler.HealthStatus(reg, ttsReg))
	mux.HandleFunc("GET /health/ready", handler.Ready(reg))
	mux.HandleFunc("GET /version", handler.VersionInfo())
	mux.HandleFunc("GET /openapi.yaml", handler.OpenAPI())
	mux.HandleFunc("GET /docs", handler.SwaggerUI())
	mux.Handle("GET /metrics", promhttp.Handler())

	// OpenAI-compatible API endpoints — auth + rate limiting required.
	mux.Handle("POST /v1/chat/completions", protected(handler.ChatCompletions(reg, hc, cfg.Routing, localBackends, decisions, logger)))
	mux.Handle("GET /v1/models", protected(handler.Models(reg, hc, cfg.Routing, decisions, logger)))
	mux.Handle("POST /v1/embeddings", protected(handler.Embeddings(reg, hc, logger)))
	mux.Handle("POST /v1/systemone", protected(handler.SystemOne(decisions, logger)))
	if ttsReg != nil && ttsReg.Len() > 0 {
		mux.Handle("POST /v1/audio/speech", protected(handler.Audio(ttsReg, hc, logger)))
	}

	return &http.Server{
		Addr:              cfg.Server.Addr(),
		Handler:           mux,
		ReadTimeout:       cfg.Server.ReadTimeout,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      cfg.Server.WriteTimeout,
		IdleTimeout:       60 * time.Second,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}
}

// Shutdown gracefully shuts down the server with the given context.
func Shutdown(ctx context.Context, srv *http.Server, logger *slog.Logger) {
	logger.Info("shutting down server")
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("server shutdown error", "err", err)
	}
}

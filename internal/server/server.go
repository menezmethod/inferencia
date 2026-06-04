// Package server configures and runs the HTTP server.
package server

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/menezmethod/inferencia/internal/auth"
	"github.com/menezmethod/inferencia/internal/backend"
	"github.com/menezmethod/inferencia/internal/config"
	"github.com/menezmethod/inferencia/internal/handler"
	"github.com/menezmethod/inferencia/internal/middleware"
	"github.com/menezmethod/inferencia/internal/router"
)

// New creates a configured *http.Server with all routes and middleware wired.
func New(cfg config.Config, reg *backend.Registry, ks *auth.KeyStore, logger *slog.Logger) *http.Server {
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

	// Health, docs, version, and metrics — no auth required.
	mux.HandleFunc("GET /health", handler.Health())
	mux.HandleFunc("GET /health/ready", handler.Ready(reg))
	mux.HandleFunc("GET /version", handler.VersionInfo())
	mux.HandleFunc("GET /openapi.yaml", handler.OpenAPI())
	mux.HandleFunc("GET /docs", handler.SwaggerUI())
	mux.Handle("GET /metrics", promhttp.Handler())

	// OpenAI-compatible API endpoints — auth + rate limiting required.
	mux.Handle("POST /v1/chat/completions", protected(handler.ChatCompletions(reg, logger)))
	mux.Handle("GET /v1/models", protected(handler.Models(reg, logger)))
	mux.Handle("POST /v1/embeddings", protected(handler.Embeddings(reg, logger)))

	return &http.Server{
		Addr:         cfg.Server.Addr(),
		Handler:      mux,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		ErrorLog:     slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}
}

// RegisterTTSRoutes adds the TTS audio/speech endpoint to an existing server's mux.
// It requires a router registry with TTS backends registered.
// The server must have been created with http.NewServeMux() and expose its mux.
//
// Usage:
//
//	srv := server.New(cfg, reg, ks, logger)
//	server.RegisterTTSRoutes(srv, rtr, logger)
func RegisterTTSRoutes(srv *http.Server, rtr *router.Registry, logger *slog.Logger, protected func(http.Handler) http.Handler) {
	if srv.Handler == nil || rtr == nil {
		return
	}
	if mux, ok := srv.Handler.(*http.ServeMux); ok {
		mux.Handle("POST /v1/audio/speech", protected(handler.Audio(rtr, logger)))
	}
}

// RegisterTTSRoute is a convenience function that registers the TTS endpoint
// on the given mux using the standard protected middleware chain.
// It creates its own protected middleware from the given config and key store,
// making it usable directly from main when a router registry is available.
func RegisterTTSRoute(mux *http.ServeMux, rtr *router.Registry, ks *auth.KeyStore, cfg config.Config, logger *slog.Logger) {
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
	mux.Handle("POST /v1/audio/speech", protected(handler.Audio(rtr, logger)))
}

// RegisterHealthStatusRoute adds the consolidated /health/status endpoint.
// It probes all chat/embed backends and any configured TTS backends,
// returning a per-service breakdown. No auth required.
func RegisterHealthStatusRoute(srv *http.Server, reg *backend.Registry, ttsReg *router.Registry) {
	if srv.Handler == nil {
		return
	}
	if mux, ok := srv.Handler.(*http.ServeMux); ok {
		mux.HandleFunc("GET /health/status", handler.HealthStatus(reg, ttsReg))
	}
}

// Shutdown gracefully shuts down the server with the given context.
func Shutdown(ctx context.Context, srv *http.Server, logger *slog.Logger) {
	logger.Info("shutting down server")
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("server shutdown error", "err", err)
	}
}

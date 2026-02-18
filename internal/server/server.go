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

	// Health, docs, and metrics — no auth required.
	mux.HandleFunc("GET /health", handler.Health())
	mux.HandleFunc("GET /health/ready", handler.Ready(reg))
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

// Shutdown gracefully shuts down the server with the given context.
func Shutdown(ctx context.Context, srv *http.Server, logger *slog.Logger) {
	logger.Info("shutting down server")
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("server shutdown error", "err", err)
	}
}

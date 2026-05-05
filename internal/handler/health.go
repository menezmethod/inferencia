// Package handler implements HTTP handlers for the OpenAI-compatible API.
package handler

import (
	"encoding/json"
	"net/http"

	"github.com/menezmethod/inferencia/internal/backend"
	"github.com/menezmethod/inferencia/internal/middleware"
	"github.com/menezmethod/inferencia/internal/version"
)

// Health handles liveness checks. It always returns 200 if the server is running.
// Response includes "version" so you can see which inferencia build is running.
//
//	GET /health
func Health() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "ok",
			"version": version.Version,
		})
	}
}

// Ready handles readiness checks. It returns 200 only if all backends are healthy.
//
//	GET /health/ready
func Ready(reg *backend.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		for _, b := range reg.All() {
			if err := b.Health(r.Context()); err != nil {
				middleware.BackendHealth.WithLabelValues(b.Name()).Set(0)
				w.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"status":  "unavailable",
					"backend": b.Name(),
					"error":   err.Error(),
					"version": version.Version,
				})
				return
			}
			middleware.BackendHealth.WithLabelValues(b.Name()).Set(1)
		}

		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "ready",
			"version": version.Version,
		})
	}
}

// VersionInfo handles version info. Returns JSON with version and optional commit.
//
//	GET /version
func VersionInfo() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		out := map[string]string{"version": version.Version}
		if version.Commit != "" {
			out["commit"] = version.Commit
		}
		_ = json.NewEncoder(w).Encode(out)
	}
}

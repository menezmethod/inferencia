// Package handler implements HTTP handlers for the OpenAI-compatible API.
package handler

import (
	"encoding/json"
	"net/http"

	"github.com/menez/inferencia/internal/backend"
)

// Health handles liveness checks. It always returns 200 if the server is running.
//
//	GET /health
func Health() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
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
				w.WriteHeader(http.StatusServiceUnavailable)
				json.NewEncoder(w).Encode(map[string]string{
					"status":  "unavailable",
					"backend": b.Name(),
					"error":   err.Error(),
				})
				return
			}
		}

		json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
	}
}

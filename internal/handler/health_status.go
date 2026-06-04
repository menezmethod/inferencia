// Package handler implements HTTP handlers for the OpenAI-compatible API.
package handler

import (
	"encoding/json"
	"net/http"

	"github.com/menezmethod/inferencia/internal/backend"
	"github.com/menezmethod/inferencia/internal/router"
	"github.com/menezmethod/inferencia/internal/version"
)

// ServiceStatus represents the health of a single backend service.
type ServiceStatus struct {
	Status string `json:"status"`          // "healthy" or "unhealthy"
	Error  string `json:"error,omitempty"` // error message if unhealthy
}

// HealthStatusResponse is the JSON response for the consolidated health check.
type HealthStatusResponse struct {
	Status   string                   `json:"status"`   // "healthy" or "degraded"
	Version  string                   `json:"version"`  // build version
	Services map[string]ServiceStatus `json:"services"` // per-service breakdown
}

// HealthStatus returns a consolidated health check handler that probes all
// registered backends (chat, embed, TTS) and returns a per-service breakdown.
//
//	GET /health/status
//
// Returns 200 if all services are healthy, 503 if any service is down.
func HealthStatus(reg *backend.Registry, ttsReg *router.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		services := make(map[string]ServiceStatus)
		overall := "healthy"

		// Check chat/embed backends (Ollama, MLX).
		for _, b := range reg.All() {
			if err := b.Health(r.Context()); err != nil {
				services[b.Name()] = ServiceStatus{
					Status: "unhealthy",
					Error:  err.Error(),
				}
				overall = "degraded"
			} else {
				services[b.Name()] = ServiceStatus{Status: "healthy"}
			}
		}

		// Check TTS backends (Kokoro).
		if ttsReg != nil {
			for _, info := range ttsReg.All() {
				if info.TTSBackend != nil {
					if err := info.TTSBackend.Health(r.Context()); err != nil {
						services[info.Name] = ServiceStatus{
							Status: "unhealthy",
							Error:  err.Error(),
						}
						overall = "degraded"
					} else {
						services[info.Name] = ServiceStatus{Status: "healthy"}
					}
				}
			}
		}

		// If no services at all, report degraded.
		if len(services) == 0 {
			overall = "degraded"
			services["_none"] = ServiceStatus{
				Status: "unhealthy",
				Error:  "no backends registered",
			}
		}

		resp := HealthStatusResponse{
			Status:   overall,
			Version:  version.Version,
			Services: services,
		}

		if overall == "degraded" {
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		_ = json.NewEncoder(w).Encode(resp)
	}
}

// Package handler implements HTTP handlers for the OpenAI-compatible API.
package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/menezmethod/inferencia/internal/backend"
	"github.com/menezmethod/inferencia/internal/router"
	"github.com/menezmethod/inferencia/internal/version"
)

// ServiceStatus represents the health of a single backend service.
type ServiceStatus struct {
	Status   string       `json:"status"`             // "healthy" or "unhealthy"
	Error    string       `json:"error,omitempty"`    // error message if unhealthy
	Models   []ModelBrief `json:"models,omitempty"`   // available models/voices
}

// ModelBrief is a lightweight summary of a model or voice offered by a backend.
type ModelBrief struct {
	ID      string `json:"id"`
	Object  string `json:"object,omitempty"`
	OwnedBy string `json:"owned_by,omitempty"`
}

// HealthSummary aggregates service counts.
type HealthSummary struct {
	Total     int            `json:"total"`
	Healthy   int            `json:"healthy"`
	Unhealthy int            `json:"unhealthy"`
	ByType    map[string]int `json:"by_type"`
}

// HealthStatusResponse is the JSON response for the consolidated health check.
type HealthStatusResponse struct {
	Status    string                   `json:"status"`    // "healthy" or "degraded"
	Version   string                   `json:"version"`   // build version
	Timestamp string                   `json:"timestamp"` // ISO 8601
	Services  map[string]ServiceStatus `json:"services"`  // per-service breakdown
	Summary   HealthSummary            `json:"summary"`   // aggregate totals
}

// HealthStatus returns a consolidated health check handler that probes all
// registered backends (chat, embed, TTS) and reports their health, available
// models, and aggregate summary. When hc is provided, backends marked degraded
// by the watchdog are reported unhealthy without a live probe.
//
//	GET /health/status
//
// Returns 200 if all services are healthy, 503 if any service is down.
func HealthStatus(reg *backend.Registry, ttsReg *router.Registry, hc backend.HealthChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		services := make(map[string]ServiceStatus)
		overall := "healthy"
		summary := HealthSummary{
			ByType: make(map[string]int),
		}

		// Check chat/embed backends (Ollama, MLX).
		for _, b := range reg.All() {
			s := ServiceStatus{Status: "healthy"}

			if err := backend.CheckBackendHealth(r.Context(), hc, b.Name(), b.Health); err != nil {
				s.Status = "unhealthy"
				s.Error = err.Error()
				overall = "degraded"
			} else {
				// Fetch model inventory on healthy backends.
				models, listErr := b.ListModels(r.Context())
				if listErr == nil && models != nil && len(models.Data) > 0 {
					briefs := make([]ModelBrief, len(models.Data))
					for i, m := range models.Data {
						briefs[i] = ModelBrief{
							ID:      m.ID,
							Object:  m.Object,
							OwnedBy: m.OwnedBy,
						}
					}
					s.Models = briefs
				}
			}

			services[b.Name()] = s
			summary.Total++
			if s.Status == "healthy" {
				summary.Healthy++
			} else {
				summary.Unhealthy++
			}
			summary.ByType["chat"]++
		}

		// Check TTS backends (Kokoro, Chatterbox).
		if ttsReg != nil {
			for _, info := range ttsReg.All() {
				if info.TTSBackend == nil {
					continue
				}

				s := ServiceStatus{Status: "healthy"}

				if err := backend.CheckBackendHealth(r.Context(), hc, info.Name, info.TTSBackend.Health); err != nil {
					s.Status = "unhealthy"
					s.Error = err.Error()
					overall = "degraded"
				} else {
					// Fetch voice inventory on healthy TTS backends.
					voices, voicesErr := info.TTSBackend.Voices(r.Context())
					if voicesErr == nil && len(voices) > 0 {
						briefs := make([]ModelBrief, len(voices))
						for i, v := range voices {
							briefs[i] = ModelBrief{
								ID:     v.ID,
								Object: "voice",
							}
						}
						s.Models = briefs
					}
				}

				services[info.Name] = s
				summary.Total++
				if s.Status == "healthy" {
					summary.Healthy++
				} else {
					summary.Unhealthy++
				}
				summary.ByType["tts"]++
			}
		}

		// If no services at all, report degraded.
		if len(services) == 0 {
			overall = "degraded"
			services["_none"] = ServiceStatus{
				Status: "unhealthy",
				Error:  "no backends registered",
			}
			summary.Total = 1
			summary.Unhealthy = 1
		}

		resp := HealthStatusResponse{
			Status:    overall,
			Version:   version.Version,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Services:  services,
			Summary:   summary,
		}

		if overall == "degraded" {
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		_ = json.NewEncoder(w).Encode(resp)
	}
}

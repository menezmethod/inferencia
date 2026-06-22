// Package handler implements HTTP handlers for the OpenAI-compatible API.
package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/menezmethod/inferencia/internal/backend"
	"github.com/menezmethod/inferencia/internal/middleware"
	"github.com/menezmethod/inferencia/internal/router"
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

// Ready handles readiness checks. It returns 200 only when every registered
// backend passes the combined watchdog + live Health() probe.
//
//	GET /health/ready
func Ready(reg *backend.Registry, ttsReg *router.Registry, hc backend.HealthChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		for _, b := range reg.All() {
			name := b.Name()
			if err := backend.CheckBackendHealth(r.Context(), hc, name, b.Health); err != nil {
				writeReadyUnavailable(w, name, err)
				return
			}
			middleware.BackendHealth.WithLabelValues(name).Set(1)
		}

		if ttsReg != nil {
			for _, info := range ttsReg.All() {
				if info.TTSBackend == nil {
					continue
				}
				name := info.Name
				if err := backend.CheckBackendHealth(r.Context(), hc, name, info.TTSBackend.Health); err != nil {
					writeReadyUnavailable(w, name, err)
					return
				}
				middleware.BackendHealth.WithLabelValues(name).Set(1)
			}
		}

		if len(reg.All()) == 0 && (ttsReg == nil || ttsReg.Len() == 0) {
			writeReadyUnavailable(w, "default", errors.New("no backends registered"))
			return
		}

		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "ready",
			"version": version.Version,
		})
	}
}

func writeReadyUnavailable(w http.ResponseWriter, backendName string, err error) {
	middleware.BackendHealth.WithLabelValues(backendName).Set(0)
	w.WriteHeader(http.StatusServiceUnavailable)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":  "unavailable",
		"backend": backendName,
		"error":   err.Error(),
		"version": version.Version,
	})
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

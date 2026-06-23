package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/menezmethod/inferencia/internal/apierror"
	"github.com/menezmethod/inferencia/internal/backend"
)

// Embeddings handles embedding creation requests.
//
//	POST /v1/embeddings
func Embeddings(reg *backend.Registry, hc backend.HealthChecker, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req backend.EmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			apierror.Write(w, apierror.InvalidRequest("Invalid JSON in request body: "+err.Error()))
			return
		}

		if len(req.Input) == 0 || string(req.Input) == "null" || string(req.Input) == `""` || string(req.Input) == "[]" {
			apierror.Write(w, apierror.InvalidParam("input", "input is required"))
			return
		}

		b, err := reg.PrimaryHealthy(hc)
		if err != nil {
			apierror.Write(w, backendSelectError(reg, err))
			return
		}
		defer reg.ReleaseBackend(b.Name())

		resp, err := b.CreateEmbedding(r.Context(), req)
		if err != nil {
			logger.Error("create embedding failed", "backend", b.Name(), "err", err)
			apierror.Write(w, apierror.FromBackendError(b.Name(), err))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			logger.Error("failed to encode embedding response", "err", err)
		}
	}
}

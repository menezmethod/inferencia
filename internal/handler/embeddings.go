package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/menez/inferencia/internal/apierror"
	"github.com/menez/inferencia/internal/backend"
)

// Embeddings handles embedding creation requests.
//
//	POST /v1/embeddings
func Embeddings(reg *backend.Registry, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req backend.EmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			apierror.Write(w, apierror.InvalidRequest("Invalid JSON in request body: "+err.Error()))
			return
		}

		if len(req.Input) == 0 {
			apierror.Write(w, apierror.InvalidParam("input", "input is required"))
			return
		}

		b, err := reg.Primary()
		if err != nil {
			apierror.Write(w, apierror.BackendUnavailable("default"))
			return
		}

		resp, err := b.CreateEmbedding(r.Context(), req)
		if err != nil {
			logger.Error("create embedding failed", "backend", b.Name(), "err", err)
			apierror.Write(w, apierror.BackendUnavailable(b.Name()))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			logger.Error("failed to encode embedding response", "err", err)
		}
	}
}

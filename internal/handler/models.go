package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/menezmethod/inferencia/internal/apierror"
	"github.com/menezmethod/inferencia/internal/backend"
)

// Models handles model listing requests.
//
//	GET /v1/models
func Models(reg *backend.Registry, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b, err := reg.Primary()
		if err != nil {
			apierror.Write(w, apierror.BackendUnavailable("default"))
			return
		}

		resp, err := b.ListModels(r.Context())
		if err != nil {
			logger.Error("list models failed", "backend", b.Name(), "err", err)
			apierror.Write(w, apierror.BackendUnavailable(b.Name()))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			logger.Error("failed to encode models response", "err", err)
		}
	}
}

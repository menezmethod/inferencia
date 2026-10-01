package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"github.com/menezmethod/inferencia/internal/apierror"
	"github.com/menezmethod/inferencia/internal/backend"
	"github.com/menezmethod/inferencia/internal/config"
	"github.com/menezmethod/inferencia/internal/decision"
)

// Models handles model listing requests.
//
//	GET /v1/models
//
// The response aggregates virtual role models from the routing table with the
// live model inventory of every healthy backend. Role IDs are advertised as
// "inferencia-<role>"; upstream IDs are always namespaced "backend/model"
// (even when the upstream ID itself contains "/") so every advertised ID
// round-trips through POST /v1/chat/completions.
func Models(reg *backend.Registry, hc backend.HealthChecker, routing config.Routing, decisions *decision.Set, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		seen := map[string]bool{}
		var data []backend.Model

		add := func(id, ownedBy string) {
			if id == "" || seen[id] {
				return
			}
			seen[id] = true
			data = append(data, backend.Model{ID: id, Object: "model", OwnedBy: ownedBy})
		}

		for role := range routing.Roles {
			name := role
			if !strings.HasPrefix(name, "inferencia-") {
				name = "inferencia-" + name
			}
			add(name, "inferencia")
		}

		// Decision engines (Jev, Laya): the engine itself plus its model IDs,
		// namespaced "engine/model" so requests resolve back to it.
		for _, client := range decisions.All() {
			add(client.Name(), "decision")
			for _, m := range client.Models() {
				add(client.Name()+"/"+m, client.Name())
			}
		}

		for _, b := range reg.All() {
			if hc != nil && !hc.IsHealthy(b.Name()) {
				continue
			}
			resp, err := b.ListModels(r.Context())
			if err != nil {
				logger.Warn("list models failed, skipping backend", "backend", b.Name(), "err", err)
				continue
			}
			if resp == nil {
				continue
			}
			for _, m := range resp.Data {
				add(namespaced(b.Name(), m.ID), b.Name())
			}
		}

		if len(data) == 0 {
			apierror.Write(w, apierror.BackendUnavailable(firstBackendName(reg)))
			return
		}

		sort.Slice(data, func(i, j int) bool { return data[i].ID < data[j].ID })
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(backend.ModelsResponse{Object: "list", Data: data}); err != nil {
			logger.Error("failed to encode models response", "err", err)
		}
	}
}

func namespaced(backendName, id string) string {
	if backendName == "" {
		return id
	}
	return backendName + "/" + id
}

func firstBackendName(reg *backend.Registry) string {
	if name := reg.PrimaryName(); name != "" {
		return name
	}
	return "default"
}

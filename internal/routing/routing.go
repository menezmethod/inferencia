// Package routing resolves a requested chat model name to a concrete
// backend + upstream model ID, supporting virtual role models
// (inferencia-planner, inferencia-worker, inferencia-auto, inferencia-fast,
// inferencia-reviewer) and direct "backend/model" or plain model IDs.
package routing

import (
	"strings"

	"github.com/menezmethod/inferencia/internal/backend"
	"github.com/menezmethod/inferencia/internal/config"
)

// Resolution is the outcome of Resolve: which backend executes the request
// and which upstream model ID it must send.
type Resolution struct {
	Backend       backend.Backend
	BackendName   string
	UpstreamModel string
	Role          string
	Advertised    string
}

// Resolve maps a requested model string to a backend and upstream model.
// Mapping rules, in order:
//  1. Exact virtual role ("inferencia-planner", "inferencia-auto", ...)
//     or bare role alias ("planner", "auto", ...) -> role table, first healthy backend.
//  2. "backend/model" with a known backend name -> that backend, upstream model verbatim.
//  3. "prefix/rest" with an unknown prefix -> backend whose name matches the prefix
//     if healthy (covers "commandcode/..." when the backend is named "commandcode"),
//     else unknown-model fallback.
//  4. Bare model ID -> healthy backend advertising it, else primary healthy.
//  5. Unknown model -> default role when configured, else primary healthy.
//
// Unknown models never hard-fail when any backend is healthy; the resolution
// reports the fallback role so handlers can warn.
//
// Callers must always ReleaseBackend(res.BackendName) when the request
// completes; the primary-healthy fallback acquires a load-balancer slot here.
func Resolve(reg *backend.Registry, hc backend.HealthChecker, cfg config.Routing, requested string) (Resolution, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		requested = cfg.DefaultRole
	}
	if role, ok := lookupRole(cfg, requested); ok {
		return resolveRole(reg, hc, cfg, role, requested)
	}
	if backendName, upstream, ok := strings.Cut(requested, "/"); ok {
		if b, err := reg.Get(backendName); err == nil {
			if hc == nil || hc.IsHealthy(b.Name()) {
				return Resolution{Backend: b, BackendName: b.Name(), UpstreamModel: upstream, Role: "", Advertised: requested}, nil
			}
		}
		if b, err := healthyByPrefix(reg, hc, backendName); err == nil {
			rest := strings.TrimPrefix(requested, backendName+"/")
			return Resolution{Backend: b, BackendName: b.Name(), UpstreamModel: rest, Role: "", Advertised: requested}, nil
		}
		return resolveUnknown(reg, hc, cfg, requested)
	}
	return resolveUnknown(reg, hc, cfg, requested)
}

func lookupRole(cfg config.Routing, requested string) (string, bool) {
	if _, ok := cfg.Roles[requested]; ok {
		return requested, true
	}
	if strings.HasPrefix(requested, "inferencia-") {
		if _, ok := cfg.Roles[strings.TrimPrefix(requested, "inferencia-")]; ok {
			return strings.TrimPrefix(requested, "inferencia-"), true
		}
		return "", false
	}
	if _, ok := cfg.Roles["inferencia-"+requested]; ok {
		return "inferencia-" + requested, true
	}
	return "", false
}

func resolveRole(reg *backend.Registry, hc backend.HealthChecker, cfg config.Routing, role, advertised string) (Resolution, error) {
	r := cfg.Roles[role]
	var backends []backend.Backend
	if r.Backend != "" {
		if b, err := reg.Get(r.Backend); err == nil && (hc == nil || hc.IsHealthy(b.Name())) {
			backends = append(backends, b)
		}
	} else {
		for _, b := range reg.All() {
			if hc == nil || hc.IsHealthy(b.Name()) {
				backends = append(backends, b)
			}
		}
	}
	if len(backends) == 0 {
		return Resolution{}, backend.ErrNoHealthyBackend
	}
	b := leastLoaded(backends)
	upstream := ""
	if len(r.Models) > 0 {
		upstream = stripBackendPrefix(r.Models[0], b.Name())
	}
	return Resolution{Backend: b, BackendName: b.Name(), UpstreamModel: upstream, Role: roleName(role), Advertised: advertised}, nil
}

func resolveUnknown(reg *backend.Registry, hc backend.HealthChecker, cfg config.Routing, requested string) (Resolution, error) {
	if cfg.DefaultRole != "" {
		if role, ok := lookupRole(cfg, cfg.DefaultRole); ok {
			res, err := resolveRole(reg, hc, cfg, role, requested)
			if err == nil {
				res.Advertised = requested
				return res, nil
			}
		}
	}
	b, err := reg.PrimaryHealthy(hc)
	if err != nil {
		return Resolution{}, err
	}
	defer reg.ReleaseBackend(b.Name())
	return Resolution{Backend: b, BackendName: b.Name(), UpstreamModel: requested, Role: "", Advertised: requested}, nil
}

func healthyByPrefix(reg *backend.Registry, hc backend.HealthChecker, prefix string) (backend.Backend, error) {
	for _, b := range reg.All() {
		if strings.HasPrefix(b.Name(), prefix) || strings.HasPrefix(prefix, b.Name()) {
			if hc == nil || hc.IsHealthy(b.Name()) {
				return b, nil
			}
		}
	}
	return nil, backend.ErrNoHealthyBackend
}

func roleName(role string) string {
	return strings.TrimPrefix(role, "inferencia-")
}

func stripBackendPrefix(model, backendName string) string {
	if rest, ok := strings.CutPrefix(model, backendName+"/"); ok {
		return rest
	}
	return model
}

// leastLoaded picks deterministically among healthy backends: the caller
// owns load balancing via Acquire/Release around the actual request, so
// Resolve stays side-effect free and the first candidate wins ties.
func leastLoaded(backends []backend.Backend) backend.Backend {
	return backends[0]
}

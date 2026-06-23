package router

import (
	"sort"

	"github.com/menezmethod/inferencia/internal/backend"
)

// SelectBackend selects the best backend for the given capability and optional model name.
// If a model is specified, it prefers backends that advertise that model.
// If no model is specified, it returns the first backend that supports the capability.
func (r *Registry) SelectBackend(kind Capability, model string) (BackendInfo, error) {
	return r.selectBackend(kind, model, nil)
}

// SelectHealthyBackend skips backends the health checker marks degraded.
func (r *Registry) SelectHealthyBackend(kind Capability, model string, hc backend.HealthChecker) (BackendInfo, error) {
	return r.selectBackend(kind, model, hc)
}

// ReleaseBackend decrements the in-flight counter after a routed request completes.
func (r *Registry) ReleaseBackend(name string) {
	r.lb.Release(name)
}

func (r *Registry) selectBackend(kind Capability, model string, hc backend.HealthChecker) (BackendInfo, error) {
	candidates := r.BackendsByCapability(kind)
	if len(candidates) == 0 {
		return BackendInfo{}, ErrCapabilityNotSupported
	}

	var healthy []BackendInfo
	for _, c := range candidates {
		if hc == nil || hc.IsHealthy(c.Name) {
			healthy = append(healthy, c)
		}
	}
	if len(healthy) == 0 {
		return BackendInfo{}, backend.ErrNoHealthyBackend
	}

	if model == "" {
		return r.pickBalanced(healthy), nil
	}

	type scored struct {
		info  BackendInfo
		score int
	}
	var scoredCandidates []scored

	for _, c := range healthy {
		s := scoreBackend(c, kind, model)
		scoredCandidates = append(scoredCandidates, scored{info: c, score: s})
	}

	sort.SliceStable(scoredCandidates, func(i, j int) bool {
		return scoredCandidates[i].score > scoredCandidates[j].score
	})

	// Require at least a prefix model match (score >= 50). A capability-only
	// match (score 10) means no healthy backend advertises the requested model.
	if scoredCandidates[0].score < 50 {
		return BackendInfo{}, backend.ErrNoHealthyBackend
	}

	topScore := scoredCandidates[0].score
	var topTier []BackendInfo
	for _, sc := range scoredCandidates {
		if sc.score == topScore {
			topTier = append(topTier, sc.info)
		}
	}
	return r.pickBalanced(topTier), nil
}

func (r *Registry) pickBalanced(candidates []BackendInfo) BackendInfo {
	names := make([]string, len(candidates))
	byName := make(map[string]BackendInfo, len(candidates))
	for i, c := range candidates {
		names[i] = c.Name
		byName[c.Name] = c
	}
	picked := r.lb.Select(names)
	r.lb.Acquire(picked)
	return byName[picked]
}

// scoreBackend computes a match score for a backend against a capability + model.
// Higher score = better match.
func scoreBackend(info BackendInfo, kind Capability, model string) int {
	score := 0

	// Exact model match is the strongest signal.
	for _, m := range info.Models {
		if m.ID == model && m.Kind == kind {
			score += 100
			continue
		}
		// Partial/prefix model match.
		if len(model) <= len(m.ID) && m.ID[:len(model)] == model && m.Kind == kind {
			score += 50
		}
	}

	// Bonus for having the capability (already filtered, but confirms).
	for _, c := range info.Capabilities {
		if c == kind {
			score += 10
			break
		}
	}

	return score
}

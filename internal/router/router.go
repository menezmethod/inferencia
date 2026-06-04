package router

import "sort"

// SelectBackend selects the best backend for the given capability and optional model name.
// If a model is specified, it prefers backends that advertise that model.
// If no model is specified, it returns the first backend that supports the capability.
func (r *Registry) SelectBackend(kind Capability, model string) (BackendInfo, error) {
	candidates := r.BackendsByCapability(kind)
	if len(candidates) == 0 {
		return BackendInfo{}, ErrCapabilityNotSupported
	}

	// If no model specified, return the first candidate.
	if model == "" {
		return candidates[0], nil
	}

	// Score candidates: higher score = better match.
	type scored struct {
		info  BackendInfo
		score int
	}
	var scoredCandidates []scored

	for _, c := range candidates {
		s := scoreBackend(c, kind, model)
		scoredCandidates = append(scoredCandidates, scored{info: c, score: s})
	}

	// Sort by score descending.
	sort.SliceStable(scoredCandidates, func(i, j int) bool {
		return scoredCandidates[i].score > scoredCandidates[j].score
	})

	return scoredCandidates[0].info, nil
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

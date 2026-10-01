package handler

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/menezmethod/inferencia/internal/apierror"
	"github.com/menezmethod/inferencia/internal/backend"
	"github.com/menezmethod/inferencia/internal/config"
	"github.com/menezmethod/inferencia/internal/decision"
)

// Auto policies are named variants over the same planner, selected as model
// suffixes: "auto" (= balanced), "auto:quality", "auto:economy", "auto:free".
const (
	policyBalanced = "balanced"
	policyQuality  = "quality"
	policyEconomy  = "economy"
	policyFree     = "free"
)

// Machine-readable reason codes for auto decisions, exposed as X-Routed-Reason
// and recorded in the "auto decision" log line for later learning.
const (
	reasonTierSelected   = "AUTO_TIER_SELECTED"
	reasonLocalOnly      = "AUTO_POLICY_LOCAL_ONLY"
	reasonLowConfidence  = "AUTO_LOW_CONFIDENCE_FALLBACK"
	reasonUnavailable    = "AUTO_DECISION_UNAVAILABLE"
	reasonStaticFallback = "AUTO_STATIC_FALLBACK"
)

var autoPolicyNames = map[string]bool{
	policyBalanced: true,
	policyQuality:  true,
	policyEconomy:  true,
	policyFree:     true,
}

// parseAutoModel reports whether the requested model asks for auto routing and
// which policy it names. Bare "auto" is balanced; "inferencia-auto:free" is
// accepted too.
func parseAutoModel(model string) (string, bool) {
	trimmed := strings.TrimPrefix(strings.TrimSpace(model), "inferencia-")
	base, policy, found := strings.Cut(trimmed, ":")
	if base != "auto" {
		return "", false
	}
	if !found {
		return policyBalanced, true
	}
	return policy, true
}

// autoPlan is the outcome of one decision-engine consultation.
type autoPlan struct {
	tier       string
	confidence float64
	reason     string
	fallback   bool
	stateBytes int
}

// planAuto asks the configured decision engine which role should handle the
// request under the given policy, applying the confidence gate and the
// local-only constraint deterministically.
func planAuto(ctx context.Context, decisions *decision.Set, cfg config.Routing, local map[string]bool, policy string, messages []backend.Message) (autoPlan, *apierror.Error) {
	p := effectivePolicy(policy, cfg.Auto.Policies[policy])
	criteria := p.Criteria
	if len(criteria) == 0 {
		criteria = autoCriteria(cfg)
	}
	minConfidence := p.MinConfidence
	if minConfidence == 0 {
		minConfidence = cfg.Auto.MinConfidence
	}
	fallbackTier := autoFallbackTier(cfg, local, p)

	client, ok := decisions.Get(cfg.Auto.Decision)
	if !ok {
		return autoPlan{tier: fallbackTier, reason: reasonUnavailable, fallback: true}, nil
	}
	state := lastUserText(messages)
	if state == "" {
		return autoPlan{tier: fallbackTier, reason: reasonUnavailable, fallback: true}, nil
	}

	plan := autoPlan{stateBytes: len(state)}
	tier, confidence, err := client.Pick(ctx, state, cfg.Auto.Question, cfg.Auto.Model, criteria, minConfidence)
	switch {
	case err == nil:
		plan.tier, plan.confidence, plan.reason = tier, confidence, reasonTierSelected
	case errors.Is(err, decision.ErrAbstained):
		plan.tier, plan.reason, plan.fallback = fallbackTier, reasonLowConfidence, true
	default:
		plan.tier, plan.reason, plan.fallback = fallbackTier, reasonUnavailable, true
	}

	// free policy: the planner may never spend paid credits.
	if p.LocalOnly && !isLocalTier(cfg, local, plan.tier) {
		localTier, found := preferredLocalTier(cfg, local, plan.tier, p.Fallback)
		if !found {
			return autoPlan{}, apierror.BackendUnavailable("local")
		}
		// The engine's confidence described the paid pick; it no longer applies.
		plan.tier, plan.confidence, plan.reason, plan.fallback = localTier, 0, reasonLocalOnly, true
	}
	return plan, nil
}

// effectivePolicy applies the built-in semantics of each policy on top of the
// configured overrides: free never spends paid credits, economy holds a higher
// confidence bar, quality leans to the strongest role on fallback.
func effectivePolicy(policy string, configured config.AutoPolicy) config.AutoPolicy {
	switch policy {
	case policyFree:
		configured.LocalOnly = true
	case policyEconomy:
		if configured.MinConfidence == 0 {
			configured.MinConfidence = 0.6
		}
	case policyQuality:
		if configured.Fallback == "" {
			configured.Fallback = "inferencia-planner"
		}
	}
	return configured
}

// autoFallbackTier resolves the role used when the engine cannot decide.
func autoFallbackTier(cfg config.Routing, local map[string]bool, p config.AutoPolicy) string {
	if p.LocalOnly {
		if tier, ok := firstLocalTier(cfg, local); ok {
			return tier
		}
		return ""
	}
	if p.Fallback != "" {
		return p.Fallback
	}
	if cfg.DefaultRole != "" {
		return cfg.DefaultRole
	}
	return "auto" // static auto role, when configured
}

// autoCriteria returns the choice criteria for auto routing: configured
// criteria when present, otherwise every configured role as its own label.
func autoCriteria(cfg config.Routing) map[string]string {
	if len(cfg.Auto.Criteria) > 0 {
		return cfg.Auto.Criteria
	}
	out := make(map[string]string)
	for name := range cfg.Roles {
		bare := strings.TrimPrefix(name, "inferencia-")
		if bare == "auto" {
			continue
		}
		out[bare] = bare
	}
	return out
}

// roleKey resolves a tier name to its configured role key, accepting bare and
// prefixed forms ("worker" or "inferencia-worker").
func roleKey(cfg config.Routing, tier string) (string, bool) {
	if _, ok := cfg.Roles[tier]; ok {
		return tier, true
	}
	if _, ok := cfg.Roles["inferencia-"+tier]; ok {
		return "inferencia-" + tier, true
	}
	if bare := strings.TrimPrefix(tier, "inferencia-"); bare != tier {
		if _, ok := cfg.Roles[bare]; ok {
			return bare, true
		}
	}
	return "", false
}

// isLocalTier reports whether the tier's role runs on a local (free) backend.
func isLocalTier(cfg config.Routing, local map[string]bool, tier string) bool {
	key, ok := roleKey(cfg, tier)
	if !ok {
		return false
	}
	backendName := cfg.Roles[key].Backend
	return backendName != "" && local[backendName]
}

// preferredLocalTier picks a free/local role for a free-policy request: the
// engine's pick when it is already local, else the policy fallback, else the
// first local role in sorted order.
func preferredLocalTier(cfg config.Routing, local map[string]bool, picked, fallback string) (string, bool) {
	if isLocalTier(cfg, local, picked) {
		return picked, true
	}
	if fallback != "" && isLocalTier(cfg, local, fallback) {
		return fallback, true
	}
	return firstLocalTier(cfg, local)
}

// firstLocalTier returns the first local role in deterministic (sorted) order.
func firstLocalTier(cfg config.Routing, local map[string]bool) (string, bool) {
	names := make([]string, 0, len(cfg.Roles))
	for name := range cfg.Roles {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if isLocalTier(cfg, local, name) {
			return name, true
		}
	}
	return "", false
}

// lastUserText returns the most recent user message as plain text, joining
// content parts when the message uses the array form.
func lastUserText(messages []backend.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "user" {
			continue
		}
		var text string
		if err := json.Unmarshal(messages[i].Content, &text); err == nil && strings.TrimSpace(text) != "" {
			return text
		}
		var parts []struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(messages[i].Content, &parts); err == nil {
			var b strings.Builder
			for _, p := range parts {
				if p.Text != "" {
					b.WriteString(p.Text)
					b.WriteString("\n")
				}
			}
			if joined := strings.TrimSpace(b.String()); joined != "" {
				return joined
			}
		}
	}
	return ""
}

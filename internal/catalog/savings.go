package catalog

import (
	"cmp"
	"errors"
	"slices"

	"github.com/menezmethod/inferencia/internal/ledger"
)

// TokenizerBand is the assumed spread when the same work runs on a model with
// a different tokenizer. It is an assumption, not a measurement; reports
// print it as a range and label it.
const TokenizerBand = 0.15

// CostAs prices usage as if it had run on m with the same token split. Where m
// lacks a price the source model used, it maps the tokens to the nearest price
// m has: 1-hour writes to 5-minute writes, writes and reads to plain input.
// The result is an estimate. It fails only on invalid usage.
func (m *Model) CostAs(u ledger.Usage) (float64, error) {
	p := m.PriceFor(u)
	if p.CacheWrite1h == nil {
		u.CacheWrite1h = 0
	}
	if p.CacheWrite == nil {
		u.Input += u.CacheWrite
		u.CacheWrite, u.CacheWrite1h = 0, 0
	}
	if p.CacheRead == nil {
		u.Input += u.CacheRead
		u.CacheRead = 0
	}
	return m.Cost(u)
}

// Line is one row of a savings or subs report.
type Line struct {
	Key    ledger.Key
	Totals ledger.Totals
	Actual float64            // API-equivalent cost at the model's own list price
	As     map[string]float64 // counterfactual cost per alternative model id
}

// Unpriced collects usage the catalog cannot price, keyed by model and reason,
// so a report can show it instead of counting it as $0.
type Unpriced map[string]ledger.Totals

// file records u as unpriced when err says it cannot be priced, and reports
// whether it did. Any other error is left to the caller.
func (up Unpriced) file(u ledger.Usage, err error) bool {
	reason := ""
	switch {
	case errors.Is(err, ErrUnknownModel):
		reason = "not in catalog"
	case errors.Is(err, ErrMissingPrice):
		reason = "price missing"
	default:
		return false
	}
	k := u.Model + " (" + reason + ")"
	t := up[k]
	t.Add(u)
	up[k] = t
	return true
}

func empty(u ledger.Usage) bool {
	return u.Input == 0 && u.CacheRead == 0 && u.CacheWrite == 0 && u.Output == 0
}

// Savings prices each usage group at its own model and at each alternative.
func (c *Catalog) Savings(us []ledger.Usage, alts []string) ([]Line, Unpriced, error) {
	altModels := make([]*Model, 0, len(alts))
	for _, a := range alts {
		m, err := c.Lookup(a)
		if err != nil {
			return nil, nil, err
		}
		altModels = append(altModels, m)
	}
	lines := map[ledger.Key]*Line{}
	unpriced := Unpriced{}
	for _, u := range us {
		if empty(u) {
			continue // no tokens, nothing to price
		}
		actual, err := c.Cost(u)
		if unpriced.file(u, err) {
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		k := ledger.Key{Harness: u.Harness, Upstream: u.Upstream, Model: u.Model, Billing: u.Billing, Delegated: u.Delegated}
		l := lines[k]
		if l == nil {
			l = &Line{Key: k, As: map[string]float64{}}
			lines[k] = l
		}
		l.Totals.Add(u)
		l.Actual += actual
		for _, m := range altModels {
			v, err := m.CostAs(u)
			if err != nil {
				return nil, nil, err
			}
			l.As[m.ID] += v
		}
	}
	return sortedLines(lines), unpriced, nil
}

// CheapestPerTier picks, for each tier, the model that would have cost the
// least on this usage. It is the default set of alternatives for savings.
func (c *Catalog) CheapestPerTier(us []ledger.Usage) []string {
	best := map[string]struct {
		id  string
		usd float64
	}{}
	for i := range c.Models {
		m := &c.Models[i]
		var usd float64
		ok := true
		for _, u := range us {
			v, err := m.CostAs(u)
			if err != nil {
				ok = false
				break
			}
			usd += v
		}
		if !ok {
			continue
		}
		if b, seen := best[m.Tier]; !seen || usd < b.usd {
			best[m.Tier] = struct {
				id  string
				usd float64
			}{m.ID, usd}
		}
	}
	var out []string
	for _, tier := range []string{"frontier", "strong", "fast", "local"} {
		if b, ok := best[tier]; ok {
			out = append(out, b.id)
		}
	}
	return out
}

// PlanMonth is one plan's usage in one calendar month (UTC).
type PlanMonth struct {
	Plan   *Plan
	Month  string // YYYY-MM
	Totals ledger.Totals
	Value  float64 // API-equivalent dollars at catalog list prices
}

// Subs prices plan-covered usage per plan and month.
func (c *Catalog) Subs(us []ledger.Usage) ([]PlanMonth, Unpriced, error) {
	type key struct{ plan, month string }
	months := map[key]*PlanMonth{}
	unpriced := Unpriced{}
	for _, u := range us {
		p, ok := c.PlanFor(u)
		if !ok || empty(u) || u.Time.IsZero() {
			continue
		}
		v, err := c.Cost(u)
		if unpriced.file(u, err) {
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		k := key{p.ID, u.Time.UTC().Format("2006-01")}
		pm := months[k]
		if pm == nil {
			pm = &PlanMonth{Plan: p, Month: k.month}
			months[k] = pm
		}
		pm.Totals.Add(u)
		pm.Value += v
	}
	out := make([]PlanMonth, 0, len(months))
	for _, pm := range months {
		out = append(out, *pm)
	}
	slices.SortFunc(out, func(a, b PlanMonth) int {
		return cmp.Or(cmp.Compare(a.Plan.ID, b.Plan.ID), cmp.Compare(a.Month, b.Month))
	})
	return out, unpriced, nil
}

func sortedLines(m map[ledger.Key]*Line) []Line {
	out := make([]Line, 0, len(m))
	for _, l := range m {
		out = append(out, *l)
	}
	slices.SortFunc(out, func(a, b Line) int {
		return cmp.Or(cmp.Compare(b.Actual, a.Actual), cmp.Compare(a.Key.Harness, b.Key.Harness), cmp.Compare(a.Key.Model, b.Key.Model),
			cmp.Compare(a.Key.Upstream, b.Key.Upstream), boolInt(a.Key.Delegated)-boolInt(b.Key.Delegated))
	})
	return out
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

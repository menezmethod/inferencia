// Package replay re-prices recorded usage under routing policies. It answers
// "what would these exact sessions have cost if a router had chosen the
// models?" without calling any model. All results are estimates.
package replay

import (
	"cmp"
	"errors"
	"slices"

	"github.com/menezmethod/inferencia/internal/catalog"
	"github.com/menezmethod/inferencia/internal/ledger"
)

// Policy picks the model for one call. prev is the model the previous call in
// the same session ran on, or "" for the first call.
type Policy func(u ledger.Usage, prev string) string

// Result is one policy's outcome over a set of usage.
type Result struct {
	Name         string
	USD          float64 // estimated cost, cache losses included
	CacheLossUSD float64 // the part of USD caused by switching models mid-session
	Switches     int     // calls that ran on a different model than the call before
	Calls        int
	Unpriced     ledger.Totals // usage skipped because a model has no price
}

// Run applies p to every call in session order and prices the result. When a
// policy switches model inside a session where the recorded run did not, the
// new model has none of the conversation in its cache: the tokens the old
// model read from cache are written again, so that call pays a cache write
// (or plain input where the model has no write price) instead of a cache read.
// A switch the recorded run already made is already in its token counts.
//
// A conversation is (harness, session id, agent, delegated): Claude Code
// subagents share their parent's session id but each has its own
// conversation and cache. Models are compared by catalog id, so an alias is
// not a switch. Only single-call records can switch: a Codex record is a whole
// session, so there is no turn boundary inside it to switch at. The re-written
// tokens go to the 1-hour cache when the recorded call used it.
func Run(c *catalog.Catalog, us []ledger.Usage, name string, p Policy) (Result, error) {
	calls := slices.Clone(us)
	slices.SortStableFunc(calls, func(a, b ledger.Usage) int {
		return cmp.Or(cmp.Compare(a.Harness, b.Harness), cmp.Compare(a.Session, b.Session), a.Time.Compare(b.Time))
	})
	r := Result{Name: name}
	prev := map[string]string{}    // conversation -> catalog id of the policy model of its last call
	prevRun := map[string]string{} // conversation -> catalog id of the recorded model of its last call
	for _, u := range calls {
		key := u.Harness + "\x00" + u.Session + "\x00" + u.Agent
		if u.Delegated {
			key += "\x00sub"
		}
		m, err := c.Lookup(p(u, prev[key]))
		if err == nil {
			_, err = m.CostAs(u) // probe: is this usage priceable on m?
		}
		if errors.Is(err, catalog.ErrUnknownModel) || errors.Is(err, catalog.ErrMissingPrice) {
			r.Unpriced.Add(u)
			continue
		}
		if err != nil {
			return Result{}, err
		}
		run := u.Model
		if rm, lookErr := c.Lookup(u.Model); lookErr == nil {
			run = rm.ID
		}
		base, _ := m.CostAs(u)
		cost := base
		if last := prev[key]; u.Calls == 1 && last != "" && last != m.ID && prevRun[key] == run {
			r.Switches++
			lost := u
			lost.CacheWrite += lost.CacheRead
			if u.CacheWrite1h > 0 {
				lost.CacheWrite1h += lost.CacheRead
			}
			lost.CacheRead = 0
			if cost, err = m.CostAs(lost); err != nil {
				return Result{}, err
			}
			r.CacheLossUSD += cost - base
		}
		prev[key], prevRun[key] = m.ID, run
		r.USD += cost
		r.Calls += u.Calls
	}
	return r, nil
}

// Priceable splits usage into the calls the catalog can price as they ran and
// the rest. Compare policies only on the priceable set, so every policy is
// judged on the same calls.
func Priceable(c *catalog.Catalog, us []ledger.Usage) (ok []ledger.Usage, unpriced ledger.Totals) {
	for _, u := range us {
		if _, err := c.Cost(u); err != nil {
			unpriced.Add(u)
			continue
		}
		ok = append(ok, u)
	}
	return ok, unpriced
}

// AsRun keeps every call on the model it ran on.
func AsRun(u ledger.Usage, _ string) string { return u.Model }

// Floor keeps the main loop (solo calls) on the user's model and sends
// delegated calls to the given cheaper model. It never switches inside a
// session, because a subagent runs in its own session.
func Floor(delegatedModel string) Policy {
	return func(u ledger.Usage, _ string) string {
		if u.Delegated {
			return delegatedModel
		}
		return u.Model
	}
}

// MainOn moves solo calls on any of the given models to target, for the whole
// session; delegated calls stay as run.
func MainOn(from map[string]bool, target string) Policy {
	return func(u ledger.Usage, _ string) string {
		if !u.Delegated && from[u.Model] {
			return target
		}
		return u.Model
	}
}

// NaivePerCall routes each call by its own context size: below the threshold
// to small, otherwise to large. It ignores the session, so it switches models
// whenever a session crosses the threshold. It shows the cache trap.
func NaivePerCall(threshold int64, small, large string) Policy {
	return func(u ledger.Usage, _ string) string {
		perCall := (u.Input + u.CacheRead + u.CacheWrite) / int64(max(u.Calls, 1))
		if perCall < threshold {
			return small
		}
		return large
	}
}

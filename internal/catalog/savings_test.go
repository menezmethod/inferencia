package catalog

import (
	"testing"
	"time"

	"github.com/menezmethod/inferencia/internal/ledger"
)

func TestCostAsMapsMissingPricesToNearest(t *testing.T) {
	c := mustDefault(t)
	m, _ := c.Lookup("deepseek-v4-flash") // input 0.091, output 0.182, cache read 0.0182; no write prices
	u := ledger.Usage{Model: "claude-opus-5-5", Input: 1_000_000, CacheRead: 1_000_000, CacheWrite: 2_000_000, CacheWrite1h: 1_000_000, Output: 1_000_000}
	got, err := m.CostAs(u)
	if err != nil {
		t.Fatal(err)
	}
	near(t, got, 3*0.091+1*0.0182+1*0.182) // writes priced as plain input
	if _, err := m.Cost(u); err == nil {
		t.Error("Cost (not CostAs) must refuse writes the model has no price for")
	}
}

func TestSavingsGroupsAndFlagsUnpriced(t *testing.T) {
	c := mustDefault(t)
	us := []ledger.Usage{
		{Harness: "h", Model: "claude-opus-5-5", Calls: 1, Input: 1_000_000, Output: 1_000_000},
		{Harness: "h", Model: "claude-opus-5-5", Calls: 1, Input: 1_000_000},
		{Harness: "h", Model: "mystery-model", Calls: 3, Input: 7},
	}
	lines, unpriced, err := c.Savings(us, []string{"deepseek-v4-flash"})
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 {
		t.Fatalf("%d lines", len(lines))
	}
	l := lines[0]
	near(t, l.Actual, 2*4+1*20)                       // 28
	near(t, l.As["deepseek-v4-flash"], 2*0.091+0.182) // 0.364
	if l.Totals.Calls != 2 {
		t.Errorf("calls %d", l.Totals.Calls)
	}
	if unpriced["mystery-model (not in catalog)"].Calls != 3 {
		t.Errorf("unpriced: %+v", unpriced)
	}
}

func TestSavingsFilesMissingPricesAndSkipsEmptyUsage(t *testing.T) {
	c := mustDefault(t)
	us := []ledger.Usage{
		{Harness: "h", Model: "gpt-5.4", Calls: 1, CacheWrite: 10}, // gpt-5.4 has no cache_write price
		{Harness: "h", Model: "no-tokens-model", Calls: 5},         // nothing to price: skipped, not flagged
	}
	lines, unpriced, err := c.Savings(us, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 0 || len(unpriced) != 1 || unpriced["gpt-5.4 (price missing)"].Calls != 1 {
		t.Fatalf("lines %v, unpriced %v", lines, unpriced)
	}
}

func TestCostAsFallsBackFromOneHourToFiveMinuteWrites(t *testing.T) {
	c := mustDefault(t)
	m, _ := c.Lookup("gpt-6-astra") // cache_write 12.50, no 1-hour price
	got, err := m.CostAs(ledger.Usage{CacheWrite: 1_000_000, CacheWrite1h: 1_000_000})
	if err != nil {
		t.Fatal(err)
	}
	near(t, got, 12.50)
}

func TestLongContextPriceAppliesPerCall(t *testing.T) {
	c := mustDefault(t)
	// claude-haiku-5-5: base input 0.10; above 100k prompt tokens per request, input 0.50.
	short := ledger.Usage{Model: "claude-haiku-5-5", Calls: 10, Input: 900_000} // 90k per call
	long := ledger.Usage{Model: "claude-haiku-5-5", Calls: 9, Input: 900_000}   // 100,000 per call: not over
	longer := ledger.Usage{Model: "claude-haiku-5-5", Calls: 1, Input: 100_001}
	for _, tc := range []struct {
		u    ledger.Usage
		want float64
	}{{short, 0.09}, {long, 0.09}, {longer, 0.100001 * 0.50}} {
		got, err := c.Cost(tc.u)
		if err != nil {
			t.Fatal(err)
		}
		near(t, got, tc.want)
	}
}

func TestSavingsRejectsUnknownAlternative(t *testing.T) {
	c := mustDefault(t)
	if _, _, err := c.Savings(nil, []string{"nope"}); err == nil {
		t.Fatal("want an error for an unknown alternative")
	}
}

func TestCheapestPerTier(t *testing.T) {
	c := mustDefault(t)
	us := []ledger.Usage{{Model: "x", Input: 1_000_000, CacheRead: 9_000_000, Output: 100_000}}
	alts := c.CheapestPerTier(us)
	if len(alts) != 3 { // the embedded catalog has no local-tier model
		t.Fatalf("got %v, want one per tier (frontier, strong, fast)", alts)
	}
	for i, tier := range []string{"frontier", "strong", "fast"} {
		m, _ := c.Lookup(alts[i])
		if m.Tier != tier {
			t.Errorf("pick %d is %s (%s), want tier %s", i, m.ID, m.Tier, tier)
		}
	}
	for _, id := range alts {
		if _, err := c.Lookup(id); err != nil {
			t.Error(err)
		}
	}
	// The fast pick must not cost more than any other fast model on this usage.
	fast, _ := c.Lookup(alts[2])
	best, _ := fast.CostAs(us[0])
	for i := range c.Models {
		m := &c.Models[i]
		if m.Tier != "fast" {
			continue
		}
		if v, err := m.CostAs(us[0]); err == nil && v < best-1e-12 {
			t.Errorf("%s (%.6f) is cheaper than the pick %s (%.6f)", m.ID, v, fast.ID, best)
		}
	}
}

func TestSubsGroupsByPlanAndMonth(t *testing.T) {
	c := mustDefault(t)
	sep := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
	oct := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	us := []ledger.Usage{
		{Harness: "opencode", Upstream: "opencode-go", Model: "glm-5.2", Time: sep, Calls: 1, Input: 1_000_000},
		{Harness: "opencode", Upstream: "opencode-go", Model: "glm-5.2", Time: oct, Calls: 1, Input: 2_000_000},
		{Harness: "opencode", Upstream: "openrouter", Model: "glm-5.2", Time: oct, Calls: 1, Input: 5_000_000}, // pay per token, not a plan
		{Harness: "codex", Upstream: "openai", Model: "gpt-6-astra", Time: oct, Calls: 2, Input: 1_000_000},
	}
	pms, unpriced, err := c.Subs(us)
	if err != nil {
		t.Fatal(err)
	}
	if len(unpriced) != 0 {
		t.Errorf("unpriced: %v", unpriced)
	}
	if len(pms) != 3 {
		t.Fatalf("got %d plan-months: %+v", len(pms), pms)
	}
	if pms[0].Plan.ID != "chatgpt" || pms[0].Month != "2026-10" {
		t.Errorf("first: %s %s", pms[0].Plan.ID, pms[0].Month)
	}
	near(t, pms[0].Value, 10)   // 1M input on gpt-6-astra at $10
	near(t, pms[1].Value, 0.62) // September, glm-5.2
	near(t, pms[2].Value, 1.24) // October, openrouter call excluded
}

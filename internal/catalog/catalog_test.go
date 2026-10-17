package catalog

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/menezmethod/inferencia/internal/ledger"
)

func mustDefault(t *testing.T) *Catalog {
	t.Helper()
	c, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func near(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("got %.10f, want %.10f", got, want)
	}
}

func TestEmbeddedCatalogIsValid(t *testing.T) {
	c := mustDefault(t)
	if len(c.Models) < 15 {
		t.Fatalf("only %d models", len(c.Models))
	}
	for _, m := range c.Models {
		if !strings.HasPrefix(m.Source, "https://") {
			t.Errorf("%s: no source", m.ID)
		}
	}
	if len(c.Plans) == 0 {
		t.Error("no plans")
	}
}

func TestCostSplitsCacheWritesByDuration(t *testing.T) {
	c := mustDefault(t)
	// claude-opus-5-5: input 4, output 20, cache read 0.20, 5m write 5, 1h write 8 ($/MTok).
	u := ledger.Usage{Model: "claude-opus-5-5", Input: 1_000_000, CacheRead: 10_000_000,
		CacheWrite: 3_000_000, CacheWrite1h: 2_000_000, Output: 500_000}
	got, err := c.Cost(u)
	if err != nil {
		t.Fatal(err)
	}
	near(t, got, 4+10*0.20+1*5+2*8+0.5*20) // 37.0
}

func TestCostResolvesAliases(t *testing.T) {
	c := mustDefault(t)
	for _, name := range []string{"claude-haiku-4-5-20251001", "meta/muse-spark-1.3-contributor", "deepseek/deepseek-v4.1-flash", "minimax-m2.5:cloud"} {
		if _, err := c.Lookup(name); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestUnknownModelFailsLoud(t *testing.T) {
	c := mustDefault(t)
	_, err := c.Cost(ledger.Usage{Model: "no-such-model", Input: 1})
	if !errors.Is(err, ErrUnknownModel) {
		t.Fatalf("got %v, want ErrUnknownModel", err)
	}
}

func TestMissingPriceFailsLoudOnlyWhenNeeded(t *testing.T) {
	c := mustDefault(t)
	// gpt-5.4 has no cache_write price.
	if _, err := c.Cost(ledger.Usage{Model: "gpt-5.4", Input: 10, Output: 10}); err != nil {
		t.Fatalf("no cache writes, want no error: %v", err)
	}
	_, err := c.Cost(ledger.Usage{Model: "gpt-5.4", CacheWrite: 10})
	if !errors.Is(err, ErrMissingPrice) {
		t.Fatalf("got %v, want ErrMissingPrice", err)
	}
}

func TestZeroUsageCostsZero(t *testing.T) {
	c := mustDefault(t)
	got, err := c.Cost(ledger.Usage{Model: "gpt-6-astra"})
	if err != nil || got != 0 {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestParseRejectsBadEntries(t *testing.T) {
	base := "as_of: 2026-10-17\nmodels:\n"
	cases := map[string]string{
		"no source":      base + "  - {id: m, tier: fast, data_policy: unknown, price: {input: 1, output: 1}}\n",
		"http source":    base + "  - {id: m, tier: fast, data_policy: unknown, price: {input: 1, output: 1}, source: http://x}\n",
		"bad tier":       base + "  - {id: m, tier: huge, data_policy: unknown, price: {input: 1, output: 1}, source: https://x}\n",
		"bad policy":     base + "  - {id: m, tier: fast, data_policy: maybe, price: {input: 1, output: 1}, source: https://x}\n",
		"no output":      base + "  - {id: m, tier: fast, data_policy: unknown, price: {input: 1}, source: https://x}\n",
		"duplicate name": base + "  - {id: m, tier: fast, data_policy: unknown, price: {input: 1, output: 1}, source: https://x}\n  - {id: n, aliases: [m], tier: fast, data_policy: unknown, price: {input: 1, output: 1}, source: https://x}\n",
		"unknown field":  base + "  - {id: m, tier: fast, data_policy: unknown, price: {input: 1, output: 1}, source: https://x, cost: 3}\n",
		"no as_of":       "models: []\n",
		"negative price": base + "  - {id: m, tier: fast, data_policy: unknown, price: {input: -1, output: 1}, source: https://x}\n",
		"bad long ctx":   base + "  - {id: m, tier: fast, data_policy: unknown, price: {input: 1, output: 1}, long_context: {above: 0, price: {input: 1, output: 1}}, source: https://x}\n",
		"zero tier":      "as_of: x\nmodels: []\nplans:\n  - {id: p, tiers: {a: 0}, source: https://x}\n",
		"no tiers":       "as_of: x\nmodels: []\nplans:\n  - {id: p, source: https://x}\n",
	}
	for name, doc := range cases {
		if _, err := Parse([]byte(doc)); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestCostRejectsImpossibleUsage(t *testing.T) {
	c := mustDefault(t)
	for _, u := range []ledger.Usage{
		{Model: "claude-opus-5-5", Input: -1},
		{Model: "claude-opus-5-5", CacheWrite: 1, CacheWrite1h: 2},
	} {
		if _, err := c.Cost(u); !errors.Is(err, ErrBadUsage) {
			t.Errorf("%+v: got %v, want ErrBadUsage", u, err)
		}
	}
}

func TestLoadReadsAFile(t *testing.T) {
	c, err := Load("../../cmd/inferencia/testdata/catalog.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Lookup("gpt-test"); err != nil {
		t.Error(err)
	}
	if _, err := Load("no-such-catalog.yaml"); err == nil {
		t.Error("want an error for a missing file")
	}
}

func TestPlanFor(t *testing.T) {
	c := mustDefault(t)
	p, ok := c.PlanFor(ledger.Usage{Harness: "opencode", Upstream: "opencode-go"})
	if !ok || p.ID != "opencode-go" || p.Tiers["go"] != 10 {
		t.Fatalf("got %+v, %v", p, ok)
	}
	if _, ok := c.PlanFor(ledger.Usage{Harness: "opencode", Upstream: "openrouter"}); ok {
		t.Error("openrouter is pay-per-token, not a plan")
	}
}

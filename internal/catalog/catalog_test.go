package catalog

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	g "github.com/onsi/gomega"

	"github.com/menezmethod/inferencia/internal/ledger"
)

var (
	Expect        = g.Expect
	Equal         = g.Equal
	BeNumerically = g.BeNumerically
	HaveLen       = g.HaveLen
	BeEmpty       = g.BeEmpty
	MatchError    = g.MatchError
	HaveOccurred  = g.HaveOccurred
	BeTrue        = g.BeTrue
	BeFalse       = g.BeFalse
)

func mustDefault() *Catalog {
	c, err := Default()
	Expect(err).NotTo(HaveOccurred())
	return c
}

func near(got, want float64) {
	Expect(got).To(BeNumerically("~", want, 1e-9))
}

var _ = Describe("Embedded Catalog", func() {
	It("is valid", func() {
		c := mustDefault()
		Expect(len(c.Models)).To(BeNumerically(">=", 15), "only %d models", len(c.Models))
		for _, m := range c.Models {
			Expect(strings.HasPrefix(m.Source, "https://")).To(BeTrue(), "%s: no source", m.ID)
		}
		Expect(c.Plans).NotTo(BeEmpty(), "no plans")
	})
})

var _ = Describe("Cost", func() {
	It("splits cache writes by duration", func() {
		c := mustDefault()
		// claude-opus-5-5: input 4, output 20, cache read 0.20, 5m write 5, 1h write 8 ($/MTok).
		u := ledger.Usage{Model: "claude-opus-5-5", Input: 1_000_000, CacheRead: 10_000_000,
			CacheWrite: 3_000_000, CacheWrite1h: 2_000_000, Output: 500_000}
		got, err := c.Cost(u)
		Expect(err).NotTo(HaveOccurred())
		near(got, 4+10*0.20+1*5+2*8+0.5*20) // 37.0
	})

	It("resolves aliases", func() {
		c := mustDefault()
		for _, name := range []string{"claude-haiku-4-5-20251001", "meta/muse-spark-1.3-contributor", "deepseek/deepseek-v4.1-flash", "minimax-m2.5:cloud"} {
			_, err := c.Lookup(name)
			Expect(err).NotTo(HaveOccurred(), "%s: %v", name, err)
		}
	})

	It("fails loud on unknown model", func() {
		c := mustDefault()
		_, err := c.Cost(ledger.Usage{Model: "no-such-model", Input: 1})
		Expect(err).To(MatchError(ErrUnknownModel))
	})

	It("fails loud on missing price only when needed", func() {
		c := mustDefault()
		// gpt-5.4 has no cache_write price.
		_, err := c.Cost(ledger.Usage{Model: "gpt-5.4", Input: 10, Output: 10})
		Expect(err).NotTo(HaveOccurred())
		_, err = c.Cost(ledger.Usage{Model: "gpt-5.4", CacheWrite: 10})
		Expect(err).To(MatchError(ErrMissingPrice))
	})

	It("costs zero on zero usage", func() {
		c := mustDefault()
		got, err := c.Cost(ledger.Usage{Model: "gpt-6-astra"})
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(Equal(0.0))
	})

	It("rejects impossible usage", func() {
		c := mustDefault()
		for _, u := range []ledger.Usage{
			{Model: "claude-opus-5-5", Input: -1},
			{Model: "claude-opus-5-5", CacheWrite: 1, CacheWrite1h: 2},
		} {
			_, err := c.Cost(u)
			Expect(err).To(MatchError(ErrBadUsage), "%+v: got %v, want ErrBadUsage", u, err)
		}
	})
})

var _ = Describe("Parse", func() {
	It("rejects bad entries", func() {
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
			_, err := Parse([]byte(doc))
			Expect(err).To(HaveOccurred(), "%s: want an error", name)
		}
	})
})

var _ = Describe("Load", func() {
	It("reads a file", func() {
		c, err := Load("../../cmd/inferencia/testdata/catalog.yaml")
		Expect(err).NotTo(HaveOccurred())
		_, err = c.Lookup("gpt-test")
		Expect(err).NotTo(HaveOccurred())
		_, err = Load("no-such-catalog.yaml")
		Expect(err).To(HaveOccurred(), "want an error for a missing file")
	})
})

var _ = Describe("PlanFor", func() {
	It("identifies flat plans and rejects pay-per-token", func() {
		c := mustDefault()
		p, ok := c.PlanFor(ledger.Usage{Harness: "opencode", Upstream: "opencode-go"})
		Expect(ok).To(BeTrue())
		Expect(p.ID).To(Equal("opencode-go"))
		Expect(p.Tiers["go"]).To(Equal(10.0))

		_, ok = c.PlanFor(ledger.Usage{Harness: "opencode", Upstream: "openrouter"})
		Expect(ok).To(BeFalse(), "openrouter is pay-per-token, not a plan")
	})
})

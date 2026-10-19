package catalog

import (
	"time"

	. "github.com/onsi/ginkgo/v2"

	"github.com/menezmethod/inferencia/internal/ledger"
)

var _ = Describe("CostAs", func() {
	It("maps missing prices to nearest", func() {
		c := mustDefault()
		m, err := c.Lookup("deepseek-v4-flash") // input 0.091, output 0.182, cache read 0.0182; no write prices
		Expect(err).NotTo(HaveOccurred())
		u := ledger.Usage{Model: "claude-opus-5-5", Input: 1_000_000, CacheRead: 1_000_000, CacheWrite: 2_000_000, CacheWrite1h: 1_000_000, Output: 1_000_000}
		got, err := m.CostAs(u)
		Expect(err).NotTo(HaveOccurred())
		near(got, 3*0.091+1*0.0182+1*0.182) // writes priced as plain input
		_, err = m.Cost(u)
		Expect(err).To(HaveOccurred(), "Cost (not CostAs) must refuse writes the model has no price for")
	})

	It("falls back from one-hour to five-minute writes", func() {
		c := mustDefault()
		m, err := c.Lookup("gpt-6-astra") // cache_write 12.50, no 1-hour price
		Expect(err).NotTo(HaveOccurred())
		got, err := m.CostAs(ledger.Usage{CacheWrite: 1_000_000, CacheWrite1h: 1_000_000})
		Expect(err).NotTo(HaveOccurred())
		near(got, 12.50)
	})
})

var _ = Describe("Savings", func() {
	It("groups and flags unpriced", func() {
		c := mustDefault()
		us := []ledger.Usage{
			{Harness: "h", Model: "claude-opus-5-5", Calls: 1, Input: 1_000_000, Output: 1_000_000},
			{Harness: "h", Model: "claude-opus-5-5", Calls: 1, Input: 1_000_000},
			{Harness: "h", Model: "mystery-model", Calls: 3, Input: 7},
		}
		lines, unpriced, err := c.Savings(us, []string{"deepseek-v4-flash"})
		Expect(err).NotTo(HaveOccurred())
		Expect(lines).To(HaveLen(1))
		l := lines[0]
		near(l.Actual, 2*4+1*20)                       // 28
		near(l.As["deepseek-v4-flash"], 2*0.091+0.182) // 0.364
		Expect(l.Totals.Calls).To(Equal(2))
		Expect(unpriced["mystery-model (not in catalog)"].Calls).To(Equal(3))
	})

	It("flags missing prices and skips empty usage", func() {
		c := mustDefault()
		us := []ledger.Usage{
			{Harness: "h", Model: "gpt-5.4", Calls: 1, CacheWrite: 10}, // gpt-5.4 has no cache_write price
			{Harness: "h", Model: "no-tokens-model", Calls: 5},         // nothing to price: skipped, not flagged
		}
		lines, unpriced, err := c.Savings(us, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(lines).To(BeEmpty())
		Expect(unpriced).To(HaveLen(1))
		Expect(unpriced["gpt-5.4 (price missing)"].Calls).To(Equal(1))
	})

	It("applies long context price per call", func() {
		c := mustDefault()
		// claude-haiku-5-5: base input 0.10; above 100k prompt tokens per request, input 0.50.
		short := ledger.Usage{Model: "claude-haiku-5-5", Calls: 10, Input: 900_000} // 90k per call
		long := ledger.Usage{Model: "claude-haiku-5-5", Calls: 9, Input: 900_000}   // 100,000 per call: not over
		longer := ledger.Usage{Model: "claude-haiku-5-5", Calls: 1, Input: 100_001}
		for _, tc := range []struct {
			u    ledger.Usage
			want float64
		}{{short, 0.09}, {long, 0.09}, {longer, 0.100001 * 0.50}} {
			got, err := c.Cost(tc.u)
			Expect(err).NotTo(HaveOccurred())
			near(got, tc.want)
		}
	})

	It("rejects unknown alternative", func() {
		c := mustDefault()
		_, _, err := c.Savings(nil, []string{"nope"})
		Expect(err).To(HaveOccurred(), "want an error for an unknown alternative")
	})

	It("finds cheapest per tier", func() {
		c := mustDefault()
		us := []ledger.Usage{{Model: "x", Input: 1_000_000, CacheRead: 9_000_000, Output: 100_000}}
		alts := c.CheapestPerTier(us)
		Expect(alts).To(HaveLen(3), "want one per tier (frontier, strong, fast)")
		for i, tier := range []string{"frontier", "strong", "fast"} {
			m, err := c.Lookup(alts[i])
			Expect(err).NotTo(HaveOccurred())
			Expect(m.Tier).To(Equal(tier), "pick %d is %s (%s), want tier %s", i, m.ID, m.Tier, tier)
		}
		for _, id := range alts {
			_, err := c.Lookup(id)
			Expect(err).NotTo(HaveOccurred())
		}
		// The fast pick must not cost more than any other fast model on this usage.
		fast, err := c.Lookup(alts[2])
		Expect(err).NotTo(HaveOccurred())
		best, err := fast.CostAs(us[0])
		Expect(err).NotTo(HaveOccurred())
		for i := range c.Models {
			m := &c.Models[i]
			if m.Tier != "fast" {
				continue
			}
			if v, err := m.CostAs(us[0]); err == nil && v < best-1e-12 {
				Fail(m.ID + " is cheaper than the pick " + fast.ID)
			}
		}
	})
})

var _ = Describe("Subs", func() {
	It("groups by plan and month", func() {
		c := mustDefault()
		sep := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
		oct := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
		us := []ledger.Usage{
			{Harness: "opencode", Upstream: "opencode-go", Model: "glm-5.2", Time: sep, Calls: 1, Input: 1_000_000},
			{Harness: "opencode", Upstream: "opencode-go", Model: "glm-5.2", Time: oct, Calls: 1, Input: 2_000_000},
			{Harness: "opencode", Upstream: "openrouter", Model: "glm-5.2", Time: oct, Calls: 1, Input: 5_000_000}, // pay per token, not a plan
			{Harness: "codex", Upstream: "openai", Model: "gpt-6-astra", Time: oct, Calls: 2, Input: 1_000_000},
		}
		pms, unpriced, err := c.Subs(us)
		Expect(err).NotTo(HaveOccurred())
		Expect(unpriced).To(BeEmpty())
		Expect(pms).To(HaveLen(3))
		Expect(pms[0].Plan.ID).To(Equal("chatgpt"))
		Expect(pms[0].Month).To(Equal("2026-10"))
		near(pms[0].Value, 10)   // 1M input on gpt-6-astra at $10
		near(pms[1].Value, 0.62) // September, glm-5.2
		near(pms[2].Value, 1.24) // October, openrouter call excluded
	})
})

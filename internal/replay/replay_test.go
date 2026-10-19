package replay

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/menezmethod/inferencia/internal/catalog"
	"github.com/menezmethod/inferencia/internal/ledger"
)

func parseCatalog() *catalog.Catalog {
	c, err := catalog.Parse([]byte(`as_of: 2026-10-17
models:
  - {id: big, aliases: [big-alias], tier: frontier, data_policy: unknown, price: {input: 10, output: 50, cache_read: 1, cache_write: 12.5, cache_write_1h: 20}, source: https://example.test/a}
  - {id: small, tier: fast, data_policy: unknown, price: {input: 1, output: 5, cache_read: 0.1}, source: https://example.test/b}
`))
	Expect(err).NotTo(HaveOccurred())
	return c
}

func near(name string, got, want float64) {
	Expect(got).To(BeNumerically("~", want, 1e-9), "%s: got %.9f, want %.9f", name, got, want)
}

var t0 = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

// One session of three calls on "big": 1M uncached each, then 9M cache reads on calls 2 and 3.
func session() []ledger.Usage {
	return []ledger.Usage{
		{Harness: "h", Session: "s", Model: "big", Time: t0.Add(2 * time.Minute), Calls: 1, Input: 1_000_000, CacheRead: 9_000_000},
		{Harness: "h", Session: "s", Model: "big", Time: t0, Calls: 1, Input: 1_000_000},
		{Harness: "h", Session: "s", Model: "big", Time: t0.Add(time.Minute), Calls: 1, Input: 1_000_000, CacheRead: 9_000_000},
	}
}

var _ = Describe("Replay", func() {
	It("matches catalog cost for as-run", func() {
		r, err := Run(parseCatalog(), session(), "as-run", AsRun)
		Expect(err).NotTo(HaveOccurred())
		near("usd", r.USD, 3*10+18*1) // 48
		Expect(r.Switches).To(Equal(0))
		Expect(r.CacheLossUSD).To(Equal(0.0))
		Expect(r.Calls).To(Equal(3))
	})

	It("pays cache loss when switching mid-session", func() {
		// Call 1 (t0) on small, calls 2-3 on big: the first big call switches, so
		// its 9M cache reads are written again at big's cache-write price.
		p := func(u ledger.Usage, prev string) string {
			if u.Time.Equal(t0) {
				return "small"
			}
			return "big"
		}
		r, err := Run(parseCatalog(), session(), "switch", p)
		Expect(err).NotTo(HaveOccurred())
		Expect(r.Switches).To(Equal(1), "switches %d, want 1 (sorting by time must put call t0 first)", r.Switches)
		noLoss := 1*1.0 + (10 + 9*1) + (10 + 9*1) // 39
		loss := 9 * (12.5 - 1)                    // 103.5: 9M re-written at 12.5 instead of read at 1
		near("usd", r.USD, noLoss+loss)
		near("cache loss", r.CacheLossUSD, loss)
	})

	It("moves only delegated calls with floor", func() {
		us := []ledger.Usage{
			{Harness: "h", Session: "main", Model: "big", Calls: 1, Input: 1_000_000},
			{Harness: "h", Session: "sub", Model: "big", Delegated: true, Calls: 1, Input: 1_000_000},
		}
		r, err := Run(parseCatalog(), us, "floor", Floor("small"))
		Expect(err).NotTo(HaveOccurred())
		near("usd", r.USD, 10+1)
		Expect(r.Switches).To(Equal(0), "separate sessions never switch")
	})

	It("switches at threshold for naive per-call", func() {
		// Calls of 1M, 10M, 10M context: threshold 5M puts call 1 on small, 2-3 on big.
		r, err := Run(parseCatalog(), session(), "naive", NaivePerCall(5_000_000, "small", "big"))
		Expect(err).NotTo(HaveOccurred())
		Expect(r.Switches).To(Equal(1))
		// The first big call re-writes 9M tokens at 12.5 instead of reading them at 1.
		near("cache loss", r.CacheLossUSD, 9*(12.5-1))
	})

	It("treats unknown models as unpriced not free", func() {
		us := []ledger.Usage{{Harness: "h", Session: "s", Model: "mystery", Calls: 2, Input: 5}}
		r, err := Run(parseCatalog(), us, "as-run", AsRun)
		Expect(err).NotTo(HaveOccurred())
		Expect(r.Unpriced.Calls).To(Equal(2))
		Expect(r.USD).To(Equal(0.0))
		Expect(r.Calls).To(Equal(0))
	})

	It("keeps only calls priced as run in priceable", func() {
		us := []ledger.Usage{{Model: "big", Calls: 1, Input: 1}, {Model: "mystery", Calls: 4, Input: 1}}
		ok, unpriced := Priceable(parseCatalog(), us)
		Expect(ok).To(HaveLen(1))
		Expect(ok[0].Model).To(Equal("big"))
		Expect(unpriced.Calls).To(Equal(4))
	})

	It("does not charge recorded switches again", func() {
		// The recorded run already switched big -> small -> big; its tokens show it.
		us := session()
		us[2].Model = "small" // the call at t0+1m
		r, err := Run(parseCatalog(), us, "as-run", AsRun)
		Expect(err).NotTo(HaveOccurred())
		Expect(r.Switches).To(Equal(0))
		Expect(r.CacheLossUSD).To(Equal(0.0), "as-run must never pay a cache loss")
	})

	It("treats subagent calls as their own session", func() {
		// Recorded: main loop and subagent both on big, sharing a session id. The
		// policy moves only the subagent to small. If main and subagent shared one
		// conversation key, the policy sequence big, small, big would count 2 switches.
		us := []ledger.Usage{
			{Harness: "h", Session: "s", Model: "big", Time: t0, Calls: 1, Input: 1, CacheRead: 1000},
			{Harness: "h", Session: "s", Model: "big", Delegated: true, Agent: "a1", Time: t0.Add(time.Second), Calls: 1, Input: 1, CacheRead: 1000},
			{Harness: "h", Session: "s", Model: "big", Time: t0.Add(2 * time.Second), Calls: 1, Input: 1, CacheRead: 1000},
		}
		r, err := Run(parseCatalog(), us, "floor", Floor("small"))
		Expect(err).NotTo(HaveOccurred())
		Expect(r.Switches).To(Equal(0))
	})

	It("treats interleaved subagents as separate conversations", func() {
		// Two subagents in one session alternate; the naive router puts A on small, B on big.
		var us []ledger.Usage
		for i := 0; i < 4; i++ {
			agent, read := "A", int64(1_000)
			if i%2 == 1 {
				agent, read = "B", 9_000_000
			}
			us = append(us, ledger.Usage{Harness: "h", Session: "s", Agent: agent, Delegated: true, Model: "big",
				Time: t0.Add(time.Duration(i) * time.Second), Calls: 1, Input: 1, CacheRead: read})
		}
		r, err := Run(parseCatalog(), us, "naive", NaivePerCall(5_000_000, "small", "big"))
		Expect(err).NotTo(HaveOccurred())
		Expect(r.Switches).To(Equal(0))
		Expect(r.CacheLossUSD).To(Equal(0.0), "each agent stays on one model, so no switch")
	})

	It("charges switch-back each time", func() {
		// Policy alternates small, big, small on a recorded all-big session: 2 switches.
		p := func(u ledger.Usage, _ string) string {
			if u.Time.Equal(t0.Add(time.Minute)) {
				return "big"
			}
			return "small"
		}
		r, err := Run(parseCatalog(), session(), "back", p)
		Expect(err).NotTo(HaveOccurred())
		Expect(r.Switches).To(Equal(2))
		// Into big: 9M at 12.5 instead of 1. Into small (no write price): 9M at input 1 instead of read 0.1.
		near("cache loss", r.CacheLossUSD, 9*(12.5-1)+9*(1-0.1))
	})

	It("rewrites one-hour cache at the one-hour price", func() {
		us := []ledger.Usage{
			{Harness: "h", Session: "s", Model: "big", Time: t0, Calls: 1, Input: 1_000_000},
			{Harness: "h", Session: "s", Model: "big", Time: t0.Add(time.Minute), Calls: 1, CacheRead: 1_000_000, CacheWrite: 10, CacheWrite1h: 10},
		}
		p := func(u ledger.Usage, _ string) string {
			if u.Time.Equal(t0) {
				return "small"
			}
			return "big"
		}
		r, err := Run(parseCatalog(), us, "1h", p)
		Expect(err).NotTo(HaveOccurred())
		near("cache loss", r.CacheLossUSD, 1*(20-1)) // 1M re-written at the 1-hour price 20, not read at 1
	})

	It("never switches multi-call records", func() {
		// Codex-style records are whole sessions; there is no turn boundary to switch at.
		us := []ledger.Usage{
			{Harness: "codex", Session: "s", Model: "big", Time: t0, Calls: 5, Input: 1, CacheRead: 1_000_000},
			{Harness: "codex", Session: "s", Model: "big", Time: t0.Add(time.Hour), Calls: 5, Input: 1, CacheRead: 1_000_000},
		}
		p := func(u ledger.Usage, _ string) string {
			if u.Time.Equal(t0) {
				return "small"
			}
			return "big"
		}
		r, err := Run(parseCatalog(), us, "multi", p)
		Expect(err).NotTo(HaveOccurred())
		Expect(r.Switches).To(Equal(0))
		Expect(r.CacheLossUSD).To(Equal(0.0))
	})

	It("does not treat alias as a switch", func() {
		r, err := Run(parseCatalog(), session(), "alias", func(ledger.Usage, string) string { return "big-alias" })
		Expect(err).NotTo(HaveOccurred())
		Expect(r.Switches).To(Equal(0))
		near("usd", r.USD, 48)
	})
})

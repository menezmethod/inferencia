package replay

import (
	"math"
	"testing"
	"time"

	"github.com/menezmethod/inferencia/internal/catalog"
	"github.com/menezmethod/inferencia/internal/ledger"
)

func cat(t *testing.T) *catalog.Catalog {
	t.Helper()
	c, err := catalog.Parse([]byte(`as_of: 2026-10-17
models:
  - {id: big, aliases: [big-alias], tier: frontier, data_policy: unknown, price: {input: 10, output: 50, cache_read: 1, cache_write: 12.5, cache_write_1h: 20}, source: https://example.test/a}
  - {id: small, tier: fast, data_policy: unknown, price: {input: 1, output: 5, cache_read: 0.1}, source: https://example.test/b}
`))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func near(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("%s: got %.9f, want %.9f", name, got, want)
	}
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

func TestAsRunMatchesCatalogCost(t *testing.T) {
	r, err := Run(cat(t), session(), "as-run", AsRun)
	if err != nil {
		t.Fatal(err)
	}
	near(t, "usd", r.USD, 3*10+18*1) // 48
	if r.Switches != 0 || r.CacheLossUSD != 0 || r.Calls != 3 {
		t.Errorf("%+v", r)
	}
}

func TestSwitchingMidSessionPaysCacheLoss(t *testing.T) {
	// Call 1 (t0) on small, calls 2-3 on big: the first big call switches, so
	// its 9M cache reads are written again at big's cache-write price.
	p := func(u ledger.Usage, prev string) string {
		if u.Time.Equal(t0) {
			return "small"
		}
		return "big"
	}
	r, err := Run(cat(t), session(), "switch", p)
	if err != nil {
		t.Fatal(err)
	}
	if r.Switches != 1 {
		t.Fatalf("switches %d, want 1 (sorting by time must put call t0 first)", r.Switches)
	}
	noLoss := 1*1.0 + (10 + 9*1) + (10 + 9*1) // 39
	loss := 9 * (12.5 - 1)                    // 103.5: 9M re-written at 12.5 instead of read at 1
	near(t, "usd", r.USD, noLoss+loss)
	near(t, "cache loss", r.CacheLossUSD, loss)
}

func TestFloorMovesOnlyDelegatedCalls(t *testing.T) {
	us := []ledger.Usage{
		{Harness: "h", Session: "main", Model: "big", Calls: 1, Input: 1_000_000},
		{Harness: "h", Session: "sub", Model: "big", Delegated: true, Calls: 1, Input: 1_000_000},
	}
	r, err := Run(cat(t), us, "floor", Floor("small"))
	if err != nil {
		t.Fatal(err)
	}
	near(t, "usd", r.USD, 10+1)
	if r.Switches != 0 {
		t.Errorf("switches %d: separate sessions never switch", r.Switches)
	}
}

func TestNaivePerCallSwitchesAtThreshold(t *testing.T) {
	// Calls of 1M, 10M, 10M context: threshold 5M puts call 1 on small, 2-3 on big.
	r, err := Run(cat(t), session(), "naive", NaivePerCall(5_000_000, "small", "big"))
	if err != nil {
		t.Fatal(err)
	}
	if r.Switches != 1 {
		t.Errorf("switches %d, want 1", r.Switches)
	}
	// The first big call re-writes 9M tokens at 12.5 instead of reading them at 1.
	near(t, "cache loss", r.CacheLossUSD, 9*(12.5-1))
}

func TestUnknownModelIsUnpricedNotFree(t *testing.T) {
	us := []ledger.Usage{{Harness: "h", Session: "s", Model: "mystery", Calls: 2, Input: 5}}
	r, err := Run(cat(t), us, "as-run", AsRun)
	if err != nil {
		t.Fatal(err)
	}
	if r.Unpriced.Calls != 2 || r.USD != 0 || r.Calls != 0 {
		t.Errorf("%+v", r)
	}
}

func TestPriceableKeepsOnlyCallsPricedAsRun(t *testing.T) {
	us := []ledger.Usage{{Model: "big", Calls: 1, Input: 1}, {Model: "mystery", Calls: 4, Input: 1}}
	ok, unpriced := Priceable(cat(t), us)
	if len(ok) != 1 || ok[0].Model != "big" || unpriced.Calls != 4 {
		t.Fatalf("ok %v, unpriced %+v", ok, unpriced)
	}
}

func TestRecordedSwitchesAreNotChargedAgain(t *testing.T) {
	// The recorded run already switched big -> small -> big; its tokens show it.
	us := session()
	us[2].Model = "small" // the call at t0+1m
	r, err := Run(cat(t), us, "as-run", AsRun)
	if err != nil {
		t.Fatal(err)
	}
	if r.Switches != 0 || r.CacheLossUSD != 0 {
		t.Errorf("as-run must never pay a cache loss: %+v", r)
	}
}

func TestSubagentCallsAreTheirOwnSession(t *testing.T) {
	// Recorded: main loop and subagent both on big, sharing a session id. The
	// policy moves only the subagent to small. If main and subagent shared one
	// conversation key, the policy sequence big, small, big would count 2 switches.
	us := []ledger.Usage{
		{Harness: "h", Session: "s", Model: "big", Time: t0, Calls: 1, Input: 1, CacheRead: 1000},
		{Harness: "h", Session: "s", Model: "big", Delegated: true, Agent: "a1", Time: t0.Add(time.Second), Calls: 1, Input: 1, CacheRead: 1000},
		{Harness: "h", Session: "s", Model: "big", Time: t0.Add(2 * time.Second), Calls: 1, Input: 1, CacheRead: 1000},
	}
	r, err := Run(cat(t), us, "floor", Floor("small"))
	if err != nil {
		t.Fatal(err)
	}
	if r.Switches != 0 {
		t.Errorf("switches %d, want 0", r.Switches)
	}
}

func TestInterleavedSubagentsAreSeparateConversations(t *testing.T) {
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
	r, err := Run(cat(t), us, "naive", NaivePerCall(5_000_000, "small", "big"))
	if err != nil {
		t.Fatal(err)
	}
	if r.Switches != 0 || r.CacheLossUSD != 0 {
		t.Errorf("each agent stays on one model, so no switch: %+v", r)
	}
}

func TestSwitchBackIsChargedEachTime(t *testing.T) {
	// Policy alternates small, big, small on a recorded all-big session: 2 switches.
	p := func(u ledger.Usage, _ string) string {
		if u.Time.Equal(t0.Add(time.Minute)) {
			return "big"
		}
		return "small"
	}
	r, err := Run(cat(t), session(), "back", p)
	if err != nil {
		t.Fatal(err)
	}
	if r.Switches != 2 {
		t.Fatalf("switches %d, want 2", r.Switches)
	}
	// Into big: 9M at 12.5 instead of 1. Into small (no write price): 9M at input 1 instead of read 0.1.
	near(t, "cache loss", r.CacheLossUSD, 9*(12.5-1)+9*(1-0.1))
}

func TestOneHourCacheIsRewrittenAtTheOneHourPrice(t *testing.T) {
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
	r, err := Run(cat(t), us, "1h", p)
	if err != nil {
		t.Fatal(err)
	}
	near(t, "cache loss", r.CacheLossUSD, 1*(20-1)) // 1M re-written at the 1-hour price 20, not read at 1
}

func TestMultiCallRecordsNeverSwitch(t *testing.T) {
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
	r, err := Run(cat(t), us, "multi", p)
	if err != nil {
		t.Fatal(err)
	}
	if r.Switches != 0 || r.CacheLossUSD != 0 {
		t.Errorf("%+v", r)
	}
}

func TestAliasIsNotASwitch(t *testing.T) {
	r, err := Run(cat(t), session(), "alias", func(ledger.Usage, string) string { return "big-alias" })
	if err != nil {
		t.Fatal(err)
	}
	if r.Switches != 0 {
		t.Errorf("switches %d, want 0", r.Switches)
	}
	near(t, "usd", r.USD, 48)
}

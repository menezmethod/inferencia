package ledger

import (
	"bytes"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Load", func() {
	It("loads present sources and turns a missing one into a warning", func() {
		us, warns, loaded := Load(Sources{
			ClaudeDir:      "testdata/claude",
			CodexDir:       "testdata/codex",
			CommandCodeDir: "testdata/commandcode",
			OpenCodeDB:     "testdata/no-such.db",
		}, Window{Until: ts("2026-10-06T00:10:34Z")})
		Expect(loaded).To(Equal(3))
		Expect(warns).To(HaveLen(2)) // opencode missing; claude has one unreadable line
		Expect(warns[0].Error()).To(ContainSubstring("claude-code: skipped 1 unreadable lines"))
		Expect(warns[1].Error()).To(ContainSubstring("opencode: no logs"))
		harnesses := map[string]bool{}
		for _, u := range us {
			harnesses[u.Harness] = true
		}
		Expect(harnesses).To(HaveKey("claude-code"))
		Expect(harnesses).To(HaveKey("codex"))
		Expect(harnesses).To(HaveKey("command-code"))
	})
})

var _ = Describe("WriteReport", func() {
	It("prints groups, the solo/delegated split and per-harness totals", func() {
		us := []Usage{
			{Harness: "opencode", Upstream: "opencode-go", Model: "m1", Billing: BillingPlan, Calls: 2, Input: 100, CacheRead: 900, Output: 10, CostUSD: 1.5, HasCost: true, Time: time.Now()},
			{Harness: "opencode", Upstream: "openrouter", Model: "m2", Billing: BillingAPI, Delegated: true, Calls: 1, Input: 50, CacheRead: 50, Output: 5, CostUSD: 0.25, HasCost: true},
			{Harness: "claude-code", Upstream: "anthropic", Model: "m3", Billing: BillingUnknown, Calls: 1, Input: 1_500_000, CacheRead: 0, Output: 1},
		}
		var buf bytes.Buffer
		Expect(WriteReport(&buf, us)).To(Succeed())
		out := buf.String()
		Expect(out).To(ContainSubstring("opencode-go"))
		Expect(out).To(ContainSubstring("$1.50"))
		Expect(out).To(ContainSubstring("90.0%")) // 900 of 1000 input tokens
		Expect(out).To(ContainSubstring("1.5M"))
		Expect(out).To(MatchRegexp(`delegated\s+1\s+100\s+50\.0%\s+5\s+\$0\.25`))
		Expect(out).To(MatchRegexp(`all\s+4\s`))
		Expect(out).To(ContainSubstring("notional"))
	})

	It("marks rows with no cost as - and rows with partial cost as *", func() {
		us := []Usage{
			{Harness: "h", Upstream: "u", Model: "none", Calls: 1, Input: 1},
			{Harness: "h", Upstream: "u", Model: "some", Calls: 1, Input: 1, CostUSD: 2, HasCost: true},
			{Harness: "h", Upstream: "u", Model: "some", Calls: 1, Input: 1},
		}
		var buf bytes.Buffer
		Expect(WriteReport(&buf, us)).To(Succeed())
		Expect(buf.String()).To(MatchRegexp(`none\s+false\s+1\s+1\s+0\.0%\s+0\s+-`))
		Expect(buf.String()).To(ContainSubstring("$2.00*"))
	})

	It("strips control characters from names taken from logs", func() {
		var buf bytes.Buffer
		Expect(WriteReport(&buf, []Usage{{Harness: "h", Model: "evil\x1b[31m", Calls: 1}})).To(Succeed())
		Expect(buf.String()).NotTo(ContainSubstring("\x1b"))
		Expect(buf.String()).To(ContainSubstring("evil[31m"))
	})

	It("handles an empty set without dividing by zero", func() {
		var buf bytes.Buffer
		Expect(WriteReport(&buf, nil)).To(Succeed())
		Expect(buf.String()).To(MatchRegexp(`all\s+0\s+0\s+0\.0%`))
	})
})

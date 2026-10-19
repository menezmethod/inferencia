package ledger

import (
	"math"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Golden numbers check measure against the numbers in docs/PLAN_V4.md, read from
// the real harness logs in $HOME. It runs only with INFERENCIA_GOLDEN=1 (D3).
// Each source has a frozen cutoff so later runs do not move the numbers.
var _ = Describe("Golden numbers", func() {
	BeforeEach(func() {
		if os.Getenv("INFERENCIA_GOLDEN") != "1" {
			Skip("set INFERENCIA_GOLDEN=1 to check against real harness logs")
		}
	})

	near := func(name string, got, want, tol float64) {
		Expect(want).NotTo(BeZero())
		diff := math.Abs(got-want) / want
		Expect(diff).To(BeNumerically("<=", tol), "%s: got %.4f, want %.4f (±%.0f%%)", name, got, want, tol*100)
	}

	until := func(s string) Window {
		v, err := time.Parse(time.RFC3339, s)
		Expect(err).NotTo(HaveOccurred())
		return Window{Until: v}
	}

	It("checks opencode golden numbers", func() {
		home, err := os.UserHomeDir()
		Expect(err).NotTo(HaveOccurred())
		src := DefaultSources(home)
		us, err := OpenCode(src.OpenCodeDB, until("2026-09-21T13:41:18Z"))
		Expect(err).NotTo(HaveOccurred())
		tt := Sum(us)
		near("calls", float64(tt.Calls), 8733, 0.02)
		near("cost", tt.CostUSD, 51.95, 0.02)
		near("cache share", tt.CacheShare(), 0.982, 0.02)
	})

	It("checks codex golden numbers", func() {
		home, err := os.UserHomeDir()
		Expect(err).NotTo(HaveOccurred())
		src := DefaultSources(home)
		us, err := Codex(src.CodexDir, until("2026-10-16T04:22:03Z"))
		Expect(err).NotTo(HaveOccurred())
		tt := Sum(us)
		near("sessions with usage", float64(len(us)), 8, 0.02)
		near("cache share", tt.CacheShare(), 0.961, 0.02)
		near("input tokens", float64(tt.AllInput()), 444.9e6, 0.02)
	})

	It("checks command-code golden numbers", func() {
		home, err := os.UserHomeDir()
		Expect(err).NotTo(HaveOccurred())
		src := DefaultSources(home)
		us, err := CommandCode(src.CommandCodeDir, until("2026-10-06T00:10:34Z"))
		Expect(err).NotTo(HaveOccurred())
		tt := Sum(us)
		near("calls", float64(tt.Calls), 504, 0.02)
		near("cost", tt.CostUSD, 3.33, 0.02)
	})

	It("checks claude-code golden numbers", func() {
		// A gap here is a finding, not a failure: Claude Code deletes old
		// transcripts, and the 12,247 figure came from a different script.
		home, err := os.UserHomeDir()
		Expect(err).NotTo(HaveOccurred())
		src := DefaultSources(home)
		us, err := ClaudeCode(src.ClaudeDir, until("2026-09-30T23:59:59Z"))
		Expect(err).NotTo(HaveOccurred())
		tt := Sum(us)
		GinkgoWriter.Printf("calls: got %d, plan says about 12,247; cache share %.4f (plan: 0.982)\n", tt.Calls, tt.CacheShare())
	})
})

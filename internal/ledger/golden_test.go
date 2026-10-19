package ledger

import (
	"math"
	"os"
	"testing"
	"time"
)

// TestGolden checks measure against the numbers in docs/PLAN_V4.md, read from
// the real harness logs in $HOME. It runs only with INFERENCIA_GOLDEN=1 (D3).
// Each source has a frozen cutoff so later runs do not move the numbers.
func TestGolden(t *testing.T) {
	if os.Getenv("INFERENCIA_GOLDEN") != "1" {
		t.Skip("set INFERENCIA_GOLDEN=1 to check against real harness logs")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	src := DefaultSources(home)
	until := func(s string) Window {
		v, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatal(err)
		}
		return Window{Until: v}
	}
	near := func(name string, got, want, tol float64) {
		t.Helper()
		if want == 0 || math.Abs(got-want)/want > tol {
			t.Errorf("%s: got %.4f, want %.4f (±%.0f%%)", name, got, want, tol*100)
		} else {
			t.Logf("%s: got %.4f, want %.4f: ok", name, got, want)
		}
	}

	t.Run("opencode", func(t *testing.T) {
		us, err := OpenCode(src.OpenCodeDB, until("2026-09-21T13:41:18Z"))
		if err != nil {
			t.Fatal(err)
		}
		tt := Sum(us)
		near("calls", float64(tt.Calls), 8733, 0.02)
		near("cost", tt.CostUSD, 51.95, 0.02)
		near("cache share", tt.CacheShare(), 0.982, 0.02)
	})
	t.Run("codex", func(t *testing.T) {
		us, err := Codex(src.CodexDir, until("2026-10-16T04:22:03Z"))
		if err != nil {
			t.Fatal(err)
		}
		tt := Sum(us)
		near("sessions with usage", float64(len(us)), 8, 0.02)
		near("cache share", tt.CacheShare(), 0.961, 0.02)
		near("input tokens", float64(tt.AllInput()), 444.9e6, 0.02)
	})
	t.Run("command-code", func(t *testing.T) {
		us, err := CommandCode(src.CommandCodeDir, until("2026-10-06T00:10:34Z"))
		if err != nil {
			t.Fatal(err)
		}
		tt := Sum(us)
		near("calls", float64(tt.Calls), 504, 0.02)
		near("cost", tt.CostUSD, 3.33, 0.02)
	})
	t.Run("claude-code", func(t *testing.T) {
		// A gap here is a finding, not a failure: Claude Code deletes old
		// transcripts, and the 12,247 figure came from a different script.
		us, err := ClaudeCode(src.ClaudeDir, until("2026-09-30T23:59:59Z"))
		if err != nil {
			t.Fatal(err)
		}
		tt := Sum(us)
		t.Logf("calls: got %d, plan says about 12,247; cache share %.4f (plan: 0.982)", tt.Calls, tt.CacheShare())
	})
}

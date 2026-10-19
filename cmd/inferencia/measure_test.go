package main

import (
	"bytes"
	"io"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var fixtureSources = []string{
	"--claude-dir", "../../internal/ledger/testdata/claude",
	"--codex-dir", "../../internal/ledger/testdata/codex",
	"--commandcode-dir", "",
	"--opencode-db", "",
	"--until", "2026-10-01",
	"--catalog", "testdata/catalog.yaml",
}

type failOnFootnoteWriter struct {
	buf bytes.Buffer
}

func (f *failOnFootnoteWriter) Write(p []byte) (int, error) {
	if bytes.Contains(p, []byte("list prices")) {
		return 0, io.ErrClosedPipe
	}
	return f.buf.Write(p)
}

var _ = Describe("Measure", func() {
	It("runs measure report", func() {
		var out, errOut bytes.Buffer
		code := runMeasure([]string{
			"--until", "2026-10-06",
			"--claude-dir", "../../internal/ledger/testdata/claude",
			"--codex-dir", "../../internal/ledger/testdata/codex",
			"--commandcode-dir", "../../internal/ledger/testdata/commandcode",
			"--opencode-db", "",
		}, &out, &errOut)
		Expect(code).To(Equal(0), "stderr: %s", errOut.String())
		for _, want := range []string{"claude-code", "codex", "command-code", "SPLIT", "HARNESS TOTAL"} {
			Expect(out.String()).To(ContainSubstring(want))
		}
		// The fixture holds one line with a fractional token count: reported, not dropped.
		Expect(errOut.String()).To(Equal("measure: warning: claude-code: skipped 1 unreadable lines and 0 unreadable files\n"))
	})

	It("rejects bad date", func() {
		var out, errOut bytes.Buffer
		code := runMeasure([]string{"--since", "yesterday"}, &out, &errOut)
		Expect(code).To(Equal(2))
		Expect(errOut.String()).To(ContainSubstring("RFC 3339"))
	})

	It("parses bound dates", func() {
		end, err := parseBound("2026-09-30", true)
		Expect(err).NotTo(HaveOccurred())
		wantEnd := time.Date(2026, 9, 30, 23, 59, 59, 999999999, time.UTC)
		Expect(end.Equal(wantEnd)).To(BeTrue(), "end bound %v, want %v", end, wantEnd)

		start, err := parseBound("2026-09-30", false)
		Expect(err).NotTo(HaveOccurred())
		wantStart := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
		Expect(start.Equal(wantStart)).To(BeTrue(), "start bound %v", start)
	})

	It("checks exit codes", func() {
		for name, tc := range map[string]struct {
			args []string
			want int
		}{
			"help":       {[]string{"-h"}, 0},
			"positional": {[]string{"--claude-dir", "", "--codex-dir", "", "--opencode-db", "", "--commandcode-dir", "", "extra"}, 2},
			"no source":  {[]string{"--claude-dir", "/no/such", "--codex-dir", "", "--opencode-db", "", "--commandcode-dir", ""}, 1},
		} {
			var out, errOut bytes.Buffer
			got := runMeasure(tc.args, &out, &errOut)
			Expect(got).To(Equal(tc.want), "%s: exit %d, want %d (stderr: %s)", name, got, tc.want, errOut.String())
		}
	})

	It("rejects catalog flag on measure", func() {
		var out, errOut bytes.Buffer
		code := runMeasure([]string{"--catalog", "x.yaml"}, &out, &errOut)
		Expect(code).To(Equal(2), "measure reads no prices")
	})
})

var _ = Describe("Savings", func() {
	It("runs savings report", func() {
		var out, errOut bytes.Buffer
		code := runSavings(append([]string{"--as", "cheap-test"}, fixtureSources...), &out, &errOut)
		Expect(code).To(Equal(0), "stderr: %s", errOut.String())
		s := out.String()
		// Codex fixture until 10-01: 200 uncached, 2800 cache read, 120 output on gpt-test.
		// As run: 200*0.1 + 2800*0.01 + 120*0.5 = 108.00. As cheap-test: 0.2 + 0.28 + 0.12 = 0.60.
		// The Claude fixture's models are unpriced, so the totals hold only the Codex row.
		Expect(s).To(MatchRegexp(`all\s+108\.00\s+0\.60 \(0\.51-0\.69\)`))
		for _, want := range []string{"AS cheap-test $", "claude-sonnet-test (not in catalog)", "estimates", "assumed ±15%", "30% more tokens"} {
			Expect(s).To(ContainSubstring(want))
		}
	})

	It("rejects unknown alternative", func() {
		var out, errOut bytes.Buffer
		code := runSavings(append([]string{"--as", "nope"}, fixtureSources...), &out, &errOut)
		Expect(code).To(Equal(1))
		Expect(errOut.String()).To(ContainSubstring("not in catalog"))
	})
})

var _ = Describe("Subs", func() {
	It("runs subs report", func() {
		var out, errOut bytes.Buffer
		code := runSubs(fixtureSources, &out, &errOut)
		Expect(code).To(Equal(0), "stderr: %s", errOut.String())
		s := out.String()
		// 108.00 of API-equivalent value against a $20 plan: 5.4x.
		Expect(s).To(MatchRegexp(`chatgpt\s+2026-10 \(partial\)\s+2\s+3000\s+93\.3%\s+108\.00\s+plus \$20: 5\.4x`))
		for _, want := range []string{"if on a plan", "from $100"} {
			Expect(s).To(ContainSubstring(want))
		}
	})
})

var _ = Describe("Replay", func() {
	It("runs replay report", func() {
		var out, errOut bytes.Buffer
		code := runReplay(fixtureSources, &out, &errOut)
		Expect(code).To(Equal(0), "stderr: %s", errOut.String())
		s := out.String()
		// as-run: the Codex row only (Claude fixture models are unpriced) = 108.00.
		Expect(s).To(MatchRegexp(`as-run\s+108\.00\s+1\.00x\s+0\s+0\.00`))
		// The Codex fixture session is delegated, so moving delegated work to strong-test:
		// 200*0.01 + 2800*0.001 + 120*0.05 = 10.80.
		Expect(s).To(MatchRegexp(`delegated -> strong-test\s+10\.80\s+0\.10x`))
		// Subscription-first: the Codex usage is under the $20 chatgpt plan; nothing else is priced.
		Expect(s).To(MatchRegexp(`subscription-first.*\s+20\.00-20\.00\s+0\.19x-0\.19x`))
		// Naive router: the Codex record averages 1,500 prompt tokens per call, so cheap-test: 0.2 + 0.28 + 0.12.
		Expect(s).To(MatchRegexp(`naive per call.*\s+0\.60\s+0\.01x`))
		for _, want := range []string{"same 1 calls", "4 calls that cannot be priced", "CACHE LOSS", "lower bound", "Quality is not modeled"} {
			Expect(s).To(ContainSubstring(want))
		}
	})

	It("rejects unknown target", func() {
		var out, errOut bytes.Buffer
		code := runReplay(append([]string{"--strong", "typo-model"}, fixtureSources...), &out, &errOut)
		Expect(code).To(Equal(1))
	})
})

var _ = Describe("Footnote writer failure", func() {
	It("exits 1 on stdout footnote write failures in savings", func() {
		var errOut bytes.Buffer
		w := &failOnFootnoteWriter{}
		code := runSavings(append([]string{"--as", "cheap-test"}, fixtureSources...), w, &errOut)
		Expect(code).To(Equal(1))
		Expect(w.buf.String()).To(ContainSubstring("HARNESS"))
		Expect(errOut.String()).To(ContainSubstring("savings: write stdout:"))
	})

	It("exits 1 on stdout footnote write failures in subs", func() {
		var errOut bytes.Buffer
		w := &failOnFootnoteWriter{}
		code := runSubs(fixtureSources, w, &errOut)
		Expect(code).To(Equal(1))
		Expect(w.buf.String()).To(ContainSubstring("PLAN"))
		Expect(errOut.String()).To(ContainSubstring("subs: write stdout:"))
	})

	It("exits 1 on stdout footnote write failures in replay", func() {
		var errOut bytes.Buffer
		w := &failOnFootnoteWriter{}
		code := runReplay(fixtureSources, w, &errOut)
		Expect(code).To(Equal(1))
		Expect(w.buf.String()).To(ContainSubstring("POLICY"))
		Expect(errOut.String()).To(ContainSubstring("replay: write stdout:"))
	})
})

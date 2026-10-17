package main

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestRunMeasure(t *testing.T) {
	var out, errOut bytes.Buffer
	code := runMeasure([]string{
		"--until", "2026-10-06",
		"--claude-dir", "../../internal/ledger/testdata/claude",
		"--codex-dir", "../../internal/ledger/testdata/codex",
		"--commandcode-dir", "../../internal/ledger/testdata/commandcode",
		"--opencode-db", "",
	}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut.String())
	}
	for _, want := range []string{"claude-code", "codex", "command-code", "SPLIT", "HARNESS TOTAL"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("report lacks %q:\n%s", want, out.String())
		}
	}
	// The fixture holds one line with a fractional token count: reported, not dropped.
	if got := errOut.String(); got != "measure: warning: claude-code: skipped 1 unreadable lines and 0 unreadable files\n" {
		t.Errorf("stderr: %q", got)
	}
}

func TestRunMeasureRejectsBadDate(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := runMeasure([]string{"--since", "yesterday"}, &out, &errOut); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "RFC 3339") {
		t.Errorf("stderr: %s", errOut.String())
	}
}

func TestParseBound(t *testing.T) {
	end, err := parseBound("2026-09-30", true)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 9, 30, 23, 59, 59, 999999999, time.UTC); !end.Equal(want) {
		t.Errorf("end bound %v, want %v", end, want)
	}
	start, _ := parseBound("2026-09-30", false)
	if !start.Equal(time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("start bound %v", start)
	}
}

func TestRunMeasureExitCodes(t *testing.T) {
	for name, tc := range map[string]struct {
		args []string
		want int
	}{
		"help":       {[]string{"-h"}, 0},
		"positional": {[]string{"--claude-dir", "", "--codex-dir", "", "--opencode-db", "", "--commandcode-dir", "", "extra"}, 2},
		"no source":  {[]string{"--claude-dir", "/no/such", "--codex-dir", "", "--opencode-db", "", "--commandcode-dir", ""}, 1},
	} {
		var out, errOut bytes.Buffer
		if got := runMeasure(tc.args, &out, &errOut); got != tc.want {
			t.Errorf("%s: exit %d, want %d (stderr: %s)", name, got, tc.want, errOut.String())
		}
	}
}

var fixtureSources = []string{
	"--claude-dir", "../../internal/ledger/testdata/claude",
	"--codex-dir", "../../internal/ledger/testdata/codex",
	"--commandcode-dir", "",
	"--opencode-db", "",
	"--until", "2026-10-01",
	"--catalog", "testdata/catalog.yaml",
}

func TestRunSavings(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := runSavings(append([]string{"--as", "cheap-test"}, fixtureSources...), &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	s := out.String()
	// Codex fixture until 10-01: 200 uncached, 2800 cache read, 120 output on gpt-test.
	// As run: 200*0.1 + 2800*0.01 + 120*0.5 = 108.00. As cheap-test: 0.2 + 0.28 + 0.12 = 0.60.
	// The Claude fixture's models are unpriced, so the totals hold only the Codex row.
	if !regexp.MustCompile(`all\s+108\.00\s+0\.60 \(0\.51-0\.69\)`).MatchString(s) {
		t.Errorf("totals row wrong:\n%s", s)
	}
	for _, want := range []string{"AS cheap-test $", "claude-sonnet-test (not in catalog)", "estimates", "assumed ±15%", "30% more tokens"} {
		if !strings.Contains(s, want) {
			t.Errorf("savings output lacks %q:\n%s", want, s)
		}
	}
}

func TestRunSubs(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := runSubs(fixtureSources, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	s := out.String()
	// 108.00 of API-equivalent value against a $20 plan: 5.4x.
	if !regexp.MustCompile(`chatgpt\s+2026-10 \(partial\)\s+2\s+3000\s+93\.3%\s+108\.00\s+plus \$20: 5\.4x`).MatchString(s) {
		t.Errorf("subs row wrong:\n%s", s)
	}
	for _, want := range []string{"if on a plan", "from $100"} {
		if !strings.Contains(s, want) {
			t.Errorf("subs output lacks %q:\n%s", want, s)
		}
	}
}

func TestRunSavingsUnknownAlternative(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := runSavings(append([]string{"--as", "nope"}, fixtureSources...), &out, &errOut); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "not in catalog") {
		t.Errorf("stderr: %s", errOut.String())
	}
}

func TestMeasureRejectsCatalogFlag(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := runMeasure([]string{"--catalog", "x.yaml"}, &out, &errOut); code != 2 {
		t.Fatalf("exit %d, want 2: measure reads no prices", code)
	}
}

package main

import (
	"bytes"
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

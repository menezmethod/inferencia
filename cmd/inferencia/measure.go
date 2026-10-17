package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/menezmethod/inferencia/internal/ledger"
)

// runMeasure implements `inferencia measure`: a read-only spend report from
// coding-harness logs. It never writes to the logs and needs no gateway.
func runMeasure(args []string, stdout, stderr io.Writer) int {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(stderr, "measure: warning: no home directory (%v); pass each source flag explicitly\n", err)
	}
	def := ledger.DefaultSources(home)

	fs := flag.NewFlagSet("measure", flag.ContinueOnError)
	fs.SetOutput(stderr)
	since := fs.String("since", "", "start of the window, RFC 3339 or YYYY-MM-DD (inclusive)")
	until := fs.String("until", "", "end of the window, RFC 3339 or YYYY-MM-DD (inclusive, whole day for a date)")
	var src ledger.Sources
	fs.StringVar(&src.ClaudeDir, "claude-dir", def.ClaudeDir, "Claude Code transcripts (empty to skip)")
	fs.StringVar(&src.CodexDir, "codex-dir", def.CodexDir, "Codex CLI sessions (empty to skip)")
	fs.StringVar(&src.OpenCodeDB, "opencode-db", def.OpenCodeDB, "OpenCode database (empty to skip)")
	fs.StringVar(&src.CommandCodeDir, "commandcode-dir", def.CommandCodeDir, "Command Code projects (empty to skip)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "measure: unexpected argument %q\n", fs.Arg(0))
		return 2
	}

	var w ledger.Window
	if w.Since, err = parseBound(*since, false); err != nil {
		fmt.Fprintf(stderr, "measure: --since: %v\n", err)
		return 2
	}
	if w.Until, err = parseBound(*until, true); err != nil {
		fmt.Fprintf(stderr, "measure: --until: %v\n", err)
		return 2
	}

	us, warns, loaded := ledger.Load(src, w)
	for _, e := range warns {
		fmt.Fprintf(stderr, "measure: warning: %v\n", e)
	}
	if loaded == 0 {
		fmt.Fprintln(stderr, "measure: no source could be read")
		return 1
	}
	if err := ledger.WriteReport(stdout, us); err != nil {
		fmt.Fprintf(stderr, "measure: %v\n", err)
		return 1
	}
	return 0
}

// parseBound accepts RFC 3339 or a bare date. A bare date used as an upper
// bound means the end of that day (UTC).
func parseBound(s string, end bool) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("want RFC 3339 or YYYY-MM-DD, got %q", s)
	}
	if end {
		t = t.Add(24*time.Hour - time.Nanosecond)
	}
	return t, nil
}

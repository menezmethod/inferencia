package ledger

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"
	"unicode"
)

// Sources are the log locations measure reads. An empty field is skipped.
type Sources struct {
	ClaudeDir      string
	CodexDir       string
	OpenCodeDB     string
	CommandCodeDir string
}

// DefaultSources returns the standard log locations under home. An empty home
// returns empty Sources, so nothing is read relative to the working directory.
func DefaultSources(home string) Sources {
	if home == "" {
		return Sources{}
	}
	return Sources{
		ClaudeDir:      filepath.Join(home, ".claude", "projects"),
		CodexDir:       filepath.Join(home, ".codex", "sessions"),
		OpenCodeDB:     filepath.Join(home, ".local", "share", "opencode", "opencode.db"),
		CommandCodeDir: filepath.Join(home, ".commandcode", "projects"),
	}
}

// Load reads every configured source. A missing or failed source becomes a
// warning and the others still load; partly readable input keeps its records.
// loaded counts sources that returned records or loaded cleanly.
func Load(s Sources, w Window) (us []Usage, warns []error, loaded int) {
	type source struct {
		name, path string
		read       func(string, Window) ([]Usage, error)
	}
	for _, src := range []source{
		{"claude-code", s.ClaudeDir, ClaudeCode},
		{"codex", s.CodexDir, Codex},
		{"opencode", s.OpenCodeDB, OpenCode},
		{"command-code", s.CommandCodeDir, CommandCode},
	} {
		if src.path == "" {
			continue
		}
		got, err := src.read(src.path, w)
		var pe *PartialError
		switch {
		case err == nil:
			loaded++
		case errors.As(err, &pe):
			loaded++
			warns = append(warns, fmt.Errorf("%s: %w", src.name, err))
		case errors.Is(err, fs.ErrNotExist):
			warns = append(warns, fmt.Errorf("%s: no logs at %s", src.name, src.path))
		default:
			warns = append(warns, fmt.Errorf("%s: %w", src.name, err))
		}
		us = append(us, got...)
	}
	return us, warns, loaded
}

// WriteReport prints spend by harness, upstream and model, then solo vs
// delegated, then totals per harness.
func WriteReport(out io.Writer, us []Usage) error {
	groups := Summarize(us)
	keys := make([]Key, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b Key) int {
		ta, tb := groups[a], groups[b]
		return cmp.Or(
			cmp.Compare(a.Harness, b.Harness),
			cmp.Compare(tb.CostUSD, ta.CostUSD),
			cmp.Compare(tb.AllInput(), ta.AllInput()),
			cmp.Compare(a.Upstream, b.Upstream),
			cmp.Compare(a.Model, b.Model),
			cmp.Compare(a.Billing, b.Billing),
			boolCmp(a.Delegated, b.Delegated),
		)
	})

	tw := tabwriter.NewWriter(out, 0, 4, 2, ' ', tabwriter.AlignRight)
	const cols = "CALLS\tINPUT\tCACHE READ\tOUTPUT\tHARNESS $\t"
	fmt.Fprintln(tw, "HARNESS\tUPSTREAM\tMODEL\tBILLING\tDELEGATED\t"+cols)
	for _, k := range keys {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%t\t%s\n", clean(k.Harness), clean(k.Upstream), clean(k.Model), k.Billing, k.Delegated, row(groups[k]))
	}

	var solo, delegated []Usage
	byHarness := map[string][]Usage{}
	for _, u := range us {
		if u.Delegated {
			delegated = append(delegated, u)
		} else {
			solo = append(solo, u)
		}
		byHarness[u.Harness] = append(byHarness[u.Harness], u)
	}
	fmt.Fprintln(tw, "\t\t\t\t\t\t\t\t\t\t")
	fmt.Fprintln(tw, "SPLIT\t\t\t\t\t"+cols)
	fmt.Fprintf(tw, "solo\t\t\t\t\t%s\n", row(Sum(solo)))
	fmt.Fprintf(tw, "delegated\t\t\t\t\t%s\n", row(Sum(delegated)))
	fmt.Fprintln(tw, "\t\t\t\t\t\t\t\t\t\t")
	fmt.Fprintln(tw, "HARNESS TOTAL\t\t\t\t\t"+cols)
	names := make([]string, 0, len(byHarness))
	for n := range byHarness {
		names = append(names, n)
	}
	slices.Sort(names)
	for _, n := range names {
		fmt.Fprintf(tw, "%s\t\t\t\t\t%s\n", clean(n), row(Sum(byHarness[n])))
	}
	fmt.Fprintf(tw, "all\t\t\t\t\t%s\n", row(Sum(us)))
	if err := tw.Flush(); err != nil {
		return err
	}
	_, err := fmt.Fprint(out, "\nHARNESS $ is computed by the harness from its own prices; for plan billing it is notional, not billed.\n"+
		"\"-\" means the harness logs no cost; \"*\" means only some calls in the row carry a cost.\n")
	return err
}

func row(t Totals) string {
	return fmt.Sprintf("%d\t%s\t%s\t%s\t%s\t", t.Calls, tokens(t.AllInput()), pct(t.CacheShare()), tokens(t.Output), dollars(t))
}

func tokens(n int64) string {
	switch {
	case n >= 1_000_000_000:
		return fmt.Sprintf("%.2fB", float64(n)/1e9)
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1e3)
	}
	return fmt.Sprint(n)
}

func pct(f float64) string { return fmt.Sprintf("%.1f%%", f*100) }

func dollars(t Totals) string {
	if t.CostCalls == 0 {
		return "-"
	}
	s := fmt.Sprintf("$%.2f", t.CostUSD)
	if t.CostCalls < t.Calls {
		s += "*"
	}
	return s
}

// clean drops control characters, so a log cannot inject terminal escapes.
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

func boolCmp(a, b bool) int {
	switch {
	case a == b:
		return 0
	case !a:
		return -1
	}
	return 1
}

// Package ledger reads token usage from coding-harness logs and summarizes it.
//
// Each harness adapter is a plain function that returns []Usage. Usage is the
// one normalized record every report and price calculation works from.
package ledger

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Billing says how an upstream charges for a call.
type Billing string

const (
	BillingAPI     Billing = "api"     // pay per token
	BillingPlan    Billing = "plan"    // flat subscription; dollars are notional
	BillingLocal   Billing = "local"   // runs on your own hardware
	BillingUnknown Billing = "unknown" // not known from the logs
)

// Usage is one model call, or for Codex one session (Codex logs cumulative
// totals only). Token fields never overlap: Input is uncached input only.
type Usage struct {
	Harness   string // claude-code, codex, opencode, command-code
	Upstream  string // who served the call: anthropic, openai, opencode-go, openrouter, ...
	Model     string
	Session   string
	Time      time.Time
	Billing   Billing
	Delegated bool // made by a subagent or child session

	Calls      int
	Input      int64 // uncached input tokens
	CacheRead  int64
	CacheWrite int64
	// CacheWrite1h is the part of CacheWrite written to Anthropic's 1-hour
	// cache, which costs more than the 5-minute default.
	CacheWrite1h int64
	Output       int64 // includes reasoning tokens
	Reasoning    int64 // the reasoning part of Output, when the log reports it

	CostUSD float64 // computed by the harness from its own price table
	HasCost bool
}

// clamp zeroes negative token counts, which only a corrupt log can produce.
func (u *Usage) clamp() {
	for _, p := range []*int64{&u.Input, &u.CacheRead, &u.CacheWrite, &u.CacheWrite1h, &u.Output, &u.Reasoning} {
		*p = max(*p, 0)
	}
	u.CacheWrite1h = min(u.CacheWrite1h, u.CacheWrite)
}

// Window bounds a report in time. A zero bound is open. Both bounds are inclusive.
type Window struct {
	Since, Until time.Time
}

// Contains reports whether t lies inside w.
func (w Window) Contains(t time.Time) bool {
	if !w.Since.IsZero() && t.Before(w.Since) {
		return false
	}
	if !w.Until.IsZero() && t.After(w.Until) {
		return false
	}
	return true
}

// Totals is the sum of a group of Usage records.
type Totals struct {
	Calls      int
	Input      int64
	CacheRead  int64
	CacheWrite int64
	Output     int64
	Reasoning  int64
	CostUSD    float64
	CostCalls  int // calls that carried a harness-computed cost
}

// Add adds one record to the totals.
func (t *Totals) Add(u Usage) {
	t.Calls += u.Calls
	t.Input += u.Input
	t.CacheRead += u.CacheRead
	t.CacheWrite += u.CacheWrite
	t.Output += u.Output
	t.Reasoning += u.Reasoning
	if u.HasCost {
		t.CostUSD += u.CostUSD
		t.CostCalls += u.Calls
	}
}

// Sum totals a set of records.
func Sum(us []Usage) Totals {
	var t Totals
	for _, u := range us {
		t.Add(u)
	}
	return t
}

// AllInput is every input token: uncached, cache reads and cache writes.
func (t Totals) AllInput() int64 { return t.Input + t.CacheRead + t.CacheWrite }

// CacheShare is the fraction of all input tokens that were cache reads.
func (t Totals) CacheShare() float64 {
	all := t.AllInput()
	if all == 0 {
		return 0
	}
	return float64(t.CacheRead) / float64(all)
}

// Key groups Usage records in a report.
type Key struct {
	Harness   string
	Upstream  string
	Model     string
	Billing   Billing
	Delegated bool
}

// Summarize groups records by Key.
func Summarize(us []Usage) map[Key]Totals {
	out := map[Key]Totals{}
	for _, u := range us {
		k := Key{u.Harness, u.Upstream, u.Model, u.Billing, u.Delegated}
		t := out[k]
		t.Add(u)
		out[k] = t
	}
	return out
}

// PartialError reports input an adapter could not read. The adapter still
// returns the records it did read, so callers keep them and warn.
type PartialError struct {
	BadLines int // lines that are not valid JSON or do not fit the expected shape
	BadFiles int // files that could not be opened or read to the end
}

func (e *PartialError) Error() string {
	return fmt.Sprintf("skipped %d unreadable lines and %d unreadable files", e.BadLines, e.BadFiles)
}

// walkJSONL calls fn for every line of every *.jsonl file under root. fn
// returns false for a line it could not decode. A symlinked root is followed;
// symlinks below it are not, so a link loop cannot occur. Lines can be
// megabytes long (tool output), so it reads with ReadBytes, not a Scanner.
// The returned error is a *PartialError when only some input was unreadable.
func walkJSONL(root string, fn func(rel string, line []byte) bool) error {
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	var partial PartialError
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == root {
				return err
			}
			partial.BadFiles++
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if err := readLines(path, func(line []byte) {
			if !fn(rel, line) {
				partial.BadLines++
			}
		}); err != nil {
			partial.BadFiles++
		}
		return nil
	})
	if err != nil {
		return err
	}
	if partial != (PartialError{}) {
		return &partial
	}
	return nil
}

func readLines(path string, fn func([]byte)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	r := bufio.NewReader(f)
	for {
		line, err := r.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			fn(line)
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// keepPartial turns a *PartialError into a warning: the records stay.
func keepPartial(us []Usage, err error) ([]Usage, error) {
	var pe *PartialError
	if errors.As(err, &pe) {
		return us, err
	}
	if err != nil {
		return nil, err
	}
	return us, nil
}

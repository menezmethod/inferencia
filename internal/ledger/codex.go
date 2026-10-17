package ledger

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"time"
)

type codexLine struct {
	Timestamp time.Time       `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type codexTotals struct {
	Input     int64 `json:"input_tokens"`
	Cached    int64 `json:"cached_input_tokens"`
	CacheW    int64 `json:"cache_write_input_tokens"`
	Output    int64 `json:"output_tokens"`
	Reasoning int64 `json:"reasoning_output_tokens"`
}

func (a codexTotals) plus(b codexTotals) codexTotals {
	return codexTotals{a.Input + b.Input, a.Cached + b.Cached, a.CacheW + b.CacheW, a.Output + b.Output, a.Reasoning + b.Reasoning}
}

func (a codexTotals) minus(b codexTotals) codexTotals {
	return codexTotals{a.Input - b.Input, a.Cached - b.Cached, a.CacheW - b.CacheW, a.Output - b.Output, a.Reasoning - b.Reasoning}
}

// codexFile follows one session file. Codex counters are cumulative; if one
// drops (a counter reset), the finished segment moves into done, so the
// running total done+cur never goes down.
type codexFile struct {
	start       time.Time // first session_meta; earlier token events are replayed history
	session     string
	provider    string
	delegated   bool
	model       string
	done, cur   codexTotals
	before, end codexTotals // running total before Since, and at the last event in the window
	calls       int
	last        time.Time
}

var codexKinds = [][]byte{[]byte(`"session_meta"`), []byte(`"turn_context"`), []byte(`"token_count"`)}

// Codex reads Codex CLI session files (~/.codex/sessions). Each file yields one
// Usage: the running total at the last event in the window minus the running
// total before it. Codex runs under a ChatGPT plan here, so billing is plan.
func Codex(root string, w Window) ([]Usage, error) {
	files := map[string]*codexFile{}
	var order []string
	err := walkJSONL(root, func(rel string, line []byte) bool {
		f := files[rel]
		if f == nil {
			f = &codexFile{}
			files[rel] = f
			order = append(order, rel)
		}
		if !containsAny(line, codexKinds) {
			return true // skip the decode of large response items
		}
		var l codexLine
		if json.Unmarshal(line, &l) != nil {
			return false
		}
		switch l.Type {
		case "session_meta":
			if !f.start.IsZero() {
				return true // a forked file repeats the parent's meta; the first is ours
			}
			var p struct {
				ID       string          `json:"id"`
				Provider string          `json:"model_provider"`
				Source   json.RawMessage `json:"source"`
			}
			if json.Unmarshal(l.Payload, &p) != nil {
				return false
			}
			f.start, f.session, f.provider = l.Timestamp, p.ID, p.Provider
			f.delegated = bytes.Contains(p.Source, []byte(`"subagent"`))
		case "turn_context":
			var p struct {
				Model string `json:"model"`
			}
			if json.Unmarshal(l.Payload, &p) != nil {
				return false
			}
			if p.Model != "" && (w.Until.IsZero() || !l.Timestamp.After(w.Until)) {
				f.model = p.Model // the model in use at the end of the window
			}
		case "event_msg":
			var p struct {
				Type string `json:"type"`
				Info *struct {
					Total codexTotals `json:"total_token_usage"`
				} `json:"info"`
			}
			if json.Unmarshal(l.Payload, &p) != nil {
				return false
			}
			// Events before this file's session_meta are replayed history.
			if p.Type != "token_count" || p.Info == nil || f.start.IsZero() || l.Timestamp.Before(f.start) {
				return true
			}
			t := p.Info.Total
			if t.Input < f.cur.Input || t.Output < f.cur.Output {
				f.done = f.done.plus(f.cur)
			}
			f.cur = t
			run := f.done.plus(f.cur)
			switch {
			case !w.Since.IsZero() && l.Timestamp.Before(w.Since):
				f.before = run
			case w.Contains(l.Timestamp):
				f.end, f.calls, f.last = run, f.calls+1, l.Timestamp
			}
		}
		return true
	})
	var out []Usage
	for _, rel := range order {
		f := files[rel]
		if f.calls == 0 {
			continue
		}
		d := f.end.minus(f.before)
		session := f.session
		if session == "" {
			session = strings.TrimSuffix(filepath.Base(rel), ".jsonl")
		}
		u := Usage{
			Harness:    "codex",
			Upstream:   f.provider,
			Model:      f.model,
			Session:    session,
			Time:       f.last,
			Billing:    BillingPlan,
			Delegated:  f.delegated,
			Calls:      f.calls,
			Input:      d.Input - d.Cached - d.CacheW,
			CacheRead:  d.Cached,
			CacheWrite: d.CacheW,
			Output:     d.Output,
			Reasoning:  d.Reasoning,
		}
		u.clamp()
		out = append(out, u)
	}
	return keepPartial(out, err)
}

func containsAny(b []byte, subs [][]byte) bool {
	for _, s := range subs {
		if bytes.Contains(b, s) {
			return true
		}
	}
	return false
}

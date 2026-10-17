package ledger

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"time"
)

type commandCodeLine struct {
	Timestamp time.Time `json:"timestamp"`
	Model     string    `json:"model"`
	Usage     *struct {
		Input      int64    `json:"inputTokens"`
		Output     int64    `json:"outputTokens"`
		CacheRead  int64    `json:"cacheReadTokens"`
		CacheWrite int64    `json:"cacheWriteTokens"`
		CostUSD    *float64 `json:"costUsd"`
	} `json:"usage"`
}

// CommandCode reads Command Code session logs (~/.commandcode/projects).
// Checkpoint files repeat usage lines, so they are skipped. inputTokens
// includes cache reads (no record has cacheReadTokens > inputTokens).
func CommandCode(root string, w Window) ([]Usage, error) {
	var out []Usage
	err := walkJSONL(root, func(rel string, line []byte) bool {
		if strings.HasSuffix(rel, ".checkpoints.jsonl") || !bytes.Contains(line, []byte(`"usage"`)) {
			return true
		}
		var l commandCodeLine
		if json.Unmarshal(line, &l) != nil {
			return false
		}
		if l.Usage == nil || !w.Contains(l.Timestamp) {
			return true
		}
		u := l.Usage
		rec := Usage{
			Harness:    "command-code",
			Upstream:   "command-code",
			Model:      l.Model,
			Session:    strings.TrimSuffix(filepath.Base(rel), ".jsonl"),
			Time:       l.Timestamp,
			Billing:    BillingUnknown,
			Calls:      1,
			Input:      u.Input - u.CacheRead,
			CacheRead:  u.CacheRead,
			CacheWrite: u.CacheWrite,
			Output:     u.Output,
		}
		rec.clamp()
		if u.CostUSD != nil {
			rec.CostUSD, rec.HasCost = *u.CostUSD, true
		}
		out = append(out, rec)
		return true
	})
	return keepPartial(out, err)
}

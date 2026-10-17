package ledger

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"slices"
	"time"
)

type claudeLine struct {
	Type        string    `json:"type"`
	SessionID   string    `json:"sessionId"`
	IsSidechain bool      `json:"isSidechain"`
	Timestamp   time.Time `json:"timestamp"`
	Message     struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage *struct {
			Input      int64 `json:"input_tokens"`
			CacheWrite int64 `json:"cache_creation_input_tokens"`
			CacheRead  int64 `json:"cache_read_input_tokens"`
			Output     int64 `json:"output_tokens"`
			Creation   struct {
				OneHour int64 `json:"ephemeral_1h_input_tokens"`
				FiveMin int64 `json:"ephemeral_5m_input_tokens"`
			} `json:"cache_creation"`
		} `json:"usage"`
	} `json:"message"`
}

// ClaudeCode reads Claude Code transcripts (~/.claude/projects). One API call
// spans several lines that share a message id, and a resumed session copies
// earlier calls into a new file; for each id the copy with the most output
// wins (a streamed partial has less). "<synthetic>" entries are skipped.
func ClaudeCode(root string, w Window) ([]Usage, error) {
	byID := map[string]Usage{}
	var order []string
	err := walkJSONL(root, func(rel string, line []byte) bool {
		if !bytes.Contains(line, []byte(`"assistant"`)) {
			return true // not an assistant line; skip the decode
		}
		var l claudeLine
		if json.Unmarshal(line, &l) != nil {
			return false
		}
		m := l.Message
		if l.Type != "assistant" || m.Usage == nil || m.ID == "" || m.Model == "<synthetic>" || !w.Contains(l.Timestamp) {
			return true
		}
		u := Usage{
			Harness:      "claude-code",
			Upstream:     "anthropic",
			Model:        m.Model,
			Session:      l.SessionID,
			Time:         l.Timestamp,
			Billing:      BillingUnknown, // API key or Claude plan: not in the transcript
			Delegated:    l.IsSidechain || slices.Contains(splitPath(rel), "subagents"),
			Calls:        1,
			Input:        m.Usage.Input,
			CacheRead:    m.Usage.CacheRead,
			CacheWrite:   max(m.Usage.CacheWrite, m.Usage.Creation.OneHour+m.Usage.Creation.FiveMin),
			CacheWrite1h: m.Usage.Creation.OneHour,
			Output:       m.Usage.Output,
		}
		u.clamp()
		prev, seen := byID[m.ID]
		if !seen {
			order = append(order, m.ID)
		}
		if !seen || u.Output >= prev.Output {
			byID[m.ID] = u
		}
		return true
	})
	out := make([]Usage, 0, len(order))
	for _, id := range order {
		out = append(out, byID[id])
	}
	return keepPartial(out, err)
}

// splitPath splits a relative path into its directory names.
func splitPath(rel string) []string {
	var parts []string
	for dir := filepath.Dir(rel); dir != "." && dir != string(filepath.Separator); dir = filepath.Dir(dir) {
		parts = append(parts, filepath.Base(dir))
	}
	return parts
}

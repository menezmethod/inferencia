package ledger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// openCodeQuery reads only the message and session tables. It never touches
// the credential or account tables. json_valid keeps one corrupt row from
// failing the whole query.
const openCodeQuery = `
SELECT m.session_id AS session,
       s.parent_id AS parent,
       json_extract(m.data,'$.providerID') AS provider,
       json_extract(m.data,'$.modelID') AS model,
       json_extract(m.data,'$.time.created') AS created,
       json_extract(m.data,'$.cost') AS cost,
       json_extract(m.data,'$.tokens.input') AS input,
       json_extract(m.data,'$.tokens.output') AS output,
       json_extract(m.data,'$.tokens.reasoning') AS reasoning,
       json_extract(m.data,'$.tokens.cache.read') AS cache_read,
       json_extract(m.data,'$.tokens.cache.write') AS cache_write
FROM message m LEFT JOIN session s ON s.id = m.session_id
WHERE json_valid(m.data) AND json_extract(m.data,'$.role') = 'assistant';`

type openCodeRow struct {
	Session    string   `json:"session"`
	Parent     *string  `json:"parent"`
	Provider   string   `json:"provider"`
	Model      string   `json:"model"`
	Created    int64    `json:"created"`
	Cost       *float64 `json:"cost"`
	Input      int64    `json:"input"`
	Output     int64    `json:"output"`
	Reasoning  int64    `json:"reasoning"`
	CacheRead  int64    `json:"cache_read"`
	CacheWrite int64    `json:"cache_write"`
}

// OpenCode reads OpenCode's database (~/.local/share/opencode/opencode.db)
// through the sqlite3 CLI in read-only mode. Rows without a creation time
// count as unreadable lines in the returned *PartialError.
// ponytail: shells out to sqlite3 to avoid a driver dependency; switch to
// modernc.org/sqlite when the gateway needs a persistent ledger (D2).
func OpenCode(dbPath string, w Window) ([]Usage, error) {
	abs, err := filepath.Abs(dbPath) // an absolute path cannot be read as a sqlite3 flag
	if err != nil {
		return nil, err
	}
	if _, err = os.Stat(abs); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sqlite3", "-readonly", "-json", "-cmd", ".timeout 3000", abs, openCodeQuery)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := bytes.TrimSpace(stderr.Bytes()); len(msg) > 0 {
			return nil, fmt.Errorf("sqlite3 %s: %w: %s", abs, err, msg)
		}
		return nil, fmt.Errorf("sqlite3 %s: %w", abs, err)
	}
	var rows []openCodeRow
	if len(bytes.TrimSpace(out)) > 0 {
		if err := json.Unmarshal(out, &rows); err != nil {
			return nil, fmt.Errorf("decode sqlite3 output: %w", err)
		}
	}
	var us []Usage
	var partial PartialError
	for _, r := range rows {
		if r.Created <= 0 {
			partial.BadLines++
			continue
		}
		t := time.UnixMilli(r.Created).UTC()
		if !w.Contains(t) {
			continue
		}
		u := Usage{
			Harness:    "opencode",
			Upstream:   r.Provider,
			Model:      r.Model,
			Session:    r.Session,
			Time:       t,
			Billing:    openCodeBilling(r.Provider),
			Delegated:  r.Parent != nil && *r.Parent != "",
			Calls:      1,
			Input:      r.Input,
			CacheRead:  r.CacheRead,
			CacheWrite: r.CacheWrite,
			Output:     r.Output + r.Reasoning, // OpenCode reports reasoning apart from output
			Reasoning:  r.Reasoning,
		}
		u.clamp()
		if r.Cost != nil {
			u.CostUSD, u.HasCost = *r.Cost, true
		}
		us = append(us, u)
	}
	if partial != (PartialError{}) {
		return us, &partial
	}
	return us, nil
}

// openCodeBilling maps an OpenCode provider ID to how it bills (D1).
func openCodeBilling(provider string) Billing {
	switch provider {
	case "opencode-go", "ollama-cloud":
		return BillingPlan
	case "ollama", "lmstudio":
		return BillingLocal
	case "":
		return BillingUnknown
	default:
		return BillingAPI
	}
}

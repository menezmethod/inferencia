package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

func newHTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		return &http.Client{}
	}
	return &http.Client{Timeout: timeout}
}

// doHead is applied to every request built by doJSON (auth headers, etc).
type doHead func(*http.Request)

func withBearer(key string) doHead {
	return func(r *http.Request) {
		if key != "" {
			r.Header.Set("Authorization", "Bearer "+key)
		}
	}
}

// doJSON sends an optional JSON body and, when out is a non-nil pointer to a
// value, decodes a 2xx JSON response into it. label prefixes error messages
// (e.g. "ollama chat"); upstream error bodies are included in the returned
// error so apierror.FromBackendError can classify them.
func doJSON(ctx context.Context, client *http.Client, label, method, url string, body []byte, out any, heads ...doHead) error {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return fmt.Errorf("%s: create request: %w", label, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, h := range heads {
		h(req)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s: status %d: %s", label, resp.StatusCode, string(respBody))
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	return nil
}

package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// TTSHTTP implements the TTSBackend interface for HTTP TTS servers
// that expose an OpenAI-compatible /v1/audio/speech endpoint.
type TTSHTTP struct {
	name    string
	baseURL string
	client  *http.Client
}

// NewTTSHTTP creates a new TTSHTTP backend adapter.
func NewTTSHTTP(name, baseURL string, timeout time.Duration) *TTSHTTP {
	return &TTSHTTP{
		name:    name,
		baseURL: strings.TrimRight(baseURL, "/"),
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

// Name returns the backend identifier.
func (t *TTSHTTP) Name() string { return t.name }

// Health checks whether the TTS server is reachable.
func (t *TTSHTTP) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.baseURL+"/health", nil)
	if err != nil {
		return fmt.Errorf("create tts health request: %w", err)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("tts health check: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return fmt.Errorf("tts health check: status %d", resp.StatusCode)
	}
	return nil
}

// Synthesize calls POST /v1/audio/speech on the TTS server and returns the
// raw audio bytes.
func (t *TTSHTTP) Synthesize(ctx context.Context, req TTSRequest) (*TTSResponse, error) {
	// Copy to avoid mutating the caller's request.
	local := req

	// Apply defaults.
	if local.ResponseFormat == "" {
		local.ResponseFormat = "wav"
	}
	if local.Speed <= 0 {
		local.Speed = 1.0
	}
	if local.Voice == "" {
		local.Voice = "default"
	}

	body, err := json.Marshal(local)
	if err != nil {
		return nil, fmt.Errorf("marshal tts request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL+"/v1/audio/speech", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create tts request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("tts synthesize: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tts synthesize: status %d: %s", resp.StatusCode, string(respBody))
	}

	audio, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read tts audio: %w", err)
	}

	// Determine content type from response header or format.
	format := resp.Header.Get("Content-Type")
	if format == "" {
		format = mimeTypeForFormat(req.ResponseFormat)
	}

	return &TTSResponse{
		Audio:  audio,
		Format: format,
	}, nil
}

// Voices calls GET /v1/models on the TTS server and extracts voice info.
// Many TTS servers serve model entries as voice identifiers.
func (t *TTSHTTP) Voices(ctx context.Context) ([]Voice, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.baseURL+"/v1/models", nil)
	if err != nil {
		return nil, fmt.Errorf("create voices request: %w", err)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tts list models: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tts list models: status %d: %s", resp.StatusCode, string(respBody))
	}

	var modelsResp ModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&modelsResp); err != nil {
		return nil, fmt.Errorf("decode models response: %w", err)
	}

	voices := make([]Voice, 0, len(modelsResp.Data))
	for _, m := range modelsResp.Data {
		voices = append(voices, Voice{
			ID:   m.ID,
			Name: m.ID,
		})
	}
	return voices, nil
}

// mimeTypeForFormat maps the response_format string to a MIME type.
func mimeTypeForFormat(format string) string {
	switch strings.ToLower(format) {
	case "mp3":
		return "audio/mpeg"
	case "opus":
		return "audio/opus"
	case "flac":
		return "audio/flac"
	case "wav":
		return "audio/wav"
	case "pcm":
		return "audio/L16"
	default:
		return "audio/wav"
	}
}

package backend

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// MLX implements the Backend interface for MSTY's MLX server.
// Since MLX already speaks the OpenAI-compatible format, this adapter
// is a thin proxy: it forwards requests and passes responses through
// with minimal transformation.
type MLX struct {
	name    string
	baseURL string
	client  *http.Client
}

// NewMLX creates an MLX backend adapter.
func NewMLX(name, baseURL string, timeout time.Duration) *MLX {
	return &MLX{
		name:    name,
		baseURL: strings.TrimRight(baseURL, "/"),
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

// Name returns the backend identifier.
func (m *MLX) Name() string { return m.name }

// Health checks whether the MLX server is reachable by listing models.
func (m *MLX) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.baseURL+"/v1/models", nil)
	if err != nil {
		return fmt.Errorf("create health request: %w", err)
	}

	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("mlx health check: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("mlx health check: status %d", resp.StatusCode)
	}
	return nil
}

// ChatCompletion forwards a non-streaming chat completion request to MLX.
func (m *MLX) ChatCompletion(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	// Ensure streaming is off for this path.
	req.Stream = false

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal chat request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, m.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create chat request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("mlx chat completion: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("mlx chat completion: status %d: %s", resp.StatusCode, string(respBody))
	}

	var result ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode chat response: %w", err)
	}
	return &result, nil
}

// ChatCompletionStream forwards a streaming chat completion request to MLX.
// It reads SSE events from the MLX response and passes each data line to the
// send function. The raw JSON bytes are passed through without re-encoding,
// preserving the backend's response format.
func (m *MLX) ChatCompletionStream(ctx context.Context, req ChatRequest, send StreamFunc) error {
	req.Stream = true

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal chat request: %w", err)
	}

	// Use a client without timeout for streaming — context handles cancellation.
	streamClient := &http.Client{}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, m.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create stream request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := streamClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("mlx stream request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("mlx stream: status %d: %s", resp.StatusCode, string(respBody))
	}

	// Read SSE events line by line.
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()

		// SSE format: "data: {...}" or "data: [DONE]"
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			// Send the [DONE] sentinel so the handler can forward it.
			if err := send([]byte(data)); err != nil {
				return err
			}
			break
		}

		if err := send([]byte(data)); err != nil {
			return err
		}
	}

	return scanner.Err()
}

// ListModels retrieves available models from the MLX server.
func (m *MLX) ListModels(ctx context.Context) (*ModelsResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.baseURL+"/v1/models", nil)
	if err != nil {
		return nil, fmt.Errorf("create models request: %w", err)
	}

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("mlx list models: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("mlx list models: status %d: %s", resp.StatusCode, string(respBody))
	}

	var result ModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode models response: %w", err)
	}
	return &result, nil
}

// CreateEmbedding forwards an embeddings request to the MLX server.
func (m *MLX) CreateEmbedding(ctx context.Context, req EmbedRequest) (*EmbedResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal embed request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, m.baseURL+"/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create embed request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("mlx create embedding: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("mlx create embedding: status %d: %s", resp.StatusCode, string(respBody))
	}

	var result EmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode embed response: %w", err)
	}
	return &result, nil
}

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

// MLX implements the Backend interface for MLX servers.
// Since MLX already speaks the OpenAI-compatible format, this adapter
// is a thin proxy: it forwards requests and passes responses through
// with minimal transformation.
type MLX struct {
	name            string
	baseURL         string
	healthClient    *http.Client
	inferenceClient *http.Client
}

// NewMLX creates an MLX backend adapter.
func NewMLX(name, baseURL string, healthTimeout, inferenceTimeout time.Duration) *MLX {
	return &MLX{
		name:            name,
		baseURL:         strings.TrimRight(baseURL, "/"),
		healthClient:    newHTTPClient(healthTimeout),
		inferenceClient: newHTTPClient(inferenceTimeout),
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

	resp, err := m.healthClient.Do(req)
	if err != nil {
		return fmt.Errorf("mlx health check: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return fmt.Errorf("mlx health check: status %d", resp.StatusCode)
	}
	return nil
}

// ChatCompletion forwards a non-streaming chat completion request to MLX.
func (m *MLX) ChatCompletion(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	local := req
	local.Stream = false

	body, err := json.Marshal(local)
	if err != nil {
		return nil, fmt.Errorf("marshal chat request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, m.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create chat request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := m.inferenceClient.Do(httpReq)
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
func (m *MLX) ChatCompletionStream(ctx context.Context, req ChatRequest, send StreamFunc) error {
	local := req
	local.Stream = true

	body, err := json.Marshal(local)
	if err != nil {
		return fmt.Errorf("marshal chat request: %w", err)
	}

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

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
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

	resp, err := m.healthClient.Do(req)
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

	resp, err := m.inferenceClient.Do(httpReq)
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

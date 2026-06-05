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

// Ollama implements the Backend interface for Ollama servers.
// This adapter uses Ollama's OpenAI-compatible /v1 endpoints.
type Ollama struct {
	name    string
	baseURL string
	client  *http.Client
}

// NewOllama creates an Ollama backend adapter.
func NewOllama(name, baseURL string, timeout time.Duration) *Ollama {
	return &Ollama{
		name:    name,
		baseURL: strings.TrimRight(baseURL, "/"),
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

func (o *Ollama) Name() string { return o.name }
func (o *Ollama) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.baseURL+"/v1/models", nil)
	if err != nil {
		return fmt.Errorf("create health request: %w", err)
	}

	resp, err := o.client.Do(req)
	if err != nil {
		return fmt.Errorf("ollama health check: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return fmt.Errorf("ollama health check: status %d", resp.StatusCode)
	}
	return nil
}

func (o *Ollama) ChatCompletion(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	// Copy to avoid mutating the caller's request.
	local := req
	local.Stream = false

	body, err := json.Marshal(local)
	if err != nil {
		return nil, fmt.Errorf("marshal chat request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create chat request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama chat completion: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama chat completion: status %d: %s", resp.StatusCode, string(respBody))
	}

	var result ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode chat response: %w", err)
	}
	return &result, nil
}

func (o *Ollama) ChatCompletionStream(ctx context.Context, req ChatRequest, send StreamFunc) error {
	// Copy to avoid mutating the caller's request.
	local := req
	local.Stream = true

	body, err := json.Marshal(local)
	if err != nil {
		return fmt.Errorf("marshal chat request: %w", err)
	}

	streamClient := &http.Client{}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create stream request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := streamClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("ollama stream request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ollama stream: status %d: %s", resp.StatusCode, string(respBody))
	}

	scanner := bufio.NewScanner(resp.Body)
	// Increase scanner buffer from default 64KB to 1MB for large SSE payloads.
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

func (o *Ollama) ListModels(ctx context.Context) (*ModelsResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.baseURL+"/v1/models", nil)
	if err != nil {
		return nil, fmt.Errorf("create models request: %w", err)
	}

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama list models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama list models: status %d: %s", resp.StatusCode, string(respBody))
	}

	var result ModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode models response: %w", err)
	}
	return &result, nil
}

func (o *Ollama) CreateEmbedding(ctx context.Context, req EmbedRequest) (*EmbedResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal embed request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create embed request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama create embedding: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama create embedding: status %d: %s", resp.StatusCode, string(respBody))
	}

	var result EmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode embed response: %w", err)
	}
	return &result, nil
}

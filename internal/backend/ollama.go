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
// This adapter uses Ollama's OpenAI-compatible /v1 endpoints for inference
// and the native /api/tags endpoint for lightweight health checks.
type Ollama struct {
	name            string
	baseURL         string
	healthClient    *http.Client
	inferenceClient *http.Client
}

// NewOllama creates an Ollama backend adapter.
// healthTimeout applies to health probes and model listing; inferenceTimeout
// applies to chat and embeddings (0 disables the client timeout for long runs).
func NewOllama(name, baseURL string, healthTimeout, inferenceTimeout time.Duration) *Ollama {
	return &Ollama{
		name:            name,
		baseURL:         strings.TrimRight(baseURL, "/"),
		healthClient:    newHTTPClient(healthTimeout),
		inferenceClient: newHTTPClient(inferenceTimeout),
	}
}

func (o *Ollama) Name() string { return o.name }

func (o *Ollama) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.baseURL+"/api/tags", nil)
	if err != nil {
		return fmt.Errorf("create health request: %w", err)
	}

	resp, err := o.healthClient.Do(req)
	if err != nil {
		return fmt.Errorf("ollama health check: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return fmt.Errorf("ollama health check: status %d", resp.StatusCode)
	}
	return nil
}

func (o *Ollama) ChatCompletion(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	if req.Think != nil {
		return o.chatCompletionNative(ctx, req)
	}

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

	resp, err := o.inferenceClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama chat completion: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

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

// chatCompletionNative uses Ollama's native endpoint only for its `think`
// switch. Ollama's OpenAI-compatible endpoint currently ignores that switch,
// leaving reasoning models with an empty assistant content field.
func (o *Ollama) chatCompletionNative(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	maxTokens := req.MaxTokens
	if maxTokens == nil {
		maxTokens = req.MaxCompletionTokens
	}
	type nativeOptions struct {
		Temperature *float64 `json:"temperature,omitempty"`
		NumPredict  *int     `json:"num_predict,omitempty"`
	}
	type nativeRequest struct {
		Model    string        `json:"model"`
		Messages []Message     `json:"messages"`
		Stream   bool          `json:"stream"`
		Think    *bool         `json:"think,omitempty"`
		Options  nativeOptions `json:"options,omitempty"`
	}
	type nativeResponse struct {
		Model           string  `json:"model"`
		Message         Message `json:"message"`
		DoneReason      string  `json:"done_reason"`
		PromptEvalCount int     `json:"prompt_eval_count"`
		EvalCount       int     `json:"eval_count"`
	}

	body, err := json.Marshal(nativeRequest{
		Model: req.Model, Messages: req.Messages, Stream: false, Think: req.Think,
		Options: nativeOptions{Temperature: req.Temperature, NumPredict: maxTokens},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal native chat request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create native chat request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := o.inferenceClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama native chat completion: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama native chat completion: status %d: %s", resp.StatusCode, string(respBody))
	}
	var result nativeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode native chat response: %w", err)
	}
	finishReason := result.DoneReason
	if finishReason == "" {
		finishReason = "stop"
	}
	return &ChatResponse{
		Object: "chat.completion", Model: result.Model,
		Choices: []Choice{{Index: 0, Message: &result.Message, FinishReason: &finishReason}},
		Usage:   &Usage{PromptTokens: result.PromptEvalCount, CompletionTokens: result.EvalCount, TotalTokens: result.PromptEvalCount + result.EvalCount},
	}, nil
}

func (o *Ollama) ChatCompletionStream(ctx context.Context, req ChatRequest, send StreamFunc) error {
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
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ollama stream: status %d: %s", resp.StatusCode, string(respBody))
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

func (o *Ollama) ListModels(ctx context.Context) (*ModelsResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.baseURL+"/v1/models", nil)
	if err != nil {
		return nil, fmt.Errorf("create models request: %w", err)
	}

	resp, err := o.healthClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama list models: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

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

	resp, err := o.inferenceClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama create embedding: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

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

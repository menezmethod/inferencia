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

// Ollama implements the Backend interface for OpenAI-compatible servers:
// Ollama (with a native /api/chat path for `think`) and plain MLX servers.
type Ollama struct {
	name            string
	baseURL         string
	healthClient    *http.Client
	inferenceClient *http.Client
}

// NewOllama creates an adapter for an OpenAI-compatible server.
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

// Health probes Ollama's lightweight /api/tags first; OpenAI-compatible servers
// without native endpoints (MLX) fall back to /v1/models.
func (o *Ollama) Health(ctx context.Context) error {
	err := doJSON(ctx, o.healthClient, "ollama health check", http.MethodGet, o.baseURL+"/api/tags", nil, nil)
	if err == nil {
		return nil
	}
	if models, listErr := o.ListModels(ctx); listErr == nil && len(models.Data) > 0 {
		return nil
	}
	return err
}

func (o *Ollama) ChatCompletion(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	if req.Think != nil {
		return o.chatCompletionNative(ctx, req)
	}

	local := req
	local.Stream = false
	local.Think = nil // OpenAI-compatible endpoint ignores it

	body, err := json.Marshal(local)
	if err != nil {
		return nil, fmt.Errorf("marshal chat request: %w", err)
	}

	var result ChatResponse
	if err := doJSON(ctx, o.inferenceClient, "ollama chat completion", http.MethodPost, o.baseURL+"/v1/chat/completions", body, &result); err != nil {
		return nil, err
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

	var result nativeResponse
	if err := doJSON(ctx, o.inferenceClient, "ollama native chat completion", http.MethodPost, o.baseURL+"/api/chat", body, &result); err != nil {
		return nil, err
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
	// Ollama's OpenAI-compatible streaming endpoint ignores `think:false`.
	// That can stream only reasoning tokens and exhaust the token budget without
	// ever yielding assistant content. Use the native endpoint in this case,
	// then emit its completed answer as a standards-compatible SSE response.
	// The caller still gets a definitive [DONE] boundary rather than a guessed
	// timeout, and narration consumers never mistake private reasoning for text.
	if req.Think != nil {
		return o.chatCompletionNativeStream(ctx, req, send)
	}

	local := req
	local.Stream = true
	local.Think = nil

	body, err := json.Marshal(local)
	if err != nil {
		return fmt.Errorf("marshal chat request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create stream request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	streamClient := &http.Client{}
	resp, err := streamClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("ollama stream request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ollama stream: status %d: %s", resp.StatusCode, string(respBody))
	}

	return relaySSE(resp.Body, send)
}

func (o *Ollama) chatCompletionNativeStream(ctx context.Context, req ChatRequest, send StreamFunc) error {
	result, err := o.chatCompletionNative(ctx, req)
	if err != nil {
		return err
	}
	if len(result.Choices) == 0 || result.Choices[0].Message == nil {
		return fmt.Errorf("ollama native stream: response has no assistant message")
	}

	content := result.Choices[0].Message.Content
	if len(content) == 0 || string(content) == `""` || string(content) == "null" {
		return fmt.Errorf("ollama native stream: response has empty assistant content")
	}
	chunk, err := json.Marshal(ChatResponse{
		ID:      result.ID,
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   result.Model,
		Choices: []Choice{{
			Index: 0,
			Delta: &Message{Role: "assistant", Content: content},
		}},
	})
	if err != nil {
		return fmt.Errorf("marshal native stream chunk: %w", err)
	}
	if err := send(chunk); err != nil {
		return err
	}
	return send([]byte("[DONE]"))
}

func (o *Ollama) ListModels(ctx context.Context) (*ModelsResponse, error) {
	var result ModelsResponse
	if err := doJSON(ctx, o.healthClient, "ollama list models", http.MethodGet, o.baseURL+"/v1/models", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (o *Ollama) EmbeddingsSupported() bool { return true }

func (o *Ollama) CreateEmbedding(ctx context.Context, req EmbedRequest) (*EmbedResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal embed request: %w", err)
	}

	var result EmbedResponse
	if err := doJSON(ctx, o.inferenceClient, "ollama create embedding", http.MethodPost, o.baseURL+"/v1/embeddings", body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

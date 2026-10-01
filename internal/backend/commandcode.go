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

// CommandCode implements the Backend interface for the CommandCode provider API,
// an OpenAI-compatible HTTPS endpoint serving many models (DeepSeek, Muse, Kimi, GLM, ...).
//
// Model IDs are namespaced upstream (e.g. "deepseek/deepseek-v4.1-flash") and are
// forwarded verbatim; the router strips an optional "commandcode/" prefix before calling.
type CommandCode struct {
	name            string
	baseURL         string
	apiKey          string
	healthClient    *http.Client
	inferenceClient *http.Client
}

// NewCommandCode creates a CommandCode backend adapter.
// healthTimeout applies to health probes and model listing; inferenceTimeout
// applies to non-streaming chat (0 disables the client timeout for long runs,
// streaming always follows the request context).
func NewCommandCode(name, baseURL, apiKey string, healthTimeout, inferenceTimeout time.Duration) *CommandCode {
	return &CommandCode{
		name:            name,
		baseURL:         strings.TrimRight(baseURL, "/"),
		apiKey:          apiKey,
		healthClient:    newHTTPClient(healthTimeout),
		inferenceClient: newHTTPClient(inferenceTimeout),
	}
}

func (c *CommandCode) Name() string { return c.name }

func (c *CommandCode) Health(ctx context.Context) error {
	models, err := c.listModels(ctx)
	if err != nil {
		return err
	}
	if len(models.Data) == 0 {
		return fmt.Errorf("commandcode health check: empty model list")
	}
	return nil
}

func (c *CommandCode) ChatCompletion(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	local := req
	local.Stream = false

	body, err := json.Marshal(local)
	if err != nil {
		return nil, fmt.Errorf("marshal chat request: %w", err)
	}

	var result ChatResponse
	if err := doJSON(ctx, c.inferenceClient, "commandcode chat completion", http.MethodPost, c.baseURL+"/chat/completions", body, &result, withBearer(c.apiKey)); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *CommandCode) ChatCompletionStream(ctx context.Context, req ChatRequest, send StreamFunc) error {
	local := req
	local.Stream = true

	body, err := json.Marshal(local)
	if err != nil {
		return fmt.Errorf("marshal chat request: %w", err)
	}

	streamClient := &http.Client{}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create stream request: %w", err)
	}
	c.setHeaders(httpReq)

	resp, err := streamClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("commandcode stream request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("commandcode stream: status %d: %s", resp.StatusCode, string(respBody))
	}

	return relaySSE(resp.Body, send)
}

func (c *CommandCode) ListModels(ctx context.Context) (*ModelsResponse, error) {
	return c.listModels(ctx)
}

func (c *CommandCode) listModels(ctx context.Context) (*ModelsResponse, error) {
	var result ModelsResponse
	if err := doJSON(ctx, c.healthClient, "commandcode list models", http.MethodGet, c.baseURL+"/models", nil, &result, withBearer(c.apiKey)); err != nil {
		return nil, err
	}
	return &result, nil
}

// EmbeddingsSupported reports that the CommandCode provider has no embeddings route.
func (c *CommandCode) EmbeddingsSupported() bool { return false }

func (c *CommandCode) CreateEmbedding(_ context.Context, _ EmbedRequest) (*EmbedResponse, error) {
	return nil, fmt.Errorf("%w: commandcode provider has no embeddings route", ErrEmbeddingsUnsupported)
}

func (c *CommandCode) setHeaders(r *http.Request) {
	r.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		r.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
}

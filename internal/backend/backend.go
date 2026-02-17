// Package backend defines the interface for LLM inference backends
// and provides a registry for managing multiple backends.
//
// Each backend adapter translates between the OpenAI-compatible API
// format and the backend's native protocol. Backends that already
// speak OpenAI format (like MLX via MSTY) need minimal translation.
package backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
)

// ErrBackendNotFound is returned when a requested backend doesn't exist.
var ErrBackendNotFound = errors.New("backend not found")

// Backend represents a local LLM inference server.
type Backend interface {
	// ChatCompletion sends a non-streaming chat completion request.
	ChatCompletion(ctx context.Context, req ChatRequest) (*ChatResponse, error)

	// ChatCompletionStream sends a streaming chat completion request.
	// The send function is called for each SSE chunk. Returning an error
	// from send cancels the stream.
	ChatCompletionStream(ctx context.Context, req ChatRequest, send StreamFunc) error

	// ListModels returns the available models from this backend.
	ListModels(ctx context.Context) (*ModelsResponse, error)

	// CreateEmbedding generates embeddings for the given input.
	CreateEmbedding(ctx context.Context, req EmbedRequest) (*EmbedResponse, error)

	// Health checks whether the backend is reachable and operational.
	Health(ctx context.Context) error

	// Name returns the backend's identifier.
	Name() string
}

// StreamFunc is called for each SSE chunk during streaming completions.
type StreamFunc func(data []byte) error

// Registry manages multiple named backends and routes requests to the appropriate one.
type Registry struct {
	mu       sync.RWMutex
	backends map[string]Backend
	primary  string // default backend name
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		backends: make(map[string]Backend),
	}
}

// Register adds a backend to the registry. The first registered backend
// becomes the primary (default) backend.
func (r *Registry) Register(b Backend) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.backends[b.Name()] = b
	if r.primary == "" {
		r.primary = b.Name()
	}
}

// Get returns a backend by name. If name is empty, the primary backend is returned.
func (r *Registry) Get(name string) (Backend, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if name == "" {
		name = r.primary
	}
	b, ok := r.backends[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrBackendNotFound, name)
	}
	return b, nil
}

// Primary returns the default backend.
func (r *Registry) Primary() (Backend, error) {
	return r.Get("")
}

// All returns all registered backends.
func (r *Registry) All() []Backend {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Backend, 0, len(r.backends))
	for _, b := range r.backends {
		result = append(result, b)
	}
	return result
}

// --- OpenAI-compatible request/response types ---

// ChatRequest represents an OpenAI chat completion request.
// All fields are passed through to the backend, including tool calling fields.
type ChatRequest struct {
	Model            string          `json:"model"`
	Messages         []Message       `json:"messages"`
	Temperature      *float64        `json:"temperature,omitempty"`
	TopP             *float64        `json:"top_p,omitempty"`
	N                *int            `json:"n,omitempty"`
	MaxTokens        *int            `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int         `json:"max_completion_tokens,omitempty"`
	Stop             json.RawMessage `json:"stop,omitempty"`
	Stream           bool            `json:"stream"`
	PresencePenalty  *float64        `json:"presence_penalty,omitempty"`
	FrequencyPenalty *float64        `json:"frequency_penalty,omitempty"`
	User             string          `json:"user,omitempty"`

	// Tool calling support (OpenAI function calling protocol).
	Tools      []Tool          `json:"tools,omitempty"`
	ToolChoice json.RawMessage `json:"tool_choice,omitempty"`

	// Response format (structured outputs).
	ResponseFormat json.RawMessage `json:"response_format,omitempty"`
}

// Message represents a single message in a chat conversation.
type Message struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"` // string or array of content parts
	Name       string          `json:"name,omitempty"`
	ToolCalls  []ToolCall      `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

// Tool represents a tool definition in the OpenAI format.
type Tool struct {
	Type     string       `json:"type"` // "function"
	Function ToolFunction `json:"function"`
}

// ToolFunction describes a function tool.
type ToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// ToolCall represents a tool call made by the model.
type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"` // "function"
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction contains the function name and arguments in a tool call.
type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ChatResponse represents an OpenAI chat completion response.
type ChatResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   *Usage   `json:"usage,omitempty"`
}

// Choice represents a single completion choice.
type Choice struct {
	Index        int      `json:"index"`
	Message      *Message `json:"message,omitempty"`
	Delta        *Message `json:"delta,omitempty"`
	FinishReason *string  `json:"finish_reason"`
}

// Usage reports token consumption.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ModelsResponse represents the OpenAI models list response.
type ModelsResponse struct {
	Object string  `json:"object"`
	Data   []Model `json:"data"`
}

// Model represents a single model in the models list.
type Model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// EmbedRequest represents an OpenAI embeddings request.
type EmbedRequest struct {
	Model          string          `json:"model"`
	Input          json.RawMessage `json:"input"` // string or []string
	EncodingFormat string          `json:"encoding_format,omitempty"`
}

// EmbedResponse represents an OpenAI embeddings response.
type EmbedResponse struct {
	Object string      `json:"object"`
	Data   []Embedding `json:"data"`
	Model  string      `json:"model"`
	Usage  *Usage      `json:"usage,omitempty"`
}

// Embedding represents a single embedding vector.
type Embedding struct {
	Object    string    `json:"object"`
	Index     int       `json:"index"`
	Embedding []float64 `json:"embedding"`
}

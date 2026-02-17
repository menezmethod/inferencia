package backend

import (
	"context"
	"errors"
)

// Ollama implements the Backend interface for Ollama servers.
// This is a stub for v1 — Ollama support will be wired in a future release.
// Ollama's /api/chat endpoint uses a different format than OpenAI, so this
// adapter will need to translate between the two.
type Ollama struct {
	name    string
	baseURL string
}

// NewOllama creates an Ollama backend adapter (stub).
func NewOllama(name, baseURL string) *Ollama {
	return &Ollama{name: name, baseURL: baseURL}
}

var errNotImplemented = errors.New("ollama backend not yet implemented")

func (o *Ollama) Name() string                      { return o.name }
func (o *Ollama) Health(context.Context) error       { return errNotImplemented }

func (o *Ollama) ChatCompletion(context.Context, ChatRequest) (*ChatResponse, error) {
	return nil, errNotImplemented
}

func (o *Ollama) ChatCompletionStream(context.Context, ChatRequest, StreamFunc) error {
	return errNotImplemented
}

func (o *Ollama) ListModels(context.Context) (*ModelsResponse, error) {
	return nil, errNotImplemented
}

func (o *Ollama) CreateEmbedding(context.Context, EmbedRequest) (*EmbedResponse, error) {
	return nil, errNotImplemented
}

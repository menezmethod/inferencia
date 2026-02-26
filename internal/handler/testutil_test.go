package handler

import (
	"context"
	"io"
	"log/slog"

	"github.com/menezmethod/inferencia/internal/backend"
)

// discardLogger returns a logger that writes to /dev/null.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// mockBackend implements backend.Backend for testing.
type mockBackend struct {
	chatResp   *backend.ChatResponse
	chatErr    error
	modelsResp *backend.ModelsResponse
	modelsErr  error
	embedResp  *backend.EmbedResponse
	embedErr   error
	healthErr  error
}

func (m *mockBackend) Name() string { return "mock" }

func (m *mockBackend) Health(context.Context) error { return m.healthErr }

func (m *mockBackend) ChatCompletion(_ context.Context, _ backend.ChatRequest) (*backend.ChatResponse, error) {
	return m.chatResp, m.chatErr
}

func (m *mockBackend) ChatCompletionStream(_ context.Context, _ backend.ChatRequest, send backend.StreamFunc) error {
	chunk := `{"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"hi"}}]}`
	if err := send([]byte(chunk)); err != nil {
		return err
	}
	return send([]byte("[DONE]"))
}

func (m *mockBackend) ListModels(context.Context) (*backend.ModelsResponse, error) {
	return m.modelsResp, m.modelsErr
}

func (m *mockBackend) CreateEmbedding(_ context.Context, _ backend.EmbedRequest) (*backend.EmbedResponse, error) {
	return m.embedResp, m.embedErr
}

func newTestRegistry(b backend.Backend) *backend.Registry {
	reg := backend.NewRegistry()
	reg.Register(b)
	return reg
}

package handler

import (
	"context"
	"io"
	"log/slog"

	"github.com/menezmethod/inferencia/internal/backend"
	"github.com/menezmethod/inferencia/internal/router"
)

// discardLogger returns a logger that writes to /dev/null.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// mockBackend implements backend.Backend for testing.
type mockBackend struct {
	chatResp    *backend.ChatResponse
	chatErr     error
	lastChatReq backend.ChatRequest
	modelsResp  *backend.ModelsResponse
	modelsErr   error
	embedResp   *backend.EmbedResponse
	embedErr    error
	healthErr   error
}

func (m *mockBackend) Name() string { return "mock" }

func (m *mockBackend) Health(context.Context) error { return m.healthErr }

func (m *mockBackend) ChatCompletion(_ context.Context, req backend.ChatRequest) (*backend.ChatResponse, error) {
	m.lastChatReq = req
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

type stubHealthChecker struct {
	healthy map[string]bool
}

func (s stubHealthChecker) IsHealthy(name string) bool {
	if s.healthy == nil {
		return true
	}
	return s.healthy[name]
}

func newTestRegistry(b backend.Backend) *backend.Registry {
	reg := backend.NewRegistry()
	reg.Register(b)
	return reg
}

// mockTTSBackend implements backend.TTSBackend for testing.
type mockTTSBackend struct {
	name      string
	healthErr error
	voices    []backend.Voice
	voicesErr error
}

func (m *mockTTSBackend) Name() string { return m.name }

func (m *mockTTSBackend) Health(context.Context) error { return m.healthErr }

func (m *mockTTSBackend) Synthesize(_ context.Context, _ backend.TTSRequest) (*backend.TTSResponse, error) {
	return &backend.TTSResponse{Audio: []byte("mock-audio"), Format: "audio/wav"}, nil
}

func (m *mockTTSBackend) Voices(context.Context) ([]backend.Voice, error) {
	return m.voices, m.voicesErr
}

// newTestTTSRegistry creates a router.Registry with a single TTS backend.
func newTestTTSRegistry(mock *mockTTSBackend) *router.Registry {
	r := router.NewRegistry()
	r.Register(router.BackendInfo{
		Name:       mock.name,
		TTSBackend: mock,
		Capabilities: []router.Capability{router.CapTTS},
		Models:     []router.ModelInfo{{ID: mock.name, Kind: router.CapTTS}},
	})
	return r
}

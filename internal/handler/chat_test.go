package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/menezmethod/inferencia/internal/backend"
)

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

func TestChatCompletionsJSON(t *testing.T) {
	finish := "stop"
	mock := &mockBackend{
		chatResp: &backend.ChatResponse{
			ID:     "chatcmpl-test",
			Object: "chat.completion",
			Choices: []backend.Choice{
				{
					Index:        0,
					Message:      &backend.Message{Role: "assistant", Content: json.RawMessage(`"Hello!"`)},
					FinishReason: &finish,
				},
			},
		},
	}
	reg := newTestRegistry(mock)
	handler := ChatCompletions(reg, discardLogger())

	body := `{"model":"test","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}

	var resp backend.ChatResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.ID != "chatcmpl-test" {
		t.Errorf("response ID = %q, want chatcmpl-test", resp.ID)
	}
}

func TestChatCompletionsEmptyMessages(t *testing.T) {
	reg := newTestRegistry(&mockBackend{})
	handler := ChatCompletions(reg, discardLogger())

	body := `{"model":"test","messages":[]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestChatCompletionsInvalidJSON(t *testing.T) {
	reg := newTestRegistry(&mockBackend{})
	handler := ChatCompletions(reg, discardLogger())

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("not json"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestChatCompletionsStream(t *testing.T) {
	mock := &mockBackend{}
	reg := newTestRegistry(mock)
	handler := ChatCompletions(reg, discardLogger())

	body := `{"model":"test","messages":[{"role":"user","content":"hi"}],"stream":true}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}

	respBody := rec.Body.String()
	if !strings.Contains(respBody, "data: ") {
		t.Error("response should contain SSE data lines")
	}
	if !strings.Contains(respBody, "[DONE]") {
		t.Error("response should contain [DONE] sentinel")
	}
}

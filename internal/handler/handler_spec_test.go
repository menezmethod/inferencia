package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/menezmethod/inferencia/internal/backend"
	"github.com/menezmethod/inferencia/internal/config"
)

var _ = Describe("Ready", func() {
	When("the backend is healthy", func() {
		It("returns 200", func() {
			mock := &mockBackend{healthErr: nil}
			reg := newTestRegistry(mock)
			h := Ready(reg)
			req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			Expect(rec.Code).To(Equal(http.StatusOK))
		})
	})

	When("the backend is unhealthy", func() {
		It("returns 503", func() {
			mock := &mockBackend{healthErr: errors.New("connection refused")}
			reg := newTestRegistry(mock)
			h := Ready(reg)
			req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			Expect(rec.Code).To(Equal(http.StatusServiceUnavailable))
		})
	})
})

var _ = Describe("HealthStatus", func() {
	When("all backends are healthy", func() {
		It("returns 200 with per-service breakdown", func() {
			mock := &mockBackend{healthErr: nil}
			reg := newTestRegistry(mock)
			h := HealthStatus(reg, nil)

			req := httptest.NewRequest(http.MethodGet, "/health/status", nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusOK))
			var resp HealthStatusResponse
			Expect(json.NewDecoder(rec.Body).Decode(&resp)).NotTo(HaveOccurred())
			Expect(resp.Status).To(Equal("healthy"))
			Expect(resp.Services).To(HaveKey("mock"))
			Expect(resp.Services["mock"].Status).To(Equal("healthy"))
			Expect(resp.Version).NotTo(BeEmpty())
		})
	})

	When("a chat backend is unhealthy", func() {
		It("returns 503 with error detail", func() {
			mock := &mockBackend{healthErr: errors.New("connection refused")}
			reg := newTestRegistry(mock)
			h := HealthStatus(reg, nil)

			req := httptest.NewRequest(http.MethodGet, "/health/status", nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusServiceUnavailable))
			var resp HealthStatusResponse
			Expect(json.NewDecoder(rec.Body).Decode(&resp)).NotTo(HaveOccurred())
			Expect(resp.Status).To(Equal("degraded"))
			Expect(resp.Services["mock"].Status).To(Equal("unhealthy"))
			Expect(resp.Services["mock"].Error).To(ContainSubstring("connection refused"))
		})
	})

	When("a TTS backend is unhealthy", func() {
		It("returns 503 and reports TTS as degraded", func() {
			mock := &mockBackend{healthErr: nil}
			reg := newTestRegistry(mock)
			ttsMock := &mockTTSBackend{
				name:      "kokoro",
				healthErr: errors.New("tts timeout"),
			}
			ttsReg := newTestTTSRegistry(ttsMock)
			h := HealthStatus(reg, ttsReg)

			req := httptest.NewRequest(http.MethodGet, "/health/status", nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusServiceUnavailable))
			var resp HealthStatusResponse
			Expect(json.NewDecoder(rec.Body).Decode(&resp)).NotTo(HaveOccurred())
			Expect(resp.Status).To(Equal("degraded"))
			Expect(resp.Services["mock"].Status).To(Equal("healthy"))
			Expect(resp.Services["kokoro"].Status).To(Equal("unhealthy"))
			Expect(resp.Services["kokoro"].Error).To(ContainSubstring("tts timeout"))
		})
	})

	When("both chat and TTS are unhealthy", func() {
		It("returns 503 with both reported", func() {
			mock := &mockBackend{healthErr: errors.New("ollama down")}
			reg := newTestRegistry(mock)
			ttsMock := &mockTTSBackend{
				name:      "kokoro",
				healthErr: errors.New("kokoro down"),
			}
			ttsReg := newTestTTSRegistry(ttsMock)
			h := HealthStatus(reg, ttsReg)

			req := httptest.NewRequest(http.MethodGet, "/health/status", nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusServiceUnavailable))
			var resp HealthStatusResponse
			Expect(json.NewDecoder(rec.Body).Decode(&resp)).NotTo(HaveOccurred())
			Expect(resp.Status).To(Equal("degraded"))
			Expect(resp.Services["mock"].Status).To(Equal("unhealthy"))
			Expect(resp.Services["kokoro"].Status).To(Equal("unhealthy"))
		})
	})

	When("no backends are registered at all", func() {
		It("returns 503 with _none service", func() {
			reg := backend.NewRegistry()
			h := HealthStatus(reg, nil)

			req := httptest.NewRequest(http.MethodGet, "/health/status", nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusServiceUnavailable))
			var resp HealthStatusResponse
			Expect(json.NewDecoder(rec.Body).Decode(&resp)).NotTo(HaveOccurred())
			Expect(resp.Status).To(Equal("degraded"))
			Expect(resp.Services["_none"].Status).To(Equal("unhealthy"))
		})
	})

	When("a healthy backend returns models", func() {
		It("includes the model list in health status", func() {
			mock := &mockBackend{
				healthErr: nil,
				modelsResp: &backend.ModelsResponse{
					Object: "list",
					Data: []backend.Model{
						{ID: "gemma4:e4b", Object: "model", OwnedBy: "local"},
						{ID: "llama3.2", Object: "model", OwnedBy: "local"},
					},
				},
			}
			reg := newTestRegistry(mock)
			h := HealthStatus(reg, nil)

			req := httptest.NewRequest(http.MethodGet, "/health/status", nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusOK))
			var resp HealthStatusResponse
			Expect(json.NewDecoder(rec.Body).Decode(&resp)).NotTo(HaveOccurred())
			Expect(resp.Services["mock"].Models).To(HaveLen(2))
			Expect(resp.Services["mock"].Models[0].ID).To(Equal("gemma4:e4b"))
			Expect(resp.Services["mock"].Models[1].ID).To(Equal("llama3.2"))
			Expect(resp.Summary.Total).To(Equal(1))
			Expect(resp.Summary.Healthy).To(Equal(1))
			Expect(resp.Timestamp).NotTo(BeEmpty())
		})
	})

	When("a TTS backend returns voices", func() {
		It("includes the voice list in health status", func() {
			mock := &mockBackend{healthErr: nil}
			reg := newTestRegistry(mock)
			ttsMock := &mockTTSBackend{
				name:      "kokoro",
				healthErr: nil,
				voices: []backend.Voice{
					{ID: "af_bella", Name: "af_bella", Gender: "female"},
					{ID: "am_michael", Name: "am_michael", Gender: "male"},
				},
			}
			ttsReg := newTestTTSRegistry(ttsMock)
			h := HealthStatus(reg, ttsReg)

			req := httptest.NewRequest(http.MethodGet, "/health/status", nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusOK))
			var resp HealthStatusResponse
			Expect(json.NewDecoder(rec.Body).Decode(&resp)).NotTo(HaveOccurred())
			Expect(resp.Services["kokoro"].Models).To(HaveLen(2))
			Expect(resp.Services["kokoro"].Models[0].ID).To(Equal("af_bella"))
			Expect(resp.Summary.Total).To(Equal(2))
			Expect(resp.Summary.Healthy).To(Equal(2))
			Expect(resp.Summary.ByType).To(HaveKeyWithValue("chat", 1))
			Expect(resp.Summary.ByType).To(HaveKeyWithValue("tts", 1))
		})
	})

	When("a backend has models but a TTS is unhealthy", func() {
		It("includes models for healthy backends only", func() {
			chatMock := &mockBackend{
				healthErr: nil,
				modelsResp: &backend.ModelsResponse{
					Object: "list",
					Data:   []backend.Model{{ID: "gemma4:e4b", Object: "model", OwnedBy: "local"}},
				},
			}
			reg := newTestRegistry(chatMock)
			ttsMock := &mockTTSBackend{
				name:      "chatterbox",
				healthErr: errors.New("connection timeout"),
			}
			ttsReg := newTestTTSRegistry(ttsMock)
			h := HealthStatus(reg, ttsReg)

			req := httptest.NewRequest(http.MethodGet, "/health/status", nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusServiceUnavailable))
			var resp HealthStatusResponse
			Expect(json.NewDecoder(rec.Body).Decode(&resp)).NotTo(HaveOccurred())
			Expect(resp.Status).To(Equal("degraded"))
			Expect(resp.Services["mock"].Models).To(HaveLen(1))
			Expect(resp.Services["mock"].Models[0].ID).To(Equal("gemma4:e4b"))
			Expect(resp.Services["chatterbox"].Models).To(BeEmpty())
			Expect(resp.Summary.Total).To(Equal(2))
			Expect(resp.Summary.Healthy).To(Equal(1))
			Expect(resp.Summary.Unhealthy).To(Equal(1))
		})
	})
})

var _ = Describe("Models", func() {
	When("the backend returns a model list", func() {
		It("returns 200 and the list", func() {
			mock := &mockBackend{
				modelsResp: &backend.ModelsResponse{
					Object: "list",
					Data:   []backend.Model{{ID: "gpt-oss-20b", Object: "model", OwnedBy: "local"}},
				},
			}
			reg := newTestRegistry(mock)
			h := Models(reg, nil, config.Routing{}, nil, discardLogger())
			req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusOK))
			var resp backend.ModelsResponse
			Expect(json.NewDecoder(rec.Body).Decode(&resp)).NotTo(HaveOccurred())
			Expect(resp.Data).To(HaveLen(1))
			Expect(resp.Data[0].ID).To(Equal("mock/gpt-oss-20b"))
		})
	})

	When("the backend is down", func() {
		It("returns 503", func() {
			mock := &mockBackend{modelsErr: errors.New("connection refused")}
			reg := newTestRegistry(mock)
			h := Models(reg, nil, config.Routing{}, nil, discardLogger())
			req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			Expect(rec.Code).To(Equal(http.StatusServiceUnavailable))
		})
	})

	When("no primary backend is registered", func() {
		It("returns 503 BackendUnavailable", func() {
			reg := backend.NewRegistry()
			h := Models(reg, nil, config.Routing{}, nil, discardLogger())
			req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			Expect(rec.Code).To(Equal(http.StatusServiceUnavailable))
		})
	})
})

var _ = Describe("ChatCompletions", func() {
	When("request is valid and backend returns a completion", func() {
		It("returns 200 and the completion", func() {
			finish := "stop"
			mock := &mockBackend{
				chatResp: &backend.ChatResponse{
					ID:      "chatcmpl-test",
					Object:  "chat.completion",
					Model:   "test",
					Choices: []backend.Choice{{Index: 0, Message: &backend.Message{Role: "assistant", Content: json.RawMessage(`"Hello!"`)}, FinishReason: &finish}},
					Usage:   &backend.Usage{PromptTokens: 2, CompletionTokens: 3},
				},
			}
			reg := newTestRegistry(mock)
			h := ChatCompletions(reg, nil, config.Routing{}, nil, nil, discardLogger())
			body := `{"model":"test","messages":[{"role":"user","content":"hi"}]}`
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusOK))
			var resp backend.ChatResponse
			Expect(json.NewDecoder(rec.Body).Decode(&resp)).NotTo(HaveOccurred())
			Expect(resp.ID).To(Equal("chatcmpl-test"))
		})
	})

	When("messages are empty", func() {
		It("returns 400", func() {
			reg := newTestRegistry(&mockBackend{})
			h := ChatCompletions(reg, nil, config.Routing{}, nil, nil, discardLogger())
			body := `{"model":"test","messages":[]}`
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			Expect(rec.Code).To(Equal(http.StatusBadRequest))
		})
	})

	When("model is missing", func() {
		It("uses the default chat model fallback", func() {
			mock := &mockBackend{
				chatResp: &backend.ChatResponse{
					ID:      "chatcmpl-test",
					Object:  "chat.completion",
					Model:   defaultChatModel,
					Choices: []backend.Choice{{Index: 0, Message: &backend.Message{Role: "assistant", Content: json.RawMessage(`"Hello!"`)}, FinishReason: func() *string { s := "stop"; return &s }()}},
				},
			}
			reg := newTestRegistry(mock)
			h := ChatCompletions(reg, nil, config.Routing{}, nil, nil, discardLogger())
			body := `{"messages":[{"role":"user","content":"hi"}]}`
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			h.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusOK))
			Expect(mock.lastChatReq.Model).To(Equal(defaultChatModel))
		})
	})

	When("logprobs and seed are provided", func() {
		It("passes them through to the backend", func() {
			finish := "stop"
			mock := &mockBackend{
				chatResp: &backend.ChatResponse{
					ID:      "chatcmpl-test",
					Object:  "chat.completion",
					Model:   "test",
					Choices: []backend.Choice{{Index: 0, Message: &backend.Message{Role: "assistant", Content: json.RawMessage(`"Hello!"`)}, FinishReason: &finish}},
				},
			}
			reg := newTestRegistry(mock)
			h := ChatCompletions(reg, nil, config.Routing{}, nil, nil, discardLogger())
			body := `{"model":"test","messages":[{"role":"user","content":"hi"}],"logprobs":true,"top_logprobs":5,"seed":42}`
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusOK))
			Expect(mock.lastChatReq.Logprobs).NotTo(BeNil())
			Expect(*mock.lastChatReq.Logprobs).To(BeTrue())
			Expect(mock.lastChatReq.TopLogprobs).NotTo(BeNil())
			Expect(*mock.lastChatReq.TopLogprobs).To(Equal(5))
			Expect(mock.lastChatReq.Seed).NotTo(BeNil())
			Expect(*mock.lastChatReq.Seed).To(Equal(42))
		})
	})

	When("body is invalid JSON", func() {
		It("returns 400", func() {
			reg := newTestRegistry(&mockBackend{})
			h := ChatCompletions(reg, nil, config.Routing{}, nil, nil, discardLogger())
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("not json"))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			Expect(rec.Code).To(Equal(http.StatusBadRequest))
		})
	})

	When("stream is true", func() {
		It("returns 200 with SSE and [DONE]", func() {
			reg := newTestRegistry(&mockBackend{})
			h := ChatCompletions(reg, nil, config.Routing{}, nil, nil, discardLogger())
			body := `{"model":"test","messages":[{"role":"user","content":"hi"}],"stream":true}`
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusOK))
			Expect(rec.Header().Get("Content-Type")).To(Equal("text/event-stream"))
			respBody := rec.Body.String()
			Expect(respBody).To(ContainSubstring("data: "))
			Expect(respBody).To(ContainSubstring("[DONE]"))
		})
	})

	When("no primary backend is registered", func() {
		It("returns 503 BackendUnavailable", func() {
			reg := backend.NewRegistry() // empty
			h := ChatCompletions(reg, nil, config.Routing{}, nil, nil, discardLogger())
			body := `{"model":"test","messages":[{"role":"user","content":"hi"}]}`
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			Expect(rec.Code).To(Equal(http.StatusServiceUnavailable))
		})
	})

	When("backend times out", func() {
		It("returns 504 backend_timeout", func() {
			mock := &mockBackend{chatErr: errors.New("ollama chat completion: context deadline exceeded")}
			reg := newTestRegistry(mock)
			h := ChatCompletions(reg, nil, config.Routing{}, nil, nil, discardLogger())
			body := `{"model":"test","messages":[{"role":"user","content":"hi"}]}`
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(504))
			var resp struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			Expect(json.NewDecoder(rec.Body).Decode(&resp)).NotTo(HaveOccurred())
			Expect(resp.Error.Code).To(Equal("backend_timeout"))
		})
	})

	When("backend is overloaded", func() {
		It("returns 503 backend_overloaded", func() {
			mock := &mockBackend{chatErr: errors.New("ollama chat completion: status 429: server busy")}
			reg := newTestRegistry(mock)
			h := ChatCompletions(reg, nil, config.Routing{}, nil, nil, discardLogger())
			body := `{"model":"test","messages":[{"role":"user","content":"hi"}]}`
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusServiceUnavailable))
			var resp struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			Expect(json.NewDecoder(rec.Body).Decode(&resp)).NotTo(HaveOccurred())
			Expect(resp.Error.Code).To(Equal("backend_overloaded"))
		})
	})

	When("backend is degraded by watchdog", func() {
		It("returns 503 without calling the backend", func() {
			mock := &mockBackend{
				chatResp: &backend.ChatResponse{ID: "should-not-run"},
			}
			reg := newTestRegistry(mock)
			hc := stubHealthChecker{healthy: map[string]bool{"mock": false}}
			h := ChatCompletions(reg, hc, config.Routing{}, nil, nil, discardLogger())
			body := `{"model":"test","messages":[{"role":"user","content":"hi"}]}`
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusServiceUnavailable))
			Expect(mock.lastChatReq.Model).To(BeEmpty())
		})
	})

	When("stream is true but ResponseWriter is not a Flusher", func() {
		It("returns 500 Internal", func() {
			reg := newTestRegistry(&mockBackend{})
			h := ChatCompletions(reg, nil, config.Routing{}, nil, nil, discardLogger())
			body := `{"model":"test","messages":[{"role":"user","content":"hi"}],"stream":true}`
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			// Wrap so that w.(http.Flusher) fails in handleStream
			type noFlusher struct{ http.ResponseWriter }
			h.ServeHTTP(noFlusher{rec}, req)
			Expect(rec.Code).To(Equal(http.StatusInternalServerError))
		})
	})
})

var _ = Describe("Role routing", func() {
	When("a virtual role resolves to an upstream model", func() {
		It("rewrites the model and sets routing headers", func() {
			finish := "stop"
			mock := &mockBackend{
				chatResp: &backend.ChatResponse{
					ID: "chatcmpl-test", Object: "chat.completion", Model: "deepseek/deepseek-v4.1-flash",
					Choices: []backend.Choice{{Index: 0, Message: &backend.Message{Role: "assistant", Content: json.RawMessage(`"ok"`)}, FinishReason: &finish}},
				},
				modelsResp: &backend.ModelsResponse{Object: "list"},
			}
			reg := newTestRegistry(mock)
			routing := config.Routing{
				DefaultRole: "inferencia-worker",
				Roles: map[string]config.Role{
					"inferencia-worker": {Backend: "mock", Models: []string{"deepseek/deepseek-v4.1-flash"}},
				},
			}
			h := ChatCompletions(reg, nil, routing, nil, nil, discardLogger())
			body := `{"model":"inferencia-worker","messages":[{"role":"user","content":"hi"}]}`
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusOK))
			Expect(mock.lastChatReq.Model).To(Equal("deepseek/deepseek-v4.1-flash"))
			Expect(rec.Header().Get("X-Routed-Backend")).To(Equal("mock"))
			Expect(rec.Header().Get("X-Routed-Model")).To(Equal("deepseek/deepseek-v4.1-flash"))
			Expect(rec.Header().Get("X-Routed-Role")).To(Equal("worker"))
		})
	})

	When("an unknown model arrives with a default role", func() {
		It("falls back to the default role model", func() {
			finish := "stop"
			mock := &mockBackend{
				chatResp: &backend.ChatResponse{
					ID: "chatcmpl-test", Object: "chat.completion", Model: "x",
					Choices: []backend.Choice{{Index: 0, Message: &backend.Message{Role: "assistant", Content: json.RawMessage(`"ok"`)}, FinishReason: &finish}},
				},
			}
			reg := newTestRegistry(mock)
			routing := config.Routing{
				DefaultRole: "inferencia-worker",
				Roles: map[string]config.Role{
					"inferencia-worker": {Backend: "mock", Models: []string{"fallback-model"}},
				},
			}
			h := ChatCompletions(reg, nil, routing, nil, nil, discardLogger())
			body := `{"model":"nope-unknown","messages":[{"role":"user","content":"hi"}]}`
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusOK))
			Expect(mock.lastChatReq.Model).To(Equal("fallback-model"))
			Expect(rec.Header().Get("X-Routed-Fallback")).To(Equal("worker"))
			Expect(rec.Header().Get("X-Routed-Role")).To(Equal("worker"))
		})

		It("does not mark fallback when a direct backend/model is served", func() {
			finish := "stop"
			mock := &mockBackend{
				chatResp: &backend.ChatResponse{
					ID: "chatcmpl-test", Object: "chat.completion", Model: "x",
					Choices: []backend.Choice{{Index: 0, Message: &backend.Message{Role: "assistant", Content: json.RawMessage(`"ok"`)}, FinishReason: &finish}},
				},
			}
			reg := newTestRegistry(mock)
			h := ChatCompletions(reg, nil, config.Routing{}, nil, nil, discardLogger())
			body := `{"model":"mock/direct-model","messages":[{"role":"user","content":"hi"}]}`
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusOK))
			Expect(mock.lastChatReq.Model).To(Equal("direct-model"))
			Expect(rec.Header().Get("X-Routed-Fallback")).To(BeEmpty())
			Expect(rec.Header().Get("X-Routed-Backend")).To(Equal("mock"))
		})
	})

	When("GET /v1/models aggregates roles and backends", func() {
		It("lists virtual roles plus namespaced upstream models", func() {
			mock := &mockBackend{
				modelsResp: &backend.ModelsResponse{
					Object: "list",
					Data: []backend.Model{
						{ID: "qwen3", Object: "model", OwnedBy: "local"},
						{ID: "deepseek/deepseek-v4.1-flash", Object: "model", OwnedBy: "commandcode"},
					},
				},
			}
			reg := newTestRegistry(mock)
			routing := config.Routing{
				Roles: map[string]config.Role{
					"inferencia-worker": {Backend: "mock", Models: []string{"m"}},
				},
			}
			h := Models(reg, nil, routing, nil, discardLogger())
			req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusOK))
			var resp backend.ModelsResponse
			Expect(json.NewDecoder(rec.Body).Decode(&resp)).NotTo(HaveOccurred())
			ids := []string{}
			for _, m := range resp.Data {
				ids = append(ids, m.ID)
			}
			Expect(ids).To(ContainElement("inferencia-worker"))
			Expect(ids).To(ContainElement("mock/qwen3"))
			Expect(ids).To(ContainElement("mock/deepseek/deepseek-v4.1-flash"))
		})
	})

	When("a chat-only backend and an embed backend are registered", func() {
		It("routes embeddings to the embed-capable backend", func() {
			chatOnly := &mockEmbedUnsupportedBackend{}
			embedder := &mockBackend{name: "embedder", embedResp: &backend.EmbedResponse{Object: "list"}}
			reg := backend.NewRegistry()
			reg.Register(chatOnly)
			reg.Register(embedder)
			h := Embeddings(reg, nil, discardLogger())
			body := `{"model":"x","input":"hi"}`
			req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(body))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			Expect(rec.Code).To(Equal(http.StatusOK))
		})
	})

	When("only a chat-only backend is registered", func() {
		It("returns 503, not a bogus probe", func() {
			mock := &mockEmbedUnsupportedBackend{}
			reg := newTestRegistry(mock)
			h := Embeddings(reg, nil, discardLogger())
			body := `{"model":"x","input":"hi"}`
			req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(body))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			Expect(rec.Code).To(Equal(http.StatusServiceUnavailable))
		})
	})
})

var _ = Describe("Embeddings", func() {
	When("request is valid and backend returns embeddings", func() {
		It("returns 200 and the embeddings", func() {
			mock := &mockBackend{
				embedResp: &backend.EmbedResponse{
					Object: "list",
					Data:   []backend.Embedding{{Object: "embedding", Index: 0, Embedding: []float64{0.1, 0.2}}},
					Model:  "test-embed",
					Usage:  &backend.Usage{PromptTokens: 2},
				},
			}
			reg := newTestRegistry(mock)
			h := Embeddings(reg, nil, discardLogger())
			body := `{"model":"test-embed","input":"hello world"}`
			req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusOK))
			var resp backend.EmbedResponse
			Expect(json.NewDecoder(rec.Body).Decode(&resp)).NotTo(HaveOccurred())
			Expect(resp.Object).To(Equal("list"))
			Expect(resp.Data).To(HaveLen(1))
			Expect(resp.Data[0].Embedding).To(HaveLen(2))
		})
	})

	When("input is missing", func() {
		It("returns 400", func() {
			reg := newTestRegistry(&mockBackend{})
			h := Embeddings(reg, nil, discardLogger())
			body := `{"model":"test"}`
			req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			Expect(rec.Code).To(Equal(http.StatusBadRequest))
		})
	})

	When("body is invalid JSON", func() {
		It("returns 400", func() {
			reg := newTestRegistry(&mockBackend{})
			h := Embeddings(reg, nil, discardLogger())
			req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader("not json"))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			Expect(rec.Code).To(Equal(http.StatusBadRequest))
		})
	})

	When("the backend returns an error", func() {
		It("returns 503", func() {
			mock := &mockBackend{embedErr: errors.New("backend down")}
			reg := newTestRegistry(mock)
			h := Embeddings(reg, nil, discardLogger())
			body := `{"model":"test","input":"hi"}`
			req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			Expect(rec.Code).To(Equal(http.StatusServiceUnavailable))
		})
	})

	When("no primary backend is registered", func() {
		It("returns 503 BackendUnavailable", func() {
			reg := backend.NewRegistry()
			h := Embeddings(reg, nil, discardLogger())
			body := `{"model":"test","input":"hi"}`
			req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			Expect(rec.Code).To(Equal(http.StatusServiceUnavailable))
		})
	})
})

var _ = Describe("Audio", func() {
	When("speed is omitted", func() {
		It("defaults to 1.0 before synthesis", func() {
			ttsMock := &mockTTSBackend{name: "kokoro"}
			ttsReg := newTestTTSRegistry(ttsMock)
			h := Audio(ttsReg, nil, discardLogger())
			body := `{"input":"hello","model":"kokoro"}`
			req := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusOK))
			Expect(ttsMock.lastTTSReq.Speed).To(Equal(1.0))
		})
	})

	When("speed is below the minimum", func() {
		It("clamps to 0.25", func() {
			ttsMock := &mockTTSBackend{name: "kokoro"}
			ttsReg := newTestTTSRegistry(ttsMock)
			h := Audio(ttsReg, nil, discardLogger())
			body := `{"input":"hello","model":"kokoro","speed":0.1}`
			req := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusOK))
			Expect(ttsMock.lastTTSReq.Speed).To(Equal(0.25))
		})
	})
})

var _ = Describe("VersionInfo", func() {
	It("returns 200 with version in JSON", func() {
		h := VersionInfo()
		req := httptest.NewRequest(http.MethodGet, "/version", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(rec.Header().Get("Content-Type")).To(Equal("application/json"))
		Expect(rec.Header().Get("Cache-Control")).To(Equal("no-store"))
		var resp map[string]string
		Expect(json.NewDecoder(rec.Body).Decode(&resp)).NotTo(HaveOccurred())
		Expect(resp).To(HaveKey("version"))
	})
})

var _ = Describe("OpenAPI", func() {
	It("returns 200 and application/x-yaml with openapi spec", func() {
		h := OpenAPI()
		req := httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(rec.Header().Get("Content-Type")).To(Equal("application/x-yaml"))
		Expect(rec.Body.String()).To(ContainSubstring("openapi"))
	})
})

var _ = Describe("SwaggerUI", func() {
	It("returns 200 and text/html with swagger-ui", func() {
		h := SwaggerUI()
		req := httptest.NewRequest(http.MethodGet, "/docs", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(rec.Header().Get("Content-Type")).To(ContainSubstring("text/html"))
		Expect(rec.Body.String()).To(ContainSubstring("swagger-ui"))
	})
})

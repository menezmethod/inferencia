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
)

var _ = Describe("Health", func() {
	It("returns 200 and status ok", func() {
		h := Health()
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		Expect(rec.Code).To(Equal(http.StatusOK))
		var resp map[string]string
		Expect(json.NewDecoder(rec.Body).Decode(&resp)).NotTo(HaveOccurred())
		Expect(resp["status"]).To(Equal("ok"))
	})
})

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
			h := Models(reg, nil, discardLogger())
			req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusOK))
			var resp backend.ModelsResponse
			Expect(json.NewDecoder(rec.Body).Decode(&resp)).NotTo(HaveOccurred())
			Expect(resp.Data).To(HaveLen(1))
			Expect(resp.Data[0].ID).To(Equal("gpt-oss-20b"))
		})
	})

	When("the backend is down", func() {
		It("returns 503", func() {
			mock := &mockBackend{modelsErr: errors.New("connection refused")}
			reg := newTestRegistry(mock)
			h := Models(reg, nil, discardLogger())
			req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			Expect(rec.Code).To(Equal(http.StatusServiceUnavailable))
		})
	})

	When("no primary backend is registered", func() {
		It("returns 503 BackendUnavailable", func() {
			reg := backend.NewRegistry()
			h := Models(reg, nil, discardLogger())
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
			h := ChatCompletions(reg, nil, discardLogger())
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
			h := ChatCompletions(reg, nil, discardLogger())
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
			h := ChatCompletions(reg, nil, discardLogger())
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
			h := ChatCompletions(reg, nil, discardLogger())
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
			h := ChatCompletions(reg, nil, discardLogger())
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("not json"))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			Expect(rec.Code).To(Equal(http.StatusBadRequest))
		})
	})

	When("stream is true", func() {
		It("returns 200 with SSE and [DONE]", func() {
			reg := newTestRegistry(&mockBackend{})
			h := ChatCompletions(reg, nil, discardLogger())
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
			h := ChatCompletions(reg, nil, discardLogger())
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
			h := ChatCompletions(reg, nil, discardLogger())
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
			h := ChatCompletions(reg, nil, discardLogger())
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
			h := ChatCompletions(reg, hc, discardLogger())
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
			h := ChatCompletions(reg, nil, discardLogger())
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

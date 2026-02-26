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
			h := Models(reg, discardLogger())
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
			h := Models(reg, discardLogger())
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
					Choices: []backend.Choice{{Index: 0, Message: &backend.Message{Role: "assistant", Content: json.RawMessage(`"Hello!"`)}, FinishReason: &finish}},
				},
			}
			reg := newTestRegistry(mock)
			h := ChatCompletions(reg, discardLogger())
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
			h := ChatCompletions(reg, discardLogger())
			body := `{"model":"test","messages":[]}`
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			Expect(rec.Code).To(Equal(http.StatusBadRequest))
		})
	})

	When("body is invalid JSON", func() {
		It("returns 400", func() {
			reg := newTestRegistry(&mockBackend{})
			h := ChatCompletions(reg, discardLogger())
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("not json"))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			Expect(rec.Code).To(Equal(http.StatusBadRequest))
		})
	})

	When("stream is true", func() {
		It("returns 200 with SSE and [DONE]", func() {
			reg := newTestRegistry(&mockBackend{})
			h := ChatCompletions(reg, discardLogger())
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
			h := Embeddings(reg, discardLogger())
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
			h := Embeddings(reg, discardLogger())
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
			h := Embeddings(reg, discardLogger())
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
			h := Embeddings(reg, discardLogger())
			body := `{"model":"test","input":"hi"}`
			req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			Expect(rec.Code).To(Equal(http.StatusServiceUnavailable))
		})
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

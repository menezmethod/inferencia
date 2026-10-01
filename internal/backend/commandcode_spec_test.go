package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("CommandCode", func() {
	chatBody := func(model string) string {
		finish := "stop"
		resp := ChatResponse{
			ID: "chatcmpl-test", Object: "chat.completion", Model: model,
			Choices: []Choice{{Index: 0, Message: &Message{Role: "assistant", Content: json.RawMessage(`"ok"`)}, FinishReason: &finish}},
			Usage:   &Usage{PromptTokens: 13, CompletionTokens: 3, TotalTokens: 16},
		}
		b, _ := json.Marshal(resp)
		return string(b)
	}

	newServer := func(handler http.HandlerFunc) (*httptest.Server, *[]string) {
		var paths []string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			paths = append(paths, r.Method+" "+r.URL.Path)
			handler(w, r)
		}))
		DeferCleanup(srv.Close)
		return srv, &paths
	}

	Describe("ChatCompletion", func() {
		// stdlib testing.T shim for Ginkgo: use GinkgoT via helper below.
		It("posts to /chat/completions with bearer key and verbatim model", func() {
			srv, paths := newServer(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.Header.Get("Authorization")).To(Equal("Bearer sk-test"))
				var req ChatRequest
				Expect(json.NewDecoder(r.Body).Decode(&req)).To(Succeed())
				Expect(req.Model).To(Equal("deepseek/deepseek-v4.1-flash"))
				Expect(req.Stream).To(BeFalse())
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(chatBody("deepseek/deepseek-v4.1-flash")))
			})
			c := NewCommandCode("cc", srv.URL, "sk-test", 5*time.Second, 30*time.Second)
			resp, err := c.ChatCompletion(context.Background(), ChatRequest{
				Model:    "deepseek/deepseek-v4.1-flash",
				Messages: []Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Model).To(Equal("deepseek/deepseek-v4.1-flash"))
			Expect(*paths).To(Equal([]string{"POST /chat/completions"}))
		})

		It("does not send Authorization header when key is empty", func() {
			srv, _ := newServer(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.Header.Get("Authorization")).To(BeEmpty())
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(chatBody("m")))
			})
			c := NewCommandCode("cc", srv.URL, "", 5*time.Second, 30*time.Second)
			_, err := c.ChatCompletion(context.Background(), ChatRequest{Model: "m"})
			Expect(err).NotTo(HaveOccurred())
		})

		It("surfaces upstream 401/429/503 in the error", func() {
			for _, code := range []int{401, 429, 503} {
				srv, _ := newServer(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(code)
					_, _ = w.Write([]byte(`{"error":"denied"}`))
				})
				c := NewCommandCode("cc", srv.URL, "k", 5*time.Second, 30*time.Second)
				_, err := c.ChatCompletion(context.Background(), ChatRequest{Model: "m"})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("status %d", code)))
			}
		})

		It("fails on invalid JSON response", func() {
			srv, _ := newServer(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("not json"))
			})
			c := NewCommandCode("cc", srv.URL, "k", 5*time.Second, 30*time.Second)
			_, err := c.ChatCompletion(context.Background(), ChatRequest{Model: "m"})
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("ChatCompletionStream", func() {
		It("relays SSE chunks and [DONE]", func() {
			srv, _ := newServer(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprintln(w, `data: {"delta":"hi"}`)
				_, _ = fmt.Fprintln(w, "")
				_, _ = fmt.Fprintln(w, `data: [DONE]`)
			})
			c := NewCommandCode("cc", srv.URL, "k", 5*time.Second, 0)
			var got []string
			err := c.ChatCompletionStream(context.Background(), ChatRequest{Model: "m"}, func(d []byte) error {
				got = append(got, string(d))
				return nil
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(got[len(got)-1]).To(Equal("[DONE]"))
			Expect(strings.Join(got, "")).To(ContainSubstring("hi"))
		})

		It("propagates send errors (client disconnect)", func() {
			srv, _ := newServer(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprintln(w, `data: {"delta":"hi"}`)
				_, _ = fmt.Fprintln(w, `data: [DONE]`)
			})
			c := NewCommandCode("cc", srv.URL, "k", 5*time.Second, 0)
			err := c.ChatCompletionStream(context.Background(), ChatRequest{Model: "m"}, func(d []byte) error {
				return fmt.Errorf("client gone")
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("client gone"))
		})

		It("maps upstream 429 to an overload error", func() {
			srv, _ := newServer(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(429)
				_, _ = w.Write([]byte("busy"))
			})
			c := NewCommandCode("cc", srv.URL, "k", 5*time.Second, 0)
			err := c.ChatCompletionStream(context.Background(), ChatRequest{Model: "m"}, func(d []byte) error { return nil })
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("status 429"))
		})
	})

	Describe("ListModels and Health", func() {
		It("lists models from /models", func() {
			srv, _ := newServer(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.URL.Path).To(Equal("/models"))
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"deepseek/deepseek-v4.1-flash","object":"model"}]}`))
			})
			c := NewCommandCode("cc", srv.URL, "k", 5*time.Second, 30*time.Second)
			resp, err := c.ListModels(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Data).To(HaveLen(1))
		})

		It("Health fails on empty model list and on 500", func() {
			empty, _ := newServer(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`{"object":"list","data":[]}`))
			})
			Expect(NewCommandCode("cc", empty.URL, "k", 5*time.Second, 30*time.Second).Health(context.Background())).To(HaveOccurred())

			bad, _ := newServer(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(500)
			})
			Expect(NewCommandCode("cc", bad.URL, "k", 5*time.Second, 30*time.Second).Health(context.Background())).To(HaveOccurred())
		})

		It("Health fails when the server is unreachable", func() {
			c := NewCommandCode("cc", "http://127.0.0.1:1", "k", 100*time.Millisecond, 30*time.Second)
			Expect(c.Health(context.Background())).To(HaveOccurred())
		})
	})

	Describe("CreateEmbedding", func() {
		It("returns an unsupported error", func() {
			c := NewCommandCode("cc", "http://127.0.0.1:1", "k", time.Second, time.Second)
			_, err := c.CreateEmbedding(context.Background(), EmbedRequest{Model: "x"})
			Expect(err).To(MatchError(ContainSubstring("embeddings not supported")))
		})
	})
})

var _ = Describe("relaySSE", func() {
	It("skips non-data lines and tolerates missing [DONE]", func() {
		var got []string
		err := relaySSE(strings.NewReader(": comment\n\ndata: {\"a\":1}\n\n"), func(d []byte) error {
			got = append(got, string(d))
			return nil
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(Equal([]string{`{"a":1}`}))
	})
})

package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/menezmethod/inferencia/internal/backend"
	"github.com/menezmethod/inferencia/internal/config"
	"github.com/menezmethod/inferencia/internal/decision"
)

// decisionEngine starts a stub /v1/systemone engine that records the model it
// received and returns a minimal typed answer.
func decisionEngine(name string, lastModel *string) (*decision.Client, string) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		*lastModel = req.Model
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"` + name + `-1.0","answers":{"urgency":{"type":"noul","noul":1.0}}}`))
	}))
	DeferCleanup(srv.Close)
	return decision.New(name, srv.URL, ""), srv.URL
}

var _ = Describe("SystemOne", func() {
	When("multiple decision engines are configured", func() {
		It("routes a namespaced model to its engine and strips the prefix", func() {
			var layaModel, jevModel string
			laya, _ := decisionEngine("laya", &layaModel)
			jev, _ := decisionEngine("jev", &jevModel)
			h := SystemOne(decision.NewSet(laya, jev), discardLogger())

			body := `{"model":"jev/jev-latest","state":"urgent ticket"}`
			req := httptest.NewRequest(http.MethodPost, "/v1/systemone", strings.NewReader(body))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusOK))
			Expect(rec.Header().Get("X-Decision-Backend")).To(Equal("jev"))
			Expect(jevModel).To(Equal("jev-latest"))
			Expect(layaModel).To(BeEmpty())
		})

		It("routes an advertised model ID to the engine that lists it", func() {
			var layaModel string
			_, layaURL := decisionEngine("laya", &layaModel)
			set := decision.NewSet(
				decision.New("laya", layaURL, "", "laya", "laya-multilingual"),
				decision.New("jev", "http://127.0.0.1:1", "", "jev-latest"),
			)
			h := SystemOne(set, discardLogger())

			body := `{"model":"laya-multilingual","state":"bonjour"}`
			req := httptest.NewRequest(http.MethodPost, "/v1/systemone", strings.NewReader(body))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusOK))
			Expect(rec.Header().Get("X-Decision-Backend")).To(Equal("laya"))
			Expect(layaModel).To(Equal("laya-multilingual"))
		})

		It("defaults to the first configured engine (local-first)", func() {
			var layaModel string
			laya, _ := decisionEngine("laya", &layaModel)
			set := decision.NewSet(laya, decision.New("jev", "http://127.0.0.1:1", ""))
			h := SystemOne(set, discardLogger())

			req := httptest.NewRequest(http.MethodPost, "/v1/systemone", strings.NewReader(`{"state":"no model"}`))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusOK))
			Expect(rec.Header().Get("X-Decision-Backend")).To(Equal("laya"))
		})
	})

	When("the engine is unreachable", func() {
		It("returns an OpenAI-shaped 503", func() {
			h := SystemOne(decision.NewSet(decision.New("laya", "http://127.0.0.1:1", "")), discardLogger())
			req := httptest.NewRequest(http.MethodPost, "/v1/systemone", strings.NewReader(`{"state":"x"}`))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusServiceUnavailable))
			Expect(rec.Body.String()).To(ContainSubstring(`"error"`))
		})
	})

	When("no decision engines are configured", func() {
		It("returns 503", func() {
			h := SystemOne(decision.NewSet(), discardLogger())
			req := httptest.NewRequest(http.MethodPost, "/v1/systemone", strings.NewReader(`{}`))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			Expect(rec.Code).To(Equal(http.StatusServiceUnavailable))
		})
	})
})

var _ = Describe("Models with decision engines", func() {
	It("advertises engines and their models", func() {
		set := decision.NewSet(
			decision.New("laya", "http://127.0.0.1:1", "", "laya-multilingual"),
			decision.New("jev", "http://127.0.0.1:2", "", "jev-latest"),
		)
		reg := newTestRegistry(&mockBackend{})
		h := Models(reg, nil, config.Routing{}, set, discardLogger())

		req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		var resp backend.ModelsResponse
		Expect(json.NewDecoder(rec.Body).Decode(&resp)).To(Succeed())
		ids := []string{}
		for _, m := range resp.Data {
			ids = append(ids, m.ID)
		}
		Expect(ids).To(ContainElement("laya"))
		Expect(ids).To(ContainElement("laya/laya-multilingual"))
		Expect(ids).To(ContainElement("jev/jev-latest"))
	})
})

var _ = Describe("Chat auto routing", func() {
	chatResp := func(model string) *backend.ChatResponse {
		finish := "stop"
		return &backend.ChatResponse{
			ID: "chatcmpl-auto", Object: "chat.completion", Model: model,
			Choices: []backend.Choice{{Index: 0, Message: &backend.Message{Role: "assistant", Content: json.RawMessage(`"ok"`)}, FinishReason: &finish}},
		}
	}

	newAutoRegistry := func() (*backend.Registry, *mockBackend, *mockBackend, *mockBackend) {
		reg := backend.NewRegistry()
		planner := &mockBackend{name: "planner-b", chatResp: chatResp("big")}
		worker := &mockBackend{name: "worker-b", chatResp: chatResp("mid")}
		fast := &mockBackend{name: "fast-b", chatResp: chatResp("small")}
		reg.Register(planner)
		reg.Register(worker)
		reg.Register(fast)
		return reg, planner, worker, fast
	}

	autoRouting := func() config.Routing {
		return config.Routing{
			DefaultRole: "inferencia-worker",
			Roles: map[string]config.Role{
				"inferencia-planner": {Backend: "planner-b", Models: []string{"big"}},
				"inferencia-worker":  {Backend: "worker-b", Models: []string{"mid"}},
				"inferencia-fast":    {Backend: "fast-b", Models: []string{"small"}},
			},
			Auto: config.Auto{
				Decision: "laya",
				Criteria: map[string]string{
					"planner": "complex reasoning",
					"worker":  "routine execution",
					"fast":    "trivial requests",
				},
			},
		}
	}

	It("routes the auto role to the tier chosen by the decision engine", func() {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"answers":{"tier":{"type":"choice","choice":"fast","confidence":0.88}}}`))
		}))
		DeferCleanup(srv.Close)
		set := decision.NewSet(decision.New("laya", srv.URL, ""))

		reg, _, worker, fast := newAutoRegistry()
		h := ChatCompletions(reg, nil, autoRouting(), nil, set, discardLogger())
		body := `{"model":"inferencia-auto","messages":[{"role":"user","content":"what is 2+2"}]}`
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(rec.Header().Get("X-Routed-Auto")).To(Equal("fast 0.88"))
		Expect(rec.Header().Get("X-Routed-Reason")).To(Equal("AUTO_TIER_SELECTED"))
		Expect(rec.Header().Get("X-Routed-Role")).To(Equal("fast"))
		Expect(fast.lastChatReq.Model).To(Equal("small"))
		Expect(worker.lastChatReq.Model).To(BeEmpty())
	})

	It("falls back to the default role when the decision engine fails", func() {
		set := decision.NewSet(decision.New("laya", "http://127.0.0.1:1", ""))

		reg, _, worker, fast := newAutoRegistry()
		h := ChatCompletions(reg, nil, autoRouting(), nil, set, discardLogger())
		body := `{"model":"inferencia-auto","messages":[{"role":"user","content":"hello"}]}`
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(rec.Header().Get("X-Routed-Auto")).To(Equal("inferencia-worker"))
		Expect(rec.Header().Get("X-Routed-Reason")).To(Equal("AUTO_DECISION_UNAVAILABLE"))
		Expect(worker.lastChatReq.Model).To(Equal("mid"))
		Expect(fast.lastChatReq.Model).To(BeEmpty())
	})

	It("does not consult the decision engine for other roles", func() {
		var called bool
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			called = true
			_, _ = w.Write([]byte(`{}`))
		}))
		DeferCleanup(srv.Close)
		set := decision.NewSet(decision.New("laya", srv.URL, ""))

		reg, _, worker, _ := newAutoRegistry()
		h := ChatCompletions(reg, nil, autoRouting(), nil, set, discardLogger())
		body := `{"model":"inferencia-worker","messages":[{"role":"user","content":"hello"}]}`
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(called).To(BeFalse())
		Expect(worker.lastChatReq.Model).To(Equal("mid"))
	})

	It("auto:free overrides a paid pick with a local role", func() {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"answers":{"tier":{"type":"choice","choice":"worker","confidence":0.9}}}`))
		}))
		DeferCleanup(srv.Close)
		set := decision.NewSet(decision.New("laya", srv.URL, ""))

		reg := backend.NewRegistry()
		paid := &mockBackend{name: "paid-b", chatResp: chatResp("big")}
		local := &mockBackend{name: "local-b", chatResp: chatResp("small")}
		reg.Register(paid)
		reg.Register(local)
		routing := config.Routing{
			DefaultRole: "inferencia-worker",
			Roles: map[string]config.Role{
				"inferencia-worker": {Backend: "paid-b", Models: []string{"big"}},
				"inferencia-fast":   {Backend: "local-b", Models: []string{"small"}},
			},
			Auto: config.Auto{Decision: "laya", Criteria: map[string]string{"worker": "w", "fast": "f"}},
		}
		h := ChatCompletions(reg, nil, routing, map[string]bool{"local-b": true}, set, discardLogger())
		body := `{"model":"auto:free","messages":[{"role":"user","content":"hello"}]}`
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(rec.Header().Get("X-Routed-Reason")).To(Equal("AUTO_POLICY_LOCAL_ONLY"))
		Expect(rec.Header().Get("X-Routed-Backend")).To(Equal("local-b"))
		Expect(local.lastChatReq.Model).To(Equal("small"))
		Expect(paid.lastChatReq.Model).To(BeEmpty())
	})

	It("rejects an unknown auto policy with 400", func() {
		reg, _, _, _ := newAutoRegistry()
		h := ChatCompletions(reg, nil, autoRouting(), nil, decision.NewSet(), discardLogger())
		body := `{"model":"auto:plaid","messages":[{"role":"user","content":"hi"}]}`
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		Expect(rec.Code).To(Equal(http.StatusBadRequest))
		Expect(rec.Body.String()).To(ContainSubstring("unknown auto policy"))
	})

	It("falls back without a confidence when the engine abstains", func() {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"answers":{"tier":{"type":"choice","choice":"planner","confidence":0.2,"abstention":"abstained"}}}`))
		}))
		DeferCleanup(srv.Close)
		set := decision.NewSet(decision.New("laya", srv.URL, ""))

		reg, _, worker, _ := newAutoRegistry()
		routing := autoRouting()
		routing.Auto.MinConfidence = 0.6
		h := ChatCompletions(reg, nil, routing, nil, set, discardLogger())
		body := `{"model":"inferencia-auto","messages":[{"role":"user","content":"ambiguous"}]}`
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(rec.Header().Get("X-Routed-Reason")).To(Equal("AUTO_LOW_CONFIDENCE_FALLBACK"))
		Expect(rec.Header().Get("X-Routed-Auto")).To(Equal("inferencia-worker"))
		Expect(worker.lastChatReq.Model).To(Equal("mid"))
	})

	It("sends policy-specific criteria to the engine", func() {
		var receivedCriteria []string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req struct {
				Questions map[string]struct {
					Criteria map[string]string `json:"criteria"`
				} `json:"questions"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			for k := range req.Questions["tier"].Criteria {
				receivedCriteria = append(receivedCriteria, k)
			}
			_, _ = w.Write([]byte(`{"answers":{"tier":{"type":"choice","choice":"planner","confidence":0.8}}}`))
		}))
		DeferCleanup(srv.Close)
		set := decision.NewSet(decision.New("laya", srv.URL, ""))

		reg, planner, _, _ := newAutoRegistry()
		routing := autoRouting()
		routing.Auto.Policies = map[string]config.AutoPolicy{
			"quality": {Criteria: map[string]string{"planner": "only the strongest tier"}},
		}
		h := ChatCompletions(reg, nil, routing, nil, set, discardLogger())
		body := `{"model":"auto:quality","messages":[{"role":"user","content":"design a system"}]}`
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(receivedCriteria).To(Equal([]string{"planner"}))
		Expect(planner.lastChatReq.Model).To(Equal("big"))
	})
})

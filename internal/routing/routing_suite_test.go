package routing

import (
	"context"
	"testing"

	"github.com/menezmethod/inferencia/internal/backend"
	"github.com/menezmethod/inferencia/internal/config"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

func TestRouting(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Routing Suite")
}

type stubBackend struct {
	name   string
	models []string
}

func (s *stubBackend) Name() string                 { return s.name }
func (s *stubBackend) Health(context.Context) error { return nil }
func (s *stubBackend) ChatCompletion(_ context.Context, req backend.ChatRequest) (*backend.ChatResponse, error) {
	return &backend.ChatResponse{Model: req.Model}, nil
}
func (s *stubBackend) ChatCompletionStream(_ context.Context, _ backend.ChatRequest, _ backend.StreamFunc) error {
	return nil
}
func (s *stubBackend) ListModels(context.Context) (*backend.ModelsResponse, error) {
	return &backend.ModelsResponse{Object: "list"}, nil
}
func (s *stubBackend) CreateEmbedding(_ context.Context, _ backend.EmbedRequest) (*backend.EmbedResponse, error) {
	return &backend.EmbedResponse{}, nil
}

type stubHC struct{ unhealthy map[string]bool }

func (s stubHC) IsHealthy(name string) bool { return !s.unhealthy[name] }

func testRouting() config.Routing {
	return config.Routing{
		DefaultRole: "inferencia-worker",
		Roles: map[string]config.Role{
			"inferencia-planner": {Backend: "cc", Models: []string{"meta/muse-spark-1.3-contributor"}},
			"inferencia-worker":  {Backend: "cc", Models: []string{"deepseek/deepseek-v4.1-flash"}},
			"inferencia-fast":    {Backend: "cc", Models: []string{"deepseek/deepseek-v4.1-flash"}},
		},
	}
}

func testRegistry() *backend.Registry {
	reg := backend.NewRegistry()
	reg.Register(&stubBackend{name: "cc"})
	return reg
}

var _ = ginkgo.Describe("Resolve", func() {
	ginkgo.It("maps inferencia-planner to the planner model", func() {
		res, err := Resolve(testRegistry(), nil, testRouting(), "inferencia-planner")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(res.BackendName).To(gomega.Equal("cc"))
		gomega.Expect(res.UpstreamModel).To(gomega.Equal("meta/muse-spark-1.3-contributor"))
		gomega.Expect(res.Role).To(gomega.Equal("planner"))
	})

	ginkgo.It("accepts bare role aliases", func() {
		res, err := Resolve(testRegistry(), nil, testRouting(), "worker")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(res.UpstreamModel).To(gomega.Equal("deepseek/deepseek-v4.1-flash"))
		gomega.Expect(res.Role).To(gomega.Equal("worker"))
	})

	ginkgo.It("passes commandcode/ namespaced models verbatim to that backend", func() {
		res, err := Resolve(testRegistry(), nil, testRouting(), "cc/deepseek/deepseek-v4.1-flash")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(res.BackendName).To(gomega.Equal("cc"))
		gomega.Expect(res.UpstreamModel).To(gomega.Equal("deepseek/deepseek-v4.1-flash"))
	})

	ginkgo.It("round-trips newly namespaced advertised IDs", func() {
		// /v1/models now advertises "cc/model" even when the upstream model ID
		// contains slashes; the exact ID must resolve back to the same backend.
		res, err := Resolve(testRegistry(), nil, testRouting(), "cc/meta/muse-spark-1.3-contributor")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(res.BackendName).To(gomega.Equal("cc"))
		gomega.Expect(res.UpstreamModel).To(gomega.Equal("meta/muse-spark-1.3-contributor"))
		gomega.Expect(res.Role).To(gomega.BeEmpty())
	})

	ginkgo.It("falls back to the default role for unknown models", func() {
		res, err := Resolve(testRegistry(), nil, testRouting(), "totally-unknown-model")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(res.UpstreamModel).To(gomega.Equal("deepseek/deepseek-v4.1-flash"))
		gomega.Expect(res.Advertised).To(gomega.Equal("totally-unknown-model"))
	})

	ginkgo.It("defaults empty model to the default role", func() {
		res, err := Resolve(testRegistry(), nil, testRouting(), "")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(res.Role).To(gomega.Equal("worker"))
	})

	ginkgo.It("errors when the role backend is degraded", func() {
		hc := stubHC{unhealthy: map[string]bool{"cc": true}}
		_, err := Resolve(testRegistry(), hc, testRouting(), "inferencia-planner")
		gomega.Expect(err).To(gomega.MatchError(backend.ErrNoHealthyBackend))
	})

	ginkgo.It("errors when no backends are registered", func() {
		_, err := Resolve(backend.NewRegistry(), nil, testRouting(), "inferencia-worker")
		gomega.Expect(err).To(gomega.HaveOccurred())
	})

	ginkgo.It("falls back to the primary backend for bare models without a role", func() {
		reg := backend.NewRegistry()
		reg.Register(&stubBackend{name: "cc", models: []string{"custom-model"}})
		res, err := Resolve(reg, nil, config.Routing{}, "custom-model")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(res.BackendName).To(gomega.Equal("cc"))
	})
})

package backend

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// minimalBackend implements Backend for registry tests (no HTTP).
type minimalBackend struct {
	name string
}

func (m minimalBackend) Name() string                          { return m.name }
func (m minimalBackend) Health(context.Context) error         { return nil }
func (m minimalBackend) ChatCompletion(context.Context, ChatRequest) (*ChatResponse, error) {
	return nil, nil
}
func (m minimalBackend) ChatCompletionStream(context.Context, ChatRequest, StreamFunc) error {
	return nil
}
func (m minimalBackend) ListModels(context.Context) (*ModelsResponse, error) {
	return nil, nil
}
func (m minimalBackend) CreateEmbedding(context.Context, EmbedRequest) (*EmbedResponse, error) {
	return nil, nil
}

var _ = Describe("Registry", func() {
	Describe("NewRegistry", func() {
		It("returns an empty registry", func() {
			reg := NewRegistry()
			Expect(reg).NotTo(BeNil())
			backends := reg.All()
			Expect(backends).To(BeEmpty())
		})
	})

	Describe("Register and Get", func() {
		It("registers a backend and returns it as primary when name is empty", func() {
			reg := NewRegistry()
			b := &minimalBackend{name: "mlx"}
			reg.Register(b)

			got, err := reg.Get("")
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(Equal(b))
			Expect(got.Name()).To(Equal("mlx"))
		})

		It("returns backend by name", func() {
			reg := NewRegistry()
			b1 := &minimalBackend{name: "first"}
			b2 := &minimalBackend{name: "second"}
			reg.Register(b1)
			reg.Register(b2)

			got, err := reg.Get("second")
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(Equal(b2))
		})

		It("returns ErrBackendNotFound for unknown name", func() {
			reg := NewRegistry()
			reg.Register(&minimalBackend{name: "mlx"})

			_, err := reg.Get("unknown")
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(ContainSubstring("backend not found")))
		})
	})

	Describe("Primary", func() {
		It("returns the first registered backend", func() {
			reg := NewRegistry()
			b := &minimalBackend{name: "primary"}
			reg.Register(b)

			got, err := reg.Primary()
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(Equal(b))
		})

		It("returns error when no backends registered", func() {
			reg := NewRegistry()
			_, err := reg.Primary()
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("All", func() {
		It("returns all registered backends", func() {
			reg := NewRegistry()
			b1 := &minimalBackend{name: "a"}
			b2 := &minimalBackend{name: "b"}
			reg.Register(b1)
			reg.Register(b2)

			all := reg.All()
			Expect(all).To(HaveLen(2))
			names := []string{all[0].Name(), all[1].Name()}
			Expect(names).To(ConsistOf("a", "b"))
		})
	})
})

package backend

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type stubHealthChecker struct {
	healthy map[string]bool
}

func (s stubHealthChecker) IsHealthy(name string) bool {
	if s.healthy == nil {
		return true
	}
	return s.healthy[name]
}

var _ = Describe("PrimaryHealthy", func() {
	It("returns primary when healthy", func() {
		reg := NewRegistry()
		primary := &minimalBackend{name: "ollama"}
		reg.Register(primary)

		got, err := reg.PrimaryHealthy(nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(Equal(primary))
	})

	It("skips degraded primary and returns another healthy backend", func() {
		reg := NewRegistry()
		primary := &minimalBackend{name: "ollama"}
		fallback := &minimalBackend{name: "mlx"}
		reg.Register(primary)
		reg.Register(fallback)

		hc := stubHealthChecker{healthy: map[string]bool{"ollama": false, "mlx": true}}
		got, err := reg.PrimaryHealthy(hc)
		Expect(err).NotTo(HaveOccurred())
		Expect(got.Name()).To(Equal("mlx"))
	})

	It("returns ErrNoHealthyBackend when all are degraded", func() {
		reg := NewRegistry()
		reg.Register(&minimalBackend{name: "ollama"})
		hc := stubHealthChecker{healthy: map[string]bool{"ollama": false}}

		_, err := reg.PrimaryHealthy(hc)
		Expect(err).To(MatchError(ErrNoHealthyBackend))
	})

	It("load-balances across multiple healthy backends", func() {
		reg := NewRegistry()
		reg.Register(&minimalBackend{name: "ollama"})
		reg.Register(&minimalBackend{name: "mlx"})

		b1, err := reg.PrimaryHealthy(nil)
		Expect(err).NotTo(HaveOccurred())
		// Keep b1 in-flight.

		b2, err := reg.PrimaryHealthy(nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(b2.Name()).NotTo(Equal(b1.Name()))

		reg.ReleaseBackend(b1.Name())
		reg.ReleaseBackend(b2.Name())
	})
})

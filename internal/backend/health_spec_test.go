package backend

import (
	"context"
	"errors"

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

var _ = Describe("CheckBackendHealth", func() {
	It("returns degraded when the watchdog marks the backend unhealthy", func() {
		hc := stubHealthChecker{healthy: map[string]bool{"ollama": false}}
		err := CheckBackendHealth(context.Background(), hc, "ollama", func(context.Context) error {
			return errors.New("live probe should not run")
		})
		Expect(err).To(MatchError(ContainSubstring("degraded")))
		Expect(errors.Is(err, ErrDegraded)).To(BeTrue())
	})

	It("runs the live probe when the watchdog reports healthy", func() {
		called := false
		hc := stubHealthChecker{healthy: map[string]bool{"ollama": true}}
		err := CheckBackendHealth(context.Background(), hc, "ollama", func(context.Context) error {
			called = true
			return nil
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(called).To(BeTrue())
	})

	It("skips the watchdog when hc is nil", func() {
		called := false
		err := CheckBackendHealth(context.Background(), nil, "ollama", func(context.Context) error {
			called = true
			return nil
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(called).To(BeTrue())
	})
})

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
})

var _ = Describe("HealthyBackends", func() {
	It("returns only backends marked healthy by the checker", func() {
		reg := NewRegistry()
		ollama := &minimalBackend{name: "ollama"}
		mlx := &minimalBackend{name: "mlx"}
		reg.Register(ollama)
		reg.Register(mlx)

		hc := stubHealthChecker{healthy: map[string]bool{"ollama": false, "mlx": true}}
		healthy := reg.HealthyBackends(hc)
		Expect(healthy).To(HaveLen(1))
		Expect(healthy[0].Name()).To(Equal("mlx"))
	})
})

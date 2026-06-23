package backend

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("LoadBalancer", func() {
	It("round-robins among backends with equal load", func() {
		lb := NewLoadBalancer()
		names := []string{"a", "b", "c"}

		Expect(lb.Select(names)).To(Equal("a"))
		Expect(lb.Select(names)).To(Equal("b"))
		Expect(lb.Select(names)).To(Equal("c"))
		Expect(lb.Select(names)).To(Equal("a"))
	})

	It("prefers the backend with fewer in-flight requests", func() {
		lb := NewLoadBalancer()
		lb.Acquire("a")
		lb.Acquire("a")
		lb.Acquire("b")

		Expect(lb.Select([]string{"a", "b"})).To(Equal("b"))
	})

	It("tracks acquire and release", func() {
		lb := NewLoadBalancer()
		lb.Acquire("x")
		lb.Acquire("x")
		Expect(lb.InFlight("x")).To(Equal(2))

		lb.Release("x")
		Expect(lb.InFlight("x")).To(Equal(1))

		lb.Release("x")
		Expect(lb.InFlight("x")).To(Equal(0))
	})

	It("does not underflow on release", func() {
		lb := NewLoadBalancer()
		lb.Release("missing")
		Expect(lb.InFlight("missing")).To(Equal(0))
	})
})

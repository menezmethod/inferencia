package apierror

import (
	"context"
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("FromBackendError", func() {
	It("maps context deadline exceeded to backend_timeout", func() {
		e := FromBackendError("ollama", context.DeadlineExceeded)
		Expect(e.Status).To(Equal(504))
		Expect(e.Code).To(Equal("backend_timeout"))
	})

	It("maps client timeout strings to backend_timeout", func() {
		err := errors.New(`ollama chat completion: Post "http://localhost:11434/v1/chat/completions": context deadline exceeded`)
		e := FromBackendError("ollama", err)
		Expect(e.Status).To(Equal(504))
		Expect(e.Code).To(Equal("backend_timeout"))
	})

	It("maps connection refused to backend_unavailable", func() {
		err := errors.New(`ollama chat completion: Post "http://localhost:11434/v1/chat/completions": dial tcp 127.0.0.1:11434: connect: connection refused`)
		e := FromBackendError("ollama", err)
		Expect(e.Status).To(Equal(503))
		Expect(e.Code).To(Equal("backend_unavailable"))
	})

	It("maps upstream 429 to backend_overloaded", func() {
		err := fmt.Errorf("ollama chat completion: status 429: server busy")
		e := FromBackendError("ollama", err)
		Expect(e.Status).To(Equal(503))
		Expect(e.Code).To(Equal("backend_overloaded"))
	})

	It("maps upstream 503 to backend_overloaded", func() {
		err := fmt.Errorf("ollama chat completion: status 503: overloaded")
		e := FromBackendError("ollama", err)
		Expect(e.Status).To(Equal(503))
		Expect(e.Code).To(Equal("backend_overloaded"))
	})

	It("maps upstream 504 to backend_timeout", func() {
		err := fmt.Errorf("ollama chat completion: status 504: gateway timeout")
		e := FromBackendError("ollama", err)
		Expect(e.Status).To(Equal(504))
		Expect(e.Code).To(Equal("backend_timeout"))
	})
})

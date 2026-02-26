package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Chain", func() {
	It("applies middleware in order: first is outermost", func() {
		var order []string
		inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "handler")
			w.WriteHeader(http.StatusOK)
		})
		mwA := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, "A-in")
				next.ServeHTTP(w, r)
				order = append(order, "A-out")
			})
		}
		mwB := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, "B-in")
				next.ServeHTTP(w, r)
				order = append(order, "B-out")
			})
		}

		chained := Chain(inner, mwA, mwB)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		chained.ServeHTTP(rec, req)

		Expect(rec.Code).To(Equal(http.StatusOK))
		// First in list is outermost: request order A → B → handler; response order handler → B → A
		Expect(strings.Join(order, " ")).To(Equal("A-in B-in handler B-out A-out"))
	})

	It("works with no middleware", func() {
		inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})
		chained := Chain(inner)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		chained.ServeHTTP(rec, req)
		Expect(rec.Code).To(Equal(http.StatusNoContent))
	})
})

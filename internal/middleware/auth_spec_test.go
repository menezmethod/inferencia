package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/menezmethod/inferencia/internal/auth"
)

func newTestKeyStore(keys ...string) *auth.KeyStore {
	path := filepath.Join(GinkgoT().TempDir(), "keys.txt")
	content := ""
	for _, k := range keys {
		content += k + "\n"
	}
	Expect(os.WriteFile(path, []byte(content), 0644)).NotTo(HaveOccurred())
	ks, err := auth.NewKeyStore(path)
	Expect(err).NotTo(HaveOccurred())
	return ks
}

var _ = Describe("Auth middleware", func() {
	When("Authorization header is valid Bearer token", func() {
		It("calls next and sets key in context", func() {
			ks := newTestKeyStore("sk-valid")
			called := false
			handler := Auth(ks)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				key := APIKeyFromContext(r.Context())
				Expect(key).To(Equal("sk-valid"))
				called = true
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
			req.Header.Set("Authorization", "Bearer sk-valid")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusOK))
			Expect(called).To(BeTrue())
		})
	})

	When("Authorization header is invalid key", func() {
		It("returns 401", func() {
			ks := newTestKeyStore("sk-valid")
			handler := Auth(ks)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
			req.Header.Set("Authorization", "Bearer sk-wrong")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusUnauthorized))
		})
	})

	When("Authorization header is missing", func() {
		It("returns 401", func() {
			ks := newTestKeyStore("sk-valid")
			handler := Auth(ks)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusUnauthorized))
		})
	})

	When("Authorization header is malformed (not Bearer)", func() {
		It("returns 401", func() {
			ks := newTestKeyStore("sk-valid")
			handler := Auth(ks)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
			req.Header.Set("Authorization", "Token sk-valid")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusUnauthorized))
		})
	})

	When("Authorization header is Bearer with empty token", func() {
		It("returns 401", func() {
			ks := newTestKeyStore("sk-valid")
			handler := Auth(ks)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
			req.Header.Set("Authorization", "Bearer ")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusUnauthorized))
		})
	})

	Describe("APIKeyFromContext", func() {
		It("returns empty string when key not in context", func() {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			key := APIKeyFromContext(req.Context())
			Expect(key).To(BeEmpty())
		})
	})
})

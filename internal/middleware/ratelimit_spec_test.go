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

func newKeyStore(keys ...string) *auth.KeyStore {
	path := filepath.Join(GinkgoT().TempDir(), "keys.txt")
	var content string
	for _, k := range keys {
		content += k + "\n"
	}
	Expect(os.WriteFile(path, []byte(content), 0644)).NotTo(HaveOccurred())
	ks, err := auth.NewKeyStore(path)
	Expect(err).NotTo(HaveOccurred())
	return ks
}

var _ = Describe("RateLimiter", func() {
	Describe("Allow", func() {
		It("allows requests up to burst size then denies", func() {
			rl := NewRateLimiter(10, 5) // 10 rps, burst 5

			for i := 0; i < 5; i++ {
				remaining, ok := rl.Allow("key-1")
				Expect(ok).To(BeTrue(), "request %d should be allowed", i+1)
				Expect(remaining).To(Equal(5 - i - 1))
			}

			_, ok := rl.Allow("key-1")
			Expect(ok).To(BeFalse(), "6th request should be denied after burst exhausted")
		})

		It("tracks keys independently", func() {
			rl := NewRateLimiter(10, 2)

			rl.Allow("key-1")
			rl.Allow("key-1")
			_, ok := rl.Allow("key-1")
			Expect(ok).To(BeFalse())

			remaining, ok := rl.Allow("key-2")
			Expect(ok).To(BeTrue())
			Expect(remaining).To(Equal(1))
		})

		It("gives new keys full burst", func() {
			rl := NewRateLimiter(1, 3)

			remaining, ok := rl.Allow("fresh-key")
			Expect(ok).To(BeTrue())
			Expect(remaining).To(Equal(2))
		})
	})
})

var _ = Describe("RateLimit middleware", func() {
	When("no API key in context", func() {
		It("calls next handler (pass-through)", func() {
			rl := NewRateLimiter(10, 5)
			called := false
			handler := RateLimit(rl)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			}))
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			Expect(called).To(BeTrue())
			Expect(rec.Code).To(Equal(http.StatusOK))
		})
	})

	When("key exceeds rate limit", func() {
		It("returns 429 with X-RateLimit headers and Retry-After", func() {
			ks := newKeyStore("sk-ratekey")
			rl := NewRateLimiter(10, 1) // burst 1
			handler := Chain(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }),
				Auth(ks),
				RateLimit(rl),
			)
			req1 := httptest.NewRequest(http.MethodGet, "/", nil)
			req1.Header.Set("Authorization", "Bearer sk-ratekey")
			rec1 := httptest.NewRecorder()
			handler.ServeHTTP(rec1, req1)
			Expect(rec1.Code).To(Equal(http.StatusOK))

			req2 := httptest.NewRequest(http.MethodGet, "/", nil)
			req2.Header.Set("Authorization", "Bearer sk-ratekey")
			rec2 := httptest.NewRecorder()
			handler.ServeHTTP(rec2, req2)
			Expect(rec2.Code).To(Equal(http.StatusTooManyRequests))
			Expect(rec2.Header().Get("X-RateLimit-Limit")).To(Equal("1"))
			Expect(rec2.Header().Get("X-RateLimit-Remaining")).To(Equal("0"))
			Expect(rec2.Header().Get("Retry-After")).To(Equal("1"))
		})
	})
})

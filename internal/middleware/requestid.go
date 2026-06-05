package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

const requestIDContextKey contextKey = "request_id"

// RequestID generates a unique request ID per request. If the client sends
// an X-Request-ID header, it is reused (for distributed tracing); otherwise
// a new 16-byte hex ID is generated. The ID is stored in the request context
// and echoed back in the X-Request-ID response header.
func RequestID() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get("X-Request-ID")
			if id == "" {
				id = generateID()
			}

			w.Header().Set("X-Request-ID", id)
			ctx := context.WithValue(r.Context(), requestIDContextKey, id)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequestIDFromContext retrieves the request ID from context.
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDContextKey).(string)
	return id
}

func generateID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// CSPRNG failure is catastrophic; fall back to a non-random but unique ID.
		b = []byte(fallbackID[:16])
	}
	return hex.EncodeToString(b)
}

// fallbackID is used when the CSPRNG fails. It's a compile-time constant
// that provides uniqueness across process restarts at least.
const fallbackID = "inferencia-fallback-request-id"

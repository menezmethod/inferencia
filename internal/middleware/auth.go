package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/menez/inferencia/internal/apierror"
	"github.com/menez/inferencia/internal/auth"
)

// contextKey is an unexported type for context keys in this package.
type contextKey string

const apiKeyContextKey contextKey = "api_key"

// Auth returns middleware that validates Bearer tokens against the KeyStore.
// Requests without a valid token receive a 401 response in OpenAI error format.
func Auth(ks *auth.KeyStore) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key, ok := extractBearerToken(r)
			if !ok {
				apierror.Write(w, apierror.Unauthorized("Missing or malformed Authorization header. Expected: Bearer <api_key>"))
				return
			}

			if err := ks.Validate(key); err != nil {
				apierror.Write(w, apierror.Unauthorized("Invalid API key."))
				return
			}

			// Store the key in context for downstream use (rate limiting, logging).
			ctx := context.WithValue(r.Context(), apiKeyContextKey, key)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// APIKeyFromContext retrieves the authenticated API key from the request context.
func APIKeyFromContext(ctx context.Context) string {
	key, _ := ctx.Value(apiKeyContextKey).(string)
	return key
}

// extractBearerToken parses the Authorization header for a Bearer token.
func extractBearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return "", false
	}

	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return "", false
	}

	token := strings.TrimSpace(h[len(prefix):])
	if token == "" {
		return "", false
	}

	return token, true
}

package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/menezmethod/inferencia/internal/auth"
)

func newTestKeyStore(t *testing.T, keys ...string) *auth.KeyStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "keys.txt")
	content := ""
	for _, k := range keys {
		content += k + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	ks, err := auth.NewKeyStore(path)
	if err != nil {
		t.Fatal(err)
	}
	return ks
}

func TestAuthMiddleware(t *testing.T) {
	ks := newTestKeyStore(t, "sk-valid")

	handler := Auth(ks)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify key is in context.
		key := APIKeyFromContext(r.Context())
		if key != "sk-valid" {
			t.Errorf("APIKeyFromContext = %q, want sk-valid", key)
		}
		w.WriteHeader(http.StatusOK)
	}))

	tests := []struct {
		name       string
		authHeader string
		wantStatus int
	}{
		{"valid key", "Bearer sk-valid", http.StatusOK},
		{"invalid key", "Bearer sk-wrong", http.StatusUnauthorized},
		{"missing header", "", http.StatusUnauthorized},
		{"malformed header", "Token sk-valid", http.StatusUnauthorized},
		{"empty bearer", "Bearer ", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

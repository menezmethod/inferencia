package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/menezmethod/inferencia/internal/apierror"
)

// Recover returns middleware that catches panics, logs the stack trace,
// and returns a 500 error in OpenAI format instead of crashing the server.
func Recover(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					logger.Error("panic recovered",
						"error", err,
						"stack", string(debug.Stack()),
						"method", r.Method,
						"path", r.URL.Path,
					)
					apierror.Write(w, apierror.Internal("Internal server error."))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

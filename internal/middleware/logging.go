package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

// Logging emits a single canonical log line per request after the response
// completes. This is the Stripe-style "canonical log line" pattern: one
// structured log event that captures everything needed for debugging,
// alerting, and analytics.
//
// Fields emitted:
//
//	request_id  — unique per-request (from RequestID middleware)
//	method      — HTTP method
//	path        — request path
//	status      — HTTP status code
//	duration_ms — wall-clock milliseconds
//	bytes       — response body bytes written
//	remote_addr — client IP (may be proxy IP behind tunnel)
//	user_agent  — client User-Agent
//	api_key     — last 8 chars of the authenticated key (safe to log)
//
// When used with JSON format + Loki/Promtail, every field is indexed
// and queryable: {job="inferencia"} | json | status >= 500
func Logging(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(sw, r)

			attrs := []slog.Attr{
				slog.String("request_id", RequestIDFromContext(r.Context())),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", sw.status),
				slog.Int64("duration_ms", time.Since(start).Milliseconds()),
				slog.Int("bytes", sw.bytes),
				slog.String("remote_addr", r.RemoteAddr),
				slog.String("user_agent", r.UserAgent()),
			}

			if key := APIKeyFromContext(r.Context()); key != "" {
				attrs = append(attrs, slog.String("api_key", maskKey(key)))
			}

			level := slog.LevelInfo
			if sw.status >= 500 {
				level = slog.LevelError
			} else if sw.status >= 400 {
				level = slog.LevelWarn
			}

			logger.LogAttrs(r.Context(), level, "request", attrs...)
		})
	}
}

// maskKey returns the last 8 characters of an API key prefixed with "...".
func maskKey(key string) string {
	if len(key) <= 8 {
		return "***"
	}
	return "..." + key[len(key)-8:]
}

// statusWriter wraps http.ResponseWriter to capture the status code and bytes written.
type statusWriter struct {
	http.ResponseWriter
	status      int
	bytes       int
	wroteHeader bool
}

// WriteHeader captures the status code before delegating to the underlying writer.
func (sw *statusWriter) WriteHeader(code int) {
	if !sw.wroteHeader {
		sw.status = code
		sw.wroteHeader = true
	}
	sw.ResponseWriter.WriteHeader(code)
}

// Write captures bytes written.
func (sw *statusWriter) Write(b []byte) (int, error) {
	n, err := sw.ResponseWriter.Write(b)
	sw.bytes += n
	return n, err
}

// Flush implements http.Flusher for streaming support.
func (sw *statusWriter) Flush() {
	if f, ok := sw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

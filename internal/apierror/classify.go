package apierror

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
)

// BackendTimeout returns 504 when the backend did not respond in time.
func BackendTimeout(backend string) *Error {
	return &Error{
		Status:  504,
		Message: "Backend " + backend + " timed out.",
		Type:    TypeBackendDown,
		Code:    "backend_timeout",
	}
}

// BackendOverloaded returns 503 when the backend is busy or rate-limited.
func BackendOverloaded(backend string) *Error {
	return &Error{
		Status:  503,
		Message: "Backend " + backend + " is overloaded. Please retry shortly.",
		Type:    TypeBackendDown,
		Code:    "backend_overloaded",
	}
}

// FromBackendError maps a backend transport or upstream error to an OpenAI-compatible API error.
func FromBackendError(backend string, err error) *Error {
	if err == nil {
		return nil
	}

	if errors.Is(err, context.DeadlineExceeded) || isClientTimeout(err) {
		return BackendTimeout(backend)
	}

	if isConnectionUnavailable(err) {
		return BackendUnavailable(backend)
	}

	if status, ok := upstreamHTTPStatus(err); ok {
		switch status {
		case 429, 502, 503:
			return BackendOverloaded(backend)
		case 504:
			return BackendTimeout(backend)
		}
	}

	if isOverloadMessage(err.Error()) {
		return BackendOverloaded(backend)
	}

	return BackendUnavailable(backend)
}

func isClientTimeout(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "client.timeout exceeded") ||
		strings.Contains(msg, "context deadline exceeded") ||
		strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "timeout awaiting response headers")
}

func isConnectionUnavailable(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return false
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "network is unreachable") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "dial tcp")
}

func upstreamHTTPStatus(err error) (int, bool) {
	msg := err.Error()
	idx := strings.Index(msg, "status ")
	if idx < 0 {
		return 0, false
	}
	rest := msg[idx+len("status "):]
	end := strings.IndexAny(rest, ": ")
	if end < 0 {
		end = len(rest)
	}
	code, parseErr := strconv.Atoi(strings.TrimSpace(rest[:end]))
	if parseErr != nil {
		return 0, false
	}
	return code, true
}

func isOverloadMessage(msg string) bool {
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "server busy") ||
		strings.Contains(lower, "too many requests") ||
		strings.Contains(lower, "overloaded") ||
		strings.Contains(lower, "rate limit")
}

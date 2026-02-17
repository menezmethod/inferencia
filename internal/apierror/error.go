// Package apierror provides OpenAI-compatible error types and HTTP responses.
//
// All errors returned by the API conform to the OpenAI error response format,
// ensuring compatibility with existing clients and SDKs.
package apierror

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// Type constants follow the OpenAI error type taxonomy.
const (
	TypeInvalidRequest  = "invalid_request_error"
	TypeAuthentication  = "authentication_error"
	TypeRateLimit       = "rate_limit_error"
	TypeServer          = "server_error"
	TypeBackendDown     = "backend_error"
)

// Error represents an OpenAI-compatible API error.
type Error struct {
	Status  int    `json:"-"`
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code,omitempty"`
	Param   string `json:"param,omitempty"`
}

// Error implements the error interface.
func (e *Error) Error() string {
	return e.Message
}

// response wraps an Error in the OpenAI envelope format.
type response struct {
	Error *Error `json:"error"`
}

// Write sends an Error as a JSON HTTP response.
func Write(w http.ResponseWriter, err *Error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(err.Status)

	if encErr := json.NewEncoder(w).Encode(response{Error: err}); encErr != nil {
		slog.Error("failed to encode error response", "err", encErr)
	}
}

// InvalidRequest returns a 400 error for malformed requests.
func InvalidRequest(msg string) *Error {
	return &Error{
		Status:  http.StatusBadRequest,
		Message: msg,
		Type:    TypeInvalidRequest,
	}
}

// InvalidParam returns a 400 error for a specific invalid parameter.
func InvalidParam(param, msg string) *Error {
	return &Error{
		Status:  http.StatusBadRequest,
		Message: msg,
		Type:    TypeInvalidRequest,
		Param:   param,
	}
}

// Unauthorized returns a 401 error for authentication failures.
func Unauthorized(msg string) *Error {
	return &Error{
		Status:  http.StatusUnauthorized,
		Message: msg,
		Type:    TypeAuthentication,
		Code:    "invalid_api_key",
	}
}

// RateLimited returns a 429 error when rate limits are exceeded.
func RateLimited() *Error {
	return &Error{
		Status:  http.StatusTooManyRequests,
		Message: "Rate limit exceeded. Please retry after a brief wait.",
		Type:    TypeRateLimit,
		Code:    "rate_limit_exceeded",
	}
}

// BackendUnavailable returns a 503 error when the LLM backend is unreachable.
func BackendUnavailable(backend string) *Error {
	return &Error{
		Status:  http.StatusServiceUnavailable,
		Message: "Backend " + backend + " is currently unavailable.",
		Type:    TypeBackendDown,
		Code:    "backend_unavailable",
	}
}

// Internal returns a 500 error for unexpected server failures.
func Internal(msg string) *Error {
	return &Error{
		Status:  http.StatusInternalServerError,
		Message: msg,
		Type:    TypeServer,
	}
}

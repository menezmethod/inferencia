// Package middleware provides HTTP middleware for authentication,
// rate limiting, logging, and panic recovery.
package middleware

import "net/http"

// Middleware wraps an http.Handler with additional behavior.
type Middleware func(http.Handler) http.Handler

// Chain composes middleware in the order given. The first middleware
// in the list is the outermost (runs first on request, last on response).
//
//	chain(handler, logging, auth, ratelimit)
//	// Request order:  logging → auth → ratelimit → handler
//	// Response order: handler → ratelimit → auth → logging
func Chain(h http.Handler, mw ...Middleware) http.Handler {
	for i := len(mw) - 1; i >= 0; i-- {
		h = mw[i](h)
	}
	return h
}

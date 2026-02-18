package middleware

import (
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/menez/inferencia/internal/apierror"
)

// RateLimiter implements a per-key token bucket rate limiter.
type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rate    float64 // tokens per second
	burst   int     // maximum tokens
}

type bucket struct {
	tokens   float64
	lastSeen time.Time
}

// NewRateLimiter creates a RateLimiter with the given refill rate and burst size.
func NewRateLimiter(rps float64, burst int) *RateLimiter {
	rl := &RateLimiter{
		buckets: make(map[string]*bucket),
		rate:    rps,
		burst:   burst,
	}
	go rl.cleanup()
	return rl
}

// RateLimit returns middleware that enforces per-key rate limits.
// It expects the API key to be in the request context (set by Auth middleware).
func RateLimit(rl *RateLimiter) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := APIKeyFromContext(r.Context())
			if key == "" {
				// No key in context — skip rate limiting (shouldn't happen
				// if auth middleware runs first, but be defensive).
				next.ServeHTTP(w, r)
				return
			}

			remaining, ok := rl.Allow(key)
			if !ok {
				RateLimitRejections.Inc()
				w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rl.burst))
				w.Header().Set("X-RateLimit-Remaining", "0")
				w.Header().Set("Retry-After", "1")
				apierror.Write(w, apierror.RateLimited())
				return
			}

			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rl.burst))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))

			next.ServeHTTP(w, r)
		})
	}
}

// Allow checks whether the key has tokens available and consumes one if so.
// It returns the remaining token count and whether the request is allowed.
func (rl *RateLimiter) Allow(key string) (int, bool) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	b, exists := rl.buckets[key]
	if !exists {
		b = &bucket{tokens: float64(rl.burst), lastSeen: now}
		rl.buckets[key] = b
	}

	// Refill tokens based on elapsed time.
	elapsed := now.Sub(b.lastSeen).Seconds()
	b.tokens = math.Min(float64(rl.burst), b.tokens+elapsed*rl.rate)
	b.lastSeen = now

	if b.tokens < 1 {
		return 0, false
	}

	b.tokens--
	remaining := int(b.tokens)
	return remaining, true
}

// cleanup periodically removes stale buckets to prevent unbounded memory growth.
// A bucket is stale if it hasn't been seen in 10 minutes.
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		cutoff := time.Now().Add(-10 * time.Minute)
		for key, b := range rl.buckets {
			if b.lastSeen.Before(cutoff) {
				delete(rl.buckets, key)
			}
		}
		rl.mu.Unlock()
	}
}

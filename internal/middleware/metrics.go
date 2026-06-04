package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	httpRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "inferencia",
		Subsystem: "http",
		Name:      "requests_total",
		Help:      "Total HTTP requests by method, path, and status code.",
	}, []string{"method", "path", "status"})

	httpRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "inferencia",
		Subsystem: "http",
		Name:      "request_duration_seconds",
		Help:      "HTTP request latency in seconds.",
		Buckets:   []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120},
	}, []string{"method", "path"})

	httpRequestsInFlight = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "inferencia",
		Subsystem: "http",
		Name:      "requests_in_flight",
		Help:      "Number of HTTP requests currently being processed.",
	})

	TokensTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "inferencia",
		Name:      "tokens_total",
		Help:      "Total tokens processed by type (prompt, completion).",
	}, []string{"model", "type"})

	BackendHealth = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "inferencia",
		Name:      "backend_healthy",
		Help:      "Whether each backend is healthy (1) or down (0).",
	}, []string{"backend"})

	BackendRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "inferencia",
		Subsystem: "backend",
		Name:      "request_duration_seconds",
		Help:      "Backend request latency in seconds.",
		Buckets:   []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120},
	}, []string{"backend", "operation"})

	RateLimitRejections = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "inferencia",
		Name:      "ratelimit_rejections_total",
		Help:      "Total requests rejected by the rate limiter.",
	})

	TTSRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "inferencia",
		Subsystem: "tts",
		Name:      "requests_total",
		Help:      "Total TTS synthesis requests by backend and status.",
	}, []string{"backend", "status"})

	TTSRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "inferencia",
		Subsystem: "tts",
		Name:      "request_duration_seconds",
		Help:      "TTS synthesis latency in seconds.",
		Buckets:   []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
	}, []string{"backend"})

	TTSCharactersTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "inferencia",
		Subsystem: "tts",
		Name:      "characters_total",
		Help:      "Total characters sent for TTS synthesis.",
	}, []string{"backend"})

	RoutingDecisionsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "inferencia",
		Subsystem: "router",
		Name:      "decisions_total",
		Help:      "Total routing decisions by capability and selected backend.",
	}, []string{"capability", "backend"})
)

// normalizePath maps request paths to metric-safe labels to avoid cardinality explosion.
func normalizePath(path string) string {
	switch path {
	case "/v1/chat/completions":
		return "/v1/chat/completions"
	case "/v1/models":
		return "/v1/models"
	case "/v1/embeddings":
		return "/v1/embeddings"
	case "/v1/audio/speech":
		return "/v1/audio/speech"
	case "/health":
		return "/health"
	case "/health/ready":
		return "/health/ready"
	case "/health/status":
		return "/health/status"
	case "/metrics":
		return "/metrics"
	case "/openapi.yaml":
		return "/openapi.yaml"
	case "/docs":
		return "/docs"
	default:
		return "/other"
	}
}

// Metrics returns middleware that records Prometheus metrics for every request.
func Metrics() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			path := normalizePath(r.URL.Path)

			httpRequestsInFlight.Inc()
			defer httpRequestsInFlight.Dec()

			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r)

			status := strconv.Itoa(sw.status)
			httpRequestsTotal.WithLabelValues(r.Method, path, status).Inc()
			httpRequestDuration.WithLabelValues(r.Method, path).Observe(time.Since(start).Seconds())
		})
	}
}

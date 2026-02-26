// Package observability provides optional OpenTelemetry tracing.
package observability

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.28.0"
)

// TracerProvider holds the SDK TracerProvider for shutdown.
type TracerProvider struct {
	provider *sdktrace.TracerProvider
}

// NewTracerProvider creates and sets a global TracerProvider that exports
// spans via OTLP HTTP to the given endpoint (e.g. http://localhost:4318 or https://otel.example.com).
// TLS is used when the endpoint URL scheme is https; http uses insecure transport (local/dev).
func NewTracerProvider(ctx context.Context, endpoint, serviceName string) (*TracerProvider, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		endpoint = "http://localhost:4318"
	}
	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpointURL(endpoint),
	}
	if u, err := url.Parse(endpoint); err == nil && u.Scheme == "http" {
		opts = append(opts, otlptracehttp.WithInsecure())
	}
	exporter, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		return nil, err
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
		),
	)
	if err != nil {
		return nil, err
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter, sdktrace.WithBatchTimeout(5*time.Second)),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(provider)
	return &TracerProvider{provider: provider}, nil
}

// Shutdown flushes and stops the TracerProvider.
func (tp *TracerProvider) Shutdown(ctx context.Context) error {
	if tp == nil || tp.provider == nil {
		return nil
	}
	return tp.provider.Shutdown(ctx)
}

// HTTPHandler wraps the given handler with OpenTelemetry HTTP tracing.
// Use when observability.otel_enabled is true.
func HTTPHandler(h http.Handler, operation string) http.Handler {
	return otelhttp.NewHandler(h, operation, otelhttp.WithSpanNameFormatter(func(operation string, r *http.Request) string {
		return r.Method + " " + r.URL.Path
	}))
}

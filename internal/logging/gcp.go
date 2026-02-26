// Package logging provides cloud-friendly log handlers (e.g. GCP Cloud Logging).
package logging

import (
	"context"
	"io"
	"log/slog"
)

// severity maps slog.Level to GCP Cloud Logging severity strings.
// https://cloud.google.com/logging/docs/structured-logging#special-payload-fields
var severityByLevel = map[slog.Level]string{
	slog.LevelDebug: "DEBUG",
	slog.LevelInfo:  "INFO",
	slog.LevelWarn:  "WARNING",
	slog.LevelError: "ERROR",
}

// GCPHandler wraps a slog.Handler and adds "severity" (and optionally "resource")
// so that JSON logs are natively parsed by GCP Cloud Logging.
type GCPHandler struct {
	inner    slog.Handler
	addResource bool
}

// NewGCPHandler returns a handler that adds severity to every record.
// If addResource is true, adds a "resource" object with type "generic_task"
// and labels suitable for GCP (service_name, location, etc. can be set via env).
func NewGCPHandler(inner slog.Handler, addResource bool) *GCPHandler {
	return &GCPHandler{inner: inner, addResource: addResource}
}

// Enabled reports whether the inner handler would log this level.
func (h *GCPHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

// Handle adds severity (and optionally resource) then forwards to the inner handler.
func (h *GCPHandler) Handle(ctx context.Context, r slog.Record) error {
	sev := severityByLevel[r.Level]
	if sev == "" {
		sev = "DEFAULT"
	}
	r.AddAttrs(slog.String("severity", sev))
	if h.addResource {
		r.AddAttrs(slog.Any("resource", map[string]any{
			"type": "generic_task",
			"labels": map[string]string{
				"service": "inferencia",
			},
		}))
	}
	return h.inner.Handle(ctx, r)
}

// WithAttrs returns a new handler with the given attributes.
func (h *GCPHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &GCPHandler{
		inner:       h.inner.WithAttrs(attrs),
		addResource: h.addResource,
	}
}

// WithGroup returns a new handler for the given group.
func (h *GCPHandler) WithGroup(name string) slog.Handler {
	return &GCPHandler{
		inner:       h.inner.WithGroup(name),
		addResource: h.addResource,
	}
}

// NewLogger returns a *slog.Logger configured for the given format and cloud mode.
// Cloud mode: "" (none), "gcp" (add severity), "gcp_with_resource" (severity + resource).
func NewLogger(w io.Writer, level slog.Level, format string, cloudFormat string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: level}
	var base slog.Handler
	if format == "text" {
		base = slog.NewTextHandler(w, opts)
	} else {
		base = slog.NewJSONHandler(w, opts)
	}
	if cloudFormat == "gcp" {
		base = NewGCPHandler(base, false)
	} else if cloudFormat == "gcp_with_resource" {
		base = NewGCPHandler(base, true)
	}
	return slog.New(base)
}

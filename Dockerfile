# inferencia — Coolify-ready, production-style image
# Multi-stage build: build on Go Alpine, run on minimal Alpine with non-root user and healthcheck.

# ------------------------------------------------------------------------------
# Build stage
# ------------------------------------------------------------------------------
FROM golang:1.26-alpine AS builder
WORKDIR /app

# Dependencies first (better layer cache)
COPY go.mod go.sum ./
RUN go mod download

# Source and build (no CGO, stripped, reproducible). VERSION set in CI/release (e.g. v1.0.0).
ARG VERSION=dev
COPY . .
RUN cp docs/openapi.yaml internal/openapi/spec.yaml
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w -X github.com/menezmethod/inferencia/internal/version.Version=${VERSION}" \
    -o /inferencia \
    ./cmd/inferencia

# ------------------------------------------------------------------------------
# Runtime stage
# ------------------------------------------------------------------------------
FROM alpine:3.19

# OCI image metadata (registries, Coolify, inspect)
LABEL org.opencontainers.image.title="inferencia" \
      org.opencontainers.image.description="OpenAI-compatible API gateway for local LLM servers" \
      org.opencontainers.image.source="https://github.com/menezmethod/inferencia"

# Only what's needed for TLS outbound (e.g. HTTPS backends); healthcheck uses busybox wget
RUN apk --no-cache add ca-certificates

# Non-root user with fixed UID/GID (auditable, predictable for volumes)
RUN addgroup -g 1000 app \
 && adduser -u 1000 -G app -H -s /sbin/nologin -D app

# Single binary, no config in image (config via INFERENCIA_* env)
COPY --from=builder /inferencia /app/inferencia
RUN chown -R app:app /app

WORKDIR /app
USER app

EXPOSE 8080

# Orchestrators can use this; Coolify also supports HTTP health checks
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -q -O- http://127.0.0.1:8080/health || exit 1

ENTRYPOINT ["/app/inferencia"]
# No config file: app uses defaults + env. Set INFERENCIA_HOST=0.0.0.0 and INFERENCIA_PORT=8080.

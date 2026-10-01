# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.1.0] - 2026-10-01

### Added

- Task-aware routing: virtual role models (`inferencia-planner`, `inferencia-worker`, `inferencia-fast`, `inferencia-auto`) resolve to a backend + upstream model, advertised in `GET /v1/models` as `backend/model` IDs that round-trip through chat.
- Command Code hosted provider (`type: commandcode`) with `INFERENCIA_COMMANDCODE_KEY` / `INFERENCIA_COMMANDCODE_URL`.
- System One decision engines (TypeSafe Jev, Laya) over the shared `/v1/systemone` protocol: relay endpoint `POST /v1/systemone` and `decisions:` config.
- Decision-powered auto routing (`routing.auto`): the decision engine picks the role; policies `auto`, `auto:quality`, `auto:balanced`, `auto:economy`, `auto:free` with confidence gating, local-only enforcement, and machine-readable reason codes.
- Response headers for routing decisions: `X-Routed-Backend`, `X-Routed-Model`, `X-Routed-Role`, `X-Routed-Fallback`, `X-Routed-Auto`, `X-Routed-Reason`.
- Embedding-capable backend selection; `GET /v1/models` aggregates role models, decision engines, and every healthy backend.
- `docs/ADAPTIVE_RUNTIME.md` — north-star specification and status map.

### Changed

- MLX backends run on the shared OpenAI-compatible adapter; the Ollama health probe falls back to `/v1/models` when native endpoints are absent.
- Backend selection is deterministically ordered for load balancing.
- Unknown models fall back to `routing.default_role` (or the primary healthy backend) instead of failing.

### Fixed

- Advertised model IDs resolve back to their backend (no more silent fallback for namespaced IDs).
- Embeddings skip chat-only backends instead of probing them with a live request.
- `GET /v1/models` no longer fails when a single backend is down.
- A Command Code backend without an API key is skipped with a warning instead of stopping the gateway.

### Removed

- Dead code across router, middleware, auth, observability, watchdog, and server; duplicated HTTP/SSE logic across backend adapters.

## [1.0.0] - 2025-01-20

### Added

- Initial AI gateway API for Coolify deployment
- OpenAI-compatible `/v1/chat/completions` endpoint
- Ollama backend integration (default) with Qwen 3 BF16 fallback
- LLaMA.cpp backend support
- Kokoro TTS backend support
- Smart router — per-kind backend interfaces with fallback chains
- Docker Compose with Coolify-ready env substitution
- OpenAPI 3.1 spec with Swagger UI
- Prometheus metrics with Grafana dashboards and Alertmanager alerts
- Structured logging with request IDs and Loki log aggregation
- OpenTelemetry tracing (OTLP HTTP exporter)
- Comprehensive health check with model/voice inventory
- Version endpoint
- Watchdog goroutine for backend health monitoring
- CI/CD — GitHub Actions, BDD tests, sensitive-data scanning (gitleaks)

### Changed

- Switched default backend from LLaMA.cpp to Ollama (Qwen 3.6:35b-a3b-coding-bf16)
- Enhanced health endpoint with per-service breakdown and TTS checks
- Upgraded Go module path to github.com/menezmethod
- Updated CI to remove golangci-lint (targets older Go version)

### Fixed

- Kokoro voice default incorrectly applied to non-kokoro TTS backends (#28)
- Backend error classification — split health vs. inference timeouts (#31)
- Sensitive data detection in CI/CD pipeline

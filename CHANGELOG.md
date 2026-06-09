# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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

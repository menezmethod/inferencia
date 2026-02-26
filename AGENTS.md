# inferencia — Agent context

This file gives AI agents (Cursor, Codex, etc.) accurate context about the app. Use it to stay consistent with the codebase and API. **Use only placeholders in examples: `https://your-inferencia.example.com`, `sk-your-key`, `192.168.0.x`. Never commit real prod URLs or API keys.**

## What inferencia is

- **OpenAI-compatible API gateway** for local LLM servers. It proxies chat completions, models, and embeddings to backends (MLX; Ollama stubbed) and adds auth, rate limiting, and observability.
- **Stack**: Go 1.26, stdlib `net/http`, `slog`, Prometheus client, optional OpenTelemetry. One main external dep: `gopkg.in/yaml.v3`.
- **Default chat model**: `mlx-community/gpt-oss-20b-MXFP4-Q8` (20B). Use `mlx-community/gpt-oss-120b-MXFP4-Q8` when the client requests the 120B model.

## API surface (match OpenAPI and handlers)

**No auth:**
- `GET /health` — Liveness. JSON: `{"status":"ok","version":"..."}`.
- `GET /health/ready` — Readiness; 200 if all backends healthy, 503 with `backend` and `error` otherwise. JSON includes `version`.
- `GET /version` — JSON: `{"version":"1.0.0"}` (and optional `commit`). Set at build via ldflags.
- `GET /metrics` — Prometheus exposition.
- `GET /docs` — Swagger UI (HTML).
- `GET /openapi.yaml` — OpenAPI 3.1 spec.

**Bearer auth required:**
- `POST /v1/chat/completions` — Chat (streaming and non-streaming, tool calling). Default model 20B; request 120B by model id.
- `GET /v1/models` — List models from primary backend.
- `POST /v1/embeddings` — Embeddings.

Auth: `Authorization: Bearer <key>`. Keys from file or `INFERENCIA_API_KEYS` env. No key → 401.

## Config and env

- Config: YAML (optional) + env overrides. Prefix `INFERENCIA_`: e.g. `INFERENCIA_PORT`, `INFERENCIA_BACKEND_URL`, `INFERENCIA_API_KEYS`, `INFERENCIA_LOG_LEVEL`, `INFERENCIA_OTEL_ENABLED`, `INFERENCIA_OTEL_ENDPOINT`.
- Backend URL: first backend URL can be overridden with `INFERENCIA_BACKEND_URL` (e.g. `http://192.168.0.x:11973` for MLX on LAN).
- No config file in Docker image; use env only (see `env.coolify.example`).

## Repo layout

- `cmd/inferencia` — Main entry; loads config, wires server, graceful shutdown.
- `internal/config` — Load and validate config; env overrides.
- `internal/auth` — KeyStore (file + env); Validate(key).
- `internal/handler` — HTTP handlers (health, ready, version, chat, models, embeddings, docs, OpenAPI).
- `internal/server` — Mux, middleware chain (RequestID, Recover, Metrics, Logging, Auth, RateLimit).
- `internal/backend` — Registry, MLX client (health, chat, stream).
- `internal/middleware` — Rate limit, auth, logging, metrics, recover.
- `internal/openapi` — Embedded spec; copied from `docs/openapi.yaml` at build.
- `internal/version` — Version and Commit (ldflags).
- `docs/openapi.yaml` — Source of truth for API; copied to `internal/openapi/spec.yaml` before build.
- Tests: Ginkgo/Gomega in `internal/handler`, `internal/config`, `internal/auth` (`*_suite_test.go`, `*_spec_test.go`).

## Run and test

- **Local**: `make build` then `./inferencia -config config.yaml` or env only. `make run` uses config.yaml.
- **Docker**: `docker build -t inferencia .` then run with `INFERENCIA_HOST=0.0.0.0`, `INFERENCIA_PORT=8080`, `INFERENCIA_BACKEND_URL`, `INFERENCIA_API_KEYS`.
- **Tests**: `make test` (Ginkgo + race). `make smoke-prod` runs `scripts/smoke-prod.sh` (set `INFERENCIA_SMOKE_BASE_URL` to your deployment, e.g. `https://your-inferencia.example.com`; optional `INFERENCIA_E2E_API_KEY`).
- **Lint**: `make lint` (golangci-lint); CI runs Lint, Build & test, Integration (Docker smoke), Sensitive data (blocklist + gitleaks).

## Sensitive data and docs

- **Blocklist** (`scripts/check-sensitive-data.sh`): Patterns from SENSITIVE_BLOCKLIST (secret/env); if unset, skipped. On match, pattern/content masked in logs. CI fails if blocklisted strings (e.g. real prod hostnames) appear anywhere in the repo.
- **Gitleaks** (`.gitleaks.toml`): CI runs gitleaks with allowlisted example files and placeholder patterns. Keep placeholders in docs: `your-inferencia.example.com`, `sk-your-key`, `192.168.0.x`.
- In **AGENTS.md**, **README**, and **docs**: never use real production URLs, real API keys, or real internal hostnames. Use the placeholders above.

## References
pu
- **OpenAPI**: `docs/openapi.yaml` (and `/openapi.yaml` at runtime).
- **Client setup**: `docs/AGENT_ONBOARDING.md`.
- **Metrics/logging**: `docs/METRICS_AND_LOGGING.md`.
- **Publishing and releases**: `docs/PUBLISHING.md`.

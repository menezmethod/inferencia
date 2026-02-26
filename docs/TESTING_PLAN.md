# Testing plan

This doc outlines the testing strategy for inferencia so the app stays reliable in production. CI runs on every push and PR; tests must pass before merging.

---

## 1. Current state

### 1.1 Prod smoke (manual)

Set `INFERENCIA_SMOKE_BASE_URL` to your deployment URL, then run `./scripts/smoke-prod.sh` or `make smoke-prod`. Optional: `INFERENCIA_E2E_API_KEY` to include `/v1/models`.

| Endpoint | Expected |
|----------|----------|
| `GET /health` | 200 `{"status":"ok"}` |
| `GET /health/ready` | 200 `{"status":"ready"}` |
| `GET /metrics` | 200 Prometheus exposition |
| `GET /docs` | 200 Swagger UI |
| `GET /openapi.yaml` | 200 OpenAPI spec |
| `GET /v1/models` (with key) | 200 or 503 (backend down) |

### 1.2 Existing tests and coverage

| Package | Coverage (approx) | Notes |
|---------|-------------------|--------|
| `internal/auth` | ~94% | KeyStore from file/env, validation |
| `internal/config` | ~75% | Load, env overrides, validation |
| `internal/handler` | ~58% | Health, Ready, Models, Chat (JSON + stream); **missing**: Embeddings, OpenAPI, error paths |
| `internal/middleware` | ~32% | Auth tested; **missing**: Logging, Metrics, Recover, RateLimit, Chain |
| `internal/backend` | 0% | No tests; Registry + MLX adapter untested |
| `internal/server` | 0% | Full stack not exercised |
| `internal/observability` | 0% | OTel init not tested |
| `internal/logging` | 0% | GCP handler not tested |
| `internal/apierror` | 0% | Error JSON shape not asserted |

Tests use **Ginkgo** (BDD) and **Gomega** (matchers) for handler, config, and auth; middleware still uses the standard library. Mocks (e.g. `mockBackend` in handler) and `httptest` are used; no real HTTP server or MLX in unit tests.

---

## 2. Goals

- **CI**: Every push/PR runs `go build`, `go test -race`, `go vet`. No merge to `main` if they fail.
- **Coverage**: Raise unit test coverage for critical paths (handlers, auth, config, middleware) toward **≥70%** per package where practical.
- **Integration tests**: One or more tests that start the real `inferencia` server (test config, no OTel), hit public routes (health, ready, metrics, optional `/v1/models` with test key), and shut down. No external MLX.
- **Functional / E2E**: Optional: script or make target that hits **production** (e.g. `https://llm.menezmethod.com`) for health/ready/metrics and, if `INFERENCIA_E2E_API_KEY` is set, `/v1/models` and a minimal chat. Used for post-deploy or scheduled checks, not as a gate for PRs.

---

## 3. Phased plan

### Phase 1 — CI and stability (done)

- [x] Add **GitHub Actions workflow** (`.github/workflows/ci.yml`):
  - Trigger: `push` to any branch, `pull_request` to any branch.
  - Jobs: build, test (with `-race`), vet. Use Go 1.26.
- [x] Add **prod smoke script** `scripts/smoke-prod.sh`: curl health, ready, metrics, docs, openapi (and optionally `/v1/models` if `INFERENCIA_E2E_API_KEY` set). `make smoke-prod` runs it.
- [x] Add **handler tests**: Embeddings (success, empty input, invalid JSON, backend error), OpenAPI and SwaggerUI (200 and body shape).
- [x] Ensure `make test` and `make build` pass locally and in CI.

### Phase 2 — Unit tests and coverage

- [ ] **Handler**
  - Add tests for **Embeddings** (success, empty input, invalid JSON, backend error), mirroring `chat_test.go` / `models_test.go`.
  - Add test for **OpenAPI** handler (GET returns 200 and non-empty spec).
  - Add tests for chat/models/embeddings **error paths** (backend down, invalid params) and response shape (e.g. `apierror` JSON).
- [ ] **Middleware**
  - **Logging**: request/response logged, status reflected in level (e.g. 5xx → error).
  - **Metrics**: after request, counters/histograms updated (e.g. `inferencia_http_requests_total`).
  - **Recover**: handler panic returns 500 and doesn’t crash.
  - **RateLimit**: 429 when over limit; headers present.
  - **Chain**: order of middleware (e.g. RequestID present in context for Logging).
- [ ] **Backend**
  - **Registry**: Register, Get (primary by name and empty name), Primary(), behaviour with no backends.
  - **MLX**: consider table-driven tests with a small fake HTTP server (e.g. `httptest.Server`) that returns fixed `/v1/models` and `/v1/chat/completions` responses; assert request/response mapping and errors on 5xx/timeout.
- [ ] **Config**
  - Increase coverage for edge cases (invalid YAML, invalid env values, observability validation).
- [ ] **apierror**
  - Test that `Write` produces valid JSON and expected status codes for a few error types.
- [ ] **logging (GCP)**
  - Test that `GCPHandler` adds `severity` (and optionally `resource`) to records.
- [ ] **observability**
  - Unit test for TLS vs insecure based on endpoint URL scheme (optional; or cover via integration).

Target: **≥70%** coverage for `handler`, `middleware`, `config`, `auth`, `backend` (registry at minimum).

### Phase 3 — Integration tests

- [ ] **Server integration**
  - Start `server.New` with test config (in-memory keys, mock or no backend), then:
    - `GET /health` → 200.
    - `GET /health/ready` → 503 if no backend, or 200 with a healthy mock.
    - `GET /metrics` → 200 and body contains `inferencia_`.
    - `GET /v1/models` with valid Bearer → 200 or 503 depending on backend.
  - Use `httptest.Server` or a real listener on `127.0.0.1:0`; no external services.
- [ ] Optional: **with real MLX** (behind env flag, e.g. `INTEGRATION_MLX=1`): start inferencia pointing at local MLX, call `/v1/models` and one non-streaming chat. Not required for CI; for local/optional verification.

### Phase 4 — Functional / E2E and monitoring

- [ ] **Prod smoke in CI (optional)**
  - Separate job or workflow that runs `scripts/smoke-prod.sh` against `https://llm.menezmethod.com`, only when `INFERENCIA_E2E_API_KEY` is set in repo secrets. Run on schedule (e.g. cron) or after deploy; fail the job if health/ready/metrics fail (and optionally if `/v1/models` returns non-2xx with valid key).
- [ ] Document in README: “Production is at https://llm.menezmethod.com; CI ensures tests pass before merge; optional prod smoke can be run with `make smoke-prod`.”

---

## 4. CI/CD check (current)

- **Workflow**: `.github/workflows/ci.yml`
- **On**: `push`, `pull_request`
- **Steps**:
  1. Checkout repo.
  2. Set up Go 1.26.
  3. **Build**: `go build ./...`
  4. **Test**: `go test -race -count=1 ./...`
  5. **Vet**: `go vet ./...`
- **Branch protection (recommended)**: Require “CI” to pass before merging to `main` so production always stays protected.

---

## 5. Running tests locally

```bash
make test        # all tests (Ginkgo + std) with race detector
make test-v      # verbose
go test -v ./internal/handler/...   # Ginkgo handler specs (verbose)
go test -race -cover ./...          # with coverage
```

**BDD (Ginkgo/Gomega):** Handler, config, and auth use `Describe`/`Context`/`It` and `Expect(...).To(...)`. Suites live in `*_suite_test.go`; specs in `*_spec_test.go`.

Optional prod smoke (requires network; set your deployment URL and optional API key):

```bash
export INFERENCIA_SMOKE_BASE_URL=https://your-inferencia.example.com
export INFERENCIA_E2E_API_KEY=sk-your-key   # optional, for /v1/models
./scripts/smoke-prod.sh
# or: make smoke-prod
```

---

## 6. Summary

| Area | Action |
|------|--------|
| **CI** | GitHub Actions: build + test + vet on every push/PR. |
| **Unit** | Add handler (embeddings, openapi, errors), middleware (logging, metrics, recover, ratelimit), backend (registry, optional MLX with fake server), config/apierror/logging. |
| **Integration** | Server test: start app with test config, hit health/ready/metrics/v1/models. |
| **Functional** | Prod smoke script; optional CI job with secret API key for post-deploy/scheduled checks. |
| **Gate** | Protect `main`: require CI to pass. |

This keeps the app in prod reliable while we add tests incrementally.

# CLAUDE.md — Agent Context for inferencia

## Project Overview

inferencia is a lightweight, production-grade AI gateway written in Go. It exposes local LLM and TTS servers to the internet through an OpenAI-compatible REST API with bearer-token authentication, token-bucket rate limiting, structured logging, Prometheus metrics, and a background watchdog.

## Architecture Topology

```
Internet → Cloudflare Tunnel → Pi5:80 → Traefik → inferencia:8080
                                                         │
                            ┌────────────────────────────┴────────────────────────────┐
                            ↓                                                         ↓
                   Ollama (:11434)                            Kokoro (:50051) / Chatterbox (:50052)
                   (chat, embed)                                       (TTS)
                   Mac M4 Max — 192.168.0.109
```

- **Raspberry Pi 5** — Runs only inferencia (Go binary). No ML models.
- **Mac M4 Max (128 GB)** — All ML compute: Ollama (`:11434`), Kokoro TTS (`:50051`), Chatterbox TTS (`:50052`).
- All backend communication is over local LAN. Pi 5 is the only machine exposed to the internet.

## Key Conventions

### Coding Style (Karpathy-style)
- **Zero frameworks** — stdlib `net/http` with Go 1.22 routing (`mux.HandleFunc("GET /path", handler)`). No gorilla/mux, no chi, no gin.
- **Minimal dependencies** — external deps only where essential: Prometheus client, slog, Yaml (for config), OTLP (optional tracing).
- **Explicit wiring** — `main.go` wires everything manually. No DI framework. No init() magic.
- **Canonical log lines** — Every request produces one structured JSON line (Stripe-style) with request_id, method, path, status, duration_ms, bytes, remote_addr, user_agent, api_key (masked last 8 chars).
- **Middleware chains** — RequestID → Recover → Metrics → Logging → Auth → RateLimit. Logging runs AFTER Auth so the canonical log includes the masked API key.

### Test Patterns
- **Ginkgo + Gomega** — BDD-style specs (`Describe`, `It`, `Expect`).
- **Integration tests** — `make integration` spins up the app, runs Ginkgo suite + Newman (Postman CLI).
- **Race detector** — `-race` always on in `make test`.
- **CI must pass before merge** — build + test (`-race`) + vet run on every push/PR.

### Project Structure
```
inferencia/
├── cmd/inferencia/main.go       # Entry point, wiring, graceful shutdown, watchdog
├── internal/
│   ├── config/config.go         # YAML + env configuration
│   ├── server/server.go         # HTTP server, route registration
│   ├── handler/                 # HTTP handlers (chat, models, embeddings, audio/tts, health)
│   ├── middleware/               # Auth, rate limiting, logging, recovery, metrics
│   ├── backend/                  # Backend interface + adapters (Ollama, MLX, TTS)
│   ├── router/                   # Smart backend routing with capability-based selection
│   ├── watchdog/                 # Background health-check loop with fail threshold
│   ├── auth/keystore.go         # API key storage & validation
│   └── openapi/spec.yaml       # Embedded OpenAPI spec (served at /openapi.yaml)
├── docs/                         # Documentation
├── deploy/                       # Observability stack (Prometheus, Grafana, Loki, Alertmanager)
├── integration/                  # Ginkgo integration tests
├── postman/                      # Postman collection + env for Newman
├── Dockerfile                    # Multi-stage, non-root, HEALTHCHECK
└── config.example.yaml
```

## API Usage (for Agents)

### Base URL
```
https://llm.menezmethod.com/v1
```

### Default Chat Model
`gemma4:e4b` — used when `model` is omitted in chat completions requests.

### Authentication
All `/v1/*` endpoints require Bearer auth. No auth needed for `/health/*`, `/metrics`, `/version`, `/docs`, `/openapi.yaml`.

```
Authorization: Bearer sk-...
```

### Key Endpoints

| Endpoint | Method | Auth | Purpose |
|---|---|---|---|
| `GET /health` | GET | No | Comprehensive health + model inventory |
| `GET /health/status` | GET | No | Same as `/health` |
| `GET /health/ready` | GET | No | Per-backend ready check |
| `POST /v1/chat/completions` | POST | Bearer | Chat with streaming + tools |
| `POST /v1/audio/speech` | POST | Bearer | TTS synthesis (Kokoro/Chatterbox) |
| `POST /v1/embeddings` | POST | Bearer | Generate embeddings |
| `GET /v1/models` | GET | Bearer | List models |

## Health Check Format

Both `GET /health` and `GET /health/status` return a `HealthStatusResponse`:

```go
type HealthStatusResponse struct {
    Status    string                   // "healthy" or "degraded"
    Version   string                   // build version
    Timestamp string                   // ISO 8601
    Services  map[string]ServiceStatus // per-service breakdown
    Summary   HealthSummary            // aggregate totals
}

type ServiceStatus struct {
    Status   string       // "healthy" or "unhealthy"
    Error    string       // omitempty, present only when unhealthy
    Models   []ModelBrief // omitempty, model/voice inventory when healthy
}

type ModelBrief struct {
    ID      string
    Object  string // "model" or "voice"
    OwnedBy string
}

type HealthSummary struct {
    Total     int
    Healthy   int
    Unhealthy int
    ByType    map[string]int // e.g. {"chat": 1, "tts": 2}
}
```

Returns **200** if all services healthy, **503** if any service is degraded.

`GET /health/ready` returns 200 `{"status":"ready","version":"..."}` or 503 `{"status":"unavailable","backend":"...","error":"..."}`.

## TTS Backend Routing Rules

1. **Backend selection** — Client sets `model` field: `"kokoro"` or `"chatterbox"`.
2. **Voice defaults** — Selected AFTER backend:
   - Kokoro: `af_bella` (default if `voice` omitted, 21 voices available)
   - Chatterbox: Do NOT send `voice` field (only `chatterbox-default`)
3. **Response formats**: `wav` (default), `mp3`, `opus`, `flac`, `pcm`
4. **Supported response formats by backend**: Kokoro (all formats), Chatterbox (all formats)

## Deployment Flow (Coolify on Pi5)

1. Push to GitHub (private repo)
2. Coolify: New resource → Application → Dockerfile build
3. Set env vars (use `env.coolify.example` as template)
4. Coolify configures Cloudflare Tunnel + TLS + subdomain
5. **Auto-deploy on `main`** — only CI-passing code can merge, so every deploy is green
6. Verify: hit `/health`, then `/v1/models` with auth

### Required Coolify Env Vars
```
INFERENCIA_HOST=0.0.0.0
INFERENCIA_PORT=8080
INFERENCIA_BACKEND_URL=http://192.168.0.109:11434
INFERENCIA_KOKORO_URL=http://192.168.0.109:50051
INFERENCIA_CHATTERBOX_URL=http://192.168.0.109:50052
INFERENCIA_API_KEYS=sk-...
```

## Config Override Priority

1. Environment variables (`INFERENCIA_*`) — highest priority
2. `config.yaml` file — fallback
3. Built-in defaults — lowest priority

## Watchdog Behavior

- Interval: 30s (configurable)
- Fail threshold: 3 consecutive failures → DEGRADED
- Per-probe timeout: 5s
- Prometheus gauge `inferencia_backend_healthy` set to 0 when degraded, 1 when recovered
- Degraded backends removed from auto-rotation

## Metrics Always On

- `inferencia_http_requests_total` — Counter by method, path, status
- `inferencia_http_request_duration_seconds` — Histogram (5ms–120s)
- `inferencia_tokens_total` — Counter by model and type
- `inferencia_backend_healthy` — Gauge per-backend
- `inferencia_ratelimit_rejections_total` — Counter
- `inferencia_tts_requests_total` — Counter by backend and status
- `inferencia_tts_characters_total` — Counter by backend
- `inferencia_routing_decisions_total` — Counter by capability and backend

## OpenAI Compatibility Notes

- **Chat completions**: Full streaming (SSE), tool calling, JSON mode, vision via content parts
- **Embeddings**: `float` and `base64` encoding
- **TTS**: Uses `model` field for backend selection (not standard OpenAI)
- **Error envelope**: OpenAI-compatible `{error: {message, type, code, param}}`

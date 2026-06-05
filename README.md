# inferencia

A lightweight, secure AI gateway that exposes local LLM and TTS servers to the internet through an OpenAI-compatible API.

Run models on your own hardware. Access them from anywhere.

**Default chat model:** **gemma4:e4b** when the request omits `model`. Deploy with Coolify or any container platform; metrics and logging are always on.

## Why

Cloud inference is expensive. If you have capable hardware (Mac M4 Max, 128GB), you can serve local models at ~150 tokens/second for free. **inferencia** sits between the internet and your local LLM and TTS servers, adding authentication, rate limiting, routing, and observability — making your local setup behave like a hosted API provider.

## Architecture

```
Internet → Cloudflare Tunnel (coolify-tunnel) → Pi5:80 → Traefik → inferencia:8080
                                                                         │
                                                    ┌────────────────────┴────────────────────┐
                                                    ↓                                         ↓
                                           Ollama (:11434)                  Kokoro (:50051) / Chatterbox (:50052)
                                           (chat, embed)                             (TTS)
                                           Mac M4 Max — 192.168.0.109
```

**Compute split:**
- **Raspberry Pi 5** — Runs only inferencia (Go binary, ~20 MB). No ML models run on the Pi.
- **Mac M4 Max (128 GB)** — All ML models run here: Ollama (chat, embeddings) at `:11434`, Kokoro (TTS) at `:50051`, Chatterbox (TTS) at `:50052`.

All backend communication is over the local LAN. The Pi 5 is the only machine exposed to the internet (via Cloudflare Tunnel).

## Features

- **OpenAI-compatible API** — `/v1/chat/completions`, `/v1/models`, `/v1/embeddings` with full tool calling support
- **TTS synthesis** — `/v1/audio/speech` with multiple voice backends (Kokoro, Chatterbox)
- **Streaming** — Server-Sent Events (SSE) for real-time token streaming
- **Multi-backend routing** — Pluggable backend system with smart backend selection (Ollama, MLX, Kokoro, Chatterbox)
- **Bearer token auth** — File-based or environment variable API keys
- **Token bucket rate limiting** — Per-key with configurable burst
- **Background watchdog** — Periodic health checks every 30s with fail threshold (3) and Prometheus gauge updates
- **Structured logging** — JSON or text via `slog`, with GCP-compatible cloud formats
- **Prometheus metrics** — Always-on `/metrics` endpoint with HTTP, backend, and TTS metrics
- **OpenTelemetry tracing** — Optional OTLP HTTP tracing for distributed traces
- **Graceful shutdown** — Clean connection draining on SIGINT/SIGTERM
- **Zero frameworks** — stdlib `net/http` with Go 1.22 routing. Minimal external dependencies

## Quick Start

```bash
# Clone and build
git clone https://github.com/menezmethod/inferencia.git
cd inferencia
cp config.example.yaml config.yaml
cp keys.example.txt keys.txt

# Edit config.yaml to match your setup, then:
make run
```

## Configuration

Copy `config.example.yaml` to `config.yaml`:

```yaml
server:
  host: "127.0.0.1"
  port: 8080
  write_timeout: 120s

auth:
  keys_file: "./keys.txt"

backends:
  - name: "ollama"
    type: "ollama"
    url: "http://localhost:11434"
    timeout: 60s

tts_backends:
  - name: "kokoro"
    url: "http://localhost:50051"
    timeout: 30s
  # - name: "chatterbox"
  #   url: "http://localhost:50052"
  #   timeout: 30s

ratelimit:
  requests_per_second: 10
  burst: 20

watchdog:
  interval: 30s
  fail_threshold: 3
  request_timeout: 5s

log:
  level: "info"
  format: "json"

observability:
  otel_enabled: false
  otel_endpoint: "http://localhost:4318"
  otel_service_name: "inferencia"
```

### Environment Variables

All settings can be overridden via environment variables (prefix `INFERENCIA_`):

| Variable | Default | Description |
|----------|---------|-------------|
| `INFERENCIA_HOST` | `127.0.0.1` | Listen address (use `0.0.0.0` in Docker/Coolify) |
| `INFERENCIA_PORT` | `8080` | Listen port |
| `INFERENCIA_BACKEND_URL` | `http://localhost:11434` | Override first backend URL (e.g. Ollama on another host) |
| `INFERENCIA_KOKORO_URL` | — | Kokoro TTS backend URL (e.g. `http://192.168.0.109:50051`) |
| `INFERENCIA_CHATTERBOX_URL` | — | Chatterbox TTS backend URL (e.g. `http://192.168.0.109:50052`) |
| `INFERENCIA_MISOTTS_URL` | — | MisoTTS backend URL (currently blocked/unloaded) |
| `INFERENCIA_ELEVENLABS_URL` | — | ElevenLabs TTS backend URL |
| `INFERENCIA_API_KEYS` | — | Comma-separated API keys (overrides `keys_file`, use in Docker/Coolify) |
| `INFERENCIA_LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `INFERENCIA_LOG_FORMAT` | `json` | Log format: `json` or `text` |
| `INFERENCIA_LOG_CLOUD_FORMAT` | — | GCP-compatible severity: `gcp` or `gcp_with_resource` |
| `INFERENCIA_RATELIMIT_RPS` | `10` | Requests per second per key |
| `INFERENCIA_RATELIMIT_BURST` | `20` | Burst allowance |
| `INFERENCIA_WATCHDOG_INTERVAL` | `30s` | Health-check interval (e.g. `15s`, `60s`) |
| `INFERENCIA_WATCHDOG_FAIL_THRESHOLD` | `3` | Consecutive failures before DEGRADED |
| `INFERENCIA_WATCHDOG_TIMEOUT` | `5s` | Per-probe HTTP timeout |
| `INFERENCIA_OTEL_ENABLED` | `false` | Enable OpenTelemetry tracing |
| `INFERENCIA_OTEL_ENDPOINT` | — | OTLP HTTP collector URL (e.g. `http://localhost:4318`) |
| `INFERENCIA_OTEL_SERVICE_NAME` | `inferencia` | OpenTelemetry service name |

## API

All API endpoints except `/health`, `/health/ready`, `/health/status`, `/metrics`, `/version`, `/docs`, and `/openapi.yaml` require a Bearer token.

### Chat Completions

Default model fallback (when `model` is omitted): **gemma4:e4b**.

```bash
curl http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer sk-your-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemma4:e4b",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

With streaming:

```bash
curl http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer sk-your-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemma4:e4b",
    "messages": [{"role": "user", "content": "Hello!"}],
    "stream": true
  }'
```

With tool calling:

```bash
curl http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer sk-your-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemma4:e4b",
    "messages": [{"role": "user", "content": "What is the weather in SF?"}],
    "tools": [{
      "type": "function",
      "function": {
        "name": "get_weather",
        "description": "Get current weather",
        "parameters": {
          "type": "object",
          "properties": {"location": {"type": "string"}},
          "required": ["location"]
        }
      }
    }]
  }'
```

### List Models

```bash
curl http://localhost:8080/v1/models \
  -H "Authorization: Bearer sk-your-key"
```

### Embeddings

```bash
curl http://localhost:8080/v1/embeddings \
  -H "Authorization: Bearer sk-your-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "nomic-embed-text:latest",
    "input": "Hello world"
  }'
```

### Text-to-Speech (TTS)

```bash
curl http://localhost:8080/v1/audio/speech \
  -H "Authorization: Bearer sk-your-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "kokoro",
    "input": "Hello, this is a test of the TTS system.",
    "voice": "af_bella",
    "response_format": "wav"
  }' --output speech.wav
```

**TTS backends:**

| Backend | Port | Voices | Default Voice |
|---------|------|--------|---------------|
| Kokoro | `50051` | 21 voices (`af_bella`, `af_heart`, `af_nicole`, `am_michael`, `bf_emma`, etc.) | `af_bella` |
| Chatterbox | `50052` | 1 voice (`chatterbox-default`) | `chatterbox-default` |

**Supported response formats:** `wav`, `mp3`, `opus`, `flac`, `pcm` (default: `wav`)

**Voice selection rules:**
- Kokoro: `af_bella` (default if omitted), any of 21 available voices
- Chatterbox: Do **not** include a `voice` field — it only accepts its default
- Select a backend via the `model` field (`"kokoro"` or `"chatterbox"`)

### Health

All health endpoints require **no authentication**.

**Liveness + comprehensive health (`GET /health`):**

Returns the same comprehensive response as `/health/status`:

```json
{
  "status": "healthy",
  "version": "dev",
  "timestamp": "2026-06-05T00:49:40Z",
  "services": {
    "ollama": {
      "status": "healthy",
      "models": [{"id": "gemma4:e4b", "object": "model"}, {"id": "nomic-embed-text:latest", "object": "model"}]
    },
    "kokoro": {
      "status": "healthy",
      "models": [{"id": "af_bella", "object": "voice", "owned_by": "af_bella"}]
    },
    "chatterbox": {
      "status": "healthy",
      "models": [{"id": "chatterbox-default", "object": "voice", "owned_by": "chatterbox-default"}]
    }
  },
  "summary": {
    "total": 3,
    "healthy": 3,
    "unhealthy": 0,
    "by_type": {"chat": 1, "tts": 2}
  }
}
```

Returns `200` if all services are healthy, `503` if any service is down.

**Comprehensive health status (`GET /health/status`):**

Same as `GET /health` — identical comprehensive response.

**Readiness (`GET /health/ready`):**

Returns `200` with `{"status": "ready", "version": "..."}` if all backends are healthy. Returns `503` with `{"status": "unavailable", "backend": "ollama", "error": "..."}` if any backend is unreachable.

**Version (`GET /version`):**

```json
{"version": "1.0.0", "commit": "abc1234"}
```

## Available Models

From the current Ollama deployment on the Mac M4 Max:

**Chat models:**
- `gemma4:e4b` (default)
- `qwen3.5` (122b, 35b, 27b, 9b, 4b, 2b, 0.8b variants)
- `qwen3` (0.6b)
- `qwen3.6` (27b-code, 35b-code, 35b-mlx variants)
- `gpt-oss:20b-cloud`

**Embedding models:**
- `nomic-embed-text:latest`

**Vision models:**
- `moondream:1.8b`

## Exposing to the Internet

Use [Cloudflare Tunnel](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/) to expose inferencia without opening firewall ports. Traffic goes: **Internet → Cloudflare edge → tunnel → inferencia (:8080)**. Auth and rate limiting are handled by inferencia.

### Option 1: Quick tunnel (no account)

Best for trying it out. You get a random `*.trycloudflare.com` URL; no login or DNS setup.

```bash
# Install cloudflared (macOS)
brew install cloudflare/cloudflare/cloudflared

# Start inferencia (in one terminal)
make run

# Start quick tunnel (in another terminal); use the URL it prints
cloudflared tunnel --url http://127.0.0.1:8080
```

Visit the printed URL (e.g. `https://something.trycloudflare.com`). Test health and API:

```bash
curl https://YOUR-URL.trycloudflare.com/health
curl -H "Authorization: Bearer sk-your-key" https://YOUR-URL.trycloudflare.com/v1/models
```

Quick tunnels are not guaranteed for production and may be rate-limited.

### Option 2: Named tunnel (production, with DNS)

Use a Cloudflare account and a fixed hostname.

```bash
# Install and log in (opens browser)
brew install cloudflare/cloudflare/cloudflared
cloudflared tunnel login

# Create a named tunnel
cloudflared tunnel create inferencia

# List tunnels to get the tunnel ID
cloudflared tunnel list

# Configure DNS: route a hostname to this tunnel (replace TUNNEL_ID and yourdomain.com)
cloudflared tunnel route dns inferencia llm.yourdomain.com

# Run the tunnel (replace TUNNEL_ID with the ID from tunnel list)
cloudflared tunnel --url http://127.0.0.1:8080 run inferencia
```

Then use `https://llm.yourdomain.com` (or whatever hostname you chose). See [Cloudflare Tunnel docs](https://developers.cloudflare.com/cloudflare-one/connections/connect-apps/) for config file and running as a service.

### Tunnel troubleshooting

| Issue | Check |
|-------|--------|
| Tunnel URL returns 502 / connection refused | inferencia must be running and listening on the same host/port as in `--url` (e.g. `http://127.0.0.1:8080`). |
| `/health` works but `/v1/models` returns 401 | Use the `Authorization: Bearer` header; key must be in `keys.txt` or `INFERENCIA_API_KEYS`. |
| `/health/ready` fails | Backend (e.g. Ollama at `localhost:11434`) must be reachable; start your LLM server or fix `backends[].url`. |
| TTS `/v1/audio/speech` returns 401 | TTS endpoint requires the same Bearer auth as chat endpoints. |
| TTS synthesis fails with backend unavailable | Ensure Kokoro (`:50051`) / Chatterbox (`:50052`) is running on the Mac. Check `INFERENCIA_KOKORO_URL` / `INFERENCIA_CHATTERBOX_URL`. |
| Port already in use | Change `server.port` in `config.yaml` or set `INFERENCIA_PORT`, and use that port in `cloudflared tunnel --url`. |

## Deploy on Coolify (Raspberry Pi 5 → Mac M4 Max)

Run inferencia on a Raspberry Pi 5 that can reach the Mac M4 Max over the LAN. Coolify builds the image, runs the container, and handles the tunnel and subdomain (e.g. `llm.menezmethod.com`).

1. **Push this repo to GitHub** (private is fine). Coolify will clone and build from it.

2. **In Coolify**: New resource → Application → GitHub → select repo. Build: **Dockerfile** (root). No need to mount config or keys if you use env vars.

3. **Environment variables** (required; no config file in the image):

   | Variable | Example | Purpose |
   |----------|---------|--------|
   | `INFERENCIA_HOST` | `0.0.0.0` | Listen on all interfaces so Coolify can proxy |
   | `INFERENCIA_PORT` | `8080` | Port the app listens on (match Coolify's proxy) |
   | `INFERENCIA_BACKEND_URL` | `http://192.168.0.109:11434` | Ollama on Mac M4 Max LAN IP |
   | `INFERENCIA_KOKORO_URL` | `http://192.168.0.109:50051` | Kokoro TTS on Mac M4 Max |
   | `INFERENCIA_CHATTERBOX_URL` | `http://192.168.0.109:50052` | Chatterbox TTS on Mac M4 Max |
   | `INFERENCIA_API_KEYS` | `sk-your-key` | Comma-separated API keys (no keys file in container) |

4. **Subdomain**: In Coolify, set the public URL to your domain (e.g. `llm.menezmethod.com`). Coolify will configure the tunnel and TLS.

5. **Test**: After deploy:
   ```bash
   curl https://llm.menezmethod.com/health
   curl -H "Authorization: Bearer sk-your-key" https://llm.menezmethod.com/v1/models
   curl -H "Authorization: Bearer sk-your-key" -H "Content-Type: application/json" \
     -d '{"model":"kokoro","input":"Hello","voice":"af_bella","response_format":"wav"}' \
     https://llm.menezmethod.com/v1/audio/speech --output test.wav
   ```

If `/health/ready` fails, the container cannot reach the Mac at `INFERENCIA_BACKEND_URL`; check LAN connectivity and that the Mac is on with Ollama listening on `:11434`.

**Deploy only after CI passes** — **Coolify Auto Deploy** on `main` + **branch protection**: `main` always auto-deploys when updated; only CI-passing code can be merged to `main`, so every deploy is from a green build. No webhook or secrets. See [docs/PUBLISHING.md](docs/PUBLISHING.md#coolify-main-auto-deploys).

### Production checklist

- [ ] **API key**: Use a strong key (e.g. `openssl rand -hex 32`, prefix with `sk-`). Set in Coolify as `INFERENCIA_API_KEYS` only; never commit keys.
- [ ] **Backend URL**: Use the Mac's **fixed LAN IP** (DHCP reservation) in `INFERENCIA_BACKEND_URL`.
- [ ] **TTS URLs**: Set `INFERENCIA_KOKORO_URL` and `INFERENCIA_CHATTERBOX_URL` to the Mac's LAN IP and respective ports.
- [ ] **HTTPS**: Coolify provides TLS and tunnel; ensure the public URL uses `https://`.
- [ ] **Rate limit**: Defaults (10 req/s, burst 20) are in config; override with `INFERENCIA_RATELIMIT_RPS` / `INFERENCIA_RATELIMIT_BURST` if needed.
- [ ] **Watchdog**: Enabled by default (30s interval, 3 fail threshold). Adjust `INFERENCIA_WATCHDOG_INTERVAL` / `INFERENCIA_WATCHDOG_FAIL_THRESHOLD` if needed.
- [ ] **Logs**: Set `INFERENCIA_LOG_LEVEL=info` (or `debug` only when troubleshooting).
- [ ] **Compute split**: Confirm the Pi 5 runs ONLY inferencia (Go binary). All ML models run on the Mac M4 Max. No models on the Pi.

## API documentation

- **Swagger UI**: `https://your-deployment/docs`
- **OpenAPI spec**: `https://your-deployment/openapi.yaml`
- **Version**: `GET /version` returns `{"version":"1.0.0"}` (and optional `commit`). All health endpoints also include `version` in the JSON.
- **Quickstart guide**: [docs/AGENT_ONBOARDING.md](docs/AGENT_ONBOARDING.md) — how to connect any OpenAI-compatible client (Python, Node.js, curl, LangChain, etc.) to inferencia.

## Observability

inferencia ships with **Prometheus metrics** (always on), **structured logging**, optional **OpenTelemetry** tracing, and **GCP/cloud-friendly log formats** for easy integration with Google Cloud Logging and other backends. A full Grafana/Loki/Alertmanager stack is provided in `deploy/`.

**Full setup guide:** [docs/METRICS_AND_LOGGING.md](docs/METRICS_AND_LOGGING.md) — step-by-step metrics scraping, logging (JSON, GCP), and optional tracing.

### Prometheus (metrics — always on)

The `/metrics` endpoint (no auth) is **always enabled**. It exposes HTTP, backend, and TTS metrics; no config or feature flag required.

- **Local run**: With inferencia on your machine, scrape `http://127.0.0.1:8080/metrics` or use the deploy stack: run `docker compose -f deploy/docker-compose.observability.yaml up -d` and point Prometheus at `host.docker.internal:8080` (the `inferencia-local` job in `deploy/prometheus/prometheus.yaml` does this).
- **Quick check**: `curl -s http://127.0.0.1:8080/metrics | head -20`
- **Production**: Scrape your inferencia host:port; the deploy stack also includes an `inferencia` job for when the app runs in the same Compose network.

### OpenTelemetry (optional)

Enable OTLP HTTP tracing for distributed traces (e.g. Jaeger, Grafana Tempo, Google Cloud Trace):

```yaml
# config.yaml
observability:
  otel_enabled: true
  otel_endpoint: "http://localhost:4318"   # OTLP HTTP collector (e.g. otelcol)
  otel_service_name: "inferencia"
```

Or via env:

```bash
export INFERENCIA_OTEL_ENABLED=true
export INFERENCIA_OTEL_ENDPOINT=http://localhost:4318
export INFERENCIA_OTEL_SERVICE_NAME=inferencia
```

When enabled, every HTTP request is traced; export to any OTLP-capable backend. Use `https://` endpoints in production (TLS); `http://` uses insecure transport for local collectors.

### GCP / cloud logging

For **Google Cloud Logging** (or any consumer expecting a `severity` field), set `log.cloud_format` so JSON logs include a `severity` string (`DEBUG`, `INFO`, `WARNING`, `ERROR`) and optionally a `resource` object:

```yaml
log:
  level: "info"
  format: "json"
  cloud_format: "gcp"                # add severity only
  # cloud_format: "gcp_with_resource" # add severity + resource (generic_task)
```

Env: `INFERENCIA_LOG_CLOUD_FORMAT=gcp` or `gcp_with_resource`. Works with GCP's log ingestion (e.g. Cloud Run, GKE, or VM logging agent).

### Canonical log lines

Every request produces a single structured JSON log line (Stripe-style "canonical log line") with all fields needed for debugging, alerting, and analytics:

```json
{
  "level": "INFO",
  "msg": "request",
  "request_id": "a1b2c3d4e5f6...",
  "method": "POST",
  "path": "/v1/chat/completions",
  "status": 200,
  "duration_ms": 1423,
  "bytes": 512,
  "remote_addr": "192.168.0.60:41234",
  "user_agent": "OpenAI/Python 1.0.0",
  "api_key": "...bd09b03"
}
```

- **Request ID**: Auto-generated 16-byte hex ID per request. Pass `X-Request-ID` header to propagate your own (for distributed tracing). Echoed back in the response.
- **API key**: Masked to last 8 chars (safe to log, sufficient to identify the caller).
- **Log level**: `INFO` for 2xx/3xx, `WARN` for 4xx, `ERROR` for 5xx.

### Loki integration

Logs are Loki-native when running in JSON format (default). Query in Grafana:

```logql
{service="inferencia"} | json | status >= 500
{service="inferencia"} | json | path="/v1/chat/completions" | duration_ms > 5000
{service="inferencia"} | json | api_key="...bd09b03"
```

### Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `inferencia_http_requests_total` | Counter | Total requests by method, path, status |
| `inferencia_http_request_duration_seconds` | Histogram | Request latency (14 buckets, 5ms–120s) |
| `inferencia_http_requests_in_flight` | Gauge | Active requests |
| `inferencia_tokens_total` | Counter | Tokens by model and type (prompt/completion) |
| `inferencia_backend_healthy` | Gauge | Backend health (1=up, 0=down) — also reflects watchdog state |
| `inferencia_ratelimit_rejections_total` | Counter | Rate-limited requests |
| `inferencia_backend_request_duration_seconds` | Histogram | Backend latency |
| `inferencia_tts_requests_total` | Counter | TTS requests by backend and status |
| `inferencia_tts_request_duration_seconds` | Histogram | TTS synthesis latency |
| `inferencia_tts_characters_total` | Counter | TTS characters synthesized by backend |
| `inferencia_routing_decisions_total` | Counter | Routing decisions by capability and backend |

### Quick start (full stack)

```bash
docker compose -f deploy/docker-compose.observability.yaml up -d
```

| Service | URL | Credentials |
|---------|-----|-------------|
| Prometheus | http://localhost:9090 | — |
| Grafana | http://localhost:3000 | admin / admin |
| Alertmanager | http://localhost:9093 | — |
| Loki | http://localhost:3100 | — (queried via Grafana) |

Grafana auto-provisions both **Prometheus** and **Loki** datasources, plus an **inferencia** dashboard with request rates, latency percentiles, token throughput, backend health, error rates, rate-limit rejections, and TTS metrics.

### Alert rules

| Alert | Condition | Severity |
|-------|-----------|----------|
| HighErrorRate | >5% of requests returning 5xx for 2min | critical |
| BackendDown | Backend health gauge = 0 for 1min | critical |
| RateLimitSpike | >10 rejections/sec for 2min | warning |
| HighLatency | p99 chat latency >30s for 5min | warning |
| NoTraffic | Zero requests for 10min | warning |
| HighTokenBurnRate | >100k tokens/min for 5min | warning |
| TTSBackendDown | TTS backend health gauge = 0 for 1min | critical |

Edit `deploy/alertmanager/alertmanager.yaml` to route alerts to Slack, email, PagerDuty, etc.

## Watchdog

The watchdog is a background goroutine that periodically health-checks all registered backends (chat, embed, and TTS) and updates Prometheus gauges.

- **Interval**: 30s (configurable via `watchdog.interval` or `INFERENCIA_WATCHDOG_INTERVAL`)
- **Fail threshold**: 3 consecutive failures before a backend is marked **DEGRADED** (configurable via `watchdog.fail_threshold` or `INFERENCIA_WATCHDOG_FAIL_THRESHOLD`)
- **Per-probe timeout**: 5s (configurable via `watchdog.request_timeout` or `INFERENCIA_WATCHDOG_TIMEOUT`)

When a backend is marked DEGRADED:
- The `inferencia_backend_healthy` Prometheus gauge is set to `0`
- The backend is removed from auto-rotation for new requests
- When the backend recovers, the gauge is set back to `1` and it rejoins rotation

## Project Structure

```
inferencia/
├── cmd/inferencia/main.go       # Entry point, wiring, graceful shutdown, watchdog
├── deploy/                      # Observability stack
│   ├── docker-compose.observability.yaml
│   ├── prometheus/              # Scrape config + alert rules
│   ├── loki/                    # Loki config (log aggregation)
│   ├── promtail/                # Promtail config (log shipping)
│   ├── grafana/                 # Dashboards + datasource provisioning (Prometheus + Loki)
│   └── alertmanager/            # Alertmanager config (Slack, email, etc.)
├── docs/
│   ├── AGENT_ONBOARDING.md     # API quickstart guide for clients and agents
│   ├── METRICS_AND_LOGGING.md  # Metrics and logging setup guide
│   ├── TESTING_PLAN.md         # Testing plan and CI
│   └── openapi.yaml            # OpenAPI 3.1 spec (reference copy)
├── scripts/
│   ├── run-integration-and-newman.sh  # Start app, run Ginkgo integration + Newman
│   └── smoke-prod.sh           # Production smoke test (health, ready, metrics)
├── integration/                 # Ginkgo integration suite (spins up app, hits API)
├── postman/                     # Postman collection + env for Newman (API contract tests)
├── internal/
│   ├── config/config.go         # YAML + env configuration (includes watchdog, TTS)
│   ├── server/server.go         # HTTP server, route registration, health status
│   ├── handler/                 # HTTP handlers (chat, models, embeddings, audio/tts, health)
│   ├── middleware/               # Auth, rate limiting, logging, recovery, metrics
│   ├── backend/                  # Backend interface, Ollama adapter, MLX adapter, TTS adapter
│   ├── router/                   # Smart backend routing with capability-based selection
│   ├── watchdog/                 # Background health-check loop with fail threshold
│   ├── auth/keystore.go         # API key storage & validation
│   ├── apierror/error.go       # OpenAI-compatible error responses
│   └── openapi/spec.yaml       # Embedded OpenAPI spec (served at /openapi.yaml)
├── config.example.yaml
├── keys.example.txt
├── Dockerfile                   # Multi-stage, non-root, healthcheck (Coolify-ready)
├── .dockerignore
├── docker-compose.yaml         # Compose for Coolify + local (Coolify expects .yaml)
├── .env.example                 # Env template (copy to .env; never commit .env)
├── Makefile
└── README.md
```

## Testing

CI runs on every push and PR: **build**, **test** (with `-race`), and **vet** must pass. See [docs/TESTING_PLAN.md](docs/TESTING_PLAN.md) for the full testing plan. To configure GitHub (branch protection, Coolify deploy webhook, smoke-test secrets) in one go: copy `.env.gh.secrets.example` to `.env.gh.secrets`, fill in values, and run `./scripts/setup-repo.sh` (requires `gh auth login`). See [docs/PUBLISHING.md](docs/PUBLISHING.md) for sanitization and going public. For a reusable CI/CD and hosting path across apps, see [docs/CI_CD_AND_HOSTING_PLAYBOOK.md](docs/CI_CD_AND_HOSTING_PLAYBOOK.md). [SECURITY.md](SECURITY.md) describes how to report vulnerabilities.

```bash
make test        # Unit tests (Ginkgo/Gomega) with race detector
make integration # Integration tests: spin up app, run Ginkgo suite + Newman (Postman CLI); must pass in CI
make smoke-prod  # Smoke test your deployment (set INFERENCIA_SMOKE_BASE_URL; optional INFERENCIA_E2E_API_KEY for /v1/models)
```

The **integration** suite lives in `integration/` and uses Ginkgo to start the app and hit real endpoints; **Newman** runs the Postman collection in `postman/` (same flows). Both run in CI and must pass before merge.

Unit tests use **Ginkgo** and **Gomega** for BDD-style specs in `internal/handler`, `internal/config`, `internal/auth`, `internal/watchdog`, and other packages (e.g. `Describe("Health", func() { It("returns 200 and status ok", ...) })`).

## Development

```bash
make build    # Build binary
make run      # Build and run
make test     # Run tests with race detector
make fmt      # Format code
make vet      # Run go vet
make lint     # Run golangci-lint
make clean    # Remove binary
```

## Docker (local or Coolify)

The image is **Coolify-ready**: multi-stage build, non-root user (UID 1000), healthcheck, no config or secrets in the image (env only).

**Build and run with env vars:**

```bash
docker build -t inferencia:latest .
docker run --rm -p 8080:8080 \
  -e INFERENCIA_HOST=0.0.0.0 \
  -e INFERENCIA_PORT=8080 \
  -e INFERENCIA_BACKEND_URL=http://host.docker.internal:11434 \
  -e INFERENCIA_API_KEYS=sk-your-key \
  inferencia:latest
```

**Or use Docker Compose (copy env first):**

```bash
cp .env.example .env
# Edit .env with your INFERENCIA_BACKEND_URL and INFERENCIA_API_KEYS
docker compose up --build
```

Then: `curl http://localhost:8080/health` and `curl -H "Authorization: Bearer sk-your-key" http://localhost:8080/v1/models`.

For **Coolify**, see [Deploy on Coolify](#deploy-on-coolify-raspberry-pi-5--mac-m4-max) above: connect repo, build Dockerfile, set the same env vars in the Coolify UI.

## License

MIT

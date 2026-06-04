# Inferencia AI Gateway — Corrected Architecture

## Topology

```
Internet (Cloudflare)
    │
    ▼ Coolify (Traefik)
    │
    ▼ Pi5 (192.168.0.207) — inferencia (Go binary, Docker, ~20MB)
    │    Port 8080
    │    Auth • Rate Limit • Metrics • Smart Router
    │
    ▼ LAN (192.168.0.0/24)
    │
    ▼ Mac M4 Max (192.168.0.109) — 128GB RAM
    │
    ├── ollama (:11434) — launchd, RUNNING
    ├── kokoro-tts (:50051) — launchd, NEW
    ├── chatterbox-tts (:50052) — launchd, NEW (when installed)
    ├── misotts (:50053) — launchd, FUTURE (when installed)
    └── elevenlabs-proxy (:50054) — launchd, FUTURE
```

## Port Map

| Service | Port | Protocol | Status |
|---------|------|----------|--------|
| ollama | 11434 | HTTP (Ollama API) | Running |
| kokoro-tts | 50051 | HTTP (OpenAI TTS compatible) | To build |
| chatterbox-tts | 50052 | HTTP (OpenAI TTS compatible) | To build |
| misotts | 50053 | HTTP (OpenAI TTS compatible) | Future |
| elevenlabs-proxy | 50054 | HTTP (proxy to API) | Future |

## Each TTS Server Exposes

```
GET  /health         → {"status":"ok","engine":"kokoro","version":"1.0.0"}
GET  /metrics        → Prometheus text format
GET  /v1/models      → {"data":[{"id":"kokoro","object":"model",...}]}
POST /v1/audio/speech → Audio bytes (WAV/MP3)

Request body (OpenAI-compatible):
{
  "model": "kokoro",
  "input": "Hello world",
  "voice": "af_bella",
  "response_format": "mp3",
  "speed": 1.0
}
```

## Each Service Runs via launchd

- Plist in `~/Library/LaunchAgents/com.menez.<service>.plist`
- `RunAtLoad: true` — starts on login
- `KeepAlive: true` — auto-restart on crash
- `StandardOutPath` / `StandardErrorPath` — logs to `~/Library/Logs/<service>/`
- Health check: inferencia pings `/health` every 30s

## inferencia Watchdog

- Background goroutine, runs every 30s
- Pings all configured backends' `/health` endpoints
- Updates Prometheus gauge: `inferencia_backend_healthy{name="kokoro"} 1`
- On 3 consecutive failures → mark backend DEGRADED, remove from auto-rotation
- On recovery → re-add to rotation
- On ALL backends for a capability (TTS) failing → set alert, return 503 with `"all_tts_backends_unavailable"`

## inferencia TTS Backend Adapter

```go
type TTSBackend interface {
    Synthesize(ctx context.Context, req TTSRequest) (*TTSResponse, error)
    Voices(ctx context.Context) ([]Voice, error)
    Name() string
    Health(ctx context.Context) error
}

type TTSRequest struct {
    Model    string `json:"model"`
    Input    string `json:"input"`
    Voice    string `json:"voice"`
    Format   string `json:"response_format"` // wav, mp3, opus
    Speed    float64 `json:"speed"`
}

type TTSResponse struct {
    Audio      []byte
    Format     string
    DurationMs int
}
```

## Prometheus Metrics (each TTS server)

```
tts_requests_total{engine="kokoro",status="success"} 42
tts_request_duration_seconds{engine="kokoro"} 0.234
tts_characters_total{engine="kokoro"} 15000
tts_voices_loaded{engine="kokoro"} 2
```

## Prometheus on Pi5 — Scrape Config

Add to existing `deploy/prometheus/prometheus.yaml`:
```yaml
scrape_configs:
  - job_name: 'kokoro-tts'
    scrape_interval: 15s
    static_configs:
      - targets: ['192.168.0.109:50051']
  - job_name: 'chatterbox-tts'
    static_configs:
      - targets: ['192.168.0.109:50052']
```

## inferencia Env Vars

```ini
# Current
INFERENCIA_BACKEND_URL=http://192.168.0.109:11434
INFERENCIA_API_KEYS=sk-...

# New - TTS backend addresses
INFERENCIA_KOKORO_URL=http://192.168.0.109:50051
INFERENCIA_CHATTERBOX_URL=http://192.168.0.109:50052
INFERENCIA_MISOTTS_URL=http://192.168.0.109:50053

# Watchdog
INFERENCIA_WATCHDOG_INTERVAL=30s
INFERENCIA_WATCHDOG_FAIL_THRESHOLD=3
```

## Build Order

1. **Workstream A — Mac TTS Servers** (Hermes → Python servers + launchd)
   - `kokoro-server.py` — FastAPI wrapper with /health, /metrics, /v1/models, /v1/audio/speech
   - `com.menez.kokoro-tts.plist` — launchd config
   - Same for chatterbox when installed

2. **Workstream B — inferencia Go Updates** (delegate to Cursor/Claude)
   - Phase 0: Per-kind interfaces, backend registry, model catalog
   - Phase 3: TTS backend adapters (HTTP to Mac servers)
   - Phase 1: Smart router (auto model selection)
   - Watchdog goroutine
   - Prometheus metrics for routing decisions
   - GitHub CI updated

3. **Workstream C — Pi5 Observability** (SSH)
   - Update Prometheus scrape config
   - Deploy Grafana dashboard for TTS metrics

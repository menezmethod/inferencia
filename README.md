# inferencia

A lightweight, secure API gateway that exposes local LLM servers to the internet through an OpenAI-compatible API.

Run models on your own hardware. Access them from anywhere.

## Why

Cloud inference is expensive. If you have capable hardware (M4 Pro, 128GB), you can serve local models at ~150 tokens/second for free. **inferencia** sits between the internet and your local LLM servers, adding authentication, rate limiting, and observability — making your local setup behave like a hosted API provider.

## Features

- **OpenAI-compatible API** — `/v1/chat/completions`, `/v1/models`, `/v1/embeddings` with full tool calling support
- **Streaming** — Server-Sent Events (SSE) for real-time token streaming
- **Multi-backend** — Pluggable backend system. MLX (MSTY) ships ready; Ollama stubbed for v2
- **Bearer token auth** — File-based or environment variable API keys
- **Token bucket rate limiting** — Per-key with configurable burst
- **Structured logging** — JSON or text via `slog`
- **Graceful shutdown** — Clean connection draining on SIGINT/SIGTERM
- **Zero frameworks** — stdlib `net/http` with Go 1.22 routing. One external dependency: `gopkg.in/yaml.v3`

## Quick Start

```bash
# Clone and build
git clone https://github.com/menez/inferencia.git
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
  - name: "mlx"
    type: "mlx"
    url: "http://localhost:11973"
    timeout: 60s

ratelimit:
  requests_per_second: 10
  burst: 20

log:
  level: "info"
  format: "json"
```

Environment variables override file values (prefix `INFERENCIA_`):

```bash
export INFERENCIA_PORT=9000
export INFERENCIA_HOST=0.0.0.0              # Required in Docker so the app is reachable
export INFERENCIA_API_KEYS=sk-key1,sk-key2  # Overrides keys_file (use in Docker/Coolify)
export INFERENCIA_BACKEND_URL=http://192.168.0.x:11973  # Override first backend URL (e.g. MLX on another host)
export INFERENCIA_LOG_LEVEL=debug
```

## API

### Chat Completions

```bash
curl http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer sk-your-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-oss-20b-MXFP4-Q8",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

With streaming:

```bash
curl http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer sk-your-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-oss-20b-MXFP4-Q8",
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
    "model": "gpt-oss-20b-MXFP4-Q8",
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
    "model": "gpt-oss-20b-MXFP4-Q8",
    "input": "Hello world"
  }'
```

### Health

```bash
# Liveness (no auth)
curl http://localhost:8080/health

# Readiness — checks backend connectivity (no auth)
curl http://localhost:8080/health/ready
```

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
| `/health` works but `/v1/models` returns 401 | Use the `Authorization: Bearer sk-your-key` header; key must be in `keys.txt` or `INFERENCIA_API_KEYS`. |
| Readiness fails (`/health/ready` not ok) | Backend (e.g. MLX at `localhost:11973`) must be reachable; start your LLM server or fix `config.yaml` `backends[].url`. |
| Port already in use | Change `server.port` in `config.yaml` or set `INFERENCIA_PORT`, and use that port in `cloudflared tunnel --url`. |

## Deploy on Coolify (e.g. Raspberry Pi → MLX on M4)

Run inferencia on a host that can reach your MLX server over the LAN (e.g. Pi at 192.168.0.207, MLX on M4 at a fixed LAN IP). Coolify builds the image, runs the container, and handles the tunnel and subdomain (e.g. `llm.menezmethod.com`).

1. **Push this repo to GitHub** (private is fine). Coolify will clone and build from it.

2. **In Coolify**: New resource → Application → GitHub → select repo. Build: **Dockerfile** (root). No need to mount config or keys if you use env vars.

3. **Environment variables** (required; no config file in the image). Copy from [env.coolify.example](env.coolify.example), replace `YOUR_M4_LAN_IP` and `sk-PASTE_YOUR_KEY_HERE`, then paste into Coolify’s env editor:

   | Variable | Example | Purpose |
   |----------|---------|--------|
   | `INFERENCIA_HOST` | `0.0.0.0` | Listen on all interfaces so Coolify can proxy |
   | `INFERENCIA_PORT` | `8080` | Port the app listens on (match Coolify’s proxy) |
   | `INFERENCIA_BACKEND_URL` | `http://192.168.0.50:11973` | MLX server URL (use your M4’s LAN IP; prefer DHCP reservation) |
   | `INFERENCIA_API_KEYS` | `sk-your-secret-key` | Comma-separated API keys (no keys file in container) |

4. **Subdomain**: In Coolify, set the public URL to `llm.menezmethod.com` (or your domain). Coolify will configure the tunnel and TLS.

5. **Test**: After deploy, `curl https://llm.menezmethod.com/health` and `curl -H "Authorization: Bearer sk-your-secret-key" https://llm.menezmethod.com/v1/models`.

If `/health/ready` fails, the container cannot reach the MLX host at `INFERENCIA_BACKEND_URL`; check LAN connectivity and that the M4 is on and MLX is listening on 11973.

### Production checklist

- [ ] **API key**: Use a strong key (e.g. `openssl rand -hex 32`, prefix with `sk-`). Set in Coolify as `INFERENCIA_API_KEYS` only; never commit keys.
- [ ] **Backend URL**: Use your M4’s **fixed LAN IP** (DHCP reservation) in `INFERENCIA_BACKEND_URL`.
- [ ] **HTTPS**: Coolify provides TLS and tunnel; ensure the public URL uses `https://`.
- [ ] **Rate limit**: Defaults (10 req/s, burst 20) are in config; override with `INFERENCIA_RATELIMIT_RPS` / `INFERENCIA_RATELIMIT_BURST` if needed.
- [ ] **Logs**: Set `INFERENCIA_LOG_LEVEL=info` (or `debug` only when troubleshooting).

## Architecture

```
Internet → Cloudflare Tunnel → inferencia (:8080)
                                     │
                              ┌──────┴──────┐
                              │  Middleware  │
                              │  recover →   │
                              │  logging →   │
                              │  auth →      │
                              │  ratelimit   │
                              └──────┬──────┘
                                     │
                              ┌──────┴──────┐
                              │   Backend   │
                              │   Registry  │
                              └──────┬──────┘
                                     │
                              ┌──────┴──────┐
                              │  MLX Server │
                              │  (:11973)   │
                              └─────────────┘
```

## Project Structure

```
inferencia/
├── cmd/inferencia/main.go       # Entry point, wiring, graceful shutdown
├── internal/
│   ├── config/config.go         # YAML + env configuration
│   ├── server/server.go         # HTTP server & route registration
│   ├── handler/                 # HTTP handlers (chat, models, embeddings, health)
│   ├── middleware/               # Auth, rate limiting, logging, recovery
│   ├── backend/                  # Backend interface, MLX adapter, Ollama stub
│   ├── auth/keystore.go         # API key storage & validation
│   └── apierror/error.go       # OpenAI-compatible error responses
├── config.example.yaml
├── keys.example.txt
├── Dockerfile                   # Multi-stage, non-root, healthcheck (Coolify-ready)
├── .dockerignore
├── docker-compose.yaml         # Compose for Coolify + local (Coolify expects .yaml)
├── .env.example                 # Env template (copy to .env; never commit .env)
├── Makefile
└── README.md
```

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
  -e INFERENCIA_BACKEND_URL=http://host.docker.internal:11973 \
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

For **Coolify**, see [Deploy on Coolify](#deploy-on-coolify-eg-raspberry-pi--mlx-on-m4) above: connect repo, build Dockerfile, set the same env vars in the Coolify UI.

## License

MIT

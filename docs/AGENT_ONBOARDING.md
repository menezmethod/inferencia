# inferencia API — Agent & Client Onboarding

A lightweight, production-grade AI gateway that exposes local LLM and TTS servers through an OpenAI-compatible REST API. Hosted on a Raspberry Pi 5 with all ML compute running on a Mac M4 Max over LAN.

**Default chat model (when `model` is omitted):** `gemma4:e4b`

---

## Base URL

```
https://llm.menezmethod.com/v1
```

Interactive docs (Swagger UI):

```
https://llm.menezmethod.com/docs
```

---

## Authentication

All `/v1/*` endpoints require a Bearer token in the `Authorization` header:

```
Authorization: Bearer sk-your-api-key-here
```

Health endpoints (`/health`, `/health/status`, `/health/ready`), `/metrics`, `/version`, `/docs`, and `/openapi.yaml` do **not** require authentication.

Contact the administrator to obtain an API key.

---

## Endpoints

| Endpoint | Method | Auth | Description |
|---|---|---|---|
| `/health` | GET | No | Liveness + comprehensive health (same as `/health/status`) |
| `/health/status` | GET | No | Comprehensive per-service health breakdown |
| `/health/ready` | GET | No | Readiness probe (per-backend check) |
| `/metrics` | GET | No | Prometheus metrics |
| `/version` | GET | No | Build version info |
| `/docs` | GET | No | Swagger UI |
| `/openapi.yaml` | GET | No | OpenAPI 3.1 spec |
| `/v1/models` | GET | Bearer | List available models |
| `/v1/chat/completions` | POST | Bearer | Chat completions (streaming, tool calling) |
| `/v1/embeddings` | POST | Bearer | Generate embeddings |
| `/v1/audio/speech` | POST | Bearer | Text-to-speech synthesis |

---

## Chat Completions

**Request body** (OpenAI-compatible):

| Field | Type | Required | Description |
|---|---|---|---|
| `model` | string | Yes | Model ID from `/v1/models` (default: `gemma4:e4b`) |
| `messages` | array | Yes | Conversation messages (`role` + `content`) |
| `temperature` | number | No | Sampling temperature (0–2) |
| `max_tokens` | integer | No | Maximum tokens to generate |
| `stream` | boolean | No | Enable SSE streaming (default: `false`) |
| `tools` | array | No | Tool/function definitions for function calling |
| `tool_choice` | string\|object | No | `"none"`, `"auto"`, `"required"`, or a specific function |
| `top_p` | number | No | Nucleus sampling (0–1) |
| `stop` | string\|array | No | Stop sequences (up to 4) |
| `presence_penalty` | number | No | Presence penalty (-2 to 2) |
| `frequency_penalty` | number | No | Frequency penalty (-2 to 2) |
| `response_format` | object | No | `{"type": "json_object"}` for JSON mode |

### curl — simple

```bash
curl https://llm.menezmethod.com/v1/chat/completions \
  -H "Authorization: Bearer sk-your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemma4:e4b",
    "messages": [{"role": "user", "content": "What is 2+2?"}],
    "max_tokens": 100
  }'
```

### curl — streaming

```bash
curl https://llm.menezmethod.com/v1/chat/completions \
  -H "Authorization: Bearer sk-your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemma4:e4b",
    "messages": [{"role": "user", "content": "Hello!"}],
    "stream": true
  }'
```

Responds with `Content-Type: text/event-stream`. Each event is `data: {json}\n\n`. The final event is `data: [DONE]\n\n`.

### curl — tool calling

```bash
curl https://llm.menezmethod.com/v1/chat/completions \
  -H "Authorization: Bearer sk-your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemma4:e4b",
    "messages": [{"role": "user", "content": "What is the weather in SF?"}],
    "tools": [{
      "type": "function",
      "function": {
        "name": "get_weather",
        "description": "Get current weather for a location",
        "parameters": {
          "type": "object",
          "properties": { "location": { "type": "string" } },
          "required": ["location"]
        }
      }
    }]
  }'
```

### Python (openai SDK)

```python
from openai import OpenAI

client = OpenAI(
    base_url="https://llm.menezmethod.com/v1",
    api_key="sk-your-api-key",
)

response = client.chat.completions.create(
    model="gemma4:e4b",
    messages=[{"role": "user", "content": "Hello!"}],
)
print(response.choices[0].message.content)
```

### Node.js (openai SDK)

```javascript
import OpenAI from "openai";

const client = new OpenAI({
  baseURL: "https://llm.menezmethod.com/v1",
  apiKey: "sk-your-api-key",
});

const response = await client.chat.completions.create({
  model: "gemma4:e4b",
  messages: [{ role: "user", content: "Hello!" }],
});
console.log(response.choices[0].message.content);
```

### Environment Variables

For any client that reads the standard OpenAI env vars:

```bash
export OPENAI_BASE_URL=https://llm.menezmethod.com/v1
export OPENAI_API_KEY=sk-your-api-key
```

---

## Text-to-Speech (TTS)

TTS is available via `/v1/audio/speech`. Select the backend using the `model` field.

### TTS Backends

| Backend | Port | Voices | Default Voice |
|---|---|---|---|
| Kokoro | `50051` | 21 voices | `af_bella` |
| Chatterbox | `50052` | 1 voice (`chatterbox-default`) | `chatterbox-default` |

### Kokoro Voices (21 total)

`af_bella`, `af_heart`, `af_nicole`, `af_aoede`, `af_kore`, `af_sarah`, `af_nova`, `af_sky`, `am_michael`, `am_fenrir`, `am_puck`, `am_liam`, `am_onyx`, `am_echo`, `am_eric`, `bf_emma`, `bf_isabella`, `bm_george`, `bm_fable`, `bm_lewis`, `bm_robert`

Default: `af_bella` (used if `voice` is omitted).

### Request Parameters

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `model` | string | No | `kokoro` | Backend: `"kokoro"` or `"chatterbox"` |
| `input` | string | Yes | — | Text to synthesize |
| `voice` | string | No | `af_bella` | Voice ID (omit for Chatterbox) |
| `response_format` | string | No | `wav` | `wav`, `mp3`, `opus`, `flac`, or `pcm` |
| `speed` | number | No | `1.0` | Speech speed (0.25–4.0) |

### Voice Selection Rules

- **Kokoro**: Pass any of the 21 voices above. Defaults to `af_bella` if omitted.
- **Chatterbox**: Do **not** include a `voice` field — it only accepts its single default.
- Select the backend via the `model` field (`"kokoro"` or `"chatterbox"`).

### curl — Kokoro TTS

```bash
curl https://llm.menezmethod.com/v1/audio/speech \
  -H "Authorization: Bearer sk-your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "kokoro",
    "input": "Hello, this is a test of the Kokoro TTS engine.",
    "voice": "af_bella",
    "response_format": "wav"
  }' --output speech.wav
```

### curl — Chatterbox TTS (no voice field)

```bash
curl https://llm.menezmethod.com/v1/audio/speech \
  -H "Authorization: Bearer sk-your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "chatterbox",
    "input": "Hello, this is a test of Chatterbox.",
    "response_format": "wav"
  }' --output speech.wav
```

### Python — TTS

```python
from openai import OpenAI

client = OpenAI(
    base_url="https://llm.menezmethod.com/v1",
    api_key="sk-your-api-key",
)

# Kokoro TTS
response = client.audio.speech.create(
    model="kokoro",
    voice="af_bella",
    input="Hello from Kokoro!",
    response_format="wav",
)
response.stream_to_file("speech.wav")

# Chatterbox TTS (no voice parameter)
response = client.audio.speech.create(
    model="chatterbox",
    input="Hello from Chatterbox!",
    response_format="wav",
)
response.stream_to_file("speech_chatterbox.wav")
```

### Node.js — TTS

```javascript
import OpenAI from "openai";
import fs from "fs";

const client = new OpenAI({
  baseURL: "https://llm.menezmethod.com/v1",
  apiKey: "sk-your-api-key",
});

// Kokoro TTS
const response = await client.audio.speech.create({
  model: "kokoro",
  voice: "af_bella",
  input: "Hello from Kokoro!",
  response_format: "wav",
});
const buffer = Buffer.from(await response.arrayBuffer());
fs.writeFileSync("speech.wav", buffer);
```

---

## Embeddings

### curl

```bash
curl https://llm.menezmethod.com/v1/embeddings \
  -H "Authorization: Bearer sk-your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "nomic-embed-text:latest",
    "input": "The quick brown fox jumps over the lazy dog."
  }'
```

### Python

```python
from openai import OpenAI

client = OpenAI(
    base_url="https://llm.menezmethod.com/v1",
    api_key="sk-your-api-key",
)

response = client.embeddings.create(
    model="nomic-embed-text:latest",
    input="The quick brown fox jumps over the lazy dog.",
)
print(response.data[0].embedding)
```

---

## List Models

### curl

```bash
curl https://llm.menezmethod.com/v1/models \
  -H "Authorization: Bearer sk-your-api-key"
```

### Python

```python
from openai import OpenAI

client = OpenAI(
    base_url="https://llm.menezmethod.com/v1",
    api_key="sk-your-api-key",
)

models = client.models.list()
for m in models.data:
    print(f"{m.id} ({m.owned_by})")
```

---

## Health Checks (no auth required)

### `GET /health` and `GET /health/status`

Same comprehensive response. Returns **200** if all services healthy, **503** if any service is degraded.

```json
{
  "status": "healthy",
  "version": "1.0.0",
  "timestamp": "2026-06-05T00:49:40Z",
  "services": {
    "ollama": {
      "status": "healthy",
      "models": [
        {"id": "gemma4:e4b", "object": "model", "owned_by": "ollama"},
        {"id": "nomic-embed-text:latest", "object": "model", "owned_by": "ollama"}
      ]
    },
    "kokoro": {
      "status": "healthy",
      "models": [
        {"id": "af_bella", "object": "voice", "owned_by": "af_bella"},
        {"id": "af_heart", "object": "voice", "owned_by": "af_heart"}
      ]
    },
    "chatterbox": {
      "status": "healthy",
      "models": [
        {"id": "chatterbox-default", "object": "voice", "owned_by": "chatterbox-default"}
      ]
    }
  },
  "summary": {
    "total": 3,
    "healthy": 3,
    "unhealthy": 0,
    "by_type": {
      "chat": 1,
      "tts": 2
    }
  }
}
```

### `GET /health/ready`

Returns **200** if all backends are reachable:

```json
{"status": "ready", "version": "1.0.0"}
```

Returns **503** with per-backend detail:

```json
{"status": "unavailable", "backend": "ollama", "error": "...", "version": "1.0.0"}
```

### `GET /version`

```json
{"version": "1.0.0", "commit": "abc1234"}
```

---

## Available Models

### Chat models (via Ollama on Mac M4 Max)

- `gemma4:e4b` — **default**
- `qwen3.5` (122b, 35b, 27b, 9b, 4b, 2b, 0.8b)
- `qwen3` (0.6b)
- `qwen3.6` (27b-code, 35b-code, 35a3b-mlx)
- `gpt-oss:20b-cloud`

### Embedding models

- `nomic-embed-text:latest`

### Vision models

- `moondream:1.8b`

---

## Errors

All errors follow the OpenAI error envelope format:

```json
{
  "error": {
    "message": "Human-readable description",
    "type": "error_category",
    "code": "machine_readable_code",
    "param": "field_name_if_applicable"
  }
}
```

| HTTP | Type | Code | Description |
|---|---|---|---|
| 400 | `invalid_request_error` | — | Malformed request or missing required field |
| 401 | `authentication_error` | `invalid_api_key` | Missing or invalid API key |
| 429 | `rate_limit_error` | `rate_limit_exceeded` | Per-key rate limit exceeded |
| 500 | `server_error` | — | Unexpected server failure |
| 503 | `backend_error` | `backend_unavailable` | Inference backend unreachable |

---

## Rate Limits

Requests are rate-limited per API key using a token-bucket algorithm.

| Header | Description |
|---|---|
| `X-RateLimit-Limit` | Maximum burst size |
| `X-RateLimit-Remaining` | Tokens remaining in the current window |
| `Retry-After` | Seconds to wait (returned with 429 responses) |

Default: 10 requests/second, burst of 20.

---

## Client SDKs

Any OpenAI-compatible SDK works by changing the `base_url` and `api_key`.

### Python (openai SDK)

```python
from openai import OpenAI

client = OpenAI(
    base_url="https://llm.menezmethod.com/v1",
    api_key="sk-your-api-key",
)
```

### Node.js (openai SDK)

```javascript
import OpenAI from "openai";

const client = new OpenAI({
  baseURL: "https://llm.menezmethod.com/v1",
  apiKey: "sk-your-api-key",
});
```

### Environment Variables

```bash
export OPENAI_BASE_URL=https://llm.menezmethod.com/v1
export OPENAI_API_KEY=sk-your-api-key
```

---

## Architecture (for context)

```
Internet → Cloudflare Tunnel → Pi5:80 → Traefik → inferencia:8080
                                                         │
                            ┌────────────────────────────┴────────────────────────────┐
                            ↓                                                         ↓
                   Ollama (:11434)                            Kokoro (:50051) / Chatterbox (:50052)
                   (chat, embed)                                       (TTS)
                   Mac M4 Max — 192.168.0.109
```

- **Raspberry Pi 5** — Runs only inferencia (Go binary, ~20 MB). No ML models.
- **Mac M4 Max (128 GB)** — All ML models: Ollama (`:11434`), Kokoro TTS (`:50051`), Chatterbox TTS (`:50052`).
- All backend communication is over the local LAN.

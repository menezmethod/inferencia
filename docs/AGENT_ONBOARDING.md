# inferencia API

OpenAI-compatible REST API for chat completions, embeddings, and model management. **inferencia** is hosted on Coolify. Default chat model: **mlx-community/gpt-oss-20b-MXFP4-Q8** (20B); use **mlx-community/gpt-oss-120b-MXFP4-Q8** (120B) when requested.

**Base URL**

```
https://llm.menezmethod.com/v1
```

**Interactive docs**

```
https://llm.menezmethod.com/docs
```

## Authentication

All `/v1/*` endpoints require a Bearer token in the `Authorization` header:

```
Authorization: Bearer YOUR_API_KEY
```

Contact the administrator to obtain an API key.

## Rate limits

Requests are rate-limited per API key using a token-bucket algorithm.

| Header | Description |
|--------|-------------|
| `X-RateLimit-Limit` | Maximum burst size |
| `X-RateLimit-Remaining` | Tokens remaining in the current window |
| `Retry-After` | Seconds to wait (returned with 429 responses) |

Default: 10 requests/second, burst of 20.

## Endpoints

| Endpoint | Method | Auth | Description |
|----------|--------|------|-------------|
| `/health` | GET | No | Liveness probe |
| `/health/ready` | GET | No | Readiness (backend connectivity) |
| `/metrics` | GET | No | Prometheus metrics |
| `/docs` | GET | No | Swagger UI |
| `/openapi.yaml` | GET | No | OpenAPI 3.1 spec |
| `/v1/models` | GET | Bearer | List available models |
| `/v1/chat/completions` | POST | Bearer | Chat completions (streaming, tool calling) |
| `/v1/embeddings` | POST | Bearer | Generate embeddings |

**Metrics and logging:** See [METRICS_AND_LOGGING.md](METRICS_AND_LOGGING.md) for setting up Prometheus metrics and logging.

---

### GET /v1/models

Returns available models from the inference backend.

```bash
curl https://llm.menezmethod.com/v1/models \
  -H "Authorization: Bearer YOUR_API_KEY"
```

```json
{
  "object": "list",
  "data": [
    { "id": "mlx-community/gpt-oss-120b-MXFP4-Q8", "object": "model", "created": 0, "owned_by": "mlx-knife-2.0" },
    { "id": "mlx-community/gpt-oss-20b-MXFP4-Q8", "object": "model", "created": 0, "owned_by": "mlx-knife-2.0" },
    { "id": "mlx-community/Llama-3.2-3B-Instruct-4bit", "object": "model", "created": 0, "owned_by": "mlx-knife-2.0" },
    { "id": "mlx-community/granite-3.3-2b-instruct-4bit", "object": "model", "created": 0, "owned_by": "mlx-knife-2.0" },
    { "id": "mlx-community/Qwen3-Embedding-4B-4bit-DWQ", "object": "model", "created": 0, "owned_by": "mlx-knife-2.0" }
  ]
}
```

### POST /v1/chat/completions

Generates a model response for the given conversation.

**Request body**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `model` | string | Yes | Model ID from `/v1/models` |
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

**Example — simple** (default: 20B)

```bash
curl https://llm.menezmethod.com/v1/chat/completions \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "mlx-community/gpt-oss-20b-MXFP4-Q8",
    "messages": [{"role": "user", "content": "What is 2+2?"}],
    "max_tokens": 100
  }'
```

Use `mlx-community/gpt-oss-120b-MXFP4-Q8` in the request body when you want the 120B model.

```json
{
  "id": "chatcmpl-abc123",
  "object": "chat.completion",
  "created": 1677858242,
  "model": "mlx-community/gpt-oss-20b-MXFP4-Q8",
  "choices": [{
    "index": 0,
    "message": { "role": "assistant", "content": "2+2 equals 4." },
    "finish_reason": "stop"
  }],
  "usage": { "prompt_tokens": 13, "completion_tokens": 7, "total_tokens": 20 }
}
```

**Example — streaming**

```bash
curl https://llm.menezmethod.com/v1/chat/completions \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "mlx-community/gpt-oss-20b-MXFP4-Q8",
    "messages": [{"role": "user", "content": "Hello!"}],
    "stream": true
  }'
```

Responds with `Content-Type: text/event-stream`. Each event is `data: {json}\n\n`. The final event is `data: [DONE]\n\n`.

**Example — tool calling**

```bash
curl https://llm.menezmethod.com/v1/chat/completions \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "mlx-community/gpt-oss-20b-MXFP4-Q8",
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

### POST /v1/embeddings

Generates embedding vectors for text input.

**Request body**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `model` | string | Yes | Embedding model ID |
| `input` | string\|array | Yes | Text or array of texts to embed |
| `encoding_format` | string | No | `"float"` (default) or `"base64"` |

```bash
curl https://llm.menezmethod.com/v1/embeddings \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "mlx-community/Qwen3-Embedding-4B-4bit-DWQ",
    "input": "The quick brown fox jumps over the lazy dog."
  }'
```

### GET /health

Liveness probe. Returns 200 if the server is running. No authentication required.

### GET /health/ready

Readiness probe. Returns 200 only when all backends are reachable. No authentication required.

## Client SDKs

Any OpenAI-compatible SDK works by changing the base URL.

### Python

```python
from openai import OpenAI

client = OpenAI(
    base_url="https://llm.menezmethod.com/v1",
    api_key="YOUR_API_KEY",
)

response = client.chat.completions.create(
    model="mlx-community/gpt-oss-20b-MXFP4-Q8",
    messages=[{"role": "user", "content": "Hello!"}],
)
print(response.choices[0].message.content)
```

### Node.js

```javascript
import OpenAI from "openai";

const client = new OpenAI({
  baseURL: "https://llm.menezmethod.com/v1",
  apiKey: "YOUR_API_KEY",
});

const response = await client.chat.completions.create({
  model: "mlx-community/gpt-oss-20b-MXFP4-Q8",
  messages: [{ role: "user", content: "Hello!" }],
});
console.log(response.choices[0].message.content);
```

### Environment variables

For any client that reads the standard OpenAI env vars:

```bash
export OPENAI_BASE_URL=https://llm.menezmethod.com/v1
export OPENAI_API_KEY=YOUR_API_KEY
```

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
|------|------|------|-------------|
| 400 | `invalid_request_error` | — | Malformed request or missing required field |
| 401 | `authentication_error` | `invalid_api_key` | Missing or invalid API key |
| 429 | `rate_limit_error` | `rate_limit_exceeded` | Per-key rate limit exceeded |
| 500 | `server_error` | — | Unexpected server failure |
| 503 | `backend_error` | `backend_unavailable` | Inference backend unreachable |

## OpenAPI specification

The full OpenAPI 3.1 spec is available at:

- **YAML**: [https://llm.menezmethod.com/openapi.yaml](https://llm.menezmethod.com/openapi.yaml)
- **Swagger UI**: [https://llm.menezmethod.com/docs](https://llm.menezmethod.com/docs)

# inferencia — Roadmap

Phased plan for closing feature gaps. Each phase is self-contained and shippable.
Agents: pick the next `[ ]` item, implement it, test it, and check it off.

---

## Phase 1 — Core protocol completeness

Fill gaps in the OpenAI chat completions contract so every mainstream SDK works without surprises.

- [ ] **Add `logprobs` and `seed` to ChatRequest** — add fields to `backend.ChatRequest`, pass through to backend
- [ ] **Stream usage reporting** — parse the final streaming chunk for `usage` data; record in metrics (currently only non-streaming tracks tokens)
- [ ] **Add `stream_options` to ChatRequest** — support `include_usage: true` per OpenAI spec
- [ ] **Legacy completions endpoint** — `POST /v1/completions` (non-chat) for clients that still use it

## Phase 2 — Multi-backend & routing

Move from single-backend proxy to smart routing across multiple backends.

- [ ] **Ollama backend adapter** — implement `backend.Ollama` (currently stubbed); translate between Ollama native API and OpenAI format
- [ ] **Model-to-backend routing** — route requests to the backend that serves the requested model (e.g. `llama` → Ollama, `gpt-oss` → MLX)
- [ ] **Backend failover** — if primary backend is down, try the next healthy one
- [ ] **Health-based routing** — skip backends where `Health()` fails; combine with readiness probe
- [ ] **Backend load balancing** — round-robin or least-connections across backends of the same type

## Phase 3 — Auth & key management

Harden auth for multi-tenant use without requiring a full database.

- [ ] **Hot-reload API keys** — watch `keys.txt` or poll `INFERENCIA_API_KEYS` env without restart
- [ ] **Per-key rate limits** — allow different keys to have different rate limits (e.g. `sk-admin:100rps`, `sk-agent:10rps`)
- [ ] **Per-key model restrictions** — restrict which models a key can access
- [ ] **Per-key usage tracking** — track and expose token/request counts per key (in metrics and/or a `/v1/usage` endpoint)
- [ ] **Key expiration** — support optional TTL on keys

## Phase 4 — Observability hardening

Make metrics and logging production-complete.

- [ ] **Per-key metrics labels** — add masked `api_key` label to Prometheus counters (watch cardinality)
- [ ] **Streaming token estimation** — estimate tokens from streaming chunks when backend doesn't report usage
- [ ] **Grafana Loki alerting rules** — add LogQL-based alert rules (e.g. `rate({service="inferencia"} | json | status=500 [5m]) > 0.1`)
- [ ] **OpenTelemetry tracing** — optional OTLP exporter for distributed tracing (behind `INFERENCIA_OTEL_ENDPOINT` env flag)
- [ ] **Audit log** — optional separate log stream for auth events (key used, key rejected, key rate-limited)

## Phase 5 — Extended API surface

Add endpoints that agents and apps commonly need beyond chat.

- [ ] **Vision passthrough** — test and document multimodal (image) content parts in messages
- [ ] **Audio transcription proxy** — `POST /v1/audio/transcriptions` forwarding to Whisper-compatible backends
- [ ] **Image generation proxy** — `POST /v1/images/generations` forwarding to Stable Diffusion or similar
- [ ] **Moderation proxy** — `POST /v1/moderations` for content safety checks

## Phase 6 — Operational maturity

Features for running inferencia reliably in production long-term.

- [ ] **Config hot-reload** — reload `config.yaml` on SIGHUP without downtime
- [ ] **Graceful backend drain** — when a backend goes down, finish in-flight requests before marking unavailable
- [ ] **Admin API** — `POST /admin/keys` (add/remove keys), `GET /admin/stats` (live stats), protected by a separate admin token
- [ ] **Prometheus service discovery** — support file-based or DNS SD for dynamic backend lists
- [ ] **Helm chart / Kubernetes manifests** — for k8s deployments

---

## How to use this file

**For agents (AI or human):**

1. Read this file to understand what's done and what's next
2. Pick the next unchecked `[ ]` item in the lowest incomplete phase
3. Implement it on a branch, with tests
4. Update this file: change `[ ]` to `[x]` for completed items
5. Open a PR or commit to main

**Priorities:** Phases are ordered by impact. Complete Phase 1 before Phase 2, etc.
Within a phase, items are roughly ordered by importance but can be done in any order.

**Adding new items:** Append to the appropriate phase. If it doesn't fit, add a new phase at the end.

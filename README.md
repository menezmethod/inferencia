# inferencia

A small Go gateway that puts an OpenAI-compatible API in front of self-hosted LLM and TTS backends, with API-key auth, rate limiting, and Prometheus metrics. It is designed to run on small self-hosted hardware such as a Raspberry Pi, with the model servers on another machine on the same network.

[![CI](https://img.shields.io/github/actions/workflow/status/menezmethod/inferencia/ci.yml?branch=main&logo=github&label=CI)](https://github.com/menezmethod/inferencia/actions)
[![Go Version](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-blue)](LICENSE)

## What it does

- Serves `POST /v1/chat/completions`, `POST /v1/embeddings`, `GET /v1/models`, and `POST /v1/audio/speech`.
- Backends: Ollama and MLX servers for chat and embeddings, and Kokoro-style HTTP servers for TTS.
- Routes each request by capability (chat vs. TTS) and requested model, skips backends the health checker marks degraded, and picks the backend with the fewest in-flight requests (ties are round-robin).
- Bearer-token auth from a keys file or `INFERENCIA_API_KEYS`, plus a token-bucket rate limiter.
- Health and version endpoints (`/health`, `/health/ready`, `/version`), Prometheus metrics at `/metrics`, structured JSON logs, optional OpenTelemetry tracing, and Swagger UI at `/docs`.
- Docker image and a Compose file written for Coolify. Grafana, Loki, and alert rules live under `deploy/`.

## Quick start

```bash
git clone https://github.com/menezmethod/inferencia.git
cd inferencia

cp config.example.yaml config.yaml   # backend URLs, rate limits
cp keys.example.txt keys.txt         # one API key per line
make build
./inferencia -config config.yaml
```

Or with Docker: `cp .env.example .env`, set your backend URLs and keys, then `docker compose up -d`.

## Development

```bash
make test    # go test -race -count=1 ./...
make lint    # go vet, then golangci-lint if installed
```

The unit tests use Ginkgo/Gomega. CI (`.github/workflows/ci.yml`) runs build, tests with the race detector, vet, golangci-lint, a sensitive-data check, gitleaks, and an integration job with Docker and Compose smoke tests. Merging to `main` triggers a Coolify deploy, so those checks are required by branch protection; see [docs/PUBLISHING.md](docs/PUBLISHING.md).

## Documentation

- [docs/openapi.yaml](docs/openapi.yaml): API spec (also served at `/openapi.yaml`)
- [docs/AGENT_ONBOARDING.md](docs/AGENT_ONBOARDING.md): client setup examples
- [docs/METRICS_AND_LOGGING.md](docs/METRICS_AND_LOGGING.md): metrics and log fields
- [ROADMAP.md](ROADMAP.md): what is done and what is not
- [AGENTS.md](AGENTS.md), [CLAUDE.md](CLAUDE.md): context files for AI coding assistants used on this repo
- [SECURITY.md](SECURITY.md)

## License

MIT, see [LICENSE](LICENSE).

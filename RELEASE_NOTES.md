# inferencia v1.0.0

First official production release. 🚀

## Features

- **OpenAI-compatible API** — `/v1/chat/completions`, `/v1/models`, `/v1/embeddings`; works with OpenAI SDKs, LangChain, and any compatible client.
- **MLX backend** — Default model: `mlx-community/gpt-oss-20b-MXFP4-Q8` (20B); request `mlx-community/gpt-oss-120b-MXFP4-Q8` for 120B.
- **Auth** — API keys via file or `INFERENCIA_API_KEYS`; optional key per request.
- **Rate limiting** — Token bucket (configurable RPS and burst).
- **Observability** — Prometheus metrics (`/metrics`), structured logging (JSON, optional GCP format), optional OpenTelemetry tracing.
- **Health & version** — `GET /health` and `GET /health/ready` include `version`; `GET /version` returns `{"version":"1.0.0"}` so you can see which build is running.
- **Docs** — Swagger UI at `/docs`, OpenAPI YAML at `/openapi.yaml`; [AGENT_ONBOARDING.md](docs/AGENT_ONBOARDING.md) for client setup.
- **Deploy** — Coolify-ready Docker image; main auto-deploys with branch protection so only CI-passing code is merged.

## CI/CD

- GitHub Actions: Build & test, Lint, Integration, optional Connectivity; branch protection requires all checks before merge.
- Production smoke workflow (optional) and [PUBLISHING.md](docs/PUBLISHING.md) for going public and Coolify setup.

## Upgrade / install

- **Docker**: `docker pull ghcr.io/menezmethod/inferencia:1.0.0` (or build from source with `VERSION=1.0.0`).
- **From source**: `make build VERSION=1.0.0` or use the release tarball.

**Full changelog**: See [commits on main](https://github.com/menezmethod/inferencia/commits/main) for this release.

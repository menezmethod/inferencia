# inferencia

A lightweight, secure AI gateway that exposes local LLM and TTS servers to the internet through an OpenAI-compatible API.

[![CI](https://img.shields.io/github/actions/workflow/status/menezmethod/inferencia/ci.yml?branch=main&logo=github&label=CI)](https://github.com/menezmethod/inferencia/actions)
[![Go Version](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-blue)](LICENSE)

Run models on your own hardware. Access them from anywhere. Supports Ollama, LLaMA.cpp, and Kokoro TTS backends with smart routing, observability, and Coolify deployment.

## 🚀 Quick Start

```bash
# Clone
git clone https://github.com/menezmethod/inferencia.git
cd inferencia

# Build
go build -o inferencia .

# Configure
cp .env.example .env
# Set your backend URLs and API keys

# Run
./inferencia

# Or with Docker
docker compose up -d
```

## 🏗️ Architecture

Inferencia is a Go HTTP server that acts as a universal AI gateway, routing requests to configured LLM (Ollama, LLaMA.cpp) and TTS (Kokoro) backends. It exposes an OpenAI-compatible `/v1/chat/completions` endpoint plus custom endpoints for TTS, health monitoring, and model inventory. Smart routing selects the optimal backend per request kind (chat vs. TTS) with configurable fallback chains. The server includes Prometheus metrics, structured logging with Loki, OpenTelemetry tracing, and a comprehensive health check system.

### Key Components

- **Smart Router** — Per-kind backend interfaces (LLM vs TTS) with configurable fallback chains
- **Health System** — Comprehensive `/health` endpoint with model/voice inventory and per-service breakdown
- **Observability** — Prometheus metrics, Grafana dashboards, Loki log aggregation, OpenTelemetry tracing
- **OpenAPI** — Full OpenAPI 3.1 spec with Swagger UI
- **Coolify Ready** — Docker Compose with env substitution, production image

## 🤖 Auto-Pipeline

This repo is part of an autonomous fleet. PRs are auto-reviewed, auto-tested, and auto-merged by the fleet pipeline.

## 📚 Documentation

- [PRD](./PRD.md) — Product Requirements Document
- [Architecture (RICO)](./RICO.md) — Architecture decision records
- [OpenAPI Spec](./api/openapi.yaml) — API documentation
- [AGENTS.md](./AGENTS.md) — Agent instructions
- [ROADMAP.md](./ROADMAP.md) — Development roadmap

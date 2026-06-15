#!/usr/bin/env bash
# Validate docker-compose.yaml for Coolify: internal expose only, no host port 8080 bind.
# Usage: from repo root, ./scripts/check-docker-compose.sh
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

COMPOSE_FILE="docker-compose.yaml"

if [[ ! -f "$COMPOSE_FILE" ]]; then
  echo "check-docker-compose: $COMPOSE_FILE not found"
  exit 1
fi

echo "--- docker compose config ---"
export INFERENCIA_BACKEND_URL="${INFERENCIA_BACKEND_URL:-http://127.0.0.1:11434}"
export INFERENCIA_API_KEYS="${INFERENCIA_API_KEYS:-sk-compose-check}"
docker compose -f "$COMPOSE_FILE" config -q

echo "--- Coolify port policy ---"
if grep -E '^[[:space:]]*-[[:space:]]*"?8080:8080"?' "$COMPOSE_FILE"; then
  echo "FAIL: $COMPOSE_FILE publishes host port 8080:8080 (conflicts with coolify-proxy/Traefik on Pi)"
  exit 1
fi

if ! grep -qE '^[[:space:]]*expose:' "$COMPOSE_FILE"; then
  echo "FAIL: $COMPOSE_FILE must declare expose (Coolify routes via Traefik, no host bind)"
  exit 1
fi

if ! grep -qE '8080' "$COMPOSE_FILE"; then
  echo "FAIL: $COMPOSE_FILE must expose container port 8080"
  exit 1
fi

if ! grep -q '/version' "$COMPOSE_FILE"; then
  echo "FAIL: healthcheck should use GET /version (liveness; /health returns 503 when backends are down)"
  exit 1
fi

echo "docker-compose Coolify checks passed."

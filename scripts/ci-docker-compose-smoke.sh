#!/usr/bin/env bash
# Build and run docker-compose.yaml without host port publish; verify healthcheck (/version).
# Simulates Coolify: Traefik reaches the service on the internal network.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

PROJECT="${COMPOSE_PROJECT_NAME:-inferencia-ci-compose}"
export INFERENCIA_BACKEND_URL="${INFERENCIA_BACKEND_URL:-http://127.0.0.1:11434}"
export INFERENCIA_API_KEYS="${INFERENCIA_API_KEYS:-sk-ci-compose-test}"

cleanup() {
  docker compose -p "$PROJECT" -f docker-compose.yaml down -v --remove-orphans 2>/dev/null || true
}
trap cleanup EXIT

echo "--- docker compose build ---"
docker compose -p "$PROJECT" -f docker-compose.yaml build

echo "--- docker compose up (--wait for healthcheck) ---"
docker compose -p "$PROJECT" -f docker-compose.yaml up -d --wait --wait-timeout 120

echo "--- in-container /version ---"
docker compose -p "$PROJECT" -f docker-compose.yaml exec -T inferencia \
  wget -qO- http://127.0.0.1:8080/version

echo "--- network reachability (no host port publish) ---"
network="$(docker compose -p "$PROJECT" -f docker-compose.yaml ps -q inferencia | xargs docker inspect -f '{{range $k, $v := .NetworkSettings.Networks}}{{$k}}{{end}}')"
docker run --rm --network "$network" curlimages/curl:8.12.1 -sf "http://inferencia:8080/version" >/dev/null

echo "docker compose smoke passed."

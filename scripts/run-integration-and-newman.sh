#!/usr/bin/env bash
# Start the app, run Ginkgo integration tests and (optionally) Newman, then stop the app.
# Usage: from repo root, ./scripts/run-integration-and-newman.sh
# Set SKIP_NEWMAN=1 to skip Newman (e.g. if Node is not installed).
set -e

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

PORT="${INTEGRATION_PORT:-18080}"
BASE_URL="http://127.0.0.1:${PORT}"
BINARY_NAME="inferencia_integration_test"

echo "--- Copy OpenAPI spec ---"
cp -f docs/openapi.yaml internal/openapi/spec.yaml

echo "--- Build binary ---"
go build -o "$BINARY_NAME" ./cmd/inferencia

echo "--- Start app on port $PORT ---"
INFERENCIA_HOST=127.0.0.1 INFERENCIA_PORT="$PORT" INFERENCIA_API_KEYS=sk-integration-test "./$BINARY_NAME" &
APP_PID=$!
cleanup() {
  echo "--- Stop app ---"
  kill "$APP_PID" 2>/dev/null || true
  wait "$APP_PID" 2>/dev/null || true
  rm -f "$BINARY_NAME"
}
trap cleanup EXIT

echo "--- Wait for health ---"
for i in {1..30}; do
  if curl -sf "$BASE_URL/health" >/dev/null; then
    break
  fi
  if [[ $i -eq 30 ]]; then
    echo "App did not become healthy"
    exit 1
  fi
  sleep 0.5
done

echo "--- Run Ginkgo integration tests ---"
INTEGRATION_BASE_URL="$BASE_URL" go test -v ./integration/...

if [[ "${SKIP_NEWMAN:-0}" != "1" ]]; then
  if command -v npx &>/dev/null; then
    echo "--- Run Newman ---"
    npx --yes newman run postman/inferencia-api.postman_collection.json \
      -e postman/inferencia-integration.postman_environment.json \
      --env-var "baseUrl=$BASE_URL" \
      --reporters cli
  else
    echo "--- Skipping Newman (npx not found); set SKIP_NEWMAN=1 to suppress) ---"
  fi
else
  echo "--- Skipping Newman (SKIP_NEWMAN=1) ---"
fi

echo "--- Done ---"

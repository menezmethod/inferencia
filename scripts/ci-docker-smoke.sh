#!/usr/bin/env bash
# Smoke-test a running inferencia container (CI or local). Expects container on localhost:PORT.
# Usage: PORT=8080 ./scripts/ci-docker-smoke.sh
set -euo pipefail

PORT="${PORT:-8080}"
BASE="http://127.0.0.1:${PORT}"

echo "--- Docker smoke: $BASE ---"

curl -sf "$BASE/version" >/dev/null
echo "OK   GET /version"

curl -sf "$BASE/metrics" | head -5 >/dev/null
echo "OK   GET /metrics"

status=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/docs")
test "$status" = "200" || { echo "FAIL GET /docs: HTTP $status"; exit 1; }
echo "OK   GET /docs HTTP $status"

status=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/openapi.yaml")
test "$status" = "200" || { echo "FAIL GET /openapi.yaml: HTTP $status"; exit 1; }
echo "OK   GET /openapi.yaml HTTP $status"

status=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/v1/models")
test "$status" = "401" || { echo "FAIL GET /v1/models (no auth): expected 401, got $status"; exit 1; }
echo "OK   GET /v1/models (no auth) HTTP $status"

status=$(curl -s -o /dev/null -w "%{http_code}" -H "Authorization: Bearer ${INFERENCIA_API_KEYS:-sk-ci-test-key}" "$BASE/v1/models")
test "$status" = "503" || test "$status" = "200" || {
  echo "FAIL GET /v1/models (auth): expected 503 or 200, got $status"
  exit 1
}
echo "OK   GET /v1/models (auth) HTTP $status"

echo "Docker smoke passed."

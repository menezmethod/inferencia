#!/usr/bin/env bash
# Smoke test production (https://llm.menezmethod.com).
# No auth: health, ready, metrics. With INFERENCIA_E2E_API_KEY: /v1/models (optional: chat).
set -e

# Require explicit base URL so we never accidentally hit a real deployment from CI/clones
if [[ -z "${INFERENCIA_SMOKE_BASE_URL:-}" ]]; then
  echo "Set INFERENCIA_SMOKE_BASE_URL to the inferencia base URL (e.g. https://your-inferencia.example.com)"
  exit 1
fi
BASE_URL="$INFERENCIA_SMOKE_BASE_URL"
API_KEY="${INFERENCIA_E2E_API_KEY:-}"

echo "Smoke testing $BASE_URL"
echo "---"

check() {
  local method="$1"
  local path="$2"
  local want="$3"
  local url="${BASE_URL}${path}"
  local status
  if [[ -n "$API_KEY" && "$path" == /v1/* ]]; then
    status=$(curl -s -o /dev/null -w "%{http_code}" -X "$method" -H "Authorization: Bearer $API_KEY" "$url")
  else
    status=$(curl -s -o /dev/null -w "%{http_code}" -X "$method" "$url")
  fi
  if [[ "$status" != "$want" ]]; then
    echo "FAIL $method $path: got HTTP $status, want $want"
    return 1
  fi
  echo "OK   $method $path HTTP $status"
  return 0
}

failed=0

check GET /health 200 || failed=1
check GET /health/ready 200 || failed=1
check GET /metrics 200 || failed=1
check GET /docs 200 || failed=1
check GET /openapi.yaml 200 || failed=1

if [[ -n "$API_KEY" ]]; then
  check GET /v1/models 200 || failed=1
  echo "OK   /v1/models (with API key)"
else
  echo "Skip /v1/models (set INFERENCIA_E2E_API_KEY to include)"
fi

echo "---"
if [[ $failed -eq 0 ]]; then
  echo "Smoke test passed."
  exit 0
else
  echo "Smoke test failed."
  exit 1
fi

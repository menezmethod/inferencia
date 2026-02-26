#!/usr/bin/env bash
# Blocklist check: fail if any tracked file contains strings that must not be committed.
# Use placeholders in docs: your-inferencia.example.com, sk-your-secret-key, 192.168.0.x
# Add new blocklisted patterns here (one per line; no leading/trailing spaces).
set -e

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

BLOCKLIST=(
  # Real production / internal hostnames (use your-inferencia.example.com in docs)
  "llm.menezmethod.com"
  "cp.menezmethod.com"
)

failed=0
for pattern in "${BLOCKLIST[@]}"; do
  matches=$(git grep -l -- "$pattern" 2>/dev/null || true)
  if [[ -n "$matches" ]]; then
    echo "Sensitive data check FAILED: blocklisted string '$pattern' found in:"
    echo "$matches"
    failed=1
  fi
done

if [[ $failed -eq 1 ]]; then
  echo ""
  echo "Use placeholders in docs: your-inferencia.example.com, sk-your-secret-key, 192.168.0.x"
  exit 1
fi
echo "Sensitive data check passed (no blocklisted strings in docs)."
exit 0

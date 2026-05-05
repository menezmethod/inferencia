#!/usr/bin/env bash
# Blocklist check: fail if any tracked file contains patterns from SENSITIVE_BLOCKLIST.
# Blocklist is NOT stored in repo (public-safe). Set GitHub secret SENSITIVE_BLOCKLIST
# (newline-separated patterns) to enable. When unset, check is skipped and passes.
# On failure we never echo the pattern or matched content (masked for public CI logs).
set -e

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

if [[ -z "${SENSITIVE_BLOCKLIST:-}" ]]; then
  echo "Sensitive data check: no blocklist configured (set SENSITIVE_BLOCKLIST secret to enable). Skipping."
  exit 0
fi

failed=0
while IFS= read -r pattern || [[ -n "$pattern" ]]; do
  pattern=$(echo "$pattern" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
  [[ -z "$pattern" ]] && continue
  matches=$(git grep -l -- "$pattern" 2>/dev/null || true)
  if [[ -n "$matches" ]]; then
    echo "Sensitive data check FAILED: blocklisted pattern matched in:"
    echo "$matches"
    echo "[Match redacted for security. Use placeholders: your-inferencia.example.com, sk-your-secret-key.]"
    failed=1
  fi
done <<< "$SENSITIVE_BLOCKLIST"

if [[ $failed -eq 1 ]]; then
  exit 1
fi
echo "Sensitive data check passed."
exit 0

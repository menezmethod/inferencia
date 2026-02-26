#!/usr/bin/env bash
# Configure the GitHub repo: branch protection, secrets (from .env.gh.secrets or env), workflow check.
# Requires: gh CLI installed and authenticated (gh auth login).
# Optional: copy .env.gh.secrets.example to .env.gh.secrets and fill values, or pass env vars.
set -e

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

echo "Setting up repo (run from repo root)..."

# 0. Ensure gh is authenticated and we're in a repo
echo "--- Checking gh auth and repo ---"
gh auth status -h github.com 2>/dev/null || { echo "Run: gh auth login"; exit 1; }
gh repo view --json name -q .name >/dev/null 2>/dev/null || { echo "Not a GitHub repo or gh cannot see it."; exit 1; }
echo "Repo: $(gh repo view --json nameWithOwner -q .nameWithOwner)"

# 1. Branch protection: require CI to pass before merging to main
echo "--- Branch protection for main ---"
VISIBILITY=$(gh repo view --json visibility -q .visibility 2>/dev/null || echo "unknown")
if [[ "$VISIBILITY" != "PUBLIC" ]]; then
  echo "Repo is $VISIBILITY. Branch protection requires a public repo (free) or GitHub Team/Pro (private)."
  echo "To enable: gh repo edit --visibility public   then re-run this script."
fi
echo '{"required_status_checks":{"strict":true,"contexts":["Build & test","Lint","Integration","Sensitive data"]},"enforce_admins":true,"required_pull_request_reviews":null,"restrictions":null,"allow_force_pushes":false,"allow_deletions":false}' \
  | gh api repos/:owner/:repo/branches/main/protection -X PUT -H "Accept: application/vnd.github+json" --input - \
  --silent 2>/dev/null || {
  echo "Branch protection failed (private repo on free tier, or status checks not yet run)."
  echo "  If private: make repo public (gh repo edit --visibility public) or upgrade to GitHub Team/Pro."
  echo "  Or set manually: Settings → Branches → Add rule for 'main' → Require status checks: Build & test, Lint, Integration, Sensitive data"
}

# 2. Vulnerability alerts (Dependabot)
echo "--- Enabling vulnerability alerts ---"
gh api repos/:owner/:repo/vulnerability-alerts -X PUT --silent || true

# 3. Repository secrets from .env.gh.secrets or environment
echo "--- Repository secrets ---"
SECRET_NAMES=(COOLIFY_WEBHOOK COOLIFY_TOKEN INFERENCIA_SMOKE_BASE_URL INFERENCIA_E2E_API_KEY INFERENCIA_CI_BACKEND_URL INFERENCIA_CI_API_KEYS)
if [[ -f .env.gh.secrets ]]; then
  set -a
  source .env.gh.secrets
  set +a
  echo "Loaded .env.gh.secrets"
fi
for name in "${SECRET_NAMES[@]}"; do
  val="${!name:-}"
  if [[ -n "$val" ]]; then
    echo -n "Setting $name... "
    if gh secret set "$name" --body "$val" 2>/dev/null; then
      echo "OK"
    else
      echo "Failed (check gh auth and repo)"
    fi
  fi
done
if [[ ! -f .env.gh.secrets ]]; then
  echo "Tip: copy .env.gh.secrets.example to .env.gh.secrets and add values, then re-run this script to set COOLIFY_WEBHOOK, etc."
fi

# 4. Verify workflows exist
echo "--- Workflows ---"
if gh workflow list 2>/dev/null | grep -q "CI"; then
  echo "CI workflow: present"
else
  echo "CI workflow not found. Push .github/workflows/ci.yml and re-run."
fi
if gh workflow list 2>/dev/null | grep -q "Smoke"; then
  echo "Smoke (production) workflow: present"
fi

# 5. Optional: trigger CI run to verify (comment out if you prefer manual only)
# echo "--- Triggering CI run ---"
# gh workflow run "CI" 2>/dev/null && echo "CI run triggered. Check: gh run list --workflow=ci.yml" || true

echo ""
echo "Done. Next steps:"
echo "  1. Deploy-after-CI: leave Coolify Auto Deploy ON; branch protection ensures only CI-passing code reaches main."
echo "  2. Optional: set INFERENCIA_SMOKE_BASE_URL (and INFERENCIA_E2E_API_KEY) for production smoke workflow."
echo "  3. To make repo public: gh repo edit --visibility public"
echo "  4. To trigger CI now: gh workflow run \"CI\""

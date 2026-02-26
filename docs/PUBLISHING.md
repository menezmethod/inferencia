# Publishing the repo as public (open source)

This guide covers sanitizing the repository and configuring GitHub so it is safe to make public and acts as the source of truth for deployment.

**Reusable path for other apps:** For a single playbook that covers CI/CD, auto deploy (Coolify), sensitive-data checks, and branch protection so every app feels consistent, see [CI_CD_AND_HOSTING_PLAYBOOK.md](CI_CD_AND_HOSTING_PLAYBOOK.md).

---

## 1. Sanitization checklist

Before making the repo public, ensure no sensitive data exists in **current files** or **git history**.

### 1.1 Current files — must be clean

- [ ] **No real API keys** — Only placeholders in `keys.example.txt`, `config.example.yaml`, `.env.example`, `env.coolify.example` (e.g. `sk-inferencia-dev-key-change-me`, `sk-your-secret-key`, `sk-PASTE_YOUR_KEY_HERE`).
- [ ] **No real backend URLs** — Examples use `localhost:11973`, `host.docker.internal:11973`, or `http://YOUR_M4_LAN_IP:11973` / `192.168.0.x`, never a real LAN IP or hostname that points to your infra.
- [ ] **No real credentials** — No passwords, tokens, or secrets in any committed file. Grafana/Coolify examples use placeholders or default dev values with a comment to change them.
- [ ] **Config and secrets not tracked** — `config.yaml`, `keys.txt`, `.env`, `.env.local` are in `.gitignore` and never committed.

### 1.2 Git history — remove leaked secrets

If you ever committed a real key, backend URL, or password:

1. **Option A — Rewrite history (recommended if secrets were committed)**
   - Use [BFG Repo-Cleaner](https://rsc.io/bfg/) or [git-filter-repo](https://github.com/newren/git-filter-repo) to replace or remove sensitive strings from all commits.
   - Example (BFG): `bfg --replace-text passwords.txt` where `passwords.txt` lists strings to replace with `***REMOVED***`.
   - Then force-push: `git push --force origin main`. **Warning:** Rewrites history; coordinate with any collaborators and consider making a fresh clone after.

2. **Option B — Fresh history (simplest if repo is young)**
   - Create a new orphan branch with only the current tree (no history):
     ```bash
     git checkout --orphan main-clean
     git add -A
     git commit -m "Initial public release"
     git branch -D main
     git branch -m main
     git push -f origin main
     ```
   - All prior history is gone; no secrets can be recovered from it.

3. **After rewriting**
   - Rotate any credentials that ever appeared in history (API keys, passwords). Assume they are compromised.

### 1.3 What stays (safe for public)

- **Public gateway URL** (e.g. `https://your-inferencia.example.com`) — The docs can mention an example deployment URL; that is the public API, not your backend. Prefer a placeholder in generic docs and set the real URL only in your own deployment/config.
- **Package path** `github.com/menezmethod/inferencia` — Identifies the project; no secret.
- **OpenAPI / README examples** — Use placeholders for keys and backend URLs; example IPs like `192.168.0.x` are fine.

---

## 2. Repo setup with `gh` CLI

Install [GitHub CLI](https://cli.github.com/) and run `gh auth login`. Then run these from the repo root.

### 2.1 Visibility and description

```bash
# Make repo public (run when sanitization is done)
gh repo edit --visibility public

# Set description and topics
gh repo edit --description "OpenAI-compatible API gateway for local LLM servers (MLX, Ollama). Auth, rate limiting, observability."
gh repo edit --add-topic go --add-topic openai-api --add-topic llm --add-topic mlx --add-topic prometheus
```

### 2.2 Branch protection (main as source of truth)

**Note:** Branch protection (required status checks, block force-push/deletion) is **not available for private repos on the free tier**; it requires GitHub Team/Pro or a **public** repo. To use it for free, make the repo public first: `gh repo edit --visibility public`.

Require CI to pass before merging; prevent force-push and deletion of `main`:

```bash
# Require job names from .github/workflows/ci.yml (run after at least one CI run so these exist)
echo '{"required_status_checks":{"strict":true,"contexts":["Build & test","Lint","Integration","Sensitive data"]},"enforce_admins":true,"required_pull_request_reviews":null,"restrictions":null,"allow_force_pushes":false,"allow_deletions":false}' \
  | gh api repos/:owner/:repo/branches/main/protection -X PUT -H "Accept: application/vnd.github+json" --input -
```
Or use `./scripts/setup-repo.sh` to apply this and set secrets in one go (script will warn if repo is private and protection fails).

Required status checks (job names from `.github/workflows/ci.yml`): **Build & test**, **Lint**, **Integration**, and **Sensitive data**. All must pass before a PR can be merged. The Sensitive data job runs a blocklist check (patterns from **SENSITIVE_BLOCKLIST** secret; when unset the check is skipped and passes) and gitleaks; see `scripts/check-sensitive-data.sh` and `.gitleaks.toml`. On blocklist match, the pattern and matched content are never echoed (masked for public logs). (Do **not** require **Connectivity (backend)** unless you set `INFERENCIA_CI_BACKEND_URL` and want it as a gate.) **Trigger Coolify deploy** is skipped on PRs and runs only on push to `main`. Or set the same in the GitHub UI: **Settings → Branches → Add rule** for `main` → Require status checks → select the four jobs.

### 2.3 Security and dependency updates

```bash
# Enable Dependabot (see .github/dependabot.yml in repo)
gh api repos/:owner/:repo/vulnerability-alerts -X PUT

# Optional: enable code scanning (CodeQL) from GitHub Security tab
```

### 2.4 One-shot setup: branch protection + secrets + workflows

From the repo root, after `gh auth login`:

1. **Secrets** — Copy the example file and add your values (no quotes needed for simple values):
   ```bash
   cp .env.gh.secrets.example .env.gh.secrets
   # Edit .env.gh.secrets: optional COOLIFY_WEBHOOK/COOLIFY_TOKEN (only if using webhook-triggered deploy);
   # optional INFERENCIA_SMOKE_BASE_URL, INFERENCIA_E2E_API_KEY, INFERENCIA_CI_BACKEND_URL, INFERENCIA_CI_API_KEYS
   ```

2. **Run the setup script** — It configures branch protection, vulnerability alerts, and pushes secrets from `.env.gh.secrets` (or from current env vars) to the repo:
   ```bash
   ./scripts/setup-repo.sh
   ```
   Or set secrets via env and run (no file):
   ```bash
   INFERENCIA_SMOKE_BASE_URL="https://llm.yourdomain.com" ./scripts/setup-repo.sh
   ```

3. **Verify** — Ensure CI has run at least once (push a commit or run `gh workflow run "CI"`), then branch protection will apply. List workflows: `gh workflow list`. List runs: `gh run list --workflow=ci.yml`.

4. **Production smoke** — The **Smoke (production)** workflow runs on schedule (daily) and on **Run workflow** in the Actions tab. It uses `INFERENCIA_SMOKE_BASE_URL` and optional `INFERENCIA_E2E_API_KEY`; if the base URL secret is not set, the run is skipped.

---

## 3. CI/CD as source of truth

- **CI** (`.github/workflows/ci.yml`) runs on every push and pull request: **Build & test** (build, test with `-race`, vet), **Lint** (golangci-lint), **Sensitive data** (blocklist + gitleaks), **Integration** (build image, run container, curl health/metrics/docs and assert auth for `/v1/models`), and optionally **Connectivity (backend)** (see below). No production URL or secrets are required for CI unless you enable optional steps.
- **Branch protection** — Require **Build & test**, **Lint**, **Integration**, and **Sensitive data** so `main` only accepts changes that pass those checks. Do not require **Connectivity (backend)** unless you set the optional backend secret and want it as a merge gate.
- **Deploy only after checks** — **Coolify Auto Deploy** on `main` + **branch protection**: `main` always auto-deploys when it’s updated; the only way to update `main` is to merge a PR that passed CI (Build & test, Lint, Integration, Sensitive data). So deploys are always from CI-passing code. No webhook or secrets. See [Coolify: main auto-deploys](#coolify-main-auto-deploys) below.
- **API contract** — Handler and integration tests assert that `/v1/chat/completions`, `/v1/models`, and `/v1/embeddings` behave as documented; OpenAPI spec is the contract. Changes that break request/response shapes should be caught by tests before merge.
- **Production smoke** — The **Smoke (production)** workflow (`.github/workflows/smoke-prod.yml`) runs on schedule (daily) and via **workflow_dispatch**. Set **INFERENCIA_SMOKE_BASE_URL** (and optionally **INFERENCIA_E2E_API_KEY**) in repo secrets; if unset, the run is skipped. You can also run `scripts/smoke-prod.sh` locally or in Coolify post-deploy.

### Coolify: main auto-deploys

1. **In Coolify** — Leave **Auto Deploy** on for your inferencia app (deploy on push to the connected branch, e.g. `main`). Pushes to `main` trigger a deploy automatically.
2. **In GitHub** — Use **branch protection** on `main`: require status checks **Build & test**, **Lint**, **Integration**, and **Sensitive data** before merge. The only way code gets onto `main` is via a merged PR that passed CI, so Coolify only ever deploys CI-passing code. No webhook, no auth, no deploy-related secrets.

**Optional**  
- **Connectivity check** — Add **INFERENCIA_CI_BACKEND_URL** (and optionally **INFERENCIA_CI_API_KEYS**) in GitHub secrets so CI verifies the app can reach your backend before merge (and optionally require that job in branch protection).  
- **Health check** — In Coolify, set the application health check to `/health/ready` so failed backend connectivity marks the deployment unhealthy.

---

## 4. After going public

- Rotate any credentials that might have been in history or in private forks.
- Document in README that the project is open source and that users should deploy their own instance with their own backend URL and API keys.
- Point users to SECURITY.md for reporting vulnerabilities.

## 5. Releases

To cut an official release (e.g. v1.0.0) from `main`:

1. Ensure the version is set at build time: Dockerfile uses `ARG VERSION=dev`; for release builds pass `--build-arg VERSION=1.0.0`. Makefile: `make build VERSION=1.0.0`.
2. From repo root on `main` (after merge):  
   `git tag -a v1.0.0 -m "Release v1.0.0"`  
   `git push origin v1.0.0`
3. Create the GitHub release:  
   `gh release create v1.0.0 --notes-file RELEASE_NOTES.md`  
   Optionally attach binaries or use the tag’s source zip from the GitHub UI.
4. Deployed instances show the version in `GET /health` and `GET /version` (set at image build time).

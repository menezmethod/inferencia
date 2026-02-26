# CI/CD and hosting playbook

A reusable path for **CI/CD**, **auto deploy** (e.g. Coolify), and **sensitive-data safety** so every app feels consistent: green CI → merge → main → auto deploy, with no secrets in the repo and no accidental leaks in logs.

This repo (inferencia) is the reference implementation. Use this doc as a checklist when adding a new app or aligning an existing one.

---

## 1. Outcomes

- **CI** runs on every push and PR: build, test, integration (Docker smoke), and sensitive-data check.
- **Branch protection** on `main`: only PRs that pass CI can merge.
- **Auto deploy**: Coolify (or similar) deploys on push to `main`. Because only CI-passing code reaches `main`, every deploy is from a green build. No webhook required.
- **No secrets in repo**: blocklist and real URLs/keys live in GitHub secrets; placeholders only in docs and examples. On match, CI fails without echoing the pattern or content (safe for public logs).
- **Optional**: connectivity check (app → backend), production smoke (scheduled or manual), webhook-triggered deploy.

---

## 2. Sanitization (before going public or enabling CI)

- [ ] **No real API keys or tokens** in any file. Use placeholders: `sk-your-secret-key`, `YOUR_API_KEY`, `Bearer YOUR_API_KEY`, etc.
- [ ] **No real backend/prod URLs or hostnames** in repo. Use `localhost`, `host.docker.internal`, `your-app.example.com`, `192.168.0.x`.
- [ ] **Config and secrets not tracked**: `config.yaml`, `.env`, `keys.txt` in `.gitignore`; only `*.example` / `*.example.yaml` committed.
- [ ] **Git history**: if secrets were ever committed, rewrite history (BFG / git-filter-repo) or use an orphan branch, then rotate all affected credentials.

---

## 3. Sensitive-data pipeline (blocklist + gitleaks)

### 3.1 Blocklist check (custom patterns)

- **Script** (e.g. `scripts/check-sensitive-data.sh`):
  - Read patterns from **one env var** (e.g. `SENSITIVE_BLOCKLIST`), **never** from a file in the repo. This keeps the blocklist out of public view and avoids the script matching itself.
  - When the env var is unset: exit 0 (check skipped).
  - When set: newline-separated patterns; for each pattern, run `git grep -l` (or equivalent). If any file matches, **do not echo the pattern or the matched content** — only print a generic message and the list of file paths, then exit 1.
- **CI**: one job step that sets `SENSITIVE_BLOCKLIST: ${{ secrets.SENSITIVE_BLOCKLIST }}` and runs the script. Store the real blocklist (e.g. prod hostnames, internal URLs) in GitHub **Secrets** so it never appears in the repo or in logs.

### 3.2 Gitleaks (accidental secrets)

- **Config** (e.g. `.gitleaks.toml`):
  - `[extend] useDefault = true`.
  - **Allowlist**: add `paths` (regex) for example/placeholder files (e.g. `.*\.example\.yaml`, `docs/.*\.md` if those only contain placeholders). Add `regexes` for known placeholder strings (e.g. `Bearer YOUR_API_KEY`, `sk-your-`, `your-app\.example\.com`) so default rules (e.g. `curl-auth-header`) don’t flag docs.
- **CI**: run gitleaks in a **run** step (not only the gitleaks-action) so it works on PRs/shallow clones. Example:
  ```bash
  curl -sSfL https://github.com/zricethezav/gitleaks/releases/download/v8.24.3/gitleaks_8.24.3_linux_x64.tar.gz | tar -xz -C /tmp
  /tmp/gitleaks detect --no-git --redact -v --config .gitleaks.toml
  ```
- When gitleaks flags a **valid** placeholder (e.g. in onboarding docs), add that path or regex to the allowlist rather than disabling the rule.

---

## 4. CI workflow structure

Suggested jobs (names matter for branch protection):

| Job | Purpose |
|-----|--------|
| **Build & test** | Build, unit tests (e.g. `-race`), vet. No secrets required. |
| **Lint** | golangci-lint (or equivalent). No secrets required. |
| **Sensitive data** | Blocklist script + gitleaks. Uses `SENSITIVE_BLOCKLIST` secret; when unset, blocklist step is skipped. |
| **Integration** | Start app, run Ginkgo integration suite + Newman (Postman); then Docker smoke (build image, curl health/auth). No prod URL. |
| **Connectivity (backend)** | Optional. Run only when a secret (e.g. `APP_CI_BACKEND_URL`) is set; start app, hit health/ready to verify app → backend. |
| **Trigger Coolify deploy** | Optional. Only on push to `main`; call Coolify webhook if `COOLIFY_WEBHOOK` is set. Prefer **Coolify Auto Deploy** so this job is optional. |

- **Required for merge**: Build & test, Lint, Integration, Sensitive data. Do **not** require Connectivity unless you rely on it and set the backend secret.
- **On PR**: run Build & test, Lint, Sensitive data, Integration (and optionally Connectivity). Do **not** run the deploy step on PRs.

---

## 5. Branch protection

- **Branch**: `main` (or `master`).
- **Rules**: Require status checks **Build & test**, **Lint**, **Integration**, **Sensitive data**. Strict, no force-push, no deletion.
- **Note**: On GitHub free tier, branch protection with required status checks is available for **public** repos; for private you need GitHub Team/Pro or make the repo public.

Apply via API (after at least one CI run so the check names exist):

```bash
# Replace with your job names if different
echo '{"required_status_checks":{"strict":true,"contexts":["Build & test","Lint","Integration","Sensitive data"]},"enforce_admins":true,"required_pull_request_reviews":null,"restrictions":null,"allow_force_pushes":false,"allow_deletions":false}' \
  | gh api repos/:owner/:repo/branches/main/protection -X PUT -H "Accept: application/vnd.github+json" --input -
```

Or use a **setup script** (e.g. `scripts/setup-repo.sh`) that applies this and pushes secrets from `.env.gh.secrets`.

---

## 6. Auto deploy (Coolify)

- **Coolify**: Enable **Auto Deploy** for the app (deploy on push to the connected branch, e.g. `main`).
- **GitHub**: Branch protection ensures only CI-passing code is merged into `main`. So every push to `main` is already green → Coolify deploys that commit. No webhook or deploy secrets required.
- **Optional**: Health check in Coolify → e.g. `GET /health/ready` so failed backend connectivity marks the deployment unhealthy.

---

## 7. One-shot repo setup

1. **Secrets** (Settings → Secrets and variables → Actions, or `gh secret set`):
   - `SENSITIVE_BLOCKLIST` — newline-separated patterns (e.g. prod hostnames). When unset, blocklist check is skipped.
   - Optional: `COOLIFY_WEBHOOK`, `COOLIFY_TOKEN` (only if you use webhook-triggered deploy instead of Auto Deploy).
   - Optional: `APP_CI_BACKEND_URL`, `APP_CI_API_KEYS` for connectivity job.
   - Optional: `SMOKE_BASE_URL`, `E2E_API_KEY` for production smoke workflow.

2. **Setup script** (recommended):
   - Copy `.env.gh.secrets.example` → `.env.gh.secrets`, fill values, run `./scripts/setup-repo.sh` to apply branch protection and push secrets. Script should **not** contain the blocklist patterns; it only reads env (or `.env.gh.secrets`) and calls `gh secret set`.

3. **Verify**: Push a commit or run the CI workflow; confirm the three required checks appear, then branch protection will take effect.

---

## 8. Optional: production smoke

- **Workflow**: Scheduled (e.g. daily) + `workflow_dispatch`. If `SMOKE_BASE_URL` is unset, skip.
- **Script**: e.g. `scripts/smoke-prod.sh` that curls health/version and maybe one authenticated endpoint. Use placeholders in the script (or read URL/key from env); never commit real prod URL/key.

---

## 9. Version and health (for parity across apps)

- **Version**: Set at build time (e.g. Docker `ARG VERSION=dev`, ldflags). Expose `GET /version` and optionally include version in `GET /health` and `GET /health/ready`.
- **Health**: `GET /health` (liveness), `GET /health/ready` (readiness, e.g. backend reachable). Integration job and Coolify can use these.

---

## 10. Checklist for a new app

- [ ] Sanitize repo (placeholders only; history clean if needed).
- [ ] Add `scripts/check-sensitive-data.sh` (blocklist from env; mask on match).
- [ ] Add `.gitleaks.toml` (default rules + allowlist paths/regexes for docs and examples).
- [ ] CI workflow: Build & test, Lint, Sensitive data, Integration; optional Connectivity and deploy trigger.
- [ ] Branch protection: require Build & test, Lint, Integration, Sensitive data.
- [ ] Coolify: Auto Deploy on `main`; optional health check URL.
- [ ] Set `SENSITIVE_BLOCKLIST` (and optional secrets) in GitHub; run setup script if you have one.
- [ ] Optional: production smoke workflow + script; version/health endpoints.

---

## Reference in this repo

| Item | Location |
|------|----------|
| CI workflow | `.github/workflows/ci.yml` |
| Sensitive data script | `scripts/check-sensitive-data.sh` |
| Gitleaks config | `.gitleaks.toml` |
| Branch protection + secrets | `scripts/setup-repo.sh` |
| Secrets example | `.env.gh.secrets.example` |
| Publishing and releases | `docs/PUBLISHING.md` |
| Production smoke | `.github/workflows/smoke-prod.yml`, `scripts/smoke-prod.sh` |

Use this playbook together with **PUBLISHING.md** (sanitization, going public, releases) so the path is consistent across all your apps.

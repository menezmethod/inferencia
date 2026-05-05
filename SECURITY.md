# Security

## Reporting a vulnerability

If you believe you have found a security vulnerability, please report it responsibly:

- **Do not** open a public GitHub issue.
- Email the maintainers (see repository description or GitHub profile) with a clear description of the issue and steps to reproduce, or use GitHub’s [private vulnerability reporting](https://github.com/menezmethod/inferencia/security/advisories/new) if enabled.

We will acknowledge receipt and work with you to understand and address the issue.

## Security considerations for deployers

- **API keys** — Use strong, random keys (e.g. `openssl rand -hex 32`). Store them only in environment variables or a file that is not committed to version control (e.g. `keys.txt` in `.gitignore`).
- **Backend URL** — The LLM backend (e.g. MLX) should not be exposed directly to the internet. Run inferencia as the only public entrypoint; inferencia connects to the backend over a private network (e.g. LAN, Docker network).
- **Metrics and docs** — `/metrics` and `/docs` are unauthenticated by design. If your deployment is public, put inferencia behind a reverse proxy and restrict or protect these paths if needed.
- **Secrets in config** — Never commit `config.yaml`, `keys.txt`, or `.env` containing real credentials. Use `config.example.yaml` and `keys.example.txt` as templates only.
- **Dependencies** — Keep Go modules and Docker base images up to date. Dependabot and CI help catch known issues.

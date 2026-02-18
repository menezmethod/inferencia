# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 1.x     | :white_check_mark: |

## Reporting a Vulnerability

If you discover a security vulnerability, please report it responsibly:

1. **Do not** open a public GitHub issue.
2. Email the maintainers directly or use a private security advisory.
3. Include a description of the vulnerability and steps to reproduce.
4. Allow reasonable time for a fix before public disclosure.

We will acknowledge receipt within 48 hours and provide an initial assessment within 7 days.

## Security Best Practices

- **API keys**: Use strong keys (e.g. `openssl rand -hex 32`), prefix with `sk-`, never commit to version control.
- **Secrets**: Store keys in environment variables or external secret managers; never in config files in the repo.
- **HTTPS**: Always use HTTPS in production; inferencia relies on reverse proxies (Cloudflare, Coolify) for TLS termination.
- **Rate limiting**: Defaults (10 req/s, burst 20) mitigate abuse; adjust via `INFERENCIA_RATELIMIT_RPS` and `INFERENCIA_RATELIMIT_BURST`.
- **Backend exposure**: Keep MLX/Ollama backends on private networks; inferencia proxies requests with auth.

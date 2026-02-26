# Metrics and logging setup guide

Inferencia is **hosted on Coolify**, so metrics and logging work out of the box: Coolify can scrape `/metrics`, and logs are available from the container. This guide also covers local runs and optional stacks (Grafana, Loki, OTel).

---

## 1. Run inferencia

```bash
cd inferencia
cp config.example.yaml config.yaml
cp keys.example.txt keys.txt   # or set INFERENCIA_API_KEYS=sk-your-key,...
make run
```

Inferencia listens on `http://127.0.0.1:8080` by default. All steps below assume this.

---

## 2. Metrics (Prometheus)

Metrics are **always on**. No config or flag required.

### 2.1 Quick check

```bash
curl -s http://127.0.0.1:8080/metrics | head -30
```

You should see Prometheus exposition format: `# HELP`, `# TYPE`, and lines like `inferencia_http_requests_total`, `inferencia_http_request_duration_seconds`, etc.

### 2.2 What you get

| Metric | Type | Description |
|--------|------|-------------|
| `inferencia_http_requests_total` | Counter | Requests by method, path, status |
| `inferencia_http_request_duration_seconds` | Histogram | Request latency (5ms–120s buckets) |
| `inferencia_http_requests_in_flight` | Gauge | Active requests |
| `inferencia_tokens_total` | Counter | Tokens by model and type (prompt/completion) |
| `inferencia_backend_healthy` | Gauge | Backend up (1) or down (0) |
| `inferencia_backend_request_duration_seconds` | Histogram | Backend latency |
| `inferencia_ratelimit_rejections_total` | Counter | Rate-limited requests |

### 2.3 Scraping with Prometheus (optional)

If you want graphs and alerts, run the observability stack so Prometheus scrapes inferencia.

**Inferencia on host (typical local setup):**

1. Start inferencia (e.g. `make run`).
2. Start the stack; Prometheus will scrape inferencia via `host.docker.internal:8080`:

```bash
cd deploy
docker compose -f docker-compose.observability.yaml up -d
```

3. Open **Prometheus**: http://localhost:9090  
   - Explore → query e.g. `inferencia_http_requests_total` or `rate(inferencia_http_request_duration_seconds_count[5m])`.
4. Open **Grafana**: http://localhost:3000 (admin / admin)  
   - Dashboards are auto-provisioned; use the inferencia dashboard for request rate, latency, tokens, backend health.

The scrape config is in `deploy/prometheus/prometheus.yaml`:

- **inferencia-local**: targets `host.docker.internal:8080` (inferencia on your machine).
- **inferencia**: targets `inferencia:8080` (when inferencia runs inside the same Compose).

**Inferencia in Docker:**

Run inferencia in the same Compose network (or add it to the observability compose) and use the `inferencia` job; set the target to your inferencia service name and port.

---

## 3. Logging

### 3.1 Default: JSON to stdout

Default config:

```yaml
log:
  level: "info"   # debug | info | warn | error
  format: "json"   # json | text
```

Every request produces one **canonical log line** (JSON) with `request_id`, `method`, `path`, `status`, `duration_ms`, `bytes`, `remote_addr`, `user_agent`, and masked `api_key`. Example:

```json
{"time":"...","level":"INFO","msg":"request","request_id":"a1b2c3...","method":"POST","path":"/v1/chat/completions","status":200,"duration_ms":1423,"bytes":512,"remote_addr":"127.0.0.1:...","user_agent":"...","api_key":"...bd09b03"}
```

- **Debug**: Set `log.level: "debug"` or `INFERENCIA_LOG_LEVEL=debug`.
- **Human-readable**: Set `log.format: "text"` or `INFERENCIA_LOG_FORMAT=text`.

### 3.2 GCP / cloud logging (optional)

For **Google Cloud Logging** (or any system that expects a `severity` field), set `log.cloud_format`:

```yaml
log:
  level: "info"
  format: "json"
  cloud_format: "gcp"                # adds severity (DEBUG, INFO, WARNING, ERROR)
  # cloud_format: "gcp_with_resource" # adds severity + resource object
```

Or env: `INFERENCIA_LOG_CLOUD_FORMAT=gcp` or `gcp_with_resource`.

Then ingest stdout (e.g. Cloud Run, GKE, or the logging agent); the JSON is parsed and `severity` is used natively.

### 3.3 Loki (optional, with deploy stack)

If you run the observability stack, **Promtail** can ship container logs to **Loki**. Query in Grafana with LogQL, e.g.:

```logql
{job="inferencia"} | json | status >= 500
{job="inferencia"} | json | path="/v1/chat/completions" | duration_ms > 5000
```

---

## 4. OpenTelemetry tracing (optional)

For **distributed tracing** (e.g. Jaeger, Grafana Tempo, Google Cloud Trace):

```yaml
observability:
  otel_enabled: true
  otel_endpoint: "http://localhost:4318"   # OTLP HTTP; use https:// in production
  otel_service_name: "inferencia"
```

Or env:

```bash
export INFERENCIA_OTEL_ENABLED=true
export INFERENCIA_OTEL_ENDPOINT=http://localhost:4318
export INFERENCIA_OTEL_SERVICE_NAME=inferencia
```

- **Local**: `http://` endpoint uses insecure transport (fine for a local collector).
- **Production**: Use an `https://` endpoint; TLS is enabled automatically.

Run an OTLP collector (e.g. OpenTelemetry Collector, Jaeger) that accepts HTTP on 4318 and exports to your backend.

---

## 5. Summary

| Want | Action |
|------|--------|
| **See metrics now** | `curl http://127.0.0.1:8080/metrics` |
| **Graphs + alerts** | `docker compose -f deploy/docker-compose.observability.yaml up -d`; use Prometheus + Grafana (and optionally Loki). |
| **JSON logs** | Default; adjust `log.level` / `log.format` in config or env. |
| **GCP-compatible logs** | Set `log.cloud_format: "gcp"` (or env `INFERENCIA_LOG_CLOUD_FORMAT=gcp`). |
| **Tracing** | Set `observability.otel_enabled: true` and `observability.otel_endpoint` (and use `https://` in prod). |

For alert rules, dashboards, and Alertmanager routing, see `deploy/` and the main [README](../README.md) Observability section.

# Observability

The three pillars: Metrics, Logs, Traces.

## Metrics — Prometheus + Grafana

### Access
```bash
make prometheus-port-forward   # http://localhost:9090
make grafana-port-forward       # http://localhost:3000 (admin/admin)
```

### Custom Metrics (added in Week 5)
| Metric | Type | Description |
|--------|------|-------------|
| `jpp_jobs_submitted_total` | Counter | Jobs received by api-service |
| `jpp_jobs_completed_total` | Counter | Jobs completed by workers (labeled by type) |
| `jpp_jobs_failed_total` | Counter | Failed jobs (labeled by type) |
| `jpp_job_processing_duration_seconds` | Histogram | Worker processing time |
| `jpp_queue_depth` | Gauge | RabbitMQ queue depth per queue |

### Useful PromQL Queries
```promql
# Job throughput (per minute)
rate(jpp_jobs_submitted_total[1m])

# Worker success rate
rate(jpp_jobs_completed_total[5m]) / rate(jpp_jobs_submitted_total[5m])

# 95th percentile processing time
histogram_quantile(0.95, rate(jpp_job_processing_duration_seconds_bucket[5m]))
```

## Logs — Loki

### Access via Grafana
Grafana → Explore → Loki data source

### Log Query Examples
```logql
# All logs from api-service
{app="api-service"}

# Failed jobs only
{namespace="app"} |= "failed"

# Errors with job_id
{app="image-worker"} | json | level="error" | line_format "{{.job_id}}: {{.msg}}"
```

## Traces — Jaeger

### Access
```bash
make jaeger-port-forward   # http://localhost:16686
```

### What's Instrumented (Week 7)
- HTTP request → api-service handler
- RabbitMQ publish (api-service → jobs.pending)
- RabbitMQ consume (coordinator, workers)
- SQL queries (api-service, workers)

Trace IDs are injected into logs as `trace_id` for correlation.

---

_Add dashboard screenshots and findings as you build out observability._

# Job Processing Platform

A learning project for Go microservices, Kubernetes, and Observability.

## Architecture

```
Client → api-service → RabbitMQ → job-coordinator → workers (image/data/report)
                     ↕                                      ↕
                  Postgres                              Postgres
```

**Services**
- `api-service` — REST API for job submission and status queries (Gin)
- `job-coordinator` — Consumes `jobs.pending`, routes to typed queues
- `image-worker` — Processes image jobs
- `data-worker` — Processes data transformation jobs
- `report-worker` — Generates PDF/report jobs

**Infrastructure**
- Postgres — persistent job storage
- Redis — result caching
- RabbitMQ — async job queues
- Keycloak — auth (future)

**Observability**
- Prometheus + Grafana — metrics and dashboards
- Loki — structured log aggregation
- Jaeger — distributed tracing

## Quick Start (Local Docker, No Kubernetes)

```bash
# Start app + observability locally
docker compose --profile observability up --build

# Test API
curl http://localhost:8080/health
```

Local UIs:
- API: `http://localhost:8080`
- RabbitMQ: `http://localhost:15672` (`guest/guest`)
- Prometheus: `http://localhost:9090`
- Grafana: `http://localhost:3000` (`admin/admin`)
- Jaeger: `http://localhost:16686`

Grafana dashboards are auto-provisioned in the `JPP` folder:
- `JPP Overview`
- `JPP Job Throughput & Latency`
- `JPP Logs`

Stop everything:

```bash
docker compose down

# Wipe volumes too (fresh start)
docker compose down -v
```

## Quick Start (Kubernetes)

```bash
# 1. Check prerequisites and create local kind cluster
make setup

# 2. Deploy everything
make k8s-deploy-all

# 3. Check cluster status
make k8s-status
```

## Run a Service Locally (Faster Iteration)

```bash
# Keep data services in K8s
make k8s-up
make k8s-deploy-data

# Run API locally
cd backend/api-service
go run main.go
```

## Prerequisites

- Go 1.23+
- Docker
- kind (for Kubernetes path)
- kubectl (for Kubernetes path)
- Helm (for Kubernetes path)

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

## Quick Start

```bash
# 1. Check all prerequisites and create the local K8s cluster
make setup

# 2. Deploy everything to Kubernetes
make k8s-deploy-all

# 3. Check cluster status
make k8s-status

# 4. Port-forward the API and test it
make api-port-forward &
curl http://localhost:8080/health
```

## Run a Service Locally (faster iteration)

```bash
# Keep data services in K8s
make k8s-up
make k8s-deploy-data

# Run the API locally
cd backend/api-service
go run main.go
```

## All Commands

```bash
make help
```

## Documentation

**Full project report:** [docs/PROJECT_REPORT.md](docs/PROJECT_REPORT.md) — what the project does, how it works, how to navigate it, and how to proceed with learning/development.

See also the `docs/` directory:
- [SETUP.md](docs/SETUP.md) — detailed setup instructions
- [ARCHITECTURE.md](docs/ARCHITECTURE.md) — system design
- [LOCAL_DEV.md](docs/LOCAL_DEV.md) — local development workflow
- [K8S_CONCEPTS.md](docs/K8S_CONCEPTS.md) — Kubernetes concepts explained
- [OBSERVABILITY.md](docs/OBSERVABILITY.md) — metrics, logs, traces
- [API_REFERENCE.md](docs/API_REFERENCE.md) — endpoint documentation
- [TROUBLESHOOTING.md](docs/TROUBLESHOOTING.md) — common issues

## Learning Path

See [repoguide.md](repoguide.md) for the full 8-week learning roadmap.

## Prerequisites

- Go 1.23+
- Docker
- kind (`brew install kind`)
- kubectl (`brew install kubectl`)
- Helm (`brew install helm`)

# Job Processing Platform — Complete Project Report

This document is the **single reference** for what the project does, how it works, how to navigate the codebase, and how to proceed with learning and development.

---

## Part 1: What This Project Does

### Purpose

The **Job Processing Platform (JPP)** is an **asynchronous job processing system**. Users (or other systems) submit **jobs** via an HTTP API; the platform queues them, routes them by type, runs them in background **workers**, and stores results so clients can poll for status and outcome.

### What Is a Job?

A **job** is one unit of work with:

| Concept | Meaning |
|--------|--------|
| **Type** | Kind of work: `image`, `data`, or `report`. Determines which worker runs it. |
| **Payload** | Input data (e.g. image URL, report parameters) as a JSON string. |
| **Status** | Lifecycle: `pending` → `processing` → `completed` or `failed`. |
| **Result / Error** | Filled when the job finishes; client reads these via the API. |

### What It Does Today (vs Learning Goal)

- **Implemented**: Full flow from HTTP → database → message queue → coordinator → workers → database. You can submit jobs, see them move through queues, and read status/result. Workers run **stub logic** (e.g. short sleep + fake result).
- **Learning focus**: Architecture (Go microservices, RabbitMQ, Postgres, Kubernetes), not production-grade business logic. Real image/data/report processing would be added later inside each worker.

### One-Sentence Summary

**Submit a job via API → it is stored in Postgres and queued → a coordinator routes it to the right worker queue → a worker runs it (stub) and updates Postgres → you poll the API for status and result.**

---

## Part 2: How It Works

### High-Level Architecture

```
                    ┌──────────────────┐
                    │      Client      │
                    │ (curl, app, UI)  │
                    └────────┬─────────┘
                             │ HTTP: POST /jobs, GET /jobs/:id
                             ▼
                    ┌──────────────────┐
                    │   api-service    │  ← Gin HTTP server
                    │  (port 8080)     │
                    └────────┬─────────┘
                             │
         ┌───────────────────┼───────────────────┐
         │                   │                   │
         ▼                   ▼                   ▼
  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐
  │  Postgres   │    │  RabbitMQ   │    │   Redis     │
  │  (jobs DB)  │    │  (queues)   │    │  (cache)    │
  └─────────────┘    └──────┬──────┘    └─────────────┘
         ▲                  │
         │                  │ consume jobs.pending
         │                  ▼
         │           ┌──────────────────┐
         │           │ job-coordinator  │
         │           └────────┬─────────┘
         │                    │ route by type
         │     ┌──────────────┼──────────────┐
         │     ▼              ▼              ▼
         │  jobs.image   jobs.data   jobs.report
         │     │              │              │
         │     ▼              ▼              ▼
         │  image-worker  data-worker  report-worker
         │     │              │              │
         └─────┴──────────────┴──────────────┘
               (UPDATE job status/result in Postgres)
```

### Step-by-Step Flow

1. **Client** sends `POST /jobs` with `{"type": "image", "payload": "..."}`.
2. **api-service** validates the request, inserts a row in Postgres (status `pending`), and publishes a message to the RabbitMQ queue `jobs.pending`. It returns the job `id` to the client.
3. **job-coordinator** consumes from `jobs.pending`, reads the job type, and re-publishes the same message to one of:
   - `jobs.image` → image-worker  
   - `jobs.data` → data-worker  
   - `jobs.report` → report-worker  
4. The **worker** for that queue:
   - Consumes the message.
   - Updates the job in Postgres to `processing`.
   - Runs its logic (currently a stub: sleep + fake result).
   - Updates the job to `completed` (with result) or `failed` (with error).
   - Acknowledges the message so RabbitMQ removes it from the queue.
5. **Client** calls `GET /jobs/:id` to read current status, result, or error.

### Data Flow Summary

| Stage | Where | What |
|-------|--------|------|
| Submit | api-service | INSERT into Postgres, PUBLISH to `jobs.pending` |
| Route | job-coordinator | CONSUME from `jobs.pending`, PUBLISH to `jobs.image` / `jobs.data` / `jobs.report` |
| Execute | workers | CONSUME from typed queue, UPDATE Postgres (processing → completed/failed), ACK |
| Query | api-service | SELECT from Postgres, return JSON |

### Job Status Lifecycle

```
  pending ──► processing ──► completed
                    │
                    └──────────────► failed
```

---

## Part 3: How to Navigate the Project

### Directory Map

```
JPP/
├── README.md                 # Quick start, links to docs
├── Makefile                  # All commands (make help)
├── go.work                   # Go workspace (lists all modules)
├── .gitignore
├── repoguide.md              # Original 8-week learning roadmap
│
├── backend/                  # All Go services
│   ├── shared/               # Shared types (Job, JobStatus, JobType, JobMessage)
│   │   ├── go.mod
│   │   └── models.go
│   ├── api-service/          # HTTP API
│   │   ├── go.mod
│   │   └── main.go           # Routes, DB, RabbitMQ publish
│   ├── job-coordinator/
│   │   ├── go.mod
│   │   └── main.go           # Consume jobs.pending, publish to typed queues
│   └── workers/
│       ├── image-worker/
│       ├── data-worker/
│       └── report-worker/
│           ├── go.mod
│           └── main.go        # Consume queue, update Postgres, stub process
│
├── infra/                    # Deployment and infrastructure
│   ├── kind/                 # Local Kubernetes cluster config
│   │   └── kind-config.yaml
│   ├── k8s/                  # Kubernetes manifests
│   │   ├── namespaces.yaml
│   │   ├── postgres/         # StatefulSet, Service, Secret, ConfigMap
│   │   ├── redis/
│   │   ├── rabbitmq/
│   │   ├── keycloak/
│   │   ├── services/         # api-service, job-coordinator, workers
│   │   └── observability/   # Prometheus, Grafana, Loki, Jaeger
│   ├── helm/
│   │   └── job-platform/     # Umbrella chart
│   ├── docker/               # Dockerfiles (placeholders)
│   └── terraform/            # Future GCP IaC
│
├── scripts/
│   ├── setup.sh              # One-command setup (deps, kind, namespaces)
│   ├── load-test.sh          # Submit many jobs (stub)
│   ├── chaos-test.sh         # Simulate failures (stub)
│   └── db/
│       └── init.sql          # Jobs table schema
│
├── docs/                     # All documentation
│   ├── README.md             # Doc index and reading order
│   ├── PROJECT_REPORT.md     # This file
│   ├── GO_BASICS.md          # Go concepts for this project
│   ├── SETUP.md
│   ├── ARCHITECTURE.md
│   ├── LOCAL_DEV.md
│   ├── API_REFERENCE.md
│   ├── K8S_CONCEPTS.md
│   ├── OBSERVABILITY.md
│   ├── DEPLOYMENT.md
│   └── TROUBLESHOOTING.md
│
└── .github/workflows/        # CI/CD
    ├── test.yml
    ├── build.yml
    └── deploy.yml
```

### Where to Look for What

| Goal | Location |
|------|----------|
| Change job data shape | `backend/shared/models.go` |
| Change API behaviour | `backend/api-service/main.go` |
| Change routing logic | `backend/job-coordinator/main.go` |
| Implement real image/data/report work | `backend/workers/<type>-worker/main.go` → `process*Job()` |
| Change DB schema | `scripts/db/init.sql` and `infra/k8s/postgres/configmap.yaml` (init script) |
| Change K8s resources | `infra/k8s/` (by component) |
| Add a new worker type | New dir under `backend/workers/`, extend `shared` and coordinator, add K8s manifest |

### Key Files to Read First

1. **`backend/shared/models.go`** — Defines `Job`, `JobMessage`, `JobType`, `JobStatus`. Everything else uses these.
2. **`backend/api-service/main.go`** — How HTTP turns into DB + queue (createJobHandler, getJobHandler).
3. **`backend/job-coordinator/main.go`** — How one queue becomes three (typeToQueue map, consume loop).
4. **`backend/workers/image-worker/main.go`** — One worker pattern: consume → update DB → process → update DB → ack.
5. **`scripts/db/init.sql`** — The single source of truth for the `jobs` table (mirrored in Postgres ConfigMap for K8s).

---

## Part 4: How to Proceed — Learning & Development

### Prerequisites

Install once:

- **Go** 1.23+ (`brew install go`)
- **Docker** (Docker Desktop or equivalent)
- **kind** (`brew install kind`)
- **kubectl** (`brew install kubectl`)
- **Helm** (`brew install helm`)

### Phase A: Get It Running (Day 1–2)

1. **One-time setup**
   ```bash
   chmod +x scripts/setup.sh scripts/load-test.sh scripts/chaos-test.sh
   make setup
   ```
   This checks tools, creates the kind cluster, creates namespaces, and syncs Go deps.

2. **Deploy data layer**
   ```bash
   make k8s-deploy-data
   make k8s-status
   ```
   Postgres, Redis, and RabbitMQ should be running in the `data` namespace.

3. **Run the API locally** (easiest way to iterate)
   ```bash
   kubectl port-forward svc/postgres 5432:5432 -n data &
   kubectl port-forward svc/rabbitmq 5672:5672 -n data &
   cd backend/api-service && go run main.go
   ```
   Then: `curl http://localhost:8080/health`

4. **Run coordinator + one worker** (in other terminals)
   ```bash
   cd backend/job-coordinator && go run main.go
   cd backend/workers/image-worker && go run main.go
   ```

5. **Submit a job and watch it flow**
   ```bash
   curl -X POST http://localhost:8080/jobs -H "Content-Type: application/json" \
     -d '{"type":"image","payload":"{}"}'
   # Use the returned "id" in:
   curl http://localhost:8080/jobs/<id>
   ```

### Phase B: Understand the Code (Week 1)

1. Read **`docs/GO_BASICS.md`** if you are new to Go.
2. Read **`backend/shared/models.go`** and **`backend/api-service/main.go`** with the flow in mind (submit → insert → publish).
3. Trace one job in logs: add temporary `log.Printf` in api-service (after insert), coordinator (after route), and worker (after process), then submit one job and follow the logs.

### Phase C: Deploy Full Stack in Kubernetes (Week 2)

1. Build images and load into kind (or use a real registry):
   - Either add Dockerfiles under `infra/docker/` and use `make docker-build SERVICE=api-service` etc., then load into kind.
   - Or keep running api/coordinator/workers locally and only deploy data + observability to K8s.

2. Deploy everything (once images exist):
   ```bash
   make k8s-deploy-all
   make k8s-status
   make api-port-forward
   curl http://localhost:8080/health
   ```

3. Use **`make help`** for the full command list (build, test, deploy, port-forward, logs, scale, clean).

### Phase D: Implement Real Logic (Week 3+)

- **image-worker**: Resize/compress/convert images (e.g. use an image library or call a tool).
- **data-worker**: Parse CSV/JSON, transform, aggregate — whatever “data” means for you.
- **report-worker**: Render HTML or PDF (e.g. template + PDF library), optionally store in object storage and put URL in `result`.

Keep the same contract: read `JobMessage`, update Postgres, ack. Only the body of `process*Job()` changes.

### Phase E: Observability & Production Readiness (Week 5–8)

- **Metrics**: Add Prometheus client to each service, expose `/metrics`, define counters/histograms for jobs submitted/completed/failed and latency. Use Grafana dashboards (see `docs/OBSERVABILITY.md`).
- **Logging**: Structured logs (e.g. zerolog/zap) with `job_id`, `trace_id`; ship to Loki.
- **Tracing**: OpenTelemetry in api-service and workers; export to Jaeger. See `docs/OBSERVABILITY.md`.
- **K8s**: Health probes, resource limits, HPA, NetworkPolicies, RBAC (see `docs/K8S_CONCEPTS.md`, `docs/DEPLOYMENT.md`).

### Documentation Reading Order

| Order | Document | When to read |
|-------|----------|--------------|
| 1 | **PROJECT_REPORT.md** (this file) | First — overview and navigation |
| 2 | **GO_BASICS.md** | If new to Go |
| 3 | **SETUP.md** | Before first run |
| 4 | **ARCHITECTURE.md** | To deepen system design |
| 5 | **API_REFERENCE.md** | When calling or changing the API |
| 6 | **LOCAL_DEV.md** | For daily dev workflow |
| 7 | **K8S_CONCEPTS.md** | When working with Kubernetes |
| 8 | **OBSERVABILITY.md** | When adding metrics/logs/traces |
| 9 | **DEPLOYMENT.md** | When deploying beyond local |
| 10 | **TROUBLESHOOTING.md** | When something breaks |

### Makefile Quick Reference

| Category | Commands |
|----------|----------|
| Setup | `make setup`, `make deps` |
| Build | `make build SERVICE=api-service`, `make build-all` |
| Test | `make test`, `make fmt` |
| Kubernetes | `make k8s-up`, `make k8s-deploy-all`, `make k8s-status`, `make k8s-logs SERVICE=api-service`, `make k8s-scale SERVICE=image-worker REPLICAS=3`, `make k8s-down` |
| Port-forward | `make api-port-forward`, `make rabbitmq-port-forward`, `make grafana-port-forward`, `make prometheus-port-forward`, `make jaeger-port-forward` |
| Clean | `make clean`, `make clean-docker` |

Run **`make help`** for the full list.

---

## Part 5: Summary

- **What it does**: Async job processing — submit via API, jobs are queued, routed by type, executed by workers, results stored; client polls for status/result.
- **How it works**: api-service (Postgres + RabbitMQ) → job-coordinator (routing) → workers (typed queues + Postgres updates). Same `Job`/`JobMessage` types everywhere.
- **How to navigate**: Start from `backend/shared/models.go` and `backend/api-service/main.go`; use the directory map and “Where to look” table above.
- **How to proceed**: Get it running locally (setup → k8s-deploy-data → run api + coordinator + worker locally), trace one job, then implement real worker logic and add observability/K8s as you go. Use **PROJECT_REPORT.md** and **GO_BASICS.md** as your main references, and the rest of `docs/` for deeper topics.

This report is the **master entry point** for understanding and developing the Job Processing Platform.

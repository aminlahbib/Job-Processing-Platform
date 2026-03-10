# Architecture

This document gives a **deep architectural view** of the Job Processing Platform:

- **What components exist and how they talk to each other**
- **How a single HTTP request turns into background work and back into a result**
- **How data, queues, and observability pieces fit together**
- **How the system runs in Docker Compose vs. Kubernetes**

If you want a narrative overview, read `PROJECT_REPORT.md` first; this file is more diagram- and detail-focused.

---

## 1. High‑Level System Overview

At the core, JPP is an **async job pipeline**:

```text
                    ┌──────────────────┐
 Client (curl, UI)  │                  │
 ─────────────────▶ │   api-service    │  (Gin / Go)
  HTTP /jobs,*       │                  │
                    └────────┬─────────┘
                             │
                             │ SQL (jobs table)
                             ▼
                    ┌──────────────────┐
                    │     Postgres     │
                    │    jobs table    │
                    └────────┬─────────┘
                             │
              INSERT / UPDATE│
                             │
        AMQP                 ▼
   (RabbitMQ queues)  ┌───────────────┐   CONSUME
                      │   RabbitMQ    │◀──────────── api-service publishes
                      │   jobs.*      │
                      └──────┬────────┘
                             │ CONSUME
                             ▼
                    ┌──────────────────┐
                    │ job-coordinator  │
                    └──────┬───────────┘
                           │ route by job type
         ┌─────────────────┼────────────────────┐
         ▼                 ▼                    ▼
   jobs.image         jobs.data            jobs.report
         │                 │                    │
   ┌─────▼────┐      ┌─────▼─────┐       ┌──────▼─────┐
   │ image    │      │ data      │       │ report     │
   │ worker   │      │ worker    │       │ worker     │
   └─────┬────┘      └────┬──────┘       └─────┬──────┘
         └────────────────┴────────────────────┘
                         │
                         │ UPDATE status / result
                         ▼
                    ┌──────────────────┐
                    │     Postgres     │
                    └──────────────────┘
```

**Key properties:**

- **Write path**: HTTP → api-service → Postgres (`jobs` row with `pending`) → RabbitMQ (`jobs.pending`).
- **Route path**: RabbitMQ (`jobs.pending`) → job-coordinator → typed queues (`jobs.image|data|report`).
- **Execute path**: Worker consumes typed queue, updates Postgres → `processing` → `completed`/`failed`.
- **Read path**: HTTP `GET /jobs/:id` → api-service → Postgres → JSON to client.

---

## 2. Components and Responsibilities

### 2.1 api-service (`backend/api-service`)

- **Role**: HTTP gateway and API surface.
- **Responsibilities**:
  - Validate incoming JSON for `POST /jobs`.
  - Persist a new `jobs` row in Postgres with `pending` status.
  - Publish a `JobMessage` to RabbitMQ `jobs.pending` queue.
  - Expose read endpoints: `GET /jobs/:id`, `GET /jobs`.
  - Expose **health** (`/health`, `/healthz`) and **metrics** (`/metrics`).
  - Emit **structured logs** and **traces** (via OpenTelemetry).
- **Failure semantics**:
  - If DB insert fails → request fails (no job created).
  - If message publish fails after DB insert → job is stored but not queued (logged as warning).

### 2.2 job-coordinator (`backend/job-coordinator`)

- **Role**: Routing brain between generic queue and typed queues.
- **Responsibilities**:
  - Consume from `jobs.pending`.
  - Inspect `JobMessage.Type`.
  - Publish to:
    - `jobs.image` (for `image` jobs),
    - `jobs.data` (for `data` jobs),
    - `jobs.report` (for `report` jobs).
  - Expose `/metrics` (Prometheus) for routed job counts.
- **Failure semantics**:
  - If routing fails (e.g. RabbitMQ issue), message is not ACKed, so RabbitMQ can redeliver.

### 2.3 Workers (`backend/workers/*-worker`)

Each worker follows the same pattern:

- **Queues**:
  - image-worker → `jobs.image`
  - data-worker → `jobs.data`
  - report-worker → `jobs.report`
- **Responsibilities**:
  - Consume from its queue.
  - Update job to `processing` in Postgres.
  - Run **stubbed business logic** (simulated work).
  - Update job to `completed` or `failed`, filling `result` or `error` columns.
  - Emit metrics, logs, traces.
- **Idempotence**:
  - The system aims for **at-least-once** delivery from RabbitMQ.
  - Workers should be written so that a duplicate message does not corrupt state (e.g. re‑setting `completed` to the same result is safe).

### 2.4 Postgres (jobs database)

- Single `jobs` table managed by `scripts/db/init.sql`.
- Source of truth for:
  - Current job status (`pending`, `processing`, `completed`, `failed`).
  - Payload, result, error.
  - Timestamps for auditing / latency measurements.
- Accessed by:
  - `api-service` (insert + read),
  - All workers (status transitions).

### 2.5 RabbitMQ (message broker)

- Queues:
  - `jobs.pending` — ingress queue from api-service.
  - `jobs.image`, `jobs.data`, `jobs.report` — per-type worker queues.
- Responsibilities:
  - Reliable buffering between HTTP and workers.
  - Allow independent scaling of coordinator and workers.
  - Provide back-pressure (queues grow under load).

### 2.6 Redis (cache, infra only today)

- In Kubernetes manifests, Redis is deployed as a **data service**.
- As of now, code does not actively use Redis; it is included for future exercises:
  - e.g. caching hot job lookups for `GET /jobs/:id`.
  - rate-limiting or deduplication experiments.

---

## 3. Data & Messaging Architecture

### 3.1 Jobs table (Postgres)

Conceptually:

```text
jobs (
  id          UUID (PK),
  type        ENUM('image','data','report'),
  status      ENUM('pending','processing','completed','failed'),
  payload     TEXT,      -- JSON string
  result      TEXT,      -- JSON string, nullable
  error       TEXT,      -- error message, nullable
  created_at  TIMESTAMPTZ,
  updated_at  TIMESTAMPTZ
)
```

- All services use the **shared Go types** in `backend/shared/models.go`, so the schema and code stay consistent.
- Status transitions are **monotonic** (you never go backwards, e.g. from `completed` to `pending`).

### 3.2 Queues and messages (RabbitMQ)

Queues:

- `jobs.pending` (fan‑out input queue),
- `jobs.image`,
- `jobs.data`,
- `jobs.report`.

Message payload is a `JobMessage`:

- `JobID` — UUID of the row in Postgres.
- `Type` — `image|data|report`.
- `Payload` — same string as in DB row (so workers do not need to re‑query before starting work, although they still do for status).

---

## 4. Request & Job Workflows (Detailed)

### 4.1 Submit job (`POST /jobs`)

1. Client sends JSON:
   - `{ "type": "image", "payload": "{\"url\": \"https://example.com/image.jpg\"}" }`.
2. `api-service`:
   - Parses and validates JSON (Gin binding).
   - Checks that `type` is one of `image|data|report`.
   - Generates a new UUID for `id`.
   - Inserts into Postgres:
     - `status = pending`.
   - Attempts to publish `JobMessage` to `jobs.pending`.
3. API responds **201 Created** with the new job JSON (including `id` and `status=pending`), even if RabbitMQ publish fails (that failure is logged).

Sequence diagram (simplified):

```text
Client      api-service       Postgres            RabbitMQ
  | POST /jobs   |                |                   |
  |------------->|                |                   |
  |              | INSERT jobs(id,status=pending,...) |
  |              |-------------->|                    |
  |              |     OK        |                    |
  |              |<--------------|                    |
  |              | PUBLISH JobMessage(job_id, type..) |
  |              |----------------------------------->|
  | 201 + job id |                |                   |
  |<-------------|                |                   |
```

### 4.2 Routing and execution

1. **job-coordinator** consumes from `jobs.pending`:
   - Reads `JobMessage.Type`.
   - Publishes the same message to one of:
     - `jobs.image`,
     - `jobs.data`,
     - `jobs.report`.
2. **Worker** consumes from its queue:
   - Updates Postgres `status = processing`.
   - Runs simulated work (e.g. sleep + fake JSON).
   - On success:
     - Updates Postgres `status = completed`, sets `result`.
   - On error:
     - Updates Postgres `status = failed`, sets `error`.
   - ACKs the message in RabbitMQ.

Job state machine:

```text
submitted (API) → pending (DB insert) → processing (worker picked up) → completed
                                                          └──────────→ failed
```

### 4.3 Read job (`GET /jobs/:id`)

1. Client calls `GET /jobs/:id`.
2. `api-service` performs a `SELECT` from `jobs` by `id`.
3. Returns 200 with full job row, or 404 if missing.

Polling pattern:

```text
POST /jobs       → receive id
GET /jobs/:id    → see pending / processing
GET /jobs/:id    → eventually see completed / failed
```

---

## 5. Observability Architecture

JPP implements the **three pillars** of observability using a standard CNCF stack.

### 5.1 Metrics (Prometheus + Grafana)

- Each Go service exposes `/metrics` on its HTTP port via `prometheus/client_golang`.
- **Prometheus** scrapes:
  - `api-service:8080/metrics`,
  - `job-coordinator:8080/metrics`,
  - each worker `:8080/metrics`.
- **Custom metrics**:
  - `jpp_jobs_submitted_total` (counter, labeled by type),
  - `jpp_jobs_completed_total` / `jpp_jobs_failed_total` (per worker, by type),
  - `jpp_job_processing_duration_seconds` (histogram).
- **Grafana** reads from Prometheus and uses a prebuilt dashboard JSON (`infra/k8s/observability/grafana-dashboard-jobs.json`).

Metrics flow:

```text
Go services ──► /metrics ──► Prometheus ──► Grafana dashboards
```

### 5.2 Logs (Loki)

- Services log in **structured JSON** using `zerolog` to stdout.
- In Kubernetes, **Promtail**:
  - Tails container logs,
  - Parses JSON and attaches labels (e.g. `service`, `namespace`),
  - Ships entries to **Loki**.
- In Grafana “Explore”, Loki is used to query logs:
  - Example: `{app="api-service"}` or filtered by `job_id` / `trace_id`.

Log flow:

```text
Go services (JSON logs) ──► stdout ──► Promtail ──► Loki ──► Grafana Explore
```

### 5.3 Traces (OpenTelemetry + Jaeger)

- `api-service`, `job-coordinator`, and workers are instrumented with **OpenTelemetry**:
  - Spans around HTTP handlers and message processing.
  - Attributes like `job.id`, `job.type`.
- Traces are exported via OTLP HTTP to **Jaeger** (all‑in‑one).
- `trace_id` is injected into logs so you can correlate:
  - Start from a log line → copy `trace_id` → paste in Jaeger to view full trace.

Trace flow:

```text
Go services (OTel SDK) ──► OTLP HTTP ──► Jaeger ──► Jaeger UI
```

---

## 6. Deployment Topologies

### 6.1 Local (Docker Compose)

File: `docker-compose.yaml`.

Services:

- `postgres` on `localhost:5432`,
- `rabbitmq` on `5672` (AMQP) and `15672` (UI),
- `api-service` on `8080`,
- `job-coordinator`, `image-worker`, `data-worker`, `report-worker`,
- Optional profile `observability`:
  - `jaeger` (`16686`, `4317`, `4318`),
  - `prometheus` (`9090`),
  - `grafana` (`3000`).

Startup options:

- Everything:
  - `docker compose up --build`
- Infra only (to run Go with `go run`):
  - `docker compose up postgres rabbitmq`
- With observability:
  - `docker compose --profile observability up --build`

### 6.2 Kubernetes (kind cluster)

Key manifests under `infra/k8s/`:

- Namespaces:
  - `app` (services),
  - `data` (Postgres, Redis, RabbitMQ),
  - `observability` (Prometheus, Grafana, Loki, Jaeger).
- Deployments for:
  - `api-service`, `job-coordinator`, and the workers.
- Production hardening:
  - **NetworkPolicies** (e.g. `infra/k8s/network-policy.yaml`) restricting cross‑namespace traffic.
  - **PodDisruptionBudgets** (`infra/k8s/pdb.yaml`) to keep at least one replica alive.
  - **HorizontalPodAutoscaler** for workers (`infra/k8s/services/hpa-workers.yaml`) scaling by CPU.

In K8s, observability components are first‑class workloads rather than optional extras.

---

## 7. Data Stores and Cache Strategy

### 7.1 Postgres as source of truth

- All job lifecycle information lives in Postgres.
- Reads (`GET /jobs/:id`, `GET /jobs`) come directly from the database.
- This keeps the mental model simple for learning:
  - One authoritative source, no eventual consistency between DB and cache.

### 7.2 Future Redis usage

Redis is intentionally included in `infra/k8s` but not yet wired into the code, to leave space for future exercises:

- Caching frequently requested job results to reduce DB load.
- Implementing a **read‑through** cache for `GET /jobs/:id`.
- Storing short‑lived rate‑limit counters or deduplication keys.

When you introduce Redis into the code, you will be able to compare:

- **Pure DB reads** vs. **DB + cache**, and
- Observe the difference in metrics and latency in Grafana and Jaeger.

---

## 8. Scaling & Resilience

### 8.1 Horizontal scaling

- **api-service**:
  - Stateless; multiple replicas can sit behind a Service / LoadBalancer.
  - Safe to scale horizontally.
- **Workers**:
  - Also stateless; all coordination happens via RabbitMQ + Postgres.
  - Adding more worker pods simply means more consumers on the same queue(s).

### 8.2 Back‑pressure and queueing

- Under high load:
  - `jobs.pending` and the typed queues will grow.
  - API continues to accept jobs (up to any explicit limits you set).
  - Workers drain queues at their own pace.
- This decouples **request rate** from **processing rate**, which is the main value of this architecture.

### 8.3 Failure modes (high‑level)

- Postgres down:
  - `POST /jobs` fails (cannot persist).
  - `/health` will report `database: unreachable`.
- RabbitMQ down:
  - Jobs are persisted but not queued.
  - Workers will eventually drain once RabbitMQ recovers (for already queued jobs).
- Worker crash:
  - Messages that were not ACKed will be redelivered.
  - With idempotent worker logic, this is safe and just slower.

---

## 9. Where to Go Next

- For a narrative walkthrough and learning roadmap, read `PROJECT_REPORT.md`.
- For concrete endpoint examples and payloads, read `API_REFERENCE.md`.
- For observability details and PromQL / LogQL examples, read `OBSERVABILITY.md`.
- For daily local workflow, read `LOCAL_DEV.md`.

Use this `ARCHITECTURE.md` as your **mental model reference** when you are unsure how a particular piece (API, queue, worker, or observability tool) fits into the whole system.

# Architecture

## System Overview

```
                    ┌──────────────┐
         HTTP       │              │
Client ──────────▶  │  api-service │
                    │  (Gin/Go)    │
                    └──────┬───────┘
                           │ SQL (jobs table)
                           ▼
                    ┌──────────────┐
                    │   Postgres   │
                    └──────────────┘
                           │
                    ┌──────▼───────┐   AMQP
                    │   RabbitMQ   │◀────────── api-service publishes
                    │              │
                    │ jobs.pending │
                    └──────┬───────┘
                           │ consume
                    ┌──────▼───────────┐
                    │  job-coordinator │
                    └──────┬───────────┘
               route by job type
         ┌─────────┬──────────┴──────────┐
         ▼         ▼                     ▼
   jobs.image  jobs.data           jobs.report
         │         │                     │
  ┌──────▼──┐ ┌────▼──────┐ ┌───────────▼──┐
  │  image  │ │   data    │ │    report    │
  │ worker  │ │  worker   │ │   worker     │
  └──────┬──┘ └────┬──────┘ └───────────┬──┘
         └─────────┴───────────────────┘
                           │ UPDATE status/result
                           ▼
                    ┌──────────────┐
                    │   Postgres   │
                    └──────────────┘
```

## Job Lifecycle

```
submitted → pending → processing → completed
                              └──▶ failed
```

1. `POST /jobs` — API creates job row (status=pending), publishes to `jobs.pending`
2. coordinator — consumes `jobs.pending`, re-publishes to typed queue
3. worker — consumes typed queue, updates status to `processing`, executes, updates to `completed` or `failed`
4. `GET /jobs/:id` — client polls for result

## Key Design Decisions

### Why RabbitMQ over Kafka?
For this scale, RabbitMQ is simpler to operate. Its per-message ACK model maps naturally to the job processing pattern (exactly-once delivery intent).

### Why separate job-coordinator?
Workers only understand their own job type. The coordinator allows workers to be scaled and deployed independently without any routing logic.

### Why Postgres over a dedicated job queue (like Faktory)?
Learning goal: understand how to build exactly-once semantics manually. A real system might use a purpose-built queue.

---

_Fill in more detail as you implement each component._

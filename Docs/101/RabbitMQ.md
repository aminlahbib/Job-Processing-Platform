## RabbitMQ 101 — How It Fits Into JPP

This guide explains **what RabbitMQ is**, **how we use it in the Job Processing Platform (JPP)**, and **how to inspect it using the management UI at `http://localhost:15672`**.

---

## 1. What RabbitMQ Is (In Plain Terms)

- **Message broker**: a middleman that lets one service send messages to another **asynchronously**.
- Instead of calling workers directly, the API **drops a message into a queue** and returns immediately.
- Workers **pull messages when they are ready**, so:
  - Short HTTP requests,
  - Long‑running work in the background,
  - Built‑in buffering when load spikes.

RabbitMQ stores messages in **queues**. A producer **publishes** messages to a queue; a consumer **subscribes** and processes them.

---

## 2. Queues Used in This Project

In JPP we use **four main queues**:

- `jobs.pending`
  - **Producer**: `api-service`
  - **Consumer**: `job-coordinator`
  - **Meaning**: “A new job was created in Postgres and is ready to be routed.”

- `jobs.image`
  - **Producer**: `job-coordinator`
  - **Consumer**: `image-worker`
  - **Meaning**: “This job is of type `image` and should be processed by the image worker.”

- `jobs.data`
  - **Producer**: `job-coordinator`
  - **Consumer**: `data-worker`
  - **Meaning**: “This job is of type `data` and should be processed by the data worker.”

- `jobs.report`
  - **Producer**: `job-coordinator`
  - **Consumer**: `report-worker`
  - **Meaning**: “This job is of type `report` and should be processed by the report worker.”

Each message on these queues is a **`JobMessage`** that contains:

- `JobID` (the ID of the row in Postgres),
- `Type` (`image`, `data`, or `report`),
- `Payload` (JSON string with the job input).

Workers use `JobID` to update the correct row in Postgres.

---

## 3. Why We Use RabbitMQ Here

RabbitMQ gives us a few important properties:

- **Async processing**:
  - The HTTP request that creates a job returns fast; the actual work happens later.

- **Decoupling**:
  - `api-service` doesn’t need to know **when** a job is processed or **which worker** handles it.
  - Workers don’t care **who** submitted the job.

- **Back‑pressure and buffering**:
  - If many jobs arrive quickly, queues hold them until workers catch up.
  - You can see this as a growing “to do” list in the UI.

- **Independent scaling**:
  - You can scale up workers (more consumers) without touching the API.
  - You can also add new worker types later by adding new queues and routing logic.

For learning, RabbitMQ is also easier to run locally than something heavier like Kafka.

---

## 4. End‑to‑End Flow With RabbitMQ

Putting it all together:

1. **Client → api-service**
   - `POST /jobs` with `{ "type": "image", "payload": "..." }`.
   - `api-service` inserts a row into Postgres with `status = pending`.
   - Then it publishes a `JobMessage` to `jobs.pending`.

2. **job-coordinator**
   - Consumes `jobs.pending`.
   - Looks at `Type`:
     - `image` → publishes to `jobs.image`,
     - `data` → publishes to `jobs.data`,
     - `report` → publishes to `jobs.report`.

3. **Workers**
   - Each worker consumes from its own queue.
   - It:
     - Updates the job’s `status` in Postgres:
       - `pending` → `processing` → `completed` or `failed`.
     - Acknowledges the message so RabbitMQ removes it from the queue.

4. **Client polling**
   - Client calls `GET /jobs/:id` to read status and result from Postgres.

RabbitMQ never stores the final job result — it only carries **events** that tell workers “here is a job to process.”

---

## 5. RabbitMQ Management UI (`http://localhost:15672`)

When you run the app via Docker Compose, RabbitMQ exposes a **management UI** at:

- URL: `http://localhost:15672`
- **Username**: `guest`
- **Password**: `guest`

### 5.1 Useful Tabs

- **Overview**
  - See total connections, channels, queues.
  - Message rates (publish, deliver, ack) across the whole broker.

- **Connections / Channels**
  - See which services are currently connected:
    - `api-service`, `job-coordinator`, `image-worker`, `data-worker`, `report-worker`.
  - Helpful to confirm that your services are actually talking to RabbitMQ.

- **Queues**
  - The most important tab for this project.
  - For each queue (`jobs.pending`, `jobs.image`, `jobs.data`, `jobs.report`) you see:
    - **Ready** messages: waiting to be processed.
    - **Unacked** messages: being processed by workers.
    - **Consumers**: how many workers are attached.
    - **Message rates**: how fast messages come in and go out.

From the queue details page you can:

- **Peek at messages** (Get messages) — useful for debugging payloads.
- **Purge** a queue — clear all messages (careful, this drops in‑flight work).

You generally do **not** need to change advanced settings here for local development; use it as an **observability and debugging tool**.

---

## 6. How to Use the UI While Learning

A simple workflow to understand how the system behaves:

1. Open the UI at `http://localhost:15672` (guest / guest).
2. Click **Queues** and keep this tab open.
3. In a terminal, submit a job:

   ```bash
   curl -X POST http://localhost:8080/jobs \
     -H "Content-Type: application/json" \
     -d '{"type":"image","payload":"{}"}'
   ```

4. Watch `jobs.pending`:
   - You should briefly see the **Ready** count go up.
   - Then it should go back down as `job-coordinator` consumes and re‑publishes.

5. Watch `jobs.image`:
   - The job appears there until `image-worker` picks it up.
   - **Ready** decreases, **Unacked** may briefly increase while the worker is processing.

6. Call:

   ```bash
   curl http://localhost:8080/jobs/<id>
   ```

   and watch the job’s `status` go from `pending` → `processing` → `completed`.

This tight feedback loop (curl + RabbitMQ UI + `GET /jobs/:id`) helps you **see** the pipeline rather than just read about it.

---

## 7. Next Steps

- For a higher‑level architecture view, read `docs/ARCHITECTURE.md`.
- For details on metrics, logs, and traces (how RabbitMQ behavior shows up in Prometheus / Grafana / Loki / Jaeger), read `docs/OBSERVABILITY.md`.
- For HTTP endpoints and example payloads, read `docs/API_REFERENCE.md`.

Use this file any time you want to remind yourself **what the queues mean** and **how to interpret what you see in the RabbitMQ UI**.


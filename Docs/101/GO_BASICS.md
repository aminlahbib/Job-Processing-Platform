# Go Basics — Understanding This Project

You've never programmed in Go before. This doc explains **what we built** and **the Go concepts** behind it, so you can read and change the code with confidence.

---

## 1. Go in 60 Seconds

- **Compiled language** — you run `go build` or `go run main.go`; the compiler produces a single binary (no interpreter).
- **Explicit and simple** — no classes; you use **packages**, **structs**, and **functions**. Errors are returned explicitly (no exceptions).
- **One entry point per program** — `func main()` in `main.go` is where execution starts.
- **Modules** — each service is a **module** (a folder with `go.mod`). Modules can import other modules (we use `shared` everywhere).

---

## 2. Project Layout (What Lives Where)

```
backend/
├── shared/           ← Types used by ALL services (Job, JobStatus, etc.)
│   ├── go.mod        ← "I am module github.com/jpp/shared"
│   └── models.go     ← struct definitions
│
├── api-service/      ← HTTP server: receives POST /jobs, GET /jobs/:id
│   ├── go.mod        ← depends on shared, gin, rabbitmq, postgres
│   └── main.go       ← starts server, defines routes
│
├── job-coordinator/  ← Reads from one queue, writes to type-specific queues
│   ├── go.mod
│   └── main.go
│
└── workers/
    ├── image-worker/   ← Consumes jobs.image queue, does "image" work
    ├── data-worker/    ← Consumes jobs.data queue
    └── report-worker/  ← Consumes jobs.report queue
```

- **One service = one module = one `main.go`**. Each compiles to its own binary.
- **`shared`** has no `main` — it's a **library** other modules import.

---

## 3. Key Go Concepts We Use

### Packages and imports

Every `.go` file starts with `package main` (for runnable programs) or `package shared` (for the library). Then we import other packages:

```go
import (
    "encoding/json"   // standard library
    "github.com/gin-gonic/gin"   // external dependency
    "github.com/jpp/shared"     // our shared types
)
```

- **Standard library**: `encoding/json`, `database/sql`, `net/http`, `log`, `os`, etc. — no `go get` needed.
- **External**: `github.com/gin-gonic/gin` — listed in `go.mod` and downloaded with `go mod tidy`.
- **Our code**: `github.com/jpp/shared` — resolved via `go.work` (workspace) or `replace` in `go.mod` to the local `../shared` folder.

---

### Structs (our “data shapes”)

In `shared/models.go` we define **structs** — they’re like a single “object” type with fixed fields:

```go
type Job struct {
    ID        string    `json:"id"`
    Type      JobType   `json:"type"`
    Status    JobStatus `json:"status"`
    Payload   string    `json:"payload"`
    Result    *string   `json:"result,omitempty"`
    Error     *string   `json:"error,omitempty"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}
```

- **`json:"..."`** — “when we encode/decode JSON, use this name.” So Go’s `Status` becomes `"status"` in JSON.
- **`*string`** — pointer: “maybe no value yet.” In JSON we can omit it (`omitempty`). In the DB this is NULL.
- **`JobType`** and **`JobStatus`** — custom types we define in the same file (see “Constants and custom types” below).

So: **structs = the shapes of our jobs and messages**; they’re shared so API, coordinator, and workers all agree on the same structure.

---

### Constants and custom types

We use **typed constants** so “status” and “type” aren’t random strings:

```go
type JobStatus string

const (
    StatusPending    JobStatus = "pending"
    StatusProcessing JobStatus = "processing"
    StatusCompleted  JobStatus = "completed"
    StatusFailed     JobStatus = "failed"
)

type JobType string

const (
    JobTypeImage  JobType = "image"
    JobTypeData   JobType = "data"
    JobTypeReport JobType = "report"
)
```

- **`JobStatus`** and **`JobType`** are still strings underneath, but the type system prevents mixing them with other strings and gives autocomplete.
- Anywhere we use `shared.StatusPending` or `shared.JobTypeImage`, the meaning is clear and consistent.

---

### Functions

- **Capitalized name** = exported (other packages can use it): `shared.Job`, `shared.StatusPending`.
- **Lowercase name** = private to the package: `loadConfig()`, `getEnv()`.

We often return **error** as the last value:

```go
func doSomething() (string, error) {
    if err != nil {
        return "", err   // signal failure
    }
    return result, nil   // nil = no error
}
```

Callers check it:

```go
result, err := doSomething()
if err != nil {
    log.Printf("failed: %v", err)
    return
}
// use result
```

No try/catch — you handle errors where they happen.

---

### HTTP server (api-service)

We use **Gin**, a popular HTTP framework. Pattern:

```go
r := gin.Default()
r.GET("/health", app.healthHandler)
r.POST("/jobs", app.createJobHandler)
r.GET("/jobs/:id", app.getJobHandler)
r.Run(":8080")
```

- **Handlers** are functions that receive `*gin.Context` (request + response). We read body/params and call `c.JSON(status, data)` to respond.
- **`app`** is a struct holding DB and RabbitMQ connections so every handler can use them (dependency injection by hand).

Example of reading JSON body and writing to DB:

```go
var req struct {
    Type    shared.JobType `json:"type" binding:"required"`
    Payload string         `json:"payload"`
}
if err := c.ShouldBindJSON(&req); err != nil {
    c.JSON(400, gin.H{"error": err.Error()})
    return
}
// ... create job, insert into DB, publish to RabbitMQ
c.JSON(201, job)
```

- **`binding:"required"`** — Gin validates that `type` is present.
- **`gin.H`** — shortcut for `map[string]interface{}`, easy for small JSON objects.

---

### Database (Postgres)

We use **`database/sql`** plus the **`lib/pq`** driver. No ORM in this project.

```go
db, err := sql.Open("postgres", cfg.DatabaseURL)
// ...
_, err = db.Exec(
    `INSERT INTO jobs (id, type, status, payload, created_at, updated_at)
     VALUES ($1, $2, $3, $4, $5, $6)`,
    job.ID, job.Type, job.Status, job.Payload, job.CreatedAt, job.UpdatedAt,
)
```

- **`$1, $2, ...`** — placeholders; arguments follow in order. This avoids SQL injection.
- **`db.Exec`** — run a statement that doesn’t return rows.
- **`db.QueryRow`** — one row; we **Scan** into variables:  
  `err := row.Scan(&job.ID, &job.Type, ...)`
- **`db.Query`** — multiple rows; we **iterate** with `rows.Next()` and `rows.Scan(...)`.

So: **we write raw SQL and map rows to our structs manually.** Simple and transparent.

---

### RabbitMQ (message queue)

We use **amqp091-go**. Pattern:

**Publish (api-service):**

```go
body, _ := json.Marshal(msg)
err = ch.Publish("", "jobs.pending", false, false, amqp.Publishing{
    ContentType:  "application/json",
    DeliveryMode: amqp.Persistent,
    Body:         body,
})
```

**Consume (coordinator and workers):**

```go
msgs, err := ch.Consume("jobs.pending", "coordinator", false, false, false, false, nil)
for msg := range msgs {
    var job shared.JobMessage
    json.Unmarshal(msg.Body, &job)
    // ... do work ...
    msg.Ack(false)   // tell RabbitMQ we're done with this message
}
```

- **Queue names** — `jobs.pending`, `jobs.image`, `jobs.data`, `jobs.report` — are just strings we all agree on.
- **JSON** — we **marshal** our structs to bytes for the body and **unmarshal** back into structs when consuming.
- **Ack/Nack** — we must ack when we’re done (or nack to requeue); otherwise RabbitMQ thinks the message wasn’t processed.

---

### Configuration via environment

We read config from the **environment** so the same binary works locally and in Kubernetes:

```go
func getEnv(key, fallback string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return fallback
}

port := getEnv("PORT", "8080")
dbURL := getEnv("DATABASE_URL", "postgres://jpp:jpp@localhost:5432/jpp?sslmode=disable")
```

- **Locally** — we don’t set env vars; defaults (e.g. localhost Postgres) are used.
- **In K8s** — we set `DATABASE_URL` (or inject from a Secret) in the deployment YAML.

---

## 4. How the Pieces Fit Together

| Step | Who | What happens in Go |
|------|-----|---------------------|
| 1 | Client | HTTP `POST /jobs` with JSON body |
| 2 | api-service | `createJobHandler`: bind JSON → struct, insert row in Postgres, publish `JobMessage` to `jobs.pending` |
| 3 | job-coordinator | Consume from `jobs.pending`, unmarshal to `JobMessage`, switch on `Type`, publish to `jobs.image` / `jobs.data` / `jobs.report` |
| 4 | worker | Consume from e.g. `jobs.image`, unmarshal, update row to `processing`, call `processImageJob()`, update row to `completed` or `failed`, ack message |
| 5 | Client | HTTP `GET /jobs/:id` → api-service reads row from Postgres and returns JSON |

So: **same structs in shared**, **JSON over HTTP and AMQP**, **Postgres as source of truth**. Go just ties HTTP, SQL, and RabbitMQ together with a small amount of code.

---

## 5. Commands You’ll Use

| Command | Meaning |
|--------|--------|
| `go mod tidy` | Fix `go.mod` and download deps (run inside a module directory or at repo root with workspace). |
| `go run main.go` | Build and run the current module (e.g. `backend/api-service`). |
| `go build ./...` | Build all packages in the current module. |
| `go test ./...` | Run tests in the current module. |
| `go work sync` | Sync workspace modules (when using `go.work`). |

We use **`replace github.com/jpp/shared => ../shared`** in each service’s `go.mod` so they use the local `shared` package instead of a remote repo.

---

## 6. Where to Look Next

- **Change an API field** — edit the struct in `shared/models.go` and the handlers in `backend/api-service/main.go`.
- **Add a new job type** — add constant in `shared`, new queue and worker folder, new case in coordinator.
- **See the full flow** — put a `log.Printf(...)` in api-service (after insert), coordinator (after route), and a worker (after process); submit a job and watch logs.

Once you’re comfortable with structs, handlers, and DB/queue calls, the rest is the same ideas repeated across services.

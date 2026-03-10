# Local Development

The fastest development loop is: run **data services in K8s**, run **your service locally**.

## Start Data Services in K8s

```bash
make k8s-up
make k8s-namespaces
make k8s-deploy-data
```

Then port-forward data services so your local code can reach them:

```bash
# Postgres on localhost:5432
kubectl port-forward svc/postgres 5432:5432 -n data &

# RabbitMQ on localhost:5672
kubectl port-forward svc/rabbitmq 5672:5672 -n data &
```

## Run api-service Locally

```bash
cd backend/api-service
go run main.go
```

Environment variables with defaults (no `.env` needed for local dev):

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP listen port |
| `DATABASE_URL` | `postgres://jpp:jpp@localhost:5432/jpp?sslmode=disable` | Postgres DSN |
| `RABBITMQ_URL` | `amqp://guest:guest@localhost:5672/` | RabbitMQ URL |

## Run a Worker Locally

```bash
cd backend/workers/image-worker
go run main.go
```

## Rebuild and Redeploy to K8s

```bash
# Build Docker image and load into kind cluster
make docker-build SERVICE=api-service
kind load docker-image localhost:5001/api-service:latest --name kind-job-platform

# Apply updated manifest
make k8s-deploy SERVICE=api-service
make k8s-restart SERVICE=api-service
```

## Run Tests

```bash
make test
```

---

_Add notes here as you discover useful local dev tricks._

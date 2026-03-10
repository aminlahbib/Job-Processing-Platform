# Deployment

## Local (kind)

See [SETUP.md](SETUP.md).

## Production (GCP — Week 8+)

_To be filled once the local setup is stable._

Planned stack:
- GKE (Google Kubernetes Engine) cluster
- Cloud SQL for Postgres
- Memorystore for Redis
- Cloud Pub/Sub (or keep RabbitMQ on GKE)
- Artifact Registry for Docker images
- Cloud Load Balancing for ingress

## CI/CD

GitHub Actions workflows in `.github/workflows/`:
- `test.yml` — runs on every PR: `go test ./...`
- `build.yml` — on merge to main: builds and pushes Docker images
- `deploy.yml` — on release tag: deploys to GKE

---

_Fill in GCP-specific steps as you reach Week 8._

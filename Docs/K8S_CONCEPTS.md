# Kubernetes Concepts

This project touches the most important K8s primitives. Each is explained in context.

## Namespace

Logical isolation within a cluster. We use 4:
- `app` — application services (api, coordinator, workers)
- `data` — stateful data services (postgres, redis, rabbitmq)
- `auth` — Keycloak
- `observability` — Prometheus, Grafana, Loki, Jaeger

## Deployment

Manages stateless pods with rolling updates and replica control.
Used for: api-service, job-coordinator, workers, Keycloak, Grafana.

```bash
kubectl get deployments -n app
kubectl rollout status deployment/api-service -n app
kubectl rollout history deployment/api-service -n app
```

## StatefulSet

Like a Deployment but gives pods a stable identity and persistent storage.
Used for: Postgres, RabbitMQ.

```bash
kubectl get statefulsets -n data
```

## Service

Stable network endpoint for a group of pods (load-balances across replicas).
Types:
- `ClusterIP` — internal only (most services here)
- `NodePort` — exposed on the node (used for local kind access)

## ConfigMap

Key-value config injected into pods as env vars or files.
Used for: Postgres init SQL, Prometheus scrape config.

## Secret

Like ConfigMap but base64-encoded (and ideally encrypted at rest).
Used for: Postgres passwords, RabbitMQ credentials.

**Never commit real secrets to git.** Use `.gitignore` patterns for secret files.

## PersistentVolumeClaim (PVC)

Requests persistent disk storage. Kind provisions local-path storage automatically.
Used for: Postgres data directory.

## HorizontalPodAutoscaler (HPA)

Scales replicas based on CPU, memory, or custom metrics (e.g. queue depth).
Added in Week 7 for the workers.

## Liveness vs Readiness Probes

- **Liveness** — is the container still alive? If not, K8s restarts it.
- **Readiness** — is the container ready to receive traffic? If not, K8s removes it from load balancing.

---

_Add notes on each concept as you encounter it._

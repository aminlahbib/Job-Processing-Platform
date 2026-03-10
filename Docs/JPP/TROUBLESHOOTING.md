# Troubleshooting

## Cluster Issues

### `kind create cluster` hangs
- Check Docker is running: `docker ps`
- Try: `kind delete cluster --name kind-job-platform` then `make k8s-up` again

### Pods stuck in `Pending`
```bash
kubectl describe pod <pod-name> -n <namespace>
```
Common causes: insufficient resources, PVC not binding, image pull failure.

### Pods in `CrashLoopBackOff`
```bash
kubectl logs <pod-name> -n <namespace> --previous
```
Check the logs from the last crash.

## Networking Issues

### `Connection refused` to Postgres locally
Make sure port-forward is running:
```bash
kubectl port-forward svc/postgres 5432:5432 -n data
```

### API can't reach RabbitMQ in K8s
Check the service name resolves: services in the same namespace use their short name, cross-namespace needs `svc.namespace.svc.cluster.local`.

## Go Build Issues

### `cannot find module providing package`
Run from repo root:
```bash
go work sync
cd backend/<service> && go mod tidy
```

### Import cycle
The `shared` module must not import any service module. Only services import `shared`.

## RabbitMQ

### Queue not receiving messages
1. Check RabbitMQ Management UI: `make rabbitmq-port-forward`
2. Verify queue is declared (it should be idempotent on service start)
3. Check credentials match between publisher and consumer

---

_Add new issues here as you encounter them._

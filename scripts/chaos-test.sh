#!/usr/bin/env bash
# chaos-test.sh — simulate failure scenarios and verify the system recovers
# TODO (Week 8): implement with proper chaos tooling (chaos-mesh or similar)
# Usage: ./scripts/chaos-test.sh <scenario>
# Scenarios: kill-worker, kill-rabbitmq, kill-postgres, network-partition

set -euo pipefail

SCENARIO=${1:-""}
NAMESPACE_APP="app"
NAMESPACE_DATA="data"

case "$SCENARIO" in
  kill-worker)
    echo "Chaos: deleting image-worker pods (K8s will restart them)..."
    kubectl delete pods -l app=image-worker -n "$NAMESPACE_APP"
    echo "Watch recovery: kubectl get pods -n $NAMESPACE_APP -w"
    ;;
  kill-rabbitmq)
    echo "Chaos: deleting rabbitmq pod (StatefulSet will restart it)..."
    kubectl delete pod rabbitmq-0 -n "$NAMESPACE_DATA"
    echo "Watch recovery: kubectl get pods -n $NAMESPACE_DATA -w"
    ;;
  kill-postgres)
    echo "Chaos: deleting postgres pod (StatefulSet will restart it with same PVC)..."
    kubectl delete pod postgres-0 -n "$NAMESPACE_DATA"
    echo "Watch recovery: kubectl get pods -n $NAMESPACE_DATA -w"
    ;;
  network-partition)
    echo "Chaos: applying NetworkPolicy to isolate app namespace from data..."
    echo "TODO: implement with NetworkPolicy resources"
    ;;
  *)
    echo "Usage: $0 <scenario>"
    echo "Scenarios:"
    echo "  kill-worker        Delete worker pods (tests K8s restart)"
    echo "  kill-rabbitmq      Delete RabbitMQ pod (tests message durability)"
    echo "  kill-postgres      Delete Postgres pod (tests PVC persistence)"
    echo "  network-partition  Isolate namespaces (tests error handling)"
    exit 1
    ;;
esac

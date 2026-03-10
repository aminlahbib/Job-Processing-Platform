#!/usr/bin/env bash
# setup.sh — one-command initialization for the Job Processing Platform
# Run: make setup

set -euo pipefail

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

ok()   { echo -e "${GREEN}✓${NC} $1"; }
warn() { echo -e "${YELLOW}!${NC} $1"; }
fail() { echo -e "${RED}✗${NC} $1"; exit 1; }
step() { echo -e "\n${YELLOW}→${NC} $1"; }

echo ""
echo "Job Processing Platform — Setup"
echo "================================"

# ─────────────────────────────────────────────
# 1. Check prerequisites
# ─────────────────────────────────────────────
step "Checking prerequisites..."

check_cmd() {
  if command -v "$1" &>/dev/null; then
    ok "$1 found: $(command -v "$1")"
  else
    fail "$1 not found. Install it and re-run setup."
  fi
}

check_cmd go
check_cmd docker
check_cmd kind
check_cmd kubectl
check_cmd helm

# Verify Go version >= 1.23
GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
REQUIRED="1.23"
if [ "$(printf '%s\n' "$REQUIRED" "$GO_VERSION" | sort -V | head -n1)" != "$REQUIRED" ]; then
  fail "Go $REQUIRED+ required, found $GO_VERSION"
fi
ok "Go version: $GO_VERSION"

# Verify Docker is running
if ! docker info &>/dev/null; then
  fail "Docker is not running. Start Docker Desktop and retry."
fi
ok "Docker is running"

# ─────────────────────────────────────────────
# 2. Create kind cluster
# ─────────────────────────────────────────────
step "Setting up Kubernetes cluster..."

CLUSTER_NAME="kind-job-platform"

if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
  warn "Cluster '${CLUSTER_NAME}' already exists — skipping creation"
else
  kind create cluster --name "$CLUSTER_NAME" --config infra/kind/kind-config.yaml
  ok "Cluster '${CLUSTER_NAME}' created"
fi

# Set kubectl context to the new cluster
kubectl config use-context "kind-${CLUSTER_NAME}" 2>/dev/null || \
  kubectl config use-context "${CLUSTER_NAME}" 2>/dev/null || \
  warn "Could not switch kubectl context — you may need to do this manually"

ok "kubectl context: $(kubectl config current-context)"

# ─────────────────────────────────────────────
# 3. Create namespaces
# ─────────────────────────────────────────────
step "Creating namespaces..."
kubectl apply -f infra/k8s/namespaces.yaml
ok "Namespaces created: app, data, auth, observability"

# ─────────────────────────────────────────────
# 4. Download Go dependencies
# ─────────────────────────────────────────────
step "Downloading Go dependencies..."
go work sync
ok "Go workspace synced"

for dir in backend/shared backend/api-service backend/job-coordinator \
           backend/workers/image-worker backend/workers/data-worker backend/workers/report-worker; do
  if [ -f "$dir/go.mod" ]; then
    (cd "$dir" && go mod tidy) && ok "  $dir — dependencies ready"
  fi
done

# ─────────────────────────────────────────────
# Done
# ─────────────────────────────────────────────
echo ""
echo -e "${GREEN}Setup complete!${NC}"
echo ""
echo "Next steps:"
echo "  make k8s-deploy-data        # Deploy Postgres, Redis, RabbitMQ"
echo "  make k8s-deploy-all         # Deploy everything"
echo "  make k8s-status             # Check cluster status"
echo "  cd backend/api-service && go run main.go   # Run API locally"
echo ""

# ============================================================
# Job Processing Platform — Makefile
# Run `make help` to see all available commands.
# ============================================================

.PHONY: help setup deps \
        build build-all test lint fmt \
        docker-build docker-build-all docker-push \
        k8s-up k8s-down k8s-status \
        k8s-namespaces k8s-deploy-data k8s-deploy-auth k8s-deploy-services k8s-deploy-observability k8s-deploy-all \
        k8s-deploy k8s-logs k8s-scale k8s-shell k8s-restart \
        grafana-port-forward prometheus-port-forward jaeger-port-forward rabbitmq-port-forward api-port-forward \
        clean clean-docker

.DEFAULT_GOAL := help

CLUSTER_NAME   := kind-job-platform
REGISTRY       := localhost:5001
NAMESPACE_APP  := app
NAMESPACE_DATA := data

SERVICES := api-service job-coordinator image-worker data-worker report-worker

# ─────────────────────────────────────────────
# Help
# ─────────────────────────────────────────────

help: ## Show this help message
	@echo ""
	@echo "Job Processing Platform — Available Commands"
	@echo "============================================"
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-32s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)
	@echo ""

# ─────────────────────────────────────────────
# Initial Setup
# ─────────────────────────────────────────────

setup: ## One-time setup: check deps, create cluster, bootstrap base services
	@chmod +x scripts/setup.sh && ./scripts/setup.sh

deps: ## Download all Go module dependencies (run after cloning)
	@echo "→ Syncing Go workspace..."
	@go work sync
	@echo "→ Tidying shared module..."
	@cd backend/shared && go mod tidy
	@echo "→ Tidying api-service..."
	@cd backend/api-service && go mod tidy
	@echo "→ Tidying job-coordinator..."
	@cd backend/job-coordinator && go mod tidy
	@echo "→ Tidying image-worker..."
	@cd backend/workers/image-worker && go mod tidy
	@echo "→ Tidying data-worker..."
	@cd backend/workers/data-worker && go mod tidy
	@echo "→ Tidying report-worker..."
	@cd backend/workers/report-worker && go mod tidy
	@echo "✓ All dependencies downloaded"

# ─────────────────────────────────────────────
# Build & Test
# ─────────────────────────────────────────────

build: ## Build one service binary: make build SERVICE=api-service
	@[ -n "$(SERVICE)" ] || (echo "Usage: make build SERVICE=<name>"; exit 1)
	@echo "→ Building $(SERVICE)..."
	@mkdir -p bin
	@go build -o bin/$(SERVICE) ./backend/$(SERVICE)/...

build-all: ## Build all service binaries into bin/
	@mkdir -p bin
	@echo "→ Building all services..."
	@go build ./backend/shared/...
	@go build ./backend/api-service/...
	@go build ./backend/job-coordinator/...
	@go build ./backend/workers/image-worker/...
	@go build ./backend/workers/data-worker/...
	@go build ./backend/workers/report-worker/...
	@echo "✓ Build complete"

test: ## Run all tests across all modules
	@echo "→ Running tests..."
	@go test ./backend/shared/... ./backend/api-service/... ./backend/job-coordinator/... \
	  ./backend/workers/image-worker/... ./backend/workers/data-worker/... ./backend/workers/report-worker/... \
	  -v -timeout 60s

lint: ## Run golangci-lint (install: brew install golangci-lint)
	@golangci-lint run ./backend/...

fmt: ## Format all Go code with gofmt
	@gofmt -w ./backend/
	@echo "✓ Code formatted"

# ─────────────────────────────────────────────
# Docker
# ─────────────────────────────────────────────

docker-build: ## Build Docker image: make docker-build SERVICE=api-service
	@[ -n "$(SERVICE)" ] || (echo "Usage: make docker-build SERVICE=<name>"; exit 1)
	@echo "→ Building Docker image: $(REGISTRY)/$(SERVICE):latest"
	@docker build -t $(REGISTRY)/$(SERVICE):latest \
		-f infra/docker/$(SERVICE).Dockerfile .

docker-build-all: ## Build Docker images for all services
	@for svc in $(SERVICES); do \
		$(MAKE) docker-build SERVICE=$$svc; \
	done

docker-push: ## Push image to registry: make docker-push SERVICE=api-service
	@[ -n "$(SERVICE)" ] || (echo "Usage: make docker-push SERVICE=<name>"; exit 1)
	@docker push $(REGISTRY)/$(SERVICE):latest

# ─────────────────────────────────────────────
# Kubernetes — Cluster Lifecycle
# ─────────────────────────────────────────────

k8s-up: ## Create the local kind cluster
	@echo "→ Creating kind cluster: $(CLUSTER_NAME)"
	@kind create cluster --name $(CLUSTER_NAME) --config infra/kind/kind-config.yaml \
		|| echo "Cluster '$(CLUSTER_NAME)' may already exist."

k8s-down: ## DESTRUCTIVE: Delete the local kind cluster and all data
	@echo "→ Deleting cluster: $(CLUSTER_NAME)"
	@kind delete cluster --name $(CLUSTER_NAME)

k8s-status: ## Show pod status across all namespaces
	@echo "\n=== Nodes ==="; kubectl get nodes
	@echo "\n=== app namespace ==="; kubectl get pods -n app 2>/dev/null || true
	@echo "\n=== data namespace ==="; kubectl get pods -n data 2>/dev/null || true
	@echo "\n=== auth namespace ==="; kubectl get pods -n auth 2>/dev/null || true
	@echo "\n=== observability namespace ==="; kubectl get pods -n observability 2>/dev/null || true

# ─────────────────────────────────────────────
# Kubernetes — Deploy
# ─────────────────────────────────────────────

k8s-namespaces: ## Create all namespaces
	kubectl apply -f infra/k8s/namespaces.yaml

k8s-deploy-data: ## Deploy data tier: Postgres, Redis, RabbitMQ
	kubectl apply -f infra/k8s/postgres/
	kubectl apply -f infra/k8s/redis/
	kubectl apply -f infra/k8s/rabbitmq/

k8s-deploy-auth: ## Deploy Keycloak
	kubectl apply -f infra/k8s/keycloak/

k8s-deploy-services: ## Deploy application services (api, coordinator, workers)
	kubectl apply -f infra/k8s/services/

k8s-deploy-observability: ## Deploy Prometheus, Grafana, Loki, Jaeger
	kubectl apply -f infra/k8s/observability/

k8s-deploy-production: ## Deploy NetworkPolicies and PodDisruptionBudgets
	kubectl apply -f infra/k8s/network-policy.yaml
	kubectl apply -f infra/k8s/pdb.yaml
	kubectl apply -f infra/k8s/services/hpa-workers.yaml

k8s-deploy-all: k8s-namespaces k8s-deploy-data k8s-deploy-auth k8s-deploy-services k8s-deploy-observability k8s-deploy-production ## Deploy everything
	@echo "✓ All resources applied. Check with: make k8s-status"

k8s-deploy: ## Re-deploy one service: make k8s-deploy SERVICE=api-service
	@[ -n "$(SERVICE)" ] || (echo "Usage: make k8s-deploy SERVICE=<name>"; exit 1)
	kubectl apply -f infra/k8s/services/$(SERVICE).yaml

# ─────────────────────────────────────────────
# Kubernetes — Operations
# ─────────────────────────────────────────────

k8s-logs: ## Tail logs for a service: make k8s-logs SERVICE=api-service
	@[ -n "$(SERVICE)" ] || (echo "Usage: make k8s-logs SERVICE=<name>"; exit 1)
	kubectl logs -f deployment/$(SERVICE) -n $(NAMESPACE_APP) --tail=100

k8s-scale: ## Scale a service: make k8s-scale SERVICE=image-worker REPLICAS=3
	@[ -n "$(SERVICE)" ] || (echo "Usage: make k8s-scale SERVICE=<name> REPLICAS=<n>"; exit 1)
	@[ -n "$(REPLICAS)" ] || (echo "Usage: make k8s-scale SERVICE=<name> REPLICAS=<n>"; exit 1)
	kubectl scale deployment/$(SERVICE) --replicas=$(REPLICAS) -n $(NAMESPACE_APP)

k8s-shell: ## Open a shell in a running pod: make k8s-shell SERVICE=api-service
	@[ -n "$(SERVICE)" ] || (echo "Usage: make k8s-shell SERVICE=<name>"; exit 1)
	kubectl exec -it deployment/$(SERVICE) -n $(NAMESPACE_APP) -- /bin/sh

k8s-restart: ## Rolling restart a deployment: make k8s-restart SERVICE=api-service
	@[ -n "$(SERVICE)" ] || (echo "Usage: make k8s-restart SERVICE=<name>"; exit 1)
	kubectl rollout restart deployment/$(SERVICE) -n $(NAMESPACE_APP)

# ─────────────────────────────────────────────
# Port Forwarding
# ─────────────────────────────────────────────

api-port-forward: ## Port-forward API to localhost:8080
	@echo "→ API: http://localhost:8080"
	kubectl port-forward svc/api-service 8080:8080 -n $(NAMESPACE_APP)

grafana-port-forward: ## Open Grafana at http://localhost:3000 (admin/admin)
	@echo "→ Grafana: http://localhost:3000 (admin/admin)"
	kubectl port-forward svc/grafana 3000:3000 -n observability

prometheus-port-forward: ## Open Prometheus at http://localhost:9090
	@echo "→ Prometheus: http://localhost:9090"
	kubectl port-forward svc/prometheus 9090:9090 -n observability

jaeger-port-forward: ## Open Jaeger UI at http://localhost:16686
	@echo "→ Jaeger: http://localhost:16686"
	kubectl port-forward svc/jaeger 16686:16686 -n observability

rabbitmq-port-forward: ## Open RabbitMQ Management UI at http://localhost:15672 (guest/guest)
	@echo "→ RabbitMQ: http://localhost:15672 (guest/guest)"
	kubectl port-forward svc/rabbitmq 15672:15672 -n $(NAMESPACE_DATA)

# ─────────────────────────────────────────────
# Cleanup
# ─────────────────────────────────────────────

clean: ## Remove build artifacts
	@rm -rf bin/
	@find . -name '*.test' -delete
	@find . -name 'coverage.out' -delete
	@echo "✓ Cleaned build artifacts"

clean-docker: ## Remove locally built Docker images
	@for svc in $(SERVICES); do \
		docker rmi $(REGISTRY)/$$svc:latest 2>/dev/null || true; \
	done
	@echo "✓ Cleaned Docker images"

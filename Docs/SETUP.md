# Setup Guide

## Prerequisites

Install these tools before starting:

| Tool | Install | Purpose |
|------|---------|---------|
| Go 1.23+ | `brew install go` | Build backend services |
| Docker | [docker.com](https://docker.com) | Container runtime |
| kind | `brew install kind` | Local Kubernetes cluster |
| kubectl | `brew install kubectl` | Kubernetes CLI |
| Helm | `brew install helm` | K8s package manager |

Verify all tools:
```bash
go version       # go1.23+
docker version   # 24+
kind version     # v0.20+
kubectl version  # v1.28+
helm version     # v3.13+
```

## One-Command Setup

```bash
make setup
```

This script:
1. Checks all prerequisites above
2. Creates a local Kubernetes cluster (`kind-job-platform`)
3. Creates namespaces: `app`, `data`, `auth`, `observability`
4. Downloads Go dependencies

## Verify Setup

```bash
make k8s-status
```

## Deploy Everything

```bash
make k8s-deploy-all
make k8s-status
```

## Test the API

```bash
make api-port-forward &
curl http://localhost:8080/health
```

Expected response:
```json
{"database":"ok","service":"api-service","status":"ok","time":"..."}
```

## Submit Your First Job

```bash
curl -X POST http://localhost:8080/jobs \
  -H "Content-Type: application/json" \
  -d '{"type":"image","payload":"{\"url\":\"https://example.com/image.jpg\"}"}'
```

---

_Fill this section with step-by-step notes as you work through setup._

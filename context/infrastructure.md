# Penfold Infrastructure

Deployment-specific configuration for Penfold services. Last verified: 2026-01-20.

> **See also:** [ARCHITECTURE.md](ARCHITECTURE.md) for component design and data flow patterns.

## Hostnames

| Hostname | IP | Role |
|----------|-----|------|
| `dev01.brown.chat` | 10.0.10.144 | Development machine, MLX inference |
| `home-01.brown.chat` | 10.0.10.253 | Data services, Gateway |

Use hostnames in all configs for portability. IPs may change.

## Deployment Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        dev01.brown.chat (Mac Mini M4)                       │
│                                                                             │
│  ┌──────────────────────────┐    ┌──────────────────────────┐              │
│  │  MLX Embeddings Sidecar  │    │  Penfold Worker          │              │
│  │  localhost:8081          │◄───│  Health: localhost:8085  │              │
│  │  mxbai-embed-large-v1    │    │                          │              │
│  └──────────────────────────┘    └────────────┬─────────────┘              │
│                                               │                             │
│  ┌──────────────────────────┐                 │                             │
│  │  penf CLI                │                 │                             │
│  │  → home-01.brown.chat    │                 │                             │
│  └──────────────────────────┘                 │                             │
└───────────────────────────────────────────────┼─────────────────────────────┘
                                                │
                                    Network (1 Gbps)
                                                │
┌───────────────────────────────────────────────┼─────────────────────────────┐
│                      home-01.brown.chat (Intel NUC)                         │
│                                               │                             │
│  ┌──────────────────────────┐                 │                             │
│  │  Penfold Gateway         │◄────────────────┘                             │
│  │  gRPC: :50051            │                                               │
│  │  HTTP: :8080             │                                               │
│  └────────────┬─────────────┘                                               │
│               │ localhost                                                   │
│  ┌────────────▼─────────────┐  ┌─────────────────┐  ┌─────────────────┐    │
│  │  PostgreSQL              │  │  Redis          │  │  Temporal       │    │
│  │  :5432                   │  │  :6379          │  │  :7233          │    │
│  │  penfold-postgres        │  │  penfold-redis  │  │  UI: :8088      │    │
│  └──────────────────────────┘  └─────────────────┘  └─────────────────┘    │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Service Configuration

### dev01.brown.chat

| Service | Port | Co-located Access | Notes |
|---------|------|-------------------|-------|
| MLX Embeddings | 8081 | `localhost:8081` | Apple Silicon required |
| MLX LLM Server | 8080 | `localhost:8080` | Qwen2.5-32B for mention resolution |
| Worker | 8085 | - | Health endpoint only |

**Embeddings Sidecar:**
```bash
# Location
penfold-go-pipeline/sidecar

# Model
mixedbread-ai/mxbai-embed-large-v1 (1024 dimensions)

# Managed by launchd (auto-restarts on reboot)
launchctl list | grep penfold.mlx-embeddings

# Manual start (if not using launchd)
.venv/bin/uvicorn app:app --host 0.0.0.0 --port 8081
```

**LLM Server (mlx_lm.server):**
```bash
# Location
penfold-go-pipeline/sidecar

# Model
mlx-community/Qwen2.5-32B-Instruct-4bit

# Managed by launchd (auto-restarts on reboot)
launchctl list | grep penfold.mlx-llm-server

# Manual start (if not using launchd)
.venv/bin/mlx_lm.server --model mlx-community/Qwen2.5-32B-Instruct-4bit --port 8080 --host 0.0.0.0
```

**Launchd Services (dev01):**
```bash
# List penfold services
launchctl list | grep penfold

# Service plists location
~/Library/LaunchAgents/com.penfold.mlx-embeddings.plist
~/Library/LaunchAgents/com.penfold.mlx-llm-server.plist

# Reload a service
launchctl unload ~/Library/LaunchAgents/com.penfold.mlx-llm-server.plist
launchctl load ~/Library/LaunchAgents/com.penfold.mlx-llm-server.plist

# View logs
tail -f /tmp/mlx-embeddings.log
tail -f /tmp/mlx-llm-server.log
```

**Worker Environment:**
```bash
# See ~/github/otherjamesbrown/secrets/.env.penfold for actual credentials
PENFOLD_DB_HOST=home-01.brown.chat
PENFOLD_DB_PORT=5432
PENFOLD_DB_USER=penfold
PENFOLD_DB_PASSWORD=<see secrets/.env.penfold>
PENFOLD_DB_NAME=penfold
PENFOLD_TEMPORAL_HOST=home-01.brown.chat:7233
AI_SERVICE_URL=http://localhost:8081  # MLX embeddings (local to dev01)
LLM_URL=http://localhost:8080         # MLX LLM server (local to dev01)
LLM_MODEL=mlx-community/Qwen2.5-32B-Instruct-4bit
```

### home-01.brown.chat

| Service | Port | Container | Notes |
|---------|------|-----------|-------|
| Gateway (gRPC) | 50051 | /tmp/penfold-gateway | Main API |
| Gateway (HTTP) | 8080 | /tmp/penfold-gateway | Health, metrics |
| PostgreSQL | 5432 | penfold-postgres | pgvector, TimescaleDB |
| Redis | 6379 | penfold-redis | No password |
| Temporal | 7233 | penfold-temporal | Workflow engine |
| Temporal UI | 8088 | penfold-temporal-ui | Web dashboard |
| Langfuse Web | 3000 | langfuse-web | AI Provenance UI |
| Langfuse PostgreSQL | 5433 | langfuse-postgres | Langfuse data |
| Langfuse Redis | 6380 | langfuse-redis | Langfuse cache |
| Langfuse ClickHouse | 8123 | langfuse-clickhouse | Trace storage |
| Langfuse MinIO | 9092 | langfuse-minio | Blob storage |

**Gateway Environment:**
```bash
# See ~/github/otherjamesbrown/secrets/.env.penfold for actual credentials
PENFOLD_SERVICE_NAME=gateway
PENFOLD_DB_HOST=localhost  # Co-located with PostgreSQL
PENFOLD_DB_PORT=5432
PENFOLD_DB_USER=penfold
PENFOLD_DB_PASSWORD=<see secrets/.env.penfold>
PENFOLD_DB_NAME=penfold
```

## Connection Strings

### PostgreSQL (from dev01)
```bash
# Load environment from secrets
source ~/github/otherjamesbrown/secrets/.env.penfold

# DSN uses keyword-value format (handles special chars in password)
host=$PENFOLD_DB_HOST port=$PENFOLD_DB_PORT user=$PENFOLD_DB_USER password=$PENFOLD_DB_PASSWORD dbname=$PENFOLD_DB_NAME sslmode=disable

# psql (after sourcing .env.penfold)
psql "host=$PENFOLD_DB_HOST port=$PENFOLD_DB_PORT user=$PENFOLD_DB_USER password=$PENFOLD_DB_PASSWORD dbname=$PENFOLD_DB_NAME"
```

### PostgreSQL (from Gateway on home-01)
```bash
# Co-located, use localhost
PENFOLD_DB_HOST=localhost
```

### Redis
```bash
REDIS_HOST=home-01.brown.chat
REDIS_PORT=6379
# No password
```

### Temporal
```bash
PENFOLD_TEMPORAL_HOST=home-01.brown.chat:7233
TEMPORAL_NAMESPACE=default
```

### Langfuse (AI Provenance)
```bash
# Web UI
LANGFUSE_HOST=http://home-01.brown.chat:3000

# API keys for trace ingestion
LANGFUSE_PUBLIC_KEY=pk-lf-penfold
LANGFUSE_SECRET_KEY=sk-lf-penfold-secret

# OpenTelemetry endpoint (for OTEL SDK)
OTEL_EXPORTER_OTLP_ENDPOINT=http://home-01.brown.chat:3000/api/public/otel
```

Deployment: `~/langfuse/docker-compose.yml` on home-01
Credentials: See `~/github/otherjamesbrown/secrets/.env.langfuse`

### penf CLI
```yaml
# ~/.penf/config.yaml
server_address: home-01.brown.chat:50051
timeout: 30s
output_format: text
insecure: true
```

## Starting Services

### Full Stack Startup

```bash
# 1. Verify Docker services on home-01
ssh home-01.brown.chat "docker ps --filter 'name=penfold'"

# 2. Start Gateway on home-01 (if not running)
ssh home-01.brown.chat "PENFOLD_SERVICE_NAME=gateway \
  PENFOLD_DB_HOST=localhost \
  PENFOLD_DB_PASSWORD=penfold \
  nohup /tmp/penfold-gateway > /tmp/gateway.log 2>&1 &"

# 3. Start Embeddings on dev01
cd penfold-go-pipeline/sidecar
.venv/bin/uvicorn app:app --host 0.0.0.0 --port 8081 &

# 4. Start LLM Server on dev01 (for mention resolution)
.venv/bin/mlx_lm.server --model mlx-community/Qwen2.5-32B-Instruct-4bit --port 8080 --host 0.0.0.0 &

# 5. Start Worker on dev01
PENFOLD_DB_HOST=home-01.brown.chat \
PENFOLD_TEMPORAL_HOST=home-01.brown.chat:7233 \
AI_SERVICE_URL=http://localhost:8081 \
LLM_URL=http://localhost:8080 \
./bin/penfold-worker &

# 6. Verify CLI connection
penf status
```

## Health Checks

### CLI Commands

```bash
# Check all services via gateway (from any machine)
penf health gateway

# Check local ML services (from dev01 only)
penf health local

# Standard health check via gRPC
penf health
penf status
```

### Gateway Health Aggregation

The gateway aggregates health status from all backend services and exposes it via HTTP:
- `http://home-01.brown.chat:8080/health` - Full health status with all services
- `http://home-01.brown.chat:8080/ready` - Kubernetes readiness probe
- `http://home-01.brown.chat:8080/live` - Kubernetes liveness probe

**Services monitored:**
- `database` - PostgreSQL connection (critical)
- `embeddings` - MLX embeddings service on dev01 (non-critical)
- `llm` - MLX LLM server on dev01 (non-critical)
- `worker` - Worker health endpoint on dev01 (non-critical)

**Gateway environment variables for ML service URLs:**
```bash
GATEWAY_EMBEDDINGS_URL=http://dev01.brown.chat:8081
GATEWAY_LLM_URL=http://dev01.brown.chat:8080
GATEWAY_WORKER_HEALTH_URL=http://dev01.brown.chat:8085
```

### Direct Service Health Checks

```bash
# Embeddings health (from dev01)
curl -s http://localhost:8081/health

# LLM Server health (from dev01)
curl -s http://localhost:8080/v1/models

# Worker health (from dev01)
curl -s http://localhost:8085/health

# Gateway aggregated health (from any machine)
curl -s http://home-01.brown.chat:8080/health | jq
```

## Verification Commands

```bash
# CLI → Gateway
penf status

# Full system health (includes ML services)
penf health gateway

# PostgreSQL (from dev01)
psql "host=home-01.brown.chat user=penfold password=penfold dbname=penfold" -c "SELECT 1"

# Docker containers (home-01)
ssh home-01.brown.chat "docker ps --filter 'name=penfold'"

# Temporal UI
open http://home-01.brown.chat:8088
```

## Design Rationale

**Why Worker on dev01?**
- Needs Apple Silicon for MLX embeddings
- Embedding calls stay local (large vectors, fast)
- DB/Temporal calls cross network (small payloads, acceptable latency)

**Why Gateway on home-01?**
- Co-located with PostgreSQL for fast queries
- CLI calls cross network (small gRPC payloads)

> **See also:** [ARCHITECTURE.md](ARCHITECTURE.md) for component responsibilities and data flow patterns.

## Credentials

See `~/github/otherjamesbrown/secrets/infrastructure.md` for passwords and API keys.

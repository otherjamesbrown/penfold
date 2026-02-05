# Penfold Infrastructure

Deployment-specific configuration for Penfold services. Last verified: 2026-01-31.

> **See also:** [ARCHITECTURE.md](ARCHITECTURE.md) for component design and data flow patterns.

---

## CRITICAL: Deployment Architecture

> **The CLI runs on James's LAPTOP, not on any server.**
>
> ```
> LAPTOP (MacBook Pro)          SERVERS
> ┌──────────────────┐          ┌─────────────────────────────────────┐
> │  penf CLI        │──gRPC───►│  dev02: Gateway (:50051)            │
> │  Claude Code     │          │         PostgreSQL, Temporal, Redis │
> └──────────────────┘          ├─────────────────────────────────────┤
>                               │  dev01: Worker (:8085)              │
>                               │         MLX Embeddings, MLX LLM     │
>                               └─────────────────────────────────────┘
> ```
>
> **The CLI has NO direct database access.** All commands go via gRPC to the Gateway.
> The Gateway handles search, review, relationships, etc. directly - these are NOT separate services.
> **Never bypass the Gateway. Never add direct DB calls to the CLI.**

---

## Deployment Topology

### Service-to-Host Mapping

| Service | Host | gRPC Port | HTTP Port | Status |
|---------|------|-----------|-----------|--------|
| **penf CLI** | laptop (MacBook Pro) | - | - | Installed |
| **Gateway** | dev02.brown.chat | 50051 | 8080 | Deployed |
| **Worker** | dev01.brown.chat | - | 8085 | Deployed |
| **MLX Embeddings** | dev01.brown.chat | - | 8081 | Deployed |
| **MLX LLM Server** | dev01.brown.chat | - | 8080 | Deployed |
| **mlx-lm-exporter** | dev01.brown.chat | - | 9101 | Deployed |
| **node_exporter** | dev01.brown.chat | - | 9100 | Deployed |
| **promtail** | dev01.brown.chat | - | 9080 | Deployed |
| **postgres_exporter** | dev02.brown.chat | - | 9187 | Deployed |
| **Agent Mail** | dev02.brown.chat | - | 8765 | Deployed |
| **AI Service** | dev01.brown.chat | 50055 | 8086 | Code exists |
| Gmail Connector | (not deployed) | 50056 | 8087 | Code exists |

**Note:** Search, Review, Content, and Relationship functionality is built into the Gateway (not separate services).

### Service Dependencies and Startup Order

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                            STARTUP DEPENDENCY ORDER                             │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                 │
│  Level 0 (Infrastructure):                                                      │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                             │
│  │ PostgreSQL  │  │   Redis     │  │  Temporal   │                             │
│  │   :5432     │  │   :6379     │  │   :7233     │                             │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘                             │
│         │                │                │                                     │
│  Level 1 (ML Services - dev01):          │                                      │
│  ┌──────────────────────────────────┐    │                                      │
│  │  MLX Embeddings    MLX LLM       │    │                                      │
│  │    :8081            :8080        │    │                                      │
│  └──────────────┬───────────────────┘    │                                      │
│                 │                         │                                     │
│  Level 2 (Core Services):                │                                      │
│  ┌──────────────▼───────────────┐  ┌─────▼─────────┐                           │
│  │  Worker (dev01)              │  │  Gateway      │                           │
│  │  → Needs: DB, Temporal, MLX  │  │  → Needs: DB  │                           │
│  │  Health: :8085               │  │  gRPC: :50051 │                           │
│  └──────────────────────────────┘  │  HTTP: :8080  │                           │
│                                    └───────────────┘                           │
│  Level 3 (CLI - any machine):                                                  │
│  ┌──────────────────────────────┐                                              │
│  │  penf CLI                    │                                              │
│  │  → Connects to Gateway       │                                              │
│  └──────────────────────────────┘                                              │
│                                                                                 │
└─────────────────────────────────────────────────────────────────────────────────┘
```

**Startup sequence:**
1. PostgreSQL, Redis, Temporal (Docker containers on dev02)
2. MLX Embeddings Sidecar (launchd on dev01)
3. MLX LLM Server (launchd on dev01)
4. Gateway (process on dev02)
5. Worker (process on dev01)
6. CLI ready to use

### Inter-Service Communication Paths

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│                           NETWORK COMMUNICATION MAP                              │
├──────────────────────────────────────────────────────────────────────────────────┤
│                                                                                  │
│   LAPTOP (MacBook Pro)                                                           │
│  ┌────────────────────────────┐                                                  │
│  │  penf CLI                  │                                                  │
│  │  Claude Code               │─────────────────────┐                            │
│  └────────────────────────────┘                     │                            │
│                                                     │ gRPC :50051                │
│   dev01.brown.chat                         dev02.brown.chat                      │
│  ┌────────────────────────────┐           ┌────────▼───────────────────┐        │
│  │                            │           │                            │        │
│  │  ┌──────────────────┐      │           │      ┌──────────────────┐  │        │
│  │  │  Worker          │──────┼───TCP─────┼─────►│  Gateway         │  │        │
│  │  │  :8085           │      │  :5432    │      │  :50051/:8080    │  │        │
│  │  │                  │──────┼───TCP─────┼─────►│  (search, review,│  │        │
│  │  │                  │      │  :7233    │      │   relationships) │  │        │
│  │  └────────┬─────────┘      │           │      └────────┬─────────┘  │        │
│  │           │                │           │               │            │        │
│  │           │ HTTP           │           │      ┌────────▼─────────┐  │        │
│  │           │ (localhost)    │           │      │  PostgreSQL      │  │        │
│  │  ┌────────▼─────────┐      │           │      │  :5432           │  │        │
│  │  │  MLX Embeddings  │      │           │      └──────────────────┘  │        │
│  │  │  :8081           │      │           │                            │        │
│  │  └──────────────────┘      │           │      ┌──────────────────┐  │        │
│  │                            │           │      │  Temporal        │  │        │
│  │  ┌──────────────────┐      │           │      │  :7233/:8088     │  │        │
│  │  │  MLX LLM Server  │      │           │      └──────────────────┘  │        │
│  │  │  :8080           │      │           │                            │        │
│  │  └──────────────────┘      │           │      ┌──────────────────┐  │        │
│  │                            │           │      │  Redis :6379     │  │        │
│  │  ┌──────────────────┐      │           │      └──────────────────┘  │        │
│  │  │  AI Service      │      │           │                            │        │
│  │  │  :50055          │      │           │      ┌──────────────────┐  │        │
│  │  └──────────────────┘      │           │      │  Langfuse :3000  │  │        │
│  │                            │           │      └──────────────────┘  │        │
│  └────────────────────────────┘           └────────────────────────────┘        │
│                                                                                  │
│  Legend: ───► Network call (with port)                                          │
│          CLI is on LAPTOP - NOT on dev01!                                        │
│                                                                                  │
└──────────────────────────────────────────────────────────────────────────────────┘
```

### Health Check Endpoints

All Go services expose standardized health endpoints:

| Endpoint | Purpose | Response |
|----------|---------|----------|
| `/health` | Full health status with dependency checks | JSON with service status |
| `/ready` | Kubernetes-style readiness probe | 200 OK or 503 |
| `/live` | Kubernetes-style liveness probe | 200 OK |
| `/metrics` | Prometheus metrics | OpenMetrics format |

**Service-specific health checks:**

| Service | URL | Checks |
|---------|-----|--------|
| Gateway | `http://dev02.brown.chat:8080/health` | database (critical), embeddings, llm, worker |
| Worker | `http://dev01.brown.chat:8085/health` | temporal_connection (critical), database (critical), embeddings (critical), llm (critical), worker_penfold-main, worker_penfold-ai, worker_penfold-email |
| MLX Embeddings | `http://localhost:8081/health` | Basic health (Python service) |
| MLX LLM Server | `http://localhost:8080/v1/models` | Model availability check |

---

## Hostnames

| Hostname | IP | Role |
|----------|-----|------|
| `dev01.brown.chat` | 10.0.10.144 | Development machine (Mac Mini M4), MLX inference, Worker |
| `dev02.brown.chat` | 10.0.10.251 | Data services (Intel N150), PostgreSQL, Redis, Temporal, Langfuse, Gateway |
| `home-01.brown.chat` | 10.0.10.253 | **Legacy - decommission pending** |

Use hostnames in all configs for portability. IPs may change.

## Deployment Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  Client Machine (any)                                                       │
│  ┌──────────────────────────┐                                              │
│  │  Claude Code + penf CLI  │──────────────┐                               │
│  │  → dev02.brown.chat      │              │                               │
│  └──────────────────────────┘              │                               │
└────────────────────────────────────────────┼────────────────────────────────┘
                                             │ gRPC :50051
┌────────────────────────────────────────────┼────────────────────────────────┐
│                        dev01.brown.chat (Mac Mini M4)                       │
│                                            │                                │
│  ┌──────────────────────────┐    ┌─────────┼────────────────┐              │
│  │  MLX Embeddings Sidecar  │    │  Penfold Worker          │              │
│  │  localhost:8081          │◄───│  Health: localhost:8085  │              │
│  │  mxbai-embed-large-v1    │    │                          │              │
│  └──────────────────────────┘    └────────────┬─────────────┘              │
│                                               │                             │
│  ┌──────────────────────────┐                 │                             │
│  │  MLX LLM Server          │                 │                             │
│  │  localhost:8080          │                 │                             │
│  │  Qwen2.5-7B-Instruct     │                 │                             │
│  └──────────────────────────┘                 │                             │
└───────────────────────────────────────────────┼─────────────────────────────┘
                                                │
                                    Network (1 Gbps)
                                                │
┌───────────────────────────────────────────────┼─────────────────────────────┐
│                      dev02.brown.chat (Intel N150)                          │
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
│                                                                             │
│  ┌──────────────────────────┐                                              │
│  │  Langfuse                │                                              │
│  │  Web: :3000              │                                              │
│  │  (AI Provenance)         │                                              │
│  └──────────────────────────┘                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Service Configuration

### dev01.brown.chat

| Service | Port | Co-located Access | Notes |
|---------|------|-------------------|-------|
| MLX Embeddings | 8081 | `localhost:8081` | Apple Silicon required, `/metrics` for Prometheus |
| MLX LLM Server | 8080 | `localhost:8080` | Qwen2.5-7B — SLM for pipeline Stages 1-2 (Triage, Extract) and mention resolution |
| mlx-lm-exporter | 9101 | `localhost:9101` | Prometheus metrics for MLX LLM Server |
| Worker | 8085 | - | Health endpoint + `/metrics` |
| node_exporter | 9100 | `localhost:9100` | System metrics |
| promtail | 9080 | `localhost:9080` | Log shipping to Loki |

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
mlx-community/Qwen2.5-7B-Instruct-4bit

# Managed by launchd (auto-restarts on reboot)
launchctl list | grep penfold.mlx-llm-server

# Manual start (if not using launchd)
.venv/bin/mlx_lm.server --model mlx-community/Qwen2.5-7B-Instruct-4bit --port 8080 --host 0.0.0.0
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
PENFOLD_DB_HOST=dev02.brown.chat
PENFOLD_DB_PORT=5432
PENFOLD_DB_USER=penfold
PENFOLD_DB_PASSWORD=<see secrets/.env.penfold>
PENFOLD_DB_NAME=penfold
PENFOLD_TEMPORAL_HOST=dev02.brown.chat:7233
AI_SERVICE_URL=http://localhost:8081  # MLX embeddings (local to dev01)
LLM_URL=http://localhost:8080         # MLX LLM server (local to dev01)
LLM_MODEL=mlx-community/Qwen2.5-7B-Instruct-4bit
```

**AI Service (dev01):**
```bash
# Binary location
/tmp/penfold-ai

# Build from source
cd services/ai && go build -o /tmp/penfold-ai .

# Start with default config (uses localhost MLX services)
AI_GRPC_PORT=50055 AI_HTTP_PORT=8086 nohup /tmp/penfold-ai > /tmp/penfold-ai.log 2>&1 &

# Health check
curl -s http://localhost:8086/health

# Environment variables (all have sensible defaults)
AI_GRPC_PORT=50055              # gRPC server port
AI_HTTP_PORT=8086               # HTTP health/metrics port
AI_MLX_EMBEDDINGS_URL=http://localhost:8081
AI_MLX_LLM_URL=http://localhost:8080
AI_DEFAULT_EMBEDDING_MODEL=mxbai-embed-large-v1
AI_DEFAULT_LLM_MODEL=mlx-community/Qwen2.5-7B-Instruct-4bit
```

### dev02.brown.chat

| Service | Port | Container | Notes |
|---------|------|-----------|-------|
| Gateway (gRPC) | 50051 | /tmp/penfold-gateway | Main API |
| Gateway (HTTP) | 8080 | /tmp/penfold-gateway | Health, metrics |
| Agent Mail | 8765 | - | Client-dev communication |
| PostgreSQL | 5432 | penfold-postgres | pgvector, TimescaleDB |
| Redis | 6379 | penfold-redis | No password |
| Temporal | 7233 | penfold-temporal | Workflow engine |
| Temporal UI | 8088 | penfold-temporal-ui | Web dashboard |
| Langfuse Web | 3000 | langfuse-web | AI Provenance UI |
| Langfuse PostgreSQL | 5433 | langfuse-postgres | Langfuse data |
| Langfuse Redis | 6380 | langfuse-redis | Langfuse cache |
| Langfuse ClickHouse | 8123 | langfuse-clickhouse | Trace storage |
| Langfuse MinIO | 9092 | langfuse-minio | Blob storage |
| Prometheus | 9090 | prometheus | Metrics collection |
| Loki | 3100 | loki | Log aggregation |
| Grafana | 3001 | grafana | Dashboards (internal 3000→3001) |

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

### PostgreSQL (from Gateway on dev02)
```bash
# Co-located, use localhost
PENFOLD_DB_HOST=localhost
```

### Redis
```bash
REDIS_HOST=dev02.brown.chat
REDIS_PORT=6379
# No password
```

### Temporal
```bash
PENFOLD_TEMPORAL_HOST=dev02.brown.chat:7233
TEMPORAL_NAMESPACE=default
```

### Langfuse (AI Provenance)
```bash
# Web UI
LANGFUSE_HOST=http://dev02.brown.chat:3000

# API keys for trace ingestion
LANGFUSE_PUBLIC_KEY=pk-lf-penfold
LANGFUSE_SECRET_KEY=sk-lf-penfold-secret

# OpenTelemetry endpoint (for OTEL SDK)
OTEL_EXPORTER_OTLP_ENDPOINT=http://dev02.brown.chat:3000/api/public/otel
```

Deployment: `~/langfuse/docker-compose.yml` on dev02
Credentials: See `~/github/otherjamesbrown/secrets/.env.langfuse`

### penf CLI
```yaml
# ~/.penf/config.yaml
server_address: dev02.brown.chat:50051
timeout: 30s
output_format: text
insecure: true
```

### Agent Mail (Client-Dev Communication)

Agent Mail enables two-way communication between Client Claude (laptop) and Dev Claude (server).

**Server:** `http://dev02.brown.chat:8765`

**Agents:**
| Agent Name | Role | Program |
|------------|------|---------|
| RedWolf | Client agent | Claude Code Client |
| RusticDesert | Dev agent | Claude Code Dev |

**Client Config (~/.penf/config.yaml):**
```yaml
agent_mail:
  server: "http://dev02.brown.chat:8765"
  project: "/Users/james/github/otherjamesbrown/penfold"
  client_agent: "RedWolf"
  dev_agent: "RusticDesert"
  bearer_token: "<see secrets/.env.penfold>"
```

**Canonical Project Key:** `/Users/james/github/otherjamesbrown/penfold`
(Use this regardless of which machine you're on - it's the shared identifier)

**Health Check:**
```bash
curl -s http://dev02.brown.chat:8765/health/liveness
```

**Web UI:** http://dev02.brown.chat:8765/mail

**Claude Code MCP Configuration:**

Agent Mail is accessed via MCP (Model Context Protocol). Add to Claude Code settings (`~/.claude/settings.json` or project `.mcp.json`):

```json
{
  "mcpServers": {
    "agent-mail": {
      "command": "uvx",
      "args": ["mcp-agentmail", "--host", "dev02.brown.chat", "--port", "8765"],
      "env": {
        "MCP_AGENT_MAIL_OUTPUT_FORMAT": "toon"
      }
    }
  }
}
```

This provides the `mcp__agent-mail__*` tools: `fetch_inbox`, `send_message`, `reply_message`, `register_agent`, etc.

## Starting Services

### Full Stack Startup

```bash
# 1. Verify Docker services on dev02
ssh dev02.brown.chat "docker ps --filter 'name=penfold'"

# 2. Start Gateway on dev02 (if not running)
ssh dev02.brown.chat "PENFOLD_SERVICE_NAME=gateway \
  PENFOLD_DB_HOST=localhost \
  PENFOLD_DB_PASSWORD=penfold \
  nohup /tmp/penfold-gateway > /tmp/gateway.log 2>&1 &"

# 3. Start Embeddings on dev01
cd penfold-go-pipeline/sidecar
.venv/bin/uvicorn app:app --host 0.0.0.0 --port 8081 &

# 4. Start LLM Server on dev01 (for mention resolution)
.venv/bin/mlx_lm.server --model mlx-community/Qwen2.5-7B-Instruct-4bit --port 8080 --host 0.0.0.0 &

# 5. Start Worker on dev01
PENFOLD_DB_HOST=dev02.brown.chat \
PENFOLD_TEMPORAL_HOST=dev02.brown.chat:7233 \
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
- `http://dev02.brown.chat:8080/health` - Full health status with all services
- `http://dev02.brown.chat:8080/ready` - Kubernetes readiness probe
- `http://dev02.brown.chat:8080/live` - Kubernetes liveness probe

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
curl -s http://dev02.brown.chat:8080/health | jq
```

## Verification Commands

```bash
# CLI → Gateway
penf status

# Full system health (includes ML services)
penf health gateway

# PostgreSQL (from dev01)
psql "host=dev02.brown.chat user=penfold password=penfold dbname=penfold" -c "SELECT 1"

# Docker containers (dev02)
ssh dev02.brown.chat "docker ps --filter 'name=penfold'"

# Temporal UI
open http://dev02.brown.chat:8088
```

## Design Rationale

**Why Worker on dev01?**
- Needs Apple Silicon for MLX embeddings
- Embedding calls stay local (large vectors, fast)
- DB/Temporal calls cross network (small payloads, acceptable latency)

**Why Gateway on dev02?**
- Co-located with PostgreSQL for fast queries
- CLI calls cross network (small gRPC payloads)

> **See also:** [ARCHITECTURE.md](ARCHITECTURE.md) for component responsibilities and data flow patterns.

## Credentials

See `~/github/otherjamesbrown/secrets/infrastructure.md` for passwords and API keys.

---

## Service Reference

### All Available Services

Complete inventory of Penfold Go services with their default configurations.

| Service | Binary/Path | Default gRPC | Default HTTP | Task Queues | Status |
|---------|-------------|--------------|--------------|-------------|--------|
| Gateway | `services/gateway` | 50051 | 8080 | - | Production |
| Worker | `services/worker` | - | 8085 | penfold-main, penfold-ai, penfold-email | Production |
| AI Service | `services/ai` | 50055 | 8086 | - | Production |
| Gmail Connector | `services/gmail` | 50056 | 8087 | - | Developed |

**Note:** Search, Review, Content, and Relationship are handled directly by the Gateway (built-in services, not separate binaries).

### External Dependencies (Docker on dev02)

| Service | Container Name | Port | Health Check |
|---------|---------------|------|--------------|
| PostgreSQL | penfold-postgres | 5432 | `pg_isready` |
| Redis | penfold-redis | 6379 | `redis-cli ping` |
| Temporal | penfold-temporal | 7233 | UI at :8088 |
| Temporal UI | penfold-temporal-ui | 8088 | HTTP |
| Langfuse Web | langfuse-web | 3000 | HTTP |
| Langfuse PostgreSQL | langfuse-postgres | 5433 | `pg_isready` |
| Langfuse Redis | langfuse-redis | 6380 | `redis-cli ping` |
| Langfuse ClickHouse | langfuse-clickhouse | 8123 | HTTP |
| Langfuse MinIO | langfuse-minio | 9092 | HTTP |
| Prometheus | prometheus | 9090 | HTTP `/targets`, `/metrics` |
| Loki | loki | 3100 | HTTP `/ready`, `/loki/api/v1/labels` |
| Grafana | grafana | 3001 | HTTP (UI port 3000 mapped to 3001) |

### ML Sidecars (Python on dev01)

| Service | Manager | Port | Model/Purpose | Health Check |
|---------|---------|------|---------------|--------------|
| MLX Embeddings | launchd | 8081 | mxbai-embed-large-v1 | `/health`, `/metrics` |
| MLX LLM Server | launchd | 8080 | Qwen2.5-7B-Instruct-4bit | `/v1/models` |
| mlx-lm-exporter | nohup | 9101 | Prometheus metrics for LLM | `/health`, `/metrics` |

### Observability Sidecars (dev01)

| Service | Manager | Port | Purpose | Health Check |
|---------|---------|------|---------|--------------|
| node_exporter | nohup | 9100 | System metrics | `/metrics` |
| promtail | nohup | 9080 | Log shipping to Loki | `/ready` |

### Environment Variables Reference

#### Gateway (`GATEWAY_*`)
```bash
GATEWAY_GRPC_PORT=50051          # gRPC server port
GATEWAY_HTTP_PORT=8080           # HTTP health/metrics port
GATEWAY_AUTH_ENABLED=false       # Enable auth middleware
GATEWAY_EMBEDDINGS_URL=...       # MLX embeddings URL for health checks
GATEWAY_LLM_URL=...              # MLX LLM URL for health checks
GATEWAY_WORKER_HEALTH_URL=...    # Worker health endpoint URL
```

#### Worker (`WORKER_*`, `TEMPORAL_*`)
```bash
WORKER_HTTP_PORT=8085            # HTTP health/metrics port
WORKER_TASK_QUEUES=penfold-main,penfold-ai,penfold-email
TEMPORAL_HOST_PORT=dev02.brown.chat:7233
TEMPORAL_NAMESPACE=default
AI_SERVICE_URL=http://localhost:8081  # MLX embeddings
LLM_URL=http://localhost:8080         # MLX LLM server
LLM_MODEL=mlx-community/Qwen2.5-7B-Instruct-4bit
WORKER_MAX_CONCURRENT_ACTIVITIES=1  # Limit concurrent activities per queue worker
WORKER_MAX_CONCURRENT_WORKFLOWS=10  # Limit concurrent workflow executions
```

#### Database (`PENFOLD_DB_*`)
```bash
PENFOLD_DB_HOST=dev02.brown.chat
PENFOLD_DB_PORT=5432
PENFOLD_DB_USER=penfold
PENFOLD_DB_PASSWORD=<see secrets>
PENFOLD_DB_NAME=penfold
```

---

## Rate Limits

Rate limiting protects the LLM server from OOM crashes when processing multiple sources concurrently.

### Temporal Worker Concurrency

The Worker creates **separate workers for each task queue**, each with its own concurrency limit:

| Setting | Default | Current | Scope |
|---------|---------|---------|-------|
| `WORKER_MAX_CONCURRENT_ACTIVITIES` | 10 | **1** | Per queue worker |
| `WORKER_MAX_CONCURRENT_WORKFLOWS` | 10 | 10 | Per queue worker |

**Task Queues:**
- `penfold-main` - General workflows, content ingestion
- `penfold-ai` - AI-intensive operations
- `penfold-email` - Email processing (includes AI activities)

**Effective Total**: With 3 queues × 1 concurrent activity = **3 max concurrent activities globally**.

**Why concurrency=1?** The Qwen2.5-7B-Instruct-4bit model requires ~4-5GB VRAM at 4-bit quantization. While smaller than the 32B model, we keep concurrency=1 to ensure consistent response times and avoid memory pressure during concurrent requests.

### Activity-Level Limits

Activities use different timeout/retry profiles defined in `pkg/temporal/options.go`:

| Activity Type | Timeout | Heartbeat | Retries | Use Case |
|---------------|---------|-----------|---------|----------|
| `FastActivityOptions()` | 30s | - | 3 | DB queries, status updates |
| `EmbeddingActivityOptions()` | 30s | 10s | 3 | MLX embeddings, SLM calls |
| `LLMActivityOptions()` | 2min | 15s | 2 | LLM deep analysis |
| `BatchActivityOptions()` | 5min | 30s | 2 | Batch processing |

### LLM-Specific Activities

Activities that call the MLX LLM Server (`:8080`):

| Activity | Queue | Timeout | Description |
|----------|-------|---------|-------------|
| `Triage` | penfold-main | 30s | Content classification (via AI Coordinator) |
| `ExtractEntities` | penfold-main | 30s | Entity extraction (via AI Coordinator) |
| `DeepAnalyze` | penfold-main | 2min | Full LLM analysis |
| `ExtractMentions` | penfold-main | 30s | Direct vLLM-MLX call |
| `GenerateSummary` | penfold-ai | 30s | Summary generation |

**Note:** Activities on `penfold-main` all compete for the single concurrent slot, ensuring sequential LLM access.

### Configuration Location

| Setting | File | Host |
|---------|------|------|
| Worker concurrency | `~/Library/LaunchAgents/com.penfold.worker.plist` | dev01 |
| Activity timeouts | `pkg/temporal/options.go` | Codebase |
| Activity registration | `services/worker/activities/register.go` | Codebase |

### Changing Concurrency

```bash
# On dev01 - update plist
ssh dev01 "plutil -replace EnvironmentVariables.WORKER_MAX_CONCURRENT_ACTIVITIES -string '2' ~/Library/LaunchAgents/com.penfold.worker.plist"

# Restart worker (kill and launchd restarts it)
ssh dev01 "pkill -f penfold-worker"

# Or use nohup if launchctl isn't working over SSH
ssh dev01 "nohup /opt/penfold/bin/penfold-worker > ~/penfold-worker.log 2>&1 &"
```

### Monitoring

```bash
# Check active workflows
penf pipeline list --status=running

# Check Temporal UI for activity queue depth
open http://dev02.brown.chat:8088

# Watch worker logs for concurrent activity
ssh dev01 "tail -f ~/penfold-worker.log | grep -E 'Starting|Completed|activity'"
```

### Observability Stack

Metrics and logs are collected on dev02:

| Service | URL | Purpose |
|---------|-----|---------|
| Prometheus | http://dev02.brown.chat:9090 | Metrics collection and queries |
| Loki | http://dev02.brown.chat:3100 | Log aggregation |
| Grafana | http://dev02.brown.chat:3001 | Dashboards and visualization |

#### Prometheus Targets (12 total)

**dev01 (Apple Silicon):**

| Target | Port | Source | Metrics |
|--------|------|--------|---------|
| penfold-worker | 8085 | Native | Request counts, latency, Temporal activity metrics |
| mlx-embeddings | 8081 | Native | Request counts, latency (prometheus-fastapi-instrumentator) |
| mlx-llm | 9101 | [mlx-lm-exporter](https://github.com/otherjamesbrown/mlx-lm-exporter) | LLM up/down, request counts, token usage, latency |
| node-dev01 | 9100 | node_exporter | CPU, memory, disk, network |

**dev02 (Intel):**

| Target | Port | Source | Metrics |
|--------|------|--------|---------|
| penfold-gateway | 8080 | Native | gRPC request counts, latency |
| penfold-ai-coordinator | 8090 | Native | AI service metrics |
| postgresql | 9187 | postgres_exporter | DB connections, queries, replication |
| temporal | 8233 | Native | Workflow counts, task queue depth, latency |
| prometheus | 9090 | Self | Scrape stats |
| loki | 3100 | Native | Ingestion rate, storage |
| promtail | 9080 | Native | Log shipping stats |
| node-dev02 | 9100 | node_exporter | CPU, memory, disk, network |

#### Loki Log Sources

**dev01 (via promtail on port 9080):**

| Job | Log File | Description |
|-----|----------|-------------|
| penfold-worker | `/Users/james/penfold-worker.log` | Worker activity logs |
| mlx-llm | `/tmp/mlx-llm-server.log` | vLLM-MLX server logs |
| mlx-embeddings | `/tmp/mlx-embeddings.log` | Embedding service logs |
| mlx-lm-exporter | `/tmp/mlx-lm-exporter.log` | Metrics exporter logs |
| node-exporter | `/tmp/node_exporter.log` | Node exporter logs |

**dev02 (via promtail in Docker):**

| Job | Source | Description |
|-----|--------|-------------|
| docker | `/var/lib/docker/containers/*/*.log` | All container logs |
| systemd-journal | journald | System service logs |

#### Exporter Processes (dev01)

These run via nohup (not launchd, due to SSH domain issues):

```bash
# node_exporter
nohup /opt/homebrew/opt/node_exporter/bin/node_exporter > /tmp/node_exporter.log 2>&1 &

# mlx-lm-exporter (proxies to mlx-lm on 8080)
cd ~/github/otherjamesbrown/mlx-lm-exporter
nohup .venv/bin/python mlx_lm_exporter.py --mlx-server http://localhost:8080 --port 9101 > /tmp/mlx-lm-exporter.log 2>&1 &

# promtail
nohup /opt/homebrew/opt/promtail/bin/promtail -config.file=/tmp/promtail-config.yaml > /tmp/promtail.log 2>&1 &
```

#### Quick Access

```bash
# Prometheus - check targets
open http://dev02.brown.chat:9090/targets

# Grafana - dashboards
open http://dev02.brown.chat:3001

# Query Loki logs
curl -s "http://dev02.brown.chat:3100/loki/api/v1/query_range" \
  --data-urlencode 'query={job="penfold-worker"}' \
  --data-urlencode 'limit=100'

# Check all targets health
curl -s http://dev02.brown.chat:9090/api/v1/targets | \
  jq '.data.activeTargets | map({job: .labels.job, health: .health})'
```

#### Example Grafana Queries

```promql
# MLX LLM request rate
rate(mlx_lm_request_total[5m])

# MLX LLM token throughput
rate(mlx_lm_tokens_total[5m])

# Worker activity duration
histogram_quantile(0.95, rate(temporal_activity_execution_latency_bucket[5m]))

# Embedding request latency
histogram_quantile(0.95, rate(http_request_duration_seconds_bucket{job="mlx-embeddings"}[5m]))
```

### Troubleshooting OOM

If the LLM server crashes with OOM:

1. **Check vLLM-MLX logs**: `ssh dev01 "tail -50 /tmp/mlx-llm-server.log"`
2. **Reduce model size**: Switch from 32B to 7B model
3. **Reduce concurrency**: Set `WORKER_MAX_CONCURRENT_ACTIVITIES=1`
4. **Check GPU memory**: `ssh dev01 "ioreg -l | grep CurrentPowerState"` (Mac)

**Recovery steps:**
```bash
# 1. Restart vLLM-MLX with smaller model
ssh dev01 "pkill -f mlx_lm.server"
ssh dev01 "cd ~/github/otherjamesbrown/penfold/penfold-go-pipeline/sidecar && nohup .venv/bin/mlx_lm.server --model mlx-community/Qwen2.5-7B-Instruct-4bit --port 8080 --host 0.0.0.0 > /tmp/mlx-llm-server.log 2>&1 &"

# 2. Restart worker
ssh dev01 "pkill -f penfold-worker"
# Worker auto-restarts via launchd, or start manually with nohup
```


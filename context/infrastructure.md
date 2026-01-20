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
| Worker | 8085 | - | Health endpoint only |

**Embeddings Sidecar:**
```bash
# Location
penfold-go-pipeline/sidecar

# Model
mixedbread-ai/mxbai-embed-large-v1 (1024 dimensions)

# Start
.venv/bin/uvicorn app:app --host 0.0.0.0 --port 8081
```

**Worker Environment:**
```bash
PENFOLD_DB_HOST=home-01.brown.chat
PENFOLD_DB_PORT=5432
PENFOLD_DB_USER=penfold
PENFOLD_DB_PASSWORD=penfold
PENFOLD_DB_NAME=penfold
PENFOLD_TEMPORAL_HOST=home-01.brown.chat:7233
AI_SERVICE_URL=http://localhost:8081  # Local to dev01
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

**Gateway Environment:**
```bash
PENFOLD_SERVICE_NAME=gateway
PENFOLD_DB_HOST=localhost  # Co-located with PostgreSQL
PENFOLD_DB_PORT=5432
PENFOLD_DB_USER=penfold
PENFOLD_DB_PASSWORD=penfold
PENFOLD_DB_NAME=penfold
```

## Connection Strings

### PostgreSQL (from dev01)
```bash
# Environment variables
PENFOLD_DB_HOST=home-01.brown.chat
PENFOLD_DB_PORT=5432
PENFOLD_DB_USER=penfold
PENFOLD_DB_PASSWORD=penfold
PENFOLD_DB_NAME=penfold

# DSN (keyword-value, handles special chars)
host=home-01.brown.chat port=5432 user=penfold password=penfold dbname=penfold sslmode=disable

# psql
psql "host=home-01.brown.chat port=5432 user=penfold password=penfold dbname=penfold"
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

# 4. Start Worker on dev01
PENFOLD_DB_HOST=home-01.brown.chat \
PENFOLD_TEMPORAL_HOST=home-01.brown.chat:7233 \
AI_SERVICE_URL=http://localhost:8081 \
./bin/penfold-worker &

# 5. Verify CLI connection
penf status
```

## Verification Commands

```bash
# CLI → Gateway
penf status

# Embeddings health
curl -s http://localhost:8081/health

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

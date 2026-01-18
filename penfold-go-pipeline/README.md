# Penfold Go AI Processing Pipeline

A Go service that subscribes to Redis events and processes content through AI services (MLX embeddings + vLLM-MLX), storing results in PostgreSQL.

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│  Go AI Processing Pipeline                                       │
│                                                                  │
│  Redis ──► Event Router ──► Handlers ──► Processing Orchestrator │
│                                               │                  │
│                         ┌─────────────────────┼──────────────┐   │
│                         ▼                     ▼              ▼   │
│                   MLX Sidecar           vLLM-MLX      PostgreSQL │
│                   (embeddings)          (LLM tasks)   (storage)  │
│                   :8001                 :8000                    │
└─────────────────────────────────────────────────────────────────┘
```

## Prerequisites

- Go 1.22+
- PostgreSQL 16+ with pgvector extension
- Redis 6+
- vLLM-MLX running on port 8000
- MLX Embeddings sidecar running on port 8001

## Building

```bash
# Build the binary
make build

# Or directly with Go
go build -o bin/penfold-pipeline ./cmd/pipeline
```

## Configuration

Configuration is via environment variables:

### Database
| Variable | Default | Description |
|----------|---------|-------------|
| `DB_HOST` | 10.0.10.253 | PostgreSQL host |
| `DB_PORT` | 5432 | PostgreSQL port |
| `DB_NAME` | penfold_dev | Database name |
| `DB_USER` | penfold | Database user |
| `DB_PASSWORD` | | Database password |
| `DB_MAX_OPEN_CONNS` | 25 | Max open connections |
| `DB_MAX_IDLE_CONNS` | 5 | Max idle connections |

### Redis
| Variable | Default | Description |
|----------|---------|-------------|
| `REDIS_HOST` | 10.0.10.253 | Redis host |
| `REDIS_PORT` | 6379 | Redis port |
| `REDIS_PASSWORD` | | Redis password |
| `REDIS_DB` | 0 | Redis database |
| `REDIS_POOL_SIZE` | 10 | Connection pool size |

### AI Services
| Variable | Default | Description |
|----------|---------|-------------|
| `EMBEDDINGS_URL` | http://localhost:8001 | MLX sidecar URL |
| `EMBEDDINGS_MODEL` | mxbai-embed-large-v1 | Embedding model |
| `EMBEDDINGS_TIMEOUT` | 30s | Request timeout |
| `LLM_URL` | http://localhost:8000 | vLLM-MLX URL |
| `LLM_MODEL` | Qwen2.5-32B-4bit | LLM model |
| `LLM_TIMEOUT` | 120s | Request timeout |

### Processing
| Variable | Default | Description |
|----------|---------|-------------|
| `WORKER_COUNT` | 4 | Worker pool size |
| `MAX_RETRIES` | 3 | Max retry attempts |
| `QUEUE_SIZE` | 1000 | Event queue size |

### Health Server
| Variable | Default | Description |
|----------|---------|-------------|
| `HEALTH_PORT` | 8080 | Health check port |

## Running

```bash
# Start the MLX embeddings sidecar first
cd sidecar && uvicorn app:app --host 0.0.0.0 --port 8001

# Then start the Go pipeline
make run

# Or with environment variables
DB_PASSWORD=xxx ./bin/penfold-pipeline
```

## Health Endpoints

- `GET /health` - Overall health status with component details
- `GET /ready` - Readiness check (all dependencies healthy)
- `GET /live` - Liveness probe

## Event Processing

The pipeline subscribes to these Redis channels:

- `events.manual_email.ingested` - Manual email ingestion events
- `events.content.ingested` - General content ingestion events

### Processing Flow

1. Receive event from Redis
2. Fetch source content from PostgreSQL
3. Generate embedding via MLX sidecar (768-dim vector)
4. Generate summary via vLLM-MLX
5. Extract assertions via vLLM-MLX
6. Store results in PostgreSQL
7. Update source processing status

## MLX Embeddings Sidecar

The sidecar is a minimal Python FastAPI service:

```bash
cd sidecar
pip install -r requirements.txt
uvicorn app:app --host 0.0.0.0 --port 8001
```

See `sidecar/` directory for the complete service.

## Testing

```bash
# Run all tests
make test

# Run with coverage
make test-cover

# Run integration tests
make integration-test
```

## Verification

After running the pipeline, verify processing with:

```sql
-- Check embeddings
SELECT COUNT(*) FROM embeddings WHERE embedding_model = 'mxbai-embed-large-v1';

-- Check processing results
SELECT * FROM processing_results ORDER BY created_at DESC LIMIT 5;

-- Check source processing status
SELECT processing_status, COUNT(*) FROM sources GROUP BY processing_status;
```

## Project Structure

```
penfold-go-pipeline/
├── cmd/pipeline/main.go           # Entry point
├── internal/
│   ├── config/config.go           # Environment-based config
│   ├── events/
│   │   ├── schemas.go             # Event types (matches Python)
│   │   ├── subscriber.go          # Redis subscription
│   │   └── router.go              # Event routing
│   ├── handlers/
│   │   ├── handler.go             # Handler interface
│   │   └── email.go               # Email processing handler
│   ├── clients/
│   │   ├── embeddings.go          # MLX sidecar client
│   │   └── llm.go                 # vLLM-MLX client
│   ├── storage/
│   │   ├── postgres.go            # DB connection + pgvector
│   │   ├── embeddings.go          # Embedding repository
│   │   ├── results.go             # Processing results repo
│   │   └── sources.go             # Source content fetcher
│   ├── pipeline/
│   │   ├── processor.go           # Orchestrator
│   │   └── worker.go              # Worker pool
│   └── health/health.go           # Health endpoints
├── sidecar/
│   ├── app.py                     # MLX embeddings service
│   └── requirements.txt           # Python dependencies
├── go.mod
├── Makefile
└── README.md
```

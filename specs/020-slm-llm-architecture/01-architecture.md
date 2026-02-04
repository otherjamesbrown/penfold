# Penfold: System Architecture

## Service Topology

Penfold is a multi-service Go application with 4 main services communicating via gRPC and Temporal workflows.

```
┌────────────────────────────────────────────────────────────────────────┐
│                          USER INTERACTION                              │
│                                                                        │
│   User ──(natural language)──▶ Claude Code ──(CLI)──▶ penf            │
│                                                                        │
└──────────────────────────────────────┬─────────────────────────────────┘
                                       │ gRPC
                                       ▼
┌──────────────────────────────────────────────────────────────────────┐
│                       GATEWAY SERVICE (dev02)                         │
│                                                                       │
│  ┌─────────┐ ┌──────────┐ ┌─────────┐ ┌──────────┐ ┌────────────┐  │
│  │ Search  │ │ Ingest   │ │ Entity  │ │ Product  │ │ AI Coord.  │  │
│  │ Service │ │ Service  │ │ Service │ │ Service  │ │ (proxy)    │  │
│  └────┬────┘ └────┬─────┘ └────┬────┘ └────┬─────┘ └─────┬──────┘  │
│       │           │            │            │             │          │
│       ▼           ▼            ▼            ▼             │          │
│  ┌─────────────────────────────────────────────┐          │          │
│  │           PostgreSQL 16+ (pgx)              │          │          │
│  │           with pgvector extension           │          │          │
│  └─────────────────────────────────────────────┘          │          │
│       ▲                    │                              │          │
└───────┼────────────────────┼──────────────────────────────┼──────────┘
        │                    │ Temporal                      │ gRPC
        │                    ▼                               ▼
┌───────┼──────────────────────────┐  ┌────────────────────────────────┐
│       │  WORKER SERVICE (dev01)  │  │   AI SERVICE (dev01)           │
│       │                          │  │                                │
│  ┌────┴─────┐  ┌──────────────┐  │  │  ┌────────┐  ┌─────────────┐ │
│  │ Content  │  │ Enrichment   │  │  │  │ Router │  │ Model       │ │
│  │ Workflow │  │ Activities   │  │  │  │        │  │ Registry    │ │
│  └──────────┘  └──────────────┘  │  │  └───┬────┘  └─────────────┘ │
│                                   │  │      │                       │
│  ┌──────────┐  ┌──────────────┐  │  │      ▼                       │
│  │ Email    │  │ Mention      │  │  │  ┌────────┐ ┌────────┐       │
│  │ Workflow │  │ Resolution   │  │  │  │  MLX   │ │ Gemini │       │
│  └──────────┘  └──────────────┘  │  │  │(local) │ │(remote)│       │
│                                   │  │  └────────┘ └────────┘       │
└───────────────────────────────────┘  └──────────────────────────────┘
```

## Services

### Gateway Service
- **Location**: `services/gateway/`
- **Runs on**: dev02 (Linux, AMD64)
- **Port**: gRPC 50051, HTTP 8080
- **Purpose**: Central API entry point. Routes requests to internal services. Hosts 20+ registered gRPC services.
- **Key responsibilities**: Search, ingestion orchestration, entity management, product management, AI proxy, health aggregation, authentication (JWT + API key)

### Worker Service
- **Location**: `services/worker/`
- **Runs on**: dev01 (Mac Mini, Apple Silicon)
- **Purpose**: Temporal-based async processing engine. Executes workflows and activities for content ingestion, enrichment, and AI processing.
- **Key workflows**: Content ingestion (8-stage pipeline), email processing, Gmail sync, mention resolution, relationship discovery
- **Pattern**: Saga pattern with compensation stack for rollback on failure

### AI Service
- **Location**: `services/ai/`
- **Runs on**: dev01 (Mac Mini, Apple Silicon — co-located with MLX models)
- **Port**: gRPC 50055
- **Purpose**: Coordinates all AI/ML operations. Manages embeddings and LLM access across multiple backends.
- **Key components**: Backend abstraction layer, model router with circuit breakers, model registry, Langfuse integration for tracing

### Gmail Service
- **Location**: `services/gmail/`
- **Purpose**: Email connector for Gmail integration (OAuth2, push notifications, message sync)

## Communication Patterns

| From | To | Protocol | Purpose |
|------|----|----------|---------|
| CLI (`penf`) | Gateway | gRPC + mTLS | All user-facing operations |
| Gateway | Worker | Temporal | Async workflow dispatch |
| Worker | AI Service | gRPC | Embedding, summarization, extraction |
| Worker | Database | pgx (PostgreSQL) | Content storage, entity management |
| Gateway | Database | pgx (PostgreSQL) | Queries, search, entity CRUD |
| Gateway | AI Service | gRPC | AI model management, query proxy |
| AI Service | MLX/Gemini/OpenAI | HTTP REST | Backend AI calls |

## gRPC API Surface

### Gateway API (`gateway/v1`)
- `ProcessEmail` — Ingest and process emails through AI pipeline
- `Search` — Unified search (semantic, keyword, hybrid modes)
- `GetDailyReview` — Aggregated daily digest
- `HealthCheck` — Health checks with dependency monitoring

### AI Coordinator API (`ai/v1`)
- `GenerateEmbedding` — Vector embeddings (1024 dimensions)
- `GenerateSummary` — Content summarization (brief, detailed, bullet points, technical)
- `ExtractAssertions` — Subject-predicate-object triples with confidence
- `ClassifyContent` — Multi-label content classification
- `Query` — RAG-style Q&A over knowledge base
- `SummarizeByID` / `AnalyzeByID` — Operations by content ID
- `ListModels` / `RegisterModel` / `UpdateModel` / `DeleteModel` — Model management
- `GetRoutingRules` / `UpdateRoutingRule` — Task-based routing configuration

### Workflow API (`workflow/v1`)
- `ListWorkflows` / `GetWorkflowStatus` / `CancelWorkflow` / `TerminateWorkflow`
- Activity I/O types: FetchSource, GenerateEmbedding, GenerateSummary, ExtractAssertions, UpdateSourceStatus, StoreResults

### Context API (proposed — see `guide.md` and `00-overview.md`)
- `ContextMorning` — Session bootstrap: returns watch list, recent changes, active projects, trusted people, last session summary. Claude's first call every session.
- `ContextSessionEnd` — Persist session summary for next session bootstrap.
- `AssertionBriefing` — Full golden thread for an assertion: origin, lifecycle, people, escalation, linked content. On-demand depth when the spotlight moves.
- `WatchListManage` — Add/remove/annotate items on the human's watch list.
- `PeripheralChanges` — Query for pattern changes on non-spotlight items (seniority shifts, frequency changes, stale items).

### Other Services (39 proto files total)
Glossary, Questions, Review, Mentions, Entities, Products, Projects, Teams, Tenants, Relationships, Logs, Search, Content, Ingest, Pipeline, Intelligence, Email (Gmail), Orchestration, Processing

## Deployment

| Component | Machine | OS | Architecture | Deploy Method |
|-----------|---------|------|--------------|---------------|
| Gateway | dev02 | Ubuntu Linux | AMD64 | systemd service |
| Worker | dev01 | macOS | Apple M4, 32GB | launchd plist |
| AI Service | dev01 | macOS | Apple Silicon | launchd plist |
| MLX Server | dev01 | macOS | Apple Silicon | Runs locally |
| PostgreSQL | dev02 | Ubuntu Linux | AMD64 | System service |
| Temporal | dev02 | Ubuntu Linux | AMD64 | Docker |
| Langfuse | dev02 | Ubuntu Linux | AMD64 | Docker |
| CLI (penf) | User machines | Any | Any | GitHub Release |

### Deployment Scripts
- `scripts/deploy-gateway.sh` — Cross-compile, deploy, restart, verify, rollback on failure
- `scripts/deploy-worker.sh` — Worker deployment
- `scripts/deploy-ai-coordinator.sh` — AI service deployment
- `scripts/verify-deployment.sh` — Post-deploy health verification

## Technology Stack

| Layer | Technology |
|-------|-----------|
| Language | Go 1.24 |
| CLI Framework | Cobra |
| RPC | gRPC with Protocol Buffers |
| Database | PostgreSQL 16+ with pgvector |
| Database Driver | pgx |
| Workflow Engine | Temporal |
| Local AI | MLX (Apple Silicon optimized) |
| Remote AI | Gemini, OpenAI (Azure compatible) |
| Observability | Langfuse (LLM tracing), Prometheus, Grafana, Jaeger |
| Auth | JWT + API keys + mTLS |

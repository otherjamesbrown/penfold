# Penfold Go Migration Overview

## Executive Summary

Migration of Penfold from a monolithic Python application to a distributed Go microservices architecture, designed for:
- **Thin CLI** on laptop → connects to remote services
- **API Gateway** on dev01 → orchestrates microservices
- **Go Microservices** → handle distinct domains with clear boundaries

## Current State

| Metric | Value |
|--------|-------|
| Python LOC | ~68,000 |
| Core Modules | 10+ |
| Database Tables | 25+ |
| Event Types | 15+ |
| External Integrations | Gmail, Gemini, Redis, PostgreSQL |

## Target Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                           LAPTOP                                     │
│  ┌─────────────┐                                                    │
│  │  penf CLI   │  (thin client, gRPC/REST to dev01)                │
│  └──────┬──────┘                                                    │
└─────────┼───────────────────────────────────────────────────────────┘
          │ gRPC / REST
          ▼
┌─────────────────────────────────────────────────────────────────────┐
│                           DEV01 (Mac Mini M4)                        │
│                                                                      │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │                     API Gateway (:8080)                      │   │
│  │              (auth, routing, rate limiting)                  │   │
│  └──────┬──────────────┬──────────────┬──────────────┬─────────┘   │
│         │              │              │              │              │
│         ▼              ▼              ▼              ▼              │
│  ┌───────────┐  ┌───────────┐  ┌───────────┐  ┌───────────┐       │
│  │  Search   │  │  Gmail    │  │ Content   │  │ Review    │       │
│  │  Service  │  │ Connector │  │ Processor │  │ Service   │       │
│  │  (:8081)  │  │  (:8082)  │  │  (:8083)  │  │  (:8084)  │       │
│  └─────┬─────┘  └─────┬─────┘  └─────┬─────┘  └─────┬─────┘       │
│        │              │              │              │               │
│        ▼              ▼              ▼              ▼               │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │                     Event Router (:8090)                     │   │
│  │              (Redis pub-sub orchestration)                   │   │
│  └──────────────────────────┬──────────────────────────────────┘   │
│                             │                                       │
│         ┌───────────────────┼───────────────────┐                  │
│         ▼                   ▼                   ▼                  │
│  ┌───────────┐       ┌───────────┐       ┌───────────┐            │
│  │ Embedding │       │    AI     │       │Relationship│            │
│  │ Pipeline  │       │Coordinator│       │ Discovery  │            │
│  │  (:8001)  │       │  (:8085)  │       │  (:8086)   │            │
│  └─────┬─────┘       └─────┬─────┘       └─────┬─────┘            │
│        │                   │                   │                   │
│        └───────────────────┼───────────────────┘                   │
│                            ▼                                        │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │                   Shared Infrastructure                      │   │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐    │   │
│  │  │PostgreSQL│  │  Redis   │  │ vLLM-MLX │  │   MLX    │    │   │
│  │  │ (home-01)│  │(home-01) │  │  (:8000) │  │ Sidecar  │    │   │
│  │  └──────────┘  └──────────┘  └──────────┘  └──────────┘    │   │
│  └─────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────┘
```

## Migration Principles

1. **Incremental Migration**: One service at a time, Python and Go coexist
2. **Event Compatibility**: Same Redis channels, same event schemas
3. **Database Shared**: Single PostgreSQL, services own their tables
4. **API First**: Define gRPC/protobuf contracts before implementation
5. **Observability Built-in**: Structured logging, metrics, tracing from day 1

## Service Inventory

| Service | Port | Priority | Status | Description |
|---------|------|----------|--------|-------------|
| API Gateway | 8080 | P0 | Planned | Entry point, auth, routing |
| Embedding Pipeline | 8001 | P0 | **In Progress** | Vector embeddings via MLX |
| Event Router | 8090 | P0 | Planned | Redis subscription, job orchestration |
| Search Service | 8081 | P1 | Planned | Hybrid search, ranking |
| Gmail Connector | 8082 | P1 | Planned | OAuth, sync, attachments |
| Content Processor | 8083 | P2 | Planned | AI extraction, classification |
| Review Service | 8084 | P2 | Planned | Review workflows, feedback |
| AI Coordinator | 8085 | P2 | Planned | Model selection, escalation |
| Relationship Discovery | 8086 | P3 | Planned | Network analysis, confidence |

## Technology Stack

| Layer | Technology |
|-------|-----------|
| Language | Go 1.22+ |
| RPC | gRPC + Protocol Buffers |
| REST | Chi or Gin (Gateway only) |
| Database | pgx (PostgreSQL driver) |
| Vector | pgvector via pgx |
| Events | go-redis |
| Config | Viper |
| Logging | slog (structured) |
| Metrics | Prometheus |
| Tracing | OpenTelemetry |

## Document Index

| Document | Description |
|----------|-------------|
| [01-thin-cli.md](./01-thin-cli.md) | CLI design and commands |
| [02-api-gateway.md](./02-api-gateway.md) | Gateway service spec |
| [03-embedding-pipeline.md](./03-embedding-pipeline.md) | Embedding service (existing) |
| [04-event-router.md](./04-event-router.md) | Event orchestration |
| [05-search-service.md](./05-search-service.md) | Search and ranking |
| [06-gmail-connector.md](./06-gmail-connector.md) | Gmail integration |
| [07-content-processor.md](./07-content-processor.md) | AI content processing |
| [08-review-service.md](./08-review-service.md) | Review workflows |
| [09-ai-coordinator.md](./09-ai-coordinator.md) | Model orchestration |
| [10-relationship-discovery.md](./10-relationship-discovery.md) | Relationship network |
| [11-shared-libraries.md](./11-shared-libraries.md) | Common Go packages |
| [12-migration-phases.md](./12-migration-phases.md) | Migration roadmap |

## Migration Phases

### Phase 0: Foundation (Current)
- [x] Embedding Pipeline service running
- [ ] Define protobuf contracts
- [ ] Shared Go libraries (db, events, logging)
- [ ] API Gateway skeleton

### Phase 1: Core Infrastructure
- [ ] API Gateway with auth
- [ ] Event Router
- [ ] Thin CLI (basic commands)
- [ ] Search Service (read path)

### Phase 2: Content Ingestion
- [ ] Gmail Connector
- [ ] Content Processor
- [ ] Full CLI commands

### Phase 3: Intelligence Layer
- [ ] AI Coordinator
- [ ] Relationship Discovery
- [ ] Review Service

### Phase 4: Decommission Python
- [ ] Migrate remaining endpoints
- [ ] Data migration tooling
- [ ] Python service shutdown

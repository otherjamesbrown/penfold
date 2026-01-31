# Penfold Architecture

**Last Updated**: 2026-01-31

> **Deployment Details:** See [infrastructure.md](infrastructure.md) for hostnames, ports, connection strings, and startup commands.

## Overview

Penfold is an AI-powered personal information system built with Go. It aggregates and correlates information from communication channels into a queryable institutional memory.

## CRITICAL: Where Things Run

> **The CLI runs on James's LAPTOP (MacBook Pro), NOT on any server.**
>
> - **LAPTOP**: penf CLI, Claude Code
> - **dev02**: Gateway, PostgreSQL, Temporal, Redis
> - **dev01**: Worker, MLX Embeddings, MLX LLM
>
> **The CLI has NO direct database access.** All requests go over gRPC to the Gateway on dev02.
> Never bypass the Gateway. Never add direct DB calls to the CLI.

## System Architecture

```
  LAPTOP (MacBook Pro)
┌─────────────────────────────────────────────────────────────────┐
│                         CLI (penf)                              │
│                        cmd/penf/cmd                             │
└─────────────────────────┬───────────────────────────────────────┘
                          │ gRPC (to dev02.brown.chat:50051)
                          │
  dev02.brown.chat        ▼
┌─────────────────────────────────────────────────────────────────┐
│                     API Gateway                                  │
│                   services/gateway                               │
│                                                                  │
│  • Authentication & Authorization    • searchservice (built-in) │
│  • Request routing                   • reviewservice (built-in) │
│  • Rate limiting                     • relationshipservice      │
│                                                                  │
│  Connects to: PostgreSQL (localhost), Temporal (localhost)      │
└─────────────────────────┬───────────────────────────────────────┘
                          │ Temporal task dispatch
  dev01.brown.chat        ▼
┌─────────────────────────────────────────────────────────────────┐
│                 Temporal Worker                                  │
│                 services/worker                                  │
│                                                                  │
│  • Durable workflow execution        • Connects to MLX locally  │
│  • Content processing activities     • Connects to DB remotely  │
│  • Embedding generation              • Connects to Temporal     │
└─────────────────────────────────────────────────────────────────┘

  dev02.brown.chat (data layer)
┌─────────────────────────────────────────────────────────────────┐
│              PostgreSQL + pgvector                               │
│              Temporal Server                                     │
│              Redis                                               │
└─────────────────────────────────────────────────────────────────┘
```

**Key Point:** Search, Review, Relationship, and Content services are NOT separate processes.
They are built into the Gateway (as `gateway/*service/` packages) or Worker (as activities).

## Core Components

### CLI (`cmd/penf/`)
Cobra-based command-line interface providing access to all system functionality.

| Command Group | Description |
|--------------|-------------|
| `auth` | OAuth2 authentication for Gmail and other services |
| `search` | Hybrid full-text and semantic search |
| `review` | Daily review queue management |
| `relationship` | Entity relationship discovery and management |
| `tenant` | Multi-tenant configuration |
| `workflow` | Temporal workflow management |

### Gateway (`services/gateway/`)
gRPC and HTTP API gateway handling authentication, routing, and rate limiting.

| Component | Responsibility |
|-----------|---------------|
| `server/` | HTTP and gRPC server setup |
| `router/` | Request routing and middleware |
| `orchestrator/` | Service coordination |
| `workflows/` | Gateway-specific workflows |

### Gmail Connector (`services/gmail/`)
Full Gmail integration with OAuth2 PKCE, real-time sync, and push notifications.

| Component | Responsibility |
|-----------|---------------|
| `oauth/` | OAuth2 PKCE flow with AES-256-GCM token encryption |
| `sync/` | Message synchronization with history tracking |
| `push/` | Cloud Pub/Sub push notifications |
| `scheduler/` | Multi-account sync scheduling |
| `attachment/` | Attachment processing pipeline |

### Worker (`services/worker/`)
Temporal worker executing durable workflows and activities.

| Component | Responsibility |
|-----------|---------------|
| `activities/` | Activity implementations |
| `workflows/` | Workflow definitions |
| `observability/` | Metrics and tracing |

### Shared Packages (`pkg/`)

| Package | Responsibility |
|---------|---------------|
| `db/` | PostgreSQL utilities with pgvector support |
| `temporal/` | Temporal SDK helpers and observability |
| `tracing/` | OpenTelemetry distributed tracing |
| `embeddings/` | Vector embedding generation |

## Protocol Buffers (`api/proto/`)

| Service | Description |
|---------|-------------|
| `gateway/v1` | Gateway API definitions |
| `search/v1` | Search service API |
| `review/v1` | Review service API |
| `gmail/v1` | Gmail connector API |
| `relationship/v1` | Relationship service API |
| `workflow/v1` | Workflow management API |

## Detailed Pattern Documentation

Architecture patterns are documented in detail in separate files:

| Document | Contents |
|----------|----------|
| [Core Patterns](architecture/core-patterns.md) | Pipeline processing, AI processing, entity resolution, version control, search, file handling, review workflows |
| [Testing Patterns](architecture/testing-patterns.md) | AI mocking, environment isolation, test data, benchmarking, categorization |
| [Observability Patterns](architecture/observability-patterns.md) | Agent health, workflow tracing, decision logging, KPIs, time-series storage |
| [Email Patterns](architecture/email-patterns.md) | OAuth2, push/poll sync, multi-account scheduling, attachments, privacy filters |
| [Relationship Patterns](architecture/relationship-patterns.md) | Confidence scoring, temporal decay, conflict resolution, lifecycle, network analysis |

## Technology Stack

| Layer | Technology |
|-------|------------|
| Language | Go 1.22+ |
| API | gRPC + Protocol Buffers |
| Database | PostgreSQL 16+ with pgvector |
| Workflows | Temporal |
| Embeddings | MLX (Apple Silicon sidecar) |
| Tracing | OpenTelemetry |

## Performance Targets

| Metric | Target |
|--------|--------|
| Context reconstruction | <15 minutes |
| Search accuracy | 90% |
| Search response time | <3 seconds |
| Email sync latency | <60 seconds |
| Relationship validation rate | >80% |

## Deployment Topology

Components are distributed across machines based on their requirements:

| Machine | Components | Rationale |
|---------|------------|-----------|
| **LAPTOP** (MacBook Pro) | penf CLI, Claude Code | User's development machine |
| **dev01** (Apple Silicon) | Worker, MLX Embeddings, MLX LLM, AI Service | GPU/Neural Engine for embeddings & LLM |
| **dev02** (Intel) | Gateway, PostgreSQL, Redis, Temporal | Data storage, no GPU needed |

**Key Principles:**
- CLI runs on laptop, connects to Gateway over the network (NO localhost)
- Co-locate services that exchange large data (Worker ↔ MLX on dev01)
- Co-locate services that need fast DB access (Gateway ↔ PostgreSQL on dev02)
- Cross-network traffic should be small payloads (gRPC calls, task dispatch)

> **Configuration:** See [infrastructure.md](infrastructure.md) for specific hostnames, ports, and connection strings.

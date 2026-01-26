# Penfold Architecture

**Last Updated**: 2026-01-20

> **Deployment Details:** See [infrastructure.md](infrastructure.md) for hostnames, ports, connection strings, and startup commands.

## Overview

Penfold is an AI-powered personal information system built with Go. It aggregates and correlates information from communication channels into a queryable institutional memory.

## System Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         CLI (penf)                              │
│                        cmd/penf/cmd                             │
└─────────────────────────┬───────────────────────────────────────┘
                          │ gRPC
┌─────────────────────────▼───────────────────────────────────────┐
│                     API Gateway                                  │
│                   services/gateway                               │
│  • Authentication & Authorization                                │
│  • Request routing                                               │
│  • Rate limiting                                                 │
└───────┬─────────────────┬─────────────────┬─────────────────────┘
        │                 │                 │
┌───────▼───────┐ ┌───────▼───────┐ ┌───────▼───────┐
│  Gmail Sync   │ │    Search     │ │    Review     │
│services/gmail │ │   Service     │ │   Service     │
│               │ │               │ │               │
│ • OAuth2 PKCE │ │ • Hybrid      │ │ • Daily queue │
│ • Push/Poll   │ │   search      │ │ • AI triage   │
│ • Attachments │ │ • Vector +    │ │ • Escalation  │
└───────┬───────┘ │   fulltext    │ └───────────────┘
        │         └───────────────┘
┌───────▼───────────────────────────────────────────┐
│                 Temporal Worker                    │
│                 services/worker                    │
│  • Durable workflow execution                      │
│  • Activity implementations                        │
│  • Retry and error handling                        │
└───────────────────────┬───────────────────────────┘
                        │
┌───────────────────────▼───────────────────────────┐
│              PostgreSQL + pgvector                 │
│                                                    │
│  • Content storage                                 │
│  • Vector embeddings                               │
│  • Relationship graph                              │
│  • Audit trails                                    │
└────────────────────────────────────────────────────┘
```

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

Components are distributed across two machines based on their requirements:

| Machine | Components | Rationale |
|---------|------------|-----------|
| **dev01** (Apple Silicon) | Worker, MLX Embeddings, CLI | GPU/Neural Engine for embeddings |
| **dev02** (Intel) | Gateway, PostgreSQL, Redis, Temporal | Data storage, no GPU needed |

**Key Principles:**
- Co-locate services that exchange large data (Worker ↔ Embeddings)
- Co-locate services that need fast DB access (Gateway ↔ PostgreSQL)
- Cross-network traffic should be small payloads (gRPC calls, task dispatch)

> **Configuration:** See [infrastructure.md](infrastructure.md) for specific hostnames, ports, and connection strings.

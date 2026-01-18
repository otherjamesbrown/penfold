# Migration Phases Roadmap

## Overview

This document outlines the phased migration from Python monolith to Go microservices. Each phase is designed to be independently deployable with Python and Go services coexisting.

## Phase Summary

| Phase | Focus | Services | Duration Est. |
|-------|-------|----------|---------------|
| 0 | Foundation | Protobuf, shared libs, Embedding Pipeline | Current |
| 1 | Core Infrastructure | API Gateway, Event Router, Thin CLI | 2-3 weeks |
| 2 | Read Path | Search Service | 1-2 weeks |
| 3 | Write Path | Gmail Connector, Content Processor | 3-4 weeks |
| 4 | Intelligence | AI Coordinator, Relationship Discovery | 3-4 weeks |
| 5 | User Facing | Review Service, Full CLI | 2-3 weeks |
| 6 | Decommission | Python shutdown, cleanup | 1-2 weeks |

## Phase 0: Foundation (Current)

### Status: In Progress

**Completed:**
- [x] Embedding Pipeline service (penfold-go-pipeline)
- [x] Redis event subscription
- [x] PostgreSQL + pgvector writes
- [x] MLX sidecar integration

**Remaining:**
- [ ] Define protobuf contracts for all services
- [ ] Create shared Go packages (pkg/)
- [ ] Set up monorepo structure
- [ ] CI/CD pipeline for Go services

### Deliverables

1. **Protobuf Definitions**
   ```
   api/proto/
   ├── penf/v1/cli.proto           # CLI service contract
   ├── gateway/v1/gateway.proto    # Gateway service
   ├── search/v1/search.proto      # Search service
   ├── gmail/v1/gmail.proto        # Gmail connector
   ├── content/v1/content.proto    # Content processor
   ├── review/v1/review.proto      # Review service
   ├── embedding/v1/embedding.proto # Embedding pipeline
   ├── ai/v1/ai.proto              # AI coordinator
   ├── relationship/v1/relationship.proto
   ├── eventrouter/v1/eventrouter.proto
   └── common/v1/common.proto      # Shared types
   ```

2. **Shared Libraries**
   ```
   pkg/
   ├── db/         # Database utilities
   ├── events/     # Event pub-sub
   ├── config/     # Configuration
   ├── logging/    # Structured logging
   ├── metrics/    # Prometheus metrics
   ├── health/     # Health checks
   └── auth/       # Authentication
   ```

3. **Monorepo Structure**
   ```
   penfold/
   ├── api/proto/              # All protobuf definitions
   ├── pkg/                    # Shared Go packages
   ├── services/               # Go microservices
   │   ├── gateway/
   │   ├── search/
   │   ├── gmail-connector/
   │   ├── content-processor/
   │   ├── event-router/
   │   ├── embedding-pipeline/
   │   ├── ai-coordinator/
   │   ├── relationship-discovery/
   │   └── review/
   ├── cmd/penf/               # Thin CLI
   ├── penf_lib/               # Python library (existing)
   ├── penf/                   # Python CLI (existing)
   └── app/                    # Python web app (existing)
   ```

### Success Criteria

- [ ] All protobuf files compile without errors
- [ ] Shared libraries have >80% test coverage
- [ ] Embedding Pipeline processes 100 events/minute
- [ ] CI builds all Go code on every commit

---

## Phase 1: Core Infrastructure

### Focus: API Gateway, Event Router, Thin CLI

**Goal:** Establish the core infrastructure that other services depend on.

### Services

#### 1.1 API Gateway
- gRPC server for CLI communication
- Authentication (token validation)
- Request routing to backend services
- Rate limiting
- Health aggregation

#### 1.2 Event Router
- Redis subscription for all event types
- Job management (create, claim, complete, fail)
- Retry with exponential backoff
- Dead letter queue
- Metrics and monitoring

#### 1.3 Thin CLI
- Connect to gateway
- Basic commands: health, search, tenant
- Config file management
- Token storage

### Dependencies
- Phase 0 complete (protobuf, shared libs)
- Python services continue running

### Migration Strategy

```
Before Phase 1:
┌─────────┐     ┌─────────────────────┐
│ Python  │────▶│  penf_lib + Redis   │
│  CLI    │     │  (Python handles    │
└─────────┘     │   everything)       │
                └─────────────────────┘

After Phase 1:
┌─────────┐     ┌─────────────┐     ┌─────────────────────┐
│ Go CLI  │────▶│ API Gateway │────▶│  Python Services    │
└─────────┘     │   (Go)      │     │  (search, etc.)     │
                └─────────────┘     └─────────────────────┘
                      │
                      ▼
                ┌─────────────┐
                │Event Router │────▶ Redis ────▶ Embedding Pipeline
                │    (Go)     │
                └─────────────┘
```

### Tasks

- [ ] Implement API Gateway with auth middleware
- [ ] Implement Event Router with job management
- [ ] Create thin CLI with connect/health/tenant commands
- [ ] Gateway proxies to Python FastAPI for existing endpoints
- [ ] Event Router coexists with Python event subscribers
- [ ] Deploy and validate

### Success Criteria

- [ ] CLI connects to gateway successfully
- [ ] `penf health` shows all services healthy
- [ ] Event Router processes 1000 events/minute
- [ ] No regression in Python functionality

---

## Phase 2: Read Path

### Focus: Search Service

**Goal:** Move the search functionality to Go for improved performance.

### Services

#### 2.1 Search Service
- Query parsing
- Query embedding (via Embedding Pipeline)
- BM25 full-text search
- pgvector similarity search
- RRF ranking fusion
- Result caching
- Search analytics

### Dependencies
- Phase 1 complete
- Embedding Pipeline for query embeddings

### Migration Strategy

```
Before Phase 2:
Gateway ────▶ Python FastAPI ────▶ penf_lib/search

After Phase 2:
Gateway ────▶ Search Service (Go) ────▶ PostgreSQL
                     │
                     └────▶ Embedding Pipeline (Go)
```

### Tasks

- [ ] Implement query parser
- [ ] Implement hybrid search (BM25 + vector)
- [ ] Implement RRF ranking
- [ ] Add search result caching
- [ ] Add search analytics tracking
- [ ] Update gateway to route search to Go service
- [ ] Deprecate Python search endpoints
- [ ] CLI search commands working end-to-end

### Success Criteria

- [ ] Search latency <500ms for typical queries
- [ ] Search results match Python implementation
- [ ] Cache hit rate >50%
- [ ] `penf search` and `penf ask` working

---

## Phase 3: Write Path

### Focus: Gmail Connector, Content Processor

**Goal:** Handle content ingestion in Go.

### Services

#### 3.1 Gmail Connector
- OAuth2 token management
- Gmail API sync (history API)
- Real-time push notifications
- Attachment processing
- Multi-account support
- Rate limiting

#### 3.2 Content Processor
- AI-powered content analysis
- Entity extraction
- Categorization
- Event publishing

### Dependencies
- Phase 1, 2 complete
- Event Router for job orchestration
- Embedding Pipeline for vectors

### Migration Strategy

```
Before Phase 3:
Python GmailConnector ────▶ Gmail API
       │
       └────▶ Redis events ────▶ Python processors

After Phase 3:
Go Gmail Connector ────▶ Gmail API
       │
       └────▶ Redis events ────▶ Event Router ────▶ Go Content Processor
                                                         │
                                                         ├─▶ Embedding Pipeline
                                                         └─▶ AI Coordinator
```

### Tasks

- [ ] Implement OAuth2 flow for Gmail
- [ ] Implement Gmail sync (incremental + full)
- [ ] Implement attachment processing
- [ ] Implement content processor with AI integration
- [ ] Migrate existing Gmail connections
- [ ] Update CLI with ingest commands
- [ ] Deprecate Python Gmail connector

### Success Criteria

- [ ] Gmail sync processes 100+ emails/minute
- [ ] OAuth tokens migrated successfully
- [ ] No data loss during migration
- [ ] `penf gmail` and `penf ingest` working

---

## Phase 4: Intelligence Layer

### Focus: AI Coordinator, Relationship Discovery

**Goal:** Move AI orchestration and relationship discovery to Go.

### Services

#### 4.1 AI Coordinator
- Model selection (local vs cloud)
- Confidence-based escalation
- Cost tracking
- Performance monitoring
- Ensemble result combining

#### 4.2 Relationship Discovery
- Entity relationship extraction
- Confidence scoring
- Conflict resolution
- Network analysis
- Feedback integration

### Dependencies
- Phase 1, 2, 3 complete
- vLLM-MLX for local models
- Gemini API for cloud models

### Migration Strategy

```
Before Phase 4:
Python AICoordination ────▶ vLLM / Gemini
Python RelationshipDiscovery ────▶ AI results ────▶ PostgreSQL

After Phase 4:
Go AI Coordinator ────▶ vLLM / Gemini (OpenAI-compatible API)
Go Relationship Discovery ────▶ AI results ────▶ PostgreSQL
```

### Tasks

- [ ] Implement AI Coordinator with model selection
- [ ] Implement cost tracking and budgets
- [ ] Implement relationship extraction
- [ ] Implement confidence scoring
- [ ] Implement network analysis
- [ ] Update CLI with relationship commands
- [ ] Deprecate Python AI coordination

### Success Criteria

- [ ] AI processing latency <2s average
- [ ] Relationship confidence matches Python
- [ ] Cost tracking within 5% accuracy
- [ ] `penf relationships` working

---

## Phase 5: User Facing

### Focus: Review Service, Full CLI

**Goal:** Complete user-facing functionality in Go.

### Services

#### 5.1 Review Service
- Review session management
- Review queue prioritization
- Feedback collection
- Automation rule engine
- Pattern detection

### Dependencies
- All previous phases complete

### Tasks

- [ ] Implement review service
- [ ] Implement automation rules
- [ ] Complete all CLI commands
- [ ] TUI for interactive review
- [ ] Deprecate Python review module

### Success Criteria

- [ ] Full review workflow in Go
- [ ] All CLI commands functional
- [ ] TUI matches Python UX
- [ ] `penf review` working

---

## Phase 6: Decommission Python

### Focus: Final cleanup

**Goal:** Remove Python services, single Go deployment.

### Tasks

- [ ] Remove Python FastAPI endpoints
- [ ] Remove Python CLI
- [ ] Archive penf_lib (keep for reference)
- [ ] Update deployment scripts
- [ ] Final documentation update
- [ ] Performance benchmarking

### Success Criteria

- [ ] No Python services running
- [ ] Single deployment artifact
- [ ] Documentation complete
- [ ] All metrics within targets

---

## Risk Mitigation

### Data Consistency
- All services share same PostgreSQL database
- Event schemas remain compatible (JSON)
- Rollback plan: Keep Python services ready to restart

### Feature Parity
- Each phase includes comparison testing
- Python tests adapted for Go
- A/B testing during migration

### Performance
- Benchmark each phase before/after
- Monitor latency, throughput, error rates
- Rollback if degradation >10%

### Downtime
- Zero-downtime deployments
- Blue-green deployment strategy
- Feature flags for gradual rollout

---

## Timeline (Estimated)

```
Week 1-2:   Phase 0 completion (protobuf, shared libs)
Week 3-5:   Phase 1 (Gateway, Event Router, CLI)
Week 6-7:   Phase 2 (Search Service)
Week 8-11:  Phase 3 (Gmail, Content Processor)
Week 12-15: Phase 4 (AI Coordinator, Relationships)
Week 16-18: Phase 5 (Review Service, Full CLI)
Week 19-20: Phase 6 (Decommission)

Total: ~20 weeks (5 months)
```

## Next Steps

1. Complete Phase 0 deliverables
2. Review protobuf contracts with team
3. Set up CI/CD for Go services
4. Begin Phase 1 implementation

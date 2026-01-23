# Architecture Review: Structure & Patterns

**Review Date**: 2026-01-23
**Reviewer**: Architecture Review Agent
**Context Reference**: pass-00-context.md

---

## Summary

Penfold employs a **modular monolith with service-oriented deployments** architecture style. The codebase is organized as a Go workspace (`go.work`) containing multiple independent modules that share a common `pkg/` library layer. Services are deployed independently but share domain models and infrastructure code, balancing the benefits of microservices (independent deployment, clear boundaries) with the simplicity demanded by a single-developer maintainability requirement.

The architecture demonstrates strong alignment with the constitution's principles, particularly around:
- **Single-developer maintainability**: Clean separation without excessive abstraction
- **CLI + Library pattern**: Every feature implemented as library first with CLI exposure
- **Local-first processing**: Clear architectural support for MLX embeddings sidecar
- **Evidence-based relationships**: Domain models include audit trails and confidence scoring

---

## Alignment with System Goals

### Mission Support Assessment

| System Goal | Architectural Support | Rating |
|-------------|----------------------|--------|
| Context Assembly Time < 15 min | Hybrid search (BM25 + vector), enrichment pipeline | Strong |
| Source Truth Preservation | Immutable `sources` table, versioned enrichments, audit trails | Excellent |
| Local-First Processing | Dedicated task queues, MLX sidecar integration, configurable cloud escalation | Strong |
| Single-Developer Maintainability | Go workspace with shared `pkg/`, clear module boundaries | Good |
| ADHD-Friendly Data Access | Repository pattern supports filtered queries; needs more temporal navigation | Moderate |
| Learning Laboratory | AI processing stage, multiple model support via registry pattern | Good |

### Constitutional Principle Alignment

**Immutable Content, Evolving Understanding**: The `sources` table stores raw content immutably. The `enrichments` table with `StageResult` history provides versioned analysis. This is architecturally well-supported.

**Local-First, Cloud-Strategic**: The worker separates AI-intensive work into dedicated task queues (`penfold-ai`, `penfold-main`, `penfold-email`). The MLX embeddings sidecar (port 8081) provides local embedding generation. Cloud escalation points are explicitly designed but not hardcoded.

**Evidence-Based Relationships**: The `mentions` package exemplifies this with `ContentMention.Candidates`, `ResolutionSource`, and `ConfidenceFactors`. Every relationship includes traceable provenance.

**User Control Preservation**: The review queue system (`pkg/reviewqueue`) and mention resolution workflow support human-in-the-loop validation. Status transitions (`pending` -> `auto_resolved` | `user_resolved` | `dismissed`) maintain user control.

---

## Findings

### Architecture Style: Modular Monolith with Service Facades

The system is organized as:

```
penfold/
├── cmd/penf/           # CLI entry point (Cobra-based)
├── services/           # Independently deployable services
│   ├── gateway/        # gRPC API Gateway (home-01)
│   ├── worker/         # Temporal worker (dev01)
│   ├── search/         # Search service (mostly stubs)
│   ├── gmail/          # Gmail connector
│   ├── ai/             # AI coordination service
│   ├── content/        # Content processing service
│   ├── relationship/   # Relationship discovery
│   └── review/         # Daily review service
├── pkg/                # Shared domain libraries
│   ├── auth/           # Authentication/authorization
│   ├── db/             # Database utilities
│   ├── embeddings/     # Embedding client/cache
│   ├── enrichment/     # Content enrichment pipeline
│   ├── glossary/       # Glossary management
│   ├── health/         # Health check framework
│   ├── ingest/         # Content ingestion
│   ├── mentions/       # Mention resolution
│   ├── metrics/        # Prometheus metrics
│   ├── products/       # Product management
│   ├── reviewqueue/    # Review queue operations
│   ├── sources/        # Source management
│   ├── temporal/       # Temporal client/worker
│   └── tracing/        # Distributed tracing
├── api/proto/          # Protocol Buffer definitions (versioned)
├── migrations/         # PostgreSQL migrations (21 files)
└── tests/              # Test hierarchy (unit, integration, e2e, live)
```

### Design Patterns in Use

#### 1. Repository Pattern (Ubiquitous)
**Implementation**: Every domain package (`mentions`, `glossary`, `sources`, `reviewqueue`, etc.) defines a `Repository` interface with a PostgreSQL implementation.

**Example** (`pkg/mentions/repository.go`):
```go
type Repository interface {
    CreateMention(ctx context.Context, input MentionInput) (*ContentMention, error)
    ListMentions(ctx context.Context, filter MentionFilter) ([]ContentMention, error)
    // ...
}
```

**Assessment**: Excellent fit for the system's goals. Enables:
- Test isolation via mock implementations
- Source truth preservation via explicit data access
- Single point for audit trail enforcement

#### 2. Pipeline/Stage Pattern (Enrichment)
**Implementation**: `pkg/enrichment/pipeline/` orchestrates content through stages:
1. Classification
2. Common Enrichment
3. Type-Specific Extraction
4. AI Routing
5. AI Processing
6. Post-Processing

**Assessment**: Well-aligned with "Immutable Content, Evolving Understanding". Each stage records results to `StageResult`, enabling:
- Analysis versioning
- Re-processing as models improve
- Audit trail for AI decisions

#### 3. Registry Pattern (Processors)
**Implementation**: `processors.ProcessorRegistry` allows registration of type-specific handlers:
```go
type ProcessorRegistry interface {
    GetByStage(stage Stage) []Processor
    GetTypeSpecificProcessor(subtype Subtype) (TypeSpecificProcessor, bool)
}
```

**Assessment**: Enables "Learning Laboratory" by supporting multiple processor implementations. Good for experimentation but watch for registry explosion.

#### 4. Temporal Workflows (Durable Execution)
**Implementation**: `services/worker/workflows/` with distinct task queues:
- `penfold-main`: General workflows (ContentIngestion, RelationshipDiscovery, DailyReview)
- `penfold-ai`: AI-intensive analysis
- `penfold-email`: Email processing

**Assessment**: Excellent architectural choice for:
- Graceful degradation (workflows survive crashes)
- Long-running operations (meeting transcription)
- Audit trail (Temporal maintains execution history)

#### 5. gRPC with Protocol Buffers (Service Contracts)
**Implementation**: 15 proto packages under `api/proto/` with versioned namespaces (`v1`).

**Assessment**: Strong contract definition. Supports:
- CLI-to-service communication
- Service-to-service calls
- Future external API exposure

### Strengths

#### 1. Clean Package Boundaries
The `pkg/` layer enforces domain boundaries without over-abstraction. Each package owns its types, repository interface, and business logic. Services compose these packages rather than duplicating logic.

**Example**: The gateway service imports `pkg/mentions`, `pkg/glossary`, `pkg/reviewqueue` and exposes them via gRPC without reimplementing domain logic.

#### 2. Test Infrastructure Hierarchy
Well-structured test organization:
```
tests/
├── unit/        # Package-level unit tests (colocated in pkg/)
├── integration/ # Database integration tests
├── e2e/         # End-to-end pipeline tests
└── live/        # Tests against real external services
```

This supports "Real-World Testing" without requiring live services for development.

#### 3. Explicit Multi-Tenancy
Every domain type includes `TenantID`. Default tenant (`00000001-0000-0000-0000-000000000001`) supports single-user mode while architecture supports future multi-tenancy.

#### 4. Configuration as Code
Dedicated config packages per service (`services/gateway/config`, `services/worker/config`) with environment variable binding. Supports "Implementation Simplicity" by avoiding external config systems.

#### 5. Observability Integration
Consistent observability across services:
- `pkg/logging`: Structured zerolog-based logging
- `pkg/metrics`: Prometheus metrics with middleware
- `pkg/tracing`: Langfuse integration for AI tracing
- `pkg/health`: Health check framework with aggregation

### Concerns

#### 1. Search Service Implementation Gap
The `services/search/server/server.go` contains stubs (`Unimplemented` status codes) for core search functionality. Given the constitutional target of "< 15 seconds for search", this represents a significant implementation gap.

**Risk**: Core value proposition (context assembly time) cannot be validated architecturally.

**Recommendation**: Prioritize search engine implementation. The `engine/` package has BM25 and vector search components but they are not integrated into the server.

#### 2. Proto Package Proliferation
The `go.work` file lists 15 separate proto module directories, each with its own `go.mod`. This creates maintenance overhead.

**Current State**:
```
api/proto/ai/v1
api/proto/cli/v1
api/proto/common/v1
api/proto/content/v1
... (15 total)
```

**Recommendation**: Consider consolidating into fewer proto modules while maintaining namespace separation within protos. A single `api/proto` module with package-based organization would reduce maintenance burden.

#### 3. Service Boundary Ambiguity
Some services have unclear deployment intent:
- `services/ai/`: Has full implementation but unclear how it relates to worker AI activities
- `services/content/`: Has pipeline code but worker also has content workflows
- `services/review/`: Duplicates logic present in `pkg/reviewqueue`

**Risk**: Cognitive overhead for single developer; unclear which components are actively deployed.

**Recommendation**: Document the deployment topology explicitly. Consider consolidating services that are always deployed together.

#### 4. Missing Temporal UI Integration
While Temporal workflows are well-implemented, there is no indication of Temporal UI or operational tooling for workflow inspection.

**Risk**: "Transparent AI Decision-Making" principle requires visibility into workflow execution. Without UI, debugging long-running AI workflows becomes difficult.

**Recommendation**: Deploy Temporal UI alongside Temporal server for workflow visibility.

#### 5. Enrichment Repository Coupling
The enrichment pipeline directly accesses `enrichment.Repository` for stage recording. This creates tight coupling between pipeline orchestration and persistence.

**Current** (`pkg/enrichment/pipeline/pipeline.go`):
```go
p.repository.RecordStage(ctx, stage)
p.repository.Update(ctx, e)
```

**Recommendation**: Consider event-based decoupling if pipeline needs to support alternative persistence or event sourcing in future.

### Recommendations

#### High Priority

1. **Complete Search Implementation**: The hybrid search architecture (BM25 + pgvector) is designed but not implemented in the server layer. This blocks the primary value proposition validation.

2. **Consolidate Proto Modules**: Reduce from 15 to 3-4 proto modules organized by domain (core, ai, content, workflow) to reduce maintenance overhead.

3. **Document Deployment Topology**: Create an architecture diagram showing which services run on which hosts (dev01 vs home-01) and their dependencies.

#### Medium Priority

4. **Temporal UI Deployment**: Add Temporal UI to the infrastructure for workflow visibility and debugging support.

5. **Service Consolidation Audit**: Review `services/ai`, `services/content`, and `services/review` for opportunities to consolidate with worker activities or eliminate unused code.

6. **Add Temporal Navigation to Domain Models**: Current repository patterns support filtered queries but lack first-class temporal navigation (e.g., "context around timestamp X"). This would better support ADHD-friendly browsing.

#### Low Priority

7. **Consider Event Sourcing for Enrichments**: The current stage recording pattern would benefit from event sourcing if analysis re-running becomes a frequent operation.

8. **Standardize Logger Types**: Mix of `zerolog.Logger` and `logging.Logger` across packages creates minor inconsistency.

---

## Module Dependency Graph

```
cmd/penf
    └── pkg/* (all packages)
    └── services/search (for types only)

services/gateway
    └── pkg/{auth, glossary, mentions, products, reviewqueue, sources}
    └── api/proto/*

services/worker
    └── pkg/{mentions, temporal, health, metrics, tracing}
    └── api/proto/*

services/gmail
    └── pkg/{auth, config, db, logging}

pkg/enrichment
    └── pkg/enrichment/{classification, config, entities, extraction, handlers, pipeline, processors, queues, workers}

pkg/mentions
    └── pkg/mentions/{audit, learning, resolver}
```

**Observation**: The `pkg/` layer forms a cohesive domain model. Services are thin orchestration layers over `pkg/` functionality, which is the intended CLI + Library pattern.

---

## Conclusion

The Penfold architecture is well-suited to its stated goals. The modular monolith approach appropriately balances single-developer maintainability with clean component boundaries. Key patterns (Repository, Pipeline, Temporal Workflows) directly support constitutional principles around source truth, evidence-based relationships, and transparent AI decision-making.

The primary risks are implementation gaps (search service) and minor organizational overhead (proto proliferation, service boundary clarity). These do not represent fundamental architectural concerns but should be addressed to realize the full value of the design.

**Overall Architecture Fitness**: Good alignment with system goals and constitutional principles.

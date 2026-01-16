# Research: Relationship Discovery and Management

**Feature**: 009-relationship-discovery-and-management
**Date**: 2026-01-15
**Status**: Complete

## Research Tasks

### 1. Relationship Extraction from Email/Meeting Content

**Decision**: Use structured prompting with local LLM (llama-3.1-8b) for relationship extraction, with cloud escalation for low-confidence results.

**Rationale**:
- Aligns with existing AI coordination framework (003-ai-coordination)
- Local processing maintains privacy for sensitive relationship data
- Structured JSON output enables reliable parsing
- Confidence-based escalation ensures quality without excessive cloud costs

**Alternatives Considered**:
- Named Entity Recognition (NER) only: Rejected - misses implicit relationships
- Rule-based extraction: Rejected - too rigid for natural language variation
- Cloud-only processing: Rejected - privacy concerns, cost at scale

### 2. Confidence Scoring Algorithm

**Decision**: Multi-factor confidence scoring combining:
1. AI extraction confidence (0.0-1.0)
2. Evidence strength (interaction frequency, recency, diversity)
3. Entity resolution confidence (from existing cross-tenant links)
4. User feedback history (learning component)

**Rationale**:
- Single AI confidence is insufficient for relationship quality
- Evidence-based scoring aligns with temporal-first architecture
- User feedback enables continuous improvement (SC-002: 25% improvement target)
- Composite score supports the 70% threshold for auto-resolution

**Alternatives Considered**:
- AI confidence only: Rejected - doesn't account for evidence strength
- Binary classification: Rejected - loses nuance for user review prioritization
- Manual scoring: Rejected - doesn't scale

### 3. Conflict Resolution Strategy

**Decision**: Confidence-weighted auto-resolution with 30% gap threshold, otherwise user escalation.

**Rationale**:
- Clear decision boundary reduces ambiguity
- 30% gap provides sufficient differentiation for automated decisions
- User escalation for close calls maintains quality (from clarification)
- Audit trail preserves resolution history

**Alternatives Considered**:
- Always highest confidence: Rejected - misses nuanced conflicts
- Always user review: Rejected - doesn't scale
- Voting among multiple extractions: Rejected - adds complexity without clear benefit

### 4. Relationship Type Taxonomy

**Decision**: Hierarchical type system with base types and subtypes:

```
person-to-person:
  - professional (colleague, manager, report, mentor)
  - communication (frequent_contact, cc_only, meeting_attendee)

person-to-project:
  - role (owner, contributor, stakeholder, reviewer)
  - involvement (active, historical, mentioned)

project-to-project:
  - dependency (blocks, blocked_by, related, successor)
  - organizational (parent, child, sibling)

topic-to-entity:
  - association (primary, secondary, mentioned)
```

**Rationale**:
- Covers FR-002 requirements
- Hierarchical structure enables both specific and general queries
- Subtypes provide actionable insight
- Extensible for future relationship types

**Alternatives Considered**:
- Flat type list: Rejected - loses semantic hierarchy
- Free-form types: Rejected - inconsistent, harder to analyze
- Graph-only (no types): Rejected - loses semantic meaning

### 5. Lifecycle State Machine

**Decision**: Four-state lifecycle with explicit transitions:

```
States: pending -> active -> historical -> archived

Transitions:
- pending -> active: User confirmation OR high confidence threshold
- active -> historical: Inactivity period OR project completion
- historical -> archived: 2-year retention period exceeded
- active -> archived: User rejection
- historical -> active: New evidence (re-activation)
```

**Rationale**:
- Clear state boundaries for maintenance jobs
- Aligns with clarified retention policy (2-year default)
- Supports FR-006 lifecycle tracking
- Re-activation path prevents premature archival

**Alternatives Considered**:
- Binary (active/inactive): Rejected - loses historical context
- No archival: Rejected - unbounded storage growth
- Immediate deletion: Rejected - loses audit trail

### 6. Entity Resolution Integration

**Decision**: Delegate to existing CrossTenantPersonLink system with relationship-specific confidence overlay.

**Rationale**:
- Reuses proven infrastructure (from clarification)
- Avoids duplicate entity resolution logic
- Relationship confidence can augment person link confidence
- Maintains consistency with existing entity model

**Alternatives Considered**:
- Separate relationship entity resolution: Rejected - duplication
- No entity resolution: Rejected - fails multi-alias edge case

### 7. Database Schema Design

**Decision**: Five new tables in existing schema:

1. `relationships` - Core relationship records
2. `relationship_evidence` - Supporting evidence with source links
3. `relationship_feedback` - User validation history
4. `relationship_versions` - Change tracking (temporal audit)
5. `relationship_conflicts` - Pending conflicts for resolution

**Rationale**:
- Normalized design supports complex queries
- Evidence separation enables efficient pruning
- Feedback table supports learning algorithm
- Version table enables historical analysis
- Conflict table supports async resolution workflow

**Alternatives Considered**:
- Single denormalized table: Rejected - evidence bloat, update complexity
- Graph database: Rejected - adds infrastructure, PostgreSQL sufficient
- JSONB-only storage: Rejected - loses query optimization

### 8. Event Integration

**Decision**: Extend existing event schema with relationship events:

```
relationship.discovered - New relationship found
relationship.updated - Confidence/type changed
relationship.conflict - Conflict requiring resolution
relationship.validated - User feedback received
relationship.archived - Relationship archived
```

**Rationale**:
- Consistent with event processing framework (002-event-processing)
- Enables reactive processing and notifications
- Supports daily review integration (006-daily-review)
- Decouples discovery from downstream consumers

**Alternatives Considered**:
- Direct database polling: Rejected - inefficient, not reactive
- Separate message queue: Rejected - adds infrastructure
- No events: Rejected - breaks event-driven architecture

### 9. Performance Optimization

**Decision**: Batch processing with incremental updates:

1. Content processing triggers relationship extraction (async)
2. Batch confidence recalculation (scheduled)
3. Incremental evidence pruning (retention job)
4. Indexed queries for network analysis

**Rationale**:
- Async extraction prevents ingestion blocking (SC-006: 60s limit)
- Batch recalculation amortizes cost
- Retention job maintains storage bounds
- Indexes support sub-second queries

**Alternatives Considered**:
- Synchronous processing: Rejected - blocks ingestion
- Real-time recalculation: Rejected - expensive at scale
- No batching: Rejected - inefficient database access

### 10. Network Analysis Approach (P2)

**Decision**: Deferred to P2 implementation, but schema designed to support:

1. Degree centrality (connection count)
2. Betweenness centrality (bridge identification)
3. Cluster detection (community finding)
4. Path analysis (relationship pathways)

**Rationale**:
- P2 priority per spec
- Schema supports future analysis
- Basic metrics can be computed with SQL
- Advanced analysis may need specialized library (networkx)

**Alternatives Considered**:
- Implement now: Rejected - YAGNI principle
- Different analysis: TBD based on user feedback

## Integration Points

| System | Integration Type | Notes |
|--------|-----------------|-------|
| 001-database-schema | Schema extension | New tables in existing DB |
| 002-event-processing | Event producer/consumer | Relationship events |
| 003-ai-coordination | Service consumer | Extraction via coordinator |
| 004-gmail-integration | Content source | Email relationships |
| 005-meeting-pipeline | Content source | Meeting relationships |
| 006-daily-review | UI integration | Validation workflow |
| 007-search-interface | Query enhancement | Relationship context |

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Low extraction accuracy | Medium | High | Confidence thresholds, user feedback loop |
| Performance impact on ingestion | Low | High | Async processing, batching |
| Storage growth | Medium | Medium | 2-year retention, evidence pruning |
| Conflict resolution bottleneck | Low | Medium | 30% auto-resolution threshold |

## Open Questions (Resolved)

1. **Entity resolution approach** - Resolved: Use existing system with overlay
2. **Retention policy** - Resolved: 2-year default, configurable
3. **Conflict resolution** - Resolved: 30% gap auto-resolution

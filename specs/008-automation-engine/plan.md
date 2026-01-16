# Implementation Plan: Automation Rules Engine

**Branch**: `008-automation-engine` | **Date**: 2026-01-15 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/008-automation-engine/spec.md`

## Summary

The Automation Rules Engine enables intelligent, progressive automation of content categorization based on user feedback patterns and AI confidence scores. It integrates with the existing AI coordination framework to automatically process high-confidence suggestions, create and manage automation rules from user behavior patterns, and progressively increase automation rates while maintaining accuracy targets.

**Technical Approach**: Build on existing `penf_lib` patterns with new automation models, a rule engine service, and CLI commands. Leverage PostgreSQL for rule storage with JSONB for flexible conditions, integrate with existing event processing for real-time automation, and use the AI coordination framework for confidence scoring.

## Technical Context

**Language/Version**: Python 3.12
**Primary Dependencies**: SQLAlchemy 2.0+ (async), asyncpg, Click 8.1+, Pydantic 2.5+, Redis 5.0+
**Storage**: PostgreSQL 16+ with pgvector extension (existing database infrastructure)
**Testing**: pytest with pytest-asyncio, 80%+ coverage required
**Target Platform**: macOS/Linux CLI application
**Project Type**: Single library project with CLI interface (existing `penf_lib` pattern)
**Performance Goals**: Rule processing < 500ms per item (SC-009), 1000+ rules supported
**Constraints**: Async/await throughout, user-scoped rules, audit trail required
**Scale/Scope**: Single user initially, 100+ rules per user, 10K+ automation decisions tracked

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Evidence |
|-----------|--------|----------|
| I. Specification-First | PASS | Comprehensive spec completed with 5 user stories, 15 FRs, 10 SCs, 5 clarifications |
| II. CLI + Library Architecture | PASS | Design follows existing penf_lib pattern with library + CLI commands |
| III. Test-First Development | PASS | All implementation via TDD workflow required |
| IV. Integration Testing | PASS | Contract tests planned for rule engine integration with AI coordination |
| V. Observability & Debugging | PASS | Audit trail (FR-006), structured logging, decision tracking included |
| VI. Versioning & Breaking Changes | PASS | Rule versioning with full history (Clarification #4) |
| VII. Simplicity & YAGNI | PASS | Core automation first, advanced features (conflict resolution) are P3 |
| VIII. Temporal-First Data Organization | PASS | All entities timestamped, version history tracked |
| IX. Research-Driven Design | PASS | Research phase completed for rule engine patterns |
| X. Local-First AI with Privacy | PASS | Rules are user-scoped, local processing default |

**Gate Result**: PASS - All constitution principles satisfied

## Project Structure

### Documentation (this feature)

```text
specs/008-automation-engine/
├── plan.md              # This file
├── research.md          # Phase 0 output - rule engine patterns research
├── data-model.md        # Phase 1 output - entity definitions
├── quickstart.md        # Phase 1 output - getting started guide
├── contracts/           # Phase 1 output - API contracts
│   ├── automation-api.yaml
│   └── events.md
└── tasks.md             # Phase 2 output (from /speckit.tasks)
```

### Source Code (repository root)

```text
penf_lib/
├── automation/                    # NEW: Automation rules engine
│   ├── __init__.py
│   ├── models.py                  # SQLAlchemy models for rules, decisions
│   ├── engine.py                  # Core rule matching and execution engine
│   ├── conditions.py              # Rule condition evaluation logic
│   ├── patterns.py                # Pattern detection from user behavior
│   ├── progressive.py             # Progressive automation advancement
│   └── repository.py              # Data access layer for automation entities
│
├── cli/
│   ├── automation_commands.py     # NEW: CLI commands for rule management
│   └── ... (existing)
│
├── storage/
│   ├── migrations/
│   │   └── versions/
│   │       └── 20260115_xxxx_add_automation_tables.py  # NEW: Migration
│   └── ... (existing)
│
└── ... (existing modules)

tests/
├── unit/
│   ├── test_automation_engine.py      # NEW: Rule engine unit tests
│   ├── test_automation_conditions.py  # NEW: Condition evaluation tests
│   └── test_automation_patterns.py    # NEW: Pattern detection tests
│
├── integration/
│   ├── test_automation_integration.py # NEW: Full workflow tests
│   └── test_automation_ai_coord.py    # NEW: AI coordination integration
│
└── contract/
    └── test_automation_contracts.py   # NEW: API contract tests
```

**Structure Decision**: Extends existing `penf_lib` with new `automation/` module following established patterns. No new top-level packages needed.

## Complexity Tracking

> No constitution violations requiring justification.

| Aspect | Decision | Rationale |
|--------|----------|-----------|
| Rule storage | JSONB conditions in PostgreSQL | Flexible schema for varying condition types without schema migration overhead |
| Version history | Soft-versioned records | Each edit creates new version; aligns with existing SoftDeleteMixin pattern |
| Conflict resolution | Confidence-based auto | Self-optimizing; reduces user burden per Clarification #2 |

## Implementation Phases

### Phase 1: Core Rule Engine (P1 Stories)
**Target**: Basic rule storage, matching, and execution

1. Database models for AutomationRule, AutomationDecision, ConfidenceThreshold
2. Rule condition evaluation engine
3. Integration with AI coordination confidence scores
4. Basic CLI for rule CRUD operations

### Phase 2: Progressive Automation (P1 Stories)
**Target**: Learning and advancement system

1. Pattern detection from user feedback
2. Rule suggestion generation
3. Confidence threshold adjustment logic
4. Automation rate tracking and optimization

### Phase 3: Monitoring & Optimization (P2 Stories)
**Target**: Rule effectiveness tracking

1. RuleEffectiveness metrics collection
2. Performance analytics and reporting
3. Rule degradation detection
4. CLI commands for analytics

### Phase 4: Conflict Resolution (P3 Stories)
**Target**: Advanced multi-rule scenarios

1. Conflict detection logic
2. Resolution strategies (confidence-based)
3. User notification system
4. Conflict prevention at rule creation

## Dependencies Map

```text
[001-database-schema] ─────┐
                           │
[002-event-processing] ────┼──▶ [008-automation-engine]
                           │
[003-ai-coordination] ─────┤
                           │
[006-daily-review] ────────┘
```

**Critical Path**:
1. AI coordination provides confidence scores for automation decisions
2. Event processing triggers automation rule evaluation
3. Daily review workflow integrates for user override capability

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Rule performance at scale | Medium | High | Implement rule caching, indexed lookups |
| Confidence score drift | Low | Medium | Monitor accuracy, alert on degradation |
| User trust erosion | Medium | High | Conservative defaults (85%), clear audit trail |
| Rule explosion | Low | Medium | Epic-based organization, cleanup recommendations |

## Success Metrics Validation

| Metric | Target | Validation Method |
|--------|--------|-------------------|
| SC-002 | 95% accuracy | Automated testing against labeled dataset |
| SC-009 | <500ms processing | Performance tests in CI pipeline |
| SC-003 | 60% auto-processing after 30 days | Integration test with simulated learning |
| SC-004 | 85% rule effectiveness | Rule metrics tracking + reporting |

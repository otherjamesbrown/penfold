# Implementation Plan: Relationship Discovery and Management

**Branch**: `009-relationship-discovery-and-management` | **Date**: 2026-01-15 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/009-relationship-discovery-and-management/spec.md`

## Summary

Implement an AI-powered relationship discovery and management system that automatically identifies connections between people, projects, and topics from email and meeting content. The system uses the existing AI coordination framework for extraction, provides confidence-based validation workflows, and maintains relationship lifecycle with time-based retention policies.

## Technical Context

**Language/Version**: Python 3.12 with async/await
**Primary Dependencies**: SQLAlchemy 2.0, asyncpg, Redis, Click, Pydantic 2.x, pgvector
**Storage**: PostgreSQL 16+ with pgvector extension (existing infrastructure)
**Testing**: pytest with pytest-asyncio, pytest-cov (80% minimum coverage)
**Target Platform**: Linux/macOS server (Mac Mini M4 development)
**Project Type**: Single project - library extension with CLI commands
**Performance Goals**: <60 seconds per content item processing, real-time ingestion unimpacted
**Constraints**: <30% false positive rate for high-confidence relationships, 2-year retention default
**Scale/Scope**: Personal information system - thousands of relationships, millions of evidence records

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Specification-First | PASS | Spec complete with clarifications |
| II. CLI + Library Architecture | PASS | Extends penf_lib with CLI commands |
| III. Test-First Development | PENDING | Tests to be written before implementation |
| IV. Integration Testing | PENDING | Contract tests needed for AI coordination integration |
| V. Observability & Debugging | PASS | Uses existing structured logging infrastructure |
| VI. Versioning & Breaking Changes | PASS | New tables with migrations |
| VII. Simplicity & YAGNI | PASS | Builds on existing entity resolution, AI coordination |
| VIII. Temporal-First Data | PASS | All relationships track temporal context |
| IX. Research-Driven Design | PASS | Uses proven graph/relationship patterns |
| X. Local-First AI with Privacy | PASS | Local LLM for extraction, tenant isolation |

## Project Structure

### Documentation (this feature)

```text
specs/009-relationship-discovery-and-management/
├── plan.md              # This file
├── research.md          # Phase 0 output - relationship extraction patterns
├── data-model.md        # Phase 1 output - relationship entity models
├── quickstart.md        # Phase 1 output - developer guide
├── contracts/           # Phase 1 output - API contracts
└── tasks.md             # Phase 2 output (/speckit.tasks command)
```

### Source Code (repository root)

```text
penf_lib/
├── relationships/                    # NEW: Relationship discovery module
│   ├── __init__.py                   # Module exports
│   ├── models.py                     # Relationship domain models (Pydantic)
│   ├── discovery.py                  # RelationshipDiscoveryService
│   ├── extractor.py                  # AI-based relationship extraction
│   ├── confidence.py                 # Confidence scoring algorithms
│   ├── conflict_resolver.py          # Conflict resolution (30% gap rule)
│   ├── lifecycle.py                  # Lifecycle management and maintenance
│   ├── feedback.py                   # User feedback processing
│   └── network.py                    # Network analysis (P2 feature)
│
├── storage/
│   ├── models.py                     # EXTEND: Add relationship tables
│   ├── migrations/versions/
│   │   └── YYYYMMDD_HHMM_add_relationship_tables.py  # NEW migration
│   └── repositories/
│       └── relationship.py           # NEW: RelationshipRepository
│
├── cli/
│   └── relationships.py              # NEW: CLI commands for relationships
│
└── events/
    └── schemas.py                    # EXTEND: Relationship event schemas

tests/
├── unit/
│   └── relationships/                # Unit tests for relationship module
│       ├── test_discovery.py
│       ├── test_extractor.py
│       ├── test_confidence.py
│       ├── test_conflict_resolver.py
│       ├── test_lifecycle.py
│       └── test_feedback.py
├── integration/
│   └── test_relationship_workflow.py # End-to-end relationship processing
└── contract/
    └── test_relationship_events.py   # Event contract tests
```

**Structure Decision**: Extends existing penf_lib architecture with new `relationships/` module following established patterns from `ai_coordination/` and `connectors/gmail/`.

## Complexity Tracking

No constitution violations requiring justification. Design follows existing patterns.

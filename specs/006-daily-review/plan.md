# Implementation Plan: Daily Review Workflow Interface

**Branch**: `006-daily-review` | **Date**: 2026-01-15 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/006-daily-review/spec.md`

## Summary

The Daily Review Workflow Interface provides a streamlined CLI for reviewing overnight AI processing results, validating or correcting AI suggestions about email and meeting categorization, and feeding corrections back to the learning system. The implementation leverages the existing penf_lib architecture with Click CLI framework, Rich terminal rendering, and async PostgreSQL storage via SQLAlchemy 2.0.

## Technical Context

**Language/Version**: Python 3.12 with async/await throughout
**Primary Dependencies**: Click 8.1+, Rich 13.7+, SQLAlchemy 2.0 (asyncpg), Pydantic 2.5+
**Storage**: PostgreSQL 16+ with pgvector, Redis for session management
**Testing**: pytest with pytest-asyncio, 80%+ coverage target
**Target Platform**: macOS (development), Linux server (production)
**Project Type**: Single project - extends existing penf_lib CLI
**Performance Goals**: <2 second response for all review operations, 10+ items/minute navigation rate
**Constraints**: <200ms for queue navigation, session persistence across interruptions
**Scale/Scope**: 50-200 review items per day, single user CLI workflow

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Compliance Notes |
|-----------|--------|------------------|
| I. Specification-First Development | PASS | Comprehensive spec.md completed with user stories, requirements, success criteria |
| II. CLI + Library Architecture | PASS | Extends penf_lib with dedicated review module; CLI exposed via Click; independently testable |
| III. Test-First Development (TDD) | PASS | Plan includes TDD approach; tests written before implementation for all review operations |
| IV. Integration Testing | PASS | Contract tests for AI coordination integration; integration tests for database operations |
| V. Observability & Debugging | PASS | Structured logging for review decisions; Rich terminal output for debugging; traceable operations |
| VI. Versioning & Breaking Changes | PASS | New module; no breaking changes to existing interfaces |
| VII. Simplicity & YAGNI | PASS | Focus on core P1 features first; P2/P3 planned for later phases |
| VIII. Temporal-First Data Organization | PASS | Review items organized by processing timestamp; session state temporal |
| IX. Research-Driven Design | PASS | Phase 0 research addresses CLI UX patterns, queue prioritization algorithms |
| X. Local-First AI with Privacy | PASS | Review interface is local CLI; AI processing results come from existing local-first pipeline |

## Project Structure

### Documentation (this feature)

```text
specs/006-daily-review/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
penf_lib/
├── review/                      # NEW: Daily Review module
│   ├── __init__.py
│   ├── models.py               # ReviewQueue, ReviewItem, ReviewSession, UserFeedback
│   ├── queue.py                # Queue management and prioritization
│   ├── session.py              # Session persistence and state management
│   ├── feedback.py             # User feedback capture and learning integration
│   ├── batch.py                # Batch operations support
│   └── analytics.py            # Review analytics (P3)
├── cli/
│   ├── review_commands.py      # NEW: penf review command group
│   └── review_display.py       # NEW: Rich terminal rendering for review UI
├── storage/
│   ├── repositories/
│   │   └── review.py           # NEW: Review repository for database operations
│   └── migrations/versions/
│       └── 20260115_xxxx_add_review_tables.py  # NEW: Migration for review entities

tests/
├── contract/
│   └── test_review_contracts.py     # NEW: Contract tests for AI coordination integration
├── integration/
│   └── test_review_integration.py   # NEW: Integration tests for review workflow
└── unit/
    └── review/                       # NEW: Unit tests for review module
        ├── test_queue.py
        ├── test_session.py
        ├── test_feedback.py
        └── test_batch.py
```

**Structure Decision**: Extends existing penf_lib single-project structure. New `review/` module added alongside existing modules (ai_coordination, connectors, events, storage). CLI commands added to existing cli/ directory.

## Complexity Tracking

> No constitution violations - all principles pass. No complexity justifications needed.

## Constitution Check (Post-Design)

*Re-evaluated after Phase 1 design completion.*

| Principle | Status | Post-Design Notes |
|-----------|--------|-------------------|
| I. Specification-First Development | PASS | Spec informed all design decisions; data-model.md derived from spec entities |
| II. CLI + Library Architecture | PASS | Clear separation: penf_lib/review/ (library), cli/review_commands.py (interface) |
| III. Test-First Development (TDD) | PASS | Test structure defined; unit/integration/contract test files specified |
| IV. Integration Testing | PASS | Contract tests for AI coordination; integration tests for full workflow |
| V. Observability & Debugging | PASS | Structured logging in service layer; CLI output with Rich formatting |
| VI. Versioning & Breaking Changes | PASS | New tables follow existing migration pattern; no schema conflicts |
| VII. Simplicity & YAGNI | PASS | P1 features only in initial implementation; P2/P3 deferred to later phases |
| VIII. Temporal-First Data Organization | PASS | All entities include timestamps; source_timestamp for ordering; session expiry |
| IX. Research-Driven Design | PASS | research.md captures CLI UX patterns, prioritization algorithms, batch safety |
| X. Local-First AI with Privacy | PASS | All data remains in local PostgreSQL; no cloud dependencies for review |

**Design Artifacts Validated**:
- data-model.md: 6 entities defined with full schema, constraints, indexes
- contracts/cli-api.md: Complete CLI command reference with options and output formats
- quickstart.md: User documentation for immediate productivity
- research.md: All technical decisions documented with rationale

**Ready for Phase 2**: Task generation via `/speckit.tasks` or `/speckit.beads`

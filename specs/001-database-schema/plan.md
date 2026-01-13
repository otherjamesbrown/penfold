# Implementation Plan: Database Schema and Storage Layer

**Branch**: `001-database-schema` | **Date**: 2026-01-12 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/001-database-schema/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/commands/plan.md` for the execution workflow.

## Summary

Foundational PostgreSQL database with pgvector extension providing hybrid relational and vector storage for Penfold's AI-powered information system. Implements logical schema separation (core, events, vector), event-driven processing framework with Redis pub-sub, HNSW vector indexing for 768-dimensional embeddings, and comprehensive audit capabilities with soft deletes and archive tables.

## Technical Context

**Language/Version**: Python 3.12
**Primary Dependencies**: PostgreSQL 16+, pgvector extension, SQLAlchemy 2.0+ with asyncpg driver for async operations, Redis, alembic for migrations
**Storage**: PostgreSQL with pgvector for hybrid relational and vector storage
**Testing**: pytest with asyncio support, database fixtures, contract testing for API schemas
**Target Platform**: Mac Mini M4 with 32GB RAM (local development), Linux server deployment
**Project Type**: Library with CLI interface - foundational data layer for AI processing system
**Performance Goals**: Sub-100ms CRUD operations, sub-500ms vector similarity searches, sub-50ms event pub/sub operations
**Constraints**: 99.9% uptime, ACID compliance, concurrent operation support (50+ connections), 30-day processing result retention
**Scale/Scope**: Support 100k content items, 100k vectors, real-time event processing, horizontal scaling readiness

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

**✅ I. Specification-First Development**: Complete specification with clarifications resolved through product management review
**✅ II. CLI + Library Architecture**: Database layer designed as standalone library with clear interfaces
**✅ III. Test-First Development**: Plan includes comprehensive testing with pytest, fixtures, and contract tests
**✅ IV. Integration Testing**: Contract tests planned for schema changes and external dependencies
**✅ V. Observability & Debugging**: Structured logging planned for all database operations
**✅ VI. Versioning & Breaking Changes**: Alembic migrations provide versioned schema evolution
**✅ VII. Simplicity & YAGNI**: Core storage functionality only, no speculative features
**✅ VIII. Temporal-First Data Organization**: All entities include timestamps, supports temporal queries
**✅ IX. Research-Driven Design**: PostgreSQL + pgvector choice based on current vector database research
**✅ X. Local-First AI with Privacy**: Local PostgreSQL deployment with data sovereignty

**Gate Status**: ✅ PASS - All constitution principles satisfied

**Post-Design Re-evaluation**: ✅ CONFIRMED
- **Test-First Development**: Comprehensive testing strategy with pytest, fixtures, and performance benchmarks
- **Integration Testing**: Contract tests for schema changes, Redis pub-sub, and vector operations
- **Observability**: Structured logging, performance monitoring, and comprehensive error handling
- **Simplicity**: Core storage functionality only, no speculative features, clear modular design

## Project Structure

### Documentation (this feature)

```text
specs/[###-feature]/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)
<!--
  ACTION REQUIRED: Replace the placeholder tree below with the concrete layout
  for this feature. Delete unused options and expand the chosen structure with
  real paths (e.g., apps/admin, packages/something). The delivered plan must
  not include Option labels.
-->

```text
penf_lib/
├── storage/
│   ├── __init__.py
│   ├── models.py           # SQLAlchemy 2.0 async entity models
│   ├── schemas.py          # Database logical schema definitions
│   ├── migrations/         # Alembic migration scripts
│   ├── connections.py      # Async connection pooling and session management
│   ├── events.py          # Event publishing and subscription
│   └── vector.py          # Vector storage and similarity search
├── cli/
│   ├── __init__.py
│   ├── database.py        # Database management commands
│   └── migrations.py      # Migration CLI commands
└── __init__.py

tests/
├── fixtures/
│   ├── database.py        # Test database setup/teardown
│   └── sample_data.py     # Test data generation
├── integration/
│   ├── test_migrations.py # Schema migration testing
│   ├── test_events.py     # Event processing testing
│   └── test_vector.py     # Vector search testing
├── unit/
│   ├── test_models.py     # Entity model testing
│   └── test_connections.py # Connection management testing
└── contract/
    └── test_schemas.py     # Database schema contract tests
```

**Structure Decision**: Single library project structure under `penf_lib/storage/` for core database functionality. Separate modules for async models, schema management, connections, events, and vector operations. SQLAlchemy 2.0 with asyncpg driver provides async/await support for high-performance concurrent operations. Comprehensive test structure with async fixtures, unit tests, integration tests, and contract tests. CLI interface under `penf_lib/cli/` for database management operations.

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| [e.g., 4th project] | [current need] | [why 3 projects insufficient] |
| [e.g., Repository pattern] | [specific problem] | [why direct DB access insufficient] |

# Implementation Plan: Manual Content Ingest Framework

**Branch**: `012-manual-ingest` | **Date**: 2026-01-16 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/012-manual-ingest/spec.md`

## Summary

Implement a type-based manual ingest framework enabling users to upload archived .eml email files into Penfold. The framework will establish the `penf ingest <type>` CLI pattern with email as the first content type. Integrates with existing `sources` table and AI processing pipeline (events, embeddings, summarization) while adding new entities for job tracking (IngestJob), attachment handling (EmailAttachment), and encrypted file archival (ArchivedFile).

**Key technical approach:**
- Extend existing CLI with new `ingest` command group
- Use Python standard library `email` module for RFC 822/2822 parsing
- Leverage existing Source model with `source_system = "manual_eml"`
- Implement AES-256-GCM encryption for archived original files using tenant-scoped keys
- Publish `content.ingested` events to trigger existing AI pipeline

## Technical Context

**Language/Version**: Python 3.12
**Primary Dependencies**: Click 8.1+, Rich 13.7+, SQLAlchemy 2.0 (asyncpg), Pydantic 2.5+, Redis 5.0+, cryptography
**Storage**: PostgreSQL 16+ with pgvector extension
**Testing**: pytest 7.4+ with pytest-asyncio, pytest-cov
**Target Platform**: Linux/macOS CLI (local development server)
**Project Type**: Single project - extends existing `penf_lib` library
**Performance Goals**: Single file <5s, batch 100 files <2 minutes (SC-001, SC-002)
**Constraints**: RFC 822/2822 compliance, <10,000 files per batch, 25MB attachment limit
**Scale/Scope**: Manual uploads up to 10,000 files per batch, unified search with Gmail emails

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Evidence |
|-----------|--------|----------|
| I. Specification-First | ✅ PASS | spec.md complete with 6 user stories, 23 FRs, 10 SCs |
| II. CLI + Library Architecture | ✅ PASS | Adding `ingest` CLI group + `penf_lib.ingest` module |
| III. Test-First Development | ✅ REQUIRED | TDD workflow for all components |
| IV. Integration Testing | ✅ REQUIRED | Contract tests for events, storage, deduplication |
| V. Observability & Debugging | ✅ PASS | Progress bars, structured logging, error summaries |
| VI. Versioning & Breaking Changes | ✅ PASS | New feature, no breaking changes to existing APIs |
| VII. Simplicity & YAGNI | ✅ PASS | Reuses existing Source model, events, AI pipeline |
| VIII. Temporal-First | ✅ PASS | Email Date header used for timeline positioning (FR-104) |
| IX. Research-Driven | ✅ PASS | Python email stdlib for RFC 822, standard encryption |
| X. Local-First AI with Privacy | ✅ PASS | AES-256-GCM encrypted archival, tenant-scoped keys |

**Gate Status**: PASSED - All principles satisfied or have clear compliance path.

## Project Structure

### Documentation (this feature)

```text
specs/012-manual-ingest/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
│   ├── events.md        # Event schemas for ingest
│   └── cli.md           # CLI contract specification
└── tasks.md             # Phase 2 output (via /speckit.tasks)
```

### Source Code (repository root)

```text
penf_lib/
├── cli/
│   ├── main.py              # Add ingest_group registration
│   └── ingest_commands.py   # NEW: ingest CLI commands
├── ingest/                  # NEW: Ingest module
│   ├── __init__.py
│   ├── models.py            # IngestJob, IngestError Pydantic models
│   ├── parser.py            # EML parsing with email stdlib
│   ├── processor.py         # Batch processing, progress tracking
│   ├── deduplication.py     # Message-ID and hash-based dedup
│   └── archiver.py          # AES-256-GCM file archival
├── storage/
│   ├── models.py            # Add IngestJob, EmailAttachment, ArchivedFile SQLAlchemy models
│   └── repositories/
│       └── ingest.py        # NEW: IngestJob repository
├── events/
│   ├── schemas.py           # Add ManualEmailIngestedEvent
│   └── publishers.py        # Add IngestEventPublisher
└── connectors/
    └── email/               # NEW: Standalone email connector (future extensibility)
        └── __init__.py

tests/
├── unit/
│   ├── test_ingest_parser.py
│   ├── test_ingest_processor.py
│   ├── test_ingest_deduplication.py
│   └── test_ingest_archiver.py
├── integration/
│   └── test_ingest_pipeline.py
└── contract/
    └── test_ingest_events.py
```

**Structure Decision**: Extends existing `penf_lib` single-project structure. New `ingest/` module for manual ingest logic, following existing patterns from `automation/`, `review/`, and `search/` modules. CLI commands in `cli/ingest_commands.py` following `gmail_commands.py` pattern.

## Complexity Tracking

> No constitution violations requiring justification. Implementation reuses existing infrastructure.

| Decision | Rationale | Alternative Considered |
|----------|-----------|------------------------|
| Reuse Source table | Unified search, existing AI pipeline integration | New email_messages table - rejected (duplicate search logic) |
| Python email stdlib | RFC 822/2822 compliant, no external dependency | mail-parser library - rejected (unnecessary dependency) |
| AES-256-GCM encryption | Industry standard, built into cryptography lib | Fernet (existing) - rejected (spec requires GCM explicitly) |


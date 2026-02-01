# Implementation Plan: Meeting Series Support

**Branch**: `019-meeting-series` | **Date**: 2026-02-01 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/019-meeting-series/spec.md`

## Summary

Add MeetingSeries entity to group recurring meetings (e.g., "TER Weekly", "TikTok MTC PMO"). Extend the meeting ingest CLI with `--series`, `--title`, and `--date` flags. Add series management commands (`penf meeting series list/create/show/delete`) and meeting-series linking commands (`set-series`, `unset-series`). Enable filtering meetings by series via `penf meeting list --series`.

## Technical Context

**Language/Version**: Go 1.24
**Primary Dependencies**: Cobra (CLI), gRPC, Protocol Buffers, pgx (PostgreSQL)
**Storage**: PostgreSQL 16+ with existing `meetings` table
**Testing**: Go testing with testify, integration tests with test database
**Target Platform**: Linux server (gateway), macOS/Linux (CLI)
**Project Type**: Multi-service (CLI + Gateway + Database)
**Performance Goals**: Sub-second series CRUD operations
**Constraints**: Must not break existing meeting ingest workflow
**Scale/Scope**: Single user, 100s of series, 1000s of meetings

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Specification-First | ✅ PASS | Spec complete with 17 functional requirements |
| II. CLI + Library Architecture | ✅ PASS | Feature adds CLI commands with gateway service backend |
| III. Test-First Development | ⚠️ MUST ENFORCE | All new code requires tests written first |
| IV. Integration Testing | ✅ WILL IMPLEMENT | Contract tests for new gRPC methods |
| V. Observability & Debugging | ✅ PASS | Will use existing structured logging |
| VI. Versioning & Breaking Changes | ✅ PASS | Additive changes only, no breaking changes |
| VII. Simplicity & YAGNI | ✅ PASS | Minimal new entity, extends existing patterns |
| VIII. Temporal-First Data | ✅ PASS | Series include timestamps, meetings already temporal |
| IX. Research-Driven Design | ✅ PASS | Follows existing Penfold patterns |
| X. Local-First AI with Privacy | N/A | No AI processing in this feature |

**Gate Status**: ✅ PASS - May proceed to implementation

## Project Structure

### Documentation (this feature)

```text
specs/019-meeting-series/
├── spec.md              # Feature specification
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output (proto changes)
└── shards.md            # Phase 2 output (/speckit.shards command)
```

### Source Code (repository root)

```text
# Files to modify/create

migrations/
└── 020_meeting_series.sql       # NEW: MeetingSeries table, series_id FK

api/proto/
└── ingest/v1/
    └── ingest.proto             # MODIFY: Add series fields to IngestMeetingRequest
    └── series.proto             # NEW: MeetingSeries CRUD messages

cmd/penf/cmd/
├── ingest_meeting.go            # MODIFY: Add --series, --title, --date flags
├── meeting.go                   # MODIFY: Add series subcommand, list --series filter
├── meeting_series.go            # NEW: series list/create/show/delete commands
├── meeting_set_series.go        # NEW: set-series command
└── meeting_unset_series.go      # NEW: unset-series command

services/gateway/
├── ingestservice/service.go     # MODIFY: Handle series in IngestMeeting
└── seriesservice/               # NEW: Series CRUD service
    ├── service.go
    └── service_test.go

pkg/repository/
└── series_repository.go         # NEW: MeetingSeries data access

tests/
├── integration/
│   └── meeting_series_test.go   # NEW: Integration tests
└── unit/
    └── series_repository_test.go # NEW: Unit tests
```

**Structure Decision**: Extends existing multi-service architecture. New `seriesservice` follows gateway pattern. CLI commands follow existing Cobra patterns in `cmd/penf/cmd/`.

## Complexity Tracking

> No constitution violations. Feature is additive and follows existing patterns.

| Aspect | Approach | Rationale |
|--------|----------|-----------|
| New entity | Single table with FK | Minimal schema change |
| CLI commands | Follows existing patterns | Consistency with `penf meeting` commands |
| Service layer | New gRPC service | Follows gateway architecture |

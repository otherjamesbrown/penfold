# Research: Meeting Series Support

**Feature**: 019-meeting-series
**Date**: 2026-02-01

## Summary

No significant unknowns or clarifications needed. Feature follows established Penfold patterns for database entities, gRPC services, and CLI commands.

## Decisions

### 1. Series ID Format

**Decision**: Use `ms-` prefix with UUID suffix (e.g., `ms-abc123`)
**Rationale**: Follows existing Penfold entity ID conventions (meetings use `mtg-`, sources use UUIDs)
**Alternatives considered**:
- Auto-increment integers - rejected, not portable
- Plain UUIDs - rejected, no type identification

### 2. Series Name Uniqueness

**Decision**: Enforce unique constraint on series name at database level
**Rationale**: Spec requires exact name matching; duplicates would cause confusion
**Alternatives considered**:
- Case-insensitive uniqueness - rejected, adds complexity without clear benefit
- Allow duplicates - rejected, conflicts with auto-create behavior

### 3. Auto-Create Behavior

**Decision**: Create series in gateway service during meeting ingest if not found
**Rationale**: Single transaction ensures atomicity; keeps CLI simple
**Alternatives considered**:
- Create in CLI before calling gateway - rejected, requires two round trips
- Return error if series not found - rejected, poor UX

### 4. Series Deletion Cascade

**Decision**: SET NULL on meetings.series_id when series deleted (orphan meetings)
**Rationale**: Matches spec requirement FR-010; preserves meeting data
**Alternatives considered**:
- CASCADE delete meetings - rejected, data loss risk
- RESTRICT deletion - rejected, blocks cleanup operations

### 5. gRPC Service Organization

**Decision**: Add series operations to existing ingest service rather than new service
**Rationale**: Series are tightly coupled to meeting ingest; fewer service boundaries
**Alternatives considered**:
- New SeriesService - rejected after review, adds unnecessary complexity

## Technology Notes

### Database Migration Pattern

Follow existing migration conventions:
- Sequential numbering (020_meeting_series.sql)
- Include both UP and DOWN migrations
- Use transactions for atomicity

### Proto Message Pattern

Extend existing ingest.proto:
- Add optional `series_name` field to `IngestMeetingRequest`
- Add series CRUD messages in same file
- Regenerate Go code with `make proto`

### CLI Command Pattern

Follow existing cmd/penf/cmd conventions:
- Use `newXCommand()` factory functions
- Register subcommands in parent command init
- Use `pkg/gateway` client for gRPC calls

## Open Questions

None - all requirements are clear from the spec.

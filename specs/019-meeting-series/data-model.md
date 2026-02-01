# Data Model: Meeting Series Support

**Feature**: 019-meeting-series
**Date**: 2026-02-01

## Entities

### MeetingSeries (new)

Represents a recurring meeting series that groups related meetings.

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | TEXT | PRIMARY KEY | Unique ID with `ms-` prefix (e.g., `ms-abc123`) |
| name | TEXT | NOT NULL, UNIQUE | Display name (e.g., "TER Weekly") |
| description | TEXT | NULL | Optional description of the series purpose |
| project_id | UUID | NULL, FK→projects | Optional link to a project |
| created_at | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Creation timestamp |
| updated_at | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Last update timestamp |

**Indexes**:
- `idx_meeting_series_name` on `name` (for exact match lookups)

### Meeting (extended)

Existing `meetings` table, extended with series linkage.

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| ... | ... | ... | (existing fields unchanged) |
| series_id | TEXT | NULL, FK→meeting_series | Optional link to series |

**Indexes**:
- `idx_meetings_series_id` on `series_id` (for filtering by series)

## Relationships

```
┌──────────────────┐       ┌──────────────────┐
│  MeetingSeries   │       │     Meeting      │
├──────────────────┤       ├──────────────────┤
│ id (PK)          │──────<│ series_id (FK)   │
│ name (UNIQUE)    │   0..N│ id (PK)          │
│ description      │       │ title            │
│ project_id       │       │ date             │
│ created_at       │       │ platform         │
│ updated_at       │       │ ...              │
└──────────────────┘       └──────────────────┘
```

## State Transitions

### Series Lifecycle

```
[Created] → [Active] → [Deleted]
              ↓
         (meetings orphaned on delete)
```

### Meeting-Series Linkage

```
Meeting.series_id:
  NULL ─────────────→ series.id    (set-series, ingest --series)
  series.id ────────→ NULL         (unset-series, series deletion)
  series_A.id ──────→ series_B.id  (set-series to different series)
```

## Validation Rules

1. **Series name**: Non-empty, max 255 characters, unique
2. **Series ID**: Must start with `ms-` prefix
3. **Series_id on meeting**: Must reference existing series or be NULL
4. **Description**: Optional, max 1000 characters

## Migration Script

```sql
-- Migration: 020_meeting_series.sql

-- UP
BEGIN;

CREATE TABLE IF NOT EXISTS meeting_series (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    project_id UUID REFERENCES projects(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_meeting_series_name ON meeting_series(name);

ALTER TABLE meetings
ADD COLUMN IF NOT EXISTS series_id TEXT REFERENCES meeting_series(id) ON DELETE SET NULL;

CREATE INDEX idx_meetings_series_id ON meetings(series_id);

-- Trigger to update updated_at
CREATE OR REPLACE FUNCTION update_meeting_series_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_meeting_series_updated_at
    BEFORE UPDATE ON meeting_series
    FOR EACH ROW
    EXECUTE FUNCTION update_meeting_series_updated_at();

COMMIT;

-- DOWN
BEGIN;

DROP TRIGGER IF EXISTS trigger_meeting_series_updated_at ON meeting_series;
DROP FUNCTION IF EXISTS update_meeting_series_updated_at();
DROP INDEX IF EXISTS idx_meetings_series_id;
ALTER TABLE meetings DROP COLUMN IF EXISTS series_id;
DROP INDEX IF EXISTS idx_meeting_series_name;
DROP TABLE IF EXISTS meeting_series;

COMMIT;
```

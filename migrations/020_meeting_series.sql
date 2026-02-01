-- Migration: 020_meeting_series
-- Description: Add meeting_series table and series_id to meetings for recurring meetings
-- Author: Claude Code
-- Date: 2026-02-01

-- UP: Create meeting_series table and add series_id to meetings
BEGIN;

-- Create meeting_series table
CREATE TABLE IF NOT EXISTS meeting_series (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    project_id BIGINT REFERENCES projects(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for meeting_series lookups
CREATE INDEX IF NOT EXISTS idx_meeting_series_name ON meeting_series(name);

-- Add series_id column to meetings table
ALTER TABLE meetings
ADD COLUMN IF NOT EXISTS series_id TEXT REFERENCES meeting_series(id) ON DELETE SET NULL;

-- Index for meetings by series
CREATE INDEX IF NOT EXISTS idx_meetings_series_id ON meetings(series_id);

-- Trigger function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_meeting_series_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger to automatically update updated_at on meeting_series updates
CREATE TRIGGER trigger_meeting_series_updated_at
    BEFORE UPDATE ON meeting_series
    FOR EACH ROW
    EXECUTE FUNCTION update_meeting_series_updated_at();

COMMIT;

-- Comments for documentation
COMMENT ON TABLE meeting_series IS 'Defines meeting series for grouping recurring meetings';
COMMENT ON COLUMN meeting_series.name IS 'Unique name identifying the meeting series';
COMMENT ON COLUMN meeting_series.project_id IS 'Optional project association for the meeting series';
COMMENT ON COLUMN meetings.series_id IS 'References meeting_series to group recurring meetings';

-- DOWN: Rollback all changes
-- BEGIN;
-- DROP TRIGGER IF EXISTS trigger_meeting_series_updated_at ON meeting_series;
-- DROP FUNCTION IF EXISTS update_meeting_series_updated_at();
-- DROP INDEX IF EXISTS idx_meetings_series_id;
-- ALTER TABLE meetings DROP COLUMN IF EXISTS series_id;
-- DROP INDEX IF EXISTS idx_meeting_series_name;
-- DROP TABLE IF EXISTS meeting_series;
-- COMMIT;

-- Migration: 031_watch_list
-- Description: Create watch_list table for tracking user-prioritized assertions and projects
-- Author: Claude Code (data-dev agent)
-- Date: 2026-02-04

-- UP: Create watch_list table
BEGIN;

-- Create watch_list table
CREATE TABLE IF NOT EXISTS watch_list (
    id BIGSERIAL PRIMARY KEY,
    user_id TEXT NOT NULL,               -- single user system but future-proof
    assertion_root_id BIGINT REFERENCES assertions(id) ON DELETE CASCADE,
    project_id BIGINT REFERENCES projects(id) ON DELETE CASCADE,
    notes TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),

    -- At least one of assertion_root_id or project_id must be set
    CONSTRAINT watch_list_has_target CHECK (
        assertion_root_id IS NOT NULL OR project_id IS NOT NULL
    )
);

-- Indexes for watch_list
CREATE INDEX IF NOT EXISTS idx_watch_list_user ON watch_list(user_id);
CREATE INDEX IF NOT EXISTS idx_watch_list_assertion ON watch_list(assertion_root_id) WHERE assertion_root_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_watch_list_project ON watch_list(project_id) WHERE project_id IS NOT NULL;

COMMIT;

-- Comments for documentation
COMMENT ON TABLE watch_list IS 'User-maintained list of high-priority assertions and projects for prioritized briefings';
COMMENT ON COLUMN watch_list.user_id IS 'User who added this item to their watch list';
COMMENT ON COLUMN watch_list.assertion_root_id IS 'Root assertion being watched (null if watching a project)';
COMMENT ON COLUMN watch_list.project_id IS 'Project being watched (null if watching an assertion)';
COMMENT ON COLUMN watch_list.notes IS 'User notes about why this is being watched';

-- DOWN: Rollback all changes
-- BEGIN;
-- DROP INDEX IF EXISTS idx_watch_list_project;
-- DROP INDEX IF EXISTS idx_watch_list_assertion;
-- DROP INDEX IF EXISTS idx_watch_list_user;
-- DROP TABLE IF EXISTS watch_list;
-- COMMIT;

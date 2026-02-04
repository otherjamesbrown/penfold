-- Migration: 028_assertion_lifecycle
-- Description: Add assertion lifecycle tracking and reference relationships
-- Author: Claude Code (data-dev agent)
-- Date: 2026-02-04

-- UP: Add assertion lifecycle fields and references table
BEGIN;

-- Add assertion_root_id to track the original assertion in a lifecycle chain
ALTER TABLE assertions ADD COLUMN IF NOT EXISTS assertion_root_id BIGINT
    REFERENCES assertions(id) ON DELETE SET NULL;

-- Add lifecycle_event to track the current lifecycle state
ALTER TABLE assertions ADD COLUMN IF NOT EXISTS lifecycle_event VARCHAR(50);

COMMENT ON COLUMN assertions.assertion_root_id IS 'Points to the original assertion that started this lifecycle chain';
COMMENT ON COLUMN assertions.lifecycle_event IS 'Current lifecycle state: raised, escalated, assigned, decided, deferred, resolved, reopened';

-- Backfill: all existing assertions are roots of their own lifecycle
UPDATE assertions SET assertion_root_id = id WHERE assertion_root_id IS NULL;

-- Create index for looking up assertion lifecycles
CREATE INDEX IF NOT EXISTS idx_assertions_root_id ON assertions(assertion_root_id) WHERE assertion_root_id IS NOT NULL;

-- Create assertion_references table to track how content relates to assertions
CREATE TABLE IF NOT EXISTS assertion_references (
    id BIGSERIAL PRIMARY KEY,
    assertion_id BIGINT NOT NULL REFERENCES assertions(id) ON DELETE CASCADE,
    assertion_root_id BIGINT NOT NULL REFERENCES assertions(id) ON DELETE CASCADE,
    source_id BIGINT NOT NULL,  -- the content item

    -- What role did this content play?
    reference_type TEXT NOT NULL,
    -- origination:   this is where the risk was first raised
    -- escalation:    this content escalated the risk
    -- decision:      a decision about the risk was made here
    -- discussion:    substantive discussion (> passing mention)
    -- resolution:    the risk was closed/resolved here
    -- mention:       passing reference, no new information

    significance TEXT NOT NULL,
    -- primary:       this content is a key moment in the lifecycle
    -- supporting:    provides additional evidence or context
    -- passing:       mentioned but no material change

    context_excerpt TEXT,       -- the relevant quote from the content
    occurred_at TIMESTAMPTZ,    -- when this reference happened
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Indexes for assertion_references
CREATE INDEX IF NOT EXISTS idx_assertion_refs_root ON assertion_references(assertion_root_id);
CREATE INDEX IF NOT EXISTS idx_assertion_refs_source ON assertion_references(source_id);
CREATE INDEX IF NOT EXISTS idx_assertion_refs_significance ON assertion_references(significance);

COMMIT;

-- Comments for documentation
COMMENT ON TABLE assertion_references IS 'Tracks how content items reference and contribute to assertion lifecycles';
COMMENT ON COLUMN assertion_references.assertion_id IS 'The specific assertion mentioned in the content';
COMMENT ON COLUMN assertion_references.assertion_root_id IS 'The root assertion in the lifecycle chain';
COMMENT ON COLUMN assertion_references.source_id IS 'The content item (email, meeting, document) containing the reference';
COMMENT ON COLUMN assertion_references.reference_type IS 'Role of the reference: origination, escalation, decision, discussion, resolution, mention';
COMMENT ON COLUMN assertion_references.significance IS 'Significance level: primary, supporting, passing';
COMMENT ON COLUMN assertion_references.context_excerpt IS 'Relevant quote from the content';
COMMENT ON COLUMN assertion_references.occurred_at IS 'When the reference occurred (content timestamp)';

-- DOWN: Rollback all changes
-- BEGIN;
-- DROP INDEX IF EXISTS idx_assertion_refs_significance;
-- DROP INDEX IF EXISTS idx_assertion_refs_source;
-- DROP INDEX IF EXISTS idx_assertion_refs_root;
-- DROP TABLE IF EXISTS assertion_references;
-- DROP INDEX IF EXISTS idx_assertions_root_id;
-- ALTER TABLE assertions DROP COLUMN IF EXISTS lifecycle_event;
-- ALTER TABLE assertions DROP COLUMN IF EXISTS assertion_root_id;
-- COMMIT;

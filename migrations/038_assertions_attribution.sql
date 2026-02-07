-- 038_assertions_attribution.sql
-- Adds assertion_attributions table and indexes for cross-content assertion queries.
-- Enables tracking who is attributed to each assertion (owner, assignee, decision_maker, mentioned).
-- Adds indexes to optimize filtering by type, date, and attribution.

BEGIN;

-- 1. Create assertion_attributions table
CREATE TABLE IF NOT EXISTS assertion_attributions (
    assertion_id BIGINT NOT NULL REFERENCES assertions(id) ON DELETE CASCADE,
    entity_id TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('owner', 'assignee', 'decision_maker', 'mentioned')),
    PRIMARY KEY (assertion_id, entity_id)
);

-- 2. Index for querying by entity (e.g., "all assertions attributed to James Brown")
CREATE INDEX IF NOT EXISTS idx_assertion_attributions_entity ON assertion_attributions(entity_id);

-- 3. Index for filtering assertions by type and created date (DESC for recent-first queries)
CREATE INDEX IF NOT EXISTS idx_assertions_type_created ON assertions(assertion_type, created_at DESC);

-- 4. Index for filtering all assertions by created date (DESC for recent-first queries)
CREATE INDEX IF NOT EXISTS idx_assertions_created_at ON assertions(created_at DESC);

COMMIT;

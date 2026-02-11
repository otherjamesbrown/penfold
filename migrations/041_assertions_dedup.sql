-- 041_assertions_dedup.sql
-- Deduplicates existing assertions and adds a unique constraint to prevent future duplicates.
-- Fixes: pf-ab90cb (duplicate assertions on reprocess)
--
-- Root cause: StoreAssertions uses INSERT with ON CONFLICT DO NOTHING but no unique constraint
-- exists, so duplicates are never detected. Example: em-goTacbq8 has "Rishi pushed back" 10 times.
--
-- Solution:
-- 1. Delete duplicate assertions (keep the oldest based on lowest ID)
-- 2. Add unique constraint on (source_id, assertion_type, description)
--
-- Note: description is TEXT, which could exceed btree index limits. We use a partial hash
-- if needed, but for most cases the full column works.

BEGIN;

-- Step 1: Deduplicate existing records
-- Keep the oldest assertion (lowest ID) for each unique (source_id, assertion_type, description) group
-- Use a CTE with ROW_NUMBER() for safe deduplication
WITH duplicates AS (
    SELECT
        id,
        ROW_NUMBER() OVER (
            PARTITION BY source_id, assertion_type, description
            ORDER BY id ASC  -- Keep the oldest (lowest ID)
        ) AS rn
    FROM assertions
)
DELETE FROM assertions
WHERE id IN (
    SELECT id FROM duplicates WHERE rn > 1
);

-- Step 2: Add unique constraint
-- This makes ON CONFLICT DO NOTHING in assertion_repo.go line 115 work correctly
-- The constraint uses (source_id, assertion_type, description) to identify unique assertions
--
-- Note: For TEXT columns, PostgreSQL btree indexes work up to ~2700 bytes.
-- If description exceeds this, the CREATE INDEX will fail. In that case, we'd need
-- to use md5(description) or left(description, 2000). For now, we use the full column
-- since typical assertion descriptions are short (<1000 chars).
CREATE UNIQUE INDEX IF NOT EXISTS idx_assertions_unique_content
    ON assertions (source_id, assertion_type, description);

COMMIT;

-- 048_fix_assertion_dedup_key.sql
-- Fixes assertion deduplication to use source_quote instead of description.
-- Fixes: pf-35906f (Phase 4 of pf-4b82a6: Assertion deduplication on insert)
--
-- Root cause: Migration 041 created unique index on (source_id, assertion_type, description),
-- but the correct dedup key should be (source_id, assertion_type, source_quote).
--
-- Why source_quote and not description?
-- - description is the SPO triple (Subject-Predicate-Object), e.g., "Budget is approved"
-- - source_quote is the context excerpt from the email, e.g., "The budget has been approved by finance"
-- - Two assertions with the same description but different source_quotes are NOT duplicates
--   (same fact mentioned in different parts of the email with different context)
-- - Two assertions with the same source_quote ARE duplicates (same exact context)
--
-- Example bug case (from TestStoreAssertions_SameDescription_DifferentQuote_BugRepro):
--   Assertion 1: description="Budget is approved", source_quote="In the meeting, we discussed the budget..."
--   Assertion 2: description="Budget is approved", source_quote="Just to confirm: the budget is approved..."
--   Current behavior: Assertion 2 rejected as duplicate (wrong!)
--   Expected behavior: Both created (different context quotes)
--
-- Solution:
-- 1. Drop the old unique index on description
-- 2. Deduplicate existing records based on source_quote
-- 3. Add new unique index on (source_id, assertion_type, source_quote)

BEGIN;

-- Step 1: Drop the old incorrect unique index
DROP INDEX IF EXISTS idx_assertions_unique_content;

-- Step 2: Deduplicate existing records based on source_quote
-- Keep the oldest assertion (lowest ID) for each unique (source_id, assertion_type, source_quote) group
WITH duplicates AS (
    SELECT
        id,
        ROW_NUMBER() OVER (
            PARTITION BY source_id, assertion_type, source_quote
            ORDER BY id ASC  -- Keep the oldest (lowest ID)
        ) AS rn
    FROM assertions
    WHERE source_quote IS NOT NULL  -- Skip rows with NULL source_quote
)
DELETE FROM assertions
WHERE id IN (
    SELECT id FROM duplicates WHERE rn > 1
);

-- Step 3: Add new unique index on (source_id, assertion_type, source_quote)
-- This makes ON CONFLICT DO NOTHING in assertion_repo.go work correctly
-- The constraint uses (source_id, assertion_type, source_quote) to identify unique assertions
--
-- Note: source_quote is TEXT, but PostgreSQL btree indexes work up to ~2700 bytes.
-- Typical source_quote values are context excerpts (<500 chars), so full column indexing is safe.
-- If we encounter indexing issues, we can switch to md5(source_quote) or left(source_quote, 2000).
CREATE UNIQUE INDEX IF NOT EXISTS idx_assertions_unique_source_quote
    ON assertions (source_id, assertion_type, source_quote)
    WHERE source_quote IS NOT NULL;  -- Partial index: only index rows with non-NULL source_quote

-- Step 4: Handle NULL source_quote edge case
-- If there are assertions with NULL source_quote, we need a separate constraint
-- to prevent duplicate NULLs. For now, we allow multiple NULLs (partial index excludes them).
-- This is acceptable because:
-- 1. Stage 2 (SLM) always populates source_quote (it's the SourceText field)
-- 2. Stage 4 (DeepAnalyze) always populates source_quote (it's the ContextExcerpt field)
-- 3. If source_quote is NULL, it's likely an incomplete/invalid assertion
-- 4. Allowing duplicate NULLs is safer than blocking them during a reprocess

COMMIT;

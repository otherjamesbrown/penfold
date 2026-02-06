-- 034_assertions_schema_align.sql
-- Aligns the assertions table schema with the code expectations.
-- Renames columns, adds missing columns, updates constraints.
-- Fixes: TestSLMPipeline_FullOutcomeVerification, TestSLMPipeline_HighImportanceRisk, TestSLMPipeline_GoldenThread

BEGIN;

-- 1. Drop constraints that reference columns being renamed
ALTER TABLE assertions DROP CONSTRAINT IF EXISTS ck_assertions_content_not_empty;
ALTER TABLE assertions DROP CONSTRAINT IF EXISTS ck_assertions_content_max_length;
ALTER TABLE assertions DROP CONSTRAINT IF EXISTS ck_assertions_confidence_range;

-- 2. Rename columns to match code expectations
ALTER TABLE assertions RENAME COLUMN content TO description;
ALTER TABLE assertions RENAME COLUMN context TO source_quote;
ALTER TABLE assertions RENAME COLUMN confidence_score TO confidence;

-- 3. Change confidence type from numeric(4,3) to REAL
ALTER TABLE assertions ALTER COLUMN confidence TYPE REAL USING confidence::real;

-- 4. Add missing columns
ALTER TABLE assertions ADD COLUMN IF NOT EXISTS is_current BOOLEAN DEFAULT TRUE;
ALTER TABLE assertions ADD COLUMN IF NOT EXISTS thread_id BIGINT REFERENCES email_threads(id) ON DELETE SET NULL;
ALTER TABLE assertions ADD COLUMN IF NOT EXISTS project_id BIGINT REFERENCES projects(id) ON DELETE SET NULL;
ALTER TABLE assertions ADD COLUMN IF NOT EXISTS owner_person_id BIGINT REFERENCES people(id) ON DELETE SET NULL;
ALTER TABLE assertions ADD COLUMN IF NOT EXISTS assignee_person_id BIGINT REFERENCES people(id) ON DELETE SET NULL;
ALTER TABLE assertions ADD COLUMN IF NOT EXISTS severity VARCHAR(20);
ALTER TABLE assertions ADD COLUMN IF NOT EXISTS status VARCHAR(20) DEFAULT 'open';
ALTER TABLE assertions ADD COLUMN IF NOT EXISTS superseded_by BIGINT REFERENCES assertions(id) ON DELETE SET NULL;

-- 5. Re-create constraints with new column names
ALTER TABLE assertions ADD CONSTRAINT ck_assertions_description_not_empty
    CHECK (length(description) > 0);
ALTER TABLE assertions ADD CONSTRAINT ck_assertions_description_max_length
    CHECK (length(description) <= 10000);
ALTER TABLE assertions ADD CONSTRAINT ck_assertions_confidence_range
    CHECK (confidence IS NULL OR (confidence >= 0.0 AND confidence <= 1.0));

-- 6. Update type CHECK to include 'action' and 'question'
ALTER TABLE assertions DROP CONSTRAINT IF EXISTS ck_assertions_type_valid;
ALTER TABLE assertions ADD CONSTRAINT ck_assertions_type_valid
    CHECK (assertion_type IN ('decision', 'risk', 'commitment', 'milestone', 'outcome', 'dependency', 'assumption', 'issue', 'action', 'question'));

-- 7. Add indexes for new columns
CREATE INDEX IF NOT EXISTS idx_assertions_thread_id ON assertions (thread_id) WHERE thread_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_assertions_project_id ON assertions (project_id) WHERE project_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_assertions_is_current ON assertions (is_current) WHERE is_current = true;
CREATE INDEX IF NOT EXISTS idx_assertions_assignee ON assertions (assignee_person_id) WHERE assignee_person_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_assertions_status ON assertions (status) WHERE status IS NOT NULL;

-- 8. Backfill is_current for existing rows
UPDATE assertions SET is_current = true WHERE is_current IS NULL;

COMMIT;

-- Migration: 030_trust_seniority
-- Description: Add trust and seniority tracking to people, project status, and archive status to products
-- Author: Claude Code (data-dev agent)
-- Date: 2026-02-04

-- UP: Add trust/seniority fields and project status
BEGIN;

-- Add seniority_tier to people (1-7 scale, NULL if unknown)
ALTER TABLE people ADD COLUMN IF NOT EXISTS seniority_tier SMALLINT;

-- Add trust_level to people (0-5 scale, NULL if unset, human-assigned only)
ALTER TABLE people ADD COLUMN IF NOT EXISTS trust_level SMALLINT;

-- Add trust_domains to people (array of domains where trust applies)
ALTER TABLE people ADD COLUMN IF NOT EXISTS trust_domains TEXT[];

COMMENT ON COLUMN people.seniority_tier IS 'Organizational seniority level 1-7 (1=junior, 7=executive), NULL if unknown';
COMMENT ON COLUMN people.trust_level IS 'Human-assigned trust level 0-5 (0=unverified, 5=highest trust), NULL if unset';
COMMENT ON COLUMN people.trust_domains IS 'Domains where trust applies (e.g., technical-risk, timeline, budget)';

-- Create indexes for seniority and trust queries
CREATE INDEX IF NOT EXISTS idx_people_seniority ON people(seniority_tier) WHERE seniority_tier IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_people_trust ON people(trust_level) WHERE trust_level IS NOT NULL;

-- Create project_status enum
DO $$ BEGIN
    CREATE TYPE project_status AS ENUM ('active', 'archived');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Add status to projects table
ALTER TABLE projects ADD COLUMN IF NOT EXISTS status project_status NOT NULL DEFAULT 'active';

COMMENT ON COLUMN projects.status IS 'Project lifecycle status: active, archived';

-- Create index for project status queries
CREATE INDEX IF NOT EXISTS idx_projects_status ON projects(status);

-- Add 'archived' value to product_status enum if it doesn't exist
DO $$ BEGIN
    -- Check if 'archived' already exists in product_status
    IF NOT EXISTS (
        SELECT 1 FROM pg_enum
        WHERE enumlabel = 'archived'
        AND enumtypid = (SELECT oid FROM pg_type WHERE typname = 'product_status')
    ) THEN
        ALTER TYPE product_status ADD VALUE 'archived';
    END IF;
END $$;

COMMIT;

-- DOWN: Rollback all changes
-- BEGIN;
-- -- Note: Cannot remove enum value 'archived' from product_status in PostgreSQL
-- DROP INDEX IF EXISTS idx_projects_status;
-- ALTER TABLE projects DROP COLUMN IF EXISTS status;
-- DROP TYPE IF EXISTS project_status;
-- DROP INDEX IF EXISTS idx_people_trust;
-- DROP INDEX IF EXISTS idx_people_seniority;
-- ALTER TABLE people DROP COLUMN IF EXISTS trust_domains;
-- ALTER TABLE people DROP COLUMN IF EXISTS trust_level;
-- ALTER TABLE people DROP COLUMN IF EXISTS seniority_tier;
-- COMMIT;

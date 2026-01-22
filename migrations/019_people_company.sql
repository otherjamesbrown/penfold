-- Migration: Add company field to people table
-- Purpose: Store company/organization affiliation for people (used in entity seeding)

ALTER TABLE people ADD COLUMN IF NOT EXISTS company VARCHAR(255);

COMMENT ON COLUMN people.company IS 'Company or organization the person is affiliated with';

-- Add index for filtering by company
CREATE INDEX IF NOT EXISTS idx_people_company ON people(tenant_id, company) WHERE company IS NOT NULL;

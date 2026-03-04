-- Migration 112: AD Sync Support
-- Adds columns needed for Azure AD directory sync (Phase 5 of Graph Connector).
--
-- people.manager_email: stores the manager's email for org hierarchy queries
-- teams.source: distinguishes AD-synced teams from manually-created ones
-- teams.external_id: stores the Azure AD group object ID for idempotent sync

-- Add manager_email to people for org hierarchy
ALTER TABLE people ADD COLUMN IF NOT EXISTS manager_email VARCHAR(500);

-- Add source and external_id to teams for AD group sync
ALTER TABLE teams ADD COLUMN IF NOT EXISTS source VARCHAR(50) NOT NULL DEFAULT 'manual';
ALTER TABLE teams ADD COLUMN IF NOT EXISTS external_id VARCHAR(255);

-- Index for looking up teams by external_id (AD group object ID)
CREATE UNIQUE INDEX IF NOT EXISTS idx_teams_tenant_external_id
    ON teams (tenant_id, external_id) WHERE external_id IS NOT NULL;

-- Index for manager lookups
CREATE INDEX IF NOT EXISTS idx_people_manager_email
    ON people (tenant_id, LOWER(manager_email)) WHERE manager_email IS NOT NULL;

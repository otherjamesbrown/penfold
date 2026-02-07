-- 037_entity_lifecycle.sql
-- Adds entity lifecycle management: soft deletion, filter rules, and entity search
-- Enables reject/restore workflow, pattern-based bulk rejection, and lifecycle tracking

BEGIN;

-- 1. Add rejection columns to people table
ALTER TABLE people ADD COLUMN IF NOT EXISTS rejected_at TIMESTAMPTZ;
ALTER TABLE people ADD COLUMN IF NOT EXISTS rejected_reason TEXT;
ALTER TABLE people ADD COLUMN IF NOT EXISTS rejected_by TEXT;

COMMENT ON COLUMN people.rejected_at IS 'Timestamp when entity was rejected (soft deleted)';
COMMENT ON COLUMN people.rejected_reason IS 'Reason for rejection (e.g., "spam account", "duplicate", "test user")';
COMMENT ON COLUMN people.rejected_by IS 'User or service that rejected this entity';

-- Create index for filtering rejected entities
CREATE INDEX IF NOT EXISTS idx_people_rejected_at ON people (rejected_at) WHERE rejected_at IS NOT NULL;

-- 2. Create entity_filter_rules table
CREATE TABLE entity_filter_rules (
    id SERIAL PRIMARY KEY,
    tenant_id VARCHAR(255) NOT NULL,
    email_pattern TEXT,
    name_pattern TEXT,
    entity_type TEXT,
    reason TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT now(),
    created_by TEXT,
    CONSTRAINT at_least_one_pattern CHECK (email_pattern IS NOT NULL OR name_pattern IS NOT NULL)
);

COMMENT ON TABLE entity_filter_rules IS 'Pattern-based rules to automatically reject entity creation during pipeline processing';
COMMENT ON COLUMN entity_filter_rules.email_pattern IS 'SQL LIKE pattern for email matching (e.g., "%@aha.io", "%noreply%")';
COMMENT ON COLUMN entity_filter_rules.name_pattern IS 'SQL LIKE pattern for name matching (e.g., "Bot%", "%Automated%")';
COMMENT ON COLUMN entity_filter_rules.entity_type IS 'Optional entity type filter: person, bot, role, distribution, external_service';
COMMENT ON COLUMN entity_filter_rules.reason IS 'Why this filter rule exists (e.g., "Block known spam domain")';

-- Create indexes for fast pattern matching during entity creation
CREATE INDEX IF NOT EXISTS idx_entity_filter_rules_tenant ON entity_filter_rules(tenant_id);
CREATE INDEX IF NOT EXISTS idx_entity_filter_rules_email_pattern ON entity_filter_rules(email_pattern) WHERE email_pattern IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_entity_filter_rules_name_pattern ON entity_filter_rules(name_pattern) WHERE name_pattern IS NOT NULL;

COMMIT;

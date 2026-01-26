-- Migration 024: Fix Tenant Schema
-- Aligns tenant table with Go code expectations (pe-1ez8)
--
-- Changes:
-- 1. Add slug column (generated from display_name)
-- 2. Drop restrictive CHECK constraint on name column
-- 3. Keep UUID id (matches all existing tenant_id FKs in 44+ tables)

-- =============================================================================
-- ADD SLUG COLUMN
-- =============================================================================

-- Add slug column
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS slug VARCHAR(63);

-- Generate slugs from display_name for existing rows
UPDATE tenants
SET slug = lower(regexp_replace(regexp_replace(display_name, '[^a-zA-Z0-9 -]', '', 'g'), ' +', '-', 'g'))
WHERE slug IS NULL;

-- Make slug NOT NULL and UNIQUE after populating
ALTER TABLE tenants ALTER COLUMN slug SET NOT NULL;

-- Add unique constraint if not exists
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'tenants_slug_key') THEN
        ALTER TABLE tenants ADD CONSTRAINT tenants_slug_key UNIQUE (slug);
    END IF;
END
$$;

-- Add index on slug for fast lookups
CREATE INDEX IF NOT EXISTS idx_tenants_slug ON tenants(slug);

-- =============================================================================
-- RELAX NAME CONSTRAINT
-- =============================================================================

-- Drop the restrictive CHECK constraint that only allows work/personal/family
ALTER TABLE tenants DROP CONSTRAINT IF EXISTS ck_tenant_valid_names;

-- The name column now serves as a category/type field (work/personal/family)
-- and display_name holds the actual tenant name.
-- We keep both for backwards compatibility.

-- =============================================================================
-- COMMENTS
-- =============================================================================

COMMENT ON COLUMN tenants.slug IS 'URL-safe identifier used in paths and CLI references';
COMMENT ON COLUMN tenants.name IS 'Tenant category or type (legacy: work/personal/family)';
COMMENT ON COLUMN tenants.display_name IS 'Human-readable tenant name shown in UI';

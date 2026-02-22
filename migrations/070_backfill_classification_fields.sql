-- Migration 070: Backfill existing items with classification fields
-- Wave 4 of content classification epic (pf-778226).
-- Shard: pf-5d227d
--
-- Prerequisites (all deployed):
--   066: Added 'meeting' to content_type enum, content_structure column
--   064/065: Classification rules engine and seed data
--   067/068: Newsletter rules and routing table
--
-- This migration backfills existing content_enrichment rows with correct
-- classification values derived from sources table metadata.
-- Note: penf db migrate wraps each migration in a transaction automatically.

-- ============================================================
-- Step 1: Fix Teams meetings → content_type = 'meeting'
-- Teams meeting transcripts have source_system = 'teams' in the
-- sources table but were originally classified as 'email' or 'document'.
-- ============================================================
UPDATE content_enrichment ce
SET content_type = 'meeting',
    updated_at = NOW()
FROM sources s
WHERE ce.source_id = s.id
  AND s.source_system = 'teams'
  AND ce.content_type != 'meeting';

-- ============================================================
-- Step 2: Fix calendar items → content_type = 'calendar'
-- Items with calendar subtypes may have content_type = 'email'
-- from initial classification. Correct to 'calendar'.
-- ============================================================
UPDATE content_enrichment
SET content_type = 'calendar',
    updated_at = NOW()
WHERE content_subtype IN ('invite', 'cancellation', 'update', 'response')
  AND content_type != 'calendar';

-- ============================================================
-- Step 3: Backfill content_structure from ingestion_metadata
-- Classify email structure based on subject prefix and in_reply_to header.
-- Order matters: forward first, then reply, then standalone (default).
-- ============================================================

-- 3a: Forwards (subject starts with FW: or Fwd:)
UPDATE content_enrichment ce
SET content_structure = 'forward',
    updated_at = NOW()
FROM sources s
WHERE ce.source_id = s.id
  AND ce.content_structure IS NULL
  AND ce.content_type = 'email'
  AND (
    LOWER(TRIM(s.ingestion_metadata->>'subject')) LIKE 'fw:%'
    OR LOWER(TRIM(s.ingestion_metadata->>'subject')) LIKE 'fwd:%'
  );

-- 3b: Replies (has in_reply_to header or subject starts with Re:)
UPDATE content_enrichment ce
SET content_structure = 'reply',
    updated_at = NOW()
FROM sources s
WHERE ce.source_id = s.id
  AND ce.content_structure IS NULL
  AND ce.content_type = 'email'
  AND (
    COALESCE(TRIM(s.ingestion_metadata->>'in_reply_to'), '') != ''
    OR LOWER(TRIM(s.ingestion_metadata->>'subject')) LIKE 're:%'
  );

-- 3c: Standalone (remaining emails with no structure classification)
UPDATE content_enrichment
SET content_structure = 'standalone',
    updated_at = NOW()
WHERE content_structure IS NULL
  AND content_type = 'email';

-- ============================================================
-- Step 4: Update triage_category from OTHER to FYI
-- The 'OTHER' category was a catch-all that has been renamed to
-- 'FYI' in the new taxonomy. Update in sources.ingestion_metadata.
-- ============================================================
UPDATE sources
SET ingestion_metadata = jsonb_set(
    COALESCE(ingestion_metadata, '{}'::jsonb),
    '{triage_category}',
    '"FYI"'
),
    updated_at = NOW()
WHERE ingestion_metadata->>'triage_category' = 'OTHER';

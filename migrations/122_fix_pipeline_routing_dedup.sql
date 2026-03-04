-- Migration 122: Fix pipeline_routing MEETING/TRANSCRIPT data and deduplicate rows.
-- Author: agent-mycroft
-- Date: 2026-03-04
-- Shard: pf-87a81a
--
-- Problem 1: MEETING/TRANSCRIPT rows have pipeline='standard' instead of 'transcript'.
-- Migration 075 attempted this fix but migration 093 re-seeded per-tenant rows with
-- the wrong value, undoing the earlier correction.
--
-- Problem 2: Migration 093 created per-tenant duplicates without a dedup guard,
-- resulting in ~4,287 rows when there should be ~9 per tenant.
--
-- Note: Migration 075 (migrations/075_fix_meeting_transcript_route.sql) has goose
-- tags. The migration runner at pkg/db/migrations.go (line 271-273) strips the
-- goose Down section automatically, so those tags are harmless but misleading.
-- Old migrations are not modified.

BEGIN;

-- Step 1: Fix MEETING/TRANSCRIPT routing.
-- All rows with this type/subtype combination should route to the 'transcript'
-- pipeline, not 'standard'. The transcript pipeline uses parse_transcript as its
-- first stage, which is designed specifically for meeting transcripts.
UPDATE pipeline_routing
SET pipeline = 'transcript'
WHERE content_type = 'MEETING'
  AND content_subtype = 'TRANSCRIPT'
  AND pipeline = 'standard';

-- Step 2: Deduplicate pipeline_routing rows.
-- Keep the lowest ID per (tenant_id, content_type, content_subtype, pipeline)
-- combination and delete all higher-ID duplicates.
DELETE FROM pipeline_routing a
USING pipeline_routing b
WHERE a.id > b.id
  AND a.tenant_id = b.tenant_id
  AND a.content_type = b.content_type
  AND a.content_subtype = b.content_subtype
  AND a.pipeline = b.pipeline;

-- Step 3: Add unique constraint to prevent future duplication.
ALTER TABLE pipeline_routing
  ADD CONSTRAINT uq_pipeline_routing_tenant_type_subtype_pipeline
  UNIQUE (tenant_id, content_type, content_subtype, pipeline);

COMMIT;

-- Migration 080: Fix transcript pipeline definition
-- Author: agent-mycroft
-- Date: 2026-02-24
-- Shard: pf-1e4d4d
--
-- Fixes three problems in the transcript pipeline definition:
-- 1. skip_when_low=true on extraction stages (transcripts should never skip)
-- 2. Missing summary and segment stages
-- 3. Timeouts too short (NER 120s overrides code-level 10min fix)
--
-- Also adds summary stage to standard pipeline.

BEGIN;

-- ============================================================================
-- 1. TRANSCRIPT PIPELINE: Fix skip_when_low and timeouts on existing stages
-- ============================================================================

-- Set skip_when_low=false on ALL transcript stages (transcripts never skip)
UPDATE pipeline_definitions
SET skip_when_low = false
WHERE pipeline = 'transcript'
  AND skip_when_low = true;

-- Fix timeouts: NER=600s (match llmOpts 10min), assertions/semantic=300s,
-- resolve=120s, analyze=300s
UPDATE pipeline_definitions
SET timeout_seconds = 120
WHERE pipeline = 'transcript' AND stage = 'triage';

UPDATE pipeline_definitions
SET timeout_seconds = 600
WHERE pipeline = 'transcript' AND stage = 'extract_ner';

UPDATE pipeline_definitions
SET timeout_seconds = 300
WHERE pipeline = 'transcript' AND stage = 'extract_assertions';

UPDATE pipeline_definitions
SET timeout_seconds = 300
WHERE pipeline = 'transcript' AND stage = 'extract_semantic';

UPDATE pipeline_definitions
SET timeout_seconds = 120
WHERE pipeline = 'transcript' AND stage = 'resolve';

UPDATE pipeline_definitions
SET timeout_seconds = 300
WHERE pipeline = 'transcript' AND stage = 'analyze';

-- Remove optional flag from analyze (transcript pipeline runs full extraction)
UPDATE pipeline_definitions
SET optional = false
WHERE pipeline = 'transcript' AND stage = 'analyze';

-- ============================================================================
-- 2. TRANSCRIPT PIPELINE: Reorder stages and insert segment + summary
-- ============================================================================

-- Current order:
--   0: parse_transcript, 1: triage, 2: extract_ner, 3: extract_assertions,
--   4: extract_semantic, 5: resolve, 6: analyze, 7: persist, 8: embed
--
-- Target order:
--   0: parse_transcript, 1: segment, 2: triage, 3: extract_ner,
--   4: extract_assertions, 5: extract_semantic, 6: resolve, 7: analyze,
--   8: summary, 9: persist, 10: embed

-- Shift existing stages to make room. Work from highest to lowest to avoid
-- unique constraint conflicts on (tenant_id, pipeline, stage).

-- persist: 7 → 9, embed: 8 → 10
UPDATE pipeline_definitions
SET stage_order = 10
WHERE pipeline = 'transcript' AND stage = 'embed';

UPDATE pipeline_definitions
SET stage_order = 9
WHERE pipeline = 'transcript' AND stage = 'persist';

-- analyze: 6 → 7
UPDATE pipeline_definitions
SET stage_order = 7
WHERE pipeline = 'transcript' AND stage = 'analyze';

-- resolve: 5 → 6
UPDATE pipeline_definitions
SET stage_order = 6
WHERE pipeline = 'transcript' AND stage = 'resolve';

-- extract_semantic: 4 → 5
UPDATE pipeline_definitions
SET stage_order = 5
WHERE pipeline = 'transcript' AND stage = 'extract_semantic';

-- extract_assertions: 3 → 4
UPDATE pipeline_definitions
SET stage_order = 4
WHERE pipeline = 'transcript' AND stage = 'extract_assertions';

-- extract_ner: 2 → 3
UPDATE pipeline_definitions
SET stage_order = 3
WHERE pipeline = 'transcript' AND stage = 'extract_ner';

-- triage: 1 → 2
UPDATE pipeline_definitions
SET stage_order = 2
WHERE pipeline = 'transcript' AND stage = 'triage';

-- Insert segment at position 1 (enabled=false — no Temporal activity yet)
DO $$
DECLARE
  t_id UUID;
BEGIN
  SELECT id INTO t_id FROM tenants LIMIT 1;
  IF t_id IS NULL THEN
    RAISE EXCEPTION 'No tenant found';
  END IF;

  INSERT INTO pipeline_definitions (tenant_id, pipeline, stage, stage_order, enabled, skip_when_low, optional, timeout_seconds, model_override)
  VALUES (t_id, 'transcript', 'segment', 1, false, false, false, 120, 'gemini-2.5-flash')
  ON CONFLICT (tenant_id, pipeline, stage) DO UPDATE
    SET stage_order = 1, enabled = false, skip_when_low = false, timeout_seconds = 120, model_override = 'gemini-2.5-flash';

  -- Insert summary at position 8 (after analyze=7, before persist=9)
  INSERT INTO pipeline_definitions (tenant_id, pipeline, stage, stage_order, enabled, skip_when_low, optional, timeout_seconds, model_override)
  VALUES (t_id, 'transcript', 'summary', 8, true, false, false, 300, 'gemini-2.5-pro')
  ON CONFLICT (tenant_id, pipeline, stage) DO UPDATE
    SET stage_order = 8, enabled = true, skip_when_low = false, timeout_seconds = 300, model_override = 'gemini-2.5-pro';

  -- ============================================================================
  -- 3. STANDARD PIPELINE: Add summary stage
  -- ============================================================================

  -- Current order:
  --   0: parse, 1: triage, 2: extract_ner, 3: extract_assertions,
  --   4: extract_semantic, 5: resolve, 6: analyze, 7: persist, 8: embed
  --
  -- Target: insert summary after analyze (6), before persist.
  -- Shift persist and embed up by 1.

  UPDATE pipeline_definitions
  SET stage_order = 9
  WHERE pipeline = 'standard' AND stage = 'embed';

  UPDATE pipeline_definitions
  SET stage_order = 8
  WHERE pipeline = 'standard' AND stage = 'persist';

  INSERT INTO pipeline_definitions (tenant_id, pipeline, stage, stage_order, enabled, skip_when_low, optional, timeout_seconds)
  VALUES (t_id, 'standard', 'summary', 7, true, true, false, 120)
  ON CONFLICT (tenant_id, pipeline, stage) DO UPDATE
    SET stage_order = 7, enabled = true, skip_when_low = true, timeout_seconds = 120;

END $$;

COMMIT;

-- Rollback (manual):
-- UPDATE pipeline_definitions SET skip_when_low = true WHERE pipeline = 'transcript' AND stage IN ('extract_ner', 'extract_assertions', 'extract_semantic', 'resolve', 'analyze', 'persist');
-- DELETE FROM pipeline_definitions WHERE pipeline = 'transcript' AND stage IN ('segment', 'summary');
-- DELETE FROM pipeline_definitions WHERE pipeline = 'standard' AND stage = 'summary';
-- Revert stage_order and timeout values to migration 074 values.

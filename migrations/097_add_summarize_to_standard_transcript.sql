-- Migration 097: Add summarize stage to standard and transcript pipelines
-- Author: agent-mycroft
-- Date: 2026-03-01
-- Shard: pf-c42209 (Pipeline/Summarize - wrong gating function)
--
-- The summarize stage was running on standard/transcript pipelines only because
-- the gating function (isStageEnabled) defaulted to true for stages not in the
-- pipeline definition. With the fix to use stageInPipeline (which defaults to
-- false for missing stages), summarize would stop running unless explicitly added.
--
-- Changes:
-- 1. Renumber standard pipeline stages to 10-spaced values
-- 2. Insert summarize at stage_order 15 (between triage=10 and extract_ner=20)
-- 3. Renumber transcript pipeline stages to 10-spaced values
-- 4. Insert summarize at stage_order 15 (between triage=10 and extract_ner=20)

BEGIN;

DO $$
DECLARE
  t_id UUID;
  t_rec RECORD;
BEGIN
  FOR t_rec IN SELECT id FROM tenants LOOP
    t_id := t_rec.id;

    -- Standard pipeline: renumber to 10-spaced values
    UPDATE pipeline_definitions SET stage_order = 10
    WHERE tenant_id = t_id AND pipeline = 'standard' AND stage = 'parse';

    UPDATE pipeline_definitions SET stage_order = 20
    WHERE tenant_id = t_id AND pipeline = 'standard' AND stage = 'triage';

    UPDATE pipeline_definitions SET stage_order = 30
    WHERE tenant_id = t_id AND pipeline = 'standard' AND stage = 'extract_ner';

    UPDATE pipeline_definitions SET stage_order = 40
    WHERE tenant_id = t_id AND pipeline = 'standard' AND stage = 'extract_assertions';

    UPDATE pipeline_definitions SET stage_order = 50
    WHERE tenant_id = t_id AND pipeline = 'standard' AND stage = 'extract_semantic';

    UPDATE pipeline_definitions SET stage_order = 60
    WHERE tenant_id = t_id AND pipeline = 'standard' AND stage = 'resolve';

    UPDATE pipeline_definitions SET stage_order = 70
    WHERE tenant_id = t_id AND pipeline = 'standard' AND stage = 'analyze';

    UPDATE pipeline_definitions SET stage_order = 80
    WHERE tenant_id = t_id AND pipeline = 'standard' AND stage = 'persist';

    UPDATE pipeline_definitions SET stage_order = 90
    WHERE tenant_id = t_id AND pipeline = 'standard' AND stage = 'embed';

    -- Standard pipeline: insert summarize between triage (20) and extract_ner (30)
    INSERT INTO pipeline_definitions (tenant_id, pipeline, stage, stage_order, enabled, skip_when_low, optional, timeout_seconds)
    VALUES (t_id, 'standard', 'summarize', 25, true, true, true, 60)
    ON CONFLICT (tenant_id, pipeline, stage) DO NOTHING;

    -- Transcript pipeline: renumber to 10-spaced values
    UPDATE pipeline_definitions SET stage_order = 10
    WHERE tenant_id = t_id AND pipeline = 'transcript' AND stage = 'parse_transcript';

    UPDATE pipeline_definitions SET stage_order = 20
    WHERE tenant_id = t_id AND pipeline = 'transcript' AND stage = 'triage';

    UPDATE pipeline_definitions SET stage_order = 30
    WHERE tenant_id = t_id AND pipeline = 'transcript' AND stage = 'extract_ner';

    UPDATE pipeline_definitions SET stage_order = 40
    WHERE tenant_id = t_id AND pipeline = 'transcript' AND stage = 'extract_assertions';

    UPDATE pipeline_definitions SET stage_order = 50
    WHERE tenant_id = t_id AND pipeline = 'transcript' AND stage = 'extract_semantic';

    UPDATE pipeline_definitions SET stage_order = 60
    WHERE tenant_id = t_id AND pipeline = 'transcript' AND stage = 'resolve';

    UPDATE pipeline_definitions SET stage_order = 70
    WHERE tenant_id = t_id AND pipeline = 'transcript' AND stage = 'analyze';

    UPDATE pipeline_definitions SET stage_order = 80
    WHERE tenant_id = t_id AND pipeline = 'transcript' AND stage = 'persist';

    UPDATE pipeline_definitions SET stage_order = 90
    WHERE tenant_id = t_id AND pipeline = 'transcript' AND stage = 'embed';

    -- Transcript pipeline: insert summarize between triage (20) and extract_ner (30)
    INSERT INTO pipeline_definitions (tenant_id, pipeline, stage, stage_order, enabled, skip_when_low, optional, timeout_seconds)
    VALUES (t_id, 'transcript', 'summarize', 25, true, true, true, 60)
    ON CONFLICT (tenant_id, pipeline, stage) DO NOTHING;

  END LOOP;
END $$;

COMMIT;

-- Rollback (manual):
-- DELETE FROM pipeline_definitions WHERE stage = 'summarize' AND pipeline IN ('standard', 'transcript');
-- Then restore original stage_order values from migration 074.

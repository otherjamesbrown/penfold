-- Migration 094: Fix pipeline stage definitions to match actual execution
-- Author: agent-mycroft
-- Date: 2026-02-28
-- Shard: pf-83a646 (Pipeline/Metadata - stage lists don't match execution)
--
-- The notification and auto_reply pipeline definitions are missing stages that
-- actually execute. This causes Langfuse trace metadata (pipeline_stages) to
-- not match the observed generation spans, breaking monitoring and cost estimation.
--
-- Changes:
-- 1. Add 'summarize' stage to notification pipeline (runs between triage and extract)
-- 2. Add 'extract_semantic' stage to notification pipeline (runs alongside extract_ner)
-- 3. Add 'summarize' stage to auto_reply pipeline (gated by contribution, but part of pipeline)

BEGIN;

DO $$
DECLARE
  t_id UUID;
  t_rec RECORD;
BEGIN
  FOR t_rec IN SELECT id FROM tenants LOOP
    t_id := t_rec.id;

    -- notification pipeline: add summarize (stage_order 1.5 → use 15 to sort between triage=1 and extract_ner=2)
    -- Renumber: triage=10, summarize=15, extract_ner=20, extract_semantic=25, persist=30, embed=40
    UPDATE pipeline_definitions SET stage_order = 10
    WHERE tenant_id = t_id AND pipeline = 'notification' AND stage = 'triage';

    UPDATE pipeline_definitions SET stage_order = 30
    WHERE tenant_id = t_id AND pipeline = 'notification' AND stage = 'extract_ner';

    UPDATE pipeline_definitions SET stage_order = 50
    WHERE tenant_id = t_id AND pipeline = 'notification' AND stage = 'persist';

    UPDATE pipeline_definitions SET stage_order = 60
    WHERE tenant_id = t_id AND pipeline = 'notification' AND stage = 'embed';

    INSERT INTO pipeline_definitions (tenant_id, pipeline, stage, stage_order, enabled, skip_when_low, optional, timeout_seconds)
    VALUES (t_id, 'notification', 'summarize', 20, true, true, true, 60)
    ON CONFLICT (tenant_id, pipeline, stage) DO NOTHING;

    INSERT INTO pipeline_definitions (tenant_id, pipeline, stage, stage_order, enabled, skip_when_low, optional, timeout_seconds, model_override)
    VALUES (t_id, 'notification', 'extract_semantic', 40, true, false, false, 120, 'gemini-2.5-flash')
    ON CONFLICT (tenant_id, pipeline, stage) DO NOTHING;

    -- auto_reply pipeline: add summarize (between triage and embed)
    -- Renumber: triage=10, summarize=20, embed=30
    UPDATE pipeline_definitions SET stage_order = 10
    WHERE tenant_id = t_id AND pipeline = 'auto_reply' AND stage = 'triage';

    UPDATE pipeline_definitions SET stage_order = 30
    WHERE tenant_id = t_id AND pipeline = 'auto_reply' AND stage = 'embed';

    INSERT INTO pipeline_definitions (tenant_id, pipeline, stage, stage_order, enabled, skip_when_low, optional, timeout_seconds)
    VALUES (t_id, 'auto_reply', 'summarize', 20, true, true, true, 60)
    ON CONFLICT (tenant_id, pipeline, stage) DO NOTHING;

  END LOOP;
END $$;

COMMIT;

-- Rollback (manual):
-- DELETE FROM pipeline_definitions WHERE stage = 'summarize' AND pipeline IN ('notification', 'auto_reply');
-- DELETE FROM pipeline_definitions WHERE stage = 'extract_semantic' AND pipeline = 'notification';
-- Then restore original stage_order values from migration 093.

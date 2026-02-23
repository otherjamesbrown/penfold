-- Migration 074: Pipeline definitions table
-- Author: agent-mycroft
-- Date: 2026-02-23
-- Shard: pf-448da3
--
-- Defines which stages each named pipeline runs, in order.
-- Replaces the hardcoded SLMPipelineStages Go slice as the source of truth
-- for pipeline stage composition.

BEGIN;

CREATE TABLE IF NOT EXISTS pipeline_definitions (
    id SERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    pipeline TEXT NOT NULL,
    stage TEXT NOT NULL,
    stage_order INT NOT NULL,
    enabled BOOLEAN DEFAULT true,
    model_override TEXT,
    prompt_override INT,
    skip_when_low BOOLEAN DEFAULT false,
    optional BOOLEAN DEFAULT false,
    timeout_seconds INT DEFAULT 120,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(tenant_id, pipeline, stage)
);

CREATE INDEX IF NOT EXISTS idx_pipeline_definitions_tenant_pipeline
    ON pipeline_definitions(tenant_id, pipeline, stage_order);

COMMENT ON TABLE pipeline_definitions IS 'Defines ordered stages for each named pipeline. Source of truth for which stages run and in what order.';
COMMENT ON COLUMN pipeline_definitions.pipeline IS 'Pipeline name (e.g., standard, transcript, attendees_only)';
COMMENT ON COLUMN pipeline_definitions.stage IS 'Stage identifier matching pipeline_stages registry and pipeline_runs.stage';
COMMENT ON COLUMN pipeline_definitions.stage_order IS 'Execution order within the pipeline (0-based)';
COMMENT ON COLUMN pipeline_definitions.skip_when_low IS 'Skip this stage when triage importance is LOW';
COMMENT ON COLUMN pipeline_definitions.optional IS 'If true, stage failure does not block the pipeline';
COMMENT ON COLUMN pipeline_definitions.model_override IS 'Override model for this stage (NULL = use stage_model_config default)';
COMMENT ON COLUMN pipeline_definitions.prompt_override IS 'Override prompt version for this stage (NULL = use active version)';
COMMENT ON COLUMN pipeline_definitions.timeout_seconds IS 'Activity timeout for this stage';

-- Seed pipeline definitions for the first tenant.
DO $$
DECLARE
  t_id UUID;
BEGIN
  SELECT id INTO t_id FROM tenants LIMIT 1;
  IF t_id IS NULL THEN
    RAISE EXCEPTION 'No tenant found for seeding pipeline definitions';
  END IF;

  -- Standard pipeline (9 stages) — matches current SLMPipelineStages behavior.
  -- Stage names match pipeline_stages registry and pipeline_runs.stage values.
  INSERT INTO pipeline_definitions (tenant_id, pipeline, stage, stage_order, enabled, skip_when_low, optional, timeout_seconds) VALUES
    (t_id, 'standard', 'parse',               0, true, false, false,  30),
    (t_id, 'standard', 'triage',              1, true, false, false,  60),
    (t_id, 'standard', 'extract_ner',         2, true, true,  false, 120),
    (t_id, 'standard', 'extract_assertions',  3, true, true,  false, 120),
    (t_id, 'standard', 'extract_semantic',    4, true, true,  false, 120),
    (t_id, 'standard', 'resolve',             5, true, true,  false,  60),
    (t_id, 'standard', 'analyze',             6, true, true,  true,  180),
    (t_id, 'standard', 'persist',             7, true, true,  false,  60),
    (t_id, 'standard', 'embed',              8, true, false, false,  60)
  ON CONFLICT (tenant_id, pipeline, stage) DO NOTHING;

  -- Transcript pipeline (9 stages) — for meeting transcripts.
  -- Uses parse_transcript instead of parse; otherwise same as standard.
  INSERT INTO pipeline_definitions (tenant_id, pipeline, stage, stage_order, enabled, skip_when_low, optional, timeout_seconds) VALUES
    (t_id, 'transcript', 'parse_transcript',   0, true, false, false,  30),
    (t_id, 'transcript', 'triage',             1, true, false, false,  60),
    (t_id, 'transcript', 'extract_ner',        2, true, true,  false, 120),
    (t_id, 'transcript', 'extract_assertions', 3, true, true,  false, 120),
    (t_id, 'transcript', 'extract_semantic',   4, true, true,  false, 120),
    (t_id, 'transcript', 'resolve',            5, true, true,  false,  60),
    (t_id, 'transcript', 'analyze',            6, true, true,  true,  180),
    (t_id, 'transcript', 'persist',            7, true, true,  false,  60),
    (t_id, 'transcript', 'embed',             8, true, false, false,  60)
  ON CONFLICT (tenant_id, pipeline, stage) DO NOTHING;

  -- Attendees-only pipeline (4 stages) — minimal for calendar invites.
  -- Parse + triage + entity extraction + embed. No deep analysis.
  INSERT INTO pipeline_definitions (tenant_id, pipeline, stage, stage_order, enabled, skip_when_low, optional, timeout_seconds) VALUES
    (t_id, 'attendees_only', 'parse',       0, true, false, false,  30),
    (t_id, 'attendees_only', 'triage',      1, true, false, false,  60),
    (t_id, 'attendees_only', 'extract_ner', 2, true, false, false, 120),
    (t_id, 'attendees_only', 'embed',       3, true, false, false,  60)
  ON CONFLICT (tenant_id, pipeline, stage) DO NOTHING;

END $$;

COMMIT;

-- Rollback (manual):
-- DROP TABLE IF EXISTS pipeline_definitions;

-- Migration 123: Add 'summarize' to pipeline_stages registry
-- Author: agent-mycroft (worker-dev)
-- Date: 2026-03-05
-- Shard: pf-e05c78
--
-- Root cause:
--   Migration 069 registered the stage as 'summary' in pipeline_stages.
--   Migrations 094, 097, 099 and pipeline.go all reference the stage as 'summarize'
--   (matching the code at GenerateContentSummary / stageInPipeline("summarize")).
--   This means pipeline_definitions.stage='summarize' and pipeline_runs.stage='summarize'
--   both violate the FK constraint to pipeline_stages(stage), causing silent failures
--   in RecordSkippedStage and broken pipeline definitions.
--
-- Fix:
--   Insert 'summarize' into pipeline_stages (if not exists).
--   The existing 'summary' row is retained — nothing currently references it from
--   pipeline_definitions or pipeline_runs so it is dead config.
--   Cleanup of 'summary' is deferred to a separate migration after audit.
--
-- Audit note on 'summary':
--   prompt_templates references stage='summary' (migrations 069, 098).
--   pipeline_definitions and pipeline_runs use stage='summarize'.
--   The prompt lookup in AI coordinator must be verified to use 'summarize'.
--   Full rename of prompt_templates stage='summary' → 'summarize' is left for
--   a follow-up migration once AI coordinator prompt resolution is confirmed.

BEGIN;

INSERT INTO pipeline_stages (stage, display_name, description, stage_type, model_dependent, has_prompt, depends_on, downstream)
VALUES (
    'summarize',
    'Stage 1.5: Summarize',
    'Generate a concise summary of content using LLM. Runs after triage, before extraction. Non-blocking.',
    'llm',
    true,
    true,
    '{triage}',
    '{extract_ner,extract_semantic}'
)
ON CONFLICT (stage) DO NOTHING;

COMMIT;

-- Rollback (manual):
-- DELETE FROM pipeline_stages WHERE stage = 'summarize';
-- Note: only safe if no pipeline_definitions or pipeline_runs rows reference 'summarize'.

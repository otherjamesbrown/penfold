-- Migration: 073_seed_stage_model_config
-- Description: Seed model_config with stage-specific model overrides for stages
--              that need gemini-2.0-flash instead of the default qwen2.5:7b.
--              extract_assertions and conversation_summarize require judgment
--              (dedup, confidence scoring, state assessment) which qwen struggles with.
-- Author: agent-mycroft
-- Date: 2026-02-23
-- Shard: pf-60b1f3

BEGIN;

-- extract_assertions: dedup judgment, confidence scoring, SPO triples
-- Qwen produces duplicates and 0.00 confidence scores on this stage.
INSERT INTO model_config (tenant_id, config_key, model_name, updated_by)
VALUES ('c3170310-78bd-409c-b186-126f40bfa6ad', 'stage.extract_assertions', 'gemini-2.0-flash', 'system')
ON CONFLICT (tenant_id, config_key) DO UPDATE SET model_name = EXCLUDED.model_name;

-- conversation_summarize: rolling summary with state assessment needs judgment
-- about conversation trajectory, not just extraction.
INSERT INTO model_config (tenant_id, config_key, model_name, updated_by)
VALUES ('c3170310-78bd-409c-b186-126f40bfa6ad', 'stage.conversation_summarize', 'gemini-2.0-flash', 'system')
ON CONFLICT (tenant_id, config_key) DO UPDATE SET model_name = EXCLUDED.model_name;

COMMIT;

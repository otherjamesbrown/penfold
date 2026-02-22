-- Migration: 071_seed_model_config
-- Description: Seed model_config with stage-specific model overrides for stages
--              that don't use the default qwen2.5:7b model.
-- Author: agent-mycroft
-- Date: 2026-02-22

BEGIN;

-- deep_analyze uses gemini-2.5-pro (cloud LLM for complex reasoning)
INSERT INTO model_config (tenant_id, config_key, model_name, updated_by)
VALUES ('c3170310-78bd-409c-b186-126f40bfa6ad', 'stage.deep_analyze', 'gemini-2.5-pro', 'system')
ON CONFLICT (tenant_id, config_key) DO UPDATE SET model_name = EXCLUDED.model_name;

-- embedding uses mxbai-embed-large (local MLX embedding model)
INSERT INTO model_config (tenant_id, config_key, model_name, updated_by)
VALUES ('c3170310-78bd-409c-b186-126f40bfa6ad', 'stage.embedding', 'mxbai-embed-large', 'system')
ON CONFLICT (tenant_id, config_key) DO UPDATE SET model_name = EXCLUDED.model_name;

COMMIT;

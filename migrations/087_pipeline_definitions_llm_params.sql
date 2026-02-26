BEGIN;

ALTER TABLE pipeline_definitions
    ADD COLUMN IF NOT EXISTS temperature DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS max_tokens INTEGER,
    ADD COLUMN IF NOT EXISTS max_retries INTEGER;

COMMENT ON COLUMN pipeline_definitions.temperature IS 'LLM temperature for this stage (NULL = use handler default)';
COMMENT ON COLUMN pipeline_definitions.max_tokens IS 'LLM max output tokens for this stage (NULL = use handler default)';
COMMENT ON COLUMN pipeline_definitions.max_retries IS 'Max parse-retry attempts for this stage (NULL = use handler default)';

-- Seed values matching current hardcoded params (all tenants)
UPDATE pipeline_definitions SET temperature = 0.1, max_tokens = 8192, max_retries = 2
WHERE stage = 'extract_ner' AND temperature IS NULL;

UPDATE pipeline_definitions SET temperature = 0.1, max_tokens = 8192, max_retries = 2
WHERE stage = 'extract_semantic' AND temperature IS NULL;

UPDATE pipeline_definitions SET temperature = 0.2, max_tokens = 8192, max_retries = 2
WHERE stage = 'analyze' AND temperature IS NULL;

-- quality_gate_risk: virtual stage, not in pipeline stage_order.
-- Insert for every tenant that has a 'standard' pipeline but no quality_gate_risk row.
INSERT INTO pipeline_definitions (tenant_id, pipeline, stage, stage_order, enabled, temperature, max_tokens, max_retries)
SELECT DISTINCT tenant_id, 'standard', 'quality_gate_risk', -1, true, 0.1, 8192, 2
FROM pipeline_definitions
WHERE pipeline = 'standard'
  AND NOT EXISTS (
    SELECT 1 FROM pipeline_definitions pd2
    WHERE pd2.tenant_id = pipeline_definitions.tenant_id
      AND pd2.pipeline = 'standard'
      AND pd2.stage = 'quality_gate_risk'
  )
ON CONFLICT (tenant_id, pipeline, stage) DO UPDATE
  SET temperature = 0.1, max_tokens = 8192, max_retries = 2;

COMMIT;

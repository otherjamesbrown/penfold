-- 055_model_stage_config.sql
-- Extends pipeline_config to support string values (for model names)
-- and adds per-stage model configuration keys

BEGIN;

-- 1. Extend pipeline_config CHECK constraint to allow 'string' type
ALTER TABLE pipeline_config DROP CONSTRAINT IF EXISTS pipeline_config_value_type_check;
ALTER TABLE pipeline_config ADD CONSTRAINT pipeline_config_value_type_check
    CHECK (value_type IN ('duration','integer','float','boolean','string'));

-- 2. Insert per-stage model configuration keys
-- Default values are empty (no model override) - inherits from global config
INSERT INTO pipeline_config (key, value, value_type, description, min_value, max_value, default_value)
VALUES
  ('model.stage.triage', '', 'string', 'Model override for triage stage (empty = use global default)', NULL, NULL, ''),
  ('model.stage.extract', '', 'string', 'Model override for extract stage (empty = use global default)', NULL, NULL, ''),
  ('model.stage.enrich', '', 'string', 'Model override for enrich stage (empty = use global default)', NULL, NULL, ''),
  ('model.stage.embed', '', 'string', 'Model override for embed stage (empty = use global default)', NULL, NULL, ''),
  ('model.stage.review', '', 'string', 'Model override for review stage (empty = use global default)', NULL, NULL, '')
ON CONFLICT (key) DO NOTHING;

COMMIT;

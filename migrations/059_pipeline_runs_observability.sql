-- 059_pipeline_runs_observability.sql
-- Add observability columns to pipeline_runs for token tracking, skip reasons, and trace linkage

BEGIN;

ALTER TABLE pipeline_runs ADD COLUMN IF NOT EXISTS input_tokens INT;
ALTER TABLE pipeline_runs ADD COLUMN IF NOT EXISTS output_tokens INT;
ALTER TABLE pipeline_runs ADD COLUMN IF NOT EXISTS skip_reason TEXT;
ALTER TABLE pipeline_runs ADD COLUMN IF NOT EXISTS langfuse_trace_id TEXT;

COMMIT;

-- Migration: 039_pipeline_stage_io
-- Description: Add JSONB columns to pipeline_runs for capturing input/output/parsed data at each stage
-- Author: Claude Code (data-dev agent)
-- Date: 2026-02-07

-- UP: Add IO data columns to pipeline_runs
BEGIN;

ALTER TABLE pipeline_runs ADD COLUMN IF NOT EXISTS input_data JSONB;
ALTER TABLE pipeline_runs ADD COLUMN IF NOT EXISTS output_data JSONB;
ALTER TABLE pipeline_runs ADD COLUMN IF NOT EXISTS parsed_data JSONB;

COMMIT;

-- Comments for documentation
COMMENT ON COLUMN pipeline_runs.input_data IS 'JSONB snapshot of input data sent to this stage (prompt + context)';
COMMENT ON COLUMN pipeline_runs.output_data IS 'JSONB snapshot of raw output from this stage (model response)';
COMMENT ON COLUMN pipeline_runs.parsed_data IS 'JSONB snapshot of parsed/structured output from this stage';

-- DOWN: Remove IO data columns
-- BEGIN;
-- ALTER TABLE pipeline_runs DROP COLUMN IF EXISTS input_data;
-- ALTER TABLE pipeline_runs DROP COLUMN IF EXISTS output_data;
-- ALTER TABLE pipeline_runs DROP COLUMN IF EXISTS parsed_data;
-- COMMIT;

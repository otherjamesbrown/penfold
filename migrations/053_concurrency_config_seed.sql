-- Migration 053: Seed concurrency configuration
-- Add pipeline.max_concurrent config key with default value of 1

INSERT INTO pipeline_config (key, value, value_type, description, default_value)
VALUES ('pipeline.max_concurrent', '1', 'integer', 'Maximum concurrent pipeline workflows', '1')
ON CONFLICT (key) DO NOTHING;

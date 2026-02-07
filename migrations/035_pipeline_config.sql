-- 035_pipeline_config.sql
-- Creates pipeline_config table for runtime configuration management
-- Extends failure_category constraint to include timeout/rate_limit/model_unavailable/context_cancelled
-- Adds error_code column to pipeline_runs for structured error tracking

BEGIN;

-- 1. Create pipeline_config table
CREATE TABLE pipeline_config (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    value_type TEXT NOT NULL CHECK (value_type IN ('duration','integer','float','boolean')),
    description TEXT NOT NULL,
    min_value TEXT,
    max_value TEXT,
    default_value TEXT NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT now(),
    updated_by TEXT
);

COMMENT ON TABLE pipeline_config IS 'Runtime configuration for pipeline timeouts and thresholds';
COMMENT ON COLUMN pipeline_config.key IS 'Configuration key (dotted notation, e.g., timeout.activity.fast.start_to_close)';
COMMENT ON COLUMN pipeline_config.value IS 'Current value as text (parse according to value_type)';
COMMENT ON COLUMN pipeline_config.value_type IS 'Type of value: duration (Go duration string), integer, float, boolean';
COMMENT ON COLUMN pipeline_config.description IS 'Human-readable description of what this config controls';
COMMENT ON COLUMN pipeline_config.min_value IS 'Minimum allowed value (optional, for validation)';
COMMENT ON COLUMN pipeline_config.max_value IS 'Maximum allowed value (optional, for validation)';
COMMENT ON COLUMN pipeline_config.default_value IS 'Default value if not explicitly set';
COMMENT ON COLUMN pipeline_config.updated_at IS 'Timestamp of last update';
COMMENT ON COLUMN pipeline_config.updated_by IS 'User or service that last updated this config';

-- 2. Seed pipeline_config with 12 timeout keys
INSERT INTO pipeline_config (key, value, value_type, description, min_value, max_value, default_value) VALUES
    ('timeout.ai_client.request', '120s', 'duration', 'AI gRPC client per-request timeout', '10s', '600s', '120s'),
    ('timeout.activity.fast.start_to_close', '30s', 'duration', 'Fast activity StartToClose timeout', '5s', '120s', '30s'),
    ('timeout.activity.fast.heartbeat', '10s', 'duration', 'Fast activity heartbeat timeout', '3s', '60s', '10s'),
    ('timeout.activity.embedding.start_to_close', '120s', 'duration', 'Embedding activity StartToClose timeout', '30s', '600s', '120s'),
    ('timeout.activity.embedding.heartbeat', '30s', 'duration', 'Embedding activity heartbeat timeout', '10s', '120s', '30s'),
    ('timeout.activity.llm.start_to_close', '600s', 'duration', 'LLM activity StartToClose timeout', '60s', '1800s', '600s'),
    ('timeout.activity.llm.heartbeat', '300s', 'duration', 'LLM activity heartbeat timeout', '30s', '900s', '300s'),
    ('timeout.activity.batch.start_to_close', '1800s', 'duration', 'Batch activity StartToClose timeout', '300s', '7200s', '1800s'),
    ('timeout.activity.batch.heartbeat', '300s', 'duration', 'Batch activity heartbeat timeout', '60s', '1800s', '300s'),
    ('timeout.http.backend.gemini', '120s', 'duration', 'Gemini backend HTTP timeout', '10s', '600s', '120s'),
    ('timeout.http.backend.mlx', '60s', 'duration', 'MLX backend HTTP timeout', '5s', '300s', '60s'),
    ('timeout.schedule_to_close.default', '3600s', 'duration', 'Default ScheduleToClose timeout', '600s', '14400s', '3600s');

-- Create index for quick config lookups
CREATE INDEX IF NOT EXISTS idx_pipeline_config_value_type ON pipeline_config (value_type);

-- 3. Extend failure_category constraint on sources table
ALTER TABLE sources DROP CONSTRAINT IF EXISTS ck_sources_failure_category_valid;
ALTER TABLE sources ADD CONSTRAINT ck_sources_failure_category_valid
    CHECK (
        failure_category IS NULL OR
        failure_category IN (
            'empty_content',
            'corrupt_file',
            'malformed_structure',
            'processing_error',
            'unsupported_format',
            'timeout',
            'rate_limit',
            'model_unavailable',
            'context_cancelled'
        )
    );

-- Update comment to reflect new categories
COMMENT ON COLUMN sources.failure_category IS 'Category of validation failure: empty_content, corrupt_file, malformed_structure, processing_error, unsupported_format, timeout, rate_limit, model_unavailable, context_cancelled';

-- 4. Add error_code column to pipeline_runs
ALTER TABLE pipeline_runs ADD COLUMN IF NOT EXISTS error_code TEXT;

COMMENT ON COLUMN pipeline_runs.error_code IS 'Structured error code for programmatic error handling (e.g., TIMEOUT, RATE_LIMIT, MODEL_UNAVAILABLE)';

-- Create index for querying pipeline runs by error code
CREATE INDEX IF NOT EXISTS idx_pipeline_runs_error_code ON pipeline_runs (error_code) WHERE error_code IS NOT NULL;

COMMIT;

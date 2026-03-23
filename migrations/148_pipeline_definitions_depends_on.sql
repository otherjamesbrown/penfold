-- Migration 148: Add depends_on column to pipeline_definitions
-- Enables DAG-based stage dependency declarations for the config-driven dispatch loop.
-- Existing rows default to an empty array (no dependencies), preserving current behaviour.

ALTER TABLE pipeline_definitions
    ADD COLUMN IF NOT EXISTS depends_on JSONB NOT NULL DEFAULT '[]';

COMMENT ON COLUMN pipeline_definitions.depends_on IS
    'JSON array of stage names that must complete successfully before this stage runs. '
    'Empty array means no dependencies (stage runs as soon as its position in stage_order is reached). '
    'Validated as a DAG at pipeline load time — circular dependencies are rejected.';

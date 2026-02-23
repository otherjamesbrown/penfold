-- Migration 077: Delete dead timeout.stage.* rows from pipeline_config.
-- These rows are no longer read — timeout config is stored in
-- pipeline_definitions.timeout_seconds (migration 074).
-- Author: agent-mycroft
-- Date: 2026-02-23
-- Shard: pf-17b872

BEGIN;

-- Remove all timeout.stage.* rows from pipeline_config.
-- resolveStageTimeouts now reads from pipeline_definitions instead.
DELETE FROM pipeline_config WHERE key LIKE 'timeout.stage.%';

COMMIT;

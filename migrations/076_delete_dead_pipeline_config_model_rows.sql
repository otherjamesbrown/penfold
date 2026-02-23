-- Migration 076: Delete dead model.stage.* rows from pipeline_config.
-- These 5 rows were never populated (all empty values) and never read.
-- Model config is stored in the model_config table (migration 052).
-- Author: agent-mycroft
-- Date: 2026-02-23
-- Shard: pf-afd46e

BEGIN;

-- Remove the 5 dead model.stage.* rows from pipeline_config
-- These were created but never populated — all have empty string values.
-- Model configuration is managed via the model_config table.
DELETE FROM pipeline_config WHERE key LIKE 'model.stage.%';

COMMIT;

-- pf-c4682e: Fix prompt_override for notification pipeline summarize and extract_semantic stages.
-- Migration 099 ran before these stages were added to pipeline_definitions for the notification
-- pipeline, so the UPDATE had no rows to affect. This migration applies the same override
-- idempotently, scoped to all tenants.
--
-- The notification pipeline uses prompt version 2 (notification-tailored prompts).

-- +goose Up
UPDATE pipeline_definitions
SET prompt_override = 2
WHERE pipeline = 'notification'
  AND stage IN ('summarize', 'extract_semantic')
  AND prompt_override IS DISTINCT FROM 2;

-- +goose Down
UPDATE pipeline_definitions
SET prompt_override = NULL
WHERE pipeline = 'notification'
  AND stage IN ('summarize', 'extract_semantic')
  AND prompt_override = 2;

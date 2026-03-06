BEGIN;

-- Seed triage stage params in pipeline_definitions
-- This allows DB-driven configuration of triage max_tokens, temperature, retries
-- following the same pattern as migration 087 for extract_ner and analyze stages
UPDATE pipeline_definitions
SET max_tokens = 512, temperature = 0.1, max_retries = 2
WHERE stage = 'triage' AND max_tokens IS NULL;

COMMIT;

-- +goose Up
BEGIN;

ALTER TABLE pipeline_definitions
  ADD COLUMN IF NOT EXISTS stage_kind VARCHAR(50) NOT NULL DEFAULT 'llm',
  ADD COLUMN IF NOT EXISTS persist_key VARCHAR(100);

COMMENT ON COLUMN pipeline_definitions.stage_kind IS 'How the workflow executes this stage: llm, code_only, embedding, structured_extract';
COMMENT ON COLUMN pipeline_definitions.persist_key IS 'For structured_extract stages: the key under which output is stored in extracted_data JSONB';

-- Backfill stage_kind for non-LLM stages
UPDATE pipeline_definitions SET stage_kind = 'code_only' WHERE stage IN ('parse', 'parse_transcript', 'resolve', 'enrich_entities', 'persist');
UPDATE pipeline_definitions SET stage_kind = 'embedding' WHERE stage IN ('embed');
UPDATE pipeline_definitions SET stage_kind = 'structured_extract' WHERE stage IN ('newsletter_extract', 'notification_extract');

-- Backfill persist_key for structured_extract stages
UPDATE pipeline_definitions SET persist_key = 'newsletter' WHERE stage = 'newsletter_extract';
UPDATE pipeline_definitions SET persist_key = 'notification' WHERE stage = 'notification_extract';

COMMIT;

-- +goose Down
BEGIN;
ALTER TABLE pipeline_definitions DROP COLUMN IF EXISTS persist_key;
ALTER TABLE pipeline_definitions DROP COLUMN IF EXISTS stage_kind;
COMMIT;

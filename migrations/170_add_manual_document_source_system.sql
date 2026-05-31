-- +goose Up

-- Migration 170: Add manual_document source system to the sources CHECK constraint.
-- Purpose: The IngestDocument RPC (PEN-20) persists ad-hoc files/URLs ingested via
-- `penf ingest file` / `penf ingest url` with source_system='manual_document'. The
-- CHECK constraint on the sources table did not include this value, causing every
-- document ingest to fail with ck_sources_source_system_valid (SQLSTATE 23514).

ALTER TABLE sources DROP CONSTRAINT IF EXISTS ck_sources_source_system_valid;
ALTER TABLE sources ADD CONSTRAINT ck_sources_source_system_valid CHECK (
  source_system::text = ANY (ARRAY[
    'gmail', 'slack', 'zoom', 'calendar', 'drive', 'confluence', 'jira',
    'teams', 'manual_eml', 'manual_document', 'attachment', 'embedded_email',
    'webex', 'google_meet', 'meeting_transcript', 'meeting_chat',
    'outlook_mail', 'teams_channel', 'teams_transcript'
  ]::text[])
);

-- +goose Down

ALTER TABLE sources DROP CONSTRAINT IF EXISTS ck_sources_source_system_valid;
ALTER TABLE sources ADD CONSTRAINT ck_sources_source_system_valid CHECK (
  source_system::text = ANY (ARRAY[
    'gmail', 'slack', 'zoom', 'calendar', 'drive', 'confluence', 'jira',
    'teams', 'manual_eml', 'attachment', 'embedded_email', 'webex',
    'google_meet', 'meeting_transcript', 'meeting_chat',
    'outlook_mail', 'teams_channel', 'teams_transcript'
  ]::text[])
);

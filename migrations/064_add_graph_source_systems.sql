-- Migration: Add Graph connector source systems to CHECK constraint
-- Purpose: Allow outlook_mail, teams_channel, teams_transcript in sources table
-- Shard: pf-ce1105
-- Date: 2026-03-04
--
-- The Graph connector activities (ProcessOutlookMessage, ProcessTeamsMessage,
-- ProcessTeamsTranscript) use these source_system values but the CHECK constraint
-- on the sources table didn't include them, causing all Graph syncs to fail.

ALTER TABLE sources DROP CONSTRAINT IF EXISTS ck_sources_source_system_valid;
ALTER TABLE sources ADD CONSTRAINT ck_sources_source_system_valid CHECK (
  source_system::text = ANY (ARRAY[
    'gmail', 'slack', 'zoom', 'calendar', 'drive', 'confluence', 'jira',
    'teams', 'manual_eml', 'attachment', 'embedded_email', 'webex',
    'google_meet', 'meeting_transcript', 'meeting_chat',
    'outlook_mail', 'teams_channel', 'teams_transcript'
  ]::text[])
);

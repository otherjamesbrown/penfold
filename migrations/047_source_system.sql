-- Migration: Add source_system classification to content_enrichment
-- Purpose: Track originating system for content classification and processing
-- Shard: pf-e8f26e
-- Date: 2026-02-15
--
-- Context: Adds source_system column to enable classification based on content origin
-- (Jira, Aha, Google Docs, Webex, Smartsheet, Outlook Calendar, auto-replies, human email).
-- Default 'unknown' for existing rows which will be backfilled in later waves.
--
-- This migration is safe to run multiple times (idempotent).

-- Add source_system column with default value
ALTER TABLE content_enrichment
ADD COLUMN IF NOT EXISTS source_system VARCHAR(50) NOT NULL DEFAULT 'unknown';

-- Create index for filtering by source system
CREATE INDEX IF NOT EXISTS idx_content_enrichment_source_system
ON content_enrichment (source_system);

-- Add documentation comment
COMMENT ON COLUMN content_enrichment.source_system IS 'Originating system: jira, aha, google_docs, webex, smartsheet, outlook_calendar, auto_reply, human_email, unknown';

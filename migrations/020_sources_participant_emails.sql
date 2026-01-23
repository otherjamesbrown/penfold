-- =====================================================
-- Sources Participant Emails Migration
-- Created: 2026-01-23
-- Bead: pe-13u3 - participant_emails not populated during email ingest
-- Description: Add participant_emails array to sources table for
--              efficient querying of email participants without
--              parsing JSONB metadata
-- =====================================================

-- Add participant_emails column to store From, To, Cc, Bcc addresses
ALTER TABLE sources ADD COLUMN IF NOT EXISTS participant_emails TEXT[];

-- Create GIN index for efficient array containment queries
-- Enables queries like: WHERE participant_emails @> ARRAY['user@example.com']
CREATE INDEX IF NOT EXISTS idx_sources_participant_emails
    ON sources USING GIN(participant_emails)
    WHERE participant_emails IS NOT NULL;

-- Comments for documentation
COMMENT ON COLUMN sources.participant_emails IS 'Array of all participant email addresses (From, To, Cc, Bcc) for efficient querying';

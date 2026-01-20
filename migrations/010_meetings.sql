-- Migration: 010_meetings
-- Description: Create meetings table for meeting ingest feature
-- Author: Claude Code
-- Date: 2026-01-19

-- Create meetings table
CREATE TABLE IF NOT EXISTS meetings (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL DEFAULT '00000001-0000-0000-0000-000000000001',

    -- Meeting metadata
    title VARCHAR(500) NOT NULL,
    meeting_date TIMESTAMPTZ,
    platform VARCHAR(50),  -- 'webex', 'teams', 'zoom', 'google_meet'
    duration_seconds INTEGER,

    -- Participants
    participant_count INTEGER,
    participants JSONB,  -- Array of participant names/emails

    -- Source tracking
    source_tag VARCHAR(255),
    source_path VARCHAR(1000),

    -- File references
    has_transcript BOOLEAN DEFAULT FALSE,
    has_chat BOOLEAN DEFAULT FALSE,
    has_video BOOLEAN DEFAULT FALSE,
    has_audio BOOLEAN DEFAULT FALSE,

    -- Processing status
    processing_status VARCHAR(50) DEFAULT 'pending',

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,

    -- Constraints
    CONSTRAINT meetings_tenant_id_fkey FOREIGN KEY (tenant_id)
        REFERENCES tenants(id) ON DELETE CASCADE
);

-- Add meeting_id to sources table to link sources to meetings
ALTER TABLE sources
ADD COLUMN IF NOT EXISTS meeting_id BIGINT REFERENCES meetings(id) ON DELETE SET NULL;

-- Indexes for meetings table
CREATE INDEX IF NOT EXISTS idx_meetings_tenant_id ON meetings(tenant_id);
CREATE INDEX IF NOT EXISTS idx_meetings_meeting_date ON meetings(meeting_date);
CREATE INDEX IF NOT EXISTS idx_meetings_platform ON meetings(platform);
CREATE INDEX IF NOT EXISTS idx_meetings_source_tag ON meetings(source_tag);
CREATE INDEX IF NOT EXISTS idx_meetings_processing_status ON meetings(processing_status);

-- Index for sources.meeting_id
CREATE INDEX IF NOT EXISTS idx_sources_meeting_id ON sources(meeting_id);

-- Row-level security for multi-tenant isolation
ALTER TABLE meetings ENABLE ROW LEVEL SECURITY;

CREATE POLICY meetings_tenant_isolation_policy ON meetings
    USING (tenant_id = COALESCE(
        (current_setting('app.current_tenant_id', true))::uuid,
        '00000000-0000-0000-0000-000000000000'::uuid
    ));

CREATE POLICY meetings_superuser_access_policy ON meetings
    USING (
        current_setting('app.current_tenant_id', true) IS NULL
        OR current_setting('app.current_tenant_id', true) = ''
        OR CURRENT_USER = 'postgres'
    );

-- Comments
COMMENT ON TABLE meetings IS 'Meeting records with metadata, linked to transcript/chat sources';
COMMENT ON COLUMN meetings.platform IS 'Meeting platform: webex, teams, zoom, google_meet';
COMMENT ON COLUMN meetings.participants IS 'JSON array of participant names extracted from transcript';
COMMENT ON COLUMN meetings.processing_status IS 'Processing status: pending, processing, completed, failed';

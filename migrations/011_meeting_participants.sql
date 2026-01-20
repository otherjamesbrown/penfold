-- Migration: meeting_participants junction table
-- Links meeting participants to resolved people for entity correlation

-- Create meeting_participants junction table
CREATE TABLE IF NOT EXISTS meeting_participants (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL DEFAULT '00000001-0000-0000-0000-000000000001',
    meeting_id BIGINT NOT NULL REFERENCES meetings(id) ON DELETE CASCADE,
    person_id BIGINT REFERENCES people(id) ON DELETE SET NULL,
    display_name VARCHAR(255) NOT NULL,
    match_type VARCHAR(20), -- 'exact', 'alias', 'fuzzy', NULL if unmatched
    confidence DECIMAL(3,2), -- 0.00 to 1.00
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Each participant appears once per meeting
    CONSTRAINT uq_meeting_participant UNIQUE (meeting_id, display_name)
);

-- Indexes for common queries
CREATE INDEX IF NOT EXISTS idx_meeting_participants_meeting_id
    ON meeting_participants(meeting_id);
CREATE INDEX IF NOT EXISTS idx_meeting_participants_person_id
    ON meeting_participants(person_id) WHERE person_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_meeting_participants_tenant_id
    ON meeting_participants(tenant_id);
CREATE INDEX IF NOT EXISTS idx_meeting_participants_unmatched
    ON meeting_participants(meeting_id) WHERE person_id IS NULL;

-- Row Level Security
ALTER TABLE meeting_participants ENABLE ROW LEVEL SECURITY;

-- RLS policy for tenant isolation
DROP POLICY IF EXISTS meeting_participants_tenant_isolation ON meeting_participants;
CREATE POLICY meeting_participants_tenant_isolation ON meeting_participants
    USING (tenant_id = current_setting('app.tenant_id', true)::uuid
           OR current_setting('app.tenant_id', true) IS NULL
           OR current_setting('app.tenant_id', true) = '');

-- Comment
COMMENT ON TABLE meeting_participants IS 'Links meeting participants to resolved people via entity resolution';
COMMENT ON COLUMN meeting_participants.display_name IS 'Original name as it appeared in the transcript';
COMMENT ON COLUMN meeting_participants.match_type IS 'How the participant was matched: exact, alias, fuzzy, or NULL';
COMMENT ON COLUMN meeting_participants.confidence IS 'Match confidence score from 0.00 to 1.00';

-- Migration: meeting_mentions table
-- Tracks people mentioned/discussed in meeting transcripts (distinct from attendees)

CREATE TABLE IF NOT EXISTS meeting_mentions (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL DEFAULT '00000001-0000-0000-0000-000000000001',
    meeting_id BIGINT NOT NULL REFERENCES meetings(id) ON DELETE CASCADE,
    source_id BIGINT REFERENCES sources(id) ON DELETE CASCADE,
    person_id BIGINT NOT NULL REFERENCES people(id) ON DELETE CASCADE,
    matched_text VARCHAR(255) NOT NULL,  -- The text that matched (e.g., "Adam", "Weingarten")
    match_type VARCHAR(20) NOT NULL,     -- 'canonical', 'alias', 'partial'
    context TEXT,                         -- Surrounding text snippet for context
    mention_count INTEGER DEFAULT 1,      -- How many times mentioned in this meeting
    confidence DECIMAL(3,2) DEFAULT 1.0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Each person mentioned once per meeting (aggregate count)
    CONSTRAINT uq_meeting_mention UNIQUE (meeting_id, person_id)
);

-- Indexes for common queries
CREATE INDEX IF NOT EXISTS idx_meeting_mentions_meeting_id
    ON meeting_mentions(meeting_id);
CREATE INDEX IF NOT EXISTS idx_meeting_mentions_person_id
    ON meeting_mentions(person_id);
CREATE INDEX IF NOT EXISTS idx_meeting_mentions_tenant_id
    ON meeting_mentions(tenant_id);

-- Row Level Security
ALTER TABLE meeting_mentions ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS meeting_mentions_tenant_isolation ON meeting_mentions;
CREATE POLICY meeting_mentions_tenant_isolation ON meeting_mentions
    USING (tenant_id = current_setting('app.tenant_id', true)::uuid
           OR current_setting('app.tenant_id', true) IS NULL
           OR current_setting('app.tenant_id', true) = '');

-- Comments
COMMENT ON TABLE meeting_mentions IS 'People mentioned/discussed in meeting transcripts (not attendees)';
COMMENT ON COLUMN meeting_mentions.matched_text IS 'The actual text that triggered the match';
COMMENT ON COLUMN meeting_mentions.context IS 'Surrounding text snippet showing the mention in context';
COMMENT ON COLUMN meeting_mentions.mention_count IS 'Number of times this person was mentioned in the meeting';

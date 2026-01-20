-- Review queue for AI questions requiring human input
-- Supports: acronym definitions, person disambiguation, entity confirmations

-- Question types
-- acronym: Unknown acronym needs definition
-- person: Ambiguous person reference needs clarification
-- entity: Entity needs confirmation or correction
-- duplicate: Potential duplicate needs review
-- other: General questions

-- Priority levels
-- high: Blocks understanding or processing
-- medium: Would improve search/correlation
-- low: Nice to have, cosmetic improvement

CREATE TABLE IF NOT EXISTS review_queue (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL DEFAULT '00000001-0000-0000-0000-000000000001',

    -- Question details
    question_type VARCHAR(50) NOT NULL,
    priority VARCHAR(20) NOT NULL DEFAULT 'medium',
    question TEXT NOT NULL,
    context TEXT,                          -- Surrounding text or additional info

    -- Source reference (where this question originated)
    source_type VARCHAR(50),               -- meeting, email, document, etc.
    source_id BIGINT,                      -- ID in the source table
    source_reference VARCHAR(255),         -- Human-readable reference (e.g., "TER meeting 2025-01-15")

    -- For acronym questions
    suggested_term VARCHAR(100),           -- The acronym/term in question
    suggested_expansion VARCHAR(500),      -- AI's best guess (if any)

    -- For person disambiguation
    candidate_person_ids BIGINT[],         -- Possible matches
    matched_text VARCHAR(255),             -- The text that matched ambiguously

    -- Resolution
    status VARCHAR(20) NOT NULL DEFAULT 'pending',  -- pending, resolved, dismissed, deferred
    resolution TEXT,                       -- User's answer/decision
    resolved_at TIMESTAMPTZ,
    resolved_by VARCHAR(255),

    -- AI confidence in needing this question
    confidence DECIMAL(3,2) DEFAULT 0.5,   -- How confident AI is this needs human input

    -- Metadata
    metadata JSONB DEFAULT '{}'::jsonb,

    -- Audit
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Constraints
    CONSTRAINT ck_review_queue_type CHECK (question_type IN ('acronym', 'person', 'entity', 'duplicate', 'other')),
    CONSTRAINT ck_review_queue_priority CHECK (priority IN ('high', 'medium', 'low')),
    CONSTRAINT ck_review_queue_status CHECK (status IN ('pending', 'resolved', 'dismissed', 'deferred'))
);

-- Indexes for common queries
CREATE INDEX IF NOT EXISTS idx_review_queue_status
    ON review_queue (tenant_id, status);

CREATE INDEX IF NOT EXISTS idx_review_queue_priority
    ON review_queue (tenant_id, status, priority);

CREATE INDEX IF NOT EXISTS idx_review_queue_type
    ON review_queue (tenant_id, question_type, status);

CREATE INDEX IF NOT EXISTS idx_review_queue_source
    ON review_queue (source_type, source_id);

-- Index for finding existing questions about a term (deduplication)
CREATE INDEX IF NOT EXISTS idx_review_queue_term
    ON review_queue (tenant_id, suggested_term)
    WHERE suggested_term IS NOT NULL;

-- Trigger to update updated_at
CREATE OR REPLACE FUNCTION update_review_queue_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_review_queue_updated_at ON review_queue;
CREATE TRIGGER trg_review_queue_updated_at
    BEFORE UPDATE ON review_queue
    FOR EACH ROW
    EXECUTE FUNCTION update_review_queue_updated_at();

-- Comments
COMMENT ON TABLE review_queue IS 'Queue for AI questions requiring human review';
COMMENT ON COLUMN review_queue.question_type IS 'Type: acronym, person, entity, duplicate, other';
COMMENT ON COLUMN review_queue.priority IS 'Priority: high, medium, low';
COMMENT ON COLUMN review_queue.confidence IS 'AI confidence that this needs human input (0.0-1.0)';
COMMENT ON COLUMN review_queue.status IS 'Status: pending, resolved, dismissed, deferred';

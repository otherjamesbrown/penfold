-- =====================================================
-- Source Attachments Migration
-- Created: 2026-01-19
-- Description: Links parent sources to child attachment sources
-- Bead: pe-dj4i
-- Epic: pe-wt9x (Attachment Extraction Pipeline)
-- =====================================================

-- Enum for attachment processing tiers
DO $$ BEGIN
    CREATE TYPE attachment_tier AS ENUM (
        'auto_process',    -- High-value: PDFs, docs, large images
        'auto_skip',       -- Low-value: signatures, logos, tracking pixels
        'pending_review',  -- Uncertain: needs manual triage
        'manual_process',  -- User marked for processing
        'manual_skip'      -- User marked to skip
    );
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Source attachments link table
-- Links parent email sources to child attachment sources
-- Enables attachments to be first-class searchable/embeddable content
CREATE TABLE IF NOT EXISTS source_attachments (
    id BIGSERIAL PRIMARY KEY,

    -- Parent-child relationship
    parent_source_id BIGINT NOT NULL,  -- Email source
    child_source_id BIGINT,            -- Attachment source (NULL if auto_skip)

    -- Attachment metadata (always captured)
    filename TEXT NOT NULL,
    mime_type VARCHAR(255),
    size_bytes BIGINT NOT NULL,
    content_hash VARCHAR(64),          -- SHA-256 for dedup across emails

    -- Position and inline handling
    position INT NOT NULL DEFAULT 0,   -- Order in email (0-indexed)
    content_id TEXT,                   -- Content-ID for inline references
    is_inline BOOLEAN DEFAULT FALSE,   -- Referenced in HTML body

    -- Classification/triage
    processing_tier attachment_tier NOT NULL DEFAULT 'pending_review',
    tier_reason TEXT,                  -- Human-readable explanation

    -- Extensible pipeline tracking
    -- Each step records: {step: "heuristic", result: "auto_process", confidence: 0.95, ts: "..."}
    processing_steps JSONB DEFAULT '[]'::jsonb,

    -- Special handling flags
    is_embedded_email BOOLEAN DEFAULT FALSE,  -- .eml/.msg attachment

    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,

    -- Constraints
    CONSTRAINT source_attachments_filename_not_empty CHECK (length(trim(filename)) > 0),
    CONSTRAINT source_attachments_size_positive CHECK (size_bytes >= 0),
    CONSTRAINT source_attachments_steps_array CHECK (jsonb_typeof(processing_steps) = 'array'),
    CONSTRAINT source_attachments_child_or_skip CHECK (
        child_source_id IS NOT NULL OR processing_tier IN ('auto_skip', 'manual_skip')
    )
);

-- Indexes for source_attachments
CREATE INDEX IF NOT EXISTS idx_source_attachments_parent ON source_attachments (parent_source_id);
CREATE INDEX IF NOT EXISTS idx_source_attachments_child ON source_attachments (child_source_id) WHERE child_source_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_source_attachments_content_hash ON source_attachments (content_hash) WHERE content_hash IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_source_attachments_tier ON source_attachments (processing_tier);
CREATE INDEX IF NOT EXISTS idx_source_attachments_pending ON source_attachments (created_at) WHERE processing_tier = 'pending_review';
CREATE INDEX IF NOT EXISTS idx_source_attachments_embedded ON source_attachments (parent_source_id) WHERE is_embedded_email = TRUE;

-- Unique constraint: one link per parent-position (can't have two attachments at same position)
CREATE UNIQUE INDEX IF NOT EXISTS idx_source_attachments_parent_position ON source_attachments (parent_source_id, position);

-- Comments for documentation
COMMENT ON TABLE source_attachments IS 'Links parent email sources to child attachment sources, enabling attachments as first-class content';
COMMENT ON COLUMN source_attachments.parent_source_id IS 'Source ID of the parent email';
COMMENT ON COLUMN source_attachments.child_source_id IS 'Source ID of the attachment (NULL if skipped)';
COMMENT ON COLUMN source_attachments.processing_tier IS 'Classification tier: auto_process (docs), auto_skip (signatures), pending_review (uncertain)';
COMMENT ON COLUMN source_attachments.tier_reason IS 'Human-readable explanation for tier classification';
COMMENT ON COLUMN source_attachments.processing_steps IS 'Pipeline step history for debugging: [{step, result, confidence, ts}]';
COMMENT ON COLUMN source_attachments.is_embedded_email IS 'True if attachment is .eml/.msg and was recursively ingested as email';
COMMENT ON COLUMN source_attachments.content_id IS 'MIME Content-ID for inline image references in HTML body';

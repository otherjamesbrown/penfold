-- =====================================================
-- Content Enrichment Tables Migration
-- Created: 2026-01-19
-- Description: Tables for content enrichment pipeline
-- Specification: specs/013-content-enrichment/
-- =====================================================

-- Enum types for content classification
DO $$ BEGIN
    CREATE TYPE content_type AS ENUM ('email', 'calendar', 'document', 'attachment');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TYPE processing_profile AS ENUM (
        'full_ai',
        'full_ai_chunked',
        'metadata_only',
        'state_tracking',
        'structure_only',
        'ocr_if_text'
    );
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TYPE enrichment_status AS ENUM (
        'pending',
        'classifying',
        'enriching',
        'extracting',
        'ai_processing',
        'completed',
        'failed',
        'skipped'
    );
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Main content enrichment table
-- Stores classification and enrichment results for each source
CREATE TABLE IF NOT EXISTS content_enrichment (
    id BIGSERIAL PRIMARY KEY,
    source_id BIGINT NOT NULL,
    tenant_id VARCHAR(255) NOT NULL,

    -- Classification (Stage 1)
    content_type content_type NOT NULL,
    content_subtype VARCHAR(100) NOT NULL,
    processing_profile processing_profile NOT NULL,
    classification_confidence REAL DEFAULT 1.0,
    classification_reason TEXT,

    -- Processing status
    status enrichment_status NOT NULL DEFAULT 'pending',
    current_stage VARCHAR(50),
    error_message TEXT,
    retry_count INTEGER DEFAULT 0,

    -- Common enrichment results (Stage 2)
    participants JSONB DEFAULT '[]'::jsonb,
    resolved_participants JSONB DEFAULT '[]'::jsonb,
    extracted_links JSONB DEFAULT '[]'::jsonb,
    thread_id VARCHAR(255),
    project_id BIGINT,

    -- Type-specific extraction results (Stage 3)
    extracted_data JSONB DEFAULT '{}'::jsonb,

    -- AI processing results (Stage 5)
    ai_processed BOOLEAN DEFAULT FALSE,
    ai_skip_reason TEXT,
    ai_processed_at TIMESTAMPTZ,

    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    completed_at TIMESTAMPTZ,

    -- Constraints
    CONSTRAINT content_enrichment_source_unique UNIQUE (source_id),
    CONSTRAINT content_enrichment_subtype_not_empty CHECK (length(trim(content_subtype)) > 0),
    CONSTRAINT content_enrichment_participants_array CHECK (jsonb_typeof(participants) = 'array'),
    CONSTRAINT content_enrichment_resolved_participants_array CHECK (jsonb_typeof(resolved_participants) = 'array'),
    CONSTRAINT content_enrichment_links_array CHECK (jsonb_typeof(extracted_links) = 'array'),
    CONSTRAINT content_enrichment_extracted_data_object CHECK (jsonb_typeof(extracted_data) = 'object')
);

-- Indexes for content_enrichment
CREATE INDEX IF NOT EXISTS idx_content_enrichment_source_id ON content_enrichment (source_id);
CREATE INDEX IF NOT EXISTS idx_content_enrichment_tenant_status ON content_enrichment (tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_content_enrichment_type_subtype ON content_enrichment (content_type, content_subtype);
CREATE INDEX IF NOT EXISTS idx_content_enrichment_processing_profile ON content_enrichment (processing_profile);
CREATE INDEX IF NOT EXISTS idx_content_enrichment_thread_id ON content_enrichment (thread_id) WHERE thread_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_content_enrichment_project_id ON content_enrichment (project_id) WHERE project_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_content_enrichment_pending ON content_enrichment (tenant_id, created_at) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_content_enrichment_failed ON content_enrichment (tenant_id, retry_count) WHERE status = 'failed';

-- Enrichment stage tracking table
-- Records each processing stage for observability and debugging
CREATE TABLE IF NOT EXISTS enrichment_stages (
    id BIGSERIAL PRIMARY KEY,
    enrichment_id BIGINT NOT NULL REFERENCES content_enrichment(id) ON DELETE CASCADE,
    stage_name VARCHAR(100) NOT NULL,
    processor_name VARCHAR(100) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'completed', 'failed', 'skipped')),

    -- Stage results
    input_data JSONB,
    output_data JSONB,
    error_message TEXT,

    -- Timing
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    duration_ms INTEGER,

    -- Metadata
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL
);

-- Indexes for enrichment_stages
CREATE INDEX IF NOT EXISTS idx_enrichment_stages_enrichment_id ON enrichment_stages (enrichment_id);
CREATE INDEX IF NOT EXISTS idx_enrichment_stages_stage_name ON enrichment_stages (stage_name, status);

-- Comments for documentation
COMMENT ON TABLE content_enrichment IS 'Stores classification and enrichment results for each ingested source';
COMMENT ON COLUMN content_enrichment.content_type IS 'Primary content type: email, calendar, document, attachment';
COMMENT ON COLUMN content_enrichment.content_subtype IS 'Subtype within content type, e.g., thread, notification/jira, invite';
COMMENT ON COLUMN content_enrichment.processing_profile IS 'Determines AI processing level: full_ai, metadata_only, state_tracking, etc.';
COMMENT ON COLUMN content_enrichment.participants IS 'Raw participant addresses extracted from content';
COMMENT ON COLUMN content_enrichment.resolved_participants IS 'Participants resolved to person_ids with roles';
COMMENT ON COLUMN content_enrichment.extracted_links IS 'URLs extracted from content with categories';
COMMENT ON COLUMN content_enrichment.extracted_data IS 'Type-specific extraction results (Jira tickets, meeting details, etc.)';
COMMENT ON TABLE enrichment_stages IS 'Tracks individual processing stages for debugging and observability';

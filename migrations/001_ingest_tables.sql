-- =====================================================
-- Email Ingest Tables Migration
-- Created: 2026-01-19
-- Description: Tables for batch email ingest job tracking
-- =====================================================

-- Ingest job tracking table for resumable batch imports
CREATE TABLE IF NOT EXISTS ingest_jobs (
    id VARCHAR(255) PRIMARY KEY,
    tenant_id VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled')),
    source_tag VARCHAR(255) NOT NULL,
    total_items INTEGER NOT NULL DEFAULT 0,
    processed_items INTEGER NOT NULL DEFAULT 0,
    failed_items INTEGER NOT NULL DEFAULT 0,
    last_file_path TEXT,
    labels JSONB DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    completed_at TIMESTAMPTZ,

    -- Constraints
    CONSTRAINT ingest_jobs_tenant_id_not_empty CHECK (length(trim(tenant_id)) > 0),
    CONSTRAINT ingest_jobs_source_tag_not_empty CHECK (length(trim(source_tag)) > 0),
    CONSTRAINT ingest_jobs_labels_is_array CHECK (jsonb_typeof(labels) = 'array'),
    CONSTRAINT ingest_jobs_completed_after_created CHECK (completed_at IS NULL OR completed_at >= created_at),
    CONSTRAINT ingest_jobs_processed_le_total CHECK (processed_items <= total_items OR total_items = 0),
    CONSTRAINT ingest_jobs_failed_le_processed CHECK (failed_items <= processed_items)
);

-- Indexes for ingest_jobs
CREATE INDEX IF NOT EXISTS idx_ingest_jobs_tenant_status ON ingest_jobs (tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_ingest_jobs_created_at ON ingest_jobs (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ingest_jobs_source_tag ON ingest_jobs (source_tag);

-- Ingest errors table for tracking per-file failures
CREATE TABLE IF NOT EXISTS ingest_errors (
    id BIGSERIAL PRIMARY KEY,
    job_id VARCHAR(255) NOT NULL REFERENCES ingest_jobs(id) ON DELETE CASCADE,
    file_path TEXT NOT NULL,
    error_message TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,

    -- Constraints
    CONSTRAINT ingest_errors_file_path_not_empty CHECK (length(trim(file_path)) > 0),
    CONSTRAINT ingest_errors_error_message_not_empty CHECK (length(trim(error_message)) > 0)
);

-- Indexes for ingest_errors
CREATE INDEX IF NOT EXISTS idx_ingest_errors_job_id ON ingest_errors (job_id);
CREATE INDEX IF NOT EXISTS idx_ingest_errors_created_at ON ingest_errors (created_at DESC);

-- Comments for documentation
COMMENT ON TABLE ingest_jobs IS 'Tracks batch email ingest operations for progress reporting and resume capability';
COMMENT ON TABLE ingest_errors IS 'Records individual file processing errors during batch ingest';
COMMENT ON COLUMN ingest_jobs.source_tag IS 'User-defined label identifying the source of emails (e.g., "outlook-backup-2024")';
COMMENT ON COLUMN ingest_jobs.last_file_path IS 'Path of last successfully processed file, used for resume';
COMMENT ON COLUMN ingest_jobs.labels IS 'Optional labels to apply to all ingested emails';

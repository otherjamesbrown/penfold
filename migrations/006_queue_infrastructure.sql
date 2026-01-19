-- =====================================================
-- Queue Infrastructure Tables Migration
-- Created: 2026-01-19
-- Description: Tables for dead letter queue and queue monitoring
-- Specification: specs/013-content-enrichment/queues-workers.md
-- =====================================================

-- =====================================================
-- Dead Letter Queue Table
-- =====================================================

-- Enum for error categories
DO $$ BEGIN
    CREATE TYPE error_category AS ENUM ('transient', 'permanent', 'partial', 'dependency');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Enum for DLQ resolution types
DO $$ BEGIN
    CREATE TYPE dlq_resolution AS ENUM ('retry_succeeded', 'manually_fixed', 'ignored', 'auto_expired');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Dead letter items table
CREATE TABLE IF NOT EXISTS dead_letter_items (
    id BIGSERIAL PRIMARY KEY,
    tenant_id VARCHAR(255) NOT NULL,

    -- Source identification
    source_id BIGINT, -- May be null if source creation failed
    batch_id VARCHAR(100), -- For batch tracking

    -- Queue info
    queue_name VARCHAR(100) NOT NULL, -- Which queue it failed in
    message_type VARCHAR(100) NOT NULL, -- IngestMessage, EnrichmentMessage, etc.

    -- Error details
    error_category error_category NOT NULL,
    error_code VARCHAR(100),
    error_message TEXT NOT NULL,
    error_stack TEXT, -- Stack trace if available

    -- Retry tracking
    retry_count INT DEFAULT 0 NOT NULL,
    last_retry_at TIMESTAMPTZ,
    next_retry_at TIMESTAMPTZ, -- When auto-retry will be attempted

    -- Original message (for replay)
    original_message JSONB NOT NULL,

    -- Resolution
    resolved_at TIMESTAMPTZ,
    resolution dlq_resolution,
    resolution_notes TEXT,
    resolved_by VARCHAR(255),

    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,

    -- Constraints
    CONSTRAINT dead_letter_items_error_not_empty CHECK (length(trim(error_message)) > 0)
);

-- Indexes for dead_letter_items
CREATE INDEX IF NOT EXISTS idx_dlq_tenant ON dead_letter_items (tenant_id);
CREATE INDEX IF NOT EXISTS idx_dlq_source ON dead_letter_items (source_id) WHERE source_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_dlq_queue ON dead_letter_items (queue_name);
CREATE INDEX IF NOT EXISTS idx_dlq_category ON dead_letter_items (error_category);
CREATE INDEX IF NOT EXISTS idx_dlq_unresolved ON dead_letter_items (tenant_id, resolved_at) WHERE resolved_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_dlq_created ON dead_letter_items (tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_dlq_next_retry ON dead_letter_items (next_retry_at) WHERE resolved_at IS NULL AND next_retry_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_dlq_batch ON dead_letter_items (batch_id) WHERE batch_id IS NOT NULL;

-- =====================================================
-- Queue Statistics Table (for monitoring)
-- =====================================================

-- Queue stats table (aggregated metrics)
CREATE TABLE IF NOT EXISTS queue_stats (
    id BIGSERIAL PRIMARY KEY,
    tenant_id VARCHAR(255) NOT NULL,
    queue_name VARCHAR(100) NOT NULL,

    -- Time bucket (hourly aggregation)
    bucket_start TIMESTAMPTZ NOT NULL,
    bucket_end TIMESTAMPTZ NOT NULL,

    -- Volume metrics
    messages_enqueued INT DEFAULT 0,
    messages_processed INT DEFAULT 0,
    messages_failed INT DEFAULT 0,
    messages_retried INT DEFAULT 0,

    -- Latency metrics (in milliseconds)
    avg_processing_time_ms INT,
    p50_processing_time_ms INT,
    p95_processing_time_ms INT,
    p99_processing_time_ms INT,
    max_processing_time_ms INT,

    -- Queue depth (point-in-time at bucket end)
    queue_depth INT,

    -- Worker metrics
    active_workers INT,

    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,

    -- Constraints
    CONSTRAINT queue_stats_bucket_unique UNIQUE (tenant_id, queue_name, bucket_start)
);

-- Indexes for queue_stats
CREATE INDEX IF NOT EXISTS idx_queue_stats_tenant ON queue_stats (tenant_id);
CREATE INDEX IF NOT EXISTS idx_queue_stats_queue ON queue_stats (queue_name);
CREATE INDEX IF NOT EXISTS idx_queue_stats_bucket ON queue_stats (tenant_id, bucket_start DESC);
CREATE INDEX IF NOT EXISTS idx_queue_stats_recent ON queue_stats (bucket_start DESC);

-- =====================================================
-- Batch Tracking Table
-- =====================================================

-- Batch status enum
DO $$ BEGIN
    CREATE TYPE batch_status AS ENUM ('pending', 'processing', 'completed', 'partial', 'failed');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Batch tracking for group processing
CREATE TABLE IF NOT EXISTS processing_batches (
    id BIGSERIAL PRIMARY KEY,
    batch_id VARCHAR(100) NOT NULL UNIQUE,
    tenant_id VARCHAR(255) NOT NULL,

    -- Batch info
    batch_type VARCHAR(50) NOT NULL, -- email_ingest, re_enrichment, backfill
    description TEXT,

    -- Progress tracking
    status batch_status NOT NULL DEFAULT 'pending',
    total_items INT NOT NULL,
    processed_items INT DEFAULT 0,
    failed_items INT DEFAULT 0,
    skipped_items INT DEFAULT 0,

    -- Priority
    priority INT DEFAULT 1 CHECK (priority >= 0 AND priority <= 2), -- 0=low, 1=normal, 2=high

    -- Timing
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    estimated_completion_at TIMESTAMPTZ,

    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,

    -- Constraints
    CONSTRAINT processing_batches_total_positive CHECK (total_items > 0)
);

-- Indexes for processing_batches
CREATE INDEX IF NOT EXISTS idx_batches_tenant ON processing_batches (tenant_id);
CREATE INDEX IF NOT EXISTS idx_batches_status ON processing_batches (tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_batches_created ON processing_batches (tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_batches_active ON processing_batches (tenant_id) WHERE status IN ('pending', 'processing');

-- =====================================================
-- Worker Registration Table (for distributed coordination)
-- =====================================================

-- Worker health status
DO $$ BEGIN
    CREATE TYPE worker_status AS ENUM ('starting', 'healthy', 'unhealthy', 'draining', 'stopped');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Workers table
CREATE TABLE IF NOT EXISTS workers (
    id BIGSERIAL PRIMARY KEY,
    worker_id VARCHAR(100) NOT NULL UNIQUE, -- UUID or hostname-based
    worker_type VARCHAR(50) NOT NULL, -- ingest, enrichment, ai

    -- Configuration
    queue_name VARCHAR(100) NOT NULL,
    batch_size INT DEFAULT 1,
    concurrency INT DEFAULT 1,

    -- Status
    status worker_status NOT NULL DEFAULT 'starting',
    current_batch_id VARCHAR(100), -- If processing a batch
    items_in_progress INT DEFAULT 0,

    -- Health tracking
    last_heartbeat_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ DEFAULT NOW(),
    stopped_at TIMESTAMPTZ,

    -- Metrics
    items_processed INT DEFAULT 0,
    items_failed INT DEFAULT 0,
    avg_processing_time_ms INT,

    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT NOW() NOT NULL
);

-- Indexes for workers
CREATE INDEX IF NOT EXISTS idx_workers_type ON workers (worker_type);
CREATE INDEX IF NOT EXISTS idx_workers_queue ON workers (queue_name);
CREATE INDEX IF NOT EXISTS idx_workers_status ON workers (status);
CREATE INDEX IF NOT EXISTS idx_workers_heartbeat ON workers (last_heartbeat_at) WHERE status = 'healthy';
CREATE INDEX IF NOT EXISTS idx_workers_batch ON workers (current_batch_id) WHERE current_batch_id IS NOT NULL;

-- =====================================================
-- Comments
-- =====================================================

COMMENT ON TABLE dead_letter_items IS 'Failed queue messages for investigation and replay';
COMMENT ON COLUMN dead_letter_items.error_category IS 'transient (retry), permanent (fix needed), partial (continue), dependency (external failure)';
COMMENT ON COLUMN dead_letter_items.original_message IS 'Full message JSON for replay';
COMMENT ON COLUMN dead_letter_items.next_retry_at IS 'When auto-retry will be attempted for transient errors';

COMMENT ON TABLE queue_stats IS 'Hourly aggregated statistics per queue for monitoring';
COMMENT ON COLUMN queue_stats.bucket_start IS 'Start of the hourly aggregation bucket';

COMMENT ON TABLE processing_batches IS 'Tracks batch processing jobs for progress monitoring';
COMMENT ON COLUMN processing_batches.batch_id IS 'Unique identifier for the batch (UUID or descriptive)';
COMMENT ON COLUMN processing_batches.priority IS '0=low (backfill), 1=normal (batch ingest), 2=high (real-time)';

COMMENT ON TABLE workers IS 'Registered workers for distributed coordination';
COMMENT ON COLUMN workers.worker_id IS 'Unique identifier (typically UUID or hostname:pid)';
COMMENT ON COLUMN workers.last_heartbeat_at IS 'Workers must heartbeat every 30s or be considered unhealthy';

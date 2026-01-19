-- =====================================================
-- Penfold Review Queue Database Schema
-- Created: 2026-01-18
-- Description: PostgreSQL schema for review queue management
-- Requirements: PostgreSQL 16+
-- =====================================================

-- Enable required extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- =====================================================
-- REVIEW QUEUE TABLE
-- =====================================================

-- Main review queue table for pending items
CREATE TABLE IF NOT EXISTS review_queue (
    -- Identity
    id TEXT NOT NULL,
    tenant_id UUID NOT NULL,

    -- Priority and ordering
    priority FLOAT NOT NULL DEFAULT 1.0,
    original_priority INTEGER NOT NULL DEFAULT 1,

    -- Content information
    content_type TEXT NOT NULL DEFAULT '',
    content_summary TEXT DEFAULT '',
    source TEXT DEFAULT '',
    category TEXT DEFAULT '',

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    enqueued_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),

    -- Deferral tracking
    deferred_until TIMESTAMPTZ,
    defer_count INTEGER NOT NULL DEFAULT 0,

    -- Status (pending, processing, completed)
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'completed')),

    -- Metadata
    metadata JSONB DEFAULT '{}' NOT NULL,

    -- Primary key is composite for tenant isolation
    PRIMARY KEY (id, tenant_id),

    -- Constraints
    CONSTRAINT review_queue_id_not_empty CHECK (length(trim(id)) > 0),
    CONSTRAINT review_queue_priority_positive CHECK (priority >= 0),
    CONSTRAINT review_queue_original_priority_valid CHECK (original_priority BETWEEN 0 AND 4),
    CONSTRAINT review_queue_defer_count_positive CHECK (defer_count >= 0),
    CONSTRAINT review_queue_metadata_is_object CHECK (jsonb_typeof(metadata) = 'object')
);

-- =====================================================
-- REVIEW QUEUE HISTORY TABLE (for throughput tracking)
-- =====================================================

-- History of processed queue items for metrics/analytics
CREATE TABLE IF NOT EXISTS review_queue_history (
    id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    item_id TEXT NOT NULL,
    tenant_id UUID NOT NULL,

    -- Action taken
    action TEXT NOT NULL CHECK (action IN ('approved', 'rejected', 'completed')),

    -- Timing information
    enqueued_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    wait_time_seconds FLOAT GENERATED ALWAYS AS (EXTRACT(EPOCH FROM (completed_at - enqueued_at))) STORED,

    -- Item details at time of completion
    priority FLOAT NOT NULL,
    content_type TEXT DEFAULT '',
    category TEXT DEFAULT '',

    -- Additional context
    metadata JSONB DEFAULT '{}' NOT NULL,

    -- Constraints
    CONSTRAINT review_queue_history_item_not_empty CHECK (length(trim(item_id)) > 0)
);

-- =====================================================
-- PERFORMANCE OPTIMIZATION INDEXES
-- =====================================================

-- Primary queue access pattern: get pending items by priority
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_review_queue_tenant_status_priority
    ON review_queue (tenant_id, status, priority DESC, enqueued_at ASC)
    WHERE status = 'pending';

-- Deferred items that are now ready
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_review_queue_deferred_ready
    ON review_queue (tenant_id, status, deferred_until)
    WHERE status = 'pending' AND deferred_until IS NOT NULL;

-- Queue depth by category
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_review_queue_tenant_category
    ON review_queue (tenant_id, category)
    WHERE status = 'pending';

-- Queue depth by content type
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_review_queue_tenant_content_type
    ON review_queue (tenant_id, content_type)
    WHERE status = 'pending';

-- Queue depth by original priority
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_review_queue_tenant_orig_priority
    ON review_queue (tenant_id, original_priority)
    WHERE status = 'pending';

-- History for throughput analysis
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_review_queue_history_tenant_completed
    ON review_queue_history (tenant_id, completed_at DESC);

-- History by action type
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_review_queue_history_tenant_action
    ON review_queue_history (tenant_id, action, completed_at DESC);

-- =====================================================
-- ROW-LEVEL SECURITY (RLS) FOR MULTI-TENANT ISOLATION
-- =====================================================

-- Enable RLS on review queue tables
ALTER TABLE review_queue ENABLE ROW LEVEL SECURITY;
ALTER TABLE review_queue_history ENABLE ROW LEVEL SECURITY;

-- Create tenant isolation policies
CREATE POLICY IF NOT EXISTS tenant_isolation_review_queue ON review_queue
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid);

CREATE POLICY IF NOT EXISTS tenant_isolation_review_queue_history ON review_queue_history
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid);

-- =====================================================
-- UTILITY FUNCTIONS
-- =====================================================

-- Function to archive completed items to history
CREATE OR REPLACE FUNCTION archive_completed_queue_item()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.status = 'completed' AND OLD.status != 'completed' THEN
        INSERT INTO review_queue_history (
            item_id, tenant_id, action, enqueued_at, completed_at,
            priority, content_type, category, metadata
        ) VALUES (
            NEW.id, NEW.tenant_id, 'completed', NEW.enqueued_at, NOW(),
            NEW.priority, NEW.content_type, NEW.category, NEW.metadata
        );
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger to automatically archive completed items
DROP TRIGGER IF EXISTS trg_archive_completed_queue_item ON review_queue;
CREATE TRIGGER trg_archive_completed_queue_item
    AFTER UPDATE OF status ON review_queue
    FOR EACH ROW
    WHEN (NEW.status = 'completed')
    EXECUTE FUNCTION archive_completed_queue_item();

-- Function to get queue statistics for a tenant
CREATE OR REPLACE FUNCTION get_queue_stats(p_tenant_id UUID)
RETURNS TABLE (
    total_items BIGINT,
    pending_items BIGINT,
    deferred_items BIGINT,
    high_priority_items BIGINT,
    oldest_age_seconds FLOAT,
    avg_wait_seconds FLOAT
) AS $$
BEGIN
    RETURN QUERY
    SELECT
        COUNT(*) as total_items,
        COUNT(*) FILTER (WHERE status = 'pending' AND (deferred_until IS NULL OR deferred_until <= NOW())) as pending_items,
        COUNT(*) FILTER (WHERE deferred_until > NOW()) as deferred_items,
        COUNT(*) FILTER (WHERE original_priority >= 3) as high_priority_items,
        EXTRACT(EPOCH FROM (NOW() - MIN(enqueued_at) FILTER (WHERE status = 'pending'))) as oldest_age_seconds,
        AVG(EXTRACT(EPOCH FROM (NOW() - enqueued_at))) FILTER (WHERE status = 'pending') as avg_wait_seconds
    FROM review_queue
    WHERE tenant_id = p_tenant_id;
END;
$$ LANGUAGE plpgsql;

-- Function to apply priority aging to all pending items
CREATE OR REPLACE FUNCTION apply_queue_aging(p_aging_factor FLOAT DEFAULT 0.1)
RETURNS INTEGER AS $$
DECLARE
    updated_count INTEGER;
BEGIN
    UPDATE review_queue
    SET priority = priority + (p_aging_factor * EXTRACT(EPOCH FROM (NOW() - enqueued_at)) / 3600),
        updated_at = NOW()
    WHERE status = 'pending'
      AND deferred_until IS NULL;

    GET DIAGNOSTICS updated_count = ROW_COUNT;
    RETURN updated_count;
END;
$$ LANGUAGE plpgsql;

-- =====================================================
-- DATA RETENTION FUNCTION
-- =====================================================

-- Function to clean up old history records
CREATE OR REPLACE FUNCTION cleanup_queue_history(retention_days INTEGER DEFAULT 90)
RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER;
BEGIN
    DELETE FROM review_queue_history
    WHERE completed_at < NOW() - (retention_days || ' days')::INTERVAL;

    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

-- =====================================================
-- VERIFICATION
-- =====================================================

DO $$
BEGIN
    RAISE NOTICE 'Review queue schema setup complete!';
    RAISE NOTICE '- Created review_queue table with priority ordering';
    RAISE NOTICE '- Created review_queue_history table for throughput tracking';
    RAISE NOTICE '- Added performance indexes for common access patterns';
    RAISE NOTICE '- Enabled Row-Level Security for tenant isolation';
    RAISE NOTICE '- Created utility functions for queue management';
END
$$;

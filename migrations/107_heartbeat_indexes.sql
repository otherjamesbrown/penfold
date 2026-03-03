-- Migration: heartbeat query indexes
-- Adds composite indexes to support heartbeat health check queries.

-- Composite index for heartbeat watch_matches query:
-- SELECT COUNT(*) FROM assertions WHERE tenant_id = $1 AND created_at > NOW() - interval
-- The existing idx_assertions_created_at (created_at DESC) doesn't include tenant_id,
-- so this composite index enables an index-only scan.
CREATE INDEX IF NOT EXISTS idx_assertions_tenant_created
    ON assertions (tenant_id, created_at DESC);

-- Partial index for heartbeat stale_content query:
-- SELECT COUNT(*) FROM sources WHERE tenant_id = $1 AND processing_status IN ('processing','pending') AND updated_at < $2
-- The existing idx_sources_rejected_skipped covers different statuses.
-- This index targets the specific statuses checked by the heartbeat.
CREATE INDEX IF NOT EXISTS idx_sources_stale_content
    ON sources (tenant_id, updated_at)
    WHERE processing_status IN ('processing', 'pending');

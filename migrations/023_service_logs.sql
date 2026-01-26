-- =====================================================
-- Service Logs Migration
-- Created: 2026-01-26
-- Bead: pe-iaw8 - Create logs service and connect penf logs CLI
-- Description: Centralized service log storage for aggregated log viewing
--              via the penf CLI. Supports filtering by service, level, time,
--              and content search.
-- =====================================================

-- +goose Up

-- =====================================================
-- Service Logs Table
-- =====================================================
-- Central log storage for all Penfold services.
-- Enables log aggregation and querying through the gateway.

CREATE TABLE IF NOT EXISTS service_logs (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID DEFAULT '00000000-0000-0000-0000-000000000000'::uuid,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    level TEXT NOT NULL,                               -- debug, info, warn, error
    service TEXT NOT NULL,                             -- gateway, worker, ai_service, etc.
    message TEXT NOT NULL,                             -- Log message content
    fields JSONB DEFAULT '{}',                         -- Structured log fields
    trace_id TEXT,                                     -- Optional trace ID for correlation
    span_id TEXT,                                      -- Optional span ID for correlation
    caller TEXT,                                       -- File:line that generated the log
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,

    -- Constraints
    CONSTRAINT service_logs_level_check CHECK (level IN ('debug', 'info', 'warn', 'error'))
);

-- =====================================================
-- Indexes for Efficient Querying
-- =====================================================

-- Primary query patterns: filter by time, service, level
CREATE INDEX IF NOT EXISTS idx_service_logs_timestamp ON service_logs (timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_service_logs_service ON service_logs (service);
CREATE INDEX IF NOT EXISTS idx_service_logs_level ON service_logs (level);
CREATE INDEX IF NOT EXISTS idx_service_logs_trace_id ON service_logs (trace_id) WHERE trace_id IS NOT NULL;

-- Composite index for common filter combinations
CREATE INDEX IF NOT EXISTS idx_service_logs_service_timestamp ON service_logs (service, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_service_logs_level_timestamp ON service_logs (level, timestamp DESC);

-- Full-text search on message content
CREATE INDEX IF NOT EXISTS idx_service_logs_message_fts ON service_logs USING GIN (to_tsvector('english', message));

-- JSONB index for field filtering
CREATE INDEX IF NOT EXISTS idx_service_logs_fields ON service_logs USING GIN (fields);

-- =====================================================
-- Retention Policy Function
-- =====================================================
-- Function to clean up old logs (call via scheduled job or Temporal workflow)

CREATE OR REPLACE FUNCTION cleanup_old_service_logs(retention_days INTEGER DEFAULT 30)
RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER;
BEGIN
    DELETE FROM service_logs
    WHERE timestamp < NOW() - (retention_days || ' days')::INTERVAL;

    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

-- =====================================================
-- Comments
-- =====================================================

COMMENT ON TABLE service_logs IS 'Centralized log storage for all Penfold services';
COMMENT ON COLUMN service_logs.level IS 'Log severity: debug, info, warn, error';
COMMENT ON COLUMN service_logs.service IS 'Service name that generated the log (gateway, worker, etc.)';
COMMENT ON COLUMN service_logs.fields IS 'Structured key-value log fields as JSONB';
COMMENT ON COLUMN service_logs.trace_id IS 'OpenTelemetry trace ID for distributed tracing';
COMMENT ON COLUMN service_logs.span_id IS 'OpenTelemetry span ID for distributed tracing';
COMMENT ON COLUMN service_logs.caller IS 'Source file:line that generated the log';
COMMENT ON FUNCTION cleanup_old_service_logs IS 'Removes logs older than retention_days (default 30)';

-- +goose Down

DROP FUNCTION IF EXISTS cleanup_old_service_logs;
DROP TABLE IF EXISTS service_logs;

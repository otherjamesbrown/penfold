-- =====================================================
-- Penfold Observability Framework Database Schema
-- Created: 2026-01-14
-- Description: Complete TimescaleDB-based observability setup
-- Requirements: PostgreSQL 16+ with TimescaleDB extension
-- =====================================================

-- Enable required extensions
CREATE EXTENSION IF NOT EXISTS timescaledb CASCADE;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Set search path to include observability tables in main schema
-- Note: Using main schema since tenants table already exists there
SET search_path TO public;

-- Verify TimescaleDB installation
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_available_extensions
        WHERE name = 'timescaledb' AND installed_version IS NOT NULL
    ) THEN
        RAISE EXCEPTION 'TimescaleDB extension is not installed';
    END IF;
END
$$;

-- =====================================================
-- CORE OBSERVABILITY TABLES
-- =====================================================

-- Agent registry table
CREATE TABLE IF NOT EXISTS agents (
    agent_id TEXT PRIMARY KEY,
    agent_name TEXT NOT NULL,
    agent_type TEXT NOT NULL CHECK (agent_type IN ('processing', 'analysis', 'coordination', 'review')),
    configuration JSONB DEFAULT '{}' NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    last_active_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    is_active BOOLEAN DEFAULT true NOT NULL,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,

    -- Constraints
    CONSTRAINT agents_agent_id_not_empty CHECK (length(trim(agent_id)) > 0),
    CONSTRAINT agents_agent_name_not_empty CHECK (length(trim(agent_name)) > 0),
    CONSTRAINT agents_configuration_is_object CHECK (jsonb_typeof(configuration) = 'object')
);

-- Time-series metrics table (will become TimescaleDB hypertable)
CREATE TABLE IF NOT EXISTS agent_metrics (
    id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    agent_id TEXT NOT NULL REFERENCES agents(agent_id) ON DELETE CASCADE,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    metric_name TEXT NOT NULL,
    metric_value FLOAT NOT NULL,
    metric_type TEXT DEFAULT 'gauge' CHECK (metric_type IN ('counter', 'gauge', 'histogram')),
    labels JSONB DEFAULT '{}' NOT NULL,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,

    -- Constraints
    CONSTRAINT agent_metrics_metric_name_not_empty CHECK (length(trim(metric_name)) > 0),
    CONSTRAINT agent_metrics_metric_value_finite CHECK (metric_value IS NOT NULL AND metric_value = metric_value), -- No NaN/Infinity
    CONSTRAINT agent_metrics_timestamp_not_future CHECK (timestamp <= NOW() + INTERVAL '1 minute'),
    CONSTRAINT agent_metrics_labels_is_object CHECK (jsonb_typeof(labels) = 'object')
);

-- Decision traces for debugging (will become TimescaleDB hypertable)
CREATE TABLE IF NOT EXISTS decision_traces (
    id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    agent_id TEXT NOT NULL REFERENCES agents(agent_id) ON DELETE CASCADE,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    decision_type TEXT NOT NULL,
    decision_context JSONB NOT NULL,
    alternatives_considered JSONB DEFAULT '[]' NOT NULL,
    confidence_score FLOAT NOT NULL CHECK (confidence_score >= 0.0 AND confidence_score <= 1.0),
    reasoning TEXT,
    workflow_id UUID,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,

    -- Constraints
    CONSTRAINT decision_traces_decision_type_not_empty CHECK (length(trim(decision_type)) > 0),
    CONSTRAINT decision_traces_context_is_object CHECK (jsonb_typeof(decision_context) = 'object'),
    CONSTRAINT decision_traces_alternatives_is_array CHECK (jsonb_typeof(alternatives_considered) = 'array'),
    CONSTRAINT decision_traces_reasoning_length CHECK (reasoning IS NULL OR length(reasoning) <= 10000)
);

-- Workflow events for cross-agent tracking (will become TimescaleDB hypertable)
CREATE TABLE IF NOT EXISTS workflow_events (
    id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    workflow_id UUID NOT NULL,
    agent_id TEXT NOT NULL REFERENCES agents(agent_id) ON DELETE CASCADE,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    event_type TEXT NOT NULL CHECK (event_type IN ('workflow_started', 'stage_completed', 'handoff', 'workflow_finished', 'error')),
    event_status TEXT NOT NULL CHECK (event_status IN ('success', 'failure', 'timeout', 'retry')),
    event_data JSONB DEFAULT '{}' NOT NULL,
    processing_time_ms FLOAT CHECK (processing_time_ms IS NULL OR processing_time_ms >= 0),
    parent_workflow_id UUID,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,

    -- Constraints
    CONSTRAINT workflow_events_event_data_is_object CHECK (jsonb_typeof(event_data) = 'object'),
    CONSTRAINT workflow_events_parent_not_self CHECK (parent_workflow_id IS NULL OR parent_workflow_id != workflow_id)
);

-- Structured agent logs (will become TimescaleDB hypertable)
CREATE TABLE IF NOT EXISTS agent_logs (
    id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    agent_id TEXT NOT NULL REFERENCES agents(agent_id) ON DELETE CASCADE,
    level TEXT NOT NULL CHECK (level IN ('DEBUG', 'INFO', 'WARNING', 'ERROR', 'CRITICAL')),
    event TEXT NOT NULL,
    context JSONB DEFAULT '{}' NOT NULL,
    workflow_id UUID,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,

    -- Constraints
    CONSTRAINT agent_logs_event_not_empty CHECK (length(trim(event)) > 0),
    CONSTRAINT agent_logs_event_length CHECK (length(event) <= 1000),
    CONSTRAINT agent_logs_context_is_object CHECK (jsonb_typeof(context) = 'object')
);

-- Alert thresholds configuration
CREATE TABLE IF NOT EXISTS alert_thresholds (
    id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    agent_id TEXT NOT NULL REFERENCES agents(agent_id) ON DELETE CASCADE,
    metric_name TEXT NOT NULL,
    threshold_type TEXT DEFAULT 'performance' CHECK (threshold_type IN ('performance', 'error_rate', 'resource_usage')),
    threshold_value FLOAT NOT NULL,
    comparison_operator TEXT NOT NULL CHECK (comparison_operator IN ('greater_than', 'less_than', 'equals', 'not_equals')),
    evaluation_window_minutes INTEGER NOT NULL CHECK (evaluation_window_minutes BETWEEN 1 AND 1440),
    is_active BOOLEAN DEFAULT true NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,

    -- Constraints
    CONSTRAINT alert_thresholds_metric_name_not_empty CHECK (length(trim(metric_name)) > 0),
    CONSTRAINT alert_thresholds_threshold_value_finite CHECK (threshold_value = threshold_value),

    -- Unique constraint to prevent duplicate thresholds
    UNIQUE (agent_id, metric_name, threshold_type, tenant_id)
);

-- Alert events for audit trail
CREATE TABLE IF NOT EXISTS alert_events (
    id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    threshold_id UUID NOT NULL REFERENCES alert_thresholds(id) ON DELETE CASCADE,
    triggered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ,
    alert_status TEXT DEFAULT 'active' CHECK (alert_status IN ('active', 'resolved', 'acknowledged', 'suppressed')),
    trigger_value FLOAT NOT NULL,
    alert_context JSONB DEFAULT '{}' NOT NULL,
    resolution_notes TEXT,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,

    -- Constraints
    CONSTRAINT alert_events_resolved_after_triggered CHECK (resolved_at IS NULL OR resolved_at >= triggered_at),
    CONSTRAINT alert_events_trigger_value_finite CHECK (trigger_value = trigger_value),
    CONSTRAINT alert_events_alert_context_is_object CHECK (jsonb_typeof(alert_context) = 'object'),
    CONSTRAINT alert_events_resolution_notes_length CHECK (resolution_notes IS NULL OR length(resolution_notes) <= 2000)
);

-- =====================================================
-- CONVERT TABLES TO TIMESCALEDB HYPERTABLES
-- =====================================================

-- Convert agent_metrics to hypertable (1-day chunks)
DO $$
BEGIN
    -- Check if already a hypertable
    IF NOT EXISTS (
        SELECT 1 FROM timescaledb_information.hypertables
        WHERE hypertable_name = 'agent_metrics' AND hypertable_schema = 'public'
    ) THEN
        PERFORM create_hypertable('agent_metrics', 'timestamp',
                                chunk_time_interval => INTERVAL '1 day',
                                if_not_exists => TRUE);
    END IF;
END
$$;

-- Convert decision_traces to hypertable (1-day chunks)
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM timescaledb_information.hypertables
        WHERE hypertable_name = 'decision_traces' AND hypertable_schema = 'public'
    ) THEN
        PERFORM create_hypertable('decision_traces', 'timestamp',
                                chunk_time_interval => INTERVAL '1 day',
                                if_not_exists => TRUE);
    END IF;
END
$$;

-- Convert workflow_events to hypertable (1-day chunks)
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM timescaledb_information.hypertables
        WHERE hypertable_name = 'workflow_events' AND hypertable_schema = 'public'
    ) THEN
        PERFORM create_hypertable('workflow_events', 'timestamp',
                                chunk_time_interval => INTERVAL '1 day',
                                if_not_exists => TRUE);
    END IF;
END
$$;

-- Convert agent_logs to hypertable (6-hour chunks for faster log processing)
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM timescaledb_information.hypertables
        WHERE hypertable_name = 'agent_logs' AND hypertable_schema = 'public'
    ) THEN
        PERFORM create_hypertable('agent_logs', 'timestamp',
                                chunk_time_interval => INTERVAL '6 hours',
                                if_not_exists => TRUE);
    END IF;
END
$$;

-- =====================================================
-- PERFORMANCE OPTIMIZATION INDEXES
-- =====================================================

-- Agent metrics indexes
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_agent_metrics_agent_time
    ON agent_metrics (agent_id, timestamp DESC);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_agent_metrics_name_time
    ON agent_metrics (metric_name, timestamp DESC);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_agent_metrics_labels
    ON agent_metrics USING gin(labels);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_agent_metrics_tenant_time
    ON agent_metrics (tenant_id, timestamp DESC);

-- Decision traces indexes
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_decision_traces_agent_time
    ON decision_traces (agent_id, timestamp DESC);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_decision_traces_workflow
    ON decision_traces (workflow_id) WHERE workflow_id IS NOT NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_decision_traces_type_time
    ON decision_traces (decision_type, timestamp DESC);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_decision_traces_tenant_time
    ON decision_traces (tenant_id, timestamp DESC);

-- Workflow events indexes
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_workflow_events_workflow_time
    ON workflow_events (workflow_id, timestamp);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_workflow_events_agent_time
    ON workflow_events (agent_id, timestamp DESC);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_workflow_events_type_status
    ON workflow_events (event_type, event_status);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_workflow_events_tenant_time
    ON workflow_events (tenant_id, timestamp DESC);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_workflow_events_parent
    ON workflow_events (parent_workflow_id) WHERE parent_workflow_id IS NOT NULL;

-- Agent logs indexes
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_agent_logs_agent_time
    ON agent_logs (agent_id, timestamp DESC);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_agent_logs_level_time
    ON agent_logs (level, timestamp DESC);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_agent_logs_workflow
    ON agent_logs (workflow_id) WHERE workflow_id IS NOT NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_agent_logs_context
    ON agent_logs USING gin(context);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_agent_logs_tenant_time
    ON agent_logs (tenant_id, timestamp DESC);

-- Alert indexes
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_alert_thresholds_agent_active
    ON alert_thresholds (agent_id, is_active);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_alert_thresholds_tenant_active
    ON alert_thresholds (tenant_id, is_active);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_alert_events_status_triggered
    ON alert_events (alert_status, triggered_at DESC);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_alert_events_tenant_triggered
    ON alert_events (tenant_id, triggered_at DESC);

-- Agents table indexes
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_agents_tenant_active
    ON agents (tenant_id, is_active);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_agents_type_active
    ON agents (agent_type, is_active);

-- =====================================================
-- TIMESCALEDB DATA MANAGEMENT POLICIES
-- =====================================================

-- Compression policies (compress data older than 7 days)
DO $$
BEGIN
    -- Check if compression policy already exists before adding
    IF NOT EXISTS (
        SELECT 1 FROM timescaledb_information.jobs
        WHERE hypertable_name = 'agent_metrics'
        AND job_type = 'compression'
    ) THEN
        PERFORM add_compression_policy('agent_metrics', INTERVAL '7 days');
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM timescaledb_information.jobs
        WHERE hypertable_name = 'decision_traces'
        AND job_type = 'compression'
    ) THEN
        PERFORM add_compression_policy('decision_traces', INTERVAL '7 days');
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM timescaledb_information.jobs
        WHERE hypertable_name = 'workflow_events'
        AND job_type = 'compression'
    ) THEN
        PERFORM add_compression_policy('workflow_events', INTERVAL '7 days');
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM timescaledb_information.jobs
        WHERE hypertable_name = 'agent_logs'
        AND job_type = 'compression'
    ) THEN
        PERFORM add_compression_policy('agent_logs', INTERVAL '1 day');
    END IF;
END
$$;

-- Retention policies (delete data older than 90 days)
DO $$
BEGIN
    -- Check if retention policy already exists before adding
    IF NOT EXISTS (
        SELECT 1 FROM timescaledb_information.jobs
        WHERE hypertable_name = 'agent_metrics'
        AND job_type = 'retention'
    ) THEN
        PERFORM add_retention_policy('agent_metrics', INTERVAL '90 days');
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM timescaledb_information.jobs
        WHERE hypertable_name = 'decision_traces'
        AND job_type = 'retention'
    ) THEN
        PERFORM add_retention_policy('decision_traces', INTERVAL '90 days');
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM timescaledb_information.jobs
        WHERE hypertable_name = 'workflow_events'
        AND job_type = 'retention'
    ) THEN
        PERFORM add_retention_policy('workflow_events', INTERVAL '90 days');
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM timescaledb_information.jobs
        WHERE hypertable_name = 'agent_logs'
        AND job_type = 'retention'
    ) THEN
        PERFORM add_retention_policy('agent_logs', INTERVAL '90 days');
    END IF;
END
$$;

-- =====================================================
-- CONTINUOUS AGGREGATES FOR DASHBOARD PERFORMANCE
-- =====================================================

-- Hourly agent performance summary
CREATE MATERIALIZED VIEW IF NOT EXISTS agent_performance_hourly
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 hour', timestamp) as hour,
    agent_id,
    metric_name,
    avg(metric_value) as avg_value,
    max(metric_value) as max_value,
    min(metric_value) as min_value,
    count(*) as sample_count,
    tenant_id
FROM agent_metrics
WHERE metric_name IN ('processing_time_ms', 'memory_usage_mb', 'confidence_score')
GROUP BY hour, agent_id, metric_name, tenant_id;

-- Daily error summary
CREATE MATERIALIZED VIEW IF NOT EXISTS agent_errors_daily
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 day', timestamp) as day,
    agent_id,
    count(*) FILTER (WHERE level = 'ERROR') as error_count,
    count(*) FILTER (WHERE level = 'WARNING') as warning_count,
    count(*) as total_logs,
    CASE
        WHEN count(*) > 0 THEN (count(*) FILTER (WHERE level = 'ERROR') * 100.0 / count(*))
        ELSE 0.0
    END as error_rate_percent,
    tenant_id
FROM agent_logs
GROUP BY day, agent_id, tenant_id;

-- Workflow performance summary
CREATE MATERIALIZED VIEW IF NOT EXISTS workflow_performance_hourly
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 hour', timestamp) as hour,
    agent_id,
    event_type,
    count(*) as event_count,
    count(*) FILTER (WHERE event_status = 'success') as success_count,
    count(*) FILTER (WHERE event_status = 'failure') as failure_count,
    avg(processing_time_ms) as avg_processing_time,
    max(processing_time_ms) as max_processing_time,
    tenant_id
FROM workflow_events
WHERE processing_time_ms IS NOT NULL
GROUP BY hour, agent_id, event_type, tenant_id;

-- Add refresh policies for continuous aggregates
DO $$
BEGIN
    -- Hourly performance aggregate refresh policy
    IF NOT EXISTS (
        SELECT 1 FROM timescaledb_information.jobs j
        JOIN timescaledb_information.continuous_aggregates ca
            ON j.hypertable_name = ca.materialization_hypertable_name
        WHERE ca.view_name = 'agent_performance_hourly'
    ) THEN
        PERFORM add_continuous_aggregate_policy('agent_performance_hourly',
            start_offset => INTERVAL '2 hours',
            end_offset => INTERVAL '1 hour',
            schedule_interval => INTERVAL '1 hour');
    END IF;

    -- Daily errors aggregate refresh policy
    IF NOT EXISTS (
        SELECT 1 FROM timescaledb_information.jobs j
        JOIN timescaledb_information.continuous_aggregates ca
            ON j.hypertable_name = ca.materialization_hypertable_name
        WHERE ca.view_name = 'agent_errors_daily'
    ) THEN
        PERFORM add_continuous_aggregate_policy('agent_errors_daily',
            start_offset => INTERVAL '1 day',
            end_offset => INTERVAL '1 hour',
            schedule_interval => INTERVAL '1 hour');
    END IF;

    -- Workflow performance aggregate refresh policy
    IF NOT EXISTS (
        SELECT 1 FROM timescaledb_information.jobs j
        JOIN timescaledb_information.continuous_aggregates ca
            ON j.hypertable_name = ca.materialization_hypertable_name
        WHERE ca.view_name = 'workflow_performance_hourly'
    ) THEN
        PERFORM add_continuous_aggregate_policy('workflow_performance_hourly',
            start_offset => INTERVAL '2 hours',
            end_offset => INTERVAL '1 hour',
            schedule_interval => INTERVAL '1 hour');
    END IF;
END
$$;

-- =====================================================
-- ROW-LEVEL SECURITY (RLS) FOR MULTI-TENANT ISOLATION
-- =====================================================

-- Enable RLS on all observability tables
ALTER TABLE agents ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_metrics ENABLE ROW LEVEL SECURITY;
ALTER TABLE decision_traces ENABLE ROW LEVEL SECURITY;
ALTER TABLE workflow_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_logs ENABLE ROW LEVEL SECURITY;
ALTER TABLE alert_thresholds ENABLE ROW LEVEL SECURITY;
ALTER TABLE alert_events ENABLE ROW LEVEL SECURITY;

-- Create tenant isolation policies
CREATE POLICY IF NOT EXISTS tenant_isolation_agents ON agents
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid);

CREATE POLICY IF NOT EXISTS tenant_isolation_metrics ON agent_metrics
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid);

CREATE POLICY IF NOT EXISTS tenant_isolation_traces ON decision_traces
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid);

CREATE POLICY IF NOT EXISTS tenant_isolation_workflows ON workflow_events
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid);

CREATE POLICY IF NOT EXISTS tenant_isolation_logs ON agent_logs
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid);

CREATE POLICY IF NOT EXISTS tenant_isolation_thresholds ON alert_thresholds
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid);

CREATE POLICY IF NOT EXISTS tenant_isolation_alert_events ON alert_events
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid);

-- =====================================================
-- UTILITY FUNCTIONS
-- =====================================================

-- Function to get current tenant ID (for applications to use)
CREATE OR REPLACE FUNCTION get_current_tenant_id()
RETURNS UUID AS $$
BEGIN
    RETURN current_setting('app.current_tenant', true)::uuid;
EXCEPTION WHEN OTHERS THEN
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

-- Function to set tenant context (for applications to use)
CREATE OR REPLACE FUNCTION set_tenant_context(tenant_uuid UUID)
RETURNS VOID AS $$
BEGIN
    PERFORM set_config('app.current_tenant', tenant_uuid::text, false);
END;
$$ LANGUAGE plpgsql;

-- Function to validate agent exists before foreign key operations
CREATE OR REPLACE FUNCTION validate_agent_exists(p_agent_id TEXT, p_tenant_id UUID)
RETURNS BOOLEAN AS $$
BEGIN
    RETURN EXISTS (
        SELECT 1 FROM agents
        WHERE agent_id = p_agent_id
        AND tenant_id = p_tenant_id
        AND is_active = true
    );
END;
$$ LANGUAGE plpgsql;

-- =====================================================
-- VERIFICATION AND SUMMARY
-- =====================================================

-- Display setup summary
DO $$
DECLARE
    hypertable_count INTEGER;
    policy_count INTEGER;
    aggregate_count INTEGER;
BEGIN
    -- Count created hypertables
    SELECT COUNT(*) INTO hypertable_count
    FROM timescaledb_information.hypertables
    WHERE hypertable_schema = 'public'
    AND hypertable_name IN ('agent_metrics', 'decision_traces', 'workflow_events', 'agent_logs');

    -- Count policies
    SELECT COUNT(*) INTO policy_count
    FROM timescaledb_information.jobs
    WHERE job_type IN ('compression', 'retention');

    -- Count continuous aggregates
    SELECT COUNT(*) INTO aggregate_count
    FROM timescaledb_information.continuous_aggregates
    WHERE view_schema = 'public'
    AND view_name IN ('agent_performance_hourly', 'agent_errors_daily', 'workflow_performance_hourly');

    RAISE NOTICE 'Observability framework setup complete!';
    RAISE NOTICE '- Created % TimescaleDB hypertables', hypertable_count;
    RAISE NOTICE '- Configured % data management policies', policy_count;
    RAISE NOTICE '- Created % continuous aggregates', aggregate_count;
    RAISE NOTICE '- Enabled Row-Level Security for tenant isolation';
    RAISE NOTICE '- Created performance indexes for optimal query performance';

    IF hypertable_count = 4 THEN
        RAISE NOTICE 'SUCCESS: All core observability tables are properly configured as hypertables';
    ELSE
        RAISE WARNING 'WARNING: Expected 4 hypertables, found %', hypertable_count;
    END IF;
END
$$;

-- Display next steps
SELECT
    'Setup complete. Next steps:' as message
UNION ALL SELECT
    '1. Install Python dependencies: pip install timescaledb-python structlog'
UNION ALL SELECT
    '2. Configure tenant context in application code'
UNION ALL SELECT
    '3. Register agents using agents table'
UNION ALL SELECT
    '4. Start collecting metrics via agent instrumentation'
UNION ALL SELECT
    '5. Access dashboard at http://localhost:8001/dashboard';
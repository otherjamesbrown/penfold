# Observability Framework Quickstart Guide

**Last Updated**: 2026-01-14
**Prerequisites**: Python 3.12+, PostgreSQL 16+ with TimescaleDB, existing Penfold development environment

## Development Environment Setup

### 1. Core Dependencies

```bash
# Activate existing Penfold virtual environment
source venv/bin/activate

# Install observability-specific dependencies
pip install timescaledb-python
pip install structlog
pip install fastapi-sse-starlette
pip install prometheus-client  # For metrics exposition format
pip install psutil  # For system resource monitoring
```

### 2. Database Setup (TimescaleDB Extension)

```bash
# Connect to existing Penfold PostgreSQL database
psql -d penfold_dev

# Enable TimescaleDB extension
CREATE EXTENSION IF NOT EXISTS timescaledb CASCADE;

# Verify TimescaleDB installation
SELECT default_version, installed_version
FROM pg_available_extensions
WHERE name = 'timescaledb';

# Create observability schema (optional - can use main schema)
CREATE SCHEMA IF NOT EXISTS observability;
SET search_path TO observability, public;
```

### 3. Initialize Observability Database Schema

```sql
-- Core agent registry
CREATE TABLE agents (
    agent_id TEXT PRIMARY KEY,
    agent_name TEXT NOT NULL,
    agent_type TEXT NOT NULL CHECK (agent_type IN ('processing', 'analysis', 'coordination', 'review')),
    configuration JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    last_active_at TIMESTAMPTZ DEFAULT NOW(),
    is_active BOOLEAN DEFAULT true,
    tenant_id UUID REFERENCES tenants(id)
);

-- Time-series metrics table
CREATE TABLE agent_metrics (
    id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    agent_id TEXT NOT NULL REFERENCES agents(agent_id),
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    metric_name TEXT NOT NULL,
    metric_value FLOAT NOT NULL,
    metric_type TEXT DEFAULT 'gauge' CHECK (metric_type IN ('counter', 'gauge', 'histogram')),
    labels JSONB DEFAULT '{}',
    tenant_id UUID NOT NULL REFERENCES tenants(id)
);

-- Convert to TimescaleDB hypertable (1-day chunks)
SELECT create_hypertable('agent_metrics', 'timestamp',
                        chunk_time_interval => INTERVAL '1 day');

-- Decision traces for debugging
CREATE TABLE decision_traces (
    id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    agent_id TEXT NOT NULL REFERENCES agents(agent_id),
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    decision_type TEXT NOT NULL,
    decision_context JSONB NOT NULL,
    alternatives_considered JSONB DEFAULT '[]',
    confidence_score FLOAT NOT NULL CHECK (confidence_score >= 0.0 AND confidence_score <= 1.0),
    reasoning TEXT,
    workflow_id UUID,
    tenant_id UUID NOT NULL REFERENCES tenants(id)
);

-- Convert to hypertable
SELECT create_hypertable('decision_traces', 'timestamp',
                        chunk_time_interval => INTERVAL '1 day');

-- Workflow events for cross-agent tracking
CREATE TABLE workflow_events (
    id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    workflow_id UUID NOT NULL,
    agent_id TEXT NOT NULL REFERENCES agents(agent_id),
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    event_type TEXT NOT NULL CHECK (event_type IN ('workflow_started', 'stage_completed', 'handoff', 'workflow_finished', 'error')),
    event_status TEXT NOT NULL CHECK (event_status IN ('success', 'failure', 'timeout', 'retry')),
    event_data JSONB DEFAULT '{}',
    processing_time_ms FLOAT,
    parent_workflow_id UUID,
    tenant_id UUID NOT NULL REFERENCES tenants(id)
);

-- Convert to hypertable
SELECT create_hypertable('workflow_events', 'timestamp',
                        chunk_time_interval => INTERVAL '1 day');

-- Structured agent logs
CREATE TABLE agent_logs (
    id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    agent_id TEXT NOT NULL REFERENCES agents(agent_id),
    level TEXT NOT NULL CHECK (level IN ('DEBUG', 'INFO', 'WARNING', 'ERROR', 'CRITICAL')),
    event TEXT NOT NULL,
    context JSONB DEFAULT '{}',
    workflow_id UUID,
    tenant_id UUID NOT NULL REFERENCES tenants(id)
);

-- Convert to hypertable (6-hour chunks for faster log processing)
SELECT create_hypertable('agent_logs', 'timestamp',
                        chunk_time_interval => INTERVAL '6 hours');

-- Alert thresholds configuration
CREATE TABLE alert_thresholds (
    id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    agent_id TEXT NOT NULL REFERENCES agents(agent_id),
    metric_name TEXT NOT NULL,
    threshold_type TEXT DEFAULT 'performance' CHECK (threshold_type IN ('performance', 'error_rate', 'resource_usage')),
    threshold_value FLOAT NOT NULL,
    comparison_operator TEXT NOT NULL CHECK (comparison_operator IN ('greater_than', 'less_than', 'equals', 'not_equals')),
    evaluation_window_minutes INTEGER NOT NULL CHECK (evaluation_window_minutes BETWEEN 1 AND 1440),
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    tenant_id UUID NOT NULL REFERENCES tenants(id)
);

-- Alert events for audit trail
CREATE TABLE alert_events (
    id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    threshold_id UUID NOT NULL REFERENCES alert_thresholds(id),
    triggered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ,
    alert_status TEXT DEFAULT 'active' CHECK (alert_status IN ('active', 'resolved', 'acknowledged', 'suppressed')),
    trigger_value FLOAT NOT NULL,
    alert_context JSONB DEFAULT '{}',
    resolution_notes TEXT,
    tenant_id UUID NOT NULL REFERENCES tenants(id)
);
```

### 4. Performance Optimization Indexes

```sql
-- Agent metrics indexes
CREATE INDEX idx_agent_metrics_agent_time ON agent_metrics (agent_id, timestamp DESC);
CREATE INDEX idx_agent_metrics_name_time ON agent_metrics (metric_name, timestamp DESC);
CREATE INDEX idx_agent_metrics_labels ON agent_metrics USING gin(labels);

-- Decision traces indexes
CREATE INDEX idx_decision_traces_agent_time ON decision_traces (agent_id, timestamp DESC);
CREATE INDEX idx_decision_traces_workflow ON decision_traces (workflow_id);
CREATE INDEX idx_decision_traces_type_time ON decision_traces (decision_type, timestamp DESC);

-- Workflow events indexes
CREATE INDEX idx_workflow_events_workflow_time ON workflow_events (workflow_id, timestamp);
CREATE INDEX idx_workflow_events_agent_time ON workflow_events (agent_id, timestamp DESC);
CREATE INDEX idx_workflow_events_type_status ON workflow_events (event_type, event_status);

-- Agent logs indexes
CREATE INDEX idx_agent_logs_agent_time ON agent_logs (agent_id, timestamp DESC);
CREATE INDEX idx_agent_logs_level_time ON agent_logs (level, timestamp DESC);
CREATE INDEX idx_agent_logs_workflow ON agent_logs (workflow_id) WHERE workflow_id IS NOT NULL;
CREATE INDEX idx_agent_logs_context ON agent_logs USING gin(context);

-- Alert indexes
CREATE INDEX idx_alert_thresholds_agent_active ON alert_thresholds (agent_id, is_active);
CREATE INDEX idx_alert_events_status_triggered ON alert_events (alert_status, triggered_at DESC);
```

### 5. TimescaleDB Data Management Policies

```sql
-- Compression policies (compress data older than 7 days)
SELECT add_compression_policy('agent_metrics', INTERVAL '7 days');
SELECT add_compression_policy('decision_traces', INTERVAL '7 days');
SELECT add_compression_policy('workflow_events', INTERVAL '7 days');
SELECT add_compression_policy('agent_logs', INTERVAL '1 day');

-- Retention policies (delete data older than 90 days)
SELECT add_retention_policy('agent_metrics', INTERVAL '90 days');
SELECT add_retention_policy('decision_traces', INTERVAL '90 days');
SELECT add_retention_policy('workflow_events', INTERVAL '90 days');
SELECT add_retention_policy('agent_logs', INTERVAL '90 days');

-- Continuous aggregates for dashboard performance
CREATE MATERIALIZED VIEW agent_performance_hourly
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 hour', timestamp) as hour,
    agent_id,
    metric_name,
    avg(metric_value) as avg_value,
    max(metric_value) as max_value,
    min(metric_value) as min_value,
    count(*) as sample_count
FROM agent_metrics
WHERE metric_name IN ('processing_time_ms', 'memory_usage_mb', 'confidence_score')
GROUP BY hour, agent_id, metric_name;

-- Refresh policy for continuous aggregates
SELECT add_continuous_aggregate_policy('agent_performance_hourly',
    start_offset => INTERVAL '2 hours',
    end_offset => INTERVAL '1 hour',
    schedule_interval => INTERVAL '1 hour');

-- Daily error summary
CREATE MATERIALIZED VIEW agent_errors_daily
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 day', timestamp) as day,
    agent_id,
    count(*) FILTER (WHERE level = 'ERROR') as error_count,
    count(*) FILTER (WHERE level = 'WARNING') as warning_count,
    count(*) as total_logs,
    (count(*) FILTER (WHERE level = 'ERROR') * 100.0 / count(*)) as error_rate_percent
FROM agent_logs
GROUP BY day, agent_id;

SELECT add_continuous_aggregate_policy('agent_errors_daily',
    start_offset => INTERVAL '1 day',
    end_offset => INTERVAL '1 hour',
    schedule_interval => INTERVAL '1 hour');
```

## Quick Start Commands

### 1. Initialize Observability Library

```python
# observability_lib/__init__.py
from .services.instrumentation import monitor_agent, workflow_trace
from .services.metrics_collector import MetricsCollector
from .services.dashboard_api import create_monitoring_app
from .models.agent_metrics import MetricPoint, DecisionContext

__version__ = "1.0.0"
__all__ = [
    "monitor_agent",
    "workflow_trace",
    "MetricsCollector",
    "create_monitoring_app",
    "MetricPoint",
    "DecisionContext"
]
```

### 2. Test Database Connection

```python
# test_observability_setup.py
import asyncio
import asyncpg
from datetime import datetime
from observability_lib.models.agent_metrics import MetricPoint

async def test_setup():
    # Connect to database
    conn = await asyncpg.connect("postgresql://localhost/penfold_dev")

    # Test agent registration
    await conn.execute("""
        INSERT INTO agents (agent_id, agent_name, agent_type, tenant_id)
        VALUES ('test_agent', 'Test Agent', 'processing', $1)
        ON CONFLICT (agent_id) DO NOTHING
    """, "your-tenant-id")

    # Test metrics insertion
    await conn.execute("""
        INSERT INTO agent_metrics (agent_id, metric_name, metric_value, tenant_id)
        VALUES ('test_agent', 'test_metric', 42.0, $1)
    """, "your-tenant-id")

    # Test query performance
    start_time = datetime.now()
    rows = await conn.fetch("""
        SELECT * FROM agent_metrics
        WHERE agent_id = 'test_agent'
        AND timestamp >= NOW() - INTERVAL '1 hour'
        ORDER BY timestamp DESC
    """)
    end_time = datetime.now()

    print(f"Query returned {len(rows)} rows in {(end_time - start_time).total_seconds():.3f}s")

    await conn.close()

# Run test
asyncio.run(test_setup())
```

### 3. Start Development Services

```bash
# Terminal 1: Start observability API server
uvicorn observability_lib.cli.dashboard:app --reload --port 8001

# Terminal 2: Start background alert evaluator (if implemented)
python -m observability_lib.cli.monitor --evaluate-alerts --interval 60

# Terminal 3: Test agent with monitoring
python test_monitored_agent.py
```

### 4. Example Monitored Agent

```python
# test_monitored_agent.py
import asyncio
import uuid
from datetime import datetime
from observability_lib.services.instrumentation import monitor_agent
from observability_lib.services.metrics_collector import MetricsCollector
from observability_lib.services.decision_tracer import DecisionTracer
from observability_lib.services.workflow_tracker import WorkflowTracker
from observability_lib.services.alert_manager import AlertManager

@monitor_agent("test_email_processor")
class TestEmailProcessor:
    async def process_emails(self, emails):
        async with self.workflow_trace("email_batch_test") as tracer:

            # Record batch size metric
            await self.record_metric("batch_size", len(emails))

            async with tracer.stage("validation"):
                # Simulate email validation
                await asyncio.sleep(0.1)
                valid_emails = [e for e in emails if "@" in e]

                # Log decision about email validation
                await tracer.log_decision(
                    decision_type="email_validation",
                    decision_context={"total_emails": len(emails)},
                    alternatives=[
                        {"method": "regex", "accuracy": 0.95},
                        {"method": "strict_parsing", "accuracy": 0.99}
                    ],
                    confidence=0.95,
                    reasoning="Used regex validation for speed"
                )

            async with tracer.stage("processing"):
                # Simulate processing
                await asyncio.sleep(0.2)
                processed = len(valid_emails)

                # Record processing metrics
                await self.record_metric("emails_processed", processed)
                await self.record_metric("processing_time_ms", 200)

                # Log handoff to next agent
                await tracer.log_handoff(
                    to_agent="categorizer",
                    data_passed={"processed_emails": processed},
                    handoff_reason="validation_complete"
                )

            return valid_emails

async def test_monitored_agent():
    # Initialize observability components (normally done by DI container)
    metrics_collector = MetricsCollector()
    decision_tracer = DecisionTracer()
    workflow_tracker = WorkflowTracker()
    alert_manager = AlertManager()

    # Create and configure agent
    agent = TestEmailProcessor()
    agent.set_observability_components(
        metrics_collector=metrics_collector,
        decision_tracer=decision_tracer,
        workflow_tracker=workflow_tracker,
        alert_manager=alert_manager
    )

    # Test with sample data
    test_emails = ["user1@example.com", "invalid-email", "user2@test.com"]
    result = await agent.process_emails(test_emails)

    print(f"Processed {len(result)} valid emails from {len(test_emails)} total")

if __name__ == "__main__":
    asyncio.run(test_monitored_agent())
```

### 5. Basic Dashboard Access

```bash
# Open browser to monitoring dashboard
open http://localhost:8001/dashboard

# API health check
curl http://localhost:8001/api/v1/observability/dashboard/status

# Get agent metrics
curl "http://localhost:8001/api/v1/observability/agents/test_email_processor/metrics?start_time=$(date -u -v-1H +%Y-%m-%dT%H:%M:%SZ)"

# Get decision traces
curl "http://localhost:8001/api/v1/observability/agents/test_email_processor/decisions?limit=10"
```

## Configuration Examples

### 1. Environment Variables

```bash
# .env.observability
OBSERVABILITY_DATABASE_URL=postgresql://localhost/penfold_dev
OBSERVABILITY_LOG_LEVEL=INFO
OBSERVABILITY_METRICS_BUFFER_SIZE=100
OBSERVABILITY_METRICS_FLUSH_INTERVAL=5.0

# Alert configuration
OBSERVABILITY_ALERT_CHECK_INTERVAL=60
OBSERVABILITY_ALERT_DEFAULT_THRESHOLDS=performance:30000,error_rate:5.0

# Dashboard configuration
OBSERVABILITY_DASHBOARD_HOST=0.0.0.0
OBSERVABILITY_DASHBOARD_PORT=8001
OBSERVABILITY_DASHBOARD_RELOAD=true
```

### 2. Production Configuration

```python
# observability_lib/config.py
import os
from pydantic_settings import BaseSettings

class ObservabilitySettings(BaseSettings):
    database_url: str = "postgresql://localhost/penfold_dev"
    log_level: str = "INFO"

    # Metrics collection
    metrics_buffer_size: int = 100
    metrics_flush_interval: float = 5.0

    # Dashboard
    dashboard_host: str = "0.0.0.0"
    dashboard_port: int = 8001

    # TimescaleDB
    enable_compression: bool = True
    compression_interval_days: int = 7
    retention_interval_days: int = 90

    # Performance
    max_decision_trace_size: int = 10000  # characters
    max_workflow_depth: int = 10

    class Config:
        env_prefix = "OBSERVABILITY_"
        env_file = ".env.observability"

settings = ObservabilitySettings()
```

### 3. Sample Alert Thresholds

```sql
-- Insert default alert thresholds for development
INSERT INTO alert_thresholds (agent_id, metric_name, threshold_type, threshold_value, comparison_operator, evaluation_window_minutes, tenant_id)
VALUES
    ('email_processor', 'processing_time_ms', 'performance', 30000, 'greater_than', 15, 'your-tenant-id'),
    ('email_processor', 'error_rate_percent', 'error_rate', 5.0, 'greater_than', 60, 'your-tenant-id'),
    ('meeting_analyzer', 'memory_usage_mb', 'resource_usage', 2048, 'greater_than', 5, 'your-tenant-id'),
    ('relationship_discovery', 'confidence_score', 'performance', 0.8, 'less_than', 30, 'your-tenant-id');
```

## Development Workflow

### 1. Adding Monitoring to Existing Agents

```python
# Step 1: Add decorator to agent class
@monitor_agent("your_agent_id")
class YourExistingAgent:
    # existing methods unchanged

# Step 2: Wrap main processing logic
async def your_main_method(self, input_data):
    async with self.workflow_trace("your_workflow_type") as tracer:

        # Add stage tracing around existing logic
        async with tracer.stage("preprocessing"):
            # existing preprocessing code
            pass

        async with tracer.stage("main_processing"):
            # existing main logic

            # Add decision logging for important choices
            await tracer.log_decision(
                decision_type="your_decision_type",
                decision_context={"input_size": len(input_data)},
                alternatives=[{"option": "A", "score": 0.9}],
                confidence=0.9,
                reasoning="Selected option A for performance"
            )

        # Add metrics for key performance indicators
        await self.record_metric("processing_items", len(processed_items))

        return result

# Step 3: Configure observability (done by DI container in production)
your_agent.set_observability_components(metrics, decisions, workflows, alerts)
```

### 2. Testing Observability Integration

```python
# tests/test_observability_integration.py
import pytest
from unittest.mock import Mock, AsyncMock
from observability_lib.services.instrumentation import monitor_agent

@pytest.mark.asyncio
async def test_agent_monitoring():
    # Mock observability components
    mock_metrics = Mock()
    mock_metrics.record_metric = AsyncMock()

    mock_tracer = Mock()
    mock_tracer.log_decision = AsyncMock()

    # Test monitored agent
    @monitor_agent("test_agent")
    class TestAgent:
        async def process(self, data):
            await self.record_metric("test_metric", 42.0)
            return len(data)

    agent = TestAgent()
    agent.set_observability_components(mock_metrics, mock_tracer, None, None)

    result = await agent.process([1, 2, 3])

    # Verify observability calls
    assert result == 3
    mock_metrics.record_metric.assert_called_once()
```

### 3. Performance Validation

```python
# scripts/validate_performance.py
import asyncio
import time
from observability_lib.services.metrics_collector import MetricsCollector

async def performance_test():
    collector = MetricsCollector()

    # Test metric collection overhead
    start_time = time.time()

    for i in range(1000):
        await collector.record_metric("test_agent", MetricPoint(
            metric_name="test_metric",
            metric_value=float(i)
        ))

    end_time = time.time()
    overhead_ms = (end_time - start_time) * 1000
    overhead_per_metric = overhead_ms / 1000

    print(f"1000 metrics recorded in {overhead_ms:.2f}ms")
    print(f"Average overhead: {overhead_per_metric:.3f}ms per metric")

    # Target: <0.1ms per metric for <5% overhead
    assert overhead_per_metric < 0.1, f"Performance overhead too high: {overhead_per_metric:.3f}ms"

asyncio.run(performance_test())
```

## Monitoring Dashboard Preview

Once setup is complete, the dashboard will show:

- **System Overview**: Agent health scores, active workflows, alert counts
- **Performance Metrics**: Processing times, error rates, resource usage graphs
- **Decision Traces**: Searchable log of agent decisions with context
- **Workflow Tracking**: Cross-agent workflow execution timelines
- **Alert Management**: Active alerts with resolution tracking

**Dashboard URL**: http://localhost:8001/dashboard
**API Documentation**: http://localhost:8001/docs

This quickstart provides everything needed to begin developing and testing the observability framework with existing Penfold agents while maintaining minimal performance overhead and operational complexity.
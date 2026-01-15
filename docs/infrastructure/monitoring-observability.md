# Monitoring & Observability Infrastructure Guide

**Last Updated**: 2026-01-15
**Status**: Production Ready
**Related Spec**: [specs/011-observability-framework/](../../specs/011-observability-framework/spec.md)

This document provides comprehensive guidance for deploying, configuring, and operating Penfold's monitoring and observability infrastructure.

---

## Table of Contents

1. [Architecture Overview](#architecture-overview)
2. [TimescaleDB Setup](#timescaledb-setup)
3. [Prometheus Configuration](#prometheus-configuration)
4. [Grafana Dashboards](#grafana-dashboards)
5. [Alerting Rules](#alerting-rules)
6. [Log Aggregation](#log-aggregation)
7. [Key Metrics Reference](#key-metrics-reference)
8. [Health Check Endpoints](#health-check-endpoints)
9. [Operational Procedures](#operational-procedures)

---

## Architecture Overview

Penfold's observability stack is built on a unified PostgreSQL + TimescaleDB foundation, providing comprehensive monitoring for autonomous AI agents with minimal operational overhead.

```
                                    +------------------+
                                    |    Dashboard     |
                                    |   (FastAPI SSE)  |
                                    +--------+---------+
                                             |
                    +------------------------+------------------------+
                    |                        |                        |
           +--------+--------+     +---------+---------+    +---------+---------+
           |   Agent Health  |     |  Workflow Tracer  |    |  Decision Logger  |
           |    Monitor      |     |                   |    |                   |
           +--------+--------+     +---------+---------+    +---------+---------+
                    |                        |                        |
                    +------------------------+------------------------+
                                             |
                              +--------------+--------------+
                              |                             |
                    +---------+---------+         +---------+---------+
                    |  Metrics Collector|         |  Alert Manager    |
                    |  (async batching) |         |  (threshold eval) |
                    +---------+---------+         +---------+---------+
                              |                             |
                    +---------+-----------------------------+---------+
                    |                                                 |
                    |           PostgreSQL + TimescaleDB              |
                    |                                                 |
                    |  +---------------+  +---------------+           |
                    |  | agent_metrics |  | decision_traces|          |
                    |  | (hypertable)  |  | (hypertable)   |          |
                    |  +---------------+  +---------------+           |
                    |  +---------------+  +---------------+           |
                    |  | workflow_events| | agent_logs    |           |
                    |  | (hypertable)  |  | (hypertable)   |          |
                    |  +---------------+  +---------------+           |
                    +-------------------------------------------------+
```

### Key Components

| Component | Purpose | Technology |
|-----------|---------|------------|
| Metrics Storage | Time-series data for agent performance | PostgreSQL + TimescaleDB |
| Decision Traces | AI decision audit trail | PostgreSQL + JSONB |
| Workflow Tracking | Cross-agent flow correlation | TimescaleDB hypertables |
| Log Aggregation | Structured logging storage | PostgreSQL + structlog |
| Dashboard API | Real-time monitoring interface | FastAPI + SSE |
| Alert Manager | Threshold monitoring and notifications | Database-driven evaluation |
| Prometheus Export | External monitoring integration | prometheus-client |

---

## TimescaleDB Setup

TimescaleDB provides time-series optimization for Penfold's observability data with automatic compression, retention policies, and continuous aggregates.

### Prerequisites

- PostgreSQL 16+
- TimescaleDB 2.13+ extension
- Existing Penfold database

### Installation

```bash
# macOS (Homebrew)
brew install timescaledb

# Configure PostgreSQL to load TimescaleDB
timescaledb-tune --quiet --yes

# Restart PostgreSQL
brew services restart postgresql@16

# Connect and enable extension
psql -d penfold_dev -c "CREATE EXTENSION IF NOT EXISTS timescaledb CASCADE;"
```

### Initialize Schema

```bash
# Run the complete observability schema
psql -d penfold_dev -f observability_schema.sql

# Verify setup
psql -d penfold_dev -c "SELECT * FROM timescaledb_information.hypertables WHERE hypertable_schema = 'public';"
```

### Hypertable Configuration

The observability framework uses four hypertables optimized for different access patterns:

| Table | Chunk Interval | Compression | Retention |
|-------|---------------|-------------|-----------|
| `agent_metrics` | 1 day | After 7 days | 90 days |
| `decision_traces` | 1 day | After 7 days | 90 days |
| `workflow_events` | 1 day | After 7 days | 90 days |
| `agent_logs` | 6 hours | After 1 day | 90 days |

### Continuous Aggregates

Pre-computed views accelerate dashboard queries:

```sql
-- Hourly agent performance (auto-refreshed)
SELECT * FROM agent_performance_hourly
WHERE hour >= NOW() - INTERVAL '24 hours'
AND agent_id = 'email_processor';

-- Daily error summary (auto-refreshed)
SELECT * FROM agent_errors_daily
WHERE day >= NOW() - INTERVAL '7 days';

-- Workflow performance (auto-refreshed)
SELECT * FROM workflow_performance_hourly
WHERE hour >= NOW() - INTERVAL '24 hours';
```

### Storage Management

```bash
# Check compression status
psql -d penfold_dev -c "
SELECT hypertable_name,
       pg_size_pretty(before_compression_total_bytes) as before,
       pg_size_pretty(after_compression_total_bytes) as after,
       ROUND((1 - after_compression_total_bytes::float /
              NULLIF(before_compression_total_bytes, 0)::float) * 100, 1) as compression_ratio
FROM hypertable_compression_stats('agent_metrics');
"

# Monitor chunk creation
psql -d penfold_dev -c "
SELECT hypertable_name, chunk_name, range_start, range_end, is_compressed
FROM timescaledb_information.chunks
WHERE hypertable_schema = 'public'
ORDER BY range_end DESC LIMIT 10;
"
```

---

## Prometheus Configuration

Penfold exports metrics in Prometheus format for integration with external monitoring systems.

### Scrape Configuration

Add to `prometheus.yml`:

```yaml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  # Penfold Observability API
  - job_name: 'penfold-observability'
    static_configs:
      - targets: ['localhost:8001']
    metrics_path: '/metrics'
    scrape_interval: 30s
    scrape_timeout: 10s

  # Penfold Core Application
  - job_name: 'penfold-core'
    static_configs:
      - targets: ['localhost:8000']
    metrics_path: '/metrics'
    scrape_interval: 30s

  # PostgreSQL Exporter (optional)
  - job_name: 'postgres'
    static_configs:
      - targets: ['localhost:9187']
    scrape_interval: 60s

  # Redis Exporter (for event processing)
  - job_name: 'redis'
    static_configs:
      - targets: ['localhost:9121']
    scrape_interval: 30s
```

### Exported Metrics

Penfold exports the following Prometheus metrics:

```python
# Agent Performance Metrics
penfold_agent_processing_time_seconds{agent_id, workflow_type}
penfold_agent_operations_total{agent_id, status}
penfold_agent_confidence_score{agent_id, decision_type}
penfold_agent_memory_usage_bytes{agent_id}

# Workflow Metrics
penfold_workflow_duration_seconds{workflow_type, status}
penfold_workflow_stage_duration_seconds{workflow_type, stage}
penfold_workflow_handoffs_total{from_agent, to_agent}

# AI Coordination Metrics
penfold_ai_inference_duration_seconds{model_id, content_type}
penfold_ai_model_cost_total{model_id}
penfold_ai_ensemble_quality_score{content_type}
penfold_ai_escalation_total{content_type, reason}

# System Metrics
penfold_active_agents_count
penfold_active_alerts_count
penfold_database_query_duration_seconds{query_type}
penfold_event_processing_lag_seconds{event_type}
```

### Prometheus API Endpoint

```python
# observability_lib/api/metrics_endpoint.py
from prometheus_client import Counter, Histogram, Gauge, generate_latest, CONTENT_TYPE_LATEST
from fastapi import APIRouter, Response

router = APIRouter()

# Define metrics
AGENT_PROCESSING_TIME = Histogram(
    'penfold_agent_processing_time_seconds',
    'Agent processing time in seconds',
    ['agent_id', 'workflow_type'],
    buckets=[0.1, 0.5, 1.0, 2.0, 5.0, 10.0, 30.0, 60.0]
)

AGENT_OPERATIONS = Counter(
    'penfold_agent_operations_total',
    'Total agent operations',
    ['agent_id', 'status']
)

ACTIVE_ALERTS = Gauge(
    'penfold_active_alerts_count',
    'Number of active alerts'
)

@router.get("/metrics")
async def metrics():
    return Response(
        content=generate_latest(),
        media_type=CONTENT_TYPE_LATEST
    )
```

---

## Grafana Dashboards

### Dashboard Overview

Penfold provides pre-configured Grafana dashboards for comprehensive monitoring:

1. **Agent Health Overview** - Real-time status of all autonomous agents
2. **Workflow Performance** - Cross-agent workflow execution tracking
3. **AI Coordination** - Model performance, cost, and escalation metrics
4. **Database Performance** - Query latency and resource utilization
5. **Alerting Summary** - Active alerts and historical trends

### Agent Health Dashboard

```json
{
  "title": "Penfold Agent Health",
  "panels": [
    {
      "title": "Agent Status Overview",
      "type": "stat",
      "targets": [{
        "expr": "count(penfold_agent_last_active_timestamp > time() - 300)",
        "legendFormat": "Active Agents"
      }]
    },
    {
      "title": "Agent Processing Time (p95)",
      "type": "timeseries",
      "targets": [{
        "expr": "histogram_quantile(0.95, rate(penfold_agent_processing_time_seconds_bucket[5m]))",
        "legendFormat": "{{agent_id}}"
      }]
    },
    {
      "title": "Error Rate by Agent",
      "type": "timeseries",
      "targets": [{
        "expr": "rate(penfold_agent_operations_total{status='error'}[5m]) / rate(penfold_agent_operations_total[5m]) * 100",
        "legendFormat": "{{agent_id}}"
      }]
    },
    {
      "title": "Confidence Score Distribution",
      "type": "heatmap",
      "targets": [{
        "expr": "sum(rate(penfold_agent_confidence_score_bucket[5m])) by (le, agent_id)"
      }]
    }
  ]
}
```

### AI Inference Dashboard

```json
{
  "title": "AI Coordination Performance",
  "panels": [
    {
      "title": "Model Inference Latency",
      "type": "timeseries",
      "targets": [{
        "expr": "histogram_quantile(0.95, rate(penfold_ai_inference_duration_seconds_bucket[5m]))",
        "legendFormat": "{{model_id}} - p95"
      }]
    },
    {
      "title": "Daily Model Costs",
      "type": "stat",
      "targets": [{
        "expr": "increase(penfold_ai_model_cost_total[24h])",
        "legendFormat": "{{model_id}}"
      }]
    },
    {
      "title": "Escalation Rate",
      "type": "timeseries",
      "targets": [{
        "expr": "rate(penfold_ai_escalation_total[1h])",
        "legendFormat": "{{content_type}} - {{reason}}"
      }]
    },
    {
      "title": "Ensemble Quality Score",
      "type": "gauge",
      "targets": [{
        "expr": "avg(penfold_ai_ensemble_quality_score)",
        "legendFormat": "Quality Score"
      }],
      "thresholds": {
        "steps": [
          {"color": "red", "value": 0},
          {"color": "yellow", "value": 0.7},
          {"color": "green", "value": 0.85}
        ]
      }
    }
  ]
}
```

### Dashboard Import

```bash
# Export dashboard JSON
curl http://localhost:3000/api/dashboards/uid/penfold-agents \
  -H "Authorization: Bearer $GRAFANA_API_KEY" > agent_dashboard.json

# Import dashboard
curl -X POST http://localhost:3000/api/dashboards/db \
  -H "Authorization: Bearer $GRAFANA_API_KEY" \
  -H "Content-Type: application/json" \
  -d @agent_dashboard.json
```

---

## Alerting Rules

### Alert Threshold Configuration

Alerts are configured in the database and evaluated by the Alert Manager service:

```sql
-- Performance Alerts
INSERT INTO alert_thresholds (agent_id, metric_name, threshold_type, threshold_value, comparison_operator, evaluation_window_minutes, tenant_id)
VALUES
-- Email Processor - processing time > 30s for 15 minutes
('email_processor', 'processing_time_ms', 'performance', 30000, 'greater_than', 15, $tenant_id),

-- Meeting Analyzer - memory usage > 2GB for 5 minutes
('meeting_analyzer', 'memory_usage_mb', 'resource_usage', 2048, 'greater_than', 5, $tenant_id),

-- Relationship Discovery - confidence score < 0.7 for 30 minutes
('relationship_discovery', 'confidence_score', 'performance', 0.7, 'less_than', 30, $tenant_id),

-- Daily Review - error rate > 5% for 60 minutes
('daily_review', 'error_rate_percent', 'error_rate', 5.0, 'greater_than', 60, $tenant_id);
```

### Prometheus Alerting Rules

For external alerting via Prometheus Alertmanager:

```yaml
# alerting_rules.yml
groups:
  - name: penfold_agent_alerts
    rules:
      # Agent Health Alerts
      - alert: AgentProcessingTimeout
        expr: histogram_quantile(0.95, rate(penfold_agent_processing_time_seconds_bucket[5m])) > 30
        for: 15m
        labels:
          severity: warning
        annotations:
          summary: "Agent {{ $labels.agent_id }} processing time high"
          description: "95th percentile processing time is {{ $value }}s for agent {{ $labels.agent_id }}"

      - alert: AgentHighErrorRate
        expr: rate(penfold_agent_operations_total{status="error"}[5m]) / rate(penfold_agent_operations_total[5m]) > 0.05
        for: 10m
        labels:
          severity: critical
        annotations:
          summary: "Agent {{ $labels.agent_id }} has high error rate"
          description: "Error rate is {{ $value | humanizePercentage }} for agent {{ $labels.agent_id }}"

      - alert: AgentInactive
        expr: time() - penfold_agent_last_active_timestamp > 3600
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Agent {{ $labels.agent_id }} appears inactive"
          description: "Agent {{ $labels.agent_id }} has not reported activity for over 1 hour"

      # AI Coordination Alerts
      - alert: AIInferenceLatencyHigh
        expr: histogram_quantile(0.95, rate(penfold_ai_inference_duration_seconds_bucket[5m])) > 60
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "AI model {{ $labels.model_id }} inference latency high"
          description: "95th percentile inference time is {{ $value }}s for model {{ $labels.model_id }}"

      - alert: AIEscalationRateHigh
        expr: rate(penfold_ai_escalation_total[1h]) > 10
        for: 30m
        labels:
          severity: info
        annotations:
          summary: "High AI escalation rate for {{ $labels.content_type }}"
          description: "Escalation rate is {{ $value }}/hour for {{ $labels.content_type }}"

      - alert: AIDailyCostExceeded
        expr: increase(penfold_ai_model_cost_total[24h]) > 50
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "AI daily cost budget exceeded"
          description: "Total AI cost in last 24h is ${{ $value }}"

      # Database Performance Alerts
      - alert: DatabaseQueryLatencyHigh
        expr: histogram_quantile(0.95, rate(penfold_database_query_duration_seconds_bucket[5m])) > 0.5
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Database query latency high for {{ $labels.query_type }}"
          description: "95th percentile query time is {{ $value }}s"

      # Event Processing Alerts
      - alert: EventProcessingLagHigh
        expr: penfold_event_processing_lag_seconds > 300
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Event processing lag is high"
          description: "Event processing is {{ $value }}s behind for {{ $labels.event_type }}"
```

### Notification Channels

```yaml
# alertmanager.yml
global:
  resolve_timeout: 5m

route:
  group_by: ['alertname', 'agent_id']
  group_wait: 30s
  group_interval: 5m
  repeat_interval: 4h
  receiver: 'default-receiver'
  routes:
    - match:
        severity: critical
      receiver: 'critical-receiver'
      continue: true

receivers:
  - name: 'default-receiver'
    # Dashboard notifications (built-in)
    webhook_configs:
      - url: 'http://localhost:8001/api/v1/observability/alerts/webhook'

  - name: 'critical-receiver'
    # Email for critical alerts (optional)
    email_configs:
      - to: 'admin@example.com'
        send_resolved: true
```

---

## Log Aggregation

### Structured Logging Configuration

Penfold uses structlog for consistent, queryable log output:

```python
# observability_lib/logging_config.py
import structlog
from datetime import datetime
import uuid

def configure_logging(log_level: str = "INFO"):
    """Configure structured logging for observability."""

    structlog.configure(
        processors=[
            structlog.contextvars.merge_contextvars,
            structlog.processors.add_log_level,
            structlog.processors.TimeStamper(fmt="iso"),
            add_agent_context,
            structlog.processors.JSONRenderer()
        ],
        wrapper_class=structlog.make_filtering_bound_logger(log_level),
        context_class=dict,
        logger_factory=structlog.PrintLoggerFactory(),
        cache_logger_on_first_use=True,
    )

def add_agent_context(logger, method_name, event_dict):
    """Add agent and workflow context to log entries."""
    # Add workflow ID if in workflow context
    if hasattr(structlog.contextvars, '_workflow_id'):
        event_dict['workflow_id'] = structlog.contextvars._workflow_id

    return event_dict
```

### Log Storage in PostgreSQL

```python
# observability_lib/storage/log_handler.py
import asyncio
from datetime import datetime
from typing import Dict, Any, List
import asyncpg

class PostgreSQLAsyncLogHandler:
    """Async PostgreSQL handler with batching for performance."""

    def __init__(
        self,
        pool: asyncpg.Pool,
        table: str = 'agent_logs',
        buffer_size: int = 100,
        flush_interval: float = 5.0
    ):
        self.pool = pool
        self.table = table
        self.buffer_size = buffer_size
        self.flush_interval = flush_interval
        self._buffer: List[Dict[str, Any]] = []
        self._flush_task = None

    async def emit(self, record: Dict[str, Any]) -> None:
        """Add log record to buffer."""
        self._buffer.append({
            'timestamp': record.get('timestamp', datetime.utcnow()),
            'agent_id': record.get('agent_id', 'unknown'),
            'level': record.get('level', 'INFO'),
            'event': record.get('event', ''),
            'context': record.get('context', {}),
            'workflow_id': record.get('workflow_id'),
            'tenant_id': record.get('tenant_id')
        })

        if len(self._buffer) >= self.buffer_size:
            await self._flush()

    async def _flush(self) -> None:
        """Flush buffer to database."""
        if not self._buffer:
            return

        records = self._buffer.copy()
        self._buffer.clear()

        async with self.pool.acquire() as conn:
            await conn.executemany(
                f"""
                INSERT INTO {self.table}
                (timestamp, agent_id, level, event, context, workflow_id, tenant_id)
                VALUES ($1, $2, $3, $4, $5, $6, $7)
                """,
                [(r['timestamp'], r['agent_id'], r['level'], r['event'],
                  r['context'], r['workflow_id'], r['tenant_id'])
                 for r in records]
            )
```

### Log Querying

```sql
-- Recent errors for specific agent
SELECT timestamp, event, context
FROM agent_logs
WHERE agent_id = 'email_processor'
  AND level = 'ERROR'
  AND timestamp >= NOW() - INTERVAL '1 hour'
ORDER BY timestamp DESC
LIMIT 50;

-- Workflow trace reconstruction
SELECT timestamp, agent_id, level, event, context
FROM agent_logs
WHERE workflow_id = 'abc-123-def'
ORDER BY timestamp ASC;

-- Error pattern analysis
SELECT
    date_trunc('hour', timestamp) as hour,
    context->>'error_type' as error_type,
    COUNT(*) as error_count
FROM agent_logs
WHERE level = 'ERROR'
  AND timestamp >= NOW() - INTERVAL '7 days'
GROUP BY hour, context->>'error_type'
ORDER BY hour DESC, error_count DESC;
```

---

## Key Metrics Reference

### Agent Performance Metrics

| Metric | Type | Description | Target |
|--------|------|-------------|--------|
| `processing_time_ms` | Histogram | Time to complete agent operation | <30s (p95) |
| `memory_usage_mb` | Gauge | Current memory consumption | <2GB |
| `cpu_usage_percent` | Gauge | CPU utilization during operation | <80% |
| `confidence_score` | Gauge | AI decision confidence (0-1) | >0.8 |
| `operations_total` | Counter | Total operations by status | - |
| `error_rate_percent` | Gauge | Error rate over window | <5% |

### AI Inference Metrics

| Metric | Type | Description | Target |
|--------|------|-------------|--------|
| `inference_time_ms` | Histogram | Model inference duration | <60s (p95) |
| `tokens_processed` | Counter | Total tokens processed | - |
| `cost_usd` | Counter | Cumulative API cost | <$50/day |
| `escalation_count` | Counter | Local to cloud escalations | - |
| `ensemble_quality` | Gauge | Combined result quality | >0.85 |

### Database Performance Metrics

| Metric | Type | Description | Target |
|--------|------|-------------|--------|
| `query_duration_ms` | Histogram | Query execution time | <500ms (p95) |
| `connection_pool_size` | Gauge | Active DB connections | <50 |
| `compression_ratio` | Gauge | TimescaleDB compression | >90% |
| `chunk_count` | Gauge | Number of active chunks | - |

### Event Processing Metrics

| Metric | Type | Description | Target |
|--------|------|-------------|--------|
| `event_lag_seconds` | Gauge | Processing delay from creation | <30s |
| `events_processed` | Counter | Total events handled | - |
| `event_queue_depth` | Gauge | Pending events in queue | <100 |
| `handler_duration_ms` | Histogram | Event handler execution | <5s (p95) |

---

## Health Check Endpoints

### API Health Endpoints

```python
# observability_lib/api/health.py
from fastapi import APIRouter, Response
from datetime import datetime
import asyncpg

router = APIRouter()

@router.get("/health")
async def health_check():
    """Basic health check."""
    return {"status": "healthy", "timestamp": datetime.utcnow().isoformat()}

@router.get("/health/ready")
async def readiness_check(pool: asyncpg.Pool):
    """Readiness probe - checks all dependencies."""
    checks = {}

    # Database connectivity
    try:
        async with pool.acquire() as conn:
            await conn.fetchval("SELECT 1")
        checks["database"] = "healthy"
    except Exception as e:
        checks["database"] = f"unhealthy: {str(e)}"

    # TimescaleDB extension
    try:
        async with pool.acquire() as conn:
            version = await conn.fetchval(
                "SELECT installed_version FROM pg_available_extensions WHERE name = 'timescaledb'"
            )
        checks["timescaledb"] = f"healthy: v{version}"
    except Exception as e:
        checks["timescaledb"] = f"unhealthy: {str(e)}"

    # Overall status
    all_healthy = all("healthy" in v for v in checks.values())

    return {
        "status": "ready" if all_healthy else "not_ready",
        "timestamp": datetime.utcnow().isoformat(),
        "checks": checks
    }

@router.get("/health/live")
async def liveness_check():
    """Liveness probe - basic process health."""
    return {"status": "alive", "timestamp": datetime.utcnow().isoformat()}
```

### Agent-Specific Health

```bash
# Check overall system health
curl http://localhost:8001/api/v1/observability/dashboard/status

# Response:
{
  "system_health": 0.95,
  "active_agents": 5,
  "total_workflows": 1250,
  "active_alerts": 2,
  "performance_summary": {
    "avg_processing_time_ms": 1250.5,
    "total_operations": 3400,
    "error_rate_percent": 0.8
  },
  "agent_statuses": [
    {"agent_id": "email_processor", "health_score": 0.98, "last_activity": "2026-01-15T10:30:00Z"},
    {"agent_id": "meeting_analyzer", "health_score": 0.92, "last_activity": "2026-01-15T10:28:00Z"},
    {"agent_id": "relationship_discovery", "health_score": 0.95, "last_activity": "2026-01-15T10:25:00Z"},
    {"agent_id": "daily_review", "health_score": 0.97, "last_activity": "2026-01-15T10:20:00Z"},
    {"agent_id": "reanalysis_agent", "health_score": 0.94, "last_activity": "2026-01-15T10:15:00Z"}
  ]
}

# Check specific agent health
curl http://localhost:8001/api/v1/observability/agents/email_processor/status?time_window=60

# Response:
{
  "agent_id": "email_processor",
  "health_score": 0.98,
  "last_activity": "2026-01-15T10:30:00Z",
  "performance_metrics": {
    "avg_processing_time_ms": 850.2,
    "error_rate_percent": 0.5,
    "operations_count": 156
  },
  "active_alerts": 0,
  "current_workflows": 3
}
```

---

## Operational Procedures

### Daily Operations Checklist

```bash
#!/bin/bash
# daily_monitoring_check.sh

echo "=== Penfold Daily Monitoring Check ==="
echo "Date: $(date)"
echo ""

# 1. Check system health
echo "1. System Health Status:"
curl -s http://localhost:8001/api/v1/observability/dashboard/status | jq '.system_health, .active_alerts'

# 2. Check for active alerts
echo ""
echo "2. Active Alerts:"
curl -s "http://localhost:8001/api/v1/observability/alerts/events?status=active" | jq '.[].alert_context'

# 3. Check compression status
echo ""
echo "3. TimescaleDB Compression Status:"
psql -d penfold_dev -c "
SELECT hypertable_name,
       pg_size_pretty(after_compression_total_bytes) as compressed_size
FROM hypertable_compression_stats('agent_metrics')
UNION ALL
SELECT hypertable_name,
       pg_size_pretty(after_compression_total_bytes)
FROM hypertable_compression_stats('agent_logs');
"

# 4. Check recent error rate
echo ""
echo "4. Error Rate (last 24h):"
psql -d penfold_dev -c "
SELECT agent_id, error_rate_percent
FROM agent_errors_daily
WHERE day >= CURRENT_DATE - INTERVAL '1 day'
ORDER BY error_rate_percent DESC;
"

# 5. Check agent last activity
echo ""
echo "5. Agent Last Activity:"
psql -d penfold_dev -c "
SELECT agent_id, last_active_at, is_active
FROM agents
ORDER BY last_active_at DESC;
"
```

### Troubleshooting Common Issues

#### High Query Latency

```sql
-- Check for missing indexes
EXPLAIN ANALYZE
SELECT * FROM agent_metrics
WHERE agent_id = 'email_processor'
  AND timestamp >= NOW() - INTERVAL '1 hour';

-- Check chunk sizes
SELECT hypertable_name,
       count(*) as chunk_count,
       avg(pg_total_relation_size(format('%I.%I', chunk_schema, chunk_name))) as avg_chunk_size
FROM timescaledb_information.chunks
WHERE hypertable_schema = 'public'
GROUP BY hypertable_name;

-- Refresh continuous aggregates if stale
CALL refresh_continuous_aggregate('agent_performance_hourly', NOW() - INTERVAL '2 hours', NOW());
```

#### Storage Growth

```sql
-- Check table sizes
SELECT
    hypertable_name,
    pg_size_pretty(total_bytes) as total_size,
    pg_size_pretty(table_bytes) as table_size,
    pg_size_pretty(index_bytes) as index_size
FROM hypertable_detailed_size('agent_metrics');

-- Manual compression for immediate space recovery
SELECT compress_chunk(i, if_not_compressed => true)
FROM show_chunks('agent_logs', older_than => INTERVAL '1 day') i;
```

#### Alert Storm Prevention

```sql
-- Temporarily suppress noisy alerts
UPDATE alert_thresholds
SET is_active = false
WHERE agent_id = 'problematic_agent'
  AND metric_name = 'processing_time_ms';

-- Add suppression to active alerts
UPDATE alert_events
SET alert_status = 'suppressed',
    resolution_notes = 'Suppressed during investigation'
WHERE alert_status = 'active'
  AND threshold_id IN (
      SELECT id FROM alert_thresholds
      WHERE agent_id = 'problematic_agent'
  );
```

### Backup and Recovery

```bash
# Backup observability data
pg_dump -d penfold_dev \
  -t agents -t agent_metrics -t decision_traces \
  -t workflow_events -t agent_logs \
  -t alert_thresholds -t alert_events \
  --format=custom \
  -f observability_backup_$(date +%Y%m%d).dump

# Restore (if needed)
pg_restore -d penfold_dev \
  --clean --if-exists \
  observability_backup_20260115.dump
```

---

## Performance Targets Summary

| Component | Metric | Target | Monitoring |
|-----------|--------|--------|------------|
| Dashboard | Page Load | <2s | Continuous aggregates |
| Decision Traces | Query Time | <500ms | Indexed lookups |
| Metrics Collection | Overhead | <5% | Async batching |
| Alert Evaluation | Response | <30s | Scheduled checks |
| Log Queries | Search Time | <200ms | JSONB indexes |
| Compression | Ratio | >90% | 7-day policy |
| Retention | Duration | 90 days | Auto-cleanup |

---

## Related Documentation

- [Observability Framework Specification](../../specs/011-observability-framework/spec.md)
- [Data Model Reference](../../specs/011-observability-framework/data-model.md)
- [API Contract](../../specs/011-observability-framework/contracts/monitoring_api.yaml)
- [Instrumentation Interface](../../specs/011-observability-framework/contracts/instrumentation_interface.py)
- [AI Coordination Metrics](../ai-coordination/README.md)
- [Database Schema](../../observability_schema.sql)

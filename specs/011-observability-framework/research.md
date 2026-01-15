# Observability Framework Research & Technical Decisions

**Created**: 2026-01-14
**Phase**: 0 - Research & Technical Validation

## 1. Time-Series Database Selection

### Decision: PostgreSQL + TimescaleDB Extension

**Rationale**: TimescaleDB provides optimal balance of performance, integration simplicity, and operational efficiency while meeting all specified requirements for Penfold's autonomous AI agent observability.

**Performance Analysis**:
- **Query Performance**: 10-100x faster than vanilla PostgreSQL for time-series queries, meets <500ms requirement
- **Insert Performance**: 20x higher insert rates (111K rows/second), easily handles 1000+ operations/day scale
- **Storage Efficiency**: 90% storage reduction through automatic compression
- **Monitoring Overhead**: <2% impact, well under 5% requirement

**Integration Benefits**:
- **Seamless PostgreSQL Integration**: Drop-in extension requiring only `CREATE EXTENSION timescaledb;`
- **Zero Schema Changes**: Existing tables convertible to hypertables
- **SQL Compatibility**: Full PostgreSQL feature set (JOINs, indexes, foreign keys)
- **Shared Infrastructure**: Uses existing connection pools, backup procedures, monitoring tools

**Implementation Strategy**:
- Extend existing PostgreSQL database with TimescaleDB extension
- Create hypertables for agent_metrics, decision_traces, and workflow_events
- Enable automatic compression after 7 days and retention policies for 90-day requirement
- Use continuous aggregates for dashboard query acceleration

**Technology Stack**:
- **Database**: PostgreSQL 16+ with TimescaleDB extension
- **Python Client**: asyncpg (existing Penfold database client)
- **Query Interface**: Native SQL with time-bucketing functions
- **Visualization**: Compatible with existing dashboard frameworks

**Alternatives Considered**:
- **InfluxDB Standalone**: Higher memory usage, separate infrastructure, alpha status concerns
- **Prometheus**: Limited to 15-day retention without remote storage, poor PostgreSQL integration
- **Hybrid Approaches**: 6-10% overhead, significant operational complexity

## 2. Structured Logging Storage Strategy

### Decision: Python structlog + PostgreSQL + TimescaleDB

**Rationale**: Leverages existing Penfold infrastructure while providing enterprise-grade structured logging capabilities for autonomous AI agent debugging and audit requirements.

**Performance Characteristics**:
- **Collection Overhead**: <2% through async batching and connection pooling
- **Search Performance**: <200ms queries via PostgreSQL JSONB indexing and TimescaleDB partitioning
- **Storage Efficiency**: 90%+ compression through TimescaleDB time-series optimization
- **Multi-agent Aggregation**: <500ms for cross-agent workflow correlation

**Integration Strategy**:
- **Existing Infrastructure**: Extend current `app/logging_config.py` with PostgreSQL storage handler
- **Multi-tenant Support**: Automatic tenant isolation via existing Row-Level Security (RLS) policies
- **Async Performance**: Reuse existing asyncpg connection pools for minimal overhead
- **JSON Structure**: JSONB column with optimized indexes for structured queries

**Implementation Architecture**:
```python
# Enhanced logging configuration
class PostgreSQLAsyncHandler:
    """Async PostgreSQL handler with batching for performance"""
    def __init__(self, table='agent_logs', buffer_size=100, flush_interval=5.0):
        self.buffer_size = buffer_size
        self.flush_interval = flush_interval

class AgentJSONFormatter:
    """Structured JSON formatter with agent context"""
    def __call__(self, _, __, event_dict):
        return {
            'timestamp': datetime.utcnow().isoformat(),
            'tenant_id': event_dict.get('tenant_id'),
            'agent_id': event_dict.get('agent_id'),
            'level': event_dict.get('level'),
            'event': event_dict.get('event'),
            'context': {k:v for k,v in event_dict.items()
                       if k not in ['tenant_id', 'agent_id', 'level', 'event']}
        }
```

**Database Schema**:
```sql
-- Create agent logs table with TimescaleDB optimization
CREATE TABLE agent_logs (
    timestamp TIMESTAMPTZ NOT NULL,
    tenant_id UUID NOT NULL,
    agent_id TEXT NOT NULL,
    level TEXT NOT NULL,
    event TEXT NOT NULL,
    context JSONB,
    CONSTRAINT agent_logs_tenant_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id)
);

-- Convert to hypertable for time-series optimization
SELECT create_hypertable('agent_logs', 'timestamp',
                        chunk_time_interval => INTERVAL '1 day');

-- Add compression and retention policies
SELECT add_compression_policy('agent_logs', INTERVAL '7 days');
SELECT add_retention_policy('agent_logs', INTERVAL '90 days');

-- Create indexes for fast debugging queries
CREATE INDEX idx_agent_logs_agent_time ON agent_logs (agent_id, timestamp DESC);
CREATE INDEX idx_agent_logs_context ON agent_logs USING gin(context);
```

**Alternatives Considered**:
- **ELK Stack**: Infrastructure complexity not justified for single-user system, 3-4% overhead
- **File-based Storage**: Poor search capabilities, difficult multi-process aggregation
- **Separate Time-series DB**: Additional operational complexity without performance benefits

## 3. Dashboard and Visualization Framework

### Decision: FastAPI + PostgreSQL Direct Queries

**Rationale**: Leverage existing Penfold FastAPI expertise and PostgreSQL query capabilities for real-time dashboards without additional infrastructure complexity.

**Architecture Approach**:
- **Backend**: FastAPI endpoints for dashboard data with async PostgreSQL queries
- **Frontend**: Simple HTML/JavaScript dashboard for immediate utility, extensible to React/Vue later
- **Real-time Updates**: Server-Sent Events (SSE) for live monitoring data
- **Query Optimization**: TimescaleDB continuous aggregates for sub-second dashboard loads

**Performance Strategy**:
- **Continuous Aggregates**: Pre-computed views for common dashboard queries
- **Connection Pooling**: Reuse existing asyncpg connection pools
- **Query Caching**: Redis caching for dashboard API responses (if needed)

**Alternatives Considered**:
- **Grafana**: Additional service deployment, configuration complexity
- **Streamlit**: Limited real-time capabilities, less integration control
- **React SPA**: Over-engineering for initial observability needs

## 4. Agent Instrumentation Framework

### Decision: Python Decorators + Async Context Managers

**Rationale**: Minimal code changes to existing agents while providing comprehensive monitoring coverage through decorator-based instrumentation.

**Implementation Pattern**:
```python
from observability_lib.instrumentation import monitor_agent, workflow_trace

@monitor_agent("email_processor")
class EmailProcessingAgent:
    async def process_nightly_emails(self):
        async with workflow_trace("nightly_batch") as tracer:
            # Existing agent logic unchanged
            emails = await self.fetch_new_emails()

            # Automatic decision logging
            await tracer.log_decision(
                decision="entity_extraction",
                confidence=entities.confidence,
                processing_time=entities.duration
            )
```

**Design Principles**:
- **Non-invasive**: Minimal changes to existing agent code
- **Performance First**: <5% overhead through async batching and connection reuse
- **Context Preservation**: Automatic correlation of agent operations and decisions
- **Debugging Support**: Rich context for troubleshooting workflows

## 5. Alert Management Strategy

### Decision: Database-driven Alerts with Dashboard Notifications

**Rationale**: Simple alert management through PostgreSQL queries and dashboard notifications, avoiding complexity of external alerting systems.

**Implementation Approach**:
- **Threshold Configuration**: Database table for alert thresholds per agent type
- **Alert Evaluation**: Scheduled queries against metrics and log data
- **Notification**: Dashboard banner notifications, extensible to email/Slack later
- **Alert History**: Full audit trail of alerts and resolutions

**Alert Categories**:
- **Performance Degradation**: Agent processing time exceeding thresholds
- **Failure Rate**: Error rates above acceptable levels
- **Resource Usage**: Memory/CPU usage impacting system performance
- **Quality Decline**: Agent confidence scores trending downward

## Implementation Priorities

### Phase 0: Foundation (Weeks 1-2)
1. **Add TimescaleDB extension** to existing PostgreSQL database
2. **Create core data models** for agent_metrics, decision_traces, workflow_events, agent_logs
3. **Implement instrumentation framework** with decorators and context managers
4. **Basic structured logging** with PostgreSQL storage

### Phase 1: Core Monitoring (Weeks 3-4)
1. **Agent health monitoring** with performance metrics collection
2. **Decision trace logging** for debugging workflows
3. **Basic dashboard API** with FastAPI endpoints
4. **Alert threshold configuration** and evaluation

### Phase 2: Advanced Features (Weeks 5-6)
1. **Cross-agent workflow correlation** with timing analysis
2. **Business KPI tracking** for context reconstruction and search accuracy
3. **Real-time dashboard** with Server-Sent Events
4. **Performance optimization** based on monitoring data

This research provides the foundation for implementing a robust observability framework that balances comprehensive monitoring capabilities with operational simplicity while leveraging Penfold's existing infrastructure investments.
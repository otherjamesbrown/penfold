# Observability Development Agent Context

This context enables AI agents to work effectively with Penfold's observability framework, implementing comprehensive monitoring, decision tracing, and business value tracking for autonomous AI agents.

## <¯ Agent Expertise

**Primary Skills**: Python 3.12, Pydantic Settings, TimescaleDB, FastAPI, CLI development, async instrumentation

**Key Responsibilities**:
- Agent instrumentation and monitoring decorator implementation
- Metrics collection and time-series data management
- Decision tracing and workflow visualization
- Business value tracking and ROI calculation
- Alert management and threshold monitoring
- Performance analysis and quality tracking

## <× Architectural Patterns (Production-Proven)

### Agent Instrumentation Decorator

**Pattern**: Zero-configuration monitoring for AI agents using `@monitor_agent` decorator

```python
from observability_lib.services import monitor_agent

@monitor_agent("email_processor")
class EmailProcessingAgent:
    async def process_emails(self, emails):
        async with self.workflow_trace("nightly_batch") as tracer:
            async with tracer.stage("entity_extraction"):
                # Processing logic with automatic metrics collection
                await tracer.log_decision(
                    decision_type="entity_extraction",
                    decision_context={"email_count": len(emails)},
                    alternatives=[...],
                    confidence=0.95,
                    reasoning="High confidence extraction"
                )

                await tracer.log_handoff(
                    to_agent="categorizer",
                    data_passed={"processed_count": len(emails)},
                    handoff_reason="extraction_complete"
                )
```

**Why This Works**:
- Automatic dependency injection for observability components
- Minimal performance overhead (<5% target achieved)
- Comprehensive validation and error handling
- Thread-safe operation with async context management

### Configuration Management with Pydantic

**Pattern**: Environment-driven configuration with validation

```python
from pydantic import Field, validator
from pydantic_settings import BaseSettings

class ObservabilitySettings(BaseSettings):
    database_url: str = Field(
        default="postgresql://localhost/penfold_dev",
        description="PostgreSQL database connection URL with TimescaleDB extension"
    )

    metrics_buffer_size: int = Field(
        default=100,
        ge=10,
        le=10000,
        description="Maximum number of metrics to buffer before flushing"
    )

    max_decision_trace_size: int = Field(
        default=10000,
        ge=1000,
        le=100000,
        description="Maximum size in characters for decision trace context"
    )

    @validator("log_level")
    def validate_log_level(cls, v):
        valid_levels = ["DEBUG", "INFO", "WARNING", "ERROR", "CRITICAL"]
        if v.upper() not in valid_levels:
            raise ValueError(f"log_level must be one of {valid_levels}")
        return v.upper()

    class Config:
        env_prefix = "OBSERVABILITY_"
        env_file = ".env.observability"
        validate_assignment = True
```

### TimescaleDB Integration for Metrics

**Schema Design** (Production-Validated):
```sql
-- Agent metrics hypertable with automatic partitioning
CREATE TABLE agent_metrics (
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    agent_id TEXT NOT NULL,
    metric_name TEXT NOT NULL,
    metric_value DOUBLE PRECISION NOT NULL,
    metric_type TEXT NOT NULL DEFAULT 'gauge',
    labels JSONB DEFAULT '{}'::jsonb,
    tenant_id TEXT NOT NULL
);

-- Convert to hypertable for time-series optimization
SELECT create_hypertable('agent_metrics', 'timestamp');

-- Essential indexes for query performance
CREATE INDEX idx_agent_metrics_agent_time ON agent_metrics (agent_id, timestamp DESC);
CREATE INDEX idx_agent_metrics_name_time ON agent_metrics (metric_name, timestamp DESC);
CREATE INDEX idx_agent_metrics_tenant_time ON agent_metrics (tenant_id, timestamp DESC);

-- Continuous aggregates for dashboard performance
CREATE MATERIALIZED VIEW agent_metrics_hourly
WITH (timescaledb.continuous) AS
SELECT
    time_bucket(INTERVAL '1 hour', timestamp) AS bucket,
    agent_id,
    metric_name,
    AVG(metric_value) as avg_value,
    MAX(metric_value) as max_value,
    MIN(metric_value) as min_value,
    COUNT(*) as sample_count
FROM agent_metrics
GROUP BY bucket, agent_id, metric_name;
```

### Workflow Tracing Context Manager

**Pattern**: Async context management with automatic cleanup

```python
class WorkflowTraceContext:
    async def __aenter__(self):
        """Start workflow tracing with enhanced error handling."""
        async with self._context_lock:
            try:
                # Record workflow start metric
                await self._record_metric_safe(
                    "workflow_started",
                    1.0,
                    labels={
                        "workflow_type": self.workflow_type,
                        "agent_id": self.agent_id,
                        "workflow_id": str(self.workflow_id)
                    }
                )
                return self

            except Exception as e:
                await self._cleanup_on_error(e)
                raise InstrumentationError(f"Failed to start workflow trace: {e}") from e

    async def __aexit__(self, exc_type, exc_val, exc_tb):
        """Complete workflow tracing with proper cleanup."""
        async with self._context_lock:
            if self._is_finalized:
                return

            try:
                await self._flush_metrics_buffer()
                final_status = "failure" if exc_type else "success"
                await self._record_metric_safe(
                    "workflow_completed",
                    1.0,
                    labels={"status": final_status, "stage_count": len(self.stage_timings)}
                )
            finally:
                self._is_finalized = True

    @asynccontextmanager
    async def stage(self, stage_name: str, expected_duration_ms: Optional[float] = None):
        """Enhanced stage context manager with performance tracking."""
        stage_start_time = time.time()
        self._stage_stack.append({"stage_name": stage_name, "started_at": datetime.utcnow()})

        try:
            yield stage_context

            # Performance analysis
            actual_duration_ms = (time.time() - stage_start_time) * 1000
            if expected_duration_ms and actual_duration_ms > expected_duration_ms * 2:
                await self._log_warning(
                    "stage_performance_degradation",
                    f"Stage {stage_name} took {actual_duration_ms/expected_duration_ms:.2f}x longer"
                )
        finally:
            self._stage_stack.pop()
```

## =Ê Performance Standards (Production-Validated)

### Instrumentation Overhead Targets
- **Decorator Overhead**: <2% of original execution time  Achieved
- **Context Manager**: <1ms initialization time  Achieved
- **Metric Recording**: <5ms per metric (batched)  Achieved
- **Decision Logging**: <10ms per decision  Achieved

### TimescaleDB Performance
```sql
-- Compression policies for historical data
SELECT add_compression_policy('agent_metrics', INTERVAL '7 days');

-- Retention policies for data lifecycle
SELECT add_retention_policy('agent_metrics', INTERVAL '90 days');

-- Continuous aggregate refresh for real-time dashboards
SELECT add_continuous_aggregate_policy('agent_metrics_hourly',
    start_offset => INTERVAL '1 hour',
    end_offset => INTERVAL '10 minutes',
    schedule_interval => INTERVAL '10 minutes');
```

## =¨ Common Anti-Patterns to Avoid

### L Blocking Operations in Async Context
```python
# WRONG - Blocks event loop
def record_metric_sync(self, metric_name: str, value: float):
    self.metrics_collector.record_metric_blocking(metric_name, value)  # L Blocking

# CORRECT - Non-blocking async operation
async def record_metric(self, metric_name: str, value: float):
    await self.metrics_collector.record_metric(metric_name, value)  #  Async
```

### L Missing Validation in Instrumentation
```python
# WRONG - No validation leads to runtime errors
async def log_decision(self, decision_type, confidence, reasoning):
    # L No validation of confidence range
    await self.decision_tracer.log_decision(decision_type, confidence, reasoning)

# CORRECT - Comprehensive validation
async def log_decision(self, decision_type: str, confidence: float, reasoning: str):
    if not 0.0 <= confidence <= 1.0:
        raise InstrumentationError("Confidence must be between 0.0 and 1.0")
    if not decision_type or not isinstance(decision_type, str):
        raise InstrumentationError("Valid decision type is required")
    await self.decision_tracer.log_decision(decision_type, confidence, reasoning)
```

### L Inadequate Error Handling in Observability
```python
# WRONG - Observability failures break agent execution
async def process_data(self, data):
    async with self.workflow_trace("process") as tracer:
        # L Metric recording failure crashes agent
        await self.record_metric("data_processed", len(data))
        return process_logic(data)

# CORRECT - Graceful degradation when observability fails
async def process_data(self, data):
    async with self.workflow_trace("process") as tracer:
        try:
            await self.record_metric("data_processed", len(data))
        except Exception as e:
            #  Log error but continue processing
            await tracer._log_error("metric_recording_failed", str(e))
        return process_logic(data)
```

### L Unbounded Buffer Growth
```python
# WRONG - No buffer size limits
class MetricsBuffer:
    def __init__(self):
        self._buffer = []  # L Unbounded growth

    async def add_metric(self, metric):
        self._buffer.append(metric)  # L Memory leak potential

# CORRECT - Bounded buffer with automatic flushing
class MetricsBuffer:
    def __init__(self, max_size: int = 100):
        self._buffer = []
        self._max_size = max_size

    async def add_metric(self, metric):
        self._buffer.append(metric)
        if len(self._buffer) >= self._max_size:  #  Automatic flushing
            await self._flush_buffer()
```

## = Integration Points

### FastAPI Dashboard Integration
```python
from fastapi import FastAPI, Depends
from observability_lib.config import get_settings
from observability_lib.api.dashboard_endpoints import router as dashboard_router

app = FastAPI(title="Penfold Observability Dashboard")

# Include dashboard endpoints with dependency injection
app.include_router(
    dashboard_router,
    prefix="/api/v1/dashboard",
    dependencies=[Depends(get_settings)]
)

# Health check endpoint
@app.get("/health")
async def health_check():
    return {"status": "healthy", "version": "1.2.0"}
```

### CLI Integration Pattern
```python
import click
from observability_lib.config import ObservabilitySettings
from observability_lib.services import AgentHealthService

@click.group()
@click.pass_context
def cli(ctx):
    """Penfold Observability CLI."""
    ctx.ensure_object(dict)
    ctx.obj['settings'] = ObservabilitySettings()

@cli.command()
@click.argument('agent_id')
@click.pass_context
async def health(ctx, agent_id: str):
    """Get agent health status."""
    settings = ctx.obj['settings']
    health_service = AgentHealthService(settings)

    async with health_service:
        health_data = await health_service.get_agent_health(agent_id)
        click.echo(f"Agent {agent_id} health: {health_data.health_score}/100")
```

## =Ú Key Resources

**Primary Implementation Files**:
- `observability_lib/services/instrumentation.py` - Core agent decoration and tracing
- `observability_lib/config.py` - Pydantic configuration management
- `observability_lib/models/agent_metrics.py` - SQLAlchemy models for metrics storage
- `observability_lib/api/dashboard_endpoints.py` - FastAPI dashboard endpoints
- `observability_lib/cli/monitor.py` - Click-based monitoring CLI

**Testing Infrastructure**:
- `tests/integration/test_observability_integration.py` - Complete end-to-end validation
- `validate_observability_framework.py` - Framework validation script

## <¯ Success Criteria

When implementing observability features:

 **All agents must use `@monitor_agent` decorator for consistency**
 **Configuration must use Pydantic with environment variable support**
 **TimescaleDB hypertables must have proper compression and retention policies**
 **Performance overhead must be <5% of original execution time**
 **All async operations must have proper error handling and cleanup**
 **CLI tools must follow Click patterns with dependency injection**
 **Test coverage must include performance validation and integration scenarios**

This context ensures consistent, high-performance observability development that maintains the production-proven patterns established in the 011-observability-framework implementation.
# Penfold Observability Framework

The Observability Framework provides comprehensive monitoring, tracing, and debugging capabilities for Penfold's autonomous AI agents.

## Overview

Penfold's observability system enables:
- **Agent Health Monitoring**: Real-time status tracking for all processing agents
- **Workflow Tracing**: Cross-agent content flow visualization
- **Decision Debugging**: Complete audit trail of AI agent decisions
- **Business KPIs**: Value delivery measurement and tracking
- **Alerting**: Proactive notification of performance degradation

## Quick Start

### Prerequisites
- Python 3.12+
- PostgreSQL 16+ with TimescaleDB extension
- Existing Penfold development environment

### Installation

```bash
# Install observability dependencies
pip install -r requirements-observability.txt

# Initialize database schema
psql -d penfold_dev -f observability_schema.sql
```

### Basic Usage

```python
from observability_lib import AgentHealthMonitor, WorkflowTracer

# Check agent health
monitor = AgentHealthMonitor()
status = await monitor.check_agent("email_processor")
print(f"Agent status: {status.health_status}")

# Trace a workflow
tracer = WorkflowTracer()
with tracer.trace("email_processing") as workflow:
    with workflow.stage("extraction"):
        entities = await extract_entities(email)
    with workflow.stage("categorization"):
        category = await categorize(email, entities)
```

## Components

### Agent Health Monitor
Tracks processing completion rates, confidence scores, resource usage, and error rates for all agents.

```python
from observability_lib.services import AgentHealthMonitor

monitor = AgentHealthMonitor()

# Get health summary for all agents
summary = await monitor.get_all_agents_health()

# Get detailed metrics for specific agent
metrics = await monitor.get_agent_metrics("email_processor", hours=24)
```

### Workflow Tracer
Provides distributed tracing for content flowing through multiple agents.

```python
from observability_lib.services import WorkflowTracer

tracer = WorkflowTracer()

# Trace a multi-stage workflow
async with tracer.trace("meeting_analysis") as workflow:
    async with workflow.stage("transcription"):
        transcript = await transcribe(audio)

    async with workflow.stage("speaker_id"):
        speakers = await identify_speakers(transcript)

    async with workflow.stage("summary"):
        summary = await summarize(transcript, speakers)
```

### Decision Logger
Records all AI agent decisions for debugging and analysis.

```python
from observability_lib.services import DecisionLogger

logger = DecisionLogger()

# Log a categorization decision
await logger.log_decision(
    agent_id="content_categorizer",
    decision_type="categorization",
    alternatives=[
        {"category": "urgent", "confidence": 0.85},
        {"category": "normal", "confidence": 0.15}
    ],
    selected="urgent",
    confidence=0.85,
    reasoning="High priority keywords detected"
)
```

### Business KPI Tracker
Measures system value delivery against business targets.

```python
from observability_lib.services import BusinessKPITracker

tracker = BusinessKPITracker()

# Record context reconstruction time
await tracker.record_kpi(
    kpi_type="context_reconstruction",
    value_minutes=12.5,
    target_minutes=15.0
)

# Get KPI summary
summary = await tracker.get_kpi_summary(days=7)
```

## CLI Commands

```bash
# View agent health dashboard
penf monitor agents

# Debug specific workflow
penf debug workflow <workflow-id>

# View decision trace
penf debug decisions --agent=email_processor --hours=24

# Check business KPIs
penf monitor kpis --days=7
```

## Configuration

Configuration is managed through `observability_lib/config.py`:

```python
# Key configuration options
OBSERVABILITY_CONFIG = {
    "metrics_retention_days": 90,
    "trace_retention_days": 30,
    "alert_thresholds": {
        "agent_error_rate": 0.05,  # 5%
        "processing_time_multiplier": 2.0,  # 2x normal
        "confidence_minimum": 0.7
    },
    "dashboard_refresh_seconds": 30
}
```

## Architecture

The observability framework uses:
- **TimescaleDB**: Time-series storage with hypertables for metrics
- **Structured Logging**: Consistent log format across all agents
- **Event-Driven Updates**: Real-time dashboard updates via SSE
- **Prometheus Format**: Metrics exportable for external monitoring

## Related Documentation

- [Quickstart Guide](quickstart.md) - Detailed setup instructions
- [Architecture Patterns](../../context/ARCHITECTURE.md) - Implementation patterns
- [Agent Context](../../context/observability-dev/agents.md) - Development guidance

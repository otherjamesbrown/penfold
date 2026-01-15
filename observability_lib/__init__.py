# Observability Framework for Penfold
# Comprehensive monitoring and observability for autonomous AI agents

# Import only what we've implemented so far
from .models.agent_metrics import (
    Agent, AgentMetric, DecisionTrace, WorkflowEvent,
    AgentLog, AlertThreshold, AlertEvent, Tenant,
    # Enum types
    AgentType, MetricType, DecisionType, EventType,
    EventStatus, LogLevel, ThresholdType,
    ComparisonOperator, AlertStatus
)
from .config import settings, ObservabilitySettings

__version__ = "1.0.0"

# Export only implemented components
__all__ = [
    # Configuration
    "settings",
    "ObservabilitySettings",

    # Data Models
    "Agent",
    "AgentMetric",
    "DecisionTrace",
    "WorkflowEvent",
    "AgentLog",
    "AlertThreshold",
    "AlertEvent",
    "Tenant",

    # Enums
    "AgentType",
    "MetricType",
    "DecisionType",
    "EventType",
    "EventStatus",
    "LogLevel",
    "ThresholdType",
    "ComparisonOperator",
    "AlertStatus",

    # Instrumentation Services
    "monitor_agent",
    "WorkflowTraceContext",
    "InstrumentationError",
    "is_agent_monitored",
    "get_agent_id",
    "get_observability_metadata"
]

# Import implemented services
from .services.instrumentation import (
    monitor_agent, WorkflowTraceContext, InstrumentationError,
    is_agent_monitored, get_agent_id, get_observability_metadata
)

# TODO: Add these when remaining service modules are implemented
# from .services.metrics_collector import MetricsCollector
# from .services.decision_tracer import DecisionTracer
# from .services.workflow_tracker import WorkflowTracker
# from .services.alert_manager import AlertManager
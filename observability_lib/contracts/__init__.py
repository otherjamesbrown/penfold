"""Contract interfaces for the Penfold observability framework.

This package defines the abstract interfaces that all observability components
must implement. These contracts ensure consistent behavior and enable
dependency injection patterns throughout the framework.

Key Interfaces:
    - MetricsCollectorInterface: Time-series metrics collection and storage
    - DecisionTracerInterface: Agent decision tracking and analysis
    - WorkflowTrackerInterface: Cross-agent workflow execution monitoring
    - AlertManagerInterface: Threshold-based alerting and notification

The interfaces use abstract base classes (ABC) to define method signatures,
return types, and behavioral contracts that implementations must satisfy.
"""

from .instrumentation_interface import (
    # Core data types
    MetricType,
    EventType,
    EventStatus,
    AlertStatus,
    MetricPoint,
    DecisionContext,
    WorkflowEventData,

    # Interface contracts
    MetricsCollectorInterface,
    DecisionTracerInterface,
    WorkflowTrackerInterface,
    AlertManagerInterface,
    WorkflowTraceContext,

    # Decorator function
    monitor_agent,
)

__version__ = "1.0.0"

# Export public API
__all__ = [
    # Data Types
    "MetricType",
    "EventType",
    "EventStatus",
    "AlertStatus",
    "MetricPoint",
    "DecisionContext",
    "WorkflowEventData",

    # Interface Contracts
    "MetricsCollectorInterface",
    "DecisionTracerInterface",
    "WorkflowTrackerInterface",
    "AlertManagerInterface",
    "WorkflowTraceContext",

    # Decorator
    "monitor_agent",
]
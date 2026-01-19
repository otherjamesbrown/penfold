"""Observability framework data models.

This module provides SQLAlchemy models for tracking agent metrics, decision traces,
workflow events, logging, alerting, and business value metrics within the Penfold
observability system.

All models support multi-tenant isolation and TimescaleDB optimization for
time-series data with high-performance requirements.
"""

from .agent_metrics import (
    Agent,
    AgentMetric,
    DecisionTrace,
    WorkflowEvent,
    AgentLog,
    AlertThreshold,
    AlertEvent,
    Tenant,
    # Enum types
    AgentType,
    MetricType,
    DecisionType,
    EventType,
    EventStatus,
    LogLevel,
    ThresholdType,
    ComparisonOperator,
    AlertStatus,
)

from .business_metrics import (
    BusinessKpi,
    UsageMetric,
    RoiMetric,
    BusinessGoal,
    ValueCalculation,
    # Enum types
    KpiType,
    UsageMetricType,
    RoiMetricType,
    BusinessGoalType,
    GoalStatus,
    ValueCalculationType,
)

__all__ = [
    # Core entities
    "Agent",
    "AgentMetric",
    "DecisionTrace",
    "WorkflowEvent",
    "AgentLog",
    "AlertThreshold",
    "AlertEvent",
    "Tenant",
    # Business metrics entities
    "BusinessKpi",
    "UsageMetric",
    "RoiMetric",
    "BusinessGoal",
    "ValueCalculation",
    # Core enum types
    "AgentType",
    "MetricType",
    "DecisionType",
    "EventType",
    "EventStatus",
    "LogLevel",
    "ThresholdType",
    "ComparisonOperator",
    "AlertStatus",
    # Business enum types
    "KpiType",
    "UsageMetricType",
    "RoiMetricType",
    "BusinessGoalType",
    "GoalStatus",
    "ValueCalculationType",
]
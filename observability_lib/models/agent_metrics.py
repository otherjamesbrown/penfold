"""SQLAlchemy models for Penfold observability framework.

This module provides async-compatible models for tracking agent performance,
decision traces, workflow events, and alerting with TimescaleDB optimization.

Models support multi-tenant isolation using Row-Level Security and include
comprehensive validation and indexing for high-performance operations.
"""

import uuid
from datetime import datetime, timedelta
from decimal import Decimal
from typing import Any, Dict, List, Optional
from enum import Enum

from sqlalchemy import (
    Boolean,
    Column,
    DateTime,
    Integer,
    String,
    Text,
    Float,
    CheckConstraint,
    UniqueConstraint,
    Index,
    ForeignKey,
    event,
)
from sqlalchemy.dialects.postgresql import UUID, JSONB
from sqlalchemy.ext.declarative import declarative_base
from sqlalchemy.sql import func, text
from sqlalchemy.orm import relationship, validates
from sqlalchemy.ext.hybrid import hybrid_property

# Import base classes from the main storage module
from penf_lib.storage.models import Base, TimestampMixin, TenantMixin

# Import validation functions
from penf_lib.storage.validators import (
    validate_string_length,
    validate_positive_integer,
    validate_non_negative_integer,
    validate_confidence_score,
    validate_email,
    ValidationError,
)


# Enum types for observability framework
class AgentType(Enum):
    """Types of autonomous AI agents in the system."""
    PROCESSING = "processing"
    ANALYSIS = "analysis"
    COORDINATION = "coordination"
    REVIEW = "review"


class MetricType(Enum):
    """Types of metrics for monitoring."""
    COUNTER = "counter"
    GAUGE = "gauge"
    HISTOGRAM = "histogram"


class DecisionType(Enum):
    """Types of agent decisions that can be traced."""
    ENTITY_EXTRACTION = "entity_extraction"
    PROJECT_CATEGORIZATION = "project_categorization"
    CONFIDENCE_THRESHOLD = "confidence_threshold"
    WORKFLOW_ROUTING = "workflow_routing"
    QUALITY_ASSESSMENT = "quality_assessment"
    ERROR_HANDLING = "error_handling"


class EventType(Enum):
    """Types of workflow events."""
    WORKFLOW_STARTED = "workflow_started"
    STAGE_COMPLETED = "stage_completed"
    HANDOFF = "handoff"
    WORKFLOW_FINISHED = "workflow_finished"
    ERROR_OCCURRED = "error_occurred"


class EventStatus(Enum):
    """Status of workflow events."""
    SUCCESS = "success"
    FAILURE = "failure"
    TIMEOUT = "timeout"
    RETRY = "retry"


class LogLevel(Enum):
    """Logging levels for agent operations."""
    DEBUG = "DEBUG"
    INFO = "INFO"
    WARNING = "WARNING"
    ERROR = "ERROR"
    CRITICAL = "CRITICAL"


class ThresholdType(Enum):
    """Types of alert thresholds."""
    PERFORMANCE = "performance"
    ERROR_RATE = "error_rate"
    RESOURCE_USAGE = "resource_usage"
    QUALITY = "quality"


class ComparisonOperator(Enum):
    """Comparison operators for threshold evaluation."""
    GREATER_THAN = "greater_than"
    LESS_THAN = "less_than"
    EQUALS = "equals"
    NOT_EQUALS = "not_equals"
    GREATER_EQUAL = "greater_equal"
    LESS_EQUAL = "less_equal"


class AlertStatus(Enum):
    """Status of alert events."""
    ACTIVE = "active"
    RESOLVED = "resolved"
    ACKNOWLEDGED = "acknowledged"
    SUPPRESSED = "suppressed"


# Core observability models
class Tenant(Base, TimestampMixin):
    """Tenant context for multi-tenant isolation.

    Note: This extends the base Tenant model from storage for observability-specific
    settings while maintaining compatibility with the main tenant system.
    """

    __tablename__ = "observability_tenants"

    id = Column(
        UUID(as_uuid=True),
        primary_key=True,
        default=uuid.uuid4,
        nullable=False,
        comment="Unique tenant identifier",
    )

    name = Column(
        String(50),
        nullable=False,
        unique=True,
        comment="Tenant name (work, personal, family)",
    )

    display_name = Column(
        String(100),
        nullable=False,
        comment="Human-readable tenant name",
    )

    is_active = Column(
        Boolean,
        nullable=False,
        default=True,
        comment="Whether tenant is active for observability",
    )

    observability_settings = Column(
        JSONB,
        nullable=True,
        default={},
        comment="Tenant-specific observability configuration",
    )

    # Relationships
    agents = relationship("Agent", back_populates="tenant", lazy="select")

    # Indexes for efficient queries
    __table_args__ = (
        Index("idx_obs_tenants_name", "name"),
        Index("idx_obs_tenants_active", "is_active", "created_at"),
        CheckConstraint(
            "name IN ('work', 'personal', 'family')",
            name="ck_obs_tenant_valid_names"
        ),
        CheckConstraint(
            "char_length(name) >= 3",
            name="ck_obs_tenant_name_length"
        ),
    )

    @validates("name")
    def validate_name(self, key, name):
        """Validate tenant name."""
        validate_string_length(name, "Tenant name", 3, 50)
        if name not in {"work", "personal", "family"}:
            raise ValidationError(f"Invalid tenant name: {name}. Must be work, personal, or family")
        return name.lower()

    def __repr__(self):
        return f"<Tenant(id={self.id}, name='{self.name}')>"


class Agent(Base, TimestampMixin, TenantMixin):
    """Registry of autonomous AI agents in the Penfold system.

    Tracks agent registration, configuration, and operational status
    for monitoring and coordination purposes.
    """

    __tablename__ = "agents"

    agent_id = Column(
        String(100),
        primary_key=True,
        comment="Unique identifier for agent (e.g., 'email_processor', 'meeting_analyzer')",
    )

    agent_name = Column(
        String(200),
        nullable=False,
        comment="Human-readable name for dashboard display",
    )

    agent_type = Column(
        String(50),
        nullable=False,
        comment="Category of agent (processing, analysis, coordination, review)",
    )

    configuration = Column(
        JSONB,
        nullable=True,
        default={},
        comment="Agent-specific configuration and capabilities",
    )

    is_active = Column(
        Boolean,
        nullable=False,
        default=True,
        comment="Whether agent is currently operational",
    )

    last_active_at = Column(
        DateTime(timezone=True),
        nullable=True,
        comment="When agent was last seen active",
    )

    # Relationships
    tenant = relationship("Tenant", back_populates="agents", lazy="select")
    metrics = relationship("AgentMetric", back_populates="agent", lazy="select")
    decision_traces = relationship("DecisionTrace", back_populates="agent", lazy="select")
    workflow_events = relationship("WorkflowEvent", back_populates="agent", lazy="select")
    logs = relationship("AgentLog", back_populates="agent", lazy="select")
    thresholds = relationship("AlertThreshold", back_populates="agent", lazy="select")

    # Indexes for efficient queries
    __table_args__ = (
        Index("idx_agents_tenant_type", "tenant_id", "agent_type"),
        Index("idx_agents_active", "is_active", "last_active_at"),
        Index("idx_agents_name", "agent_name"),
        CheckConstraint(
            "agent_type IN ('processing', 'analysis', 'coordination', 'review')",
            name="ck_agents_valid_type"
        ),
        CheckConstraint(
            "char_length(agent_id) >= 3",
            name="ck_agents_id_length"
        ),
        CheckConstraint(
            "char_length(agent_name) >= 1",
            name="ck_agents_name_not_empty"
        ),
    )

    @validates("agent_id")
    def validate_agent_id(self, key, agent_id):
        """Validate agent ID format."""
        validate_string_length(agent_id, "Agent ID", 3, 100)
        # Allow alphanumeric and underscores only
        if not all(c.isalnum() or c == '_' for c in agent_id):
            raise ValidationError("Agent ID must contain only alphanumeric characters and underscores")
        return agent_id.lower()

    @validates("agent_name")
    def validate_agent_name(self, key, agent_name):
        """Validate agent name."""
        validate_string_length(agent_name, "Agent name", 1, 200)
        return agent_name.strip()

    @validates("agent_type")
    def validate_agent_type(self, key, agent_type):
        """Validate agent type."""
        try:
            AgentType(agent_type)
        except ValueError:
            valid_types = [e.value for e in AgentType]
            raise ValidationError(f"Invalid agent type: {agent_type}. Must be one of {valid_types}")
        return agent_type

    def update_last_active(self):
        """Update the last active timestamp."""
        self.last_active_at = datetime.utcnow()

    def __repr__(self):
        return f"<Agent(id='{self.agent_id}', name='{self.agent_name}', type='{self.agent_type}')>"


class AgentMetric(Base, TimestampMixin, TenantMixin):
    """Time-series performance and health metrics for agent monitoring.

    Optimized for TimescaleDB with hypertable partitioning by timestamp
    for high-throughput metric ingestion and efficient querying.
    """

    __tablename__ = "agent_metrics"

    id = Column(
        UUID(as_uuid=True),
        primary_key=True,
        default=uuid.uuid4,
        comment="Unique metric record identifier",
    )

    agent_id = Column(
        String(100),
        ForeignKey("agents.agent_id"),
        nullable=False,
        comment="Reference to monitored agent",
    )

    timestamp = Column(
        DateTime(timezone=True),
        nullable=False,
        default=func.now(),
        comment="When metric was recorded (TimescaleDB partition key)",
    )

    metric_name = Column(
        String(100),
        nullable=False,
        comment="Name of metric (processing_time_ms, confidence_score, memory_usage_mb)",
    )

    metric_value = Column(
        Float,
        nullable=False,
        comment="Numeric value of the metric",
    )

    metric_type = Column(
        String(20),
        nullable=False,
        comment="Type of metric (counter, gauge, histogram)",
    )

    labels = Column(
        JSONB,
        nullable=True,
        default={},
        comment="Additional metric dimensions (operation_type, status, etc.)",
    )

    # Relationships
    agent = relationship("Agent", back_populates="metrics", lazy="select")

    # Indexes for efficient queries (optimized for TimescaleDB)
    __table_args__ = (
        Index("idx_agent_metrics_agent_time", "agent_id", "timestamp", postgresql_ops={"timestamp": "DESC"}),
        Index("idx_agent_metrics_name_time", "metric_name", "timestamp", postgresql_ops={"timestamp": "DESC"}),
        Index("idx_agent_metrics_labels", "labels", postgresql_using="gin"),
        Index("idx_agent_metrics_tenant", "tenant_id", "timestamp"),
        CheckConstraint(
            "metric_type IN ('counter', 'gauge', 'histogram')",
            name="ck_metrics_valid_type"
        ),
        CheckConstraint(
            "metric_value = metric_value",  # Check for NaN
            name="ck_metrics_value_finite"
        ),
        CheckConstraint(
            "char_length(metric_name) >= 1",
            name="ck_metrics_name_not_empty"
        ),
    )

    @validates("metric_name")
    def validate_metric_name(self, key, metric_name):
        """Validate metric name."""
        validate_string_length(metric_name, "Metric name", 1, 100)
        return metric_name

    @validates("metric_type")
    def validate_metric_type(self, key, metric_type):
        """Validate metric type."""
        try:
            MetricType(metric_type)
        except ValueError:
            valid_types = [e.value for e in MetricType]
            raise ValidationError(f"Invalid metric type: {metric_type}. Must be one of {valid_types}")
        return metric_type

    @validates("metric_value")
    def validate_metric_value(self, key, metric_value):
        """Validate metric value is finite."""
        if metric_value is None:
            raise ValidationError("Metric value cannot be None")
        if not isinstance(metric_value, (int, float)):
            raise ValidationError("Metric value must be numeric")
        import math
        if math.isnan(metric_value) or math.isinf(metric_value):
            raise ValidationError("Metric value must be finite (no NaN or Infinity)")
        return float(metric_value)

    def __repr__(self):
        return f"<AgentMetric(agent='{self.agent_id}', metric='{self.metric_name}', value={self.metric_value})>"


class DecisionTrace(Base, TimestampMixin, TenantMixin):
    """Captures agent decision points with context and reasoning for debugging.

    Provides detailed traces of agent decision-making processes to enable
    debugging, optimization, and understanding of agent behavior.
    """

    __tablename__ = "decision_traces"

    id = Column(
        UUID(as_uuid=True),
        primary_key=True,
        default=uuid.uuid4,
        comment="Unique decision trace identifier",
    )

    agent_id = Column(
        String(100),
        ForeignKey("agents.agent_id"),
        nullable=False,
        comment="Agent that made this decision",
    )

    timestamp = Column(
        DateTime(timezone=True),
        nullable=False,
        default=func.now(),
        comment="When decision was made (TimescaleDB partition key)",
    )

    decision_type = Column(
        String(50),
        nullable=False,
        comment="Type of decision (entity_extraction, project_categorization, etc.)",
    )

    decision_context = Column(
        JSONB,
        nullable=False,
        comment="Input context and data that influenced decision",
    )

    alternatives_considered = Column(
        JSONB,
        nullable=False,
        comment="Array of alternative options with scores",
    )

    confidence_score = Column(
        Float,
        nullable=False,
        comment="Agent confidence in decision (0.0-1.0)",
    )

    reasoning = Column(
        Text,
        nullable=True,
        comment="Human-readable explanation of decision logic",
    )

    workflow_id = Column(
        UUID(as_uuid=True),
        nullable=True,
        comment="Links decision to broader workflow execution",
    )

    # Relationships
    agent = relationship("Agent", back_populates="decision_traces", lazy="select")

    # Indexes for efficient queries
    __table_args__ = (
        Index("idx_decision_traces_agent_time", "agent_id", "timestamp", postgresql_ops={"timestamp": "DESC"}),
        Index("idx_decision_traces_type_time", "decision_type", "timestamp", postgresql_ops={"timestamp": "DESC"}),
        Index("idx_decision_traces_workflow", "workflow_id"),
        Index("idx_decision_traces_confidence", "confidence_score"),
        Index("idx_decision_traces_tenant", "tenant_id", "timestamp"),
        CheckConstraint(
            "confidence_score >= 0.0 AND confidence_score <= 1.0",
            name="ck_decision_confidence_range"
        ),
        CheckConstraint(
            "decision_type IN ('entity_extraction', 'project_categorization', 'confidence_threshold', 'workflow_routing', 'quality_assessment', 'error_handling')",
            name="ck_decision_valid_type"
        ),
    )

    @validates("decision_type")
    def validate_decision_type(self, key, decision_type):
        """Validate decision type."""
        try:
            DecisionType(decision_type)
        except ValueError:
            valid_types = [e.value for e in DecisionType]
            raise ValidationError(f"Invalid decision type: {decision_type}. Must be one of {valid_types}")
        return decision_type

    @validates("confidence_score")
    def validate_confidence_score(self, key, confidence_score):
        """Validate confidence score range."""
        validate_confidence_score(confidence_score)
        return float(confidence_score)

    @validates("decision_context")
    def validate_decision_context(self, key, decision_context):
        """Validate decision context is not empty."""
        if not decision_context:
            raise ValidationError("Decision context cannot be empty")
        return decision_context

    @validates("alternatives_considered")
    def validate_alternatives_considered(self, key, alternatives_considered):
        """Validate alternatives is a non-empty array."""
        if not isinstance(alternatives_considered, list) or len(alternatives_considered) == 0:
            raise ValidationError("Alternatives considered must be a non-empty array")
        return alternatives_considered

    def __repr__(self):
        return f"<DecisionTrace(agent='{self.agent_id}', type='{self.decision_type}', confidence={self.confidence_score})>"


class WorkflowEvent(Base, TimestampMixin, TenantMixin):
    """Tracks cross-agent workflow execution and timing for performance analysis.

    Enables tracking of complex workflows that span multiple agents with
    detailed timing and status information for optimization.
    """

    __tablename__ = "workflow_events"

    id = Column(
        UUID(as_uuid=True),
        primary_key=True,
        default=uuid.uuid4,
        comment="Unique workflow event identifier",
    )

    workflow_id = Column(
        UUID(as_uuid=True),
        nullable=False,
        comment="Unique identifier for workflow instance",
    )

    agent_id = Column(
        String(100),
        ForeignKey("agents.agent_id"),
        nullable=False,
        comment="Agent executing this workflow stage",
    )

    timestamp = Column(
        DateTime(timezone=True),
        nullable=False,
        default=func.now(),
        comment="When event occurred (TimescaleDB partition key)",
    )

    event_type = Column(
        String(50),
        nullable=False,
        comment="Type of event (workflow_started, stage_completed, handoff, etc.)",
    )

    event_status = Column(
        String(20),
        nullable=False,
        comment="Status of event (success, failure, timeout, retry)",
    )

    event_data = Column(
        JSONB,
        nullable=True,
        default={},
        comment="Event-specific data and metrics",
    )

    processing_time_ms = Column(
        Float,
        nullable=True,
        comment="Time spent in this workflow stage (milliseconds)",
    )

    parent_workflow_id = Column(
        UUID(as_uuid=True),
        nullable=True,
        comment="Reference to parent workflow for nested workflows",
    )

    # Relationships
    agent = relationship("Agent", back_populates="workflow_events", lazy="select")

    # Indexes for efficient queries
    __table_args__ = (
        Index("idx_workflow_events_workflow_time", "workflow_id", "timestamp", postgresql_ops={"timestamp": "DESC"}),
        Index("idx_workflow_events_agent_time", "agent_id", "timestamp", postgresql_ops={"timestamp": "DESC"}),
        Index("idx_workflow_events_type_status", "event_type", "event_status"),
        Index("idx_workflow_events_parent", "parent_workflow_id"),
        Index("idx_workflow_events_tenant", "tenant_id", "timestamp"),
        CheckConstraint(
            "processing_time_ms IS NULL OR processing_time_ms >= 0",
            name="ck_workflow_processing_time_positive"
        ),
        CheckConstraint(
            "event_type IN ('workflow_started', 'stage_completed', 'handoff', 'workflow_finished', 'error_occurred')",
            name="ck_workflow_valid_event_type"
        ),
        CheckConstraint(
            "event_status IN ('success', 'failure', 'timeout', 'retry')",
            name="ck_workflow_valid_status"
        ),
    )

    @validates("event_type")
    def validate_event_type(self, key, event_type):
        """Validate event type."""
        try:
            EventType(event_type)
        except ValueError:
            valid_types = [e.value for e in EventType]
            raise ValidationError(f"Invalid event type: {event_type}. Must be one of {valid_types}")
        return event_type

    @validates("event_status")
    def validate_event_status(self, key, event_status):
        """Validate event status."""
        try:
            EventStatus(event_status)
        except ValueError:
            valid_statuses = [e.value for e in EventStatus]
            raise ValidationError(f"Invalid event status: {event_status}. Must be one of {valid_statuses}")
        return event_status

    @validates("processing_time_ms")
    def validate_processing_time(self, key, processing_time_ms):
        """Validate processing time is non-negative."""
        if processing_time_ms is not None and processing_time_ms < 0:
            raise ValidationError("Processing time must be non-negative")
        return processing_time_ms

    def __repr__(self):
        return f"<WorkflowEvent(workflow='{self.workflow_id}', agent='{self.agent_id}', type='{self.event_type}')>"


class AgentLog(Base, TimestampMixin, TenantMixin):
    """Structured logging for agent operations and debugging.

    Provides comprehensive logging with structured context data for
    debugging, monitoring, and operational insights.
    """

    __tablename__ = "agent_logs"

    id = Column(
        UUID(as_uuid=True),
        primary_key=True,
        default=uuid.uuid4,
        comment="Unique log entry identifier",
    )

    timestamp = Column(
        DateTime(timezone=True),
        nullable=False,
        default=func.now(),
        comment="When log entry was created (TimescaleDB partition key)",
    )

    agent_id = Column(
        String(100),
        ForeignKey("agents.agent_id"),
        nullable=False,
        comment="Agent that generated this log entry",
    )

    level = Column(
        String(10),
        nullable=False,
        comment="Log level (DEBUG, INFO, WARNING, ERROR, CRITICAL)",
    )

    event = Column(
        String(200),
        nullable=False,
        comment="Short description of what happened",
    )

    context = Column(
        JSONB,
        nullable=True,
        default={},
        comment="Structured context data for debugging",
    )

    workflow_id = Column(
        UUID(as_uuid=True),
        nullable=True,
        comment="Links log entry to workflow for correlation",
    )

    # Relationships
    agent = relationship("Agent", back_populates="logs", lazy="select")

    # Indexes for efficient queries (optimized for TimescaleDB)
    __table_args__ = (
        Index("idx_agent_logs_agent_time", "agent_id", "timestamp", postgresql_ops={"timestamp": "DESC"}),
        Index("idx_agent_logs_level_time", "level", "timestamp", postgresql_ops={"timestamp": "DESC"}),
        Index("idx_agent_logs_workflow", "workflow_id"),
        Index("idx_agent_logs_tenant", "tenant_id", "timestamp"),
        Index("idx_agent_logs_context", "context", postgresql_using="gin"),
        CheckConstraint(
            "level IN ('DEBUG', 'INFO', 'WARNING', 'ERROR', 'CRITICAL')",
            name="ck_logs_valid_level"
        ),
        CheckConstraint(
            "char_length(event) >= 1",
            name="ck_logs_event_not_empty"
        ),
    )

    @validates("level")
    def validate_level(self, key, level):
        """Validate log level."""
        try:
            LogLevel(level)
        except ValueError:
            valid_levels = [e.value for e in LogLevel]
            raise ValidationError(f"Invalid log level: {level}. Must be one of {valid_levels}")
        return level.upper()

    @validates("event")
    def validate_event(self, key, event):
        """Validate event description."""
        validate_string_length(event, "Event", 1, 200)
        return event.strip()

    def __repr__(self):
        return f"<AgentLog(agent='{self.agent_id}', level='{self.level}', event='{self.event[:50]}...')>"


class AlertThreshold(Base, TimestampMixin, TenantMixin):
    """Configuration for automated agent health and performance monitoring.

    Defines thresholds that trigger alerts when agent metrics exceed
    acceptable operating parameters.
    """

    __tablename__ = "alert_thresholds"

    id = Column(
        UUID(as_uuid=True),
        primary_key=True,
        default=uuid.uuid4,
        comment="Unique threshold configuration identifier",
    )

    agent_id = Column(
        String(100),
        ForeignKey("agents.agent_id"),
        nullable=False,
        comment="Agent this threshold monitors",
    )

    metric_name = Column(
        String(100),
        nullable=False,
        comment="Name of metric to monitor",
    )

    threshold_type = Column(
        String(50),
        nullable=False,
        comment="Type of threshold (performance, error_rate, resource_usage, quality)",
    )

    threshold_value = Column(
        Float,
        nullable=False,
        comment="Threshold value that triggers alert",
    )

    comparison_operator = Column(
        String(20),
        nullable=False,
        comment="Comparison operator (greater_than, less_than, equals)",
    )

    evaluation_window_minutes = Column(
        Integer,
        nullable=False,
        comment="Time window for threshold evaluation (1-1440 minutes)",
    )

    is_active = Column(
        Boolean,
        nullable=False,
        default=True,
        comment="Whether threshold is currently being monitored",
    )

    # Relationships
    agent = relationship("Agent", back_populates="thresholds", lazy="select")
    alert_events = relationship("AlertEvent", back_populates="threshold", lazy="select")

    # Indexes for efficient queries
    __table_args__ = (
        Index("idx_alert_thresholds_agent_metric", "agent_id", "metric_name"),
        Index("idx_alert_thresholds_active", "is_active", "threshold_type"),
        Index("idx_alert_thresholds_tenant", "tenant_id", "is_active"),
        UniqueConstraint(
            "tenant_id", "agent_id", "metric_name", "threshold_type",
            name="uq_threshold_per_agent_metric_type"
        ),
        CheckConstraint(
            "threshold_type IN ('performance', 'error_rate', 'resource_usage', 'quality')",
            name="ck_threshold_valid_type"
        ),
        CheckConstraint(
            "comparison_operator IN ('greater_than', 'less_than', 'equals', 'not_equals', 'greater_equal', 'less_equal')",
            name="ck_threshold_valid_operator"
        ),
        CheckConstraint(
            "evaluation_window_minutes >= 1 AND evaluation_window_minutes <= 1440",
            name="ck_threshold_window_range"
        ),
    )

    @validates("threshold_type")
    def validate_threshold_type(self, key, threshold_type):
        """Validate threshold type."""
        try:
            ThresholdType(threshold_type)
        except ValueError:
            valid_types = [e.value for e in ThresholdType]
            raise ValidationError(f"Invalid threshold type: {threshold_type}. Must be one of {valid_types}")
        return threshold_type

    @validates("comparison_operator")
    def validate_comparison_operator(self, key, comparison_operator):
        """Validate comparison operator."""
        try:
            ComparisonOperator(comparison_operator)
        except ValueError:
            valid_operators = [e.value for e in ComparisonOperator]
            raise ValidationError(f"Invalid comparison operator: {comparison_operator}. Must be one of {valid_operators}")
        return comparison_operator

    @validates("evaluation_window_minutes")
    def validate_evaluation_window(self, key, evaluation_window_minutes):
        """Validate evaluation window is within acceptable range."""
        validate_positive_integer(evaluation_window_minutes, "Evaluation window minutes")
        if evaluation_window_minutes < 1 or evaluation_window_minutes > 1440:
            raise ValidationError("Evaluation window must be between 1 and 1440 minutes (24 hours)")
        return evaluation_window_minutes

    @validates("metric_name")
    def validate_metric_name(self, key, metric_name):
        """Validate metric name."""
        validate_string_length(metric_name, "Metric name", 1, 100)
        return metric_name

    def __repr__(self):
        return f"<AlertThreshold(agent='{self.agent_id}', metric='{self.metric_name}', threshold={self.threshold_value})>"


class AlertEvent(Base, TimestampMixin, TenantMixin):
    """Records triggered alerts and their resolution for audit and analysis.

    Maintains a complete history of alert activations, acknowledgments,
    and resolutions for operational monitoring and improvement.
    """

    __tablename__ = "alert_events"

    id = Column(
        UUID(as_uuid=True),
        primary_key=True,
        default=uuid.uuid4,
        comment="Unique alert event identifier",
    )

    threshold_id = Column(
        UUID(as_uuid=True),
        ForeignKey("alert_thresholds.id"),
        nullable=False,
        comment="Reference to triggered threshold",
    )

    triggered_at = Column(
        DateTime(timezone=True),
        nullable=False,
        default=func.now(),
        comment="When alert was triggered",
    )

    resolved_at = Column(
        DateTime(timezone=True),
        nullable=True,
        comment="When alert was resolved (null if active)",
    )

    alert_status = Column(
        String(20),
        nullable=False,
        default="active",
        comment="Current status (active, resolved, acknowledged, suppressed)",
    )

    trigger_value = Column(
        Float,
        nullable=False,
        comment="Value that triggered the alert",
    )

    alert_context = Column(
        JSONB,
        nullable=True,
        default={},
        comment="Additional context about alert conditions",
    )

    resolution_notes = Column(
        Text,
        nullable=True,
        comment="Notes about how alert was resolved",
    )

    # Relationships
    threshold = relationship("AlertThreshold", back_populates="alert_events", lazy="select")

    # Indexes for efficient queries
    __table_args__ = (
        Index("idx_alert_events_threshold_time", "threshold_id", "triggered_at", postgresql_ops={"triggered_at": "DESC"}),
        Index("idx_alert_events_status", "alert_status", "triggered_at"),
        Index("idx_alert_events_active", "alert_status", "resolved_at"),
        Index("idx_alert_events_tenant", "tenant_id", "triggered_at"),
        CheckConstraint(
            "alert_status IN ('active', 'resolved', 'acknowledged', 'suppressed')",
            name="ck_alert_events_valid_status"
        ),
        CheckConstraint(
            "resolved_at IS NULL OR resolved_at >= triggered_at",
            name="ck_alert_events_resolved_after_triggered"
        ),
    )

    @validates("alert_status")
    def validate_alert_status(self, key, alert_status):
        """Validate alert status."""
        try:
            AlertStatus(alert_status)
        except ValueError:
            valid_statuses = [e.value for e in AlertStatus]
            raise ValidationError(f"Invalid alert status: {alert_status}. Must be one of {valid_statuses}")
        return alert_status

    @validates("trigger_value")
    def validate_trigger_value(self, key, trigger_value):
        """Validate trigger value is finite."""
        if trigger_value is None:
            raise ValidationError("Trigger value cannot be None")
        import math
        if math.isnan(trigger_value) or math.isinf(trigger_value):
            raise ValidationError("Trigger value must be finite")
        return float(trigger_value)

    @property
    def duration_minutes(self) -> Optional[float]:
        """Calculate alert duration in minutes."""
        if self.resolved_at:
            delta = self.resolved_at - self.triggered_at
            return delta.total_seconds() / 60.0
        return None

    @property
    def is_active(self) -> bool:
        """Check if alert is currently active."""
        return self.alert_status == AlertStatus.ACTIVE.value and self.resolved_at is None

    def resolve(self, resolution_notes: Optional[str] = None):
        """Mark alert as resolved."""
        self.alert_status = AlertStatus.RESOLVED.value
        self.resolved_at = datetime.utcnow()
        if resolution_notes:
            self.resolution_notes = resolution_notes

    def acknowledge(self):
        """Mark alert as acknowledged."""
        self.alert_status = AlertStatus.ACKNOWLEDGED.value

    def suppress(self):
        """Mark alert as suppressed."""
        self.alert_status = AlertStatus.SUPPRESSED.value

    def __repr__(self):
        return f"<AlertEvent(threshold='{self.threshold_id}', status='{self.alert_status}', trigger_value={self.trigger_value})>"


# Event handlers for automatic timestamp updates
@event.listens_for(Agent, "before_update")
def update_agent_last_active(mapper, connection, target):
    """Update last_active_at when agent is updated."""
    if hasattr(target, '_update_last_active') and target._update_last_active:
        target.last_active_at = datetime.utcnow()
        target._update_last_active = False
"""Business metrics models for tracking KPIs, usage analytics, and ROI.

This module extends the core observability models to track business value metrics,
usage analytics, and return on investment calculations for the Penfold system.

Key Features:
- Business KPI tracking: context reconstruction speed, search accuracy, relationship validation
- Usage analytics: daily review engagement, search feature usage, workflow utilization
- ROI calculations: time savings, productivity gains, value measurements
- Performance targets tracking: <15min context reconstruction, >90% search accuracy
- Multi-tenant support with proper isolation and indexing

Models:
    BusinessKpi: Core business key performance indicators
    UsageMetric: User engagement and feature utilization tracking
    RoiMetric: Return on investment and value measurements
    BusinessGoal: Target setting and progress tracking
    ValueCalculation: Detailed value calculation records
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
    Numeric,
    CheckConstraint,
    UniqueConstraint,
    Index,
    ForeignKey,
    event,
)
from sqlalchemy.dialects.postgresql import UUID, JSONB
from sqlalchemy.sql import func
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
    ValidationError,
)


# Enum types for business metrics
class KpiType(Enum):
    """Types of business key performance indicators."""
    CONTEXT_RECONSTRUCTION_SPEED = "context_reconstruction_speed"
    SEARCH_ACCURACY = "search_accuracy"
    RELATIONSHIP_VALIDATION_RATE = "relationship_validation_rate"
    USER_SATISFACTION_SCORE = "user_satisfaction_score"
    DATA_QUALITY_SCORE = "data_quality_score"
    WORKFLOW_EFFICIENCY = "workflow_efficiency"


class UsageMetricType(Enum):
    """Types of usage analytics metrics."""
    DAILY_REVIEW_ENGAGEMENT = "daily_review_engagement"
    SEARCH_FEATURE_USAGE = "search_feature_usage"
    PROJECT_QUERY_FREQUENCY = "project_query_frequency"
    WORKFLOW_UTILIZATION = "workflow_utilization"
    FEATURE_ADOPTION_RATE = "feature_adoption_rate"
    SESSION_DURATION = "session_duration"
    QUERY_COMPLEXITY_SCORE = "query_complexity_score"


class RoiMetricType(Enum):
    """Types of return on investment metrics."""
    TIME_SAVINGS_MINUTES = "time_savings_minutes"
    PRODUCTIVITY_GAIN_PERCENTAGE = "productivity_gain_percentage"
    DECISION_SPEED_IMPROVEMENT = "decision_speed_improvement"
    INFORMATION_RETRIEVAL_EFFICIENCY = "information_retrieval_efficiency"
    CONTEXT_SWITCH_REDUCTION = "context_switch_reduction"
    MEETING_PREPARATION_TIME_SAVED = "meeting_preparation_time_saved"


class BusinessGoalType(Enum):
    """Types of business goals."""
    PERFORMANCE_TARGET = "performance_target"
    USAGE_TARGET = "usage_target"
    ROI_TARGET = "roi_target"
    QUALITY_TARGET = "quality_target"


class GoalStatus(Enum):
    """Status of business goals."""
    NOT_STARTED = "not_started"
    IN_PROGRESS = "in_progress"
    ON_TRACK = "on_track"
    AT_RISK = "at_risk"
    ACHIEVED = "achieved"
    MISSED = "missed"


class ValueCalculationType(Enum):
    """Types of value calculations."""
    TIME_SAVINGS = "time_savings"
    COST_REDUCTION = "cost_reduction"
    PRODUCTIVITY_INCREASE = "productivity_increase"
    QUALITY_IMPROVEMENT = "quality_improvement"
    DECISION_ACCELERATION = "decision_acceleration"


# Business metrics models
class BusinessKpi(Base, TimestampMixin, TenantMixin):
    """Business key performance indicators for tracking system value delivery.

    Tracks core business metrics that demonstrate the value of the Penfold system,
    including context reconstruction speed, search accuracy, and validation rates.
    """

    __tablename__ = "business_kpis"

    id = Column(
        UUID(as_uuid=True),
        primary_key=True,
        default=uuid.uuid4,
        comment="Unique business KPI record identifier",
    )

    timestamp = Column(
        DateTime(timezone=True),
        nullable=False,
        default=func.now(),
        comment="When KPI was measured (TimescaleDB partition key)",
    )

    kpi_type = Column(
        String(50),
        nullable=False,
        comment="Type of KPI being tracked",
    )

    kpi_name = Column(
        String(100),
        nullable=False,
        comment="Human-readable name of the KPI",
    )

    current_value = Column(
        Float,
        nullable=False,
        comment="Current measured value of the KPI",
    )

    target_value = Column(
        Float,
        nullable=True,
        comment="Target value for this KPI",
    )

    baseline_value = Column(
        Float,
        nullable=True,
        comment="Baseline value before Penfold implementation",
    )

    measurement_unit = Column(
        String(50),
        nullable=False,
        comment="Unit of measurement (minutes, percentage, score, etc.)",
    )

    measurement_context = Column(
        JSONB,
        nullable=True,
        default={},
        comment="Additional context about the measurement",
    )

    is_improvement_positive = Column(
        Boolean,
        nullable=False,
        default=True,
        comment="Whether higher values indicate better performance",
    )

    # Performance calculation properties
    @hybrid_property
    def target_achievement_percentage(self) -> Optional[float]:
        """Calculate percentage of target achieved."""
        if self.target_value is None or self.target_value == 0:
            return None
        return (self.current_value / self.target_value) * 100

    @hybrid_property
    def improvement_from_baseline(self) -> Optional[float]:
        """Calculate improvement from baseline value."""
        if self.baseline_value is None:
            return None
        return self.current_value - self.baseline_value

    @hybrid_property
    def improvement_percentage(self) -> Optional[float]:
        """Calculate percentage improvement from baseline."""
        if self.baseline_value is None or self.baseline_value == 0:
            return None
        return ((self.current_value - self.baseline_value) / abs(self.baseline_value)) * 100

    # Indexes for efficient queries
    __table_args__ = (
        Index("idx_business_kpis_type_time", "kpi_type", "timestamp", postgresql_ops={"timestamp": "DESC"}),
        Index("idx_business_kpis_tenant_time", "tenant_id", "timestamp", postgresql_ops={"timestamp": "DESC"}),
        Index("idx_business_kpis_name", "kpi_name"),
        Index("idx_business_kpis_context", "measurement_context", postgresql_using="gin"),
        CheckConstraint(
            "kpi_type IN ('context_reconstruction_speed', 'search_accuracy', 'relationship_validation_rate', 'user_satisfaction_score', 'data_quality_score', 'workflow_efficiency')",
            name="ck_business_kpis_valid_type"
        ),
        CheckConstraint(
            "current_value = current_value",  # Check for NaN
            name="ck_business_kpis_current_value_finite"
        ),
        CheckConstraint(
            "target_value IS NULL OR target_value = target_value",  # Check for NaN
            name="ck_business_kpis_target_value_finite"
        ),
        CheckConstraint(
            "char_length(kpi_name) >= 1",
            name="ck_business_kpis_name_not_empty"
        ),
        CheckConstraint(
            "char_length(measurement_unit) >= 1",
            name="ck_business_kpis_unit_not_empty"
        ),
    )

    @validates("kpi_type")
    def validate_kpi_type(self, key, kpi_type):
        """Validate KPI type."""
        try:
            KpiType(kpi_type)
        except ValueError:
            valid_types = [e.value for e in KpiType]
            raise ValidationError(f"Invalid KPI type: {kpi_type}. Must be one of {valid_types}")
        return kpi_type

    @validates("kpi_name")
    def validate_kpi_name(self, key, kpi_name):
        """Validate KPI name."""
        validate_string_length(kpi_name, "KPI name", 1, 100)
        return kpi_name.strip()

    @validates("measurement_unit")
    def validate_measurement_unit(self, key, measurement_unit):
        """Validate measurement unit."""
        validate_string_length(measurement_unit, "Measurement unit", 1, 50)
        return measurement_unit.strip()

    @validates("current_value")
    def validate_current_value(self, key, current_value):
        """Validate current value is finite."""
        if current_value is None:
            raise ValidationError("Current value cannot be None")
        import math
        if math.isnan(current_value) or math.isinf(current_value):
            raise ValidationError("Current value must be finite")
        return float(current_value)

    def __repr__(self):
        return f"<BusinessKpi(type='{self.kpi_type}', value={self.current_value}, unit='{self.measurement_unit}')>"


class UsageMetric(Base, TimestampMixin, TenantMixin):
    """Usage analytics for tracking user engagement and feature utilization.

    Monitors how users interact with different features of the Penfold system
    to understand adoption patterns and identify optimization opportunities.
    """

    __tablename__ = "usage_metrics"

    id = Column(
        UUID(as_uuid=True),
        primary_key=True,
        default=uuid.uuid4,
        comment="Unique usage metric record identifier",
    )

    timestamp = Column(
        DateTime(timezone=True),
        nullable=False,
        default=func.now(),
        comment="When usage was recorded (TimescaleDB partition key)",
    )

    metric_type = Column(
        String(50),
        nullable=False,
        comment="Type of usage metric being tracked",
    )

    user_identifier = Column(
        String(255),
        nullable=True,
        comment="Identifier for the user (hashed for privacy)",
    )

    feature_name = Column(
        String(100),
        nullable=False,
        comment="Name of the feature being used",
    )

    usage_count = Column(
        Integer,
        nullable=False,
        default=1,
        comment="Number of times feature was used in this measurement",
    )

    duration_seconds = Column(
        Float,
        nullable=True,
        comment="Duration of usage session in seconds",
    )

    success_rate = Column(
        Float,
        nullable=True,
        comment="Success rate for this usage session (0.0-1.0)",
    )

    usage_context = Column(
        JSONB,
        nullable=True,
        default={},
        comment="Additional context about the usage session",
    )

    session_id = Column(
        UUID(as_uuid=True),
        nullable=True,
        comment="Session identifier for grouping related usage events",
    )

    # Indexes for efficient queries
    __table_args__ = (
        Index("idx_usage_metrics_type_time", "metric_type", "timestamp", postgresql_ops={"timestamp": "DESC"}),
        Index("idx_usage_metrics_feature_time", "feature_name", "timestamp", postgresql_ops={"timestamp": "DESC"}),
        Index("idx_usage_metrics_user_time", "user_identifier", "timestamp", postgresql_ops={"timestamp": "DESC"}),
        Index("idx_usage_metrics_session", "session_id", "timestamp"),
        Index("idx_usage_metrics_tenant_time", "tenant_id", "timestamp", postgresql_ops={"timestamp": "DESC"}),
        Index("idx_usage_metrics_context", "usage_context", postgresql_using="gin"),
        CheckConstraint(
            "metric_type IN ('daily_review_engagement', 'search_feature_usage', 'project_query_frequency', 'workflow_utilization', 'feature_adoption_rate', 'session_duration', 'query_complexity_score')",
            name="ck_usage_metrics_valid_type"
        ),
        CheckConstraint(
            "usage_count > 0",
            name="ck_usage_metrics_count_positive"
        ),
        CheckConstraint(
            "duration_seconds IS NULL OR duration_seconds >= 0",
            name="ck_usage_metrics_duration_non_negative"
        ),
        CheckConstraint(
            "success_rate IS NULL OR (success_rate >= 0.0 AND success_rate <= 1.0)",
            name="ck_usage_metrics_success_rate_range"
        ),
        CheckConstraint(
            "char_length(feature_name) >= 1",
            name="ck_usage_metrics_feature_not_empty"
        ),
    )

    @validates("metric_type")
    def validate_metric_type(self, key, metric_type):
        """Validate usage metric type."""
        try:
            UsageMetricType(metric_type)
        except ValueError:
            valid_types = [e.value for e in UsageMetricType]
            raise ValidationError(f"Invalid usage metric type: {metric_type}. Must be one of {valid_types}")
        return metric_type

    @validates("feature_name")
    def validate_feature_name(self, key, feature_name):
        """Validate feature name."""
        validate_string_length(feature_name, "Feature name", 1, 100)
        return feature_name.strip()

    @validates("usage_count")
    def validate_usage_count(self, key, usage_count):
        """Validate usage count is positive."""
        validate_positive_integer(usage_count, "Usage count")
        return usage_count

    @validates("duration_seconds")
    def validate_duration_seconds(self, key, duration_seconds):
        """Validate duration is non-negative."""
        if duration_seconds is not None and duration_seconds < 0:
            raise ValidationError("Duration seconds must be non-negative")
        return duration_seconds

    @validates("success_rate")
    def validate_success_rate(self, key, success_rate):
        """Validate success rate range."""
        if success_rate is not None:
            validate_confidence_score(success_rate)
        return success_rate

    def __repr__(self):
        return f"<UsageMetric(type='{self.metric_type}', feature='{self.feature_name}', count={self.usage_count})>"


class RoiMetric(Base, TimestampMixin, TenantMixin):
    """Return on investment metrics for measuring business value delivery.

    Tracks quantifiable benefits and value delivered by the Penfold system,
    including time savings, productivity gains, and efficiency improvements.
    """

    __tablename__ = "roi_metrics"

    id = Column(
        UUID(as_uuid=True),
        primary_key=True,
        default=uuid.uuid4,
        comment="Unique ROI metric record identifier",
    )

    timestamp = Column(
        DateTime(timezone=True),
        nullable=False,
        default=func.now(),
        comment="When ROI was measured (TimescaleDB partition key)",
    )

    roi_type = Column(
        String(50),
        nullable=False,
        comment="Type of ROI metric being tracked",
    )

    measurement_period = Column(
        String(20),
        nullable=False,
        comment="Time period for measurement (daily, weekly, monthly)",
    )

    value_amount = Column(
        Numeric(15, 4),
        nullable=False,
        comment="Quantified value amount (time, money, efficiency)",
    )

    value_unit = Column(
        String(20),
        nullable=False,
        comment="Unit of value (minutes, hours, dollars, percentage)",
    )

    baseline_amount = Column(
        Numeric(15, 4),
        nullable=True,
        comment="Baseline amount before Penfold implementation",
    )

    cost_amount = Column(
        Numeric(15, 4),
        nullable=True,
        comment="Associated cost for this value delivery",
    )

    confidence_level = Column(
        Float,
        nullable=False,
        default=0.8,
        comment="Confidence in the ROI measurement (0.0-1.0)",
    )

    calculation_method = Column(
        String(100),
        nullable=False,
        comment="Method used to calculate this ROI value",
    )

    supporting_data = Column(
        JSONB,
        nullable=True,
        default={},
        comment="Supporting data and assumptions for ROI calculation",
    )

    # ROI calculation properties
    @hybrid_property
    def net_value(self) -> Decimal:
        """Calculate net value (value - cost)."""
        cost = self.cost_amount or Decimal('0')
        return self.value_amount - cost

    @hybrid_property
    def roi_percentage(self) -> Optional[float]:
        """Calculate ROI as percentage."""
        if self.cost_amount is None or self.cost_amount == 0:
            return None
        return float((self.net_value / self.cost_amount) * 100)

    @hybrid_property
    def improvement_from_baseline(self) -> Optional[Decimal]:
        """Calculate improvement from baseline."""
        if self.baseline_amount is None:
            return None
        return self.value_amount - self.baseline_amount

    @hybrid_property
    def improvement_percentage(self) -> Optional[float]:
        """Calculate percentage improvement from baseline."""
        if self.baseline_amount is None or self.baseline_amount == 0:
            return None
        improvement = self.improvement_from_baseline
        if improvement is None:
            return None
        return float((improvement / abs(self.baseline_amount)) * 100)

    # Indexes for efficient queries
    __table_args__ = (
        Index("idx_roi_metrics_type_time", "roi_type", "timestamp", postgresql_ops={"timestamp": "DESC"}),
        Index("idx_roi_metrics_period_time", "measurement_period", "timestamp", postgresql_ops={"timestamp": "DESC"}),
        Index("idx_roi_metrics_tenant_time", "tenant_id", "timestamp", postgresql_ops={"timestamp": "DESC"}),
        Index("idx_roi_metrics_value", "value_amount", "value_unit"),
        Index("idx_roi_metrics_supporting_data", "supporting_data", postgresql_using="gin"),
        CheckConstraint(
            "roi_type IN ('time_savings_minutes', 'productivity_gain_percentage', 'decision_speed_improvement', 'information_retrieval_efficiency', 'context_switch_reduction', 'meeting_preparation_time_saved')",
            name="ck_roi_metrics_valid_type"
        ),
        CheckConstraint(
            "measurement_period IN ('daily', 'weekly', 'monthly', 'quarterly', 'annually')",
            name="ck_roi_metrics_valid_period"
        ),
        CheckConstraint(
            "confidence_level >= 0.0 AND confidence_level <= 1.0",
            name="ck_roi_metrics_confidence_range"
        ),
        CheckConstraint(
            "value_amount >= 0",
            name="ck_roi_metrics_value_non_negative"
        ),
        CheckConstraint(
            "cost_amount IS NULL OR cost_amount >= 0",
            name="ck_roi_metrics_cost_non_negative"
        ),
        CheckConstraint(
            "char_length(calculation_method) >= 1",
            name="ck_roi_metrics_method_not_empty"
        ),
    )

    @validates("roi_type")
    def validate_roi_type(self, key, roi_type):
        """Validate ROI metric type."""
        try:
            RoiMetricType(roi_type)
        except ValueError:
            valid_types = [e.value for e in RoiMetricType]
            raise ValidationError(f"Invalid ROI type: {roi_type}. Must be one of {valid_types}")
        return roi_type

    @validates("measurement_period")
    def validate_measurement_period(self, key, measurement_period):
        """Validate measurement period."""
        valid_periods = ["daily", "weekly", "monthly", "quarterly", "annually"]
        if measurement_period not in valid_periods:
            raise ValidationError(f"Invalid measurement period: {measurement_period}. Must be one of {valid_periods}")
        return measurement_period

    @validates("value_unit")
    def validate_value_unit(self, key, value_unit):
        """Validate value unit."""
        validate_string_length(value_unit, "Value unit", 1, 20)
        return value_unit.strip()

    @validates("calculation_method")
    def validate_calculation_method(self, key, calculation_method):
        """Validate calculation method."""
        validate_string_length(calculation_method, "Calculation method", 1, 100)
        return calculation_method.strip()

    @validates("confidence_level")
    def validate_confidence_level(self, key, confidence_level):
        """Validate confidence level range."""
        validate_confidence_score(confidence_level)
        return float(confidence_level)

    def __repr__(self):
        return f"<RoiMetric(type='{self.roi_type}', value={self.value_amount}, unit='{self.value_unit}')>"


class BusinessGoal(Base, TimestampMixin, TenantMixin):
    """Business goals and targets for tracking progress and success metrics.

    Defines specific, measurable goals for business KPIs with target values,
    deadlines, and progress tracking to measure success of the Penfold system.
    """

    __tablename__ = "business_goals"

    id = Column(
        UUID(as_uuid=True),
        primary_key=True,
        default=uuid.uuid4,
        comment="Unique business goal identifier",
    )

    goal_name = Column(
        String(100),
        nullable=False,
        comment="Human-readable name of the business goal",
    )

    goal_type = Column(
        String(50),
        nullable=False,
        comment="Type of goal (performance_target, usage_target, roi_target, quality_target)",
    )

    goal_description = Column(
        Text,
        nullable=True,
        comment="Detailed description of the goal and its importance",
    )

    target_metric_type = Column(
        String(50),
        nullable=False,
        comment="Type of metric this goal targets (KPI, usage, ROI)",
    )

    target_value = Column(
        Float,
        nullable=False,
        comment="Target value to achieve for this goal",
    )

    current_value = Column(
        Float,
        nullable=True,
        comment="Current value towards the goal",
    )

    baseline_value = Column(
        Float,
        nullable=True,
        comment="Starting value when goal was set",
    )

    target_date = Column(
        DateTime(timezone=True),
        nullable=True,
        comment="Target date to achieve this goal",
    )

    status = Column(
        String(20),
        nullable=False,
        default="not_started",
        comment="Current status of the goal",
    )

    priority = Column(
        Integer,
        nullable=False,
        default=3,
        comment="Priority level (1=highest, 5=lowest)",
    )

    owner = Column(
        String(100),
        nullable=True,
        comment="Person or team responsible for this goal",
    )

    success_criteria = Column(
        JSONB,
        nullable=True,
        default={},
        comment="Specific criteria that define success for this goal",
    )

    progress_notes = Column(
        Text,
        nullable=True,
        comment="Notes about progress, obstacles, and updates",
    )

    last_reviewed_at = Column(
        DateTime(timezone=True),
        nullable=True,
        comment="When goal was last reviewed or updated",
    )

    # Progress calculation properties
    @hybrid_property
    def progress_percentage(self) -> Optional[float]:
        """Calculate progress towards goal as percentage."""
        if self.current_value is None or self.baseline_value is None:
            return None

        if self.target_value == self.baseline_value:
            return 100.0 if self.current_value == self.target_value else 0.0

        progress = (self.current_value - self.baseline_value) / (self.target_value - self.baseline_value)
        return max(0.0, min(100.0, progress * 100))

    @hybrid_property
    def days_remaining(self) -> Optional[int]:
        """Calculate days remaining until target date."""
        if self.target_date is None:
            return None
        delta = self.target_date - datetime.utcnow()
        return delta.days

    @hybrid_property
    def is_at_risk(self) -> bool:
        """Determine if goal is at risk based on progress and time remaining."""
        progress = self.progress_percentage
        days_remaining = self.days_remaining

        if progress is None or days_remaining is None:
            return False

        # Goal is at risk if less than 50% progress with less than 30 days remaining
        # or if past target date without achieving goal
        return (
            (days_remaining < 30 and progress < 50) or
            (days_remaining < 0 and progress < 100)
        )

    # Indexes for efficient queries
    __table_args__ = (
        Index("idx_business_goals_type_status", "goal_type", "status"),
        Index("idx_business_goals_target_date", "target_date"),
        Index("idx_business_goals_priority", "priority", "status"),
        Index("idx_business_goals_tenant", "tenant_id", "status"),
        Index("idx_business_goals_owner", "owner"),
        Index("idx_business_goals_metric_type", "target_metric_type"),
        UniqueConstraint("tenant_id", "goal_name", name="uq_business_goals_name_per_tenant"),
        CheckConstraint(
            "goal_type IN ('performance_target', 'usage_target', 'roi_target', 'quality_target')",
            name="ck_business_goals_valid_type"
        ),
        CheckConstraint(
            "status IN ('not_started', 'in_progress', 'on_track', 'at_risk', 'achieved', 'missed')",
            name="ck_business_goals_valid_status"
        ),
        CheckConstraint(
            "priority >= 1 AND priority <= 5",
            name="ck_business_goals_priority_range"
        ),
        CheckConstraint(
            "char_length(goal_name) >= 1",
            name="ck_business_goals_name_not_empty"
        ),
        CheckConstraint(
            "target_date IS NULL OR target_date > created_at",
            name="ck_business_goals_target_after_creation"
        ),
    )

    @validates("goal_type")
    def validate_goal_type(self, key, goal_type):
        """Validate goal type."""
        try:
            BusinessGoalType(goal_type)
        except ValueError:
            valid_types = [e.value for e in BusinessGoalType]
            raise ValidationError(f"Invalid goal type: {goal_type}. Must be one of {valid_types}")
        return goal_type

    @validates("status")
    def validate_status(self, key, status):
        """Validate goal status."""
        try:
            GoalStatus(status)
        except ValueError:
            valid_statuses = [e.value for e in GoalStatus]
            raise ValidationError(f"Invalid goal status: {status}. Must be one of {valid_statuses}")
        return status

    @validates("goal_name")
    def validate_goal_name(self, key, goal_name):
        """Validate goal name."""
        validate_string_length(goal_name, "Goal name", 1, 100)
        return goal_name.strip()

    @validates("priority")
    def validate_priority(self, key, priority):
        """Validate priority range."""
        if priority < 1 or priority > 5:
            raise ValidationError("Priority must be between 1 (highest) and 5 (lowest)")
        return priority

    def update_progress(self, current_value: float, notes: Optional[str] = None):
        """Update goal progress."""
        self.current_value = current_value
        self.last_reviewed_at = datetime.utcnow()

        if notes:
            if self.progress_notes:
                self.progress_notes += f"\n\n[{datetime.utcnow().isoformat()}] {notes}"
            else:
                self.progress_notes = f"[{datetime.utcnow().isoformat()}] {notes}"

        # Auto-update status based on progress
        progress = self.progress_percentage
        if progress is not None:
            if progress >= 100:
                self.status = GoalStatus.ACHIEVED.value
            elif self.is_at_risk:
                self.status = GoalStatus.AT_RISK.value
            elif progress > 0:
                self.status = GoalStatus.IN_PROGRESS.value if self.status == GoalStatus.NOT_STARTED.value else self.status

    def __repr__(self):
        return f"<BusinessGoal(name='{self.goal_name}', type='{self.goal_type}', status='{self.status}')>"


class ValueCalculation(Base, TimestampMixin, TenantMixin):
    """Detailed value calculation records for ROI and business impact analysis.

    Provides detailed audit trail of value calculations with methodology,
    assumptions, and supporting data for transparency and validation.
    """

    __tablename__ = "value_calculations"

    id = Column(
        UUID(as_uuid=True),
        primary_key=True,
        default=uuid.uuid4,
        comment="Unique value calculation record identifier",
    )

    calculation_name = Column(
        String(100),
        nullable=False,
        comment="Human-readable name for this calculation",
    )

    calculation_type = Column(
        String(50),
        nullable=False,
        comment="Type of value calculation",
    )

    calculation_date = Column(
        DateTime(timezone=True),
        nullable=False,
        default=func.now(),
        comment="When calculation was performed",
    )

    time_period_start = Column(
        DateTime(timezone=True),
        nullable=False,
        comment="Start of time period for this calculation",
    )

    time_period_end = Column(
        DateTime(timezone=True),
        nullable=False,
        comment="End of time period for this calculation",
    )

    calculated_value = Column(
        Numeric(15, 4),
        nullable=False,
        comment="Final calculated value",
    )

    value_unit = Column(
        String(20),
        nullable=False,
        comment="Unit of the calculated value",
    )

    methodology = Column(
        Text,
        nullable=False,
        comment="Description of calculation methodology",
    )

    assumptions = Column(
        JSONB,
        nullable=False,
        comment="Key assumptions used in the calculation",
    )

    input_data = Column(
        JSONB,
        nullable=False,
        comment="Input data sources and values used",
    )

    calculation_steps = Column(
        JSONB,
        nullable=False,
        comment="Step-by-step calculation breakdown",
    )

    confidence_score = Column(
        Float,
        nullable=False,
        comment="Confidence in calculation accuracy (0.0-1.0)",
    )

    validation_status = Column(
        String(20),
        nullable=False,
        default="pending",
        comment="Validation status of the calculation",
    )

    validator = Column(
        String(100),
        nullable=True,
        comment="Person who validated this calculation",
    )

    validation_notes = Column(
        Text,
        nullable=True,
        comment="Notes from validation process",
    )

    related_roi_metric_id = Column(
        UUID(as_uuid=True),
        ForeignKey("roi_metrics.id"),
        nullable=True,
        comment="Link to related ROI metric record",
    )

    related_kpi_id = Column(
        UUID(as_uuid=True),
        ForeignKey("business_kpis.id"),
        nullable=True,
        comment="Link to related business KPI record",
    )

    # Relationships
    related_roi_metric = relationship("RoiMetric", foreign_keys=[related_roi_metric_id])
    related_kpi = relationship("BusinessKpi", foreign_keys=[related_kpi_id])

    # Indexes for efficient queries
    __table_args__ = (
        Index("idx_value_calculations_type_date", "calculation_type", "calculation_date", postgresql_ops={"calculation_date": "DESC"}),
        Index("idx_value_calculations_period", "time_period_start", "time_period_end"),
        Index("idx_value_calculations_tenant", "tenant_id", "calculation_date"),
        Index("idx_value_calculations_validation", "validation_status", "confidence_score"),
        Index("idx_value_calculations_roi_metric", "related_roi_metric_id"),
        Index("idx_value_calculations_kpi", "related_kpi_id"),
        Index("idx_value_calculations_assumptions", "assumptions", postgresql_using="gin"),
        CheckConstraint(
            "calculation_type IN ('time_savings', 'cost_reduction', 'productivity_increase', 'quality_improvement', 'decision_acceleration')",
            name="ck_value_calculations_valid_type"
        ),
        CheckConstraint(
            "validation_status IN ('pending', 'validated', 'rejected', 'needs_review')",
            name="ck_value_calculations_valid_validation_status"
        ),
        CheckConstraint(
            "confidence_score >= 0.0 AND confidence_score <= 1.0",
            name="ck_value_calculations_confidence_range"
        ),
        CheckConstraint(
            "time_period_end > time_period_start",
            name="ck_value_calculations_valid_period"
        ),
        CheckConstraint(
            "char_length(calculation_name) >= 1",
            name="ck_value_calculations_name_not_empty"
        ),
        CheckConstraint(
            "char_length(methodology) >= 10",
            name="ck_value_calculations_methodology_sufficient"
        ),
    )

    @validates("calculation_type")
    def validate_calculation_type(self, key, calculation_type):
        """Validate calculation type."""
        try:
            ValueCalculationType(calculation_type)
        except ValueError:
            valid_types = [e.value for e in ValueCalculationType]
            raise ValidationError(f"Invalid calculation type: {calculation_type}. Must be one of {valid_types}")
        return calculation_type

    @validates("validation_status")
    def validate_validation_status(self, key, validation_status):
        """Validate validation status."""
        valid_statuses = ["pending", "validated", "rejected", "needs_review"]
        if validation_status not in valid_statuses:
            raise ValidationError(f"Invalid validation status: {validation_status}. Must be one of {valid_statuses}")
        return validation_status

    @validates("calculation_name")
    def validate_calculation_name(self, key, calculation_name):
        """Validate calculation name."""
        validate_string_length(calculation_name, "Calculation name", 1, 100)
        return calculation_name.strip()

    @validates("value_unit")
    def validate_value_unit(self, key, value_unit):
        """Validate value unit."""
        validate_string_length(value_unit, "Value unit", 1, 20)
        return value_unit.strip()

    @validates("methodology")
    def validate_methodology(self, key, methodology):
        """Validate methodology has sufficient detail."""
        if len(methodology.strip()) < 10:
            raise ValidationError("Methodology must be at least 10 characters")
        return methodology.strip()

    @validates("confidence_score")
    def validate_confidence_score(self, key, confidence_score):
        """Validate confidence score range."""
        validate_confidence_score(confidence_score)
        return float(confidence_score)

    def validate_calculation(self, validator: str, is_valid: bool, notes: Optional[str] = None):
        """Validate the calculation."""
        self.validator = validator
        self.validation_status = "validated" if is_valid else "rejected"
        if notes:
            self.validation_notes = notes

    def __repr__(self):
        return f"<ValueCalculation(name='{self.calculation_name}', type='{self.calculation_type}', value={self.calculated_value})>"
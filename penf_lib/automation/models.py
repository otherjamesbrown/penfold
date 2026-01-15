"""SQLAlchemy models for Automation Rules Engine.

Defines all database entities for the automation system:
- AutomationRule: User-defined automation rules
- AutomationRuleVersion: Immutable version history
- ConfidenceThreshold: Per-user per-content-type thresholds
- AutomationDecision: Audit trail for all decisions
- RuleEffectiveness: Performance metrics
- AutomationPattern: Detected behavior patterns
- RuleConflict: Conflict resolution records
- ProgressiveSettings: User preferences

Reference: data-model.md for complete entity definitions
"""

import uuid
from datetime import datetime, timezone
from decimal import Decimal
from typing import Any, Dict, List, Optional

from sqlalchemy import (
    Boolean,
    Column,
    DateTime,
    Integer,
    String,
    Text,
    DECIMAL,
    ARRAY,
    Index,
    ForeignKey,
    CheckConstraint,
    UniqueConstraint,
)
from sqlalchemy.dialects.postgresql import UUID, JSONB, BIGINT
from sqlalchemy.orm import relationship, validates
from sqlalchemy.sql import func

from ..storage.models import Base, TimestampMixin, TenantMixin, SoftDeleteMixin


# =============================================================================
# AUTOMATION RULE MODELS
# =============================================================================


class AutomationRule(Base, TimestampMixin, TenantMixin, SoftDeleteMixin):
    """User-defined automation rule with conditions and actions.

    Rules define when and how content should be automatically processed.
    Each rule has a conditions JSONB defining match criteria and an
    actions JSONB defining what to do when matched.

    Reference: data-model.md AutomationRule entity
    """

    __tablename__ = "automation_rules"

    id = Column(
        UUID(as_uuid=True),
        primary_key=True,
        default=uuid.uuid4,
        nullable=False,
        comment="Unique rule identifier",
    )

    # Owner (rules are user-scoped per Clarification #5)
    user_id = Column(
        String(254),
        nullable=False,
        index=True,
        comment="Rule owner (user-scoped)",
    )

    # Rule definition
    name = Column(
        String(200),
        nullable=False,
        comment="Human-readable rule name",
    )
    description = Column(
        Text,
        nullable=True,
        comment="Rule purpose description",
    )
    is_enabled = Column(
        Boolean,
        nullable=False,
        default=True,
        comment="Whether rule is active",
    )
    priority = Column(
        Integer,
        nullable=False,
        default=5,
        comment="Manual priority (1=highest, 10=lowest)",
    )

    # Rule logic (JSONB for flexible schema)
    conditions = Column(
        JSONB,
        nullable=False,
        comment="Rule conditions in structured format",
    )
    actions = Column(
        JSONB,
        nullable=False,
        comment="Actions to execute when rule matches",
    )

    # Version tracking
    current_version_id = Column(
        BIGINT,
        nullable=True,
        comment="Active version reference",
    )

    # Relationships
    versions = relationship(
        "AutomationRuleVersion",
        back_populates="rule",
        lazy="select",
        cascade="all, delete-orphan",
    )
    decisions = relationship(
        "AutomationDecision",
        back_populates="rule",
        lazy="select",
    )
    effectiveness_records = relationship(
        "RuleEffectiveness",
        back_populates="rule",
        lazy="select",
        cascade="all, delete-orphan",
    )

    __table_args__ = (
        Index("idx_rules_tenant_user", "tenant_id", "user_id", "is_enabled"),
        Index("idx_rules_conditions", "conditions", postgresql_using="gin"),
        Index("idx_rules_priority", "priority", "is_enabled"),
        Index("idx_rules_soft_delete", "is_deleted", "deleted_at"),
        CheckConstraint(
            "priority >= 1 AND priority <= 10",
            name="ck_rules_priority_range"
        ),
        CheckConstraint(
            "char_length(name) >= 1 AND char_length(name) <= 200",
            name="ck_rules_name_length"
        ),
    )

    @validates("name")
    def validate_name(self, key, name):
        """Validate rule name length."""
        if not name or len(name) < 1:
            raise ValueError("Rule name cannot be empty")
        if len(name) > 200:
            raise ValueError("Rule name cannot exceed 200 characters")
        return name.strip()

    @validates("priority")
    def validate_priority(self, key, priority):
        """Validate priority is in range 1-10."""
        if priority < 1 or priority > 10:
            raise ValueError("Priority must be between 1 and 10")
        return priority


class AutomationRuleVersion(Base):
    """Immutable version record for rule history and rollback.

    Each edit to a rule creates a new version. Previous versions are
    retained indefinitely for audit and rollback per Clarification #4.

    Reference: data-model.md AutomationRuleVersion entity
    """

    __tablename__ = "automation_rule_versions"

    id = Column(
        BIGINT,
        primary_key=True,
        autoincrement=True,
        comment="Version record identifier",
    )
    rule_id = Column(
        UUID(as_uuid=True),
        ForeignKey("automation_rules.id", ondelete="CASCADE"),
        nullable=False,
        index=True,
        comment="Parent rule reference",
    )
    version_number = Column(
        Integer,
        nullable=False,
        comment="Sequential version number",
    )
    is_active = Column(
        Boolean,
        nullable=False,
        default=False,
        comment="Currently active version",
    )

    # Snapshot of rule at this version
    conditions = Column(
        JSONB,
        nullable=False,
        comment="Conditions at this version",
    )
    actions = Column(
        JSONB,
        nullable=False,
        comment="Actions at this version",
    )

    # Change tracking
    change_description = Column(
        Text,
        nullable=True,
        comment="Description of changes",
    )
    created_at = Column(
        DateTime(timezone=True),
        server_default=func.now(),
        nullable=False,
        comment="Version creation time",
    )
    created_by = Column(
        String(254),
        nullable=False,
        comment="User who created version",
    )

    # Relationships
    rule = relationship(
        "AutomationRule",
        back_populates="versions",
    )
    decisions = relationship(
        "AutomationDecision",
        back_populates="rule_version",
        lazy="select",
    )

    __table_args__ = (
        Index("idx_versions_rule_active", "rule_id", "is_active"),
        Index("idx_versions_rule_number", "rule_id", "version_number"),
        UniqueConstraint(
            "rule_id", "version_number",
            name="uc_rule_version_number"
        ),
    )


# =============================================================================
# CONFIDENCE THRESHOLD MODEL
# =============================================================================


class ConfidenceThreshold(Base, TimestampMixin, TenantMixin):
    """Per-user, per-content-type automation thresholds.

    Defines the minimum AI confidence score required for automatic
    processing. Default is 85% per Clarification #3.

    Reference: data-model.md ConfidenceThreshold entity
    """

    __tablename__ = "confidence_thresholds"

    id = Column(
        BIGINT,
        primary_key=True,
        autoincrement=True,
        comment="Threshold record identifier",
    )
    user_id = Column(
        String(254),
        nullable=False,
        comment="Threshold owner",
    )
    content_type = Column(
        String(50),
        nullable=False,
        comment="Content type (email, meeting, document, *)",
    )
    threshold_value = Column(
        DECIMAL(precision=4, scale=3),
        nullable=False,
        default=Decimal("0.850"),
        comment="Confidence threshold (0.000-1.000)",
    )
    is_enabled = Column(
        Boolean,
        nullable=False,
        default=True,
        comment="Whether threshold is active",
    )

    __table_args__ = (
        Index("idx_thresholds_user_type", "tenant_id", "user_id", "content_type"),
        UniqueConstraint(
            "tenant_id", "user_id", "content_type",
            name="uc_threshold_user_content"
        ),
        CheckConstraint(
            "threshold_value >= 0.000 AND threshold_value <= 1.000",
            name="ck_threshold_value_range"
        ),
    )

    @validates("threshold_value")
    def validate_threshold(self, key, value):
        """Validate threshold is in valid range."""
        if value is not None:
            decimal_value = Decimal(str(value))
            if decimal_value < 0 or decimal_value > 1:
                raise ValueError("Threshold must be between 0 and 1")
        return value


# =============================================================================
# AUTOMATION DECISION MODEL
# =============================================================================


class AutomationDecision(Base, TenantMixin):
    """Audit trail for all automated processing decisions.

    Records every automation decision with full context for audit,
    learning, and user feedback integration per FR-006.

    Reference: data-model.md AutomationDecision entity
    """

    __tablename__ = "automation_decisions"

    id = Column(
        BIGINT,
        primary_key=True,
        autoincrement=True,
        comment="Decision record identifier",
    )
    user_id = Column(
        String(254),
        nullable=False,
        comment="Decision context user",
    )
    content_id = Column(
        String(255),
        nullable=False,
        index=True,
        comment="Processed content reference",
    )
    content_type = Column(
        String(50),
        nullable=False,
        comment="Content type",
    )

    # Rule reference (nullable for threshold-based decisions)
    rule_id = Column(
        UUID(as_uuid=True),
        ForeignKey("automation_rules.id", ondelete="SET NULL"),
        nullable=True,
        comment="Applied rule (null if threshold-based)",
    )
    rule_version_id = Column(
        BIGINT,
        ForeignKey("automation_rule_versions.id", ondelete="SET NULL"),
        nullable=True,
        comment="Specific version applied",
    )

    # Decision details
    decision_type = Column(
        String(50),
        nullable=False,
        comment="auto_processed, queued_review, conflict_resolved",
    )
    confidence_score = Column(
        DECIMAL(precision=4, scale=3),
        nullable=False,
        comment="AI confidence at decision time",
    )
    threshold_used = Column(
        DECIMAL(precision=4, scale=3),
        nullable=False,
        comment="Threshold applied",
    )
    actions_taken = Column(
        JSONB,
        nullable=False,
        comment="Actions executed",
    )
    reasoning = Column(
        Text,
        nullable=True,
        comment="Decision explanation",
    )

    # Processing metrics
    processing_time_ms = Column(
        Integer,
        nullable=True,
        comment="Time to process",
    )
    retry_count = Column(
        Integer,
        nullable=False,
        default=0,
        comment="Number of retry attempts",
    )
    error_details = Column(
        JSONB,
        nullable=True,
        comment="Error info if failed",
    )

    # User feedback
    user_override = Column(
        Boolean,
        nullable=False,
        default=False,
        comment="Whether user overrode decision",
    )
    user_feedback = Column(
        JSONB,
        nullable=True,
        comment="User correction if provided",
    )

    # Timestamp
    created_at = Column(
        DateTime(timezone=True),
        server_default=func.now(),
        nullable=False,
        comment="Decision timestamp",
    )

    # Relationships
    rule = relationship(
        "AutomationRule",
        back_populates="decisions",
    )
    rule_version = relationship(
        "AutomationRuleVersion",
        back_populates="decisions",
    )

    __table_args__ = (
        Index("idx_decisions_user_date", "tenant_id", "user_id", "created_at"),
        Index("idx_decisions_rule", "rule_id", "created_at"),
        Index("idx_decisions_content", "content_id", "content_type"),
        Index("idx_decisions_type", "decision_type", "created_at"),
        CheckConstraint(
            "decision_type IN ('auto_processed', 'queued_review', 'conflict_resolved')",
            name="ck_decision_type_valid"
        ),
        CheckConstraint(
            "confidence_score >= 0.000 AND confidence_score <= 1.000",
            name="ck_decision_confidence_range"
        ),
        CheckConstraint(
            "threshold_used >= 0.000 AND threshold_used <= 1.000",
            name="ck_decision_threshold_range"
        ),
    )


# =============================================================================
# RULE EFFECTIVENESS MODEL
# =============================================================================


class RuleEffectiveness(Base):
    """Performance metrics for automation rules.

    Tracks rule accuracy, coverage, and performance over time periods
    for monitoring and optimization per User Story 4.

    Reference: data-model.md RuleEffectiveness entity
    """

    __tablename__ = "rule_effectiveness"

    id = Column(
        BIGINT,
        primary_key=True,
        autoincrement=True,
        comment="Metrics record identifier",
    )
    rule_id = Column(
        UUID(as_uuid=True),
        ForeignKey("automation_rules.id", ondelete="CASCADE"),
        nullable=False,
        comment="Rule being measured",
    )

    # Time period
    period_start = Column(
        DateTime(timezone=True),
        nullable=False,
        comment="Metrics period start",
    )
    period_end = Column(
        DateTime(timezone=True),
        nullable=False,
        comment="Metrics period end",
    )

    # Counters
    total_matches = Column(
        Integer,
        nullable=False,
        default=0,
        comment="Times rule matched",
    )
    total_applied = Column(
        Integer,
        nullable=False,
        default=0,
        comment="Times rule was applied",
    )
    correct_decisions = Column(
        Integer,
        nullable=False,
        default=0,
        comment="Decisions without override",
    )
    incorrect_decisions = Column(
        Integer,
        nullable=False,
        default=0,
        comment="Decisions user overrode",
    )

    # Calculated metrics
    accuracy_rate = Column(
        DECIMAL(precision=4, scale=3),
        nullable=True,
        comment="Calculated accuracy",
    )
    avg_confidence = Column(
        DECIMAL(precision=4, scale=3),
        nullable=True,
        comment="Average confidence score",
    )
    avg_processing_time_ms = Column(
        Integer,
        nullable=True,
        comment="Average processing time",
    )

    # Conflict tracking
    conflict_count = Column(
        Integer,
        nullable=False,
        default=0,
        comment="Times in conflict",
    )
    conflict_win_count = Column(
        Integer,
        nullable=False,
        default=0,
        comment="Times won conflict",
    )

    # Timestamp
    created_at = Column(
        DateTime(timezone=True),
        server_default=func.now(),
        nullable=False,
        comment="Record creation time",
    )

    # Relationships
    rule = relationship(
        "AutomationRule",
        back_populates="effectiveness_records",
    )

    __table_args__ = (
        Index("idx_effectiveness_rule_period", "rule_id", "period_start", "period_end"),
        Index("idx_effectiveness_accuracy", "accuracy_rate", "total_applied"),
        UniqueConstraint(
            "rule_id", "period_start", "period_end",
            name="uc_effectiveness_rule_period"
        ),
    )


# =============================================================================
# AUTOMATION PATTERN MODEL
# =============================================================================


class AutomationPattern(Base, TimestampMixin, TenantMixin):
    """Detected user behavior patterns for rule suggestions.

    Identifies recurring categorization patterns from user feedback
    and suggests automation rules per User Story 3.

    Reference: data-model.md AutomationPattern entity
    """

    __tablename__ = "automation_patterns"

    id = Column(
        BIGINT,
        primary_key=True,
        autoincrement=True,
        comment="Pattern record identifier",
    )
    user_id = Column(
        String(254),
        nullable=False,
        comment="Pattern owner",
    )
    pattern_signature = Column(
        String(64),
        nullable=False,
        index=True,
        comment="Hash of pattern conditions",
    )

    # Pattern definition
    detected_conditions = Column(
        JSONB,
        nullable=False,
        comment="Conditions forming pattern",
    )
    suggested_actions = Column(
        JSONB,
        nullable=False,
        comment="Suggested automation actions",
    )

    # Pattern statistics
    occurrence_count = Column(
        Integer,
        nullable=False,
        comment="Times pattern observed",
    )
    first_seen_at = Column(
        DateTime(timezone=True),
        nullable=False,
        comment="First occurrence",
    )
    last_seen_at = Column(
        DateTime(timezone=True),
        nullable=False,
        comment="Most recent occurrence",
    )
    confidence_score = Column(
        DECIMAL(precision=4, scale=3),
        nullable=False,
        comment="Pattern reliability score",
    )

    # Status tracking
    status = Column(
        String(20),
        nullable=False,
        default="pending",
        comment="pending, accepted, rejected, expired",
    )
    converted_rule_id = Column(
        UUID(as_uuid=True),
        ForeignKey("automation_rules.id", ondelete="SET NULL"),
        nullable=True,
        comment="Rule if pattern was accepted",
    )

    __table_args__ = (
        Index("idx_patterns_user_status", "tenant_id", "user_id", "status"),
        Index("idx_patterns_signature", "pattern_signature"),
        Index("idx_patterns_confidence", "confidence_score", "occurrence_count"),
        CheckConstraint(
            "status IN ('pending', 'accepted', 'rejected', 'expired')",
            name="ck_pattern_status_valid"
        ),
        CheckConstraint(
            "confidence_score >= 0.000 AND confidence_score <= 1.000",
            name="ck_pattern_confidence_range"
        ),
    )


# =============================================================================
# RULE CONFLICT MODEL
# =============================================================================


class RuleConflict(Base, TenantMixin):
    """Records of rule conflicts for analysis and learning.

    Tracks when multiple rules match the same content and how
    conflicts were resolved per User Story 5.

    Reference: data-model.md RuleConflict entity
    """

    __tablename__ = "rule_conflicts"

    id = Column(
        BIGINT,
        primary_key=True,
        autoincrement=True,
        comment="Conflict record identifier",
    )
    content_id = Column(
        String(255),
        nullable=False,
        comment="Content causing conflict",
    )

    # Conflicting rules
    conflicting_rule_ids = Column(
        ARRAY(UUID(as_uuid=True)),
        nullable=False,
        comment="Rules that matched",
    )
    winning_rule_id = Column(
        UUID(as_uuid=True),
        ForeignKey("automation_rules.id", ondelete="SET NULL"),
        nullable=True,
        comment="Rule that was applied",
    )

    # Resolution details
    resolution_method = Column(
        String(50),
        nullable=False,
        comment="confidence, priority, user",
    )
    resolution_reasoning = Column(
        Text,
        nullable=True,
        comment="Why winner was chosen",
    )
    confidence_scores = Column(
        JSONB,
        nullable=False,
        comment="Scores per rule",
    )

    # User notification
    user_notified = Column(
        Boolean,
        nullable=False,
        default=False,
        comment="Whether user was notified",
    )
    user_resolution = Column(
        UUID(as_uuid=True),
        ForeignKey("automation_rules.id", ondelete="SET NULL"),
        nullable=True,
        comment="User's chosen rule if different",
    )

    # Timestamp
    created_at = Column(
        DateTime(timezone=True),
        server_default=func.now(),
        nullable=False,
        comment="Conflict timestamp",
    )

    __table_args__ = (
        Index("idx_conflicts_content", "content_id", "created_at"),
        Index("idx_conflicts_rules", "conflicting_rule_ids", postgresql_using="gin"),
        Index("idx_conflicts_winner", "winning_rule_id"),
        CheckConstraint(
            "resolution_method IN ('confidence', 'priority', 'user')",
            name="ck_conflict_resolution_valid"
        ),
    )


# =============================================================================
# PROGRESSIVE SETTINGS MODEL
# =============================================================================


class ProgressiveSettings(Base, TimestampMixin, TenantMixin):
    """User preferences for automation advancement.

    Configures how aggressively the system should automate and
    learn from feedback per User Story 3.

    Reference: data-model.md ProgressiveSettings entity
    """

    __tablename__ = "progressive_settings"

    id = Column(
        BIGINT,
        primary_key=True,
        autoincrement=True,
        comment="Settings record identifier",
    )
    user_id = Column(
        String(254),
        nullable=False,
        comment="Settings owner",
    )

    # Automation aggressiveness
    aggressiveness_level = Column(
        String(20),
        nullable=False,
        default="moderate",
        comment="conservative, moderate, aggressive",
    )
    auto_threshold_adjustment = Column(
        Boolean,
        nullable=False,
        default=True,
        comment="Allow system to adjust thresholds",
    )
    min_learning_period_days = Column(
        Integer,
        nullable=False,
        default=30,
        comment="Days before auto-adjustment",
    )

    # Targets
    target_automation_rate = Column(
        DECIMAL(precision=4, scale=3),
        nullable=False,
        default=Decimal("0.60"),
        comment="Target % auto-processed",
    )
    accuracy_floor = Column(
        DECIMAL(precision=4, scale=3),
        nullable=False,
        default=Decimal("0.95"),
        comment="Minimum acceptable accuracy",
    )

    # Feature toggles
    pattern_suggestion_enabled = Column(
        Boolean,
        nullable=False,
        default=True,
        comment="Show pattern suggestions",
    )
    notification_preferences = Column(
        JSONB,
        nullable=False,
        default={},
        comment="Notification settings",
    )

    __table_args__ = (
        Index("idx_settings_user", "tenant_id", "user_id"),
        UniqueConstraint(
            "tenant_id", "user_id",
            name="uc_settings_user"
        ),
        CheckConstraint(
            "aggressiveness_level IN ('conservative', 'moderate', 'aggressive')",
            name="ck_settings_aggressiveness_valid"
        ),
        CheckConstraint(
            "target_automation_rate >= 0.000 AND target_automation_rate <= 1.000",
            name="ck_settings_target_rate_range"
        ),
        CheckConstraint(
            "accuracy_floor >= 0.800 AND accuracy_floor <= 1.000",
            name="ck_settings_accuracy_floor_range"
        ),
        CheckConstraint(
            "min_learning_period_days >= 1",
            name="ck_settings_learning_period_positive"
        ),
    )

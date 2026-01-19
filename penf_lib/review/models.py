"""Pydantic models and enums for the Daily Review workflow.

This module contains data transfer objects (DTOs) for the review workflow,
including session management, review items, user feedback, learning rules,
batch operations, and analytics.

Based on data-model.md specification from spec 006-daily-review.
"""

from __future__ import annotations

from datetime import date, datetime
from decimal import Decimal
from enum import Enum
from typing import Any, Dict, List, Optional
from uuid import UUID

from pydantic import BaseModel, ConfigDict, EmailStr, Field, model_validator


# =============================================================================
# ENUMERATIONS
# =============================================================================


class ReviewMode(str, Enum):
    """Review workflow modes affecting detail level and interaction speed."""

    QUICK = "quick"
    STANDARD = "standard"
    DETAILED = "detailed"


class PriorityMode(str, Enum):
    """Queue prioritization strategies for review items."""

    CONFIDENCE = "confidence"
    IMPORTANCE = "importance"
    RECENCY = "recency"
    MIXED = "mixed"


class DecisionType(str, Enum):
    """User decision types for review items."""

    ACCEPT = "accept"
    REJECT = "reject"
    MODIFY = "modify"
    SKIP = "skip"


class SessionStatus(str, Enum):
    """Review session lifecycle states."""

    ACTIVE = "active"
    PAUSED = "paused"
    COMPLETED = "completed"
    ABANDONED = "abandoned"


class ReviewItemStatus(str, Enum):
    """Review item processing states."""

    PENDING = "pending"
    IN_REVIEW = "in_review"
    ACCEPTED = "accepted"
    REJECTED = "rejected"
    MODIFIED = "modified"
    SKIPPED = "skipped"


class ContentType(str, Enum):
    """Types of content being reviewed."""

    EMAIL = "email"
    MEETING = "meeting"
    DOCUMENT = "document"


class BatchType(str, Enum):
    """Grouping strategies for batch operations."""

    THREAD = "thread"
    SENDER = "sender"
    CATEGORY = "category"
    TIME_WINDOW = "time_window"
    CUSTOM = "custom"


class BatchActionType(str, Enum):
    """Actions that can be applied to batched items."""

    ACCEPT_ALL = "accept_all"
    REJECT_ALL = "reject_all"
    APPLY_CATEGORY = "apply_category"
    SKIP_ALL = "skip_all"


class BatchOperationStatus(str, Enum):
    """Batch operation lifecycle states."""

    PENDING = "pending"
    CONFIRMED = "confirmed"
    APPLIED = "applied"
    UNDONE = "undone"


class LearningRuleStatus(str, Enum):
    """Learning rule lifecycle states."""

    ACTIVE = "active"
    DISABLED = "disabled"
    EXPIRED = "expired"


class LearningRuleType(str, Enum):
    """Types of automated decision patterns."""

    SENDER_MATCH = "sender_match"
    CONTENT_PATTERN = "content_pattern"
    CATEGORY_OVERRIDE = "category_override"
    PARTICIPANT_MAPPING = "participant_mapping"


class AnalyticsPeriodType(str, Enum):
    """Analytics aggregation periods."""

    DAILY = "daily"
    WEEKLY = "weekly"
    MONTHLY = "monthly"


# =============================================================================
# NESTED DATA MODELS
# =============================================================================


class AISuggestion(BaseModel):
    """AI-generated suggestion for content categorization.

    Represents the AI model's classification output including
    category, participants, and tags.
    """

    model_config = ConfigDict(frozen=False)

    category: str = Field(..., description="Suggested category path")
    participants: List[str] = Field(
        default_factory=list, description="Suggested participants"
    )
    tags: List[str] = Field(default_factory=list, description="Suggested tags")


class UserCorrection(BaseModel):
    """User's correction to an AI suggestion.

    Captures modifications made by the user during review,
    including the reason for correction.
    """

    model_config = ConfigDict(frozen=False)

    category: Optional[str] = Field(default=None, description="Corrected category")
    participants: Optional[List[str]] = Field(
        default=None, description="Corrected participants"
    )
    tags: Optional[List[str]] = Field(default=None, description="Corrected tags")
    reason: Optional[str] = Field(default=None, description="Reason code for correction")
    notes: Optional[str] = Field(default=None, description="Free-form correction notes")


# =============================================================================
# DATA TRANSFER OBJECTS (DTOs)
# =============================================================================


class ReviewItemDTO(BaseModel):
    """Data transfer object for review queue items.

    Represents an individual item in the review queue, linking
    AI processing results to the review workflow state.
    """

    model_config = ConfigDict(frozen=False)

    # Identification
    id: int = Field(..., description="Item identifier")
    tenant_id: UUID = Field(..., description="Tenant isolation")
    session_id: Optional[int] = Field(
        default=None, description="Associated session (NULL if unqueued)"
    )
    source_id: int = Field(..., description="Source content reference")
    source_system: Optional[str] = Field(
        default=None, description="Source system name (e.g., gmail, slack)"
    )
    processing_result_id: Optional[UUID] = Field(
        default=None, description="AI processing result reference"
    )

    # Queue position and status
    queue_position: Optional[int] = Field(default=None, description="Position in queue")
    status: ReviewItemStatus = Field(
        default=ReviewItemStatus.PENDING, description="Current item status"
    )

    # Content information
    content_type: ContentType = Field(..., description="Type of content")
    content_preview: str = Field(..., description="Truncated content for display")

    # AI suggestion
    ai_suggestion: AISuggestion = Field(..., description="AI's suggested categorization")
    ai_confidence: Decimal = Field(
        ...,
        ge=Decimal("0.000"),
        le=Decimal("1.000"),
        description="AI confidence score",
    )
    ai_model: str = Field(..., description="Model that generated suggestion")

    # Business context
    business_importance: int = Field(
        default=5, ge=1, le=10, description="Importance score 1-10"
    )
    source_timestamp: datetime = Field(..., description="Original content timestamp")

    # Review tracking
    reviewed_at: Optional[datetime] = Field(default=None, description="When reviewed")
    review_duration_ms: Optional[int] = Field(
        default=None, description="Time spent on review"
    )
    user_decision: Optional[DecisionType] = Field(default=None, description="User decision")
    user_correction: Optional[UserCorrection] = Field(
        default=None, description="User's correction if modified"
    )

    # Batch and undo support
    batch_id: Optional[UUID] = Field(
        default=None, description="If part of batch operation"
    )
    undo_eligible: bool = Field(default=True, description="Can be undone")
    undo_deadline: Optional[datetime] = Field(
        default=None, description="Deadline for undo"
    )

    # Timestamps
    created_at: datetime = Field(..., description="Record creation")
    updated_at: datetime = Field(..., description="Last update")


class SessionDTO(BaseModel):
    """Data transfer object for review sessions.

    Represents a user's active review workflow session with
    state management and progress tracking.
    """

    model_config = ConfigDict(frozen=False)

    # Identification
    id: int = Field(..., description="Session identifier")
    session_uuid: UUID = Field(..., description="External session reference")
    tenant_id: UUID = Field(..., description="Tenant isolation")
    user_email: EmailStr = Field(..., description="User performing review")

    # Session state
    status: SessionStatus = Field(
        default=SessionStatus.ACTIVE, description="Session status"
    )
    started_at: datetime = Field(..., description="Session start time")
    last_activity_at: datetime = Field(..., description="Last interaction time")
    completed_at: Optional[datetime] = Field(
        default=None, description="Session completion time"
    )
    expires_at: datetime = Field(..., description="Auto-expiration time")

    # Progress tracking
    current_position: int = Field(default=0, description="Current queue position")
    total_items: int = Field(..., description="Total items in queue at start")
    items_reviewed: int = Field(default=0, description="Count of reviewed items")
    items_accepted: int = Field(default=0, description="Count of accepted items")
    items_rejected: int = Field(default=0, description="Count of rejected items")
    items_modified: int = Field(default=0, description="Count of modified items")
    items_skipped: int = Field(default=0, description="Count of skipped items")

    # Configuration
    review_mode: ReviewMode = Field(
        default=ReviewMode.STANDARD, description="Review mode"
    )
    priority_mode: PriorityMode = Field(
        default=PriorityMode.MIXED, description="Priority mode"
    )
    filter_criteria: Dict[str, Any] = Field(
        default_factory=dict, description="Active filters"
    )
    session_metadata: Dict[str, Any] = Field(
        default_factory=dict, description="Additional session state"
    )

    # Timestamps
    created_at: datetime = Field(..., description="Record creation")
    updated_at: datetime = Field(..., description="Last update")


class UserFeedbackDTO(BaseModel):
    """Data transfer object for user feedback records.

    Captures user validation decisions for learning system integration.
    """

    model_config = ConfigDict(frozen=False)

    # Identification
    id: int = Field(..., description="Feedback identifier")
    feedback_uuid: UUID = Field(..., description="External feedback reference")
    tenant_id: UUID = Field(..., description="Tenant isolation")
    review_item_id: int = Field(..., description="Associated review item")
    session_id: int = Field(..., description="Session context")

    # Decision details
    decision_type: DecisionType = Field(..., description="accept, reject, modify")
    original_suggestion: AISuggestion = Field(..., description="AI's original suggestion")
    user_correction: Optional[UserCorrection] = Field(
        default=None, description="User's correction (for modify)"
    )
    correction_reason: Optional[str] = Field(
        default=None, description="Reason code for correction"
    )
    correction_notes: Optional[str] = Field(
        default=None, description="Free-form explanation"
    )
    confidence_feedback: Optional[str] = Field(
        default=None, description="too_high, accurate, too_low"
    )

    # Timing
    time_spent_ms: int = Field(..., description="Time user spent on decision")

    # Batch context
    was_batch_decision: bool = Field(
        default=False, description="Part of batch operation"
    )
    batch_id: Optional[UUID] = Field(default=None, description="Batch operation reference")

    # Learning integration
    learning_rule_id: Optional[int] = Field(
        default=None, description="Generated learning rule"
    )
    learning_signal_quality: Optional[Decimal] = Field(
        default=None,
        ge=Decimal("0.00"),
        le=Decimal("1.00"),
        description="Quality score",
    )

    # Timestamp
    created_at: datetime = Field(..., description="Record creation")


class LearningRuleDTO(BaseModel):
    """Data transfer object for learning rules.

    Represents automated decision patterns derived from user feedback.
    """

    model_config = ConfigDict(frozen=False)

    # Identification
    id: int = Field(..., description="Rule identifier")
    rule_uuid: UUID = Field(..., description="External rule reference")
    tenant_id: UUID = Field(..., description="Tenant isolation")

    # Rule definition
    name: str = Field(..., max_length=200, description="Human-readable rule name")
    description: Optional[str] = Field(default=None, description="Rule description")
    status: LearningRuleStatus = Field(
        default=LearningRuleStatus.ACTIVE, description="Rule status"
    )
    rule_type: LearningRuleType = Field(..., description="Type of rule")
    match_criteria: Dict[str, Any] = Field(
        ..., description="Conditions for rule match"
    )
    action: Dict[str, Any] = Field(..., description="Action to apply when matched")

    # Thresholds
    confidence_threshold: Decimal = Field(
        default=Decimal("0.800"),
        ge=Decimal("0.000"),
        le=Decimal("1.000"),
        description="Min confidence to apply",
    )
    priority: int = Field(
        default=100, ge=1, le=1000, description="Lower = higher priority"
    )

    # Statistics
    derived_from_count: int = Field(
        default=0, description="Number of feedback items that created rule"
    )
    times_applied: int = Field(default=0, description="Application count")
    times_overridden: int = Field(default=0, description="User override count")
    effectiveness_score: Optional[Decimal] = Field(
        default=None,
        ge=Decimal("0.000"),
        le=Decimal("1.000"),
        description="Calculated effectiveness",
    )
    last_applied_at: Optional[datetime] = Field(
        default=None, description="Last application time"
    )

    # Lifecycle
    expires_at: Optional[datetime] = Field(default=None, description="Optional expiration")
    created_by: str = Field(..., max_length=254, description="User who created rule")

    # Timestamps
    created_at: datetime = Field(..., description="Record creation")
    updated_at: datetime = Field(..., description="Last update")

    # Soft delete
    is_deleted: bool = Field(default=False, description="Soft delete flag")
    deleted_at: Optional[datetime] = Field(default=None, description="Deletion timestamp")


class BatchOperationDTO(BaseModel):
    """Data transfer object for batch operations.

    Groups related review items for bulk actions.
    """

    model_config = ConfigDict(frozen=False)

    # Identification
    id: int = Field(..., description="Batch identifier")
    batch_uuid: UUID = Field(..., description="External batch reference")
    tenant_id: UUID = Field(..., description="Tenant isolation")
    session_id: int = Field(..., description="Parent session")

    # Batch definition
    batch_type: BatchType = Field(..., description="Grouping type")
    group_criteria: Dict[str, Any] = Field(..., description="How items were grouped")
    action_type: BatchActionType = Field(..., description="Action to apply")
    action_details: Dict[str, Any] = Field(..., description="Action parameters")
    item_count: int = Field(..., gt=0, description="Number of items in batch")

    # Status tracking
    status: BatchOperationStatus = Field(
        default=BatchOperationStatus.PENDING, description="Operation status"
    )
    confirmed_at: Optional[datetime] = Field(
        default=None, description="User confirmation time"
    )
    applied_at: Optional[datetime] = Field(
        default=None, description="Action application time"
    )
    undone_at: Optional[datetime] = Field(
        default=None, description="Undo time if reversed"
    )

    # Undo support
    undo_eligible: bool = Field(default=True, description="Can be undone")
    undo_deadline: Optional[datetime] = Field(default=None, description="Deadline for undo")

    # Timestamps
    created_at: datetime = Field(..., description="Record creation")
    updated_at: datetime = Field(..., description="Last update")


class ReviewAnalyticsDTO(BaseModel):
    """Data transfer object for review analytics (P3).

    Aggregated metrics for review patterns and system learning progress.
    """

    model_config = ConfigDict(frozen=False)

    # Identification
    id: int = Field(..., description="Analytics identifier")
    tenant_id: UUID = Field(..., description="Tenant isolation")

    # Period
    period_start: date = Field(..., description="Analytics period start")
    period_end: date = Field(..., description="Analytics period end")
    period_type: AnalyticsPeriodType = Field(..., description="Period granularity")

    # Session metrics
    total_sessions: int = Field(..., description="Session count")
    total_items_reviewed: int = Field(..., description="Items reviewed")
    avg_session_duration_min: Decimal = Field(..., description="Average session length")
    avg_items_per_minute: Decimal = Field(..., description="Review velocity")

    # Decision breakdown
    acceptance_rate: Decimal = Field(..., description="Accept percentage")
    modification_rate: Decimal = Field(..., description="Modify percentage")
    rejection_rate: Decimal = Field(..., description="Reject percentage")
    skip_rate: Decimal = Field(..., description="Skip percentage")

    # AI performance
    avg_ai_confidence: Decimal = Field(..., description="Mean confidence score")
    ai_accuracy: Optional[Decimal] = Field(default=None, description="Calculated AI accuracy")

    # Feature usage
    batch_usage_rate: Decimal = Field(..., description="Batch operation frequency")
    learning_rules_created: int = Field(..., description="New rules count")
    learning_rules_applied: int = Field(..., description="Rule applications")

    # Breakdowns
    content_type_breakdown: Dict[str, int] = Field(
        ..., description="By content type metrics"
    )
    correction_type_breakdown: Dict[str, int] = Field(
        ..., description="By correction type"
    )

    # Timestamp
    created_at: datetime = Field(..., description="Record creation")

    @model_validator(mode="after")
    def validate_period(self) -> "ReviewAnalyticsDTO":
        """Ensure period_end is not before period_start."""
        if self.period_end < self.period_start:
            raise ValueError("period_end must be >= period_start")
        return self

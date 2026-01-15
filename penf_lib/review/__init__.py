"""Daily Review workflow module.

This module provides the core functionality for the daily review workflow,
allowing users to review and validate AI processing results for emails,
meetings, and documents.

Key components:
- models: Pydantic DTOs and enums for review entities
- queue: Queue management and prioritization
- session: Session lifecycle management
- exceptions: Custom exceptions for error handling
- feedback: User feedback capture and learning integration (future)
- batch: Batch operations support (future)
- analytics: Review analytics (P3, future)
"""

from penf_lib.review.exceptions import (
    ActiveSessionExistsError,
    BatchOperationError,
    DatabaseOperationError,
    InvalidFilterCriteriaError,
    InvalidSessionStateError,
    ItemNotFoundError,
    PendingItemsError,
    ReviewError,
    SessionExpiredError,
    SessionNotFoundError,
    UndoNotEligibleError,
)
from penf_lib.review.models import (
    # Enums
    AnalyticsPeriodType,
    BatchActionType,
    BatchOperationStatus,
    BatchType,
    ContentType,
    DecisionType,
    LearningRuleStatus,
    LearningRuleType,
    PriorityMode,
    ReviewItemStatus,
    ReviewMode,
    SessionStatus,
    # Nested models
    AISuggestion,
    UserCorrection,
    # DTOs
    BatchOperationDTO,
    LearningRuleDTO,
    ReviewAnalyticsDTO,
    ReviewItemDTO,
    SessionDTO,
    UserFeedbackDTO,
)
from penf_lib.review.queue import QueueManager
from penf_lib.review.queue import ReviewRepositoryProtocol as QueueRepositoryProtocol
from penf_lib.review.session import SessionManager

__all__ = [
    # Enums
    "ReviewMode",
    "PriorityMode",
    "DecisionType",
    "SessionStatus",
    "ReviewItemStatus",
    "ContentType",
    "BatchType",
    "BatchActionType",
    "BatchOperationStatus",
    "LearningRuleStatus",
    "LearningRuleType",
    "AnalyticsPeriodType",
    # Nested models
    "AISuggestion",
    "UserCorrection",
    # DTOs
    "ReviewItemDTO",
    "SessionDTO",
    "UserFeedbackDTO",
    "LearningRuleDTO",
    "BatchOperationDTO",
    "ReviewAnalyticsDTO",
    # Queue management
    "QueueManager",
    "QueueRepositoryProtocol",
    # Session management
    "SessionManager",
    # Exceptions
    "ReviewError",
    "SessionNotFoundError",
    "ActiveSessionExistsError",
    "SessionExpiredError",
    "InvalidSessionStateError",
    "ItemNotFoundError",
    "UndoNotEligibleError",
    "PendingItemsError",
    "BatchOperationError",
    "InvalidFilterCriteriaError",
    "DatabaseOperationError",
]

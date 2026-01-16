"""Daily Review workflow module.

This module provides the core functionality for the daily review workflow,
allowing users to review and validate AI processing results for emails,
meetings, and documents.

Key components:
- models: Pydantic DTOs and enums for review entities
- queue: Queue management and prioritization
- session: Session lifecycle management
- exceptions: Custom exceptions for error handling
- feedback: User feedback capture and learning integration
- batch: Batch operations support
- rules: Learning rule management
- analytics: Review analytics and insights
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
from penf_lib.review.feedback import FeedbackManager
from penf_lib.review.feedback import FeedbackRepositoryProtocol
from penf_lib.review.batch import BatchManager
from penf_lib.review.batch import BatchRepositoryProtocol
from penf_lib.review.batch import BatchGroupType, BatchAction as BatchActionEnum
from penf_lib.review.batch import BatchPreview, BatchResult, FilterExpression
from penf_lib.review.rules import RuleManager
from penf_lib.review.rules import RuleRepositoryProtocol
from penf_lib.review.rules import RuleApplicationResult, RuleSuggestion
from penf_lib.review.analytics import AnalyticsManager
from penf_lib.review.analytics import AnalyticsRepositoryProtocol
from penf_lib.review.analytics import (
    TrendData,
    ReviewInsight,
    ContentTypeStats,
    CorrectionTypeStats,
    AnalyticsSummary,
)

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
    # Feedback management
    "FeedbackManager",
    "FeedbackRepositoryProtocol",
    # Batch management
    "BatchManager",
    "BatchRepositoryProtocol",
    "BatchGroupType",
    "BatchActionEnum",
    "BatchPreview",
    "BatchResult",
    "FilterExpression",
    # Rule management
    "RuleManager",
    "RuleRepositoryProtocol",
    "RuleApplicationResult",
    "RuleSuggestion",
    # Analytics management
    "AnalyticsManager",
    "AnalyticsRepositoryProtocol",
    "TrendData",
    "ReviewInsight",
    "ContentTypeStats",
    "CorrectionTypeStats",
    "AnalyticsSummary",
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

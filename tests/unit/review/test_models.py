"""Unit tests for review module Pydantic models.

Tests the data transfer objects and enums used in the daily review workflow.
Following TDD - these tests are written before the implementation.
"""

import pytest
from datetime import datetime, timedelta, timezone
from decimal import Decimal
from uuid import UUID, uuid4

from pydantic import ValidationError


class TestReviewModeEnum:
    """Tests for ReviewMode enumeration."""

    def test_review_mode_values(self):
        """Test all expected review mode values exist."""
        from penf_lib.review.models import ReviewMode

        assert ReviewMode.QUICK == "quick"
        assert ReviewMode.STANDARD == "standard"
        assert ReviewMode.DETAILED == "detailed"

    def test_review_mode_from_string(self):
        """Test creating ReviewMode from string."""
        from penf_lib.review.models import ReviewMode

        assert ReviewMode("quick") == ReviewMode.QUICK
        assert ReviewMode("standard") == ReviewMode.STANDARD
        assert ReviewMode("detailed") == ReviewMode.DETAILED


class TestPriorityModeEnum:
    """Tests for PriorityMode enumeration."""

    def test_priority_mode_values(self):
        """Test all expected priority mode values exist."""
        from penf_lib.review.models import PriorityMode

        assert PriorityMode.CONFIDENCE == "confidence"
        assert PriorityMode.IMPORTANCE == "importance"
        assert PriorityMode.RECENCY == "recency"
        assert PriorityMode.MIXED == "mixed"


class TestDecisionTypeEnum:
    """Tests for DecisionType enumeration."""

    def test_decision_type_values(self):
        """Test all expected decision type values exist."""
        from penf_lib.review.models import DecisionType

        assert DecisionType.ACCEPT == "accept"
        assert DecisionType.REJECT == "reject"
        assert DecisionType.MODIFY == "modify"
        assert DecisionType.SKIP == "skip"


class TestSessionStatusEnum:
    """Tests for SessionStatus enumeration."""

    def test_session_status_values(self):
        """Test all expected session status values exist."""
        from penf_lib.review.models import SessionStatus

        assert SessionStatus.ACTIVE == "active"
        assert SessionStatus.PAUSED == "paused"
        assert SessionStatus.COMPLETED == "completed"
        assert SessionStatus.ABANDONED == "abandoned"


class TestReviewItemStatusEnum:
    """Tests for ReviewItemStatus enumeration."""

    def test_review_item_status_values(self):
        """Test all expected review item status values exist."""
        from penf_lib.review.models import ReviewItemStatus

        assert ReviewItemStatus.PENDING == "pending"
        assert ReviewItemStatus.IN_REVIEW == "in_review"
        assert ReviewItemStatus.ACCEPTED == "accepted"
        assert ReviewItemStatus.REJECTED == "rejected"
        assert ReviewItemStatus.MODIFIED == "modified"
        assert ReviewItemStatus.SKIPPED == "skipped"


class TestContentTypeEnum:
    """Tests for ContentType enumeration."""

    def test_content_type_values(self):
        """Test all expected content type values exist."""
        from penf_lib.review.models import ContentType

        assert ContentType.EMAIL == "email"
        assert ContentType.MEETING == "meeting"
        assert ContentType.DOCUMENT == "document"


class TestBatchTypeEnum:
    """Tests for BatchType enumeration."""

    def test_batch_type_values(self):
        """Test all expected batch type values exist."""
        from penf_lib.review.models import BatchType

        assert BatchType.THREAD == "thread"
        assert BatchType.SENDER == "sender"
        assert BatchType.CATEGORY == "category"
        assert BatchType.TIME_WINDOW == "time_window"
        assert BatchType.CUSTOM == "custom"


class TestBatchActionTypeEnum:
    """Tests for BatchActionType enumeration."""

    def test_batch_action_type_values(self):
        """Test all expected batch action type values exist."""
        from penf_lib.review.models import BatchActionType

        assert BatchActionType.ACCEPT_ALL == "accept_all"
        assert BatchActionType.REJECT_ALL == "reject_all"
        assert BatchActionType.APPLY_CATEGORY == "apply_category"
        assert BatchActionType.SKIP_ALL == "skip_all"


class TestBatchOperationStatusEnum:
    """Tests for BatchOperationStatus enumeration."""

    def test_batch_operation_status_values(self):
        """Test all expected batch operation status values exist."""
        from penf_lib.review.models import BatchOperationStatus

        assert BatchOperationStatus.PENDING == "pending"
        assert BatchOperationStatus.CONFIRMED == "confirmed"
        assert BatchOperationStatus.APPLIED == "applied"
        assert BatchOperationStatus.UNDONE == "undone"


class TestLearningRuleStatusEnum:
    """Tests for LearningRuleStatus enumeration."""

    def test_learning_rule_status_values(self):
        """Test all expected learning rule status values exist."""
        from penf_lib.review.models import LearningRuleStatus

        assert LearningRuleStatus.ACTIVE == "active"
        assert LearningRuleStatus.DISABLED == "disabled"
        assert LearningRuleStatus.EXPIRED == "expired"


class TestLearningRuleTypeEnum:
    """Tests for LearningRuleType enumeration."""

    def test_learning_rule_type_values(self):
        """Test all expected learning rule type values exist."""
        from penf_lib.review.models import LearningRuleType

        assert LearningRuleType.SENDER_MATCH == "sender_match"
        assert LearningRuleType.CONTENT_PATTERN == "content_pattern"
        assert LearningRuleType.CATEGORY_OVERRIDE == "category_override"
        assert LearningRuleType.PARTICIPANT_MAPPING == "participant_mapping"


class TestAISuggestionDTO:
    """Tests for AISuggestion data transfer object."""

    def test_create_valid_ai_suggestion(self):
        """Test creating a valid AI suggestion."""
        from penf_lib.review.models import AISuggestion

        suggestion = AISuggestion(
            category="project/infrastructure",
            participants=["John Doe", "Jane Smith"],
            tags=["decision", "planning"],
        )

        assert suggestion.category == "project/infrastructure"
        assert suggestion.participants == ["John Doe", "Jane Smith"]
        assert suggestion.tags == ["decision", "planning"]

    def test_ai_suggestion_optional_fields(self):
        """Test AI suggestion with optional fields."""
        from penf_lib.review.models import AISuggestion

        suggestion = AISuggestion(category="email/newsletter")

        assert suggestion.category == "email/newsletter"
        assert suggestion.participants == []
        assert suggestion.tags == []

    def test_ai_suggestion_to_dict(self):
        """Test AI suggestion serializes to dict."""
        from penf_lib.review.models import AISuggestion

        suggestion = AISuggestion(
            category="project/infrastructure",
            participants=["John Doe"],
            tags=["decision"],
        )

        data = suggestion.model_dump()
        assert data["category"] == "project/infrastructure"
        assert data["participants"] == ["John Doe"]


class TestUserCorrectionDTO:
    """Tests for UserCorrection data transfer object."""

    def test_create_valid_user_correction(self):
        """Test creating a valid user correction."""
        from penf_lib.review.models import UserCorrection

        correction = UserCorrection(
            category="project/backend",
            reason="wrong_category",
            notes="This is about backend, not infrastructure",
        )

        assert correction.category == "project/backend"
        assert correction.reason == "wrong_category"
        assert correction.notes == "This is about backend, not infrastructure"

    def test_user_correction_optional_fields(self):
        """Test user correction with minimal fields."""
        from penf_lib.review.models import UserCorrection

        correction = UserCorrection()

        assert correction.category is None
        assert correction.reason is None


class TestReviewItemDTO:
    """Tests for ReviewItem data transfer object."""

    def test_create_valid_review_item(self):
        """Test creating a valid review item DTO."""
        from penf_lib.review.models import (
            ReviewItemDTO,
            ReviewItemStatus,
            ContentType,
            AISuggestion,
        )

        tenant_id = uuid4()
        source_id = 123
        now = datetime.now(timezone.utc)

        item = ReviewItemDTO(
            id=1,
            tenant_id=tenant_id,
            source_id=source_id,
            status=ReviewItemStatus.PENDING,
            content_type=ContentType.EMAIL,
            content_preview="Thanks for the update...",
            ai_suggestion=AISuggestion(category="project/test"),
            ai_confidence=Decimal("0.850"),
            ai_model="llama-3.1-8b",
            source_timestamp=now,
            created_at=now,
            updated_at=now,
        )

        assert item.id == 1
        assert item.tenant_id == tenant_id
        assert item.status == ReviewItemStatus.PENDING
        assert item.ai_confidence == Decimal("0.850")

    def test_review_item_confidence_validation(self):
        """Test AI confidence must be between 0 and 1."""
        from penf_lib.review.models import (
            ReviewItemDTO,
            ReviewItemStatus,
            ContentType,
            AISuggestion,
        )

        with pytest.raises(ValidationError) as exc_info:
            ReviewItemDTO(
                id=1,
                tenant_id=uuid4(),
                source_id=123,
                status=ReviewItemStatus.PENDING,
                content_type=ContentType.EMAIL,
                content_preview="Test",
                ai_suggestion=AISuggestion(category="test"),
                ai_confidence=Decimal("1.500"),  # Invalid: > 1
                ai_model="test-model",
                source_timestamp=datetime.now(timezone.utc),
                created_at=datetime.now(timezone.utc),
                updated_at=datetime.now(timezone.utc),
            )

        assert "ai_confidence" in str(exc_info.value)

    def test_review_item_business_importance_validation(self):
        """Test business importance must be between 1 and 10."""
        from penf_lib.review.models import (
            ReviewItemDTO,
            ReviewItemStatus,
            ContentType,
            AISuggestion,
        )

        with pytest.raises(ValidationError) as exc_info:
            ReviewItemDTO(
                id=1,
                tenant_id=uuid4(),
                source_id=123,
                status=ReviewItemStatus.PENDING,
                content_type=ContentType.EMAIL,
                content_preview="Test",
                ai_suggestion=AISuggestion(category="test"),
                ai_confidence=Decimal("0.500"),
                ai_model="test-model",
                business_importance=15,  # Invalid: > 10
                source_timestamp=datetime.now(timezone.utc),
                created_at=datetime.now(timezone.utc),
                updated_at=datetime.now(timezone.utc),
            )

        assert "business_importance" in str(exc_info.value)


class TestSessionDTO:
    """Tests for ReviewSession data transfer object."""

    def test_create_valid_session(self):
        """Test creating a valid session DTO."""
        from penf_lib.review.models import (
            SessionDTO,
            SessionStatus,
            ReviewMode,
            PriorityMode,
        )

        tenant_id = uuid4()
        session_uuid = uuid4()
        now = datetime.now(timezone.utc)
        expires = now + timedelta(hours=24)

        session = SessionDTO(
            id=1,
            session_uuid=session_uuid,
            tenant_id=tenant_id,
            user_email="user@example.com",
            status=SessionStatus.ACTIVE,
            started_at=now,
            last_activity_at=now,
            expires_at=expires,
            total_items=100,
            review_mode=ReviewMode.STANDARD,
            priority_mode=PriorityMode.MIXED,
            created_at=now,
            updated_at=now,
        )

        assert session.id == 1
        assert session.session_uuid == session_uuid
        assert session.user_email == "user@example.com"
        assert session.total_items == 100

    def test_session_email_validation(self):
        """Test session validates email format."""
        from penf_lib.review.models import (
            SessionDTO,
            SessionStatus,
            ReviewMode,
            PriorityMode,
        )

        now = datetime.now(timezone.utc)

        with pytest.raises(ValidationError) as exc_info:
            SessionDTO(
                id=1,
                session_uuid=uuid4(),
                tenant_id=uuid4(),
                user_email="not-an-email",  # Invalid email
                status=SessionStatus.ACTIVE,
                started_at=now,
                last_activity_at=now,
                expires_at=now + timedelta(hours=24),
                total_items=100,
                review_mode=ReviewMode.STANDARD,
                priority_mode=PriorityMode.MIXED,
                created_at=now,
                updated_at=now,
            )

        assert "user_email" in str(exc_info.value)

    def test_session_counts_defaults(self):
        """Test session count fields default to zero."""
        from penf_lib.review.models import (
            SessionDTO,
            SessionStatus,
            ReviewMode,
            PriorityMode,
        )

        now = datetime.now(timezone.utc)

        session = SessionDTO(
            id=1,
            session_uuid=uuid4(),
            tenant_id=uuid4(),
            user_email="user@example.com",
            status=SessionStatus.ACTIVE,
            started_at=now,
            last_activity_at=now,
            expires_at=now + timedelta(hours=24),
            total_items=100,
            review_mode=ReviewMode.STANDARD,
            priority_mode=PriorityMode.MIXED,
            created_at=now,
            updated_at=now,
        )

        assert session.current_position == 0
        assert session.items_reviewed == 0
        assert session.items_accepted == 0
        assert session.items_rejected == 0
        assert session.items_modified == 0
        assert session.items_skipped == 0


class TestUserFeedbackDTO:
    """Tests for UserFeedback data transfer object."""

    def test_create_valid_user_feedback(self):
        """Test creating a valid user feedback DTO."""
        from penf_lib.review.models import UserFeedbackDTO, DecisionType, AISuggestion

        now = datetime.now(timezone.utc)
        feedback_uuid = uuid4()

        feedback = UserFeedbackDTO(
            id=1,
            feedback_uuid=feedback_uuid,
            tenant_id=uuid4(),
            review_item_id=100,
            session_id=10,
            decision_type=DecisionType.ACCEPT,
            original_suggestion=AISuggestion(category="project/test"),
            time_spent_ms=5000,
            created_at=now,
        )

        assert feedback.id == 1
        assert feedback.decision_type == DecisionType.ACCEPT
        assert feedback.time_spent_ms == 5000

    def test_user_feedback_learning_signal_quality_validation(self):
        """Test learning signal quality must be between 0 and 1."""
        from penf_lib.review.models import UserFeedbackDTO, DecisionType, AISuggestion

        with pytest.raises(ValidationError) as exc_info:
            UserFeedbackDTO(
                id=1,
                feedback_uuid=uuid4(),
                tenant_id=uuid4(),
                review_item_id=100,
                session_id=10,
                decision_type=DecisionType.ACCEPT,
                original_suggestion=AISuggestion(category="project/test"),
                time_spent_ms=5000,
                learning_signal_quality=Decimal("1.50"),  # Invalid: > 1
                created_at=datetime.now(timezone.utc),
            )

        assert "learning_signal_quality" in str(exc_info.value)


class TestLearningRuleDTO:
    """Tests for LearningRule data transfer object."""

    def test_create_valid_learning_rule(self):
        """Test creating a valid learning rule DTO."""
        from penf_lib.review.models import (
            LearningRuleDTO,
            LearningRuleStatus,
            LearningRuleType,
        )

        now = datetime.now(timezone.utc)

        rule = LearningRuleDTO(
            id=1,
            rule_uuid=uuid4(),
            tenant_id=uuid4(),
            name="Auto-categorize John's emails",
            status=LearningRuleStatus.ACTIVE,
            rule_type=LearningRuleType.SENDER_MATCH,
            match_criteria={"sender": "john@example.com"},
            action={"category": "project/infrastructure"},
            created_by="user@example.com",
            created_at=now,
            updated_at=now,
        )

        assert rule.name == "Auto-categorize John's emails"
        assert rule.status == LearningRuleStatus.ACTIVE
        assert rule.rule_type == LearningRuleType.SENDER_MATCH

    def test_learning_rule_confidence_threshold_validation(self):
        """Test confidence threshold must be between 0 and 1."""
        from penf_lib.review.models import (
            LearningRuleDTO,
            LearningRuleStatus,
            LearningRuleType,
        )

        with pytest.raises(ValidationError) as exc_info:
            LearningRuleDTO(
                id=1,
                rule_uuid=uuid4(),
                tenant_id=uuid4(),
                name="Test rule",
                status=LearningRuleStatus.ACTIVE,
                rule_type=LearningRuleType.SENDER_MATCH,
                match_criteria={},
                action={},
                confidence_threshold=Decimal("1.500"),  # Invalid: > 1
                created_by="user@example.com",
                created_at=datetime.now(timezone.utc),
                updated_at=datetime.now(timezone.utc),
            )

        assert "confidence_threshold" in str(exc_info.value)

    def test_learning_rule_priority_validation(self):
        """Test priority must be between 1 and 1000."""
        from penf_lib.review.models import (
            LearningRuleDTO,
            LearningRuleStatus,
            LearningRuleType,
        )

        with pytest.raises(ValidationError) as exc_info:
            LearningRuleDTO(
                id=1,
                rule_uuid=uuid4(),
                tenant_id=uuid4(),
                name="Test rule",
                status=LearningRuleStatus.ACTIVE,
                rule_type=LearningRuleType.SENDER_MATCH,
                match_criteria={},
                action={},
                priority=1500,  # Invalid: > 1000
                created_by="user@example.com",
                created_at=datetime.now(timezone.utc),
                updated_at=datetime.now(timezone.utc),
            )

        assert "priority" in str(exc_info.value)


class TestBatchOperationDTO:
    """Tests for BatchOperation data transfer object."""

    def test_create_valid_batch_operation(self):
        """Test creating a valid batch operation DTO."""
        from penf_lib.review.models import (
            BatchOperationDTO,
            BatchType,
            BatchActionType,
            BatchOperationStatus,
        )

        now = datetime.now(timezone.utc)

        batch = BatchOperationDTO(
            id=1,
            batch_uuid=uuid4(),
            tenant_id=uuid4(),
            session_id=10,
            batch_type=BatchType.SENDER,
            group_criteria={"sender": "john@example.com"},
            action_type=BatchActionType.ACCEPT_ALL,
            action_details={},
            item_count=5,
            status=BatchOperationStatus.PENDING,
            created_at=now,
            updated_at=now,
        )

        assert batch.batch_type == BatchType.SENDER
        assert batch.action_type == BatchActionType.ACCEPT_ALL
        assert batch.item_count == 5

    def test_batch_operation_item_count_validation(self):
        """Test item count must be positive."""
        from penf_lib.review.models import (
            BatchOperationDTO,
            BatchType,
            BatchActionType,
            BatchOperationStatus,
        )

        with pytest.raises(ValidationError) as exc_info:
            BatchOperationDTO(
                id=1,
                batch_uuid=uuid4(),
                tenant_id=uuid4(),
                session_id=10,
                batch_type=BatchType.SENDER,
                group_criteria={},
                action_type=BatchActionType.ACCEPT_ALL,
                action_details={},
                item_count=0,  # Invalid: must be > 0
                status=BatchOperationStatus.PENDING,
                created_at=datetime.now(timezone.utc),
                updated_at=datetime.now(timezone.utc),
            )

        assert "item_count" in str(exc_info.value)


class TestReviewAnalyticsDTO:
    """Tests for ReviewAnalytics data transfer object."""

    def test_create_valid_review_analytics(self):
        """Test creating a valid review analytics DTO."""
        from penf_lib.review.models import ReviewAnalyticsDTO, AnalyticsPeriodType
        from datetime import date

        now = datetime.now(timezone.utc)
        today = date.today()

        analytics = ReviewAnalyticsDTO(
            id=1,
            tenant_id=uuid4(),
            period_start=today,
            period_end=today,
            period_type=AnalyticsPeriodType.DAILY,
            total_sessions=10,
            total_items_reviewed=500,
            avg_session_duration_min=Decimal("15.5"),
            avg_items_per_minute=Decimal("8.2"),
            acceptance_rate=Decimal("0.7500"),
            modification_rate=Decimal("0.1500"),
            rejection_rate=Decimal("0.0500"),
            skip_rate=Decimal("0.0500"),
            avg_ai_confidence=Decimal("0.850"),
            batch_usage_rate=Decimal("0.2000"),
            learning_rules_created=3,
            learning_rules_applied=25,
            content_type_breakdown={"email": 400, "meeting": 100},
            correction_type_breakdown={"category": 50, "participants": 25},
            created_at=now,
        )

        assert analytics.total_sessions == 10
        assert analytics.acceptance_rate == Decimal("0.7500")

    def test_review_analytics_period_validation(self):
        """Test period_end must be >= period_start."""
        from penf_lib.review.models import ReviewAnalyticsDTO, AnalyticsPeriodType
        from datetime import date

        with pytest.raises(ValidationError) as exc_info:
            ReviewAnalyticsDTO(
                id=1,
                tenant_id=uuid4(),
                period_start=date(2026, 1, 15),
                period_end=date(2026, 1, 10),  # Invalid: before start
                period_type=AnalyticsPeriodType.DAILY,
                total_sessions=10,
                total_items_reviewed=500,
                avg_session_duration_min=Decimal("15.5"),
                avg_items_per_minute=Decimal("8.2"),
                acceptance_rate=Decimal("0.7500"),
                modification_rate=Decimal("0.1500"),
                rejection_rate=Decimal("0.0500"),
                skip_rate=Decimal("0.0500"),
                avg_ai_confidence=Decimal("0.850"),
                batch_usage_rate=Decimal("0.2000"),
                learning_rules_created=3,
                learning_rules_applied=25,
                content_type_breakdown={},
                correction_type_breakdown={},
                created_at=datetime.now(timezone.utc),
            )

        # Validation error should mention period
        assert "period" in str(exc_info.value).lower()


class TestModuleImports:
    """Test that the module can be imported correctly."""

    def test_import_review_module(self):
        """Test that review module imports successfully."""
        from penf_lib.review import models

        assert hasattr(models, "ReviewMode")
        assert hasattr(models, "PriorityMode")
        assert hasattr(models, "DecisionType")
        assert hasattr(models, "ReviewItemDTO")
        assert hasattr(models, "SessionDTO")

    def test_import_from_review_init(self):
        """Test that models are exported from __init__.py."""
        from penf_lib.review import (
            ReviewMode,
            PriorityMode,
            DecisionType,
            ReviewItemDTO,
            SessionDTO,
        )

        assert ReviewMode.STANDARD == "standard"
        assert PriorityMode.MIXED == "mixed"

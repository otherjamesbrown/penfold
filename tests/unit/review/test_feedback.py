"""Unit tests for FeedbackManager.

Tests the feedback collection module that captures user decisions (accept/reject/modify/skip)
for the daily review workflow. Includes tests for recording decisions, undo functionality,
and learning signal quality calculations.
"""

from datetime import datetime, timedelta, timezone
from decimal import Decimal
from unittest.mock import AsyncMock, MagicMock
from uuid import uuid4

import pytest

from penf_lib.review.exceptions import UndoNotEligibleError
from penf_lib.review.models import (
    AISuggestion,
    ContentType,
    DecisionType,
    ReviewItemDTO,
    ReviewItemStatus,
    UserCorrection,
    UserFeedbackDTO,
)


# =============================================================================
# FIXTURES
# =============================================================================


@pytest.fixture
def tenant_id():
    """Test tenant ID."""
    return uuid4()


@pytest.fixture
def session_id():
    """Test session ID."""
    return 1


@pytest.fixture
def mock_repository():
    """Create mock repository for testing."""
    repo = AsyncMock()
    repo.create_feedback = AsyncMock()
    repo.get_feedback_by_id = AsyncMock()
    repo.get_undo_eligible_feedback = AsyncMock()
    repo.mark_feedback_undone = AsyncMock()
    repo.update_review_item = AsyncMock()
    return repo


@pytest.fixture
def feedback_manager(mock_repository):
    """Create FeedbackManager with mocked repository."""
    from penf_lib.review.feedback import FeedbackManager

    return FeedbackManager(repository=mock_repository)


def create_review_item(
    item_id: int = 1,
    tenant_id=None,
    session_id: int = 1,
    source_id: int = 100,
    queue_position: int = 1,
    status: ReviewItemStatus = ReviewItemStatus.PENDING,
    content_type: ContentType = ContentType.EMAIL,
    content_preview: str = "Re: Q1 Planning Meeting - Let's discuss the roadmap...",
    category: str = "project/planning",
    participants: list = None,
    tags: list = None,
    ai_confidence: Decimal = Decimal("0.85"),
    ai_model: str = "gemini-1.5",
    business_importance: int = 5,
    user_decision: DecisionType = None,
    user_correction: UserCorrection = None,
) -> ReviewItemDTO:
    """Create a ReviewItemDTO for testing."""
    if tenant_id is None:
        tenant_id = uuid4()
    if participants is None:
        participants = ["john@example.com", "jane@example.com"]
    if tags is None:
        tags = ["meeting", "q1", "planning"]

    now = datetime.now(timezone.utc)
    return ReviewItemDTO(
        id=item_id,
        tenant_id=tenant_id,
        session_id=session_id,
        source_id=source_id,
        queue_position=queue_position,
        status=status,
        content_type=content_type,
        content_preview=content_preview,
        ai_suggestion=AISuggestion(
            category=category,
            participants=participants,
            tags=tags,
        ),
        ai_confidence=ai_confidence,
        ai_model=ai_model,
        business_importance=business_importance,
        source_timestamp=now - timedelta(hours=2),
        user_decision=user_decision,
        user_correction=user_correction,
        created_at=now,
        updated_at=now,
    )


# Sentinel value to distinguish "not passed" from "explicitly passed as None"
_UNSET = object()


def create_mock_feedback(
    feedback_id: int = 1,
    tenant_id=None,
    review_item_id: int = 1,
    session_id: int = 1,
    decision_type: DecisionType = DecisionType.ACCEPT,
    time_spent_ms: int = 5000,
    undo_eligible: bool = True,
    undo_deadline: datetime = _UNSET,
    is_undone: bool = False,
    original_suggestion: AISuggestion = None,
    user_correction: UserCorrection = None,
) -> MagicMock:
    """Create a mock feedback object for testing."""
    if tenant_id is None:
        tenant_id = uuid4()
    # Use sentinel to allow explicitly setting undo_deadline to None
    if undo_deadline is _UNSET:
        undo_deadline = datetime.now(timezone.utc) + timedelta(minutes=5)
    if original_suggestion is None:
        original_suggestion = AISuggestion(
            category="project/planning",
            participants=["john@example.com"],
            tags=["meeting"],
        )

    now = datetime.now(timezone.utc)
    feedback = MagicMock()
    feedback.id = feedback_id
    feedback.feedback_uuid = uuid4()
    feedback.tenant_id = tenant_id
    feedback.review_item_id = review_item_id
    feedback.session_id = session_id
    feedback.decision_type = decision_type
    feedback.original_suggestion = original_suggestion
    feedback.user_correction = user_correction
    feedback.correction_reason = None
    feedback.correction_notes = None
    feedback.confidence_feedback = None
    feedback.time_spent_ms = time_spent_ms
    feedback.was_batch_decision = False
    feedback.batch_id = None
    feedback.learning_rule_id = None
    feedback.learning_signal_quality = None
    feedback.created_at = now
    feedback.undo_eligible = undo_eligible
    feedback.undo_deadline = undo_deadline
    feedback.is_undone = is_undone
    return feedback


# =============================================================================
# RECORD ACCEPT TESTS
# =============================================================================


class TestRecordAccept:
    """Tests for FeedbackManager.record_accept()."""

    @pytest.mark.asyncio
    async def test_record_accept_creates_feedback(
        self,
        feedback_manager,
        mock_repository: AsyncMock,
        tenant_id,
        session_id,
    ):
        """Test that recording accept creates feedback with correct decision type."""
        item = create_review_item(tenant_id=tenant_id, session_id=session_id)
        mock_feedback = create_mock_feedback(
            tenant_id=tenant_id,
            review_item_id=item.id,
            session_id=session_id,
            decision_type=DecisionType.ACCEPT,
        )
        mock_repository.create_feedback.return_value = mock_feedback

        result = await feedback_manager.record_accept(
            item=item,
            session_id=session_id,
            time_spent_ms=5000,
        )

        assert result.decision_type == DecisionType.ACCEPT
        mock_repository.create_feedback.assert_called_once()
        call_args = mock_repository.create_feedback.call_args[0][0]
        assert call_args["decision_type"] == DecisionType.ACCEPT

    @pytest.mark.asyncio
    async def test_record_accept_captures_original_suggestion(
        self,
        feedback_manager,
        mock_repository: AsyncMock,
        tenant_id,
        session_id,
    ):
        """Test that accept captures the AI's original suggestion."""
        item = create_review_item(
            tenant_id=tenant_id,
            session_id=session_id,
            category="project/alpha",
            participants=["alice@example.com", "bob@example.com"],
            tags=["urgent", "budget"],
        )
        mock_feedback = create_mock_feedback(
            tenant_id=tenant_id,
            review_item_id=item.id,
            session_id=session_id,
            original_suggestion=item.ai_suggestion,
        )
        mock_repository.create_feedback.return_value = mock_feedback

        await feedback_manager.record_accept(
            item=item,
            session_id=session_id,
            time_spent_ms=3000,
        )

        call_args = mock_repository.create_feedback.call_args[0][0]
        assert call_args["original_suggestion"] == item.ai_suggestion

    @pytest.mark.asyncio
    async def test_record_accept_tracks_time_spent(
        self,
        feedback_manager,
        mock_repository: AsyncMock,
        tenant_id,
        session_id,
    ):
        """Test that accept records time_spent_ms correctly."""
        item = create_review_item(tenant_id=tenant_id, session_id=session_id)
        mock_feedback = create_mock_feedback(
            tenant_id=tenant_id,
            review_item_id=item.id,
            session_id=session_id,
            time_spent_ms=7500,
        )
        mock_repository.create_feedback.return_value = mock_feedback

        await feedback_manager.record_accept(
            item=item,
            session_id=session_id,
            time_spent_ms=7500,
        )

        call_args = mock_repository.create_feedback.call_args[0][0]
        assert call_args["time_spent_ms"] == 7500

    @pytest.mark.asyncio
    async def test_record_accept_sets_undo_data_in_repository(
        self,
        feedback_manager,
        mock_repository: AsyncMock,
        tenant_id,
        session_id,
    ):
        """Test that accept sets undo eligibility with deadline in repository call."""
        item = create_review_item(tenant_id=tenant_id, session_id=session_id)
        mock_feedback = create_mock_feedback(
            tenant_id=tenant_id,
            review_item_id=item.id,
            session_id=session_id,
            undo_eligible=True,
        )
        mock_repository.create_feedback.return_value = mock_feedback

        await feedback_manager.record_accept(
            item=item,
            session_id=session_id,
            time_spent_ms=5000,
        )

        # Verify undo data is passed to repository
        call_args = mock_repository.create_feedback.call_args[0][0]
        assert call_args["undo_eligible"] is True
        assert call_args["undo_deadline"] is not None
        # Undo deadline should be in the future
        assert call_args["undo_deadline"] > datetime.now(timezone.utc)


# =============================================================================
# RECORD REJECT TESTS
# =============================================================================


class TestRecordReject:
    """Tests for FeedbackManager.record_reject()."""

    @pytest.mark.asyncio
    async def test_record_reject_creates_feedback(
        self,
        feedback_manager,
        mock_repository: AsyncMock,
        tenant_id,
        session_id,
    ):
        """Test that recording reject creates feedback with correct decision type."""
        item = create_review_item(tenant_id=tenant_id, session_id=session_id)
        mock_feedback = create_mock_feedback(
            tenant_id=tenant_id,
            review_item_id=item.id,
            session_id=session_id,
            decision_type=DecisionType.REJECT,
        )
        mock_repository.create_feedback.return_value = mock_feedback

        result = await feedback_manager.record_reject(
            item=item,
            session_id=session_id,
            time_spent_ms=3000,
        )

        assert result.decision_type == DecisionType.REJECT
        mock_repository.create_feedback.assert_called_once()
        call_args = mock_repository.create_feedback.call_args[0][0]
        assert call_args["decision_type"] == DecisionType.REJECT

    @pytest.mark.asyncio
    async def test_record_reject_with_reason(
        self,
        feedback_manager,
        mock_repository: AsyncMock,
        tenant_id,
        session_id,
    ):
        """Test that reject stores reason code when provided."""
        item = create_review_item(tenant_id=tenant_id, session_id=session_id)
        mock_feedback = create_mock_feedback(
            tenant_id=tenant_id,
            review_item_id=item.id,
            session_id=session_id,
            decision_type=DecisionType.REJECT,
        )
        mock_feedback.correction_reason = "wrong_category"
        mock_repository.create_feedback.return_value = mock_feedback

        await feedback_manager.record_reject(
            item=item,
            session_id=session_id,
            time_spent_ms=4000,
            reason="wrong_category",
        )

        call_args = mock_repository.create_feedback.call_args[0][0]
        assert call_args["correction_reason"] == "wrong_category"

    @pytest.mark.asyncio
    async def test_record_reject_without_reason(
        self,
        feedback_manager,
        mock_repository: AsyncMock,
        tenant_id,
        session_id,
    ):
        """Test that reject works without a reason (optional parameter)."""
        item = create_review_item(tenant_id=tenant_id, session_id=session_id)
        mock_feedback = create_mock_feedback(
            tenant_id=tenant_id,
            review_item_id=item.id,
            session_id=session_id,
            decision_type=DecisionType.REJECT,
        )
        mock_repository.create_feedback.return_value = mock_feedback

        result = await feedback_manager.record_reject(
            item=item,
            session_id=session_id,
            time_spent_ms=2000,
        )

        assert result.decision_type == DecisionType.REJECT
        call_args = mock_repository.create_feedback.call_args[0][0]
        assert call_args.get("correction_reason") is None


# =============================================================================
# RECORD MODIFY TESTS
# =============================================================================


class TestRecordModify:
    """Tests for FeedbackManager.record_modify()."""

    @pytest.mark.asyncio
    async def test_record_modify_creates_feedback(
        self,
        feedback_manager,
        mock_repository: AsyncMock,
        tenant_id,
        session_id,
    ):
        """Test that recording modify creates feedback with correct decision type."""
        item = create_review_item(tenant_id=tenant_id, session_id=session_id)
        correction = UserCorrection(
            category="corrected/category",
            participants=["corrected@example.com"],
            tags=["corrected-tag"],
        )
        mock_feedback = create_mock_feedback(
            tenant_id=tenant_id,
            review_item_id=item.id,
            session_id=session_id,
            decision_type=DecisionType.MODIFY,
            user_correction=correction,
        )
        mock_repository.create_feedback.return_value = mock_feedback

        result = await feedback_manager.record_modify(
            item=item,
            session_id=session_id,
            correction=correction,
            time_spent_ms=8000,
        )

        assert result.decision_type == DecisionType.MODIFY
        mock_repository.create_feedback.assert_called_once()
        call_args = mock_repository.create_feedback.call_args[0][0]
        assert call_args["decision_type"] == DecisionType.MODIFY

    @pytest.mark.asyncio
    async def test_record_modify_stores_correction(
        self,
        feedback_manager,
        mock_repository: AsyncMock,
        tenant_id,
        session_id,
    ):
        """Test that modify stores UserCorrection with all changes."""
        item = create_review_item(tenant_id=tenant_id, session_id=session_id)
        correction = UserCorrection(
            category="projects/beta/planning",
            participants=["alice@example.com", "charlie@example.com"],
            tags=["high-priority", "q2"],
            reason="misclassified_project",
            notes="This belongs to the beta project, not alpha",
        )
        mock_feedback = create_mock_feedback(
            tenant_id=tenant_id,
            review_item_id=item.id,
            session_id=session_id,
            decision_type=DecisionType.MODIFY,
            user_correction=correction,
        )
        mock_repository.create_feedback.return_value = mock_feedback

        await feedback_manager.record_modify(
            item=item,
            session_id=session_id,
            correction=correction,
            time_spent_ms=10000,
        )

        call_args = mock_repository.create_feedback.call_args[0][0]
        stored_correction = call_args["user_correction"]
        assert stored_correction.category == "projects/beta/planning"
        assert stored_correction.participants == ["alice@example.com", "charlie@example.com"]
        assert stored_correction.tags == ["high-priority", "q2"]
        assert stored_correction.reason == "misclassified_project"
        assert stored_correction.notes == "This belongs to the beta project, not alpha"

    @pytest.mark.asyncio
    async def test_record_modify_partial_correction(
        self,
        feedback_manager,
        mock_repository: AsyncMock,
        tenant_id,
        session_id,
    ):
        """Test that modify handles corrections with only some fields changed."""
        item = create_review_item(tenant_id=tenant_id, session_id=session_id)
        # Only correcting the category, leaving participants and tags unchanged
        correction = UserCorrection(
            category="corrected/category/only",
            participants=None,  # Not changed
            tags=None,  # Not changed
        )
        mock_feedback = create_mock_feedback(
            tenant_id=tenant_id,
            review_item_id=item.id,
            session_id=session_id,
            decision_type=DecisionType.MODIFY,
            user_correction=correction,
        )
        mock_repository.create_feedback.return_value = mock_feedback

        await feedback_manager.record_modify(
            item=item,
            session_id=session_id,
            correction=correction,
            time_spent_ms=6000,
        )

        call_args = mock_repository.create_feedback.call_args[0][0]
        stored_correction = call_args["user_correction"]
        assert stored_correction.category == "corrected/category/only"
        assert stored_correction.participants is None
        assert stored_correction.tags is None


# =============================================================================
# RECORD SKIP TESTS
# =============================================================================


class TestRecordSkip:
    """Tests for FeedbackManager.record_skip()."""

    @pytest.mark.asyncio
    async def test_record_skip_creates_feedback(
        self,
        feedback_manager,
        mock_repository: AsyncMock,
        tenant_id,
        session_id,
    ):
        """Test that recording skip creates feedback with correct decision type."""
        item = create_review_item(tenant_id=tenant_id, session_id=session_id)
        mock_feedback = create_mock_feedback(
            tenant_id=tenant_id,
            review_item_id=item.id,
            session_id=session_id,
            decision_type=DecisionType.SKIP,
        )
        mock_repository.create_feedback.return_value = mock_feedback

        result = await feedback_manager.record_skip(
            item=item,
            session_id=session_id,
            time_spent_ms=1000,
        )

        assert result.decision_type == DecisionType.SKIP
        mock_repository.create_feedback.assert_called_once()
        call_args = mock_repository.create_feedback.call_args[0][0]
        assert call_args["decision_type"] == DecisionType.SKIP

    @pytest.mark.asyncio
    async def test_record_skip_with_reason(
        self,
        feedback_manager,
        mock_repository: AsyncMock,
        tenant_id,
        session_id,
    ):
        """Test that skip stores reason in correction_notes when provided."""
        item = create_review_item(tenant_id=tenant_id, session_id=session_id)
        mock_feedback = create_mock_feedback(
            tenant_id=tenant_id,
            review_item_id=item.id,
            session_id=session_id,
            decision_type=DecisionType.SKIP,
        )
        mock_feedback.correction_notes = "need_more_context"
        mock_repository.create_feedback.return_value = mock_feedback

        await feedback_manager.record_skip(
            item=item,
            session_id=session_id,
            time_spent_ms=1500,
            reason="need_more_context",
        )

        # Skip reason is stored in correction_notes, not correction_reason
        call_args = mock_repository.create_feedback.call_args[0][0]
        assert call_args["correction_notes"] == "need_more_context"


# =============================================================================
# UNDO TESTS
# =============================================================================


class TestUndoDecision:
    """Tests for FeedbackManager.undo_decision()."""

    @pytest.mark.asyncio
    async def test_undo_eligible_decision(
        self,
        feedback_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Test successfully undoing a recent decision."""
        feedback = create_mock_feedback(
            feedback_id=1,
            tenant_id=tenant_id,
            undo_eligible=True,
            undo_deadline=datetime.now(timezone.utc) + timedelta(minutes=5),
            is_undone=False,
        )
        mock_repository.get_feedback_by_id.return_value = feedback
        mock_repository.mark_feedback_undone.return_value = feedback

        result = await feedback_manager.undo_decision(feedback_id=1)

        mock_repository.mark_feedback_undone.assert_called_once_with(1)
        assert result is not None

    @pytest.mark.asyncio
    async def test_undo_expired_decision(
        self,
        feedback_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Test that undo fails if deadline has passed."""
        feedback = create_mock_feedback(
            feedback_id=1,
            tenant_id=tenant_id,
            undo_eligible=True,
            undo_deadline=datetime.now(timezone.utc) - timedelta(minutes=5),  # Expired
            is_undone=False,
        )
        mock_repository.get_feedback_by_id.return_value = feedback

        with pytest.raises(UndoNotEligibleError) as exc_info:
            await feedback_manager.undo_decision(feedback_id=1)

        assert exc_info.value.code == "REVIEW_005"
        assert "expired" in exc_info.value.details["reason"].lower()
        mock_repository.mark_feedback_undone.assert_not_called()

    @pytest.mark.asyncio
    async def test_undo_returns_true_on_success(
        self,
        feedback_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Test that undo returns True when mark_feedback_undone succeeds."""
        feedback = create_mock_feedback(
            feedback_id=1,
            tenant_id=tenant_id,
            undo_eligible=True,
            undo_deadline=datetime.now(timezone.utc) + timedelta(minutes=5),
            is_undone=False,
        )
        mock_repository.get_feedback_by_id.return_value = feedback
        mock_repository.mark_feedback_undone.return_value = True

        result = await feedback_manager.undo_decision(feedback_id=1)

        assert result is True
        mock_repository.mark_feedback_undone.assert_called_once_with(1)

    @pytest.mark.asyncio
    async def test_undo_with_no_deadline_raises_error(
        self,
        feedback_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Test that undo fails if undo_deadline is None."""
        feedback = create_mock_feedback(
            feedback_id=1,
            tenant_id=tenant_id,
            undo_eligible=True,
            undo_deadline=None,  # No deadline set
            is_undone=False,
        )
        mock_repository.get_feedback_by_id.return_value = feedback

        with pytest.raises(UndoNotEligibleError) as exc_info:
            await feedback_manager.undo_decision(feedback_id=1)

        assert exc_info.value.code == "REVIEW_005"
        mock_repository.mark_feedback_undone.assert_not_called()


class TestGetUndoEligible:
    """Tests for FeedbackManager.get_undo_eligible()."""

    @pytest.mark.asyncio
    async def test_get_undo_eligible_returns_recent(
        self,
        feedback_manager,
        mock_repository: AsyncMock,
        tenant_id,
        session_id,
    ):
        """Test that get_undo_eligible returns most recent undoable decisions."""
        now = datetime.now(timezone.utc)
        feedbacks = [
            create_mock_feedback(
                feedback_id=3,
                tenant_id=tenant_id,
                session_id=session_id,
                undo_eligible=True,
                undo_deadline=now + timedelta(minutes=5),
            ),
            create_mock_feedback(
                feedback_id=2,
                tenant_id=tenant_id,
                session_id=session_id,
                undo_eligible=True,
                undo_deadline=now + timedelta(minutes=4),
            ),
        ]
        mock_repository.get_undo_eligible_feedback.return_value = feedbacks

        result = await feedback_manager.get_undo_eligible(
            session_id=session_id,
            count=5,
        )

        assert len(result) == 2
        mock_repository.get_undo_eligible_feedback.assert_called_once()

    @pytest.mark.asyncio
    async def test_get_undo_eligible_respects_count(
        self,
        feedback_manager,
        mock_repository: AsyncMock,
        tenant_id,
        session_id,
    ):
        """Test that get_undo_eligible returns up to count items."""
        now = datetime.now(timezone.utc)
        # Return only 3 even though more exist
        feedbacks = [
            create_mock_feedback(
                feedback_id=i,
                tenant_id=tenant_id,
                session_id=session_id,
                undo_eligible=True,
                undo_deadline=now + timedelta(minutes=5),
            )
            for i in range(3)
        ]
        mock_repository.get_undo_eligible_feedback.return_value = feedbacks

        result = await feedback_manager.get_undo_eligible(
            session_id=session_id,
            count=3,
        )

        assert len(result) <= 3
        call_args = mock_repository.get_undo_eligible_feedback.call_args
        assert call_args[1]["count"] == 3 or call_args[0][1] == 3

    @pytest.mark.asyncio
    async def test_get_undo_eligible_empty_list(
        self,
        feedback_manager,
        mock_repository: AsyncMock,
        session_id,
    ):
        """Test get_undo_eligible returns empty list when no eligible items."""
        mock_repository.get_undo_eligible_feedback.return_value = []

        result = await feedback_manager.get_undo_eligible(
            session_id=session_id,
            count=5,
        )

        assert result == []


# =============================================================================
# LEARNING SIGNAL QUALITY TESTS
# =============================================================================


class TestLearningSignalQuality:
    """Tests for FeedbackManager.calculate_learning_signal_quality() method."""

    @pytest.fixture
    def quality_calculator(self, mock_repository):
        """Create FeedbackManager instance for quality calculation tests."""
        from penf_lib.review.feedback import FeedbackManager

        return FeedbackManager(repository=mock_repository)

    def test_accept_base_score(self, quality_calculator):
        """Accept decision should have base score of 0.3."""
        # Standard accept with normal timing and confidence
        score = quality_calculator.calculate_learning_signal_quality(
            decision=DecisionType.ACCEPT,
            confidence=Decimal("0.70"),
            time_spent_ms=5000,
        )

        # Base score for accept is 0.3, with normal modifiers
        assert Decimal("0.0") <= score <= Decimal("1.0")
        # Accept base is low since it provides less learning signal
        assert score < Decimal("0.5")

    def test_reject_base_score(self, quality_calculator):
        """Reject decision should have base score of 0.5."""
        score = quality_calculator.calculate_learning_signal_quality(
            decision=DecisionType.REJECT,
            confidence=Decimal("0.70"),
            time_spent_ms=5000,
        )

        assert Decimal("0.0") <= score <= Decimal("1.0")
        # Reject provides moderate learning signal
        assert score >= Decimal("0.3")

    def test_modify_base_score(self, quality_calculator):
        """Modify decision should have base score of 0.8."""
        score = quality_calculator.calculate_learning_signal_quality(
            decision=DecisionType.MODIFY,
            confidence=Decimal("0.70"),
            time_spent_ms=5000,
        )

        assert Decimal("0.0") <= score <= Decimal("1.0")
        # Modify provides high learning signal
        assert score >= Decimal("0.5")

    def test_skip_base_score(self, quality_calculator):
        """Skip decision should have base score of 0.1."""
        score = quality_calculator.calculate_learning_signal_quality(
            decision=DecisionType.SKIP,
            confidence=Decimal("0.70"),
            time_spent_ms=5000,
        )

        assert Decimal("0.0") <= score <= Decimal("1.0")
        # Skip provides minimal learning signal
        assert score < Decimal("0.3")

    def test_high_confidence_correction_bonus(self, quality_calculator):
        """Modify/reject with AI confidence > 0.8 should get 1.5x multiplier."""
        # High confidence correction (AI was confident but wrong)
        high_conf_score = quality_calculator.calculate_learning_signal_quality(
            decision=DecisionType.MODIFY,
            confidence=Decimal("0.90"),  # High confidence
            time_spent_ms=5000,
        )

        # Normal confidence correction
        normal_conf_score = quality_calculator.calculate_learning_signal_quality(
            decision=DecisionType.MODIFY,
            confidence=Decimal("0.60"),  # Normal confidence
            time_spent_ms=5000,
        )

        # High confidence corrections should score higher (more valuable learning signal)
        assert high_conf_score > normal_conf_score

    def test_low_confidence_accept_penalty(self, quality_calculator):
        """Accept with AI confidence < 0.5 should get 0.5x multiplier."""
        # Low confidence accept
        low_conf_score = quality_calculator.calculate_learning_signal_quality(
            decision=DecisionType.ACCEPT,
            confidence=Decimal("0.40"),  # Low confidence
            time_spent_ms=5000,
        )

        # High confidence accept
        high_conf_score = quality_calculator.calculate_learning_signal_quality(
            decision=DecisionType.ACCEPT,
            confidence=Decimal("0.90"),  # High confidence
            time_spent_ms=5000,
        )

        # Low confidence accepts may indicate less reliable signal
        assert low_conf_score <= high_conf_score

    def test_quick_decision_penalty(self, quality_calculator):
        """Decisions made in < 2 seconds should get 0.7x multiplier."""
        # Very quick decision (potentially not thoughtful)
        quick_score = quality_calculator.calculate_learning_signal_quality(
            decision=DecisionType.ACCEPT,
            confidence=Decimal("0.70"),
            time_spent_ms=1000,  # 1 second - very fast
        )

        # Normal timing decision
        normal_score = quality_calculator.calculate_learning_signal_quality(
            decision=DecisionType.ACCEPT,
            confidence=Decimal("0.70"),
            time_spent_ms=5000,  # 5 seconds - normal
        )

        # Quick decisions should have lower signal quality
        assert quick_score < normal_score

    def test_thoughtful_decision_bonus(self, quality_calculator):
        """Decisions taking > 10 seconds should get 1.2x multiplier."""
        # Thoughtful decision (took time to review)
        thoughtful_score = quality_calculator.calculate_learning_signal_quality(
            decision=DecisionType.MODIFY,
            confidence=Decimal("0.70"),
            time_spent_ms=15000,  # 15 seconds - thoughtful
        )

        # Quick decision
        quick_score = quality_calculator.calculate_learning_signal_quality(
            decision=DecisionType.MODIFY,
            confidence=Decimal("0.70"),
            time_spent_ms=3000,  # 3 seconds - quick
        )

        # Thoughtful decisions should have higher signal quality
        assert thoughtful_score > quick_score

    def test_score_clamped_to_range(self, quality_calculator):
        """Final score should always be in [0.0, 1.0] range."""
        # Test with extreme values that might push score out of range
        test_cases = [
            # High multipliers (modify + high confidence + long time)
            (DecisionType.MODIFY, Decimal("0.99"), 30000),
            # Low multipliers (skip + low confidence + quick)
            (DecisionType.SKIP, Decimal("0.10"), 500),
            # Edge cases
            (DecisionType.ACCEPT, Decimal("0.00"), 0),
            (DecisionType.REJECT, Decimal("1.00"), 100000),
        ]

        for decision_type, confidence, time_ms in test_cases:
            score = quality_calculator.calculate_learning_signal_quality(
                decision=decision_type,
                confidence=confidence,
                time_spent_ms=time_ms,
            )
            assert Decimal("0.0") <= score <= Decimal("1.0"), (
                f"Score {score} out of range for {decision_type}, "
                f"confidence={confidence}, time={time_ms}"
            )

    @pytest.mark.parametrize(
        "decision_type,expected_min,expected_max",
        [
            (DecisionType.ACCEPT, Decimal("0.0"), Decimal("0.5")),
            (DecisionType.REJECT, Decimal("0.2"), Decimal("0.8")),
            (DecisionType.MODIFY, Decimal("0.4"), Decimal("1.0")),
            (DecisionType.SKIP, Decimal("0.0"), Decimal("0.3")),
        ],
    )
    def test_decision_type_score_ranges(
        self,
        quality_calculator,
        decision_type: DecisionType,
        expected_min: Decimal,
        expected_max: Decimal,
    ):
        """Test that each decision type falls within expected score ranges."""
        # Test with neutral modifiers
        score = quality_calculator.calculate_learning_signal_quality(
            decision=decision_type,
            confidence=Decimal("0.70"),  # Neutral confidence
            time_spent_ms=5000,  # Neutral timing
        )

        assert expected_min <= score <= expected_max, (
            f"Score {score} for {decision_type} outside expected range "
            f"[{expected_min}, {expected_max}]"
        )


# =============================================================================
# CONFIDENCE FEEDBACK TESTS
# =============================================================================


class TestConfidenceFeedback:
    """Tests for confidence feedback field initialization."""

    @pytest.mark.asyncio
    async def test_accept_initializes_confidence_feedback_as_none(
        self,
        feedback_manager,
        mock_repository: AsyncMock,
        tenant_id,
        session_id,
    ):
        """Test that accept initializes confidence_feedback as None."""
        item = create_review_item(tenant_id=tenant_id, session_id=session_id)
        mock_feedback = create_mock_feedback(
            tenant_id=tenant_id,
            review_item_id=item.id,
            session_id=session_id,
            decision_type=DecisionType.ACCEPT,
        )
        mock_repository.create_feedback.return_value = mock_feedback

        await feedback_manager.record_accept(
            item=item,
            session_id=session_id,
            time_spent_ms=5000,
        )

        # Confidence feedback is initialized to None in _build_feedback_data
        call_args = mock_repository.create_feedback.call_args[0][0]
        assert call_args["confidence_feedback"] is None

    @pytest.mark.asyncio
    async def test_reject_initializes_confidence_feedback_as_none(
        self,
        feedback_manager,
        mock_repository: AsyncMock,
        tenant_id,
        session_id,
    ):
        """Test that reject initializes confidence_feedback as None."""
        item = create_review_item(
            tenant_id=tenant_id,
            session_id=session_id,
            ai_confidence=Decimal("0.95"),  # High confidence
        )
        mock_feedback = create_mock_feedback(
            tenant_id=tenant_id,
            review_item_id=item.id,
            session_id=session_id,
            decision_type=DecisionType.REJECT,
        )
        mock_repository.create_feedback.return_value = mock_feedback

        await feedback_manager.record_reject(
            item=item,
            session_id=session_id,
            time_spent_ms=4000,
        )

        # Confidence feedback is initialized to None in _build_feedback_data
        call_args = mock_repository.create_feedback.call_args[0][0]
        assert call_args["confidence_feedback"] is None


# =============================================================================
# BATCH DECISION TESTS
# =============================================================================


class TestBatchDecisions:
    """Tests for batch decision recording."""

    @pytest.mark.asyncio
    async def test_record_accept_captures_batch_from_item(
        self,
        feedback_manager,
        mock_repository: AsyncMock,
        tenant_id,
        session_id,
    ):
        """Test that batch context is captured from item.batch_id when recording accept."""
        batch_id = uuid4()
        # Create item with batch_id set - this indicates it's part of a batch
        item = create_review_item(tenant_id=tenant_id, session_id=session_id)
        item.batch_id = batch_id  # Set batch_id on the item
        mock_feedback = create_mock_feedback(
            tenant_id=tenant_id,
            review_item_id=item.id,
            session_id=session_id,
            decision_type=DecisionType.ACCEPT,
        )
        mock_feedback.was_batch_decision = True
        mock_feedback.batch_id = batch_id
        mock_repository.create_feedback.return_value = mock_feedback

        await feedback_manager.record_accept(
            item=item,
            session_id=session_id,
            time_spent_ms=500,
        )

        # Batch context is derived from item.batch_id in _build_feedback_data
        call_args = mock_repository.create_feedback.call_args[0][0]
        assert call_args["was_batch_decision"] is True
        assert call_args["batch_id"] == batch_id

    @pytest.mark.asyncio
    async def test_record_accept_non_batch_item(
        self,
        feedback_manager,
        mock_repository: AsyncMock,
        tenant_id,
        session_id,
    ):
        """Test that non-batch items have was_batch_decision=False."""
        # Create item without batch_id
        item = create_review_item(tenant_id=tenant_id, session_id=session_id)
        assert item.batch_id is None  # Verify no batch_id
        mock_feedback = create_mock_feedback(
            tenant_id=tenant_id,
            review_item_id=item.id,
            session_id=session_id,
            decision_type=DecisionType.ACCEPT,
        )
        mock_repository.create_feedback.return_value = mock_feedback

        await feedback_manager.record_accept(
            item=item,
            session_id=session_id,
            time_spent_ms=500,
        )

        call_args = mock_repository.create_feedback.call_args[0][0]
        assert call_args["was_batch_decision"] is False
        assert call_args["batch_id"] is None


# =============================================================================
# ERROR HANDLING TESTS
# =============================================================================


class TestFeedbackErrorHandling:
    """Tests for error handling in FeedbackManager."""

    @pytest.mark.asyncio
    async def test_undo_feedback_not_found(
        self,
        feedback_manager,
        mock_repository: AsyncMock,
    ):
        """Test that undo raises ItemNotFoundError when feedback not found."""
        from penf_lib.review.exceptions import ItemNotFoundError

        mock_repository.get_feedback_by_id.return_value = None

        with pytest.raises(ItemNotFoundError) as exc_info:
            await feedback_manager.undo_decision(feedback_id=999)

        assert exc_info.value.code == "REVIEW_004"
        assert exc_info.value.details["item_id"] == 999

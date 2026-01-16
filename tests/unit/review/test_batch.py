"""Unit tests for BatchManager.

Tests the batch operations module that groups similar review items and applies
bulk actions (accept/reject/skip) with undo support. Includes tests for finding
similar items, preview/execute/undo batch operations, and filter expressions.
"""

from datetime import datetime, timedelta, timezone
from decimal import Decimal
from unittest.mock import AsyncMock, MagicMock
from uuid import uuid4

import pytest

from penf_lib.review.batch import (
    BatchAction,
    BatchGroupType,
    BatchManager,
    BatchPreview,
    BatchResult,
    FilterExpression,
)
from penf_lib.review.exceptions import BatchOperationError, UndoNotEligibleError
from penf_lib.review.models import (
    AISuggestion,
    BatchOperationStatus,
    BatchType,
    ContentType,
    DecisionType,
    ReviewItemDTO,
    ReviewItemStatus,
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
    repo.get_pending_items_for_session = AsyncMock()
    repo.get_items_by_source_id = AsyncMock()
    repo.update_items_batch = AsyncMock(return_value=0)
    repo.create_batch_operation = AsyncMock()
    repo.get_batch_operation = AsyncMock()
    repo.update_batch_operation = AsyncMock()
    repo.get_items_by_batch_id = AsyncMock()
    return repo


@pytest.fixture
def mock_feedback_manager():
    """Create mock feedback manager for testing."""
    fm = AsyncMock()
    fm.record_accept = AsyncMock()
    fm.record_reject = AsyncMock()
    fm.record_skip = AsyncMock()
    fm.record_modify = AsyncMock()
    return fm


@pytest.fixture
def batch_manager(mock_repository, mock_feedback_manager):
    """Create BatchManager with mocked dependencies."""
    return BatchManager(
        repository=mock_repository,
        feedback_manager=mock_feedback_manager,
    )


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
    source_timestamp: datetime = None,
    batch_id=None,
) -> ReviewItemDTO:
    """Create a ReviewItemDTO for testing."""
    if tenant_id is None:
        tenant_id = uuid4()
    if participants is None:
        participants = ["john@example.com", "jane@example.com"]
    if tags is None:
        tags = ["meeting", "q1", "planning"]
    if source_timestamp is None:
        source_timestamp = datetime.now(timezone.utc) - timedelta(hours=2)

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
        source_timestamp=source_timestamp,
        batch_id=batch_id,
        created_at=now,
        updated_at=now,
    )


def create_mock_batch_operation(
    batch_id: int = 1,
    batch_uuid=None,
    tenant_id=None,
    session_id: int = 1,
    batch_type: BatchType = BatchType.THREAD,
    action_type: BatchAction = BatchAction.ACCEPT,
    item_count: int = 5,
    status: BatchOperationStatus = BatchOperationStatus.PENDING,
    undo_eligible: bool = True,
    undo_deadline: datetime = None,
):
    """Create a mock batch operation for testing."""
    if batch_uuid is None:
        batch_uuid = uuid4()
    if tenant_id is None:
        tenant_id = uuid4()
    if undo_deadline is None:
        undo_deadline = datetime.now(timezone.utc) + timedelta(minutes=5)

    now = datetime.now(timezone.utc)
    batch = MagicMock()
    batch.id = batch_id
    batch.batch_uuid = batch_uuid
    batch.tenant_id = tenant_id
    batch.session_id = session_id
    batch.batch_type = batch_type
    batch.group_criteria = {"source_id": 100}
    batch.action_type = action_type
    batch.action_details = {}
    batch.item_count = item_count
    batch.status = status
    batch.confirmed_at = None
    batch.applied_at = None
    batch.undone_at = None
    batch.undo_eligible = undo_eligible
    batch.undo_deadline = undo_deadline
    batch.created_at = now
    batch.updated_at = now
    return batch


# =============================================================================
# FIND SIMILAR ITEMS TESTS
# =============================================================================


class TestFindSimilarItems:
    """Tests for BatchManager.find_similar_items()."""

    @pytest.mark.asyncio
    async def test_find_by_thread(
        self,
        batch_manager,
        mock_repository: AsyncMock,
        tenant_id,
        session_id,
    ):
        """Should find items with same source_id (thread)."""
        base_item = create_review_item(
            item_id=1,
            tenant_id=tenant_id,
            session_id=session_id,
            source_id=100,
        )
        similar_items = [
            create_review_item(
                item_id=i,
                tenant_id=tenant_id,
                session_id=session_id,
                source_id=100,
            )
            for i in range(2, 6)
        ]
        mock_repository.get_pending_items_for_session.return_value = [base_item] + similar_items

        result = await batch_manager.find_similar_items(
            session_id=session_id,
            reference_item=base_item,
            group_by=BatchGroupType.THREAD,
        )

        # Excludes base_item, returns 4 similar items
        assert len(result) == 4
        assert all(item.source_id == 100 for item in result)

    @pytest.mark.asyncio
    async def test_find_by_sender(
        self,
        batch_manager,
        mock_repository: AsyncMock,
        tenant_id,
        session_id,
    ):
        """Should find items with same first participant (sender)."""
        sender = "john@example.com"
        base_item = create_review_item(
            item_id=1,
            tenant_id=tenant_id,
            session_id=session_id,
            participants=[sender, "other@example.com"],
        )
        similar_items = [
            create_review_item(
                item_id=i,
                tenant_id=tenant_id,
                session_id=session_id,
                source_id=100 + i,
                participants=[sender, f"other{i}@example.com"],
            )
            for i in range(2, 5)
        ]
        # Add item with different sender
        different_sender = create_review_item(
            item_id=10,
            tenant_id=tenant_id,
            session_id=session_id,
            source_id=200,
            participants=["different@example.com"],
        )
        all_items = [base_item] + similar_items + [different_sender]
        mock_repository.get_pending_items_for_session.return_value = all_items

        result = await batch_manager.find_similar_items(
            session_id=session_id,
            reference_item=base_item,
            group_by=BatchGroupType.SENDER,
        )

        # Excludes base_item, finds 3 matching items
        assert len(result) == 3
        assert all(item.ai_suggestion.participants[0] == sender for item in result)

    @pytest.mark.asyncio
    async def test_find_by_category(
        self,
        batch_manager,
        mock_repository: AsyncMock,
        tenant_id,
        session_id,
    ):
        """Should find items with same AI category."""
        category = "project/planning"
        base_item = create_review_item(
            item_id=1,
            tenant_id=tenant_id,
            session_id=session_id,
            category=category,
        )
        similar_items = [
            create_review_item(
                item_id=i,
                tenant_id=tenant_id,
                session_id=session_id,
                source_id=100 + i,
                category=category,
            )
            for i in range(2, 6)
        ]
        # Add item with different category
        different_category = create_review_item(
            item_id=10,
            tenant_id=tenant_id,
            session_id=session_id,
            category="finance/expenses",
        )
        all_items = [base_item] + similar_items + [different_category]
        mock_repository.get_pending_items_for_session.return_value = all_items

        result = await batch_manager.find_similar_items(
            session_id=session_id,
            reference_item=base_item,
            group_by=BatchGroupType.CATEGORY,
        )

        # Excludes base_item, finds 4 matching items
        assert len(result) == 4
        assert all(item.ai_suggestion.category == category for item in result)

    @pytest.mark.asyncio
    async def test_find_by_time_window(
        self,
        batch_manager,
        mock_repository: AsyncMock,
        tenant_id,
        session_id,
    ):
        """Should find items within same hour."""
        base_time = datetime.now(timezone.utc).replace(minute=30, second=0, microsecond=0)
        base_item = create_review_item(
            item_id=1,
            tenant_id=tenant_id,
            session_id=session_id,
            source_timestamp=base_time,
        )
        # Items within same hour (minutes 0-59 of the same hour)
        similar_items = [
            create_review_item(
                item_id=i,
                tenant_id=tenant_id,
                session_id=session_id,
                source_id=100 + i,
                source_timestamp=base_time.replace(minute=(i * 10) % 60),
            )
            for i in range(2, 5)
        ]
        # Item from different hour
        different_time = create_review_item(
            item_id=10,
            tenant_id=tenant_id,
            session_id=session_id,
            source_timestamp=base_time - timedelta(hours=2),
        )
        all_items = [base_item] + similar_items + [different_time]
        mock_repository.get_pending_items_for_session.return_value = all_items

        result = await batch_manager.find_similar_items(
            session_id=session_id,
            reference_item=base_item,
            group_by=BatchGroupType.TIME_WINDOW,
        )

        # Excludes base_item, finds items within same hour
        assert len(result) == 3
        for item in result:
            # Items should be in the same hour as base_item
            assert item.source_timestamp.hour == base_time.hour

    @pytest.mark.asyncio
    async def test_find_excludes_non_pending(
        self,
        batch_manager,
        mock_repository: AsyncMock,
        tenant_id,
        session_id,
    ):
        """Should only return pending items."""
        base_item = create_review_item(
            item_id=1,
            tenant_id=tenant_id,
            session_id=session_id,
            source_id=100,
            status=ReviewItemStatus.PENDING,
        )
        pending_items = [
            create_review_item(
                item_id=i,
                tenant_id=tenant_id,
                session_id=session_id,
                source_id=100,
                status=ReviewItemStatus.PENDING,
            )
            for i in range(2, 4)
        ]
        # Add accepted and rejected items
        accepted_item = create_review_item(
            item_id=10,
            tenant_id=tenant_id,
            session_id=session_id,
            source_id=100,
            status=ReviewItemStatus.ACCEPTED,
        )
        rejected_item = create_review_item(
            item_id=11,
            tenant_id=tenant_id,
            session_id=session_id,
            source_id=100,
            status=ReviewItemStatus.REJECTED,
        )
        all_items = [base_item] + pending_items + [accepted_item, rejected_item]
        mock_repository.get_pending_items_for_session.return_value = all_items

        result = await batch_manager.find_similar_items(
            session_id=session_id,
            reference_item=base_item,
            group_by=BatchGroupType.THREAD,
        )

        # Excludes base_item and non-pending items, returns 2 pending items
        assert len(result) == 2
        assert all(item.status == ReviewItemStatus.PENDING for item in result)

    @pytest.mark.asyncio
    async def test_find_with_filter(
        self,
        batch_manager,
        mock_repository: AsyncMock,
        tenant_id,
        session_id,
    ):
        """Should apply filter expression."""
        base_item = create_review_item(
            item_id=1,
            tenant_id=tenant_id,
            session_id=session_id,
            source_id=100,
            ai_confidence=Decimal("0.90"),
        )
        high_conf_items = [
            create_review_item(
                item_id=i,
                tenant_id=tenant_id,
                session_id=session_id,
                source_id=100,
                ai_confidence=Decimal("0.85"),
            )
            for i in range(2, 4)
        ]
        low_conf_items = [
            create_review_item(
                item_id=i,
                tenant_id=tenant_id,
                session_id=session_id,
                source_id=100,
                ai_confidence=Decimal("0.50"),
            )
            for i in range(10, 12)
        ]
        all_items = [base_item] + high_conf_items + low_conf_items
        mock_repository.get_pending_items_for_session.return_value = all_items

        result = await batch_manager.find_similar_items(
            session_id=session_id,
            reference_item=base_item,
            group_by=BatchGroupType.THREAD,
            filter_expr="confidence>0.8",
        )

        # Excludes base_item, filters to high confidence items only
        assert len(result) == 2
        assert all(item.ai_confidence > Decimal("0.8") for item in result)


# =============================================================================
# PREVIEW BATCH TESTS
# =============================================================================


class TestPreviewBatch:
    """Tests for BatchManager.preview_batch()."""

    @pytest.mark.asyncio
    async def test_preview_shows_items(
        self,
        batch_manager,
        tenant_id,
        session_id,
    ):
        """Should list all items in preview."""
        items = [
            create_review_item(
                item_id=i,
                tenant_id=tenant_id,
                session_id=session_id,
                source_id=100,
            )
            for i in range(1, 6)
        ]

        preview = await batch_manager.preview_batch(
            items=items,
            action=BatchAction.ACCEPT,
        )

        assert preview.item_count == 5
        assert len(preview.items) == 5

    @pytest.mark.asyncio
    async def test_preview_shows_action(
        self,
        batch_manager,
        tenant_id,
        session_id,
    ):
        """Should indicate action to be taken."""
        items = [
            create_review_item(
                item_id=i,
                tenant_id=tenant_id,
                session_id=session_id,
            )
            for i in range(1, 4)
        ]

        preview = await batch_manager.preview_batch(
            items=items,
            action=BatchAction.REJECT,
        )

        assert preview.action == BatchAction.REJECT

    @pytest.mark.asyncio
    async def test_preview_estimates_time_saved(
        self,
        batch_manager,
        tenant_id,
        session_id,
    ):
        """Should calculate estimated time savings."""
        items = [
            create_review_item(
                item_id=i,
                tenant_id=tenant_id,
                session_id=session_id,
            )
            for i in range(1, 11)
        ]

        preview = await batch_manager.preview_batch(
            items=items,
            action=BatchAction.ACCEPT,
        )

        # 10 items at AVG_REVIEW_TIME_SECONDS (15 sec) = 150 seconds
        assert preview.estimated_time_saved_seconds > 0
        assert preview.estimated_time_saved_seconds == 10 * BatchManager.AVG_REVIEW_TIME_SECONDS


# =============================================================================
# EXECUTE BATCH TESTS
# =============================================================================


class TestExecuteBatch:
    """Tests for BatchManager.execute_batch()."""

    @pytest.mark.asyncio
    async def test_execute_accept(
        self,
        batch_manager,
        mock_repository: AsyncMock,
        mock_feedback_manager: AsyncMock,
        tenant_id,
        session_id,
    ):
        """Should mark all items as accepted."""
        items = [
            create_review_item(
                item_id=i,
                tenant_id=tenant_id,
                session_id=session_id,
            )
            for i in range(1, 4)
        ]
        mock_repository.update_items_batch.return_value = 3

        result = await batch_manager.execute_batch(
            session_id=session_id,
            items=items,
            action=BatchAction.ACCEPT,
        )

        assert result.success is True
        assert result.items_affected == 3
        assert mock_feedback_manager.record_accept.call_count == 3

    @pytest.mark.asyncio
    async def test_execute_reject(
        self,
        batch_manager,
        mock_repository: AsyncMock,
        mock_feedback_manager: AsyncMock,
        tenant_id,
        session_id,
    ):
        """Should mark all items as rejected."""
        items = [
            create_review_item(
                item_id=i,
                tenant_id=tenant_id,
                session_id=session_id,
            )
            for i in range(1, 4)
        ]
        mock_repository.update_items_batch.return_value = 3

        result = await batch_manager.execute_batch(
            session_id=session_id,
            items=items,
            action=BatchAction.REJECT,
        )

        assert result.success is True
        assert result.items_affected == 3
        assert mock_feedback_manager.record_reject.call_count == 3

    @pytest.mark.asyncio
    async def test_execute_skip(
        self,
        batch_manager,
        mock_repository: AsyncMock,
        mock_feedback_manager: AsyncMock,
        tenant_id,
        session_id,
    ):
        """Should mark all items as skipped."""
        items = [
            create_review_item(
                item_id=i,
                tenant_id=tenant_id,
                session_id=session_id,
            )
            for i in range(1, 4)
        ]
        mock_repository.update_items_batch.return_value = 3

        result = await batch_manager.execute_batch(
            session_id=session_id,
            items=items,
            action=BatchAction.SKIP,
        )

        assert result.success is True
        assert result.items_affected == 3
        assert mock_feedback_manager.record_skip.call_count == 3

    @pytest.mark.asyncio
    async def test_execute_generates_batch_id(
        self,
        batch_manager,
        mock_repository: AsyncMock,
        tenant_id,
        session_id,
    ):
        """Should create unique batch_id for all items."""
        items = [
            create_review_item(
                item_id=i,
                tenant_id=tenant_id,
                session_id=session_id,
            )
            for i in range(1, 4)
        ]
        mock_repository.update_items_batch.return_value = 3

        result = await batch_manager.execute_batch(
            session_id=session_id,
            items=items,
            action=BatchAction.ACCEPT,
        )

        assert result.batch_id is not None
        mock_repository.create_batch_operation.assert_called_once()

    @pytest.mark.asyncio
    async def test_execute_sets_undo_deadline(
        self,
        batch_manager,
        mock_repository: AsyncMock,
        tenant_id,
        session_id,
    ):
        """Should set 5-minute undo deadline."""
        items = [
            create_review_item(
                item_id=i,
                tenant_id=tenant_id,
                session_id=session_id,
            )
            for i in range(1, 3)
        ]
        mock_repository.update_items_batch.return_value = 2
        now = datetime.now(timezone.utc)

        result = await batch_manager.execute_batch(
            session_id=session_id,
            items=items,
            action=BatchAction.ACCEPT,
        )

        assert result.undo_eligible is True
        assert result.undo_deadline is not None
        # Deadline should be approximately 5 minutes in the future
        time_diff = (result.undo_deadline - now).total_seconds()
        assert 290 <= time_diff <= 310  # ~5 minutes with tolerance

    @pytest.mark.asyncio
    async def test_execute_records_feedback(
        self,
        batch_manager,
        mock_repository: AsyncMock,
        mock_feedback_manager: AsyncMock,
        tenant_id,
        session_id,
    ):
        """Should record feedback for each item."""
        items = [
            create_review_item(
                item_id=i,
                tenant_id=tenant_id,
                session_id=session_id,
            )
            for i in range(1, 4)
        ]
        mock_repository.update_items_batch.return_value = 3

        await batch_manager.execute_batch(
            session_id=session_id,
            items=items,
            action=BatchAction.ACCEPT,
        )

        # Verify feedback recorded for each item
        assert mock_feedback_manager.record_accept.call_count == 3


# =============================================================================
# UNDO BATCH TESTS
# =============================================================================


class TestUndoBatch:
    """Tests for BatchManager.undo_batch()."""

    @pytest.mark.asyncio
    async def test_undo_restores_items(
        self,
        batch_manager,
        mock_repository: AsyncMock,
        tenant_id,
        session_id,
    ):
        """Should restore all items to pending."""
        batch_uuid = uuid4()
        mock_batch = create_mock_batch_operation(
            batch_uuid=batch_uuid,
            status=BatchOperationStatus.APPLIED,
            undo_eligible=True,
            undo_deadline=datetime.now(timezone.utc) + timedelta(minutes=5),
        )
        mock_repository.get_batch_operation.return_value = mock_batch

        items = [
            create_review_item(
                item_id=i,
                tenant_id=tenant_id,
                session_id=session_id,
                status=ReviewItemStatus.ACCEPTED,
                batch_id=batch_uuid,
            )
            for i in range(1, 4)
        ]
        mock_repository.get_items_by_batch_id.return_value = items
        mock_repository.update_items_batch.return_value = 3

        result = await batch_manager.undo_batch(batch_id=batch_uuid)

        assert result == 3
        mock_repository.update_items_batch.assert_called_once()

    @pytest.mark.asyncio
    async def test_undo_within_deadline(
        self,
        batch_manager,
        mock_repository: AsyncMock,
        tenant_id,
        session_id,
    ):
        """Should succeed before deadline."""
        batch_uuid = uuid4()
        mock_batch = create_mock_batch_operation(
            batch_uuid=batch_uuid,
            status=BatchOperationStatus.APPLIED,
            undo_eligible=True,
            undo_deadline=datetime.now(timezone.utc) + timedelta(minutes=3),
        )
        mock_repository.get_batch_operation.return_value = mock_batch
        # Need at least one item for the undo to actually process
        items = [
            create_review_item(
                item_id=1,
                tenant_id=tenant_id,
                session_id=session_id,
                batch_id=batch_uuid,
            )
        ]
        mock_repository.get_items_by_batch_id.return_value = items
        mock_repository.update_items_batch.return_value = 1

        result = await batch_manager.undo_batch(batch_id=batch_uuid)

        assert result == 1
        mock_repository.update_batch_operation.assert_called_once()

    @pytest.mark.asyncio
    async def test_undo_after_deadline_fails(
        self,
        batch_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should fail after 5 minutes."""
        batch_uuid = uuid4()
        mock_batch = create_mock_batch_operation(
            batch_uuid=batch_uuid,
            status=BatchOperationStatus.APPLIED,
            undo_eligible=True,
            undo_deadline=datetime.now(timezone.utc) - timedelta(minutes=1),  # Expired
        )
        mock_repository.get_batch_operation.return_value = mock_batch

        with pytest.raises(UndoNotEligibleError) as exc_info:
            await batch_manager.undo_batch(batch_id=batch_uuid)

        assert "expired" in exc_info.value.details["reason"].lower()

    @pytest.mark.asyncio
    async def test_undo_not_eligible_fails(
        self,
        batch_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should fail if batch is not undo eligible."""
        batch_uuid = uuid4()
        mock_batch = create_mock_batch_operation(
            batch_uuid=batch_uuid,
            status=BatchOperationStatus.APPLIED,
            undo_eligible=False,
        )
        mock_repository.get_batch_operation.return_value = mock_batch

        with pytest.raises(UndoNotEligibleError):
            await batch_manager.undo_batch(batch_id=batch_uuid)


# =============================================================================
# FILTER EXPRESSION TESTS
# =============================================================================


class TestFilterExpression:
    """Tests for filter expression parsing and application."""

    def test_parse_confidence_greater(self):
        """confidence>0.8 should parse correctly."""
        expr = FilterExpression.parse("confidence>0.8")
        assert expr.field == "confidence"
        assert expr.operator == ">"
        assert expr.value == "0.8"

    def test_parse_confidence_less(self):
        """confidence<0.6 should parse correctly."""
        expr = FilterExpression.parse("confidence<0.6")
        assert expr.field == "confidence"
        assert expr.operator == "<"
        assert expr.value == "0.6"

    def test_parse_type_equals(self):
        """type=email should parse correctly."""
        expr = FilterExpression.parse("type=email")
        assert expr.field == "type"
        assert expr.operator == "="
        assert expr.value == "email"

    def test_parse_greater_equals(self):
        """confidence>=0.8 should parse correctly."""
        expr = FilterExpression.parse("confidence>=0.8")
        assert expr.field == "confidence"
        assert expr.operator == ">="
        assert expr.value == "0.8"

    def test_parse_less_equals(self):
        """confidence<=0.6 should parse correctly."""
        expr = FilterExpression.parse("confidence<=0.6")
        assert expr.field == "confidence"
        assert expr.operator == "<="
        assert expr.value == "0.6"

    def test_parse_invalid_raises_error(self):
        """Invalid filter should raise ValueError."""
        with pytest.raises(ValueError):
            FilterExpression.parse("invalid_filter_syntax")

    def test_matches_confidence_greater(self):
        """Should match items with confidence > threshold."""
        expr = FilterExpression.parse("confidence>0.8")
        item_high = create_review_item(item_id=1, ai_confidence=Decimal("0.90"))
        item_low = create_review_item(item_id=2, ai_confidence=Decimal("0.70"))

        assert expr.matches(item_high) is True
        assert expr.matches(item_low) is False

    def test_matches_type_equals(self):
        """Should match items with matching content type."""
        expr = FilterExpression.parse("type=email")
        item_email = create_review_item(item_id=1, content_type=ContentType.EMAIL)
        item_meeting = create_review_item(item_id=2, content_type=ContentType.MEETING)

        assert expr.matches(item_email) is True
        assert expr.matches(item_meeting) is False

    def test_matches_sender(self):
        """Should match items by sender (first participant)."""
        expr = FilterExpression.parse("sender=john@example.com")
        item_john = create_review_item(
            item_id=1,
            participants=["john@example.com", "other@example.com"],
        )
        item_jane = create_review_item(
            item_id=2,
            participants=["jane@example.com"],
        )

        assert expr.matches(item_john) is True
        assert expr.matches(item_jane) is False

    def test_matches_category(self):
        """Should match items by category."""
        expr = FilterExpression.parse("category=project/planning")
        item_match = create_review_item(item_id=1, category="project/planning")
        item_other = create_review_item(item_id=2, category="finance/expenses")

        assert expr.matches(item_match) is True
        assert expr.matches(item_other) is False


# =============================================================================
# EDGE CASES TESTS
# =============================================================================


class TestBatchEdgeCases:
    """Tests for edge cases in batch operations."""

    @pytest.mark.asyncio
    async def test_empty_batch(
        self,
        batch_manager,
        mock_repository: AsyncMock,
        session_id,
    ):
        """Should handle no items gracefully."""
        with pytest.raises(BatchOperationError) as exc_info:
            await batch_manager.execute_batch(
                session_id=session_id,
                items=[],
                action=BatchAction.ACCEPT,
            )

        assert "no items" in exc_info.value.details["reason"].lower()

    @pytest.mark.asyncio
    async def test_single_item_batch(
        self,
        batch_manager,
        mock_repository: AsyncMock,
        mock_feedback_manager: AsyncMock,
        tenant_id,
        session_id,
    ):
        """Should work with just one item."""
        item = create_review_item(
            item_id=1,
            tenant_id=tenant_id,
            session_id=session_id,
        )
        mock_repository.update_items_batch.return_value = 1

        result = await batch_manager.execute_batch(
            session_id=session_id,
            items=[item],
            action=BatchAction.ACCEPT,
        )

        assert result.items_affected == 1
        mock_feedback_manager.record_accept.assert_called_once()

    @pytest.mark.asyncio
    async def test_large_batch(
        self,
        batch_manager,
        mock_repository: AsyncMock,
        mock_feedback_manager: AsyncMock,
        tenant_id,
        session_id,
    ):
        """Should handle many items efficiently."""
        item_count = 100
        items = [
            create_review_item(
                item_id=i,
                tenant_id=tenant_id,
                session_id=session_id,
            )
            for i in range(1, item_count + 1)
        ]
        mock_repository.update_items_batch.return_value = item_count

        result = await batch_manager.execute_batch(
            session_id=session_id,
            items=items,
            action=BatchAction.ACCEPT,
        )

        assert result.items_affected == item_count
        assert mock_feedback_manager.record_accept.call_count == item_count

    @pytest.mark.asyncio
    async def test_batch_not_found(
        self,
        batch_manager,
        mock_repository: AsyncMock,
    ):
        """Should raise error when batch not found for undo."""
        batch_uuid = uuid4()
        mock_repository.get_batch_operation.return_value = None

        with pytest.raises(BatchOperationError) as exc_info:
            await batch_manager.undo_batch(batch_id=batch_uuid)

        assert "not found" in exc_info.value.details["reason"].lower()

    @pytest.mark.asyncio
    async def test_find_similar_with_no_matches(
        self,
        batch_manager,
        mock_repository: AsyncMock,
        tenant_id,
        session_id,
    ):
        """Should return empty list when no similar items exist."""
        base_item = create_review_item(
            item_id=1,
            tenant_id=tenant_id,
            session_id=session_id,
            source_id=100,
        )
        # Only the base item in session, no similar items
        mock_repository.get_pending_items_for_session.return_value = [base_item]

        result = await batch_manager.find_similar_items(
            session_id=session_id,
            reference_item=base_item,
            group_by=BatchGroupType.THREAD,
        )

        # Base item is excluded, no other matches
        assert len(result) == 0

    @pytest.mark.asyncio
    async def test_apply_category_requires_override(
        self,
        batch_manager,
        mock_repository: AsyncMock,
        tenant_id,
        session_id,
    ):
        """Should require category_override for apply_category action."""
        items = [
            create_review_item(
                item_id=i,
                tenant_id=tenant_id,
                session_id=session_id,
            )
            for i in range(1, 3)
        ]

        with pytest.raises(BatchOperationError) as exc_info:
            await batch_manager.execute_batch(
                session_id=session_id,
                items=items,
                action=BatchAction.APPLY_CATEGORY,
            )

        assert "category_override required" in exc_info.value.details["reason"]


# =============================================================================
# BATCH PREVIEW DATACLASS TESTS
# =============================================================================


class TestBatchPreviewDataclass:
    """Tests for BatchPreview dataclass structure."""

    @pytest.mark.asyncio
    async def test_preview_has_required_fields(
        self,
        batch_manager,
        tenant_id,
        session_id,
    ):
        """BatchPreview should have all required fields."""
        items = [
            create_review_item(
                item_id=i,
                tenant_id=tenant_id,
                session_id=session_id,
            )
            for i in range(1, 4)
        ]

        preview = await batch_manager.preview_batch(
            items=items,
            action=BatchAction.ACCEPT,
        )

        # Verify all required fields exist
        assert hasattr(preview, "item_count")
        assert hasattr(preview, "items")
        assert hasattr(preview, "action")
        assert hasattr(preview, "estimated_time_saved_seconds")
        assert hasattr(preview, "group_type")
        assert hasattr(preview, "group_value")

    @pytest.mark.asyncio
    async def test_preview_item_summaries(
        self,
        batch_manager,
        tenant_id,
        session_id,
    ):
        """Preview should include item summaries."""
        items = [
            create_review_item(
                item_id=1,
                tenant_id=tenant_id,
                session_id=session_id,
                content_preview="Important email about project",
            ),
            create_review_item(
                item_id=2,
                tenant_id=tenant_id,
                session_id=session_id,
                content_preview="Follow-up meeting notes",
            ),
        ]

        preview = await batch_manager.preview_batch(
            items=items,
            action=BatchAction.ACCEPT,
        )

        assert len(preview.items) == 2
        # Each item should have content preview
        for item in preview.items:
            assert hasattr(item, "content_preview")


# =============================================================================
# BATCH RESULT DATACLASS TESTS
# =============================================================================


class TestBatchResultDataclass:
    """Tests for BatchResult dataclass structure."""

    @pytest.mark.asyncio
    async def test_result_has_required_fields(
        self,
        batch_manager,
        mock_repository: AsyncMock,
        mock_feedback_manager: AsyncMock,
        tenant_id,
        session_id,
    ):
        """BatchResult should have all required fields."""
        items = [
            create_review_item(
                item_id=i,
                tenant_id=tenant_id,
                session_id=session_id,
            )
            for i in range(1, 3)
        ]
        mock_repository.update_items_batch.return_value = 2

        result = await batch_manager.execute_batch(
            session_id=session_id,
            items=items,
            action=BatchAction.ACCEPT,
        )

        # Verify all required fields exist
        assert hasattr(result, "batch_id")
        assert hasattr(result, "items_affected")
        assert hasattr(result, "success")
        assert hasattr(result, "undo_eligible")
        assert hasattr(result, "undo_deadline")
        assert hasattr(result, "action")

    @pytest.mark.asyncio
    async def test_result_items_affected_matches(
        self,
        batch_manager,
        mock_repository: AsyncMock,
        tenant_id,
        session_id,
    ):
        """Result items_affected should match processed items."""
        items = [
            create_review_item(
                item_id=i,
                tenant_id=tenant_id,
                session_id=session_id,
            )
            for i in range(1, 6)
        ]
        mock_repository.update_items_batch.return_value = 5

        result = await batch_manager.execute_batch(
            session_id=session_id,
            items=items,
            action=BatchAction.ACCEPT,
        )

        assert result.items_affected == 5

    def test_batch_result_is_undoable(self):
        """BatchResult.is_undoable should check deadline."""
        future_deadline = datetime.now(timezone.utc) + timedelta(minutes=5)
        past_deadline = datetime.now(timezone.utc) - timedelta(minutes=1)

        result_undoable = BatchResult(
            batch_id=uuid4(),
            items_affected=3,
            action=BatchAction.ACCEPT,
            success=True,
            undo_eligible=True,
            undo_deadline=future_deadline,
        )
        result_expired = BatchResult(
            batch_id=uuid4(),
            items_affected=3,
            action=BatchAction.ACCEPT,
            success=True,
            undo_eligible=True,
            undo_deadline=past_deadline,
        )
        result_not_eligible = BatchResult(
            batch_id=uuid4(),
            items_affected=3,
            action=BatchAction.ACCEPT,
            success=True,
            undo_eligible=False,
            undo_deadline=future_deadline,
        )

        assert result_undoable.is_undoable is True
        assert result_expired.is_undoable is False
        assert result_not_eligible.is_undoable is False

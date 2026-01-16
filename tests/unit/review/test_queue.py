"""Unit tests for QueueManager.

Tests the queue population and prioritization logic including different
priority modes, filter criteria, and edge cases. Uses mocked repository
for isolation.
"""

from datetime import datetime, timedelta, timezone
from decimal import Decimal
from typing import List, Optional
from unittest.mock import AsyncMock
from uuid import uuid4

import pytest

from penf_lib.review.models import (
    AISuggestion,
    ContentType,
    PriorityMode,
    ReviewItemDTO,
    ReviewItemStatus,
)
from penf_lib.review.queue import QueueManager


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
    repo.get_pending_items_for_queue = AsyncMock(return_value=[])
    repo.get_queue_items = AsyncMock(return_value=[])
    repo.update_item_queue_position = AsyncMock()
    return repo


@pytest.fixture
def queue_manager(mock_repository):
    """Create QueueManager with mocked repository."""
    return QueueManager(repository=mock_repository)


def create_mock_item(
    item_id: int = 1,
    ai_confidence: float = 0.5,
    business_importance: int = 5,
    source_timestamp: Optional[datetime] = None,
    content_type: ContentType = ContentType.EMAIL,
    status: ReviewItemStatus = ReviewItemStatus.PENDING,
    category: str = "project/test",
    tags: Optional[List[str]] = None,
) -> ReviewItemDTO:
    """Create a mock review item for testing."""
    if source_timestamp is None:
        source_timestamp = datetime.now(timezone.utc)
    if tags is None:
        tags = []

    return ReviewItemDTO(
        id=item_id,
        tenant_id=uuid4(),
        session_id=None,
        source_id=item_id,
        processing_result_id=uuid4(),
        queue_position=None,
        status=status,
        content_type=content_type,
        content_preview=f"Test content for item {item_id}",
        ai_suggestion=AISuggestion(
            category=category,
            participants=["test@example.com"],
            tags=tags,
        ),
        ai_confidence=Decimal(str(ai_confidence)),
        ai_model="test-model",
        business_importance=business_importance,
        source_timestamp=source_timestamp,
        created_at=datetime.now(timezone.utc),
        updated_at=datetime.now(timezone.utc),
    )


class TestPopulateQueue:
    """Tests for QueueManager.populate_queue()."""

    @pytest.mark.asyncio
    async def test_populate_queue_confidence_priority(
        self,
        queue_manager: QueueManager,
        mock_repository: AsyncMock,
        tenant_id,
        session_id,
    ):
        """Test queue population with confidence priority mode."""
        # Create items with different confidence scores
        items = [
            create_mock_item(item_id=1, ai_confidence=0.9),  # High conf - lowest priority
            create_mock_item(item_id=2, ai_confidence=0.3),  # Low conf - highest priority
            create_mock_item(item_id=3, ai_confidence=0.6),  # Medium conf
        ]
        mock_repository.get_pending_items_for_queue.return_value = items

        result = await queue_manager.populate_queue(
            tenant_id=tenant_id,
            session_id=session_id,
            priority_mode=PriorityMode.CONFIDENCE,
        )

        # Lowest confidence should be first
        assert len(result) == 3
        assert result[0].id == 2  # 0.3 confidence
        assert result[1].id == 3  # 0.6 confidence
        assert result[2].id == 1  # 0.9 confidence

        # Check positions are assigned
        assert result[0].queue_position == 1
        assert result[1].queue_position == 2
        assert result[2].queue_position == 3

    @pytest.mark.asyncio
    async def test_populate_queue_importance_priority(
        self,
        queue_manager: QueueManager,
        mock_repository: AsyncMock,
        tenant_id,
        session_id,
    ):
        """Test queue population with importance priority mode."""
        items = [
            create_mock_item(item_id=1, business_importance=3),  # Low
            create_mock_item(item_id=2, business_importance=9),  # High
            create_mock_item(item_id=3, business_importance=5),  # Medium
        ]
        mock_repository.get_pending_items_for_queue.return_value = items

        result = await queue_manager.populate_queue(
            tenant_id=tenant_id,
            session_id=session_id,
            priority_mode=PriorityMode.IMPORTANCE,
        )

        # Highest importance should be first
        assert len(result) == 3
        assert result[0].id == 2  # importance 9
        assert result[1].id == 3  # importance 5
        assert result[2].id == 1  # importance 3

    @pytest.mark.asyncio
    async def test_populate_queue_recency_priority(
        self,
        queue_manager: QueueManager,
        mock_repository: AsyncMock,
        tenant_id,
        session_id,
    ):
        """Test queue population with recency priority mode."""
        now = datetime.now(timezone.utc)
        items = [
            create_mock_item(item_id=1, source_timestamp=now - timedelta(days=10)),
            create_mock_item(item_id=2, source_timestamp=now - timedelta(hours=1)),  # Most recent
            create_mock_item(item_id=3, source_timestamp=now - timedelta(days=5)),
        ]
        mock_repository.get_pending_items_for_queue.return_value = items

        result = await queue_manager.populate_queue(
            tenant_id=tenant_id,
            session_id=session_id,
            priority_mode=PriorityMode.RECENCY,
        )

        # Most recent should be first
        assert len(result) == 3
        assert result[0].id == 2  # 1 hour ago
        assert result[1].id == 3  # 5 days ago
        assert result[2].id == 1  # 10 days ago

    @pytest.mark.asyncio
    async def test_populate_queue_mixed_priority(
        self,
        queue_manager: QueueManager,
        mock_repository: AsyncMock,
        tenant_id,
        session_id,
    ):
        """Test queue population with mixed priority mode."""
        now = datetime.now(timezone.utc)
        items = [
            # High confidence, low importance, old - should score low
            create_mock_item(
                item_id=1,
                ai_confidence=0.9,
                business_importance=2,
                source_timestamp=now - timedelta(days=20),
            ),
            # Low confidence, high importance, recent - should score high
            create_mock_item(
                item_id=2,
                ai_confidence=0.2,
                business_importance=8,
                source_timestamp=now - timedelta(hours=1),
            ),
            # Medium across all
            create_mock_item(
                item_id=3,
                ai_confidence=0.5,
                business_importance=5,
                source_timestamp=now - timedelta(days=5),
            ),
        ]
        mock_repository.get_pending_items_for_queue.return_value = items

        result = await queue_manager.populate_queue(
            tenant_id=tenant_id,
            session_id=session_id,
            priority_mode=PriorityMode.MIXED,
        )

        # Item 2 should score highest (low conf + high importance + recent)
        assert len(result) == 3
        assert result[0].id == 2

    @pytest.mark.asyncio
    async def test_populate_queue_with_filter_criteria(
        self,
        queue_manager: QueueManager,
        mock_repository: AsyncMock,
        tenant_id,
        session_id,
    ):
        """Test queue population with filter criteria."""
        items = [
            create_mock_item(item_id=1, content_type=ContentType.EMAIL),
            create_mock_item(item_id=2, content_type=ContentType.MEETING),
            create_mock_item(item_id=3, content_type=ContentType.EMAIL),
        ]
        mock_repository.get_pending_items_for_queue.return_value = items

        result = await queue_manager.populate_queue(
            tenant_id=tenant_id,
            session_id=session_id,
            filter_criteria={"content_type": "email"},
        )

        # Only emails should be included
        assert len(result) == 2
        assert all(item.content_type == ContentType.EMAIL for item in result)

    @pytest.mark.asyncio
    async def test_populate_queue_empty_result(
        self,
        queue_manager: QueueManager,
        mock_repository: AsyncMock,
        tenant_id,
        session_id,
    ):
        """Test queue population with no items."""
        mock_repository.get_pending_items_for_queue.return_value = []

        result = await queue_manager.populate_queue(
            tenant_id=tenant_id,
            session_id=session_id,
        )

        assert result == []

    @pytest.mark.asyncio
    async def test_populate_queue_filter_by_confidence_range(
        self,
        queue_manager: QueueManager,
        mock_repository: AsyncMock,
        tenant_id,
        session_id,
    ):
        """Test filtering by confidence range."""
        items = [
            create_mock_item(item_id=1, ai_confidence=0.2),
            create_mock_item(item_id=2, ai_confidence=0.5),
            create_mock_item(item_id=3, ai_confidence=0.8),
        ]
        mock_repository.get_pending_items_for_queue.return_value = items

        result = await queue_manager.populate_queue(
            tenant_id=tenant_id,
            session_id=session_id,
            filter_criteria={
                "min_confidence": 0.3,
                "max_confidence": 0.7,
            },
        )

        # Only item 2 should match
        assert len(result) == 1
        assert result[0].id == 2

    @pytest.mark.asyncio
    async def test_populate_queue_filter_by_tags(
        self,
        queue_manager: QueueManager,
        mock_repository: AsyncMock,
        tenant_id,
        session_id,
    ):
        """Test filtering by tags (any match)."""
        items = [
            create_mock_item(item_id=1, tags=["urgent", "planning"]),
            create_mock_item(item_id=2, tags=["routine"]),
            create_mock_item(item_id=3, tags=["urgent", "budget"]),
        ]
        mock_repository.get_pending_items_for_queue.return_value = items

        result = await queue_manager.populate_queue(
            tenant_id=tenant_id,
            session_id=session_id,
            filter_criteria={"tags": ["urgent"]},
        )

        # Items 1 and 3 have "urgent" tag
        assert len(result) == 2
        assert {r.id for r in result} == {1, 3}


class TestGetNextItem:
    """Tests for QueueManager.get_next_item()."""

    @pytest.mark.asyncio
    async def test_get_next_item_returns_pending(
        self,
        queue_manager: QueueManager,
        mock_repository: AsyncMock,
        session_id,
    ):
        """Test get_next_item returns next pending item."""
        item = create_mock_item(item_id=1, status=ReviewItemStatus.PENDING)
        mock_repository.get_queue_items.return_value = [item]

        result = await queue_manager.get_next_item(session_id=session_id)

        assert result is not None
        assert result.id == 1
        mock_repository.get_queue_items.assert_called_once_with(
            session_id=session_id,
            status=["pending"],
            limit=1,
            offset=0,
        )

    @pytest.mark.asyncio
    async def test_get_next_item_skips_reviewed(
        self,
        queue_manager: QueueManager,
        mock_repository: AsyncMock,
        session_id,
    ):
        """Test get_next_item with offset skips reviewed items."""
        item = create_mock_item(item_id=5)
        mock_repository.get_queue_items.return_value = [item]

        result = await queue_manager.get_next_item(
            session_id=session_id,
            current_position=4,
        )

        assert result is not None
        assert result.id == 5
        mock_repository.get_queue_items.assert_called_once_with(
            session_id=session_id,
            status=["pending"],
            limit=1,
            offset=4,
        )

    @pytest.mark.asyncio
    async def test_get_next_item_empty_queue_returns_none(
        self,
        queue_manager: QueueManager,
        mock_repository: AsyncMock,
        session_id,
    ):
        """Test get_next_item returns None when queue is empty."""
        mock_repository.get_queue_items.return_value = []

        result = await queue_manager.get_next_item(session_id=session_id)

        assert result is None


class TestReorderQueue:
    """Tests for QueueManager.reorder_queue()."""

    @pytest.mark.asyncio
    async def test_reorder_queue_updates_positions(
        self,
        queue_manager: QueueManager,
        mock_repository: AsyncMock,
        session_id,
    ):
        """Test reorder_queue updates queue positions."""
        items = [
            create_mock_item(item_id=1, ai_confidence=0.8),
            create_mock_item(item_id=2, ai_confidence=0.3),
            create_mock_item(item_id=3, ai_confidence=0.5),
        ]
        mock_repository.get_queue_items.return_value = items

        result = await queue_manager.reorder_queue(
            session_id=session_id,
            priority_mode=PriorityMode.CONFIDENCE,
        )

        # Items should be reordered by confidence (lowest first)
        assert len(result) == 3
        assert result[0].id == 2  # 0.3
        assert result[0].queue_position == 1
        assert result[1].id == 3  # 0.5
        assert result[1].queue_position == 2
        assert result[2].id == 1  # 0.8
        assert result[2].queue_position == 3

    @pytest.mark.asyncio
    async def test_reorder_queue_empty_returns_empty(
        self,
        queue_manager: QueueManager,
        mock_repository: AsyncMock,
        session_id,
    ):
        """Test reorder_queue returns empty list for empty queue."""
        mock_repository.get_queue_items.return_value = []

        result = await queue_manager.reorder_queue(
            session_id=session_id,
            priority_mode=PriorityMode.IMPORTANCE,
        )

        assert result == []


class TestCalculatePriorityScore:
    """Tests for QueueManager._calculate_priority_score()."""

    def test_calculate_priority_score_confidence_mode(self, queue_manager: QueueManager):
        """Test priority score calculation for confidence mode."""
        item = create_mock_item(ai_confidence=0.3)

        score = queue_manager._calculate_priority_score(item, PriorityMode.CONFIDENCE)

        # 1.0 - 0.3 = 0.7
        assert score == pytest.approx(0.7, rel=1e-2)

    def test_calculate_priority_score_importance_mode(self, queue_manager: QueueManager):
        """Test priority score calculation for importance mode."""
        item = create_mock_item(business_importance=8)

        score = queue_manager._calculate_priority_score(item, PriorityMode.IMPORTANCE)

        # 8 / 10 = 0.8
        assert score == pytest.approx(0.8, rel=1e-2)

    def test_calculate_priority_score_recency_mode(self, queue_manager: QueueManager):
        """Test priority score calculation for recency mode."""
        # Very recent item should have score close to 1.0
        now = datetime.now(timezone.utc)
        item = create_mock_item(source_timestamp=now - timedelta(minutes=5))

        score = queue_manager._calculate_priority_score(item, PriorityMode.RECENCY)

        assert score > 0.99  # Very recent

    def test_calculate_priority_score_mixed_mode(self, queue_manager: QueueManager):
        """Test priority score calculation for mixed mode."""
        now = datetime.now(timezone.utc)
        item = create_mock_item(
            ai_confidence=0.5,  # contrib: 0.4 * 0.5 = 0.2
            business_importance=10,  # contrib: 0.35 * 1.0 = 0.35
            source_timestamp=now,  # contrib: 0.25 * 1.0 = 0.25
        )

        score = queue_manager._calculate_priority_score(item, PriorityMode.MIXED)

        # Expected: 0.2 + 0.35 + ~0.25 = ~0.8
        assert score > 0.75
        assert score <= 1.0


class TestRecencyScore:
    """Tests for recency score calculation."""

    def test_recency_score_very_recent(self, queue_manager: QueueManager):
        """Test recency score for very recent timestamp."""
        now = datetime.now(timezone.utc)
        score = queue_manager._calculate_recency_score(now)

        assert score == pytest.approx(1.0, rel=0.01)

    def test_recency_score_very_old(self, queue_manager: QueueManager):
        """Test recency score for old timestamp."""
        old = datetime.now(timezone.utc) - timedelta(days=60)
        score = queue_manager._calculate_recency_score(old)

        assert score == 0.0

    def test_recency_score_half_age(self, queue_manager: QueueManager):
        """Test recency score for timestamp at half max age."""
        half_age = datetime.now(timezone.utc) - timedelta(days=15)
        score = queue_manager._calculate_recency_score(half_age)

        assert score == pytest.approx(0.5, rel=0.05)

    def test_recency_score_handles_naive_timestamp(self, queue_manager: QueueManager):
        """Test recency score handles naive timestamps."""
        naive = datetime.now() - timedelta(days=5)  # Naive timestamp
        score = queue_manager._calculate_recency_score(naive)

        # Should not raise and return valid score
        assert 0.0 <= score <= 1.0

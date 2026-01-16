"""Queue management for the Daily Review workflow.

This module provides queue population and prioritization logic for review items,
supporting multiple priority modes and filter criteria.
"""

from __future__ import annotations

from datetime import datetime, timezone
from decimal import Decimal
from typing import Any, Protocol
from uuid import UUID

from penf_lib.review.models import PriorityMode, ReviewItemDTO, ReviewItemStatus


class ReviewRepositoryProtocol(Protocol):
    """Protocol for repository dependency injection.

    Defines the interface that any repository implementation must satisfy
    to work with the QueueManager.
    """

    async def get_pending_items_for_queue(
        self,
        tenant_id: UUID,
        limit: int,
        confidence_threshold: float,
    ) -> list[ReviewItemDTO]:
        """Fetch pending items that are eligible for review queue.

        Args:
            tenant_id: Tenant isolation identifier
            limit: Maximum number of items to fetch
            confidence_threshold: Minimum AI confidence score

        Returns:
            List of review items eligible for queuing
        """
        ...

    async def get_queue_items(
        self,
        session_id: int,
        status: list[str] | None,
        limit: int,
        offset: int,
    ) -> list[ReviewItemDTO]:
        """Get items currently in the review queue.

        Args:
            session_id: Session identifier
            status: Optional list of status values to filter by
            limit: Maximum number of items to return
            offset: Number of items to skip

        Returns:
            List of review items in the queue
        """
        ...

    async def update_item_queue_position(
        self,
        item_id: int,
        session_id: int,
        queue_position: int,
    ) -> ReviewItemDTO:
        """Update an item's queue position.

        Args:
            item_id: Review item identifier
            session_id: Session to associate item with
            queue_position: New position in queue

        Returns:
            Updated review item
        """
        ...


class QueueManager:
    """Manages review queue population and prioritization.

    The QueueManager is responsible for:
    - Populating the review queue from pending items
    - Applying priority ordering based on different modes
    - Getting the next item for review
    - Reordering existing queue items

    Priority Modes:
    - CONFIDENCE: Sort by AI confidence ascending (lowest first)
    - IMPORTANCE: Sort by business importance descending
    - RECENCY: Sort by source timestamp descending (newest first)
    - MIXED: Weighted combination of all factors
    """

    # Weights for MIXED priority mode
    CONFIDENCE_WEIGHT = 0.4
    IMPORTANCE_WEIGHT = 0.35
    RECENCY_WEIGHT = 0.25

    # Recency scoring parameters (items older than this get minimum score)
    RECENCY_DAYS_MAX = 30

    def __init__(self, repository: ReviewRepositoryProtocol) -> None:
        """Initialize the QueueManager.

        Args:
            repository: Repository implementing ReviewRepositoryProtocol
        """
        self._repository = repository

    async def populate_queue(
        self,
        tenant_id: UUID,
        session_id: int,
        priority_mode: PriorityMode = PriorityMode.MIXED,
        filter_criteria: dict[str, Any] | None = None,
        limit: int = 100,
        confidence_threshold: float = 0.0,
    ) -> list[ReviewItemDTO]:
        """Populate queue with items based on priority mode and filters.

        Fetches pending items from the repository, applies priority ordering
        based on the specified mode, and assigns queue positions.

        Args:
            tenant_id: Tenant isolation identifier
            session_id: Session to associate items with
            priority_mode: How to prioritize items in the queue
            filter_criteria: Optional filters to apply to items
            limit: Maximum number of items to queue
            confidence_threshold: Minimum confidence score for items

        Returns:
            List of review items ordered by priority with queue positions assigned
        """
        # Fetch pending items from repository
        items = await self._repository.get_pending_items_for_queue(
            tenant_id=tenant_id,
            limit=limit,
            confidence_threshold=confidence_threshold,
        )

        # Apply filter criteria if provided
        if filter_criteria:
            items = self._apply_filters(items, filter_criteria)

        # Sort items based on priority mode
        sorted_items = self._sort_by_priority(items, priority_mode)

        # Assign queue positions and update items
        result: list[ReviewItemDTO] = []
        for position, item in enumerate(sorted_items, start=1):
            # Create updated item with queue position and session
            updated_item = item.model_copy(
                update={
                    "queue_position": position,
                    "session_id": session_id,
                }
            )
            result.append(updated_item)

        return result

    async def get_next_item(
        self,
        session_id: int,
        current_position: int = 0,
    ) -> ReviewItemDTO | None:
        """Get next pending item in queue after current position.

        Retrieves items from the queue that have not yet been reviewed,
        returning the next one after the current position.

        Args:
            session_id: Session identifier
            current_position: Current position in queue (0 for start)

        Returns:
            Next pending item, or None if queue is exhausted
        """
        # Get pending items from the queue
        items = await self._repository.get_queue_items(
            session_id=session_id,
            status=[ReviewItemStatus.PENDING.value],
            limit=1,
            offset=current_position,
        )

        if not items:
            return None

        return items[0]

    async def reorder_queue(
        self,
        session_id: int,
        priority_mode: PriorityMode,
    ) -> list[ReviewItemDTO]:
        """Reorder existing queue items by new priority mode.

        Retrieves all pending items in the queue and reorders them
        according to the new priority mode.

        Args:
            session_id: Session identifier
            priority_mode: New priority mode to apply

        Returns:
            List of items with updated queue positions
        """
        # Get all pending items in the queue
        items = await self._repository.get_queue_items(
            session_id=session_id,
            status=[ReviewItemStatus.PENDING.value],
            limit=1000,  # Get all pending items
            offset=0,
        )

        if not items:
            return []

        # Sort items based on new priority mode
        sorted_items = self._sort_by_priority(items, priority_mode)

        # Reassign queue positions
        result: list[ReviewItemDTO] = []
        for position, item in enumerate(sorted_items, start=1):
            updated_item = item.model_copy(
                update={"queue_position": position}
            )
            result.append(updated_item)

        return result

    def _calculate_priority_score(
        self,
        item: ReviewItemDTO,
        priority_mode: PriorityMode,
    ) -> float:
        """Calculate combined priority score for an item.

        Higher scores indicate higher priority (should appear earlier in queue).

        Args:
            item: Review item to score
            priority_mode: Priority calculation mode

        Returns:
            Priority score (higher = more priority)
        """
        if priority_mode == PriorityMode.CONFIDENCE:
            # Lower confidence = higher priority (need more review)
            return 1.0 - float(item.ai_confidence)

        elif priority_mode == PriorityMode.IMPORTANCE:
            # Higher importance = higher priority
            return float(item.business_importance) / 10.0

        elif priority_mode == PriorityMode.RECENCY:
            # More recent = higher priority
            return self._calculate_recency_score(item.source_timestamp)

        elif priority_mode == PriorityMode.MIXED:
            # Weighted combination of all factors
            confidence_score = 1.0 - float(item.ai_confidence)
            importance_score = float(item.business_importance) / 10.0
            recency_score = self._calculate_recency_score(item.source_timestamp)

            return (
                self.CONFIDENCE_WEIGHT * confidence_score
                + self.IMPORTANCE_WEIGHT * importance_score
                + self.RECENCY_WEIGHT * recency_score
            )

        # Default to confidence-based if unknown mode
        return 1.0 - float(item.ai_confidence)

    def _calculate_recency_score(self, timestamp: datetime) -> float:
        """Calculate a recency score for a timestamp.

        More recent items get higher scores (closer to 1.0).
        Items older than RECENCY_DAYS_MAX get minimum score.

        Args:
            timestamp: The timestamp to score

        Returns:
            Recency score between 0.0 and 1.0
        """
        now = datetime.now(timezone.utc)

        # Handle naive timestamps by assuming UTC
        if timestamp.tzinfo is None:
            timestamp = timestamp.replace(tzinfo=timezone.utc)

        age_seconds = (now - timestamp).total_seconds()
        max_age_seconds = self.RECENCY_DAYS_MAX * 24 * 60 * 60

        if age_seconds <= 0:
            return 1.0
        elif age_seconds >= max_age_seconds:
            return 0.0
        else:
            # Linear decay from 1.0 to 0.0 over the max age period
            return 1.0 - (age_seconds / max_age_seconds)

    def _sort_by_priority(
        self,
        items: list[ReviewItemDTO],
        priority_mode: PriorityMode,
    ) -> list[ReviewItemDTO]:
        """Sort items by priority score.

        Args:
            items: Items to sort
            priority_mode: Priority calculation mode

        Returns:
            Items sorted by priority (highest first)
        """
        return sorted(
            items,
            key=lambda item: self._calculate_priority_score(item, priority_mode),
            reverse=True,  # Higher scores first
        )

    def _apply_filters(
        self,
        items: list[ReviewItemDTO],
        filter_criteria: dict[str, Any],
    ) -> list[ReviewItemDTO]:
        """Apply filter criteria to items.

        Supported filter keys:
        - content_type: Filter by content type (email, meeting, document)
        - min_confidence: Minimum AI confidence score
        - max_confidence: Maximum AI confidence score
        - min_importance: Minimum business importance
        - tags: List of required tags (any match)
        - category: Required category prefix

        Args:
            items: Items to filter
            filter_criteria: Dictionary of filter conditions

        Returns:
            Filtered list of items
        """
        result = items

        # Filter by content type
        if "content_type" in filter_criteria:
            content_type = filter_criteria["content_type"]
            result = [
                item for item in result
                if item.content_type.value == content_type
            ]

        # Filter by confidence range
        if "min_confidence" in filter_criteria:
            min_conf = Decimal(str(filter_criteria["min_confidence"]))
            result = [
                item for item in result
                if item.ai_confidence >= min_conf
            ]

        if "max_confidence" in filter_criteria:
            max_conf = Decimal(str(filter_criteria["max_confidence"]))
            result = [
                item for item in result
                if item.ai_confidence <= max_conf
            ]

        # Filter by business importance
        if "min_importance" in filter_criteria:
            min_importance = filter_criteria["min_importance"]
            result = [
                item for item in result
                if item.business_importance >= min_importance
            ]

        # Filter by tags (any match)
        if "tags" in filter_criteria:
            required_tags = set(filter_criteria["tags"])
            result = [
                item for item in result
                if required_tags & set(item.ai_suggestion.tags)
            ]

        # Filter by category prefix
        if "category" in filter_criteria:
            category_prefix = filter_criteria["category"]
            result = [
                item for item in result
                if item.ai_suggestion.category.startswith(category_prefix)
            ]

        return result

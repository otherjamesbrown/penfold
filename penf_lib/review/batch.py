"""Batch operations support for the Daily Review workflow.

This module provides batch management capabilities for efficient processing
of multiple similar review items, including similarity detection, preview
generation, batch execution, and undo support.

Based on R4 requirements from specs/006-daily-review/research.md.
"""

from __future__ import annotations

import re
from dataclasses import dataclass, field
from datetime import datetime, timedelta, timezone
from decimal import Decimal
from enum import Enum
from typing import Any, Protocol
from uuid import UUID, uuid4

from penf_lib.review.exceptions import (
    BatchOperationError,
    UndoNotEligibleError,
)
from penf_lib.review.feedback import FeedbackManager
from penf_lib.review.models import (
    AISuggestion,
    BatchOperationDTO,
    BatchOperationStatus,
    BatchType,
    DecisionType,
    ReviewItemDTO,
    ReviewItemStatus,
    UserCorrection,
)


# =============================================================================
# ENUMERATIONS
# =============================================================================


class BatchGroupType(str, Enum):
    """How to group items for batch operations."""

    THREAD = "thread"
    SENDER = "sender"
    CATEGORY = "category"
    TIME_WINDOW = "time"
    CUSTOM = "custom"


class BatchAction(str, Enum):
    """Actions that can be applied in batch.

    Note: This is distinct from BatchAction in models.py which uses
    ACCEPT_ALL, REJECT_ALL etc. for DTO serialization.
    """

    ACCEPT = "accept"
    REJECT = "reject"
    SKIP = "skip"
    APPLY_CATEGORY = "apply_category"


# =============================================================================
# DATA CLASSES
# =============================================================================


@dataclass
class BatchPreview:
    """Preview of a batch operation before execution.

    Provides information about what will happen if the batch operation
    is confirmed, including affected items and estimated time savings.
    """

    items: list[ReviewItemDTO]
    action: BatchAction
    group_type: BatchGroupType
    group_value: str  # e.g., sender email, category name
    estimated_time_saved_seconds: int

    @property
    def item_count(self) -> int:
        """Return the number of items in the batch."""
        return len(self.items)


@dataclass
class BatchResult:
    """Result of an executed batch operation.

    Contains information about the batch execution including
    success status, affected items, and undo capability.
    """

    batch_id: UUID
    items_affected: int
    action: BatchAction
    success: bool
    undo_eligible: bool
    undo_deadline: datetime
    error_message: str | None = None

    @property
    def is_undoable(self) -> bool:
        """Check if the batch can still be undone."""
        if not self.undo_eligible:
            return False
        return datetime.now(timezone.utc) < self.undo_deadline


@dataclass
class FilterExpression:
    """Parsed filter expression for batch operations.

    Supports simple expressions like:
    - confidence>0.8 - Filter by confidence threshold
    - type=email - Filter by content type
    - sender=john@example.com - Filter by sender
    """

    field: str
    operator: str
    value: str

    @classmethod
    def parse(cls, expression: str) -> "FilterExpression":
        """Parse a filter expression string.

        Args:
            expression: Filter expression (e.g., "confidence>0.8")

        Returns:
            Parsed FilterExpression

        Raises:
            ValueError: If expression is invalid
        """
        # Match patterns like: field>value, field<value, field=value, field>=value, field<=value
        pattern = r"^(\w+)(>=|<=|>|<|=)(.+)$"
        match = re.match(pattern, expression.strip())
        if not match:
            raise ValueError(
                f"Invalid filter expression: {expression}. "
                "Expected format: field<operator>value (e.g., confidence>0.8)"
            )
        return cls(
            field=match.group(1),
            operator=match.group(2),
            value=match.group(3),
        )

    def matches(self, item: ReviewItemDTO) -> bool:
        """Check if a review item matches this filter.

        Args:
            item: The review item to check

        Returns:
            True if the item matches the filter
        """
        # Get the field value from the item
        field_value = self._get_field_value(item)
        if field_value is None:
            return False

        # Compare based on operator
        return self._compare(field_value)

    def _get_field_value(self, item: ReviewItemDTO) -> Any:
        """Extract the field value from a review item.

        Args:
            item: The review item

        Returns:
            The field value or None if not found
        """
        field_map = {
            "confidence": item.ai_confidence,
            "type": item.content_type.value,
            "content_type": item.content_type.value,
            "status": item.status.value,
            "importance": item.business_importance,
        }

        # Check for sender (first participant from ai_suggestion)
        if self.field == "sender":
            participants = item.ai_suggestion.participants
            return participants[0] if participants else None

        # Check for category
        if self.field == "category":
            return item.ai_suggestion.category

        return field_map.get(self.field)

    def _compare(self, field_value: Any) -> bool:
        """Compare the field value against the filter value.

        Args:
            field_value: The value from the item

        Returns:
            True if comparison matches
        """
        # Handle numeric comparisons
        if self.field in ("confidence", "importance"):
            try:
                if isinstance(field_value, Decimal):
                    compare_value = Decimal(self.value)
                else:
                    compare_value = type(field_value)(self.value)
            except (ValueError, TypeError):
                return False

            if self.operator == ">":
                return field_value > compare_value
            elif self.operator == "<":
                return field_value < compare_value
            elif self.operator == ">=":
                return field_value >= compare_value
            elif self.operator == "<=":
                return field_value <= compare_value
            elif self.operator == "=":
                return field_value == compare_value

        # Handle string comparisons (equals only for non-numeric)
        if self.operator == "=":
            return str(field_value).lower() == self.value.lower()

        return False


# =============================================================================
# REPOSITORY PROTOCOL
# =============================================================================


class BatchRepositoryProtocol(Protocol):
    """Protocol for batch repository dependency injection.

    Defines the interface that any repository implementation must satisfy
    to work with the BatchManager.
    """

    async def get_pending_items_for_session(
        self,
        session_id: int,
    ) -> list[Any]:
        """Get all pending items for a session.

        Args:
            session_id: Session identifier

        Returns:
            List of pending review items
        """
        ...

    async def get_items_by_source_id(
        self,
        session_id: int,
        source_id: int,
    ) -> list[Any]:
        """Get items with the same source_id (thread matching).

        Args:
            session_id: Session identifier
            source_id: Source content reference

        Returns:
            List of matching items
        """
        ...

    async def update_items_batch(
        self,
        item_ids: list[int],
        updates: dict[str, Any],
    ) -> int:
        """Update multiple items in a single transaction.

        Args:
            item_ids: List of item IDs to update
            updates: Dictionary of field updates

        Returns:
            Number of items updated
        """
        ...

    async def create_batch_operation(
        self,
        batch_data: dict[str, Any],
    ) -> Any:
        """Create a batch operation record.

        Args:
            batch_data: Batch operation data

        Returns:
            Created batch operation model
        """
        ...

    async def get_batch_operation(
        self,
        batch_id: UUID,
    ) -> Any | None:
        """Get a batch operation by ID.

        Args:
            batch_id: Batch operation UUID

        Returns:
            Batch operation model or None
        """
        ...

    async def update_batch_operation(
        self,
        batch_operation: Any,
    ) -> Any:
        """Update a batch operation record.

        Args:
            batch_operation: Batch operation model to update

        Returns:
            Updated batch operation model
        """
        ...

    async def get_items_by_batch_id(
        self,
        batch_id: UUID,
    ) -> list[Any]:
        """Get all items associated with a batch operation.

        Args:
            batch_id: Batch operation UUID

        Returns:
            List of items in the batch
        """
        ...


# =============================================================================
# BATCH MANAGER
# =============================================================================


class BatchManager:
    """Manages batch operations for review items.

    The BatchManager is responsible for:
    - Finding similar items for batch operations
    - Generating previews of batch operations
    - Executing batch operations in single transactions
    - Managing batch undo capability
    """

    # Undo eligibility window in minutes
    UNDO_ELIGIBILITY_MINUTES = 5

    # Average time per review item (seconds) for time-saved estimation
    AVG_REVIEW_TIME_SECONDS = 15

    # Time window sizes for TIME_WINDOW grouping
    HOUR_WINDOW_SECONDS = 3600
    DAY_WINDOW_SECONDS = 86400

    def __init__(
        self,
        repository: BatchRepositoryProtocol,
        feedback_manager: FeedbackManager,
    ) -> None:
        """Initialize the BatchManager.

        Args:
            repository: Repository implementing BatchRepositoryProtocol
            feedback_manager: FeedbackManager for recording batch decisions
        """
        self._repository = repository
        self._feedback_manager = feedback_manager

    async def find_similar_items(
        self,
        session_id: int,
        reference_item: ReviewItemDTO,
        group_by: BatchGroupType,
        filter_expr: str | None = None,
    ) -> list[ReviewItemDTO]:
        """Find items similar to reference item.

        Locates items in the session that match the reference item
        based on the specified grouping strategy.

        Args:
            session_id: Current session ID
            reference_item: Item to match against
            group_by: How to group (thread, sender, category, time)
            filter_expr: Optional filter expression (e.g., "confidence>0.8")

        Returns:
            List of matching pending items (excluding reference item)

        Raises:
            BatchOperationError: If filter expression is invalid
        """
        # Parse filter expression if provided
        parsed_filter: FilterExpression | None = None
        if filter_expr:
            try:
                parsed_filter = FilterExpression.parse(filter_expr)
            except ValueError as e:
                raise BatchOperationError(
                    batch_id="N/A",
                    reason=str(e),
                )

        # Get all pending items for the session
        all_items = await self._repository.get_pending_items_for_session(session_id)
        pending_items = [self._model_to_dto(item) for item in all_items]

        # Filter to only pending status items (not the reference item)
        pending_items = [
            item
            for item in pending_items
            if item.status == ReviewItemStatus.PENDING and item.id != reference_item.id
        ]

        # Apply grouping strategy
        matching_items = self._apply_grouping(
            reference_item=reference_item,
            candidates=pending_items,
            group_by=group_by,
        )

        # Apply additional filter if provided
        if parsed_filter:
            matching_items = [
                item for item in matching_items if parsed_filter.matches(item)
            ]

        return matching_items

    async def preview_batch(
        self,
        items: list[ReviewItemDTO],
        action: BatchAction,
        group_type: BatchGroupType = BatchGroupType.CUSTOM,
        group_value: str = "",
    ) -> BatchPreview:
        """Generate preview of batch operation.

        Creates a preview showing what will happen if the batch
        operation is confirmed.

        Args:
            items: Items to include in the batch
            action: Action to apply
            group_type: How items were grouped
            group_value: The value used for grouping (e.g., sender email)

        Returns:
            BatchPreview with item list and projected outcome
        """
        # Calculate estimated time saved
        estimated_time_saved = len(items) * self.AVG_REVIEW_TIME_SECONDS

        return BatchPreview(
            items=items,
            action=action,
            group_type=group_type,
            group_value=group_value,
            estimated_time_saved_seconds=estimated_time_saved,
        )

    async def execute_batch(
        self,
        session_id: int,
        items: list[ReviewItemDTO],
        action: BatchAction,
        reason: str | None = None,
        category_override: str | None = None,
    ) -> BatchResult:
        """Execute batch operation on items.

        All items are updated in a single transaction with a shared
        batch_id for group undo capability. Feedback is recorded for
        each item.

        Args:
            session_id: Current session ID
            items: Items to process in batch
            action: Action to apply (accept, reject, skip, apply_category)
            reason: Optional reason for the batch action
            category_override: New category (required for apply_category action)

        Returns:
            BatchResult with counts and batch_id

        Raises:
            BatchOperationError: If batch operation fails
        """
        if not items:
            raise BatchOperationError(
                batch_id="N/A",
                reason="Cannot execute batch with no items",
            )

        if action == BatchAction.APPLY_CATEGORY and not category_override:
            raise BatchOperationError(
                batch_id="N/A",
                reason="category_override required for apply_category action",
            )

        # Generate batch ID and calculate undo deadline
        batch_id = uuid4()
        now = datetime.now(timezone.utc)
        undo_deadline = now + timedelta(minutes=self.UNDO_ELIGIBILITY_MINUTES)

        # Determine new status based on action
        new_status = self._action_to_status(action)

        # Prepare updates for all items
        item_ids = [item.id for item in items]
        updates: dict[str, Any] = {
            "status": new_status,
            "batch_id": batch_id,
            "undo_eligible": True,
            "undo_deadline": undo_deadline,
            "reviewed_at": now,
            "updated_at": now,
        }

        # Map action to decision type for feedback
        decision_type = self._action_to_decision_type(action)

        try:
            # Update all items in single transaction
            affected_count = await self._repository.update_items_batch(
                item_ids=item_ids,
                updates=updates,
            )

            # Create batch operation record
            batch_data = {
                "batch_uuid": batch_id,
                "tenant_id": items[0].tenant_id,
                "session_id": session_id,
                "batch_type": self._infer_batch_type(items),
                "group_criteria": {
                    "item_ids": item_ids,
                    "reason": reason,
                    "category_override": category_override,
                },
                "action_type": self._batch_action_to_model_action(action),
                "action_details": {
                    "new_status": new_status.value,
                    "reason": reason,
                },
                "item_count": len(items),
                "status": BatchOperationStatus.APPLIED,
                "confirmed_at": now,
                "applied_at": now,
                "undo_eligible": True,
                "undo_deadline": undo_deadline,
                "created_at": now,
                "updated_at": now,
            }

            await self._repository.create_batch_operation(batch_data)

            # Record feedback for each item
            for item in items:
                # Update item reference with batch info
                item.batch_id = batch_id
                item.user_decision = decision_type

                await self._record_batch_feedback(
                    item=item,
                    session_id=session_id,
                    decision_type=decision_type,
                    reason=reason,
                    category_override=category_override,
                )

            return BatchResult(
                batch_id=batch_id,
                items_affected=affected_count,
                action=action,
                success=True,
                undo_eligible=True,
                undo_deadline=undo_deadline,
            )

        except Exception as e:
            return BatchResult(
                batch_id=batch_id,
                items_affected=0,
                action=action,
                success=False,
                undo_eligible=False,
                undo_deadline=undo_deadline,
                error_message=str(e),
            )

    async def undo_batch(
        self,
        batch_id: UUID,
    ) -> int:
        """Undo an entire batch operation.

        Restores all items in the batch to their pending state.

        Args:
            batch_id: UUID of the batch to undo

        Returns:
            Number of items restored

        Raises:
            UndoNotEligibleError: If batch cannot be undone
            BatchOperationError: If batch not found
        """
        # Get batch operation record
        batch_op = await self._repository.get_batch_operation(batch_id)
        if batch_op is None:
            raise BatchOperationError(
                batch_id=str(batch_id),
                reason="Batch operation not found",
            )

        # Check undo eligibility
        now = datetime.now(timezone.utc)
        undo_deadline = self._get_undo_deadline(batch_op)

        if not self._is_undo_eligible(batch_op):
            raise UndoNotEligibleError(
                item_id=0,  # Batch undo - no single item
                reason="Batch is not eligible for undo",
            )

        if undo_deadline is None or now > undo_deadline:
            raise UndoNotEligibleError(
                item_id=0,
                reason="Undo window has expired",
            )

        # Get all items in the batch
        batch_items = await self._repository.get_items_by_batch_id(batch_id)
        item_ids = [self._get_item_id(item) for item in batch_items]

        if not item_ids:
            return 0

        # Restore items to pending state
        updates: dict[str, Any] = {
            "status": ReviewItemStatus.PENDING,
            "batch_id": None,
            "undo_eligible": False,
            "undo_deadline": None,
            "reviewed_at": None,
            "user_decision": None,
            "updated_at": now,
        }

        restored_count = await self._repository.update_items_batch(
            item_ids=item_ids,
            updates=updates,
        )

        # Update batch operation status
        batch_op.status = BatchOperationStatus.UNDONE
        batch_op.undone_at = now
        batch_op.undo_eligible = False
        batch_op.updated_at = now

        await self._repository.update_batch_operation(batch_op)

        return restored_count

    def _apply_grouping(
        self,
        reference_item: ReviewItemDTO,
        candidates: list[ReviewItemDTO],
        group_by: BatchGroupType,
    ) -> list[ReviewItemDTO]:
        """Apply grouping strategy to find matching items.

        Args:
            reference_item: The reference item to match against
            candidates: Candidate items to filter
            group_by: Grouping strategy

        Returns:
            List of matching items
        """
        if group_by == BatchGroupType.THREAD:
            # Match by source_id (thread)
            return [
                item
                for item in candidates
                if item.source_id == reference_item.source_id
            ]

        elif group_by == BatchGroupType.SENDER:
            # Match by first participant (sender)
            ref_sender = self._get_sender(reference_item)
            if not ref_sender:
                return []
            return [
                item
                for item in candidates
                if self._get_sender(item) == ref_sender
            ]

        elif group_by == BatchGroupType.CATEGORY:
            # Match by AI-suggested category
            ref_category = reference_item.ai_suggestion.category
            return [
                item
                for item in candidates
                if item.ai_suggestion.category == ref_category
            ]

        elif group_by == BatchGroupType.TIME_WINDOW:
            # Match by time window (same hour)
            ref_hour = reference_item.source_timestamp.replace(
                minute=0, second=0, microsecond=0
            )
            return [
                item
                for item in candidates
                if item.source_timestamp.replace(
                    minute=0, second=0, microsecond=0
                ) == ref_hour
            ]

        elif group_by == BatchGroupType.CUSTOM:
            # Custom grouping - return all candidates (filter should be applied separately)
            return candidates

        return []

    def _get_sender(self, item: ReviewItemDTO) -> str | None:
        """Extract sender from review item.

        Args:
            item: The review item

        Returns:
            First participant (sender) or None
        """
        participants = item.ai_suggestion.participants
        return participants[0] if participants else None

    def _action_to_status(self, action: BatchAction) -> ReviewItemStatus:
        """Convert batch action to review item status.

        Args:
            action: The batch action

        Returns:
            Corresponding ReviewItemStatus
        """
        status_map = {
            BatchAction.ACCEPT: ReviewItemStatus.ACCEPTED,
            BatchAction.REJECT: ReviewItemStatus.REJECTED,
            BatchAction.SKIP: ReviewItemStatus.SKIPPED,
            BatchAction.APPLY_CATEGORY: ReviewItemStatus.MODIFIED,
        }
        return status_map.get(action, ReviewItemStatus.PENDING)

    def _action_to_decision_type(self, action: BatchAction) -> DecisionType:
        """Convert batch action to decision type.

        Args:
            action: The batch action

        Returns:
            Corresponding DecisionType
        """
        decision_map = {
            BatchAction.ACCEPT: DecisionType.ACCEPT,
            BatchAction.REJECT: DecisionType.REJECT,
            BatchAction.SKIP: DecisionType.SKIP,
            BatchAction.APPLY_CATEGORY: DecisionType.MODIFY,
        }
        return decision_map.get(action, DecisionType.ACCEPT)

    def _batch_action_to_model_action(
        self,
        action: BatchAction,
    ) -> "BatchType":
        """Convert BatchAction to model BatchType for storage.

        Note: This maps to BatchType enum from models.py for action_type field.
        The BatchOperationDTO uses BatchAction for action semantics.

        Args:
            action: The batch action type

        Returns:
            Corresponding BatchType for storage
        """
        # Map to the closest BatchType for storage
        # BatchType is about grouping, not action, so we return CUSTOM
        return BatchType.CUSTOM

    def _infer_batch_type(self, items: list[ReviewItemDTO]) -> BatchType:
        """Infer the batch type from items.

        Args:
            items: Items in the batch

        Returns:
            Inferred BatchType
        """
        if len(items) < 2:
            return BatchType.CUSTOM

        # Check if all items have same source_id (thread)
        source_ids = {item.source_id for item in items}
        if len(source_ids) == 1:
            return BatchType.THREAD

        # Check if all items have same sender
        senders = {self._get_sender(item) for item in items}
        if len(senders) == 1 and None not in senders:
            return BatchType.SENDER

        # Check if all items have same category
        categories = {item.ai_suggestion.category for item in items}
        if len(categories) == 1:
            return BatchType.CATEGORY

        return BatchType.CUSTOM

    async def _record_batch_feedback(
        self,
        item: ReviewItemDTO,
        session_id: int,
        decision_type: DecisionType,
        reason: str | None,
        category_override: str | None,
    ) -> None:
        """Record feedback for a batch item.

        Args:
            item: The review item
            session_id: Session ID
            decision_type: Type of decision
            reason: Optional reason
            category_override: New category for modify actions
        """
        if decision_type == DecisionType.ACCEPT:
            await self._feedback_manager.record_accept(
                item=item,
                session_id=session_id,
                time_spent_ms=0,  # Batch operations are instant
            )
        elif decision_type == DecisionType.REJECT:
            await self._feedback_manager.record_reject(
                item=item,
                session_id=session_id,
                reason=reason,
                time_spent_ms=0,
            )
        elif decision_type == DecisionType.SKIP:
            await self._feedback_manager.record_skip(
                item=item,
                session_id=session_id,
                reason=reason,
                time_spent_ms=0,
            )
        elif decision_type == DecisionType.MODIFY:
            correction = UserCorrection(
                category=category_override,
                reason=reason,
            )
            await self._feedback_manager.record_modify(
                item=item,
                session_id=session_id,
                correction=correction,
                time_spent_ms=0,
            )

    def _model_to_dto(self, model: Any) -> ReviewItemDTO:
        """Convert database model to ReviewItemDTO.

        Handles both dictionary-like and object-like models from the
        repository layer.

        Args:
            model: Database model or dictionary

        Returns:
            ReviewItemDTO instance
        """
        if isinstance(model, ReviewItemDTO):
            return model

        if isinstance(model, dict):
            # Handle AISuggestion conversion if needed
            ai_suggestion = model.get("ai_suggestion")
            if isinstance(ai_suggestion, dict):
                model["ai_suggestion"] = AISuggestion(**ai_suggestion)

            # Handle UserCorrection conversion if needed
            user_correction = model.get("user_correction")
            if isinstance(user_correction, dict):
                model["user_correction"] = UserCorrection(**user_correction)

            return ReviewItemDTO(**model)

        # Handle SQLAlchemy model or similar object
        ai_suggestion = model.ai_suggestion
        if isinstance(ai_suggestion, dict):
            ai_suggestion = AISuggestion(**ai_suggestion)

        user_correction = model.user_correction
        if isinstance(user_correction, dict):
            user_correction = UserCorrection(**user_correction)

        return ReviewItemDTO(
            id=model.id,
            tenant_id=model.tenant_id,
            session_id=model.session_id,
            source_id=model.source_id,
            processing_result_id=model.processing_result_id,
            queue_position=model.queue_position,
            status=model.status,
            content_type=model.content_type,
            content_preview=model.content_preview,
            ai_suggestion=ai_suggestion,
            ai_confidence=model.ai_confidence,
            ai_model=model.ai_model,
            business_importance=model.business_importance,
            source_timestamp=model.source_timestamp,
            reviewed_at=model.reviewed_at,
            review_duration_ms=model.review_duration_ms,
            user_decision=model.user_decision,
            user_correction=user_correction,
            batch_id=model.batch_id,
            undo_eligible=model.undo_eligible,
            undo_deadline=model.undo_deadline,
            created_at=model.created_at,
            updated_at=model.updated_at,
        )

    def _get_undo_deadline(self, model: Any) -> datetime | None:
        """Extract undo deadline from batch operation model.

        Args:
            model: Batch operation model (dict or object)

        Returns:
            Undo deadline datetime or None
        """
        if isinstance(model, dict):
            return model.get("undo_deadline")
        return getattr(model, "undo_deadline", None)

    def _is_undo_eligible(self, model: Any) -> bool:
        """Check if batch operation is eligible for undo.

        Args:
            model: Batch operation model (dict or object)

        Returns:
            True if undo eligible
        """
        if isinstance(model, dict):
            return model.get("undo_eligible", False)
        return getattr(model, "undo_eligible", False)

    def _get_item_id(self, model: Any) -> int:
        """Extract item ID from model.

        Args:
            model: Item model (dict or object)

        Returns:
            Item ID
        """
        if isinstance(model, dict):
            return model.get("id", 0)
        return getattr(model, "id", 0)

"""User feedback capture and learning integration for the Daily Review workflow.

This module provides feedback management including capturing user decisions
(accept/reject/modify/skip), calculating learning signal quality, and
supporting undo operations.

Based on R5 requirements from specs/006-daily-review/research.md.
"""

from __future__ import annotations

from datetime import datetime, timedelta, timezone
from decimal import Decimal
from typing import Any, Protocol
from uuid import UUID, uuid4

from penf_lib.review.exceptions import (
    ItemNotFoundError,
    UndoNotEligibleError,
)
from penf_lib.review.models import (
    AISuggestion,
    DecisionType,
    ReviewItemDTO,
    UserCorrection,
    UserFeedbackDTO,
)


class FeedbackRepositoryProtocol(Protocol):
    """Protocol for feedback repository dependency injection.

    Defines the interface that any repository implementation must satisfy
    to work with the FeedbackManager.
    """

    async def create_feedback(self, feedback_data: dict[str, Any]) -> Any:
        """Create a new feedback record in the database.

        Args:
            feedback_data: Feedback data dictionary

        Returns:
            Created feedback model
        """
        ...

    async def get_feedback_by_id(self, feedback_id: int) -> Any | None:
        """Get feedback by ID.

        Args:
            feedback_id: Feedback identifier

        Returns:
            Feedback model or None if not found
        """
        ...

    async def get_feedback_by_review_item(
        self,
        review_item_id: int,
    ) -> Any | None:
        """Get feedback for a specific review item.

        Args:
            review_item_id: Review item identifier

        Returns:
            Feedback model or None if not found
        """
        ...

    async def get_undo_eligible_feedback(
        self,
        session_id: int,
        count: int,
    ) -> list[Any]:
        """Get recent feedback records eligible for undo.

        Args:
            session_id: Session identifier
            count: Maximum number of records to return

        Returns:
            List of feedback models eligible for undo, ordered by most recent
        """
        ...

    async def mark_feedback_undone(self, feedback_id: int) -> bool:
        """Mark a feedback record as undone.

        Args:
            feedback_id: Feedback identifier

        Returns:
            True if successfully marked as undone, False otherwise
        """
        ...

    async def update_feedback(self, feedback: Any) -> Any:
        """Update feedback in database.

        Args:
            feedback: Feedback model to update

        Returns:
            Updated feedback model
        """
        ...


class FeedbackManager:
    """Manages user feedback capture and learning integration.

    The FeedbackManager is responsible for:
    - Recording user decisions (accept, reject, modify, skip)
    - Calculating learning signal quality for the learning system
    - Managing undo eligibility and operations
    - Preparing feedback data for model training
    """

    # Undo eligibility window in minutes
    UNDO_ELIGIBILITY_MINUTES = 5

    # Learning signal quality base scores by decision type
    _DECISION_BASE_SCORES: dict[DecisionType, Decimal] = {
        DecisionType.ACCEPT: Decimal("0.3"),
        DecisionType.REJECT: Decimal("0.5"),
        DecisionType.MODIFY: Decimal("0.8"),
        DecisionType.SKIP: Decimal("0.1"),
    }

    # Time thresholds for signal quality (in milliseconds)
    _QUICK_DECISION_MS = 2000
    _THOUGHTFUL_DECISION_MS = 10000

    # Confidence threshold for high-confidence signals
    _HIGH_CONFIDENCE_THRESHOLD = Decimal("0.8")
    _LOW_CONFIDENCE_THRESHOLD = Decimal("0.5")

    def __init__(self, repository: FeedbackRepositoryProtocol) -> None:
        """Initialize the FeedbackManager.

        Args:
            repository: Repository implementing FeedbackRepositoryProtocol
        """
        self._repository = repository

    async def record_accept(
        self,
        item: ReviewItemDTO,
        session_id: int,
        time_spent_ms: int = 0,
    ) -> UserFeedbackDTO:
        """Record acceptance of AI suggestion.

        The user agrees with the AI's categorization without modifications.

        Args:
            item: The review item being accepted
            session_id: Session context for the decision
            time_spent_ms: Time user spent reviewing the item

        Returns:
            Created feedback record as DTO
        """
        learning_quality = self.calculate_learning_signal_quality(
            decision=DecisionType.ACCEPT,
            confidence=item.ai_confidence,
            time_spent_ms=time_spent_ms,
        )

        feedback_data = self._build_feedback_data(
            item=item,
            session_id=session_id,
            decision_type=DecisionType.ACCEPT,
            time_spent_ms=time_spent_ms,
            learning_signal_quality=learning_quality,
        )

        created = await self._repository.create_feedback(feedback_data)
        return self._model_to_dto(created)

    async def record_reject(
        self,
        item: ReviewItemDTO,
        session_id: int,
        reason: str | None = None,
        time_spent_ms: int = 0,
    ) -> UserFeedbackDTO:
        """Record rejection of AI suggestion with optional reason.

        The user disagrees with the AI's categorization entirely.

        Args:
            item: The review item being rejected
            session_id: Session context for the decision
            reason: Optional reason code for the rejection
            time_spent_ms: Time user spent reviewing the item

        Returns:
            Created feedback record as DTO
        """
        learning_quality = self.calculate_learning_signal_quality(
            decision=DecisionType.REJECT,
            confidence=item.ai_confidence,
            time_spent_ms=time_spent_ms,
        )

        feedback_data = self._build_feedback_data(
            item=item,
            session_id=session_id,
            decision_type=DecisionType.REJECT,
            time_spent_ms=time_spent_ms,
            learning_signal_quality=learning_quality,
            correction_reason=reason,
        )

        created = await self._repository.create_feedback(feedback_data)
        return self._model_to_dto(created)

    async def record_modify(
        self,
        item: ReviewItemDTO,
        session_id: int,
        correction: UserCorrection,
        time_spent_ms: int = 0,
    ) -> UserFeedbackDTO:
        """Record modification to AI suggestion.

        The user partially agrees but makes corrections to the AI's output.
        This provides the most valuable learning signal.

        Args:
            item: The review item being modified
            session_id: Session context for the decision
            correction: User's corrections to the AI suggestion
            time_spent_ms: Time user spent reviewing the item

        Returns:
            Created feedback record as DTO
        """
        learning_quality = self.calculate_learning_signal_quality(
            decision=DecisionType.MODIFY,
            confidence=item.ai_confidence,
            time_spent_ms=time_spent_ms,
        )

        feedback_data = self._build_feedback_data(
            item=item,
            session_id=session_id,
            decision_type=DecisionType.MODIFY,
            time_spent_ms=time_spent_ms,
            learning_signal_quality=learning_quality,
            user_correction=correction,
            correction_reason=correction.reason,
            correction_notes=correction.notes,
        )

        created = await self._repository.create_feedback(feedback_data)
        return self._model_to_dto(created)

    async def record_skip(
        self,
        item: ReviewItemDTO,
        session_id: int,
        reason: str | None = None,
        time_spent_ms: int = 0,
    ) -> UserFeedbackDTO:
        """Record skip decision.

        The user defers the decision, perhaps due to uncertainty or
        needing more context. Provides minimal learning signal.

        Args:
            item: The review item being skipped
            session_id: Session context for the decision
            reason: Optional reason for skipping
            time_spent_ms: Time user spent reviewing the item

        Returns:
            Created feedback record as DTO
        """
        learning_quality = self.calculate_learning_signal_quality(
            decision=DecisionType.SKIP,
            confidence=item.ai_confidence,
            time_spent_ms=time_spent_ms,
        )

        feedback_data = self._build_feedback_data(
            item=item,
            session_id=session_id,
            decision_type=DecisionType.SKIP,
            time_spent_ms=time_spent_ms,
            learning_signal_quality=learning_quality,
            correction_notes=reason,
        )

        created = await self._repository.create_feedback(feedback_data)
        return self._model_to_dto(created)

    async def undo_decision(
        self,
        feedback_id: int,
    ) -> bool:
        """Undo a recent decision if eligible.

        Checks that the feedback exists and is still within the undo
        eligibility window.

        Args:
            feedback_id: Identifier of the feedback to undo

        Returns:
            True if successfully undone

        Raises:
            ItemNotFoundError: If feedback record doesn't exist
            UndoNotEligibleError: If feedback cannot be undone
        """
        feedback = await self._repository.get_feedback_by_id(feedback_id)
        if feedback is None:
            raise ItemNotFoundError(item_id=feedback_id)

        # Check undo eligibility
        now = datetime.now(timezone.utc)
        undo_deadline = self._get_undo_deadline(feedback)

        if undo_deadline is None or now > undo_deadline:
            raise UndoNotEligibleError(
                item_id=feedback_id,
                reason="Undo window has expired",
            )

        return await self._repository.mark_feedback_undone(feedback_id)

    async def get_undo_eligible(
        self,
        session_id: int,
        count: int = 1,
    ) -> list[UserFeedbackDTO]:
        """Get recent decisions eligible for undo.

        Returns the most recent feedback records that are still within
        the undo eligibility window.

        Args:
            session_id: Session to get eligible feedback for
            count: Maximum number of records to return

        Returns:
            List of feedback DTOs eligible for undo, most recent first
        """
        feedback_records = await self._repository.get_undo_eligible_feedback(
            session_id=session_id,
            count=count,
        )
        return [self._model_to_dto(record) for record in feedback_records]

    def calculate_learning_signal_quality(
        self,
        decision: DecisionType,
        confidence: Decimal,
        time_spent_ms: int,
    ) -> Decimal:
        """Calculate quality score for learning system.

        The quality score indicates how valuable this feedback is for
        model training:
        - High-confidence corrections are most valuable (model was confident but wrong)
        - Quick decisions on high-confidence items provide less signal
        - Thoughtful corrections (more time spent) are more valuable

        Algorithm:
        1. Base score from decision type: accept=0.3, reject=0.5, modify=0.8, skip=0.1
        2. Confidence factor:
           - If modify/reject with confidence > 0.8: multiply by 1.5
           - If accept with confidence < 0.5: multiply by 0.5
        3. Time factor:
           - < 2 seconds: multiply by 0.7 (too quick, possibly accidental)
           - 2-10 seconds: multiply by 1.0 (reasonable decision time)
           - > 10 seconds: multiply by 1.2 (thoughtful, deliberate decision)
        4. Clamp final score to [0.0, 1.0]

        Args:
            decision: The type of decision made
            confidence: AI confidence score for the original suggestion
            time_spent_ms: Time user spent on the decision

        Returns:
            Quality score between 0.0 and 1.0
        """
        # Start with base score for decision type
        score = self._DECISION_BASE_SCORES.get(decision, Decimal("0.3"))

        # Apply confidence factor
        if decision in (DecisionType.MODIFY, DecisionType.REJECT):
            # Corrections to high-confidence predictions are more valuable
            if confidence > self._HIGH_CONFIDENCE_THRESHOLD:
                score *= Decimal("1.5")
        elif decision == DecisionType.ACCEPT:
            # Accepting low-confidence predictions is less valuable
            if confidence < self._LOW_CONFIDENCE_THRESHOLD:
                score *= Decimal("0.5")

        # Apply time factor
        if time_spent_ms < self._QUICK_DECISION_MS:
            # Very quick decisions may be less reliable
            score *= Decimal("0.7")
        elif time_spent_ms > self._THOUGHTFUL_DECISION_MS:
            # Thoughtful decisions are more valuable
            score *= Decimal("1.2")
        # Between 2-10 seconds: factor is 1.0 (no change)

        # Clamp to valid range [0.0, 1.0]
        return max(Decimal("0.0"), min(Decimal("1.0"), score))

    def _build_feedback_data(
        self,
        item: ReviewItemDTO,
        session_id: int,
        decision_type: DecisionType,
        time_spent_ms: int,
        learning_signal_quality: Decimal,
        user_correction: UserCorrection | None = None,
        correction_reason: str | None = None,
        correction_notes: str | None = None,
    ) -> dict[str, Any]:
        """Build feedback data dictionary for repository.

        Args:
            item: The review item
            session_id: Session context
            decision_type: Type of decision
            time_spent_ms: Time spent on decision
            learning_signal_quality: Calculated quality score
            user_correction: User's corrections (for modify)
            correction_reason: Reason code for correction
            correction_notes: Free-form correction notes

        Returns:
            Dictionary ready for repository creation
        """
        now = datetime.now(timezone.utc)
        undo_deadline = now + timedelta(minutes=self.UNDO_ELIGIBILITY_MINUTES)

        return {
            "feedback_uuid": uuid4(),
            "tenant_id": item.tenant_id,
            "review_item_id": item.id,
            "session_id": session_id,
            "decision_type": decision_type,
            "original_suggestion": AISuggestion(
                category=item.ai_suggestion.category,
                participants=item.ai_suggestion.participants,
                tags=item.ai_suggestion.tags,
            ),
            "user_correction": user_correction,
            "correction_reason": correction_reason,
            "correction_notes": correction_notes,
            "confidence_feedback": None,
            "time_spent_ms": time_spent_ms,
            "was_batch_decision": item.batch_id is not None,
            "batch_id": item.batch_id,
            "learning_rule_id": None,
            "learning_signal_quality": learning_signal_quality,
            "undo_eligible": True,
            "undo_deadline": undo_deadline,
            "created_at": now,
        }

    def _get_undo_deadline(self, model: Any) -> datetime | None:
        """Extract undo deadline from feedback model.

        Args:
            model: Feedback model (dict or object)

        Returns:
            Undo deadline datetime or None
        """
        if isinstance(model, dict):
            return model.get("undo_deadline")
        return getattr(model, "undo_deadline", None)

    def _model_to_dto(self, model: Any) -> UserFeedbackDTO:
        """Convert database model to DTO.

        Handles both dictionary-like and object-like models from the
        repository layer.

        Args:
            model: Database model or dictionary

        Returns:
            UserFeedbackDTO instance
        """
        if isinstance(model, dict):
            # Handle AISuggestion conversion if needed
            original = model.get("original_suggestion")
            if isinstance(original, dict):
                model["original_suggestion"] = AISuggestion(**original)

            # Handle UserCorrection conversion if needed
            correction = model.get("user_correction")
            if isinstance(correction, dict):
                model["user_correction"] = UserCorrection(**correction)

            return UserFeedbackDTO(**model)

        # Handle SQLAlchemy model or similar object
        original_suggestion = model.original_suggestion
        if isinstance(original_suggestion, dict):
            original_suggestion = AISuggestion(**original_suggestion)

        user_correction = model.user_correction
        if isinstance(user_correction, dict):
            user_correction = UserCorrection(**user_correction)

        return UserFeedbackDTO(
            id=model.id,
            feedback_uuid=model.feedback_uuid,
            tenant_id=model.tenant_id,
            review_item_id=model.review_item_id,
            session_id=model.session_id,
            decision_type=model.decision_type,
            original_suggestion=original_suggestion,
            user_correction=user_correction,
            correction_reason=model.correction_reason,
            correction_notes=model.correction_notes,
            confidence_feedback=model.confidence_feedback,
            time_spent_ms=model.time_spent_ms,
            was_batch_decision=model.was_batch_decision,
            batch_id=model.batch_id,
            learning_rule_id=model.learning_rule_id,
            learning_signal_quality=model.learning_signal_quality,
            created_at=model.created_at,
        )

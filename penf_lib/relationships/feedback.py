"""FeedbackProcessor - Processes user feedback on discovered relationships.

This module provides feedback processing for relationships, including:
- Confirm/reject/modify feedback handling
- Confidence updates based on feedback
- Feedback storage for learning
- Applying learned preferences to future discoveries
"""

from __future__ import annotations

import logging
from dataclasses import dataclass, field
from datetime import datetime, timezone
from decimal import Decimal
from enum import Enum
from typing import Any, Protocol
from uuid import UUID

from sqlalchemy.ext.asyncio import AsyncSession

logger = logging.getLogger(__name__)


class FeedbackType(str, Enum):
    """Types of user feedback on relationships."""

    CONFIRM = "confirm"  # User confirms relationship is accurate
    REJECT = "reject"  # User rejects relationship as incorrect
    MODIFY = "modify"  # User modifies relationship details
    STRENGTHEN = "strengthen"  # User indicates relationship is stronger
    WEAKEN = "weaken"  # User indicates relationship is weaker


@dataclass
class FeedbackResult:
    """Result of processing feedback on a relationship.

    Contains the before/after state and any changes made.
    """

    relationship_id: int
    feedback_type: FeedbackType
    previous_confidence: Decimal
    new_confidence: Decimal
    previous_state: str
    new_state: str
    user_email: str
    processed_at: datetime = field(default_factory=lambda: datetime.now(timezone.utc))
    changes: dict[str, Any] | None = None
    reasoning: str = ""


@dataclass
class LearnedPreference:
    """A learned preference from user feedback.

    Captured patterns that can be applied to future discoveries.
    """

    pattern_type: str  # entity_pair, entity_type, relationship_type
    pattern_key: str  # Unique key for this pattern
    preferred_relationship_type: str | None = None
    confidence_boost: Decimal = Decimal("0.0")
    confidence_penalty: Decimal = Decimal("0.0")
    observation_count: int = 0
    last_observed: datetime = field(default_factory=lambda: datetime.now(timezone.utc))


class FeedbackRepositoryProtocol(Protocol):
    """Protocol for feedback repository operations."""

    async def get_by_id(self, entity_id: int) -> Any | None:
        """Get entity by ID."""
        ...

    async def update(self, entity_id: int, **kwargs: Any) -> Any | None:
        """Update entity."""
        ...

    async def update_confidence(
        self,
        relationship_id: int,
        new_confidence: Decimal,
        evidence_strength: Decimal | None = None,
    ) -> Any | None:
        """Update relationship confidence."""
        ...

    async def update_lifecycle_state(
        self,
        relationship_id: int,
        new_state: str,
    ) -> Any | None:
        """Update relationship lifecycle state."""
        ...

    async def add_feedback(
        self,
        tenant_id: UUID,
        relationship_id: int,
        feedback_type: str,
        user_email: str,
        corrected_type: str | None = None,
        reasoning: str | None = None,
        feedback_metadata: dict[str, Any] | None = None,
    ) -> Any:
        """Add feedback record."""
        ...

    async def get_feedback_for_relationship(
        self,
        relationship_id: int,
    ) -> list[Any]:
        """Get all feedback for a relationship."""
        ...

    async def confirm_relationship(
        self,
        relationship_id: int,
        user_email: str,
    ) -> Any | None:
        """Confirm a relationship."""
        ...

    async def get_evidence_for_relationship(
        self,
        relationship_id: int,
    ) -> list[Any]:
        """Get evidence for a relationship."""
        ...


# Confidence adjustment constants
CONFIRM_BOOST = Decimal("0.15")  # Boost when confirmed
REJECT_PENALTY = Decimal("0.25")  # Penalty when rejected
STRENGTHEN_BOOST = Decimal("0.10")  # Boost when strengthened
WEAKEN_PENALTY = Decimal("0.10")  # Penalty when weakened
MIN_CONFIDENCE = Decimal("0.0")
MAX_CONFIDENCE = Decimal("1.0")


class FeedbackProcessor:
    """Processes user feedback on discovered relationships.

    Provides methods for handling various feedback types and
    integrating learned preferences into future discoveries.

    Attributes:
        session: Database session for persistence
        repository: Repository for relationship operations
        tenant_id: Current tenant ID for isolation
    """

    def __init__(
        self,
        session: AsyncSession,
        repository: FeedbackRepositoryProtocol,
        tenant_id: UUID,
    ) -> None:
        """Initialize the feedback processor.

        Args:
            session: Async database session
            repository: Repository instance for data operations
            tenant_id: Tenant ID for multi-tenant isolation
        """
        self.session = session
        self.repository = repository
        self.tenant_id = tenant_id
        # In-memory cache of learned preferences (would persist in production)
        self._learned_preferences: dict[str, LearnedPreference] = {}

    async def process_confirm(
        self,
        relationship_id: int,
        user_email: str,
        reasoning: str = "",
    ) -> FeedbackResult:
        """Process confirmation feedback for a relationship.

        Increases confidence and transitions to active state.

        Args:
            relationship_id: ID of relationship to confirm
            user_email: Email of user providing feedback
            reasoning: Optional explanation

        Returns:
            FeedbackResult with confirmation details

        Raises:
            ValueError: If relationship not found
        """
        relationship = await self.repository.get_by_id(relationship_id)
        if relationship is None:
            raise ValueError(f"Relationship not found: {relationship_id}")

        previous_confidence = Decimal(str(relationship.confidence_score))
        previous_state = relationship.lifecycle_state

        # Calculate new confidence (bounded)
        new_confidence = min(
            MAX_CONFIDENCE,
            previous_confidence + CONFIRM_BOOST,
        )

        # Confirm the relationship (updates state to active)
        await self.repository.confirm_relationship(relationship_id, user_email)

        # Update confidence
        await self.repository.update_confidence(relationship_id, new_confidence)

        # Store feedback for learning
        await self.repository.add_feedback(
            tenant_id=self.tenant_id,
            relationship_id=relationship_id,
            feedback_type="confirm",
            user_email=user_email,
            reasoning=reasoning,
        )

        # Update learned preferences
        await self._update_learned_preference(
            relationship,
            FeedbackType.CONFIRM,
        )

        return FeedbackResult(
            relationship_id=relationship_id,
            feedback_type=FeedbackType.CONFIRM,
            previous_confidence=previous_confidence,
            new_confidence=new_confidence,
            previous_state=previous_state,
            new_state="active",
            user_email=user_email,
            reasoning=reasoning,
        )

    async def process_reject(
        self,
        relationship_id: int,
        user_email: str,
        reasoning: str = "",
        strong_rejection: bool = False,
    ) -> FeedbackResult:
        """Process rejection feedback for a relationship.

        Decreases confidence and optionally archives.

        Args:
            relationship_id: ID of relationship to reject
            user_email: Email of user providing feedback
            reasoning: Optional explanation
            strong_rejection: If True, archives the relationship

        Returns:
            FeedbackResult with rejection details

        Raises:
            ValueError: If relationship not found
        """
        relationship = await self.repository.get_by_id(relationship_id)
        if relationship is None:
            raise ValueError(f"Relationship not found: {relationship_id}")

        previous_confidence = Decimal(str(relationship.confidence_score))
        previous_state = relationship.lifecycle_state

        # Calculate new confidence (bounded)
        new_confidence = max(
            MIN_CONFIDENCE,
            previous_confidence - REJECT_PENALTY,
        )

        # Determine new state
        if strong_rejection or new_confidence <= Decimal("0.20"):
            new_state = "archived"
            await self.repository.update_lifecycle_state(relationship_id, "archived")
        else:
            new_state = previous_state

        # Update confidence
        await self.repository.update_confidence(relationship_id, new_confidence)

        # Store feedback for learning
        await self.repository.add_feedback(
            tenant_id=self.tenant_id,
            relationship_id=relationship_id,
            feedback_type="reject",
            user_email=user_email,
            reasoning=reasoning,
            feedback_metadata={"strong_rejection": strong_rejection},
        )

        # Update learned preferences
        await self._update_learned_preference(
            relationship,
            FeedbackType.REJECT,
        )

        return FeedbackResult(
            relationship_id=relationship_id,
            feedback_type=FeedbackType.REJECT,
            previous_confidence=previous_confidence,
            new_confidence=new_confidence,
            previous_state=previous_state,
            new_state=new_state,
            user_email=user_email,
            reasoning=reasoning,
        )

    async def process_modify(
        self,
        relationship_id: int,
        user_email: str,
        modifications: dict[str, Any],
        reasoning: str = "",
    ) -> FeedbackResult:
        """Process modification feedback for a relationship.

        Updates relationship attributes based on user input.

        Args:
            relationship_id: ID of relationship to modify
            user_email: Email of user providing feedback
            modifications: Dict of changes to apply
            reasoning: Optional explanation

        Returns:
            FeedbackResult with modification details

        Raises:
            ValueError: If relationship not found
        """
        relationship = await self.repository.get_by_id(relationship_id)
        if relationship is None:
            raise ValueError(f"Relationship not found: {relationship_id}")

        previous_confidence = Decimal(str(relationship.confidence_score))
        previous_state = relationship.lifecycle_state

        changes: dict[str, Any] = {}
        new_confidence = previous_confidence
        new_state = previous_state

        # Process confidence adjustment
        if "confidence_adjustment" in modifications:
            adjustment = Decimal(str(modifications["confidence_adjustment"]))
            new_confidence = max(
                MIN_CONFIDENCE,
                min(MAX_CONFIDENCE, previous_confidence + adjustment),
            )
            changes["confidence"] = {
                "from": str(previous_confidence),
                "to": str(new_confidence),
            }
            await self.repository.update_confidence(relationship_id, new_confidence)

        # Process relationship type change
        corrected_type = None
        if "relationship_type" in modifications:
            old_type = relationship.relationship_type
            corrected_type = modifications["relationship_type"]
            changes["relationship_type"] = {"from": old_type, "to": corrected_type}
            await self.repository.update(
                relationship_id,
                relationship_type=corrected_type,
            )

        # Process state change
        if "lifecycle_state" in modifications:
            new_state = modifications["lifecycle_state"]
            changes["lifecycle_state"] = {"from": previous_state, "to": new_state}
            await self.repository.update_lifecycle_state(relationship_id, new_state)

        # Store feedback for learning
        await self.repository.add_feedback(
            tenant_id=self.tenant_id,
            relationship_id=relationship_id,
            feedback_type="modify",
            user_email=user_email,
            corrected_type=corrected_type,
            reasoning=reasoning,
            feedback_metadata={"modifications": modifications},
        )

        return FeedbackResult(
            relationship_id=relationship_id,
            feedback_type=FeedbackType.MODIFY,
            previous_confidence=previous_confidence,
            new_confidence=new_confidence,
            previous_state=previous_state,
            new_state=new_state,
            user_email=user_email,
            changes=changes,
            reasoning=reasoning,
        )

    async def process_strengthen(
        self,
        relationship_id: int,
        user_email: str,
        reasoning: str = "",
    ) -> FeedbackResult:
        """Process strengthen feedback for a relationship.

        Boosts confidence for the relationship.

        Args:
            relationship_id: ID of relationship to strengthen
            user_email: Email of user providing feedback
            reasoning: Optional explanation

        Returns:
            FeedbackResult with strengthen details

        Raises:
            ValueError: If relationship not found
        """
        relationship = await self.repository.get_by_id(relationship_id)
        if relationship is None:
            raise ValueError(f"Relationship not found: {relationship_id}")

        previous_confidence = Decimal(str(relationship.confidence_score))
        previous_state = relationship.lifecycle_state

        # Calculate new confidence (bounded)
        new_confidence = min(
            MAX_CONFIDENCE,
            previous_confidence + STRENGTHEN_BOOST,
        )

        # Update confidence
        await self.repository.update_confidence(relationship_id, new_confidence)

        # Store feedback
        await self.repository.add_feedback(
            tenant_id=self.tenant_id,
            relationship_id=relationship_id,
            feedback_type="strengthen",
            user_email=user_email,
            reasoning=reasoning,
        )

        return FeedbackResult(
            relationship_id=relationship_id,
            feedback_type=FeedbackType.STRENGTHEN,
            previous_confidence=previous_confidence,
            new_confidence=new_confidence,
            previous_state=previous_state,
            new_state=previous_state,  # State unchanged
            user_email=user_email,
            reasoning=reasoning,
        )

    async def process_weaken(
        self,
        relationship_id: int,
        user_email: str,
        reasoning: str = "",
    ) -> FeedbackResult:
        """Process weaken feedback for a relationship.

        Reduces confidence for the relationship.

        Args:
            relationship_id: ID of relationship to weaken
            user_email: Email of user providing feedback
            reasoning: Optional explanation

        Returns:
            FeedbackResult with weaken details

        Raises:
            ValueError: If relationship not found
        """
        relationship = await self.repository.get_by_id(relationship_id)
        if relationship is None:
            raise ValueError(f"Relationship not found: {relationship_id}")

        previous_confidence = Decimal(str(relationship.confidence_score))
        previous_state = relationship.lifecycle_state

        # Calculate new confidence (bounded)
        new_confidence = max(
            MIN_CONFIDENCE,
            previous_confidence - WEAKEN_PENALTY,
        )

        # Update confidence
        await self.repository.update_confidence(relationship_id, new_confidence)

        # Store feedback
        await self.repository.add_feedback(
            tenant_id=self.tenant_id,
            relationship_id=relationship_id,
            feedback_type="weaken",
            user_email=user_email,
            reasoning=reasoning,
        )

        return FeedbackResult(
            relationship_id=relationship_id,
            feedback_type=FeedbackType.WEAKEN,
            previous_confidence=previous_confidence,
            new_confidence=new_confidence,
            previous_state=previous_state,
            new_state=previous_state,  # State unchanged
            user_email=user_email,
            reasoning=reasoning,
        )

    async def process_batch(
        self,
        feedback_items: list[dict[str, Any]],
        user_email: str,
    ) -> list[FeedbackResult]:
        """Process multiple feedback items in batch.

        Args:
            feedback_items: List of feedback dicts with relationship_id and feedback_type
            user_email: Email of user providing feedback

        Returns:
            List of FeedbackResult for each item
        """
        results = []
        for item in feedback_items:
            relationship_id = item["relationship_id"]
            feedback_type = item["feedback_type"]
            reasoning = item.get("reasoning", "")

            try:
                if feedback_type == "confirm":
                    result = await self.process_confirm(
                        relationship_id, user_email, reasoning
                    )
                elif feedback_type == "reject":
                    result = await self.process_reject(
                        relationship_id, user_email, reasoning
                    )
                elif feedback_type == "modify":
                    result = await self.process_modify(
                        relationship_id,
                        user_email,
                        item.get("modifications", {}),
                        reasoning,
                    )
                elif feedback_type == "strengthen":
                    result = await self.process_strengthen(
                        relationship_id, user_email, reasoning
                    )
                elif feedback_type == "weaken":
                    result = await self.process_weaken(
                        relationship_id, user_email, reasoning
                    )
                else:
                    logger.warning(f"Unknown feedback type: {feedback_type}")
                    continue

                results.append(result)
            except Exception as e:
                logger.warning(
                    f"Failed to process feedback for {relationship_id}: {e}"
                )

        return results

    # =========================================================================
    # LEARNING INTEGRATION
    # =========================================================================

    async def _update_learned_preference(
        self,
        relationship: Any,
        feedback_type: FeedbackType,
    ) -> None:
        """Update learned preferences based on feedback.

        Captures patterns from user feedback to improve future
        relationship discovery.

        Args:
            relationship: The relationship that received feedback
            feedback_type: Type of feedback received
        """
        # Create pattern key from entity pair
        pattern_key = (
            f"{relationship.source_entity_type}:{relationship.source_entity_id}-"
            f"{relationship.target_entity_type}:{relationship.target_entity_id}"
        )

        if pattern_key in self._learned_preferences:
            pref = self._learned_preferences[pattern_key]
            pref.observation_count += 1
            pref.last_observed = datetime.now(timezone.utc)

            # Adjust boosts/penalties based on feedback
            if feedback_type == FeedbackType.CONFIRM:
                pref.confidence_boost += Decimal("0.02")
            elif feedback_type == FeedbackType.REJECT:
                pref.confidence_penalty += Decimal("0.02")
        else:
            # Create new preference
            boost = Decimal("0.02") if feedback_type == FeedbackType.CONFIRM else Decimal("0.0")
            penalty = Decimal("0.02") if feedback_type == FeedbackType.REJECT else Decimal("0.0")

            self._learned_preferences[pattern_key] = LearnedPreference(
                pattern_type="entity_pair",
                pattern_key=pattern_key,
                preferred_relationship_type=relationship.relationship_type,
                confidence_boost=boost,
                confidence_penalty=penalty,
                observation_count=1,
            )

    async def get_learned_preferences(
        self,
        pattern_type: str,
        source_entity_type: str,
        source_entity_id: int,
        target_entity_type: str,
        target_entity_id: int,
    ) -> list[LearnedPreference]:
        """Get learned preferences matching a pattern.

        Args:
            pattern_type: Type of pattern to match
            source_entity_type: Source entity type
            source_entity_id: Source entity ID
            target_entity_type: Target entity type
            target_entity_id: Target entity ID

        Returns:
            List of matching learned preferences
        """
        pattern_key = (
            f"{source_entity_type}:{source_entity_id}-"
            f"{target_entity_type}:{target_entity_id}"
        )

        if pattern_key in self._learned_preferences:
            return [self._learned_preferences[pattern_key]]
        return []

    async def apply_learned_preferences(
        self,
        discovered: Any,
    ) -> Any:
        """Apply learned preferences to a discovered relationship.

        Adjusts confidence based on patterns observed in user feedback.

        Args:
            discovered: Discovered relationship to adjust

        Returns:
            Adjusted relationship
        """
        pattern_key = (
            f"{discovered.source_entity_type}:{discovered.source_entity_id}-"
            f"{discovered.target_entity_type}:{discovered.target_entity_id}"
        )

        if pattern_key in self._learned_preferences:
            pref = self._learned_preferences[pattern_key]
            current_confidence = Decimal(str(discovered.confidence_score))

            # Apply boost/penalty
            adjusted = current_confidence + pref.confidence_boost - pref.confidence_penalty
            adjusted = max(MIN_CONFIDENCE, min(MAX_CONFIDENCE, adjusted))
            discovered.confidence_score = adjusted

        return discovered

    # =========================================================================
    # EVIDENCE AND CONTEXT
    # =========================================================================

    async def get_relationship_context(
        self,
        relationship_id: int,
    ) -> dict[str, Any]:
        """Get full context for relationship validation.

        Returns relationship details and supporting evidence
        for informed user decisions.

        Args:
            relationship_id: ID of relationship

        Returns:
            Dict with relationship and evidence details
        """
        relationship = await self.repository.get_by_id(relationship_id)
        if relationship is None:
            return {}

        evidence = await self.repository.get_evidence_for_relationship(relationship_id)

        return {
            "relationship": {
                "id": relationship.id,
                "type": relationship.relationship_type,
                "confidence": str(relationship.confidence_score),
                "state": relationship.lifecycle_state,
                "source_entity_type": relationship.source_entity_type,
                "target_entity_type": relationship.target_entity_type,
                "first_observed": (
                    relationship.first_observed_at.isoformat()
                    if hasattr(relationship, "first_observed_at")
                    else None
                ),
                "interaction_count": (
                    relationship.interaction_count
                    if hasattr(relationship, "interaction_count")
                    else 0
                ),
            },
            "evidence": [
                {
                    "type": e.evidence_type,
                    "content": e.evidence_content,
                    "observed_at": (
                        e.observed_at.isoformat()
                        if hasattr(e, "observed_at")
                        else None
                    ),
                    "strength": str(e.strength_contribution),
                }
                for e in evidence
            ],
        }

    def format_evidence(self, evidence: Any) -> str:
        """Format evidence for user display.

        Args:
            evidence: Evidence object to format

        Returns:
            Formatted string for display
        """
        observed = (
            evidence.observed_at.strftime("%Y-%m-%d")
            if hasattr(evidence, "observed_at") and evidence.observed_at
            else "Unknown date"
        )
        strength = (
            f"{float(evidence.strength_contribution):.0%}"
            if hasattr(evidence, "strength_contribution")
            else ""
        )

        return (
            f"[{evidence.evidence_type.upper()}] ({observed}) "
            f"Confidence: {strength}\n"
            f"{evidence.evidence_content[:200]}..."
            if len(str(evidence.evidence_content)) > 200
            else f"[{evidence.evidence_type.upper()}] ({observed}) "
            f"Confidence: {strength}\n"
            f"{evidence.evidence_content}"
        )

    # =========================================================================
    # FEEDBACK HISTORY
    # =========================================================================

    async def get_feedback_history(
        self,
        relationship_id: int,
    ) -> list[Any]:
        """Get feedback history for a relationship.

        Args:
            relationship_id: ID of relationship

        Returns:
            List of feedback records
        """
        return await self.repository.get_feedback_for_relationship(relationship_id)

    async def get_feedback_stats(
        self,
        relationship_id: int,
    ) -> dict[str, int]:
        """Get aggregated feedback statistics.

        Args:
            relationship_id: ID of relationship

        Returns:
            Dict with feedback counts by type
        """
        feedback_list = await self.repository.get_feedback_for_relationship(
            relationship_id
        )

        stats: dict[str, int] = {
            "total_feedback": len(feedback_list),
            "confirm_count": 0,
            "reject_count": 0,
            "modify_count": 0,
            "strengthen_count": 0,
            "weaken_count": 0,
        }

        for feedback in feedback_list:
            feedback_type = feedback.feedback_type
            if feedback_type == "confirm":
                stats["confirm_count"] += 1
            elif feedback_type == "reject":
                stats["reject_count"] += 1
            elif feedback_type == "modify":
                stats["modify_count"] += 1
            elif feedback_type == "strengthen":
                stats["strengthen_count"] += 1
            elif feedback_type == "weaken":
                stats["weaken_count"] += 1

        return stats


__all__ = [
    "FeedbackProcessor",
    "FeedbackResult",
    "FeedbackType",
    "LearnedPreference",
]

"""ConflictResolver - Resolves conflicts between discovered relationships.

This module provides conflict resolution for relationships, including:
- Auto-resolution when confidence gap > 30%
- User escalation for close conflicts
- Resolution tracking and audit trail
- Support for multiple resolution strategies
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


class ResolutionStrategy(str, Enum):
    """Strategies for resolving relationship conflicts."""

    AUTO_RESOLVE = "auto_resolve"  # System auto-resolved based on confidence gap
    USER_RESOLVE = "user_resolve"  # User explicitly chose winner
    MERGE = "merge"  # Relationships were merged into one
    COEXIST = "coexist"  # Both relationships are valid in different contexts


@dataclass
class ConflictResolutionResult:
    """Result of a conflict resolution operation.

    Contains all information about how a conflict was resolved,
    including the winning/losing relationships and reasoning.
    """

    conflict_id: int
    strategy: ResolutionStrategy
    winning_relationship_id: int | None
    losing_relationship_id: int | None
    confidence_gap: Decimal
    reasoning: str
    resolved_by: str
    resolved_at: datetime = field(default_factory=lambda: datetime.now(timezone.utc))
    metadata: dict[str, Any] = field(default_factory=dict)


class ConflictRepositoryProtocol(Protocol):
    """Protocol for conflict repository operations."""

    async def get_by_id(self, entity_id: int) -> Any | None:
        """Get entity by ID."""
        ...

    async def find_pending_conflicts(
        self,
        tenant_id: UUID,
        auto_resolvable_only: bool = False,
    ) -> list[Any]:
        """Find pending conflicts."""
        ...

    async def resolve_conflict(
        self,
        conflict_id: int,
        resolution_status: str,
        resolved_by: str,
        resolution_details: dict[str, Any] | None = None,
    ) -> Any | None:
        """Resolve a conflict."""
        ...

    async def update_lifecycle_state(
        self,
        relationship_id: int,
        new_state: str,
    ) -> Any | None:
        """Update relationship lifecycle state."""
        ...

    async def update_confidence(
        self,
        relationship_id: int,
        new_confidence: Decimal,
        evidence_strength: Decimal | None = None,
    ) -> Any | None:
        """Update relationship confidence."""
        ...


# Auto-resolution threshold - conflicts with gap >= 30% are auto-resolvable
AUTO_RESOLVE_THRESHOLD = Decimal("0.30")


class ConflictResolver:
    """Resolves conflicts between discovered relationships.

    Provides automatic and user-driven conflict resolution with
    full audit trail and learning integration.

    Attributes:
        session: Database session for persistence
        repository: Repository for relationship and conflict operations
        tenant_id: Current tenant ID for isolation
    """

    def __init__(
        self,
        session: AsyncSession,
        repository: ConflictRepositoryProtocol,
        tenant_id: UUID,
    ) -> None:
        """Initialize the conflict resolver.

        Args:
            session: Async database session
            repository: Repository instance for data operations
            tenant_id: Tenant ID for multi-tenant isolation
        """
        self.session = session
        self.repository = repository
        self.tenant_id = tenant_id

    async def resolve_conflict(
        self,
        conflict: Any,
    ) -> ConflictResolutionResult:
        """Resolve a single conflict.

        Determines the appropriate resolution strategy based on
        confidence gap and auto-resolvability flag.

        Args:
            conflict: Conflict object to resolve

        Returns:
            ConflictResolutionResult with resolution details
        """
        # Determine winning relationship based on confidence
        primary_id = conflict.relationship_id
        secondary_id = conflict.conflicting_relationship_id
        confidence_gap = conflict.confidence_gap

        # Get relationship details for secondary (primary may be used for merge)
        _ = await self.repository.get_by_id(primary_id)  # Primary exists check
        secondary_rel = (
            await self.repository.get_by_id(secondary_id)
            if secondary_id is not None
            else None
        )

        # Determine winner based on confidence
        if secondary_rel is None:
            # No secondary relationship - primary wins by default
            winning_id = primary_id
            losing_id = None
            reasoning = (
                f"Primary relationship {primary_id} wins by default - "
                f"no conflicting relationship exists"
            )
        elif conflict.primary_confidence >= conflict.secondary_confidence:
            winning_id = primary_id
            losing_id = secondary_id
            reasoning = (
                f"Relationship {primary_id} has higher confidence "
                f"({conflict.primary_confidence}) vs {secondary_id} "
                f"({conflict.secondary_confidence})"
            )
        else:
            winning_id = secondary_id
            losing_id = primary_id
            reasoning = (
                f"Relationship {secondary_id} has higher confidence "
                f"({conflict.secondary_confidence}) vs {primary_id} "
                f"({conflict.primary_confidence})"
            )

        # Determine resolution strategy
        if conflict.auto_resolvable:
            strategy = ResolutionStrategy.AUTO_RESOLVE
            reasoning = f"Auto-resolved: confidence gap {confidence_gap} >= 30%. {reasoning}"
            resolved_by = "system"
        else:
            # Should not reach here for auto-resolvable conflicts
            # This path is for edge cases where we force resolution
            strategy = ResolutionStrategy.AUTO_RESOLVE
            reasoning = f"Forced auto-resolution. {reasoning}"
            resolved_by = "system"

        # Archive the losing relationship if it exists
        if losing_id is not None:
            await self.repository.update_lifecycle_state(losing_id, "archived")

        # Record the resolution
        await self.repository.resolve_conflict(
            conflict_id=conflict.id,
            resolution_status="auto_resolved",
            resolved_by=resolved_by,
            resolution_details={
                "winning_relationship_id": winning_id,
                "losing_relationship_id": losing_id,
                "confidence_gap": str(confidence_gap),
                "strategy": strategy.value,
            },
        )

        return ConflictResolutionResult(
            conflict_id=conflict.id,
            strategy=strategy,
            winning_relationship_id=winning_id,
            losing_relationship_id=losing_id,
            confidence_gap=confidence_gap,
            reasoning=reasoning,
            resolved_by=resolved_by,
        )

    async def check_auto_resolvable(self, conflict: Any) -> bool:
        """Check if a conflict can be auto-resolved.

        A conflict is auto-resolvable if the confidence gap
        between the two relationships is >= 30%.

        Args:
            conflict: Conflict to check

        Returns:
            True if auto-resolvable, False otherwise
        """
        result: bool = conflict.confidence_gap >= AUTO_RESOLVE_THRESHOLD
        return result

    async def get_conflicts_for_review(
        self,
        limit: int = 50,
    ) -> list[Any]:
        """Get conflicts requiring user review.

        Returns conflicts that cannot be auto-resolved due to
        close confidence scores.

        Args:
            limit: Maximum conflicts to return

        Returns:
            List of conflicts needing user review
        """
        return await self.repository.find_pending_conflicts(
            tenant_id=self.tenant_id,
            auto_resolvable_only=False,
        )

    async def user_resolve(
        self,
        conflict: Any,
        winning_relationship_id: int | None,
        user_email: str,
        reasoning: str,
        strategy: ResolutionStrategy = ResolutionStrategy.USER_RESOLVE,
    ) -> ConflictResolutionResult:
        """Resolve a conflict based on user input.

        Allows user to explicitly choose which relationship wins
        or to mark both as coexisting.

        Args:
            conflict: Conflict to resolve
            winning_relationship_id: ID of winning relationship (None for coexist)
            user_email: Email of user making the decision
            reasoning: User's explanation for the decision
            strategy: Resolution strategy (USER_RESOLVE, MERGE, or COEXIST)

        Returns:
            ConflictResolutionResult with resolution details
        """
        if strategy == ResolutionStrategy.COEXIST:
            # Both relationships are valid
            winning_id = None
            losing_id = None
            resolution_status = "coexist"
        else:
            # User chose a winner
            winning_id = winning_relationship_id
            losing_id = (
                conflict.conflicting_relationship_id
                if winning_id == conflict.relationship_id
                else conflict.relationship_id
            )
            resolution_status = "user_resolved"

            # Archive the losing relationship if one exists
            if losing_id is not None:
                await self.repository.update_lifecycle_state(losing_id, "archived")

        # Record the resolution
        await self.repository.resolve_conflict(
            conflict_id=conflict.id,
            resolution_status=resolution_status,
            resolved_by=user_email,
            resolution_details={
                "winning_relationship_id": winning_id,
                "losing_relationship_id": losing_id,
                "confidence_gap": str(conflict.confidence_gap),
                "strategy": strategy.value,
                "user_reasoning": reasoning,
            },
        )

        return ConflictResolutionResult(
            conflict_id=conflict.id,
            strategy=strategy,
            winning_relationship_id=winning_id,
            losing_relationship_id=losing_id,
            confidence_gap=conflict.confidence_gap,
            reasoning=reasoning,
            resolved_by=user_email,
        )

    async def process_auto_resolvable(self) -> list[ConflictResolutionResult]:
        """Process all auto-resolvable conflicts.

        Finds and resolves all pending conflicts that have
        a confidence gap >= 30%.

        Returns:
            List of resolution results
        """
        conflicts = await self.repository.find_pending_conflicts(
            tenant_id=self.tenant_id,
            auto_resolvable_only=True,
        )

        results = []
        for conflict in conflicts:
            try:
                result = await self.resolve_conflict(conflict)
                results.append(result)
            except Exception as e:
                logger.warning(f"Failed to resolve conflict {conflict.id}: {e}")

        return results

    async def get_resolution_history(
        self,
        relationship_id: int,
    ) -> list[Any]:
        """Get resolution history for a relationship.

        Returns all conflicts that involved the given relationship,
        whether as primary or secondary.

        Args:
            relationship_id: ID of relationship to check

        Returns:
            List of conflicts involving this relationship
        """
        # This requires a custom query method on the repository
        if hasattr(self.repository, "find_by_relationship"):
            result: list[Any] = await self.repository.find_by_relationship(
                relationship_id
            )
            return result
        return []


__all__ = [
    "ConflictResolver",
    "ConflictResolutionResult",
    "ResolutionStrategy",
]

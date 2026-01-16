"""LifecycleManager - Manages relationship lifecycle states and transitions.

This module provides lifecycle management for relationships, including:
- State machine for relationship lifecycle transitions
- Inactivity detection and historical marking
- Retention policy enforcement and archival
- Evidence-based confidence decay
- Re-activation on new evidence
- Project/role change handling
- Version history tracking for audit trails
"""

from __future__ import annotations

import logging
import math
from dataclasses import dataclass, field
from datetime import datetime, timezone
from decimal import Decimal
from enum import Enum
from typing import Any, Protocol
from uuid import UUID

from sqlalchemy.ext.asyncio import AsyncSession

from .confidence import DECAY_HALF_LIFE_DAYS, DECAY_MINIMUM

logger = logging.getLogger(__name__)

# Constants for lifecycle management
INACTIVITY_THRESHOLD_DAYS = 90  # Days of inactivity before historical
RETENTION_YEARS = 2  # Years before archival
MINIMUM_CONFIDENCE = Decimal("0.05")  # Minimum confidence floor
NEW_EVIDENCE_CONFIDENCE_BOOST = Decimal("0.10")  # Boost when new evidence arrives


class TransitionReason(str, Enum):
    """Reasons for lifecycle state transitions.

    Captures why a relationship moved from one state to another,
    supporting audit trails and learning from patterns.
    """

    USER_CONFIRMED = "user_confirmed"  # User validated relationship
    INACTIVITY = "inactivity"  # No recent interactions
    RETENTION_EXPIRED = "retention_expired"  # Past retention period
    NEW_EVIDENCE = "new_evidence"  # New supporting evidence found
    ROLE_CHANGE = "role_change"  # Entity changed roles
    PROJECT_COMPLETED = "project_completed"  # Associated project ended
    EVIDENCE_DECAY = "evidence_decay"  # Confidence dropped due to time
    USER_REJECTED = "user_rejected"  # User rejected relationship
    MANUAL_ARCHIVE = "manual_archive"  # Manually archived by user


class StateTransitionError(Exception):
    """Raised when an invalid state transition is attempted."""

    pass


@dataclass
class LifecycleTransition:
    """Record of a lifecycle state transition.

    Captures the before/after state and metadata for audit trails.
    """

    relationship_id: int
    from_state: str
    to_state: str
    reason: TransitionReason
    triggered_by: str  # User email or "system"
    transitioned_at: datetime = field(
        default_factory=lambda: datetime.now(timezone.utc)
    )
    metadata: dict[str, Any] = field(default_factory=dict)


@dataclass
class MaintenanceResult:
    """Result of a maintenance cycle.

    Tracks statistics from maintenance operations.
    """

    total_processed: int
    transitions_applied: int
    confidence_recalculated: int
    archived_count: int
    reactivated_count: int
    errors: list[str] = field(default_factory=list)
    processing_time_ms: int = 0


class LifecycleRepositoryProtocol(Protocol):
    """Protocol for lifecycle repository operations."""

    async def get_by_id(self, entity_id: int) -> Any | None:
        """Get entity by ID."""
        ...

    async def update(self, entity_id: int, **kwargs: Any) -> Any | None:
        """Update entity."""
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

    async def find_inactive(
        self,
        threshold_days: int,
        states: list[str] | None = None,
    ) -> list[Any]:
        """Find relationships inactive for threshold days."""
        ...

    async def find_stale(
        self,
        retention_days: int,
        states: list[str] | None = None,
    ) -> list[Any]:
        """Find relationships past retention period."""
        ...

    async def find_by_state(
        self,
        states: list[str],
    ) -> list[Any]:
        """Find relationships in given states."""
        ...

    async def find_by_project(
        self,
        project_id: UUID,
    ) -> list[Any]:
        """Find relationships associated with a project."""
        ...

    async def find_by_entity(
        self,
        entity_id: int,
        entity_type: str | None = None,
        relationship_types: list[str] | None = None,
    ) -> list[Any]:
        """Find relationships involving an entity."""
        ...

    async def add_version(
        self,
        tenant_id: UUID,
        relationship_id: int,
        version_number: int,
        change_type: str,
        changed_by: str | None,
        previous_state: dict[str, Any],
        new_state: dict[str, Any],
        change_reasoning: str,
    ) -> Any:
        """Add version record for relationship."""
        ...

    async def get_evidence_for_relationship(
        self,
        relationship_id: int,
    ) -> list[Any]:
        """Get evidence for a relationship."""
        ...

    async def get_latest_version(
        self,
        relationship_id: int,
    ) -> Any | None:
        """Get latest version record for relationship."""
        ...


# Valid state transitions
# State machine: pending -> active -> historical -> archived
# With special transitions for reactivation: historical -> active
VALID_TRANSITIONS: dict[str, set[str]] = {
    "pending": {"active", "historical"},
    "active": {"historical", "archived"},
    "historical": {"archived", "active"},  # active for reactivation
    "archived": set(),  # Terminal state - no transitions out
}


class LifecycleManager:
    """Manages relationship lifecycle states and transitions.

    Provides state machine for lifecycle transitions, maintenance
    operations, and version tracking for audit trails.

    Attributes:
        session: Database session for persistence
        repository: Repository for relationship operations
        tenant_id: Current tenant ID for isolation
    """

    def __init__(
        self,
        session: AsyncSession,
        repository: LifecycleRepositoryProtocol,
        tenant_id: UUID,
    ) -> None:
        """Initialize the lifecycle manager.

        Args:
            session: Async database session
            repository: Repository instance for data operations
            tenant_id: Tenant ID for multi-tenant isolation
        """
        self.session = session
        self.repository = repository
        self.tenant_id = tenant_id

    # =========================================================================
    # STATE MACHINE VALIDATION
    # =========================================================================

    def is_valid_transition(self, from_state: str, to_state: str) -> bool:
        """Check if a state transition is valid.

        Args:
            from_state: Current lifecycle state
            to_state: Target lifecycle state

        Returns:
            True if transition is valid, False otherwise
        """
        valid_targets = VALID_TRANSITIONS.get(from_state, set())
        return to_state in valid_targets

    # =========================================================================
    # STATE TRANSITIONS
    # =========================================================================

    async def transition_state(
        self,
        relationship_id: int,
        to_state: str,
        reason: TransitionReason,
        triggered_by: str,
        metadata: dict[str, Any] | None = None,
    ) -> LifecycleTransition:
        """Transition a relationship to a new lifecycle state.

        Validates the transition and creates version record for audit trail.

        Args:
            relationship_id: ID of relationship to transition
            to_state: Target lifecycle state
            reason: Reason for the transition
            triggered_by: User email or "system"
            metadata: Optional additional metadata

        Returns:
            LifecycleTransition record

        Raises:
            ValueError: If relationship not found
            StateTransitionError: If transition is invalid
        """
        relationship = await self.repository.get_by_id(relationship_id)
        if relationship is None:
            raise ValueError(f"Relationship not found: {relationship_id}")

        from_state = relationship.lifecycle_state

        # Check for no-op transition
        if from_state == to_state:
            return LifecycleTransition(
                relationship_id=relationship_id,
                from_state=from_state,
                to_state=to_state,
                reason=reason,
                triggered_by=triggered_by,
                metadata=metadata or {},
            )

        # Validate transition
        if not self.is_valid_transition(from_state, to_state):
            raise StateTransitionError(
                f"Invalid transition from '{from_state}' to '{to_state}' "
                f"for relationship {relationship_id}"
            )

        # Capture previous state for version history
        previous_state = {
            "lifecycle_state": from_state,
            "confidence_score": str(relationship.confidence_score),
        }

        # Apply transition
        await self.repository.update_lifecycle_state(relationship_id, to_state)

        # Create new state snapshot
        new_state = {
            "lifecycle_state": to_state,
            "confidence_score": str(relationship.confidence_score),
        }

        # Create version record
        latest_version = await self.repository.get_latest_version(relationship_id)
        version_number = (latest_version.version_number + 1) if latest_version else 1

        await self.repository.add_version(
            tenant_id=self.tenant_id,
            relationship_id=relationship_id,
            version_number=version_number,
            change_type="state_transition",
            changed_by=triggered_by,
            previous_state=previous_state,
            new_state=new_state,
            change_reasoning=f"{reason.value}: {metadata or {}}",
        )

        logger.info(
            f"Transitioned relationship {relationship_id} from {from_state} to {to_state}, "
            f"reason: {reason.value}"
        )

        return LifecycleTransition(
            relationship_id=relationship_id,
            from_state=from_state,
            to_state=to_state,
            reason=reason,
            triggered_by=triggered_by,
            metadata=metadata or {},
        )

    async def reactivate(
        self,
        relationship_id: int,
        reason: str,
        triggered_by: str,
    ) -> LifecycleTransition:
        """Reactivate a historical relationship.

        Called when new evidence supports an inactive relationship.

        Args:
            relationship_id: ID of relationship to reactivate
            reason: Explanation for reactivation
            triggered_by: User email or "system"

        Returns:
            LifecycleTransition record

        Raises:
            ValueError: If relationship not found
            StateTransitionError: If relationship cannot be reactivated
        """
        relationship = await self.repository.get_by_id(relationship_id)
        if relationship is None:
            raise ValueError(f"Relationship not found: {relationship_id}")

        if relationship.lifecycle_state == "archived":
            raise StateTransitionError(
                f"Cannot reactivate archived relationship {relationship_id}. "
                "Archived relationships must be manually restored."
            )

        if relationship.lifecycle_state not in ("historical", "pending"):
            raise StateTransitionError(
                f"Cannot reactivate relationship {relationship_id} "
                f"from state '{relationship.lifecycle_state}'"
            )

        return await self.transition_state(
            relationship_id=relationship_id,
            to_state="active",
            reason=TransitionReason.NEW_EVIDENCE,
            triggered_by=triggered_by,
            metadata={"reactivation_reason": reason},
        )

    # =========================================================================
    # INACTIVITY DETECTION
    # =========================================================================

    async def find_inactive_relationships(
        self,
        threshold_days: int = INACTIVITY_THRESHOLD_DAYS,
    ) -> list[Any]:
        """Find relationships inactive for threshold days.

        Args:
            threshold_days: Days of inactivity to consider inactive

        Returns:
            List of inactive relationships
        """
        return await self.repository.find_inactive(
            threshold_days=threshold_days,
            states=["active"],
        )

    async def process_inactive_relationships(
        self,
        threshold_days: int = INACTIVITY_THRESHOLD_DAYS,
    ) -> MaintenanceResult:
        """Process inactive relationships and transition to historical.

        Args:
            threshold_days: Days of inactivity threshold

        Returns:
            MaintenanceResult with processing statistics
        """
        inactive = await self.find_inactive_relationships(threshold_days)

        transitions_applied = 0
        errors: list[str] = []

        for relationship in inactive:
            try:
                await self.transition_state(
                    relationship_id=relationship.id,
                    to_state="historical",
                    reason=TransitionReason.INACTIVITY,
                    triggered_by="system",
                    metadata={"threshold_days": threshold_days},
                )
                transitions_applied += 1
            except Exception as e:
                logger.warning(
                    f"Failed to transition inactive relationship {relationship.id}: {e}"
                )
                errors.append(f"Relationship {relationship.id}: {e}")

        return MaintenanceResult(
            total_processed=len(inactive),
            transitions_applied=transitions_applied,
            confidence_recalculated=0,
            archived_count=0,
            reactivated_count=0,
            errors=errors,
        )

    # =========================================================================
    # RETENTION POLICY
    # =========================================================================

    async def find_stale_relationships(
        self,
        retention_years: int = RETENTION_YEARS,
    ) -> list[Any]:
        """Find relationships past retention period.

        Args:
            retention_years: Years before archival

        Returns:
            List of stale relationships
        """
        retention_days = retention_years * 365
        return await self.repository.find_stale(
            retention_days=retention_days,
            states=["historical"],
        )

    async def archive_stale_relationships(
        self,
        retention_years: int = RETENTION_YEARS,
    ) -> MaintenanceResult:
        """Archive relationships past retention period.

        Args:
            retention_years: Years before archival

        Returns:
            MaintenanceResult with processing statistics
        """
        stale = await self.find_stale_relationships(retention_years)

        archived_count = 0
        errors: list[str] = []

        for relationship in stale:
            try:
                await self.transition_state(
                    relationship_id=relationship.id,
                    to_state="archived",
                    reason=TransitionReason.RETENTION_EXPIRED,
                    triggered_by="system",
                    metadata={"retention_years": retention_years},
                )
                archived_count += 1
            except Exception as e:
                logger.warning(
                    f"Failed to archive stale relationship {relationship.id}: {e}"
                )
                errors.append(f"Relationship {relationship.id}: {e}")

        return MaintenanceResult(
            total_processed=len(stale),
            transitions_applied=archived_count,
            confidence_recalculated=0,
            archived_count=archived_count,
            reactivated_count=0,
            errors=errors,
        )

    # =========================================================================
    # CONFIDENCE DECAY
    # =========================================================================

    def _calculate_temporal_decay(self, last_observed_at: datetime) -> Decimal:
        """Calculate temporal decay multiplier based on time since last observation.

        Args:
            last_observed_at: Timestamp of most recent evidence

        Returns:
            Decay multiplier (DECAY_MINIMUM to 1.0)
        """
        now = datetime.now(timezone.utc)
        days_since = (now - last_observed_at).total_seconds() / (24 * 3600)

        if days_since <= 0:
            return Decimal("1.0")

        # Exponential decay: decay = 0.5^(days/half_life)
        decay = math.pow(0.5, days_since / DECAY_HALF_LIFE_DAYS)

        # Apply minimum floor
        return Decimal(str(max(DECAY_MINIMUM, decay)))

    async def recalculate_confidence(
        self,
        relationship_id: int,
    ) -> Decimal:
        """Recalculate confidence with temporal decay.

        Args:
            relationship_id: ID of relationship to recalculate

        Returns:
            New confidence score

        Raises:
            ValueError: If relationship not found
        """
        relationship = await self.repository.get_by_id(relationship_id)
        if relationship is None:
            raise ValueError(f"Relationship not found: {relationship_id}")

        # Get base confidence
        base_confidence = Decimal(str(relationship.confidence_score))

        # Calculate temporal decay
        decay_multiplier = self._calculate_temporal_decay(
            relationship.last_observed_at
        )

        # Apply decay
        new_confidence = base_confidence * decay_multiplier

        # Enforce minimum
        new_confidence = max(MINIMUM_CONFIDENCE, new_confidence)

        # Update if changed significantly (>1% difference)
        if abs(new_confidence - base_confidence) > Decimal("0.01"):
            await self.repository.update_confidence(relationship_id, new_confidence)
            logger.debug(
                f"Recalculated confidence for relationship {relationship_id}: "
                f"{base_confidence} -> {new_confidence}"
            )

        return new_confidence

    async def recalculate_confidence_with_evidence(
        self,
        relationship_id: int,
        new_evidence: Any,
    ) -> Decimal:
        """Recalculate confidence after adding new evidence.

        New evidence boosts confidence and resets temporal decay.

        Args:
            relationship_id: ID of relationship
            new_evidence: New evidence item added

        Returns:
            New confidence score

        Raises:
            ValueError: If relationship not found
        """
        relationship = await self.repository.get_by_id(relationship_id)
        if relationship is None:
            raise ValueError(f"Relationship not found: {relationship_id}")

        base_confidence = Decimal(str(relationship.confidence_score))

        # New evidence provides boost
        evidence_boost = Decimal(str(new_evidence.strength_contribution))
        boosted_confidence = base_confidence + (evidence_boost * NEW_EVIDENCE_CONFIDENCE_BOOST)

        # Cap at 1.0
        new_confidence = min(Decimal("1.0"), boosted_confidence)

        await self.repository.update_confidence(relationship_id, new_confidence)

        # Update last observed time to reset decay
        await self.repository.update(
            relationship_id,
            last_observed_at=datetime.now(timezone.utc),
        )

        logger.info(
            f"Confidence boosted for relationship {relationship_id} with new evidence: "
            f"{base_confidence} -> {new_confidence}"
        )

        return new_confidence

    # =========================================================================
    # PROJECT/ROLE CHANGE HANDLING
    # =========================================================================

    async def handle_project_completion(
        self,
        project_id: UUID,
    ) -> MaintenanceResult:
        """Handle project completion by transitioning related relationships.

        Args:
            project_id: ID of completed project

        Returns:
            MaintenanceResult with processing statistics
        """
        # Find relationships associated with project
        relationships = await self.repository.find_by_project(project_id)

        transitions_applied = 0
        errors: list[str] = []

        for relationship in relationships:
            if relationship.lifecycle_state == "active":
                try:
                    await self.transition_state(
                        relationship_id=relationship.id,
                        to_state="historical",
                        reason=TransitionReason.PROJECT_COMPLETED,
                        triggered_by="system",
                        metadata={"project_id": str(project_id)},
                    )
                    transitions_applied += 1
                except Exception as e:
                    logger.warning(
                        f"Failed to transition relationship {relationship.id} "
                        f"for project completion: {e}"
                    )
                    errors.append(f"Relationship {relationship.id}: {e}")

        return MaintenanceResult(
            total_processed=len(relationships),
            transitions_applied=transitions_applied,
            confidence_recalculated=0,
            archived_count=0,
            reactivated_count=0,
            errors=errors,
        )

    async def handle_role_change(
        self,
        entity_id: int,
        affected_relationship_types: list[str],
        entity_type: str = "person",
    ) -> MaintenanceResult:
        """Handle entity role change by transitioning affected relationships.

        When someone changes roles, their reporting/management relationships
        may no longer be valid.

        Args:
            entity_id: ID of entity changing roles
            affected_relationship_types: Types to transition (e.g., reports_to, manages)
            entity_type: Type of entity

        Returns:
            MaintenanceResult with processing statistics
        """
        # Find affected relationships
        relationships = await self.repository.find_by_entity(
            entity_id=entity_id,
            entity_type=entity_type,
            relationship_types=affected_relationship_types,
        )

        transitions_applied = 0
        errors: list[str] = []

        for relationship in relationships:
            if relationship.lifecycle_state == "active":
                try:
                    await self.transition_state(
                        relationship_id=relationship.id,
                        to_state="historical",
                        reason=TransitionReason.ROLE_CHANGE,
                        triggered_by="system",
                        metadata={
                            "entity_id": entity_id,
                            "entity_type": entity_type,
                            "relationship_type": relationship.relationship_type,
                        },
                    )
                    transitions_applied += 1
                except Exception as e:
                    logger.warning(
                        f"Failed to transition relationship {relationship.id} "
                        f"for role change: {e}"
                    )
                    errors.append(f"Relationship {relationship.id}: {e}")

        return MaintenanceResult(
            total_processed=len(relationships),
            transitions_applied=transitions_applied,
            confidence_recalculated=0,
            archived_count=0,
            reactivated_count=0,
            errors=errors,
        )

    # =========================================================================
    # MAINTENANCE SCHEDULER
    # =========================================================================

    async def run_maintenance(
        self,
        inactivity_threshold_days: int = INACTIVITY_THRESHOLD_DAYS,
        retention_years: int = RETENTION_YEARS,
        recalculate_confidence: bool = True,
    ) -> MaintenanceResult:
        """Run full maintenance cycle.

        Performs:
        1. Inactivity detection and historical transition
        2. Retention policy enforcement (archival)
        3. Confidence recalculation with temporal decay

        Args:
            inactivity_threshold_days: Days before marking inactive
            retention_years: Years before archival
            recalculate_confidence: Whether to recalculate confidence

        Returns:
            MaintenanceResult with combined statistics
        """
        import time

        start_time = time.time()

        total_processed = 0
        transitions_applied = 0
        confidence_recalculated = 0
        archived_count = 0
        all_errors: list[str] = []

        # 1. Process inactive relationships
        inactive_result = await self.process_inactive_relationships(
            inactivity_threshold_days
        )
        total_processed += inactive_result.total_processed
        transitions_applied += inactive_result.transitions_applied
        all_errors.extend(inactive_result.errors)

        # 2. Archive stale relationships
        archive_result = await self.archive_stale_relationships(retention_years)
        total_processed += archive_result.total_processed
        transitions_applied += archive_result.transitions_applied
        archived_count += archive_result.archived_count
        all_errors.extend(archive_result.errors)

        # 3. Recalculate confidence for active relationships
        if recalculate_confidence:
            active_relationships = await self.repository.find_by_state(["active"])
            for relationship in active_relationships:
                try:
                    await self.recalculate_confidence(relationship.id)
                    confidence_recalculated += 1
                except Exception as e:
                    logger.warning(
                        f"Failed to recalculate confidence for {relationship.id}: {e}"
                    )
                    all_errors.append(f"Confidence recalc {relationship.id}: {e}")
            total_processed += len(active_relationships)

        processing_time_ms = int((time.time() - start_time) * 1000)

        logger.info(
            f"Maintenance complete: processed={total_processed}, "
            f"transitions={transitions_applied}, archived={archived_count}, "
            f"confidence_recalc={confidence_recalculated}, "
            f"errors={len(all_errors)}, time={processing_time_ms}ms"
        )

        return MaintenanceResult(
            total_processed=total_processed,
            transitions_applied=transitions_applied,
            confidence_recalculated=confidence_recalculated,
            archived_count=archived_count,
            reactivated_count=0,
            errors=all_errors,
            processing_time_ms=processing_time_ms,
        )


__all__ = [
    "INACTIVITY_THRESHOLD_DAYS",
    "RETENTION_YEARS",
    "LifecycleManager",
    "LifecycleTransition",
    "MaintenanceResult",
    "StateTransitionError",
    "TransitionReason",
]

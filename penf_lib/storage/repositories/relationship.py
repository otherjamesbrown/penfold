"""Relationship repository with specialized operations.

This module provides the data access layer for relationship discovery
and management, including CRUD operations, queries by entity, lifecycle
state management, evidence tracking, feedback collection, and conflict resolution.
"""

from __future__ import annotations

import uuid
from datetime import datetime, timedelta, timezone
from decimal import Decimal
from typing import Any

from sqlalchemy import and_, desc, func, select
from sqlalchemy.ext.asyncio import AsyncSession

from penf_lib.storage.models import (
    Relationship,
    RelationshipConflict,
    RelationshipEvidence,
    RelationshipFeedback,
)

from .base import BaseRepository


class RelationshipRepository(BaseRepository[Relationship]):
    """Repository for Relationship entities with specialized operations.

    Provides methods for:
    - Creating and managing relationships
    - Querying by source/target entity
    - Filtering by lifecycle state and confidence
    - Managing evidence, feedback, versions, and conflicts
    """

    def __init__(self, session: AsyncSession):
        """Initialize relationship repository.

        Args:
            session: Async SQLAlchemy session
        """
        super().__init__(session, Relationship)

    # =========================================================================
    # RELATIONSHIP CRUD OPERATIONS
    # =========================================================================

    async def create_relationship(
        self,
        tenant_id: uuid.UUID,
        source_entity_type: str,
        source_entity_id: int,
        target_entity_type: str,
        target_entity_id: int,
        relationship_type: str,
        confidence_score: Decimal,
        relationship_subtype: str | None = None,
        ai_confidence: Decimal | None = None,
        evidence_strength: Decimal | None = None,
        lifecycle_state: str = "pending",
        last_evidence_at: datetime | None = None,
    ) -> Relationship:
        """Create a new relationship.

        Args:
            tenant_id: Tenant ID for isolation
            source_entity_type: Type of source entity (person, project, topic)
            source_entity_id: ID of source entity
            target_entity_type: Type of target entity
            target_entity_id: ID of target entity
            relationship_type: Classification of relationship
            confidence_score: Overall confidence (0.0-1.0)
            relationship_subtype: Optional finer classification
            ai_confidence: AI extraction confidence
            evidence_strength: Evidence-based strength
            lifecycle_state: Initial state (default: pending)
            last_evidence_at: Timestamp of most recent evidence

        Returns:
            Created relationship instance
        """
        now = datetime.now(timezone.utc)
        return await self.create(
            tenant_id=tenant_id,
            source_entity_type=source_entity_type,
            source_entity_id=source_entity_id,
            target_entity_type=target_entity_type,
            target_entity_id=target_entity_id,
            relationship_type=relationship_type,
            relationship_subtype=relationship_subtype,
            confidence_score=confidence_score,
            ai_confidence=ai_confidence,
            evidence_strength=evidence_strength,
            lifecycle_state=lifecycle_state,
            first_observed_at=now,
            last_evidence_at=last_evidence_at or now,
        )

    # =========================================================================
    # ENTITY-BASED QUERIES
    # =========================================================================

    async def find_by_source_entity(
        self,
        tenant_id: uuid.UUID,
        entity_type: str,
        entity_id: int,
        relationship_type: str | None = None,
    ) -> list[Relationship]:
        """Find relationships by source entity.

        Args:
            tenant_id: Tenant ID
            entity_type: Type of source entity
            entity_id: ID of source entity
            relationship_type: Optional filter by type

        Returns:
            List of matching relationships
        """
        conditions = [
            Relationship.tenant_id == tenant_id,
            Relationship.source_entity_type == entity_type,
            Relationship.source_entity_id == entity_id,
        ]

        if relationship_type:
            conditions.append(Relationship.relationship_type == relationship_type)

        stmt = self._build_query().where(and_(*conditions)).order_by(
            desc(Relationship.confidence_score)
        )

        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    async def find_by_target_entity(
        self,
        tenant_id: uuid.UUID,
        entity_type: str,
        entity_id: int,
        relationship_type: str | None = None,
    ) -> list[Relationship]:
        """Find relationships by target entity.

        Args:
            tenant_id: Tenant ID
            entity_type: Type of target entity
            entity_id: ID of target entity
            relationship_type: Optional filter by type

        Returns:
            List of matching relationships
        """
        conditions = [
            Relationship.tenant_id == tenant_id,
            Relationship.target_entity_type == entity_type,
            Relationship.target_entity_id == entity_id,
        ]

        if relationship_type:
            conditions.append(Relationship.relationship_type == relationship_type)

        stmt = self._build_query().where(and_(*conditions)).order_by(
            desc(Relationship.confidence_score)
        )

        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    async def find_by_entity_pair(
        self,
        tenant_id: uuid.UUID,
        source_type: str,
        source_id: int,
        target_type: str,
        target_id: int,
        relationship_type: str | None = None,
    ) -> list[Relationship]:
        """Find relationships between specific entity pair.

        Args:
            tenant_id: Tenant ID
            source_type: Source entity type
            source_id: Source entity ID
            target_type: Target entity type
            target_id: Target entity ID
            relationship_type: Optional filter by type

        Returns:
            List of matching relationships
        """
        conditions = [
            Relationship.tenant_id == tenant_id,
            Relationship.source_entity_type == source_type,
            Relationship.source_entity_id == source_id,
            Relationship.target_entity_type == target_type,
            Relationship.target_entity_id == target_id,
        ]

        if relationship_type:
            conditions.append(Relationship.relationship_type == relationship_type)

        stmt = self._build_query().where(and_(*conditions))

        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    # =========================================================================
    # LIFECYCLE AND CONFIDENCE QUERIES
    # =========================================================================

    async def find_by_lifecycle_state(
        self,
        tenant_id: uuid.UUID,
        state: str,
        limit: int = 100,
    ) -> list[Relationship]:
        """Find relationships by lifecycle state.

        Args:
            tenant_id: Tenant ID
            state: Lifecycle state (pending, active, historical, archived)
            limit: Maximum results

        Returns:
            List of matching relationships
        """
        stmt = (
            self._build_query()
            .where(
                and_(
                    Relationship.tenant_id == tenant_id,
                    Relationship.lifecycle_state == state,
                )
            )
            .order_by(desc(Relationship.last_evidence_at))
            .limit(limit)
        )

        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    async def find_by_confidence_threshold(
        self,
        tenant_id: uuid.UUID,
        min_confidence: Decimal,
        lifecycle_state: str | None = None,
        limit: int = 100,
    ) -> list[Relationship]:
        """Find relationships meeting confidence threshold.

        Args:
            tenant_id: Tenant ID
            min_confidence: Minimum confidence score
            lifecycle_state: Optional filter by state
            limit: Maximum results

        Returns:
            List of high-confidence relationships
        """
        conditions = [
            Relationship.tenant_id == tenant_id,
            Relationship.confidence_score >= min_confidence,
        ]

        if lifecycle_state:
            conditions.append(Relationship.lifecycle_state == lifecycle_state)

        stmt = (
            self._build_query()
            .where(and_(*conditions))
            .order_by(desc(Relationship.confidence_score))
            .limit(limit)
        )

        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    async def find_stale_relationships(
        self,
        tenant_id: uuid.UUID,
        days_threshold: int = 90,
        lifecycle_state: str = "active",
    ) -> list[Relationship]:
        """Find relationships with no recent evidence.

        Args:
            tenant_id: Tenant ID
            days_threshold: Days since last evidence
            lifecycle_state: State to filter (default: active)

        Returns:
            List of stale relationships
        """
        cutoff = datetime.now(timezone.utc) - timedelta(days=days_threshold)

        stmt = (
            self._build_query()
            .where(
                and_(
                    Relationship.tenant_id == tenant_id,
                    Relationship.lifecycle_state == lifecycle_state,
                    Relationship.last_evidence_at < cutoff,
                )
            )
            .order_by(Relationship.last_evidence_at)
        )

        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    # =========================================================================
    # RELATIONSHIP UPDATES
    # =========================================================================

    async def update_confidence(
        self,
        relationship_id: int,
        new_confidence: Decimal,
        evidence_strength: Decimal | None = None,
    ) -> Relationship | None:
        """Update relationship confidence score.

        Args:
            relationship_id: Relationship ID
            new_confidence: New confidence score
            evidence_strength: Optional evidence strength update

        Returns:
            Updated relationship or None
        """
        updates: dict[str, Any] = {"confidence_score": new_confidence}
        if evidence_strength is not None:
            updates["evidence_strength"] = evidence_strength

        return await self.update(relationship_id, **updates)

    async def update_lifecycle_state(
        self,
        relationship_id: int,
        new_state: str,
    ) -> Relationship | None:
        """Update relationship lifecycle state.

        Args:
            relationship_id: Relationship ID
            new_state: New lifecycle state

        Returns:
            Updated relationship or None
        """
        updates: dict[str, Any] = {"lifecycle_state": new_state}

        if new_state == "archived":
            updates["archived_at"] = datetime.now(timezone.utc)

        return await self.update(relationship_id, **updates)

    async def confirm_relationship(
        self,
        relationship_id: int,
        user_email: str,  # noqa: ARG002
    ) -> Relationship | None:
        """Confirm a relationship via user validation.

        Args:
            relationship_id: Relationship ID
            user_email: Email of confirming user

        Returns:
            Updated relationship or None
        """
        return await self.update(
            relationship_id,
            user_confirmed=True,
            lifecycle_state="active",
        )

    async def increment_interaction_count(
        self,
        relationship_id: int,
    ) -> Relationship | None:
        """Increment relationship interaction count.

        Args:
            relationship_id: Relationship ID

        Returns:
            Updated relationship or None
        """
        relationship = await self.get_by_id(relationship_id)
        if not relationship:
            return None

        return await self.update(
            relationship_id,
            interaction_count=relationship.interaction_count + 1,
            last_evidence_at=datetime.now(timezone.utc),
        )

    async def archive_relationship(
        self,
        relationship_id: int,
    ) -> Relationship | None:
        """Archive a relationship.

        Args:
            relationship_id: Relationship ID

        Returns:
            Archived relationship or None
        """
        return await self.update_lifecycle_state(relationship_id, "archived")

    # =========================================================================
    # STATISTICS
    # =========================================================================

    async def get_relationship_stats(
        self,
        tenant_id: uuid.UUID,
    ) -> dict[str, int]:
        """Get relationship statistics for tenant.

        Args:
            tenant_id: Tenant ID

        Returns:
            Dictionary with statistics
        """
        # Total count
        total_stmt = (
            select(func.count(Relationship.id))
            .where(Relationship.tenant_id == tenant_id)
        )

        # Active count
        active_stmt = (
            select(func.count(Relationship.id))
            .where(
                and_(
                    Relationship.tenant_id == tenant_id,
                    Relationship.lifecycle_state == "active",
                )
            )
        )

        # Pending count
        pending_stmt = (
            select(func.count(Relationship.id))
            .where(
                and_(
                    Relationship.tenant_id == tenant_id,
                    Relationship.lifecycle_state == "pending",
                )
            )
        )

        total_result = await self.session.execute(total_stmt)
        active_result = await self.session.execute(active_stmt)
        pending_result = await self.session.execute(pending_stmt)

        return {
            "total_relationships": total_result.scalar() or 0,
            "active_relationships": active_result.scalar() or 0,
            "pending_relationships": pending_result.scalar() or 0,
        }

    # =========================================================================
    # EVIDENCE OPERATIONS
    # =========================================================================

    async def add_evidence(
        self,
        tenant_id: uuid.UUID,
        relationship_id: int,
        evidence_type: str,
        strength_contribution: Decimal,
        evidence_content: str | None = None,
        source_id: int | None = None,
        evidence_context: dict[str, Any] | None = None,
    ) -> RelationshipEvidence:
        """Add evidence to a relationship.

        Args:
            tenant_id: Tenant ID
            relationship_id: Parent relationship ID
            evidence_type: Type of evidence
            strength_contribution: Confidence contribution
            evidence_content: Extracted text
            source_id: Reference to source content
            evidence_context: Additional context

        Returns:
            Created evidence instance
        """
        evidence = RelationshipEvidence(
            tenant_id=tenant_id,
            relationship_id=relationship_id,
            evidence_type=evidence_type,
            strength_contribution=strength_contribution,
            evidence_content=evidence_content,
            source_id=source_id,
            evidence_context=evidence_context or {},
        )
        self.session.add(evidence)
        await self.session.flush()
        return evidence

    async def get_evidence_for_relationship(
        self,
        relationship_id: int,
    ) -> list[RelationshipEvidence]:
        """Get all evidence for a relationship.

        Args:
            relationship_id: Relationship ID

        Returns:
            List of evidence items
        """
        stmt = (
            select(RelationshipEvidence)
            .where(RelationshipEvidence.relationship_id == relationship_id)
            .order_by(desc(RelationshipEvidence.observed_at))
        )

        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    # =========================================================================
    # FEEDBACK OPERATIONS
    # =========================================================================

    async def add_feedback(
        self,
        tenant_id: uuid.UUID,
        relationship_id: int,
        feedback_type: str,
        user_email: str,
        corrected_type: str | None = None,
        reasoning: str | None = None,
        feedback_metadata: dict[str, Any] | None = None,
    ) -> RelationshipFeedback:
        """Add user feedback for a relationship.

        Args:
            tenant_id: Tenant ID
            relationship_id: Target relationship ID
            feedback_type: Type of feedback (confirm, reject, modify)
            user_email: User providing feedback
            corrected_type: User-corrected type (for modify)
            reasoning: User explanation
            feedback_metadata: Additional data

        Returns:
            Created feedback instance
        """
        # Get original relationship for recording original values
        relationship = await self.get_by_id(relationship_id)
        original_type = relationship.relationship_type if relationship else None
        original_confidence = relationship.confidence_score if relationship else None

        feedback = RelationshipFeedback(
            tenant_id=tenant_id,
            relationship_id=relationship_id,
            feedback_type=feedback_type,
            user_email=user_email,
            original_type=original_type,
            corrected_type=corrected_type,
            original_confidence=original_confidence,
            reasoning=reasoning,
            feedback_metadata=feedback_metadata or {},
        )
        self.session.add(feedback)
        await self.session.flush()
        return feedback

    async def get_feedback_for_relationship(
        self,
        relationship_id: int,
    ) -> list[RelationshipFeedback]:
        """Get all feedback for a relationship.

        Args:
            relationship_id: Relationship ID

        Returns:
            List of feedback items
        """
        stmt = (
            select(RelationshipFeedback)
            .where(RelationshipFeedback.relationship_id == relationship_id)
            .order_by(desc(RelationshipFeedback.created_at))
        )

        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    # =========================================================================
    # CONFLICT OPERATIONS
    # =========================================================================

    async def create_conflict(
        self,
        tenant_id: uuid.UUID,
        relationship_id: int,
        conflicting_relationship_id: int | None,
        conflict_type: str,
        primary_confidence: Decimal,
        secondary_confidence: Decimal | None = None,
    ) -> RelationshipConflict:
        """Create a relationship conflict.

        Args:
            tenant_id: Tenant ID
            relationship_id: Primary relationship ID
            conflicting_relationship_id: Competing relationship ID
            conflict_type: Type of conflict
            primary_confidence: Primary relationship confidence
            secondary_confidence: Conflicting relationship confidence

        Returns:
            Created conflict instance
        """
        # Calculate confidence gap
        if secondary_confidence is not None:
            confidence_gap = abs(primary_confidence - secondary_confidence)
        else:
            confidence_gap = Decimal("0.000")

        # Auto-resolvable if gap > 30%
        auto_resolvable = confidence_gap >= Decimal("0.30")

        conflict = RelationshipConflict(
            tenant_id=tenant_id,
            relationship_id=relationship_id,
            conflicting_relationship_id=conflicting_relationship_id,
            conflict_type=conflict_type,
            primary_confidence=primary_confidence,
            secondary_confidence=secondary_confidence,
            confidence_gap=confidence_gap,
            auto_resolvable=auto_resolvable,
            resolution_status="pending",
        )
        self.session.add(conflict)
        await self.session.flush()
        return conflict

    async def find_pending_conflicts(
        self,
        tenant_id: uuid.UUID,
        auto_resolvable_only: bool = False,
    ) -> list[RelationshipConflict]:
        """Find pending conflicts.

        Args:
            tenant_id: Tenant ID
            auto_resolvable_only: Filter to auto-resolvable only

        Returns:
            List of pending conflicts
        """
        conditions = [
            RelationshipConflict.tenant_id == tenant_id,
            RelationshipConflict.resolution_status == "pending",
        ]

        if auto_resolvable_only:
            conditions.append(RelationshipConflict.auto_resolvable.is_(True))

        stmt = (
            select(RelationshipConflict)
            .where(and_(*conditions))
            .order_by(desc(RelationshipConflict.confidence_gap))
        )

        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    async def resolve_conflict(
        self,
        conflict_id: int,
        resolution_status: str,
        resolved_by: str,
        resolution_details: dict[str, Any] | None = None,
    ) -> RelationshipConflict | None:
        """Resolve a conflict.

        Args:
            conflict_id: Conflict ID
            resolution_status: Resolution status
            resolved_by: Who resolved (user email or 'system')
            resolution_details: Resolution information

        Returns:
            Resolved conflict or None
        """
        stmt = (
            select(RelationshipConflict)
            .where(RelationshipConflict.id == conflict_id)
        )
        result = await self.session.execute(stmt)
        conflict = result.scalar_one_or_none()

        if not conflict:
            return None

        conflict.resolution_status = resolution_status
        conflict.resolved_by = resolved_by
        conflict.resolved_at = datetime.now(timezone.utc)
        if resolution_details:
            conflict.resolution_details = resolution_details

        await self.session.flush()
        return conflict

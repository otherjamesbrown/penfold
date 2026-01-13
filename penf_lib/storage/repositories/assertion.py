"""Assertion repository with specialized operations."""

import uuid
from typing import List, Optional, Dict, Any
from decimal import Decimal
from datetime import datetime

from sqlalchemy import select, and_, or_, func, desc
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.orm import selectinload, joinedload

from penf_lib.storage.models import Assertion, Source
from .base import BaseRepository


class AssertionRepository(BaseRepository[Assertion]):
    """Repository for Assertion entities."""

    def __init__(self, session: AsyncSession):
        """Initialize assertion repository.

        Args:
            session: Async SQLAlchemy session
        """
        super().__init__(session, Assertion)

    async def create_assertion(
        self,
        tenant_id: uuid.UUID,
        source_id: int,
        assertion_type: str,
        content: str,
        confidence_score: Optional[Decimal] = None,
        extracted_entities: Optional[Dict[str, Any]] = None,
        temporal_context: Optional[Dict[str, Any]] = None,
        validation_status: Optional[str] = None,
    ) -> Assertion:
        """Create a new assertion with validation.

        Args:
            tenant_id: Tenant ID
            source_id: Source ID
            assertion_type: Type of assertion
            content: Assertion content
            confidence_score: Confidence score (0.000-1.000)
            extracted_entities: Extracted entity data
            temporal_context: Temporal context information
            validation_status: Validation status

        Returns:
            Created assertion instance
        """
        return await self.create(
            tenant_id=tenant_id,
            source_id=source_id,
            assertion_type=assertion_type,
            content=content,
            confidence_score=confidence_score,
            extracted_entities=extracted_entities,
            temporal_context=temporal_context,
            validation_status=validation_status,
        )

    async def find_by_source(self, source_id: int) -> List[Assertion]:
        """Find assertions by source ID.

        Args:
            source_id: Source ID

        Returns:
            List of assertion instances
        """
        return await self.find_by(source_id=source_id)

    async def find_by_type(
        self, tenant_id: uuid.UUID, assertion_type: str
    ) -> List[Assertion]:
        """Find assertions by type.

        Args:
            tenant_id: Tenant ID
            assertion_type: Assertion type

        Returns:
            List of assertion instances
        """
        return await self.find_by(
            tenant_id=tenant_id,
            assertion_type=assertion_type
        )

    async def find_by_validation_status(
        self, tenant_id: uuid.UUID, validation_status: str
    ) -> List[Assertion]:
        """Find assertions by validation status.

        Args:
            tenant_id: Tenant ID
            validation_status: Validation status

        Returns:
            List of assertion instances
        """
        return await self.find_by(
            tenant_id=tenant_id,
            validation_status=validation_status
        )

    async def find_pending_validation(self, tenant_id: uuid.UUID) -> List[Assertion]:
        """Find assertions pending validation.

        Args:
            tenant_id: Tenant ID

        Returns:
            List of pending assertion instances
        """
        return await self.find_by_validation_status(tenant_id, 'pending')

    async def find_high_confidence(
        self, tenant_id: uuid.UUID, min_confidence: Decimal = Decimal('0.8')
    ) -> List[Assertion]:
        """Find high-confidence assertions.

        Args:
            tenant_id: Tenant ID
            min_confidence: Minimum confidence threshold

        Returns:
            List of high-confidence assertion instances
        """
        stmt = (
            self._build_query()
            .where(
                and_(
                    Assertion.tenant_id == tenant_id,
                    Assertion.confidence_score >= min_confidence
                )
            )
            .order_by(desc(Assertion.confidence_score))
        )

        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    async def find_with_source(self, assertion_id: int) -> Optional[Assertion]:
        """Find assertion with loaded source.

        Args:
            assertion_id: Assertion ID

        Returns:
            Assertion instance with source loaded, or None
        """
        stmt = (
            select(Assertion)
            .options(joinedload(Assertion.source))
            .where(Assertion.id == assertion_id)
        )

        if hasattr(Assertion, 'is_deleted'):
            stmt = stmt.where(Assertion.is_deleted.is_(False))

        result = await self.session.execute(stmt)
        return result.scalar_one_or_none()

    async def find_by_entity_mention(
        self, tenant_id: uuid.UUID, entity_type: str, entity_value: str
    ) -> List[Assertion]:
        """Find assertions mentioning a specific entity.

        Args:
            tenant_id: Tenant ID
            entity_type: Type of entity (person, project, etc.)
            entity_value: Entity value to search for

        Returns:
            List of assertion instances mentioning the entity
        """
        # Search in extracted_entities JSONB field
        stmt = (
            self._build_query()
            .where(
                and_(
                    Assertion.tenant_id == tenant_id,
                    Assertion.extracted_entities[entity_type].astext.ilike(f'%{entity_value}%')
                )
            )
            .order_by(desc(Assertion.confidence_score))
        )

        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    async def update_validation_status(
        self, assertion_id: int, validation_status: str
    ) -> Optional[Assertion]:
        """Update assertion validation status.

        Args:
            assertion_id: Assertion ID
            validation_status: New validation status

        Returns:
            Updated assertion instance or None if not found
        """
        return await self.update(assertion_id, validation_status=validation_status)

    async def bulk_update_validation(
        self, assertion_ids: List[int], validation_status: str
    ) -> int:
        """Bulk update validation status for multiple assertions.

        Args:
            assertion_ids: List of assertion IDs
            validation_status: New validation status

        Returns:
            Number of assertions updated
        """
        from sqlalchemy import update

        stmt = (
            update(Assertion)
            .where(Assertion.id.in_(assertion_ids))
            .values(
                validation_status=validation_status,
                updated_at=datetime.utcnow()
            )
        )

        # Filter out soft-deleted entities
        if hasattr(Assertion, 'is_deleted'):
            stmt = stmt.where(Assertion.is_deleted.is_(False))

        result = await self.session.execute(stmt)
        return result.rowcount

    async def get_stats_by_type(self, tenant_id: uuid.UUID) -> Dict[str, int]:
        """Get assertion count statistics by type.

        Args:
            tenant_id: Tenant ID

        Returns:
            Dictionary mapping assertion types to counts
        """
        stmt = (
            select(Assertion.assertion_type, func.count(Assertion.id))
            .where(
                and_(
                    Assertion.tenant_id == tenant_id,
                    Assertion.is_deleted.is_(False)
                )
            )
            .group_by(Assertion.assertion_type)
        )

        result = await self.session.execute(stmt)
        return dict(result.all())

    async def get_confidence_distribution(
        self, tenant_id: uuid.UUID
    ) -> Dict[str, int]:
        """Get confidence score distribution.

        Args:
            tenant_id: Tenant ID

        Returns:
            Dictionary with confidence ranges and counts
        """
        stmt = (
            select(
                func.count(Assertion.id),
                func.case(
                    (Assertion.confidence_score >= 0.9, 'very_high'),
                    (Assertion.confidence_score >= 0.7, 'high'),
                    (Assertion.confidence_score >= 0.5, 'medium'),
                    (Assertion.confidence_score >= 0.3, 'low'),
                    else_='very_low'
                ).label('confidence_range')
            )
            .where(
                and_(
                    Assertion.tenant_id == tenant_id,
                    Assertion.is_deleted.is_(False),
                    Assertion.confidence_score.isnot(None)
                )
            )
            .group_by('confidence_range')
        )

        result = await self.session.execute(stmt)
        return {range_name: count for count, range_name in result.all()}

    async def search_content(
        self, tenant_id: uuid.UUID, search_term: str, limit: int = 50
    ) -> List[Assertion]:
        """Search assertion content.

        Args:
            tenant_id: Tenant ID
            search_term: Search term
            limit: Maximum results to return

        Returns:
            List of matching assertion instances
        """
        stmt = (
            self._build_query()
            .where(
                and_(
                    Assertion.tenant_id == tenant_id,
                    Assertion.content.ilike(f'%{search_term}%')
                )
            )
            .order_by(desc(Assertion.confidence_score))
            .limit(limit)
        )

        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    async def find_by_date_range(
        self,
        tenant_id: uuid.UUID,
        start_date: Optional[datetime] = None,
        end_date: Optional[datetime] = None,
        assertion_type: Optional[str] = None
    ) -> List[Assertion]:
        """Find assertions by date range.

        Args:
            tenant_id: Tenant ID
            start_date: Start date (inclusive)
            end_date: End date (inclusive)
            assertion_type: Optional assertion type filter

        Returns:
            List of assertion instances
        """
        stmt = self._build_query().where(Assertion.tenant_id == tenant_id)

        if start_date:
            stmt = stmt.where(Assertion.created_at >= start_date)
        if end_date:
            stmt = stmt.where(Assertion.created_at <= end_date)
        if assertion_type:
            stmt = stmt.where(Assertion.assertion_type == assertion_type)

        stmt = stmt.order_by(desc(Assertion.created_at))

        result = await self.session.execute(stmt)
        return list(result.scalars().all())
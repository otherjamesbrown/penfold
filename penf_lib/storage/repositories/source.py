"""Source repository with specialized operations."""

import uuid
from typing import List, Optional, Dict, Any
from datetime import datetime

from sqlalchemy import select, and_, or_, func, text
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.orm import selectinload

from penf_lib.storage.models import Source
from penf_lib.storage.validators import generate_content_hash
from .base import BaseRepository


class SourceRepository(BaseRepository[Source]):
    """Repository for Source entities."""

    def __init__(self, session: AsyncSession):
        """Initialize source repository.

        Args:
            session: Async SQLAlchemy session
        """
        super().__init__(session, Source)

    async def create_source(
        self,
        tenant_id: uuid.UUID,
        source_system: str,
        external_id: str,
        raw_content: Optional[str] = None,
        content_type: Optional[str] = None,
        ingestion_metadata: Optional[Dict[str, Any]] = None,
        source_timestamp: Optional[datetime] = None,
    ) -> Source:
        """Create a new source with content hash generation.

        Args:
            tenant_id: Tenant ID
            source_system: Source system identifier
            external_id: External system ID
            raw_content: Raw content from source
            content_type: MIME type or content type
            ingestion_metadata: Additional metadata
            source_timestamp: Original timestamp from source

        Returns:
            Created source instance
        """
        content_hash = generate_content_hash(raw_content or "")

        return await self.create(
            tenant_id=tenant_id,
            source_system=source_system,
            external_id=external_id,
            content_hash=content_hash,
            raw_content=raw_content,
            content_type=content_type,
            content_size=len(raw_content or ""),
            ingestion_metadata=ingestion_metadata,
            source_timestamp=source_timestamp,
        )

    async def find_by_external_id(
        self, tenant_id: uuid.UUID, source_system: str, external_id: str
    ) -> Optional[Source]:
        """Find source by external ID.

        Args:
            tenant_id: Tenant ID
            source_system: Source system identifier
            external_id: External system ID

        Returns:
            Source instance or None if not found
        """
        return await self.find_one_by(
            tenant_id=tenant_id,
            source_system=source_system,
            external_id=external_id
        )

    async def find_by_content_hash(
        self, tenant_id: uuid.UUID, content_hash: str
    ) -> Optional[Source]:
        """Find source by content hash.

        Args:
            tenant_id: Tenant ID
            content_hash: SHA-256 content hash

        Returns:
            Source instance or None if not found
        """
        return await self.find_one_by(
            tenant_id=tenant_id,
            content_hash=content_hash
        )

    async def find_by_system(
        self, tenant_id: uuid.UUID, source_system: str, limit: Optional[int] = None
    ) -> List[Source]:
        """Find sources by system.

        Args:
            tenant_id: Tenant ID
            source_system: Source system identifier
            limit: Maximum number of sources to return

        Returns:
            List of source instances
        """
        stmt = self._build_query().where(
            and_(
                Source.tenant_id == tenant_id,
                Source.source_system == source_system
            )
        ).order_by(Source.created_at.desc())

        if limit:
            stmt = stmt.limit(limit)

        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    async def find_by_status(
        self, tenant_id: uuid.UUID, processing_status: str
    ) -> List[Source]:
        """Find sources by processing status.

        Args:
            tenant_id: Tenant ID
            processing_status: Processing status

        Returns:
            List of source instances
        """
        return await self.find_by(
            tenant_id=tenant_id,
            processing_status=processing_status
        )

    async def find_pending_processing(self, tenant_id: uuid.UUID) -> List[Source]:
        """Find sources pending processing.

        Args:
            tenant_id: Tenant ID

        Returns:
            List of pending source instances
        """
        return await self.find_by_status(tenant_id, 'pending')

    async def update_processing_status(
        self, source_id: int, status: str, metadata: Optional[Dict[str, Any]] = None
    ) -> Optional[Source]:
        """Update processing status and metadata.

        Args:
            source_id: Source ID
            status: New processing status
            metadata: Additional processing metadata

        Returns:
            Updated source instance or None if not found
        """
        update_data = {'processing_status': status}
        if metadata:
            update_data['ingestion_metadata'] = metadata

        return await self.update(source_id, **update_data)

    async def find_by_date_range(
        self,
        tenant_id: uuid.UUID,
        start_date: Optional[datetime] = None,
        end_date: Optional[datetime] = None,
        source_system: Optional[str] = None
    ) -> List[Source]:
        """Find sources by date range.

        Args:
            tenant_id: Tenant ID
            start_date: Start date (inclusive)
            end_date: End date (inclusive)
            source_system: Optional source system filter

        Returns:
            List of source instances
        """
        stmt = self._build_query().where(Source.tenant_id == tenant_id)

        if start_date:
            stmt = stmt.where(Source.source_timestamp >= start_date)
        if end_date:
            stmt = stmt.where(Source.source_timestamp <= end_date)
        if source_system:
            stmt = stmt.where(Source.source_system == source_system)

        stmt = stmt.order_by(Source.source_timestamp.desc())

        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    async def get_stats_by_system(self, tenant_id: uuid.UUID) -> Dict[str, int]:
        """Get source count statistics by system.

        Args:
            tenant_id: Tenant ID

        Returns:
            Dictionary mapping system names to counts
        """
        stmt = (
            select(Source.source_system, func.count(Source.id))
            .where(
                and_(
                    Source.tenant_id == tenant_id,
                    Source.is_deleted.is_(False)
                )
            )
            .group_by(Source.source_system)
        )

        result = await self.session.execute(stmt)
        return dict(result.all())

    async def get_stats_by_status(self, tenant_id: uuid.UUID) -> Dict[str, int]:
        """Get source count statistics by processing status.

        Args:
            tenant_id: Tenant ID

        Returns:
            Dictionary mapping status to counts
        """
        stmt = (
            select(Source.processing_status, func.count(Source.id))
            .where(
                and_(
                    Source.tenant_id == tenant_id,
                    Source.is_deleted.is_(False)
                )
            )
            .group_by(Source.processing_status)
        )

        result = await self.session.execute(stmt)
        return dict(result.all())

    async def search_content(
        self, tenant_id: uuid.UUID, search_term: str, limit: int = 50
    ) -> List[Source]:
        """Search source content using PostgreSQL full-text search.

        Args:
            tenant_id: Tenant ID
            search_term: Search term
            limit: Maximum results to return

        Returns:
            List of matching source instances
        """
        stmt = (
            self._build_query()
            .where(
                and_(
                    Source.tenant_id == tenant_id,
                    Source.raw_content.ilike(f'%{search_term}%')
                )
            )
            .order_by(Source.source_timestamp.desc())
            .limit(limit)
        )

        result = await self.session.execute(stmt)
        return list(result.scalars().all())
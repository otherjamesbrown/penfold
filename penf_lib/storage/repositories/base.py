"""Base repository class with common CRUD operations."""

import uuid
from typing import Any, Dict, List, Optional, Type, TypeVar, Generic, Union
from datetime import datetime

from sqlalchemy import select, update, delete, and_, or_, func, desc, asc
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.orm import selectinload, joinedload
from sqlalchemy.sql import Select

from penf_lib.storage.models import Base, SoftDeleteMixin

T = TypeVar("T", bound=Base)


class BaseRepository(Generic[T]):
    """Base repository with common CRUD operations."""

    def __init__(self, session: AsyncSession, model_class: Type[T]):
        """Initialize repository with session and model class.

        Args:
            session: Async SQLAlchemy session
            model_class: SQLAlchemy model class
        """
        self.session = session
        self.model_class = model_class

    async def create(self, **kwargs) -> T:
        """Create a new entity.

        Args:
            **kwargs: Entity attributes

        Returns:
            Created entity instance
        """
        entity = self.model_class(**kwargs)
        self.session.add(entity)
        await self.session.flush()
        return entity

    async def get_by_id(self, entity_id: int) -> Optional[T]:
        """Get entity by ID.

        Args:
            entity_id: Entity ID

        Returns:
            Entity instance or None if not found
        """
        stmt = select(self.model_class).where(self.model_class.id == entity_id)

        # Filter out soft-deleted entities if model supports it
        if hasattr(self.model_class, 'is_deleted'):
            stmt = stmt.where(self.model_class.is_deleted.is_(False))

        result = await self.session.execute(stmt)
        return result.scalar_one_or_none()

    async def get_by_ids(self, entity_ids: List[int]) -> List[T]:
        """Get multiple entities by IDs.

        Args:
            entity_ids: List of entity IDs

        Returns:
            List of entity instances
        """
        stmt = select(self.model_class).where(self.model_class.id.in_(entity_ids))

        # Filter out soft-deleted entities if model supports it
        if hasattr(self.model_class, 'is_deleted'):
            stmt = stmt.where(self.model_class.is_deleted.is_(False))

        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    async def update(self, entity_id: int, **kwargs) -> Optional[T]:
        """Update entity by ID.

        Args:
            entity_id: Entity ID
            **kwargs: Attributes to update

        Returns:
            Updated entity instance or None if not found
        """
        # Add updated_at timestamp if model supports it
        if hasattr(self.model_class, 'updated_at'):
            kwargs['updated_at'] = datetime.utcnow()

        stmt = (
            update(self.model_class)
            .where(self.model_class.id == entity_id)
            .values(**kwargs)
        )

        # Filter out soft-deleted entities if model supports it
        if hasattr(self.model_class, 'is_deleted'):
            stmt = stmt.where(self.model_class.is_deleted.is_(False))

        result = await self.session.execute(stmt)

        if result.rowcount == 0:
            return None

        return await self.get_by_id(entity_id)

    async def update_entity(self, entity: T, **kwargs) -> T:
        """Update entity instance.

        Args:
            entity: Entity instance to update
            **kwargs: Attributes to update

        Returns:
            Updated entity instance
        """
        for key, value in kwargs.items():
            setattr(entity, key, value)

        # Add updated_at timestamp if model supports it
        if hasattr(entity, 'updated_at'):
            entity.updated_at = datetime.utcnow()

        await self.session.flush()
        return entity

    async def delete(self, entity_id: int, hard_delete: bool = False) -> bool:
        """Delete entity by ID.

        Args:
            entity_id: Entity ID
            hard_delete: If True, permanently delete; if False, soft delete

        Returns:
            True if entity was deleted, False if not found
        """
        if not hard_delete and hasattr(self.model_class, 'soft_delete'):
            # Use soft delete if available
            entity = await self.get_by_id(entity_id)
            if not entity:
                return False
            await entity.soft_delete()
            await self.session.flush()
            return True
        else:
            # Hard delete
            stmt = delete(self.model_class).where(self.model_class.id == entity_id)
            result = await self.session.execute(stmt)
            return result.rowcount > 0

    async def delete_entity(self, entity: T, hard_delete: bool = False) -> bool:
        """Delete entity instance.

        Args:
            entity: Entity instance to delete
            hard_delete: If True, permanently delete; if False, soft delete

        Returns:
            True if entity was deleted
        """
        if not hard_delete and hasattr(entity, 'soft_delete'):
            await entity.soft_delete()
        else:
            await self.session.delete(entity)
        await self.session.flush()
        return True

    async def restore(self, entity_id: int) -> Optional[T]:
        """Restore soft-deleted entity.

        Args:
            entity_id: Entity ID

        Returns:
            Restored entity instance or None if not found or not soft-deleted
        """
        if not hasattr(self.model_class, 'restore'):
            return None

        # Get entity including soft-deleted ones
        stmt = select(self.model_class).where(self.model_class.id == entity_id)
        result = await self.session.execute(stmt)
        entity = result.scalar_one_or_none()

        if not entity or not entity.is_deleted:
            return None

        await entity.restore()
        await self.session.flush()
        return entity

    async def list_all(
        self,
        limit: Optional[int] = None,
        offset: Optional[int] = None,
        order_by: Optional[str] = None,
        ascending: bool = True,
        include_deleted: bool = False
    ) -> List[T]:
        """List all entities with pagination and ordering.

        Args:
            limit: Maximum number of entities to return
            offset: Number of entities to skip
            order_by: Column name to order by
            ascending: Sort direction
            include_deleted: Include soft-deleted entities

        Returns:
            List of entity instances
        """
        stmt = select(self.model_class)

        # Filter out soft-deleted entities unless requested
        if not include_deleted and hasattr(self.model_class, 'is_deleted'):
            stmt = stmt.where(self.model_class.is_deleted.is_(False))

        # Apply ordering
        if order_by and hasattr(self.model_class, order_by):
            order_column = getattr(self.model_class, order_by)
            stmt = stmt.order_by(asc(order_column) if ascending else desc(order_column))
        elif hasattr(self.model_class, 'created_at'):
            stmt = stmt.order_by(desc(self.model_class.created_at))

        # Apply pagination
        if offset:
            stmt = stmt.offset(offset)
        if limit:
            stmt = stmt.limit(limit)

        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    async def count(self, include_deleted: bool = False) -> int:
        """Count total entities.

        Args:
            include_deleted: Include soft-deleted entities in count

        Returns:
            Total count
        """
        stmt = select(func.count(self.model_class.id))

        # Filter out soft-deleted entities unless requested
        if not include_deleted and hasattr(self.model_class, 'is_deleted'):
            stmt = stmt.where(self.model_class.is_deleted.is_(False))

        result = await self.session.execute(stmt)
        return result.scalar()

    async def exists(self, entity_id: int) -> bool:
        """Check if entity exists.

        Args:
            entity_id: Entity ID

        Returns:
            True if entity exists and is not soft-deleted
        """
        stmt = select(func.count(self.model_class.id)).where(self.model_class.id == entity_id)

        # Filter out soft-deleted entities if model supports it
        if hasattr(self.model_class, 'is_deleted'):
            stmt = stmt.where(self.model_class.is_deleted.is_(False))

        result = await self.session.execute(stmt)
        return result.scalar() > 0

    async def find_by(self, **filters) -> List[T]:
        """Find entities by field values.

        Args:
            **filters: Field name and value pairs

        Returns:
            List of matching entity instances
        """
        stmt = select(self.model_class)

        # Apply filters
        for field_name, value in filters.items():
            if hasattr(self.model_class, field_name):
                field = getattr(self.model_class, field_name)
                stmt = stmt.where(field == value)

        # Filter out soft-deleted entities if model supports it
        if hasattr(self.model_class, 'is_deleted'):
            stmt = stmt.where(self.model_class.is_deleted.is_(False))

        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    async def find_one_by(self, **filters) -> Optional[T]:
        """Find single entity by field values.

        Args:
            **filters: Field name and value pairs

        Returns:
            Matching entity instance or None
        """
        entities = await self.find_by(**filters)
        return entities[0] if entities else None

    def _build_query(self) -> Select:
        """Build base query with common filters.

        Returns:
            SQLAlchemy Select statement
        """
        stmt = select(self.model_class)

        # Filter out soft-deleted entities if model supports it
        if hasattr(self.model_class, 'is_deleted'):
            stmt = stmt.where(self.model_class.is_deleted.is_(False))

        return stmt
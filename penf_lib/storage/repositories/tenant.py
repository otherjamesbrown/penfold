"""Tenant repository with specialized operations."""

import uuid
from typing import List, Optional, Dict, Any
from datetime import datetime, timedelta

from sqlalchemy import select, and_, or_, func, text
from sqlalchemy.ext.asyncio import AsyncSession

from penf_lib.storage.models import Tenant, TenantSession, CrossTenantPersonLink, Person, UserTenantMembership
from penf_lib.storage.connections import TenantContext, get_session, switch_tenant_context
from .base import BaseRepository


class TenantRepository(BaseRepository[Tenant]):
    """Repository for Tenant entities with management operations."""

    def __init__(self, session: AsyncSession):
        """Initialize tenant repository.

        Args:
            session: Async SQLAlchemy session
        """
        super().__init__(session, Tenant)

    async def create_tenant(
        self,
        name: str,
        display_name: str,
        owner_email: str,
        description: Optional[str] = None,
        settings: Optional[Dict[str, Any]] = None,
    ) -> Tenant:
        """Create a new tenant with validation.

        Args:
            name: Tenant name (work, personal, family)
            display_name: Human-readable name
            owner_email: Owner email address
            description: Optional description
            settings: Optional tenant settings

        Returns:
            Created tenant instance
        """
        return await self.create(
            name=name,
            display_name=display_name,
            owner_email=owner_email,
            description=description,
            settings=settings or {},
        )

    async def find_by_name(self, name: str) -> Optional[Tenant]:
        """Find tenant by name.

        Args:
            name: Tenant name

        Returns:
            Tenant instance or None if not found
        """
        stmt = (
            select(Tenant)
            .where(
                and_(
                    Tenant.name == name,
                    Tenant.is_deleted.is_(False)
                )
            )
        )
        result = await self.session.execute(stmt)
        return result.scalar_one_or_none()

    async def find_by_owner(self, owner_email: str) -> List[Tenant]:
        """Find tenants owned by user.

        Args:
            owner_email: Owner email address

        Returns:
            List of tenant instances
        """
        stmt = (
            select(Tenant)
            .where(
                and_(
                    Tenant.owner_email == owner_email.lower(),
                    Tenant.is_deleted.is_(False)
                )
            )
            .order_by(Tenant.name)
        )
        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    async def get_active_tenants(self) -> List[Tenant]:
        """Get all active tenants.

        Returns:
            List of active tenant instances
        """
        stmt = (
            select(Tenant)
            .where(
                and_(
                    Tenant.is_active.is_(True),
                    Tenant.is_deleted.is_(False)
                )
            )
            .order_by(Tenant.name)
        )
        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    async def update_settings(
        self, tenant_id: uuid.UUID, settings: Dict[str, Any]
    ) -> Optional[Tenant]:
        """Update tenant settings.

        Args:
            tenant_id: Tenant ID
            settings: New settings dictionary

        Returns:
            Updated tenant instance or None if not found
        """
        tenant = await self.get_by_id(tenant_id)
        if not tenant:
            return None

        existing_settings = tenant.settings or {}
        existing_settings.update(settings)

        return await self.update_entity(tenant, settings=existing_settings)

    async def activate_tenant(self, tenant_id: uuid.UUID) -> Optional[Tenant]:
        """Activate a tenant.

        Args:
            tenant_id: Tenant ID

        Returns:
            Updated tenant instance or None if not found
        """
        return await self.update(tenant_id, is_active=True)

    async def deactivate_tenant(self, tenant_id: uuid.UUID) -> Optional[Tenant]:
        """Deactivate a tenant.

        Args:
            tenant_id: Tenant ID

        Returns:
            Updated tenant instance or None if not found
        """
        return await self.update(tenant_id, is_active=False)


class TenantSessionRepository(BaseRepository[TenantSession]):
    """Repository for TenantSession entities."""

    def __init__(self, session: AsyncSession):
        """Initialize tenant session repository.

        Args:
            session: Async SQLAlchemy session
        """
        super().__init__(session, TenantSession)

    async def create_session(
        self,
        session_id: str,
        tenant_id: uuid.UUID,
        user_email: str,
        expires_hours: int = 24
    ) -> TenantSession:
        """Create a new tenant session.

        Args:
            session_id: Unique session identifier
            tenant_id: Tenant UUID
            user_email: User email
            expires_hours: Hours until expiration

        Returns:
            Created session instance
        """
        expires_at = datetime.utcnow() + timedelta(hours=expires_hours)

        return await self.create(
            session_id=session_id,
            tenant_id=tenant_id,
            user_email=user_email,
            expires_at=expires_at,
        )

    async def find_by_session_id(self, session_id: str) -> Optional[TenantSession]:
        """Find session by ID.

        Args:
            session_id: Session identifier

        Returns:
            Session instance or None if not found
        """
        stmt = (
            select(TenantSession)
            .where(TenantSession.session_id == session_id)
        )
        result = await self.session.execute(stmt)
        return result.scalar_one_or_none()

    async def find_active_sessions(self, user_email: str) -> List[TenantSession]:
        """Find active sessions for user.

        Args:
            user_email: User email

        Returns:
            List of active session instances
        """
        stmt = (
            select(TenantSession)
            .where(
                and_(
                    TenantSession.user_email == user_email.lower(),
                    TenantSession.expires_at > datetime.utcnow()
                )
            )
            .order_by(TenantSession.last_activity.desc())
        )
        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    async def extend_session(
        self, session_id: str, hours: int = 24
    ) -> Optional[TenantSession]:
        """Extend session expiration.

        Args:
            session_id: Session identifier
            hours: Hours to extend

        Returns:
            Updated session instance or None if not found
        """
        session = await self.find_by_session_id(session_id)
        if not session:
            return None

        new_expires = datetime.utcnow() + timedelta(hours=hours)

        return await self.update_entity(
            session,
            last_activity=datetime.utcnow(),
            expires_at=new_expires
        )

    async def cleanup_expired_sessions(self) -> int:
        """Clean up expired sessions.

        Returns:
            Number of sessions cleaned up
        """
        stmt = (
            select(func.count(TenantSession.id))
            .where(TenantSession.expires_at <= datetime.utcnow())
        )
        result = await self.session.execute(stmt)
        count = result.scalar()

        # Delete expired sessions
        delete_stmt = text(
            "DELETE FROM tenant_sessions WHERE expires_at <= NOW()"
        )
        await self.session.execute(delete_stmt)

        return count


class CrossTenantPersonLinkRepository(BaseRepository[CrossTenantPersonLink]):
    """Repository for CrossTenantPersonLink entities."""

    def __init__(self, session: AsyncSession):
        """Initialize cross-tenant person link repository.

        Args:
            session: Async SQLAlchemy session
        """
        super().__init__(session, CrossTenantPersonLink)

    async def create_person_link(
        self,
        canonical_person_id: str,
        tenant_id: uuid.UUID,
        person_id: int,
        link_confidence: float = 1.0,
        link_method: str = "manual",
        validated_by: Optional[str] = None,
    ) -> CrossTenantPersonLink:
        """Create a new person link across tenants.

        Args:
            canonical_person_id: Canonical person identifier
            tenant_id: Tenant UUID
            person_id: Person ID in specific tenant
            link_confidence: Confidence score (0.0-1.0)
            link_method: Method used to establish link
            validated_by: Email of validator (optional)

        Returns:
            Created link instance
        """
        validated_at = datetime.utcnow() if validated_by else None

        return await self.create(
            canonical_person_id=canonical_person_id,
            tenant_id=tenant_id,
            person_id=person_id,
            link_confidence=link_confidence,
            link_method=link_method,
            validated_by=validated_by,
            validated_at=validated_at,
        )

    async def find_by_canonical_id(
        self, canonical_person_id: str
    ) -> List[CrossTenantPersonLink]:
        """Find all links for a canonical person.

        Args:
            canonical_person_id: Canonical person identifier

        Returns:
            List of person link instances
        """
        return await self.find_by(canonical_person_id=canonical_person_id)

    async def find_by_person(
        self, tenant_id: uuid.UUID, person_id: int
    ) -> Optional[CrossTenantPersonLink]:
        """Find link for specific person in tenant.

        Args:
            tenant_id: Tenant UUID
            person_id: Person ID

        Returns:
            Person link instance or None if not found
        """
        return await self.find_one_by(
            tenant_id=tenant_id,
            person_id=person_id
        )

    async def find_linked_persons(
        self, canonical_person_id: str
    ) -> List[Dict[str, Any]]:
        """Find all persons linked to canonical ID.

        Args:
            canonical_person_id: Canonical person identifier

        Returns:
            List of person information across tenants
        """
        stmt = (
            select(
                CrossTenantPersonLink.tenant_id,
                CrossTenantPersonLink.person_id,
                CrossTenantPersonLink.link_confidence,
                CrossTenantPersonLink.link_method,
                CrossTenantPersonLink.validated_at,
                Tenant.name.label('tenant_name'),
                Person.canonical_name,
                Person.email_addresses,
            )
            .join(Tenant, CrossTenantPersonLink.tenant_id == Tenant.id)
            .join(Person, CrossTenantPersonLink.person_id == Person.id)
            .where(CrossTenantPersonLink.canonical_person_id == canonical_person_id)
            .order_by(CrossTenantPersonLink.link_confidence.desc())
        )

        result = await self.session.execute(stmt)
        persons = []
        for row in result:
            persons.append({
                "tenant_id": str(row.tenant_id),
                "tenant_name": row.tenant_name,
                "person_id": row.person_id,
                "canonical_name": row.canonical_name,
                "email_addresses": row.email_addresses,
                "link_confidence": float(row.link_confidence),
                "link_method": row.link_method,
                "validated_at": row.validated_at,
            })

        return persons

    async def validate_link(
        self, link_id: int, validated_by: str
    ) -> Optional[CrossTenantPersonLink]:
        """Validate a person link.

        Args:
            link_id: Link ID
            validated_by: Email of validator

        Returns:
            Updated link instance or None if not found
        """
        return await self.update(
            link_id,
            validated_by=validated_by.lower(),
            validated_at=datetime.utcnow()
        )

    async def find_unvalidated_links(self) -> List[CrossTenantPersonLink]:
        """Find all unvalidated person links.

        Returns:
            List of unvalidated link instances
        """
        stmt = (
            select(CrossTenantPersonLink)
            .where(CrossTenantPersonLink.validated_at.is_(None))
            .order_by(CrossTenantPersonLink.link_confidence.desc())
        )
        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    async def suggest_person_links(
        self, tenant_id: uuid.UUID, person_id: int
    ) -> List[Dict[str, Any]]:
        """Suggest potential person links based on email similarity.

        Args:
            tenant_id: Source tenant ID
            person_id: Source person ID

        Returns:
            List of suggested links with similarity scores
        """
        # This would implement fuzzy matching logic
        # For now, return empty list as placeholder
        return []


class UserTenantMembershipRepository(BaseRepository[UserTenantMembership]):
    """Repository for UserTenantMembership entities."""

    def __init__(self, session: AsyncSession):
        """Initialize user tenant membership repository."""
        super().__init__(session, UserTenantMembership)

    async def create_membership(
        self,
        user_email: str,
        tenant_id: uuid.UUID,
        role: str = "member",
        granted_by: Optional[str] = None,
    ) -> UserTenantMembership:
        """Grant tenant access to user."""
        return await self.create(
            user_email=user_email.lower(),
            tenant_id=tenant_id,
            role=role,
            granted_by=granted_by.lower() if granted_by else None,
        )

    async def has_access(self, user_email: str, tenant_id: uuid.UUID) -> bool:
        """Check if user has access to tenant."""
        membership = await self.find_one_by(
            user_email=user_email.lower(),
            tenant_id=tenant_id,
            is_active=True,
        )
        return membership is not None

    async def get_membership(
        self, user_email: str, tenant_id: uuid.UUID
    ) -> Optional[UserTenantMembership]:
        """Get user's membership for tenant."""
        return await self.find_one_by(
            user_email=user_email.lower(),
            tenant_id=tenant_id,
            is_active=True,
        )

    async def get_user_tenants(self, user_email: str) -> List[UserTenantMembership]:
        """Get all tenants user has access to."""
        return await self.find_by(
            user_email=user_email.lower(),
            is_active=True,
        )

    async def get_tenant_members(self, tenant_id: uuid.UUID) -> List[UserTenantMembership]:
        """Get all members of a tenant."""
        return await self.find_by(tenant_id=tenant_id, is_active=True)

    async def update_role(
        self, user_email: str, tenant_id: uuid.UUID, new_role: str
    ) -> Optional[UserTenantMembership]:
        """Update user's role in tenant."""
        membership = await self.get_membership(user_email, tenant_id)
        if membership:
            return await self.update_entity(membership, role=new_role)
        return None

    async def revoke_access(
        self, user_email: str, tenant_id: uuid.UUID
    ) -> Optional[UserTenantMembership]:
        """Revoke user's access to tenant (soft delete)."""
        membership = await self.get_membership(user_email, tenant_id)
        if membership:
            return await self.update_entity(membership, is_active=False)
        return None
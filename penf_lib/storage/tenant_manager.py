"""High-level tenant management service."""

import uuid
import logging
from typing import List, Optional, Dict, Any, Tuple
from datetime import datetime

from .connections import (
    get_session,
    TenantContext,
    switch_tenant_context,
    get_current_tenant_context,
    clear_current_tenant_context,
    get_available_tenants,
    get_current_session_info,
    extend_current_session,
    cleanup_expired_sessions
)
from .repositories.tenant import (
    TenantRepository,
    TenantSessionRepository,
    CrossTenantPersonLinkRepository,
    UserTenantMembershipRepository,
)
from .audit import TenantAuditService

logger = logging.getLogger(__name__)


class TenantManager:
    """High-level tenant management service for multi-tenant operations."""

    def __init__(self):
        """Initialize tenant manager."""
        pass

    async def create_tenant(
        self,
        name: str,
        display_name: str,
        owner_email: str,
        description: Optional[str] = None,
        settings: Optional[Dict[str, Any]] = None,
    ) -> Dict[str, Any]:
        """Create a new tenant.

        Args:
            name: Tenant name (work, personal, family)
            display_name: Human-readable name
            owner_email: Owner email address
            description: Optional description
            settings: Optional tenant settings

        Returns:
            Dictionary with tenant information

        Raises:
            ValueError: If tenant name is invalid or already exists
        """
        async with get_session() as session:  # Superuser access
            tenant_repo = TenantRepository(session)

            # Check if tenant already exists
            existing = await tenant_repo.find_by_name(name)
            if existing:
                raise ValueError(f"Tenant '{name}' already exists")

            # Create tenant
            tenant = await tenant_repo.create_tenant(
                name=name,
                display_name=display_name,
                owner_email=owner_email,
                description=description,
                settings=settings,
            )

            # Auto-grant owner membership
            membership_repo = UserTenantMembershipRepository(session)
            await membership_repo.create_membership(
                user_email=owner_email,
                tenant_id=tenant.id,
                role="owner",
                granted_by=owner_email,  # Self-granted on creation
            )

            # Log membership grant
            await TenantAuditService.log_membership_granted(
                user_email=owner_email,
                tenant_id=tenant.id,
                tenant_name=tenant.name,
                granted_by=owner_email,
                role="owner",
            )

            logger.info(f"Created tenant '{name}' with owner membership for {owner_email}")

            return {
                "id": str(tenant.id),
                "name": tenant.name,
                "display_name": tenant.display_name,
                "description": tenant.description,
                "owner_email": tenant.owner_email,
                "is_active": tenant.is_active,
                "created_at": tenant.created_at.isoformat(),
            }

    async def switch_to_tenant(
        self,
        tenant_name: str,
        user_email: str
    ) -> Dict[str, Any]:
        """Switch to a specific tenant context.

        Args:
            tenant_name: Target tenant name
            user_email: User email for session tracking

        Returns:
            Dictionary with session information

        Raises:
            ValueError: If tenant doesn't exist or user doesn't have access
        """
        async with get_session() as session:  # Superuser access
            tenant_repo = TenantRepository(session)
            membership_repo = UserTenantMembershipRepository(session)

            # Find tenant
            tenant = await tenant_repo.find_by_name(tenant_name)
            if not tenant:
                raise ValueError(f"Tenant '{tenant_name}' not found")

            if not tenant.is_active:
                raise ValueError(f"Tenant '{tenant_name}' is not active")

            # Check access - user must have active membership OR be owner
            if tenant.owner_email.lower() != user_email.lower():
                has_access = await membership_repo.has_access(user_email, tenant.id)
                if not has_access:
                    await TenantAuditService.log_switch_denied(
                        user_email=user_email,
                        tenant_name=tenant_name,
                        reason="no_membership",
                    )
                    raise ValueError(f"User does not have access to tenant '{tenant_name}'")

            # Switch context
            success = await switch_tenant_context(tenant.id, user_email)
            if not success:
                raise ValueError(f"Failed to switch to tenant '{tenant_name}'")

            # Log successful switch
            await TenantAuditService.log_switch_success(
                user_email=user_email,
                tenant_id=tenant.id,
                tenant_name=tenant.name,
            )

            # Get session info
            session_info = await get_current_session_info()

            logger.info(f"Switched to tenant '{tenant_name}' for user {user_email}")

            return session_info or {
                "tenant_id": str(tenant.id),
                "tenant_name": tenant.name,
                "tenant_display_name": tenant.display_name,
                "user_email": user_email,
            }

    async def list_tenants(self, user_email: str) -> List[Dict[str, Any]]:
        """List available tenants for user.

        Args:
            user_email: User email

        Returns:
            List of tenant information dictionaries
        """
        return await get_available_tenants(user_email)

    async def get_current_tenant(self) -> Optional[Dict[str, Any]]:
        """Get current tenant information.

        Returns:
            Current tenant information or None if no active tenant
        """
        return await get_current_session_info()

    async def clear_tenant_context(self) -> None:
        """Clear current tenant context."""
        clear_current_tenant_context()
        logger.info("Cleared tenant context")

    async def update_tenant_settings(
        self,
        tenant_name: str,
        settings: Dict[str, Any],
        user_email: str
    ) -> Dict[str, Any]:
        """Update tenant settings.

        Args:
            tenant_name: Tenant name
            settings: Settings to update
            user_email: User email for authorization

        Returns:
            Updated tenant information

        Raises:
            ValueError: If tenant not found or user lacks permission
        """
        async with get_session() as session:  # Superuser access
            tenant_repo = TenantRepository(session)

            # Find tenant
            tenant = await tenant_repo.find_by_name(tenant_name)
            if not tenant:
                raise ValueError(f"Tenant '{tenant_name}' not found")

            # Check permission (for now, only owner can update)
            if tenant.owner_email.lower() != user_email.lower():
                raise ValueError("Only tenant owner can update settings")

            # Update settings
            updated_tenant = await tenant_repo.update_settings(tenant.id, settings)

            logger.info(f"Updated settings for tenant '{tenant_name}' by {user_email}")

            return {
                "id": str(updated_tenant.id),
                "name": updated_tenant.name,
                "display_name": updated_tenant.display_name,
                "settings": updated_tenant.settings,
                "updated_at": updated_tenant.updated_at.isoformat(),
            }

    async def extend_session(self, hours: int = 24) -> bool:
        """Extend current session expiration.

        Args:
            hours: Number of hours to extend

        Returns:
            True if session was extended successfully
        """
        result = await extend_current_session(hours)
        if result:
            logger.info(f"Extended current session by {hours} hours")
        return result

    async def cleanup_sessions(self) -> int:
        """Clean up expired sessions.

        Returns:
            Number of sessions cleaned up
        """
        count = await cleanup_expired_sessions()
        if count > 0:
            logger.info(f"Cleaned up {count} expired sessions")
        return count

    async def link_person_across_tenants(
        self,
        canonical_person_id: str,
        tenant_person_mappings: List[Tuple[str, int]],  # (tenant_name, person_id)
        link_method: str = "manual",
        validated_by: Optional[str] = None
    ) -> List[Dict[str, Any]]:
        """Link a person across multiple tenants.

        Args:
            canonical_person_id: Canonical identifier for this person
            tenant_person_mappings: List of (tenant_name, person_id) tuples
            link_method: Method used to establish links
            validated_by: Email of validator

        Returns:
            List of created link information

        Raises:
            ValueError: If tenants or persons not found
        """
        async with get_session() as session:  # Superuser access
            tenant_repo = TenantRepository(session)
            link_repo = CrossTenantPersonLinkRepository(session)

            created_links = []

            for tenant_name, person_id in tenant_person_mappings:
                # Find tenant
                tenant = await tenant_repo.find_by_name(tenant_name)
                if not tenant:
                    raise ValueError(f"Tenant '{tenant_name}' not found")

                # Check if link already exists
                existing_link = await link_repo.find_by_person(tenant.id, person_id)
                if existing_link:
                    logger.warning(
                        f"Person {person_id} in tenant {tenant_name} already linked "
                        f"to {existing_link.canonical_person_id}"
                    )
                    continue

                # Create link
                link = await link_repo.create_person_link(
                    canonical_person_id=canonical_person_id,
                    tenant_id=tenant.id,
                    person_id=person_id,
                    link_method=link_method,
                    validated_by=validated_by,
                )

                created_links.append({
                    "link_id": link.id,
                    "tenant_name": tenant_name,
                    "person_id": person_id,
                    "canonical_person_id": canonical_person_id,
                    "link_confidence": float(link.link_confidence),
                    "created_at": link.created_at.isoformat(),
                })

                logger.info(
                    f"Linked person {person_id} in tenant {tenant_name} "
                    f"to canonical person {canonical_person_id}"
                )

            return created_links

    async def get_linked_persons(
        self, canonical_person_id: str
    ) -> List[Dict[str, Any]]:
        """Get all persons linked to a canonical person ID.

        Args:
            canonical_person_id: Canonical person identifier

        Returns:
            List of linked person information across tenants
        """
        async with get_session() as session:  # Superuser access
            link_repo = CrossTenantPersonLinkRepository(session)
            return await link_repo.find_linked_persons(canonical_person_id)

    async def suggest_person_links(
        self, tenant_name: str, person_id: int
    ) -> List[Dict[str, Any]]:
        """Suggest potential cross-tenant person links.

        Args:
            tenant_name: Source tenant name
            person_id: Source person ID

        Returns:
            List of suggested links with similarity scores
        """
        async with get_session() as session:  # Superuser access
            tenant_repo = TenantRepository(session)
            link_repo = CrossTenantPersonLinkRepository(session)

            # Find tenant
            tenant = await tenant_repo.find_by_name(tenant_name)
            if not tenant:
                raise ValueError(f"Tenant '{tenant_name}' not found")

            # Get suggestions (placeholder implementation)
            return await link_repo.suggest_person_links(tenant.id, person_id)

    async def validate_person_link(
        self, link_id: int, validated_by: str
    ) -> Dict[str, Any]:
        """Validate a cross-tenant person link.

        Args:
            link_id: Link ID
            validated_by: Email of validator

        Returns:
            Updated link information

        Raises:
            ValueError: If link not found
        """
        async with get_session() as session:  # Superuser access
            link_repo = CrossTenantPersonLinkRepository(session)

            link = await link_repo.validate_link(link_id, validated_by)
            if not link:
                raise ValueError(f"Person link {link_id} not found")

            logger.info(f"Person link {link_id} validated by {validated_by}")

            return {
                "link_id": link.id,
                "canonical_person_id": link.canonical_person_id,
                "validated_by": link.validated_by,
                "validated_at": link.validated_at.isoformat(),
            }


# Global tenant manager instance
tenant_manager = TenantManager()
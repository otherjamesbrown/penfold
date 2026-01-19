"""Tenant access audit logging service."""

import logging
from typing import Optional, Dict, Any
import uuid

logger = logging.getLogger(__name__)


class TenantAuditService:
    """Service for recording tenant access audit events."""

    @staticmethod
    async def log_switch_success(
        user_email: str,
        tenant_id: uuid.UUID,
        tenant_name: str,
        session_id: Optional[str] = None,
    ) -> None:
        """Log successful tenant switch."""
        await TenantAuditService._log_event(
            event_type="switch_success",
            user_email=user_email,
            tenant_id=tenant_id,
            tenant_name=tenant_name,
            session_id=session_id,
        )

    @staticmethod
    async def log_switch_denied(
        user_email: str,
        tenant_name: str,
        reason: str,
    ) -> None:
        """Log denied tenant switch attempt."""
        await TenantAuditService._log_event(
            event_type="switch_denied",
            user_email=user_email,
            tenant_name=tenant_name,
            details={"reason": reason},
        )
        logger.warning(f"Tenant access denied: {user_email} -> {tenant_name}: {reason}")

    @staticmethod
    async def log_membership_granted(
        user_email: str,
        tenant_id: uuid.UUID,
        tenant_name: str,
        granted_by: str,
        role: str,
    ) -> None:
        """Log membership grant."""
        await TenantAuditService._log_event(
            event_type="membership_granted",
            user_email=user_email,
            tenant_id=tenant_id,
            tenant_name=tenant_name,
            details={"granted_by": granted_by, "role": role},
        )

    @staticmethod
    async def log_membership_revoked(
        user_email: str,
        tenant_id: uuid.UUID,
        tenant_name: str,
        revoked_by: str,
    ) -> None:
        """Log membership revocation."""
        await TenantAuditService._log_event(
            event_type="membership_revoked",
            user_email=user_email,
            tenant_id=tenant_id,
            tenant_name=tenant_name,
            details={"revoked_by": revoked_by},
        )

    @staticmethod
    async def _log_event(
        event_type: str,
        user_email: str,
        tenant_id: Optional[uuid.UUID] = None,
        tenant_name: Optional[str] = None,
        details: Optional[Dict[str, Any]] = None,
        session_id: Optional[str] = None,
        ip_address: Optional[str] = None,
    ) -> None:
        """Internal method to log audit event."""
        try:
            from .connections import get_session
            from .models import TenantAuditLog

            async with get_session() as session:
                audit_log = TenantAuditLog(
                    event_type=event_type,
                    user_email=user_email.lower(),
                    tenant_id=tenant_id,
                    tenant_name=tenant_name,
                    details=details,
                    session_id=session_id,
                    ip_address=ip_address,
                )
                session.add(audit_log)
                await session.commit()
        except Exception as e:
            # Never fail the main operation due to audit logging
            logger.error(f"Failed to log audit event: {e}")

"""Deduplication Logic for Manual Ingest.

Two-level deduplication strategy:
1. Primary: Message-ID header matching
2. Secondary: Content hash (SHA-256) for emails without Message-ID

This enables cross-source deduplication between Gmail integration
and manual uploads.

Reference: specs/012-manual-ingest/research.md Section 4
"""

from dataclasses import dataclass
from typing import Optional
from uuid import UUID

from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from penf_lib.storage.models import Source


@dataclass
class DuplicateCheckResult:
    """Result of duplicate check.

    Attributes:
        is_duplicate: True if duplicate found
        match_type: How duplicate was detected ("message_id" or "content_hash")
        existing_source_id: ID of existing source if duplicate
    """

    is_duplicate: bool
    match_type: Optional[str] = None
    existing_source_id: Optional[int] = None


async def check_duplicate(
    session: AsyncSession,
    tenant_id: UUID,
    message_id: str,
    content_hash: str,
) -> DuplicateCheckResult:
    """Check if email already exists in storage.

    Performs two-level deduplication:
    1. Message-ID match (primary, cross-source)
    2. Content hash match (fallback, handles re-uploads)

    Args:
        session: Database session
        tenant_id: Tenant ID for isolation
        message_id: Email Message-ID (real or synthetic)
        content_hash: SHA-256 hash of raw content

    Returns:
        DuplicateCheckResult with match details
    """
    # Primary check: Message-ID match
    # This catches duplicates across Gmail and manual uploads
    stmt = select(Source).where(
        Source.tenant_id == tenant_id,
        Source.external_id == message_id,
        Source.is_deleted == False,  # noqa: E712
    )
    result = await session.execute(stmt)
    existing = result.scalar_one_or_none()

    if existing:
        return DuplicateCheckResult(
            is_duplicate=True,
            match_type="message_id",
            existing_source_id=existing.id,
        )

    # Secondary check: Content hash
    # Catches same content with different Message-IDs
    stmt = select(Source).where(
        Source.tenant_id == tenant_id,
        Source.content_hash == content_hash,
        Source.is_deleted == False,  # noqa: E712
    )
    result = await session.execute(stmt)
    existing = result.scalar_one_or_none()

    if existing:
        return DuplicateCheckResult(
            is_duplicate=True,
            match_type="content_hash",
            existing_source_id=existing.id,
        )

    return DuplicateCheckResult(is_duplicate=False)


async def check_duplicate_simple(
    session: AsyncSession,
    tenant_id: UUID,
    message_id: str,
    content_hash: str,
) -> bool:
    """Simple duplicate check returning boolean.

    Convenience wrapper for use in batch processing.

    Args:
        session: Database session
        tenant_id: Tenant ID
        message_id: Email Message-ID
        content_hash: SHA-256 content hash

    Returns:
        True if duplicate exists
    """
    result = await check_duplicate(session, tenant_id, message_id, content_hash)
    return result.is_duplicate

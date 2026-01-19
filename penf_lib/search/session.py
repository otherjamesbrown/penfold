"""Search session management for tracking user search activity.

Sessions track user queries, results, and interactions to enable
history, suggestions, and analytics.
"""

from __future__ import annotations

import uuid
from datetime import datetime, timezone, timedelta
from typing import Optional, List, TYPE_CHECKING
from dataclasses import dataclass

from sqlalchemy.ext.asyncio import AsyncSession

if TYPE_CHECKING:
    from penf_lib.storage.repositories.search import SearchRepository


@dataclass
class SearchSessionInfo:
    """Information about a search session.

    Attributes:
        session_id: Unique identifier for the session (UUID string).
        user_email: Email address of the user who owns this session.
        started_at: When the session was created.
        last_activity_at: When the session was last active.
        expires_at: When the session will expire.
        total_queries: Number of queries executed in this session.
        is_active: Whether the session is currently active.
    """

    session_id: str
    user_email: str
    started_at: datetime
    last_activity_at: datetime
    expires_at: datetime
    total_queries: int
    is_active: bool


class SessionManager:
    """Manage search sessions for users.

    Sessions provide context for search operations, enabling:
    - Query history tracking per user
    - Session-scoped query suggestions
    - Analytics aggregation by session

    Attributes:
        db_session: SQLAlchemy async session for database operations.
        tenant_id: UUID string for multi-tenant isolation.
    """

    SESSION_DURATION_HOURS = 24

    def __init__(self, db_session: AsyncSession, tenant_id: str):
        """Initialize session manager.

        Args:
            db_session: SQLAlchemy async session for database operations.
            tenant_id: Tenant UUID string for multi-tenant isolation.
        """
        self.db_session = db_session
        self.tenant_id = tenant_id
        self._repository: Optional[SearchRepository] = None

    @property
    def repository(self) -> SearchRepository:
        """Lazy-load search repository."""
        if self._repository is None:
            from penf_lib.storage.repositories.search import SearchRepository

            self._repository = SearchRepository(self.db_session)
        return self._repository

    def _to_uuid(self) -> uuid.UUID:
        """Convert tenant_id string to UUID."""
        try:
            return uuid.UUID(self.tenant_id)
        except ValueError:
            # Use a deterministic UUID from the tenant name
            return uuid.uuid5(uuid.NAMESPACE_DNS, self.tenant_id)

    async def get_or_create_session(self, user_email: str) -> SearchSessionInfo:
        """Get existing active session or create new one.

        Args:
            user_email: Email address of the user.

        Returns:
            SearchSessionInfo for the active session.
        """
        tenant_uuid = self._to_uuid()

        # Check for existing active session
        existing = await self.repository.get_active_session(tenant_uuid, user_email)
        if existing:
            await self.repository.update_session_activity(existing["id"])
            return SearchSessionInfo(
                session_id=existing["session_id"],
                user_email=existing["user_email"],
                started_at=existing["started_at"],
                last_activity_at=datetime.now(timezone.utc),
                expires_at=existing["expires_at"],
                total_queries=existing["total_queries"],
                is_active=existing["is_active"],
            )

        # Create new session
        new_session = await self.repository.create_session(
            tenant_uuid,
            user_email,
            expires_hours=self.SESSION_DURATION_HOURS,
        )
        return SearchSessionInfo(
            session_id=new_session["session_id"],
            user_email=new_session["user_email"],
            started_at=new_session["started_at"],
            last_activity_at=new_session["started_at"],
            expires_at=new_session["expires_at"],
            total_queries=0,
            is_active=True,
        )

    async def end_session(self, session_id: str) -> bool:
        """End a search session.

        Args:
            session_id: UUID string of the session to end.

        Returns:
            True if the session was successfully closed.
        """
        try:
            session_uuid = uuid.UUID(session_id)
        except ValueError:
            # Try to extract UUID from session_id format "search-<hex>"
            if session_id.startswith("search-"):
                hex_part = session_id[7:]
                # Pad to 32 hex chars if needed
                hex_part = hex_part.ljust(32, "0")
                session_uuid = uuid.UUID(hex_part)
            else:
                session_uuid = uuid.uuid5(uuid.NAMESPACE_DNS, session_id)

        await self.repository.close_session(session_uuid)
        return True

    async def get_session_history(
        self,
        user_email: str,
        limit: int = 10,
    ) -> List[dict]:
        """Get recent queries for user.

        Args:
            user_email: Email of the user (currently unused, gets all tenant queries).
            limit: Maximum number of queries to return.

        Returns:
            List of query history dictionaries with keys:
            - query_text: The search query string
            - result_count: Number of results returned
            - execution_time_ms: Query execution time in milliseconds
            - timestamp: When the query was executed
        """
        tenant_uuid = self._to_uuid()
        queries = await self.repository.get_recent_queries(tenant_uuid, limit)
        return [
            {
                "query_text": q["query_text"],
                "result_count": q["result_count"],
                "execution_time_ms": q["execution_time_ms"],
                "timestamp": q["created_at"],
            }
            for q in queries
        ]

    async def record_query(
        self,
        session_id: str,
        query_text: str,
        filters: dict,
        execution_time_ms: int,
        result_count: int,
        cache_hit: bool,
        search_strategy: str,
    ) -> dict:
        """Record a search query for history and analytics.

        Args:
            session_id: UUID string of the search session.
            query_text: The search query text.
            filters: Dictionary of applied filters.
            execution_time_ms: Query execution time in milliseconds.
            result_count: Number of results returned.
            cache_hit: Whether the query was served from cache.
            search_strategy: Search strategy used ('hybrid', 'full_text', 'semantic').

        Returns:
            Dictionary with the recorded query details.
        """
        tenant_uuid = self._to_uuid()

        # Extract session UUID
        try:
            session_uuid = uuid.UUID(session_id)
        except ValueError:
            if session_id.startswith("search-"):
                hex_part = session_id[7:].ljust(32, "0")
                session_uuid = uuid.UUID(hex_part)
            else:
                session_uuid = uuid.uuid5(uuid.NAMESPACE_DNS, session_id)

        return await self.repository.record_query(
            session_id=session_uuid,
            tenant_id=tenant_uuid,
            query_text=query_text,
            filters=filters,
            execution_time_ms=execution_time_ms,
            result_count=result_count,
            cache_hit=cache_hit,
            search_strategy=search_strategy,
        )

    async def cleanup_expired(self) -> int:
        """Clean up expired sessions.

        Returns:
            Number of sessions cleaned up.
        """
        tenant_uuid = self._to_uuid()
        return await self.repository.cleanup_expired_sessions(tenant_uuid)

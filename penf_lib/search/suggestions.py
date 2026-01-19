"""Query suggestion system for search autocomplete.

Provides suggestions based on:
- Popular queries (frequency-based)
- Recent user queries
- Contextual suggestions
"""

from __future__ import annotations

import uuid
from typing import List, Optional, TYPE_CHECKING
from dataclasses import dataclass

from sqlalchemy.ext.asyncio import AsyncSession

if TYPE_CHECKING:
    from penf_lib.storage.repositories.search import SearchRepository


@dataclass
class QuerySuggestion:
    """A suggested search query.

    Attributes:
        text: The suggested query text.
        suggestion_type: Type of suggestion ('popular', 'recent', 'contextual').
        frequency: How often this suggestion has been used.
        success_rate: Rate of successful searches with this suggestion (0.0-1.0).
    """

    text: str
    suggestion_type: str
    frequency: int
    success_rate: float


class SuggestionEngine:
    """Generate query suggestions for autocomplete.

    Provides suggestions based on:
    - Popular queries: High frequency across all users
    - Recent queries: User's own recent searches
    - Contextual: Based on current context or content

    Attributes:
        db_session: SQLAlchemy async session for database operations.
        tenant_id: UUID string for multi-tenant isolation.
    """

    def __init__(self, db_session: AsyncSession, tenant_id: str):
        """Initialize suggestion engine.

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
            return uuid.uuid5(uuid.NAMESPACE_DNS, self.tenant_id)

    async def get_suggestions(
        self,
        prefix: Optional[str] = None,
        limit: int = 5,
    ) -> List[QuerySuggestion]:
        """Get query suggestions, optionally filtered by prefix.

        Args:
            prefix: Optional prefix to filter suggestions (for autocomplete).
            limit: Maximum number of suggestions to return.

        Returns:
            List of QuerySuggestion objects sorted by relevance.
        """
        tenant_uuid = self._to_uuid()

        suggestions = await self.repository.get_suggestions(
            tenant_uuid, prefix, limit
        )
        return [
            QuerySuggestion(
                text=s["suggestion_text"],
                suggestion_type=s["suggestion_type"] or "popular",
                frequency=s["frequency"],
                success_rate=float(s["success_rate"] or 0.0),
            )
            for s in suggestions
        ]

    async def get_popular_queries(
        self,
        limit: int = 10,
    ) -> List[QuerySuggestion]:
        """Get the most popular search queries.

        Args:
            limit: Maximum number of queries to return.

        Returns:
            List of QuerySuggestion objects sorted by frequency.
        """
        # Popular queries are suggestions with highest frequency
        return await self.get_suggestions(prefix=None, limit=limit)

    async def record_successful_query(self, query_text: str) -> None:
        """Record a successful query for future suggestions.

        This updates the suggestion frequency and creates new suggestions
        for queries that returned results.

        Args:
            query_text: The search query text to record.
        """
        tenant_uuid = self._to_uuid()
        await self.repository.update_suggestion_frequency(tenant_uuid, query_text)

    async def update_success_rate(
        self,
        query_text: str,
        was_successful: bool,
    ) -> None:
        """Update success rate for a suggestion.

        Called after search execution to track which queries
        produce results users engage with.

        Args:
            query_text: The search query text.
            was_successful: Whether the search was successful
                           (returned useful results that were clicked).
        """
        tenant_uuid = self._to_uuid()
        await self.repository.update_suggestion_success_rate(
            tenant_uuid, query_text, was_successful
        )

    async def get_contextual_suggestions(
        self,
        context: str,
        limit: int = 5,
    ) -> List[QuerySuggestion]:
        """Get suggestions based on context.

        Generates suggestions based on the provided context string,
        which could be content being viewed, recent activity, etc.

        Args:
            context: Context string to base suggestions on.
            limit: Maximum number of suggestions to return.

        Returns:
            List of contextual QuerySuggestion objects.

        Note:
            Currently returns prefix-matched suggestions. Future versions
            will use semantic similarity for better contextual matching.
        """
        # For now, use prefix matching as a simple contextual approach
        # Future: Use embedding similarity for semantic matching
        words = context.lower().split()
        suggestions: List[QuerySuggestion] = []

        # Try each word as a prefix
        for word in words[:3]:  # Limit to first 3 words
            if len(word) >= 3:  # Only meaningful prefixes
                word_suggestions = await self.get_suggestions(
                    prefix=word, limit=limit
                )
                for s in word_suggestions:
                    if s not in suggestions:
                        suggestions.append(s)
                        if len(suggestions) >= limit:
                            break
            if len(suggestions) >= limit:
                break

        return suggestions[:limit]

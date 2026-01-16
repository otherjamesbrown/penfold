"""Repository for search operations with full-text and vector similarity search.

This module provides the SearchRepository with methods for executing searches,
managing search sessions, recording queries, and providing suggestions.
"""

from __future__ import annotations

import logging
import uuid
from datetime import UTC, datetime, timedelta
from decimal import Decimal
from typing import Any
from uuid import UUID

from sqlalchemy import text
from sqlalchemy.exc import SQLAlchemyError
from sqlalchemy.ext.asyncio import AsyncSession

logger = logging.getLogger(__name__)


class SearchRepository:
    """Repository for search operations across sources, assertions, and embeddings.

    Provides full-text search using PostgreSQL tsvector/tsquery, vector similarity
    search using pgvector, and session/query management for search analytics.
    """

    def __init__(self, session: AsyncSession):
        """Initialize search repository.

        Args:
            session: Async SQLAlchemy session
        """
        self.session = session

    # =========================================================================
    # FULL-TEXT SEARCH
    # =========================================================================

    async def full_text_search(
        self,
        query: str,
        tenant_id: UUID,
        content_types: list[str] | None = None,
        limit: int = 50,
    ) -> list[tuple[int, float]]:
        """Execute PostgreSQL full-text search on sources and assertions.

        Uses ts_vector and ts_rank for relevance scoring. Searches across
        Source.raw_content and Assertion.content fields.

        Args:
            query: Natural language search query
            tenant_id: Tenant ID for multi-tenant isolation
            content_types: Optional list of content types to filter
                          (e.g., ['email', 'meeting', 'document'])
            limit: Maximum number of results to return

        Returns:
            List of (entity_id, rank_score) tuples sorted by relevance.
            Entity IDs are from the sources table.

        Raises:
            SQLAlchemyError: Database operation error
        """
        try:
            # Build the full-text search query for sources
            source_query = text("""
                SELECT
                    s.id as entity_id,
                    ts_rank(
                        to_tsvector('english', COALESCE(s.raw_content, '')),
                        plainto_tsquery('english', :query)
                    ) as rank_score
                FROM sources s
                WHERE s.tenant_id = :tenant_id
                    AND s.is_deleted = false
                    AND to_tsvector('english', COALESCE(s.raw_content, ''))
                        @@ plainto_tsquery('english', :query)
                    AND (:content_types IS NULL OR s.content_type = ANY(:content_types))
                ORDER BY rank_score DESC
                LIMIT :limit
            """)

            result = await self.session.execute(
                source_query,
                {
                    "query": query,
                    "tenant_id": str(tenant_id),
                    "content_types": content_types,
                    "limit": limit,
                }
            )

            results = [(row.entity_id, float(row.rank_score)) for row in result]

            logger.debug(
                f"Full-text search for '{query}' returned {len(results)} results"
            )
            return results

        except SQLAlchemyError as e:
            logger.error(f"Full-text search failed: {e}")
            raise

    async def full_text_search_assertions(
        self,
        query: str,
        tenant_id: UUID,
        assertion_types: list[str] | None = None,
        limit: int = 50,
    ) -> list[tuple[int, float]]:
        """Execute PostgreSQL full-text search on assertions.

        Args:
            query: Natural language search query
            tenant_id: Tenant ID for multi-tenant isolation
            assertion_types: Optional list of assertion types to filter
            limit: Maximum number of results to return

        Returns:
            List of (assertion_id, rank_score) tuples sorted by relevance.

        Raises:
            SQLAlchemyError: Database operation error
        """
        try:
            assertion_query = text("""
                SELECT
                    a.id as entity_id,
                    ts_rank(
                        to_tsvector('english', COALESCE(a.content, '') || ' ' ||
                                               COALESCE(a.context, '')),
                        plainto_tsquery('english', :query)
                    ) as rank_score
                FROM assertions a
                WHERE a.tenant_id = :tenant_id
                    AND a.is_deleted = false
                    AND to_tsvector('english', COALESCE(a.content, '') || ' ' ||
                                               COALESCE(a.context, ''))
                        @@ plainto_tsquery('english', :query)
                    AND (:assertion_types IS NULL OR
                         a.assertion_type = ANY(:assertion_types))
                ORDER BY rank_score DESC
                LIMIT :limit
            """)

            result = await self.session.execute(
                assertion_query,
                {
                    "query": query,
                    "tenant_id": str(tenant_id),
                    "assertion_types": assertion_types,
                    "limit": limit,
                }
            )

            results = [(row.entity_id, float(row.rank_score)) for row in result]

            logger.debug(
                f"Full-text assertion search for '{query}' returned {len(results)} results"
            )
            return results

        except SQLAlchemyError as e:
            logger.error(f"Full-text assertion search failed: {e}")
            raise

    # =========================================================================
    # VECTOR SIMILARITY SEARCH
    # =========================================================================

    async def vector_similarity_search(
        self,
        query_vector: list[float],
        tenant_id: UUID,
        content_types: list[str] | None = None,
        limit: int = 50,
    ) -> list[tuple[int, float]]:
        """Execute pgvector similarity search on embeddings.

        Uses cosine distance (1 - cosine_similarity) for similarity scoring.
        Returns results from the embeddings table linked to sources.

        Args:
            query_vector: 768-dimensional query embedding vector
            tenant_id: Tenant ID for multi-tenant isolation
            content_types: Optional list of entity types to filter
                          (e.g., ['source', 'assertion'])
            limit: Maximum number of results to return

        Returns:
            List of (entity_id, similarity_score) tuples sorted by similarity.
            Similarity score is 1 - cosine_distance (higher is more similar).

        Raises:
            SQLAlchemyError: Database operation error
            ValueError: If query_vector dimension is incorrect
        """
        expected_dimension = 768
        if len(query_vector) != expected_dimension:
            raise ValueError(
                f"Expected {expected_dimension}-dimensional vector, "
                f"got {len(query_vector)}-dimensional"
            )

        try:
            # Use pgvector's <=> operator for cosine distance
            # Similarity = 1 - distance
            vector_query = text("""
                SELECT
                    e.entity_id,
                    1 - (e.embedding <=> :query_vector::vector) as similarity_score
                FROM embeddings e
                WHERE e.tenant_id = :tenant_id
                    AND (:content_types IS NULL OR
                         e.entity_type = ANY(:content_types))
                ORDER BY e.embedding <=> :query_vector::vector
                LIMIT :limit
            """)

            result = await self.session.execute(
                vector_query,
                {
                    "query_vector": str(query_vector),
                    "tenant_id": str(tenant_id),
                    "content_types": content_types,
                    "limit": limit,
                }
            )

            results = [(row.entity_id, float(row.similarity_score)) for row in result]

            logger.debug(
                f"Vector similarity search returned {len(results)} results"
            )
            return results

        except SQLAlchemyError as e:
            logger.error(f"Vector similarity search failed: {e}")
            raise

    async def hybrid_search(
        self,
        query_text: str,
        query_vector: list[float],
        tenant_id: UUID,
        content_types: list[str] | None = None,
        limit: int = 50,
        fts_weight: float = 0.3,
        vector_weight: float = 0.7,
    ) -> list[tuple[int, float]]:
        """Execute hybrid search combining full-text and vector similarity.

        Uses Reciprocal Rank Fusion (RRF) to combine results from both
        search methods.

        Note:
            This is a convenience method that combines full_text_search and
            vector_similarity_search with RRF fusion. For more control over
            the fusion process (e.g., custom RRF k parameter, different fusion
            strategies, or additional ranking signals), use the individual
            search methods with the RRFFusion class from penf_lib.search.ranking.

        Args:
            query_text: Natural language search query for full-text search
            query_vector: Query embedding vector for similarity search
            tenant_id: Tenant ID for multi-tenant isolation
            content_types: Optional list of content types to filter
            limit: Maximum number of results to return
            fts_weight: Weight for full-text search scores (0.0 to 1.0)
            vector_weight: Weight for vector similarity scores (0.0 to 1.0)

        Returns:
            List of (entity_id, combined_score) tuples sorted by relevance.

        Raises:
            SQLAlchemyError: Database operation error
        """
        # Get results from both search methods
        fts_results = await self.full_text_search(
            query=query_text,
            tenant_id=tenant_id,
            content_types=content_types,
            limit=limit * 2,  # Get more for fusion
        )

        vector_results = await self.vector_similarity_search(
            query_vector=query_vector,
            tenant_id=tenant_id,
            content_types=["source"] if content_types else None,
            limit=limit * 2,
        )

        # Apply Reciprocal Rank Fusion (RRF)
        # RRF score = sum(1 / (k + rank)) where k is typically 60
        k = 60
        combined_scores: dict[int, float] = {}

        # Add FTS scores with RRF
        for rank, (entity_id, _score) in enumerate(fts_results, 1):
            rrf_score = fts_weight * (1.0 / (k + rank))
            combined_scores[entity_id] = combined_scores.get(entity_id, 0) + rrf_score

        # Add vector scores with RRF
        for rank, (entity_id, _score) in enumerate(vector_results, 1):
            rrf_score = vector_weight * (1.0 / (k + rank))
            combined_scores[entity_id] = combined_scores.get(entity_id, 0) + rrf_score

        # Sort by combined score and limit
        sorted_results = sorted(
            combined_scores.items(),
            key=lambda x: x[1],
            reverse=True
        )[:limit]

        logger.debug(
            f"Hybrid search returned {len(sorted_results)} results "
            f"(FTS: {len(fts_results)}, Vector: {len(vector_results)})"
        )

        return sorted_results

    # =========================================================================
    # SESSION MANAGEMENT
    # =========================================================================

    async def create_session(
        self,
        tenant_id: UUID,
        user_email: str,
        expires_hours: int = 24,
    ) -> dict[str, Any]:
        """Create a new search session.

        Args:
            tenant_id: Tenant ID for multi-tenant isolation
            user_email: User email for the session
            expires_hours: Number of hours until session expires

        Returns:
            Dictionary representing the SearchSession with keys:
            - id: Session UUID
            - session_id: External session identifier
            - user_email: User's email
            - is_active: Whether session is active
            - started_at: Session start timestamp
            - expires_at: Session expiration timestamp
            - total_queries: Query counter (starts at 0)

        Raises:
            SQLAlchemyError: Database operation error
        """
        try:
            session_uuid = uuid.uuid4()
            session_id = f"search-{session_uuid.hex[:16]}"
            now = datetime.now(UTC)
            expires_at = now + timedelta(hours=expires_hours)

            insert_query = text("""
                INSERT INTO search_sessions (
                    id, tenant_id, session_id, user_email,
                    is_active, started_at, last_activity_at, expires_at,
                    session_context, total_queries, successful_searches,
                    created_at, updated_at
                ) VALUES (
                    :id, :tenant_id, :session_id, :user_email,
                    true, :started_at, :started_at, :expires_at,
                    :session_context, 0, 0,
                    :created_at, :updated_at
                )
                RETURNING id, session_id, user_email, is_active,
                          started_at, expires_at, total_queries
            """)

            result = await self.session.execute(
                insert_query,
                {
                    "id": str(session_uuid),
                    "tenant_id": str(tenant_id),
                    "session_id": session_id,
                    "user_email": user_email,
                    "started_at": now,
                    "expires_at": expires_at,
                    "session_context": "{}",
                    "created_at": now,
                    "updated_at": now,
                }
            )

            row = result.fetchone()

            logger.info(f"Created search session {session_id} for {user_email}")

            return {
                "id": session_uuid,
                "session_id": row.session_id,
                "user_email": row.user_email,
                "is_active": row.is_active,
                "started_at": row.started_at,
                "expires_at": row.expires_at,
                "total_queries": row.total_queries,
            }

        except SQLAlchemyError as e:
            logger.error(f"Failed to create search session: {e}")
            raise

    async def get_active_session(
        self,
        tenant_id: UUID,
        user_email: str,
    ) -> dict[str, Any] | None:
        """Get the active search session for a user.

        Args:
            tenant_id: Tenant ID for multi-tenant isolation
            user_email: User email to find session for

        Returns:
            Dictionary representing the SearchSession or None if not found.

        Raises:
            SQLAlchemyError: Database operation error
        """
        try:
            now = datetime.now(UTC)

            select_query = text("""
                SELECT id, session_id, user_email, is_active,
                       started_at, last_activity_at, expires_at,
                       session_context, total_queries, successful_searches
                FROM search_sessions
                WHERE tenant_id = :tenant_id
                    AND user_email = :user_email
                    AND is_active = true
                    AND expires_at > :now
                ORDER BY last_activity_at DESC
                LIMIT 1
            """)

            result = await self.session.execute(
                select_query,
                {
                    "tenant_id": str(tenant_id),
                    "user_email": user_email,
                    "now": now,
                }
            )

            row = result.fetchone()

            if not row:
                return None

            return {
                "id": UUID(row.id) if isinstance(row.id, str) else row.id,
                "session_id": row.session_id,
                "user_email": row.user_email,
                "is_active": row.is_active,
                "started_at": row.started_at,
                "last_activity_at": row.last_activity_at,
                "expires_at": row.expires_at,
                "session_context": row.session_context,
                "total_queries": row.total_queries,
                "successful_searches": row.successful_searches,
            }

        except SQLAlchemyError as e:
            logger.error(f"Failed to get active session: {e}")
            raise

    async def update_session_activity(self, session_id: UUID) -> None:
        """Update the last activity timestamp for a session.

        Args:
            session_id: UUID of the session to update

        Raises:
            SQLAlchemyError: Database operation error
        """
        try:
            now = datetime.now(UTC)

            update_query = text("""
                UPDATE search_sessions
                SET last_activity_at = :now,
                    updated_at = :now
                WHERE id = :session_id
            """)

            await self.session.execute(
                update_query,
                {
                    "session_id": str(session_id),
                    "now": now,
                }
            )

            logger.debug(f"Updated activity for session {session_id}")

        except SQLAlchemyError as e:
            logger.error(f"Failed to update session activity: {e}")
            raise

    async def close_session(self, session_id: UUID) -> None:
        """Close a search session.

        Args:
            session_id: UUID of the session to close

        Raises:
            SQLAlchemyError: Database operation error
        """
        try:
            now = datetime.now(UTC)

            update_query = text("""
                UPDATE search_sessions
                SET is_active = false,
                    last_activity_at = :now,
                    updated_at = :now
                WHERE id = :session_id
            """)

            await self.session.execute(
                update_query,
                {
                    "session_id": str(session_id),
                    "now": now,
                }
            )

            logger.info(f"Closed search session {session_id}")

        except SQLAlchemyError as e:
            logger.error(f"Failed to close session: {e}")
            raise

    # =========================================================================
    # QUERY RECORDING
    # =========================================================================

    async def record_query(
        self,
        session_id: UUID,
        tenant_id: UUID,
        query_text: str,
        filters: dict[str, Any],
        execution_time_ms: int,
        result_count: int,
        cache_hit: bool,
        search_strategy: str,
    ) -> dict[str, Any]:
        """Record a search query for history and analytics.

        Args:
            session_id: UUID of the search session
            tenant_id: Tenant ID for multi-tenant isolation
            query_text: The search query text
            filters: Dictionary of applied filters
            execution_time_ms: Query execution time in milliseconds
            result_count: Number of results returned
            cache_hit: Whether the query was served from cache
            search_strategy: Search strategy used ('hybrid', 'full_text', 'semantic')

        Returns:
            Dictionary representing the SearchQueryRecord with keys:
            - id: Query record UUID
            - query_text: The search query
            - execution_time_ms: Execution time
            - result_count: Result count
            - cache_hit: Cache hit indicator
            - search_strategy: Strategy used
            - created_at: Record creation timestamp

        Raises:
            SQLAlchemyError: Database operation error
        """
        import hashlib
        import json

        try:
            query_id = uuid.uuid4()
            now = datetime.now(UTC)

            # Generate query hash for caching
            query_hash = hashlib.sha256(query_text.lower().encode()).hexdigest()

            # Normalize query (lowercase, strip whitespace)
            normalized_query = " ".join(query_text.lower().split())

            insert_query = text("""
                INSERT INTO search_query_records (
                    id, session_id, tenant_id,
                    query_text, query_hash, normalized_query,
                    filters, execution_time_ms, result_count,
                    cache_hit, search_strategy,
                    created_at, updated_at
                ) VALUES (
                    :id, :session_id, :tenant_id,
                    :query_text, :query_hash, :normalized_query,
                    :filters, :execution_time_ms, :result_count,
                    :cache_hit, :search_strategy,
                    :created_at, :updated_at
                )
                RETURNING id, query_text, execution_time_ms, result_count,
                          cache_hit, search_strategy, created_at
            """)

            result = await self.session.execute(
                insert_query,
                {
                    "id": str(query_id),
                    "session_id": str(session_id),
                    "tenant_id": str(tenant_id),
                    "query_text": query_text,
                    "query_hash": query_hash,
                    "normalized_query": normalized_query,
                    "filters": json.dumps(filters),
                    "execution_time_ms": execution_time_ms,
                    "result_count": result_count,
                    "cache_hit": cache_hit,
                    "search_strategy": search_strategy,
                    "created_at": now,
                    "updated_at": now,
                }
            )

            row = result.fetchone()

            # Update session query count
            await self.session.execute(
                text("""
                    UPDATE search_sessions
                    SET total_queries = total_queries + 1,
                        last_activity_at = :now,
                        updated_at = :now
                    WHERE id = :session_id
                """),
                {"session_id": str(session_id), "now": now}
            )

            logger.debug(f"Recorded query '{query_text[:50]}...' ({execution_time_ms}ms)")

            return {
                "id": query_id,
                "query_text": row.query_text,
                "execution_time_ms": row.execution_time_ms,
                "result_count": row.result_count,
                "cache_hit": row.cache_hit,
                "search_strategy": row.search_strategy,
                "created_at": row.created_at,
            }

        except SQLAlchemyError as e:
            logger.error(f"Failed to record query: {e}")
            raise

    async def get_recent_queries(
        self,
        tenant_id: UUID,
        limit: int = 10,
    ) -> list[dict[str, Any]]:
        """Get recent queries for a tenant.

        Args:
            tenant_id: Tenant ID for multi-tenant isolation
            limit: Maximum number of queries to return

        Returns:
            List of dictionaries representing SearchQueryRecords.

        Raises:
            SQLAlchemyError: Database operation error
        """
        try:
            select_query = text("""
                SELECT id, session_id, query_text, normalized_query,
                       filters, execution_time_ms, result_count,
                       cache_hit, search_strategy, created_at
                FROM search_query_records
                WHERE tenant_id = :tenant_id
                ORDER BY created_at DESC
                LIMIT :limit
            """)

            result = await self.session.execute(
                select_query,
                {
                    "tenant_id": str(tenant_id),
                    "limit": limit,
                }
            )

            queries = []
            for row in result:
                queries.append({
                    "id": UUID(row.id) if isinstance(row.id, str) else row.id,
                    "session_id": UUID(row.session_id) if isinstance(row.session_id, str) else row.session_id,
                    "query_text": row.query_text,
                    "normalized_query": row.normalized_query,
                    "filters": row.filters,
                    "execution_time_ms": row.execution_time_ms,
                    "result_count": row.result_count,
                    "cache_hit": row.cache_hit,
                    "search_strategy": row.search_strategy,
                    "created_at": row.created_at,
                })

            return queries

        except SQLAlchemyError as e:
            logger.error(f"Failed to get recent queries: {e}")
            raise

    # =========================================================================
    # SUGGESTIONS
    # =========================================================================

    async def get_suggestions(
        self,
        tenant_id: UUID,
        prefix: str | None = None,
        limit: int = 10,
    ) -> list[dict[str, Any]]:
        """Get query suggestions for autocomplete.

        Args:
            tenant_id: Tenant ID for multi-tenant isolation
            prefix: Optional prefix to filter suggestions
            limit: Maximum number of suggestions to return

        Returns:
            List of dictionaries representing QuerySuggestions with keys:
            - id: Suggestion ID
            - suggestion_text: The suggested query text
            - suggestion_type: Type of suggestion ('popular', 'recent', 'contextual')
            - frequency: How often this suggestion has been used
            - success_rate: Rate of successful searches with this suggestion
            - last_used_at: When the suggestion was last used

        Raises:
            SQLAlchemyError: Database operation error
        """
        try:
            if prefix:
                # Prefix-based search for autocomplete
                select_query = text("""
                    SELECT id, suggestion_text, suggestion_type,
                           frequency, success_rate, last_used_at,
                           context_tags, content_types
                    FROM query_suggestions
                    WHERE tenant_id = :tenant_id
                        AND suggestion_text ILIKE :prefix
                    ORDER BY frequency DESC, success_rate DESC NULLS LAST
                    LIMIT :limit
                """)

                result = await self.session.execute(
                    select_query,
                    {
                        "tenant_id": str(tenant_id),
                        "prefix": f"{prefix}%",
                        "limit": limit,
                    }
                )
            else:
                # Get top suggestions by frequency
                select_query = text("""
                    SELECT id, suggestion_text, suggestion_type,
                           frequency, success_rate, last_used_at,
                           context_tags, content_types
                    FROM query_suggestions
                    WHERE tenant_id = :tenant_id
                    ORDER BY frequency DESC, success_rate DESC NULLS LAST
                    LIMIT :limit
                """)

                result = await self.session.execute(
                    select_query,
                    {
                        "tenant_id": str(tenant_id),
                        "limit": limit,
                    }
                )

            suggestions = []
            for row in result:
                suggestions.append({
                    "id": row.id,
                    "suggestion_text": row.suggestion_text,
                    "suggestion_type": row.suggestion_type,
                    "frequency": row.frequency,
                    "success_rate": float(row.success_rate) if row.success_rate else None,
                    "last_used_at": row.last_used_at,
                    "context_tags": row.context_tags,
                    "content_types": row.content_types,
                })

            return suggestions

        except SQLAlchemyError as e:
            logger.error(f"Failed to get suggestions: {e}")
            raise

    async def update_suggestion_frequency(
        self,
        tenant_id: UUID,
        suggestion_text: str,
    ) -> None:
        """Update the frequency counter for a suggestion.

        If the suggestion doesn't exist, it will be created.

        Args:
            tenant_id: Tenant ID for multi-tenant isolation
            suggestion_text: The suggestion text to update

        Raises:
            SQLAlchemyError: Database operation error
        """
        try:
            now = datetime.now(UTC)

            # Try to update existing suggestion
            update_query = text("""
                UPDATE query_suggestions
                SET frequency = frequency + 1,
                    last_used_at = :now,
                    updated_at = :now
                WHERE tenant_id = :tenant_id
                    AND suggestion_text = :suggestion_text
                RETURNING id
            """)

            result = await self.session.execute(
                update_query,
                {
                    "tenant_id": str(tenant_id),
                    "suggestion_text": suggestion_text,
                    "now": now,
                }
            )

            if result.fetchone():
                logger.debug(f"Updated suggestion frequency: {suggestion_text[:50]}...")
                return

            # If no rows updated, insert new suggestion
            insert_query = text("""
                INSERT INTO query_suggestions (
                    tenant_id, suggestion_text, suggestion_type,
                    frequency, last_used_at, created_at, updated_at
                ) VALUES (
                    :tenant_id, :suggestion_text, 'popular',
                    1, :now, :now, :now
                )
                ON CONFLICT (tenant_id, suggestion_text) DO UPDATE
                SET frequency = query_suggestions.frequency + 1,
                    last_used_at = :now,
                    updated_at = :now
            """)

            await self.session.execute(
                insert_query,
                {
                    "tenant_id": str(tenant_id),
                    "suggestion_text": suggestion_text,
                    "now": now,
                }
            )

            logger.debug(f"Created new suggestion: {suggestion_text[:50]}...")

        except SQLAlchemyError as e:
            logger.error(f"Failed to update suggestion frequency: {e}")
            raise

    async def update_suggestion_success_rate(
        self,
        tenant_id: UUID,
        suggestion_text: str,
        was_successful: bool,
    ) -> None:
        """Update the success rate for a suggestion based on search outcome.

        Uses exponential moving average to track success rate.

        Args:
            tenant_id: Tenant ID for multi-tenant isolation
            suggestion_text: The suggestion text to update
            was_successful: Whether the search was successful

        Raises:
            SQLAlchemyError: Database operation error
        """
        try:
            alpha = 0.3  # EMA smoothing factor
            success_value = Decimal("1.0") if was_successful else Decimal("0.0")

            # Update with exponential moving average
            update_query = text("""
                UPDATE query_suggestions
                SET success_rate = COALESCE(
                    (1 - :alpha) * success_rate + :alpha * :success_value,
                    :success_value
                ),
                    updated_at = :now
                WHERE tenant_id = :tenant_id
                    AND suggestion_text = :suggestion_text
            """)

            await self.session.execute(
                update_query,
                {
                    "tenant_id": str(tenant_id),
                    "suggestion_text": suggestion_text,
                    "alpha": alpha,
                    "success_value": success_value,
                    "now": datetime.now(UTC),
                }
            )

        except SQLAlchemyError as e:
            logger.error(f"Failed to update suggestion success rate: {e}")
            raise

    # =========================================================================
    # ANALYTICS HELPERS
    # =========================================================================

    async def get_search_statistics(
        self,
        tenant_id: UUID,
        days: int = 7,
    ) -> dict[str, Any]:
        """Get search usage statistics for a tenant.

        Args:
            tenant_id: Tenant ID for multi-tenant isolation
            days: Number of days to analyze

        Returns:
            Dictionary with search statistics including:
            - total_queries: Total number of queries
            - avg_execution_time_ms: Average execution time
            - cache_hit_rate: Percentage of cache hits
            - avg_result_count: Average number of results
            - strategy_distribution: Usage by search strategy

        Raises:
            SQLAlchemyError: Database operation error
        """
        try:
            since = datetime.now(UTC) - timedelta(days=days)

            stats_query = text("""
                SELECT
                    COUNT(*) as total_queries,
                    AVG(execution_time_ms) as avg_execution_time_ms,
                    AVG(CASE WHEN cache_hit THEN 1.0 ELSE 0.0 END) as cache_hit_rate,
                    AVG(result_count) as avg_result_count,
                    COUNT(CASE WHEN result_count = 0 THEN 1 END) as zero_result_queries
                FROM search_query_records
                WHERE tenant_id = :tenant_id
                    AND created_at >= :since
            """)

            result = await self.session.execute(
                stats_query,
                {
                    "tenant_id": str(tenant_id),
                    "since": since,
                }
            )

            row = result.fetchone()

            # Get strategy distribution
            strategy_query = text("""
                SELECT search_strategy, COUNT(*) as count
                FROM search_query_records
                WHERE tenant_id = :tenant_id
                    AND created_at >= :since
                GROUP BY search_strategy
            """)

            strategy_result = await self.session.execute(
                strategy_query,
                {
                    "tenant_id": str(tenant_id),
                    "since": since,
                }
            )

            strategy_distribution = {
                strategy_row.search_strategy: strategy_row.count
                for strategy_row in strategy_result
            }

            return {
                "total_queries": row.total_queries or 0,
                "avg_execution_time_ms": float(row.avg_execution_time_ms or 0),
                "cache_hit_rate": float(row.cache_hit_rate or 0),
                "avg_result_count": float(row.avg_result_count or 0),
                "zero_result_queries": row.zero_result_queries or 0,
                "strategy_distribution": strategy_distribution,
                "period_days": days,
            }

        except SQLAlchemyError as e:
            logger.error(f"Failed to get search statistics: {e}")
            raise

    async def cleanup_expired_sessions(
        self,
        tenant_id: UUID | None = None,
    ) -> int:
        """Clean up expired search sessions.

        Args:
            tenant_id: Optional tenant ID to limit cleanup scope

        Returns:
            Number of sessions cleaned up

        Raises:
            SQLAlchemyError: Database operation error
        """
        try:
            now = datetime.now(UTC)

            if tenant_id:
                cleanup_query = text("""
                    UPDATE search_sessions
                    SET is_active = false,
                        updated_at = :now
                    WHERE tenant_id = :tenant_id
                        AND is_active = true
                        AND expires_at < :now
                """)

                result = await self.session.execute(
                    cleanup_query,
                    {
                        "tenant_id": str(tenant_id),
                        "now": now,
                    }
                )
            else:
                cleanup_query = text("""
                    UPDATE search_sessions
                    SET is_active = false,
                        updated_at = :now
                    WHERE is_active = true
                        AND expires_at < :now
                """)

                result = await self.session.execute(
                    cleanup_query,
                    {"now": now}
                )

            count = result.rowcount
            if count > 0:
                logger.info(f"Cleaned up {count} expired search sessions")

            return count

        except SQLAlchemyError as e:
            logger.error(f"Failed to cleanup expired sessions: {e}")
            raise

"""Search engine orchestrating hybrid search operations.

This module provides the main SearchEngine class that coordinates:
- Query parsing and validation
- Hybrid search (full-text + vector)
- Result ranking and filtering
- Caching integration
- Analytics recording
"""

from __future__ import annotations

import asyncio
import logging
import uuid
from datetime import datetime
from typing import Optional, List, TYPE_CHECKING

from sqlalchemy.ext.asyncio import AsyncSession

from penf_lib.search.models import (
    SearchQuery,
    SearchResponse,
    SearchResult,
    SearchMetadata,
    ContentPreview,
    ContentTypeFilter,
)
from penf_lib.search.cache import SearchCacheManager
from penf_lib.search.query_parser import QueryParser, QueryEmbedder
from penf_lib.search.ranking import RRFFusion, SearchRanker, RankedResult

if TYPE_CHECKING:
    from penf_lib.storage.repositories.search import SearchRepository

logger = logging.getLogger(__name__)


class SearchEngine:
    """Main search engine coordinating hybrid search operations.

    Implements the search workflow:
    1. Check cache for existing results
    2. Parse query and extract temporal constraints
    3. Execute hybrid search (full-text + vector in parallel)
    4. Apply RRF fusion to combine results
    5. Apply filters and ranking
    6. Cache and return results

    Attributes:
        session: SQLAlchemy async session
        repository: Search repository for database operations
        cache_manager: Cache manager for query and embedding caching
        tenant_id: Current tenant context
    """

    def __init__(
        self,
        session: AsyncSession,
        tenant_id: str,
        cache_manager: Optional[SearchCacheManager] = None,
    ):
        """Initialize search engine.

        Args:
            session: SQLAlchemy async session for database operations
            tenant_id: Tenant UUID for multi-tenant isolation
            cache_manager: Optional cache manager (creates default if None)
        """
        self.session = session
        self.tenant_id = tenant_id
        self.cache_manager = cache_manager or SearchCacheManager()
        self._repository: Optional[SearchRepository] = None

        # Initialize query processing components
        self.parser = QueryParser()
        self.embedder = QueryEmbedder()
        self.fusion = RRFFusion(k=60)
        self.ranker = SearchRanker(
            personalization_weight=0.1,
            recency_weight=0.1,
            recency_half_life_days=30,
        )

    @property
    def repository(self) -> SearchRepository:
        """Lazy-load search repository."""
        if self._repository is None:
            from penf_lib.storage.repositories.search import SearchRepository
            self._repository = SearchRepository(self.session)
        return self._repository

    async def search(self, query: SearchQuery) -> SearchResponse:
        """Execute hybrid search and return ranked results.

        This is the main entry point for search operations.

        Args:
            query: Validated search query parameters

        Returns:
            SearchResponse with results, metadata, and suggestions
        """
        start_time = datetime.utcnow()
        query_id = str(uuid.uuid4())

        # 1. Check cache first
        cached = await self.cache_manager.get_cached_results(self.tenant_id, query)
        if cached:
            cached.metadata.cache_hit = True
            return cached

        # 2. Parse and normalize query
        normalized_query, extracted_filters = self.parser.parse(query.query_text)

        # 3. Generate query embedding
        try:
            query_vector = await self.embedder.embed(normalized_query)
        except Exception as e:
            logger.warning(f"Embedding generation failed, falling back to FTS only: {e}")
            query_vector = None

        # 4. Execute hybrid search (parallel full-text + vector)
        content_type_values = None
        if query.content_types and ContentTypeFilter.ALL not in query.content_types:
            content_type_values = [ct.value for ct in query.content_types]

        # Fetch more results than needed for better ranking
        fetch_limit = query.limit * 2

        if query_vector is not None:
            # Execute both searches in parallel
            fts_task = self.repository.full_text_search(
                normalized_query,
                tenant_id=self.tenant_id,
                content_types=content_type_values,
                limit=fetch_limit,
            )
            vector_task = self.repository.vector_similarity_search(
                query_vector,
                tenant_id=self.tenant_id,
                content_types=content_type_values,
                limit=fetch_limit,
            )

            fts_results, vector_results = await asyncio.gather(fts_task, vector_task)
        else:
            # Vector embedding failed, use FTS only
            fts_results = await self.repository.full_text_search(
                normalized_query,
                tenant_id=self.tenant_id,
                content_types=content_type_values,
                limit=fetch_limit,
            )
            vector_results = []

        # 5. Apply RRF fusion
        fused = self.fusion.fuse(fts_results, vector_results)
        rrf_scores = dict(fused)

        # 6. Fetch full entities for top results
        top_ids = [entity_id for entity_id, _ in fused[:fetch_limit]]
        results = await self._fetch_results(top_ids, rrf_scores)

        # 7. Rank results with personalization
        ranked = self.ranker.rank_results(
            results,
            rrf_scores,
            frequent_contacts=[],  # TODO: Get from user context
            preferred_types=[],  # TODO: Get from user context
        )

        # Apply confidence filter if specified
        if query.min_confidence is not None:
            ranked = self.ranker.filter_by_confidence(ranked, query.min_confidence)

        # 8. Apply pagination
        paginated = ranked[query.offset:query.offset + query.limit]

        # 9. Build response
        execution_time = int((datetime.utcnow() - start_time).total_seconds() * 1000)

        # Build filters_applied dict
        filters_applied: dict[str, any] = {}
        if query.content_types:
            filters_applied["content_types"] = [ct.value for ct in query.content_types]
        if extracted_filters:
            filters_applied.update(extracted_filters)

        response = SearchResponse(
            metadata=SearchMetadata(
                query_id=query_id,
                execution_time_ms=execution_time,
                total_results=len(ranked),
                returned_results=len(paginated),
                search_strategy="hybrid" if query_vector is not None else "full_text",
                cache_hit=False,
            ),
            results=[r.result for r in paginated],
            suggestions=[],
            filters_applied=filters_applied,
            has_more=query.offset + query.limit < len(ranked),
            next_offset=query.offset + query.limit if query.offset + query.limit < len(ranked) else None,
        )

        # 10. Cache and return
        await self.cache_manager.cache_results(self.tenant_id, query, response)

        logger.info(
            f"Search completed: query='{query.query_text[:50]}...' "
            f"results={len(ranked)} returned={len(paginated)} time={execution_time}ms"
        )

        return response

    async def _fetch_results(
        self,
        entity_ids: List[int],
        rrf_scores: dict[int, float],
    ) -> List[SearchResult]:
        """Fetch full SearchResult objects for entity IDs.

        Queries the database to build complete SearchResult objects
        including content previews and metadata.

        Args:
            entity_ids: List of source entity IDs to fetch
            rrf_scores: RRF scores by entity ID for relevance scoring

        Returns:
            List of SearchResult objects
        """
        if not entity_ids:
            return []

        from sqlalchemy import text

        # Query sources for the given IDs
        query = text("""
            SELECT
                s.id,
                s.content_type,
                s.raw_content,
                s.source_system,
                s.external_id,
                s.source_timestamp,
                s.ingestion_metadata
            FROM sources s
            WHERE s.id = ANY(:ids)
                AND s.tenant_id = :tenant_id
                AND s.is_deleted = false
        """)

        result = await self.session.execute(
            query,
            {
                "ids": entity_ids,
                "tenant_id": str(self.tenant_id),
            }
        )

        # Build SearchResult objects
        results: List[SearchResult] = []
        rows_by_id = {row.id: row for row in result}

        for entity_id in entity_ids:
            row = rows_by_id.get(entity_id)
            if row is None:
                continue

            rrf_score = rrf_scores.get(entity_id, 0.0)

            # Generate content preview
            content = row.raw_content or ""
            snippet = content[:300] + "..." if len(content) > 300 else content
            title = self._extract_title(row.content_type, row.ingestion_metadata, content)

            # Map content type
            content_type = self._map_content_type(row.content_type)

            # Build source attribution
            source_attribution = f"{row.source_system}://{row.external_id}"

            # Extract participants from metadata
            metadata = row.ingestion_metadata or {}
            participants = self._extract_participants(metadata)

            search_result = SearchResult(
                result_id=str(uuid.uuid4()),
                entity_type="source",
                entity_id=entity_id,
                content_type=content_type,
                preview=ContentPreview(
                    title=title,
                    snippet=snippet,
                    highlight_positions=[],  # TODO: Implement highlighting
                ),
                source_attribution=source_attribution,
                relevance_score=min(1.0, rrf_score * 10),  # Normalize to 0-1
                rrf_score=rrf_score,
                confidence_score=None,  # Set from AI processing if available
                timestamp=row.source_timestamp or datetime.utcnow(),
                participants=participants,
                project_refs=[],
                tags=[],
                related_content_ids=[],
            )

            results.append(search_result)

        return results

    def _extract_title(
        self,
        content_type: str | None,
        metadata: dict | None,
        content: str,
    ) -> str | None:
        """Extract title from content or metadata.

        Args:
            content_type: Type of content
            metadata: Ingestion metadata dict
            content: Raw content text

        Returns:
            Extracted title or None
        """
        if metadata:
            # Check common metadata fields
            for field in ["subject", "title", "name", "filename"]:
                if field in metadata:
                    return str(metadata[field])[:200]

        # Fall back to first line of content for some types
        if content and content_type in ("email", "document"):
            first_line = content.split("\n")[0].strip()
            if first_line and len(first_line) <= 200:
                return first_line

        return None

    def _map_content_type(self, raw_type: str | None) -> ContentTypeFilter:
        """Map raw content type to ContentTypeFilter enum.

        Args:
            raw_type: Raw content type string

        Returns:
            ContentTypeFilter enum value
        """
        if raw_type is None:
            return ContentTypeFilter.ALL

        raw_lower = raw_type.lower()

        if "email" in raw_lower or "gmail" in raw_lower:
            return ContentTypeFilter.EMAIL
        elif "meeting" in raw_lower or "calendar" in raw_lower:
            return ContentTypeFilter.MEETING
        elif "slack" in raw_lower:
            return ContentTypeFilter.SLACK
        elif "document" in raw_lower or "doc" in raw_lower:
            return ContentTypeFilter.DOCUMENT
        else:
            return ContentTypeFilter.ALL

    def _extract_participants(self, metadata: dict | None) -> List[str]:
        """Extract participant emails/names from metadata.

        Args:
            metadata: Ingestion metadata dict

        Returns:
            List of participant identifiers
        """
        if not metadata:
            return []

        participants: List[str] = []

        # Check common participant fields
        for field in ["from", "to", "cc", "participants", "attendees"]:
            if field in metadata:
                value = metadata[field]
                if isinstance(value, str):
                    participants.append(value)
                elif isinstance(value, list):
                    participants.extend(str(v) for v in value)

        # Deduplicate while preserving order
        seen = set()
        unique = []
        for p in participants:
            if p.lower() not in seen:
                seen.add(p.lower())
                unique.append(p)

        return unique[:20]  # Limit to 20 participants

    async def get_suggestions(
        self,
        prefix: Optional[str] = None,
        limit: int = 5,
    ) -> List[str]:
        """Get query suggestions for autocomplete.

        Args:
            prefix: Optional prefix to filter suggestions
            limit: Maximum suggestions to return

        Returns:
            List of suggested query strings
        """
        # TODO: Phase 6 - Implement suggestions
        return []

    async def find_related(
        self,
        content_id: int,
        content_type: str,
        limit: int = 10,
    ) -> List[SearchResult]:
        """Find content related to a given item.

        Args:
            content_id: ID of the source content
            content_type: Type of the source content
            limit: Maximum related items to return

        Returns:
            List of related search results
        """
        # TODO: Phase 5 - Implement correlation discovery
        return []

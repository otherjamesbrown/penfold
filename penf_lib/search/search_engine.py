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
    RelatedContentResponse,
    RelatedItemResponse,
    CorrelationType,
)
from penf_lib.search.cache import SearchCacheManager
from penf_lib.search.query_parser import QueryParser, QueryEmbedder, QueryCorrector
from penf_lib.search.ranking import RRFFusion, SearchRanker, RankedResult
from penf_lib.search.correlations import (
    CorrelationDiscovery,
    RelatedItem,
    CorrelationResult,
    find_related_by_person,
    find_related_by_project,
)
from penf_lib.search.filters import FilterPipeline

if TYPE_CHECKING:
    from penf_lib.storage.repositories.search import SearchRepository

logger = logging.getLogger(__name__)

# Edge case thresholds
MAX_RESULTS_LIMIT = 100  # Maximum results to return for broad queries
BROAD_QUERY_THRESHOLD = 1000  # Results count that triggers broad query suggestions
PAGINATION_WARNING_THRESHOLD = 500  # Warn about pagination at this count


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
        self.corrector = QueryCorrector()  # For spell-check suggestions
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

        Handles edge cases:
        - No results: Provides helpful suggestions for query refinement
        - Broad queries: Limits results and suggests narrowing filters
        - Spelling errors: Suggests corrections via fuzzy matching
        - Missing embeddings: Falls back to FTS-only with partial match marking

        Args:
            query: Validated search query parameters

        Returns:
            SearchResponse with results, metadata, and suggestions
        """
        start_time = datetime.utcnow()
        query_id = str(uuid.uuid4())
        suggestions: List[str] = []
        search_strategy = "hybrid"
        is_partial_match = False

        # Performance timing dict for logging
        timings: dict[str, int] = {}

        # 1. Check cache first
        cache_start = datetime.utcnow()
        cached = await self.cache_manager.get_cached_results(self.tenant_id, query)
        timings["cache_check_ms"] = int((datetime.utcnow() - cache_start).total_seconds() * 1000)

        if cached:
            cached.metadata.cache_hit = True
            logger.info(
                f"Cache HIT: query='{query.query_text[:50]}...' "
                f"cache_check={timings['cache_check_ms']}ms"
            )
            return cached

        logger.debug(f"Cache MISS: query='{query.query_text[:50]}...'")

        # 2. Parse and normalize query
        parse_start = datetime.utcnow()
        normalized_query, extracted_filters, _temporal = self.parser.parse(query.query_text)
        timings["parse_ms"] = int((datetime.utcnow() - parse_start).total_seconds() * 1000)

        # 3. Generate query embedding
        embed_start = datetime.utcnow()
        query_vector = None
        try:
            query_vector = await self.embedder.embed(normalized_query)
            timings["embed_ms"] = int((datetime.utcnow() - embed_start).total_seconds() * 1000)
        except Exception as e:
            timings["embed_ms"] = int((datetime.utcnow() - embed_start).total_seconds() * 1000)
            logger.warning(
                f"Embedding generation failed, falling back to FTS only: {e} "
                f"(embed_time={timings['embed_ms']}ms)"
            )
            search_strategy = "full_text_fallback"
            is_partial_match = True
            suggestions.append("Search used text matching only (semantic search unavailable)")

        # 4. Execute hybrid search (parallel full-text + vector)
        content_type_values = None
        if query.content_types and ContentTypeFilter.ALL not in query.content_types:
            content_type_values = [ct.value for ct in query.content_types]

        # Fetch more results than needed for better ranking, but cap at reasonable limit
        fetch_limit = min(query.limit * 2, MAX_RESULTS_LIMIT * 2)

        search_start = datetime.utcnow()
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
            search_strategy = "hybrid"
        else:
            # Vector embedding failed, use FTS only
            fts_results = await self.repository.full_text_search(
                normalized_query,
                tenant_id=self.tenant_id,
                content_types=content_type_values,
                limit=fetch_limit,
            )
            vector_results = []
            search_strategy = "full_text_fallback"

        timings["search_ms"] = int((datetime.utcnow() - search_start).total_seconds() * 1000)
        logger.debug(
            f"Search executed: fts_results={len(fts_results)} "
            f"vector_results={len(vector_results)} time={timings['search_ms']}ms"
        )

        # 5. Apply RRF fusion
        fusion_start = datetime.utcnow()
        fused = self.fusion.fuse(fts_results, vector_results)
        rrf_scores = dict(fused)
        timings["fusion_ms"] = int((datetime.utcnow() - fusion_start).total_seconds() * 1000)

        # 6. Fetch full entities for top results
        fetch_start = datetime.utcnow()
        top_ids = [entity_id for entity_id, _ in fused[:fetch_limit]]
        results = await self._fetch_results(top_ids, rrf_scores)
        timings["fetch_ms"] = int((datetime.utcnow() - fetch_start).total_seconds() * 1000)

        # 7. Rank results with personalization
        rank_start = datetime.utcnow()
        ranked = self.ranker.rank_results(
            results,
            rrf_scores,
            frequent_contacts=[],  # TODO: Get from user context
            preferred_types=[],  # TODO: Get from user context
        )
        timings["rank_ms"] = int((datetime.utcnow() - rank_start).total_seconds() * 1000)

        # 8. Apply advanced filters using FilterPipeline
        filter_start = datetime.utcnow()
        filter_pipeline = FilterPipeline.from_search_query(query)

        # Extract SearchResults from RankedResults for filtering
        search_results = [r.result for r in ranked]
        pre_filter_count = len(ranked)

        # Apply filter pipeline and get statistics
        if filter_pipeline:
            filtered_results, filter_stats = filter_pipeline.apply(search_results)
            # Rebuild ranked list with only filtered results
            filtered_ids = {r.entity_id for r in filtered_results}
            ranked = [r for r in ranked if r.result.entity_id in filtered_ids]

            logger.info(
                f"Filter pipeline applied: {filter_stats.total_before} -> {filter_stats.total_after} "
                f"({filter_stats.reduction_percentage:.1f}% reduction)"
            )

        timings["filter_ms"] = int((datetime.utcnow() - filter_start).total_seconds() * 1000)

        # === EDGE CASE HANDLING ===

        # Handle no results
        if not ranked:
            search_strategy = "no_results"
            suggestions.extend(
                self.corrector.get_no_results_suggestions(query.query_text)
            )
            logger.info(
                f"No results found for query='{query.query_text[:50]}...' "
                f"(pre_filter={pre_filter_count})"
            )

        # Handle broad queries (many results)
        elif len(ranked) >= BROAD_QUERY_THRESHOLD:
            broad_suggestions = self.corrector.get_broad_query_suggestions(
                query.query_text, len(ranked)
            )
            suggestions.extend(broad_suggestions)

            # Limit results for broad queries
            if len(ranked) > MAX_RESULTS_LIMIT:
                logger.info(
                    f"Broad query detected: {len(ranked)} results, limiting to {MAX_RESULTS_LIMIT}"
                )

        # Add spelling suggestions if query might be misspelled
        # (only if we have results, to avoid duplicate suggestions)
        elif ranked:
            spell_suggestions = self.parser.suggest_corrections(query.query_text)
            if spell_suggestions:
                suggestions.extend(spell_suggestions)

        # Mark partial match results if using fallback
        if is_partial_match:
            for r in ranked:
                # Add a note to results that they're partial matches
                if r.result.tags is None:
                    r.result.tags = []
                if "partial_match" not in r.result.tags:
                    r.result.tags.append("partial_match")

        # 9. Apply pagination with MAX_RESULTS_LIMIT cap
        effective_limit = min(query.limit, MAX_RESULTS_LIMIT)
        paginated = ranked[query.offset:query.offset + effective_limit]

        # Add pagination info if results exceed warning threshold
        total_available = min(len(ranked), MAX_RESULTS_LIMIT)
        if len(ranked) >= PAGINATION_WARNING_THRESHOLD and not any("pagination" in s.lower() for s in suggestions):
            suggestions.append(
                f"Showing {len(paginated)} of {total_available} results. "
                "Use pagination (offset parameter) to see more."
            )

        # 10. Build response
        execution_time = int((datetime.utcnow() - start_time).total_seconds() * 1000)
        timings["total_ms"] = execution_time

        # Build filters_applied dict
        filters_applied: dict[str, any] = {}
        if query.content_types:
            filters_applied["content_types"] = [ct.value for ct in query.content_types]
        if extracted_filters:
            filters_applied.update(extracted_filters)
        if query.participants:
            filters_applied["participants"] = query.participants
        if query.projects:
            filters_applied["projects"] = query.projects
        if query.min_confidence is not None:
            filters_applied["min_confidence"] = query.min_confidence
        if query.temporal:
            if query.temporal.start_date:
                filters_applied["since"] = query.temporal.start_date.isoformat()
            if query.temporal.end_date:
                filters_applied["until"] = query.temporal.end_date.isoformat()

        response = SearchResponse(
            metadata=SearchMetadata(
                query_id=query_id,
                execution_time_ms=execution_time,
                total_results=total_available,
                returned_results=len(paginated),
                search_strategy=search_strategy,
                cache_hit=False,
            ),
            results=[r.result for r in paginated],
            suggestions=suggestions,
            filters_applied=filters_applied,
            has_more=query.offset + effective_limit < total_available,
            next_offset=query.offset + effective_limit if query.offset + effective_limit < total_available else None,
        )

        # 11. Cache and return
        cache_store_start = datetime.utcnow()
        await self.cache_manager.cache_results(self.tenant_id, query, response)
        timings["cache_store_ms"] = int((datetime.utcnow() - cache_store_start).total_seconds() * 1000)

        # 12. Record analytics (fire and forget, don't block response)
        try:
            from penf_lib.search.analytics import AnalyticsCollector

            analytics = AnalyticsCollector(self.session, self.tenant_id)
            await analytics.record_search(
                query_text=query.query_text,
                result_count=len(ranked),
                execution_time_ms=execution_time,
                cache_hit=False,
                search_strategy=response.metadata.search_strategy,
                filters=filters_applied,
            )
        except Exception as analytics_error:
            # Don't fail the search if analytics recording fails
            logger.warning(f"Analytics recording failed: {analytics_error}")

        # Performance logging
        logger.info(
            f"Search completed: query='{query.query_text[:50]}...' "
            f"results={len(ranked)} returned={len(paginated)} "
            f"strategy={search_strategy} time={execution_time}ms | "
            f"timings: cache_check={timings.get('cache_check_ms', 0)}ms "
            f"parse={timings.get('parse_ms', 0)}ms "
            f"embed={timings.get('embed_ms', 0)}ms "
            f"search={timings.get('search_ms', 0)}ms "
            f"fusion={timings.get('fusion_ms', 0)}ms "
            f"fetch={timings.get('fetch_ms', 0)}ms "
            f"rank={timings.get('rank_ms', 0)}ms "
            f"filter={timings.get('filter_ms', 0)}ms "
            f"cache_store={timings.get('cache_store_ms', 0)}ms"
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
    ) -> RelatedContentResponse:
        """Find content related to a given item.

        Uses correlation discovery to find related content through:
        - Shared participants (0.30 weight)
        - Shared project references (0.25 weight)
        - Temporal proximity (0.20 weight)
        - Semantic similarity (0.15 weight)
        - Thread/conversation chains (0.10 weight)

        Args:
            content_id: ID of the source content
            content_type: Type of the source content (source, assertion)
            limit: Maximum related items to return

        Returns:
            RelatedContentResponse with discovered correlations
        """
        start_time = datetime.utcnow()

        # Create correlation discovery instance
        discovery = CorrelationDiscovery(
            session=self.session,
            tenant_id=self.tenant_id,
        )

        # Find related content
        result = await discovery.find_related_content(
            content_id=content_id,
            content_type=content_type,
            limit=limit,
        )

        # Convert to response model
        related_items: List[RelatedItemResponse] = []
        for item in result.related_items:
            # Map correlation type string to enum
            try:
                corr_type = CorrelationType(item.correlation_type)
            except ValueError:
                corr_type = CorrelationType.SEMANTIC

            related_items.append(
                RelatedItemResponse(
                    entity_id=item.entity_id,
                    entity_type=item.entity_type,
                    content_type=item.content_type,
                    title=item.title,
                    snippet=None,  # Could fetch snippets if needed
                    timestamp=item.timestamp,
                    correlation_score=item.correlation_score,
                    correlation_type=corr_type,
                    score_breakdown=item.score_breakdown,
                )
            )

        execution_time = int((datetime.utcnow() - start_time).total_seconds() * 1000)

        logger.info(
            f"find_related: content_id={content_id} type={content_type} "
            f"found={len(related_items)} time={execution_time}ms"
        )

        return RelatedContentResponse(
            entity_type=content_type,
            entity_id=content_id,
            related_content=related_items,
            total_correlations=result.total_correlations,
            execution_time_ms=execution_time,
        )

    async def find_related_by_person(
        self,
        person_identifier: str,
        limit: int = 20,
    ) -> RelatedContentResponse:
        """Find content related to a specific person.

        Args:
            person_identifier: Email or name of person
            limit: Maximum items to return

        Returns:
            RelatedContentResponse with content involving the person
        """
        start_time = datetime.utcnow()

        items = await find_related_by_person(
            session=self.session,
            tenant_id=self.tenant_id,
            person_identifier=person_identifier,
            limit=limit,
        )

        # Convert to response items
        related_items: List[RelatedItemResponse] = []
        for item in items:
            related_items.append(
                RelatedItemResponse(
                    entity_id=item.entity_id,
                    entity_type=item.entity_type,
                    content_type=item.content_type,
                    title=item.title,
                    snippet=None,
                    timestamp=item.timestamp,
                    correlation_score=item.correlation_score,
                    correlation_type=CorrelationType.PARTICIPANT,
                    score_breakdown=item.score_breakdown,
                )
            )

        execution_time = int((datetime.utcnow() - start_time).total_seconds() * 1000)

        return RelatedContentResponse(
            entity_type="person",
            entity_id=0,  # No entity ID for person search
            related_content=related_items,
            total_correlations=len(related_items),
            execution_time_ms=execution_time,
        )

    async def find_related_by_project(
        self,
        project_name: str,
        limit: int = 20,
    ) -> RelatedContentResponse:
        """Find content related to a specific project.

        Args:
            project_name: Name of the project
            limit: Maximum items to return

        Returns:
            RelatedContentResponse with content mentioning the project
        """
        start_time = datetime.utcnow()

        items = await find_related_by_project(
            session=self.session,
            tenant_id=self.tenant_id,
            project_name=project_name,
            limit=limit,
        )

        # Convert to response items
        related_items: List[RelatedItemResponse] = []
        for item in items:
            related_items.append(
                RelatedItemResponse(
                    entity_id=item.entity_id,
                    entity_type=item.entity_type,
                    content_type=item.content_type,
                    title=item.title,
                    snippet=None,
                    timestamp=item.timestamp,
                    correlation_score=item.correlation_score,
                    correlation_type=CorrelationType.PROJECT,
                    score_breakdown=item.score_breakdown,
                )
            )

        execution_time = int((datetime.utcnow() - start_time).total_seconds() * 1000)

        return RelatedContentResponse(
            entity_type="project",
            entity_id=0,  # No entity ID for project search
            related_content=related_items,
            total_correlations=len(related_items),
            execution_time_ms=execution_time,
        )

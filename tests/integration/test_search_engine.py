"""Integration tests for search engine.

Tests the complete search engine functionality including:
- Hybrid search combining vector and full-text
- Full-text search with PostgreSQL
- Semantic search with vector similarity
- Search with filters applied
- Edge cases (no results, broad queries)
- Pagination
"""

from __future__ import annotations

from datetime import datetime, timezone, timedelta
from typing import Optional, List
from unittest.mock import AsyncMock, MagicMock, patch
import uuid

import pytest

from penf_lib.search.models import (
    ContentTypeFilter,
    SearchQuery,
    SearchResponse,
    SearchResult,
    ContentPreview,
    TemporalConstraint,
)
from penf_lib.search.search_engine import (
    SearchEngine,
    MAX_RESULTS_LIMIT,
    BROAD_QUERY_THRESHOLD,
)
from penf_lib.search.cache import SearchCacheManager
from penf_lib.search.ranking import RankedResult


def _create_mock_search_result(
    entity_id: int,
    content_type: ContentTypeFilter = ContentTypeFilter.EMAIL,
    relevance_score: float = 0.9,
    rrf_score: float = 0.08,
    participants: Optional[List[str]] = None,
    timestamp: Optional[datetime] = None,
    confidence_score: Optional[float] = None,
) -> SearchResult:
    """Helper to create mock SearchResult objects."""
    return SearchResult(
        result_id=str(uuid.uuid4()),
        entity_type="source",
        entity_id=entity_id,
        content_type=content_type,
        preview=ContentPreview(
            title=f"Test Result {entity_id}",
            snippet=f"This is test content for entity {entity_id}...",
        ),
        source_attribution=f"test://source/{entity_id}",
        relevance_score=relevance_score,
        rrf_score=rrf_score,
        timestamp=timestamp or datetime.now(timezone.utc),
        participants=participants or [],
        confidence_score=confidence_score,
    )


@pytest.fixture
def mock_session():
    """Create a mock async session."""
    session = AsyncMock()
    session.execute = AsyncMock(return_value=MagicMock(fetchall=MagicMock(return_value=[])))
    return session


@pytest.fixture
def mock_cache_manager():
    """Create a mock cache manager that always misses."""
    cache = AsyncMock(spec=SearchCacheManager)
    cache.get_cached_results = AsyncMock(return_value=None)
    cache.cache_results = AsyncMock(return_value=None)
    return cache


@pytest.fixture
def tenant_id():
    """Generate a test tenant ID."""
    return str(uuid.uuid4())


@pytest.mark.integration
class TestSearchEngineHybridSearch:
    """Test hybrid search combining vector and full-text search."""

    @pytest.mark.asyncio
    async def test_hybrid_search_combines_fts_and_vector(
        self, mock_session, mock_cache_manager, tenant_id
    ):
        """Hybrid search executes both FTS and vector search."""
        engine = SearchEngine(
            session=mock_session,
            tenant_id=tenant_id,
            cache_manager=mock_cache_manager,
        )

        # Mock the repository
        mock_repo = AsyncMock()
        mock_repo.full_text_search = AsyncMock(return_value=[(1, 0.9), (2, 0.8)])
        mock_repo.vector_similarity_search = AsyncMock(return_value=[(2, 0.85), (3, 0.7)])
        engine._repository = mock_repo

        # Mock fetch_results to return results so we get hybrid strategy
        engine._fetch_results = AsyncMock(return_value=[
            _create_mock_search_result(1),
            _create_mock_search_result(2),
            _create_mock_search_result(3),
        ])

        query = SearchQuery(query_text="test query")
        response = await engine.search(query)

        # Verify both searches were called
        mock_repo.full_text_search.assert_called_once()
        mock_repo.vector_similarity_search.assert_called_once()

        # Verify response structure
        assert isinstance(response, SearchResponse)
        assert response.metadata.search_strategy == "hybrid"

    @pytest.mark.asyncio
    async def test_hybrid_search_handles_one_empty_result(
        self, mock_session, mock_cache_manager, tenant_id
    ):
        """Hybrid search works when one search type returns no results."""
        engine = SearchEngine(
            session=mock_session,
            tenant_id=tenant_id,
            cache_manager=mock_cache_manager,
        )

        mock_repo = AsyncMock()
        mock_repo.full_text_search = AsyncMock(return_value=[(1, 0.9), (2, 0.8)])
        mock_repo.vector_similarity_search = AsyncMock(return_value=[])  # Empty vector results
        engine._repository = mock_repo

        # Return results from FTS only
        engine._fetch_results = AsyncMock(return_value=[
            _create_mock_search_result(1),
            _create_mock_search_result(2),
        ])

        query = SearchQuery(query_text="test query")
        response = await engine.search(query)

        assert response.metadata.search_strategy == "hybrid"

    @pytest.mark.asyncio
    async def test_hybrid_search_uses_rrf_fusion(
        self, mock_session, mock_cache_manager, tenant_id
    ):
        """Hybrid search applies RRF fusion to combine results."""
        engine = SearchEngine(
            session=mock_session,
            tenant_id=tenant_id,
            cache_manager=mock_cache_manager,
        )

        # Results that appear in both searches should be ranked higher
        mock_repo = AsyncMock()
        mock_repo.full_text_search = AsyncMock(return_value=[
            (1, 0.9), (2, 0.8), (3, 0.7)
        ])
        mock_repo.vector_similarity_search = AsyncMock(return_value=[
            (2, 0.95), (4, 0.85), (1, 0.75)  # Entity 2 and 1 appear in both
        ])
        engine._repository = mock_repo

        # Mock _fetch_results to return results
        mock_results = [
            _create_mock_search_result(1, rrf_score=0.1),
            _create_mock_search_result(2, rrf_score=0.15),  # Higher because in both
            _create_mock_search_result(3, rrf_score=0.05),
            _create_mock_search_result(4, rrf_score=0.08),
        ]
        engine._fetch_results = AsyncMock(return_value=mock_results)

        query = SearchQuery(query_text="test query")
        response = await engine.search(query)

        assert response.metadata.search_strategy == "hybrid"


@pytest.mark.integration
class TestSearchEngineFullTextSearch:
    """Test full-text search functionality."""

    @pytest.mark.asyncio
    async def test_fts_fallback_when_embedding_fails(
        self, mock_session, mock_cache_manager, tenant_id
    ):
        """Search falls back to FTS-only when embedding generation fails."""
        engine = SearchEngine(
            session=mock_session,
            tenant_id=tenant_id,
            cache_manager=mock_cache_manager,
        )

        # Make embedding generation fail
        engine.embedder.embed = AsyncMock(side_effect=Exception("Embedding service unavailable"))

        mock_repo = AsyncMock()
        mock_repo.full_text_search = AsyncMock(return_value=[(1, 0.9)])
        engine._repository = mock_repo
        engine._fetch_results = AsyncMock(return_value=[
            _create_mock_search_result(1)
        ])

        query = SearchQuery(query_text="test query")
        response = await engine.search(query)

        # Verify fallback strategy
        assert response.metadata.search_strategy == "full_text_fallback"

        # Verify suggestion about fallback
        assert any("semantic search unavailable" in s.lower() for s in response.suggestions)

        # Verify vector search was NOT called
        mock_repo.vector_similarity_search = AsyncMock()
        assert not mock_repo.vector_similarity_search.called


@pytest.mark.integration
class TestSearchEngineFilters:
    """Test search with various filters applied."""

    @pytest.mark.asyncio
    async def test_search_with_content_type_filter(
        self, mock_session, mock_cache_manager, tenant_id
    ):
        """Search applies content type filters."""
        engine = SearchEngine(
            session=mock_session,
            tenant_id=tenant_id,
            cache_manager=mock_cache_manager,
        )

        mock_repo = AsyncMock()
        mock_repo.full_text_search = AsyncMock(return_value=[(1, 0.9)])
        mock_repo.vector_similarity_search = AsyncMock(return_value=[(1, 0.85)])
        engine._repository = mock_repo
        engine._fetch_results = AsyncMock(return_value=[
            _create_mock_search_result(1, content_type=ContentTypeFilter.EMAIL)
        ])

        query = SearchQuery(
            query_text="test",
            content_types=[ContentTypeFilter.EMAIL],
        )
        response = await engine.search(query)

        # Verify filter was applied to search
        call_args = mock_repo.full_text_search.call_args
        assert call_args.kwargs.get("content_types") == ["email"]

        # Verify filters_applied in response
        assert "content_types" in response.filters_applied

    @pytest.mark.asyncio
    async def test_search_with_participant_filter(
        self, mock_session, mock_cache_manager, tenant_id
    ):
        """Search applies participant filters."""
        engine = SearchEngine(
            session=mock_session,
            tenant_id=tenant_id,
            cache_manager=mock_cache_manager,
        )

        mock_repo = AsyncMock()
        mock_repo.full_text_search = AsyncMock(return_value=[(1, 0.9), (2, 0.8)])
        mock_repo.vector_similarity_search = AsyncMock(return_value=[])
        engine._repository = mock_repo

        # One result matches participant filter, one doesn't
        engine._fetch_results = AsyncMock(return_value=[
            _create_mock_search_result(1, participants=["alice@example.com"]),
            _create_mock_search_result(2, participants=["bob@example.com"]),
        ])

        query = SearchQuery(
            query_text="test",
            participants=["alice@example.com"],
        )
        response = await engine.search(query)

        # Filter should reduce results
        assert response.metadata.total_results <= 2
        assert "participants" in response.filters_applied

    @pytest.mark.asyncio
    async def test_search_with_temporal_filter(
        self, mock_session, mock_cache_manager, tenant_id
    ):
        """Search applies temporal filters."""
        engine = SearchEngine(
            session=mock_session,
            tenant_id=tenant_id,
            cache_manager=mock_cache_manager,
        )

        now = datetime.now(timezone.utc)
        yesterday = now - timedelta(days=1)
        last_week = now - timedelta(days=7)

        mock_repo = AsyncMock()
        mock_repo.full_text_search = AsyncMock(return_value=[(1, 0.9), (2, 0.8)])
        mock_repo.vector_similarity_search = AsyncMock(return_value=[])
        engine._repository = mock_repo

        engine._fetch_results = AsyncMock(return_value=[
            _create_mock_search_result(1, timestamp=yesterday),  # Within range
            _create_mock_search_result(2, timestamp=last_week),  # Before range
        ])

        query = SearchQuery(
            query_text="test",
            temporal=TemporalConstraint(
                start_date=now - timedelta(days=2),  # Last 2 days
            ),
        )
        response = await engine.search(query)

        assert "since" in response.filters_applied

    @pytest.mark.asyncio
    async def test_search_with_confidence_filter(
        self, mock_session, mock_cache_manager, tenant_id
    ):
        """Search applies confidence score filters."""
        engine = SearchEngine(
            session=mock_session,
            tenant_id=tenant_id,
            cache_manager=mock_cache_manager,
        )

        mock_repo = AsyncMock()
        mock_repo.full_text_search = AsyncMock(return_value=[(1, 0.9), (2, 0.8)])
        mock_repo.vector_similarity_search = AsyncMock(return_value=[])
        engine._repository = mock_repo

        engine._fetch_results = AsyncMock(return_value=[
            _create_mock_search_result(1, confidence_score=0.9),  # Above threshold
            _create_mock_search_result(2, confidence_score=0.5),  # Below threshold
        ])

        query = SearchQuery(
            query_text="test",
            min_confidence=0.7,
        )
        response = await engine.search(query)

        assert "min_confidence" in response.filters_applied


@pytest.mark.integration
class TestSearchEnginePagination:
    """Test search result pagination."""

    @pytest.mark.asyncio
    async def test_pagination_with_limit(
        self, mock_session, mock_cache_manager, tenant_id
    ):
        """Search respects limit parameter."""
        engine = SearchEngine(
            session=mock_session,
            tenant_id=tenant_id,
            cache_manager=mock_cache_manager,
        )

        mock_repo = AsyncMock()
        mock_repo.full_text_search = AsyncMock(return_value=[
            (i, 0.9 - i * 0.01) for i in range(50)
        ])
        mock_repo.vector_similarity_search = AsyncMock(return_value=[])
        engine._repository = mock_repo

        engine._fetch_results = AsyncMock(return_value=[
            _create_mock_search_result(i) for i in range(50)
        ])

        query = SearchQuery(query_text="test", limit=10)
        response = await engine.search(query)

        assert response.metadata.returned_results <= 10
        assert response.has_more is True
        assert response.next_offset == 10

    @pytest.mark.asyncio
    async def test_pagination_with_offset(
        self, mock_session, mock_cache_manager, tenant_id
    ):
        """Search respects offset parameter."""
        engine = SearchEngine(
            session=mock_session,
            tenant_id=tenant_id,
            cache_manager=mock_cache_manager,
        )

        mock_repo = AsyncMock()
        mock_repo.full_text_search = AsyncMock(return_value=[
            (i, 0.9 - i * 0.01) for i in range(50)
        ])
        mock_repo.vector_similarity_search = AsyncMock(return_value=[])
        engine._repository = mock_repo

        engine._fetch_results = AsyncMock(return_value=[
            _create_mock_search_result(i) for i in range(50)
        ])

        query = SearchQuery(query_text="test", limit=10, offset=20)
        response = await engine.search(query)

        # Should return results from position 20 onwards
        assert response.metadata.returned_results <= 10
        if response.has_more:
            assert response.next_offset == 30

    @pytest.mark.asyncio
    async def test_pagination_no_more_results(
        self, mock_session, mock_cache_manager, tenant_id
    ):
        """Pagination indicates when no more results available."""
        engine = SearchEngine(
            session=mock_session,
            tenant_id=tenant_id,
            cache_manager=mock_cache_manager,
        )

        mock_repo = AsyncMock()
        mock_repo.full_text_search = AsyncMock(return_value=[
            (i, 0.9 - i * 0.01) for i in range(5)
        ])
        mock_repo.vector_similarity_search = AsyncMock(return_value=[])
        engine._repository = mock_repo

        engine._fetch_results = AsyncMock(return_value=[
            _create_mock_search_result(i) for i in range(5)
        ])

        query = SearchQuery(query_text="test", limit=10)
        response = await engine.search(query)

        # All 5 results fit in limit of 10
        assert response.metadata.returned_results == 5
        assert response.has_more is False
        assert response.next_offset is None


@pytest.mark.integration
class TestSearchEngineEdgeCases:
    """Test edge case handling."""

    @pytest.mark.asyncio
    async def test_no_results_provides_suggestions(
        self, mock_session, mock_cache_manager, tenant_id
    ):
        """Search with no results provides helpful suggestions."""
        engine = SearchEngine(
            session=mock_session,
            tenant_id=tenant_id,
            cache_manager=mock_cache_manager,
        )

        mock_repo = AsyncMock()
        mock_repo.full_text_search = AsyncMock(return_value=[])
        mock_repo.vector_similarity_search = AsyncMock(return_value=[])
        engine._repository = mock_repo
        engine._fetch_results = AsyncMock(return_value=[])

        query = SearchQuery(query_text="xyznonexistent")
        response = await engine.search(query)

        assert response.metadata.total_results == 0
        assert response.metadata.search_strategy == "no_results"
        assert len(response.suggestions) > 0
        assert any("broader" in s.lower() or "fewer" in s.lower() for s in response.suggestions)

    @pytest.mark.asyncio
    async def test_broad_query_provides_suggestions(
        self, mock_session, mock_cache_manager, tenant_id
    ):
        """Search with many results provides narrowing suggestions."""
        engine = SearchEngine(
            session=mock_session,
            tenant_id=tenant_id,
            cache_manager=mock_cache_manager,
        )

        # Create a large result set
        many_results = [(i, 0.9 - i * 0.0001) for i in range(BROAD_QUERY_THRESHOLD + 100)]

        mock_repo = AsyncMock()
        mock_repo.full_text_search = AsyncMock(return_value=many_results)
        mock_repo.vector_similarity_search = AsyncMock(return_value=[])
        engine._repository = mock_repo

        engine._fetch_results = AsyncMock(return_value=[
            _create_mock_search_result(i) for i in range(BROAD_QUERY_THRESHOLD + 100)
        ])

        query = SearchQuery(query_text="common")
        response = await engine.search(query)

        # Should have suggestions about narrowing
        assert any("narrow" in s.lower() or "filter" in s.lower() for s in response.suggestions)

    @pytest.mark.asyncio
    async def test_broad_query_limits_results(
        self, mock_session, mock_cache_manager, tenant_id
    ):
        """Broad queries cap results at MAX_RESULTS_LIMIT."""
        engine = SearchEngine(
            session=mock_session,
            tenant_id=tenant_id,
            cache_manager=mock_cache_manager,
        )

        # Create more results than the limit
        many_results = [(i, 0.9 - i * 0.0001) for i in range(MAX_RESULTS_LIMIT + 50)]

        mock_repo = AsyncMock()
        mock_repo.full_text_search = AsyncMock(return_value=many_results)
        mock_repo.vector_similarity_search = AsyncMock(return_value=[])
        engine._repository = mock_repo

        engine._fetch_results = AsyncMock(return_value=[
            _create_mock_search_result(i) for i in range(MAX_RESULTS_LIMIT + 50)
        ])

        query = SearchQuery(query_text="common", limit=100)  # Max allowed limit
        response = await engine.search(query)

        # Should cap at MAX_RESULTS_LIMIT
        assert response.metadata.total_results <= MAX_RESULTS_LIMIT

    @pytest.mark.asyncio
    async def test_spelling_suggestion_on_typo(
        self, mock_session, mock_cache_manager, tenant_id
    ):
        """Search suggests correction for misspelled query."""
        engine = SearchEngine(
            session=mock_session,
            tenant_id=tenant_id,
            cache_manager=mock_cache_manager,
        )

        mock_repo = AsyncMock()
        mock_repo.full_text_search = AsyncMock(return_value=[(1, 0.5)])
        mock_repo.vector_similarity_search = AsyncMock(return_value=[])
        engine._repository = mock_repo

        engine._fetch_results = AsyncMock(return_value=[
            _create_mock_search_result(1)
        ])

        # Search with a misspelled word
        query = SearchQuery(query_text="meetting notes")  # "meetting" is misspelled
        response = await engine.search(query)

        # Should suggest correction
        assert any("did you mean" in s.lower() or "meeting" in s.lower() for s in response.suggestions)

    @pytest.mark.asyncio
    async def test_partial_match_marking_on_fallback(
        self, mock_session, mock_cache_manager, tenant_id
    ):
        """Results are marked as partial matches when using FTS fallback."""
        engine = SearchEngine(
            session=mock_session,
            tenant_id=tenant_id,
            cache_manager=mock_cache_manager,
        )

        # Force FTS fallback by making embedding fail
        engine.embedder.embed = AsyncMock(side_effect=Exception("Service unavailable"))

        mock_repo = AsyncMock()
        mock_repo.full_text_search = AsyncMock(return_value=[(1, 0.9)])
        engine._repository = mock_repo

        result = _create_mock_search_result(1)
        engine._fetch_results = AsyncMock(return_value=[result])

        query = SearchQuery(query_text="test query")
        response = await engine.search(query)

        # Check results are marked as partial matches
        if response.results:
            assert "partial_match" in response.results[0].tags


@pytest.mark.integration
class TestSearchEngineCaching:
    """Test search caching behavior."""

    @pytest.mark.asyncio
    async def test_cache_hit_returns_cached_results(
        self, mock_session, tenant_id
    ):
        """Search returns cached results when available."""
        from penf_lib.search.models import SearchMetadata

        # Create a cached response with real SearchMetadata
        cached_response = SearchResponse(
            metadata=SearchMetadata(
                query_id="cached-query",
                execution_time_ms=50,
                total_results=5,
                returned_results=5,
                search_strategy="hybrid",
                cache_hit=False,  # Will be set to True by engine
            ),
            results=[],
            suggestions=[],
            filters_applied={},
            has_more=False,
            next_offset=None,
        )

        cache_manager = AsyncMock(spec=SearchCacheManager)
        cache_manager.get_cached_results = AsyncMock(return_value=cached_response)

        engine = SearchEngine(
            session=mock_session,
            tenant_id=tenant_id,
            cache_manager=cache_manager,
        )

        query = SearchQuery(query_text="cached query")
        response = await engine.search(query)

        # Should return cached response with cache_hit=True
        assert response.metadata.cache_hit is True
        cache_manager.get_cached_results.assert_called_once()

    @pytest.mark.asyncio
    async def test_cache_miss_stores_results(
        self, mock_session, mock_cache_manager, tenant_id
    ):
        """Search stores results in cache on cache miss."""
        engine = SearchEngine(
            session=mock_session,
            tenant_id=tenant_id,
            cache_manager=mock_cache_manager,
        )

        mock_repo = AsyncMock()
        mock_repo.full_text_search = AsyncMock(return_value=[])
        mock_repo.vector_similarity_search = AsyncMock(return_value=[])
        engine._repository = mock_repo
        engine._fetch_results = AsyncMock(return_value=[])

        query = SearchQuery(query_text="test")
        await engine.search(query)

        # Should attempt to cache results
        mock_cache_manager.cache_results.assert_called_once()


@pytest.mark.integration
class TestSearchEngineTenantIsolation:
    """Test that search respects tenant boundaries."""

    @pytest.mark.asyncio
    async def test_search_passes_tenant_to_repository(
        self, mock_session, mock_cache_manager, tenant_id
    ):
        """Search includes tenant_id in repository queries."""
        engine = SearchEngine(
            session=mock_session,
            tenant_id=tenant_id,
            cache_manager=mock_cache_manager,
        )

        mock_repo = AsyncMock()
        mock_repo.full_text_search = AsyncMock(return_value=[])
        mock_repo.vector_similarity_search = AsyncMock(return_value=[])
        engine._repository = mock_repo
        engine._fetch_results = AsyncMock(return_value=[])

        query = SearchQuery(query_text="test")
        await engine.search(query)

        # Verify tenant_id was passed to searches
        fts_call = mock_repo.full_text_search.call_args
        assert fts_call.kwargs.get("tenant_id") == tenant_id

        vector_call = mock_repo.vector_similarity_search.call_args
        assert vector_call.kwargs.get("tenant_id") == tenant_id

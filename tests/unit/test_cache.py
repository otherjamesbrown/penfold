"""Unit tests for the search cache module.

Tests for QueryCache (Redis-based), EmbeddingCache (in-memory LRU),
and SearchCacheManager (unified interface).

Based on specs/007-search-interface/research.md caching strategy:
- Redis query cache with 5-minute TTL
- In-memory LRU cache for embeddings (10k capacity)
"""

from datetime import datetime
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from penf_lib.search.cache import (
    EmbeddingCache,
    QueryCache,
    SearchCacheManager,
)
from penf_lib.search.models import (
    ContentPreview,
    ContentTypeFilter,
    SearchMetadata,
    SearchQuery,
    SearchResponse,
    SearchResult,
)

# =============================================================================
# FIXTURES
# =============================================================================


@pytest.fixture
def sample_search_query() -> SearchQuery:
    """Create a sample search query for testing."""
    return SearchQuery(
        query_text="customer deployment issues",
        content_types=[ContentTypeFilter.EMAIL, ContentTypeFilter.MEETING],
        limit=25,
    )


@pytest.fixture
def sample_search_response() -> SearchResponse:
    """Create a sample search response for testing."""
    return SearchResponse(
        metadata=SearchMetadata(
            query_id="test-query-123",
            execution_time_ms=150,
            total_results=10,
            returned_results=10,
            search_strategy="hybrid",
            cache_hit=False,
        ),
        results=[
            SearchResult(
                result_id="result-001",
                entity_type="source",
                entity_id=12345,
                content_type=ContentTypeFilter.EMAIL,
                preview=ContentPreview(
                    title="Re: Deployment Issues",
                    snippet="The customer reported issues with the deployment...",
                    highlight_positions=[(4, 12), (35, 45)],
                ),
                source_attribution="gmail://msg/abc123",
                relevance_score=0.92,
                rrf_score=0.85,
                timestamp=datetime.fromisoformat("2026-01-10T14:30:00"),
                participants=["alice@example.com", "bob@example.com"],
            ),
        ],
        has_more=False,
    )


@pytest.fixture
def mock_redis():
    """Create a mock Redis client."""
    redis_mock = AsyncMock()
    redis_mock.get = AsyncMock(return_value=None)
    redis_mock.set = AsyncMock(return_value=True)
    redis_mock.delete = AsyncMock(return_value=1)
    redis_mock.scan = AsyncMock(return_value=(0, []))
    redis_mock.close = AsyncMock()
    return redis_mock


# =============================================================================
# QUERY CACHE TESTS
# =============================================================================


class TestQueryCache:
    """Tests for QueryCache Redis-based caching."""

    def test_cache_ttl_is_5_minutes(self):
        """Test that cache TTL is configured as 5 minutes (300 seconds)."""
        cache = QueryCache(redis_client=MagicMock())
        assert cache.ttl == 300

    def test_compute_hash_is_deterministic(self, sample_search_query: SearchQuery):
        """Test that compute_hash produces deterministic results."""
        hash1 = QueryCache.compute_hash(sample_search_query)
        hash2 = QueryCache.compute_hash(sample_search_query)
        assert hash1 == hash2
        assert len(hash1) == 64  # SHA-256 hex digest length

    def test_compute_hash_varies_with_query_text(self):
        """Test that different query texts produce different hashes."""
        query1 = SearchQuery(query_text="first query")
        query2 = SearchQuery(query_text="second query")

        hash1 = QueryCache.compute_hash(query1)
        hash2 = QueryCache.compute_hash(query2)

        assert hash1 != hash2

    def test_compute_hash_varies_with_content_types(self):
        """Test that different content types produce different hashes."""
        query1 = SearchQuery(
            query_text="same query",
            content_types=[ContentTypeFilter.EMAIL],
        )
        query2 = SearchQuery(
            query_text="same query",
            content_types=[ContentTypeFilter.MEETING],
        )

        hash1 = QueryCache.compute_hash(query1)
        hash2 = QueryCache.compute_hash(query2)

        assert hash1 != hash2

    def test_compute_hash_varies_with_limit(self):
        """Test that different limits produce different hashes."""
        query1 = SearchQuery(query_text="same query", limit=10)
        query2 = SearchQuery(query_text="same query", limit=25)

        hash1 = QueryCache.compute_hash(query1)
        hash2 = QueryCache.compute_hash(query2)

        assert hash1 != hash2

    @pytest.mark.asyncio
    async def test_get_returns_none_on_cache_miss(
        self, mock_redis, sample_search_query: SearchQuery
    ):
        """Test that get returns None when key not in cache."""
        cache = QueryCache(redis_client=mock_redis)
        query_hash = QueryCache.compute_hash(sample_search_query)

        result = await cache.get("tenant-123", query_hash)

        assert result is None
        mock_redis.get.assert_called_once()

    @pytest.mark.asyncio
    async def test_get_returns_cached_response(
        self,
        mock_redis,
        sample_search_query: SearchQuery,
        sample_search_response: SearchResponse,
    ):
        """Test that get returns cached SearchResponse."""
        cache = QueryCache(redis_client=mock_redis)
        query_hash = QueryCache.compute_hash(sample_search_query)

        # Simulate cached data
        mock_redis.get.return_value = sample_search_response.model_dump_json()

        result = await cache.get("tenant-123", query_hash)

        assert result is not None
        assert isinstance(result, SearchResponse)
        assert result.metadata.query_id == sample_search_response.metadata.query_id
        assert len(result.results) == len(sample_search_response.results)

    @pytest.mark.asyncio
    async def test_set_stores_response_with_ttl(
        self,
        mock_redis,
        sample_search_query: SearchQuery,
        sample_search_response: SearchResponse,
    ):
        """Test that set stores response with correct TTL."""
        cache = QueryCache(redis_client=mock_redis)
        query_hash = QueryCache.compute_hash(sample_search_query)

        await cache.set("tenant-123", query_hash, sample_search_response)

        mock_redis.set.assert_called_once()
        call_args = mock_redis.set.call_args
        assert call_args.kwargs.get("ex") == 300  # TTL

    @pytest.mark.asyncio
    async def test_set_uses_correct_key_format(
        self,
        mock_redis,
        sample_search_query: SearchQuery,
        sample_search_response: SearchResponse,
    ):
        """Test that set uses correct key format: search:query:{tenant_id}:{query_hash}."""
        cache = QueryCache(redis_client=mock_redis)
        query_hash = QueryCache.compute_hash(sample_search_query)
        tenant_id = "tenant-123"

        await cache.set(tenant_id, query_hash, sample_search_response)

        expected_key = f"search:query:{tenant_id}:{query_hash}"
        call_args = mock_redis.set.call_args
        assert call_args.args[0] == expected_key

    @pytest.mark.asyncio
    async def test_invalidate_clears_tenant_keys(self, mock_redis):
        """Test that invalidate removes all cached queries for a tenant."""
        cache = QueryCache(redis_client=mock_redis)
        tenant_id = "tenant-123"

        # Simulate finding some keys
        mock_redis.scan.return_value = (0, [b"search:query:tenant-123:abc", b"search:query:tenant-123:def"])

        await cache.invalidate(tenant_id)

        mock_redis.delete.assert_called()

    @pytest.mark.asyncio
    async def test_get_handles_corrupted_cache_data(self, mock_redis):
        """Test that get handles corrupted/invalid JSON gracefully."""
        cache = QueryCache(redis_client=mock_redis)
        mock_redis.get.return_value = "invalid json {"

        result = await cache.get("tenant-123", "some-hash")

        assert result is None  # Should return None, not raise

    @pytest.mark.asyncio
    async def test_get_handles_redis_connection_error(self, mock_redis):
        """Test that get handles Redis connection errors gracefully."""
        cache = QueryCache(redis_client=mock_redis)
        mock_redis.get.side_effect = ConnectionError("Redis unavailable")

        result = await cache.get("tenant-123", "some-hash")

        assert result is None  # Should return None, not raise


# =============================================================================
# EMBEDDING CACHE TESTS
# =============================================================================


class TestEmbeddingCache:
    """Tests for EmbeddingCache in-memory LRU caching."""

    def test_default_max_size_is_10000(self):
        """Test that default max size is 10,000 entries."""
        cache = EmbeddingCache()
        assert cache._max_size == 10000

    def test_custom_max_size(self):
        """Test that custom max size is accepted."""
        cache = EmbeddingCache(max_size=5000)
        assert cache._max_size == 5000

    def test_initial_size_is_zero(self):
        """Test that cache starts empty."""
        cache = EmbeddingCache()
        assert cache.size == 0

    def test_get_returns_none_for_missing_key(self):
        """Test that get returns None for missing entity."""
        cache = EmbeddingCache()
        result = cache.get(12345)
        assert result is None

    def test_set_and_get_embedding(self):
        """Test basic set and get operations."""
        cache = EmbeddingCache()
        embedding = [0.1, 0.2, 0.3, 0.4, 0.5]

        cache.set(12345, embedding)
        result = cache.get(12345)

        assert result == embedding
        assert cache.size == 1

    def test_get_updates_access_order(self):
        """Test that get updates the access order for LRU tracking."""
        cache = EmbeddingCache(max_size=3)

        # Add three embeddings
        cache.set(1, [0.1])
        cache.set(2, [0.2])
        cache.set(3, [0.3])

        # Access the first one (makes it most recently used)
        cache.get(1)

        # Add a fourth - should evict 2 (least recently used)
        cache.set(4, [0.4])

        assert cache.get(1) is not None  # Still there
        assert cache.get(2) is None  # Evicted
        assert cache.get(3) is not None  # Still there
        assert cache.get(4) is not None  # Just added

    def test_lru_eviction(self):
        """Test that LRU eviction works correctly when max size is reached."""
        cache = EmbeddingCache(max_size=3)

        cache.set(1, [0.1])
        cache.set(2, [0.2])
        cache.set(3, [0.3])
        assert cache.size == 3

        # Add fourth - should evict first (LRU)
        cache.set(4, [0.4])

        assert cache.size == 3
        assert cache.get(1) is None  # Evicted
        assert cache.get(2) is not None
        assert cache.get(3) is not None
        assert cache.get(4) is not None

    def test_clear_empties_cache(self):
        """Test that clear removes all entries."""
        cache = EmbeddingCache()

        cache.set(1, [0.1])
        cache.set(2, [0.2])
        assert cache.size == 2

        cache.clear()

        assert cache.size == 0
        assert cache.get(1) is None
        assert cache.get(2) is None

    def test_set_overwrites_existing_entry(self):
        """Test that setting an existing key overwrites the value."""
        cache = EmbeddingCache()

        cache.set(1, [0.1])
        cache.set(1, [0.9])

        result = cache.get(1)
        assert result == [0.9]
        assert cache.size == 1  # Still one entry

    def test_embedding_storage_does_not_mutate_input(self):
        """Test that storing an embedding doesn't mutate the input list."""
        cache = EmbeddingCache()
        original = [0.1, 0.2, 0.3]
        cache.set(1, original)

        # Modify original
        original.append(0.4)

        # Cached value should be unaffected
        cached = cache.get(1)
        assert len(cached) == 3


# =============================================================================
# SEARCH CACHE MANAGER TESTS
# =============================================================================


class TestSearchCacheManager:
    """Tests for SearchCacheManager unified cache interface."""

    def test_initialization_without_redis(self):
        """Test that manager initializes without Redis URL."""
        manager = SearchCacheManager()
        assert manager.query_cache is None
        assert manager.embedding_cache is not None

    def test_initialization_with_embedding_cache_size(self):
        """Test that custom embedding cache size is applied."""
        manager = SearchCacheManager(embedding_cache_size=5000)
        assert manager.embedding_cache._max_size == 5000

    @pytest.mark.asyncio
    async def test_initialize_creates_redis_connection(self):
        """Test that initialize creates Redis connection when URL provided."""
        with patch("penf_lib.search.cache.redis.from_url") as mock_from_url:
            mock_redis = AsyncMock()
            mock_from_url.return_value = mock_redis

            manager = SearchCacheManager(redis_url="redis://localhost:6379")
            await manager.initialize()

            mock_from_url.assert_called_once_with("redis://localhost:6379")
            assert manager.query_cache is not None

    @pytest.mark.asyncio
    async def test_close_closes_redis_connection(self, mock_redis):
        """Test that close properly closes Redis connection."""
        manager = SearchCacheManager()
        manager.query_cache = QueryCache(redis_client=mock_redis)
        manager._redis_client = mock_redis

        await manager.close()

        mock_redis.close.assert_called_once()

    @pytest.mark.asyncio
    async def test_get_cached_results_without_redis(
        self, sample_search_query: SearchQuery
    ):
        """Test that get_cached_results returns None when Redis not configured."""
        manager = SearchCacheManager()

        result = await manager.get_cached_results("tenant-123", sample_search_query)

        assert result is None

    @pytest.mark.asyncio
    async def test_get_cached_results_with_redis(
        self,
        mock_redis,
        sample_search_query: SearchQuery,
        sample_search_response: SearchResponse,
    ):
        """Test that get_cached_results delegates to QueryCache."""
        manager = SearchCacheManager()
        manager.query_cache = QueryCache(redis_client=mock_redis)

        # Simulate cache hit
        mock_redis.get.return_value = sample_search_response.model_dump_json()

        result = await manager.get_cached_results("tenant-123", sample_search_query)

        assert result is not None
        assert isinstance(result, SearchResponse)

    @pytest.mark.asyncio
    async def test_cache_results_without_redis(
        self,
        sample_search_query: SearchQuery,
        sample_search_response: SearchResponse,
    ):
        """Test that cache_results is no-op when Redis not configured."""
        manager = SearchCacheManager()

        # Should not raise
        await manager.cache_results(
            "tenant-123", sample_search_query, sample_search_response
        )

    @pytest.mark.asyncio
    async def test_cache_results_with_redis(
        self,
        mock_redis,
        sample_search_query: SearchQuery,
        sample_search_response: SearchResponse,
    ):
        """Test that cache_results delegates to QueryCache."""
        manager = SearchCacheManager()
        manager.query_cache = QueryCache(redis_client=mock_redis)

        await manager.cache_results(
            "tenant-123", sample_search_query, sample_search_response
        )

        mock_redis.set.assert_called_once()

    def test_get_embedding_delegates_to_embedding_cache(self):
        """Test that get_embedding uses the embedding cache."""
        manager = SearchCacheManager()
        manager.embedding_cache.set(12345, [0.1, 0.2, 0.3])

        result = manager.get_embedding(12345)

        assert result == [0.1, 0.2, 0.3]

    def test_cache_embedding_delegates_to_embedding_cache(self):
        """Test that cache_embedding uses the embedding cache."""
        manager = SearchCacheManager()

        manager.cache_embedding(12345, [0.1, 0.2, 0.3])

        assert manager.embedding_cache.get(12345) == [0.1, 0.2, 0.3]

    @pytest.mark.asyncio
    async def test_context_manager_support(self):
        """Test that manager can be used as async context manager."""
        with patch("penf_lib.search.cache.redis.from_url") as mock_from_url:
            mock_redis = AsyncMock()
            mock_from_url.return_value = mock_redis

            async with SearchCacheManager(redis_url="redis://localhost:6379") as manager:
                assert manager is not None

            mock_redis.close.assert_called_once()

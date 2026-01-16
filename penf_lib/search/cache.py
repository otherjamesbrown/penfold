"""Search caching layer for the Search Interface.

This module provides a two-tier caching strategy:
1. QueryCache: Redis-based cache for complete search results (TTL: 5 minutes)
2. EmbeddingCache: In-memory LRU cache for frequently accessed embeddings (10k entries)
3. SearchCacheManager: Unified interface for both caches

Based on specs/007-search-interface/research.md caching strategy section.

Cache Architecture:
    User Query
        |
        v
    +-----------------------+
    |  Query Hash Cache     |  Redis: full query -> results
    |  TTL: 5 minutes       |  Key: search:query:{tenant_id}:{query_hash}
    +-----------------------+
        | miss
        v
    +-----------------------+
    |  Embedding LRU        |  In-memory: entity_id -> embedding
    |  Size: 10,000         |  Used during similarity search
    +-----------------------+
        |
        v
    +-----------------------+
    |  PostgreSQL           |  Full-text index + pgvector HNSW
    +-----------------------+
"""

from __future__ import annotations

import hashlib
import json
import logging
from typing import TYPE_CHECKING

import redis.asyncio as redis

from penf_lib.search.models import SearchQuery, SearchResponse

if TYPE_CHECKING:
    pass

logger = logging.getLogger(__name__)


# =============================================================================
# QUERY CACHE (Redis-based)
# =============================================================================


class QueryCache:
    """Redis cache for search query results.

    Caches complete search responses with a 5-minute TTL. Uses SHA-256 hashing
    of the query parameters to generate deterministic cache keys.

    Attributes:
        redis: Async Redis client for cache operations.
        ttl: Cache entry time-to-live in seconds (default: 300 = 5 minutes).

    Key Format:
        search:query:{tenant_id}:{query_hash}

    Example:
        cache = QueryCache(redis_client)
        query_hash = QueryCache.compute_hash(search_query)

        # Check cache
        cached = await cache.get("tenant-123", query_hash)
        if cached:
            return cached

        # Execute search and cache results
        results = await execute_search(search_query)
        await cache.set("tenant-123", query_hash, results)
    """

    def __init__(self, redis_client: redis.Redis) -> None:
        """Initialize the query cache.

        Args:
            redis_client: Async Redis client instance.
        """
        self.redis = redis_client
        self.ttl = 300  # 5 minutes

    @staticmethod
    def compute_hash(query: SearchQuery) -> str:
        """Compute a deterministic SHA-256 hash from query parameters.

        The hash is computed from the JSON representation of the query,
        ensuring that identical queries produce identical hashes.

        Args:
            query: The search query to hash.

        Returns:
            64-character hexadecimal hash string.
        """
        # Use model_dump_json for deterministic serialization
        query_json = query.model_dump_json(exclude_none=True)
        return hashlib.sha256(query_json.encode()).hexdigest()

    def _make_key(self, tenant_id: str, query_hash: str) -> str:
        """Generate the Redis key for a cached query.

        Args:
            tenant_id: The tenant identifier.
            query_hash: The query hash from compute_hash().

        Returns:
            Redis key in format: search:query:{tenant_id}:{query_hash}
        """
        return f"search:query:{tenant_id}:{query_hash}"

    async def get(
        self, tenant_id: str, query_hash: str
    ) -> SearchResponse | None:
        """Get cached search results.

        Args:
            tenant_id: The tenant identifier.
            query_hash: The query hash from compute_hash().

        Returns:
            Cached SearchResponse if found and valid, None otherwise.
        """
        key = self._make_key(tenant_id, query_hash)
        try:
            data = await self.redis.get(key)
            if data is None:
                return None

            # Handle bytes from Redis
            if isinstance(data, bytes):
                data = data.decode("utf-8")

            return SearchResponse.model_validate_json(data)

        except (json.JSONDecodeError, ValueError) as e:
            # Corrupted cache data - log and return None
            logger.warning(
                "Corrupted cache data for key %s: %s", key, str(e)
            )
            return None
        except (ConnectionError, redis.RedisError) as e:
            # Redis unavailable - log and return None (graceful degradation)
            logger.warning("Redis connection error: %s", str(e))
            return None

    async def set(
        self, tenant_id: str, query_hash: str, response: SearchResponse
    ) -> None:
        """Cache search results.

        Args:
            tenant_id: The tenant identifier.
            query_hash: The query hash from compute_hash().
            response: The SearchResponse to cache.
        """
        key = self._make_key(tenant_id, query_hash)
        try:
            data = response.model_dump_json()
            await self.redis.set(key, data, ex=self.ttl)
        except (ConnectionError, redis.RedisError) as e:
            # Redis unavailable - log and continue (graceful degradation)
            logger.warning("Failed to cache results: %s", str(e))

    async def invalidate(self, tenant_id: str) -> None:
        """Invalidate all cached queries for a tenant.

        Scans for all keys matching the tenant pattern and deletes them.

        Args:
            tenant_id: The tenant identifier whose cache to clear.
        """
        pattern = f"search:query:{tenant_id}:*"
        try:
            cursor = 0
            while True:
                cursor, keys = await self.redis.scan(
                    cursor=cursor, match=pattern, count=100
                )
                if keys:
                    await self.redis.delete(*keys)
                if cursor == 0:
                    break
        except (ConnectionError, redis.RedisError) as e:
            logger.warning("Failed to invalidate cache for tenant %s: %s", tenant_id, str(e))


# =============================================================================
# EMBEDDING CACHE (In-memory LRU)
# =============================================================================


class EmbeddingCache:
    """In-memory LRU cache for frequently accessed embeddings.

    Provides fast access to embeddings during similarity search to avoid
    repeated database lookups. Uses a simple LRU eviction policy.

    Attributes:
        _cache: Dictionary storing entity_id -> embedding mappings.
        _access_order: List tracking access order for LRU eviction.
        _max_size: Maximum number of entries (default: 10,000).

    Example:
        cache = EmbeddingCache(max_size=10000)

        # Check cache first
        embedding = cache.get(entity_id)
        if embedding is None:
            embedding = await db.fetch_embedding(entity_id)
            cache.set(entity_id, embedding)
    """

    def __init__(self, max_size: int = 10000) -> None:
        """Initialize the embedding cache.

        Args:
            max_size: Maximum number of embeddings to cache (default: 10,000).
        """
        self._cache: dict[int, list[float]] = {}
        self._access_order: list[int] = []
        self._max_size = max_size

    def get(self, entity_id: int) -> list[float] | None:
        """Get embedding from cache.

        Updates access order for LRU tracking if the entry exists.

        Args:
            entity_id: The database ID of the entity.

        Returns:
            Embedding vector if cached, None otherwise.
        """
        if entity_id not in self._cache:
            return None

        # Update access order (move to end = most recently used)
        self._access_order.remove(entity_id)
        self._access_order.append(entity_id)

        return self._cache[entity_id]

    def set(self, entity_id: int, embedding: list[float]) -> None:
        """Cache embedding with LRU eviction.

        If the cache is at capacity, the least recently used entry is evicted.

        Args:
            entity_id: The database ID of the entity.
            embedding: The embedding vector to cache.
        """
        # If already in cache, update value and move to end of access order
        if entity_id in self._cache:
            self._cache[entity_id] = embedding.copy()  # Copy to prevent mutation
            self._access_order.remove(entity_id)
            self._access_order.append(entity_id)
            return

        # Check if we need to evict
        if len(self._cache) >= self._max_size:
            # Evict least recently used (first in access order)
            lru_id = self._access_order.pop(0)
            del self._cache[lru_id]

        # Add new entry
        self._cache[entity_id] = embedding.copy()  # Copy to prevent mutation
        self._access_order.append(entity_id)

    def clear(self) -> None:
        """Clear entire cache."""
        self._cache.clear()
        self._access_order.clear()

    @property
    def size(self) -> int:
        """Current cache size."""
        return len(self._cache)


# =============================================================================
# SEARCH CACHE MANAGER
# =============================================================================


class SearchCacheManager:
    """Manages both query and embedding caches.

    Provides a unified interface for search caching operations. Gracefully
    handles cases where Redis is not configured.

    Attributes:
        query_cache: Redis-based query cache (None if Redis not configured).
        embedding_cache: In-memory LRU cache for embeddings.

    Example:
        # With Redis
        async with SearchCacheManager(redis_url="redis://localhost:6379") as manager:
            cached = await manager.get_cached_results(tenant_id, query)
            if cached:
                return cached

            results = await execute_search(query)
            await manager.cache_results(tenant_id, query, results)
            return results

        # Without Redis (embedding cache only)
        manager = SearchCacheManager()
        embedding = manager.get_embedding(entity_id)
    """

    def __init__(
        self,
        redis_url: str | None = None,
        embedding_cache_size: int = 10000,
    ) -> None:
        """Initialize the cache manager.

        Args:
            redis_url: Redis connection URL. If None, query caching is disabled.
            embedding_cache_size: Maximum size for embedding cache (default: 10,000).
        """
        self._redis_url = redis_url
        self._redis_client: redis.Redis | None = None
        self.query_cache: QueryCache | None = None
        self.embedding_cache = EmbeddingCache(embedding_cache_size)

    async def initialize(self) -> None:
        """Initialize Redis connection.

        Creates Redis client and QueryCache if redis_url was provided.
        Should be called before using query cache operations.
        """
        if self._redis_url:
            self._redis_client = redis.from_url(self._redis_url)
            self.query_cache = QueryCache(redis_client=self._redis_client)
            logger.info("Search cache initialized with Redis: %s", self._redis_url)
        else:
            logger.info("Search cache initialized without Redis (query caching disabled)")

    async def close(self) -> None:
        """Close Redis connection.

        Should be called when the cache manager is no longer needed.
        """
        if self._redis_client:
            await self._redis_client.close()
            self._redis_client = None
            self.query_cache = None

    async def __aenter__(self) -> SearchCacheManager:
        """Async context manager entry."""
        await self.initialize()
        return self

    async def __aexit__(self, exc_type, exc_val, exc_tb) -> None:
        """Async context manager exit."""
        await self.close()

    async def get_cached_results(
        self, tenant_id: str, query: SearchQuery
    ) -> SearchResponse | None:
        """Try to get cached search results.

        Args:
            tenant_id: The tenant identifier.
            query: The search query.

        Returns:
            Cached SearchResponse if found, None otherwise.
        """
        if self.query_cache is None:
            return None

        query_hash = QueryCache.compute_hash(query)
        return await self.query_cache.get(tenant_id, query_hash)

    async def cache_results(
        self, tenant_id: str, query: SearchQuery, response: SearchResponse
    ) -> None:
        """Cache search results.

        No-op if Redis is not configured.

        Args:
            tenant_id: The tenant identifier.
            query: The search query.
            response: The search response to cache.
        """
        if self.query_cache is None:
            return

        query_hash = QueryCache.compute_hash(query)
        await self.query_cache.set(tenant_id, query_hash, response)

    def get_embedding(self, entity_id: int) -> list[float] | None:
        """Get cached embedding.

        Args:
            entity_id: The database ID of the entity.

        Returns:
            Embedding vector if cached, None otherwise.
        """
        return self.embedding_cache.get(entity_id)

    def cache_embedding(self, entity_id: int, embedding: list[float]) -> None:
        """Cache embedding.

        Args:
            entity_id: The database ID of the entity.
            embedding: The embedding vector to cache.
        """
        self.embedding_cache.set(entity_id, embedding)

    async def invalidate_tenant_cache(self, tenant_id: str) -> None:
        """Invalidate all cached queries for a tenant.

        Useful when content is updated and cached results may be stale.

        Args:
            tenant_id: The tenant identifier.
        """
        if self.query_cache:
            await self.query_cache.invalidate(tenant_id)

    def clear_embedding_cache(self) -> None:
        """Clear the embedding cache.

        Useful when embeddings are regenerated (e.g., model update).
        """
        self.embedding_cache.clear()

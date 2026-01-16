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
    repeated database lookups. Uses a simple LRU eviction policy with both
    entry-count and memory-based limits.

    Attributes:
        _cache: Dictionary storing entity_id -> embedding mappings.
        _access_order: List tracking access order for LRU eviction.
        _max_size: Maximum number of entries (default: 10,000).
        _max_memory_bytes: Maximum memory usage in bytes (default: 64MB).
        _estimated_memory: Current estimated memory usage.

    Memory Estimation:
        Each embedding uses approximately:
        - 768 floats × 8 bytes = 6,144 bytes per embedding
        - Plus ~56 bytes Python list overhead
        - Plus ~56 bytes dict entry overhead
        Total: ~6,256 bytes per 768-dimensional embedding

    Example:
        # Entry-count limited
        cache = EmbeddingCache(max_size=10000)

        # Memory-limited (recommended for production)
        cache = EmbeddingCache(max_size=10000, max_memory_mb=64)

        # Check cache first
        embedding = cache.get(entity_id)
        if embedding is None:
            embedding = await db.fetch_embedding(entity_id)
            cache.set(entity_id, embedding)
    """

    # Estimated bytes per embedding (768 floats + overhead)
    BYTES_PER_FLOAT = 8
    LIST_OVERHEAD = 56
    DICT_ENTRY_OVERHEAD = 56

    def __init__(
        self,
        max_size: int = 10000,
        max_memory_mb: int | None = 64,
    ) -> None:
        """Initialize the embedding cache.

        Args:
            max_size: Maximum number of embeddings to cache (default: 10,000).
            max_memory_mb: Maximum memory in megabytes (default: 64MB).
                          Set to None to disable memory limit.
        """
        self._cache: dict[int, list[float]] = {}
        self._access_order: list[int] = []
        self._max_size = max_size
        self._max_memory_bytes = (max_memory_mb * 1024 * 1024) if max_memory_mb else None
        self._estimated_memory: int = 0
        self._embedding_dim: int | None = None  # Detected from first entry

    def _estimate_embedding_size(self, embedding: list[float]) -> int:
        """Estimate memory size of an embedding in bytes.

        Args:
            embedding: The embedding vector.

        Returns:
            Estimated size in bytes.
        """
        return (
            len(embedding) * self.BYTES_PER_FLOAT +
            self.LIST_OVERHEAD +
            self.DICT_ENTRY_OVERHEAD
        )

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

        Evicts entries when either:
        - Entry count exceeds max_size
        - Memory usage exceeds max_memory_bytes (if configured)

        Args:
            entity_id: The database ID of the entity.
            embedding: The embedding vector to cache.
        """
        embedding_size = self._estimate_embedding_size(embedding)

        # Detect embedding dimension from first entry
        if self._embedding_dim is None:
            self._embedding_dim = len(embedding)

        # If already in cache, update value and move to end of access order
        if entity_id in self._cache:
            old_embedding = self._cache[entity_id]
            old_size = self._estimate_embedding_size(old_embedding)
            self._estimated_memory -= old_size

            self._cache[entity_id] = embedding.copy()  # Copy to prevent mutation
            self._estimated_memory += embedding_size

            self._access_order.remove(entity_id)
            self._access_order.append(entity_id)
            return

        # Evict entries until we have space
        self._evict_if_needed(embedding_size)

        # Add new entry
        self._cache[entity_id] = embedding.copy()  # Copy to prevent mutation
        self._access_order.append(entity_id)
        self._estimated_memory += embedding_size

    def _evict_if_needed(self, new_entry_size: int) -> None:
        """Evict LRU entries until there's space for a new entry.

        Evicts based on both entry count and memory limits.

        Args:
            new_entry_size: Size in bytes of the entry to be added.
        """
        # Check entry count limit
        while len(self._cache) >= self._max_size and self._access_order:
            self._evict_lru()

        # Check memory limit if configured
        if self._max_memory_bytes is not None:
            while (
                self._estimated_memory + new_entry_size > self._max_memory_bytes
                and self._access_order
            ):
                self._evict_lru()

    def _evict_lru(self) -> None:
        """Evict the least recently used entry."""
        if not self._access_order:
            return

        lru_id = self._access_order.pop(0)
        if lru_id in self._cache:
            old_embedding = self._cache.pop(lru_id)
            self._estimated_memory -= self._estimate_embedding_size(old_embedding)

    def clear(self) -> None:
        """Clear entire cache."""
        self._cache.clear()
        self._access_order.clear()
        self._estimated_memory = 0

    @property
    def size(self) -> int:
        """Current cache entry count."""
        return len(self._cache)

    @property
    def memory_usage_bytes(self) -> int:
        """Estimated current memory usage in bytes."""
        return self._estimated_memory

    @property
    def memory_usage_mb(self) -> float:
        """Estimated current memory usage in megabytes."""
        return self._estimated_memory / (1024 * 1024)

    def get_stats(self) -> dict:
        """Get cache statistics.

        Returns:
            Dictionary with cache statistics.
        """
        return {
            "entry_count": len(self._cache),
            "max_size": self._max_size,
            "memory_usage_bytes": self._estimated_memory,
            "memory_usage_mb": round(self.memory_usage_mb, 2),
            "max_memory_mb": (
                self._max_memory_bytes / (1024 * 1024)
                if self._max_memory_bytes else None
            ),
            "embedding_dimension": self._embedding_dim,
        }


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
        embedding_cache_memory_mb: int | None = 64,
    ) -> None:
        """Initialize the cache manager.

        Args:
            redis_url: Redis connection URL. If None, query caching is disabled.
            embedding_cache_size: Maximum entries for embedding cache (default: 10,000).
            embedding_cache_memory_mb: Maximum memory for embedding cache in MB
                                       (default: 64MB). Set to None to disable.
        """
        self._redis_url = redis_url
        self._redis_client: redis.Redis | None = None
        self.query_cache: QueryCache | None = None
        self.embedding_cache = EmbeddingCache(
            max_size=embedding_cache_size,
            max_memory_mb=embedding_cache_memory_mb,
        )

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

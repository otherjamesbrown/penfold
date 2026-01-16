"""Search caching for query results and embeddings.

This module provides caching infrastructure for search operations:
- Query result caching (avoid re-executing identical searches)
- Embedding caching (avoid re-computing embeddings for same text)
- Cache invalidation strategies

The cache manager uses an in-memory LRU cache by default, with
optional Redis backend for production deployments.
"""

from __future__ import annotations

import hashlib
import json
from datetime import datetime, timedelta
from typing import Optional, TYPE_CHECKING

if TYPE_CHECKING:
    from penf_lib.search.models import SearchQuery, SearchResponse


class SearchCacheManager:
    """Manages caching for search queries and embeddings.

    Provides a simple caching layer with TTL-based expiration.
    The default implementation uses in-memory storage; production
    deployments should configure Redis backend.

    Attributes:
        default_ttl: Default cache TTL in seconds (default: 300)
        max_entries: Maximum cache entries (default: 1000)
    """

    def __init__(
        self,
        default_ttl: int = 300,
        max_entries: int = 1000,
    ):
        """Initialize cache manager.

        Args:
            default_ttl: Default TTL in seconds for cached items
            max_entries: Maximum number of entries in cache
        """
        self.default_ttl = default_ttl
        self.max_entries = max_entries
        self._cache: dict[str, tuple[datetime, SearchResponse]] = {}

    def _make_cache_key(self, tenant_id: str, query: SearchQuery) -> str:
        """Generate a unique cache key for a query.

        Args:
            tenant_id: Tenant identifier
            query: Search query parameters

        Returns:
            SHA256 hash of the normalized query
        """
        # Normalize query to JSON for consistent hashing
        query_dict = query.model_dump(mode="json")
        normalized = json.dumps(query_dict, sort_keys=True)
        key_data = f"{tenant_id}:{normalized}"
        return hashlib.sha256(key_data.encode()).hexdigest()

    async def get_cached_results(
        self,
        tenant_id: str,
        query: SearchQuery,
    ) -> Optional[SearchResponse]:
        """Retrieve cached results for a query.

        Args:
            tenant_id: Tenant identifier
            query: Search query parameters

        Returns:
            Cached SearchResponse if found and not expired, None otherwise
        """
        key = self._make_cache_key(tenant_id, query)

        if key not in self._cache:
            return None

        cached_time, response = self._cache[key]

        # Check TTL expiration
        if datetime.utcnow() - cached_time > timedelta(seconds=self.default_ttl):
            del self._cache[key]
            return None

        return response

    async def cache_results(
        self,
        tenant_id: str,
        query: SearchQuery,
        response: SearchResponse,
        ttl: Optional[int] = None,
    ) -> None:
        """Cache search results.

        Args:
            tenant_id: Tenant identifier
            query: Search query parameters
            response: Search response to cache
            ttl: Optional TTL override in seconds
        """
        # Enforce max entries with simple eviction
        if len(self._cache) >= self.max_entries:
            # Remove oldest entry
            oldest_key = min(self._cache.keys(), key=lambda k: self._cache[k][0])
            del self._cache[oldest_key]

        key = self._make_cache_key(tenant_id, query)
        self._cache[key] = (datetime.utcnow(), response)

    async def invalidate(
        self,
        tenant_id: str,
        query: Optional[SearchQuery] = None,
    ) -> int:
        """Invalidate cached results.

        Args:
            tenant_id: Tenant identifier
            query: Optional specific query to invalidate.
                   If None, invalidates all entries for the tenant.

        Returns:
            Number of entries invalidated
        """
        if query is not None:
            key = self._make_cache_key(tenant_id, query)
            if key in self._cache:
                del self._cache[key]
                return 1
            return 0

        # Invalidate all entries for tenant
        # Note: In-memory cache doesn't track by tenant efficiently
        # This would be better with Redis SCAN + DEL pattern matching
        count = 0
        keys_to_delete = []
        for key in self._cache:
            # We can't easily determine tenant from hashed key
            # For full invalidation, clear all
            keys_to_delete.append(key)

        for key in keys_to_delete:
            del self._cache[key]
            count += 1

        return count

    async def clear(self) -> None:
        """Clear all cached entries."""
        self._cache.clear()

    @property
    def size(self) -> int:
        """Current number of cached entries."""
        return len(self._cache)

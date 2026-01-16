"""Search Interface module.

This module provides the core functionality for the search and query interface,
enabling hybrid search (full-text + semantic) across email, meetings, documents,
and Slack content.

Key components:
- models: Pydantic DTOs and enums for search entities
- cache: Search result and embedding caching
- search_engine: Main search orchestration

Reference: spec 007-search-interface
"""

from penf_lib.search.cache import EmbeddingCache, QueryCache, SearchCacheManager
from penf_lib.search.models import (
    # Response models
    ContentPreview,
    # Enums
    ContentTypeFilter,
    SearchMetadata,
    SearchQuery,
    SearchResponse,
    SearchResult,
    SortOrder,
    # Request models
    TemporalConstraint,
)
from penf_lib.search.search_engine import SearchEngine

__all__ = [
    # Enums
    "ContentTypeFilter",
    "SortOrder",
    # Request models
    "TemporalConstraint",
    "SearchQuery",
    # Response models
    "ContentPreview",
    "SearchResult",
    "SearchMetadata",
    "SearchResponse",
    # Cache components
    "QueryCache",
    "EmbeddingCache",
    "SearchCacheManager",
    # Core components
    "SearchEngine",
]

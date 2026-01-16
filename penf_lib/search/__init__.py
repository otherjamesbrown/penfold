"""Search Interface module.

This module provides the core functionality for the search and query interface,
enabling hybrid search (full-text + semantic) across email, meetings, documents,
and Slack content.

Key components:
- models: Pydantic DTOs and enums for search entities
- cache: Search result and embedding caching
- search_engine: Main search orchestration
- correlations: Cross-content correlation discovery

Reference: spec 007-search-interface
"""

from penf_lib.search.cache import EmbeddingCache, QueryCache, SearchCacheManager
from penf_lib.search.correlations import (
    CorrelationDiscovery,
    CorrelationResult,
    RelatedItem,
    find_related_by_person,
    find_related_by_project,
)
from penf_lib.search.models import (
    # Response models
    ContentPreview,
    # Enums
    ContentTypeFilter,
    CorrelationType,
    RelatedContentResponse,
    RelatedItemResponse,
    SearchMetadata,
    SearchQuery,
    SearchResponse,
    SearchResult,
    SortOrder,
    # Request models
    TemporalConstraint,
)
from penf_lib.search.query_parser import QueryEmbedder, QueryParser, TemporalQueryParser
from penf_lib.search.ranking import RankedResult, RRFFusion, SearchRanker
from penf_lib.search.search_engine import SearchEngine

__all__ = [
    # Enums
    "ContentTypeFilter",
    "SortOrder",
    "CorrelationType",
    # Request models
    "TemporalConstraint",
    "SearchQuery",
    # Response models
    "ContentPreview",
    "SearchResult",
    "SearchMetadata",
    "SearchResponse",
    "RelatedItemResponse",
    "RelatedContentResponse",
    # Cache components
    "QueryCache",
    "EmbeddingCache",
    "SearchCacheManager",
    # Query processing
    "QueryParser",
    "QueryEmbedder",
    "TemporalQueryParser",
    # Ranking
    "RRFFusion",
    "SearchRanker",
    "RankedResult",
    # Correlation discovery
    "CorrelationDiscovery",
    "CorrelationResult",
    "RelatedItem",
    "find_related_by_person",
    "find_related_by_project",
    # Core components
    "SearchEngine",
]

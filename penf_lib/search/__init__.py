"""Search Interface module.

This module provides the core functionality for the search and query interface,
enabling hybrid search (full-text + semantic) across email, meetings, documents,
and Slack content.

Key components:
- models: Pydantic DTOs and enums for search entities
- cache: Search result and embedding caching
- search_engine: Main search orchestration
- correlations: Cross-content correlation discovery
- session: Search session management
- suggestions: Query suggestion engine
- analytics: Search analytics collection and reporting
- filters: Advanced filtering system for search results

Reference: spec 007-search-interface
"""

from penf_lib.search.analytics import AnalyticsCollector, SearchEvent, SearchMetrics
from penf_lib.search.cache import EmbeddingCache, QueryCache, SearchCacheManager
from penf_lib.search.correlations import (
    CorrelationDiscovery,
    CorrelationResult,
    RelatedItem,
    find_related_by_person,
    find_related_by_project,
)
from penf_lib.search.filters import (
    ConfidenceFilter,
    ContentTypeFilterImpl,
    DateRangeFilter,
    FilterPipeline,
    FilterStatistics,
    ParticipantFilter,
    ProjectFilter,
    SearchFilter,
)
from penf_lib.search.models import (
    # Enums
    ContentTypeFilter,
    CorrelationType,
    SortOrder,
    # Request models
    SearchQuery,
    TemporalConstraint,
    # Response models
    ContentPreview,
    RelatedContentResponse,
    RelatedItemResponse,
    SearchMetadata,
    SearchResponse,
    SearchResult,
)
from penf_lib.search.query_parser import QueryEmbedder, QueryParser, TemporalQueryParser
from penf_lib.search.ranking import RankedResult, RRFFusion, SearchRanker
from penf_lib.search.search_engine import SearchEngine
from penf_lib.search.session import SearchSessionInfo, SessionManager
from penf_lib.search.suggestions import QuerySuggestion, SuggestionEngine

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
    # Session management
    "SessionManager",
    "SearchSessionInfo",
    # Suggestions
    "SuggestionEngine",
    "QuerySuggestion",
    # Analytics
    "AnalyticsCollector",
    "SearchMetrics",
    "SearchEvent",
    # Filters
    "SearchFilter",
    "ContentTypeFilterImpl",
    "ParticipantFilter",
    "ProjectFilter",
    "ConfidenceFilter",
    "DateRangeFilter",
    "FilterPipeline",
    "FilterStatistics",
    # Core components
    "SearchEngine",
]

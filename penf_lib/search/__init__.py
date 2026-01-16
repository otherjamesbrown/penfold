"""Search Interface module.

This module provides the core functionality for the search and query interface,
enabling hybrid search (full-text + semantic) across email, meetings, documents,
and Slack content.

Key components:
- models: Pydantic DTOs and enums for search entities

Reference: spec 007-search-interface
"""

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
]

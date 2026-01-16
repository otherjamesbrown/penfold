"""Pydantic models and enums for the Search Interface.

This module contains data transfer objects (DTOs) for the search workflow,
including query parameters, results, metadata, and response structures.
These are request/response models (not persisted) used for API validation.

Based on data-model.md specification from spec 007-search-interface.
"""

from __future__ import annotations

from datetime import datetime
from enum import Enum
from typing import Any, Optional

from pydantic import BaseModel, ConfigDict, Field, model_validator

# =============================================================================
# ENUMERATIONS
# =============================================================================


class ContentTypeFilter(str, Enum):
    """Content type filter for search queries.

    Specifies which types of content to include in search results.
    Use ALL to search across all content types.
    """

    EMAIL = "email"
    MEETING = "meeting"
    DOCUMENT = "document"
    SLACK = "slack"
    ALL = "all"


class SortOrder(str, Enum):
    """Sort ordering for search results.

    Determines how search results are ordered in the response.
    """

    RELEVANCE = "relevance"
    RECENCY = "recency"
    ALPHABETICAL = "alphabetical"


# =============================================================================
# REQUEST MODELS
# =============================================================================


class TemporalConstraint(BaseModel):
    """Temporal filtering for search queries.

    Allows filtering results by date range or relative time expressions.
    All fields are optional, but at least one should be provided for
    temporal filtering to take effect.

    Examples:
        - Absolute: start_date="2026-01-01", end_date="2026-01-15"
        - Relative: relative_expression="last week"
        - Combined: start_date="2026-01-01", relative_expression="since Monday"
    """

    model_config = ConfigDict(frozen=False)

    start_date: Optional[datetime] = Field(
        default=None,
        description="Start of date range (inclusive)",
    )
    end_date: Optional[datetime] = Field(
        default=None,
        description="End of date range (inclusive)",
    )
    relative_expression: Optional[str] = Field(
        default=None,
        description="Natural language time expression (e.g., 'last week', 'since December')",
    )

    @model_validator(mode="after")
    def validate_date_range(self) -> TemporalConstraint:
        """Ensure start_date is before end_date if both are provided."""
        if self.start_date and self.end_date:
            if self.start_date > self.end_date:
                raise ValueError("Start date must be before end date")
        return self


class SearchQuery(BaseModel):
    """Search query parameters submitted for search operations.

    This model validates incoming search requests and provides sensible
    defaults for optional parameters. Not persisted directly - used for
    request validation only.

    Attributes:
        query_text: The search query string (1-500 characters).
        content_types: Types of content to search. Defaults to ALL.
        temporal: Optional date/time constraints.
        participants: Filter by participant email/name (max 20).
        projects: Filter by project reference (max 10).
        min_confidence: Minimum AI confidence score (0-1).
        include_relationships: Include related content in results.
        sort_by: Result ordering strategy.
        limit: Maximum results to return (1-100, default 25).
        offset: Pagination offset (default 0).
    """

    model_config = ConfigDict(
        frozen=False,
        json_schema_extra={
            "example": {
                "query_text": "customer deployment issues",
                "content_types": ["email", "meeting"],
                "temporal": {"relative_expression": "last week"},
                "limit": 25,
            }
        },
    )

    # Required fields
    query_text: str = Field(
        ...,
        min_length=1,
        max_length=500,
        description="Search query text (1-500 characters)",
    )

    # Content filtering
    content_types: list[ContentTypeFilter] = Field(
        default=[ContentTypeFilter.ALL],
        description="Types of content to include in search",
    )
    temporal: Optional[TemporalConstraint] = Field(
        default=None,
        description="Temporal constraints for filtering",
    )
    participants: Optional[list[str]] = Field(
        default=None,
        max_length=20,
        description="Filter by participant names or emails (max 20)",
    )
    projects: Optional[list[str]] = Field(
        default=None,
        max_length=10,
        description="Filter by project references (max 10)",
    )

    # Quality thresholds
    min_confidence: Optional[float] = Field(
        default=None,
        ge=0.0,
        le=1.0,
        description="Minimum AI confidence score (0-1)",
    )

    # Result options
    include_relationships: bool = Field(
        default=True,
        description="Include related content in results",
    )
    sort_by: SortOrder = Field(
        default=SortOrder.RELEVANCE,
        description="Result ordering strategy",
    )

    # Pagination
    limit: int = Field(
        default=25,
        ge=1,
        le=100,
        description="Maximum results to return (1-100)",
    )
    offset: int = Field(
        default=0,
        ge=0,
        description="Pagination offset",
    )


# =============================================================================
# RESPONSE MODELS
# =============================================================================


class ContentPreview(BaseModel):
    """Preview of content for search results.

    Provides a snippet of the matched content with optional highlighting
    positions to indicate where query terms were found.

    Attributes:
        title: Optional title of the content (e.g., email subject).
        snippet: Text snippet showing relevant content (max 500 chars).
        highlight_positions: List of (start, end) positions for highlighting.
    """

    model_config = ConfigDict(frozen=False)

    title: Optional[str] = Field(
        default=None,
        description="Content title (e.g., email subject, document name)",
    )
    snippet: str = Field(
        ...,
        max_length=500,
        description="Text snippet showing relevant content",
    )
    highlight_positions: list[tuple[int, int]] = Field(
        default_factory=list,
        description="List of (start, end) positions for query term highlighting",
    )


class SearchResult(BaseModel):
    """Individual search result item.

    Represents a single matching item from the search with scoring,
    metadata, and relationship information.

    Attributes:
        result_id: Unique identifier for this result (UUID).
        entity_type: Type of entity (source, assertion, person, project).
        entity_id: Database ID of the matched entity.
        content_type: Type of content (email, meeting, etc.).
        preview: Content preview with snippet and highlights.
        source_attribution: Link/reference to original content.
        relevance_score: Search relevance score (0-1).
        rrf_score: Reciprocal Rank Fusion score from hybrid search.
        confidence_score: AI processing confidence (if available).
        timestamp: Original content timestamp.
        participants: List of participant names/emails.
        project_refs: Associated project references.
        tags: Content tags.
        related_content_ids: IDs of related content items.
        relationship_strength: Strength of relationship (if applicable).
    """

    model_config = ConfigDict(
        frozen=False,
        json_schema_extra={
            "example": {
                "result_id": "550e8400-e29b-41d4-a716-446655440000",
                "entity_type": "source",
                "entity_id": 12345,
                "content_type": "email",
                "preview": {
                    "title": "Re: Deployment Issues",
                    "snippet": "...the customer reported issues with the deployment...",
                    "highlight_positions": [[4, 12], [35, 45]],
                },
                "source_attribution": "gmail://msg/abc123",
                "relevance_score": 0.92,
                "rrf_score": 0.85,
                "timestamp": "2026-01-10T14:30:00Z",
                "participants": ["alice@example.com", "bob@example.com"],
            }
        },
    )

    # Identification
    result_id: str = Field(
        ...,
        description="Unique result identifier (UUID)",
    )
    entity_type: str = Field(
        ...,
        description="Type of entity (source, assertion, person, project)",
    )
    entity_id: int = Field(
        ...,
        description="Database ID of the matched entity",
    )

    # Content information
    content_type: ContentTypeFilter = Field(
        ...,
        description="Type of content",
    )
    preview: ContentPreview = Field(
        ...,
        description="Content preview with snippet and highlights",
    )
    source_attribution: str = Field(
        ...,
        description="Link/reference to original content source",
    )

    # Scoring
    relevance_score: float = Field(
        ...,
        ge=0.0,
        le=1.0,
        description="Search relevance score (0-1)",
    )
    rrf_score: float = Field(
        ...,
        description="Reciprocal Rank Fusion score from hybrid search",
    )
    confidence_score: Optional[float] = Field(
        default=None,
        ge=0.0,
        le=1.0,
        description="AI processing confidence score",
    )

    # Temporal
    timestamp: datetime = Field(
        ...,
        description="Original content timestamp",
    )

    # Context
    participants: list[str] = Field(
        default_factory=list,
        description="Participant names or emails",
    )
    project_refs: list[str] = Field(
        default_factory=list,
        description="Associated project references",
    )
    tags: list[str] = Field(
        default_factory=list,
        description="Content tags",
    )

    # Relationships
    related_content_ids: list[str] = Field(
        default_factory=list,
        description="IDs of related content items",
    )
    relationship_strength: Optional[float] = Field(
        default=None,
        ge=0.0,
        le=1.0,
        description="Strength of relationship to query context",
    )


class SearchMetadata(BaseModel):
    """Metadata about search execution.

    Provides information about how the search was executed, including
    timing, result counts, and caching information.

    Attributes:
        query_id: Unique identifier for this search (UUID).
        execution_time_ms: Time taken to execute the search.
        total_results: Total number of matching results.
        returned_results: Number of results in this response.
        search_strategy: Strategy used (hybrid, full_text, semantic).
        cache_hit: Whether results came from cache.
    """

    model_config = ConfigDict(frozen=False)

    query_id: str = Field(
        ...,
        description="Unique query identifier for tracking (UUID)",
    )
    execution_time_ms: int = Field(
        ...,
        ge=0,
        description="Search execution time in milliseconds",
    )
    total_results: int = Field(
        ...,
        ge=0,
        description="Total number of matching results",
    )
    returned_results: int = Field(
        ...,
        ge=0,
        description="Number of results in this response",
    )
    search_strategy: str = Field(
        ...,
        description="Search strategy used (hybrid, full_text, semantic)",
    )
    cache_hit: bool = Field(
        ...,
        description="Whether results were served from cache",
    )


class SearchResponse(BaseModel):
    """Complete search response.

    Contains search results along with metadata, suggestions, and
    pagination information.

    Attributes:
        metadata: Search execution metadata.
        results: List of search results.
        suggestions: Query refinement suggestions.
        filters_applied: Dictionary of filters that were applied.
        has_more: Whether more results are available.
        next_offset: Offset for fetching next page (if has_more is True).
    """

    model_config = ConfigDict(frozen=False)

    metadata: SearchMetadata = Field(
        ...,
        description="Search execution metadata",
    )
    results: list[SearchResult] = Field(
        ...,
        description="List of search results",
    )
    suggestions: list[str] = Field(
        default_factory=list,
        description="Query refinement suggestions",
    )
    filters_applied: dict[str, Any] = Field(
        default_factory=dict,
        description="Filters that were applied to the search",
    )

    # Pagination
    has_more: bool = Field(
        ...,
        description="Whether more results are available",
    )
    next_offset: Optional[int] = Field(
        default=None,
        ge=0,
        description="Offset for fetching next page",
    )

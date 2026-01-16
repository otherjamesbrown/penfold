"""Advanced filtering system for search results.

This module provides filter classes for refining search results by:
- Content type (email, meeting, document, slack)
- Participants (filter by email addresses)
- Projects (filter by project names)
- Confidence levels (AI extraction confidence)
- Date ranges (temporal constraints)
"""

from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime
from typing import TYPE_CHECKING, Any, List, Optional

from abc import ABC, abstractmethod

from penf_lib.search.models import ContentTypeFilter, SearchResult

if TYPE_CHECKING:
    from penf_lib.search.models import SearchQuery


class SearchFilter(ABC):
    """Base class for search result filters."""

    @abstractmethod
    def matches(self, result: SearchResult) -> bool:
        """Check if result matches this filter.

        Args:
            result: Search result to check

        Returns:
            True if result matches the filter criteria
        """
        pass

    @abstractmethod
    def to_dict(self) -> dict[str, Any]:
        """Serialize filter for storage/logging.

        Returns:
            Dictionary representation of the filter
        """
        pass


class ContentTypeFilterImpl(SearchFilter):
    """Filter results by content type.

    Supports filtering to specific content types or excluding certain types.

    Attributes:
        allowed_types: Set of content types to include
        exclude: If True, exclude matching types instead of including
    """

    def __init__(
        self,
        allowed_types: List[ContentTypeFilter],
        exclude: bool = False,
    ):
        """Initialize content type filter.

        Args:
            allowed_types: List of content types to filter by
            exclude: If True, exclude these types; otherwise include only these
        """
        self.allowed_types = set(allowed_types)
        self.exclude = exclude

    def matches(self, result: SearchResult) -> bool:
        """Check if result's content type matches filter criteria."""
        is_match = result.content_type in self.allowed_types
        return not is_match if self.exclude else is_match

    def to_dict(self) -> dict[str, Any]:
        """Serialize filter to dictionary."""
        return {
            "type": "content_type",
            "allowed": [t.value for t in self.allowed_types],
            "exclude": self.exclude,
        }


class ParticipantFilter(SearchFilter):
    """Filter results by participant email addresses.

    Supports matching any or all specified participants.

    Attributes:
        participants: Set of participant emails to match (lowercase)
        match_any: If True (default), match any participant (OR logic);
                   if False, require all participants (AND logic)
    """

    def __init__(
        self,
        participants: List[str],
        match_any: bool = True,
    ):
        """Initialize participant filter.

        Args:
            participants: List of participant email addresses or names
            match_any: If True, match any participant; if False, require all
        """
        self.participants = set(p.lower() for p in participants)
        self.match_any = match_any

    def matches(self, result: SearchResult) -> bool:
        """Check if result's participants match filter criteria."""
        result_participants = set(p.lower() for p in result.participants)
        if self.match_any:
            return bool(self.participants & result_participants)
        else:
            return self.participants.issubset(result_participants)

    def to_dict(self) -> dict[str, Any]:
        """Serialize filter to dictionary."""
        return {
            "type": "participant",
            "participants": list(self.participants),
            "match_any": self.match_any,
        }


class ProjectFilter(SearchFilter):
    """Filter results by project references.

    Supports matching any or all specified projects.

    Attributes:
        projects: Set of project names to match (lowercase)
        match_any: If True (default), match any project (OR logic);
                   if False, require all projects (AND logic)
    """

    def __init__(
        self,
        projects: List[str],
        match_any: bool = True,
    ):
        """Initialize project filter.

        Args:
            projects: List of project names to filter by
            match_any: If True, match any project; if False, require all
        """
        self.projects = set(p.lower() for p in projects)
        self.match_any = match_any

    def matches(self, result: SearchResult) -> bool:
        """Check if result's project references match filter criteria."""
        result_projects = set(p.lower() for p in result.project_refs)
        if self.match_any:
            return bool(self.projects & result_projects)
        else:
            return self.projects.issubset(result_projects)

    def to_dict(self) -> dict[str, Any]:
        """Serialize filter to dictionary."""
        return {
            "type": "project",
            "projects": list(self.projects),
            "match_any": self.match_any,
        }


class ConfidenceFilter(SearchFilter):
    """Filter results by AI confidence score.

    Filters results to only include those with confidence scores
    at or above the specified threshold. Results without confidence
    scores are included by default.

    Attributes:
        min_confidence: Minimum confidence score (0.0-1.0)
    """

    def __init__(self, min_confidence: float):
        """Initialize confidence filter.

        Args:
            min_confidence: Minimum confidence score (0.0-1.0)

        Raises:
            ValueError: If min_confidence is not between 0.0 and 1.0
        """
        if not 0.0 <= min_confidence <= 1.0:
            raise ValueError("min_confidence must be between 0.0 and 1.0")
        self.min_confidence = min_confidence

    def matches(self, result: SearchResult) -> bool:
        """Check if result's confidence meets minimum threshold.

        Results without confidence scores are included.
        """
        if result.confidence_score is None:
            return True  # Include results without confidence
        return result.confidence_score >= self.min_confidence

    def to_dict(self) -> dict[str, Any]:
        """Serialize filter to dictionary."""
        return {
            "type": "confidence",
            "min_confidence": self.min_confidence,
        }


class DateRangeFilter(SearchFilter):
    """Filter results by date range.

    Supports filtering by start date, end date, or both.

    Attributes:
        start_date: Start of date range (inclusive), or None for no lower bound
        end_date: End of date range (inclusive), or None for no upper bound
    """

    def __init__(
        self,
        start_date: Optional[datetime] = None,
        end_date: Optional[datetime] = None,
    ):
        """Initialize date range filter.

        Args:
            start_date: Start of date range (inclusive)
            end_date: End of date range (inclusive)
        """
        self.start_date = start_date
        self.end_date = end_date

    def matches(self, result: SearchResult) -> bool:
        """Check if result's timestamp falls within date range."""
        if self.start_date and result.timestamp < self.start_date:
            return False
        if self.end_date and result.timestamp > self.end_date:
            return False
        return True

    def to_dict(self) -> dict[str, Any]:
        """Serialize filter to dictionary."""
        return {
            "type": "date_range",
            "start_date": self.start_date.isoformat() if self.start_date else None,
            "end_date": self.end_date.isoformat() if self.end_date else None,
        }


@dataclass
class FilterStatistics:
    """Statistics about filter application.

    Attributes:
        total_before: Number of results before filtering
        total_after: Number of results after filtering
        by_content_type: Count of results by content type after filtering
        filters_applied: List of filter configurations applied
        reduction_percentage: Percentage of results filtered out (0-100)
    """

    total_before: int
    total_after: int
    by_content_type: dict[str, int]
    filters_applied: List[dict[str, Any]]
    reduction_percentage: float


class FilterPipeline:
    """Pipeline for applying multiple filters to search results.

    Filters are applied in sequence with AND logic - a result must
    match all filters to be included in the output.

    Usage:
        pipeline = FilterPipeline()
        pipeline.add_filter(ContentTypeFilterImpl([ContentTypeFilter.EMAIL]))
        pipeline.add_filter(ParticipantFilter(["alice@example.com"]))
        filtered, stats = pipeline.apply(results)
    """

    def __init__(self) -> None:
        """Initialize empty filter pipeline."""
        self.filters: List[SearchFilter] = []

    def add_filter(self, filter: SearchFilter) -> "FilterPipeline":
        """Add a filter to the pipeline.

        Args:
            filter: Filter to add

        Returns:
            Self for method chaining
        """
        self.filters.append(filter)
        return self

    def apply(
        self,
        results: List[SearchResult],
    ) -> tuple[List[SearchResult], FilterStatistics]:
        """Apply all filters and return filtered results with statistics.

        Args:
            results: List of search results to filter

        Returns:
            Tuple of (filtered results, statistics)
        """
        total_before = len(results)

        # Count by content type before filtering
        type_counts_before: dict[str, int] = {}
        for r in results:
            t = r.content_type.value
            type_counts_before[t] = type_counts_before.get(t, 0) + 1

        # Apply filters sequentially
        filtered = results
        for f in self.filters:
            filtered = [r for r in filtered if f.matches(r)]

        # Count by content type after filtering
        type_counts_after: dict[str, int] = {}
        for r in filtered:
            t = r.content_type.value
            type_counts_after[t] = type_counts_after.get(t, 0) + 1

        total_after = len(filtered)
        reduction = (
            ((total_before - total_after) / total_before * 100)
            if total_before > 0
            else 0.0
        )

        stats = FilterStatistics(
            total_before=total_before,
            total_after=total_after,
            by_content_type=type_counts_after,
            filters_applied=[f.to_dict() for f in self.filters],
            reduction_percentage=reduction,
        )

        return filtered, stats

    @classmethod
    def from_search_query(cls, query: "SearchQuery") -> "FilterPipeline":
        """Build filter pipeline from SearchQuery model.

        Constructs appropriate filters based on query parameters.

        Args:
            query: SearchQuery with filter parameters

        Returns:
            Configured FilterPipeline
        """
        pipeline = cls()

        # Content type filter
        if query.content_types and ContentTypeFilter.ALL not in query.content_types:
            pipeline.add_filter(ContentTypeFilterImpl(query.content_types))

        # Participant filter
        if query.participants:
            pipeline.add_filter(ParticipantFilter(query.participants))

        # Project filter
        if query.projects:
            pipeline.add_filter(ProjectFilter(query.projects))

        # Confidence filter
        if query.min_confidence is not None:
            pipeline.add_filter(ConfidenceFilter(query.min_confidence))

        # Temporal filter
        if query.temporal:
            pipeline.add_filter(
                DateRangeFilter(
                    start_date=query.temporal.start_date,
                    end_date=query.temporal.end_date,
                )
            )

        return pipeline

    def __len__(self) -> int:
        """Return number of filters in pipeline."""
        return len(self.filters)

    def __bool__(self) -> bool:
        """Return True if pipeline has any filters."""
        return len(self.filters) > 0

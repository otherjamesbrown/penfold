"""Unit tests for search filters module.

Tests filter functionality for the search interface including:
- Content type filtering
- Temporal filtering
- Participant filtering
- Project filtering
- Confidence filtering
- Filter pipeline
"""

from datetime import datetime, timedelta
from typing import List, Optional

import pytest

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
    ContentPreview,
    ContentTypeFilter,
    SearchQuery,
    SearchResult,
    TemporalConstraint,
)


# =============================================================================
# TEST FIXTURES
# =============================================================================


def make_result(
    content_type: ContentTypeFilter = ContentTypeFilter.EMAIL,
    participants: Optional[List[str]] = None,
    project_refs: Optional[List[str]] = None,
    confidence_score: Optional[float] = None,
    timestamp: Optional[datetime] = None,
) -> SearchResult:
    """Create a SearchResult for testing."""
    return SearchResult(
        result_id="test-result-id",
        entity_type="source",
        entity_id=1,
        content_type=content_type,
        preview=ContentPreview(
            title="Test Title",
            snippet="Test snippet content",
            highlight_positions=[],
        ),
        source_attribution="test://123",
        relevance_score=0.8,
        rrf_score=0.7,
        confidence_score=confidence_score,
        timestamp=timestamp or datetime.utcnow(),
        participants=participants or [],
        project_refs=project_refs or [],
        tags=[],
        related_content_ids=[],
    )


# =============================================================================
# CONTENT TYPE FILTER TESTS
# =============================================================================


@pytest.mark.unit
class TestContentTypeFilter:
    """Test content type filter functionality."""

    def test_matches_single_type(self):
        """Test filtering by single content type."""
        filter = ContentTypeFilterImpl([ContentTypeFilter.EMAIL])

        email_result = make_result(content_type=ContentTypeFilter.EMAIL)
        meeting_result = make_result(content_type=ContentTypeFilter.MEETING)

        assert filter.matches(email_result) is True
        assert filter.matches(meeting_result) is False

    def test_matches_multiple_types(self):
        """Test filtering by multiple content types (OR logic)."""
        filter = ContentTypeFilterImpl([
            ContentTypeFilter.EMAIL,
            ContentTypeFilter.MEETING,
        ])

        email_result = make_result(content_type=ContentTypeFilter.EMAIL)
        meeting_result = make_result(content_type=ContentTypeFilter.MEETING)
        slack_result = make_result(content_type=ContentTypeFilter.SLACK)

        assert filter.matches(email_result) is True
        assert filter.matches(meeting_result) is True
        assert filter.matches(slack_result) is False

    def test_exclude_mode(self):
        """Test content type exclusion."""
        filter = ContentTypeFilterImpl([ContentTypeFilter.SLACK], exclude=True)

        email_result = make_result(content_type=ContentTypeFilter.EMAIL)
        slack_result = make_result(content_type=ContentTypeFilter.SLACK)

        assert filter.matches(email_result) is True
        assert filter.matches(slack_result) is False

    def test_to_dict(self):
        """Test serialization to dictionary."""
        filter = ContentTypeFilterImpl(
            [ContentTypeFilter.EMAIL, ContentTypeFilter.MEETING],
            exclude=True,
        )

        result = filter.to_dict()

        assert result["type"] == "content_type"
        assert set(result["allowed"]) == {"email", "meeting"}
        assert result["exclude"] is True


# =============================================================================
# PARTICIPANT FILTER TESTS
# =============================================================================


@pytest.mark.unit
class TestParticipantFilter:
    """Test participant filter functionality."""

    def test_matches_single_participant_any(self):
        """Test matching any of single participant."""
        filter = ParticipantFilter(["alice@example.com"])

        matching = make_result(participants=["alice@example.com", "bob@example.com"])
        not_matching = make_result(participants=["bob@example.com"])

        assert filter.matches(matching) is True
        assert filter.matches(not_matching) is False

    def test_matches_multiple_participants_any(self):
        """Test matching any of multiple participants (OR logic)."""
        filter = ParticipantFilter(["alice@example.com", "carol@example.com"])

        has_alice = make_result(participants=["alice@example.com"])
        has_carol = make_result(participants=["carol@example.com"])
        has_neither = make_result(participants=["bob@example.com"])

        assert filter.matches(has_alice) is True
        assert filter.matches(has_carol) is True
        assert filter.matches(has_neither) is False

    def test_matches_all_participants(self):
        """Test requiring all participants (AND logic)."""
        filter = ParticipantFilter(
            ["alice@example.com", "bob@example.com"],
            match_any=False,
        )

        has_both = make_result(participants=["alice@example.com", "bob@example.com", "carol@example.com"])
        has_one = make_result(participants=["alice@example.com"])
        has_neither = make_result(participants=["carol@example.com"])

        assert filter.matches(has_both) is True
        assert filter.matches(has_one) is False
        assert filter.matches(has_neither) is False

    def test_case_insensitive(self):
        """Test case-insensitive participant matching."""
        filter = ParticipantFilter(["Alice@Example.COM"])

        result = make_result(participants=["alice@example.com"])

        assert filter.matches(result) is True

    def test_empty_participants(self):
        """Test handling when result has no participants."""
        filter = ParticipantFilter(["alice@example.com"])

        result = make_result(participants=[])

        assert filter.matches(result) is False

    def test_to_dict(self):
        """Test serialization to dictionary."""
        filter = ParticipantFilter(
            ["alice@example.com", "bob@example.com"],
            match_any=False,
        )

        result = filter.to_dict()

        assert result["type"] == "participant"
        assert set(result["participants"]) == {"alice@example.com", "bob@example.com"}
        assert result["match_any"] is False


# =============================================================================
# PROJECT FILTER TESTS
# =============================================================================


@pytest.mark.unit
class TestProjectFilter:
    """Test project filter functionality."""

    def test_matches_single_project_any(self):
        """Test matching any of single project."""
        filter = ProjectFilter(["Atlas"])

        matching = make_result(project_refs=["Atlas", "Beta"])
        not_matching = make_result(project_refs=["Beta"])

        assert filter.matches(matching) is True
        assert filter.matches(not_matching) is False

    def test_matches_multiple_projects_any(self):
        """Test matching any of multiple projects (OR logic)."""
        filter = ProjectFilter(["Atlas", "Charlie"])

        has_atlas = make_result(project_refs=["Atlas"])
        has_charlie = make_result(project_refs=["Charlie"])
        has_neither = make_result(project_refs=["Beta"])

        assert filter.matches(has_atlas) is True
        assert filter.matches(has_charlie) is True
        assert filter.matches(has_neither) is False

    def test_matches_all_projects(self):
        """Test requiring all projects (AND logic)."""
        filter = ProjectFilter(["Atlas", "Beta"], match_any=False)

        has_both = make_result(project_refs=["Atlas", "Beta", "Charlie"])
        has_one = make_result(project_refs=["Atlas"])

        assert filter.matches(has_both) is True
        assert filter.matches(has_one) is False

    def test_case_insensitive(self):
        """Test case-insensitive project matching."""
        filter = ProjectFilter(["ATLAS"])

        result = make_result(project_refs=["atlas"])

        assert filter.matches(result) is True

    def test_empty_projects(self):
        """Test handling when result has no project refs."""
        filter = ProjectFilter(["Atlas"])

        result = make_result(project_refs=[])

        assert filter.matches(result) is False

    def test_to_dict(self):
        """Test serialization to dictionary."""
        filter = ProjectFilter(["Atlas", "Beta"], match_any=False)

        result = filter.to_dict()

        assert result["type"] == "project"
        assert set(result["projects"]) == {"atlas", "beta"}
        assert result["match_any"] is False


# =============================================================================
# CONFIDENCE FILTER TESTS
# =============================================================================


@pytest.mark.unit
class TestConfidenceFilter:
    """Test confidence filter functionality."""

    def test_matches_above_threshold(self):
        """Test result with confidence above threshold passes."""
        filter = ConfidenceFilter(min_confidence=0.8)

        high_confidence = make_result(confidence_score=0.9)
        at_threshold = make_result(confidence_score=0.8)
        below_threshold = make_result(confidence_score=0.7)

        assert filter.matches(high_confidence) is True
        assert filter.matches(at_threshold) is True
        assert filter.matches(below_threshold) is False

    def test_null_confidence_passes(self):
        """Test result without confidence score passes (included by default)."""
        filter = ConfidenceFilter(min_confidence=0.8)

        no_confidence = make_result(confidence_score=None)

        assert filter.matches(no_confidence) is True

    def test_invalid_threshold_raises(self):
        """Test invalid threshold values raise ValueError."""
        with pytest.raises(ValueError, match="must be between 0.0 and 1.0"):
            ConfidenceFilter(min_confidence=-0.1)

        with pytest.raises(ValueError, match="must be between 0.0 and 1.0"):
            ConfidenceFilter(min_confidence=1.1)

    def test_edge_thresholds(self):
        """Test edge case thresholds 0.0 and 1.0."""
        zero_filter = ConfidenceFilter(min_confidence=0.0)
        one_filter = ConfidenceFilter(min_confidence=1.0)

        low_confidence = make_result(confidence_score=0.0)
        high_confidence = make_result(confidence_score=1.0)

        assert zero_filter.matches(low_confidence) is True
        assert one_filter.matches(high_confidence) is True
        assert one_filter.matches(low_confidence) is False

    def test_to_dict(self):
        """Test serialization to dictionary."""
        filter = ConfidenceFilter(min_confidence=0.75)

        result = filter.to_dict()

        assert result["type"] == "confidence"
        assert result["min_confidence"] == 0.75


# =============================================================================
# DATE RANGE FILTER TESTS
# =============================================================================


@pytest.mark.unit
class TestDateRangeFilter:
    """Test date range filter functionality."""

    def test_after_date(self):
        """Test filtering after a specific date."""
        now = datetime.utcnow()
        start = now - timedelta(days=7)

        filter = DateRangeFilter(start_date=start)

        recent = make_result(timestamp=now - timedelta(days=3))
        old = make_result(timestamp=now - timedelta(days=14))

        assert filter.matches(recent) is True
        assert filter.matches(old) is False

    def test_before_date(self):
        """Test filtering before a specific date."""
        now = datetime.utcnow()
        end = now - timedelta(days=7)

        filter = DateRangeFilter(end_date=end)

        recent = make_result(timestamp=now - timedelta(days=3))
        old = make_result(timestamp=now - timedelta(days=14))

        assert filter.matches(recent) is False
        assert filter.matches(old) is True

    def test_date_range_between(self):
        """Test filtering between two dates."""
        now = datetime.utcnow()
        start = now - timedelta(days=14)
        end = now - timedelta(days=7)

        filter = DateRangeFilter(start_date=start, end_date=end)

        too_recent = make_result(timestamp=now - timedelta(days=3))
        in_range = make_result(timestamp=now - timedelta(days=10))
        too_old = make_result(timestamp=now - timedelta(days=21))

        assert filter.matches(too_recent) is False
        assert filter.matches(in_range) is True
        assert filter.matches(too_old) is False

    def test_no_constraints(self):
        """Test filter with no date constraints matches all."""
        filter = DateRangeFilter()

        old = make_result(timestamp=datetime(2020, 1, 1))
        recent = make_result(timestamp=datetime.utcnow())

        assert filter.matches(old) is True
        assert filter.matches(recent) is True

    def test_to_dict(self):
        """Test serialization to dictionary."""
        start = datetime(2026, 1, 1, 12, 0, 0)
        end = datetime(2026, 1, 15, 12, 0, 0)

        filter = DateRangeFilter(start_date=start, end_date=end)

        result = filter.to_dict()

        assert result["type"] == "date_range"
        assert result["start_date"] == "2026-01-01T12:00:00"
        assert result["end_date"] == "2026-01-15T12:00:00"

    def test_to_dict_null_dates(self):
        """Test serialization with null dates."""
        filter = DateRangeFilter()

        result = filter.to_dict()

        assert result["start_date"] is None
        assert result["end_date"] is None


# =============================================================================
# FILTER PIPELINE TESTS
# =============================================================================


@pytest.mark.unit
class TestFilterPipeline:
    """Test filter pipeline functionality."""

    def test_empty_pipeline(self):
        """Test pipeline with no filters passes all results."""
        pipeline = FilterPipeline()

        results = [
            make_result(content_type=ContentTypeFilter.EMAIL),
            make_result(content_type=ContentTypeFilter.MEETING),
        ]

        filtered, stats = pipeline.apply(results)

        assert len(filtered) == 2
        assert stats.total_before == 2
        assert stats.total_after == 2
        assert stats.reduction_percentage == 0.0

    def test_single_filter(self):
        """Test pipeline with single filter."""
        pipeline = FilterPipeline()
        pipeline.add_filter(ContentTypeFilterImpl([ContentTypeFilter.EMAIL]))

        results = [
            make_result(content_type=ContentTypeFilter.EMAIL),
            make_result(content_type=ContentTypeFilter.MEETING),
            make_result(content_type=ContentTypeFilter.EMAIL),
        ]

        filtered, stats = pipeline.apply(results)

        assert len(filtered) == 2
        assert stats.total_before == 3
        assert stats.total_after == 2
        assert stats.by_content_type == {"email": 2}

    def test_multiple_filters_and_logic(self):
        """Test pipeline combines filters with AND logic."""
        pipeline = FilterPipeline()
        pipeline.add_filter(ContentTypeFilterImpl([ContentTypeFilter.EMAIL]))
        pipeline.add_filter(ParticipantFilter(["alice@example.com"]))

        results = [
            # Matches both: email AND has alice
            make_result(
                content_type=ContentTypeFilter.EMAIL,
                participants=["alice@example.com", "bob@example.com"],
            ),
            # Fails: meeting (wrong type)
            make_result(
                content_type=ContentTypeFilter.MEETING,
                participants=["alice@example.com"],
            ),
            # Fails: email but no alice
            make_result(
                content_type=ContentTypeFilter.EMAIL,
                participants=["bob@example.com"],
            ),
        ]

        filtered, stats = pipeline.apply(results)

        assert len(filtered) == 1
        assert stats.total_before == 3
        assert stats.total_after == 1
        assert pytest.approx(stats.reduction_percentage, rel=0.01) == 66.67

    def test_method_chaining(self):
        """Test add_filter returns self for chaining."""
        pipeline = (
            FilterPipeline()
            .add_filter(ContentTypeFilterImpl([ContentTypeFilter.EMAIL]))
            .add_filter(ParticipantFilter(["alice@example.com"]))
        )

        assert len(pipeline) == 2

    def test_statistics_by_content_type(self):
        """Test statistics tracks content type breakdown."""
        pipeline = FilterPipeline()
        pipeline.add_filter(ParticipantFilter(["alice@example.com"]))

        results = [
            make_result(
                content_type=ContentTypeFilter.EMAIL,
                participants=["alice@example.com"],
            ),
            make_result(
                content_type=ContentTypeFilter.EMAIL,
                participants=["alice@example.com"],
            ),
            make_result(
                content_type=ContentTypeFilter.MEETING,
                participants=["alice@example.com"],
            ),
            make_result(
                content_type=ContentTypeFilter.SLACK,
                participants=["bob@example.com"],
            ),
        ]

        _, stats = pipeline.apply(results)

        assert stats.by_content_type["email"] == 2
        assert stats.by_content_type["meeting"] == 1
        assert "slack" not in stats.by_content_type

    def test_empty_results(self):
        """Test pipeline handles empty input."""
        pipeline = FilterPipeline()
        pipeline.add_filter(ContentTypeFilterImpl([ContentTypeFilter.EMAIL]))

        filtered, stats = pipeline.apply([])

        assert len(filtered) == 0
        assert stats.total_before == 0
        assert stats.total_after == 0
        assert stats.reduction_percentage == 0.0

    def test_len_and_bool(self):
        """Test __len__ and __bool__ magic methods."""
        empty_pipeline = FilterPipeline()
        assert len(empty_pipeline) == 0
        assert not empty_pipeline

        pipeline = FilterPipeline()
        pipeline.add_filter(ContentTypeFilterImpl([ContentTypeFilter.EMAIL]))
        assert len(pipeline) == 1
        assert pipeline

    def test_from_search_query_content_types(self):
        """Test building pipeline from SearchQuery with content types."""
        query = SearchQuery(
            query_text="test query",
            content_types=[ContentTypeFilter.EMAIL, ContentTypeFilter.MEETING],
        )

        pipeline = FilterPipeline.from_search_query(query)

        assert len(pipeline) == 1
        assert isinstance(pipeline.filters[0], ContentTypeFilterImpl)

    def test_from_search_query_all_content_types_no_filter(self):
        """Test ALL content type doesn't add a filter."""
        query = SearchQuery(
            query_text="test query",
            content_types=[ContentTypeFilter.ALL],
        )

        pipeline = FilterPipeline.from_search_query(query)

        assert len(pipeline) == 0

    def test_from_search_query_participants(self):
        """Test building pipeline from SearchQuery with participants."""
        query = SearchQuery(
            query_text="test query",
            participants=["alice@example.com", "bob@example.com"],
        )

        pipeline = FilterPipeline.from_search_query(query)

        assert len(pipeline) == 1
        assert isinstance(pipeline.filters[0], ParticipantFilter)

    def test_from_search_query_projects(self):
        """Test building pipeline from SearchQuery with projects."""
        query = SearchQuery(
            query_text="test query",
            projects=["Atlas", "Beta"],
        )

        pipeline = FilterPipeline.from_search_query(query)

        assert len(pipeline) == 1
        assert isinstance(pipeline.filters[0], ProjectFilter)

    def test_from_search_query_confidence(self):
        """Test building pipeline from SearchQuery with min_confidence."""
        query = SearchQuery(
            query_text="test query",
            min_confidence=0.8,
        )

        pipeline = FilterPipeline.from_search_query(query)

        assert len(pipeline) == 1
        assert isinstance(pipeline.filters[0], ConfidenceFilter)

    def test_from_search_query_temporal(self):
        """Test building pipeline from SearchQuery with temporal constraint."""
        query = SearchQuery(
            query_text="test query",
            temporal=TemporalConstraint(
                start_date=datetime(2026, 1, 1),
                end_date=datetime(2026, 1, 15),
            ),
        )

        pipeline = FilterPipeline.from_search_query(query)

        assert len(pipeline) == 1
        assert isinstance(pipeline.filters[0], DateRangeFilter)

    def test_from_search_query_all_filters(self):
        """Test building pipeline from SearchQuery with all filter types."""
        query = SearchQuery(
            query_text="test query",
            content_types=[ContentTypeFilter.EMAIL],
            participants=["alice@example.com"],
            projects=["Atlas"],
            min_confidence=0.8,
            temporal=TemporalConstraint(start_date=datetime(2026, 1, 1)),
        )

        pipeline = FilterPipeline.from_search_query(query)

        assert len(pipeline) == 5


# =============================================================================
# COMBINED FILTER TESTS
# =============================================================================


@pytest.mark.unit
class TestCombinedFilters:
    """Test combining multiple filter types."""

    def test_complex_filter_scenario(self):
        """Test realistic complex filter scenario.

        Filter for:
        - Emails only
        - Involving alice
        - With confidence >= 0.7
        - From the last 30 days
        """
        now = datetime.utcnow()

        pipeline = (
            FilterPipeline()
            .add_filter(ContentTypeFilterImpl([ContentTypeFilter.EMAIL]))
            .add_filter(ParticipantFilter(["alice@example.com"]))
            .add_filter(ConfidenceFilter(0.7))
            .add_filter(DateRangeFilter(start_date=now - timedelta(days=30)))
        )

        results = [
            # PASS: email, has alice, high confidence, recent
            make_result(
                content_type=ContentTypeFilter.EMAIL,
                participants=["alice@example.com", "bob@example.com"],
                confidence_score=0.85,
                timestamp=now - timedelta(days=5),
            ),
            # FAIL: meeting (wrong type)
            make_result(
                content_type=ContentTypeFilter.MEETING,
                participants=["alice@example.com"],
                confidence_score=0.9,
                timestamp=now - timedelta(days=5),
            ),
            # FAIL: no alice
            make_result(
                content_type=ContentTypeFilter.EMAIL,
                participants=["bob@example.com"],
                confidence_score=0.9,
                timestamp=now - timedelta(days=5),
            ),
            # FAIL: low confidence
            make_result(
                content_type=ContentTypeFilter.EMAIL,
                participants=["alice@example.com"],
                confidence_score=0.5,
                timestamp=now - timedelta(days=5),
            ),
            # FAIL: too old
            make_result(
                content_type=ContentTypeFilter.EMAIL,
                participants=["alice@example.com"],
                confidence_score=0.9,
                timestamp=now - timedelta(days=45),
            ),
            # PASS: email, alice, null confidence (passes), recent
            make_result(
                content_type=ContentTypeFilter.EMAIL,
                participants=["alice@example.com"],
                confidence_score=None,
                timestamp=now - timedelta(days=10),
            ),
        ]

        filtered, stats = pipeline.apply(results)

        assert len(filtered) == 2
        assert stats.total_before == 6
        assert stats.total_after == 2


# =============================================================================
# EDGE CASE TESTS
# =============================================================================


@pytest.mark.unit
class TestEdgeCases:
    """Test edge cases and error handling."""

    def test_filter_validation(self):
        """Test filter parameter validation."""
        # Invalid confidence
        with pytest.raises(ValueError):
            ConfidenceFilter(min_confidence=-0.5)

        with pytest.raises(ValueError):
            ConfidenceFilter(min_confidence=1.5)

    def test_empty_filter_lists(self):
        """Test filters with empty lists."""
        # Empty content types - matches nothing
        content_filter = ContentTypeFilterImpl([])
        result = make_result(content_type=ContentTypeFilter.EMAIL)
        assert content_filter.matches(result) is False

        # Empty participants - matches nothing
        participant_filter = ParticipantFilter([])
        result = make_result(participants=["alice@example.com"])
        assert participant_filter.matches(result) is False

        # Empty projects - matches nothing
        project_filter = ProjectFilter([])
        result = make_result(project_refs=["Atlas"])
        assert project_filter.matches(result) is False

    def test_special_characters_in_filters(self):
        """Test handling of special characters in filter values."""
        # Email with special chars
        filter = ParticipantFilter(["alice+tag@example.com"])
        result = make_result(participants=["alice+tag@example.com"])
        assert filter.matches(result) is True

        # Project with special chars
        filter = ProjectFilter(["Q1-2026 Budget"])
        result = make_result(project_refs=["Q1-2026 Budget"])
        assert filter.matches(result) is True

    def test_unicode_in_filters(self):
        """Test handling of unicode in filter values."""
        filter = ProjectFilter(["Projet Alpha"])
        result = make_result(project_refs=["Projet Alpha"])
        assert filter.matches(result) is True

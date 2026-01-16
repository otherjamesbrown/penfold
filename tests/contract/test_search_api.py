"""Contract tests for search API.

Tests the search API contracts including:
- Request schema validation
- Response schema validation
- Query parameter validation
- Error response format
- Filter validation
"""

from datetime import datetime, timezone

import pytest
from pydantic import ValidationError

from penf_lib.search.models import (
    ContentPreview,
    ContentTypeFilter,
    RelatedContentResponse,
    RelatedItemResponse,
    CorrelationType,
    SearchMetadata,
    SearchQuery,
    SearchResponse,
    SearchResult,
    SortOrder,
    TemporalConstraint,
)


@pytest.mark.contract
class TestSearchQueryContract:
    """Test SearchQuery model validation contracts."""

    def test_search_query_minimal_valid(self):
        """SearchQuery accepts minimal valid input."""
        query = SearchQuery(query_text="test query")
        assert query.query_text == "test query"
        assert query.limit == 25  # default
        assert query.offset == 0  # default
        assert query.sort_by == SortOrder.RELEVANCE  # default

    def test_search_query_full_valid(self):
        """SearchQuery accepts full valid input with all fields."""
        query = SearchQuery(
            query_text="deployment issues",
            content_types=[ContentTypeFilter.EMAIL, ContentTypeFilter.MEETING],
            temporal=TemporalConstraint(
                start_date=datetime(2026, 1, 1, tzinfo=timezone.utc),
                end_date=datetime(2026, 1, 15, tzinfo=timezone.utc),
            ),
            participants=["alice@example.com", "bob@example.com"],
            projects=["Atlas", "Phoenix"],
            min_confidence=0.7,
            include_relationships=True,
            sort_by=SortOrder.RECENCY,
            limit=50,
            offset=10,
        )
        assert query.query_text == "deployment issues"
        assert ContentTypeFilter.EMAIL in query.content_types
        assert query.limit == 50
        assert query.offset == 10

    def test_search_query_empty_text_rejected(self):
        """SearchQuery rejects empty query text."""
        with pytest.raises(ValidationError) as exc_info:
            SearchQuery(query_text="")
        errors = exc_info.value.errors()
        assert any("query_text" in str(e) for e in errors)

    def test_search_query_text_too_long_rejected(self):
        """SearchQuery rejects query text exceeding 500 characters."""
        long_text = "a" * 501
        with pytest.raises(ValidationError) as exc_info:
            SearchQuery(query_text=long_text)
        errors = exc_info.value.errors()
        assert any("query_text" in str(e) for e in errors)

    def test_search_query_limit_bounds(self):
        """SearchQuery validates limit bounds (1-100)."""
        # Valid limits
        query_min = SearchQuery(query_text="test", limit=1)
        assert query_min.limit == 1

        query_max = SearchQuery(query_text="test", limit=100)
        assert query_max.limit == 100

        # Invalid limits
        with pytest.raises(ValidationError):
            SearchQuery(query_text="test", limit=0)

        with pytest.raises(ValidationError):
            SearchQuery(query_text="test", limit=101)

    def test_search_query_offset_non_negative(self):
        """SearchQuery requires non-negative offset."""
        query = SearchQuery(query_text="test", offset=0)
        assert query.offset == 0

        with pytest.raises(ValidationError):
            SearchQuery(query_text="test", offset=-1)

    def test_search_query_confidence_bounds(self):
        """SearchQuery validates confidence score bounds (0-1)."""
        query_min = SearchQuery(query_text="test", min_confidence=0.0)
        assert query_min.min_confidence == 0.0

        query_max = SearchQuery(query_text="test", min_confidence=1.0)
        assert query_max.min_confidence == 1.0

        with pytest.raises(ValidationError):
            SearchQuery(query_text="test", min_confidence=-0.1)

        with pytest.raises(ValidationError):
            SearchQuery(query_text="test", min_confidence=1.1)


@pytest.mark.contract
class TestTemporalConstraintContract:
    """Test TemporalConstraint model validation contracts."""

    def test_temporal_constraint_start_only(self):
        """TemporalConstraint accepts start date only."""
        tc = TemporalConstraint(
            start_date=datetime(2026, 1, 1, tzinfo=timezone.utc)
        )
        assert tc.start_date is not None
        assert tc.end_date is None

    def test_temporal_constraint_end_only(self):
        """TemporalConstraint accepts end date only."""
        tc = TemporalConstraint(
            end_date=datetime(2026, 1, 15, tzinfo=timezone.utc)
        )
        assert tc.start_date is None
        assert tc.end_date is not None

    def test_temporal_constraint_date_range(self):
        """TemporalConstraint accepts valid date range."""
        tc = TemporalConstraint(
            start_date=datetime(2026, 1, 1, tzinfo=timezone.utc),
            end_date=datetime(2026, 1, 15, tzinfo=timezone.utc),
        )
        assert tc.start_date < tc.end_date

    def test_temporal_constraint_invalid_range_rejected(self):
        """TemporalConstraint rejects start after end."""
        with pytest.raises(ValidationError) as exc_info:
            TemporalConstraint(
                start_date=datetime(2026, 1, 15, tzinfo=timezone.utc),
                end_date=datetime(2026, 1, 1, tzinfo=timezone.utc),
            )
        assert "start date" in str(exc_info.value).lower() or "before" in str(exc_info.value).lower()

    def test_temporal_constraint_with_relative_expression(self):
        """TemporalConstraint accepts relative expression."""
        tc = TemporalConstraint(
            relative_expression="last week"
        )
        assert tc.relative_expression == "last week"


@pytest.mark.contract
class TestSearchResponseContract:
    """Test SearchResponse model structure contracts."""

    def test_search_response_structure(self):
        """SearchResponse has required fields with correct types."""
        response = SearchResponse(
            metadata=SearchMetadata(
                query_id="test-query-id",
                execution_time_ms=150,
                total_results=10,
                returned_results=10,
                search_strategy="hybrid",
                cache_hit=False,
            ),
            results=[],
            suggestions=[],
            filters_applied={},
            has_more=False,
            next_offset=None,
        )
        assert response.metadata.query_id == "test-query-id"
        assert response.metadata.execution_time_ms == 150
        assert response.has_more is False
        assert response.next_offset is None

    def test_search_response_with_results(self):
        """SearchResponse correctly contains results."""
        result = SearchResult(
            result_id="result-1",
            entity_type="source",
            entity_id=12345,
            content_type=ContentTypeFilter.EMAIL,
            preview=ContentPreview(
                title="Test Email",
                snippet="This is a test email content...",
                highlight_positions=[(10, 14)],
            ),
            source_attribution="gmail://msg/abc123",
            relevance_score=0.92,
            rrf_score=0.085,
            timestamp=datetime(2026, 1, 10, 14, 30, tzinfo=timezone.utc),
            participants=["alice@example.com"],
        )

        response = SearchResponse(
            metadata=SearchMetadata(
                query_id="test-query-id",
                execution_time_ms=150,
                total_results=1,
                returned_results=1,
                search_strategy="hybrid",
                cache_hit=False,
            ),
            results=[result],
            suggestions=[],
            filters_applied={},
            has_more=False,
            next_offset=None,
        )
        assert len(response.results) == 1
        assert response.results[0].entity_id == 12345

    def test_search_response_pagination(self):
        """SearchResponse pagination fields work correctly."""
        response = SearchResponse(
            metadata=SearchMetadata(
                query_id="test-query-id",
                execution_time_ms=150,
                total_results=100,
                returned_results=25,
                search_strategy="hybrid",
                cache_hit=False,
            ),
            results=[],
            suggestions=[],
            filters_applied={},
            has_more=True,
            next_offset=25,
        )
        assert response.has_more is True
        assert response.next_offset == 25
        assert response.metadata.total_results == 100
        assert response.metadata.returned_results == 25

    def test_search_response_with_suggestions(self):
        """SearchResponse can contain suggestions."""
        response = SearchResponse(
            metadata=SearchMetadata(
                query_id="test-query-id",
                execution_time_ms=150,
                total_results=0,
                returned_results=0,
                search_strategy="no_results",
                cache_hit=False,
            ),
            results=[],
            suggestions=[
                "Did you mean: deployment",
                "Try using fewer search terms",
            ],
            filters_applied={},
            has_more=False,
            next_offset=None,
        )
        assert len(response.suggestions) == 2
        assert "Did you mean" in response.suggestions[0]


@pytest.mark.contract
class TestSearchResultContract:
    """Test SearchResult model structure contracts."""

    def test_search_result_required_fields(self):
        """SearchResult requires all mandatory fields."""
        result = SearchResult(
            result_id="result-1",
            entity_type="source",
            entity_id=12345,
            content_type=ContentTypeFilter.EMAIL,
            preview=ContentPreview(
                title="Test",
                snippet="Test content...",
            ),
            source_attribution="gmail://msg/abc123",
            relevance_score=0.9,
            rrf_score=0.08,
            timestamp=datetime(2026, 1, 10, tzinfo=timezone.utc),
        )
        assert result.result_id == "result-1"
        assert result.entity_type == "source"
        assert result.content_type == ContentTypeFilter.EMAIL

    def test_search_result_relevance_score_bounds(self):
        """SearchResult validates relevance score bounds (0-1)."""
        # Valid scores
        result = SearchResult(
            result_id="result-1",
            entity_type="source",
            entity_id=12345,
            content_type=ContentTypeFilter.EMAIL,
            preview=ContentPreview(snippet="Test"),
            source_attribution="test://source",
            relevance_score=0.0,
            rrf_score=0.0,
            timestamp=datetime.now(timezone.utc),
        )
        assert result.relevance_score == 0.0

        result_max = SearchResult(
            result_id="result-2",
            entity_type="source",
            entity_id=12346,
            content_type=ContentTypeFilter.EMAIL,
            preview=ContentPreview(snippet="Test"),
            source_attribution="test://source",
            relevance_score=1.0,
            rrf_score=0.1,
            timestamp=datetime.now(timezone.utc),
        )
        assert result_max.relevance_score == 1.0

        # Invalid scores
        with pytest.raises(ValidationError):
            SearchResult(
                result_id="result-3",
                entity_type="source",
                entity_id=12347,
                content_type=ContentTypeFilter.EMAIL,
                preview=ContentPreview(snippet="Test"),
                source_attribution="test://source",
                relevance_score=-0.1,
                rrf_score=0.0,
                timestamp=datetime.now(timezone.utc),
            )

        with pytest.raises(ValidationError):
            SearchResult(
                result_id="result-4",
                entity_type="source",
                entity_id=12348,
                content_type=ContentTypeFilter.EMAIL,
                preview=ContentPreview(snippet="Test"),
                source_attribution="test://source",
                relevance_score=1.1,
                rrf_score=0.0,
                timestamp=datetime.now(timezone.utc),
            )

    def test_search_result_optional_fields(self):
        """SearchResult optional fields have correct defaults."""
        result = SearchResult(
            result_id="result-1",
            entity_type="source",
            entity_id=12345,
            content_type=ContentTypeFilter.EMAIL,
            preview=ContentPreview(snippet="Test"),
            source_attribution="test://source",
            relevance_score=0.9,
            rrf_score=0.08,
            timestamp=datetime.now(timezone.utc),
        )
        # Check defaults
        assert result.confidence_score is None
        assert result.participants == []
        assert result.project_refs == []
        assert result.tags == []
        assert result.related_content_ids == []
        assert result.relationship_strength is None


@pytest.mark.contract
class TestSearchMetadataContract:
    """Test SearchMetadata model contracts."""

    def test_search_metadata_required_fields(self):
        """SearchMetadata requires all fields."""
        metadata = SearchMetadata(
            query_id="query-123",
            execution_time_ms=150,
            total_results=100,
            returned_results=25,
            search_strategy="hybrid",
            cache_hit=False,
        )
        assert metadata.query_id == "query-123"
        assert metadata.execution_time_ms == 150
        assert metadata.search_strategy == "hybrid"

    def test_search_metadata_execution_time_non_negative(self):
        """SearchMetadata requires non-negative execution time."""
        metadata = SearchMetadata(
            query_id="query-123",
            execution_time_ms=0,
            total_results=0,
            returned_results=0,
            search_strategy="hybrid",
            cache_hit=False,
        )
        assert metadata.execution_time_ms == 0

        with pytest.raises(ValidationError):
            SearchMetadata(
                query_id="query-123",
                execution_time_ms=-1,
                total_results=0,
                returned_results=0,
                search_strategy="hybrid",
                cache_hit=False,
            )


@pytest.mark.contract
class TestContentTypeFilterContract:
    """Test ContentTypeFilter enum contracts."""

    def test_content_type_filter_values(self):
        """ContentTypeFilter has expected values."""
        assert ContentTypeFilter.EMAIL.value == "email"
        assert ContentTypeFilter.MEETING.value == "meeting"
        assert ContentTypeFilter.DOCUMENT.value == "document"
        assert ContentTypeFilter.SLACK.value == "slack"
        assert ContentTypeFilter.ALL.value == "all"

    def test_content_type_filter_in_query(self):
        """ContentTypeFilter can be used in SearchQuery."""
        query = SearchQuery(
            query_text="test",
            content_types=[ContentTypeFilter.EMAIL, ContentTypeFilter.MEETING],
        )
        assert ContentTypeFilter.EMAIL in query.content_types
        assert ContentTypeFilter.MEETING in query.content_types
        assert ContentTypeFilter.DOCUMENT not in query.content_types


@pytest.mark.contract
class TestRelatedContentContract:
    """Test RelatedContentResponse model contracts."""

    def test_related_content_response_structure(self):
        """RelatedContentResponse has required structure."""
        response = RelatedContentResponse(
            entity_type="source",
            entity_id=12345,
            related_content=[],
            total_correlations=0,
            execution_time_ms=50,
        )
        assert response.entity_type == "source"
        assert response.entity_id == 12345
        assert response.total_correlations == 0

    def test_related_item_response_structure(self):
        """RelatedItemResponse has required structure."""
        item = RelatedItemResponse(
            entity_id=12346,
            entity_type="source",
            content_type=ContentTypeFilter.EMAIL,
            title="Related Email",
            timestamp=datetime.now(timezone.utc),
            correlation_score=0.85,
            correlation_type=CorrelationType.PARTICIPANT,
            score_breakdown={"participant": 0.9, "temporal": 0.7},
        )
        assert item.entity_id == 12346
        assert item.correlation_score == 0.85
        assert item.correlation_type == CorrelationType.PARTICIPANT

    def test_correlation_type_values(self):
        """CorrelationType has expected values."""
        assert CorrelationType.PARTICIPANT.value == "participant"
        assert CorrelationType.PROJECT.value == "project"
        assert CorrelationType.TEMPORAL.value == "temporal"
        assert CorrelationType.SEMANTIC.value == "semantic"
        assert CorrelationType.THREAD.value == "thread"


@pytest.mark.contract
class TestErrorResponseContract:
    """Test error response format contracts.

    These tests verify that validation errors follow expected patterns.
    """

    def test_validation_error_has_details(self):
        """Validation errors include field-level details."""
        with pytest.raises(ValidationError) as exc_info:
            SearchQuery(query_text="", limit=-1)

        errors = exc_info.value.errors()
        # Should have multiple errors for both invalid fields
        assert len(errors) >= 1

        # Errors should have location info
        for error in errors:
            assert "loc" in error
            assert "msg" in error
            assert "type" in error

    def test_validation_error_identifies_field(self):
        """Validation errors identify the problematic field."""
        with pytest.raises(ValidationError) as exc_info:
            SearchQuery(query_text="test", limit=200)

        errors = exc_info.value.errors()
        # Should identify 'limit' field
        field_names = [str(e.get("loc", ())) for e in errors]
        assert any("limit" in f for f in field_names)

    def test_validation_error_message_is_helpful(self):
        """Validation error messages are human-readable."""
        with pytest.raises(ValidationError) as exc_info:
            TemporalConstraint(
                start_date=datetime(2026, 1, 15, tzinfo=timezone.utc),
                end_date=datetime(2026, 1, 1, tzinfo=timezone.utc),
            )

        # Error message should explain the problem
        error_str = str(exc_info.value)
        assert len(error_str) > 0

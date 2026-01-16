"""Unit tests for query parser module.

Tests query parsing functionality for the search interface including:
- Simple query parsing
- Abbreviation expansion
- Stop word removal
- Participant filter extraction
- Content type filter extraction
- Project filter extraction
- Quoted phrase preservation
- Temporal expression parsing
"""

from datetime import datetime, timedelta, timezone
from unittest.mock import patch

import pytest

from penf_lib.search.query_parser import QueryEmbedder, QueryParser, TemporalQueryParser


@pytest.mark.unit
class TestQueryParserNormalization:
    """Test query normalization functionality."""

    def test_normalize_lowercase(self):
        """Normalize converts query to lowercase."""
        parser = QueryParser()
        result = parser.normalize("UPPERCASE Query TEXT")
        assert result == "uppercase query text"

    def test_normalize_whitespace(self):
        """Normalize collapses multiple whitespace."""
        parser = QueryParser()
        result = parser.normalize("multiple   spaces\ttabs\nnewlines")
        assert result == "multiple spaces tabs newlines"

    def test_normalize_abbreviation_expansion(self):
        """Normalize expands common abbreviations."""
        parser = QueryParser()
        result = parser.normalize("mtg with mgr about proj")
        assert result == "meeting with manager about project"

    def test_normalize_multiple_abbreviations(self):
        """Normalize expands multiple abbreviations in one query."""
        parser = QueryParser()
        result = parser.normalize("dev env doc for prod deployment")
        assert result == "development environment document for production deployment"

    def test_normalize_preserves_unknown_words(self):
        """Normalize preserves words not in abbreviation list."""
        parser = QueryParser()
        result = parser.normalize("xyz unknown123 words")
        assert result == "xyz unknown123 words"

    def test_normalize_with_stop_word_removal(self):
        """Normalize removes stop words when enabled."""
        parser = QueryParser(remove_stop_words=True)
        result = parser.normalize("the meeting with the team")
        assert result == "meeting team"

    def test_normalize_without_stop_word_removal(self):
        """Normalize preserves stop words by default."""
        parser = QueryParser()
        result = parser.normalize("the meeting with the team")
        assert result == "the meeting with the team"

    def test_normalize_preserves_quoted_phrases(self):
        """Normalize preserves text within quotes."""
        parser = QueryParser()
        result = parser.normalize('search for "EXACT Match" phrase')
        assert result == 'search for "EXACT Match" phrase'

    def test_normalize_multiple_quoted_phrases(self):
        """Normalize preserves multiple quoted phrases."""
        parser = QueryParser()
        result = parser.normalize('"First Phrase" and "Second Phrase"')
        assert result == '"First Phrase" and "Second Phrase"'

    def test_normalize_empty_string(self):
        """Normalize handles empty string."""
        parser = QueryParser()
        result = parser.normalize("")
        assert result == ""

    def test_normalize_only_whitespace(self):
        """Normalize handles whitespace-only string."""
        parser = QueryParser()
        result = parser.normalize("   \t\n  ")
        assert result == ""


@pytest.mark.unit
class TestQueryParserFilterExtraction:
    """Test filter extraction from queries."""

    def test_extract_single_from_filter(self):
        """Extract single participant from filter."""
        parser = QueryParser()
        remaining, filters = parser.extract_filters("from:alice@example.com meeting notes")
        assert remaining == "meeting notes"
        assert filters["participants"] == ["alice@example.com"]

    def test_extract_multiple_from_filters(self):
        """Extract multiple participant from filters."""
        parser = QueryParser()
        remaining, filters = parser.extract_filters(
            "from:alice@example.com from:bob@example.com discussion"
        )
        assert remaining == "discussion"
        assert filters["participants"] == ["alice@example.com", "bob@example.com"]

    def test_extract_type_filter(self):
        """Extract content type filter."""
        parser = QueryParser()
        remaining, filters = parser.extract_filters("type:email deployment issues")
        assert remaining == "deployment issues"
        assert filters["content_types"] == ["email"]

    def test_extract_multiple_type_filters(self):
        """Extract multiple content type filters."""
        parser = QueryParser()
        remaining, filters = parser.extract_filters("type:email type:meeting discussion")
        assert remaining == "discussion"
        assert filters["content_types"] == ["email", "meeting"]

    def test_extract_project_filter(self):
        """Extract project filter."""
        parser = QueryParser()
        remaining, filters = parser.extract_filters("project:Atlas deployment status")
        assert remaining == "deployment status"
        assert filters["projects"] == ["Atlas"]

    def test_extract_combined_filters(self):
        """Extract multiple different filter types."""
        parser = QueryParser()
        remaining, filters = parser.extract_filters(
            "from:alice@example.com type:email project:Atlas deployment"
        )
        assert remaining == "deployment"
        assert filters["participants"] == ["alice@example.com"]
        assert filters["content_types"] == ["email"]
        assert filters["projects"] == ["Atlas"]

    def test_extract_no_filters(self):
        """Handle query with no filters."""
        parser = QueryParser()
        remaining, filters = parser.extract_filters("plain text query")
        assert remaining == "plain text query"
        assert filters == {}

    def test_extract_filter_case_insensitive(self):
        """Filter keywords are case insensitive."""
        parser = QueryParser()
        remaining, filters = parser.extract_filters("FROM:alice@example.com TYPE:email query")
        assert remaining == "query"
        assert filters["participants"] == ["alice@example.com"]
        assert filters["content_types"] == ["email"]

    def test_extract_filters_cleans_whitespace(self):
        """Extract filters cleans up extra whitespace in remaining query."""
        parser = QueryParser()
        remaining, filters = parser.extract_filters(
            "from:alice@example.com   extra   spaces   type:email"
        )
        assert remaining == "extra spaces"


@pytest.mark.unit
class TestQueryParserParse:
    """Test the complete parse method."""

    def test_parse_simple_query(self):
        """Parse simple query without filters."""
        parser = QueryParser()
        normalized, filters, temporal = parser.parse("Simple Query Text")
        assert normalized == "simple query text"
        assert filters == {}
        assert temporal is None

    def test_parse_query_with_filters(self):
        """Parse query with filters and normalize text."""
        parser = QueryParser()
        normalized, filters, temporal = parser.parse("from:alice@example.com MTG notes about PROJ")
        assert normalized == "meeting notes about project"
        assert filters["participants"] == ["alice@example.com"]
        assert temporal is None

    def test_parse_query_with_quoted_phrase(self):
        """Parse query preserving quoted phrase."""
        parser = QueryParser()
        normalized, filters, temporal = parser.parse('type:email "Exact Subject"')
        assert normalized == '"Exact Subject"'
        assert filters["content_types"] == ["email"]
        assert temporal is None

    def test_parse_complex_query(self):
        """Parse complex query with multiple components."""
        parser = QueryParser()
        normalized, filters, temporal = parser.parse(
            'from:alice@example.com type:email project:Atlas "Critical Issue" mtg'
        )
        assert normalized == '"Critical Issue" meeting'
        assert filters["participants"] == ["alice@example.com"]
        assert filters["content_types"] == ["email"]
        assert filters["projects"] == ["Atlas"]
        assert temporal is None

    def test_parse_empty_query(self):
        """Parse empty query."""
        parser = QueryParser()
        normalized, filters, temporal = parser.parse("")
        assert normalized == ""
        assert filters == {}
        assert temporal is None

    def test_parse_only_filters(self):
        """Parse query with only filters."""
        parser = QueryParser()
        normalized, filters, temporal = parser.parse("from:alice@example.com type:email")
        assert normalized == ""
        assert filters["participants"] == ["alice@example.com"]
        assert filters["content_types"] == ["email"]
        assert temporal is None


@pytest.mark.unit
class TestQueryParserSpecialCharacters:
    """Test handling of special characters in queries."""

    def test_normalize_with_punctuation(self):
        """Normalize handles punctuation."""
        parser = QueryParser()
        result = parser.normalize("meeting! notes? about, project.")
        assert result == "meeting! notes? about, project."

    def test_normalize_abbreviation_with_punctuation(self):
        """Normalize expands abbreviation followed by punctuation."""
        parser = QueryParser()
        result = parser.normalize("mtg!")
        assert result == "meeting"

    def test_extract_filter_with_special_chars_in_value(self):
        """Extract filter with special characters in value."""
        parser = QueryParser()
        remaining, filters = parser.extract_filters("from:user+tag@example.com query")
        assert remaining == "query"
        assert filters["participants"] == ["user+tag@example.com"]


@pytest.mark.unit
class TestQueryEmbedder:
    """Test query embedding functionality."""

    @pytest.mark.asyncio
    async def test_embed_returns_vector(self):
        """Embed returns a 768-dimensional vector."""
        embedder = QueryEmbedder()
        result = await embedder.embed("test query")
        assert isinstance(result, list)
        assert len(result) == 768
        assert all(isinstance(x, float) for x in result)

    @pytest.mark.asyncio
    async def test_embed_batch_returns_vectors(self):
        """Embed batch returns list of vectors."""
        embedder = QueryEmbedder()
        queries = ["query one", "query two", "query three"]
        results = await embedder.embed_batch(queries)
        assert len(results) == 3
        assert all(len(v) == 768 for v in results)

    def test_embedder_default_model(self):
        """Embedder uses correct default model."""
        embedder = QueryEmbedder()
        assert embedder.model_name == "nomic-embed-text"

    def test_embedder_custom_model(self):
        """Embedder accepts custom model name."""
        embedder = QueryEmbedder(model_name="custom-model")
        assert embedder.model_name == "custom-model"


@pytest.mark.unit
class TestTemporalQueryParserRelativePatterns:
    """Test relative temporal expression parsing."""

    def test_last_week_pattern(self):
        """Parse 'last week' temporal expression."""
        parser = TemporalQueryParser()
        remaining, temporal = parser.extract_temporal("emails from last week")
        assert remaining == "emails from"
        assert temporal is not None
        assert temporal.start_date is not None
        assert temporal.end_date is not None
        # Start should be about 7 days ago
        now = datetime.now(timezone.utc)
        expected_start = now - timedelta(weeks=1)
        assert abs((temporal.start_date - expected_start).total_seconds()) < 60

    def test_last_month_pattern(self):
        """Parse 'last month' temporal expression."""
        parser = TemporalQueryParser()
        remaining, temporal = parser.extract_temporal("meetings last month")
        assert remaining == "meetings"
        assert temporal is not None
        now = datetime.now(timezone.utc)
        expected_start = now - timedelta(days=30)
        assert abs((temporal.start_date - expected_start).total_seconds()) < 60

    def test_yesterday_pattern(self):
        """Parse 'yesterday' temporal expression."""
        parser = TemporalQueryParser()
        remaining, temporal = parser.extract_temporal("what happened yesterday")
        assert remaining == "what happened"
        assert temporal is not None
        assert temporal.start_date is not None
        assert temporal.end_date is not None
        # Both should be yesterday
        now = datetime.now(timezone.utc)
        yesterday_start = (now - timedelta(days=1)).replace(
            hour=0, minute=0, second=0, microsecond=0
        )
        assert abs((temporal.start_date - yesterday_start).total_seconds()) < 60

    def test_today_pattern(self):
        """Parse 'today' temporal expression."""
        parser = TemporalQueryParser()
        remaining, temporal = parser.extract_temporal("emails today about project")
        assert remaining == "emails about project"
        assert temporal is not None
        now = datetime.now(timezone.utc)
        today_start = now.replace(hour=0, minute=0, second=0, microsecond=0)
        assert abs((temporal.start_date - today_start).total_seconds()) < 60

    def test_this_week_pattern(self):
        """Parse 'this week' temporal expression."""
        parser = TemporalQueryParser()
        remaining, temporal = parser.extract_temporal("tasks this week")
        assert remaining == "tasks"
        assert temporal is not None
        # Start should be Monday of current week
        now = datetime.now(timezone.utc)
        monday = now - timedelta(days=now.weekday())
        monday = monday.replace(hour=0, minute=0, second=0, microsecond=0)
        assert abs((temporal.start_date - monday).total_seconds()) < 60

    def test_this_month_pattern(self):
        """Parse 'this month' temporal expression."""
        parser = TemporalQueryParser()
        remaining, temporal = parser.extract_temporal("reports this month")
        assert remaining == "reports"
        assert temporal is not None
        now = datetime.now(timezone.utc)
        month_start = now.replace(day=1, hour=0, minute=0, second=0, microsecond=0)
        assert abs((temporal.start_date - month_start).total_seconds()) < 60

    def test_case_insensitive_relative(self):
        """Relative patterns are case insensitive."""
        parser = TemporalQueryParser()
        remaining, temporal = parser.extract_temporal("emails from LAST WEEK")
        assert remaining == "emails from"
        assert temporal is not None


@pytest.mark.unit
class TestTemporalQueryParserRangePatterns:
    """Test range temporal expression parsing."""

    def test_between_and_pattern(self):
        """Parse 'between X and Y' temporal expression."""
        parser = TemporalQueryParser()
        remaining, temporal = parser.extract_temporal(
            "emails between January 1 and January 15"
        )
        assert remaining == "emails"
        assert temporal is not None
        assert temporal.start_date is not None
        assert temporal.end_date is not None
        # Start should be January 1
        assert temporal.start_date.month == 1
        assert temporal.start_date.day == 1

    def test_from_to_pattern(self):
        """Parse 'from X to Y' temporal expression."""
        parser = TemporalQueryParser()
        remaining, temporal = parser.extract_temporal(
            "documents from December 1 to December 31"
        )
        assert remaining == "documents"
        assert temporal is not None
        assert temporal.start_date.month == 12
        assert temporal.start_date.day == 1
        assert temporal.end_date.month == 12
        assert temporal.end_date.day == 31


@pytest.mark.unit
class TestTemporalQueryParserSinceBeforeAround:
    """Test since/before/around temporal expression parsing."""

    def test_since_pattern(self):
        """Parse 'since X' temporal expression."""
        parser = TemporalQueryParser()
        remaining, temporal = parser.extract_temporal("updates since December")
        assert remaining == "updates"
        assert temporal is not None
        assert temporal.start_date is not None
        assert temporal.end_date is None
        assert temporal.relative_expression == "since December"
        assert temporal.start_date.month == 12

    def test_since_december_25(self):
        """Parse 'since December 25' temporal expression."""
        parser = TemporalQueryParser()
        remaining, temporal = parser.extract_temporal("emails since December 25")
        assert remaining == "emails"
        assert temporal is not None
        assert temporal.start_date is not None
        assert temporal.relative_expression == "since December 25"
        # Should parse as December 25
        assert temporal.start_date.month == 12
        assert temporal.start_date.day == 25

    def test_before_pattern(self):
        """Parse 'before X' temporal expression."""
        parser = TemporalQueryParser()
        remaining, temporal = parser.extract_temporal("documents before January")
        assert remaining == "documents"
        assert temporal is not None
        assert temporal.start_date is None
        assert temporal.end_date is not None
        assert temporal.relative_expression == "before January"

    def test_around_pattern(self):
        """Parse 'around X' temporal expression with fuzzy expansion."""
        parser = TemporalQueryParser(fuzzy_days=3)
        remaining, temporal = parser.extract_temporal("events around January 10")
        assert remaining == "events"
        assert temporal is not None
        assert temporal.start_date is not None
        assert temporal.end_date is not None
        assert temporal.relative_expression == "around January 10"
        # Should expand by +/- 3 days
        diff = (temporal.end_date - temporal.start_date).days
        assert diff == 6  # 3 days before + 3 days after

    def test_around_custom_fuzzy_days(self):
        """Around pattern respects custom fuzzy_days setting."""
        parser = TemporalQueryParser(fuzzy_days=5)
        remaining, temporal = parser.extract_temporal("around January 15")
        assert temporal is not None
        diff = (temporal.end_date - temporal.start_date).days
        assert diff == 10  # 5 days before + 5 days after


@pytest.mark.unit
class TestTemporalQueryParserNoMatch:
    """Test queries without temporal expressions."""

    def test_no_temporal_expression(self):
        """Query without temporal expression returns None."""
        parser = TemporalQueryParser()
        remaining, temporal = parser.extract_temporal("project status update")
        assert remaining == "project status update"
        assert temporal is None

    def test_empty_query(self):
        """Empty query returns None temporal."""
        parser = TemporalQueryParser()
        remaining, temporal = parser.extract_temporal("")
        assert remaining == ""
        assert temporal is None


@pytest.mark.unit
class TestTemporalQueryParserParseDate:
    """Test the parse_date method."""

    def test_parse_absolute_date(self):
        """Parse absolute date string."""
        parser = TemporalQueryParser()
        result = parser.parse_date("December 15, 2025")
        assert result is not None
        assert result.month == 12
        assert result.day == 15
        assert result.year == 2025

    def test_parse_iso_date(self):
        """Parse ISO format date string."""
        parser = TemporalQueryParser()
        result = parser.parse_date("2025-12-15")
        assert result is not None
        assert result.year == 2025
        assert result.month == 12
        assert result.day == 15

    def test_parse_relative_date(self):
        """Parse relative date string."""
        parser = TemporalQueryParser()
        result = parser.parse_date("3 days ago")
        assert result is not None
        now = datetime.now(timezone.utc)
        expected = now - timedelta(days=3)
        # Allow some tolerance for test execution time
        assert abs((result - expected).total_seconds()) < 60

    def test_parse_invalid_date(self):
        """Invalid date string returns None."""
        parser = TemporalQueryParser()
        result = parser.parse_date("not a date")
        assert result is None


@pytest.mark.unit
class TestQueryParserWithTemporal:
    """Test QueryParser.parse() with temporal integration."""

    def test_parse_with_temporal_expression(self):
        """Parse query with temporal expression returns constraint."""
        parser = QueryParser()
        normalized, filters, temporal = parser.parse("emails last week about project")
        assert normalized == "emails about project"
        assert filters == {}
        assert temporal is not None
        assert temporal.start_date is not None

    def test_parse_with_temporal_and_filters(self):
        """Parse query with both temporal and structured filters."""
        parser = QueryParser()
        normalized, filters, temporal = parser.parse(
            "from:alice@example.com emails since December"
        )
        assert "alice@example.com" in filters.get("participants", [])
        assert temporal is not None
        assert temporal.start_date is not None
        assert temporal.relative_expression == "since December"

    def test_parse_without_temporal(self):
        """Parse query without temporal returns None constraint."""
        parser = QueryParser()
        normalized, filters, temporal = parser.parse("project status update")
        assert normalized == "project status update"
        assert filters == {}
        assert temporal is None

    def test_parse_temporal_with_abbreviations(self):
        """Parse query normalizes abbreviations after temporal extraction."""
        parser = QueryParser()
        normalized, filters, temporal = parser.parse("mtg notes from last week")
        assert "meeting" in normalized
        assert temporal is not None

    def test_parse_complex_query_all_components(self):
        """Parse complex query with temporal, filters, and abbreviations."""
        parser = QueryParser()
        normalized, filters, temporal = parser.parse(
            "from:bob@example.com type:email proj updates since December"
        )
        assert "project" in normalized  # abbreviation expanded
        assert filters.get("participants") == ["bob@example.com"]
        assert filters.get("content_types") == ["email"]
        assert temporal is not None
        assert temporal.relative_expression == "since December"

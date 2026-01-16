"""Unit tests for query parser module.

Tests query parsing functionality for the search interface including:
- Simple query parsing
- Abbreviation expansion
- Stop word removal
- Participant filter extraction
- Content type filter extraction
- Project filter extraction
- Quoted phrase preservation
"""

import pytest

from penf_lib.search.query_parser import QueryEmbedder, QueryParser


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
        normalized, filters = parser.parse("Simple Query Text")
        assert normalized == "simple query text"
        assert filters == {}

    def test_parse_query_with_filters(self):
        """Parse query with filters and normalize text."""
        parser = QueryParser()
        normalized, filters = parser.parse("from:alice@example.com MTG notes about PROJ")
        assert normalized == "meeting notes about project"
        assert filters["participants"] == ["alice@example.com"]

    def test_parse_query_with_quoted_phrase(self):
        """Parse query preserving quoted phrase."""
        parser = QueryParser()
        normalized, filters = parser.parse('type:email "Exact Subject"')
        assert normalized == '"Exact Subject"'
        assert filters["content_types"] == ["email"]

    def test_parse_complex_query(self):
        """Parse complex query with multiple components."""
        parser = QueryParser()
        normalized, filters = parser.parse(
            'from:alice@example.com type:email project:Atlas "Critical Issue" mtg'
        )
        assert normalized == '"Critical Issue" meeting'
        assert filters["participants"] == ["alice@example.com"]
        assert filters["content_types"] == ["email"]
        assert filters["projects"] == ["Atlas"]

    def test_parse_empty_query(self):
        """Parse empty query."""
        parser = QueryParser()
        normalized, filters = parser.parse("")
        assert normalized == ""
        assert filters == {}

    def test_parse_only_filters(self):
        """Parse query with only filters."""
        parser = QueryParser()
        normalized, filters = parser.parse("from:alice@example.com type:email")
        assert normalized == ""
        assert filters["participants"] == ["alice@example.com"]
        assert filters["content_types"] == ["email"]


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

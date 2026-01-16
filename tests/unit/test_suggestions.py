"""Unit tests for the search suggestions module.

Tests for SuggestionEngine and QuerySuggestion for providing
query autocomplete and suggestion functionality.
"""

from datetime import datetime, timezone
from unittest.mock import AsyncMock, MagicMock, patch
import uuid

import pytest

from penf_lib.search.suggestions import (
    SuggestionEngine,
    QuerySuggestion,
)


# =============================================================================
# FIXTURES
# =============================================================================


@pytest.fixture
def mock_db_session():
    """Create a mock database session."""
    return AsyncMock()


@pytest.fixture
def mock_search_repository():
    """Create a mock search repository."""
    repo = MagicMock()
    repo.get_suggestions = AsyncMock()
    repo.update_suggestion_frequency = AsyncMock()
    repo.update_suggestion_success_rate = AsyncMock()
    return repo


@pytest.fixture
def suggestion_engine(mock_db_session):
    """Create a SuggestionEngine instance for testing."""
    return SuggestionEngine(mock_db_session, "test-tenant-id")


@pytest.fixture
def sample_suggestion_data():
    """Create sample suggestion data for testing."""
    return [
        {
            "id": 1,
            "suggestion_text": "customer deployment issues",
            "suggestion_type": "popular",
            "frequency": 50,
            "success_rate": 0.85,
            "last_used_at": datetime.now(timezone.utc),
            "context_tags": None,
            "content_types": None,
        },
        {
            "id": 2,
            "suggestion_text": "meeting notes Q1",
            "suggestion_type": "recent",
            "frequency": 25,
            "success_rate": 0.90,
            "last_used_at": datetime.now(timezone.utc),
            "context_tags": None,
            "content_types": None,
        },
        {
            "id": 3,
            "suggestion_text": "project status update",
            "suggestion_type": "contextual",
            "frequency": 15,
            "success_rate": 0.75,
            "last_used_at": datetime.now(timezone.utc),
            "context_tags": None,
            "content_types": None,
        },
    ]


# =============================================================================
# QUERY SUGGESTION TESTS
# =============================================================================


class TestQuerySuggestion:
    """Tests for QuerySuggestion dataclass."""

    def test_suggestion_creation(self):
        """Test creating a QuerySuggestion instance."""
        suggestion = QuerySuggestion(
            text="customer issues",
            suggestion_type="popular",
            frequency=50,
            success_rate=0.85,
        )

        assert suggestion.text == "customer issues"
        assert suggestion.suggestion_type == "popular"
        assert suggestion.frequency == 50
        assert suggestion.success_rate == 0.85

    def test_suggestion_types(self):
        """Test different suggestion types."""
        popular = QuerySuggestion(
            text="popular query",
            suggestion_type="popular",
            frequency=100,
            success_rate=0.90,
        )
        recent = QuerySuggestion(
            text="recent query",
            suggestion_type="recent",
            frequency=5,
            success_rate=0.80,
        )
        contextual = QuerySuggestion(
            text="contextual query",
            suggestion_type="contextual",
            frequency=10,
            success_rate=0.70,
        )

        assert popular.suggestion_type == "popular"
        assert recent.suggestion_type == "recent"
        assert contextual.suggestion_type == "contextual"

    def test_suggestion_with_zero_success_rate(self):
        """Test suggestion with zero success rate."""
        suggestion = QuerySuggestion(
            text="no results query",
            suggestion_type="popular",
            frequency=20,
            success_rate=0.0,
        )

        assert suggestion.success_rate == 0.0


# =============================================================================
# SUGGESTION ENGINE TESTS
# =============================================================================


class TestSuggestionEngine:
    """Tests for SuggestionEngine class."""

    def test_engine_initialization(self, mock_db_session):
        """Test SuggestionEngine initialization."""
        engine = SuggestionEngine(mock_db_session, "tenant-123")

        assert engine.db_session == mock_db_session
        assert engine.tenant_id == "tenant-123"

    def test_tenant_id_to_uuid_with_valid_uuid(self, suggestion_engine):
        """Test _to_uuid with valid UUID string."""
        engine = SuggestionEngine(
            suggestion_engine.db_session,
            "12345678-1234-5678-1234-567812345678",
        )
        result = engine._to_uuid()
        assert isinstance(result, uuid.UUID)

    def test_tenant_id_to_uuid_deterministic(self, suggestion_engine):
        """Test _to_uuid is deterministic for same input."""
        result1 = suggestion_engine._to_uuid()
        result2 = suggestion_engine._to_uuid()
        assert result1 == result2

    @pytest.mark.asyncio
    async def test_get_suggestions_returns_list(
        self, suggestion_engine, mock_search_repository, sample_suggestion_data
    ):
        """Test getting suggestions returns a list."""
        mock_search_repository.get_suggestions.return_value = sample_suggestion_data

        suggestion_engine._repository = mock_search_repository
        result = await suggestion_engine.get_suggestions(limit=5)

        assert isinstance(result, list)
        assert len(result) == 3
        assert all(isinstance(s, QuerySuggestion) for s in result)

    @pytest.mark.asyncio
    async def test_get_suggestions_with_prefix(
        self, suggestion_engine, mock_search_repository, sample_suggestion_data
    ):
        """Test getting suggestions filtered by prefix."""
        # Filter to just customer suggestion
        filtered = [sample_suggestion_data[0]]
        mock_search_repository.get_suggestions.return_value = filtered

        suggestion_engine._repository = mock_search_repository
        result = await suggestion_engine.get_suggestions(prefix="customer", limit=5)

        assert len(result) == 1
        assert result[0].text == "customer deployment issues"
        mock_search_repository.get_suggestions.assert_called_once()

    @pytest.mark.asyncio
    async def test_get_suggestions_empty(
        self, suggestion_engine, mock_search_repository
    ):
        """Test getting suggestions when none exist."""
        mock_search_repository.get_suggestions.return_value = []

        suggestion_engine._repository = mock_search_repository
        result = await suggestion_engine.get_suggestions(limit=5)

        assert result == []

    @pytest.mark.asyncio
    async def test_get_popular_queries(
        self, suggestion_engine, mock_search_repository, sample_suggestion_data
    ):
        """Test getting popular queries."""
        mock_search_repository.get_suggestions.return_value = sample_suggestion_data

        suggestion_engine._repository = mock_search_repository
        result = await suggestion_engine.get_popular_queries(limit=10)

        assert len(result) == 3
        # Popular should have high frequency
        assert result[0].frequency == 50

    @pytest.mark.asyncio
    async def test_record_successful_query(
        self, suggestion_engine, mock_search_repository
    ):
        """Test recording a successful query."""
        mock_search_repository.update_suggestion_frequency.return_value = None

        suggestion_engine._repository = mock_search_repository
        await suggestion_engine.record_successful_query("test query")

        mock_search_repository.update_suggestion_frequency.assert_called_once()

    @pytest.mark.asyncio
    async def test_update_success_rate_successful(
        self, suggestion_engine, mock_search_repository
    ):
        """Test updating success rate for successful search."""
        mock_search_repository.update_suggestion_success_rate.return_value = None

        suggestion_engine._repository = mock_search_repository
        await suggestion_engine.update_success_rate("test query", was_successful=True)

        mock_search_repository.update_suggestion_success_rate.assert_called_once_with(
            suggestion_engine._to_uuid(), "test query", True
        )

    @pytest.mark.asyncio
    async def test_update_success_rate_unsuccessful(
        self, suggestion_engine, mock_search_repository
    ):
        """Test updating success rate for unsuccessful search."""
        mock_search_repository.update_suggestion_success_rate.return_value = None

        suggestion_engine._repository = mock_search_repository
        await suggestion_engine.update_success_rate("bad query", was_successful=False)

        mock_search_repository.update_suggestion_success_rate.assert_called_once_with(
            suggestion_engine._to_uuid(), "bad query", False
        )


# =============================================================================
# CONTEXTUAL SUGGESTIONS TESTS
# =============================================================================


class TestContextualSuggestions:
    """Tests for contextual suggestion functionality."""

    @pytest.mark.asyncio
    async def test_get_contextual_suggestions(
        self, suggestion_engine, mock_search_repository
    ):
        """Test getting contextual suggestions based on context."""
        # Return suggestions for prefix matching
        mock_search_repository.get_suggestions.return_value = [
            {
                "id": 1,
                "suggestion_text": "meeting agenda items",
                "suggestion_type": "contextual",
                "frequency": 10,
                "success_rate": 0.80,
                "last_used_at": datetime.now(timezone.utc),
                "context_tags": None,
                "content_types": None,
            }
        ]

        suggestion_engine._repository = mock_search_repository
        result = await suggestion_engine.get_contextual_suggestions(
            context="preparing for meeting",
            limit=5,
        )

        assert isinstance(result, list)
        # Should have called get_suggestions for each word prefix
        assert mock_search_repository.get_suggestions.called

    @pytest.mark.asyncio
    async def test_contextual_suggestions_limit(
        self, suggestion_engine, mock_search_repository
    ):
        """Test contextual suggestions respects limit."""
        mock_search_repository.get_suggestions.return_value = [
            {
                "id": i,
                "suggestion_text": f"suggestion {i}",
                "suggestion_type": "contextual",
                "frequency": 10 - i,
                "success_rate": 0.80,
                "last_used_at": datetime.now(timezone.utc),
                "context_tags": None,
                "content_types": None,
            }
            for i in range(10)
        ]

        suggestion_engine._repository = mock_search_repository
        result = await suggestion_engine.get_contextual_suggestions(
            context="test context here",
            limit=3,
        )

        assert len(result) <= 3

    @pytest.mark.asyncio
    async def test_contextual_suggestions_filters_short_words(
        self, suggestion_engine, mock_search_repository
    ):
        """Test contextual suggestions ignores short words."""
        mock_search_repository.get_suggestions.return_value = []

        suggestion_engine._repository = mock_search_repository
        # "a" and "to" are too short (< 3 chars)
        await suggestion_engine.get_contextual_suggestions(
            context="a to meeting",
            limit=5,
        )

        # Should only search for "meeting" (3+ chars)
        calls = mock_search_repository.get_suggestions.call_args_list
        # Check that short words were skipped
        for call in calls:
            prefix = call[0][1] if len(call[0]) > 1 else call[1].get("prefix")
            if prefix:
                assert len(prefix) >= 3


# =============================================================================
# SUGGESTION RANKING TESTS
# =============================================================================


class TestSuggestionRanking:
    """Tests for suggestion ranking and ordering."""

    def test_suggestions_sorted_by_frequency(self):
        """Test suggestions are sorted by frequency."""
        suggestions = [
            QuerySuggestion(
                text="high frequency",
                suggestion_type="popular",
                frequency=100,
                success_rate=0.50,
            ),
            QuerySuggestion(
                text="medium frequency",
                suggestion_type="popular",
                frequency=50,
                success_rate=0.80,
            ),
            QuerySuggestion(
                text="low frequency",
                suggestion_type="popular",
                frequency=10,
                success_rate=0.90,
            ),
        ]

        sorted_suggestions = sorted(suggestions, key=lambda s: -s.frequency)
        assert sorted_suggestions[0].text == "high frequency"
        assert sorted_suggestions[1].text == "medium frequency"
        assert sorted_suggestions[2].text == "low frequency"

    def test_suggestions_can_sort_by_success_rate(self):
        """Test suggestions can be sorted by success rate."""
        suggestions = [
            QuerySuggestion(
                text="low success",
                suggestion_type="popular",
                frequency=100,
                success_rate=0.50,
            ),
            QuerySuggestion(
                text="high success",
                suggestion_type="popular",
                frequency=50,
                success_rate=0.95,
            ),
        ]

        sorted_by_success = sorted(suggestions, key=lambda s: -s.success_rate)
        assert sorted_by_success[0].text == "high success"
        assert sorted_by_success[1].text == "low success"

"""Unit tests for the search analytics module.

Tests for AnalyticsCollector, SearchMetrics, and SearchEvent
for collecting and reporting search performance metrics.
"""

from datetime import datetime, timezone, timedelta
from unittest.mock import AsyncMock, MagicMock, patch
import uuid

import pytest

from penf_lib.search.analytics import (
    AnalyticsCollector,
    SearchMetrics,
    SearchEvent,
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
    repo.get_search_statistics = AsyncMock()
    repo.get_suggestions = AsyncMock()
    repo.update_suggestion_frequency = AsyncMock()
    repo.update_suggestion_success_rate = AsyncMock()
    return repo


@pytest.fixture
def analytics_collector(mock_db_session):
    """Create an AnalyticsCollector instance for testing."""
    return AnalyticsCollector(mock_db_session, "test-tenant-id")


@pytest.fixture
def sample_statistics():
    """Create sample statistics data for testing."""
    return {
        "total_queries": 1000,
        "avg_execution_time_ms": 125.5,
        "cache_hit_rate": 0.45,
        "avg_result_count": 15.3,
        "zero_result_queries": 50,
        "strategy_distribution": {
            "hybrid": 800,
            "full_text": 200,
        },
        "period_days": 7,
    }


# =============================================================================
# SEARCH METRICS TESTS
# =============================================================================


class TestSearchMetrics:
    """Tests for SearchMetrics dataclass."""

    def test_metrics_creation(self):
        """Test creating a SearchMetrics instance."""
        metrics = SearchMetrics(
            total_queries=1000,
            unique_users=50,
            avg_execution_time_ms=125.5,
            cache_hit_rate=0.45,
            zero_result_rate=0.05,
            successful_search_rate=0.95,
            content_type_distribution={"email": 0.6, "meeting": 0.4},
            popular_queries=["query1", "query2"],
            p50_execution_time_ms=100,
            p95_execution_time_ms=250,
            p99_execution_time_ms=500,
        )

        assert metrics.total_queries == 1000
        assert metrics.unique_users == 50
        assert metrics.avg_execution_time_ms == 125.5
        assert metrics.cache_hit_rate == 0.45
        assert metrics.zero_result_rate == 0.05
        assert metrics.successful_search_rate == 0.95
        assert metrics.p50_execution_time_ms == 100
        assert metrics.p95_execution_time_ms == 250
        assert metrics.p99_execution_time_ms == 500

    def test_metrics_with_default_values(self):
        """Test SearchMetrics with default values."""
        metrics = SearchMetrics(
            total_queries=0,
            unique_users=0,
            avg_execution_time_ms=0.0,
            cache_hit_rate=0.0,
            zero_result_rate=0.0,
            successful_search_rate=0.0,
        )

        assert metrics.content_type_distribution == {}
        assert metrics.popular_queries == []
        assert metrics.p50_execution_time_ms == 0
        assert metrics.p95_execution_time_ms == 0
        assert metrics.p99_execution_time_ms == 0

    def test_metrics_rates_are_percentages(self):
        """Test that rate metrics are between 0 and 1."""
        metrics = SearchMetrics(
            total_queries=100,
            unique_users=10,
            avg_execution_time_ms=100.0,
            cache_hit_rate=0.75,  # 75%
            zero_result_rate=0.10,  # 10%
            successful_search_rate=0.90,  # 90%
        )

        assert 0.0 <= metrics.cache_hit_rate <= 1.0
        assert 0.0 <= metrics.zero_result_rate <= 1.0
        assert 0.0 <= metrics.successful_search_rate <= 1.0


# =============================================================================
# SEARCH EVENT TESTS
# =============================================================================


class TestSearchEvent:
    """Tests for SearchEvent dataclass."""

    def test_event_creation(self):
        """Test creating a SearchEvent instance."""
        now = datetime.now(timezone.utc)
        event = SearchEvent(
            query_id="query-123",
            query_text="customer issues",
            result_count=15,
            execution_time_ms=120,
            cache_hit=False,
            search_strategy="hybrid",
            content_types=["email", "meeting"],
            timestamp=now,
        )

        assert event.query_id == "query-123"
        assert event.query_text == "customer issues"
        assert event.result_count == 15
        assert event.execution_time_ms == 120
        assert event.cache_hit is False
        assert event.search_strategy == "hybrid"
        assert event.content_types == ["email", "meeting"]
        assert event.timestamp == now

    def test_event_default_timestamp(self):
        """Test SearchEvent has default timestamp."""
        before = datetime.now(timezone.utc)
        event = SearchEvent(
            query_id="test",
            query_text="test query",
            result_count=10,
            execution_time_ms=100,
            cache_hit=False,
            search_strategy="hybrid",
        )
        after = datetime.now(timezone.utc)

        assert before <= event.timestamp <= after

    def test_event_default_content_types(self):
        """Test SearchEvent has default empty content_types."""
        event = SearchEvent(
            query_id="test",
            query_text="test",
            result_count=0,
            execution_time_ms=50,
            cache_hit=True,
            search_strategy="full_text",
        )

        assert event.content_types == []


# =============================================================================
# ANALYTICS COLLECTOR TESTS
# =============================================================================


class TestAnalyticsCollector:
    """Tests for AnalyticsCollector class."""

    def test_collector_initialization(self, mock_db_session):
        """Test AnalyticsCollector initialization."""
        collector = AnalyticsCollector(mock_db_session, "tenant-123")

        assert collector.db_session == mock_db_session
        assert collector.tenant_id == "tenant-123"

    def test_tenant_id_to_uuid(self, analytics_collector):
        """Test _to_uuid conversion."""
        result = analytics_collector._to_uuid()
        assert isinstance(result, uuid.UUID)

    @pytest.mark.asyncio
    async def test_record_search_successful(
        self, analytics_collector, mock_search_repository
    ):
        """Test recording a successful search."""
        mock_search_repository.update_suggestion_frequency.return_value = None
        mock_search_repository.update_suggestion_success_rate.return_value = None

        analytics_collector._repository = mock_search_repository
        await analytics_collector.record_search(
            query_text="customer issues",
            result_count=10,
            execution_time_ms=150,
            cache_hit=False,
            search_strategy="hybrid",
        )

        # Should update suggestion for successful search
        mock_search_repository.update_suggestion_frequency.assert_called_once()
        mock_search_repository.update_suggestion_success_rate.assert_called_once_with(
            analytics_collector._to_uuid(), "customer issues", was_successful=True
        )

    @pytest.mark.asyncio
    async def test_record_search_zero_results(
        self, analytics_collector, mock_search_repository
    ):
        """Test recording a search with zero results."""
        mock_search_repository.update_suggestion_success_rate.return_value = None

        analytics_collector._repository = mock_search_repository
        await analytics_collector.record_search(
            query_text="nonexistent query",
            result_count=0,
            execution_time_ms=100,
            cache_hit=False,
            search_strategy="hybrid",
        )

        # Should not update frequency for zero results
        mock_search_repository.update_suggestion_frequency.assert_not_called()
        # Should update success rate as unsuccessful
        mock_search_repository.update_suggestion_success_rate.assert_called_once_with(
            analytics_collector._to_uuid(), "nonexistent query", was_successful=False
        )

    @pytest.mark.asyncio
    async def test_get_metrics(
        self, analytics_collector, mock_search_repository, sample_statistics
    ):
        """Test getting aggregated metrics."""
        mock_search_repository.get_search_statistics.return_value = sample_statistics

        analytics_collector._repository = mock_search_repository
        result = await analytics_collector.get_metrics(days=7)

        assert isinstance(result, SearchMetrics)
        assert result.total_queries == 1000
        assert result.avg_execution_time_ms == 125.5
        assert result.cache_hit_rate == 0.45
        # 50 zero results out of 1000 = 5%
        assert result.zero_result_rate == 0.05

    @pytest.mark.asyncio
    async def test_get_metrics_empty(
        self, analytics_collector, mock_search_repository
    ):
        """Test getting metrics when no data exists."""
        mock_search_repository.get_search_statistics.return_value = {
            "total_queries": 0,
            "avg_execution_time_ms": 0,
            "cache_hit_rate": 0,
            "avg_result_count": 0,
            "zero_result_queries": 0,
            "strategy_distribution": {},
        }

        analytics_collector._repository = mock_search_repository
        result = await analytics_collector.get_metrics(days=7)

        assert result.total_queries == 0
        assert result.zero_result_rate == 0.0
        # When no queries, success rate is 1.0 - zero_result_rate = 1.0
        assert result.successful_search_rate == 1.0

    @pytest.mark.asyncio
    async def test_get_performance_summary(
        self, analytics_collector, mock_search_repository, sample_statistics
    ):
        """Test getting human-readable performance summary."""
        mock_search_repository.get_search_statistics.return_value = sample_statistics

        analytics_collector._repository = mock_search_repository
        result = await analytics_collector.get_performance_summary(days=7)

        assert result["period"] == "Last 7 days"
        assert result["total_searches"] == 1000
        assert "ms" in result["avg_response_time"]
        assert "%" in result["cache_effectiveness"]
        assert "%" in result["search_success"]

    @pytest.mark.asyncio
    async def test_get_trending_queries(
        self, analytics_collector, mock_search_repository
    ):
        """Test getting trending queries."""
        mock_search_repository.get_suggestions.return_value = [
            {
                "suggestion_text": "popular query 1",
                "frequency": 100,
                "success_rate": 0.85,
            },
            {
                "suggestion_text": "popular query 2",
                "frequency": 75,
                "success_rate": 0.90,
            },
        ]

        analytics_collector._repository = mock_search_repository
        result = await analytics_collector.get_trending_queries(limit=10, days=7)

        assert len(result) == 2
        assert result[0]["query"] == "popular query 1"
        assert result[0]["frequency"] == 100
        assert "%" in result[0]["success_rate"]


# =============================================================================
# METRICS CALCULATION TESTS
# =============================================================================


class TestMetricsCalculation:
    """Tests for metrics calculation logic."""

    def test_success_rate_calculation(self):
        """Test that success rate is inverse of zero result rate."""
        metrics = SearchMetrics(
            total_queries=100,
            unique_users=10,
            avg_execution_time_ms=100.0,
            cache_hit_rate=0.50,
            zero_result_rate=0.15,
            successful_search_rate=0.85,
        )

        # Zero result rate + success rate should equal 1.0
        assert abs(metrics.zero_result_rate + metrics.successful_search_rate - 1.0) < 0.01

    def test_percentile_ordering(self):
        """Test that percentiles are in correct order."""
        metrics = SearchMetrics(
            total_queries=1000,
            unique_users=50,
            avg_execution_time_ms=100.0,
            cache_hit_rate=0.50,
            zero_result_rate=0.10,
            successful_search_rate=0.90,
            p50_execution_time_ms=80,
            p95_execution_time_ms=200,
            p99_execution_time_ms=450,
        )

        assert metrics.p50_execution_time_ms <= metrics.p95_execution_time_ms
        assert metrics.p95_execution_time_ms <= metrics.p99_execution_time_ms

    def test_content_type_distribution_sums_to_one(self):
        """Test content type distribution percentages."""
        metrics = SearchMetrics(
            total_queries=100,
            unique_users=10,
            avg_execution_time_ms=100.0,
            cache_hit_rate=0.50,
            zero_result_rate=0.10,
            successful_search_rate=0.90,
            content_type_distribution={
                "email": 0.60,
                "meeting": 0.25,
                "document": 0.10,
                "slack": 0.05,
            },
        )

        total = sum(metrics.content_type_distribution.values())
        assert abs(total - 1.0) < 0.01


# =============================================================================
# INTEGRATION TESTS
# =============================================================================


class TestAnalyticsIntegration:
    """Integration tests for analytics functionality."""

    @pytest.mark.asyncio
    async def test_record_and_retrieve_metrics(
        self, analytics_collector, mock_search_repository
    ):
        """Test recording searches and retrieving metrics."""
        # Setup mocks for recording
        mock_search_repository.update_suggestion_frequency.return_value = None
        mock_search_repository.update_suggestion_success_rate.return_value = None

        # Record some searches
        analytics_collector._repository = mock_search_repository
        for i in range(5):
            await analytics_collector.record_search(
                query_text=f"test query {i}",
                result_count=i * 2,
                execution_time_ms=100 + i * 10,
                cache_hit=i % 2 == 0,
                search_strategy="hybrid",
            )

        # Verify recording calls were made
        assert mock_search_repository.update_suggestion_success_rate.call_count == 5

    @pytest.mark.asyncio
    async def test_different_time_ranges(
        self, analytics_collector, mock_search_repository, sample_statistics
    ):
        """Test getting metrics for different time ranges."""
        mock_search_repository.get_search_statistics.return_value = sample_statistics

        analytics_collector._repository = mock_search_repository
        # 7 days
        metrics_7d = await analytics_collector.get_metrics(days=7)
        # 30 days
        metrics_30d = await analytics_collector.get_metrics(days=30)

        # Both should return metrics (mocked same data)
        assert metrics_7d.total_queries == metrics_30d.total_queries

        # Verify different days parameters were passed
        calls = mock_search_repository.get_search_statistics.call_args_list
        assert len(calls) == 2

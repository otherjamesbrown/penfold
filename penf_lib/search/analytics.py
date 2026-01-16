"""Search analytics collection and reporting.

Tracks search performance metrics and usage patterns for optimization.
"""

from __future__ import annotations

import uuid
from datetime import datetime, timezone
from typing import Optional, Dict, List, Any, TYPE_CHECKING
from dataclasses import dataclass, field

from sqlalchemy.ext.asyncio import AsyncSession

if TYPE_CHECKING:
    from penf_lib.storage.repositories.search import SearchRepository


@dataclass
class SearchMetrics:
    """Aggregated search metrics.

    Attributes:
        total_queries: Total number of search queries executed.
        unique_users: Number of unique users who performed searches.
        avg_execution_time_ms: Average query execution time in milliseconds.
        cache_hit_rate: Percentage of queries served from cache (0.0-1.0).
        zero_result_rate: Percentage of queries returning no results.
        successful_search_rate: Percentage of queries returning results.
        content_type_distribution: Distribution of searches by content type.
        popular_queries: List of most popular query strings.
        p50_execution_time_ms: 50th percentile (median) execution time.
        p95_execution_time_ms: 95th percentile execution time.
        p99_execution_time_ms: 99th percentile execution time.
    """

    total_queries: int
    unique_users: int
    avg_execution_time_ms: float
    cache_hit_rate: float
    zero_result_rate: float
    successful_search_rate: float
    content_type_distribution: Dict[str, float] = field(default_factory=dict)
    popular_queries: List[str] = field(default_factory=list)
    p50_execution_time_ms: int = 0
    p95_execution_time_ms: int = 0
    p99_execution_time_ms: int = 0


@dataclass
class SearchEvent:
    """Individual search event for analytics.

    Attributes:
        query_id: Unique identifier for this query execution.
        query_text: The search query text.
        result_count: Number of results returned.
        execution_time_ms: Query execution time in milliseconds.
        cache_hit: Whether the query was served from cache.
        search_strategy: Search strategy used ('hybrid', 'full_text', 'semantic').
        content_types: List of content types searched.
        timestamp: When the search was executed.
    """

    query_id: str
    query_text: str
    result_count: int
    execution_time_ms: int
    cache_hit: bool
    search_strategy: str
    content_types: List[str] = field(default_factory=list)
    timestamp: datetime = field(default_factory=lambda: datetime.now(timezone.utc))


class AnalyticsCollector:
    """Collect and aggregate search analytics.

    Provides methods for recording search events and retrieving
    aggregated metrics for monitoring and optimization.

    Attributes:
        db_session: SQLAlchemy async session for database operations.
        tenant_id: UUID string for multi-tenant isolation.
    """

    def __init__(self, db_session: AsyncSession, tenant_id: str):
        """Initialize analytics collector.

        Args:
            db_session: SQLAlchemy async session for database operations.
            tenant_id: Tenant UUID string for multi-tenant isolation.
        """
        self.db_session = db_session
        self.tenant_id = tenant_id
        self._repository: Optional[SearchRepository] = None

    @property
    def repository(self) -> SearchRepository:
        """Lazy-load search repository."""
        if self._repository is None:
            from penf_lib.storage.repositories.search import SearchRepository

            self._repository = SearchRepository(self.db_session)
        return self._repository

    def _to_uuid(self) -> uuid.UUID:
        """Convert tenant_id string to UUID."""
        try:
            return uuid.UUID(self.tenant_id)
        except ValueError:
            return uuid.uuid5(uuid.NAMESPACE_DNS, self.tenant_id)

    async def record_search(
        self,
        query_text: str,
        result_count: int,
        execution_time_ms: int,
        cache_hit: bool,
        search_strategy: str,
        session_id: Optional[str] = None,
        filters: Optional[Dict[str, Any]] = None,
    ) -> None:
        """Record a search execution for analytics.

        This is the main entry point for recording search events.
        It updates both query records and suggestion frequencies.

        Args:
            query_text: The search query text.
            result_count: Number of results returned.
            execution_time_ms: Query execution time in milliseconds.
            cache_hit: Whether the query was served from cache.
            search_strategy: Search strategy used ('hybrid', 'full_text', 'semantic').
            session_id: Optional session ID for session tracking.
            filters: Optional dictionary of applied filters.
        """
        tenant_uuid = self._to_uuid()

        # Update suggestion frequency for successful queries
        if result_count > 0:
            await self.repository.update_suggestion_frequency(tenant_uuid, query_text)
            await self.repository.update_suggestion_success_rate(
                tenant_uuid, query_text, was_successful=True
            )
        else:
            # Track zero-result queries for analysis
            await self.repository.update_suggestion_success_rate(
                tenant_uuid, query_text, was_successful=False
            )

    async def get_metrics(
        self,
        start_date: Optional[datetime] = None,
        end_date: Optional[datetime] = None,
        days: int = 7,
    ) -> SearchMetrics:
        """Get aggregated search metrics for date range.

        Args:
            start_date: Optional start of date range (defaults to days ago).
            end_date: Optional end of date range (defaults to now).
            days: Number of days to analyze if dates not specified.

        Returns:
            SearchMetrics with aggregated statistics.
        """
        tenant_uuid = self._to_uuid()

        stats = await self.repository.get_search_statistics(tenant_uuid, days=days)

        # Calculate rates
        total = stats.get("total_queries", 0)
        zero_results = stats.get("zero_result_queries", 0)

        zero_result_rate = zero_results / total if total > 0 else 0.0
        successful_rate = 1.0 - zero_result_rate

        # Convert strategy distribution to content type distribution
        # (For now, we track by strategy; content type distribution
        # would require additional tracking)
        strategy_dist = stats.get("strategy_distribution", {})
        content_type_dist: Dict[str, float] = {}
        if total > 0:
            for strategy, count in strategy_dist.items():
                content_type_dist[strategy] = count / total

        return SearchMetrics(
            total_queries=total,
            unique_users=stats.get("unique_users", 0),
            avg_execution_time_ms=stats.get("avg_execution_time_ms", 0.0),
            cache_hit_rate=stats.get("cache_hit_rate", 0.0),
            zero_result_rate=zero_result_rate,
            successful_search_rate=successful_rate,
            content_type_distribution=content_type_dist,
            popular_queries=stats.get("popular_queries", []),
            p50_execution_time_ms=stats.get("p50_execution_time_ms", 0),
            p95_execution_time_ms=stats.get("p95_execution_time_ms", 0),
            p99_execution_time_ms=stats.get("p99_execution_time_ms", 0),
        )

    async def get_performance_summary(self, days: int = 7) -> Dict[str, Any]:
        """Get a human-readable performance summary.

        Args:
            days: Number of days to analyze.

        Returns:
            Dictionary with performance summary including:
            - period: Time period analyzed
            - total_searches: Total search count
            - avg_response_time: Average response time string
            - cache_effectiveness: Cache hit rate percentage
            - search_success: Success rate percentage
            - strategy_usage: Strategy distribution
        """
        metrics = await self.get_metrics(days=days)

        return {
            "period": f"Last {days} days",
            "total_searches": metrics.total_queries,
            "avg_response_time": f"{metrics.avg_execution_time_ms:.0f}ms",
            "cache_effectiveness": f"{metrics.cache_hit_rate * 100:.1f}%",
            "search_success": f"{metrics.successful_search_rate * 100:.1f}%",
            "zero_result_rate": f"{metrics.zero_result_rate * 100:.1f}%",
            "strategy_usage": metrics.content_type_distribution,
            "percentiles": {
                "p50": f"{metrics.p50_execution_time_ms}ms",
                "p95": f"{metrics.p95_execution_time_ms}ms",
                "p99": f"{metrics.p99_execution_time_ms}ms",
            },
        }

    async def get_trending_queries(
        self,
        limit: int = 10,
        days: int = 7,
    ) -> List[Dict[str, Any]]:
        """Get trending search queries.

        Identifies queries that have increased in frequency recently.

        Args:
            limit: Maximum number of trending queries to return.
            days: Time window for trend analysis.

        Returns:
            List of trending query dictionaries with:
            - query: The query text
            - frequency: Number of times executed
            - success_rate: Success rate percentage
        """
        tenant_uuid = self._to_uuid()

        # Get popular suggestions which are ordered by frequency
        suggestions = await self.repository.get_suggestions(
            tenant_uuid, prefix=None, limit=limit
        )

        return [
            {
                "query": s["suggestion_text"],
                "frequency": s["frequency"],
                "success_rate": f"{float(s['success_rate'] or 0) * 100:.0f}%",
            }
            for s in suggestions
        ]

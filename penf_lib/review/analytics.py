"""Review analytics for the Daily Review workflow.

This module provides analytics management for tracking review patterns and
system learning progress, including:
- Aggregated metrics for periods (daily, weekly, monthly)
- Trend calculation over multiple periods
- Content type and correction type breakdowns
- Actionable insight generation

Based on ReviewAnalytics entity from specs/006-daily-review/data-model.md.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from datetime import date, datetime, timedelta, timezone
from decimal import Decimal
from typing import Any, Protocol
from uuid import UUID

from penf_lib.review.exceptions import (
    DatabaseOperationError,
    InvalidFilterCriteriaError,
)
from penf_lib.review.models import (
    AnalyticsPeriodType,
    ContentType,
    ReviewAnalyticsDTO,
)

# =============================================================================
# SUPPORTING DATA CLASSES
# =============================================================================


@dataclass
class TrendData:
    """Trend data for a specific metric over time.

    Contains the metric values across periods and the calculated
    trend direction and magnitude.
    """

    metric_name: str
    values: list[Decimal]
    period_labels: list[str]
    trend_direction: str  # "up", "down", "stable"
    trend_percentage: Decimal
    is_positive_trend: bool  # Whether the trend direction is good


@dataclass
class ReviewInsight:
    """An actionable insight derived from analytics.

    Represents a recommendation or observation that the user
    should be aware of based on their review patterns.
    """

    insight_type: str  # "recommendation", "warning", "achievement", "info"
    title: str
    description: str
    priority: int  # 1 = highest, 5 = lowest
    metric_context: dict[str, Any] = field(default_factory=dict)
    action_suggestion: str | None = None


@dataclass
class ContentTypeStats:
    """Statistics for a specific content type.

    Aggregates metrics for a single content type (email, meeting, document).
    """

    content_type: ContentType
    total_items: int
    accepted: int
    rejected: int
    modified: int
    skipped: int
    avg_confidence: Decimal
    acceptance_rate: Decimal


@dataclass
class CorrectionTypeStats:
    """Statistics for a specific correction type.

    Tracks how often users make specific types of corrections.
    """

    correction_type: str
    count: int
    percentage: Decimal
    common_patterns: list[str]


@dataclass
class AnalyticsSummary:
    """Summary analytics for display.

    Provides a high-level overview of review performance.
    """

    period_type: AnalyticsPeriodType
    period_start: date
    period_end: date
    total_sessions: int
    total_items: int
    acceptance_rate: Decimal
    ai_accuracy: Decimal | None
    avg_session_duration_min: Decimal
    top_content_type: ContentType | None
    trend_summary: str  # e.g., "Improving", "Stable", "Declining"


# =============================================================================
# REPOSITORY PROTOCOL
# =============================================================================


class AnalyticsRepositoryProtocol(Protocol):
    """Protocol for analytics repository dependency injection.

    Defines the interface that any repository implementation must satisfy
    to work with the AnalyticsManager.
    """

    async def get_analytics_for_period(
        self,
        tenant_id: UUID,
        period_type: AnalyticsPeriodType,
        start_date: date,
        end_date: date,
    ) -> ReviewAnalyticsDTO | None:
        """Get analytics for a specific period.

        Args:
            tenant_id: Tenant isolation identifier
            period_type: Period granularity (daily, weekly, monthly)
            start_date: Period start date
            end_date: Period end date

        Returns:
            ReviewAnalyticsDTO or None if no data for period
        """
        ...

    async def get_analytics_range(
        self,
        tenant_id: UUID,
        period_type: AnalyticsPeriodType,
        start_date: date,
        end_date: date,
    ) -> list[ReviewAnalyticsDTO]:
        """Get analytics for a date range with multiple periods.

        Args:
            tenant_id: Tenant isolation identifier
            period_type: Period granularity
            start_date: Range start date
            end_date: Range end date

        Returns:
            List of ReviewAnalyticsDTO records within the range
        """
        ...

    async def save_analytics(
        self,
        analytics_data: dict[str, Any],
    ) -> ReviewAnalyticsDTO:
        """Save or update analytics record.

        Args:
            analytics_data: Analytics data dictionary

        Returns:
            Created or updated ReviewAnalyticsDTO
        """
        ...

    async def get_session_stats(
        self,
        tenant_id: UUID,
        start_date: datetime,
        end_date: datetime,
    ) -> dict[str, Any]:
        """Get aggregated session statistics for a date range.

        Args:
            tenant_id: Tenant isolation identifier
            start_date: Range start datetime
            end_date: Range end datetime

        Returns:
            Dictionary with session statistics:
            - total_sessions: int
            - total_items_reviewed: int
            - total_accepted: int
            - total_rejected: int
            - total_modified: int
            - total_skipped: int
            - avg_session_duration_seconds: float
            - avg_items_per_session: float
        """
        ...

    async def get_feedback_stats(
        self,
        tenant_id: UUID,
        start_date: datetime,
        end_date: datetime,
    ) -> dict[str, Any]:
        """Get aggregated feedback statistics for a date range.

        Args:
            tenant_id: Tenant isolation identifier
            start_date: Range start datetime
            end_date: Range end datetime

        Returns:
            Dictionary with feedback statistics:
            - avg_ai_confidence: Decimal
            - avg_learning_signal_quality: Decimal
            - content_type_counts: dict[str, int]
            - correction_reason_counts: dict[str, int]
        """
        ...

    async def get_rule_stats(
        self,
        tenant_id: UUID,
        start_date: datetime,
        end_date: datetime,
    ) -> dict[str, Any]:
        """Get learning rule statistics for a date range.

        Args:
            tenant_id: Tenant isolation identifier
            start_date: Range start datetime
            end_date: Range end datetime

        Returns:
            Dictionary with rule statistics:
            - rules_created: int
            - rules_applied: int
            - avg_effectiveness: Decimal
        """
        ...

    async def get_batch_stats(
        self,
        tenant_id: UUID,
        start_date: datetime,
        end_date: datetime,
    ) -> dict[str, Any]:
        """Get batch operation statistics for a date range.

        Args:
            tenant_id: Tenant isolation identifier
            start_date: Range start datetime
            end_date: Range end datetime

        Returns:
            Dictionary with batch statistics:
            - total_batch_operations: int
            - total_items_in_batches: int
            - batch_usage_rate: Decimal
        """
        ...


# =============================================================================
# ANALYTICS MANAGER
# =============================================================================


class AnalyticsManager:
    """Manages review analytics and insights.

    The AnalyticsManager is responsible for:
    - Retrieving analytics for specific periods
    - Calculating trends over multiple periods
    - Generating actionable insights from patterns
    - Providing content type and correction type breakdowns
    - Computing AI accuracy metrics

    Analytics are pre-aggregated at daily, weekly, and monthly granularities
    for efficient querying of historical data.
    """

    # Thresholds for insight generation
    LOW_ACCEPTANCE_THRESHOLD = Decimal("0.60")
    HIGH_ACCEPTANCE_THRESHOLD = Decimal("0.90")
    LOW_AI_ACCURACY_THRESHOLD = Decimal("0.70")
    HIGH_AI_ACCURACY_THRESHOLD = Decimal("0.85")
    SLOW_REVIEW_THRESHOLD = Decimal("0.5")  # items per minute
    FAST_REVIEW_THRESHOLD = Decimal("3.0")  # items per minute

    # Trend thresholds
    SIGNIFICANT_TREND_THRESHOLD = Decimal("0.05")  # 5% change
    STABLE_TREND_THRESHOLD = Decimal("0.02")  # 2% change

    def __init__(self, repository: AnalyticsRepositoryProtocol) -> None:
        """Initialize the AnalyticsManager.

        Args:
            repository: Repository implementing AnalyticsRepositoryProtocol
        """
        self._repository = repository

    async def get_analytics(
        self,
        tenant_id: UUID,
        period_type: AnalyticsPeriodType,
        start_date: date,
        end_date: date | None = None,
    ) -> ReviewAnalyticsDTO | None:
        """Get analytics for a period.

        Retrieves pre-aggregated analytics for the specified period.
        If no end_date is provided, calculates based on period_type.

        Args:
            tenant_id: Tenant isolation identifier
            period_type: Period granularity (daily, weekly, monthly)
            start_date: Period start date
            end_date: Period end date (optional, calculated if not provided)

        Returns:
            ReviewAnalyticsDTO or None if no data available

        Raises:
            InvalidFilterCriteriaError: If date range is invalid
        """
        # Calculate end_date if not provided
        if end_date is None:
            end_date = self._calculate_period_end(period_type, start_date)

        # Validate date range
        if end_date < start_date:
            raise InvalidFilterCriteriaError(
                criteria={"start_date": str(start_date), "end_date": str(end_date)},
                reason="end_date must be >= start_date",
            )

        return await self._repository.get_analytics_for_period(
            tenant_id=tenant_id,
            period_type=period_type,
            start_date=start_date,
            end_date=end_date,
        )

    async def calculate_trends(
        self,
        tenant_id: UUID,
        periods: int = 7,
        period_type: AnalyticsPeriodType = AnalyticsPeriodType.DAILY,
    ) -> list[TrendData]:
        """Calculate trends over multiple periods.

        Analyzes key metrics over the specified number of periods to
        identify improving, declining, or stable trends.

        Args:
            tenant_id: Tenant isolation identifier
            periods: Number of periods to analyze (default: 7)
            period_type: Period granularity (default: daily)

        Returns:
            List of TrendData for key metrics:
            - acceptance_rate
            - ai_accuracy
            - avg_items_per_minute
            - modification_rate

        Raises:
            InvalidFilterCriteriaError: If periods < 2
        """
        if periods < 2:
            raise InvalidFilterCriteriaError(
                criteria={"periods": periods},
                reason="At least 2 periods required for trend calculation",
            )

        # Calculate date range
        end_date = date.today()
        start_date = self._subtract_periods(end_date, period_type, periods - 1)

        # Get analytics for the range
        analytics_list = await self._repository.get_analytics_range(
            tenant_id=tenant_id,
            period_type=period_type,
            start_date=start_date,
            end_date=end_date,
        )

        if len(analytics_list) < 2:
            return []

        # Sort by period_start
        analytics_list = sorted(analytics_list, key=lambda a: a.period_start)

        # Calculate trends for key metrics
        trends = []

        # Acceptance rate trend
        trends.append(
            self._calculate_metric_trend(
                metric_name="acceptance_rate",
                values=[a.acceptance_rate for a in analytics_list],
                period_labels=[str(a.period_start) for a in analytics_list],
                higher_is_better=True,
            )
        )

        # AI accuracy trend (filter out None values)
        ai_accuracy_values = [
            a.ai_accuracy for a in analytics_list if a.ai_accuracy is not None
        ]
        if len(ai_accuracy_values) >= 2:
            ai_accuracy_labels = [
                str(a.period_start)
                for a in analytics_list
                if a.ai_accuracy is not None
            ]
            trends.append(
                self._calculate_metric_trend(
                    metric_name="ai_accuracy",
                    values=ai_accuracy_values,
                    period_labels=ai_accuracy_labels,
                    higher_is_better=True,
                )
            )

        # Review speed trend
        trends.append(
            self._calculate_metric_trend(
                metric_name="avg_items_per_minute",
                values=[a.avg_items_per_minute for a in analytics_list],
                period_labels=[str(a.period_start) for a in analytics_list],
                higher_is_better=True,
            )
        )

        # Modification rate trend (lower is better - AI getting more accurate)
        trends.append(
            self._calculate_metric_trend(
                metric_name="modification_rate",
                values=[a.modification_rate for a in analytics_list],
                period_labels=[str(a.period_start) for a in analytics_list],
                higher_is_better=False,
            )
        )

        return trends

    async def generate_insights(
        self,
        tenant_id: UUID,
    ) -> list[ReviewInsight]:
        """Generate actionable insights.

        Analyzes recent analytics to generate recommendations,
        warnings, and achievements for the user.

        Args:
            tenant_id: Tenant isolation identifier

        Returns:
            List of ReviewInsight ordered by priority
        """
        insights: list[ReviewInsight] = []

        # Get recent analytics (last 7 days and last 30 days for comparison)
        today = date.today()
        week_ago = today - timedelta(days=7)
        month_ago = today - timedelta(days=30)

        weekly_analytics = await self._repository.get_analytics_range(
            tenant_id=tenant_id,
            period_type=AnalyticsPeriodType.DAILY,
            start_date=week_ago,
            end_date=today,
        )

        monthly_analytics = await self._repository.get_analytics_range(
            tenant_id=tenant_id,
            period_type=AnalyticsPeriodType.DAILY,
            start_date=month_ago,
            end_date=today,
        )

        if not weekly_analytics:
            return [
                ReviewInsight(
                    insight_type="info",
                    title="Start Your Review Journey",
                    description="Complete some review sessions to see personalized insights.",
                    priority=5,
                )
            ]

        # Calculate aggregate metrics
        weekly_avg = self._aggregate_analytics(weekly_analytics)
        monthly_avg = self._aggregate_analytics(monthly_analytics) if monthly_analytics else None

        # Generate insights based on metrics
        insights.extend(self._generate_acceptance_insights(weekly_avg, monthly_avg))
        insights.extend(self._generate_ai_accuracy_insights(weekly_avg, monthly_avg))
        insights.extend(self._generate_efficiency_insights(weekly_avg, monthly_avg))
        insights.extend(self._generate_learning_insights(weekly_avg))

        # Sort by priority
        insights.sort(key=lambda i: i.priority)

        return insights

    async def get_content_type_breakdown(
        self,
        tenant_id: UUID,
        period_type: AnalyticsPeriodType = AnalyticsPeriodType.WEEKLY,
    ) -> list[ContentTypeStats]:
        """Get content type statistics.

        Provides a breakdown of review activity by content type
        (email, meeting, document).

        Args:
            tenant_id: Tenant isolation identifier
            period_type: Period to analyze (default: weekly)

        Returns:
            List of ContentTypeStats for each content type with data
        """
        # Calculate period bounds
        end_date = date.today()
        start_date = self._subtract_periods(end_date, period_type, 1)
        start_datetime = datetime.combine(start_date, datetime.min.time(), tzinfo=timezone.utc)
        end_datetime = datetime.combine(end_date, datetime.max.time(), tzinfo=timezone.utc)

        # Get feedback stats which include content type counts
        feedback_stats = await self._repository.get_feedback_stats(
            tenant_id=tenant_id,
            start_date=start_datetime,
            end_date=end_datetime,
        )

        content_type_counts = feedback_stats.get("content_type_counts", {})
        avg_confidence = feedback_stats.get("avg_ai_confidence", Decimal("0"))

        stats_list = []
        for content_type in ContentType:
            type_key = content_type.value
            count_data = content_type_counts.get(type_key, {})

            total = count_data.get("total", 0)
            if total == 0:
                continue

            accepted = count_data.get("accepted", 0)
            rejected = count_data.get("rejected", 0)
            modified = count_data.get("modified", 0)
            skipped = count_data.get("skipped", 0)

            acceptance_rate = Decimal(accepted) / Decimal(total) if total > 0 else Decimal("0")

            stats_list.append(
                ContentTypeStats(
                    content_type=content_type,
                    total_items=total,
                    accepted=accepted,
                    rejected=rejected,
                    modified=modified,
                    skipped=skipped,
                    avg_confidence=avg_confidence,
                    acceptance_rate=acceptance_rate,
                )
            )

        # Sort by total items descending
        stats_list.sort(key=lambda s: s.total_items, reverse=True)

        return stats_list

    async def get_correction_type_breakdown(
        self,
        tenant_id: UUID,
        period_type: AnalyticsPeriodType = AnalyticsPeriodType.WEEKLY,
    ) -> list[CorrectionTypeStats]:
        """Get correction type statistics.

        Provides a breakdown of corrections by type to identify
        common patterns in user corrections.

        Args:
            tenant_id: Tenant isolation identifier
            period_type: Period to analyze (default: weekly)

        Returns:
            List of CorrectionTypeStats for each correction type
        """
        # Calculate period bounds
        end_date = date.today()
        start_date = self._subtract_periods(end_date, period_type, 1)
        start_datetime = datetime.combine(start_date, datetime.min.time(), tzinfo=timezone.utc)
        end_datetime = datetime.combine(end_date, datetime.max.time(), tzinfo=timezone.utc)

        # Get feedback stats which include correction reason counts
        feedback_stats = await self._repository.get_feedback_stats(
            tenant_id=tenant_id,
            start_date=start_datetime,
            end_date=end_datetime,
        )

        correction_counts = feedback_stats.get("correction_reason_counts", {})
        total_corrections = sum(correction_counts.values())

        stats_list = []
        for reason, count in correction_counts.items():
            percentage = (
                Decimal(count) / Decimal(total_corrections)
                if total_corrections > 0
                else Decimal("0")
            )

            # Common patterns would be derived from more detailed analysis
            # For now, we return the reason as the pattern
            stats_list.append(
                CorrectionTypeStats(
                    correction_type=reason,
                    count=count,
                    percentage=percentage,
                    common_patterns=[reason],
                )
            )

        # Sort by count descending
        stats_list.sort(key=lambda s: s.count, reverse=True)

        return stats_list

    async def get_summary(
        self,
        tenant_id: UUID,
        period_type: AnalyticsPeriodType = AnalyticsPeriodType.WEEKLY,
    ) -> AnalyticsSummary | None:
        """Get analytics summary for display.

        Provides a high-level overview suitable for dashboard display.

        Args:
            tenant_id: Tenant isolation identifier
            period_type: Period to summarize (default: weekly)

        Returns:
            AnalyticsSummary or None if no data available
        """
        end_date = date.today()
        start_date = self._subtract_periods(end_date, period_type, 1)

        analytics = await self.get_analytics(
            tenant_id=tenant_id,
            period_type=period_type,
            start_date=start_date,
            end_date=end_date,
        )

        if analytics is None:
            return None

        # Determine top content type
        content_breakdown = analytics.content_type_breakdown
        top_content_type = None
        max_count = 0
        for type_name, count in content_breakdown.items():
            if count > max_count:
                max_count = count
                try:
                    top_content_type = ContentType(type_name)
                except ValueError:
                    pass

        # Calculate trend summary
        trends = await self.calculate_trends(
            tenant_id=tenant_id,
            periods=3,
            period_type=period_type,
        )

        trend_summary = self._summarize_trends(trends)

        return AnalyticsSummary(
            period_type=period_type,
            period_start=analytics.period_start,
            period_end=analytics.period_end,
            total_sessions=analytics.total_sessions,
            total_items=analytics.total_items_reviewed,
            acceptance_rate=analytics.acceptance_rate,
            ai_accuracy=analytics.ai_accuracy,
            avg_session_duration_min=analytics.avg_session_duration_min,
            top_content_type=top_content_type,
            trend_summary=trend_summary,
        )

    async def aggregate_analytics(
        self,
        tenant_id: UUID,
        period_type: AnalyticsPeriodType,
        target_date: date,
    ) -> ReviewAnalyticsDTO:
        """Aggregate raw data into analytics record.

        Computes and stores aggregated analytics for a specific period.
        This is typically called by a background job.

        Args:
            tenant_id: Tenant isolation identifier
            period_type: Period granularity
            target_date: Date within the target period

        Returns:
            Created or updated ReviewAnalyticsDTO

        Raises:
            DatabaseOperationError: If aggregation fails
        """
        # Calculate period bounds
        start_date = self._calculate_period_start(period_type, target_date)
        end_date = self._calculate_period_end(period_type, start_date)
        start_datetime = datetime.combine(start_date, datetime.min.time(), tzinfo=timezone.utc)
        end_datetime = datetime.combine(end_date, datetime.max.time(), tzinfo=timezone.utc)

        try:
            # Gather statistics from various sources
            session_stats = await self._repository.get_session_stats(
                tenant_id=tenant_id,
                start_date=start_datetime,
                end_date=end_datetime,
            )

            feedback_stats = await self._repository.get_feedback_stats(
                tenant_id=tenant_id,
                start_date=start_datetime,
                end_date=end_datetime,
            )

            rule_stats = await self._repository.get_rule_stats(
                tenant_id=tenant_id,
                start_date=start_datetime,
                end_date=end_datetime,
            )

            batch_stats = await self._repository.get_batch_stats(
                tenant_id=tenant_id,
                start_date=start_datetime,
                end_date=end_datetime,
            )

            # Calculate rates
            total_items = session_stats.get("total_items_reviewed", 0)
            total_accepted = session_stats.get("total_accepted", 0)
            total_rejected = session_stats.get("total_rejected", 0)
            total_modified = session_stats.get("total_modified", 0)
            total_skipped = session_stats.get("total_skipped", 0)

            acceptance_rate = Decimal(total_accepted) / Decimal(total_items) if total_items > 0 else Decimal("0")
            rejection_rate = Decimal(total_rejected) / Decimal(total_items) if total_items > 0 else Decimal("0")
            modification_rate = Decimal(total_modified) / Decimal(total_items) if total_items > 0 else Decimal("0")
            skip_rate = Decimal(total_skipped) / Decimal(total_items) if total_items > 0 else Decimal("0")

            # Calculate AI accuracy (accepted / (accepted + modified + rejected))
            ai_relevant = total_accepted + total_modified + total_rejected
            ai_accuracy = Decimal(total_accepted) / Decimal(ai_relevant) if ai_relevant > 0 else None

            # Calculate review velocity
            avg_session_seconds = session_stats.get("avg_session_duration_seconds", 0)
            avg_session_min = Decimal(avg_session_seconds) / Decimal("60")
            avg_items_per_session = session_stats.get("avg_items_per_session", 0)
            avg_items_per_minute = (
                Decimal(avg_items_per_session) / avg_session_min
                if avg_session_min > 0
                else Decimal("0")
            )

            # Build analytics data
            analytics_data = {
                "tenant_id": tenant_id,
                "period_start": start_date,
                "period_end": end_date,
                "period_type": period_type,
                "total_sessions": session_stats.get("total_sessions", 0),
                "total_items_reviewed": total_items,
                "avg_session_duration_min": avg_session_min,
                "avg_items_per_minute": avg_items_per_minute,
                "acceptance_rate": acceptance_rate,
                "modification_rate": modification_rate,
                "rejection_rate": rejection_rate,
                "skip_rate": skip_rate,
                "avg_ai_confidence": feedback_stats.get("avg_ai_confidence", Decimal("0")),
                "ai_accuracy": ai_accuracy,
                "batch_usage_rate": batch_stats.get("batch_usage_rate", Decimal("0")),
                "learning_rules_created": rule_stats.get("rules_created", 0),
                "learning_rules_applied": rule_stats.get("rules_applied", 0),
                "content_type_breakdown": feedback_stats.get("content_type_counts", {}),
                "correction_type_breakdown": feedback_stats.get("correction_reason_counts", {}),
                "created_at": datetime.now(timezone.utc),
            }

            return await self._repository.save_analytics(analytics_data)

        except Exception as e:
            raise DatabaseOperationError(
                operation="aggregate_analytics",
                reason=str(e),
            ) from e

    # =========================================================================
    # PRIVATE HELPER METHODS
    # =========================================================================

    def _calculate_period_start(
        self,
        period_type: AnalyticsPeriodType,
        target_date: date,
    ) -> date:
        """Calculate the start date for a period containing target_date.

        Args:
            period_type: Period granularity
            target_date: Date within the period

        Returns:
            Start date of the period
        """
        if period_type == AnalyticsPeriodType.DAILY:
            return target_date

        if period_type == AnalyticsPeriodType.WEEKLY:
            # Start of week (Monday)
            return target_date - timedelta(days=target_date.weekday())

        if period_type == AnalyticsPeriodType.MONTHLY:
            # Start of month
            return target_date.replace(day=1)

        return target_date

    def _calculate_period_end(
        self,
        period_type: AnalyticsPeriodType,
        start_date: date,
    ) -> date:
        """Calculate the end date for a period starting at start_date.

        Args:
            period_type: Period granularity
            start_date: Start date of the period

        Returns:
            End date of the period
        """
        if period_type == AnalyticsPeriodType.DAILY:
            return start_date

        if period_type == AnalyticsPeriodType.WEEKLY:
            # End of week (Sunday)
            return start_date + timedelta(days=6)

        if period_type == AnalyticsPeriodType.MONTHLY:
            # End of month
            if start_date.month == 12:
                next_month = start_date.replace(year=start_date.year + 1, month=1, day=1)
            else:
                next_month = start_date.replace(month=start_date.month + 1, day=1)
            return next_month - timedelta(days=1)

        return start_date

    def _subtract_periods(
        self,
        from_date: date,
        period_type: AnalyticsPeriodType,
        periods: int,
    ) -> date:
        """Subtract a number of periods from a date.

        Args:
            from_date: Starting date
            period_type: Period granularity
            periods: Number of periods to subtract

        Returns:
            Resulting date
        """
        if period_type == AnalyticsPeriodType.DAILY:
            return from_date - timedelta(days=periods)

        if period_type == AnalyticsPeriodType.WEEKLY:
            return from_date - timedelta(weeks=periods)

        if period_type == AnalyticsPeriodType.MONTHLY:
            # Approximate month subtraction
            year = from_date.year
            month = from_date.month - periods
            while month <= 0:
                month += 12
                year -= 1
            # Handle day overflow
            day = min(from_date.day, 28)  # Safe for all months
            return date(year, month, day)

        return from_date

    def _calculate_metric_trend(
        self,
        metric_name: str,
        values: list[Decimal],
        period_labels: list[str],
        higher_is_better: bool,
    ) -> TrendData:
        """Calculate trend for a single metric.

        Args:
            metric_name: Name of the metric
            values: Metric values over time
            period_labels: Labels for each period
            higher_is_better: Whether higher values are positive

        Returns:
            TrendData with trend analysis
        """
        if len(values) < 2:
            return TrendData(
                metric_name=metric_name,
                values=values,
                period_labels=period_labels,
                trend_direction="stable",
                trend_percentage=Decimal("0"),
                is_positive_trend=True,
            )

        # Calculate percentage change from first to last
        first_value = values[0]
        last_value = values[-1]

        if first_value == 0:
            trend_percentage = Decimal("100") if last_value > 0 else Decimal("0")
        else:
            trend_percentage = ((last_value - first_value) / first_value) * Decimal("100")

        # Determine trend direction
        abs_change = abs(trend_percentage)
        if abs_change < self.STABLE_TREND_THRESHOLD * Decimal("100"):
            trend_direction = "stable"
        elif trend_percentage > 0:
            trend_direction = "up"
        else:
            trend_direction = "down"

        # Determine if trend is positive
        if trend_direction == "stable":
            is_positive_trend = True
        elif higher_is_better:
            is_positive_trend = trend_direction == "up"
        else:
            is_positive_trend = trend_direction == "down"

        return TrendData(
            metric_name=metric_name,
            values=values,
            period_labels=period_labels,
            trend_direction=trend_direction,
            trend_percentage=trend_percentage,
            is_positive_trend=is_positive_trend,
        )

    def _aggregate_analytics(
        self,
        analytics_list: list[ReviewAnalyticsDTO],
    ) -> dict[str, Any]:
        """Aggregate multiple analytics records into averages.

        Args:
            analytics_list: List of analytics records

        Returns:
            Dictionary with aggregated values
        """
        if not analytics_list:
            return {}

        count = len(analytics_list)

        total_sessions = sum(a.total_sessions for a in analytics_list)
        total_items = sum(a.total_items_reviewed for a in analytics_list)

        avg_acceptance_rate = sum(a.acceptance_rate for a in analytics_list) / count
        avg_modification_rate = sum(a.modification_rate for a in analytics_list) / count
        avg_rejection_rate = sum(a.rejection_rate for a in analytics_list) / count
        avg_skip_rate = sum(a.skip_rate for a in analytics_list) / count

        avg_session_duration = sum(a.avg_session_duration_min for a in analytics_list) / count
        avg_items_per_minute = sum(a.avg_items_per_minute for a in analytics_list) / count
        avg_ai_confidence = sum(a.avg_ai_confidence for a in analytics_list) / count

        # AI accuracy (filter None values)
        ai_accuracy_values = [a.ai_accuracy for a in analytics_list if a.ai_accuracy is not None]
        avg_ai_accuracy = (
            sum(ai_accuracy_values) / len(ai_accuracy_values)
            if ai_accuracy_values
            else None
        )

        avg_batch_usage_rate = sum(a.batch_usage_rate for a in analytics_list) / count
        total_rules_created = sum(a.learning_rules_created for a in analytics_list)
        total_rules_applied = sum(a.learning_rules_applied for a in analytics_list)

        return {
            "total_sessions": total_sessions,
            "total_items": total_items,
            "acceptance_rate": avg_acceptance_rate,
            "modification_rate": avg_modification_rate,
            "rejection_rate": avg_rejection_rate,
            "skip_rate": avg_skip_rate,
            "avg_session_duration_min": avg_session_duration,
            "avg_items_per_minute": avg_items_per_minute,
            "avg_ai_confidence": avg_ai_confidence,
            "ai_accuracy": avg_ai_accuracy,
            "batch_usage_rate": avg_batch_usage_rate,
            "rules_created": total_rules_created,
            "rules_applied": total_rules_applied,
        }

    def _generate_acceptance_insights(
        self,
        weekly: dict[str, Any],
        monthly: dict[str, Any] | None,
    ) -> list[ReviewInsight]:
        """Generate insights based on acceptance rate.

        Args:
            weekly: Weekly aggregated metrics
            monthly: Monthly aggregated metrics (for comparison)

        Returns:
            List of acceptance-related insights
        """
        insights = []
        acceptance_rate = weekly.get("acceptance_rate", Decimal("0"))

        if acceptance_rate >= self.HIGH_ACCEPTANCE_THRESHOLD:
            insights.append(
                ReviewInsight(
                    insight_type="achievement",
                    title="High Acceptance Rate",
                    description=f"Your acceptance rate of {acceptance_rate:.1%} shows the AI is learning your preferences well.",
                    priority=4,
                    metric_context={"acceptance_rate": float(acceptance_rate)},
                )
            )
        elif acceptance_rate < self.LOW_ACCEPTANCE_THRESHOLD:
            insights.append(
                ReviewInsight(
                    insight_type="recommendation",
                    title="Consider Creating Learning Rules",
                    description=f"With an acceptance rate of {acceptance_rate:.1%}, creating learning rules for common corrections could improve efficiency.",
                    priority=2,
                    metric_context={"acceptance_rate": float(acceptance_rate)},
                    action_suggestion="Review your most common corrections and create rules to automate them.",
                )
            )

        # Compare to monthly if available
        if monthly:
            monthly_rate = monthly.get("acceptance_rate", Decimal("0"))
            change = acceptance_rate - monthly_rate
            if change > self.SIGNIFICANT_TREND_THRESHOLD:
                insights.append(
                    ReviewInsight(
                        insight_type="achievement",
                        title="Acceptance Rate Improving",
                        description=f"Your acceptance rate has improved by {change:.1%} compared to last month.",
                        priority=3,
                        metric_context={
                            "weekly_rate": float(acceptance_rate),
                            "monthly_rate": float(monthly_rate),
                        },
                    )
                )

        return insights

    def _generate_ai_accuracy_insights(
        self,
        weekly: dict[str, Any],
        monthly: dict[str, Any] | None,
    ) -> list[ReviewInsight]:
        """Generate insights based on AI accuracy.

        Args:
            weekly: Weekly aggregated metrics
            monthly: Monthly aggregated metrics (for comparison)

        Returns:
            List of AI accuracy-related insights
        """
        insights = []
        ai_accuracy = weekly.get("ai_accuracy")

        if ai_accuracy is None:
            return insights

        if ai_accuracy >= self.HIGH_AI_ACCURACY_THRESHOLD:
            insights.append(
                ReviewInsight(
                    insight_type="info",
                    title="AI Performing Well",
                    description=f"AI accuracy of {ai_accuracy:.1%} indicates the system is well-calibrated to your needs.",
                    priority=5,
                    metric_context={"ai_accuracy": float(ai_accuracy)},
                )
            )
        elif ai_accuracy < self.LOW_AI_ACCURACY_THRESHOLD:
            insights.append(
                ReviewInsight(
                    insight_type="warning",
                    title="AI Accuracy Below Target",
                    description=f"AI accuracy of {ai_accuracy:.1%} is below the expected threshold. The system may need more training data.",
                    priority=2,
                    metric_context={"ai_accuracy": float(ai_accuracy)},
                    action_suggestion="Continue providing feedback, especially detailed corrections.",
                )
            )

        return insights

    def _generate_efficiency_insights(
        self,
        weekly: dict[str, Any],
        monthly: dict[str, Any] | None,
    ) -> list[ReviewInsight]:
        """Generate insights based on review efficiency.

        Args:
            weekly: Weekly aggregated metrics
            monthly: Monthly aggregated metrics (for comparison)

        Returns:
            List of efficiency-related insights
        """
        insights = []
        items_per_minute = weekly.get("avg_items_per_minute", Decimal("0"))
        batch_usage = weekly.get("batch_usage_rate", Decimal("0"))

        if items_per_minute < self.SLOW_REVIEW_THRESHOLD:
            insights.append(
                ReviewInsight(
                    insight_type="recommendation",
                    title="Improve Review Speed",
                    description="Your review pace is slower than average. Consider using batch operations for similar items.",
                    priority=3,
                    metric_context={"items_per_minute": float(items_per_minute)},
                    action_suggestion="Try using the batch feature to process similar items together.",
                )
            )

        if batch_usage < Decimal("0.1"):  # Less than 10% batch usage
            insights.append(
                ReviewInsight(
                    insight_type="recommendation",
                    title="Try Batch Operations",
                    description="Batch operations can save time by processing similar items together.",
                    priority=4,
                    metric_context={"batch_usage_rate": float(batch_usage)},
                    action_suggestion="Group items by sender or category for faster processing.",
                )
            )

        return insights

    def _generate_learning_insights(
        self,
        weekly: dict[str, Any],
    ) -> list[ReviewInsight]:
        """Generate insights based on learning system usage.

        Args:
            weekly: Weekly aggregated metrics

        Returns:
            List of learning-related insights
        """
        insights = []
        rules_created = weekly.get("rules_created", 0)
        rules_applied = weekly.get("rules_applied", 0)

        if rules_created > 0:
            insights.append(
                ReviewInsight(
                    insight_type="info",
                    title="Learning Rules Active",
                    description=f"You have {rules_created} new learning rules that were applied {rules_applied} times this week.",
                    priority=5,
                    metric_context={
                        "rules_created": rules_created,
                        "rules_applied": rules_applied,
                    },
                )
            )

        if rules_applied == 0 and rules_created == 0:
            insights.append(
                ReviewInsight(
                    insight_type="recommendation",
                    title="Enable Learning Rules",
                    description="Learning rules can automate repetitive corrections based on your feedback.",
                    priority=4,
                    action_suggestion="Check the suggested rules and enable those that match your workflow.",
                )
            )

        return insights

    def _summarize_trends(self, trends: list[TrendData]) -> str:
        """Generate a summary string from trend data.

        Args:
            trends: List of TrendData

        Returns:
            Summary string ("Improving", "Stable", "Declining", etc.)
        """
        if not trends:
            return "Insufficient data"

        positive_count = sum(1 for t in trends if t.is_positive_trend)
        negative_count = sum(1 for t in trends if not t.is_positive_trend and t.trend_direction != "stable")
        stable_count = sum(1 for t in trends if t.trend_direction == "stable")

        if positive_count > negative_count and positive_count > stable_count:
            return "Improving"
        if negative_count > positive_count and negative_count > stable_count:
            return "Declining"
        return "Stable"

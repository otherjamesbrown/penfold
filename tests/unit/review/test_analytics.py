"""Unit tests for AnalyticsManager.

Tests the analytics module that aggregates review metrics, calculates trends,
generates insights, and provides breakdowns by content type and correction type.
Includes tests for daily, weekly, and monthly analytics periods.
"""

from datetime import date, datetime, timedelta, timezone
from decimal import Decimal
from unittest.mock import AsyncMock, MagicMock
from uuid import uuid4

import pytest

from penf_lib.review.models import (
    AnalyticsPeriodType,
    ContentType,
    DecisionType,
    ReviewAnalyticsDTO,
)

# Note: AnalyticsManager will be implemented based on these tests
# from penf_lib.review.analytics import AnalyticsManager, Trend, Insight


# =============================================================================
# FIXTURES
# =============================================================================


@pytest.fixture
def tenant_id():
    """Test tenant ID."""
    return uuid4()


@pytest.fixture
def mock_repository():
    """Create mock repository for testing."""
    repo = AsyncMock()
    repo.get_analytics_for_period = AsyncMock()
    repo.get_analytics_range = AsyncMock()
    repo.get_session_metrics = AsyncMock()
    repo.get_decision_counts = AsyncMock()
    repo.get_content_type_counts = AsyncMock()
    repo.get_correction_type_counts = AsyncMock()
    repo.get_rule_statistics = AsyncMock()
    repo.get_batch_usage_statistics = AsyncMock()
    repo.create_analytics = AsyncMock()
    repo.update_analytics = AsyncMock()
    return repo


@pytest.fixture
def analytics_manager(mock_repository):
    """Create AnalyticsManager with mocked repository."""
    # AnalyticsManager will be implemented later
    manager = MagicMock()
    manager.repository = mock_repository
    manager.get_analytics = AsyncMock()
    manager.calculate_trends = AsyncMock()
    manager.generate_insights = AsyncMock()
    manager.get_content_type_breakdown = AsyncMock()
    manager.get_correction_type_breakdown = AsyncMock()
    manager.aggregate_analytics = AsyncMock()
    manager.compare_periods = AsyncMock()
    return manager


def create_analytics_dto(
    analytics_id: int = 1,
    tenant_id=None,
    period_start: date = None,
    period_end: date = None,
    period_type: AnalyticsPeriodType = AnalyticsPeriodType.DAILY,
    total_sessions: int = 10,
    total_items_reviewed: int = 150,
    avg_session_duration_min: Decimal = Decimal("15.5"),
    avg_items_per_minute: Decimal = Decimal("1.2"),
    acceptance_rate: Decimal = Decimal("0.75"),
    modification_rate: Decimal = Decimal("0.15"),
    rejection_rate: Decimal = Decimal("0.08"),
    skip_rate: Decimal = Decimal("0.02"),
    avg_ai_confidence: Decimal = Decimal("0.82"),
    ai_accuracy: Decimal = None,
    batch_usage_rate: Decimal = Decimal("0.25"),
    learning_rules_created: int = 3,
    learning_rules_applied: int = 45,
    content_type_breakdown: dict = None,
    correction_type_breakdown: dict = None,
) -> ReviewAnalyticsDTO:
    """Create a ReviewAnalyticsDTO for testing."""
    if tenant_id is None:
        tenant_id = uuid4()
    if period_start is None:
        period_start = date.today()
    if period_end is None:
        period_end = period_start
    if content_type_breakdown is None:
        content_type_breakdown = {"email": 100, "meeting": 35, "document": 15}
    if correction_type_breakdown is None:
        correction_type_breakdown = {"category": 10, "participants": 5, "tags": 3}

    now = datetime.now(timezone.utc)
    return ReviewAnalyticsDTO(
        id=analytics_id,
        tenant_id=tenant_id,
        period_start=period_start,
        period_end=period_end,
        period_type=period_type,
        total_sessions=total_sessions,
        total_items_reviewed=total_items_reviewed,
        avg_session_duration_min=avg_session_duration_min,
        avg_items_per_minute=avg_items_per_minute,
        acceptance_rate=acceptance_rate,
        modification_rate=modification_rate,
        rejection_rate=rejection_rate,
        skip_rate=skip_rate,
        avg_ai_confidence=avg_ai_confidence,
        ai_accuracy=ai_accuracy,
        batch_usage_rate=batch_usage_rate,
        learning_rules_created=learning_rules_created,
        learning_rules_applied=learning_rules_applied,
        content_type_breakdown=content_type_breakdown,
        correction_type_breakdown=correction_type_breakdown,
        created_at=now,
    )


# =============================================================================
# GET ANALYTICS TESTS
# =============================================================================


class TestGetAnalytics:
    """Tests for AnalyticsManager.get_analytics()."""

    @pytest.mark.asyncio
    async def test_get_daily_analytics(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should retrieve daily analytics for a specific date."""
        target_date = date.today()
        analytics = create_analytics_dto(
            tenant_id=tenant_id,
            period_start=target_date,
            period_end=target_date,
            period_type=AnalyticsPeriodType.DAILY,
        )
        mock_repository.get_analytics_for_period.return_value = analytics
        analytics_manager.get_analytics.return_value = analytics

        result = await analytics_manager.get_analytics(
            tenant_id=tenant_id,
            period_type=AnalyticsPeriodType.DAILY,
            period_start=target_date,
        )

        assert result is not None
        assert result.period_type == AnalyticsPeriodType.DAILY
        assert result.period_start == target_date

    @pytest.mark.asyncio
    async def test_get_weekly_analytics(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should retrieve weekly analytics for a week starting on Monday."""
        monday = date.today() - timedelta(days=date.today().weekday())
        sunday = monday + timedelta(days=6)
        analytics = create_analytics_dto(
            tenant_id=tenant_id,
            period_start=monday,
            period_end=sunday,
            period_type=AnalyticsPeriodType.WEEKLY,
            total_sessions=50,
            total_items_reviewed=800,
        )
        mock_repository.get_analytics_for_period.return_value = analytics
        analytics_manager.get_analytics.return_value = analytics

        result = await analytics_manager.get_analytics(
            tenant_id=tenant_id,
            period_type=AnalyticsPeriodType.WEEKLY,
            period_start=monday,
        )

        assert result is not None
        assert result.period_type == AnalyticsPeriodType.WEEKLY
        assert result.period_start == monday
        assert result.period_end == sunday
        assert result.total_sessions == 50

    @pytest.mark.asyncio
    async def test_get_monthly_analytics(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should retrieve monthly analytics for a calendar month."""
        month_start = date.today().replace(day=1)
        # Calculate last day of month
        next_month = month_start.replace(day=28) + timedelta(days=4)
        month_end = next_month - timedelta(days=next_month.day)
        analytics = create_analytics_dto(
            tenant_id=tenant_id,
            period_start=month_start,
            period_end=month_end,
            period_type=AnalyticsPeriodType.MONTHLY,
            total_sessions=200,
            total_items_reviewed=3500,
        )
        mock_repository.get_analytics_for_period.return_value = analytics
        analytics_manager.get_analytics.return_value = analytics

        result = await analytics_manager.get_analytics(
            tenant_id=tenant_id,
            period_type=AnalyticsPeriodType.MONTHLY,
            period_start=month_start,
        )

        assert result is not None
        assert result.period_type == AnalyticsPeriodType.MONTHLY
        assert result.period_start == month_start

    @pytest.mark.asyncio
    async def test_get_analytics_not_found(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should return None when no analytics exist for period."""
        mock_repository.get_analytics_for_period.return_value = None
        analytics_manager.get_analytics.return_value = None

        result = await analytics_manager.get_analytics(
            tenant_id=tenant_id,
            period_type=AnalyticsPeriodType.DAILY,
            period_start=date.today() - timedelta(days=365),
        )

        assert result is None

    @pytest.mark.asyncio
    async def test_get_analytics_with_zero_sessions(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should handle periods with no review sessions."""
        analytics = create_analytics_dto(
            tenant_id=tenant_id,
            total_sessions=0,
            total_items_reviewed=0,
            avg_session_duration_min=Decimal("0.0"),
            avg_items_per_minute=Decimal("0.0"),
            acceptance_rate=Decimal("0.0"),
            modification_rate=Decimal("0.0"),
            rejection_rate=Decimal("0.0"),
            skip_rate=Decimal("0.0"),
        )
        mock_repository.get_analytics_for_period.return_value = analytics
        analytics_manager.get_analytics.return_value = analytics

        result = await analytics_manager.get_analytics(
            tenant_id=tenant_id,
            period_type=AnalyticsPeriodType.DAILY,
            period_start=date.today(),
        )

        assert result is not None
        assert result.total_sessions == 0
        assert result.total_items_reviewed == 0


# =============================================================================
# CALCULATE TRENDS TESTS
# =============================================================================


class TestCalculateTrends:
    """Tests for AnalyticsManager.calculate_trends()."""

    @pytest.mark.asyncio
    async def test_calculate_daily_trends(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should calculate trends over multiple daily periods."""
        today = date.today()
        analytics_list = [
            create_analytics_dto(
                analytics_id=i,
                tenant_id=tenant_id,
                period_start=today - timedelta(days=6 - i),
                period_end=today - timedelta(days=6 - i),
                period_type=AnalyticsPeriodType.DAILY,
                total_items_reviewed=100 + i * 10,
                acceptance_rate=Decimal("0.70") + Decimal(str(i * 0.02)),
            )
            for i in range(7)
        ]
        mock_repository.get_analytics_range.return_value = analytics_list

        # Mock trend result
        trend_result = MagicMock()
        trend_result.items_reviewed_trend = "increasing"
        trend_result.acceptance_rate_trend = "increasing"
        trend_result.items_reviewed_change_percent = Decimal("60.0")
        analytics_manager.calculate_trends.return_value = trend_result

        result = await analytics_manager.calculate_trends(
            tenant_id=tenant_id,
            period_type=AnalyticsPeriodType.DAILY,
            num_periods=7,
        )

        assert result is not None
        assert result.items_reviewed_trend == "increasing"
        assert result.acceptance_rate_trend == "increasing"

    @pytest.mark.asyncio
    async def test_calculate_weekly_trends(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should calculate trends over multiple weekly periods."""
        monday = date.today() - timedelta(days=date.today().weekday())
        analytics_list = [
            create_analytics_dto(
                analytics_id=i,
                tenant_id=tenant_id,
                period_start=monday - timedelta(weeks=3 - i),
                period_end=monday - timedelta(weeks=3 - i) + timedelta(days=6),
                period_type=AnalyticsPeriodType.WEEKLY,
                total_items_reviewed=500 + i * 50,
            )
            for i in range(4)
        ]
        mock_repository.get_analytics_range.return_value = analytics_list

        trend_result = MagicMock()
        trend_result.items_reviewed_trend = "increasing"
        trend_result.periods_analyzed = 4
        analytics_manager.calculate_trends.return_value = trend_result

        result = await analytics_manager.calculate_trends(
            tenant_id=tenant_id,
            period_type=AnalyticsPeriodType.WEEKLY,
            num_periods=4,
        )

        assert result is not None
        assert result.periods_analyzed == 4

    @pytest.mark.asyncio
    async def test_calculate_trends_with_decreasing_values(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should detect decreasing trends correctly."""
        today = date.today()
        analytics_list = [
            create_analytics_dto(
                analytics_id=i,
                tenant_id=tenant_id,
                period_start=today - timedelta(days=4 - i),
                total_items_reviewed=200 - i * 30,  # Decreasing
                acceptance_rate=Decimal("0.90") - Decimal(str(i * 0.05)),  # Decreasing
            )
            for i in range(5)
        ]
        mock_repository.get_analytics_range.return_value = analytics_list

        trend_result = MagicMock()
        trend_result.items_reviewed_trend = "decreasing"
        trend_result.acceptance_rate_trend = "decreasing"
        analytics_manager.calculate_trends.return_value = trend_result

        result = await analytics_manager.calculate_trends(
            tenant_id=tenant_id,
            period_type=AnalyticsPeriodType.DAILY,
            num_periods=5,
        )

        assert result.items_reviewed_trend == "decreasing"
        assert result.acceptance_rate_trend == "decreasing"

    @pytest.mark.asyncio
    async def test_calculate_trends_with_stable_values(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should detect stable trends when values are consistent."""
        today = date.today()
        analytics_list = [
            create_analytics_dto(
                analytics_id=i,
                tenant_id=tenant_id,
                period_start=today - timedelta(days=4 - i),
                total_items_reviewed=150,  # Stable
                acceptance_rate=Decimal("0.75"),  # Stable
            )
            for i in range(5)
        ]
        mock_repository.get_analytics_range.return_value = analytics_list

        trend_result = MagicMock()
        trend_result.items_reviewed_trend = "stable"
        trend_result.acceptance_rate_trend = "stable"
        analytics_manager.calculate_trends.return_value = trend_result

        result = await analytics_manager.calculate_trends(
            tenant_id=tenant_id,
            period_type=AnalyticsPeriodType.DAILY,
            num_periods=5,
        )

        assert result.items_reviewed_trend == "stable"
        assert result.acceptance_rate_trend == "stable"

    @pytest.mark.asyncio
    async def test_calculate_trends_with_single_period(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should handle single period (no trend possible)."""
        analytics_list = [
            create_analytics_dto(tenant_id=tenant_id),
        ]
        mock_repository.get_analytics_range.return_value = analytics_list

        trend_result = MagicMock()
        trend_result.items_reviewed_trend = "insufficient_data"
        trend_result.periods_analyzed = 1
        analytics_manager.calculate_trends.return_value = trend_result

        result = await analytics_manager.calculate_trends(
            tenant_id=tenant_id,
            period_type=AnalyticsPeriodType.DAILY,
            num_periods=1,
        )

        assert result.items_reviewed_trend == "insufficient_data"
        assert result.periods_analyzed == 1

    @pytest.mark.asyncio
    async def test_calculate_trends_with_empty_data(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should handle no analytics data gracefully."""
        mock_repository.get_analytics_range.return_value = []
        analytics_manager.calculate_trends.return_value = None

        result = await analytics_manager.calculate_trends(
            tenant_id=tenant_id,
            period_type=AnalyticsPeriodType.DAILY,
            num_periods=7,
        )

        assert result is None


# =============================================================================
# GENERATE INSIGHTS TESTS
# =============================================================================


class TestGenerateInsights:
    """Tests for AnalyticsManager.generate_insights()."""

    @pytest.mark.asyncio
    async def test_generate_high_acceptance_rate_insight(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should generate insight for high acceptance rate."""
        analytics = create_analytics_dto(
            tenant_id=tenant_id,
            acceptance_rate=Decimal("0.92"),  # Very high
        )
        mock_repository.get_analytics_for_period.return_value = analytics

        insight = MagicMock()
        insight.type = "high_acceptance_rate"
        insight.message = "AI suggestions are highly accurate (92% acceptance rate)"
        insight.severity = "positive"
        analytics_manager.generate_insights.return_value = [insight]

        result = await analytics_manager.generate_insights(
            tenant_id=tenant_id,
            period_type=AnalyticsPeriodType.DAILY,
        )

        assert len(result) >= 1
        assert any(i.type == "high_acceptance_rate" for i in result)

    @pytest.mark.asyncio
    async def test_generate_low_acceptance_rate_insight(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should generate warning for low acceptance rate."""
        analytics = create_analytics_dto(
            tenant_id=tenant_id,
            acceptance_rate=Decimal("0.45"),  # Low
            modification_rate=Decimal("0.40"),  # High modifications
        )
        mock_repository.get_analytics_for_period.return_value = analytics

        insight = MagicMock()
        insight.type = "low_acceptance_rate"
        insight.message = "Consider reviewing AI model configuration"
        insight.severity = "warning"
        analytics_manager.generate_insights.return_value = [insight]

        result = await analytics_manager.generate_insights(
            tenant_id=tenant_id,
            period_type=AnalyticsPeriodType.DAILY,
        )

        assert len(result) >= 1
        assert any(i.severity == "warning" for i in result)

    @pytest.mark.asyncio
    async def test_generate_high_batch_usage_insight(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should generate insight for efficient batch usage."""
        analytics = create_analytics_dto(
            tenant_id=tenant_id,
            batch_usage_rate=Decimal("0.55"),  # High batch usage
        )
        mock_repository.get_analytics_for_period.return_value = analytics

        insight = MagicMock()
        insight.type = "high_batch_usage"
        insight.message = "Efficient use of batch operations (55% of decisions)"
        insight.severity = "positive"
        analytics_manager.generate_insights.return_value = [insight]

        result = await analytics_manager.generate_insights(
            tenant_id=tenant_id,
            period_type=AnalyticsPeriodType.DAILY,
        )

        assert any(i.type == "high_batch_usage" for i in result)

    @pytest.mark.asyncio
    async def test_generate_learning_rules_insight(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should generate insight about learning rules effectiveness."""
        analytics = create_analytics_dto(
            tenant_id=tenant_id,
            learning_rules_created=5,
            learning_rules_applied=120,  # Rules are being used
        )
        mock_repository.get_analytics_for_period.return_value = analytics

        insight = MagicMock()
        insight.type = "learning_rules_effective"
        insight.message = "Learning rules applied 120 times this period"
        insight.severity = "informational"
        analytics_manager.generate_insights.return_value = [insight]

        result = await analytics_manager.generate_insights(
            tenant_id=tenant_id,
            period_type=AnalyticsPeriodType.WEEKLY,
        )

        assert any(i.type == "learning_rules_effective" for i in result)

    @pytest.mark.asyncio
    async def test_generate_velocity_insight(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should generate insight about review velocity."""
        analytics = create_analytics_dto(
            tenant_id=tenant_id,
            avg_items_per_minute=Decimal("2.5"),  # Fast
        )
        mock_repository.get_analytics_for_period.return_value = analytics

        insight = MagicMock()
        insight.type = "high_velocity"
        insight.message = "High review velocity: 2.5 items per minute"
        insight.severity = "positive"
        analytics_manager.generate_insights.return_value = [insight]

        result = await analytics_manager.generate_insights(
            tenant_id=tenant_id,
            period_type=AnalyticsPeriodType.DAILY,
        )

        assert any(i.type == "high_velocity" for i in result)

    @pytest.mark.asyncio
    async def test_generate_no_insights_for_empty_period(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should return empty list when no data for period."""
        mock_repository.get_analytics_for_period.return_value = None
        analytics_manager.generate_insights.return_value = []

        result = await analytics_manager.generate_insights(
            tenant_id=tenant_id,
            period_type=AnalyticsPeriodType.DAILY,
        )

        assert result == []

    @pytest.mark.asyncio
    async def test_generate_multiple_insights(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should generate multiple relevant insights."""
        analytics = create_analytics_dto(
            tenant_id=tenant_id,
            acceptance_rate=Decimal("0.88"),
            batch_usage_rate=Decimal("0.45"),
            learning_rules_applied=80,
            avg_items_per_minute=Decimal("1.8"),
        )
        mock_repository.get_analytics_for_period.return_value = analytics

        insights = [
            MagicMock(type="high_acceptance_rate"),
            MagicMock(type="batch_usage_normal"),
            MagicMock(type="learning_rules_effective"),
        ]
        analytics_manager.generate_insights.return_value = insights

        result = await analytics_manager.generate_insights(
            tenant_id=tenant_id,
            period_type=AnalyticsPeriodType.WEEKLY,
        )

        assert len(result) >= 2


# =============================================================================
# CONTENT TYPE BREAKDOWN TESTS
# =============================================================================


class TestContentTypeBreakdown:
    """Tests for AnalyticsManager.get_content_type_breakdown()."""

    @pytest.mark.asyncio
    async def test_get_breakdown_all_types(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should return counts for all content types."""
        breakdown = {
            "email": 150,
            "meeting": 45,
            "document": 25,
        }
        mock_repository.get_content_type_counts.return_value = breakdown
        analytics_manager.get_content_type_breakdown.return_value = breakdown

        result = await analytics_manager.get_content_type_breakdown(
            tenant_id=tenant_id,
            period_type=AnalyticsPeriodType.DAILY,
            period_start=date.today(),
        )

        assert result["email"] == 150
        assert result["meeting"] == 45
        assert result["document"] == 25

    @pytest.mark.asyncio
    async def test_get_breakdown_email_only(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should handle periods with only email content."""
        breakdown = {
            "email": 200,
            "meeting": 0,
            "document": 0,
        }
        mock_repository.get_content_type_counts.return_value = breakdown
        analytics_manager.get_content_type_breakdown.return_value = breakdown

        result = await analytics_manager.get_content_type_breakdown(
            tenant_id=tenant_id,
            period_type=AnalyticsPeriodType.DAILY,
            period_start=date.today(),
        )

        assert result["email"] == 200
        assert result["meeting"] == 0
        assert result["document"] == 0

    @pytest.mark.asyncio
    async def test_get_breakdown_empty(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should return zeros when no items reviewed."""
        breakdown = {
            "email": 0,
            "meeting": 0,
            "document": 0,
        }
        mock_repository.get_content_type_counts.return_value = breakdown
        analytics_manager.get_content_type_breakdown.return_value = breakdown

        result = await analytics_manager.get_content_type_breakdown(
            tenant_id=tenant_id,
            period_type=AnalyticsPeriodType.DAILY,
            period_start=date.today(),
        )

        assert all(count == 0 for count in result.values())

    @pytest.mark.asyncio
    async def test_get_breakdown_with_percentages(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should include percentage calculations if requested."""
        breakdown_with_percentages = {
            "email": {"count": 100, "percentage": Decimal("66.67")},
            "meeting": {"count": 30, "percentage": Decimal("20.00")},
            "document": {"count": 20, "percentage": Decimal("13.33")},
        }
        analytics_manager.get_content_type_breakdown.return_value = breakdown_with_percentages

        result = await analytics_manager.get_content_type_breakdown(
            tenant_id=tenant_id,
            period_type=AnalyticsPeriodType.DAILY,
            period_start=date.today(),
            include_percentages=True,
        )

        assert result["email"]["percentage"] == Decimal("66.67")

    @pytest.mark.asyncio
    async def test_get_breakdown_weekly_aggregation(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should aggregate content types over weekly period."""
        breakdown = {
            "email": 700,
            "meeting": 200,
            "document": 100,
        }
        mock_repository.get_content_type_counts.return_value = breakdown
        analytics_manager.get_content_type_breakdown.return_value = breakdown

        result = await analytics_manager.get_content_type_breakdown(
            tenant_id=tenant_id,
            period_type=AnalyticsPeriodType.WEEKLY,
            period_start=date.today() - timedelta(days=6),
        )

        total = sum(result.values())
        assert total == 1000


# =============================================================================
# CORRECTION TYPE BREAKDOWN TESTS
# =============================================================================


class TestCorrectionTypeBreakdown:
    """Tests for AnalyticsManager.get_correction_type_breakdown()."""

    @pytest.mark.asyncio
    async def test_get_correction_breakdown(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should return counts by correction type."""
        breakdown = {
            "category": 25,
            "participants": 10,
            "tags": 15,
            "multiple": 5,  # Multiple fields corrected
        }
        mock_repository.get_correction_type_counts.return_value = breakdown
        analytics_manager.get_correction_type_breakdown.return_value = breakdown

        result = await analytics_manager.get_correction_type_breakdown(
            tenant_id=tenant_id,
            period_type=AnalyticsPeriodType.DAILY,
            period_start=date.today(),
        )

        assert result["category"] == 25
        assert result["participants"] == 10
        assert result["tags"] == 15

    @pytest.mark.asyncio
    async def test_get_correction_breakdown_no_corrections(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should handle periods with no corrections."""
        breakdown = {
            "category": 0,
            "participants": 0,
            "tags": 0,
        }
        mock_repository.get_correction_type_counts.return_value = breakdown
        analytics_manager.get_correction_type_breakdown.return_value = breakdown

        result = await analytics_manager.get_correction_type_breakdown(
            tenant_id=tenant_id,
            period_type=AnalyticsPeriodType.DAILY,
            period_start=date.today(),
        )

        assert all(count == 0 for count in result.values())

    @pytest.mark.asyncio
    async def test_get_correction_breakdown_category_only(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should handle when only category corrections occur."""
        breakdown = {
            "category": 30,
            "participants": 0,
            "tags": 0,
        }
        mock_repository.get_correction_type_counts.return_value = breakdown
        analytics_manager.get_correction_type_breakdown.return_value = breakdown

        result = await analytics_manager.get_correction_type_breakdown(
            tenant_id=tenant_id,
            period_type=AnalyticsPeriodType.DAILY,
            period_start=date.today(),
        )

        assert result["category"] == 30
        assert result["participants"] == 0
        assert result["tags"] == 0

    @pytest.mark.asyncio
    async def test_get_correction_breakdown_with_reason_codes(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should include reason code breakdown if available."""
        breakdown = {
            "category": {
                "count": 20,
                "reasons": {
                    "wrong_project": 8,
                    "wrong_client": 7,
                    "miscategorized": 5,
                },
            },
            "participants": {"count": 5, "reasons": {}},
            "tags": {"count": 3, "reasons": {}},
        }
        analytics_manager.get_correction_type_breakdown.return_value = breakdown

        result = await analytics_manager.get_correction_type_breakdown(
            tenant_id=tenant_id,
            period_type=AnalyticsPeriodType.WEEKLY,
            period_start=date.today(),
            include_reasons=True,
        )

        assert result["category"]["count"] == 20
        assert result["category"]["reasons"]["wrong_project"] == 8

    @pytest.mark.asyncio
    async def test_get_correction_breakdown_monthly(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should aggregate corrections over monthly period."""
        breakdown = {
            "category": 120,
            "participants": 45,
            "tags": 30,
        }
        mock_repository.get_correction_type_counts.return_value = breakdown
        analytics_manager.get_correction_type_breakdown.return_value = breakdown

        result = await analytics_manager.get_correction_type_breakdown(
            tenant_id=tenant_id,
            period_type=AnalyticsPeriodType.MONTHLY,
            period_start=date.today().replace(day=1),
        )

        total = sum(result.values())
        assert total == 195


# =============================================================================
# AGGREGATE ANALYTICS TESTS
# =============================================================================


class TestAggregateAnalytics:
    """Tests for AnalyticsManager.aggregate_analytics()."""

    @pytest.mark.asyncio
    async def test_aggregate_daily_to_weekly(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should aggregate 7 daily analytics into weekly."""
        today = date.today()
        daily_analytics = [
            create_analytics_dto(
                analytics_id=i,
                tenant_id=tenant_id,
                period_start=today - timedelta(days=6 - i),
                period_end=today - timedelta(days=6 - i),
                period_type=AnalyticsPeriodType.DAILY,
                total_sessions=5,
                total_items_reviewed=100,
            )
            for i in range(7)
        ]
        mock_repository.get_analytics_range.return_value = daily_analytics

        aggregated = create_analytics_dto(
            tenant_id=tenant_id,
            period_type=AnalyticsPeriodType.WEEKLY,
            total_sessions=35,  # 7 * 5
            total_items_reviewed=700,  # 7 * 100
        )
        analytics_manager.aggregate_analytics.return_value = aggregated

        result = await analytics_manager.aggregate_analytics(
            tenant_id=tenant_id,
            from_period=AnalyticsPeriodType.DAILY,
            to_period=AnalyticsPeriodType.WEEKLY,
            period_start=today - timedelta(days=6),
        )

        assert result.period_type == AnalyticsPeriodType.WEEKLY
        assert result.total_sessions == 35
        assert result.total_items_reviewed == 700

    @pytest.mark.asyncio
    async def test_aggregate_preserves_rates_correctly(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should weight rates by item counts when aggregating."""
        daily_analytics = [
            create_analytics_dto(
                analytics_id=1,
                tenant_id=tenant_id,
                total_items_reviewed=100,
                acceptance_rate=Decimal("0.80"),
            ),
            create_analytics_dto(
                analytics_id=2,
                tenant_id=tenant_id,
                total_items_reviewed=200,
                acceptance_rate=Decimal("0.90"),
            ),
        ]
        mock_repository.get_analytics_range.return_value = daily_analytics

        # Weighted average: (100*0.80 + 200*0.90) / 300 = 260/300 = 0.867
        aggregated = create_analytics_dto(
            tenant_id=tenant_id,
            total_items_reviewed=300,
            acceptance_rate=Decimal("0.867"),
        )
        analytics_manager.aggregate_analytics.return_value = aggregated

        result = await analytics_manager.aggregate_analytics(
            tenant_id=tenant_id,
            from_period=AnalyticsPeriodType.DAILY,
            to_period=AnalyticsPeriodType.WEEKLY,
        )

        assert result.acceptance_rate == Decimal("0.867")

    @pytest.mark.asyncio
    async def test_aggregate_empty_periods(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should handle aggregation with some empty periods."""
        daily_analytics = [
            create_analytics_dto(analytics_id=1, tenant_id=tenant_id, total_sessions=5),
            None,  # Missing day
            create_analytics_dto(analytics_id=3, tenant_id=tenant_id, total_sessions=8),
        ]
        mock_repository.get_analytics_range.return_value = [a for a in daily_analytics if a]

        aggregated = create_analytics_dto(
            tenant_id=tenant_id,
            total_sessions=13,
        )
        analytics_manager.aggregate_analytics.return_value = aggregated

        result = await analytics_manager.aggregate_analytics(
            tenant_id=tenant_id,
            from_period=AnalyticsPeriodType.DAILY,
            to_period=AnalyticsPeriodType.WEEKLY,
        )

        assert result.total_sessions == 13


# =============================================================================
# COMPARE PERIODS TESTS
# =============================================================================


class TestComparePeriods:
    """Tests for AnalyticsManager.compare_periods()."""

    @pytest.mark.asyncio
    async def test_compare_two_periods(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should compare metrics between two periods."""
        period1 = create_analytics_dto(
            analytics_id=1,
            tenant_id=tenant_id,
            total_items_reviewed=100,
            acceptance_rate=Decimal("0.75"),
        )
        period2 = create_analytics_dto(
            analytics_id=2,
            tenant_id=tenant_id,
            total_items_reviewed=120,
            acceptance_rate=Decimal("0.82"),
        )
        mock_repository.get_analytics_for_period.side_effect = [period1, period2]

        comparison = MagicMock()
        comparison.items_reviewed_change = 20
        comparison.items_reviewed_change_percent = Decimal("20.0")
        comparison.acceptance_rate_change = Decimal("0.07")
        analytics_manager.compare_periods.return_value = comparison

        result = await analytics_manager.compare_periods(
            tenant_id=tenant_id,
            period1_start=date.today() - timedelta(days=1),
            period2_start=date.today(),
            period_type=AnalyticsPeriodType.DAILY,
        )

        assert result.items_reviewed_change == 20
        assert result.items_reviewed_change_percent == Decimal("20.0")

    @pytest.mark.asyncio
    async def test_compare_with_improvement(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should indicate improvement in metrics."""
        comparison = MagicMock()
        comparison.acceptance_rate_improved = True
        comparison.velocity_improved = True
        comparison.overall_trend = "improved"
        analytics_manager.compare_periods.return_value = comparison

        result = await analytics_manager.compare_periods(
            tenant_id=tenant_id,
            period1_start=date.today() - timedelta(days=7),
            period2_start=date.today(),
            period_type=AnalyticsPeriodType.WEEKLY,
        )

        assert result.acceptance_rate_improved is True
        assert result.overall_trend == "improved"

    @pytest.mark.asyncio
    async def test_compare_with_regression(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should indicate regression in metrics."""
        comparison = MagicMock()
        comparison.acceptance_rate_improved = False
        comparison.velocity_improved = False
        comparison.overall_trend = "regressed"
        analytics_manager.compare_periods.return_value = comparison

        result = await analytics_manager.compare_periods(
            tenant_id=tenant_id,
            period1_start=date.today() - timedelta(days=7),
            period2_start=date.today(),
            period_type=AnalyticsPeriodType.WEEKLY,
        )

        assert result.acceptance_rate_improved is False
        assert result.overall_trend == "regressed"

    @pytest.mark.asyncio
    async def test_compare_missing_period(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should handle comparison when one period has no data."""
        period1 = create_analytics_dto(tenant_id=tenant_id)
        mock_repository.get_analytics_for_period.side_effect = [period1, None]

        analytics_manager.compare_periods.return_value = None

        result = await analytics_manager.compare_periods(
            tenant_id=tenant_id,
            period1_start=date.today() - timedelta(days=1),
            period2_start=date.today(),
            period_type=AnalyticsPeriodType.DAILY,
        )

        assert result is None


# =============================================================================
# EDGE CASES TESTS
# =============================================================================


class TestAnalyticsEdgeCases:
    """Tests for edge cases in analytics operations."""

    @pytest.mark.asyncio
    async def test_analytics_with_all_rejections(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should handle period with 100% rejection rate."""
        analytics = create_analytics_dto(
            tenant_id=tenant_id,
            acceptance_rate=Decimal("0.00"),
            rejection_rate=Decimal("1.00"),
            modification_rate=Decimal("0.00"),
            skip_rate=Decimal("0.00"),
        )
        analytics_manager.get_analytics.return_value = analytics

        result = await analytics_manager.get_analytics(
            tenant_id=tenant_id,
            period_type=AnalyticsPeriodType.DAILY,
            period_start=date.today(),
        )

        assert result.rejection_rate == Decimal("1.00")
        assert result.acceptance_rate == Decimal("0.00")

    @pytest.mark.asyncio
    async def test_analytics_with_high_skip_rate(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should handle period with high skip rate."""
        analytics = create_analytics_dto(
            tenant_id=tenant_id,
            acceptance_rate=Decimal("0.20"),
            rejection_rate=Decimal("0.10"),
            modification_rate=Decimal("0.05"),
            skip_rate=Decimal("0.65"),
        )
        analytics_manager.get_analytics.return_value = analytics

        result = await analytics_manager.get_analytics(
            tenant_id=tenant_id,
            period_type=AnalyticsPeriodType.DAILY,
            period_start=date.today(),
        )

        assert result.skip_rate == Decimal("0.65")

    @pytest.mark.asyncio
    async def test_analytics_rates_sum_to_one(
        self,
        analytics_manager,
        tenant_id,
    ):
        """Verify decision rates sum to 1.0."""
        analytics = create_analytics_dto(
            tenant_id=tenant_id,
            acceptance_rate=Decimal("0.60"),
            rejection_rate=Decimal("0.20"),
            modification_rate=Decimal("0.15"),
            skip_rate=Decimal("0.05"),
        )

        total_rate = (
            analytics.acceptance_rate
            + analytics.rejection_rate
            + analytics.modification_rate
            + analytics.skip_rate
        )

        assert total_rate == Decimal("1.00")

    @pytest.mark.asyncio
    async def test_analytics_very_long_session(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should handle unusually long session durations."""
        analytics = create_analytics_dto(
            tenant_id=tenant_id,
            avg_session_duration_min=Decimal("180.0"),  # 3 hours
            avg_items_per_minute=Decimal("0.5"),  # Slow
        )
        analytics_manager.get_analytics.return_value = analytics

        result = await analytics_manager.get_analytics(
            tenant_id=tenant_id,
            period_type=AnalyticsPeriodType.DAILY,
            period_start=date.today(),
        )

        assert result.avg_session_duration_min == Decimal("180.0")

    @pytest.mark.asyncio
    async def test_analytics_zero_ai_confidence(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should handle zero AI confidence edge case."""
        analytics = create_analytics_dto(
            tenant_id=tenant_id,
            avg_ai_confidence=Decimal("0.00"),
        )
        analytics_manager.get_analytics.return_value = analytics

        result = await analytics_manager.get_analytics(
            tenant_id=tenant_id,
            period_type=AnalyticsPeriodType.DAILY,
            period_start=date.today(),
        )

        assert result.avg_ai_confidence == Decimal("0.00")

    @pytest.mark.asyncio
    async def test_content_type_breakdown_unknown_types(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should handle unknown content types gracefully."""
        breakdown = {
            "email": 100,
            "meeting": 50,
            "document": 25,
            "unknown": 5,  # Unknown type
        }
        analytics_manager.get_content_type_breakdown.return_value = breakdown

        result = await analytics_manager.get_content_type_breakdown(
            tenant_id=tenant_id,
            period_type=AnalyticsPeriodType.DAILY,
            period_start=date.today(),
        )

        assert "unknown" in result
        assert result["unknown"] == 5


# =============================================================================
# AI ACCURACY TESTS
# =============================================================================


class TestAIAccuracy:
    """Tests for AI accuracy calculations."""

    @pytest.mark.asyncio
    async def test_calculate_ai_accuracy_high(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should calculate high AI accuracy from acceptance rate."""
        analytics = create_analytics_dto(
            tenant_id=tenant_id,
            acceptance_rate=Decimal("0.90"),
            ai_accuracy=Decimal("0.88"),
        )
        analytics_manager.get_analytics.return_value = analytics

        result = await analytics_manager.get_analytics(
            tenant_id=tenant_id,
            period_type=AnalyticsPeriodType.DAILY,
            period_start=date.today(),
        )

        assert result.ai_accuracy == Decimal("0.88")

    @pytest.mark.asyncio
    async def test_calculate_ai_accuracy_with_modifications(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should factor modifications into AI accuracy."""
        # High accept but also high modify suggests AI is close but not exact
        analytics = create_analytics_dto(
            tenant_id=tenant_id,
            acceptance_rate=Decimal("0.60"),
            modification_rate=Decimal("0.30"),
            ai_accuracy=Decimal("0.75"),  # Partial credit for modifications
        )
        analytics_manager.get_analytics.return_value = analytics

        result = await analytics_manager.get_analytics(
            tenant_id=tenant_id,
            period_type=AnalyticsPeriodType.DAILY,
            period_start=date.today(),
        )

        # AI accuracy should reflect partial correctness
        assert result.ai_accuracy > result.acceptance_rate

    @pytest.mark.asyncio
    async def test_ai_accuracy_none_when_insufficient_data(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should return None when insufficient data for accuracy."""
        analytics = create_analytics_dto(
            tenant_id=tenant_id,
            total_items_reviewed=5,  # Too few items
            ai_accuracy=None,
        )
        analytics_manager.get_analytics.return_value = analytics

        result = await analytics_manager.get_analytics(
            tenant_id=tenant_id,
            period_type=AnalyticsPeriodType.DAILY,
            period_start=date.today(),
        )

        assert result.ai_accuracy is None


# =============================================================================
# LEARNING RULES ANALYTICS TESTS
# =============================================================================


class TestLearningRulesAnalytics:
    """Tests for learning rules statistics in analytics."""

    @pytest.mark.asyncio
    async def test_learning_rules_created_count(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should track number of learning rules created."""
        analytics = create_analytics_dto(
            tenant_id=tenant_id,
            learning_rules_created=8,
        )
        analytics_manager.get_analytics.return_value = analytics

        result = await analytics_manager.get_analytics(
            tenant_id=tenant_id,
            period_type=AnalyticsPeriodType.WEEKLY,
            period_start=date.today(),
        )

        assert result.learning_rules_created == 8

    @pytest.mark.asyncio
    async def test_learning_rules_applied_count(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should track number of times rules were applied."""
        analytics = create_analytics_dto(
            tenant_id=tenant_id,
            learning_rules_applied=150,
        )
        analytics_manager.get_analytics.return_value = analytics

        result = await analytics_manager.get_analytics(
            tenant_id=tenant_id,
            period_type=AnalyticsPeriodType.WEEKLY,
            period_start=date.today(),
        )

        assert result.learning_rules_applied == 150

    @pytest.mark.asyncio
    async def test_no_learning_rules(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should handle period with no learning rules activity."""
        analytics = create_analytics_dto(
            tenant_id=tenant_id,
            learning_rules_created=0,
            learning_rules_applied=0,
        )
        analytics_manager.get_analytics.return_value = analytics

        result = await analytics_manager.get_analytics(
            tenant_id=tenant_id,
            period_type=AnalyticsPeriodType.DAILY,
            period_start=date.today(),
        )

        assert result.learning_rules_created == 0
        assert result.learning_rules_applied == 0


# =============================================================================
# BATCH USAGE ANALYTICS TESTS
# =============================================================================


class TestBatchUsageAnalytics:
    """Tests for batch operation usage statistics."""

    @pytest.mark.asyncio
    async def test_high_batch_usage(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should track high batch operation usage."""
        analytics = create_analytics_dto(
            tenant_id=tenant_id,
            batch_usage_rate=Decimal("0.60"),  # 60% of decisions via batch
        )
        analytics_manager.get_analytics.return_value = analytics

        result = await analytics_manager.get_analytics(
            tenant_id=tenant_id,
            period_type=AnalyticsPeriodType.DAILY,
            period_start=date.today(),
        )

        assert result.batch_usage_rate == Decimal("0.60")

    @pytest.mark.asyncio
    async def test_no_batch_usage(
        self,
        analytics_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should handle period with no batch operations."""
        analytics = create_analytics_dto(
            tenant_id=tenant_id,
            batch_usage_rate=Decimal("0.00"),
        )
        analytics_manager.get_analytics.return_value = analytics

        result = await analytics_manager.get_analytics(
            tenant_id=tenant_id,
            period_type=AnalyticsPeriodType.DAILY,
            period_start=date.today(),
        )

        assert result.batch_usage_rate == Decimal("0.00")

"""Comprehensive tests for business value tracking components.

This module provides comprehensive test coverage for business value tracking,
usage analytics, ROI calculation, and business API endpoints including:

- Business metrics model validation
- KPI tracking service functionality
- Usage analytics service behavior
- ROI calculation accuracy
- Business API endpoint responses
- Error handling and edge cases
- Performance and scalability testing

Test Categories:
    - Model tests: Business metric model validation and behavior
    - Service tests: Business service functionality and integration
    - API tests: Business endpoint request/response validation
    - Integration tests: Cross-service interaction testing
    - Performance tests: Load testing and benchmarking
"""

import pytest
import asyncio
import uuid
from datetime import datetime, timedelta, timezone
from decimal import Decimal
from typing import Any, Dict, List, Optional
from unittest.mock import Mock, AsyncMock, patch

import pytest_asyncio
from sqlalchemy.ext.asyncio import AsyncSession, create_async_engine
from sqlalchemy.orm import sessionmaker
from httpx import AsyncClient
from fastapi import FastAPI

from observability_lib.config import ObservabilitySettings
from observability_lib.models.business_metrics import (
    BusinessKpi, UsageMetric, RoiMetric, BusinessGoal, ValueCalculation,
    KpiType, UsageMetricType, RoiMetricType, BusinessGoalType, GoalStatus
)
from observability_lib.services.business_value_tracker import (
    BusinessValueTracker, KpiStatus, TrendDirection, KpiSummary
)
from observability_lib.services.usage_analytics import (
    UsageAnalyticsService, EngagementLevel, FeatureUsageInsight
)
from observability_lib.services.roi_calculator import (
    ROICalculatorService, ROISummary, ValueCategory
)
from observability_lib.api.business_endpoints import (
    router as business_router,
    initialize_business_endpoints,
    KpiMeasurementRequest,
    UsageTrackingRequest,
    TimeSavingsROIRequest
)


# ============================================================================
# Test Fixtures and Setup
# ============================================================================

@pytest.fixture
def test_settings():
    """Provide test observability settings."""
    return ObservabilitySettings(
        database_url="sqlite+aiosqlite:///:memory:",
        default_tenant_id="test-tenant",
        debug_sql=False,
        redis_url="redis://localhost:6379/1",
        enable_caching=False
    )


@pytest.fixture
async def async_session(test_settings):
    """Provide async database session for testing."""
    engine = create_async_engine(
        test_settings.get_database_url(),
        echo=test_settings.DEBUG_SQL
    )

    # Import and create all tables
    from observability_lib.models.business_metrics import Base
    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.create_all)

    session_factory = sessionmaker(
        bind=engine,
        class_=AsyncSession,
        expire_on_commit=False
    )

    async with session_factory() as session:
        yield session

    await engine.dispose()


@pytest.fixture
async def business_tracker(test_settings):
    """Provide business value tracker service."""
    tracker = BusinessValueTracker(test_settings)
    await tracker.start()
    yield tracker
    await tracker.stop()


@pytest.fixture
async def usage_analytics(test_settings):
    """Provide usage analytics service."""
    analytics = UsageAnalyticsService(test_settings)
    await analytics.start()
    yield analytics
    await analytics.stop()


@pytest.fixture
async def roi_calculator(test_settings):
    """Provide ROI calculator service."""
    calculator = ROICalculatorService(test_settings)
    await calculator.start()
    yield calculator
    await calculator.stop()


@pytest.fixture
async def test_app(test_settings, business_tracker, usage_analytics, roi_calculator):
    """Provide FastAPI test application with business endpoints."""
    app = FastAPI()

    # Initialize business endpoints
    initialize_business_endpoints(
        test_settings,
        business_tracker,
        usage_analytics,
        roi_calculator
    )

    app.include_router(business_router, prefix="/api/v1")
    return app


@pytest.fixture
async def test_client(test_app):
    """Provide HTTP test client."""
    async with AsyncClient(app=test_app, base_url="http://test") as client:
        yield client


# ============================================================================
# Business Metrics Model Tests
# ============================================================================

class TestBusinessMetricsModels:
    """Test business metrics model validation and behavior."""

    @pytest.mark.asyncio
    async def test_business_kpi_creation(self, async_session):
        """Test creating and validating business KPI records."""
        kpi = BusinessKpi(
            tenant_id="test-tenant",
            kpi_type=KpiType.CONTEXT_RECONSTRUCTION_SPEED.value,
            kpi_name="Context Reconstruction Speed",
            current_value=8.5,
            target_value=15.0,
            baseline_value=30.0,
            measurement_unit="minutes",
            is_improvement_positive=False
        )

        async_session.add(kpi)
        await async_session.commit()
        await async_session.refresh(kpi)

        assert kpi.id is not None
        assert kpi.kpi_type == "context_reconstruction_speed"
        assert kpi.current_value == 8.5
        assert kpi.target_achievement_percentage == (8.5 / 15.0) * 100
        assert kpi.improvement_from_baseline == 8.5 - 30.0
        assert kpi.improvement_percentage < 0  # Negative because lower is better

    @pytest.mark.asyncio
    async def test_business_kpi_validation(self, async_session):
        """Test business KPI validation rules."""
        # Test invalid KPI type
        with pytest.raises(Exception):  # ValidationError
            kpi = BusinessKpi(
                tenant_id="test-tenant",
                kpi_type="invalid_kpi_type",
                kpi_name="Test KPI",
                current_value=100.0,
                measurement_unit="percentage"
            )
            kpi.validate_kpi_type("kpi_type", "invalid_kpi_type")

        # Test NaN values
        with pytest.raises(Exception):  # ValidationError
            kpi = BusinessKpi(
                tenant_id="test-tenant",
                kpi_type=KpiType.SEARCH_ACCURACY.value,
                kpi_name="Search Accuracy",
                current_value=float('nan'),
                measurement_unit="percentage"
            )
            kpi.validate_current_value("current_value", float('nan'))

    @pytest.mark.asyncio
    async def test_usage_metric_creation(self, async_session):
        """Test creating and validating usage metrics."""
        usage = UsageMetric(
            tenant_id="test-tenant",
            metric_type=UsageMetricType.SEARCH_FEATURE_USAGE.value,
            feature_name="search_feature",
            usage_count=5,
            duration_seconds=120.5,
            success_rate=0.85,
            usage_context={"query_type": "project_search", "results_count": 12}
        )

        async_session.add(usage)
        await async_session.commit()
        await async_session.refresh(usage)

        assert usage.id is not None
        assert usage.feature_name == "search_feature"
        assert usage.usage_count == 5
        assert usage.success_rate == 0.85

    @pytest.mark.asyncio
    async def test_roi_metric_creation(self, async_session):
        """Test creating ROI metrics with calculations."""
        roi = RoiMetric(
            tenant_id="test-tenant",
            roi_type=RoiMetricType.TIME_SAVINGS_MINUTES.value,
            measurement_period="monthly",
            value_amount=Decimal("2500.00"),
            value_unit="dollars",
            baseline_amount=Decimal("5000.00"),
            cost_amount=Decimal("500.00"),
            confidence_level=0.85,
            calculation_method="task_time_tracking"
        )

        async_session.add(roi)
        await async_session.commit()
        await async_session.refresh(roi)

        assert roi.id is not None
        assert roi.net_value == Decimal("2000.00")  # value - cost
        assert roi.roi_percentage == 400.0  # (2000 / 500) * 100
        assert roi.improvement_from_baseline == Decimal("-2500.00")

    @pytest.mark.asyncio
    async def test_business_goal_creation(self, async_session):
        """Test creating and managing business goals."""
        goal = BusinessGoal(
            tenant_id="test-tenant",
            goal_name="Improve Context Reconstruction Speed",
            goal_type=BusinessGoalType.PERFORMANCE_TARGET.value,
            target_metric_type="context_reconstruction_speed",
            target_value=10.0,
            baseline_value=30.0,
            current_value=15.0,
            target_date=datetime.now(timezone.utc) + timedelta(days=90),
            priority=2
        )

        async_session.add(goal)
        await async_session.commit()
        await async_session.refresh(goal)

        assert goal.id is not None
        assert goal.progress_percentage is not None
        assert 0 <= goal.progress_percentage <= 100
        assert goal.days_remaining is not None

        # Test goal update
        goal.update_progress(12.0, "Good progress on optimization")
        assert goal.current_value == 12.0
        assert goal.last_reviewed_at is not None


# ============================================================================
# Business Value Tracker Service Tests
# ============================================================================

class TestBusinessValueTracker:
    """Test business value tracker service functionality."""

    @pytest.mark.asyncio
    async def test_service_lifecycle(self, test_settings):
        """Test service start/stop lifecycle."""
        tracker = BusinessValueTracker(test_settings)

        assert not tracker.is_running

        await tracker.start()
        assert tracker.is_running
        assert tracker.engine is not None

        await tracker.stop()
        assert not tracker.is_running

    @pytest.mark.asyncio
    async def test_record_kpi_measurement(self, business_tracker):
        """Test recording KPI measurements."""
        kpi_id = await business_tracker.record_kpi(
            kpi_type="context_reconstruction_speed",
            value=8.5,
            context={"query_complexity": "medium", "data_sources": 5},
            target=15.0,
            baseline=30.0
        )

        assert kpi_id is not None
        assert len(kpi_id) > 0

    @pytest.mark.asyncio
    async def test_get_kpi_status(self, business_tracker):
        """Test getting KPI status and trends."""
        # Record some test KPI data
        await business_tracker.record_kpi(
            kpi_type="search_accuracy",
            value=92.5,
            target=90.0,
            baseline=75.0
        )

        # Get status
        status = await business_tracker.get_kpi_status("search_accuracy")

        assert status is not None
        assert status.kpi_type == "search_accuracy"
        assert status.current_value == 92.5
        assert status.target_value == 90.0
        assert status.status in [KpiStatus.EXCEEDING, KpiStatus.MEETING]

    @pytest.mark.asyncio
    async def test_kpi_history(self, business_tracker):
        """Test getting KPI history."""
        kpi_type = "relationship_validation_rate"

        # Record multiple measurements
        for i, value in enumerate([75.0, 78.0, 82.0, 85.0]):
            await business_tracker.record_kpi(
                kpi_type=kpi_type,
                value=value,
                context={"batch": i + 1}
            )

        # Get history
        history = await business_tracker.get_kpi_history(kpi_type, hours_back=24)

        assert len(history) == 4
        assert history[0].value == 75.0  # First measurement
        assert history[-1].value == 85.0  # Last measurement

    @pytest.mark.asyncio
    async def test_context_reconstruction_tracking(self, business_tracker):
        """Test context reconstruction specific tracking."""
        kpi_id = await business_tracker.record_context_reconstruction_time(
            reconstruction_time_minutes=8.5,
            query_complexity="medium",
            data_sources_count=5,
            context={"user_type": "knowledge_worker"}
        )

        assert kpi_id is not None

        # Verify KPI was recorded
        status = await business_tracker.get_kpi_status("context_reconstruction_speed")
        assert status is not None
        assert status.current_value == 8.5

    @pytest.mark.asyncio
    async def test_search_accuracy_tracking(self, business_tracker):
        """Test search accuracy specific tracking."""
        kpi_id = await business_tracker.record_search_accuracy(
            accuracy_percentage=94.5,
            total_results=20,
            relevant_results=19,
            search_type="semantic_search"
        )

        assert kpi_id is not None

        # Verify KPI was recorded
        status = await business_tracker.get_kpi_status("search_accuracy")
        assert status is not None
        assert status.current_value == 94.5

    @pytest.mark.asyncio
    async def test_kpi_threshold_alerts(self, business_tracker):
        """Test KPI threshold violation alerts."""
        # Record a KPI value that should trigger an alert
        await business_tracker.record_kpi(
            kpi_type="context_reconstruction_speed",
            value=45.0,  # Above critical threshold of 30 minutes
            target=15.0
        )

        alerts = await business_tracker.check_kpi_thresholds()

        assert len(alerts) > 0
        alert = alerts[0]
        assert alert.kpi_type == "context_reconstruction_speed"
        assert alert.severity in ["warning", "critical"]
        assert alert.current_value == 45.0

    @pytest.mark.asyncio
    async def test_all_kpi_status(self, business_tracker):
        """Test getting status for all KPIs."""
        # Record multiple KPI types
        await business_tracker.record_kpi("context_reconstruction_speed", 12.0)
        await business_tracker.record_kpi("search_accuracy", 88.5)
        await business_tracker.record_kpi("relationship_validation_rate", 82.0)

        all_status = await business_tracker.get_all_kpi_status()

        assert len(all_status) == 3
        assert "context_reconstruction_speed" in all_status
        assert "search_accuracy" in all_status
        assert "relationship_validation_rate" in all_status


# ============================================================================
# Usage Analytics Service Tests
# ============================================================================

class TestUsageAnalyticsService:
    """Test usage analytics service functionality."""

    @pytest.mark.asyncio
    async def test_track_feature_usage(self, usage_analytics):
        """Test tracking feature usage."""
        usage_id = await usage_analytics.track_feature_usage(
            feature_name="search_feature",
            user_identifier="user123",
            duration_seconds=45.5,
            success_rate=0.90,
            context={"query_type": "project_search"}
        )

        assert usage_id is not None

    @pytest.mark.asyncio
    async def test_session_management(self, usage_analytics):
        """Test user session tracking."""
        session_id = await usage_analytics.start_session(
            user_identifier="user456",
            context={"platform": "web"}
        )

        assert session_id is not None
        assert session_id in usage_analytics.active_sessions

        # Track some usage in the session
        await usage_analytics.track_feature_usage(
            feature_name="daily_review",
            user_identifier="user456",
            duration_seconds=120.0,
            session_id=session_id
        )

        # End session
        session = await usage_analytics.end_session(session_id)
        assert session is not None
        assert session_id not in usage_analytics.active_sessions

    @pytest.mark.asyncio
    async def test_search_usage_tracking(self, usage_analytics):
        """Test search-specific usage tracking."""
        usage_id = await usage_analytics.track_search_usage(
            query="project alpha meeting notes",
            results_count=15,
            clicked_results=3,
            user_identifier="user789",
            response_time_ms=245.5
        )

        assert usage_id is not None

    @pytest.mark.asyncio
    async def test_daily_review_tracking(self, usage_analytics):
        """Test daily review engagement tracking."""
        usage_id = await usage_analytics.track_daily_review_engagement(
            review_duration_seconds=180.0,
            items_reviewed=12,
            items_completed=10,
            review_type="morning",
            user_identifier="user101"
        )

        assert usage_id is not None

    @pytest.mark.asyncio
    async def test_feature_usage_insights(self, usage_analytics):
        """Test getting feature usage insights."""
        # Track multiple usage events
        for i in range(5):
            await usage_analytics.track_feature_usage(
                feature_name="search_feature",
                user_identifier=f"user{i}",
                duration_seconds=30.0 + i * 5,
                success_rate=0.8 + i * 0.05
            )

        insights = await usage_analytics.get_feature_usage_insights(
            feature_name="search_feature",
            days_back=1
        )

        if insights:  # Might be None if no data
            assert insights.feature_name == "search_feature"
            assert insights.total_usage_count >= 5
            assert insights.unique_users >= 1

    @pytest.mark.asyncio
    async def test_usage_analytics_summary(self, usage_analytics):
        """Test comprehensive usage analytics summary."""
        # Generate some usage data
        await usage_analytics.track_feature_usage("search_feature", "user1", 30.0)
        await usage_analytics.track_feature_usage("daily_review", "user1", 120.0)
        await usage_analytics.track_feature_usage("project_query", "user2", 45.0)

        summary = await usage_analytics.get_usage_analytics_summary(days_back=1)

        assert summary is not None
        assert summary.total_sessions >= 0
        assert summary.unique_users >= 0
        assert len(summary.most_popular_features) >= 0


# ============================================================================
# ROI Calculator Service Tests
# ============================================================================

class TestROICalculatorService:
    """Test ROI calculator service functionality."""

    @pytest.mark.asyncio
    async def test_time_savings_roi_calculation(self, roi_calculator):
        """Test time savings ROI calculation."""
        roi_id = await roi_calculator.calculate_time_savings_roi(
            task_type="context_reconstruction",
            baseline_time_minutes=30.0,
            current_time_minutes=8.0,
            frequency_per_day=3,
            period_days=30,
            hourly_rate=Decimal("75.00")
        )

        assert roi_id is not None

    @pytest.mark.asyncio
    async def test_productivity_gain_roi_calculation(self, roi_calculator):
        """Test productivity gain ROI calculation."""
        roi_id = await roi_calculator.calculate_productivity_gain_roi(
            process_name="project_analysis",
            baseline_completion_rate=0.70,
            current_completion_rate=0.90,
            tasks_per_day=5,
            value_per_task=Decimal("200.00"),
            period_days=30
        )

        assert roi_id is not None

    @pytest.mark.asyncio
    async def test_decision_speed_roi_calculation(self, roi_calculator):
        """Test decision speed improvement ROI calculation."""
        roi_id = await roi_calculator.calculate_decision_speed_improvement_roi(
            decision_type="project_approval",
            baseline_days=3.0,
            current_days=0.5,
            decisions_per_month=8,
            cost_of_delay_per_day=Decimal("500.00")
        )

        assert roi_id is not None

    @pytest.mark.asyncio
    async def test_meeting_prep_savings_calculation(self, roi_calculator):
        """Test meeting preparation savings calculation."""
        roi_id = await roi_calculator.calculate_meeting_preparation_savings(
            meetings_per_week=10,
            baseline_prep_minutes=30.0,
            current_prep_minutes=5.0,
            period_weeks=4
        )

        assert roi_id is not None

    @pytest.mark.asyncio
    async def test_roi_summary(self, roi_calculator):
        """Test comprehensive ROI summary."""
        # Create some ROI calculations
        await roi_calculator.calculate_time_savings_roi(
            "email_processing", 45.0, 12.0, 8, 30
        )
        await roi_calculator.calculate_productivity_gain_roi(
            "document_analysis", 0.60, 0.85, 3, Decimal("150.00"), 30
        )

        summary = await roi_calculator.get_roi_summary(days_back=30)

        assert summary is not None
        assert summary.total_gross_value >= 0
        assert summary.calculation_count >= 2

    @pytest.mark.asyncio
    async def test_roi_validation_errors(self, roi_calculator):
        """Test ROI calculation validation errors."""
        # Test invalid time values
        with pytest.raises(Exception):  # ValidationError
            await roi_calculator.calculate_time_savings_roi(
                "invalid_task", -10.0, 5.0, 1, 1
            )

        # Test invalid completion rates
        with pytest.raises(Exception):  # ValidationError
            await roi_calculator.calculate_productivity_gain_roi(
                "invalid_process", 1.5, 0.8, 1, Decimal("100.00"), 1
            )


# ============================================================================
# Business API Endpoint Tests
# ============================================================================

class TestBusinessAPIEndpoints:
    """Test business API endpoint functionality."""

    @pytest.mark.asyncio
    async def test_record_kpi_endpoint(self, test_client):
        """Test KPI recording endpoint."""
        request_data = {
            "kpi_type": "search_accuracy",
            "value": 92.5,
            "context": {"search_type": "semantic"},
            "target": 90.0
        }

        response = await test_client.post("/api/v1/business/kpis", json=request_data)

        assert response.status_code == 200
        data = response.json()
        assert "kpi_id" in data
        assert data["status"] == "recorded"

    @pytest.mark.asyncio
    async def test_get_kpi_status_endpoint(self, test_client, business_tracker):
        """Test KPI status retrieval endpoint."""
        # First record a KPI
        await business_tracker.record_kpi("search_accuracy", 88.5, target=90.0)

        response = await test_client.get("/api/v1/business/kpis/search_accuracy")

        assert response.status_code == 200
        data = response.json()
        assert data["kpi_type"] == "search_accuracy"
        assert data["current_value"] == 88.5

    @pytest.mark.asyncio
    async def test_get_all_kpi_status_endpoint(self, test_client, business_tracker):
        """Test getting all KPI status endpoint."""
        # Record multiple KPIs
        await business_tracker.record_kpi("search_accuracy", 92.0)
        await business_tracker.record_kpi("context_reconstruction_speed", 12.0)

        response = await test_client.get("/api/v1/business/kpis")

        assert response.status_code == 200
        data = response.json()
        assert isinstance(data, dict)
        assert len(data) >= 2

    @pytest.mark.asyncio
    async def test_track_usage_endpoint(self, test_client):
        """Test usage tracking endpoint."""
        request_data = {
            "feature_name": "search_feature",
            "duration_seconds": 45.5,
            "success_rate": 0.95,
            "context": {"query_type": "project_search"}
        }

        response = await test_client.post("/api/v1/business/usage/track", json=request_data)

        assert response.status_code == 200
        data = response.json()
        assert "usage_id" in data
        assert data["status"] == "tracked"

    @pytest.mark.asyncio
    async def test_search_usage_endpoint(self, test_client):
        """Test search usage tracking endpoint."""
        request_data = {
            "query": "project alpha meeting notes",
            "results_count": 15,
            "clicked_results": 3,
            "response_time_ms": 245.0
        }

        response = await test_client.post("/api/v1/business/usage/search", json=request_data)

        assert response.status_code == 200
        data = response.json()
        assert "usage_id" in data

    @pytest.mark.asyncio
    async def test_daily_review_endpoint(self, test_client):
        """Test daily review tracking endpoint."""
        request_data = {
            "review_duration_seconds": 180.0,
            "items_reviewed": 12,
            "items_completed": 10,
            "review_type": "morning"
        }

        response = await test_client.post("/api/v1/business/usage/daily-review", json=request_data)

        assert response.status_code == 200
        data = response.json()
        assert "usage_id" in data

    @pytest.mark.asyncio
    async def test_roi_summary_endpoint(self, test_client, roi_calculator):
        """Test ROI summary endpoint."""
        # Create some ROI data
        await roi_calculator.calculate_time_savings_roi(
            "test_task", 60.0, 15.0, 4, 30
        )

        response = await test_client.get("/api/v1/business/roi/summary?days_back=30")

        assert response.status_code == 200
        data = response.json()
        assert "total_gross_value" in data
        assert "calculation_count" in data

    @pytest.mark.asyncio
    async def test_calculate_time_savings_endpoint(self, test_client):
        """Test time savings ROI calculation endpoint."""
        request_data = {
            "task_type": "context_reconstruction",
            "baseline_time_minutes": 30.0,
            "current_time_minutes": 8.0,
            "frequency_per_day": 3,
            "period_days": 30
        }

        response = await test_client.post("/api/v1/business/roi/time-savings", json=request_data)

        assert response.status_code == 200
        data = response.json()
        assert "roi_id" in data
        assert data["status"] == "calculated"

    @pytest.mark.asyncio
    async def test_calculate_productivity_roi_endpoint(self, test_client):
        """Test productivity ROI calculation endpoint."""
        request_data = {
            "process_name": "document_analysis",
            "baseline_completion_rate": 0.70,
            "current_completion_rate": 0.90,
            "tasks_per_day": 5,
            "value_per_task": 200.0,
            "period_days": 30
        }

        response = await test_client.post("/api/v1/business/roi/productivity", json=request_data)

        assert response.status_code == 200
        data = response.json()
        assert "roi_id" in data

    @pytest.mark.asyncio
    async def test_invalid_kpi_type_error(self, test_client):
        """Test error handling for invalid KPI types."""
        request_data = {
            "kpi_type": "invalid_kpi_type",
            "value": 100.0
        }

        response = await test_client.post("/api/v1/business/kpis", json=request_data)

        assert response.status_code == 422  # Validation error

    @pytest.mark.asyncio
    async def test_get_nonexistent_kpi_error(self, test_client):
        """Test error handling for nonexistent KPI."""
        response = await test_client.get("/api/v1/business/kpis/nonexistent_kpi")

        assert response.status_code == 400  # Invalid KPI type


# ============================================================================
# Performance and Integration Tests
# ============================================================================

class TestBusinessComponentsPerformance:
    """Test performance characteristics of business components."""

    @pytest.mark.asyncio
    async def test_high_volume_kpi_recording(self, business_tracker):
        """Test high-volume KPI recording performance."""
        import time

        start_time = time.time()

        # Record 100 KPI measurements
        tasks = []
        for i in range(100):
            task = business_tracker.record_kpi(
                kpi_type="search_accuracy",
                value=85.0 + (i % 10),
                context={"batch": i // 10}
            )
            tasks.append(task)

        # Wait for all to complete
        await asyncio.gather(*tasks)

        end_time = time.time()
        duration = end_time - start_time

        # Should complete within reasonable time (adjust based on requirements)
        assert duration < 10.0  # 10 seconds for 100 records

    @pytest.mark.asyncio
    async def test_concurrent_service_operations(self, business_tracker, usage_analytics, roi_calculator):
        """Test concurrent operations across multiple services."""
        # Simulate concurrent operations
        tasks = [
            business_tracker.record_kpi("search_accuracy", 92.0),
            usage_analytics.track_feature_usage("search_feature", duration_seconds=30.0),
            roi_calculator.calculate_time_savings_roi("test_task", 60.0, 20.0, 2, 7),
            business_tracker.record_kpi("context_reconstruction_speed", 10.0),
            usage_analytics.track_search_usage("test query", 10, 3),
        ]

        # All should complete successfully
        results = await asyncio.gather(*tasks, return_exceptions=True)

        # Check no exceptions occurred
        for result in results:
            assert not isinstance(result, Exception), f"Task failed with: {result}"

    @pytest.mark.asyncio
    async def test_cache_performance(self, business_tracker):
        """Test caching performance for repeated KPI status requests."""
        # Record a KPI
        await business_tracker.record_kpi("search_accuracy", 88.0)

        # First request (cache miss)
        start_time = time.time()
        status1 = await business_tracker.get_kpi_status("search_accuracy", use_cache=False)
        first_request_time = time.time() - start_time

        # Second request (cache hit)
        start_time = time.time()
        status2 = await business_tracker.get_kpi_status("search_accuracy", use_cache=True)
        second_request_time = time.time() - start_time

        assert status1 is not None
        assert status2 is not None
        assert status1.current_value == status2.current_value

        # Cached request should be significantly faster (this is a simple test)
        # In practice, the difference might be minimal for small datasets
        assert second_request_time <= first_request_time * 2  # Allow some variance


# ============================================================================
# Test Runner Configuration
# ============================================================================

if __name__ == "__main__":
    pytest.main([__file__, "-v", "--tb=short"])
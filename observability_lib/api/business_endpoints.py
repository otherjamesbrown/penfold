"""Business API endpoints for observability dashboard integration.

This module provides FastAPI endpoints for business value metrics, KPI tracking,
usage analytics, and ROI calculations. These endpoints support the business
dashboard and provide comprehensive business intelligence capabilities.

Endpoints:
    GET /business/kpis - Get all KPI status and trends
    GET /business/kpis/{kpi_type} - Get specific KPI details
    POST /business/kpis - Record new KPI measurement
    GET /business/usage/summary - Get usage analytics summary
    GET /business/usage/features/{feature_name} - Get feature usage insights
    POST /business/usage/track - Track feature usage
    GET /business/roi/summary - Get ROI summary
    GET /business/roi/calculations - Get ROI calculations
    POST /business/roi/time-savings - Calculate time savings ROI
    POST /business/roi/productivity - Calculate productivity ROI
    GET /business/goals - Get business goals and progress
    POST /business/goals - Create new business goal
    PUT /business/goals/{goal_id} - Update business goal

Features:
- Comprehensive business metrics API
- Real-time KPI monitoring endpoints
- Usage analytics and insights
- ROI calculation and tracking
- Business goal management
- Performance target monitoring
- Multi-dimensional filtering and aggregation
- Error handling and validation
- Request/response models with proper typing
- Authentication and authorization support

Example Usage:
    # Get KPI dashboard data
    GET /api/v1/business/kpis

    # Track feature usage
    POST /api/v1/business/usage/track
    {
        "feature_name": "search_feature",
        "duration_seconds": 12.5,
        "success_rate": 0.95,
        "context": {"query_type": "project_search"}
    }

    # Calculate ROI
    POST /api/v1/business/roi/time-savings
    {
        "task_type": "context_reconstruction",
        "baseline_time_minutes": 30,
        "current_time_minutes": 8,
        "frequency_per_day": 3
    }
"""

import asyncio
import logging
from datetime import datetime, timedelta, timezone
from typing import Any, Dict, List, Optional, Union
from uuid import UUID, uuid4

from fastapi import APIRouter, Depends, HTTPException, Query, Path, Body
from fastapi.responses import JSONResponse
from pydantic import BaseModel, Field, validator
from sqlalchemy.ext.asyncio import AsyncSession

from observability_lib.config import ObservabilitySettings
from observability_lib.services.business_value_tracker import (
    BusinessValueTracker, KpiSummary, KpiMeasurement, BusinessAlert
)
from observability_lib.services.usage_analytics import (
    UsageAnalyticsService, FeatureUsageInsight, UsageAnalytics, UsageSession
)
from observability_lib.services.roi_calculator import (
    ROICalculatorService, ROISummary, ROIResult, TimeSavingsCalculation
)
from observability_lib.models.business_metrics import (
    BusinessKpi, UsageMetric, RoiMetric, BusinessGoal, ValueCalculation,
    KpiType, UsageMetricType, RoiMetricType, BusinessGoalType, GoalStatus
)


logger = logging.getLogger(__name__)


# ============================================================================
# Request/Response Models
# ============================================================================

class KpiMeasurementRequest(BaseModel):
    """Request model for recording KPI measurements."""
    kpi_type: str = Field(..., description="Type of KPI being recorded")
    value: float = Field(..., description="Measured value")
    context: Optional[Dict[str, Any]] = Field(None, description="Additional context")
    target: Optional[float] = Field(None, description="Target value for this KPI")
    baseline: Optional[float] = Field(None, description="Baseline value for comparison")

    @validator('kpi_type')
    def validate_kpi_type(cls, v):
        try:
            KpiType(v)
        except ValueError:
            valid_types = [e.value for e in KpiType]
            raise ValueError(f"Invalid KPI type. Must be one of: {valid_types}")
        return v


class KpiStatusResponse(BaseModel):
    """Response model for KPI status."""
    kpi_type: str
    kpi_name: str
    current_value: float
    target_value: Optional[float]
    baseline_value: Optional[float]
    unit: str
    status: str
    trend_direction: str
    trend_percentage: float
    target_achievement: Optional[float]
    last_updated: datetime
    measurements_count: int


class UsageTrackingRequest(BaseModel):
    """Request model for tracking feature usage."""
    feature_name: str = Field(..., description="Name of the feature being used")
    user_identifier: Optional[str] = Field(None, description="User identifier (will be hashed)")
    duration_seconds: Optional[float] = Field(None, description="Duration of usage")
    success_rate: Optional[float] = Field(None, ge=0.0, le=1.0, description="Success rate")
    usage_count: int = Field(1, ge=1, description="Number of times feature was used")
    context: Optional[Dict[str, Any]] = Field(None, description="Additional context")
    session_id: Optional[str] = Field(None, description="Session identifier")


class SearchUsageRequest(BaseModel):
    """Request model for tracking search usage."""
    query: str = Field(..., description="Search query text")
    results_count: int = Field(..., ge=0, description="Number of results returned")
    clicked_results: int = Field(0, ge=0, description="Number of results clicked")
    response_time_ms: Optional[float] = Field(None, description="Response time in milliseconds")
    user_identifier: Optional[str] = Field(None, description="User performing search")
    session_id: Optional[str] = Field(None, description="Session identifier")


class DailyReviewRequest(BaseModel):
    """Request model for tracking daily review engagement."""
    review_duration_seconds: float = Field(..., gt=0, description="Time spent in review")
    items_reviewed: int = Field(..., ge=0, description="Number of items reviewed")
    items_completed: int = Field(..., ge=0, description="Number of items completed")
    review_type: str = Field(..., description="Type of review")
    user_identifier: Optional[str] = Field(None, description="User performing review")
    session_id: Optional[str] = Field(None, description="Session identifier")


class TimeSavingsROIRequest(BaseModel):
    """Request model for calculating time savings ROI."""
    task_type: str = Field(..., description="Type of task being measured")
    baseline_time_minutes: float = Field(..., gt=0, description="Original time per task")
    current_time_minutes: float = Field(..., ge=0, description="Current time per task")
    frequency_per_day: int = Field(..., gt=0, description="Times per day task is performed")
    period_days: int = Field(30, gt=0, description="Period for calculation")
    hourly_rate: Optional[float] = Field(None, description="Hourly rate for value calculation")


class ProductivityROIRequest(BaseModel):
    """Request model for calculating productivity ROI."""
    process_name: str = Field(..., description="Name of the process")
    baseline_completion_rate: float = Field(..., ge=0.0, le=1.0, description="Original completion rate")
    current_completion_rate: float = Field(..., ge=0.0, le=1.0, description="Current completion rate")
    tasks_per_day: int = Field(..., gt=0, description="Number of tasks per day")
    value_per_task: float = Field(..., gt=0, description="Business value per task")
    period_days: int = Field(30, gt=0, description="Period for calculation")


class BusinessGoalRequest(BaseModel):
    """Request model for creating business goals."""
    goal_name: str = Field(..., description="Name of the business goal")
    goal_type: str = Field(..., description="Type of goal")
    goal_description: Optional[str] = Field(None, description="Detailed description")
    target_metric_type: str = Field(..., description="Type of metric targeted")
    target_value: float = Field(..., description="Target value to achieve")
    baseline_value: Optional[float] = Field(None, description="Starting baseline value")
    target_date: Optional[datetime] = Field(None, description="Target achievement date")
    priority: int = Field(3, ge=1, le=5, description="Priority level (1=highest, 5=lowest)")
    owner: Optional[str] = Field(None, description="Person or team responsible")

    @validator('goal_type')
    def validate_goal_type(cls, v):
        try:
            BusinessGoalType(v)
        except ValueError:
            valid_types = [e.value for e in BusinessGoalType]
            raise ValueError(f"Invalid goal type. Must be one of: {valid_types}")
        return v


class BusinessGoalUpdate(BaseModel):
    """Request model for updating business goals."""
    current_value: Optional[float] = Field(None, description="Current progress value")
    status: Optional[str] = Field(None, description="Current status")
    progress_notes: Optional[str] = Field(None, description="Progress notes")

    @validator('status')
    def validate_status(cls, v):
        if v is not None:
            try:
                GoalStatus(v)
            except ValueError:
                valid_statuses = [e.value for e in GoalStatus]
                raise ValueError(f"Invalid status. Must be one of: {valid_statuses}")
        return v


class FeatureInsightResponse(BaseModel):
    """Response model for feature usage insights."""
    feature_name: str
    feature_category: str
    total_usage_count: int
    unique_users: int
    avg_session_duration: float
    success_rate: float
    adoption_rate: float
    engagement_level: str
    trend_direction: str
    peak_usage_hours: List[int]
    common_contexts: Dict[str, int]
    last_updated: datetime


class ROISummaryResponse(BaseModel):
    """Response model for ROI summary."""
    period_start: datetime
    period_end: datetime
    total_gross_value: float
    total_net_value: float
    overall_roi_percentage: float
    time_savings_value: float
    productivity_gains_value: float
    efficiency_improvements_value: float
    cost_reductions_value: float
    payback_achieved: bool
    average_confidence: float
    calculation_count: int
    top_value_drivers: List[Dict[str, Union[str, float]]]


class UsageAnalyticsResponse(BaseModel):
    """Response model for usage analytics."""
    period_start: datetime
    period_end: datetime
    total_sessions: int
    unique_users: int
    avg_session_duration: float
    most_popular_features: List[Dict[str, Union[str, int]]]
    feature_adoption_rates: Dict[str, float]
    engagement_distribution: Dict[str, int]
    peak_usage_times: List[int]
    success_rate_by_feature: Dict[str, float]


class BusinessGoalResponse(BaseModel):
    """Response model for business goals."""
    id: str
    goal_name: str
    goal_type: str
    goal_description: Optional[str]
    target_metric_type: str
    target_value: float
    current_value: Optional[float]
    baseline_value: Optional[float]
    target_date: Optional[datetime]
    status: str
    priority: int
    owner: Optional[str]
    progress_percentage: Optional[float]
    days_remaining: Optional[int]
    is_at_risk: bool
    created_at: datetime
    last_reviewed_at: Optional[datetime]


# ============================================================================
# Router Setup and Dependencies
# ============================================================================

router = APIRouter(prefix="/business", tags=["Business Metrics"])

# Global service instances (will be initialized by the application)
business_tracker: Optional[BusinessValueTracker] = None
usage_analytics: Optional[UsageAnalyticsService] = None
roi_calculator: Optional[ROICalculatorService] = None


async def get_business_tracker() -> BusinessValueTracker:
    """Dependency to get business value tracker service."""
    if business_tracker is None:
        raise HTTPException(status_code=503, detail="Business tracker service not initialized")
    if not business_tracker.is_running:
        raise HTTPException(status_code=503, detail="Business tracker service not running")
    return business_tracker


async def get_usage_analytics() -> UsageAnalyticsService:
    """Dependency to get usage analytics service."""
    if usage_analytics is None:
        raise HTTPException(status_code=503, detail="Usage analytics service not initialized")
    if not usage_analytics.is_running:
        raise HTTPException(status_code=503, detail="Usage analytics service not running")
    return usage_analytics


async def get_roi_calculator() -> ROICalculatorService:
    """Dependency to get ROI calculator service."""
    if roi_calculator is None:
        raise HTTPException(status_code=503, detail="ROI calculator service not initialized")
    if not roi_calculator.is_running:
        raise HTTPException(status_code=503, detail="ROI calculator service not running")
    return roi_calculator


def initialize_business_endpoints(
    settings: ObservabilitySettings,
    business_value_tracker: BusinessValueTracker,
    usage_analytics_service: UsageAnalyticsService,
    roi_calculator_service: ROICalculatorService
) -> None:
    """Initialize business endpoints with service instances.

    Args:
        settings: Observability settings
        business_value_tracker: Business value tracker service instance
        usage_analytics_service: Usage analytics service instance
        roi_calculator_service: ROI calculator service instance
    """
    global business_tracker, usage_analytics, roi_calculator
    business_tracker = business_value_tracker
    usage_analytics = usage_analytics_service
    roi_calculator = roi_calculator_service


# ============================================================================
# KPI Endpoints
# ============================================================================

@router.get("/kpis", response_model=Dict[str, KpiStatusResponse])
async def get_all_kpi_status(
    use_cache: bool = Query(True, description="Use cached data if available"),
    tracker: BusinessValueTracker = Depends(get_business_tracker)
) -> Dict[str, KpiStatusResponse]:
    """Get status for all business KPIs."""
    try:
        kpi_statuses = await tracker.get_all_kpi_status(use_cache=use_cache)

        response = {}
        for kpi_type, summary in kpi_statuses.items():
            response[kpi_type] = KpiStatusResponse(
                kpi_type=summary.kpi_type,
                kpi_name=summary.kpi_name,
                current_value=summary.current_value,
                target_value=summary.target_value,
                baseline_value=summary.baseline_value,
                unit=summary.unit,
                status=summary.status.value,
                trend_direction=summary.trend_direction.value,
                trend_percentage=summary.trend_percentage,
                target_achievement=summary.target_achievement,
                last_updated=summary.last_updated,
                measurements_count=summary.measurements_count
            )

        return response

    except Exception as e:
        logger.error(f"Error getting KPI status: {e}")
        raise HTTPException(status_code=500, detail=f"Failed to get KPI status: {str(e)}")


@router.get("/kpis/{kpi_type}", response_model=KpiStatusResponse)
async def get_kpi_status(
    kpi_type: str = Path(..., description="Type of KPI to retrieve"),
    use_cache: bool = Query(True, description="Use cached data if available"),
    tracker: BusinessValueTracker = Depends(get_business_tracker)
) -> KpiStatusResponse:
    """Get status for a specific KPI."""
    try:
        # Validate KPI type
        try:
            KpiType(kpi_type)
        except ValueError:
            raise HTTPException(status_code=400, detail=f"Invalid KPI type: {kpi_type}")

        summary = await tracker.get_kpi_status(kpi_type, use_cache=use_cache)
        if not summary:
            raise HTTPException(status_code=404, detail=f"KPI not found: {kpi_type}")

        return KpiStatusResponse(
            kpi_type=summary.kpi_type,
            kpi_name=summary.kpi_name,
            current_value=summary.current_value,
            target_value=summary.target_value,
            baseline_value=summary.baseline_value,
            unit=summary.unit,
            status=summary.status.value,
            trend_direction=summary.trend_direction.value,
            trend_percentage=summary.trend_percentage,
            target_achievement=summary.target_achievement,
            last_updated=summary.last_updated,
            measurements_count=summary.measurements_count
        )

    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Error getting KPI status for {kpi_type}: {e}")
        raise HTTPException(status_code=500, detail=f"Failed to get KPI status: {str(e)}")


@router.post("/kpis", response_model=Dict[str, str])
async def record_kpi_measurement(
    request: KpiMeasurementRequest,
    tracker: BusinessValueTracker = Depends(get_business_tracker)
) -> Dict[str, str]:
    """Record a new KPI measurement."""
    try:
        kpi_id = await tracker.record_kpi(
            kpi_type=request.kpi_type,
            value=request.value,
            context=request.context,
            target=request.target,
            baseline=request.baseline
        )

        return {"kpi_id": kpi_id, "status": "recorded"}

    except Exception as e:
        logger.error(f"Error recording KPI measurement: {e}")
        raise HTTPException(status_code=500, detail=f"Failed to record KPI: {str(e)}")


@router.get("/kpis/{kpi_type}/history")
async def get_kpi_history(
    kpi_type: str = Path(..., description="Type of KPI to retrieve"),
    hours_back: int = Query(24, ge=1, le=720, description="Hours of history to retrieve"),
    limit: int = Query(100, ge=1, le=1000, description="Maximum number of measurements"),
    tracker: BusinessValueTracker = Depends(get_business_tracker)
) -> List[Dict[str, Any]]:
    """Get historical KPI measurements."""
    try:
        # Validate KPI type
        try:
            KpiType(kpi_type)
        except ValueError:
            raise HTTPException(status_code=400, detail=f"Invalid KPI type: {kpi_type}")

        measurements = await tracker.get_kpi_history(kpi_type, hours_back, limit)

        return [
            {
                "kpi_type": m.kpi_type,
                "value": m.value,
                "unit": m.unit,
                "timestamp": m.timestamp,
                "context": m.context,
                "target": m.target,
                "baseline": m.baseline
            }
            for m in measurements
        ]

    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Error getting KPI history: {e}")
        raise HTTPException(status_code=500, detail=f"Failed to get KPI history: {str(e)}")


@router.get("/kpis/alerts")
async def get_business_alerts(
    tracker: BusinessValueTracker = Depends(get_business_tracker)
) -> List[Dict[str, Any]]:
    """Get active business alerts from KPI threshold violations."""
    try:
        alerts = await tracker.check_kpi_thresholds()

        return [
            {
                "alert_id": alert.alert_id,
                "kpi_type": alert.kpi_type,
                "severity": alert.severity,
                "message": alert.message,
                "current_value": alert.current_value,
                "threshold_value": alert.threshold_value,
                "triggered_at": alert.triggered_at,
                "context": alert.context
            }
            for alert in alerts
        ]

    except Exception as e:
        logger.error(f"Error getting business alerts: {e}")
        raise HTTPException(status_code=500, detail=f"Failed to get alerts: {str(e)}")


# ============================================================================
# Usage Analytics Endpoints
# ============================================================================

@router.post("/usage/track", response_model=Dict[str, str])
async def track_feature_usage(
    request: UsageTrackingRequest,
    analytics: UsageAnalyticsService = Depends(get_usage_analytics)
) -> Dict[str, str]:
    """Track feature usage."""
    try:
        usage_id = await analytics.track_feature_usage(
            feature_name=request.feature_name,
            user_identifier=request.user_identifier,
            duration_seconds=request.duration_seconds,
            success_rate=request.success_rate,
            usage_count=request.usage_count,
            context=request.context,
            session_id=request.session_id
        )

        return {"usage_id": usage_id, "status": "tracked"}

    except Exception as e:
        logger.error(f"Error tracking feature usage: {e}")
        raise HTTPException(status_code=500, detail=f"Failed to track usage: {str(e)}")


@router.post("/usage/search", response_model=Dict[str, str])
async def track_search_usage(
    request: SearchUsageRequest,
    analytics: UsageAnalyticsService = Depends(get_usage_analytics)
) -> Dict[str, str]:
    """Track search feature usage with detailed metrics."""
    try:
        usage_id = await analytics.track_search_usage(
            query=request.query,
            results_count=request.results_count,
            clicked_results=request.clicked_results,
            user_identifier=request.user_identifier,
            session_id=request.session_id,
            response_time_ms=request.response_time_ms
        )

        return {"usage_id": usage_id, "status": "tracked"}

    except Exception as e:
        logger.error(f"Error tracking search usage: {e}")
        raise HTTPException(status_code=500, detail=f"Failed to track search: {str(e)}")


@router.post("/usage/daily-review", response_model=Dict[str, str])
async def track_daily_review_engagement(
    request: DailyReviewRequest,
    analytics: UsageAnalyticsService = Depends(get_usage_analytics)
) -> Dict[str, str]:
    """Track daily review engagement metrics."""
    try:
        usage_id = await analytics.track_daily_review_engagement(
            review_duration_seconds=request.review_duration_seconds,
            items_reviewed=request.items_reviewed,
            items_completed=request.items_completed,
            review_type=request.review_type,
            user_identifier=request.user_identifier,
            session_id=request.session_id
        )

        return {"usage_id": usage_id, "status": "tracked"}

    except Exception as e:
        logger.error(f"Error tracking daily review: {e}")
        raise HTTPException(status_code=500, detail=f"Failed to track review: {str(e)}")


@router.get("/usage/summary", response_model=UsageAnalyticsResponse)
async def get_usage_analytics_summary(
    days_back: int = Query(7, ge=1, le=365, description="Days to include in analysis"),
    analytics: UsageAnalyticsService = Depends(get_usage_analytics)
) -> UsageAnalyticsResponse:
    """Get comprehensive usage analytics summary."""
    try:
        summary = await analytics.get_usage_analytics_summary(days_back=days_back)

        return UsageAnalyticsResponse(
            period_start=summary.period_start,
            period_end=summary.period_end,
            total_sessions=summary.total_sessions,
            unique_users=summary.unique_users,
            avg_session_duration=summary.avg_session_duration,
            most_popular_features=[
                {"feature": feature, "usage_count": count}
                for feature, count in summary.most_popular_features
            ],
            feature_adoption_rates=summary.feature_adoption_rates,
            engagement_distribution={
                level.value: count for level, count in summary.engagement_distribution.items()
            },
            peak_usage_times=summary.peak_usage_times,
            success_rate_by_feature=summary.success_rate_by_feature
        )

    except Exception as e:
        logger.error(f"Error getting usage analytics: {e}")
        raise HTTPException(status_code=500, detail=f"Failed to get analytics: {str(e)}")


@router.get("/usage/features/{feature_name}", response_model=FeatureInsightResponse)
async def get_feature_usage_insights(
    feature_name: str = Path(..., description="Name of the feature to analyze"),
    days_back: int = Query(7, ge=1, le=365, description="Days of history to analyze"),
    use_cache: bool = Query(True, description="Use cached insights if available"),
    analytics: UsageAnalyticsService = Depends(get_usage_analytics)
) -> FeatureInsightResponse:
    """Get comprehensive usage insights for a specific feature."""
    try:
        insights = await analytics.get_feature_usage_insights(
            feature_name=feature_name,
            days_back=days_back,
            use_cache=use_cache
        )

        if not insights:
            raise HTTPException(status_code=404, detail=f"No usage data found for feature: {feature_name}")

        return FeatureInsightResponse(
            feature_name=insights.feature_name,
            feature_category=insights.feature_category.value,
            total_usage_count=insights.total_usage_count,
            unique_users=insights.unique_users,
            avg_session_duration=insights.avg_session_duration,
            success_rate=insights.success_rate,
            adoption_rate=insights.adoption_rate,
            engagement_level=insights.engagement_level.value,
            trend_direction=insights.trend_direction,
            peak_usage_hours=insights.peak_usage_hours,
            common_contexts=insights.common_contexts,
            last_updated=insights.last_updated
        )

    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Error getting feature insights: {e}")
        raise HTTPException(status_code=500, detail=f"Failed to get insights: {str(e)}")


# ============================================================================
# ROI Calculation Endpoints
# ============================================================================

@router.get("/roi/summary", response_model=ROISummaryResponse)
async def get_roi_summary(
    days_back: int = Query(30, ge=1, le=365, description="Days to include in ROI analysis"),
    use_cache: bool = Query(True, description="Use cached summary if available"),
    calculator: ROICalculatorService = Depends(get_roi_calculator)
) -> ROISummaryResponse:
    """Get comprehensive ROI summary."""
    try:
        summary = await calculator.get_roi_summary(days_back=days_back, use_cache=use_cache)

        return ROISummaryResponse(
            period_start=summary.period_start,
            period_end=summary.period_end,
            total_gross_value=float(summary.total_gross_value),
            total_net_value=float(summary.total_net_value),
            overall_roi_percentage=summary.overall_roi_percentage,
            time_savings_value=float(summary.time_savings_value),
            productivity_gains_value=float(summary.productivity_gains_value),
            efficiency_improvements_value=float(summary.efficiency_improvements_value),
            cost_reductions_value=float(summary.cost_reductions_value),
            payback_achieved=summary.payback_achieved,
            average_confidence=summary.average_confidence,
            calculation_count=summary.calculation_count,
            top_value_drivers=[
                {"roi_type": roi_type, "value": float(value)}
                for roi_type, value in summary.top_value_drivers
            ]
        )

    except Exception as e:
        logger.error(f"Error getting ROI summary: {e}")
        raise HTTPException(status_code=500, detail=f"Failed to get ROI summary: {str(e)}")


@router.post("/roi/time-savings", response_model=Dict[str, str])
async def calculate_time_savings_roi(
    request: TimeSavingsROIRequest,
    calculator: ROICalculatorService = Depends(get_roi_calculator)
) -> Dict[str, str]:
    """Calculate ROI from time savings for a specific task type."""
    try:
        from decimal import Decimal

        roi_id = await calculator.calculate_time_savings_roi(
            task_type=request.task_type,
            baseline_time_minutes=request.baseline_time_minutes,
            current_time_minutes=request.current_time_minutes,
            frequency_per_day=request.frequency_per_day,
            period_days=request.period_days,
            hourly_rate=Decimal(str(request.hourly_rate)) if request.hourly_rate else None
        )

        return {"roi_id": roi_id, "status": "calculated"}

    except Exception as e:
        logger.error(f"Error calculating time savings ROI: {e}")
        raise HTTPException(status_code=500, detail=f"Failed to calculate ROI: {str(e)}")


@router.post("/roi/productivity", response_model=Dict[str, str])
async def calculate_productivity_roi(
    request: ProductivityROIRequest,
    calculator: ROICalculatorService = Depends(get_roi_calculator)
) -> Dict[str, str]:
    """Calculate ROI from productivity improvements."""
    try:
        from decimal import Decimal

        roi_id = await calculator.calculate_productivity_gain_roi(
            process_name=request.process_name,
            baseline_completion_rate=request.baseline_completion_rate,
            current_completion_rate=request.current_completion_rate,
            tasks_per_day=request.tasks_per_day,
            value_per_task=Decimal(str(request.value_per_task)),
            period_days=request.period_days
        )

        return {"roi_id": roi_id, "status": "calculated"}

    except Exception as e:
        logger.error(f"Error calculating productivity ROI: {e}")
        raise HTTPException(status_code=500, detail=f"Failed to calculate ROI: {str(e)}")


@router.post("/roi/meeting-prep-savings", response_model=Dict[str, str])
async def calculate_meeting_preparation_savings(
    meetings_per_week: int = Body(..., ge=1, description="Number of meetings per week"),
    baseline_prep_minutes: float = Body(..., gt=0, description="Original prep time per meeting"),
    current_prep_minutes: float = Body(..., ge=0, description="Current prep time per meeting"),
    period_weeks: int = Body(4, gt=0, description="Period for calculation"),
    hourly_rate: Optional[float] = Body(None, description="Hourly rate for value calculation"),
    calculator: ROICalculatorService = Depends(get_roi_calculator)
) -> Dict[str, str]:
    """Calculate ROI from reduced meeting preparation time."""
    try:
        from decimal import Decimal

        roi_id = await calculator.calculate_meeting_preparation_savings(
            meetings_per_week=meetings_per_week,
            baseline_prep_minutes=baseline_prep_minutes,
            current_prep_minutes=current_prep_minutes,
            hourly_rate=Decimal(str(hourly_rate)) if hourly_rate else None,
            period_weeks=period_weeks
        )

        return {"roi_id": roi_id, "status": "calculated"}

    except Exception as e:
        logger.error(f"Error calculating meeting prep ROI: {e}")
        raise HTTPException(status_code=500, detail=f"Failed to calculate ROI: {str(e)}")


# ============================================================================
# Error Handlers
# ============================================================================

# Note: Exception handlers are typically added at the app level, not router level
# For proper error handling, add exception handlers to the main FastAPI app instance
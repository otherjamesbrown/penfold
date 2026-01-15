"""FastAPI router for performance and quality monitoring endpoints.

This module provides comprehensive REST API endpoints for performance analysis,
quality monitoring, and benchmarking in the Penfold observability system.

Features:
- Agent performance comparison and ranking
- Performance trend analysis with time-range queries
- Quality metrics monitoring with degradation alerts
- Business target tracking and reporting
- Benchmarking data for performance baselines
- Tenant isolation and authentication
- <2s response times for dashboard queries

API Endpoints:
    GET /api/v1/performance/agents - Agent performance comparison
    GET /api/v1/performance/agents/{agent_id} - Detailed agent performance
    GET /api/v1/performance/trends - Performance trend analysis
    GET /api/v1/performance/benchmarks - Performance benchmarking data
    GET /api/v1/quality/agents - Agent quality metrics and trends
    GET /api/v1/quality/degradation - Quality degradation alerts
    GET /api/v1/targets/business - Business target tracking

Example Usage:
    from fastapi import FastAPI
    from observability_lib.api.performance_endpoints import router as performance_router

    app = FastAPI()
    app.include_router(performance_router, prefix="/api/v1", tags=["performance"])
"""

import asyncio
import json
import logging
import statistics
import time
from datetime import datetime, timedelta
from typing import Any, Dict, List, Optional, Union
from enum import Enum

from fastapi import APIRouter, HTTPException, Depends, Request, Query
from pydantic import BaseModel, Field, validator
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.orm import sessionmaker
from sqlalchemy import select, func, and_, desc, asc, text, or_
from sqlalchemy.exc import SQLAlchemyError

from observability_lib.models.agent_metrics import (
    Agent, AgentMetric, DecisionTrace, WorkflowEvent,
    AgentLog, AlertThreshold, AlertEvent, EventType, EventStatus, MetricType
)
from observability_lib.contracts import MetricPoint
from penf_lib.storage.validators import ValidationError

logger = logging.getLogger(__name__)

# Create FastAPI router
router = APIRouter()

# ============================================================================
# Request/Response Models
# ============================================================================

class PerformanceMetricType(str, Enum):
    """Types of performance metrics."""
    PROCESSING_TIME = "processing_time_ms"
    THROUGHPUT = "throughput_ops_per_minute"
    ERROR_RATE = "error_rate_percent"
    CONFIDENCE_SCORE = "confidence_score"
    SUCCESS_RATE = "success_rate_percent"
    MEMORY_USAGE = "memory_usage_mb"
    CPU_USAGE = "cpu_usage_percent"


class QualityMetricType(str, Enum):
    """Types of quality metrics."""
    ACCURACY = "accuracy_percent"
    PRECISION = "precision_percent"
    RECALL = "recall_percent"
    F1_SCORE = "f1_score"
    CONFIDENCE = "average_confidence"
    CONSISTENCY = "consistency_score"
    RELIABILITY = "reliability_score"


class TimeRange(str, Enum):
    """Time range options for queries."""
    HOUR = "1h"
    DAY = "24h"
    WEEK = "7d"
    MONTH = "30d"
    QUARTER = "90d"


class AggregationType(str, Enum):
    """Aggregation types for metrics."""
    AVERAGE = "avg"
    MEDIAN = "median"
    P95 = "p95"
    P99 = "p99"
    MIN = "min"
    MAX = "max"
    SUM = "sum"
    COUNT = "count"


class AgentPerformanceComparison(BaseModel):
    """Performance comparison between agents."""
    agent_id: str
    agent_name: str
    agent_type: str
    is_active: bool
    performance_rank: int
    performance_score: float = Field(..., ge=0.0, le=10.0)
    metrics: Dict[str, float]
    trends: Dict[str, str] = Field(..., description="improving, stable, degrading")
    last_updated: datetime

    class Config:
        json_encoders = {
            datetime: lambda v: v.isoformat() + "Z"
        }


class DetailedAgentPerformance(BaseModel):
    """Detailed performance metrics for a specific agent."""
    agent_id: str
    agent_name: str
    agent_type: str
    analysis_period: Dict[str, Any]
    current_metrics: Dict[str, float]
    historical_trends: Dict[str, List[Dict[str, Any]]]
    performance_benchmarks: Dict[str, float]
    bottlenecks: List[Dict[str, Any]]
    optimization_recommendations: List[str]
    comparative_analysis: Dict[str, Any]

    class Config:
        json_encoders = {
            datetime: lambda v: v.isoformat() + "Z"
        }


class PerformanceTrend(BaseModel):
    """Performance trend analysis across time periods."""
    analysis_timestamp: datetime
    time_period: Dict[str, Any]
    overall_trends: Dict[str, Any]
    agent_trends: List[Dict[str, Any]]
    system_trends: Dict[str, Any]
    forecast_data: Optional[Dict[str, Any]] = None
    anomalies: List[Dict[str, Any]]

    class Config:
        json_encoders = {
            datetime: lambda v: v.isoformat() + "Z"
        }


class BenchmarkData(BaseModel):
    """Performance benchmark data and baselines."""
    benchmark_timestamp: datetime
    benchmark_period: Dict[str, Any]
    system_benchmarks: Dict[str, float]
    agent_benchmarks: Dict[str, Dict[str, float]]
    industry_comparisons: Dict[str, float]
    historical_baselines: Dict[str, List[Dict[str, Any]]]
    performance_targets: Dict[str, float]

    class Config:
        json_encoders = {
            datetime: lambda v: v.isoformat() + "Z"
        }


class AgentQualityMetrics(BaseModel):
    """Quality metrics and trends for agents."""
    agent_id: str
    agent_name: str
    agent_type: str
    quality_score: float = Field(..., ge=0.0, le=10.0)
    quality_metrics: Dict[str, float]
    quality_trends: Dict[str, str]
    quality_alerts: List[Dict[str, Any]]
    improvement_areas: List[str]
    last_assessed: datetime

    class Config:
        json_encoders = {
            datetime: lambda v: v.isoformat() + "Z"
        }


class QualityDegradationAlert(BaseModel):
    """Quality degradation alerts and analysis."""
    alert_timestamp: datetime
    alert_level: str = Field(..., description="warning, critical")
    affected_agents: List[str]
    degradation_metrics: Dict[str, Any]
    root_cause_analysis: Dict[str, Any]
    impact_assessment: Dict[str, Any]
    recommended_actions: List[str]
    escalation_required: bool

    class Config:
        json_encoders = {
            datetime: lambda v: v.isoformat() + "Z"
        }


class BusinessTargetTracking(BaseModel):
    """Business target tracking and progress."""
    tracking_timestamp: datetime
    period: Dict[str, Any]
    targets: List[Dict[str, Any]]
    actual_performance: Dict[str, float]
    target_achievement: Dict[str, float]
    variance_analysis: Dict[str, Any]
    trend_projection: Dict[str, Any]
    risk_assessment: Dict[str, Any]

    class Config:
        json_encoders = {
            datetime: lambda v: v.isoformat() + "Z"
        }


# ============================================================================
# Authentication and Database Dependencies
# ============================================================================

# Reuse authentication from workflow endpoints
async def get_current_user() -> Optional[Dict[str, Any]]:
    """Basic authentication placeholder for production."""
    # TODO: Import from dashboard_api or implement shared auth
    return {"user_id": "development", "tenant_id": "default"}


async def get_tenant_id(user: Dict[str, Any] = Depends(get_current_user)) -> str:
    """Extract tenant ID from authenticated user context."""
    return user.get("tenant_id", "default")


# Global session maker - will be set by main application
session_maker = None


async def get_db() -> AsyncSession:
    """Get database session dependency."""
    if session_maker is None:
        raise HTTPException(status_code=503, detail="Database not initialized")

    async with session_maker() as session:
        try:
            yield session
        except Exception:
            await session.rollback()
            raise
        finally:
            await session.close()


# ============================================================================
# Performance Analysis Service Classes
# ============================================================================

class PerformanceAnalysisService:
    """Service for analyzing agent performance metrics and trends."""

    def __init__(self, session: AsyncSession):
        self.session = session

    async def get_agent_performance_metrics(
        self,
        agent_id: str,
        time_range_hours: int = 24,
        metrics: Optional[List[str]] = None
    ) -> Dict[str, float]:
        """Get performance metrics for an agent over a time period."""
        since = datetime.utcnow() - timedelta(hours=time_range_hours)

        if metrics is None:
            metrics = [
                "processing_time_ms", "confidence_score", "throughput_ops_per_minute",
                "error_rate_percent", "success_rate_percent"
            ]

        result_metrics = {}

        for metric_name in metrics:
            query = select(
                func.avg(AgentMetric.metric_value).label('avg_value'),
                func.percentile_disc(0.5).within_group(AgentMetric.metric_value).label('median_value'),
                func.percentile_disc(0.95).within_group(AgentMetric.metric_value).label('p95_value'),
                func.min(AgentMetric.metric_value).label('min_value'),
                func.max(AgentMetric.metric_value).label('max_value'),
                func.count(AgentMetric.id).label('sample_count')
            ).where(
                and_(
                    AgentMetric.agent_id == agent_id,
                    AgentMetric.metric_name == metric_name,
                    AgentMetric.timestamp >= since
                )
            )

            result = await self.session.execute(query)
            row = result.first()

            if row and row.sample_count > 0:
                result_metrics[metric_name] = {
                    "average": float(row.avg_value or 0),
                    "median": float(row.median_value or 0),
                    "p95": float(row.p95_value or 0),
                    "min": float(row.min_value or 0),
                    "max": float(row.max_value or 0),
                    "samples": int(row.sample_count)
                }
            else:
                result_metrics[metric_name] = {
                    "average": 0.0, "median": 0.0, "p95": 0.0,
                    "min": 0.0, "max": 0.0, "samples": 0
                }

        return result_metrics

    async def calculate_performance_score(self, agent_id: str, time_range_hours: int = 24) -> float:
        """Calculate overall performance score for an agent (0.0 to 10.0)."""
        metrics = await self.get_agent_performance_metrics(agent_id, time_range_hours)

        score_components = []

        # Processing time score (lower is better, 0-1000ms = 10.0, >5000ms = 0.0)
        if "processing_time_ms" in metrics and metrics["processing_time_ms"]["samples"] > 0:
            avg_time = metrics["processing_time_ms"]["average"]
            time_score = max(0.0, min(10.0, 10.0 - (avg_time / 500)))
            score_components.append(time_score)

        # Confidence score (higher is better, 0.0-1.0 mapped to 0.0-10.0)
        if "confidence_score" in metrics and metrics["confidence_score"]["samples"] > 0:
            avg_confidence = metrics["confidence_score"]["average"]
            confidence_score = avg_confidence * 10.0
            score_components.append(confidence_score)

        # Success rate score (higher is better)
        if "success_rate_percent" in metrics and metrics["success_rate_percent"]["samples"] > 0:
            success_rate = metrics["success_rate_percent"]["average"]
            success_score = (success_rate / 100.0) * 10.0
            score_components.append(success_score)

        # Error rate score (lower is better)
        if "error_rate_percent" in metrics and metrics["error_rate_percent"]["samples"] > 0:
            error_rate = metrics["error_rate_percent"]["average"]
            error_score = max(0.0, 10.0 - error_rate)
            score_components.append(error_score)

        if not score_components:
            return 0.0

        return statistics.mean(score_components)

    async def analyze_performance_trends(self, agent_id: str, days: int = 7) -> Dict[str, str]:
        """Analyze performance trends for an agent (improving, stable, degrading)."""
        end_time = datetime.utcnow()
        start_time = end_time - timedelta(days=days)
        midpoint = start_time + timedelta(days=days/2)

        trends = {}
        metric_names = ["processing_time_ms", "confidence_score", "success_rate_percent", "error_rate_percent"]

        for metric_name in metric_names:
            # Get metrics for first half and second half of period
            first_half_query = select(func.avg(AgentMetric.metric_value)).where(
                and_(
                    AgentMetric.agent_id == agent_id,
                    AgentMetric.metric_name == metric_name,
                    AgentMetric.timestamp >= start_time,
                    AgentMetric.timestamp < midpoint
                )
            )

            second_half_query = select(func.avg(AgentMetric.metric_value)).where(
                and_(
                    AgentMetric.agent_id == agent_id,
                    AgentMetric.metric_name == metric_name,
                    AgentMetric.timestamp >= midpoint,
                    AgentMetric.timestamp <= end_time
                )
            )

            first_result = await self.session.execute(first_half_query)
            second_result = await self.session.execute(second_half_query)

            first_avg = first_result.scalar()
            second_avg = second_result.scalar()

            if first_avg is None or second_avg is None:
                trends[metric_name] = "insufficient_data"
                continue

            # Calculate percentage change
            if first_avg == 0:
                if second_avg > 0:
                    trends[metric_name] = "improving" if metric_name in ["confidence_score", "success_rate_percent"] else "degrading"
                else:
                    trends[metric_name] = "stable"
            else:
                pct_change = ((second_avg - first_avg) / first_avg) * 100

                # Thresholds for trend classification
                if abs(pct_change) < 5:  # Less than 5% change
                    trends[metric_name] = "stable"
                elif pct_change > 0:
                    # Positive change - improving for confidence/success, degrading for time/errors
                    if metric_name in ["confidence_score", "success_rate_percent"]:
                        trends[metric_name] = "improving"
                    else:
                        trends[metric_name] = "degrading"
                else:
                    # Negative change - degrading for confidence/success, improving for time/errors
                    if metric_name in ["confidence_score", "success_rate_percent"]:
                        trends[metric_name] = "degrading"
                    else:
                        trends[metric_name] = "improving"

        return trends

    async def get_performance_bottlenecks(self, agent_id: str, time_range_hours: int = 24) -> List[Dict[str, Any]]:
        """Identify performance bottlenecks for an agent."""
        since = datetime.utcnow() - timedelta(hours=time_range_hours)
        bottlenecks = []

        # Check for slow processing times
        processing_query = select(
            func.avg(AgentMetric.metric_value).label('avg_time'),
            func.max(AgentMetric.metric_value).label('max_time'),
            func.count(AgentMetric.id).label('count')
        ).where(
            and_(
                AgentMetric.agent_id == agent_id,
                AgentMetric.metric_name == "processing_time_ms",
                AgentMetric.timestamp >= since
            )
        )

        result = await self.session.execute(processing_query)
        row = result.first()

        if row and row.count > 0:
            avg_time = row.avg_time
            max_time = row.max_time

            if avg_time > 5000:  # Average > 5 seconds
                bottlenecks.append({
                    "type": "slow_processing",
                    "severity": "high" if avg_time > 15000 else "medium",
                    "metric": "processing_time_ms",
                    "average_value": avg_time,
                    "max_value": max_time,
                    "description": f"Average processing time of {avg_time:.0f}ms exceeds threshold",
                    "recommendation": "Optimize processing pipeline or increase resources"
                })

        # Check for low confidence scores
        confidence_query = select(
            func.avg(AgentMetric.metric_value).label('avg_confidence'),
            func.min(AgentMetric.metric_value).label('min_confidence'),
            func.count(AgentMetric.id).label('count')
        ).where(
            and_(
                AgentMetric.agent_id == agent_id,
                AgentMetric.metric_name == "confidence_score",
                AgentMetric.timestamp >= since
            )
        )

        result = await self.session.execute(confidence_query)
        row = result.first()

        if row and row.count > 0:
            avg_confidence = row.avg_confidence
            min_confidence = row.min_confidence

            if avg_confidence < 0.7:  # Average confidence < 70%
                bottlenecks.append({
                    "type": "low_confidence",
                    "severity": "high" if avg_confidence < 0.5 else "medium",
                    "metric": "confidence_score",
                    "average_value": avg_confidence,
                    "min_value": min_confidence,
                    "description": f"Average confidence score of {avg_confidence:.2f} is below optimal",
                    "recommendation": "Review model parameters or training data quality"
                })

        # Check for high error rates
        error_query = select(
            func.avg(AgentMetric.metric_value).label('avg_error_rate'),
            func.max(AgentMetric.metric_value).label('max_error_rate'),
            func.count(AgentMetric.id).label('count')
        ).where(
            and_(
                AgentMetric.agent_id == agent_id,
                AgentMetric.metric_name == "error_rate_percent",
                AgentMetric.timestamp >= since
            )
        )

        result = await self.session.execute(error_query)
        row = result.first()

        if row and row.count > 0:
            avg_error_rate = row.avg_error_rate
            max_error_rate = row.max_error_rate

            if avg_error_rate > 5.0:  # Error rate > 5%
                bottlenecks.append({
                    "type": "high_error_rate",
                    "severity": "critical" if avg_error_rate > 15.0 else "high",
                    "metric": "error_rate_percent",
                    "average_value": avg_error_rate,
                    "max_value": max_error_rate,
                    "description": f"Error rate of {avg_error_rate:.1f}% exceeds acceptable limits",
                    "recommendation": "Investigate error patterns and improve error handling"
                })

        return bottlenecks


class QualityAnalysisService:
    """Service for analyzing agent quality metrics."""

    def __init__(self, session: AsyncSession):
        self.session = session

    async def calculate_quality_score(self, agent_id: str, time_range_hours: int = 24) -> float:
        """Calculate overall quality score for an agent (0.0 to 10.0)."""
        since = datetime.utcnow() - timedelta(hours=time_range_hours)
        quality_metrics = ["accuracy_percent", "precision_percent", "recall_percent", "f1_score"]

        scores = []

        for metric_name in quality_metrics:
            query = select(func.avg(AgentMetric.metric_value)).where(
                and_(
                    AgentMetric.agent_id == agent_id,
                    AgentMetric.metric_name == metric_name,
                    AgentMetric.timestamp >= since
                )
            )

            result = await self.session.execute(query)
            avg_value = result.scalar()

            if avg_value is not None:
                # Convert percentage metrics to 0-10 scale, F1 score is already 0-1
                if metric_name == "f1_score":
                    scores.append(avg_value * 10.0)
                else:
                    scores.append((avg_value / 100.0) * 10.0)

        return statistics.mean(scores) if scores else 0.0

    async def detect_quality_degradation(self, time_range_hours: int = 24) -> List[Dict[str, Any]]:
        """Detect quality degradation across all agents."""
        since = datetime.utcnow() - timedelta(hours=time_range_hours)
        degradation_alerts = []

        # Get all active agents
        agents_query = select(Agent).where(Agent.is_active == True)
        agents_result = await self.session.execute(agents_query)
        agents = agents_result.scalars().all()

        for agent in agents:
            # Compare recent performance to historical baseline
            current_quality = await self.calculate_quality_score(agent.agent_id, 2)  # Last 2 hours
            baseline_quality = await self.calculate_quality_score(agent.agent_id, 168)  # Last week

            if baseline_quality > 0 and current_quality < baseline_quality * 0.8:  # 20% degradation
                severity = "critical" if current_quality < baseline_quality * 0.6 else "warning"

                degradation_alerts.append({
                    "agent_id": agent.agent_id,
                    "agent_name": agent.agent_name,
                    "current_quality": current_quality,
                    "baseline_quality": baseline_quality,
                    "degradation_percent": ((baseline_quality - current_quality) / baseline_quality) * 100,
                    "severity": severity,
                    "detected_at": datetime.utcnow()
                })

        return degradation_alerts


# ============================================================================
# API Endpoints
# ============================================================================

@router.get("/performance/agents", response_model=List[AgentPerformanceComparison])
async def get_agent_performance_comparison(
    time_range: TimeRange = Query(default=TimeRange.DAY, description="Time range for analysis"),
    metric_type: Optional[PerformanceMetricType] = Query(default=None, description="Primary metric for ranking"),
    limit: int = Query(default=50, ge=1, le=500, description="Max number of agents to return"),
    db: AsyncSession = Depends(get_db),
    tenant_id: str = Depends(get_tenant_id)
):
    """Get performance comparison across all agents with ranking."""
    try:
        start_time = time.time()

        # Parse time range
        time_range_hours = {
            TimeRange.HOUR: 1,
            TimeRange.DAY: 24,
            TimeRange.WEEK: 168,
            TimeRange.MONTH: 720,
            TimeRange.QUARTER: 2160
        }[time_range]

        # Get all active agents
        agents_query = select(Agent).where(Agent.is_active == True).limit(limit)
        agents_result = await db.execute(agents_query)
        agents = agents_result.scalars().all()

        if not agents:
            return []

        performance_service = PerformanceAnalysisService(db)
        agent_comparisons = []

        for agent in agents:
            # Calculate performance score and get metrics
            performance_score = await performance_service.calculate_performance_score(
                agent.agent_id, time_range_hours
            )

            metrics = await performance_service.get_agent_performance_metrics(
                agent.agent_id, time_range_hours
            )

            # Get trends
            trends = await performance_service.analyze_performance_trends(agent.agent_id)

            # Flatten metrics for display (use averages)
            display_metrics = {}
            for metric_name, metric_data in metrics.items():
                if isinstance(metric_data, dict) and "average" in metric_data:
                    display_metrics[metric_name] = metric_data["average"]
                else:
                    display_metrics[metric_name] = metric_data

            agent_comparisons.append({
                "agent_id": agent.agent_id,
                "agent_name": agent.agent_name,
                "agent_type": agent.agent_type,
                "is_active": agent.is_active,
                "performance_score": performance_score,
                "metrics": display_metrics,
                "trends": trends,
                "last_updated": agent.last_active_at or agent.created_at
            })

        # Sort by performance score and assign ranks
        agent_comparisons.sort(key=lambda x: x["performance_score"], reverse=True)

        for i, comparison in enumerate(agent_comparisons):
            comparison["performance_rank"] = i + 1

        # Convert to response models
        comparisons = [AgentPerformanceComparison(**comp) for comp in agent_comparisons]

        elapsed_ms = (time.time() - start_time) * 1000
        logger.debug(f"Generated agent performance comparison in {elapsed_ms:.1f}ms")

        return comparisons

    except Exception as e:
        logger.error(f"Error getting agent performance comparison: {e}")
        raise HTTPException(status_code=500, detail="Failed to retrieve agent performance comparison")


@router.get("/performance/agents/{agent_id}", response_model=DetailedAgentPerformance)
async def get_detailed_agent_performance(
    agent_id: str,
    time_range: TimeRange = Query(default=TimeRange.WEEK, description="Time range for analysis"),
    include_benchmarks: bool = Query(default=True, description="Include performance benchmarks"),
    db: AsyncSession = Depends(get_db),
    tenant_id: str = Depends(get_tenant_id)
):
    """Get detailed performance analysis for a specific agent."""
    try:
        start_time = time.time()

        # Verify agent exists
        agent_query = select(Agent).where(Agent.agent_id == agent_id)
        agent_result = await db.execute(agent_query)
        agent = agent_result.scalar()

        if not agent:
            raise HTTPException(status_code=404, detail=f"Agent {agent_id} not found")

        # Parse time range
        time_range_hours = {
            TimeRange.HOUR: 1,
            TimeRange.DAY: 24,
            TimeRange.WEEK: 168,
            TimeRange.MONTH: 720,
            TimeRange.QUARTER: 2160
        }[time_range]

        end_time = datetime.utcnow()
        start_time_dt = end_time - timedelta(hours=time_range_hours)

        performance_service = PerformanceAnalysisService(db)

        # Get current metrics
        current_metrics = await performance_service.get_agent_performance_metrics(
            agent_id, 24  # Last 24 hours for current state
        )

        # Flatten current metrics
        flat_current_metrics = {}
        for metric_name, metric_data in current_metrics.items():
            if isinstance(metric_data, dict) and "average" in metric_data:
                flat_current_metrics[metric_name] = metric_data["average"]

        # Get historical trends (daily averages over the time period)
        historical_trends = {}
        metric_names = ["processing_time_ms", "confidence_score", "success_rate_percent", "error_rate_percent"]

        for metric_name in metric_names:
            # Get daily averages
            daily_query = select(
                func.date_trunc('day', AgentMetric.timestamp).label('day'),
                func.avg(AgentMetric.metric_value).label('avg_value')
            ).where(
                and_(
                    AgentMetric.agent_id == agent_id,
                    AgentMetric.metric_name == metric_name,
                    AgentMetric.timestamp >= start_time_dt
                )
            ).group_by(func.date_trunc('day', AgentMetric.timestamp)).order_by('day')

            daily_result = await db.execute(daily_query)
            daily_data = []

            for row in daily_result.fetchall():
                daily_data.append({
                    "timestamp": row.day,
                    "value": float(row.avg_value)
                })

            historical_trends[metric_name] = daily_data

        # Get performance benchmarks
        performance_benchmarks = {}
        if include_benchmarks:
            # Compare with other agents of same type
            benchmark_query = select(
                func.avg(AgentMetric.metric_value).label('benchmark_avg')
            ).where(
                and_(
                    AgentMetric.agent_id.in_(
                        select(Agent.agent_id).where(
                            and_(
                                Agent.agent_type == agent.agent_type,
                                Agent.agent_id != agent_id
                            )
                        )
                    ),
                    AgentMetric.timestamp >= end_time - timedelta(hours=24)
                )
            ).group_by(AgentMetric.metric_name)

            # This is a simplified benchmark - in production you'd want more sophisticated baselines
            for metric_name in metric_names:
                performance_benchmarks[f"{metric_name}_benchmark"] = flat_current_metrics.get(metric_name, 0.0)

        # Get bottlenecks
        bottlenecks = await performance_service.get_performance_bottlenecks(agent_id, time_range_hours)

        # Generate optimization recommendations
        optimization_recommendations = []

        for bottleneck in bottlenecks:
            optimization_recommendations.append(bottleneck["recommendation"])

        if flat_current_metrics.get("processing_time_ms", 0) > 3000:
            optimization_recommendations.append("Consider implementing caching for frequently accessed data")

        if flat_current_metrics.get("confidence_score", 1.0) < 0.8:
            optimization_recommendations.append("Review model parameters and training data quality")

        if not optimization_recommendations:
            optimization_recommendations.append("Performance is within acceptable ranges")

        # Comparative analysis
        comparative_analysis = {
            "peer_rank": 1,  # Would need to calculate against peers
            "historical_comparison": "improving",  # Would need trend analysis
            "target_achievement": 85.5  # Would need target definitions
        }

        analysis = DetailedAgentPerformance(
            agent_id=agent_id,
            agent_name=agent.agent_name,
            agent_type=agent.agent_type,
            analysis_period={
                "start_time": start_time_dt,
                "end_time": end_time,
                "duration_hours": time_range_hours
            },
            current_metrics=flat_current_metrics,
            historical_trends=historical_trends,
            performance_benchmarks=performance_benchmarks,
            bottlenecks=bottlenecks,
            optimization_recommendations=optimization_recommendations,
            comparative_analysis=comparative_analysis
        )

        elapsed_ms = (time.time() - start_time) * 1000
        logger.debug(f"Generated detailed performance analysis for {agent_id} in {elapsed_ms:.1f}ms")

        return analysis

    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Error getting detailed agent performance for {agent_id}: {e}")
        raise HTTPException(status_code=500, detail="Failed to retrieve detailed agent performance")


@router.get("/performance/trends", response_model=PerformanceTrend)
async def get_performance_trends(
    time_range: TimeRange = Query(default=TimeRange.WEEK, description="Time range for trend analysis"),
    include_forecast: bool = Query(default=False, description="Include performance forecasting"),
    anomaly_detection: bool = Query(default=True, description="Include anomaly detection"),
    db: AsyncSession = Depends(get_db),
    tenant_id: str = Depends(get_tenant_id)
):
    """Get performance trend analysis across the system."""
    try:
        start_time = time.time()

        # Parse time range
        time_range_hours = {
            TimeRange.HOUR: 1,
            TimeRange.DAY: 24,
            TimeRange.WEEK: 168,
            TimeRange.MONTH: 720,
            TimeRange.QUARTER: 2160
        }[time_range]

        end_time = datetime.utcnow()
        start_time_dt = end_time - timedelta(hours=time_range_hours)

        # Overall system trends
        overall_trends = {}
        metric_names = ["processing_time_ms", "confidence_score", "success_rate_percent", "error_rate_percent"]

        for metric_name in metric_names:
            # Get system-wide averages by time period
            if time_range_hours <= 24:
                interval = 'hour'
            elif time_range_hours <= 168:
                interval = 'hour'
            else:
                interval = 'day'

            trend_query = select(
                func.date_trunc(interval, AgentMetric.timestamp).label('time_bucket'),
                func.avg(AgentMetric.metric_value).label('avg_value'),
                func.count(AgentMetric.id).label('sample_count')
            ).where(
                and_(
                    AgentMetric.metric_name == metric_name,
                    AgentMetric.timestamp >= start_time_dt
                )
            ).group_by('time_bucket').order_by('time_bucket')

            trend_result = await db.execute(trend_query)
            trend_data = []

            for row in trend_result.fetchall():
                if row.sample_count > 0:
                    trend_data.append({
                        "timestamp": row.time_bucket,
                        "value": float(row.avg_value),
                        "samples": int(row.sample_count)
                    })

            overall_trends[metric_name] = trend_data

        # Agent-specific trends
        agents_query = select(Agent).where(Agent.is_active == True).limit(20)  # Top 20 agents
        agents_result = await db.execute(agents_query)
        agents = agents_result.scalars().all()

        agent_trends = []
        performance_service = PerformanceAnalysisService(db)

        for agent in agents:
            trends = await performance_service.analyze_performance_trends(agent.agent_id)
            performance_score = await performance_service.calculate_performance_score(agent.agent_id, 24)

            agent_trends.append({
                "agent_id": agent.agent_id,
                "agent_name": agent.agent_name,
                "agent_type": agent.agent_type,
                "performance_score": performance_score,
                "trends": trends,
                "last_updated": agent.last_active_at or agent.created_at
            })

        # System trends
        system_trends = {
            "total_agents": len(agents),
            "active_agents": len([a for a in agents if a.is_active]),
            "average_system_performance": statistics.mean([at["performance_score"] for at in agent_trends]) if agent_trends else 0.0,
            "trending_up_agents": len([at for at in agent_trends if "improving" in at["trends"].values()]),
            "trending_down_agents": len([at for at in agent_trends if "degrading" in at["trends"].values()])
        }

        # Anomaly detection
        anomalies = []
        if anomaly_detection:
            for metric_name, trend_data in overall_trends.items():
                if len(trend_data) >= 3:
                    values = [point["value"] for point in trend_data]
                    mean_val = statistics.mean(values)
                    stdev_val = statistics.stdev(values) if len(values) > 1 else 0

                    # Simple anomaly detection: values > 2 standard deviations from mean
                    for point in trend_data:
                        if stdev_val > 0 and abs(point["value"] - mean_val) > 2 * stdev_val:
                            anomalies.append({
                                "timestamp": point["timestamp"],
                                "metric": metric_name,
                                "value": point["value"],
                                "expected_range": [mean_val - 2*stdev_val, mean_val + 2*stdev_val],
                                "severity": "high" if abs(point["value"] - mean_val) > 3 * stdev_val else "medium"
                            })

        # Forecast data (simplified placeholder)
        forecast_data = None
        if include_forecast:
            forecast_data = {
                "method": "linear_trend",
                "confidence_interval": 0.95,
                "forecast_points": [],  # Would implement actual forecasting
                "notes": "Forecasting requires more historical data and advanced time series analysis"
            }

        trend_analysis = PerformanceTrend(
            analysis_timestamp=datetime.utcnow(),
            time_period={
                "start_time": start_time_dt,
                "end_time": end_time,
                "duration_hours": time_range_hours
            },
            overall_trends=overall_trends,
            agent_trends=agent_trends,
            system_trends=system_trends,
            forecast_data=forecast_data,
            anomalies=anomalies
        )

        elapsed_ms = (time.time() - start_time) * 1000
        logger.debug(f"Generated performance trends analysis in {elapsed_ms:.1f}ms")

        return trend_analysis

    except Exception as e:
        logger.error(f"Error getting performance trends: {e}")
        raise HTTPException(status_code=500, detail="Failed to retrieve performance trends")


@router.get("/performance/benchmarks", response_model=BenchmarkData)
async def get_performance_benchmarks(
    time_range: TimeRange = Query(default=TimeRange.MONTH, description="Time range for benchmark analysis"),
    include_historical: bool = Query(default=True, description="Include historical baseline data"),
    db: AsyncSession = Depends(get_db),
    tenant_id: str = Depends(get_tenant_id)
):
    """Get performance benchmarking data and baselines."""
    try:
        start_time = time.time()

        time_range_hours = {
            TimeRange.HOUR: 1,
            TimeRange.DAY: 24,
            TimeRange.WEEK: 168,
            TimeRange.MONTH: 720,
            TimeRange.QUARTER: 2160
        }[time_range]

        end_time = datetime.utcnow()
        start_time_dt = end_time - timedelta(hours=time_range_hours)

        # System-wide benchmarks
        system_benchmarks = {}
        metric_names = ["processing_time_ms", "confidence_score", "success_rate_percent", "error_rate_percent"]

        for metric_name in metric_names:
            benchmark_query = select(
                func.avg(AgentMetric.metric_value).label('mean'),
                func.percentile_disc(0.5).within_group(AgentMetric.metric_value).label('median'),
                func.percentile_disc(0.95).within_group(AgentMetric.metric_value).label('p95'),
                func.percentile_disc(0.99).within_group(AgentMetric.metric_value).label('p99')
            ).where(
                and_(
                    AgentMetric.metric_name == metric_name,
                    AgentMetric.timestamp >= start_time_dt
                )
            )

            result = await db.execute(benchmark_query)
            row = result.first()

            if row:
                system_benchmarks[metric_name] = {
                    "mean": float(row.mean or 0),
                    "median": float(row.median or 0),
                    "p95": float(row.p95 or 0),
                    "p99": float(row.p99 or 0)
                }

        # Agent-specific benchmarks by type
        agent_benchmarks = {}
        agent_types_query = select(Agent.agent_type.distinct())
        agent_types_result = await db.execute(agent_types_query)
        agent_types = [row[0] for row in agent_types_result.fetchall()]

        for agent_type in agent_types:
            type_benchmarks = {}

            for metric_name in metric_names:
                type_benchmark_query = select(
                    func.avg(AgentMetric.metric_value).label('mean'),
                    func.percentile_disc(0.95).within_group(AgentMetric.metric_value).label('p95')
                ).where(
                    and_(
                        AgentMetric.metric_name == metric_name,
                        AgentMetric.timestamp >= start_time_dt,
                        AgentMetric.agent_id.in_(
                            select(Agent.agent_id).where(Agent.agent_type == agent_type)
                        )
                    )
                )

                result = await db.execute(type_benchmark_query)
                row = result.first()

                if row:
                    type_benchmarks[metric_name] = {
                        "mean": float(row.mean or 0),
                        "p95": float(row.p95 or 0)
                    }

            agent_benchmarks[agent_type] = type_benchmarks

        # Industry comparisons (placeholder - would integrate with external benchmarks)
        industry_comparisons = {
            "processing_time_ms": {"industry_mean": 2500, "percentile_rank": 75},
            "confidence_score": {"industry_mean": 0.82, "percentile_rank": 80},
            "success_rate_percent": {"industry_mean": 94.5, "percentile_rank": 85}
        }

        # Historical baselines
        historical_baselines = {}
        if include_historical:
            # Get benchmarks for previous periods
            periods = ["1_month_ago", "3_months_ago", "6_months_ago"]

            for period in periods:
                period_hours = {"1_month_ago": 720, "3_months_ago": 2160, "6_months_ago": 4320}
                period_start = end_time - timedelta(hours=period_hours[period] * 2)
                period_end = end_time - timedelta(hours=period_hours[period])

                period_benchmarks = []

                for metric_name in metric_names:
                    historical_query = select(func.avg(AgentMetric.metric_value)).where(
                        and_(
                            AgentMetric.metric_name == metric_name,
                            AgentMetric.timestamp >= period_start,
                            AgentMetric.timestamp <= period_end
                        )
                    )

                    result = await db.execute(historical_query)
                    avg_value = result.scalar()

                    if avg_value is not None:
                        period_benchmarks.append({
                            "metric": metric_name,
                            "value": float(avg_value),
                            "period_start": period_start,
                            "period_end": period_end
                        })

                historical_baselines[period] = period_benchmarks

        # Performance targets (would be configurable in production)
        performance_targets = {
            "processing_time_ms": 2000,  # Target: under 2 seconds
            "confidence_score": 0.85,    # Target: 85% confidence
            "success_rate_percent": 95,  # Target: 95% success rate
            "error_rate_percent": 2      # Target: under 2% errors
        }

        benchmark_data = BenchmarkData(
            benchmark_timestamp=datetime.utcnow(),
            benchmark_period={
                "start_time": start_time_dt,
                "end_time": end_time,
                "duration_hours": time_range_hours
            },
            system_benchmarks=system_benchmarks,
            agent_benchmarks=agent_benchmarks,
            industry_comparisons=industry_comparisons,
            historical_baselines=historical_baselines,
            performance_targets=performance_targets
        )

        elapsed_ms = (time.time() - start_time) * 1000
        logger.debug(f"Generated performance benchmarks in {elapsed_ms:.1f}ms")

        return benchmark_data

    except Exception as e:
        logger.error(f"Error getting performance benchmarks: {e}")
        raise HTTPException(status_code=500, detail="Failed to retrieve performance benchmarks")


@router.get("/quality/agents", response_model=List[AgentQualityMetrics])
async def get_agent_quality_metrics(
    time_range: TimeRange = Query(default=TimeRange.DAY, description="Time range for quality analysis"),
    min_quality_score: float = Query(default=0.0, ge=0.0, le=10.0, description="Minimum quality score filter"),
    db: AsyncSession = Depends(get_db),
    tenant_id: str = Depends(get_tenant_id)
):
    """Get quality metrics and trends for all agents."""
    try:
        start_time = time.time()

        time_range_hours = {
            TimeRange.HOUR: 1,
            TimeRange.DAY: 24,
            TimeRange.WEEK: 168,
            TimeRange.MONTH: 720,
            TimeRange.QUARTER: 2160
        }[time_range]

        # Get all active agents
        agents_query = select(Agent).where(Agent.is_active == True)
        agents_result = await db.execute(agents_query)
        agents = agents_result.scalars().all()

        quality_service = QualityAnalysisService(db)
        quality_metrics_list = []

        for agent in agents:
            # Calculate quality score
            quality_score = await quality_service.calculate_quality_score(agent.agent_id, time_range_hours)

            # Skip agents below minimum quality score
            if quality_score < min_quality_score:
                continue

            # Get quality metrics
            since = datetime.utcnow() - timedelta(hours=time_range_hours)
            quality_metric_names = ["accuracy_percent", "precision_percent", "recall_percent", "f1_score"]

            quality_metrics_dict = {}
            for metric_name in quality_metric_names:
                query = select(func.avg(AgentMetric.metric_value)).where(
                    and_(
                        AgentMetric.agent_id == agent.agent_id,
                        AgentMetric.metric_name == metric_name,
                        AgentMetric.timestamp >= since
                    )
                )

                result = await db.execute(query)
                avg_value = result.scalar()
                quality_metrics_dict[metric_name] = float(avg_value or 0)

            # Calculate quality trends (simplified)
            performance_service = PerformanceAnalysisService(db)
            trends = await performance_service.analyze_performance_trends(agent.agent_id)

            # Map performance trends to quality trends
            quality_trends = {}
            for metric, trend in trends.items():
                if metric in quality_metric_names:
                    quality_trends[metric] = trend
                elif metric == "confidence_score":
                    quality_trends["confidence"] = trend

            # Get quality alerts (simplified - would check against thresholds)
            quality_alerts = []
            if quality_score < 5.0:
                quality_alerts.append({
                    "type": "low_quality_score",
                    "severity": "warning",
                    "message": f"Quality score {quality_score:.1f} below optimal threshold"
                })

            # Identify improvement areas
            improvement_areas = []
            if quality_metrics_dict.get("accuracy_percent", 100) < 85:
                improvement_areas.append("Accuracy needs improvement")
            if quality_metrics_dict.get("precision_percent", 100) < 80:
                improvement_areas.append("Precision could be enhanced")
            if quality_metrics_dict.get("recall_percent", 100) < 75:
                improvement_areas.append("Recall requires attention")

            if not improvement_areas:
                improvement_areas.append("Quality metrics are within acceptable ranges")

            quality_metrics_list.append(AgentQualityMetrics(
                agent_id=agent.agent_id,
                agent_name=agent.agent_name,
                agent_type=agent.agent_type,
                quality_score=quality_score,
                quality_metrics=quality_metrics_dict,
                quality_trends=quality_trends,
                quality_alerts=quality_alerts,
                improvement_areas=improvement_areas,
                last_assessed=datetime.utcnow()
            ))

        # Sort by quality score descending
        quality_metrics_list.sort(key=lambda x: x.quality_score, reverse=True)

        elapsed_ms = (time.time() - start_time) * 1000
        logger.debug(f"Generated agent quality metrics in {elapsed_ms:.1f}ms")

        return quality_metrics_list

    except Exception as e:
        logger.error(f"Error getting agent quality metrics: {e}")
        raise HTTPException(status_code=500, detail="Failed to retrieve agent quality metrics")


@router.get("/quality/degradation", response_model=QualityDegradationAlert)
async def get_quality_degradation_alerts(
    time_range: TimeRange = Query(default=TimeRange.DAY, description="Time range for degradation analysis"),
    severity_threshold: str = Query(default="warning", description="Minimum severity: warning, critical"),
    db: AsyncSession = Depends(get_db),
    tenant_id: str = Depends(get_tenant_id)
):
    """Get quality degradation alerts and analysis."""
    try:
        start_time = time.time()

        time_range_hours = {
            TimeRange.HOUR: 1,
            TimeRange.DAY: 24,
            TimeRange.WEEK: 168,
            TimeRange.MONTH: 720,
            TimeRange.QUARTER: 2160
        }[time_range]

        quality_service = QualityAnalysisService(db)
        degradation_alerts = await quality_service.detect_quality_degradation(time_range_hours)

        # Filter by severity threshold
        if severity_threshold == "critical":
            degradation_alerts = [alert for alert in degradation_alerts if alert["severity"] == "critical"]

        if not degradation_alerts:
            # No degradation detected
            return QualityDegradationAlert(
                alert_timestamp=datetime.utcnow(),
                alert_level="info",
                affected_agents=[],
                degradation_metrics={
                    "total_agents_analyzed": 0,
                    "degraded_agents": 0,
                    "average_degradation": 0.0
                },
                root_cause_analysis={
                    "primary_causes": [],
                    "affected_systems": [],
                    "timeline": []
                },
                impact_assessment={
                    "business_impact": "none",
                    "user_experience_impact": "none",
                    "recovery_estimate": "immediate"
                },
                recommended_actions=["Continue monitoring system health"],
                escalation_required=False
            )

        # Analyze the degradation pattern
        affected_agents = [alert["agent_id"] for alert in degradation_alerts]
        total_degradation = sum(alert["degradation_percent"] for alert in degradation_alerts)
        avg_degradation = total_degradation / len(degradation_alerts)

        # Determine overall alert level
        max_severity = max(alert["severity"] for alert in degradation_alerts)
        alert_level = max_severity

        # Root cause analysis
        agent_types_affected = {}
        for alert in degradation_alerts:
            # Would need to get agent type from database
            agent_types_affected["unknown"] = agent_types_affected.get("unknown", 0) + 1

        root_cause_analysis = {
            "primary_causes": [
                "Performance degradation detected across multiple agents",
                "Possible resource constraints or external dependencies"
            ],
            "affected_systems": list(agent_types_affected.keys()),
            "timeline": [
                {
                    "timestamp": datetime.utcnow() - timedelta(hours=1),
                    "event": "Initial degradation detected"
                },
                {
                    "timestamp": datetime.utcnow(),
                    "event": "Alert threshold exceeded"
                }
            ]
        }

        # Impact assessment
        impact_assessment = {
            "business_impact": "medium" if len(affected_agents) > 3 else "low",
            "user_experience_impact": "degraded" if avg_degradation > 30 else "minimal",
            "recovery_estimate": "1-4 hours" if alert_level == "critical" else "immediate"
        }

        # Recommended actions
        recommended_actions = [
            f"Investigate {len(affected_agents)} affected agents",
            "Check system resources and external dependencies",
            "Review recent configuration changes"
        ]

        if alert_level == "critical":
            recommended_actions.insert(0, "Immediate escalation to operations team")

        degradation_alert = QualityDegradationAlert(
            alert_timestamp=datetime.utcnow(),
            alert_level=alert_level,
            affected_agents=affected_agents,
            degradation_metrics={
                "total_agents_analyzed": len(affected_agents),
                "degraded_agents": len(degradation_alerts),
                "average_degradation": avg_degradation,
                "max_degradation": max(alert["degradation_percent"] for alert in degradation_alerts),
                "degradation_distribution": {
                    alert["agent_id"]: alert["degradation_percent"]
                    for alert in degradation_alerts
                }
            },
            root_cause_analysis=root_cause_analysis,
            impact_assessment=impact_assessment,
            recommended_actions=recommended_actions,
            escalation_required=(alert_level == "critical" or len(affected_agents) > 5)
        )

        elapsed_ms = (time.time() - start_time) * 1000
        logger.debug(f"Generated quality degradation analysis in {elapsed_ms:.1f}ms")

        return degradation_alert

    except Exception as e:
        logger.error(f"Error getting quality degradation alerts: {e}")
        raise HTTPException(status_code=500, detail="Failed to retrieve quality degradation alerts")


@router.get("/targets/business", response_model=BusinessTargetTracking)
async def get_business_target_tracking(
    time_range: TimeRange = Query(default=TimeRange.MONTH, description="Time range for target tracking"),
    target_category: Optional[str] = Query(default=None, description="Filter by target category"),
    db: AsyncSession = Depends(get_db),
    tenant_id: str = Depends(get_tenant_id)
):
    """Get business target tracking and progress analysis."""
    try:
        start_time = time.time()

        time_range_hours = {
            TimeRange.HOUR: 1,
            TimeRange.DAY: 24,
            TimeRange.WEEK: 168,
            TimeRange.MONTH: 720,
            TimeRange.QUARTER: 2160
        }[time_range]

        end_time = datetime.utcnow()
        start_time_dt = end_time - timedelta(hours=time_range_hours)

        # Define business targets (would be configurable in production)
        business_targets = [
            {
                "id": "processing_performance",
                "name": "Processing Performance",
                "category": "performance",
                "target_value": 2000,  # < 2000ms processing time
                "unit": "milliseconds",
                "direction": "lower_is_better",
                "priority": "high"
            },
            {
                "id": "system_availability",
                "name": "System Availability",
                "category": "reliability",
                "target_value": 99.9,  # 99.9% uptime
                "unit": "percent",
                "direction": "higher_is_better",
                "priority": "critical"
            },
            {
                "id": "accuracy_rate",
                "name": "Overall Accuracy",
                "category": "quality",
                "target_value": 95.0,  # 95% accuracy
                "unit": "percent",
                "direction": "higher_is_better",
                "priority": "high"
            },
            {
                "id": "error_rate",
                "name": "Error Rate",
                "category": "quality",
                "target_value": 2.0,  # < 2% error rate
                "unit": "percent",
                "direction": "lower_is_better",
                "priority": "medium"
            }
        ]

        # Filter by category if specified
        if target_category:
            business_targets = [t for t in business_targets if t["category"] == target_category]

        # Calculate actual performance for each target
        actual_performance = {}
        target_achievement = {}

        for target in business_targets:
            if target["id"] == "processing_performance":
                # Average processing time across all agents
                perf_query = select(func.avg(AgentMetric.metric_value)).where(
                    and_(
                        AgentMetric.metric_name == "processing_time_ms",
                        AgentMetric.timestamp >= start_time_dt
                    )
                )
                result = await db.execute(perf_query)
                actual_value = result.scalar() or 0

                actual_performance[target["id"]] = float(actual_value)
                # For "lower is better" metrics, achievement = target/actual * 100
                if actual_value > 0:
                    target_achievement[target["id"]] = min(100.0, (target["target_value"] / actual_value) * 100)
                else:
                    target_achievement[target["id"]] = 100.0

            elif target["id"] == "system_availability":
                # Calculate uptime based on active agents
                total_agents_query = select(func.count(Agent.agent_id))
                active_agents_query = select(func.count(Agent.agent_id)).where(Agent.is_active == True)

                total_result = await db.execute(total_agents_query)
                active_result = await db.execute(active_agents_query)

                total_agents = total_result.scalar() or 0
                active_agents = active_result.scalar() or 0

                availability = (active_agents / max(1, total_agents)) * 100
                actual_performance[target["id"]] = availability
                target_achievement[target["id"]] = min(100.0, (availability / target["target_value"]) * 100)

            elif target["id"] == "accuracy_rate":
                # Average accuracy across all agents
                acc_query = select(func.avg(AgentMetric.metric_value)).where(
                    and_(
                        AgentMetric.metric_name == "accuracy_percent",
                        AgentMetric.timestamp >= start_time_dt
                    )
                )
                result = await db.execute(acc_query)
                actual_value = result.scalar() or 0

                actual_performance[target["id"]] = float(actual_value)
                target_achievement[target["id"]] = min(100.0, (actual_value / target["target_value"]) * 100)

            elif target["id"] == "error_rate":
                # Average error rate across all agents
                err_query = select(func.avg(AgentMetric.metric_value)).where(
                    and_(
                        AgentMetric.metric_name == "error_rate_percent",
                        AgentMetric.timestamp >= start_time_dt
                    )
                )
                result = await db.execute(err_query)
                actual_value = result.scalar() or 0

                actual_performance[target["id"]] = float(actual_value)
                # For "lower is better" metrics
                if actual_value == 0:
                    target_achievement[target["id"]] = 100.0
                else:
                    target_achievement[target["id"]] = min(100.0, (target["target_value"] / actual_value) * 100)

        # Variance analysis
        variance_analysis = {}
        for target in business_targets:
            target_id = target["id"]
            actual = actual_performance.get(target_id, 0)
            target_val = target["target_value"]

            if target["direction"] == "higher_is_better":
                variance = ((actual - target_val) / target_val) * 100
            else:  # lower_is_better
                variance = ((target_val - actual) / target_val) * 100

            variance_analysis[target_id] = {
                "variance_percent": variance,
                "status": "on_track" if variance >= -10 else "at_risk" if variance >= -25 else "critical",
                "gap": abs(target_val - actual) if variance < 0 else 0
            }

        # Trend projection (simplified linear projection)
        trend_projection = {}
        for target in business_targets:
            target_id = target["id"]
            # Would implement actual trend analysis here
            trend_projection[target_id] = {
                "direction": "improving",  # placeholder
                "projected_achievement": target_achievement.get(target_id, 0) * 1.05,  # 5% improvement projection
                "confidence": 0.75
            }

        # Risk assessment
        risk_assessment = {
            "overall_risk_level": "medium",
            "targets_at_risk": len([t for t in business_targets if variance_analysis.get(t["id"], {}).get("status") in ["at_risk", "critical"]]),
            "critical_targets": [t["id"] for t in business_targets if variance_analysis.get(t["id"], {}).get("status") == "critical"],
            "mitigation_required": True
        }

        tracking_data = BusinessTargetTracking(
            tracking_timestamp=datetime.utcnow(),
            period={
                "start_time": start_time_dt,
                "end_time": end_time,
                "duration_hours": time_range_hours
            },
            targets=business_targets,
            actual_performance=actual_performance,
            target_achievement=target_achievement,
            variance_analysis=variance_analysis,
            trend_projection=trend_projection,
            risk_assessment=risk_assessment
        )

        elapsed_ms = (time.time() - start_time) * 1000
        logger.debug(f"Generated business target tracking in {elapsed_ms:.1f}ms")

        return tracking_data

    except Exception as e:
        logger.error(f"Error getting business target tracking: {e}")
        raise HTTPException(status_code=500, detail="Failed to retrieve business target tracking")


# ============================================================================
# Initialization Function
# ============================================================================

def initialize_performance_endpoints(db_session_maker: sessionmaker):
    """Initialize the performance endpoints with database session maker."""
    global session_maker
    session_maker = db_session_maker
    logger.info("✅ Performance endpoints initialized with database session maker")


# ============================================================================
# Module exports
# ============================================================================

__all__ = [
    "router",
    "initialize_performance_endpoints",
    "AgentPerformanceComparison",
    "DetailedAgentPerformance",
    "PerformanceTrend",
    "BenchmarkData",
    "AgentQualityMetrics",
    "QualityDegradationAlert",
    "BusinessTargetTracking",
    "PerformanceMetricType",
    "QualityMetricType",
    "TimeRange",
    "AggregationType"
]
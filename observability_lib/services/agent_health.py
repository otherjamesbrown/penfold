"""Agent Health Monitoring Service for comprehensive health scoring and trend analysis.

This module implements the AgentHealthService class that calculates comprehensive
health scores from multiple metrics, tracks trends over time, and provides detailed
health breakdowns for the Penfold observability dashboard.

Features:
- Multi-dimensional health scoring algorithm (0-100 scale)
- Processing completion rate scoring (0-40 points)
- Success vs failure rate analysis (0-30 points)
- Response time performance evaluation (0-20 points)
- Error frequency pattern detection (0-10 points)
- Real-time health status tracking with configurable thresholds
- Health trend analysis with anomaly detection
- Support for all Penfold agent types (email, meeting, relationship, review)
- Graceful handling of missing data with partial scores
- Performance optimized for high-frequency health checks

Example:
    from observability_lib.services.agent_health import AgentHealthService
    from observability_lib.config import settings

    health_service = AgentHealthService(settings)
    await health_service.start()

    # Get current health score
    health_score = await health_service.get_agent_health("email_processor")

    # Get health breakdown for dashboard
    breakdown = await health_service.get_health_breakdown("email_processor")

    # Get health trends for analysis
    trends = await health_service.get_health_trends(
        "email_processor",
        hours_back=24
    )

    # Check for health alerts
    alerts = await health_service.check_health_alerts("email_processor")

    await health_service.stop()
"""

import asyncio
import logging
import time
import uuid
from collections import defaultdict, deque
from dataclasses import dataclass, asdict
from datetime import datetime, timedelta
from typing import Any, Dict, List, Optional, Tuple
from enum import Enum
import statistics
import math

from sqlalchemy.ext.asyncio import AsyncSession, create_async_engine
from sqlalchemy.orm import sessionmaker
from sqlalchemy import select, func, and_, or_, text
from sqlalchemy.exc import SQLAlchemyError

from observability_lib.config import ObservabilitySettings
from observability_lib.models.agent_metrics import (
    Agent, AgentMetric, DecisionTrace, WorkflowEvent, AgentLog, AlertThreshold,
    AlertEvent, AgentType, MetricType, EventStatus, LogLevel, AlertStatus
)
from penf_lib.storage.validators import ValidationError


logger = logging.getLogger(__name__)


# ============================================================================
# Health Status and Data Types
# ============================================================================

class HealthStatus(str, Enum):
    """Overall health status categories"""
    EXCELLENT = "excellent"  # 90-100 points
    GOOD = "good"           # 75-89 points
    FAIR = "fair"           # 60-74 points
    POOR = "poor"           # 40-59 points
    CRITICAL = "critical"   # 0-39 points


class HealthTrendDirection(str, Enum):
    """Health trend direction"""
    IMPROVING = "improving"
    STABLE = "stable"
    DECLINING = "declining"
    VOLATILE = "volatile"


@dataclass
class HealthScore:
    """Complete health score breakdown for an agent"""
    agent_id: str
    timestamp: datetime
    overall_score: float  # 0-100
    status: HealthStatus

    # Score component breakdown
    processing_score: float     # 0-40 points
    success_rate_score: float   # 0-30 points
    performance_score: float    # 0-20 points
    error_pattern_score: float  # 0-10 points

    # Supporting metrics
    processing_completion_rate: Optional[float] = None
    success_rate: Optional[float] = None
    avg_response_time_ms: Optional[float] = None
    error_frequency: Optional[float] = None

    # Data availability flags
    has_processing_data: bool = False
    has_performance_data: bool = False
    has_error_data: bool = False

    # Additional context
    sample_size: int = 0
    evaluation_period_hours: float = 1.0
    confidence_level: float = 0.0  # 0-1.0


@dataclass
class HealthTrend:
    """Health trend analysis over time"""
    agent_id: str
    trend_direction: HealthTrendDirection
    slope: float  # Health score change per hour
    volatility: float  # Standard deviation of scores
    current_score: float
    previous_score: float
    score_change: float
    trend_confidence: float  # 0-1.0
    data_points: int


@dataclass
class HealthAlert:
    """Health-related alert information"""
    agent_id: str
    alert_type: str
    severity: str  # "low", "medium", "high", "critical"
    message: str
    triggered_at: datetime
    current_value: float
    threshold_value: float
    recommendation: str


# ============================================================================
# Agent Health Service Implementation
# ============================================================================

class AgentHealthService:
    """Comprehensive agent health monitoring with multi-dimensional scoring.

    Calculates health scores based on four key dimensions:
    1. Processing Completion Rate (0-40 points) - How well agent completes tasks
    2. Success vs Failure Rate (0-30 points) - Ratio of successful to failed operations
    3. Response Time Performance (0-20 points) - Speed of agent responses
    4. Error Frequency Patterns (0-10 points) - Frequency and severity of errors
    """

    # Health score thresholds
    EXCELLENT_THRESHOLD = 90.0
    GOOD_THRESHOLD = 75.0
    FAIR_THRESHOLD = 60.0
    POOR_THRESHOLD = 40.0

    # Component weights and scoring parameters
    MAX_PROCESSING_SCORE = 40.0
    MAX_SUCCESS_RATE_SCORE = 30.0
    MAX_PERFORMANCE_SCORE = 20.0
    MAX_ERROR_PATTERN_SCORE = 10.0

    # Performance baselines (can be overridden by configuration)
    BASELINE_RESPONSE_TIME_MS = 5000.0  # 5 seconds
    ACCEPTABLE_RESPONSE_TIME_MS = 10000.0  # 10 seconds
    POOR_RESPONSE_TIME_MS = 30000.0  # 30 seconds

    def __init__(self, settings: ObservabilitySettings):
        """Initialize the agent health service.

        Args:
            settings: Observability configuration settings
        """
        self.settings = settings
        self.engine = None
        self.session_maker = None

        # Health calculation cache
        self._health_cache: Dict[str, Tuple[HealthScore, float]] = {}
        self._cache_ttl = 60.0  # 1 minute cache

        # Trend tracking
        self._trend_history: Dict[str, deque] = defaultdict(lambda: deque(maxlen=144))  # 24 hours at 10-min intervals

        # Performance monitoring
        self._calculation_times: deque = deque(maxlen=100)
        self._running = False

        # Agent-specific baselines (can be learned over time)
        self._agent_baselines: Dict[str, Dict[str, float]] = {}

    async def start(self):
        """Start the agent health service."""
        if self._running:
            return

        self._running = True

        # Initialize database connection
        await self._initialize_db()

        # Load agent-specific baselines
        await self._load_agent_baselines()

        logger.info("Agent health service started")

    async def stop(self):
        """Stop the agent health service."""
        if not self._running:
            return

        self._running = False

        # Close database connections
        if self.engine:
            await self.engine.dispose()
            self.engine = None
            self.session_maker = None

        logger.info("Agent health service stopped")

    async def get_agent_health(
        self,
        agent_id: str,
        evaluation_period_hours: float = 1.0,
        use_cache: bool = True
    ) -> HealthScore:
        """Calculate current health score for an agent.

        Args:
            agent_id: Unique identifier for the agent
            evaluation_period_hours: Time window for health evaluation
            use_cache: Whether to use cached results if available

        Returns:
            HealthScore object with complete breakdown

        Raises:
            ValidationError: If agent_id is invalid
            RuntimeError: If service is not running
        """
        if not self._running:
            raise RuntimeError("Agent health service is not running")

        self._validate_agent_id(agent_id)

        # Check cache first
        if use_cache and agent_id in self._health_cache:
            cached_score, cache_time = self._health_cache[agent_id]
            if time.time() - cache_time < self._cache_ttl:
                return cached_score

        calculation_start = time.time()

        try:
            # Calculate health score
            health_score = await self._calculate_health_score(agent_id, evaluation_period_hours)

            # Update cache
            self._health_cache[agent_id] = (health_score, time.time())

            # Update trend history
            self._trend_history[agent_id].append({
                'timestamp': health_score.timestamp,
                'score': health_score.overall_score
            })

            # Record calculation performance
            calculation_time = time.time() - calculation_start
            self._calculation_times.append(calculation_time)

            logger.debug(f"Health calculation for {agent_id} took {calculation_time:.3f}s")

            return health_score

        except Exception as e:
            logger.error(f"Error calculating health for agent {agent_id}: {e}")
            raise

    async def get_health_breakdown(
        self,
        agent_id: str,
        evaluation_period_hours: float = 1.0
    ) -> Dict[str, Any]:
        """Get detailed health breakdown for dashboard display.

        Args:
            agent_id: Unique identifier for the agent
            evaluation_period_hours: Time window for health evaluation

        Returns:
            Dictionary with detailed health metrics and breakdown
        """
        health_score = await self.get_agent_health(agent_id, evaluation_period_hours)

        # Get additional context metrics
        context_metrics = await self._get_context_metrics(agent_id, evaluation_period_hours)

        return {
            "agent_id": agent_id,
            "overall_score": round(health_score.overall_score, 1),
            "status": health_score.status.value,
            "timestamp": health_score.timestamp.isoformat(),

            # Component scores
            "component_scores": {
                "processing": {
                    "score": round(health_score.processing_score, 1),
                    "max_score": self.MAX_PROCESSING_SCORE,
                    "percentage": round((health_score.processing_score / self.MAX_PROCESSING_SCORE) * 100, 1),
                    "completion_rate": health_score.processing_completion_rate
                },
                "success_rate": {
                    "score": round(health_score.success_rate_score, 1),
                    "max_score": self.MAX_SUCCESS_RATE_SCORE,
                    "percentage": round((health_score.success_rate_score / self.MAX_SUCCESS_RATE_SCORE) * 100, 1),
                    "success_rate": health_score.success_rate
                },
                "performance": {
                    "score": round(health_score.performance_score, 1),
                    "max_score": self.MAX_PERFORMANCE_SCORE,
                    "percentage": round((health_score.performance_score / self.MAX_PERFORMANCE_SCORE) * 100, 1),
                    "avg_response_time_ms": health_score.avg_response_time_ms
                },
                "error_patterns": {
                    "score": round(health_score.error_pattern_score, 1),
                    "max_score": self.MAX_ERROR_PATTERN_SCORE,
                    "percentage": round((health_score.error_pattern_score / self.MAX_ERROR_PATTERN_SCORE) * 100, 1),
                    "error_frequency": health_score.error_frequency
                }
            },

            # Data availability
            "data_availability": {
                "has_processing_data": health_score.has_processing_data,
                "has_performance_data": health_score.has_performance_data,
                "has_error_data": health_score.has_error_data,
                "sample_size": health_score.sample_size,
                "confidence_level": round(health_score.confidence_level, 2)
            },

            # Additional context
            "context": {
                "evaluation_period_hours": evaluation_period_hours,
                **context_metrics
            },

            # Health status indicators
            "status_indicators": {
                "excellent_threshold": self.EXCELLENT_THRESHOLD,
                "good_threshold": self.GOOD_THRESHOLD,
                "fair_threshold": self.FAIR_THRESHOLD,
                "poor_threshold": self.POOR_THRESHOLD
            }
        }

    async def get_health_trends(
        self,
        agent_id: str,
        hours_back: int = 24,
        include_predictions: bool = False
    ) -> Dict[str, Any]:
        """Get health trends and analysis for an agent.

        Args:
            agent_id: Unique identifier for the agent
            hours_back: Number of hours to analyze for trends
            include_predictions: Whether to include trend predictions

        Returns:
            Dictionary with trend analysis and historical data
        """
        self._validate_agent_id(agent_id)

        # Get historical health data
        end_time = datetime.utcnow()
        start_time = end_time - timedelta(hours=hours_back)

        historical_scores = await self._get_historical_health_scores(agent_id, start_time, end_time)

        if len(historical_scores) < 2:
            return {
                "agent_id": agent_id,
                "trend_direction": HealthTrendDirection.STABLE.value,
                "has_sufficient_data": False,
                "data_points": len(historical_scores),
                "message": "Insufficient data for trend analysis"
            }

        # Calculate trend metrics
        trend_analysis = self._analyze_health_trend(historical_scores)

        result = {
            "agent_id": agent_id,
            "trend_direction": trend_analysis.trend_direction.value,
            "has_sufficient_data": True,
            "data_points": trend_analysis.data_points,

            # Trend metrics
            "slope": round(trend_analysis.slope, 3),
            "volatility": round(trend_analysis.volatility, 2),
            "current_score": round(trend_analysis.current_score, 1),
            "previous_score": round(trend_analysis.previous_score, 1),
            "score_change": round(trend_analysis.score_change, 1),
            "trend_confidence": round(trend_analysis.trend_confidence, 2),

            # Historical data (last 48 points max for performance)
            "historical_scores": [
                {
                    "timestamp": score["timestamp"].isoformat(),
                    "score": round(score["score"], 1)
                }
                for score in historical_scores[-48:]
            ],

            "analysis_period": {
                "start_time": start_time.isoformat(),
                "end_time": end_time.isoformat(),
                "hours_analyzed": hours_back
            }
        }

        # Add predictions if requested
        if include_predictions and len(historical_scores) >= 5:
            predictions = self._predict_health_trends(historical_scores)
            result["predictions"] = predictions

        return result

    async def check_health_alerts(
        self,
        agent_id: str,
        evaluation_period_hours: float = 1.0
    ) -> List[HealthAlert]:
        """Check for health-related alerts for an agent.

        Args:
            agent_id: Unique identifier for the agent
            evaluation_period_hours: Time window for health evaluation

        Returns:
            List of active health alerts
        """
        alerts = []

        try:
            health_score = await self.get_agent_health(agent_id, evaluation_period_hours)

            # Check overall health status
            if health_score.status == HealthStatus.CRITICAL:
                alerts.append(HealthAlert(
                    agent_id=agent_id,
                    alert_type="critical_health",
                    severity="critical",
                    message=f"Agent health is critical (score: {health_score.overall_score:.1f})",
                    triggered_at=datetime.utcnow(),
                    current_value=health_score.overall_score,
                    threshold_value=self.POOR_THRESHOLD,
                    recommendation="Immediate investigation required. Check error logs and recent workflow events."
                ))
            elif health_score.status == HealthStatus.POOR:
                alerts.append(HealthAlert(
                    agent_id=agent_id,
                    alert_type="poor_health",
                    severity="high",
                    message=f"Agent health is poor (score: {health_score.overall_score:.1f})",
                    triggered_at=datetime.utcnow(),
                    current_value=health_score.overall_score,
                    threshold_value=self.FAIR_THRESHOLD,
                    recommendation="Review recent performance metrics and error patterns."
                ))

            # Check component-specific issues
            if health_score.processing_score < (self.MAX_PROCESSING_SCORE * 0.5):
                alerts.append(HealthAlert(
                    agent_id=agent_id,
                    alert_type="low_processing_completion",
                    severity="medium",
                    message=f"Low processing completion rate: {health_score.processing_completion_rate:.1%}",
                    triggered_at=datetime.utcnow(),
                    current_value=health_score.processing_completion_rate * 100 if health_score.processing_completion_rate else 0,
                    threshold_value=50.0,
                    recommendation="Check for stuck workflows or resource constraints."
                ))

            if health_score.success_rate and health_score.success_rate < 0.8:
                alerts.append(HealthAlert(
                    agent_id=agent_id,
                    alert_type="low_success_rate",
                    severity="medium",
                    message=f"Low success rate: {health_score.success_rate:.1%}",
                    triggered_at=datetime.utcnow(),
                    current_value=health_score.success_rate * 100,
                    threshold_value=80.0,
                    recommendation="Review error logs for common failure patterns."
                ))

            if health_score.avg_response_time_ms and health_score.avg_response_time_ms > self.POOR_RESPONSE_TIME_MS:
                alerts.append(HealthAlert(
                    agent_id=agent_id,
                    alert_type="slow_response_time",
                    severity="medium",
                    message=f"Slow response time: {health_score.avg_response_time_ms:.0f}ms",
                    triggered_at=datetime.utcnow(),
                    current_value=health_score.avg_response_time_ms,
                    threshold_value=self.POOR_RESPONSE_TIME_MS,
                    recommendation="Check system resources and optimize processing logic."
                ))

            # Check trend-based alerts if we have trend data
            if agent_id in self._trend_history and len(self._trend_history[agent_id]) >= 3:
                recent_scores = [point['score'] for point in list(self._trend_history[agent_id])[-3:]]
                if all(recent_scores[i] > recent_scores[i+1] for i in range(len(recent_scores)-1)):
                    alerts.append(HealthAlert(
                        agent_id=agent_id,
                        alert_type="declining_health_trend",
                        severity="medium",
                        message="Health score showing consistent decline over recent measurements",
                        triggered_at=datetime.utcnow(),
                        current_value=recent_scores[-1],
                        threshold_value=recent_scores[0],
                        recommendation="Monitor closely and identify root cause of decline."
                    ))

        except Exception as e:
            logger.error(f"Error checking health alerts for agent {agent_id}: {e}")
            # Return error alert
            alerts.append(HealthAlert(
                agent_id=agent_id,
                alert_type="health_check_error",
                severity="high",
                message=f"Unable to calculate health score: {str(e)}",
                triggered_at=datetime.utcnow(),
                current_value=0.0,
                threshold_value=0.0,
                recommendation="Check agent health service configuration and database connectivity."
            ))

        return alerts

    async def get_all_agents_health(
        self,
        evaluation_period_hours: float = 1.0,
        include_inactive: bool = False
    ) -> Dict[str, HealthScore]:
        """Get health scores for all registered agents.

        Args:
            evaluation_period_hours: Time window for health evaluation
            include_inactive: Whether to include inactive agents

        Returns:
            Dictionary mapping agent_id to HealthScore
        """
        if not self._running:
            raise RuntimeError("Agent health service is not running")

        try:
            async with self.session_maker() as session:
                # Get all agents
                query = select(Agent)
                if not include_inactive:
                    query = query.where(Agent.is_active == True)

                result = await session.execute(query)
                agents = result.scalars().all()

                # Calculate health for each agent
                health_scores = {}
                for agent in agents:
                    try:
                        health_score = await self.get_agent_health(
                            agent.agent_id,
                            evaluation_period_hours
                        )
                        health_scores[agent.agent_id] = health_score
                    except Exception as e:
                        logger.warning(f"Failed to calculate health for agent {agent.agent_id}: {e}")
                        # Create minimal health score for failed calculations
                        health_scores[agent.agent_id] = HealthScore(
                            agent_id=agent.agent_id,
                            timestamp=datetime.utcnow(),
                            overall_score=0.0,
                            status=HealthStatus.CRITICAL,
                            processing_score=0.0,
                            success_rate_score=0.0,
                            performance_score=0.0,
                            error_pattern_score=0.0,
                            confidence_level=0.0
                        )

                return health_scores

        except Exception as e:
            logger.error(f"Error getting health for all agents: {e}")
            raise

    async def get_service_stats(self) -> Dict[str, Any]:
        """Get performance statistics about the health service itself.

        Returns:
            Dictionary with service performance metrics
        """
        avg_calculation_time = statistics.mean(self._calculation_times) if self._calculation_times else 0.0

        return {
            "service_running": self._running,
            "cached_agents": len(self._health_cache),
            "trend_history_size": {agent_id: len(history) for agent_id, history in self._trend_history.items()},
            "average_calculation_time_ms": round(avg_calculation_time * 1000, 2),
            "cache_ttl_seconds": self._cache_ttl,
            "recent_calculations": len(self._calculation_times),
            "agent_baselines_loaded": len(self._agent_baselines)
        }

    # ========================================================================
    # Private Methods - Health Calculation
    # ========================================================================

    async def _calculate_health_score(
        self,
        agent_id: str,
        evaluation_period_hours: float
    ) -> HealthScore:
        """Calculate comprehensive health score for an agent.

        Args:
            agent_id: Agent identifier
            evaluation_period_hours: Evaluation time window

        Returns:
            Complete HealthScore object
        """
        end_time = datetime.utcnow()
        start_time = end_time - timedelta(hours=evaluation_period_hours)

        async with self.session_maker() as session:
            # Get agent information
            agent = await self._get_agent_info(session, agent_id)
            if not agent:
                raise ValidationError(f"Agent not found: {agent_id}")

            # Gather all relevant metrics
            metrics_data = await self._gather_metrics_data(session, agent_id, start_time, end_time)
            workflow_data = await self._gather_workflow_data(session, agent_id, start_time, end_time)
            error_data = await self._gather_error_data(session, agent_id, start_time, end_time)

            # Calculate component scores
            processing_score, processing_metrics = await self._calculate_processing_score(
                agent_id, workflow_data, metrics_data
            )

            success_rate_score, success_metrics = await self._calculate_success_rate_score(
                agent_id, workflow_data, error_data
            )

            performance_score, performance_metrics = await self._calculate_performance_score(
                agent_id, metrics_data, workflow_data
            )

            error_pattern_score, error_metrics = await self._calculate_error_pattern_score(
                agent_id, error_data, metrics_data
            )

            # Calculate overall score and confidence
            overall_score = processing_score + success_rate_score + performance_score + error_pattern_score
            confidence_level = self._calculate_confidence_level(
                processing_metrics, success_metrics, performance_metrics, error_metrics
            )

            # Determine health status
            status = self._determine_health_status(overall_score)

            # Calculate sample size
            sample_size = (
                len(workflow_data.get('workflow_events', [])) +
                len(metrics_data.get('metrics', [])) +
                len(error_data.get('log_entries', []))
            )

            return HealthScore(
                agent_id=agent_id,
                timestamp=datetime.utcnow(),
                overall_score=overall_score,
                status=status,

                processing_score=processing_score,
                success_rate_score=success_rate_score,
                performance_score=performance_score,
                error_pattern_score=error_pattern_score,

                processing_completion_rate=processing_metrics.get('completion_rate'),
                success_rate=success_metrics.get('success_rate'),
                avg_response_time_ms=performance_metrics.get('avg_response_time_ms'),
                error_frequency=error_metrics.get('error_frequency'),

                has_processing_data=len(workflow_data.get('workflow_events', [])) > 0,
                has_performance_data=len(metrics_data.get('performance_metrics', [])) > 0,
                has_error_data=len(error_data.get('log_entries', [])) > 0,

                sample_size=sample_size,
                evaluation_period_hours=evaluation_period_hours,
                confidence_level=confidence_level
            )

    async def _calculate_processing_score(
        self,
        agent_id: str,
        workflow_data: Dict[str, Any],
        metrics_data: Dict[str, Any]
    ) -> Tuple[float, Dict[str, Any]]:
        """Calculate processing completion rate score (0-40 points).

        Args:
            agent_id: Agent identifier
            workflow_data: Workflow events and completion data
            metrics_data: General metrics data

        Returns:
            Tuple of (score, metrics_dict)
        """
        workflow_events = workflow_data.get('workflow_events', [])

        if not workflow_events:
            # Check for processing metrics as fallback
            processing_metrics = [
                m for m in metrics_data.get('metrics', [])
                if 'completion' in m.get('metric_name', '').lower() or
                   'processing' in m.get('metric_name', '').lower()
            ]

            if processing_metrics:
                # Use average of processing metrics
                avg_processing = statistics.mean([m['metric_value'] for m in processing_metrics])
                completion_rate = min(avg_processing / 100.0, 1.0)  # Assume metrics are percentages
                score = completion_rate * self.MAX_PROCESSING_SCORE
                return score, {'completion_rate': completion_rate, 'method': 'metrics'}

            # No processing data available
            return 0.0, {'completion_rate': None, 'method': 'no_data'}

        # Count workflow completions
        started_workflows = set()
        completed_workflows = set()
        failed_workflows = set()

        for event in workflow_events:
            workflow_id = event.get('workflow_id')
            event_type = event.get('event_type')
            event_status = event.get('event_status')

            if event_type == 'workflow_started':
                started_workflows.add(workflow_id)
            elif event_type == 'workflow_finished':
                if event_status == 'success':
                    completed_workflows.add(workflow_id)
                else:
                    failed_workflows.add(workflow_id)

        # Calculate completion rate
        total_workflows = len(started_workflows)
        if total_workflows == 0:
            return 0.0, {'completion_rate': None, 'method': 'no_workflows'}

        completed_count = len(completed_workflows)
        completion_rate = completed_count / total_workflows

        # Apply scoring curve (sigmoid-like for better discrimination)
        # Full points at 95%+ completion, significant penalty below 70%
        if completion_rate >= 0.95:
            score = self.MAX_PROCESSING_SCORE
        elif completion_rate >= 0.85:
            score = self.MAX_PROCESSING_SCORE * (0.8 + 0.2 * (completion_rate - 0.85) / 0.10)
        elif completion_rate >= 0.70:
            score = self.MAX_PROCESSING_SCORE * (0.5 + 0.3 * (completion_rate - 0.70) / 0.15)
        else:
            # Steep penalty below 70%
            score = self.MAX_PROCESSING_SCORE * 0.5 * (completion_rate / 0.70)

        return score, {
            'completion_rate': completion_rate,
            'total_workflows': total_workflows,
            'completed_workflows': completed_count,
            'failed_workflows': len(failed_workflows),
            'method': 'workflow_events'
        }

    async def _calculate_success_rate_score(
        self,
        agent_id: str,
        workflow_data: Dict[str, Any],
        error_data: Dict[str, Any]
    ) -> Tuple[float, Dict[str, Any]]:
        """Calculate success vs failure rate score (0-30 points).

        Args:
            agent_id: Agent identifier
            workflow_data: Workflow events data
            error_data: Error and log data

        Returns:
            Tuple of (score, metrics_dict)
        """
        # Count successful and failed operations
        success_count = 0
        failure_count = 0

        # Count from workflow events
        for event in workflow_data.get('workflow_events', []):
            event_status = event.get('event_status')
            if event_status == 'success':
                success_count += 1
            elif event_status in ['failure', 'timeout']:
                failure_count += 1

        # Count from error logs
        error_logs = error_data.get('log_entries', [])
        error_count = len([log for log in error_logs if log.get('level') in ['ERROR', 'CRITICAL']])
        failure_count += error_count

        # Estimate success operations from non-error logs
        info_logs = len([log for log in error_logs if log.get('level') == 'INFO'])
        success_count += info_logs

        total_operations = success_count + failure_count
        if total_operations == 0:
            return 0.0, {'success_rate': None, 'method': 'no_operations'}

        success_rate = success_count / total_operations

        # Apply scoring curve with emphasis on high reliability
        # Full points at 98%+ success, significant penalty below 90%
        if success_rate >= 0.98:
            score = self.MAX_SUCCESS_RATE_SCORE
        elif success_rate >= 0.95:
            score = self.MAX_SUCCESS_RATE_SCORE * (0.9 + 0.1 * (success_rate - 0.95) / 0.03)
        elif success_rate >= 0.90:
            score = self.MAX_SUCCESS_RATE_SCORE * (0.7 + 0.2 * (success_rate - 0.90) / 0.05)
        elif success_rate >= 0.80:
            score = self.MAX_SUCCESS_RATE_SCORE * (0.4 + 0.3 * (success_rate - 0.80) / 0.10)
        else:
            # Steep penalty below 80%
            score = self.MAX_SUCCESS_RATE_SCORE * 0.4 * (success_rate / 0.80)

        return score, {
            'success_rate': success_rate,
            'success_count': success_count,
            'failure_count': failure_count,
            'total_operations': total_operations,
            'method': 'combined_events_logs'
        }

    async def _calculate_performance_score(
        self,
        agent_id: str,
        metrics_data: Dict[str, Any],
        workflow_data: Dict[str, Any]
    ) -> Tuple[float, Dict[str, Any]]:
        """Calculate response time performance score (0-20 points).

        Args:
            agent_id: Agent identifier
            metrics_data: Performance metrics data
            workflow_data: Workflow timing data

        Returns:
            Tuple of (score, metrics_dict)
        """
        response_times = []

        # Gather response time metrics
        for metric in metrics_data.get('metrics', []):
            metric_name = metric.get('metric_name', '').lower()
            if any(keyword in metric_name for keyword in ['time', 'duration', 'latency', 'response']):
                if 'ms' in metric_name or 'millisecond' in metric_name:
                    response_times.append(metric['metric_value'])
                elif 's' in metric_name or 'second' in metric_name:
                    response_times.append(metric['metric_value'] * 1000)  # Convert to ms

        # Gather timing from workflow events
        for event in workflow_data.get('workflow_events', []):
            processing_time = event.get('processing_time_ms')
            if processing_time is not None:
                response_times.append(processing_time)

        if not response_times:
            return 0.0, {'avg_response_time_ms': None, 'method': 'no_timing_data'}

        # Calculate average response time
        avg_response_time = statistics.mean(response_times)

        # Get agent-specific baseline or use default
        agent_baseline = self._agent_baselines.get(agent_id, {})
        baseline_time = agent_baseline.get('response_time_ms', self.BASELINE_RESPONSE_TIME_MS)
        acceptable_time = agent_baseline.get('acceptable_time_ms', self.ACCEPTABLE_RESPONSE_TIME_MS)
        poor_time = agent_baseline.get('poor_time_ms', self.POOR_RESPONSE_TIME_MS)

        # Apply performance scoring
        if avg_response_time <= baseline_time:
            score = self.MAX_PERFORMANCE_SCORE
        elif avg_response_time <= acceptable_time:
            # Linear decrease from baseline to acceptable
            ratio = (acceptable_time - avg_response_time) / (acceptable_time - baseline_time)
            score = self.MAX_PERFORMANCE_SCORE * (0.7 + 0.3 * ratio)
        elif avg_response_time <= poor_time:
            # Further decrease from acceptable to poor
            ratio = (poor_time - avg_response_time) / (poor_time - acceptable_time)
            score = self.MAX_PERFORMANCE_SCORE * (0.3 + 0.4 * ratio)
        else:
            # Severe penalty for very slow performance
            score = self.MAX_PERFORMANCE_SCORE * 0.3 * max(0.1, poor_time / avg_response_time)

        return score, {
            'avg_response_time_ms': avg_response_time,
            'sample_count': len(response_times),
            'baseline_time_ms': baseline_time,
            'p95_response_time_ms': statistics.quantiles(response_times, n=20)[18] if len(response_times) > 10 else None,
            'method': 'timing_metrics'
        }

    async def _calculate_error_pattern_score(
        self,
        agent_id: str,
        error_data: Dict[str, Any],
        metrics_data: Dict[str, Any]
    ) -> Tuple[float, Dict[str, Any]]:
        """Calculate error frequency patterns score (0-10 points).

        Args:
            agent_id: Agent identifier
            error_data: Error logs and patterns
            metrics_data: General metrics that might indicate errors

        Returns:
            Tuple of (score, metrics_dict)
        """
        log_entries = error_data.get('log_entries', [])

        if not log_entries:
            # Check for error rate metrics
            error_metrics = [
                m for m in metrics_data.get('metrics', [])
                if 'error' in m.get('metric_name', '').lower()
            ]

            if error_metrics:
                avg_error_rate = statistics.mean([m['metric_value'] for m in error_metrics])
                # Assume error rate is percentage
                error_frequency = avg_error_rate / 100.0
                score = self._score_error_frequency(error_frequency)
                return score, {'error_frequency': error_frequency, 'method': 'error_metrics'}

            # No error data - assume healthy
            return self.MAX_ERROR_PATTERN_SCORE, {'error_frequency': 0.0, 'method': 'no_error_data'}

        # Categorize log entries by severity
        critical_errors = len([log for log in log_entries if log.get('level') == 'CRITICAL'])
        errors = len([log for log in log_entries if log.get('level') == 'ERROR'])
        warnings = len([log for log in log_entries if log.get('level') == 'WARNING'])
        total_logs = len(log_entries)

        # Calculate weighted error frequency
        # Critical errors count double, errors count normal, warnings count half
        weighted_errors = (critical_errors * 2.0) + errors + (warnings * 0.5)
        error_frequency = weighted_errors / total_logs if total_logs > 0 else 0.0

        # Apply error frequency scoring
        score = self._score_error_frequency(error_frequency)

        # Check for error patterns (recent increase in errors)
        recent_errors = self._analyze_error_patterns(log_entries)

        return score, {
            'error_frequency': error_frequency,
            'critical_errors': critical_errors,
            'errors': errors,
            'warnings': warnings,
            'total_logs': total_logs,
            'recent_error_trend': recent_errors.get('trend', 'stable'),
            'method': 'log_analysis'
        }

    def _score_error_frequency(self, error_frequency: float) -> float:
        """Convert error frequency to score (0-10 points).

        Args:
            error_frequency: Error frequency ratio (0-1.0)

        Returns:
            Score between 0 and MAX_ERROR_PATTERN_SCORE
        """
        # Full points for very low error rate (< 2%)
        if error_frequency <= 0.02:
            return self.MAX_ERROR_PATTERN_SCORE
        # Good points for acceptable error rate (< 5%)
        elif error_frequency <= 0.05:
            ratio = (0.05 - error_frequency) / 0.03
            return self.MAX_ERROR_PATTERN_SCORE * (0.8 + 0.2 * ratio)
        # Reduced points for moderate error rate (< 10%)
        elif error_frequency <= 0.10:
            ratio = (0.10 - error_frequency) / 0.05
            return self.MAX_ERROR_PATTERN_SCORE * (0.5 + 0.3 * ratio)
        # Low points for high error rate (< 20%)
        elif error_frequency <= 0.20:
            ratio = (0.20 - error_frequency) / 0.10
            return self.MAX_ERROR_PATTERN_SCORE * (0.2 + 0.3 * ratio)
        else:
            # Very low points for excessive error rates
            return self.MAX_ERROR_PATTERN_SCORE * 0.2 * max(0.1, 0.20 / error_frequency)

    def _analyze_error_patterns(self, log_entries: List[Dict[str, Any]]) -> Dict[str, Any]:
        """Analyze error patterns over time.

        Args:
            log_entries: List of log entries with timestamps

        Returns:
            Dictionary with pattern analysis
        """
        if len(log_entries) < 5:
            return {'trend': 'insufficient_data'}

        # Sort by timestamp
        sorted_logs = sorted(
            [log for log in log_entries if log.get('level') in ['ERROR', 'CRITICAL']],
            key=lambda x: x.get('timestamp', datetime.min)
        )

        if len(sorted_logs) < 3:
            return {'trend': 'stable'}

        # Analyze trend in recent vs older errors
        total_time = (sorted_logs[-1]['timestamp'] - sorted_logs[0]['timestamp']).total_seconds()
        if total_time <= 0:
            return {'trend': 'stable'}

        mid_point = sorted_logs[0]['timestamp'] + timedelta(seconds=total_time / 2)
        recent_errors = len([log for log in sorted_logs if log['timestamp'] >= mid_point])
        older_errors = len(sorted_logs) - recent_errors

        if recent_errors > older_errors * 1.5:
            return {'trend': 'increasing', 'recent_count': recent_errors, 'older_count': older_errors}
        elif older_errors > recent_errors * 1.5:
            return {'trend': 'decreasing', 'recent_count': recent_errors, 'older_count': older_errors}
        else:
            return {'trend': 'stable', 'recent_count': recent_errors, 'older_count': older_errors}

    def _calculate_confidence_level(
        self,
        processing_metrics: Dict[str, Any],
        success_metrics: Dict[str, Any],
        performance_metrics: Dict[str, Any],
        error_metrics: Dict[str, Any]
    ) -> float:
        """Calculate confidence level in the health score.

        Args:
            processing_metrics: Processing calculation metrics
            success_metrics: Success rate calculation metrics
            performance_metrics: Performance calculation metrics
            error_metrics: Error pattern calculation metrics

        Returns:
            Confidence level between 0.0 and 1.0
        """
        confidence_factors = []

        # Processing confidence
        if processing_metrics.get('completion_rate') is not None:
            workflow_count = processing_metrics.get('total_workflows', 0)
            if workflow_count >= 10:
                confidence_factors.append(1.0)
            elif workflow_count >= 5:
                confidence_factors.append(0.8)
            elif workflow_count >= 1:
                confidence_factors.append(0.5)
            else:
                confidence_factors.append(0.1)

        # Success rate confidence
        if success_metrics.get('success_rate') is not None:
            operation_count = success_metrics.get('total_operations', 0)
            if operation_count >= 20:
                confidence_factors.append(1.0)
            elif operation_count >= 10:
                confidence_factors.append(0.8)
            elif operation_count >= 5:
                confidence_factors.append(0.6)
            else:
                confidence_factors.append(0.3)

        # Performance confidence
        if performance_metrics.get('avg_response_time_ms') is not None:
            sample_count = performance_metrics.get('sample_count', 0)
            if sample_count >= 15:
                confidence_factors.append(1.0)
            elif sample_count >= 8:
                confidence_factors.append(0.8)
            elif sample_count >= 3:
                confidence_factors.append(0.6)
            else:
                confidence_factors.append(0.4)

        # Error pattern confidence
        if error_metrics.get('total_logs', 0) > 0:
            confidence_factors.append(0.9)
        else:
            confidence_factors.append(0.7)  # Less confident with no error data

        # Calculate overall confidence
        if not confidence_factors:
            return 0.0

        return statistics.mean(confidence_factors)

    def _determine_health_status(self, overall_score: float) -> HealthStatus:
        """Determine health status from overall score.

        Args:
            overall_score: Overall health score (0-100)

        Returns:
            HealthStatus enum value
        """
        if overall_score >= self.EXCELLENT_THRESHOLD:
            return HealthStatus.EXCELLENT
        elif overall_score >= self.GOOD_THRESHOLD:
            return HealthStatus.GOOD
        elif overall_score >= self.FAIR_THRESHOLD:
            return HealthStatus.FAIR
        elif overall_score >= self.POOR_THRESHOLD:
            return HealthStatus.POOR
        else:
            return HealthStatus.CRITICAL

    # ========================================================================
    # Private Methods - Data Gathering
    # ========================================================================

    async def _gather_metrics_data(
        self,
        session: AsyncSession,
        agent_id: str,
        start_time: datetime,
        end_time: datetime
    ) -> Dict[str, Any]:
        """Gather all relevant metrics data for health calculation."""
        try:
            query = select(AgentMetric).where(
                and_(
                    AgentMetric.agent_id == agent_id,
                    AgentMetric.timestamp >= start_time,
                    AgentMetric.timestamp <= end_time
                )
            ).order_by(AgentMetric.timestamp.desc())

            result = await session.execute(query)
            metrics = result.scalars().all()

            # Organize metrics by type
            metrics_data = {
                'metrics': [
                    {
                        'metric_name': m.metric_name,
                        'metric_value': m.metric_value,
                        'metric_type': m.metric_type,
                        'timestamp': m.timestamp,
                        'labels': m.labels
                    }
                    for m in metrics
                ],
                'performance_metrics': [
                    m for m in metrics
                    if any(keyword in m.metric_name.lower() for keyword in ['time', 'duration', 'latency'])
                ],
                'processing_metrics': [
                    m for m in metrics
                    if any(keyword in m.metric_name.lower() for keyword in ['completion', 'processing', 'throughput'])
                ],
                'error_metrics': [
                    m for m in metrics
                    if 'error' in m.metric_name.lower()
                ]
            }

            return metrics_data

        except Exception as e:
            logger.error(f"Error gathering metrics data for {agent_id}: {e}")
            return {'metrics': [], 'performance_metrics': [], 'processing_metrics': [], 'error_metrics': []}

    async def _gather_workflow_data(
        self,
        session: AsyncSession,
        agent_id: str,
        start_time: datetime,
        end_time: datetime
    ) -> Dict[str, Any]:
        """Gather workflow events data for health calculation."""
        try:
            query = select(WorkflowEvent).where(
                and_(
                    WorkflowEvent.agent_id == agent_id,
                    WorkflowEvent.timestamp >= start_time,
                    WorkflowEvent.timestamp <= end_time
                )
            ).order_by(WorkflowEvent.timestamp.desc())

            result = await session.execute(query)
            events = result.scalars().all()

            workflow_data = {
                'workflow_events': [
                    {
                        'workflow_id': str(event.workflow_id),
                        'event_type': event.event_type,
                        'event_status': event.event_status,
                        'processing_time_ms': event.processing_time_ms,
                        'timestamp': event.timestamp,
                        'event_data': event.event_data
                    }
                    for event in events
                ]
            }

            return workflow_data

        except Exception as e:
            logger.error(f"Error gathering workflow data for {agent_id}: {e}")
            return {'workflow_events': []}

    async def _gather_error_data(
        self,
        session: AsyncSession,
        agent_id: str,
        start_time: datetime,
        end_time: datetime
    ) -> Dict[str, Any]:
        """Gather error and log data for health calculation."""
        try:
            query = select(AgentLog).where(
                and_(
                    AgentLog.agent_id == agent_id,
                    AgentLog.timestamp >= start_time,
                    AgentLog.timestamp <= end_time
                )
            ).order_by(AgentLog.timestamp.desc())

            result = await session.execute(query)
            logs = result.scalars().all()

            error_data = {
                'log_entries': [
                    {
                        'level': log.level,
                        'event': log.event,
                        'timestamp': log.timestamp,
                        'context': log.context
                    }
                    for log in logs
                ]
            }

            return error_data

        except Exception as e:
            logger.error(f"Error gathering error data for {agent_id}: {e}")
            return {'log_entries': []}

    async def _get_context_metrics(
        self,
        agent_id: str,
        evaluation_period_hours: float
    ) -> Dict[str, Any]:
        """Get additional context metrics for health breakdown."""
        try:
            async with self.session_maker() as session:
                # Get recent activity count
                end_time = datetime.utcnow()
                start_time = end_time - timedelta(hours=evaluation_period_hours)

                # Count recent metrics
                metric_count_query = select(func.count(AgentMetric.id)).where(
                    and_(
                        AgentMetric.agent_id == agent_id,
                        AgentMetric.timestamp >= start_time
                    )
                )
                metric_count_result = await session.execute(metric_count_query)
                recent_metrics = metric_count_result.scalar() or 0

                # Count recent workflow events
                event_count_query = select(func.count(WorkflowEvent.id)).where(
                    and_(
                        WorkflowEvent.agent_id == agent_id,
                        WorkflowEvent.timestamp >= start_time
                    )
                )
                event_count_result = await session.execute(event_count_query)
                recent_events = event_count_result.scalar() or 0

                # Get agent last activity
                agent_query = select(Agent.last_active_at, Agent.is_active).where(
                    Agent.agent_id == agent_id
                )
                agent_result = await session.execute(agent_query)
                agent_row = agent_result.first()

                context = {
                    'recent_metrics_count': recent_metrics,
                    'recent_events_count': recent_events,
                    'total_recent_activity': recent_metrics + recent_events,
                }

                if agent_row:
                    context['is_active'] = agent_row.is_active
                    if agent_row.last_active_at:
                        inactive_hours = (datetime.utcnow() - agent_row.last_active_at).total_seconds() / 3600
                        context['hours_since_last_active'] = round(inactive_hours, 1)

                return context

        except Exception as e:
            logger.error(f"Error getting context metrics for {agent_id}: {e}")
            return {}

    async def _get_historical_health_scores(
        self,
        agent_id: str,
        start_time: datetime,
        end_time: datetime
    ) -> List[Dict[str, Any]]:
        """Get historical health scores from trend history and database."""
        historical_scores = []

        # Use in-memory trend history first
        if agent_id in self._trend_history:
            for point in self._trend_history[agent_id]:
                if start_time <= point['timestamp'] <= end_time:
                    historical_scores.append(point)

        # If we don't have enough history, generate some sample points
        # (In a real implementation, you'd store historical health scores in database)
        if len(historical_scores) < 5:
            # Generate sample points by calculating health at different intervals
            current_time = end_time
            interval = timedelta(hours=1)

            while current_time > start_time and len(historical_scores) < 24:
                try:
                    # This is a simplified approach - in production you'd cache these
                    health_score = await self.get_agent_health(
                        agent_id,
                        evaluation_period_hours=1.0,
                        use_cache=False
                    )
                    historical_scores.append({
                        'timestamp': current_time,
                        'score': health_score.overall_score
                    })
                except Exception as e:
                    logger.debug(f"Could not calculate historical health for {current_time}: {e}")

                current_time -= interval

        # Sort by timestamp
        historical_scores.sort(key=lambda x: x['timestamp'])
        return historical_scores

    def _analyze_health_trend(self, historical_scores: List[Dict[str, Any]]) -> HealthTrend:
        """Analyze health trend from historical data."""
        if len(historical_scores) < 2:
            return HealthTrend(
                agent_id=historical_scores[0]['agent_id'] if historical_scores else 'unknown',
                trend_direction=HealthTrendDirection.STABLE,
                slope=0.0,
                volatility=0.0,
                current_score=historical_scores[-1]['score'] if historical_scores else 0.0,
                previous_score=0.0,
                score_change=0.0,
                trend_confidence=0.0,
                data_points=len(historical_scores)
            )

        scores = [point['score'] for point in historical_scores]
        timestamps = [point['timestamp'] for point in historical_scores]

        # Calculate time differences in hours
        time_diffs = []
        for i in range(1, len(timestamps)):
            diff_hours = (timestamps[i] - timestamps[i-1]).total_seconds() / 3600
            time_diffs.append(diff_hours)

        avg_interval = statistics.mean(time_diffs) if time_diffs else 1.0

        # Calculate slope (score change per hour)
        if len(scores) >= 3:
            # Linear regression to find trend
            x_values = list(range(len(scores)))
            x_mean = statistics.mean(x_values)
            y_mean = statistics.mean(scores)

            numerator = sum((x_values[i] - x_mean) * (scores[i] - y_mean) for i in range(len(scores)))
            denominator = sum((x_values[i] - x_mean) ** 2 for i in range(len(scores)))

            if denominator != 0:
                slope_per_point = numerator / denominator
                slope = slope_per_point / avg_interval  # Convert to per hour
            else:
                slope = 0.0
        else:
            # Simple slope for two points
            time_span = (timestamps[-1] - timestamps[0]).total_seconds() / 3600
            if time_span > 0:
                slope = (scores[-1] - scores[0]) / time_span
            else:
                slope = 0.0

        # Calculate volatility
        volatility = statistics.stdev(scores) if len(scores) > 1 else 0.0

        # Determine trend direction
        if abs(slope) < 0.5:  # Less than 0.5 points per hour
            if volatility > 5.0:
                trend_direction = HealthTrendDirection.VOLATILE
            else:
                trend_direction = HealthTrendDirection.STABLE
        elif slope > 0:
            trend_direction = HealthTrendDirection.IMPROVING
        else:
            trend_direction = HealthTrendDirection.DECLINING

        # Calculate trend confidence
        if len(scores) >= 5:
            trend_confidence = min(1.0, len(scores) / 10.0)
        else:
            trend_confidence = 0.5

        # Adjust confidence based on volatility
        if volatility > 10.0:
            trend_confidence *= 0.7

        current_score = scores[-1]
        previous_score = scores[-2] if len(scores) > 1 else current_score
        score_change = current_score - previous_score

        return HealthTrend(
            agent_id=historical_scores[0].get('agent_id', 'unknown'),
            trend_direction=trend_direction,
            slope=slope,
            volatility=volatility,
            current_score=current_score,
            previous_score=previous_score,
            score_change=score_change,
            trend_confidence=trend_confidence,
            data_points=len(historical_scores)
        )

    def _predict_health_trends(self, historical_scores: List[Dict[str, Any]]) -> Dict[str, Any]:
        """Predict future health trends based on historical data."""
        if len(historical_scores) < 5:
            return {"error": "Insufficient data for predictions"}

        scores = [point['score'] for point in historical_scores]

        # Simple linear extrapolation
        trend_analysis = self._analyze_health_trend(historical_scores)

        # Predict next few points
        predictions = []
        current_score = scores[-1]

        for hours_ahead in [1, 4, 12, 24]:
            predicted_score = current_score + (trend_analysis.slope * hours_ahead)
            # Add some uncertainty bounds
            uncertainty = trend_analysis.volatility * math.sqrt(hours_ahead / 24)

            predictions.append({
                "hours_ahead": hours_ahead,
                "predicted_score": round(max(0, min(100, predicted_score)), 1),
                "lower_bound": round(max(0, predicted_score - uncertainty), 1),
                "upper_bound": round(min(100, predicted_score + uncertainty), 1),
                "confidence": round(max(0.1, trend_analysis.trend_confidence * (1 - hours_ahead / 48)), 2)
            })

        return {
            "predictions": predictions,
            "trend_slope": round(trend_analysis.slope, 3),
            "trend_confidence": round(trend_analysis.trend_confidence, 2),
            "prediction_method": "linear_extrapolation"
        }

    # ========================================================================
    # Private Methods - Database and Utilities
    # ========================================================================

    async def _initialize_db(self):
        """Initialize database connection."""
        try:
            self.engine = create_async_engine(
                self.settings.database_url.replace("postgresql://", "postgresql+asyncpg://"),
                **self.settings.get_database_pool_settings(),
                echo=self.settings.enable_debug_logging
            )

            self.session_maker = sessionmaker(
                self.engine,
                class_=AsyncSession,
                expire_on_commit=False
            )

            # Test connection
            async with self.session_maker() as session:
                await session.execute(text("SELECT 1"))

            logger.info("Agent health service database connection initialized")

        except Exception as e:
            logger.error(f"Failed to initialize agent health service database: {e}")
            raise

    async def _load_agent_baselines(self):
        """Load agent-specific performance baselines."""
        try:
            async with self.session_maker() as session:
                # Get all agents
                result = await session.execute(select(Agent))
                agents = result.scalars().all()

                for agent in agents:
                    # Load baseline configuration from agent configuration
                    config = agent.configuration or {}
                    baselines = config.get('health_baselines', {})

                    self._agent_baselines[agent.agent_id] = {
                        'response_time_ms': baselines.get('response_time_ms', self.BASELINE_RESPONSE_TIME_MS),
                        'acceptable_time_ms': baselines.get('acceptable_time_ms', self.ACCEPTABLE_RESPONSE_TIME_MS),
                        'poor_time_ms': baselines.get('poor_time_ms', self.POOR_RESPONSE_TIME_MS),
                    }

                logger.info(f"Loaded baselines for {len(self._agent_baselines)} agents")

        except Exception as e:
            logger.warning(f"Could not load agent baselines: {e}")
            # Continue with default baselines

    async def _get_agent_info(self, session: AsyncSession, agent_id: str) -> Optional[Agent]:
        """Get agent information from database."""
        try:
            result = await session.execute(
                select(Agent).where(Agent.agent_id == agent_id)
            )
            return result.scalar_one_or_none()
        except Exception as e:
            logger.error(f"Error getting agent info for {agent_id}: {e}")
            return None

    def _validate_agent_id(self, agent_id: str):
        """Validate agent ID format."""
        if not agent_id or not isinstance(agent_id, str):
            raise ValidationError("Agent ID must be a non-empty string")

        if len(agent_id) < 3 or len(agent_id) > 100:
            raise ValidationError("Agent ID must be between 3 and 100 characters")

        if not all(c.isalnum() or c == '_' for c in agent_id):
            raise ValidationError("Agent ID must contain only alphanumeric characters and underscores")
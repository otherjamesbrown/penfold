"""Quality Monitoring and Degradation Detection Service.

This module implements comprehensive quality tracking capabilities for monitoring
agent performance quality, detecting degradation patterns, and providing quality
analytics and alerting for the Penfold observability framework.

Features:
- Quality metrics tracking (confidence scores, accuracy trends)
- Quality degradation detection algorithms with configurable alerts
- Agent-specific quality thresholds and baseline tracking
- Confidence score distribution analysis and trending
- Accuracy trend analysis with statistical significance testing
- Quality benchmarking and comparative analysis
- Real-time quality alerts with severity classification
- Quality target tracking and monitoring
- Comprehensive quality analytics and reporting
- Integration with DecisionTrace and AgentMetric models
- Quality pattern recognition and anomaly detection
- Performance regression detection for quality metrics
- Quality improvement recommendations and insights

Example Usage:
    from observability_lib.services.quality_tracker import QualityTracker
    from observability_lib.config import settings

    tracker = QualityTracker(settings)
    await tracker.start()

    # Track quality metrics
    await tracker.track_confidence_score("email_processor", 0.85)
    await tracker.track_accuracy_measurement("email_processor", 0.92, "entity_extraction")

    # Get current quality assessment
    quality = await tracker.get_quality_assessment("email_processor")

    # Check for quality degradation
    degradation = await tracker.check_quality_degradation(
        agent_id="email_processor",
        time_window_hours=24
    )

    # Get quality analytics
    analytics = await tracker.get_quality_analytics(
        agents=["email_processor", "meeting_analyzer"],
        time_range_hours=168
    )

    # Generate quality report
    report = await tracker.generate_quality_report(
        time_range_hours=24,
        include_benchmarks=True
    )

    await tracker.stop()
"""

import asyncio
import logging
import statistics
import time
import uuid
from collections import defaultdict, deque
from dataclasses import dataclass, asdict
from datetime import datetime, timedelta
from typing import Any, Dict, List, Optional, Tuple, Union
from enum import Enum
import math
import json

from sqlalchemy.ext.asyncio import AsyncSession, create_async_engine
from sqlalchemy.orm import sessionmaker
from sqlalchemy import select, func, and_, or_, text, desc, case
from sqlalchemy.exc import SQLAlchemyError

from observability_lib.config import ObservabilitySettings
from observability_lib.models.agent_metrics import (
    Agent, AgentMetric, DecisionTrace, WorkflowEvent, AgentLog, AlertThreshold,
    AlertEvent, AgentType, MetricType, DecisionType, EventStatus, LogLevel,
    AlertStatus, ThresholdType, ComparisonOperator
)
from penf_lib.storage.validators import ValidationError


logger = logging.getLogger(__name__)


# ============================================================================
# Quality Assessment Data Types and Enums
# ============================================================================

class QualityLevel(str, Enum):
    """Quality assessment levels"""
    EXCELLENT = "excellent"  # >95% quality
    GOOD = "good"           # 85-95% quality
    FAIR = "fair"           # 70-84% quality
    POOR = "poor"           # 50-69% quality
    CRITICAL = "critical"   # <50% quality


class QualityTrend(str, Enum):
    """Quality trend directions"""
    IMPROVING = "improving"
    STABLE = "stable"
    DEGRADING = "degrading"
    VOLATILE = "volatile"


class DegradationType(str, Enum):
    """Types of quality degradation"""
    CONFIDENCE_DROP = "confidence_drop"
    ACCURACY_DECLINE = "accuracy_decline"
    CONSISTENCY_LOSS = "consistency_loss"
    PERFORMANCE_REGRESSION = "performance_regression"
    DECISION_QUALITY_DROP = "decision_quality_drop"


class AlertSeverity(str, Enum):
    """Alert severity levels"""
    LOW = "low"
    MEDIUM = "medium"
    HIGH = "high"
    CRITICAL = "critical"


@dataclass
class QualityMetric:
    """Individual quality measurement"""
    agent_id: str
    timestamp: datetime
    metric_type: str
    metric_value: float
    confidence_score: Optional[float]
    context: Dict[str, Any]
    source: str  # "decision_trace", "agent_metric", "workflow_event"


@dataclass
class QualityAssessment:
    """Comprehensive quality assessment for an agent"""
    agent_id: str
    timestamp: datetime
    overall_quality_score: float  # 0-1.0
    quality_level: QualityLevel

    # Component scores
    confidence_score: Optional[float]
    accuracy_score: Optional[float]
    consistency_score: Optional[float]
    reliability_score: Optional[float]

    # Quality metrics
    avg_confidence: Optional[float]
    min_confidence: Optional[float]
    confidence_stddev: Optional[float]
    accuracy_trend: Optional[float]
    decision_quality: Optional[float]

    # Assessment metadata
    sample_size: int
    assessment_window_hours: float
    confidence_level: float  # Statistical confidence in assessment

    # Quality baselines and targets
    baseline_quality: Optional[float]
    quality_target: Optional[float]
    target_met: bool


@dataclass
class QualityDegradation:
    """Quality degradation detection result"""
    agent_id: str
    degradation_type: DegradationType
    severity: AlertSeverity
    detected_at: datetime

    # Degradation metrics
    current_quality: float
    baseline_quality: float
    degradation_percentage: float
    significance_score: float  # Statistical significance

    # Context
    time_window_analyzed: timedelta
    sample_size: int
    contributing_factors: List[str]
    recommendations: List[str]

    # Alert details
    should_alert: bool
    alert_message: str


@dataclass
class QualityBenchmark:
    """Quality benchmark comparison"""
    agent_id: str
    metric_type: str
    current_value: float
    benchmark_value: float
    percentile_rank: float
    comparison_status: str  # "above_benchmark", "at_benchmark", "below_benchmark"
    improvement_needed: Optional[float]


@dataclass
class QualityTrendAnalysis:
    """Quality trend analysis over time"""
    agent_id: str
    trend_direction: QualityTrend
    trend_strength: float  # 0-1.0 (confidence in trend)
    slope: float  # Rate of change
    volatility: float
    data_points: int
    time_span: timedelta

    # Predictions
    predicted_quality_1h: Optional[float]
    predicted_quality_24h: Optional[float]
    predicted_quality_7d: Optional[float]


@dataclass
class QualityAlert:
    """Quality-related alert"""
    agent_id: str
    alert_type: str
    severity: AlertSeverity
    message: str
    detected_at: datetime

    # Quality metrics
    current_quality: float
    threshold_value: float
    deviation_percentage: float

    # Context
    time_window: timedelta
    sample_size: int
    contributing_metrics: List[str]
    recommendations: List[str]

    # Alert status
    is_active: bool
    acknowledged: bool


@dataclass
class QualityReport:
    """Comprehensive quality analysis report"""
    generated_at: datetime
    time_range: Tuple[datetime, datetime]
    agents_analyzed: List[str]

    # Summary metrics
    overall_system_quality: float
    quality_distribution: Dict[QualityLevel, int]
    total_degradations_detected: int

    # Agent assessments
    agent_assessments: List[QualityAssessment]
    quality_rankings: List[Tuple[str, float]]

    # Degradations and alerts
    active_degradations: List[QualityDegradation]
    quality_alerts: List[QualityAlert]

    # Trends and benchmarks
    quality_trends: List[QualityTrendAnalysis]
    benchmark_comparisons: List[QualityBenchmark]

    # Recommendations
    system_recommendations: List[str]
    priority_actions: List[str]


# ============================================================================
# Quality Tracker Service Implementation
# ============================================================================

class QualityTracker:
    """Service for comprehensive quality monitoring and degradation detection.

    Tracks quality metrics across multiple dimensions, detects degradation patterns,
    and provides quality analytics and alerting capabilities.
    """

    # Quality assessment thresholds
    EXCELLENT_THRESHOLD = 0.95
    GOOD_THRESHOLD = 0.85
    FAIR_THRESHOLD = 0.70
    POOR_THRESHOLD = 0.50

    # Degradation detection parameters
    DEGRADATION_THRESHOLD_MINOR = 0.05  # 5% degradation
    DEGRADATION_THRESHOLD_MAJOR = 0.15  # 15% degradation
    DEGRADATION_THRESHOLD_CRITICAL = 0.30  # 30% degradation

    # Statistical significance thresholds
    SIGNIFICANCE_THRESHOLD = 0.05  # p-value threshold
    MIN_SAMPLE_SIZE = 10
    MIN_CONFIDENCE_FOR_ALERT = 0.70

    def __init__(self, settings: ObservabilitySettings):
        """Initialize quality tracker with configuration."""
        self.settings = settings
        self._engine = None
        self._session_factory = None
        self._running = False

        # Quality tracking state
        self._quality_baselines: Dict[str, Dict[str, float]] = {}
        self._quality_targets: Dict[str, Dict[str, float]] = {}
        self._agent_thresholds: Dict[str, Dict[str, float]] = {}

        # Quality measurement buffers
        self._confidence_buffer: Dict[str, deque] = defaultdict(lambda: deque(maxlen=1000))
        self._accuracy_buffer: Dict[str, deque] = defaultdict(lambda: deque(maxlen=1000))
        self._quality_history: Dict[str, deque] = defaultdict(lambda: deque(maxlen=500))

        # Degradation tracking
        self._active_degradations: Dict[str, List[QualityDegradation]] = defaultdict(list)
        self._degradation_history: Dict[str, deque] = defaultdict(lambda: deque(maxlen=100))

        # Performance monitoring
        self._assessment_times: deque = deque(maxlen=100)
        self._last_assessment_time: Dict[str, datetime] = {}

        # Alert management
        self._active_alerts: Dict[str, List[QualityAlert]] = defaultdict(list)
        self._alert_cooldowns: Dict[str, datetime] = {}
        self._alert_cooldown_period = timedelta(minutes=15)

        logger.info("QualityTracker initialized")

    async def start(self) -> None:
        """Start the quality tracker service."""
        try:
            # Create async database engine
            self._engine = create_async_engine(
                self.settings.database_url.replace("postgresql://", "postgresql+asyncpg://"),
                **self.settings.get_database_pool_settings(),
                echo=self.settings.enable_debug_logging
            )

            # Create session factory
            self._session_factory = sessionmaker(
                bind=self._engine,
                class_=AsyncSession,
                expire_on_commit=False
            )

            # Test connection
            async with self._session_factory() as session:
                await session.execute(text("SELECT 1"))

            # Load quality baselines and targets
            await self._load_quality_baselines()
            await self._load_quality_targets()
            await self._load_agent_thresholds()

            self._running = True
            logger.info("QualityTracker started successfully")

        except Exception as e:
            logger.error("Failed to start QualityTracker: %s", e)
            raise

    async def stop(self) -> None:
        """Stop the quality tracker service."""
        self._running = False

        if self._engine:
            await self._engine.dispose()

        # Clear buffers and state
        self._confidence_buffer.clear()
        self._accuracy_buffer.clear()
        self._quality_history.clear()
        self._active_degradations.clear()
        self._active_alerts.clear()

        logger.info("QualityTracker stopped")

    def _check_running(self) -> None:
        """Check if service is running."""
        if not self._running:
            raise RuntimeError("QualityTracker is not running. Call start() first.")

    # ========================================================================
    # Quality Metrics Tracking
    # ========================================================================

    async def track_confidence_score(
        self,
        agent_id: str,
        confidence_score: float,
        context: Optional[Dict[str, Any]] = None,
        decision_type: Optional[str] = None
    ) -> None:
        """Track a confidence score measurement for quality assessment.

        Args:
            agent_id: Agent identifier
            confidence_score: Confidence score (0-1.0)
            context: Optional context information
            decision_type: Optional decision type classification
        """
        self._check_running()
        self._validate_agent_id(agent_id)
        self._validate_confidence_score(confidence_score)

        # Add to confidence buffer
        measurement = QualityMetric(
            agent_id=agent_id,
            timestamp=datetime.utcnow(),
            metric_type="confidence_score",
            metric_value=confidence_score,
            confidence_score=confidence_score,
            context=context or {},
            source="manual_tracking"
        )

        self._confidence_buffer[agent_id].append(measurement)

        # Check for immediate degradation
        await self._check_immediate_degradation(agent_id, "confidence", confidence_score)

        logger.debug(f"Tracked confidence score {confidence_score:.3f} for agent {agent_id}")

    async def track_accuracy_measurement(
        self,
        agent_id: str,
        accuracy_score: float,
        measurement_type: str,
        context: Optional[Dict[str, Any]] = None
    ) -> None:
        """Track an accuracy measurement for quality assessment.

        Args:
            agent_id: Agent identifier
            accuracy_score: Accuracy score (0-1.0)
            measurement_type: Type of accuracy measurement
            context: Optional context information
        """
        self._check_running()
        self._validate_agent_id(agent_id)
        self._validate_score(accuracy_score, "accuracy_score")

        # Add to accuracy buffer
        measurement = QualityMetric(
            agent_id=agent_id,
            timestamp=datetime.utcnow(),
            metric_type=f"accuracy_{measurement_type}",
            metric_value=accuracy_score,
            confidence_score=None,
            context=context or {},
            source="manual_tracking"
        )

        self._accuracy_buffer[agent_id].append(measurement)

        # Check for immediate degradation
        await self._check_immediate_degradation(agent_id, "accuracy", accuracy_score)

        logger.debug(f"Tracked accuracy score {accuracy_score:.3f} ({measurement_type}) for agent {agent_id}")

    async def ingest_decision_trace_quality(
        self,
        decision_trace_id: str,
        tenant_id: Optional[str] = None
    ) -> None:
        """Ingest quality metrics from a decision trace.

        Args:
            decision_trace_id: Decision trace identifier
            tenant_id: Optional tenant filter
        """
        self._check_running()

        async with self._session_factory() as session:
            try:
                # Get decision trace
                query = select(DecisionTrace).where(
                    DecisionTrace.id == uuid.UUID(decision_trace_id)
                )
                if tenant_id:
                    query = query.where(DecisionTrace.tenant_id == tenant_id)

                result = await session.execute(query)
                trace = result.scalar_one_or_none()

                if not trace:
                    logger.warning(f"Decision trace not found: {decision_trace_id}")
                    return

                # Extract quality metrics
                confidence = trace.confidence_score
                context = {
                    "decision_type": trace.decision_type,
                    "alternatives_count": len(trace.alternatives_considered),
                    "has_reasoning": bool(trace.reasoning),
                    "workflow_id": str(trace.workflow_id) if trace.workflow_id else None
                }

                # Track confidence score
                await self.track_confidence_score(
                    agent_id=trace.agent_id,
                    confidence_score=confidence,
                    context=context,
                    decision_type=trace.decision_type
                )

                # Calculate decision quality score based on multiple factors
                quality_score = self._calculate_decision_quality(trace)

                # Track as quality measurement
                measurement = QualityMetric(
                    agent_id=trace.agent_id,
                    timestamp=trace.timestamp,
                    metric_type="decision_quality",
                    metric_value=quality_score,
                    confidence_score=confidence,
                    context=context,
                    source="decision_trace"
                )

                self._quality_history[trace.agent_id].append(measurement)

            except Exception as e:
                logger.error(f"Error ingesting decision trace quality: {e}")

    async def ingest_agent_metrics_quality(
        self,
        agent_id: str,
        time_range_hours: float = 1.0,
        tenant_id: Optional[str] = None
    ) -> None:
        """Ingest quality-related metrics from agent metrics.

        Args:
            agent_id: Agent identifier
            time_range_hours: Time range to analyze
            tenant_id: Optional tenant filter
        """
        self._check_running()

        async with self._session_factory() as session:
            try:
                start_time = datetime.utcnow() - timedelta(hours=time_range_hours)

                # Query quality-related metrics
                query = select(AgentMetric).where(
                    and_(
                        AgentMetric.agent_id == agent_id,
                        AgentMetric.timestamp >= start_time,
                        or_(
                            AgentMetric.metric_name.ilike('%confidence%'),
                            AgentMetric.metric_name.ilike('%accuracy%'),
                            AgentMetric.metric_name.ilike('%quality%'),
                            AgentMetric.metric_name.ilike('%precision%'),
                            AgentMetric.metric_name.ilike('%recall%'),
                            AgentMetric.metric_name.ilike('%f1%'),
                        )
                    )
                ).order_by(AgentMetric.timestamp.desc())

                if tenant_id:
                    query = query.where(AgentMetric.tenant_id == tenant_id)

                result = await session.execute(query)
                metrics = result.scalars().all()

                for metric in metrics:
                    # Normalize metric value to 0-1 range if needed
                    normalized_value = self._normalize_metric_value(
                        metric.metric_value,
                        metric.metric_name
                    )

                    context = {
                        "metric_name": metric.metric_name,
                        "metric_type": metric.metric_type,
                        "labels": metric.labels or {}
                    }

                    # Track as quality measurement
                    measurement = QualityMetric(
                        agent_id=agent_id,
                        timestamp=metric.timestamp,
                        metric_type=f"agent_metric_{metric.metric_name}",
                        metric_value=normalized_value,
                        confidence_score=None,
                        context=context,
                        source="agent_metric"
                    )

                    if 'confidence' in metric.metric_name.lower():
                        self._confidence_buffer[agent_id].append(measurement)
                    elif any(term in metric.metric_name.lower()
                           for term in ['accuracy', 'precision', 'recall', 'f1']):
                        self._accuracy_buffer[agent_id].append(measurement)
                    else:
                        self._quality_history[agent_id].append(measurement)

                logger.debug(f"Ingested {len(metrics)} quality metrics for agent {agent_id}")

            except Exception as e:
                logger.error(f"Error ingesting agent metrics quality: {e}")

    # ========================================================================
    # Quality Assessment
    # ========================================================================

    async def get_quality_assessment(
        self,
        agent_id: str,
        assessment_window_hours: float = 24.0,
        include_predictions: bool = True
    ) -> QualityAssessment:
        """Get comprehensive quality assessment for an agent.

        Args:
            agent_id: Agent identifier
            assessment_window_hours: Time window for assessment
            include_predictions: Whether to include trend predictions

        Returns:
            Complete quality assessment
        """
        self._check_running()
        self._validate_agent_id(agent_id)

        start_time = time.time()

        try:
            # Gather quality data from all sources
            quality_data = await self._gather_quality_data(agent_id, assessment_window_hours)

            # Calculate component scores
            confidence_score = self._calculate_confidence_score(quality_data)
            accuracy_score = self._calculate_accuracy_score(quality_data)
            consistency_score = self._calculate_consistency_score(quality_data)
            reliability_score = self._calculate_reliability_score(quality_data)

            # Calculate overall quality score
            overall_score = self._calculate_overall_quality_score(
                confidence_score, accuracy_score, consistency_score, reliability_score
            )

            # Determine quality level
            quality_level = self._determine_quality_level(overall_score)

            # Get statistical metrics
            stats = self._calculate_quality_statistics(quality_data)

            # Get baseline and target information
            baseline = self._quality_baselines.get(agent_id, {}).get('overall', None)
            target = self._quality_targets.get(agent_id, {}).get('overall', None)
            target_met = target is None or overall_score >= target

            assessment = QualityAssessment(
                agent_id=agent_id,
                timestamp=datetime.utcnow(),
                overall_quality_score=overall_score,
                quality_level=quality_level,

                confidence_score=confidence_score,
                accuracy_score=accuracy_score,
                consistency_score=consistency_score,
                reliability_score=reliability_score,

                avg_confidence=stats.get('avg_confidence'),
                min_confidence=stats.get('min_confidence'),
                confidence_stddev=stats.get('confidence_stddev'),
                accuracy_trend=stats.get('accuracy_trend'),
                decision_quality=stats.get('decision_quality'),

                sample_size=stats.get('total_samples', 0),
                assessment_window_hours=assessment_window_hours,
                confidence_level=stats.get('confidence_level', 0.0),

                baseline_quality=baseline,
                quality_target=target,
                target_met=target_met
            )

            # Update assessment history
            self._last_assessment_time[agent_id] = datetime.utcnow()

            # Record performance
            assessment_time = time.time() - start_time
            self._assessment_times.append(assessment_time)

            logger.debug(f"Quality assessment for {agent_id} completed in {assessment_time:.3f}s")

            return assessment

        except Exception as e:
            logger.error(f"Error calculating quality assessment for {agent_id}: {e}")
            raise

    async def get_quality_assessments_batch(
        self,
        agent_ids: List[str],
        assessment_window_hours: float = 24.0
    ) -> Dict[str, QualityAssessment]:
        """Get quality assessments for multiple agents efficiently.

        Args:
            agent_ids: List of agent identifiers
            assessment_window_hours: Time window for assessment

        Returns:
            Dictionary mapping agent_id to QualityAssessment
        """
        self._check_running()

        # Process assessments concurrently
        tasks = [
            self.get_quality_assessment(agent_id, assessment_window_hours, include_predictions=False)
            for agent_id in agent_ids
        ]

        results = await asyncio.gather(*tasks, return_exceptions=True)

        assessments = {}
        for i, result in enumerate(results):
            agent_id = agent_ids[i]
            if isinstance(result, Exception):
                logger.error(f"Failed to assess quality for agent {agent_id}: {result}")
                # Create minimal assessment
                assessments[agent_id] = QualityAssessment(
                    agent_id=agent_id,
                    timestamp=datetime.utcnow(),
                    overall_quality_score=0.0,
                    quality_level=QualityLevel.CRITICAL,
                    confidence_score=None,
                    accuracy_score=None,
                    consistency_score=None,
                    reliability_score=None,
                    avg_confidence=None,
                    min_confidence=None,
                    confidence_stddev=None,
                    accuracy_trend=None,
                    decision_quality=None,
                    sample_size=0,
                    assessment_window_hours=assessment_window_hours,
                    confidence_level=0.0,
                    baseline_quality=None,
                    quality_target=None,
                    target_met=False
                )
            else:
                assessments[agent_id] = result

        return assessments

    # ========================================================================
    # Quality Degradation Detection
    # ========================================================================

    async def check_quality_degradation(
        self,
        agent_id: str,
        time_window_hours: float = 24.0,
        comparison_window_hours: float = 168.0  # 1 week baseline
    ) -> List[QualityDegradation]:
        """Check for quality degradation patterns.

        Args:
            agent_id: Agent identifier
            time_window_hours: Recent time window to analyze
            comparison_window_hours: Baseline comparison window

        Returns:
            List of detected degradations
        """
        self._check_running()
        self._validate_agent_id(agent_id)

        degradations = []

        try:
            # Get current and baseline quality data
            current_data = await self._gather_quality_data(agent_id, time_window_hours)
            baseline_data = await self._gather_quality_data(
                agent_id,
                comparison_window_hours,
                exclude_recent_hours=time_window_hours
            )

            if not current_data or not baseline_data:
                logger.debug(f"Insufficient data for degradation analysis: {agent_id}")
                return []

            # Check different types of degradation
            degradations.extend(await self._detect_confidence_degradation(
                agent_id, current_data, baseline_data, time_window_hours
            ))

            degradations.extend(await self._detect_accuracy_degradation(
                agent_id, current_data, baseline_data, time_window_hours
            ))

            degradations.extend(await self._detect_consistency_degradation(
                agent_id, current_data, baseline_data, time_window_hours
            ))

            degradations.extend(await self._detect_performance_regression(
                agent_id, current_data, baseline_data, time_window_hours
            ))

            # Update degradation tracking
            if degradations:
                self._active_degradations[agent_id].extend(degradations)
                for degradation in degradations:
                    self._degradation_history[agent_id].append(degradation)

            # Clean up old degradations
            self._cleanup_resolved_degradations(agent_id)

            return degradations

        except Exception as e:
            logger.error(f"Error checking quality degradation for {agent_id}: {e}")
            return []

    async def check_system_wide_degradation(
        self,
        agents: Optional[List[str]] = None,
        time_window_hours: float = 24.0,
        degradation_threshold: float = 0.1  # 10% degradation threshold
    ) -> Dict[str, List[QualityDegradation]]:
        """Check for quality degradation across multiple agents.

        Args:
            agents: Optional list of agents to check (None for all active)
            time_window_hours: Time window for degradation analysis
            degradation_threshold: Minimum degradation percentage to report

        Returns:
            Dictionary mapping agent_id to list of degradations
        """
        self._check_running()

        if agents is None:
            # Get all active agents
            async with self._session_factory() as session:
                result = await session.execute(
                    select(Agent.agent_id).where(Agent.is_active == True)
                )
                agents = [row[0] for row in result.all()]

        # Check degradation for each agent concurrently
        tasks = [
            self.check_quality_degradation(agent_id, time_window_hours)
            for agent_id in agents
        ]

        results = await asyncio.gather(*tasks, return_exceptions=True)

        system_degradations = {}
        for i, result in enumerate(results):
            agent_id = agents[i]
            if isinstance(result, Exception):
                logger.error(f"Failed degradation check for agent {agent_id}: {result}")
                system_degradations[agent_id] = []
            else:
                # Filter by threshold
                significant_degradations = [
                    d for d in result
                    if d.degradation_percentage >= degradation_threshold
                ]
                system_degradations[agent_id] = significant_degradations

        return system_degradations

    # ========================================================================
    # Quality Alerts and Monitoring
    # ========================================================================

    async def check_quality_alerts(
        self,
        agent_id: str,
        assessment_window_hours: float = 1.0
    ) -> List[QualityAlert]:
        """Check for quality-related alerts.

        Args:
            agent_id: Agent identifier
            assessment_window_hours: Time window for alert assessment

        Returns:
            List of active quality alerts
        """
        self._check_running()

        alerts = []

        try:
            # Check if agent is in alert cooldown
            if self._is_in_alert_cooldown(agent_id):
                return []

            # Get current quality assessment
            assessment = await self.get_quality_assessment(agent_id, assessment_window_hours)

            # Get agent-specific thresholds
            thresholds = self._agent_thresholds.get(agent_id, {})

            # Check overall quality threshold
            quality_threshold = thresholds.get('quality_threshold', self.FAIR_THRESHOLD)
            if assessment.overall_quality_score < quality_threshold:
                severity = self._determine_alert_severity(
                    assessment.overall_quality_score,
                    quality_threshold
                )

                alert = QualityAlert(
                    agent_id=agent_id,
                    alert_type="low_quality",
                    severity=severity,
                    message=f"Quality score ({assessment.overall_quality_score:.2%}) below threshold ({quality_threshold:.2%})",
                    detected_at=datetime.utcnow(),
                    current_quality=assessment.overall_quality_score,
                    threshold_value=quality_threshold,
                    deviation_percentage=((quality_threshold - assessment.overall_quality_score) / quality_threshold) * 100,
                    time_window=timedelta(hours=assessment_window_hours),
                    sample_size=assessment.sample_size,
                    contributing_metrics=self._identify_contributing_metrics(assessment),
                    recommendations=self._generate_quality_recommendations(assessment),
                    is_active=True,
                    acknowledged=False
                )
                alerts.append(alert)

            # Check confidence score alerts
            if assessment.confidence_score is not None:
                confidence_threshold = thresholds.get('confidence_threshold', 0.6)
                if assessment.confidence_score < confidence_threshold:
                    alerts.append(QualityAlert(
                        agent_id=agent_id,
                        alert_type="low_confidence",
                        severity=self._determine_alert_severity(assessment.confidence_score, confidence_threshold),
                        message=f"Average confidence ({assessment.confidence_score:.2%}) below threshold",
                        detected_at=datetime.utcnow(),
                        current_quality=assessment.confidence_score,
                        threshold_value=confidence_threshold,
                        deviation_percentage=((confidence_threshold - assessment.confidence_score) / confidence_threshold) * 100,
                        time_window=timedelta(hours=assessment_window_hours),
                        sample_size=assessment.sample_size,
                        contributing_metrics=['confidence_score'],
                        recommendations=['Review decision-making logic', 'Check input data quality'],
                        is_active=True,
                        acknowledged=False
                    ))

            # Check accuracy alerts
            if assessment.accuracy_score is not None:
                accuracy_threshold = thresholds.get('accuracy_threshold', 0.75)
                if assessment.accuracy_score < accuracy_threshold:
                    alerts.append(QualityAlert(
                        agent_id=agent_id,
                        alert_type="low_accuracy",
                        severity=self._determine_alert_severity(assessment.accuracy_score, accuracy_threshold),
                        message=f"Accuracy ({assessment.accuracy_score:.2%}) below threshold",
                        detected_at=datetime.utcnow(),
                        current_quality=assessment.accuracy_score,
                        threshold_value=accuracy_threshold,
                        deviation_percentage=((accuracy_threshold - assessment.accuracy_score) / accuracy_threshold) * 100,
                        time_window=timedelta(hours=assessment_window_hours),
                        sample_size=assessment.sample_size,
                        contributing_metrics=['accuracy_score'],
                        recommendations=['Review training data', 'Validate model performance'],
                        is_active=True,
                        acknowledged=False
                    ))

            # Check for degradation alerts
            degradations = await self.check_quality_degradation(agent_id)
            for degradation in degradations:
                if degradation.should_alert:
                    alert = QualityAlert(
                        agent_id=agent_id,
                        alert_type=f"degradation_{degradation.degradation_type.value}",
                        severity=degradation.severity,
                        message=degradation.alert_message,
                        detected_at=degradation.detected_at,
                        current_quality=degradation.current_quality,
                        threshold_value=degradation.baseline_quality,
                        deviation_percentage=degradation.degradation_percentage,
                        time_window=degradation.time_window_analyzed,
                        sample_size=degradation.sample_size,
                        contributing_metrics=degradation.contributing_factors,
                        recommendations=degradation.recommendations,
                        is_active=True,
                        acknowledged=False
                    )
                    alerts.append(alert)

            # Update alert tracking
            if alerts:
                self._active_alerts[agent_id].extend(alerts)
                self._alert_cooldowns[agent_id] = datetime.utcnow()

            return alerts

        except Exception as e:
            logger.error(f"Error checking quality alerts for {agent_id}: {e}")
            return []

    async def acknowledge_quality_alert(
        self,
        agent_id: str,
        alert_type: str
    ) -> bool:
        """Acknowledge a quality alert.

        Args:
            agent_id: Agent identifier
            alert_type: Type of alert to acknowledge

        Returns:
            True if alert was found and acknowledged
        """
        self._check_running()

        for alert in self._active_alerts.get(agent_id, []):
            if alert.alert_type == alert_type and alert.is_active:
                alert.acknowledged = True
                logger.info(f"Acknowledged quality alert {alert_type} for agent {agent_id}")
                return True

        return False

    # ========================================================================
    # Quality Analytics and Reporting
    # ========================================================================

    async def get_quality_analytics(
        self,
        agents: Optional[List[str]] = None,
        time_range_hours: float = 168.0,  # 1 week
        include_trends: bool = True,
        include_benchmarks: bool = True
    ) -> Dict[str, Any]:
        """Get comprehensive quality analytics.

        Args:
            agents: Optional list of agents to analyze
            time_range_hours: Time range for analysis
            include_trends: Whether to include trend analysis
            include_benchmarks: Whether to include benchmark comparisons

        Returns:
            Comprehensive quality analytics
        """
        self._check_running()

        if agents is None:
            # Get all active agents
            async with self._session_factory() as session:
                result = await session.execute(
                    select(Agent.agent_id).where(Agent.is_active == True)
                )
                agents = [row[0] for row in result.all()]

        # Get quality assessments for all agents
        assessments = await self.get_quality_assessments_batch(agents, 24.0)

        # Calculate system-wide metrics
        quality_scores = [a.overall_quality_score for a in assessments.values()]
        system_quality = statistics.mean(quality_scores) if quality_scores else 0.0

        # Quality distribution
        quality_distribution = {level: 0 for level in QualityLevel}
        for assessment in assessments.values():
            quality_distribution[assessment.quality_level] += 1

        # Top and bottom performers
        sorted_agents = sorted(
            assessments.items(),
            key=lambda x: x[1].overall_quality_score,
            reverse=True
        )

        analytics = {
            "generated_at": datetime.utcnow().isoformat(),
            "time_range_hours": time_range_hours,
            "agents_analyzed": agents,
            "total_agents": len(agents),

            # System metrics
            "system_quality_score": round(system_quality, 3),
            "quality_distribution": {level.value: count for level, count in quality_distribution.items()},

            # Agent rankings
            "top_performers": [
                {"agent_id": agent_id, "quality_score": round(assessment.overall_quality_score, 3)}
                for agent_id, assessment in sorted_agents[:5]
            ],
            "bottom_performers": [
                {"agent_id": agent_id, "quality_score": round(assessment.overall_quality_score, 3)}
                for agent_id, assessment in sorted_agents[-5:]
            ],

            # Quality statistics
            "quality_statistics": {
                "mean": round(statistics.mean(quality_scores), 3) if quality_scores else 0,
                "median": round(statistics.median(quality_scores), 3) if quality_scores else 0,
                "std_dev": round(statistics.stdev(quality_scores), 3) if len(quality_scores) > 1 else 0,
                "min": round(min(quality_scores), 3) if quality_scores else 0,
                "max": round(max(quality_scores), 3) if quality_scores else 0
            },

            # Assessment details
            "agent_assessments": {
                agent_id: {
                    "overall_quality": round(assessment.overall_quality_score, 3),
                    "quality_level": assessment.quality_level.value,
                    "confidence_score": round(assessment.confidence_score, 3) if assessment.confidence_score else None,
                    "accuracy_score": round(assessment.accuracy_score, 3) if assessment.accuracy_score else None,
                    "sample_size": assessment.sample_size,
                    "target_met": assessment.target_met
                }
                for agent_id, assessment in assessments.items()
            }
        }

        # Add trend analysis if requested
        if include_trends:
            trend_tasks = [
                self._get_quality_trend_analysis(agent_id, time_range_hours)
                for agent_id in agents
            ]
            trends = await asyncio.gather(*trend_tasks, return_exceptions=True)

            analytics["quality_trends"] = {}
            for i, trend in enumerate(trends):
                agent_id = agents[i]
                if not isinstance(trend, Exception):
                    analytics["quality_trends"][agent_id] = asdict(trend)

        # Add benchmark comparisons if requested
        if include_benchmarks:
            benchmark_tasks = [
                self._get_quality_benchmarks(agent_id)
                for agent_id in agents
            ]
            benchmarks = await asyncio.gather(*benchmark_tasks, return_exceptions=True)

            analytics["benchmarks"] = {}
            for i, benchmark_list in enumerate(benchmarks):
                agent_id = agents[i]
                if not isinstance(benchmark_list, Exception):
                    analytics["benchmarks"][agent_id] = [
                        asdict(b) for b in benchmark_list
                    ]

        return analytics

    async def generate_quality_report(
        self,
        time_range_hours: float = 168.0,  # 1 week
        include_benchmarks: bool = True,
        include_detailed_analysis: bool = True,
        agents: Optional[List[str]] = None
    ) -> QualityReport:
        """Generate comprehensive quality analysis report.

        Args:
            time_range_hours: Time range for report
            include_benchmarks: Include benchmark analysis
            include_detailed_analysis: Include detailed trend analysis
            agents: Optional specific agents to include

        Returns:
            Complete quality report
        """
        self._check_running()

        end_time = datetime.utcnow()
        start_time = end_time - timedelta(hours=time_range_hours)

        if agents is None:
            # Get all active agents
            async with self._session_factory() as session:
                result = await session.execute(
                    select(Agent.agent_id).where(Agent.is_active == True)
                )
                agents = [row[0] for row in result.all()]

        # Get quality assessments
        assessments_dict = await self.get_quality_assessments_batch(agents, 24.0)
        assessments = list(assessments_dict.values())

        # Calculate system metrics
        quality_scores = [a.overall_quality_score for a in assessments]
        overall_system_quality = statistics.mean(quality_scores) if quality_scores else 0.0

        # Quality distribution
        quality_distribution = {level: 0 for level in QualityLevel}
        for assessment in assessments:
            quality_distribution[assessment.quality_level] += 1

        # Agent rankings
        quality_rankings = sorted(
            [(a.agent_id, a.overall_quality_score) for a in assessments],
            key=lambda x: x[1],
            reverse=True
        )

        # Get degradations
        degradation_tasks = [
            self.check_quality_degradation(agent_id, 24.0)
            for agent_id in agents
        ]
        degradation_results = await asyncio.gather(*degradation_tasks, return_exceptions=True)

        active_degradations = []
        for result in degradation_results:
            if not isinstance(result, Exception):
                active_degradations.extend(result)

        # Get quality alerts
        alert_tasks = [
            self.check_quality_alerts(agent_id, 1.0)
            for agent_id in agents
        ]
        alert_results = await asyncio.gather(*alert_tasks, return_exceptions=True)

        quality_alerts = []
        for result in alert_results:
            if not isinstance(result, Exception):
                quality_alerts.extend(result)

        # Get trends if detailed analysis requested
        quality_trends = []
        if include_detailed_analysis:
            trend_tasks = [
                self._get_quality_trend_analysis(agent_id, time_range_hours)
                for agent_id in agents
            ]
            trend_results = await asyncio.gather(*trend_tasks, return_exceptions=True)

            for result in trend_results:
                if not isinstance(result, Exception):
                    quality_trends.append(result)

        # Get benchmarks if requested
        benchmark_comparisons = []
        if include_benchmarks:
            benchmark_tasks = [
                self._get_quality_benchmarks(agent_id)
                for agent_id in agents
            ]
            benchmark_results = await asyncio.gather(*benchmark_tasks, return_exceptions=True)

            for result in benchmark_results:
                if not isinstance(result, Exception):
                    benchmark_comparisons.extend(result)

        # Generate recommendations
        system_recommendations = self._generate_system_recommendations(
            assessments, active_degradations, quality_alerts
        )

        priority_actions = self._generate_priority_actions(
            active_degradations, quality_alerts
        )

        return QualityReport(
            generated_at=datetime.utcnow(),
            time_range=(start_time, end_time),
            agents_analyzed=agents,

            overall_system_quality=overall_system_quality,
            quality_distribution=quality_distribution,
            total_degradations_detected=len(active_degradations),

            agent_assessments=assessments,
            quality_rankings=quality_rankings,

            active_degradations=active_degradations,
            quality_alerts=quality_alerts,

            quality_trends=quality_trends,
            benchmark_comparisons=benchmark_comparisons,

            system_recommendations=system_recommendations,
            priority_actions=priority_actions
        )

    # ========================================================================
    # Quality Baselines and Targets
    # ========================================================================

    async def set_quality_baseline(
        self,
        agent_id: str,
        baseline_values: Dict[str, float]
    ) -> None:
        """Set quality baselines for an agent.

        Args:
            agent_id: Agent identifier
            baseline_values: Dictionary of baseline values by metric type
        """
        self._check_running()
        self._validate_agent_id(agent_id)

        # Validate baseline values
        for metric, value in baseline_values.items():
            if not 0.0 <= value <= 1.0:
                raise ValidationError(f"Baseline value for {metric} must be between 0.0 and 1.0")

        self._quality_baselines[agent_id] = baseline_values.copy()

        # Store in database for persistence
        async with self._session_factory() as session:
            try:
                # Update agent configuration with baseline
                query = select(Agent).where(Agent.agent_id == agent_id)
                result = await session.execute(query)
                agent = result.scalar_one_or_none()

                if agent:
                    config = agent.configuration or {}
                    config['quality_baselines'] = baseline_values
                    agent.configuration = config
                    await session.commit()

                logger.info(f"Set quality baselines for agent {agent_id}: {baseline_values}")

            except Exception as e:
                await session.rollback()
                logger.error(f"Error storing quality baselines: {e}")
                raise

    async def set_quality_targets(
        self,
        agent_id: str,
        target_values: Dict[str, float]
    ) -> None:
        """Set quality targets for an agent.

        Args:
            agent_id: Agent identifier
            target_values: Dictionary of target values by metric type
        """
        self._check_running()
        self._validate_agent_id(agent_id)

        # Validate target values
        for metric, value in target_values.items():
            if not 0.0 <= value <= 1.0:
                raise ValidationError(f"Target value for {metric} must be between 0.0 and 1.0")

        self._quality_targets[agent_id] = target_values.copy()

        # Store in database for persistence
        async with self._session_factory() as session:
            try:
                # Update agent configuration with targets
                query = select(Agent).where(Agent.agent_id == agent_id)
                result = await session.execute(query)
                agent = result.scalar_one_or_none()

                if agent:
                    config = agent.configuration or {}
                    config['quality_targets'] = target_values
                    agent.configuration = config
                    await session.commit()

                logger.info(f"Set quality targets for agent {agent_id}: {target_values}")

            except Exception as e:
                await session.rollback()
                logger.error(f"Error storing quality targets: {e}")
                raise

    async def learn_quality_baselines(
        self,
        agent_id: str,
        learning_period_hours: float = 168.0  # 1 week
    ) -> Dict[str, float]:
        """Learn quality baselines from historical data.

        Args:
            agent_id: Agent identifier
            learning_period_hours: Period to analyze for baseline learning

        Returns:
            Learned baseline values
        """
        self._check_running()

        # Gather historical quality data
        quality_data = await self._gather_quality_data(agent_id, learning_period_hours)

        if not quality_data:
            raise ValueError(f"Insufficient data to learn baselines for agent {agent_id}")

        # Calculate baseline metrics
        confidence_scores = [m.confidence_score for m in quality_data if m.confidence_score is not None]
        accuracy_scores = [m.metric_value for m in quality_data if 'accuracy' in m.metric_type]

        baselines = {}

        if confidence_scores:
            # Use 25th percentile as conservative baseline
            baselines['confidence'] = max(0.5, statistics.quantiles(confidence_scores, n=4)[0])

        if accuracy_scores:
            baselines['accuracy'] = max(0.6, statistics.quantiles(accuracy_scores, n=4)[0])

        # Calculate overall quality baseline
        if baselines:
            baselines['overall'] = statistics.mean(baselines.values()) * 0.9  # 10% buffer

        if baselines:
            await self.set_quality_baseline(agent_id, baselines)

        return baselines

    # ========================================================================
    # Helper Methods - Quality Calculation
    # ========================================================================

    async def _gather_quality_data(
        self,
        agent_id: str,
        time_window_hours: float,
        exclude_recent_hours: float = 0.0
    ) -> List[QualityMetric]:
        """Gather quality data from all sources."""
        quality_data = []

        # Get data from buffers
        for measurement in self._confidence_buffer[agent_id]:
            if self._is_in_time_window(
                measurement.timestamp, time_window_hours, exclude_recent_hours
            ):
                quality_data.append(measurement)

        for measurement in self._accuracy_buffer[agent_id]:
            if self._is_in_time_window(
                measurement.timestamp, time_window_hours, exclude_recent_hours
            ):
                quality_data.append(measurement)

        for measurement in self._quality_history[agent_id]:
            if self._is_in_time_window(
                measurement.timestamp, time_window_hours, exclude_recent_hours
            ):
                quality_data.append(measurement)

        # Get data from database
        async with self._session_factory() as session:
            try:
                end_time = datetime.utcnow() - timedelta(hours=exclude_recent_hours)
                start_time = end_time - timedelta(hours=time_window_hours)

                # Get decision traces
                trace_query = select(DecisionTrace).where(
                    and_(
                        DecisionTrace.agent_id == agent_id,
                        DecisionTrace.timestamp >= start_time,
                        DecisionTrace.timestamp <= end_time
                    )
                ).order_by(DecisionTrace.timestamp.desc())

                result = await session.execute(trace_query)
                traces = result.scalars().all()

                for trace in traces:
                    quality_data.append(QualityMetric(
                        agent_id=agent_id,
                        timestamp=trace.timestamp,
                        metric_type="decision_trace_confidence",
                        metric_value=trace.confidence_score,
                        confidence_score=trace.confidence_score,
                        context={
                            "decision_type": trace.decision_type,
                            "alternatives_count": len(trace.alternatives_considered)
                        },
                        source="decision_trace"
                    ))

                # Get agent metrics
                metric_query = select(AgentMetric).where(
                    and_(
                        AgentMetric.agent_id == agent_id,
                        AgentMetric.timestamp >= start_time,
                        AgentMetric.timestamp <= end_time,
                        or_(
                            AgentMetric.metric_name.ilike('%confidence%'),
                            AgentMetric.metric_name.ilike('%accuracy%'),
                            AgentMetric.metric_name.ilike('%quality%'),
                        )
                    )
                ).order_by(AgentMetric.timestamp.desc())

                result = await session.execute(metric_query)
                metrics = result.scalars().all()

                for metric in metrics:
                    normalized_value = self._normalize_metric_value(
                        metric.metric_value, metric.metric_name
                    )

                    quality_data.append(QualityMetric(
                        agent_id=agent_id,
                        timestamp=metric.timestamp,
                        metric_type=f"agent_metric_{metric.metric_name}",
                        metric_value=normalized_value,
                        confidence_score=normalized_value if 'confidence' in metric.metric_name.lower() else None,
                        context={
                            "metric_name": metric.metric_name,
                            "metric_type": metric.metric_type
                        },
                        source="agent_metric"
                    ))

            except Exception as e:
                logger.error(f"Error gathering quality data from database: {e}")

        # Sort by timestamp
        quality_data.sort(key=lambda x: x.timestamp, reverse=True)

        return quality_data

    def _calculate_confidence_score(self, quality_data: List[QualityMetric]) -> Optional[float]:
        """Calculate aggregated confidence score."""
        confidence_values = [
            m.confidence_score for m in quality_data
            if m.confidence_score is not None
        ]

        if not confidence_values:
            return None

        # Weight recent measurements more heavily
        if len(confidence_values) > 10:
            recent_weight = 0.7
            historical_weight = 0.3
            recent_count = min(len(confidence_values) // 3, 10)

            recent_avg = statistics.mean(confidence_values[:recent_count])
            historical_avg = statistics.mean(confidence_values[recent_count:])

            return (recent_avg * recent_weight) + (historical_avg * historical_weight)
        else:
            return statistics.mean(confidence_values)

    def _calculate_accuracy_score(self, quality_data: List[QualityMetric]) -> Optional[float]:
        """Calculate aggregated accuracy score."""
        accuracy_values = [
            m.metric_value for m in quality_data
            if 'accuracy' in m.metric_type.lower()
        ]

        if not accuracy_values:
            return None

        return statistics.mean(accuracy_values)

    def _calculate_consistency_score(self, quality_data: List[QualityMetric]) -> Optional[float]:
        """Calculate consistency score based on variance."""
        confidence_values = [
            m.confidence_score for m in quality_data
            if m.confidence_score is not None
        ]

        if len(confidence_values) < 3:
            return None

        # Consistency is inverse of coefficient of variation
        mean_conf = statistics.mean(confidence_values)
        if mean_conf == 0:
            return 0.0

        std_dev = statistics.stdev(confidence_values)
        cv = std_dev / mean_conf

        # Convert to 0-1 scale where lower CV = higher consistency
        consistency = max(0.0, 1.0 - (cv * 2))  # CV of 0.5 = 0 consistency

        return consistency

    def _calculate_reliability_score(self, quality_data: List[QualityMetric]) -> Optional[float]:
        """Calculate reliability score based on decision quality and success patterns."""
        decision_quality_values = [
            m.metric_value for m in quality_data
            if m.metric_type == "decision_quality"
        ]

        if not decision_quality_values:
            # Fallback to confidence-based reliability
            confidence_values = [
                m.confidence_score for m in quality_data
                if m.confidence_score is not None
            ]
            if confidence_values:
                # Reliability is confidence adjusted for consistency
                avg_confidence = statistics.mean(confidence_values)
                if len(confidence_values) > 1:
                    consistency_penalty = statistics.stdev(confidence_values)
                    return max(0.0, avg_confidence - consistency_penalty)
                else:
                    return avg_confidence
            return None

        return statistics.mean(decision_quality_values)

    def _calculate_overall_quality_score(
        self,
        confidence_score: Optional[float],
        accuracy_score: Optional[float],
        consistency_score: Optional[float],
        reliability_score: Optional[float]
    ) -> float:
        """Calculate overall quality score from components."""
        scores = []
        weights = []

        if confidence_score is not None:
            scores.append(confidence_score)
            weights.append(0.3)

        if accuracy_score is not None:
            scores.append(accuracy_score)
            weights.append(0.3)

        if consistency_score is not None:
            scores.append(consistency_score)
            weights.append(0.2)

        if reliability_score is not None:
            scores.append(reliability_score)
            weights.append(0.2)

        if not scores:
            return 0.0

        # Calculate weighted average
        total_weight = sum(weights)
        weighted_sum = sum(score * weight for score, weight in zip(scores, weights))

        return weighted_sum / total_weight

    def _determine_quality_level(self, overall_score: float) -> QualityLevel:
        """Determine quality level from overall score."""
        if overall_score >= self.EXCELLENT_THRESHOLD:
            return QualityLevel.EXCELLENT
        elif overall_score >= self.GOOD_THRESHOLD:
            return QualityLevel.GOOD
        elif overall_score >= self.FAIR_THRESHOLD:
            return QualityLevel.FAIR
        elif overall_score >= self.POOR_THRESHOLD:
            return QualityLevel.POOR
        else:
            return QualityLevel.CRITICAL

    def _calculate_quality_statistics(self, quality_data: List[QualityMetric]) -> Dict[str, Any]:
        """Calculate statistical metrics for quality data."""
        stats = {"total_samples": len(quality_data)}

        # Confidence statistics
        confidence_values = [
            m.confidence_score for m in quality_data
            if m.confidence_score is not None
        ]

        if confidence_values:
            stats["avg_confidence"] = statistics.mean(confidence_values)
            stats["min_confidence"] = min(confidence_values)
            stats["confidence_stddev"] = statistics.stdev(confidence_values) if len(confidence_values) > 1 else 0.0

        # Accuracy trend
        accuracy_values = [
            m.metric_value for m in quality_data
            if 'accuracy' in m.metric_type.lower()
        ]

        if len(accuracy_values) >= 3:
            # Simple linear trend
            x_values = list(range(len(accuracy_values)))
            slope = self._calculate_trend_slope(x_values, accuracy_values)
            stats["accuracy_trend"] = slope

        # Decision quality
        decision_quality_values = [
            m.metric_value for m in quality_data
            if m.metric_type == "decision_quality"
        ]

        if decision_quality_values:
            stats["decision_quality"] = statistics.mean(decision_quality_values)

        # Calculate confidence level based on sample size
        sample_size = len(quality_data)
        if sample_size >= 50:
            stats["confidence_level"] = 0.95
        elif sample_size >= 20:
            stats["confidence_level"] = 0.80
        elif sample_size >= 10:
            stats["confidence_level"] = 0.70
        else:
            stats["confidence_level"] = max(0.30, sample_size / 10 * 0.70)

        return stats

    def _calculate_decision_quality(self, trace: DecisionTrace) -> float:
        """Calculate decision quality score from decision trace."""
        score = trace.confidence_score

        # Adjust for number of alternatives considered
        alternatives_count = len(trace.alternatives_considered)
        if alternatives_count > 1:
            score += 0.1  # Bonus for considering alternatives

        # Adjust for reasoning quality
        if trace.reasoning and len(trace.reasoning) > 50:
            score += 0.05  # Bonus for detailed reasoning

        # Adjust for decision type complexity
        complex_decisions = [
            'entity_extraction', 'project_categorization', 'quality_assessment'
        ]
        if trace.decision_type in complex_decisions:
            score += 0.05  # Bonus for handling complex decisions

        return min(1.0, score)

    def _normalize_metric_value(self, value: float, metric_name: str) -> float:
        """Normalize metric value to 0-1 range."""
        metric_name_lower = metric_name.lower()

        # Already normalized metrics
        if any(term in metric_name_lower for term in ['confidence', 'accuracy', 'precision', 'recall']):
            if value <= 1.0:
                return value

        # Percentage metrics
        if 'percent' in metric_name_lower or '%' in metric_name:
            return value / 100.0

        # F1 scores are typically 0-1
        if 'f1' in metric_name_lower:
            return min(1.0, value)

        # Response time metrics (lower is better)
        if any(term in metric_name_lower for term in ['time', 'latency', 'duration']):
            # Convert to quality score (inverse relationship)
            max_acceptable_time = 10000  # 10 seconds
            return max(0.0, 1.0 - (value / max_acceptable_time))

        # Error rate metrics (lower is better)
        if 'error' in metric_name_lower:
            if value <= 1.0:
                return 1.0 - value  # Invert error rate to quality
            else:
                return max(0.0, 1.0 - (value / 100.0))  # Assume percentage

        # Default: assume 0-100 scale
        if value > 1.0:
            return value / 100.0

        return value

    # ========================================================================
    # Degradation Detection Methods
    # ========================================================================

    async def _detect_confidence_degradation(
        self,
        agent_id: str,
        current_data: List[QualityMetric],
        baseline_data: List[QualityMetric],
        time_window_hours: float
    ) -> List[QualityDegradation]:
        """Detect confidence score degradation."""
        degradations = []

        # Get confidence values
        current_confidence = [
            m.confidence_score for m in current_data
            if m.confidence_score is not None
        ]
        baseline_confidence = [
            m.confidence_score for m in baseline_data
            if m.confidence_score is not None
        ]

        if len(current_confidence) < self.MIN_SAMPLE_SIZE or len(baseline_confidence) < self.MIN_SAMPLE_SIZE:
            return degradations

        current_mean = statistics.mean(current_confidence)
        baseline_mean = statistics.mean(baseline_confidence)

        if baseline_mean == 0:
            return degradations

        degradation_pct = (baseline_mean - current_mean) / baseline_mean

        if degradation_pct >= self.DEGRADATION_THRESHOLD_MINOR:
            # Perform statistical significance test
            significance = self._calculate_statistical_significance(
                current_confidence, baseline_confidence
            )

            if significance < self.SIGNIFICANCE_THRESHOLD:
                # Determine severity
                if degradation_pct >= self.DEGRADATION_THRESHOLD_CRITICAL:
                    severity = AlertSeverity.CRITICAL
                elif degradation_pct >= self.DEGRADATION_THRESHOLD_MAJOR:
                    severity = AlertSeverity.HIGH
                else:
                    severity = AlertSeverity.MEDIUM

                degradation = QualityDegradation(
                    agent_id=agent_id,
                    degradation_type=DegradationType.CONFIDENCE_DROP,
                    severity=severity,
                    detected_at=datetime.utcnow(),
                    current_quality=current_mean,
                    baseline_quality=baseline_mean,
                    degradation_percentage=degradation_pct * 100,
                    significance_score=significance,
                    time_window_analyzed=timedelta(hours=time_window_hours),
                    sample_size=len(current_confidence),
                    contributing_factors=self._identify_confidence_factors(current_data),
                    recommendations=self._generate_confidence_recommendations(degradation_pct),
                    should_alert=severity in [AlertSeverity.HIGH, AlertSeverity.CRITICAL],
                    alert_message=f"Confidence degradation detected: {degradation_pct:.1%} decrease"
                )

                degradations.append(degradation)

        return degradations

    async def _detect_accuracy_degradation(
        self,
        agent_id: str,
        current_data: List[QualityMetric],
        baseline_data: List[QualityMetric],
        time_window_hours: float
    ) -> List[QualityDegradation]:
        """Detect accuracy degradation."""
        degradations = []

        # Get accuracy values
        current_accuracy = [
            m.metric_value for m in current_data
            if 'accuracy' in m.metric_type.lower()
        ]
        baseline_accuracy = [
            m.metric_value for m in baseline_data
            if 'accuracy' in m.metric_type.lower()
        ]

        if len(current_accuracy) < self.MIN_SAMPLE_SIZE or len(baseline_accuracy) < self.MIN_SAMPLE_SIZE:
            return degradations

        current_mean = statistics.mean(current_accuracy)
        baseline_mean = statistics.mean(baseline_accuracy)

        if baseline_mean == 0:
            return degradations

        degradation_pct = (baseline_mean - current_mean) / baseline_mean

        if degradation_pct >= self.DEGRADATION_THRESHOLD_MINOR:
            # Statistical significance test
            significance = self._calculate_statistical_significance(
                current_accuracy, baseline_accuracy
            )

            if significance < self.SIGNIFICANCE_THRESHOLD:
                # Determine severity
                if degradation_pct >= self.DEGRADATION_THRESHOLD_CRITICAL:
                    severity = AlertSeverity.CRITICAL
                elif degradation_pct >= self.DEGRADATION_THRESHOLD_MAJOR:
                    severity = AlertSeverity.HIGH
                else:
                    severity = AlertSeverity.MEDIUM

                degradation = QualityDegradation(
                    agent_id=agent_id,
                    degradation_type=DegradationType.ACCURACY_DECLINE,
                    severity=severity,
                    detected_at=datetime.utcnow(),
                    current_quality=current_mean,
                    baseline_quality=baseline_mean,
                    degradation_percentage=degradation_pct * 100,
                    significance_score=significance,
                    time_window_analyzed=timedelta(hours=time_window_hours),
                    sample_size=len(current_accuracy),
                    contributing_factors=self._identify_accuracy_factors(current_data),
                    recommendations=self._generate_accuracy_recommendations(degradation_pct),
                    should_alert=severity in [AlertSeverity.HIGH, AlertSeverity.CRITICAL],
                    alert_message=f"Accuracy degradation detected: {degradation_pct:.1%} decrease"
                )

                degradations.append(degradation)

        return degradations

    async def _detect_consistency_degradation(
        self,
        agent_id: str,
        current_data: List[QualityMetric],
        baseline_data: List[QualityMetric],
        time_window_hours: float
    ) -> List[QualityDegradation]:
        """Detect consistency degradation (increased variance)."""
        degradations = []

        # Get confidence values for consistency analysis
        current_confidence = [
            m.confidence_score for m in current_data
            if m.confidence_score is not None
        ]
        baseline_confidence = [
            m.confidence_score for m in baseline_data
            if m.confidence_score is not None
        ]

        if len(current_confidence) < self.MIN_SAMPLE_SIZE or len(baseline_confidence) < self.MIN_SAMPLE_SIZE:
            return degradations

        current_std = statistics.stdev(current_confidence)
        baseline_std = statistics.stdev(baseline_confidence)

        if baseline_std == 0:
            return degradations

        variance_increase = (current_std - baseline_std) / baseline_std

        if variance_increase >= 0.5:  # 50% increase in standard deviation
            severity = AlertSeverity.MEDIUM
            if variance_increase >= 1.0:  # 100% increase
                severity = AlertSeverity.HIGH

            degradation = QualityDegradation(
                agent_id=agent_id,
                degradation_type=DegradationType.CONSISTENCY_LOSS,
                severity=severity,
                detected_at=datetime.utcnow(),
                current_quality=1.0 - (current_std * 2),  # Convert std to consistency score
                baseline_quality=1.0 - (baseline_std * 2),
                degradation_percentage=variance_increase * 100,
                significance_score=0.01,  # Assume significant for variance changes
                time_window_analyzed=timedelta(hours=time_window_hours),
                sample_size=len(current_confidence),
                contributing_factors=['increased_variance', 'inconsistent_decisions'],
                recommendations=[
                    'Review decision-making parameters',
                    'Check for environmental changes affecting agent'
                ],
                should_alert=severity == AlertSeverity.HIGH,
                alert_message=f"Consistency degradation: {variance_increase:.1%} increase in variance"
            )

            degradations.append(degradation)

        return degradations

    async def _detect_performance_regression(
        self,
        agent_id: str,
        current_data: List[QualityMetric],
        baseline_data: List[QualityMetric],
        time_window_hours: float
    ) -> List[QualityDegradation]:
        """Detect performance regression in quality metrics."""
        degradations = []

        # Check for declining trends in quality metrics
        quality_values = [
            m.metric_value for m in current_data
            if m.metric_type in ['decision_quality', 'overall_quality']
        ]

        if len(quality_values) >= 5:
            # Check for declining trend
            x_values = list(range(len(quality_values)))
            slope = self._calculate_trend_slope(x_values, quality_values)

            if slope < -0.1:  # Significant negative trend
                current_mean = statistics.mean(quality_values)
                baseline_quality_values = [
                    m.metric_value for m in baseline_data
                    if m.metric_type in ['decision_quality', 'overall_quality']
                ]

                if baseline_quality_values:
                    baseline_mean = statistics.mean(baseline_quality_values)
                    degradation_pct = (baseline_mean - current_mean) / baseline_mean if baseline_mean > 0 else 0

                    if degradation_pct >= self.DEGRADATION_THRESHOLD_MINOR:
                        severity = AlertSeverity.MEDIUM
                        if degradation_pct >= self.DEGRADATION_THRESHOLD_MAJOR:
                            severity = AlertSeverity.HIGH

                        degradation = QualityDegradation(
                            agent_id=agent_id,
                            degradation_type=DegradationType.PERFORMANCE_REGRESSION,
                            severity=severity,
                            detected_at=datetime.utcnow(),
                            current_quality=current_mean,
                            baseline_quality=baseline_mean,
                            degradation_percentage=degradation_pct * 100,
                            significance_score=abs(slope),
                            time_window_analyzed=timedelta(hours=time_window_hours),
                            sample_size=len(quality_values),
                            contributing_factors=['declining_trend', 'performance_regression'],
                            recommendations=[
                                'Investigate recent system changes',
                                'Review agent configuration',
                                'Check for resource constraints'
                            ],
                            should_alert=severity == AlertSeverity.HIGH,
                            alert_message=f"Performance regression: declining quality trend detected"
                        )

                        degradations.append(degradation)

        return degradations

    async def _check_immediate_degradation(
        self,
        agent_id: str,
        metric_type: str,
        value: float
    ) -> None:
        """Check for immediate quality degradation on new measurement."""
        # Get recent measurements for comparison
        recent_window_hours = 1.0
        recent_data = await self._gather_quality_data(agent_id, recent_window_hours)

        if metric_type == "confidence":
            recent_values = [
                m.confidence_score for m in recent_data
                if m.confidence_score is not None and
                   m.timestamp > datetime.utcnow() - timedelta(hours=recent_window_hours)
            ]
        else:  # accuracy
            recent_values = [
                m.metric_value for m in recent_data
                if 'accuracy' in m.metric_type.lower() and
                   m.timestamp > datetime.utcnow() - timedelta(hours=recent_window_hours)
            ]

        if len(recent_values) >= 3:
            recent_avg = statistics.mean(recent_values[:-1])  # Exclude current value

            if recent_avg > 0:
                immediate_drop = (recent_avg - value) / recent_avg

                # Check for significant immediate drop
                if immediate_drop >= 0.3:  # 30% immediate drop
                    logger.warning(
                        f"Immediate {metric_type} degradation detected for agent {agent_id}: "
                        f"{immediate_drop:.1%} drop to {value:.3f}"
                    )

    def _calculate_statistical_significance(
        self,
        current_values: List[float],
        baseline_values: List[float]
    ) -> float:
        """Calculate statistical significance using Welch's t-test."""
        try:
            # Simple implementation of Welch's t-test
            current_mean = statistics.mean(current_values)
            baseline_mean = statistics.mean(baseline_values)

            current_var = statistics.variance(current_values)
            baseline_var = statistics.variance(baseline_values)

            n1, n2 = len(current_values), len(baseline_values)

            # Standard error of the difference
            se_diff = math.sqrt(current_var/n1 + baseline_var/n2)

            if se_diff == 0:
                return 1.0  # No difference

            # T-statistic
            t_stat = abs(current_mean - baseline_mean) / se_diff

            # Degrees of freedom (Welch-Satterthwaite equation)
            df = ((current_var/n1 + baseline_var/n2)**2) / (
                (current_var/n1)**2/(n1-1) + (baseline_var/n2)**2/(n2-1)
            )

            # Approximate p-value (simplified)
            # For most practical purposes with reasonable sample sizes
            if t_stat > 3.0:
                return 0.001  # Very significant
            elif t_stat > 2.0:
                return 0.01   # Significant
            elif t_stat > 1.5:
                return 0.05   # Marginally significant
            else:
                return 0.1    # Not significant

        except Exception:
            return 1.0  # Assume not significant if calculation fails

    def _calculate_trend_slope(self, x_values: List[float], y_values: List[float]) -> float:
        """Calculate linear trend slope."""
        n = len(x_values)
        if n < 2:
            return 0.0

        x_mean = statistics.mean(x_values)
        y_mean = statistics.mean(y_values)

        numerator = sum((x_values[i] - x_mean) * (y_values[i] - y_mean) for i in range(n))
        denominator = sum((x_values[i] - x_mean) ** 2 for i in range(n))

        return numerator / denominator if denominator != 0 else 0.0

    # ========================================================================
    # Helper Methods - Utilities
    # ========================================================================

    def _is_in_time_window(
        self,
        timestamp: datetime,
        window_hours: float,
        exclude_recent_hours: float = 0.0
    ) -> bool:
        """Check if timestamp is in specified time window."""
        now = datetime.utcnow()
        window_start = now - timedelta(hours=window_hours + exclude_recent_hours)
        window_end = now - timedelta(hours=exclude_recent_hours)

        return window_start <= timestamp <= window_end

    def _is_in_alert_cooldown(self, agent_id: str) -> bool:
        """Check if agent is in alert cooldown period."""
        if agent_id not in self._alert_cooldowns:
            return False

        last_alert = self._alert_cooldowns[agent_id]
        return datetime.utcnow() - last_alert < self._alert_cooldown_period

    def _determine_alert_severity(
        self,
        current_value: float,
        threshold_value: float
    ) -> AlertSeverity:
        """Determine alert severity based on deviation."""
        if threshold_value == 0:
            return AlertSeverity.MEDIUM

        deviation_pct = abs(threshold_value - current_value) / threshold_value

        if deviation_pct >= 0.5:  # 50% deviation
            return AlertSeverity.CRITICAL
        elif deviation_pct >= 0.3:  # 30% deviation
            return AlertSeverity.HIGH
        elif deviation_pct >= 0.15:  # 15% deviation
            return AlertSeverity.MEDIUM
        else:
            return AlertSeverity.LOW

    def _identify_contributing_metrics(self, assessment: QualityAssessment) -> List[str]:
        """Identify metrics contributing to low quality."""
        contributing = []

        if assessment.confidence_score and assessment.confidence_score < 0.7:
            contributing.append("confidence_score")

        if assessment.accuracy_score and assessment.accuracy_score < 0.8:
            contributing.append("accuracy_score")

        if assessment.consistency_score and assessment.consistency_score < 0.6:
            contributing.append("consistency_score")

        if assessment.reliability_score and assessment.reliability_score < 0.7:
            contributing.append("reliability_score")

        return contributing

    def _generate_quality_recommendations(self, assessment: QualityAssessment) -> List[str]:
        """Generate quality improvement recommendations."""
        recommendations = []

        if assessment.confidence_score and assessment.confidence_score < 0.7:
            recommendations.append("Review decision-making logic and training data")

        if assessment.accuracy_score and assessment.accuracy_score < 0.8:
            recommendations.append("Validate model performance and update if necessary")

        if assessment.consistency_score and assessment.consistency_score < 0.6:
            recommendations.append("Stabilize decision parameters and reduce variance")

        if assessment.sample_size < 20:
            recommendations.append("Increase measurement frequency for better assessment")

        return recommendations or ["Continue monitoring quality metrics"]

    def _identify_confidence_factors(self, current_data: List[QualityMetric]) -> List[str]:
        """Identify factors contributing to confidence degradation."""
        factors = []

        # Check for specific decision types with low confidence
        decision_confidence = defaultdict(list)
        for m in current_data:
            if m.confidence_score and m.context.get('decision_type'):
                decision_confidence[m.context['decision_type']].append(m.confidence_score)

        for decision_type, confidences in decision_confidence.items():
            if confidences and statistics.mean(confidences) < 0.6:
                factors.append(f"low_confidence_in_{decision_type}")

        # Check for sparse alternative consideration
        alternatives_counts = [
            m.context.get('alternatives_count', 0) for m in current_data
            if 'alternatives_count' in m.context
        ]

        if alternatives_counts and statistics.mean(alternatives_counts) < 2:
            factors.append("insufficient_alternatives_considered")

        return factors or ["general_confidence_decline"]

    def _generate_confidence_recommendations(self, degradation_pct: float) -> List[str]:
        """Generate recommendations for confidence degradation."""
        recommendations = []

        if degradation_pct > 0.3:
            recommendations.extend([
                "Immediate review of decision-making algorithms required",
                "Check for data quality issues in inputs",
                "Validate agent configuration and parameters"
            ])
        elif degradation_pct > 0.15:
            recommendations.extend([
                "Review recent decision patterns",
                "Consider retraining or updating decision models",
                "Monitor input data consistency"
            ])
        else:
            recommendations.extend([
                "Continue monitoring confidence trends",
                "Consider minor parameter adjustments"
            ])

        return recommendations

    def _identify_accuracy_factors(self, current_data: List[QualityMetric]) -> List[str]:
        """Identify factors contributing to accuracy degradation."""
        return ["accuracy_measurement_decline", "potential_model_drift"]

    def _generate_accuracy_recommendations(self, degradation_pct: float) -> List[str]:
        """Generate recommendations for accuracy degradation."""
        recommendations = []

        if degradation_pct > 0.2:
            recommendations.extend([
                "Immediate model validation and potential retraining required",
                "Check ground truth data for quality issues",
                "Review recent system changes"
            ])
        else:
            recommendations.extend([
                "Monitor accuracy trends closely",
                "Consider incremental model updates"
            ])

        return recommendations

    def _cleanup_resolved_degradations(self, agent_id: str) -> None:
        """Clean up degradations that are no longer active."""
        current_time = datetime.utcnow()

        # Remove degradations older than 24 hours
        self._active_degradations[agent_id] = [
            d for d in self._active_degradations[agent_id]
            if current_time - d.detected_at < timedelta(hours=24)
        ]

    def _generate_system_recommendations(
        self,
        assessments: List[QualityAssessment],
        degradations: List[QualityDegradation],
        alerts: List[QualityAlert]
    ) -> List[str]:
        """Generate system-wide quality recommendations."""
        recommendations = []

        # Analyze overall system health
        quality_scores = [a.overall_quality_score for a in assessments]
        if quality_scores:
            avg_quality = statistics.mean(quality_scores)

            if avg_quality < 0.7:
                recommendations.append("System-wide quality below acceptable levels - comprehensive review needed")

            # Check for consistency issues
            if len(quality_scores) > 1:
                quality_std = statistics.stdev(quality_scores)
                if quality_std > 0.2:
                    recommendations.append("High variability in agent quality - standardize configurations")

        # Analyze degradation patterns
        if len(degradations) > 3:
            recommendations.append("Multiple degradations detected - investigate common causes")

        # Critical alerts
        critical_alerts = [a for a in alerts if a.severity == AlertSeverity.CRITICAL]
        if critical_alerts:
            recommendations.append("Critical quality issues require immediate attention")

        return recommendations or ["System quality appears stable"]

    def _generate_priority_actions(
        self,
        degradations: List[QualityDegradation],
        alerts: List[QualityAlert]
    ) -> List[str]:
        """Generate priority actions based on degradations and alerts."""
        actions = []

        # Critical degradations
        critical_degradations = [d for d in degradations if d.severity == AlertSeverity.CRITICAL]
        for degradation in critical_degradations:
            actions.append(
                f"Critical: Address {degradation.degradation_type.value} for {degradation.agent_id}"
            )

        # Critical alerts
        critical_alerts = [a for a in alerts if a.severity == AlertSeverity.CRITICAL]
        for alert in critical_alerts:
            actions.append(f"Critical: {alert.alert_type} for {alert.agent_id}")

        # High priority items
        high_priority = [d for d in degradations if d.severity == AlertSeverity.HIGH]
        for degradation in high_priority[:3]:  # Top 3
            actions.append(
                f"High: {degradation.degradation_type.value} in {degradation.agent_id}"
            )

        return actions

    async def _get_quality_trend_analysis(
        self,
        agent_id: str,
        time_range_hours: float
    ) -> QualityTrendAnalysis:
        """Get quality trend analysis for an agent."""
        # Get historical quality data
        quality_data = await self._gather_quality_data(agent_id, time_range_hours)

        if len(quality_data) < 5:
            return QualityTrendAnalysis(
                agent_id=agent_id,
                trend_direction=QualityTrend.STABLE,
                trend_strength=0.0,
                slope=0.0,
                volatility=0.0,
                data_points=len(quality_data),
                time_span=timedelta(hours=time_range_hours),
                predicted_quality_1h=None,
                predicted_quality_24h=None,
                predicted_quality_7d=None
            )

        # Calculate overall quality scores over time
        time_quality_pairs = []
        for i, measurement in enumerate(quality_data[-20:]):  # Last 20 measurements
            quality_score = measurement.metric_value
            if measurement.metric_type == "decision_trace_confidence":
                quality_score = measurement.confidence_score
            time_quality_pairs.append((i, quality_score))

        if len(time_quality_pairs) < 3:
            return QualityTrendAnalysis(
                agent_id=agent_id,
                trend_direction=QualityTrend.STABLE,
                trend_strength=0.0,
                slope=0.0,
                volatility=0.0,
                data_points=len(quality_data),
                time_span=timedelta(hours=time_range_hours),
                predicted_quality_1h=None,
                predicted_quality_24h=None,
                predicted_quality_7d=None
            )

        # Calculate trend metrics
        x_values = [pair[0] for pair in time_quality_pairs]
        y_values = [pair[1] for pair in time_quality_pairs]

        slope = self._calculate_trend_slope(x_values, y_values)
        volatility = statistics.stdev(y_values) if len(y_values) > 1 else 0.0

        # Determine trend direction
        if abs(slope) < 0.01:
            trend_direction = QualityTrend.STABLE
        elif slope > 0:
            trend_direction = QualityTrend.IMPROVING
        else:
            trend_direction = QualityTrend.DEGRADING

        if volatility > 0.2:
            trend_direction = QualityTrend.VOLATILE

        # Calculate trend strength
        trend_strength = min(1.0, abs(slope) * 10 + (1.0 - volatility))

        # Make predictions
        current_quality = y_values[-1] if y_values else 0.0
        predicted_1h = max(0.0, min(1.0, current_quality + slope * 1))
        predicted_24h = max(0.0, min(1.0, current_quality + slope * 24))
        predicted_7d = max(0.0, min(1.0, current_quality + slope * 168))

        return QualityTrendAnalysis(
            agent_id=agent_id,
            trend_direction=trend_direction,
            trend_strength=trend_strength,
            slope=slope,
            volatility=volatility,
            data_points=len(quality_data),
            time_span=timedelta(hours=time_range_hours),
            predicted_quality_1h=predicted_1h,
            predicted_quality_24h=predicted_24h,
            predicted_quality_7d=predicted_7d
        )

    async def _get_quality_benchmarks(self, agent_id: str) -> List[QualityBenchmark]:
        """Get quality benchmarks for an agent."""
        benchmarks = []

        # Get current assessment
        assessment = await self.get_quality_assessment(agent_id, 24.0)

        # Compare against baselines
        baselines = self._quality_baselines.get(agent_id, {})

        for metric_type, baseline_value in baselines.items():
            current_value = None

            if metric_type == "confidence" and assessment.confidence_score:
                current_value = assessment.confidence_score
            elif metric_type == "accuracy" and assessment.accuracy_score:
                current_value = assessment.accuracy_score
            elif metric_type == "overall":
                current_value = assessment.overall_quality_score

            if current_value is not None:
                if current_value >= baseline_value * 1.1:
                    status = "above_benchmark"
                    improvement_needed = None
                elif current_value >= baseline_value * 0.95:
                    status = "at_benchmark"
                    improvement_needed = None
                else:
                    status = "below_benchmark"
                    improvement_needed = baseline_value - current_value

                # Calculate percentile rank (simplified)
                percentile = min(100, (current_value / baseline_value) * 50)

                benchmark = QualityBenchmark(
                    agent_id=agent_id,
                    metric_type=metric_type,
                    current_value=current_value,
                    benchmark_value=baseline_value,
                    percentile_rank=percentile,
                    comparison_status=status,
                    improvement_needed=improvement_needed
                )
                benchmarks.append(benchmark)

        return benchmarks

    async def _load_quality_baselines(self) -> None:
        """Load quality baselines from agent configurations."""
        try:
            async with self._session_factory() as session:
                result = await session.execute(select(Agent))
                agents = result.scalars().all()

                for agent in agents:
                    config = agent.configuration or {}
                    baselines = config.get('quality_baselines', {})
                    if baselines:
                        self._quality_baselines[agent.agent_id] = baselines

                logger.info(f"Loaded quality baselines for {len(self._quality_baselines)} agents")

        except Exception as e:
            logger.warning(f"Could not load quality baselines: {e}")

    async def _load_quality_targets(self) -> None:
        """Load quality targets from agent configurations."""
        try:
            async with self._session_factory() as session:
                result = await session.execute(select(Agent))
                agents = result.scalars().all()

                for agent in agents:
                    config = agent.configuration or {}
                    targets = config.get('quality_targets', {})
                    if targets:
                        self._quality_targets[agent.agent_id] = targets

                logger.info(f"Loaded quality targets for {len(self._quality_targets)} agents")

        except Exception as e:
            logger.warning(f"Could not load quality targets: {e}")

    async def _load_agent_thresholds(self) -> None:
        """Load agent-specific quality thresholds."""
        try:
            async with self._session_factory() as session:
                # Get alert thresholds for quality metrics
                result = await session.execute(
                    select(AlertThreshold).where(
                        AlertThreshold.threshold_type == ThresholdType.QUALITY.value
                    )
                )
                thresholds = result.scalars().all()

                for threshold in thresholds:
                    if threshold.agent_id not in self._agent_thresholds:
                        self._agent_thresholds[threshold.agent_id] = {}

                    self._agent_thresholds[threshold.agent_id][threshold.metric_name] = threshold.threshold_value

                logger.info(f"Loaded quality thresholds for {len(self._agent_thresholds)} agents")

        except Exception as e:
            logger.warning(f"Could not load agent thresholds: {e}")

    def _validate_agent_id(self, agent_id: str) -> None:
        """Validate agent ID format."""
        if not agent_id or not isinstance(agent_id, str):
            raise ValidationError("Agent ID must be a non-empty string")

        if len(agent_id) < 3 or len(agent_id) > 100:
            raise ValidationError("Agent ID must be between 3 and 100 characters")

    def _validate_confidence_score(self, score: float) -> None:
        """Validate confidence score range."""
        if not isinstance(score, (int, float)):
            raise ValidationError("Confidence score must be numeric")

        if not 0.0 <= score <= 1.0:
            raise ValidationError("Confidence score must be between 0.0 and 1.0")

    def _validate_score(self, score: float, score_name: str) -> None:
        """Validate generic score range."""
        if not isinstance(score, (int, float)):
            raise ValidationError(f"{score_name} must be numeric")

        if not 0.0 <= score <= 1.0:
            raise ValidationError(f"{score_name} must be between 0.0 and 1.0")

    async def get_service_stats(self) -> Dict[str, Any]:
        """Get performance statistics about the quality tracker service."""
        return {
            "service_running": self._running,
            "agents_tracked": len(self._confidence_buffer),
            "total_confidence_measurements": sum(len(buffer) for buffer in self._confidence_buffer.values()),
            "total_accuracy_measurements": sum(len(buffer) for buffer in self._accuracy_buffer.values()),
            "active_degradations": sum(len(degradations) for degradations in self._active_degradations.values()),
            "active_alerts": sum(len(alerts) for alerts in self._active_alerts.values()),
            "quality_baselines_loaded": len(self._quality_baselines),
            "quality_targets_loaded": len(self._quality_targets),
            "average_assessment_time_ms": round(statistics.mean(self._assessment_times) * 1000, 2) if self._assessment_times else 0,
            "recent_assessments": len(self._assessment_times)
        }
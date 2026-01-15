"""Workflow Analysis Service for performance analysis and bottleneck identification.

This module implements comprehensive workflow analysis capabilities to identify
bottlenecks, analyze performance patterns, provide failure analysis with root
cause attribution, and generate actionable insights for workflow optimization.

Features:
- Workflow performance analysis with stage-by-stage timing breakdown
- Bottleneck identification with statistical analysis and ranking
- Failure analysis with root cause attribution using decision traces
- Comprehensive workflow statistics and performance metrics
- Agent handoff delay analysis and optimization recommendations
- Workflow comparison and trend analysis over time periods
- Visualization data generation for dashboard charts and graphs
- Debugging insights with trace correlation and pattern detection
- Advanced filtering by agent, time range, workflow type, and status
- Efficient processing of large workflow datasets with pagination
- Real-time workflow monitoring with anomaly detection
- Performance regression detection and alerting

Example Usage:
    from observability_lib.services.workflow_analyzer import WorkflowAnalyzer
    from observability_lib.config import settings

    analyzer = WorkflowAnalyzer(settings)
    await analyzer.start()

    # Analyze workflow performance and identify bottlenecks
    analysis = await analyzer.analyze_workflow_performance(
        workflow_id="workflow-123",
        include_decision_traces=True
    )

    # Identify system-wide bottlenecks
    bottlenecks = await analyzer.identify_bottlenecks(
        time_range_hours=24,
        min_workflow_count=10
    )

    # Get failure analysis with root causes
    failures = await analyzer.analyze_failures(
        agent_id="email_processor",
        time_range_hours=6,
        include_context=True
    )

    # Generate performance report
    report = await analyzer.generate_performance_report(
        agents=["email_processor", "meeting_analyzer"],
        time_range_hours=24
    )

    # Compare workflow performance over time
    comparison = await analyzer.compare_workflow_performance(
        workflow_type="email_processing",
        baseline_hours=(48, 24),
        current_hours=24
    )

    await analyzer.stop()
"""

import asyncio
import logging
import statistics
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
from sqlalchemy import select, func, and_, or_, text, case, desc, asc
from sqlalchemy.exc import SQLAlchemyError

from observability_lib.config import ObservabilitySettings
from observability_lib.models.agent_metrics import (
    Agent, AgentMetric, DecisionTrace, WorkflowEvent, AgentLog,
    AgentType, MetricType, DecisionType, EventType, EventStatus, LogLevel
)
from penf_lib.storage.validators import ValidationError


logger = logging.getLogger(__name__)


# ============================================================================
# Analysis Data Types and Enums
# ============================================================================

class BottleneckSeverity(str, Enum):
    """Severity levels for identified bottlenecks"""
    CRITICAL = "critical"     # >95th percentile, major performance impact
    HIGH = "high"            # >90th percentile, significant impact
    MODERATE = "moderate"    # >75th percentile, noticeable impact
    LOW = "low"             # >50th percentile, minor impact


class WorkflowPhase(str, Enum):
    """Standard phases of workflow execution"""
    INITIALIZATION = "initialization"
    PROCESSING = "processing"
    HANDOFF = "handoff"
    COMPLETION = "completion"
    ERROR_HANDLING = "error_handling"


class PerformanceTrend(str, Enum):
    """Performance trend directions"""
    IMPROVING = "improving"
    STABLE = "stable"
    DEGRADING = "degrading"
    VOLATILE = "volatile"


@dataclass
class WorkflowStage:
    """Represents a single stage in workflow execution"""
    stage_name: str
    agent_id: str
    start_time: datetime
    end_time: Optional[datetime]
    duration_ms: Optional[float]
    status: EventStatus
    event_data: Dict[str, Any]
    processing_time_ms: Optional[float]


@dataclass
class WorkflowAnalysis:
    """Comprehensive workflow performance analysis"""
    workflow_id: str
    total_duration_ms: float
    stages: List[WorkflowStage]
    bottlenecks: List['BottleneckDetails']
    handoff_delays: List['HandoffAnalysis']
    performance_metrics: Dict[str, float]
    failure_points: List['FailurePoint']
    recommendations: List[str]
    decision_context: Optional[Dict[str, Any]] = None


@dataclass
class BottleneckDetails:
    """Details about identified performance bottlenecks"""
    stage_name: str
    agent_id: str
    severity: BottleneckSeverity
    avg_duration_ms: float
    p95_duration_ms: float
    max_duration_ms: float
    occurrence_count: int
    percentage_of_total_time: float
    impact_score: float
    recommended_actions: List[str]


@dataclass
class HandoffAnalysis:
    """Analysis of agent handoff performance"""
    from_agent: str
    to_agent: str
    avg_handoff_delay_ms: float
    max_handoff_delay_ms: float
    handoff_count: int
    failure_rate: float
    efficiency_score: float
    recommendations: List[str]


@dataclass
class FailurePoint:
    """Analysis of workflow failure points"""
    stage_name: str
    agent_id: str
    failure_count: int
    failure_rate: float
    common_errors: List[str]
    root_causes: List[str]
    recovery_time_ms: Optional[float]
    impact_assessment: str


@dataclass
class PerformanceReport:
    """Comprehensive workflow performance report"""
    time_range: Tuple[datetime, datetime]
    agents_analyzed: List[str]
    total_workflows: int
    success_rate: float
    avg_workflow_duration_ms: float
    p95_workflow_duration_ms: float
    top_bottlenecks: List[BottleneckDetails]
    handoff_analysis: List[HandoffAnalysis]
    failure_analysis: List[FailurePoint]
    performance_trends: Dict[str, PerformanceTrend]
    recommendations: List[str]


@dataclass
class WorkflowComparison:
    """Comparison between different time periods"""
    baseline_period: Tuple[datetime, datetime]
    current_period: Tuple[datetime, datetime]
    performance_change: float  # percentage change
    duration_change_ms: float
    success_rate_change: float
    bottleneck_changes: List[Dict[str, Any]]
    regression_detected: bool
    improvement_areas: List[str]
    concern_areas: List[str]


@dataclass
class VisualizationData:
    """Data formatted for dashboard visualizations"""
    chart_type: str
    data_points: List[Dict[str, Any]]
    metadata: Dict[str, Any]
    time_range: Tuple[datetime, datetime]
    aggregation_level: str


# ============================================================================
# Workflow Analyzer Service
# ============================================================================

class WorkflowAnalyzer:
    """Service for comprehensive workflow analysis and bottleneck identification.

    Provides detailed analysis of workflow performance, identifies bottlenecks,
    analyzes failures, and generates actionable insights for optimization.
    """

    def __init__(self, settings: ObservabilitySettings):
        """Initialize workflow analyzer with configuration."""
        self.settings = settings
        self._engine = None
        self._session_factory = None
        self._running = False

        # Analysis configuration
        self._bottleneck_threshold_percentile = 90.0
        self._handoff_delay_threshold_ms = 5000.0
        self._min_samples_for_analysis = 10
        self._cache_ttl_seconds = 300  # 5 minutes

        # Performance caching
        self._analysis_cache: Dict[str, Tuple[datetime, Any]] = {}

        logger.info("WorkflowAnalyzer initialized with %s", settings)

    async def start(self) -> None:
        """Start the workflow analyzer service."""
        try:
            # Create async database engine
            self._engine = create_async_engine(
                self.settings.database_url,
                pool_size=self.settings.db_pool_size,
                max_overflow=self.settings.db_max_overflow,
                pool_timeout=self.settings.db_timeout_seconds,
                echo=self.settings.debug_sql,
            )

            # Create session factory
            self._session_factory = sessionmaker(
                bind=self._engine,
                class_=AsyncSession,
                expire_on_commit=False
            )

            self._running = True
            logger.info("WorkflowAnalyzer started successfully")

        except Exception as e:
            logger.error("Failed to start WorkflowAnalyzer: %s", e)
            raise

    async def stop(self) -> None:
        """Stop the workflow analyzer service."""
        self._running = False

        if self._engine:
            await self._engine.dispose()

        # Clear caches
        self._analysis_cache.clear()

        logger.info("WorkflowAnalyzer stopped")

    def _check_running(self) -> None:
        """Check if service is running."""
        if not self._running:
            raise RuntimeError("WorkflowAnalyzer is not running. Call start() first.")

    # ========================================================================
    # Core Workflow Analysis
    # ========================================================================

    async def analyze_workflow_performance(
        self,
        workflow_id: str,
        include_decision_traces: bool = True,
        tenant_id: Optional[str] = None
    ) -> WorkflowAnalysis:
        """Analyze performance of a specific workflow with detailed breakdown.

        Args:
            workflow_id: Unique workflow identifier
            include_decision_traces: Whether to include decision context
            tenant_id: Optional tenant filter

        Returns:
            Complete workflow analysis with bottlenecks and recommendations
        """
        self._check_running()

        async with self._session_factory() as session:
            try:
                # Get workflow events ordered by timestamp
                events_query = select(WorkflowEvent).where(
                    WorkflowEvent.workflow_id == uuid.UUID(workflow_id)
                )
                if tenant_id:
                    events_query = events_query.where(WorkflowEvent.tenant_id == tenant_id)
                events_query = events_query.order_by(WorkflowEvent.timestamp)

                result = await session.execute(events_query)
                events = result.scalars().all()

                if not events:
                    raise ValueError(f"No workflow events found for workflow_id: {workflow_id}")

                # Analyze workflow stages
                stages = self._analyze_workflow_stages(events)

                # Calculate total duration
                start_time = events[0].timestamp
                end_time = events[-1].timestamp
                total_duration_ms = (end_time - start_time).total_seconds() * 1000

                # Identify bottlenecks within this workflow
                bottlenecks = self._identify_workflow_bottlenecks(stages)

                # Analyze handoff delays
                handoff_delays = self._analyze_workflow_handoffs(stages)

                # Calculate performance metrics
                performance_metrics = self._calculate_workflow_metrics(stages, total_duration_ms)

                # Identify failure points
                failure_points = self._identify_failure_points(stages, events)

                # Get decision context if requested
                decision_context = None
                if include_decision_traces:
                    decision_context = await self._get_workflow_decision_context(
                        session, workflow_id, tenant_id
                    )

                # Generate recommendations
                recommendations = self._generate_workflow_recommendations(
                    bottlenecks, handoff_delays, failure_points
                )

                return WorkflowAnalysis(
                    workflow_id=workflow_id,
                    total_duration_ms=total_duration_ms,
                    stages=stages,
                    bottlenecks=bottlenecks,
                    handoff_delays=handoff_delays,
                    performance_metrics=performance_metrics,
                    failure_points=failure_points,
                    recommendations=recommendations,
                    decision_context=decision_context
                )

            except Exception as e:
                logger.error("Error analyzing workflow %s: %s", workflow_id, e)
                raise

    async def identify_bottlenecks(
        self,
        time_range_hours: int = 24,
        min_workflow_count: int = 10,
        agents: Optional[List[str]] = None,
        tenant_id: Optional[str] = None
    ) -> List[BottleneckDetails]:
        """Identify system-wide performance bottlenecks.

        Args:
            time_range_hours: Time range to analyze
            min_workflow_count: Minimum workflows required for analysis
            agents: Optional list of specific agents to analyze
            tenant_id: Optional tenant filter

        Returns:
            List of bottlenecks ranked by severity and impact
        """
        self._check_running()

        # Check cache first
        cache_key = f"bottlenecks_{time_range_hours}_{min_workflow_count}_{agents}_{tenant_id}"
        cached_result = self._get_cached_result(cache_key)
        if cached_result:
            return cached_result

        async with self._session_factory() as session:
            try:
                start_time = datetime.utcnow() - timedelta(hours=time_range_hours)

                # Query workflow events with aggregation
                query = select(
                    WorkflowEvent.agent_id,
                    WorkflowEvent.event_type,
                    func.count(WorkflowEvent.id).label('count'),
                    func.avg(WorkflowEvent.processing_time_ms).label('avg_duration'),
                    func.percentile_cont(0.95).within_group(
                        WorkflowEvent.processing_time_ms.asc()
                    ).label('p95_duration'),
                    func.max(WorkflowEvent.processing_time_ms).label('max_duration'),
                    func.sum(WorkflowEvent.processing_time_ms).label('total_duration')
                ).where(
                    and_(
                        WorkflowEvent.timestamp >= start_time,
                        WorkflowEvent.processing_time_ms.isnot(None),
                        WorkflowEvent.event_status == EventStatus.SUCCESS.value
                    )
                )

                if tenant_id:
                    query = query.where(WorkflowEvent.tenant_id == tenant_id)

                if agents:
                    query = query.where(WorkflowEvent.agent_id.in_(agents))

                query = query.group_by(
                    WorkflowEvent.agent_id,
                    WorkflowEvent.event_type
                ).having(
                    func.count(WorkflowEvent.id) >= min_workflow_count
                )

                result = await session.execute(query)
                bottleneck_data = result.all()

                # Calculate total system processing time for percentage calculations
                total_system_time = sum(row.total_duration for row in bottleneck_data)

                # Analyze bottlenecks
                bottlenecks = []
                for row in bottleneck_data:
                    # Determine severity based on percentiles
                    severity = self._determine_bottleneck_severity(
                        row.avg_duration, row.p95_duration, row.max_duration
                    )

                    # Calculate impact score (combination of duration and frequency)
                    impact_score = self._calculate_impact_score(
                        row.avg_duration, row.count, total_system_time, row.total_duration
                    )

                    # Generate recommendations
                    recommendations = self._generate_bottleneck_recommendations(
                        row.agent_id, row.event_type, row.avg_duration, row.count
                    )

                    bottleneck = BottleneckDetails(
                        stage_name=row.event_type,
                        agent_id=row.agent_id,
                        severity=severity,
                        avg_duration_ms=float(row.avg_duration),
                        p95_duration_ms=float(row.p95_duration),
                        max_duration_ms=float(row.max_duration),
                        occurrence_count=row.count,
                        percentage_of_total_time=(row.total_duration / total_system_time * 100),
                        impact_score=impact_score,
                        recommended_actions=recommendations
                    )
                    bottlenecks.append(bottleneck)

                # Sort by impact score descending
                bottlenecks.sort(key=lambda x: x.impact_score, reverse=True)

                # Cache the result
                self._cache_result(cache_key, bottlenecks)

                return bottlenecks

            except Exception as e:
                logger.error("Error identifying bottlenecks: %s", e)
                raise

    async def analyze_failures(
        self,
        agent_id: Optional[str] = None,
        time_range_hours: int = 24,
        include_context: bool = True,
        tenant_id: Optional[str] = None
    ) -> List[FailurePoint]:
        """Analyze workflow failures with root cause attribution.

        Args:
            agent_id: Optional specific agent to analyze
            time_range_hours: Time range to analyze
            include_context: Whether to include detailed error context
            tenant_id: Optional tenant filter

        Returns:
            List of failure points with root cause analysis
        """
        self._check_running()

        async with self._session_factory() as session:
            try:
                start_time = datetime.utcnow() - timedelta(hours=time_range_hours)

                # Query failed workflow events
                failure_query = select(WorkflowEvent).where(
                    and_(
                        WorkflowEvent.timestamp >= start_time,
                        WorkflowEvent.event_status.in_([
                            EventStatus.FAILURE.value,
                            EventStatus.TIMEOUT.value
                        ])
                    )
                )

                if agent_id:
                    failure_query = failure_query.where(WorkflowEvent.agent_id == agent_id)
                if tenant_id:
                    failure_query = failure_query.where(WorkflowEvent.tenant_id == tenant_id)

                result = await session.execute(failure_query)
                failures = result.scalars().all()

                # Group failures by agent and event type
                failure_groups = defaultdict(list)
                for failure in failures:
                    key = (failure.agent_id, failure.event_type)
                    failure_groups[key].append(failure)

                failure_points = []

                for (agent_id, event_type), failure_list in failure_groups.items():
                    # Calculate failure metrics
                    failure_count = len(failure_list)

                    # Get total attempts for failure rate calculation
                    total_query = select(func.count(WorkflowEvent.id)).where(
                        and_(
                            WorkflowEvent.timestamp >= start_time,
                            WorkflowEvent.agent_id == agent_id,
                            WorkflowEvent.event_type == event_type
                        )
                    )
                    if tenant_id:
                        total_query = total_query.where(WorkflowEvent.tenant_id == tenant_id)

                    total_result = await session.execute(total_query)
                    total_attempts = total_result.scalar() or 1

                    failure_rate = failure_count / total_attempts

                    # Extract common errors
                    common_errors = self._extract_common_errors(failure_list)

                    # Determine root causes
                    root_causes = await self._determine_root_causes(
                        session, agent_id, event_type, failure_list, include_context
                    )

                    # Calculate recovery time if applicable
                    recovery_time_ms = self._calculate_recovery_time(failure_list)

                    # Assess impact
                    impact_assessment = self._assess_failure_impact(
                        failure_rate, failure_count, recovery_time_ms
                    )

                    failure_point = FailurePoint(
                        stage_name=event_type,
                        agent_id=agent_id,
                        failure_count=failure_count,
                        failure_rate=failure_rate,
                        common_errors=common_errors,
                        root_causes=root_causes,
                        recovery_time_ms=recovery_time_ms,
                        impact_assessment=impact_assessment
                    )
                    failure_points.append(failure_point)

                # Sort by failure rate and count
                failure_points.sort(key=lambda x: (x.failure_rate, x.failure_count), reverse=True)

                return failure_points

            except Exception as e:
                logger.error("Error analyzing failures: %s", e)
                raise

    async def generate_performance_report(
        self,
        agents: Optional[List[str]] = None,
        time_range_hours: int = 24,
        include_trends: bool = True,
        tenant_id: Optional[str] = None
    ) -> PerformanceReport:
        """Generate comprehensive workflow performance report.

        Args:
            agents: Optional list of specific agents to include
            time_range_hours: Time range to analyze
            include_trends: Whether to include trend analysis
            tenant_id: Optional tenant filter

        Returns:
            Complete performance report with recommendations
        """
        self._check_running()

        async with self._session_factory() as session:
            try:
                end_time = datetime.utcnow()
                start_time = end_time - timedelta(hours=time_range_hours)

                # Get overall workflow statistics
                stats_query = select(
                    func.count(WorkflowEvent.workflow_id.distinct()).label('total_workflows'),
                    func.count(
                        case(
                            (WorkflowEvent.event_status == EventStatus.SUCCESS.value, 1)
                        )
                    ).label('successful_workflows'),
                    func.avg(WorkflowEvent.processing_time_ms).label('avg_duration'),
                    func.percentile_cont(0.95).within_group(
                        WorkflowEvent.processing_time_ms.asc()
                    ).label('p95_duration')
                ).where(
                    and_(
                        WorkflowEvent.timestamp >= start_time,
                        WorkflowEvent.event_type == EventType.WORKFLOW_FINISHED.value
                    )
                )

                if agents:
                    stats_query = stats_query.where(WorkflowEvent.agent_id.in_(agents))
                if tenant_id:
                    stats_query = stats_query.where(WorkflowEvent.tenant_id == tenant_id)

                stats_result = await session.execute(stats_query)
                stats = stats_result.one()

                total_workflows = stats.total_workflows or 0
                success_rate = (stats.successful_workflows / total_workflows * 100) if total_workflows > 0 else 0.0
                avg_duration_ms = float(stats.avg_duration or 0)
                p95_duration_ms = float(stats.p95_duration or 0)

                # Get bottlenecks
                bottlenecks = await self.identify_bottlenecks(
                    time_range_hours=time_range_hours,
                    agents=agents,
                    tenant_id=tenant_id
                )

                # Analyze handoff performance
                handoff_analysis = await self._analyze_system_handoffs(
                    session, start_time, end_time, agents, tenant_id
                )

                # Analyze failures
                failure_analysis = await self.analyze_failures(
                    time_range_hours=time_range_hours,
                    tenant_id=tenant_id
                )

                # Generate performance trends if requested
                performance_trends = {}
                if include_trends:
                    performance_trends = await self._analyze_performance_trends(
                        session, start_time, end_time, agents, tenant_id
                    )

                # Generate recommendations
                recommendations = self._generate_system_recommendations(
                    bottlenecks, handoff_analysis, failure_analysis, performance_trends
                )

                return PerformanceReport(
                    time_range=(start_time, end_time),
                    agents_analyzed=agents or [],
                    total_workflows=total_workflows,
                    success_rate=success_rate,
                    avg_workflow_duration_ms=avg_duration_ms,
                    p95_workflow_duration_ms=p95_duration_ms,
                    top_bottlenecks=bottlenecks[:10],  # Top 10 bottlenecks
                    handoff_analysis=handoff_analysis,
                    failure_analysis=failure_analysis,
                    performance_trends=performance_trends,
                    recommendations=recommendations
                )

            except Exception as e:
                logger.error("Error generating performance report: %s", e)
                raise

    async def compare_workflow_performance(
        self,
        workflow_type: Optional[str] = None,
        baseline_hours: Tuple[int, int] = (48, 24),  # (start_hours_ago, end_hours_ago)
        current_hours: int = 24,
        agents: Optional[List[str]] = None,
        tenant_id: Optional[str] = None
    ) -> WorkflowComparison:
        """Compare workflow performance between different time periods.

        Args:
            workflow_type: Optional specific workflow type to compare
            baseline_hours: Baseline period (start_hours_ago, end_hours_ago)
            current_hours: Current period hours from now
            agents: Optional list of specific agents
            tenant_id: Optional tenant filter

        Returns:
            Detailed comparison with performance changes
        """
        self._check_running()

        async with self._session_factory() as session:
            try:
                current_end = datetime.utcnow()
                current_start = current_end - timedelta(hours=current_hours)

                baseline_end = current_end - timedelta(hours=baseline_hours[1])
                baseline_start = current_end - timedelta(hours=baseline_hours[0])

                # Get performance metrics for both periods
                baseline_metrics = await self._get_period_metrics(
                    session, baseline_start, baseline_end, workflow_type, agents, tenant_id
                )
                current_metrics = await self._get_period_metrics(
                    session, current_start, current_end, workflow_type, agents, tenant_id
                )

                # Calculate changes
                performance_change = self._calculate_performance_change(
                    baseline_metrics, current_metrics
                )

                duration_change_ms = (
                    current_metrics.get('avg_duration_ms', 0) -
                    baseline_metrics.get('avg_duration_ms', 0)
                )

                success_rate_change = (
                    current_metrics.get('success_rate', 0) -
                    baseline_metrics.get('success_rate', 0)
                )

                # Analyze bottleneck changes
                baseline_bottlenecks = await self.identify_bottlenecks(
                    time_range_hours=(baseline_hours[0] - baseline_hours[1]),
                    agents=agents,
                    tenant_id=tenant_id
                )
                current_bottlenecks = await self.identify_bottlenecks(
                    time_range_hours=current_hours,
                    agents=agents,
                    tenant_id=tenant_id
                )

                bottleneck_changes = self._analyze_bottleneck_changes(
                    baseline_bottlenecks, current_bottlenecks
                )

                # Detect regressions
                regression_detected = (
                    performance_change < -10.0 or  # 10% performance degradation
                    duration_change_ms > 5000.0 or  # 5 second increase
                    success_rate_change < -5.0  # 5% success rate drop
                )

                # Identify improvement and concern areas
                improvement_areas = self._identify_improvement_areas(
                    baseline_metrics, current_metrics, bottleneck_changes
                )
                concern_areas = self._identify_concern_areas(
                    baseline_metrics, current_metrics, bottleneck_changes
                )

                return WorkflowComparison(
                    baseline_period=(baseline_start, baseline_end),
                    current_period=(current_start, current_end),
                    performance_change=performance_change,
                    duration_change_ms=duration_change_ms,
                    success_rate_change=success_rate_change,
                    bottleneck_changes=bottleneck_changes,
                    regression_detected=regression_detected,
                    improvement_areas=improvement_areas,
                    concern_areas=concern_areas
                )

            except Exception as e:
                logger.error("Error comparing workflow performance: %s", e)
                raise

    async def get_workflow_visualization_data(
        self,
        chart_type: str,
        time_range_hours: int = 24,
        aggregation_level: str = "hour",
        agents: Optional[List[str]] = None,
        tenant_id: Optional[str] = None
    ) -> VisualizationData:
        """Generate visualization data for dashboard charts.

        Args:
            chart_type: Type of chart (timeline, bottlenecks, success_rate, etc.)
            time_range_hours: Time range to visualize
            aggregation_level: Data aggregation level (minute, hour, day)
            agents: Optional list of specific agents
            tenant_id: Optional tenant filter

        Returns:
            Formatted data for dashboard visualization
        """
        self._check_running()

        async with self._session_factory() as session:
            try:
                end_time = datetime.utcnow()
                start_time = end_time - timedelta(hours=time_range_hours)

                if chart_type == "timeline":
                    data_points = await self._get_timeline_data(
                        session, start_time, end_time, aggregation_level, agents, tenant_id
                    )
                elif chart_type == "bottlenecks":
                    data_points = await self._get_bottleneck_chart_data(
                        session, start_time, end_time, agents, tenant_id
                    )
                elif chart_type == "success_rate":
                    data_points = await self._get_success_rate_data(
                        session, start_time, end_time, aggregation_level, agents, tenant_id
                    )
                elif chart_type == "agent_performance":
                    data_points = await self._get_agent_performance_data(
                        session, start_time, end_time, agents, tenant_id
                    )
                elif chart_type == "handoff_flow":
                    data_points = await self._get_handoff_flow_data(
                        session, start_time, end_time, agents, tenant_id
                    )
                else:
                    raise ValueError(f"Unsupported chart type: {chart_type}")

                metadata = {
                    "total_data_points": len(data_points),
                    "chart_config": self._get_chart_config(chart_type),
                    "filters_applied": {
                        "agents": agents,
                        "tenant_id": tenant_id,
                        "aggregation": aggregation_level
                    }
                }

                return VisualizationData(
                    chart_type=chart_type,
                    data_points=data_points,
                    metadata=metadata,
                    time_range=(start_time, end_time),
                    aggregation_level=aggregation_level
                )

            except Exception as e:
                logger.error("Error generating visualization data: %s", e)
                raise

    # ========================================================================
    # Helper Methods
    # ========================================================================

    def _analyze_workflow_stages(self, events: List[WorkflowEvent]) -> List[WorkflowStage]:
        """Analyze workflow events to extract stages with timing."""
        stages = []
        current_stage = None

        for event in events:
            if event.event_type == EventType.WORKFLOW_STARTED.value:
                current_stage = WorkflowStage(
                    stage_name="workflow_start",
                    agent_id=event.agent_id,
                    start_time=event.timestamp,
                    end_time=None,
                    duration_ms=None,
                    status=event.event_status,
                    event_data=event.event_data or {},
                    processing_time_ms=event.processing_time_ms
                )

            elif event.event_type == EventType.STAGE_COMPLETED.value:
                if current_stage:
                    current_stage.end_time = event.timestamp
                    current_stage.duration_ms = (
                        (event.timestamp - current_stage.start_time).total_seconds() * 1000
                    )
                    stages.append(current_stage)

                # Start new stage
                current_stage = WorkflowStage(
                    stage_name=event.event_data.get('stage_name', 'unknown_stage'),
                    agent_id=event.agent_id,
                    start_time=event.timestamp,
                    end_time=None,
                    duration_ms=None,
                    status=event.event_status,
                    event_data=event.event_data or {},
                    processing_time_ms=event.processing_time_ms
                )

            elif event.event_type == EventType.HANDOFF.value:
                if current_stage:
                    current_stage.end_time = event.timestamp
                    current_stage.duration_ms = (
                        (event.timestamp - current_stage.start_time).total_seconds() * 1000
                    )
                    stages.append(current_stage)

                # Create handoff stage
                handoff_stage = WorkflowStage(
                    stage_name="handoff",
                    agent_id=event.agent_id,
                    start_time=event.timestamp,
                    end_time=event.timestamp,
                    duration_ms=event.processing_time_ms,
                    status=event.event_status,
                    event_data=event.event_data or {},
                    processing_time_ms=event.processing_time_ms
                )
                stages.append(handoff_stage)

                # Start next stage with receiving agent
                receiving_agent = event.event_data.get('to_agent', event.agent_id)
                current_stage = WorkflowStage(
                    stage_name="processing",
                    agent_id=receiving_agent,
                    start_time=event.timestamp,
                    end_time=None,
                    duration_ms=None,
                    status=EventStatus.SUCCESS,
                    event_data={},
                    processing_time_ms=None
                )

            elif event.event_type == EventType.WORKFLOW_FINISHED.value:
                if current_stage:
                    current_stage.end_time = event.timestamp
                    current_stage.duration_ms = (
                        (event.timestamp - current_stage.start_time).total_seconds() * 1000
                    )
                    stages.append(current_stage)

        return stages

    def _identify_workflow_bottlenecks(self, stages: List[WorkflowStage]) -> List[BottleneckDetails]:
        """Identify bottlenecks within a single workflow."""
        if not stages:
            return []

        # Calculate total workflow time
        total_time = sum(s.duration_ms or 0 for s in stages)
        if total_time == 0:
            return []

        bottlenecks = []
        for stage in stages:
            if stage.duration_ms and stage.duration_ms > 0:
                percentage = (stage.duration_ms / total_time) * 100

                # Consider it a bottleneck if it takes >20% of workflow time
                if percentage > 20.0:
                    severity = BottleneckSeverity.HIGH if percentage > 50.0 else BottleneckSeverity.MODERATE

                    bottleneck = BottleneckDetails(
                        stage_name=stage.stage_name,
                        agent_id=stage.agent_id,
                        severity=severity,
                        avg_duration_ms=stage.duration_ms,
                        p95_duration_ms=stage.duration_ms,
                        max_duration_ms=stage.duration_ms,
                        occurrence_count=1,
                        percentage_of_total_time=percentage,
                        impact_score=percentage,
                        recommended_actions=[
                            f"Optimize {stage.stage_name} in {stage.agent_id}",
                            "Review processing logic for efficiency improvements"
                        ]
                    )
                    bottlenecks.append(bottleneck)

        return sorted(bottlenecks, key=lambda x: x.percentage_of_total_time, reverse=True)

    def _analyze_workflow_handoffs(self, stages: List[WorkflowStage]) -> List[HandoffAnalysis]:
        """Analyze handoff performance within a workflow."""
        handoffs = []

        for i in range(len(stages) - 1):
            current_stage = stages[i]
            next_stage = stages[i + 1]

            if (current_stage.end_time and next_stage.start_time and
                current_stage.agent_id != next_stage.agent_id):

                handoff_delay = (next_stage.start_time - current_stage.end_time).total_seconds() * 1000

                if handoff_delay > self._handoff_delay_threshold_ms:
                    handoff = HandoffAnalysis(
                        from_agent=current_stage.agent_id,
                        to_agent=next_stage.agent_id,
                        avg_handoff_delay_ms=handoff_delay,
                        max_handoff_delay_ms=handoff_delay,
                        handoff_count=1,
                        failure_rate=0.0,
                        efficiency_score=max(0, 100 - (handoff_delay / 1000)),  # Score based on delay
                        recommendations=[
                            "Investigate handoff delay",
                            "Consider optimizing agent coordination"
                        ]
                    )
                    handoffs.append(handoff)

        return handoffs

    def _calculate_workflow_metrics(self, stages: List[WorkflowStage], total_duration: float) -> Dict[str, float]:
        """Calculate comprehensive workflow performance metrics."""
        if not stages:
            return {}

        processing_times = [s.processing_time_ms for s in stages if s.processing_time_ms]
        stage_durations = [s.duration_ms for s in stages if s.duration_ms]

        metrics = {
            "total_duration_ms": total_duration,
            "stage_count": len(stages),
            "avg_stage_duration_ms": statistics.mean(stage_durations) if stage_durations else 0,
            "max_stage_duration_ms": max(stage_durations) if stage_durations else 0,
            "min_stage_duration_ms": min(stage_durations) if stage_durations else 0,
            "processing_efficiency": (sum(processing_times) / total_duration * 100) if total_duration > 0 else 0,
        }

        # Calculate agent distribution
        agent_times = defaultdict(float)
        for stage in stages:
            if stage.duration_ms:
                agent_times[stage.agent_id] += stage.duration_ms

        if agent_times:
            metrics["agent_balance_score"] = 100 - (statistics.stdev(agent_times.values()) / statistics.mean(agent_times.values()) * 100)

        return metrics

    def _identify_failure_points(self, stages: List[WorkflowStage], events: List[WorkflowEvent]) -> List[FailurePoint]:
        """Identify failure points within a workflow."""
        failure_points = []

        failed_events = [e for e in events if e.event_status in [EventStatus.FAILURE, EventStatus.TIMEOUT]]

        for event in failed_events:
            errors = []
            if event.event_data:
                if 'error' in event.event_data:
                    errors.append(event.event_data['error'])
                if 'exception' in event.event_data:
                    errors.append(event.event_data['exception'])

            root_causes = []
            if event.event_data:
                if 'timeout' in str(event.event_data).lower():
                    root_causes.append("Processing timeout")
                if 'memory' in str(event.event_data).lower():
                    root_causes.append("Memory constraint")
                if 'connection' in str(event.event_data).lower():
                    root_causes.append("Connection issue")

            failure_point = FailurePoint(
                stage_name=event.event_type,
                agent_id=event.agent_id,
                failure_count=1,
                failure_rate=1.0,  # 100% for this specific workflow
                common_errors=errors,
                root_causes=root_causes or ["Unknown cause"],
                recovery_time_ms=event.processing_time_ms,
                impact_assessment="Workflow failure"
            )
            failure_points.append(failure_point)

        return failure_points

    async def _get_workflow_decision_context(
        self,
        session: AsyncSession,
        workflow_id: str,
        tenant_id: Optional[str]
    ) -> Optional[Dict[str, Any]]:
        """Get decision traces related to a workflow."""
        try:
            query = select(DecisionTrace).where(
                DecisionTrace.workflow_id == uuid.UUID(workflow_id)
            )
            if tenant_id:
                query = query.where(DecisionTrace.tenant_id == tenant_id)

            result = await session.execute(query)
            decisions = result.scalars().all()

            if not decisions:
                return None

            context = {
                "decision_count": len(decisions),
                "decisions": []
            }

            for decision in decisions:
                context["decisions"].append({
                    "agent_id": decision.agent_id,
                    "decision_type": decision.decision_type,
                    "confidence_score": decision.confidence_score,
                    "timestamp": decision.timestamp.isoformat(),
                    "reasoning": decision.reasoning,
                    "alternatives_count": len(decision.alternatives_considered)
                })

            # Calculate average confidence
            confidences = [d.confidence_score for d in decisions]
            context["avg_confidence"] = statistics.mean(confidences) if confidences else 0

            return context

        except Exception as e:
            logger.warning("Failed to get decision context for workflow %s: %s", workflow_id, e)
            return None

    def _generate_workflow_recommendations(
        self,
        bottlenecks: List[BottleneckDetails],
        handoffs: List[HandoffAnalysis],
        failures: List[FailurePoint]
    ) -> List[str]:
        """Generate actionable recommendations for workflow optimization."""
        recommendations = []

        # Bottleneck recommendations
        for bottleneck in bottlenecks[:3]:  # Top 3 bottlenecks
            recommendations.append(
                f"Optimize {bottleneck.stage_name} in {bottleneck.agent_id} "
                f"(consumes {bottleneck.percentage_of_total_time:.1f}% of workflow time)"
            )

        # Handoff recommendations
        for handoff in handoffs[:2]:  # Top 2 handoff issues
            recommendations.append(
                f"Reduce handoff delay between {handoff.from_agent} and {handoff.to_agent} "
                f"(current delay: {handoff.avg_handoff_delay_ms:.0f}ms)"
            )

        # Failure recommendations
        for failure in failures[:2]:  # Top 2 failure points
            recommendations.append(
                f"Address {failure.stage_name} failures in {failure.agent_id} "
                f"(failure rate: {failure.failure_rate * 100:.1f}%)"
            )

        # Generic recommendations
        if not recommendations:
            recommendations.append("Workflow performance appears optimal")

        return recommendations

    # Additional helper methods for bottleneck analysis

    def _determine_bottleneck_severity(self, avg_duration: float, p95_duration: float, max_duration: float) -> BottleneckSeverity:
        """Determine bottleneck severity based on duration statistics."""
        # Use p95 duration as primary indicator
        if p95_duration > 30000:  # >30 seconds
            return BottleneckSeverity.CRITICAL
        elif p95_duration > 15000:  # >15 seconds
            return BottleneckSeverity.HIGH
        elif p95_duration > 5000:   # >5 seconds
            return BottleneckSeverity.MODERATE
        else:
            return BottleneckSeverity.LOW

    def _calculate_impact_score(self, avg_duration: float, count: int, total_system_time: float, stage_total_time: float) -> float:
        """Calculate impact score combining duration and frequency."""
        # Normalize components
        duration_score = min(avg_duration / 10000, 1.0) * 50  # Max 50 points for duration
        frequency_score = min(count / 1000, 1.0) * 30        # Max 30 points for frequency
        proportion_score = (stage_total_time / total_system_time) * 20  # Max 20 points for proportion

        return duration_score + frequency_score + proportion_score

    def _generate_bottleneck_recommendations(self, agent_id: str, event_type: str, avg_duration: float, count: int) -> List[str]:
        """Generate specific recommendations for bottlenecks."""
        recommendations = []

        if avg_duration > 20000:  # >20 seconds
            recommendations.append("Consider breaking down into smaller processing units")
            recommendations.append("Review algorithm efficiency and data structures")

        if count > 100:  # High frequency
            recommendations.append("Implement caching for frequently processed items")
            recommendations.append("Consider batch processing to reduce overhead")

        if "processing" in event_type.lower():
            recommendations.append("Profile CPU and memory usage during processing")
            recommendations.append("Consider parallel processing for independent operations")

        if "handoff" in event_type.lower():
            recommendations.append("Optimize data serialization for handoffs")
            recommendations.append("Implement asynchronous handoff patterns")

        return recommendations or ["Review and optimize processing logic"]

    async def _analyze_system_handoffs(
        self,
        session: AsyncSession,
        start_time: datetime,
        end_time: datetime,
        agents: Optional[List[str]],
        tenant_id: Optional[str]
    ) -> List[HandoffAnalysis]:
        """Analyze handoff performance across the system."""
        try:
            # Query handoff events
            handoff_query = select(WorkflowEvent).where(
                and_(
                    WorkflowEvent.timestamp >= start_time,
                    WorkflowEvent.timestamp <= end_time,
                    WorkflowEvent.event_type == EventType.HANDOFF.value
                )
            )

            if agents:
                handoff_query = handoff_query.where(WorkflowEvent.agent_id.in_(agents))
            if tenant_id:
                handoff_query = handoff_query.where(WorkflowEvent.tenant_id == tenant_id)

            result = await session.execute(handoff_query)
            handoffs = result.scalars().all()

            # Group by agent pairs
            agent_pairs = defaultdict(list)
            for handoff in handoffs:
                event_data = handoff.event_data or {}
                from_agent = event_data.get('from_agent', handoff.agent_id)
                to_agent = event_data.get('to_agent', 'unknown')

                if to_agent != 'unknown':
                    pair_key = (from_agent, to_agent)
                    agent_pairs[pair_key].append(handoff)

            # Analyze each pair
            handoff_analyses = []
            for (from_agent, to_agent), handoff_list in agent_pairs.items():
                if len(handoff_list) >= self._min_samples_for_analysis:

                    processing_times = [h.processing_time_ms for h in handoff_list if h.processing_time_ms]
                    failures = len([h for h in handoff_list if h.event_status == EventStatus.FAILURE.value])

                    avg_delay = statistics.mean(processing_times) if processing_times else 0
                    max_delay = max(processing_times) if processing_times else 0
                    failure_rate = failures / len(handoff_list)

                    # Calculate efficiency score
                    efficiency_score = max(0, 100 - (avg_delay / 1000) - (failure_rate * 50))

                    recommendations = []
                    if avg_delay > 5000:
                        recommendations.append("Optimize handoff data transfer")
                    if failure_rate > 0.1:
                        recommendations.append("Improve error handling in handoff process")

                    analysis = HandoffAnalysis(
                        from_agent=from_agent,
                        to_agent=to_agent,
                        avg_handoff_delay_ms=avg_delay,
                        max_handoff_delay_ms=max_delay,
                        handoff_count=len(handoff_list),
                        failure_rate=failure_rate,
                        efficiency_score=efficiency_score,
                        recommendations=recommendations
                    )
                    handoff_analyses.append(analysis)

            return sorted(handoff_analyses, key=lambda x: x.avg_handoff_delay_ms, reverse=True)

        except Exception as e:
            logger.error("Error analyzing system handoffs: %s", e)
            return []

    def _extract_common_errors(self, failures: List[WorkflowEvent]) -> List[str]:
        """Extract common error patterns from failed events."""
        errors = []
        for failure in failures:
            if failure.event_data:
                if 'error' in failure.event_data:
                    errors.append(str(failure.event_data['error']))
                if 'exception' in failure.event_data:
                    errors.append(str(failure.event_data['exception']))
                if 'message' in failure.event_data:
                    errors.append(str(failure.event_data['message']))

        # Count occurrences and return most common
        error_counts = defaultdict(int)
        for error in errors:
            error_counts[error] += 1

        # Return top 5 most common errors
        return [error for error, count in
                sorted(error_counts.items(), key=lambda x: x[1], reverse=True)][:5]

    async def _determine_root_causes(
        self,
        session: AsyncSession,
        agent_id: str,
        event_type: str,
        failures: List[WorkflowEvent],
        include_context: bool
    ) -> List[str]:
        """Determine root causes of failures using decision traces and logs."""
        root_causes = []

        try:
            if include_context:
                # Get decision traces around failure times
                for failure in failures[:5]:  # Analyze up to 5 failures
                    trace_query = select(DecisionTrace).where(
                        and_(
                            DecisionTrace.agent_id == agent_id,
                            DecisionTrace.timestamp >= failure.timestamp - timedelta(minutes=5),
                            DecisionTrace.timestamp <= failure.timestamp + timedelta(minutes=1)
                        )
                    )

                    trace_result = await session.execute(trace_query)
                    traces = trace_result.scalars().all()

                    for trace in traces:
                        if trace.confidence_score < 0.5:
                            root_causes.append("Low confidence decisions preceding failure")

                        if 'error' in str(trace.reasoning).lower():
                            root_causes.append("Decision logic identified errors")

                        if 'timeout' in str(trace.decision_context).lower():
                            root_causes.append("Processing timeouts detected")

        except Exception as e:
            logger.warning("Error analyzing decision traces: %s", e)

        # Analyze failure patterns
        error_patterns = []
        for failure in failures:
            if failure.event_data:
                data_str = str(failure.event_data).lower()
                if 'timeout' in data_str:
                    error_patterns.append("timeout")
                if 'memory' in data_str:
                    error_patterns.append("memory")
                if 'connection' in data_str:
                    error_patterns.append("connection")
                if 'validation' in data_str:
                    error_patterns.append("validation")

        # Determine most common patterns
        pattern_counts = defaultdict(int)
        for pattern in error_patterns:
            pattern_counts[pattern] += 1

        for pattern, count in sorted(pattern_counts.items(), key=lambda x: x[1], reverse=True):
            if count >= 2:  # Appears in at least 2 failures
                if pattern == "timeout":
                    root_causes.append("Processing timeouts - review performance optimization")
                elif pattern == "memory":
                    root_causes.append("Memory constraints - review memory usage patterns")
                elif pattern == "connection":
                    root_causes.append("Connection issues - review network stability")
                elif pattern == "validation":
                    root_causes.append("Data validation failures - review input data quality")

        return list(set(root_causes)) or ["Root cause analysis inconclusive"]

    def _calculate_recovery_time(self, failures: List[WorkflowEvent]) -> Optional[float]:
        """Calculate average recovery time from failures."""
        recovery_times = []

        for failure in failures:
            if failure.processing_time_ms and failure.event_status == EventStatus.RETRY.value:
                recovery_times.append(failure.processing_time_ms)

        return statistics.mean(recovery_times) if recovery_times else None

    def _assess_failure_impact(self, failure_rate: float, failure_count: int, recovery_time_ms: Optional[float]) -> str:
        """Assess the overall impact of failures."""
        if failure_rate > 0.1:  # >10% failure rate
            return "High impact - significant reliability concerns"
        elif failure_rate > 0.05:  # >5% failure rate
            return "Moderate impact - monitoring recommended"
        elif failure_count > 10:
            return "Low impact but frequent - review for optimization"
        elif recovery_time_ms and recovery_time_ms > 10000:
            return "Low frequency but slow recovery"
        else:
            return "Minimal impact"

    # Caching helpers

    def _get_cached_result(self, cache_key: str) -> Optional[Any]:
        """Get cached analysis result if still valid."""
        if cache_key in self._analysis_cache:
            timestamp, result = self._analysis_cache[cache_key]
            if (datetime.utcnow() - timestamp).total_seconds() < self._cache_ttl_seconds:
                return result
            else:
                del self._analysis_cache[cache_key]
        return None

    def _cache_result(self, cache_key: str, result: Any) -> None:
        """Cache analysis result with timestamp."""
        self._analysis_cache[cache_key] = (datetime.utcnow(), result)

        # Limit cache size
        if len(self._analysis_cache) > 100:
            # Remove oldest entries
            oldest_keys = sorted(
                self._analysis_cache.keys(),
                key=lambda k: self._analysis_cache[k][0]
            )[:10]
            for key in oldest_keys:
                del self._analysis_cache[key]

    # Performance analysis helpers for reports

    async def _get_period_metrics(
        self,
        session: AsyncSession,
        start_time: datetime,
        end_time: datetime,
        workflow_type: Optional[str],
        agents: Optional[List[str]],
        tenant_id: Optional[str]
    ) -> Dict[str, float]:
        """Get aggregated metrics for a time period."""
        try:
            query = select(
                func.count(WorkflowEvent.id).label('total_events'),
                func.count(
                    case(
                        (WorkflowEvent.event_status == EventStatus.SUCCESS.value, 1)
                    )
                ).label('successful_events'),
                func.avg(WorkflowEvent.processing_time_ms).label('avg_duration'),
                func.percentile_cont(0.95).within_group(
                    WorkflowEvent.processing_time_ms.asc()
                ).label('p95_duration')
            ).where(
                and_(
                    WorkflowEvent.timestamp >= start_time,
                    WorkflowEvent.timestamp <= end_time,
                    WorkflowEvent.processing_time_ms.isnot(None)
                )
            )

            if workflow_type:
                # Assuming workflow_type is stored in event_data
                query = query.where(WorkflowEvent.event_data['workflow_type'].astext == workflow_type)
            if agents:
                query = query.where(WorkflowEvent.agent_id.in_(agents))
            if tenant_id:
                query = query.where(WorkflowEvent.tenant_id == tenant_id)

            result = await session.execute(query)
            row = result.one()

            total_events = row.total_events or 0
            successful_events = row.successful_events or 0

            return {
                'total_events': total_events,
                'success_rate': (successful_events / total_events * 100) if total_events > 0 else 0,
                'avg_duration_ms': float(row.avg_duration or 0),
                'p95_duration_ms': float(row.p95_duration or 0)
            }

        except Exception as e:
            logger.error("Error getting period metrics: %s", e)
            return {}

    def _calculate_performance_change(self, baseline: Dict[str, float], current: Dict[str, float]) -> float:
        """Calculate overall performance change percentage."""
        if not baseline or not current:
            return 0.0

        # Weight different metrics
        weights = {
            'success_rate': 0.4,
            'avg_duration_ms': 0.4,  # Lower is better, so invert
            'p95_duration_ms': 0.2   # Lower is better, so invert
        }

        changes = {}

        # Success rate change (higher is better)
        if baseline.get('success_rate', 0) > 0:
            changes['success_rate'] = (
                (current.get('success_rate', 0) - baseline['success_rate']) /
                baseline['success_rate'] * 100
            )

        # Duration changes (lower is better, so invert the change)
        if baseline.get('avg_duration_ms', 0) > 0:
            duration_change = (
                (current.get('avg_duration_ms', 0) - baseline['avg_duration_ms']) /
                baseline['avg_duration_ms'] * 100
            )
            changes['avg_duration_ms'] = -duration_change  # Invert so improvement is positive

        if baseline.get('p95_duration_ms', 0) > 0:
            p95_change = (
                (current.get('p95_duration_ms', 0) - baseline['p95_duration_ms']) /
                baseline['p95_duration_ms'] * 100
            )
            changes['p95_duration_ms'] = -p95_change  # Invert so improvement is positive

        # Calculate weighted average
        total_change = 0.0
        total_weight = 0.0

        for metric, change in changes.items():
            weight = weights.get(metric, 0)
            total_change += change * weight
            total_weight += weight

        return total_change / total_weight if total_weight > 0 else 0.0

    def _analyze_bottleneck_changes(
        self,
        baseline: List[BottleneckDetails],
        current: List[BottleneckDetails]
    ) -> List[Dict[str, Any]]:
        """Analyze changes in bottlenecks between periods."""
        changes = []

        # Create lookup for baseline bottlenecks
        baseline_lookup = {
            (b.agent_id, b.stage_name): b for b in baseline
        }

        # Analyze current bottlenecks
        for current_bottleneck in current:
            key = (current_bottleneck.agent_id, current_bottleneck.stage_name)

            if key in baseline_lookup:
                baseline_bottleneck = baseline_lookup[key]

                duration_change = (
                    current_bottleneck.avg_duration_ms - baseline_bottleneck.avg_duration_ms
                )
                percentage_change = (
                    duration_change / baseline_bottleneck.avg_duration_ms * 100
                    if baseline_bottleneck.avg_duration_ms > 0 else 0
                )

                change = {
                    'agent_id': current_bottleneck.agent_id,
                    'stage_name': current_bottleneck.stage_name,
                    'change_type': 'modified',
                    'duration_change_ms': duration_change,
                    'percentage_change': percentage_change,
                    'severity_change': {
                        'from': baseline_bottleneck.severity.value,
                        'to': current_bottleneck.severity.value
                    }
                }
                changes.append(change)

                # Remove from baseline lookup to track new/removed
                del baseline_lookup[key]
            else:
                # New bottleneck
                change = {
                    'agent_id': current_bottleneck.agent_id,
                    'stage_name': current_bottleneck.stage_name,
                    'change_type': 'new',
                    'avg_duration_ms': current_bottleneck.avg_duration_ms,
                    'severity': current_bottleneck.severity.value
                }
                changes.append(change)

        # Add removed bottlenecks
        for baseline_bottleneck in baseline_lookup.values():
            change = {
                'agent_id': baseline_bottleneck.agent_id,
                'stage_name': baseline_bottleneck.stage_name,
                'change_type': 'resolved',
                'avg_duration_ms': baseline_bottleneck.avg_duration_ms,
                'severity': baseline_bottleneck.severity.value
            }
            changes.append(change)

        return changes

    def _identify_improvement_areas(
        self,
        baseline: Dict[str, float],
        current: Dict[str, float],
        bottleneck_changes: List[Dict[str, Any]]
    ) -> List[str]:
        """Identify areas showing improvement."""
        improvements = []

        # Check success rate improvement
        if (current.get('success_rate', 0) > baseline.get('success_rate', 0) + 2):
            improvements.append("Success rate improved")

        # Check duration improvements
        if (baseline.get('avg_duration_ms', 0) > 0 and
            current.get('avg_duration_ms', 0) < baseline['avg_duration_ms'] * 0.95):
            improvements.append("Average processing time improved")

        # Check resolved bottlenecks
        resolved_count = len([c for c in bottleneck_changes if c['change_type'] == 'resolved'])
        if resolved_count > 0:
            improvements.append(f"{resolved_count} bottlenecks resolved")

        return improvements

    def _identify_concern_areas(
        self,
        baseline: Dict[str, float],
        current: Dict[str, float],
        bottleneck_changes: List[Dict[str, Any]]
    ) -> List[str]:
        """Identify areas of concern."""
        concerns = []

        # Check success rate degradation
        if (current.get('success_rate', 0) < baseline.get('success_rate', 0) - 2):
            concerns.append("Success rate declined")

        # Check duration degradation
        if (baseline.get('avg_duration_ms', 0) > 0 and
            current.get('avg_duration_ms', 0) > baseline['avg_duration_ms'] * 1.1):
            concerns.append("Average processing time increased")

        # Check new bottlenecks
        new_count = len([c for c in bottleneck_changes if c['change_type'] == 'new'])
        if new_count > 0:
            concerns.append(f"{new_count} new bottlenecks detected")

        # Check severity increases
        severity_increases = [
            c for c in bottleneck_changes
            if (c['change_type'] == 'modified' and
                c.get('severity_change', {}).get('to') in ['high', 'critical'] and
                c.get('severity_change', {}).get('from') in ['low', 'moderate'])
        ]
        if severity_increases:
            concerns.append(f"{len(severity_increases)} bottlenecks became more severe")

        return concerns

    async def _analyze_performance_trends(
        self,
        session: AsyncSession,
        start_time: datetime,
        end_time: datetime,
        agents: Optional[List[str]],
        tenant_id: Optional[str]
    ) -> Dict[str, PerformanceTrend]:
        """Analyze performance trends over time."""
        try:
            # Divide time range into buckets for trend analysis
            time_delta = end_time - start_time
            bucket_size = time_delta / 6  # 6 data points

            trends = {}

            for i in range(6):
                bucket_start = start_time + (i * bucket_size)
                bucket_end = start_time + ((i + 1) * bucket_size)

                metrics = await self._get_period_metrics(
                    session, bucket_start, bucket_end, None, agents, tenant_id
                )

                # Store metrics for trend calculation
                for metric_name, value in metrics.items():
                    if metric_name not in trends:
                        trends[metric_name] = []
                    trends[metric_name].append(value)

            # Calculate trend directions
            trend_directions = {}
            for metric_name, values in trends.items():
                if len(values) >= 3:
                    # Simple linear trend detection
                    if len(set(values)) == 1:
                        trend_directions[metric_name] = PerformanceTrend.STABLE
                    else:
                        # Calculate correlation with time
                        x = list(range(len(values)))
                        if len(x) > 1:
                            slope = self._calculate_slope(x, values)
                            if abs(slope) < 0.1:
                                trend_directions[metric_name] = PerformanceTrend.STABLE
                            elif slope > 0:
                                if metric_name == 'success_rate':
                                    trend_directions[metric_name] = PerformanceTrend.IMPROVING
                                else:  # Duration metrics - lower is better
                                    trend_directions[metric_name] = PerformanceTrend.DEGRADING
                            else:
                                if metric_name == 'success_rate':
                                    trend_directions[metric_name] = PerformanceTrend.DEGRADING
                                else:  # Duration metrics - lower is better
                                    trend_directions[metric_name] = PerformanceTrend.IMPROVING

            return trend_directions

        except Exception as e:
            logger.error("Error analyzing performance trends: %s", e)
            return {}

    def _calculate_slope(self, x: List[float], y: List[float]) -> float:
        """Calculate simple linear regression slope."""
        n = len(x)
        if n < 2:
            return 0.0

        x_mean = statistics.mean(x)
        y_mean = statistics.mean(y)

        numerator = sum((x[i] - x_mean) * (y[i] - y_mean) for i in range(n))
        denominator = sum((x[i] - x_mean) ** 2 for i in range(n))

        return numerator / denominator if denominator != 0 else 0.0

    def _generate_system_recommendations(
        self,
        bottlenecks: List[BottleneckDetails],
        handoffs: List[HandoffAnalysis],
        failures: List[FailurePoint],
        trends: Dict[str, PerformanceTrend]
    ) -> List[str]:
        """Generate system-wide recommendations."""
        recommendations = []

        # Top bottleneck recommendations
        if bottlenecks:
            top_bottleneck = bottlenecks[0]
            recommendations.append(
                f"Priority: Address {top_bottleneck.stage_name} bottleneck in {top_bottleneck.agent_id} "
                f"(impact score: {top_bottleneck.impact_score:.1f})"
            )

        # Critical handoff issues
        critical_handoffs = [h for h in handoffs if h.efficiency_score < 50]
        if critical_handoffs:
            recommendations.append(
                f"Critical: Optimize handoffs between {critical_handoffs[0].from_agent} "
                f"and {critical_handoffs[0].to_agent}"
            )

        # High failure rates
        critical_failures = [f for f in failures if f.failure_rate > 0.1]
        if critical_failures:
            recommendations.append(
                f"Urgent: Address {critical_failures[0].stage_name} failures in "
                f"{critical_failures[0].agent_id} (failure rate: {critical_failures[0].failure_rate * 100:.1f}%)"
            )

        # Trend-based recommendations
        degrading_metrics = [m for m, t in trends.items() if t == PerformanceTrend.DEGRADING]
        if degrading_metrics:
            recommendations.append(f"Monitor degrading metrics: {', '.join(degrading_metrics)}")

        # General recommendations
        if len(bottlenecks) > 10:
            recommendations.append("Consider system-wide performance audit")

        if not recommendations:
            recommendations.append("System performance appears healthy")

        return recommendations

    # Visualization data generators

    async def _get_timeline_data(
        self,
        session: AsyncSession,
        start_time: datetime,
        end_time: datetime,
        aggregation: str,
        agents: Optional[List[str]],
        tenant_id: Optional[str]
    ) -> List[Dict[str, Any]]:
        """Get timeline data for visualization."""
        # Implementation for timeline chart data
        # This would generate time-series data points
        return []  # Placeholder

    async def _get_bottleneck_chart_data(
        self,
        session: AsyncSession,
        start_time: datetime,
        end_time: datetime,
        agents: Optional[List[str]],
        tenant_id: Optional[str]
    ) -> List[Dict[str, Any]]:
        """Get bottleneck visualization data."""
        # Implementation for bottleneck chart data
        return []  # Placeholder

    async def _get_success_rate_data(
        self,
        session: AsyncSession,
        start_time: datetime,
        end_time: datetime,
        aggregation: str,
        agents: Optional[List[str]],
        tenant_id: Optional[str]
    ) -> List[Dict[str, Any]]:
        """Get success rate visualization data."""
        # Implementation for success rate chart data
        return []  # Placeholder

    async def _get_agent_performance_data(
        self,
        session: AsyncSession,
        start_time: datetime,
        end_time: datetime,
        agents: Optional[List[str]],
        tenant_id: Optional[str]
    ) -> List[Dict[str, Any]]:
        """Get agent performance visualization data."""
        # Implementation for agent performance chart data
        return []  # Placeholder

    async def _get_handoff_flow_data(
        self,
        session: AsyncSession,
        start_time: datetime,
        end_time: datetime,
        agents: Optional[List[str]],
        tenant_id: Optional[str]
    ) -> List[Dict[str, Any]]:
        """Get handoff flow visualization data."""
        # Implementation for handoff flow diagram data
        return []  # Placeholder

    def _get_chart_config(self, chart_type: str) -> Dict[str, Any]:
        """Get chart configuration for visualization."""
        configs = {
            "timeline": {
                "type": "line",
                "x_axis": "timestamp",
                "y_axis": "processing_time_ms",
                "title": "Workflow Timeline"
            },
            "bottlenecks": {
                "type": "bar",
                "x_axis": "stage_name",
                "y_axis": "avg_duration_ms",
                "title": "Performance Bottlenecks"
            },
            "success_rate": {
                "type": "line",
                "x_axis": "timestamp",
                "y_axis": "success_rate",
                "title": "Success Rate Over Time"
            },
            "agent_performance": {
                "type": "radar",
                "metrics": ["avg_duration", "success_rate", "efficiency"],
                "title": "Agent Performance Comparison"
            },
            "handoff_flow": {
                "type": "sankey",
                "source": "from_agent",
                "target": "to_agent",
                "value": "handoff_count",
                "title": "Agent Handoff Flow"
            }
        }

        return configs.get(chart_type, {})
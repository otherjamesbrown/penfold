"""Advanced performance analytics service for comprehensive agent performance monitoring.

This service provides detailed performance metrics collection, trend analysis, and
benchmarking capabilities for the Penfold observability framework. It integrates
with existing AgentMetric, WorkflowEvent, and DecisionTrace models to provide
actionable performance insights.

Features:
- Comprehensive performance metrics collection and analysis
- Historical trend analysis with configurable time windows
- Performance benchmarking and agent comparison capabilities
- Configurable performance thresholds and business target tracking
- Statistical analysis with percentiles, averages, and trend detection
- Time-based analysis (hourly, daily, weekly performance trends)
- Performance ranking and comparison across agents
- Actionable insights generation for performance optimization
- Integration with business targets (email <30min, meeting <1hr processing)

Example:
    from observability_lib.services.performance_analyzer import PerformanceAnalyzer
    from observability_lib.config import settings

    analyzer = PerformanceAnalyzer(settings)
    await analyzer.start()

    # Get comprehensive performance analysis
    analysis = await analyzer.get_agent_performance_analysis("email_processor")

    # Get trend analysis
    trends = await analyzer.get_performance_trends(
        agent_id="email_processor",
        time_window="7d",
        metrics=["processing_time_ms", "confidence_score"]
    )

    # Get performance rankings
    rankings = await analyzer.get_agent_performance_rankings()

    # Get business target compliance
    compliance = await analyzer.check_business_target_compliance()
"""

import asyncio
import logging
import statistics
from collections import defaultdict, deque
from datetime import datetime, timedelta
from typing import Any, Dict, List, Optional, Tuple, Union
from dataclasses import dataclass, asdict
from enum import Enum
import uuid

import asyncpg
from sqlalchemy.ext.asyncio import AsyncSession, create_async_engine
from sqlalchemy.orm import sessionmaker
from sqlalchemy import select, func, and_, or_, text, desc, asc
from sqlalchemy.exc import SQLAlchemyError

from observability_lib.config import ObservabilitySettings
from observability_lib.models.agent_metrics import (
    AgentMetric, Agent, WorkflowEvent, DecisionTrace, AlertThreshold
)
from observability_lib.services.metrics_collector import ConnectionPool
from penf_lib.storage.validators import ValidationError


logger = logging.getLogger(__name__)


class PerformanceTrendDirection(Enum):
    """Direction of performance trends"""
    IMPROVING = "improving"
    DECLINING = "declining"
    STABLE = "stable"
    UNKNOWN = "unknown"


class BusinessTargetStatus(Enum):
    """Status of business target compliance"""
    MEETING = "meeting"
    APPROACHING_THRESHOLD = "approaching_threshold"
    EXCEEDING = "exceeding"
    CRITICAL = "critical"


@dataclass
class PerformanceMetric:
    """Single performance metric data point"""
    metric_name: str
    value: float
    timestamp: datetime
    labels: Dict[str, Any]


@dataclass
class TrendAnalysis:
    """Performance trend analysis results"""
    metric_name: str
    direction: PerformanceTrendDirection
    change_percentage: float
    current_average: float
    previous_average: float
    trend_confidence: float
    data_points: int
    time_window: str


@dataclass
class StatisticalSummary:
    """Statistical summary of performance metrics"""
    metric_name: str
    count: int
    average: float
    median: float
    p50: float
    p90: float
    p95: float
    p99: float
    min_value: float
    max_value: float
    std_deviation: float


@dataclass
class PerformanceBenchmark:
    """Performance benchmark comparison"""
    agent_id: str
    metric_name: str
    current_value: float
    benchmark_value: float
    performance_ratio: float  # current/benchmark
    status: str  # "above_benchmark", "below_benchmark", "at_benchmark"
    percentile_rank: float  # 0-100


@dataclass
class BusinessTarget:
    """Business performance target definition"""
    target_id: str
    name: str
    description: str
    metric_name: str
    target_value: float
    warning_threshold: float  # % of target (e.g., 0.8 = 80% of target)
    critical_threshold: float  # % of target (e.g., 1.2 = 120% of target)
    comparison_operator: str  # "less_than", "greater_than"
    time_window: str  # "1h", "24h", etc.


@dataclass
class BusinessTargetCompliance:
    """Business target compliance status"""
    target: BusinessTarget
    current_value: float
    status: BusinessTargetStatus
    compliance_percentage: float
    time_to_target: Optional[str]
    recommendations: List[str]


class PerformanceAnalyzer:
    """Advanced performance analytics service with comprehensive monitoring capabilities.

    This service provides deep performance analysis, trend detection, benchmarking,
    and business target compliance monitoring for the Penfold observability framework.
    """

    # Business targets configuration - these can be overridden via settings
    DEFAULT_BUSINESS_TARGETS = [
        BusinessTarget(
            target_id="email_processing_time",
            name="Email Processing Time",
            description="Process emails within 30 minutes",
            metric_name="processing_time_ms",
            target_value=30 * 60 * 1000,  # 30 minutes in ms
            warning_threshold=0.8,  # 24 minutes
            critical_threshold=1.0,  # 30 minutes
            comparison_operator="less_than",
            time_window="1h"
        ),
        BusinessTarget(
            target_id="meeting_processing_time",
            name="Meeting Processing Time",
            description="Process meeting recordings within 1 hour",
            metric_name="processing_time_ms",
            target_value=60 * 60 * 1000,  # 1 hour in ms
            warning_threshold=0.8,  # 48 minutes
            critical_threshold=1.0,  # 60 minutes
            comparison_operator="less_than",
            time_window="2h"
        ),
        BusinessTarget(
            target_id="decision_confidence",
            name="Decision Confidence",
            description="Maintain decision confidence above 80%",
            metric_name="decision_confidence",
            target_value=0.8,
            warning_threshold=0.9,  # 72% (90% of 80%)
            critical_threshold=0.8,  # 64% (80% of 80%)
            comparison_operator="greater_than",
            time_window="24h"
        ),
        BusinessTarget(
            target_id="workflow_success_rate",
            name="Workflow Success Rate",
            description="Achieve 95% workflow success rate",
            metric_name="workflow_success_rate",
            target_value=0.95,
            warning_threshold=0.9,  # 85.5% (90% of 95%)
            critical_threshold=0.8,  # 76% (80% of 95%)
            comparison_operator="greater_than",
            time_window="24h"
        )
    ]

    def __init__(self, settings: ObservabilitySettings):
        """Initialize performance analyzer.

        Args:
            settings: Observability configuration settings
        """
        self.settings = settings
        self.connection_pool = ConnectionPool(settings)

        # Performance tracking
        self._start_time = None
        self._analyses_performed = 0
        self._cache_hits = 0
        self._cache_misses = 0

        # Analysis cache (agent_id -> (analysis, timestamp))
        self._analysis_cache: Dict[str, Tuple[Dict[str, Any], datetime]] = {}
        self._cache_ttl = timedelta(minutes=5)  # 5-minute cache TTL

        # Benchmark cache
        self._benchmark_cache: Dict[str, Tuple[Dict[str, Any], datetime]] = {}
        self._benchmark_cache_ttl = timedelta(minutes=15)  # 15-minute benchmark cache

        # Business targets (can be customized via settings)
        self.business_targets = self._load_business_targets()

        self._running = False

    async def start(self):
        """Start the performance analyzer."""
        if self._running:
            return

        self._running = True
        self._start_time = datetime.utcnow()

        await self.connection_pool.initialize()
        logger.info("Performance analyzer started")

    async def stop(self):
        """Stop the performance analyzer."""
        if not self._running:
            return

        self._running = False
        await self.connection_pool.close()
        logger.info("Performance analyzer stopped")

    async def get_agent_performance_analysis(
        self,
        agent_id: str,
        time_window: str = "24h",
        include_trends: bool = True,
        include_benchmarks: bool = True
    ) -> Dict[str, Any]:
        """Get comprehensive performance analysis for an agent.

        Args:
            agent_id: Agent identifier
            time_window: Time window for analysis (1h, 24h, 7d, 30d)
            include_trends: Include trend analysis
            include_benchmarks: Include benchmark comparisons

        Returns:
            Comprehensive performance analysis dictionary

        Raises:
            ValidationError: If parameters are invalid
            RuntimeError: If analyzer is not running
        """
        if not self._running:
            raise RuntimeError("Performance analyzer is not running")

        self._validate_agent_id(agent_id)
        self._validate_time_window(time_window)

        # Check cache first
        cache_key = f"{agent_id}_{time_window}_{include_trends}_{include_benchmarks}"
        cached_result = self._get_cached_analysis(cache_key)
        if cached_result:
            self._cache_hits += 1
            return cached_result

        self._cache_misses += 1

        try:
            start_time, end_time = self._parse_time_window(time_window)

            async with self.connection_pool.get_session() as session:
                # Get basic performance metrics
                basic_metrics = await self._get_basic_performance_metrics(
                    session, agent_id, start_time, end_time
                )

                # Get statistical summaries
                statistical_summaries = await self._get_statistical_summaries(
                    session, agent_id, start_time, end_time
                )

                # Get workflow performance
                workflow_performance = await self._get_workflow_performance(
                    session, agent_id, start_time, end_time
                )

                # Get decision quality metrics
                decision_metrics = await self._get_decision_quality_metrics(
                    session, agent_id, start_time, end_time
                )

                analysis = {
                    "agent_id": agent_id,
                    "time_window": time_window,
                    "analysis_timestamp": datetime.utcnow().isoformat(),
                    "basic_metrics": basic_metrics,
                    "statistical_summaries": statistical_summaries,
                    "workflow_performance": workflow_performance,
                    "decision_metrics": decision_metrics,
                }

                # Add trend analysis if requested
                if include_trends:
                    trends = await self.get_performance_trends(
                        agent_id, time_window,
                        metrics=[summary.metric_name for summary in statistical_summaries]
                    )
                    analysis["trend_analysis"] = trends

                # Add benchmark comparisons if requested
                if include_benchmarks:
                    benchmarks = await self.get_agent_performance_benchmarks(
                        agent_id, time_window
                    )
                    analysis["benchmark_comparison"] = benchmarks

                # Add performance recommendations
                analysis["recommendations"] = await self._generate_performance_recommendations(
                    analysis
                )

                # Cache the result
                self._cache_analysis(cache_key, analysis)
                self._analyses_performed += 1

                return analysis

        except Exception as e:
            logger.error(f"Error analyzing performance for agent {agent_id}: {e}")
            raise

    async def get_performance_trends(
        self,
        agent_id: str,
        time_window: str = "7d",
        metrics: Optional[List[str]] = None,
        comparison_window: str = "previous"
    ) -> List[TrendAnalysis]:
        """Analyze performance trends for an agent.

        Args:
            agent_id: Agent identifier
            time_window: Current time window for analysis
            metrics: Optional list of specific metrics to analyze
            comparison_window: How to compare ("previous", "baseline")

        Returns:
            List of trend analyses

        Raises:
            ValidationError: If parameters are invalid
        """
        self._validate_agent_id(agent_id)
        self._validate_time_window(time_window)

        try:
            current_start, current_end = self._parse_time_window(time_window)
            window_duration = current_end - current_start

            # Calculate comparison window
            if comparison_window == "previous":
                comparison_end = current_start
                comparison_start = comparison_end - window_duration
            else:  # baseline - use a longer historical period
                comparison_end = current_start - timedelta(days=1)
                comparison_start = comparison_end - (window_duration * 2)

            async with self.connection_pool.get_session() as session:
                # Get metrics for current period
                current_metrics = await self._get_metrics_for_period(
                    session, agent_id, current_start, current_end, metrics
                )

                # Get metrics for comparison period
                comparison_metrics = await self._get_metrics_for_period(
                    session, agent_id, comparison_start, comparison_end, metrics
                )

                # Analyze trends
                trends = []
                for metric_name in set(current_metrics.keys()) | set(comparison_metrics.keys()):
                    current_values = current_metrics.get(metric_name, [])
                    comparison_values = comparison_metrics.get(metric_name, [])

                    if len(current_values) < 2 or len(comparison_values) < 2:
                        # Not enough data for trend analysis
                        continue

                    current_avg = statistics.mean(current_values)
                    comparison_avg = statistics.mean(comparison_values)

                    # Calculate change percentage
                    if comparison_avg != 0:
                        change_percentage = ((current_avg - comparison_avg) / comparison_avg) * 100
                    else:
                        change_percentage = 0.0

                    # Determine trend direction
                    direction = self._determine_trend_direction(
                        change_percentage, metric_name
                    )

                    # Calculate trend confidence based on data consistency
                    confidence = self._calculate_trend_confidence(
                        current_values, comparison_values
                    )

                    trends.append(TrendAnalysis(
                        metric_name=metric_name,
                        direction=direction,
                        change_percentage=change_percentage,
                        current_average=current_avg,
                        previous_average=comparison_avg,
                        trend_confidence=confidence,
                        data_points=len(current_values),
                        time_window=time_window
                    ))

                return trends

        except Exception as e:
            logger.error(f"Error analyzing trends for agent {agent_id}: {e}")
            raise

    async def get_agent_performance_benchmarks(
        self,
        agent_id: str,
        time_window: str = "24h"
    ) -> List[PerformanceBenchmark]:
        """Get performance benchmarks comparing agent to peer agents.

        Args:
            agent_id: Agent identifier
            time_window: Time window for benchmark comparison

        Returns:
            List of performance benchmarks
        """
        self._validate_agent_id(agent_id)
        self._validate_time_window(time_window)

        cache_key = f"benchmarks_{agent_id}_{time_window}"
        cached_result = self._get_cached_benchmark(cache_key)
        if cached_result:
            return cached_result

        try:
            start_time, end_time = self._parse_time_window(time_window)

            async with self.connection_pool.get_session() as session:
                # Get agent type for peer comparison
                agent_type = await self._get_agent_type(session, agent_id)

                # Get peer agents of the same type
                peer_agents = await self._get_peer_agents(session, agent_id, agent_type)

                if len(peer_agents) < 2:
                    logger.warning(f"Not enough peer agents for benchmarking {agent_id}")
                    return []

                # Get performance metrics for all agents
                all_agent_metrics = {}
                for peer_agent_id in peer_agents + [agent_id]:
                    metrics = await self._get_metrics_for_period(
                        session, peer_agent_id, start_time, end_time
                    )
                    all_agent_metrics[peer_agent_id] = metrics

                # Calculate benchmarks
                benchmarks = []
                for metric_name in set().union(*[metrics.keys() for metrics in all_agent_metrics.values()]):
                    peer_values = []
                    current_values = all_agent_metrics.get(agent_id, {}).get(metric_name, [])

                    if not current_values:
                        continue

                    # Collect peer values for benchmark
                    for peer_id in peer_agents:
                        peer_metric_values = all_agent_metrics.get(peer_id, {}).get(metric_name, [])
                        peer_values.extend(peer_metric_values)

                    if not peer_values:
                        continue

                    current_avg = statistics.mean(current_values)
                    benchmark_avg = statistics.mean(peer_values)

                    # Calculate performance ratio and percentile rank
                    performance_ratio = current_avg / benchmark_avg if benchmark_avg != 0 else 1.0
                    percentile_rank = self._calculate_percentile_rank(current_avg, peer_values)

                    # Determine status
                    if performance_ratio > 1.1:
                        status = "above_benchmark"
                    elif performance_ratio < 0.9:
                        status = "below_benchmark"
                    else:
                        status = "at_benchmark"

                    benchmarks.append(PerformanceBenchmark(
                        agent_id=agent_id,
                        metric_name=metric_name,
                        current_value=current_avg,
                        benchmark_value=benchmark_avg,
                        performance_ratio=performance_ratio,
                        status=status,
                        percentile_rank=percentile_rank
                    ))

                # Cache the result
                self._cache_benchmark(cache_key, benchmarks)
                return benchmarks

        except Exception as e:
            logger.error(f"Error calculating benchmarks for agent {agent_id}: {e}")
            raise

    async def get_agent_performance_rankings(
        self,
        metric_name: str = "processing_time_ms",
        time_window: str = "24h",
        agent_type: Optional[str] = None,
        limit: int = 20
    ) -> List[Dict[str, Any]]:
        """Get performance rankings across agents.

        Args:
            metric_name: Metric to rank by
            time_window: Time window for ranking
            agent_type: Optional filter by agent type
            limit: Maximum number of agents to return

        Returns:
            List of ranked agent performance data
        """
        self._validate_time_window(time_window)

        try:
            start_time, end_time = self._parse_time_window(time_window)

            async with self.connection_pool.get_session() as session:
                # Build query for agent metrics
                query = select(
                    AgentMetric.agent_id,
                    func.avg(AgentMetric.metric_value).label("avg_value"),
                    func.count(AgentMetric.id).label("data_points"),
                    func.stddev(AgentMetric.metric_value).label("std_dev"),
                    func.percentile_cont(0.5).within_group(AgentMetric.metric_value).label("median"),
                    func.percentile_cont(0.95).within_group(AgentMetric.metric_value).label("p95")
                ).where(
                    and_(
                        AgentMetric.metric_name == metric_name,
                        AgentMetric.timestamp >= start_time,
                        AgentMetric.timestamp <= end_time
                    )
                ).group_by(AgentMetric.agent_id)

                # Add agent type filter if specified
                if agent_type:
                    query = query.join(Agent).where(Agent.agent_type == agent_type)

                # Order by metric value (ascending for time metrics, descending for quality metrics)
                if "time" in metric_name.lower() or "duration" in metric_name.lower():
                    query = query.order_by(asc("avg_value"))
                else:
                    query = query.order_by(desc("avg_value"))

                query = query.limit(limit)

                result = await session.execute(query)
                rows = result.fetchall()

                # Format rankings
                rankings = []
                for rank, row in enumerate(rows, 1):
                    # Get agent details
                    agent_query = select(Agent).where(Agent.agent_id == row.agent_id)
                    agent_result = await session.execute(agent_query)
                    agent = agent_result.scalar_one_or_none()

                    rankings.append({
                        "rank": rank,
                        "agent_id": row.agent_id,
                        "agent_name": agent.agent_name if agent else row.agent_id,
                        "agent_type": agent.agent_type if agent else "unknown",
                        "metric_name": metric_name,
                        "average_value": float(row.avg_value),
                        "median_value": float(row.median or 0),
                        "p95_value": float(row.p95 or 0),
                        "std_deviation": float(row.std_dev or 0),
                        "data_points": row.data_points,
                        "time_window": time_window
                    })

                return rankings

        except Exception as e:
            logger.error(f"Error calculating performance rankings: {e}")
            raise

    async def check_business_target_compliance(
        self,
        agent_id: Optional[str] = None
    ) -> List[BusinessTargetCompliance]:
        """Check compliance with business performance targets.

        Args:
            agent_id: Optional agent filter

        Returns:
            List of business target compliance results
        """
        try:
            compliance_results = []

            for target in self.business_targets:
                # Filter by agent if specified
                if agent_id and not self._target_applies_to_agent(target, agent_id):
                    continue

                start_time, end_time = self._parse_time_window(target.time_window)

                async with self.connection_pool.get_session() as session:
                    # Get current metric value
                    if target.metric_name == "workflow_success_rate":
                        current_value = await self._calculate_workflow_success_rate(
                            session, agent_id, start_time, end_time
                        )
                    else:
                        current_value = await self._get_average_metric_value(
                            session, agent_id, target.metric_name, start_time, end_time
                        )

                    if current_value is None:
                        continue

                    # Determine compliance status
                    status, compliance_percentage = self._evaluate_target_compliance(
                        target, current_value
                    )

                    # Calculate time to target if trending
                    time_to_target = await self._estimate_time_to_target(
                        target, current_value, agent_id
                    )

                    # Generate recommendations
                    recommendations = self._generate_target_recommendations(
                        target, current_value, status
                    )

                    compliance_results.append(BusinessTargetCompliance(
                        target=target,
                        current_value=current_value,
                        status=status,
                        compliance_percentage=compliance_percentage,
                        time_to_target=time_to_target,
                        recommendations=recommendations
                    ))

            return compliance_results

        except Exception as e:
            logger.error(f"Error checking business target compliance: {e}")
            raise

    async def get_time_based_performance_analysis(
        self,
        agent_id: str,
        metric_name: str,
        time_window: str = "7d",
        bucket_size: str = "1h"
    ) -> Dict[str, Any]:
        """Get time-based performance analysis (hourly, daily patterns).

        Args:
            agent_id: Agent identifier
            metric_name: Metric to analyze
            time_window: Time window for analysis
            bucket_size: Time bucket size (1h, 4h, 1d)

        Returns:
            Time-based performance analysis
        """
        self._validate_agent_id(agent_id)
        self._validate_time_window(time_window)

        try:
            start_time, end_time = self._parse_time_window(time_window)
            bucket_delta = self._parse_bucket_size(bucket_size)

            async with self.connection_pool.get_session() as session:
                # Get metrics data
                query = select(
                    AgentMetric.timestamp,
                    AgentMetric.metric_value
                ).where(
                    and_(
                        AgentMetric.agent_id == agent_id,
                        AgentMetric.metric_name == metric_name,
                        AgentMetric.timestamp >= start_time,
                        AgentMetric.timestamp <= end_time
                    )
                ).order_by(AgentMetric.timestamp)

                result = await session.execute(query)
                rows = result.fetchall()

                if not rows:
                    return {
                        "agent_id": agent_id,
                        "metric_name": metric_name,
                        "time_window": time_window,
                        "bucket_size": bucket_size,
                        "buckets": [],
                        "patterns": {},
                        "analysis": "No data available for analysis"
                    }

                # Group by time buckets
                buckets = defaultdict(list)
                for row in rows:
                    bucket_start = self._get_bucket_start(row.timestamp, bucket_delta)
                    buckets[bucket_start].append(row.metric_value)

                # Calculate bucket statistics
                bucket_stats = []
                for bucket_start in sorted(buckets.keys()):
                    values = buckets[bucket_start]
                    bucket_stats.append({
                        "timestamp": bucket_start.isoformat(),
                        "count": len(values),
                        "average": statistics.mean(values),
                        "median": statistics.median(values),
                        "min": min(values),
                        "max": max(values),
                        "std_dev": statistics.stdev(values) if len(values) > 1 else 0.0
                    })

                # Analyze patterns
                patterns = self._analyze_time_patterns(bucket_stats, bucket_delta)

                return {
                    "agent_id": agent_id,
                    "metric_name": metric_name,
                    "time_window": time_window,
                    "bucket_size": bucket_size,
                    "buckets": bucket_stats,
                    "patterns": patterns,
                    "analysis": self._generate_time_analysis_summary(patterns)
                }

        except Exception as e:
            logger.error(f"Error performing time-based analysis: {e}")
            raise

    async def get_analyzer_statistics(self) -> Dict[str, Any]:
        """Get performance statistics about the analyzer itself.

        Returns:
            Dictionary with analyzer performance metrics
        """
        uptime = datetime.utcnow() - self._start_time if self._start_time else timedelta(0)

        return {
            "uptime_seconds": uptime.total_seconds(),
            "analyses_performed": self._analyses_performed,
            "cache_hits": self._cache_hits,
            "cache_misses": self._cache_misses,
            "cache_hit_ratio": self._cache_hits / max(self._cache_hits + self._cache_misses, 1),
            "cache_size": len(self._analysis_cache),
            "benchmark_cache_size": len(self._benchmark_cache),
            "business_targets_count": len(self.business_targets),
            "connection_healthy": self.connection_pool.is_healthy if hasattr(self.connection_pool, 'is_healthy') else True
        }

    # Private helper methods

    def _load_business_targets(self) -> List[BusinessTarget]:
        """Load business targets from configuration or defaults."""
        # TODO: Load from settings or database
        return self.DEFAULT_BUSINESS_TARGETS.copy()

    def _get_cached_analysis(self, cache_key: str) -> Optional[Dict[str, Any]]:
        """Get cached analysis if still valid."""
        if cache_key in self._analysis_cache:
            analysis, timestamp = self._analysis_cache[cache_key]
            if datetime.utcnow() - timestamp < self._cache_ttl:
                return analysis
            else:
                del self._analysis_cache[cache_key]
        return None

    def _cache_analysis(self, cache_key: str, analysis: Dict[str, Any]):
        """Cache analysis result."""
        self._analysis_cache[cache_key] = (analysis, datetime.utcnow())

        # Clean old entries
        current_time = datetime.utcnow()
        expired_keys = [
            key for key, (_, timestamp) in self._analysis_cache.items()
            if current_time - timestamp >= self._cache_ttl
        ]
        for key in expired_keys:
            del self._analysis_cache[key]

    def _get_cached_benchmark(self, cache_key: str) -> Optional[List[PerformanceBenchmark]]:
        """Get cached benchmark if still valid."""
        if cache_key in self._benchmark_cache:
            benchmark, timestamp = self._benchmark_cache[cache_key]
            if datetime.utcnow() - timestamp < self._benchmark_cache_ttl:
                return benchmark
            else:
                del self._benchmark_cache[cache_key]
        return None

    def _cache_benchmark(self, cache_key: str, benchmark: List[PerformanceBenchmark]):
        """Cache benchmark result."""
        self._benchmark_cache[cache_key] = (benchmark, datetime.utcnow())

    async def _get_basic_performance_metrics(
        self,
        session: AsyncSession,
        agent_id: str,
        start_time: datetime,
        end_time: datetime
    ) -> Dict[str, Any]:
        """Get basic performance metrics for an agent."""
        # Get metric counts and averages
        query = select(
            AgentMetric.metric_name,
            func.count(AgentMetric.id).label("count"),
            func.avg(AgentMetric.metric_value).label("average"),
            func.min(AgentMetric.metric_value).label("min_value"),
            func.max(AgentMetric.metric_value).label("max_value")
        ).where(
            and_(
                AgentMetric.agent_id == agent_id,
                AgentMetric.timestamp >= start_time,
                AgentMetric.timestamp <= end_time
            )
        ).group_by(AgentMetric.metric_name)

        result = await session.execute(query)
        rows = result.fetchall()

        metrics = {}
        for row in rows:
            metrics[row.metric_name] = {
                "count": row.count,
                "average": float(row.average),
                "min_value": float(row.min_value),
                "max_value": float(row.max_value)
            }

        return metrics

    async def _get_statistical_summaries(
        self,
        session: AsyncSession,
        agent_id: str,
        start_time: datetime,
        end_time: datetime
    ) -> List[StatisticalSummary]:
        """Get statistical summaries for agent metrics."""
        # Get all metric values
        query = select(
            AgentMetric.metric_name,
            AgentMetric.metric_value
        ).where(
            and_(
                AgentMetric.agent_id == agent_id,
                AgentMetric.timestamp >= start_time,
                AgentMetric.timestamp <= end_time
            )
        ).order_by(AgentMetric.metric_name, AgentMetric.metric_value)

        result = await session.execute(query)
        rows = result.fetchall()

        # Group by metric name
        metric_values = defaultdict(list)
        for row in rows:
            metric_values[row.metric_name].append(row.metric_value)

        # Calculate statistics
        summaries = []
        for metric_name, values in metric_values.items():
            if len(values) < 2:
                continue

            sorted_values = sorted(values)

            summaries.append(StatisticalSummary(
                metric_name=metric_name,
                count=len(values),
                average=statistics.mean(values),
                median=statistics.median(values),
                p50=self._percentile(sorted_values, 50),
                p90=self._percentile(sorted_values, 90),
                p95=self._percentile(sorted_values, 95),
                p99=self._percentile(sorted_values, 99),
                min_value=min(values),
                max_value=max(values),
                std_deviation=statistics.stdev(values)
            ))

        return summaries

    async def _get_workflow_performance(
        self,
        session: AsyncSession,
        agent_id: str,
        start_time: datetime,
        end_time: datetime
    ) -> Dict[str, Any]:
        """Get workflow performance metrics."""
        # Get workflow event statistics
        query = select(
            func.count(WorkflowEvent.id).label("total_events"),
            func.sum(
                func.case((WorkflowEvent.event_status == "success", 1), else_=0)
            ).label("success_events"),
            func.avg(WorkflowEvent.processing_time_ms).label("avg_processing_time"),
            func.count(
                func.distinct(WorkflowEvent.workflow_id)
            ).label("unique_workflows")
        ).where(
            and_(
                WorkflowEvent.agent_id == agent_id,
                WorkflowEvent.timestamp >= start_time,
                WorkflowEvent.timestamp <= end_time
            )
        )

        result = await session.execute(query)
        row = result.fetchone()

        if not row or row.total_events == 0:
            return {
                "total_events": 0,
                "success_rate": 0.0,
                "average_processing_time_ms": 0.0,
                "unique_workflows": 0
            }

        success_rate = (row.success_events or 0) / row.total_events

        return {
            "total_events": row.total_events,
            "success_rate": success_rate,
            "average_processing_time_ms": float(row.avg_processing_time or 0),
            "unique_workflows": row.unique_workflows
        }

    async def _get_decision_quality_metrics(
        self,
        session: AsyncSession,
        agent_id: str,
        start_time: datetime,
        end_time: datetime
    ) -> Dict[str, Any]:
        """Get decision quality metrics."""
        query = select(
            func.count(DecisionTrace.id).label("total_decisions"),
            func.avg(DecisionTrace.confidence_score).label("avg_confidence"),
            func.min(DecisionTrace.confidence_score).label("min_confidence"),
            func.max(DecisionTrace.confidence_score).label("max_confidence")
        ).where(
            and_(
                DecisionTrace.agent_id == agent_id,
                DecisionTrace.timestamp >= start_time,
                DecisionTrace.timestamp <= end_time
            )
        )

        result = await session.execute(query)
        row = result.fetchone()

        if not row or row.total_decisions == 0:
            return {
                "total_decisions": 0,
                "average_confidence": 0.0,
                "min_confidence": 0.0,
                "max_confidence": 0.0
            }

        return {
            "total_decisions": row.total_decisions,
            "average_confidence": float(row.avg_confidence or 0),
            "min_confidence": float(row.min_confidence or 0),
            "max_confidence": float(row.max_confidence or 0)
        }

    async def _get_metrics_for_period(
        self,
        session: AsyncSession,
        agent_id: str,
        start_time: datetime,
        end_time: datetime,
        metrics: Optional[List[str]] = None
    ) -> Dict[str, List[float]]:
        """Get metric values for a time period."""
        query = select(
            AgentMetric.metric_name,
            AgentMetric.metric_value
        ).where(
            and_(
                AgentMetric.agent_id == agent_id,
                AgentMetric.timestamp >= start_time,
                AgentMetric.timestamp <= end_time
            )
        )

        if metrics:
            query = query.where(AgentMetric.metric_name.in_(metrics))

        result = await session.execute(query)
        rows = result.fetchall()

        metric_values = defaultdict(list)
        for row in rows:
            metric_values[row.metric_name].append(row.metric_value)

        return dict(metric_values)

    async def _get_agent_type(self, session: AsyncSession, agent_id: str) -> str:
        """Get agent type."""
        query = select(Agent.agent_type).where(Agent.agent_id == agent_id)
        result = await session.execute(query)
        row = result.fetchone()
        return row.agent_type if row else "unknown"

    async def _get_peer_agents(
        self,
        session: AsyncSession,
        agent_id: str,
        agent_type: str
    ) -> List[str]:
        """Get peer agents of the same type."""
        query = select(Agent.agent_id).where(
            and_(
                Agent.agent_type == agent_type,
                Agent.agent_id != agent_id,
                Agent.is_active == True
            )
        )

        result = await session.execute(query)
        return [row.agent_id for row in result.fetchall()]

    async def _calculate_workflow_success_rate(
        self,
        session: AsyncSession,
        agent_id: Optional[str],
        start_time: datetime,
        end_time: datetime
    ) -> Optional[float]:
        """Calculate workflow success rate."""
        query = select(
            func.count(WorkflowEvent.id).label("total"),
            func.sum(
                func.case((WorkflowEvent.event_status == "success", 1), else_=0)
            ).label("success")
        ).where(
            and_(
                WorkflowEvent.timestamp >= start_time,
                WorkflowEvent.timestamp <= end_time,
                WorkflowEvent.event_type == "workflow_finished"
            )
        )

        if agent_id:
            query = query.where(WorkflowEvent.agent_id == agent_id)

        result = await session.execute(query)
        row = result.fetchone()

        if not row or row.total == 0:
            return None

        return (row.success or 0) / row.total

    async def _get_average_metric_value(
        self,
        session: AsyncSession,
        agent_id: Optional[str],
        metric_name: str,
        start_time: datetime,
        end_time: datetime
    ) -> Optional[float]:
        """Get average metric value."""
        query = select(
            func.avg(AgentMetric.metric_value)
        ).where(
            and_(
                AgentMetric.metric_name == metric_name,
                AgentMetric.timestamp >= start_time,
                AgentMetric.timestamp <= end_time
            )
        )

        if agent_id:
            query = query.where(AgentMetric.agent_id == agent_id)

        result = await session.execute(query)
        row = result.fetchone()

        return float(row[0]) if row and row[0] is not None else None

    def _determine_trend_direction(
        self,
        change_percentage: float,
        metric_name: str
    ) -> PerformanceTrendDirection:
        """Determine trend direction based on change percentage and metric type."""
        threshold = 5.0  # 5% change threshold

        if abs(change_percentage) < threshold:
            return PerformanceTrendDirection.STABLE

        # For time/duration metrics, decreasing is improving
        if "time" in metric_name.lower() or "duration" in metric_name.lower():
            if change_percentage < -threshold:
                return PerformanceTrendDirection.IMPROVING
            elif change_percentage > threshold:
                return PerformanceTrendDirection.DECLINING
        else:
            # For quality/confidence metrics, increasing is improving
            if change_percentage > threshold:
                return PerformanceTrendDirection.IMPROVING
            elif change_percentage < -threshold:
                return PerformanceTrendDirection.DECLINING

        return PerformanceTrendDirection.STABLE

    def _calculate_trend_confidence(
        self,
        current_values: List[float],
        comparison_values: List[float]
    ) -> float:
        """Calculate confidence in trend analysis."""
        # More data points = higher confidence
        data_confidence = min(
            (len(current_values) + len(comparison_values)) / 100.0, 1.0
        )

        # Lower variance = higher confidence
        try:
            current_cv = statistics.stdev(current_values) / statistics.mean(current_values)
            comparison_cv = statistics.stdev(comparison_values) / statistics.mean(comparison_values)
            variance_confidence = 1.0 / (1.0 + (current_cv + comparison_cv) / 2.0)
        except (statistics.StatisticsError, ZeroDivisionError):
            variance_confidence = 0.5

        return (data_confidence + variance_confidence) / 2.0

    def _calculate_percentile_rank(self, value: float, peer_values: List[float]) -> float:
        """Calculate percentile rank of value within peer values."""
        if not peer_values:
            return 50.0

        sorted_values = sorted(peer_values)
        rank = sum(1 for v in sorted_values if v <= value)
        return (rank / len(sorted_values)) * 100

    def _evaluate_target_compliance(
        self,
        target: BusinessTarget,
        current_value: float
    ) -> Tuple[BusinessTargetStatus, float]:
        """Evaluate compliance with business target."""
        if target.comparison_operator == "less_than":
            warning_threshold = target.target_value * target.warning_threshold
            critical_threshold = target.target_value * target.critical_threshold

            if current_value <= warning_threshold:
                status = BusinessTargetStatus.MEETING
                compliance_percentage = (warning_threshold - current_value) / warning_threshold * 100
            elif current_value <= critical_threshold:
                status = BusinessTargetStatus.APPROACHING_THRESHOLD
                compliance_percentage = (critical_threshold - current_value) / critical_threshold * 100
            else:
                status = BusinessTargetStatus.EXCEEDING
                compliance_percentage = 0.0

        else:  # greater_than
            warning_threshold = target.target_value * target.warning_threshold
            critical_threshold = target.target_value * target.critical_threshold

            if current_value >= target.target_value:
                status = BusinessTargetStatus.MEETING
                compliance_percentage = (current_value - target.target_value) / target.target_value * 100
            elif current_value >= warning_threshold:
                status = BusinessTargetStatus.APPROACHING_THRESHOLD
                compliance_percentage = (current_value - warning_threshold) / warning_threshold * 100
            else:
                status = BusinessTargetStatus.CRITICAL
                compliance_percentage = 0.0

        return status, max(0.0, min(100.0, compliance_percentage))

    async def _estimate_time_to_target(
        self,
        target: BusinessTarget,
        current_value: float,
        agent_id: Optional[str]
    ) -> Optional[str]:
        """Estimate time to reach target based on trends."""
        # Simplified estimation - could be enhanced with ML
        try:
            trends = await self.get_performance_trends(
                agent_id, "7d", [target.metric_name]
            ) if agent_id else []

            for trend in trends:
                if trend.metric_name == target.metric_name:
                    if trend.direction in [PerformanceTrendDirection.STABLE, PerformanceTrendDirection.UNKNOWN]:
                        return "No clear trend"

                    # Calculate days to target based on trend
                    if trend.change_percentage != 0:
                        days_to_target = abs(
                            (target.target_value - current_value) /
                            (current_value * trend.change_percentage / 100) * 7
                        )

                        if days_to_target < 365:
                            return f"~{int(days_to_target)} days"
                        else:
                            return ">1 year"

        except Exception:
            pass

        return None

    def _generate_target_recommendations(
        self,
        target: BusinessTarget,
        current_value: float,
        status: BusinessTargetStatus
    ) -> List[str]:
        """Generate recommendations for target compliance."""
        recommendations = []

        if status == BusinessTargetStatus.MEETING:
            recommendations.append(f"✅ Meeting {target.name} target")
        elif status == BusinessTargetStatus.APPROACHING_THRESHOLD:
            recommendations.append(f"⚠️ Approaching {target.name} threshold")
            recommendations.append("Consider optimizing performance")
        elif status == BusinessTargetStatus.EXCEEDING:
            recommendations.append(f"🚨 Exceeding {target.name} threshold")
            recommendations.append("Immediate optimization required")
        else:  # CRITICAL
            recommendations.append(f"🔥 Critical: Far from {target.name} target")
            recommendations.append("Urgent performance review needed")

        return recommendations

    async def _generate_performance_recommendations(
        self,
        analysis: Dict[str, Any]
    ) -> List[str]:
        """Generate performance optimization recommendations."""
        recommendations = []

        # Analyze workflow success rate
        workflow_perf = analysis.get("workflow_performance", {})
        success_rate = workflow_perf.get("success_rate", 1.0)

        if success_rate < 0.9:
            recommendations.append("🔧 Low workflow success rate - review error handling")

        # Analyze decision confidence
        decision_metrics = analysis.get("decision_metrics", {})
        avg_confidence = decision_metrics.get("average_confidence", 1.0)

        if avg_confidence < 0.7:
            recommendations.append("🧠 Low decision confidence - consider model tuning")

        # Analyze processing times
        basic_metrics = analysis.get("basic_metrics", {})
        for metric_name, data in basic_metrics.items():
            if "processing_time" in metric_name and data["average"] > 30000:  # 30 seconds
                recommendations.append(f"⏱️ High {metric_name} - optimize processing pipeline")

        if not recommendations:
            recommendations.append("✨ Performance looks good - continue monitoring")

        return recommendations

    def _analyze_time_patterns(
        self,
        bucket_stats: List[Dict[str, Any]],
        bucket_delta: timedelta
    ) -> Dict[str, Any]:
        """Analyze time-based patterns in performance data."""
        if not bucket_stats:
            return {}

        values = [stat["average"] for stat in bucket_stats]

        # Find peak and valley hours
        max_idx = values.index(max(values))
        min_idx = values.index(min(values))

        peak_time = bucket_stats[max_idx]["timestamp"]
        valley_time = bucket_stats[min_idx]["timestamp"]

        # Detect trends
        if len(values) >= 3:
            trend_direction = "improving" if values[-1] < values[0] else "declining"
        else:
            trend_direction = "stable"

        return {
            "peak_performance_time": peak_time,
            "peak_value": max(values),
            "valley_performance_time": valley_time,
            "valley_value": min(values),
            "overall_trend": trend_direction,
            "variance": statistics.variance(values) if len(values) > 1 else 0.0
        }

    def _generate_time_analysis_summary(self, patterns: Dict[str, Any]) -> str:
        """Generate human-readable summary of time analysis."""
        if not patterns:
            return "Insufficient data for time-based analysis"

        peak_time = patterns.get("peak_performance_time", "")
        valley_time = patterns.get("valley_performance_time", "")
        trend = patterns.get("overall_trend", "stable")

        summary = f"Performance trend: {trend}. "
        summary += f"Peak performance at {peak_time}, "
        summary += f"lowest at {valley_time}."

        return summary

    def _parse_time_window(self, time_window: str) -> Tuple[datetime, datetime]:
        """Parse time window string to start/end datetime."""
        end_time = datetime.utcnow()

        if time_window.endswith('h'):
            hours = int(time_window[:-1])
            start_time = end_time - timedelta(hours=hours)
        elif time_window.endswith('d'):
            days = int(time_window[:-1])
            start_time = end_time - timedelta(days=days)
        elif time_window.endswith('w'):
            weeks = int(time_window[:-1])
            start_time = end_time - timedelta(weeks=weeks)
        else:
            # Default to 24 hours
            start_time = end_time - timedelta(hours=24)

        return start_time, end_time

    def _parse_bucket_size(self, bucket_size: str) -> timedelta:
        """Parse bucket size string to timedelta."""
        import re

        match = re.match(r"(\d+)([smhd])", bucket_size.lower())
        if not match:
            raise ValidationError(f"Invalid bucket size format: {bucket_size}")

        value, unit = int(match.group(1)), match.group(2)

        if unit == "s":
            return timedelta(seconds=value)
        elif unit == "m":
            return timedelta(minutes=value)
        elif unit == "h":
            return timedelta(hours=value)
        elif unit == "d":
            return timedelta(days=value)
        else:
            raise ValidationError(f"Invalid bucket size unit: {unit}")

    def _get_bucket_start(self, timestamp: datetime, bucket_delta: timedelta) -> datetime:
        """Get the start time of the bucket for a given timestamp."""
        total_seconds = int(bucket_delta.total_seconds())
        timestamp_seconds = int(timestamp.timestamp())
        bucket_start_seconds = (timestamp_seconds // total_seconds) * total_seconds

        return datetime.fromtimestamp(bucket_start_seconds, tz=timestamp.tzinfo)

    def _percentile(self, sorted_values: List[float], percentile: int) -> float:
        """Calculate percentile of sorted values."""
        if not sorted_values:
            return 0.0

        index = (percentile / 100.0) * (len(sorted_values) - 1)
        lower = int(index)
        upper = min(lower + 1, len(sorted_values) - 1)

        if lower == upper:
            return sorted_values[lower]

        weight = index - lower
        return sorted_values[lower] * (1 - weight) + sorted_values[upper] * weight

    def _target_applies_to_agent(self, target: BusinessTarget, agent_id: str) -> bool:
        """Check if a business target applies to a specific agent."""
        # This could be enhanced with agent type filtering
        return True

    def _validate_agent_id(self, agent_id: str):
        """Validate agent ID format."""
        if not agent_id or not isinstance(agent_id, str):
            raise ValidationError("Agent ID must be a non-empty string")

        if len(agent_id) < 3 or len(agent_id) > 100:
            raise ValidationError("Agent ID must be between 3 and 100 characters")

        if not all(c.isalnum() or c == '_' for c in agent_id):
            raise ValidationError("Agent ID must contain only alphanumeric characters and underscores")

    def _validate_time_window(self, time_window: str):
        """Validate time window format."""
        import re

        pattern = r"^\d+[hdw]$"
        if not re.match(pattern, time_window):
            raise ValidationError("Time window must be in format: <number>[h|d|w] (e.g., '24h', '7d', '2w')")
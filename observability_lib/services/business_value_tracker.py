"""Business Value Tracker Service for KPI monitoring and value measurement.

This module implements the BusinessValueTracker service that monitors key business
performance indicators, tracks target achievement, and provides comprehensive
business value analytics for the Penfold observability framework.

Features:
- Real-time KPI tracking and measurement
- Context reconstruction speed monitoring (<15 min target)
- Search accuracy tracking (>90% target)
- Relationship validation rate monitoring
- Automated threshold checking and alerting
- Progress tracking against business goals
- Value trend analysis and forecasting
- Multi-dimensional business insights

Key Performance Indicators Tracked:
- Context reconstruction speed: Time to rebuild context from fragmented information
- Search accuracy: Percentage of relevant results in search queries
- Relationship validation rate: Success rate of automated relationship detection
- User satisfaction score: Aggregated user feedback metrics
- Data quality score: Accuracy and completeness of extracted information
- Workflow efficiency: Time savings and process improvements

Example:
    from observability_lib.services.business_value_tracker import BusinessValueTracker
    from observability_lib.config import settings

    tracker = BusinessValueTracker(settings)
    await tracker.start()

    # Record KPI measurement
    await tracker.record_kpi(
        kpi_type="context_reconstruction_speed",
        value=8.5,  # minutes
        context={"query_complexity": "medium", "data_sources": 5}
    )

    # Get current KPI status
    status = await tracker.get_kpi_status("search_accuracy")

    # Check KPI against targets
    alerts = await tracker.check_kpi_thresholds()

    await tracker.stop()
"""

import asyncio
import logging
import time
import uuid
from collections import defaultdict, deque
from dataclasses import dataclass, asdict
from datetime import datetime, timedelta, timezone
from typing import Any, Dict, List, Optional, Tuple, Union
from enum import Enum
import statistics
import math

from sqlalchemy.ext.asyncio import AsyncSession, create_async_engine
from sqlalchemy.orm import sessionmaker
from sqlalchemy import select, func, and_, or_, text, desc, asc
from sqlalchemy.exc import SQLAlchemyError

from observability_lib.config import ObservabilitySettings
from observability_lib.models.business_metrics import (
    BusinessKpi, BusinessGoal, ValueCalculation,
    KpiType, BusinessGoalType, GoalStatus, ValueCalculationType
)
from observability_lib.models.agent_metrics import (
    AgentMetric, DecisionTrace, WorkflowEvent, Agent
)
from penf_lib.storage.validators import ValidationError


logger = logging.getLogger(__name__)


# ============================================================================
# Business Value Data Types and Constants
# ============================================================================

class KpiStatus(str, Enum):
    """Status of KPI performance against targets"""
    EXCEEDING = "exceeding"      # Above target by >10%
    MEETING = "meeting"          # Within 10% of target
    BELOW_TARGET = "below_target" # Below target but above critical
    CRITICAL = "critical"        # Significantly below target


class TrendDirection(str, Enum):
    """Direction of KPI trends"""
    IMPROVING = "improving"
    STABLE = "stable"
    DECLINING = "declining"
    VOLATILE = "volatile"


@dataclass
class KpiMeasurement:
    """Single KPI measurement with context"""
    kpi_type: str
    value: float
    unit: str
    timestamp: datetime
    context: Dict[str, Any]
    target: Optional[float] = None
    baseline: Optional[float] = None


@dataclass
class KpiSummary:
    """Summary of KPI performance and trends"""
    kpi_type: str
    kpi_name: str
    current_value: float
    target_value: Optional[float]
    baseline_value: Optional[float]
    unit: str
    status: KpiStatus
    trend_direction: TrendDirection
    trend_percentage: float
    target_achievement: Optional[float]
    last_updated: datetime
    measurements_count: int


@dataclass
class BusinessAlert:
    """Alert for KPI threshold violations"""
    alert_id: str
    kpi_type: str
    severity: str
    message: str
    current_value: float
    threshold_value: float
    triggered_at: datetime
    context: Dict[str, Any]


# Performance targets for Penfold system
PENFOLD_KPI_TARGETS = {
    "context_reconstruction_speed": {
        "target": 15.0,  # minutes
        "unit": "minutes",
        "critical_threshold": 30.0,
        "improvement_positive": False,  # Lower is better
        "name": "Context Reconstruction Speed"
    },
    "search_accuracy": {
        "target": 90.0,  # percentage
        "unit": "percentage",
        "critical_threshold": 70.0,
        "improvement_positive": True,  # Higher is better
        "name": "Search Accuracy"
    },
    "relationship_validation_rate": {
        "target": 85.0,  # percentage
        "unit": "percentage",
        "critical_threshold": 60.0,
        "improvement_positive": True,
        "name": "Relationship Validation Rate"
    },
    "user_satisfaction_score": {
        "target": 80.0,  # score out of 100
        "unit": "score",
        "critical_threshold": 60.0,
        "improvement_positive": True,
        "name": "User Satisfaction Score"
    },
    "data_quality_score": {
        "target": 95.0,  # percentage
        "unit": "percentage",
        "critical_threshold": 80.0,
        "improvement_positive": True,
        "name": "Data Quality Score"
    },
    "workflow_efficiency": {
        "target": 75.0,  # percentage improvement
        "unit": "percentage",
        "critical_threshold": 25.0,
        "improvement_positive": True,
        "name": "Workflow Efficiency"
    }
}


# ============================================================================
# Business Value Tracker Service
# ============================================================================

class BusinessValueTracker:
    """Service for tracking business KPIs and value delivery metrics.

    Monitors key performance indicators that demonstrate business value
    from the Penfold system, including speed, accuracy, and efficiency metrics.
    """

    def __init__(self, settings: ObservabilitySettings):
        """Initialize the business value tracker.

        Args:
            settings: Observability configuration settings
        """
        self.settings = settings
        self.engine = None
        self.session_factory = None
        self.is_running = False
        self.tenant_id = settings.DEFAULT_TENANT_ID

        # In-memory caches for performance
        self.kpi_cache: Dict[str, KpiSummary] = {}
        self.cache_ttl_seconds = 300  # 5 minutes
        self.last_cache_update: Dict[str, datetime] = {}

        # Alert thresholds
        self.alert_thresholds = PENFOLD_KPI_TARGETS.copy()

        # Background tasks
        self.monitoring_task: Optional[asyncio.Task] = None
        self.monitoring_interval = 60  # 1 minute

    async def start(self) -> None:
        """Start the business value tracker service."""
        if self.is_running:
            logger.warning("BusinessValueTracker is already running")
            return

        logger.info("Starting BusinessValueTracker service")

        try:
            # Initialize database connection
            self.engine = create_async_engine(
                self.settings.get_database_url(),
                pool_size=10,
                max_overflow=20,
                pool_pre_ping=True,
                echo=self.settings.DEBUG_SQL
            )

            self.session_factory = sessionmaker(
                bind=self.engine,
                class_=AsyncSession,
                expire_on_commit=False
            )

            # Test database connection
            async with self.session_factory() as session:
                await session.execute(text("SELECT 1"))

            self.is_running = True

            # Start background monitoring
            self.monitoring_task = asyncio.create_task(self._monitoring_loop())

            logger.info("BusinessValueTracker service started successfully")

        except Exception as e:
            logger.error(f"Failed to start BusinessValueTracker: {e}")
            await self.stop()
            raise

    async def stop(self) -> None:
        """Stop the business value tracker service."""
        if not self.is_running:
            return

        logger.info("Stopping BusinessValueTracker service")
        self.is_running = False

        # Cancel background tasks
        if self.monitoring_task:
            self.monitoring_task.cancel()
            try:
                await self.monitoring_task
            except asyncio.CancelledError:
                pass

        # Close database connection
        if self.engine:
            await self.engine.dispose()

        # Clear caches
        self.kpi_cache.clear()
        self.last_cache_update.clear()

        logger.info("BusinessValueTracker service stopped")

    async def record_kpi(
        self,
        kpi_type: str,
        value: float,
        context: Optional[Dict[str, Any]] = None,
        timestamp: Optional[datetime] = None,
        unit: Optional[str] = None,
        target: Optional[float] = None,
        baseline: Optional[float] = None
    ) -> str:
        """Record a new KPI measurement.

        Args:
            kpi_type: Type of KPI (from KpiType enum)
            value: Measured value
            context: Additional context about the measurement
            timestamp: When measurement was taken (defaults to now)
            unit: Unit of measurement (inferred if not provided)
            target: Target value (uses default if not provided)
            baseline: Baseline value for comparison

        Returns:
            ID of the created KPI record
        """
        if not self.is_running:
            raise RuntimeError("BusinessValueTracker is not running")

        # Validate KPI type
        try:
            KpiType(kpi_type)
        except ValueError:
            raise ValidationError(f"Invalid KPI type: {kpi_type}")

        if timestamp is None:
            timestamp = datetime.now(timezone.utc)

        # Get KPI configuration
        kpi_config = PENFOLD_KPI_TARGETS.get(kpi_type, {})
        if unit is None:
            unit = kpi_config.get("unit", "value")
        if target is None:
            target = kpi_config.get("target")

        context = context or {}

        try:
            async with self.session_factory() as session:
                # Create KPI record
                kpi_record = BusinessKpi(
                    tenant_id=self.tenant_id,
                    timestamp=timestamp,
                    kpi_type=kpi_type,
                    kpi_name=kpi_config.get("name", kpi_type.replace("_", " ").title()),
                    current_value=float(value),
                    target_value=float(target) if target is not None else None,
                    baseline_value=float(baseline) if baseline is not None else None,
                    measurement_unit=unit,
                    measurement_context=context,
                    is_improvement_positive=kpi_config.get("improvement_positive", True)
                )

                session.add(kpi_record)
                await session.commit()
                await session.refresh(kpi_record)

                logger.info(f"Recorded KPI: {kpi_type} = {value} {unit}")

                # Invalidate cache for this KPI
                if kpi_type in self.kpi_cache:
                    del self.kpi_cache[kpi_type]

                # Check for threshold violations
                await self._check_kpi_thresholds(kpi_type, value, context)

                return str(kpi_record.id)

        except SQLAlchemyError as e:
            logger.error(f"Database error recording KPI {kpi_type}: {e}")
            raise
        except Exception as e:
            logger.error(f"Error recording KPI {kpi_type}: {e}")
            raise

    async def get_kpi_status(
        self,
        kpi_type: str,
        use_cache: bool = True
    ) -> Optional[KpiSummary]:
        """Get current status and trends for a specific KPI.

        Args:
            kpi_type: Type of KPI to get status for
            use_cache: Whether to use cached data if available

        Returns:
            KPI summary with current status and trends
        """
        if not self.is_running:
            raise RuntimeError("BusinessValueTracker is not running")

        # Check cache first
        if use_cache and kpi_type in self.kpi_cache:
            cache_age = datetime.now(timezone.utc) - self.last_cache_update.get(kpi_type, datetime.min.replace(tzinfo=timezone.utc))
            if cache_age.total_seconds() < self.cache_ttl_seconds:
                return self.kpi_cache[kpi_type]

        try:
            async with self.session_factory() as session:
                # Get latest KPI values
                latest_query = select(BusinessKpi).where(
                    and_(
                        BusinessKpi.tenant_id == self.tenant_id,
                        BusinessKpi.kpi_type == kpi_type
                    )
                ).order_by(desc(BusinessKpi.timestamp)).limit(10)

                result = await session.execute(latest_query)
                kpi_records = result.scalars().all()

                if not kpi_records:
                    return None

                latest_record = kpi_records[0]

                # Calculate trends
                trend_direction, trend_percentage = self._calculate_trend(kpi_records)

                # Determine status
                status = self._determine_kpi_status(
                    latest_record.current_value,
                    latest_record.target_value,
                    kpi_type
                )

                # Calculate target achievement
                target_achievement = None
                if latest_record.target_value:
                    target_achievement = (latest_record.current_value / latest_record.target_value) * 100

                summary = KpiSummary(
                    kpi_type=kpi_type,
                    kpi_name=latest_record.kpi_name,
                    current_value=latest_record.current_value,
                    target_value=latest_record.target_value,
                    baseline_value=latest_record.baseline_value,
                    unit=latest_record.measurement_unit,
                    status=status,
                    trend_direction=trend_direction,
                    trend_percentage=trend_percentage,
                    target_achievement=target_achievement,
                    last_updated=latest_record.timestamp,
                    measurements_count=len(kpi_records)
                )

                # Cache the result
                self.kpi_cache[kpi_type] = summary
                self.last_cache_update[kpi_type] = datetime.now(timezone.utc)

                return summary

        except SQLAlchemyError as e:
            logger.error(f"Database error getting KPI status for {kpi_type}: {e}")
            raise
        except Exception as e:
            logger.error(f"Error getting KPI status for {kpi_type}: {e}")
            raise

    async def get_all_kpi_status(
        self,
        use_cache: bool = True
    ) -> Dict[str, KpiSummary]:
        """Get status for all tracked KPIs.

        Args:
            use_cache: Whether to use cached data if available

        Returns:
            Dictionary mapping KPI types to their summaries
        """
        kpi_types = [kpi_type.value for kpi_type in KpiType]
        summaries = {}

        for kpi_type in kpi_types:
            summary = await self.get_kpi_status(kpi_type, use_cache)
            if summary:
                summaries[kpi_type] = summary

        return summaries

    async def get_kpi_history(
        self,
        kpi_type: str,
        hours_back: int = 24,
        limit: int = 100
    ) -> List[KpiMeasurement]:
        """Get historical KPI measurements.

        Args:
            kpi_type: Type of KPI to get history for
            hours_back: How many hours of history to retrieve
            limit: Maximum number of measurements to return

        Returns:
            List of KPI measurements in chronological order
        """
        if not self.is_running:
            raise RuntimeError("BusinessValueTracker is not running")

        since_time = datetime.now(timezone.utc) - timedelta(hours=hours_back)

        try:
            async with self.session_factory() as session:
                query = select(BusinessKpi).where(
                    and_(
                        BusinessKpi.tenant_id == self.tenant_id,
                        BusinessKpi.kpi_type == kpi_type,
                        BusinessKpi.timestamp >= since_time
                    )
                ).order_by(asc(BusinessKpi.timestamp)).limit(limit)

                result = await session.execute(query)
                records = result.scalars().all()

                measurements = []
                for record in records:
                    measurement = KpiMeasurement(
                        kpi_type=record.kpi_type,
                        value=record.current_value,
                        unit=record.measurement_unit,
                        timestamp=record.timestamp,
                        context=record.measurement_context or {},
                        target=record.target_value,
                        baseline=record.baseline_value
                    )
                    measurements.append(measurement)

                return measurements

        except SQLAlchemyError as e:
            logger.error(f"Database error getting KPI history for {kpi_type}: {e}")
            raise
        except Exception as e:
            logger.error(f"Error getting KPI history for {kpi_type}: {e}")
            raise

    async def check_kpi_thresholds(self) -> List[BusinessAlert]:
        """Check all KPIs against their thresholds and generate alerts.

        Returns:
            List of active business alerts
        """
        if not self.is_running:
            raise RuntimeError("BusinessValueTracker is not running")

        alerts = []
        kpi_statuses = await self.get_all_kpi_status(use_cache=False)

        for kpi_type, summary in kpi_statuses.items():
            if summary.status == KpiStatus.CRITICAL:
                alert = BusinessAlert(
                    alert_id=str(uuid.uuid4()),
                    kpi_type=kpi_type,
                    severity="critical",
                    message=f"{summary.kpi_name} is critically below target",
                    current_value=summary.current_value,
                    threshold_value=summary.target_value or 0,
                    triggered_at=datetime.now(timezone.utc),
                    context={"status": summary.status.value, "trend": summary.trend_direction.value}
                )
                alerts.append(alert)
            elif summary.status == KpiStatus.BELOW_TARGET:
                alert = BusinessAlert(
                    alert_id=str(uuid.uuid4()),
                    kpi_type=kpi_type,
                    severity="warning",
                    message=f"{summary.kpi_name} is below target",
                    current_value=summary.current_value,
                    threshold_value=summary.target_value or 0,
                    triggered_at=datetime.now(timezone.utc),
                    context={"status": summary.status.value, "trend": summary.trend_direction.value}
                )
                alerts.append(alert)

        return alerts

    async def record_context_reconstruction_time(
        self,
        reconstruction_time_minutes: float,
        query_complexity: str,
        data_sources_count: int,
        context: Optional[Dict[str, Any]] = None
    ) -> str:
        """Record context reconstruction speed measurement.

        Args:
            reconstruction_time_minutes: Time taken to reconstruct context
            query_complexity: Complexity level (simple, medium, complex)
            data_sources_count: Number of data sources used
            context: Additional context information

        Returns:
            ID of the created KPI record
        """
        measurement_context = {
            "query_complexity": query_complexity,
            "data_sources_count": data_sources_count,
            **(context or {})
        }

        return await self.record_kpi(
            kpi_type="context_reconstruction_speed",
            value=reconstruction_time_minutes,
            context=measurement_context
        )

    async def record_search_accuracy(
        self,
        accuracy_percentage: float,
        total_results: int,
        relevant_results: int,
        search_type: str,
        context: Optional[Dict[str, Any]] = None
    ) -> str:
        """Record search accuracy measurement.

        Args:
            accuracy_percentage: Percentage of relevant results
            total_results: Total number of search results
            relevant_results: Number of relevant results
            search_type: Type of search performed
            context: Additional context information

        Returns:
            ID of the created KPI record
        """
        measurement_context = {
            "total_results": total_results,
            "relevant_results": relevant_results,
            "search_type": search_type,
            **(context or {})
        }

        return await self.record_kpi(
            kpi_type="search_accuracy",
            value=accuracy_percentage,
            context=measurement_context
        )

    async def record_relationship_validation(
        self,
        validation_rate_percentage: float,
        total_validations: int,
        successful_validations: int,
        validation_type: str,
        context: Optional[Dict[str, Any]] = None
    ) -> str:
        """Record relationship validation rate measurement.

        Args:
            validation_rate_percentage: Success rate of relationship validation
            total_validations: Total number of validation attempts
            successful_validations: Number of successful validations
            validation_type: Type of relationship validation
            context: Additional context information

        Returns:
            ID of the created KPI record
        """
        measurement_context = {
            "total_validations": total_validations,
            "successful_validations": successful_validations,
            "validation_type": validation_type,
            **(context or {})
        }

        return await self.record_kpi(
            kpi_type="relationship_validation_rate",
            value=validation_rate_percentage,
            context=measurement_context
        )

    # ========================================================================
    # Private Methods
    # ========================================================================

    async def _monitoring_loop(self) -> None:
        """Background monitoring loop for automated KPI collection."""
        while self.is_running:
            try:
                # Collect system-generated KPIs
                await self._collect_system_kpis()

                # Check for threshold violations
                alerts = await self.check_kpi_thresholds()
                if alerts:
                    logger.info(f"Generated {len(alerts)} business alerts")

            except asyncio.CancelledError:
                break
            except Exception as e:
                logger.error(f"Error in monitoring loop: {e}")

            # Wait for next iteration
            try:
                await asyncio.sleep(self.monitoring_interval)
            except asyncio.CancelledError:
                break

    async def _collect_system_kpis(self) -> None:
        """Collect KPIs from system metrics."""
        try:
            async with self.session_factory() as session:
                # Collect workflow efficiency from recent workflow events
                await self._collect_workflow_efficiency_kpi(session)

                # Collect data quality from decision traces
                await self._collect_data_quality_kpi(session)

        except Exception as e:
            logger.error(f"Error collecting system KPIs: {e}")

    async def _collect_workflow_efficiency_kpi(self, session: AsyncSession) -> None:
        """Collect workflow efficiency KPI from workflow events."""
        # Get recent workflow completion times
        since_time = datetime.now(timezone.utc) - timedelta(hours=1)

        query = select(WorkflowEvent).where(
            and_(
                WorkflowEvent.tenant_id == self.tenant_id,
                WorkflowEvent.event_type == "workflow_finished",
                WorkflowEvent.event_status == "success",
                WorkflowEvent.timestamp >= since_time,
                WorkflowEvent.processing_time_ms.isnot(None)
            )
        )

        result = await session.execute(query)
        events = result.scalars().all()

        if events:
            # Calculate average processing time improvement
            processing_times = [event.processing_time_ms / 1000 / 60 for event in events]  # Convert to minutes
            avg_time = statistics.mean(processing_times)

            # Assume baseline of 30 minutes per workflow (before Penfold)
            baseline_time = 30.0
            efficiency_improvement = ((baseline_time - avg_time) / baseline_time) * 100

            await self.record_kpi(
                kpi_type="workflow_efficiency",
                value=max(0, efficiency_improvement),
                context={
                    "avg_processing_time_minutes": avg_time,
                    "baseline_time_minutes": baseline_time,
                    "workflows_analyzed": len(events)
                }
            )

    async def _collect_data_quality_kpi(self, session: AsyncSession) -> None:
        """Collect data quality KPI from decision traces."""
        # Get recent decision traces
        since_time = datetime.now(timezone.utc) - timedelta(hours=1)

        query = select(DecisionTrace).where(
            and_(
                DecisionTrace.tenant_id == self.tenant_id,
                DecisionTrace.timestamp >= since_time
            )
        )

        result = await session.execute(query)
        traces = result.scalars().all()

        if traces:
            # Calculate average confidence score as data quality indicator
            confidence_scores = [trace.confidence_score for trace in traces]
            avg_confidence = statistics.mean(confidence_scores)
            data_quality_score = avg_confidence * 100  # Convert to percentage

            await self.record_kpi(
                kpi_type="data_quality_score",
                value=data_quality_score,
                context={
                    "avg_confidence": avg_confidence,
                    "decision_traces_analyzed": len(traces),
                    "confidence_std_dev": statistics.stdev(confidence_scores) if len(confidence_scores) > 1 else 0
                }
            )

    def _calculate_trend(self, kpi_records: List[BusinessKpi]) -> Tuple[TrendDirection, float]:
        """Calculate trend direction and percentage change."""
        if len(kpi_records) < 2:
            return TrendDirection.STABLE, 0.0

        # Use last 5 measurements for trend calculation
        recent_values = [record.current_value for record in kpi_records[:5]]

        if len(recent_values) < 2:
            return TrendDirection.STABLE, 0.0

        # Calculate linear trend
        x_values = list(range(len(recent_values)))
        mean_x = statistics.mean(x_values)
        mean_y = statistics.mean(recent_values)

        numerator = sum((x - mean_x) * (y - mean_y) for x, y in zip(x_values, recent_values))
        denominator = sum((x - mean_x) ** 2 for x in x_values)

        if denominator == 0:
            return TrendDirection.STABLE, 0.0

        slope = numerator / denominator

        # Calculate percentage change from first to last value
        first_value = recent_values[-1]  # Oldest (records are in desc order)
        last_value = recent_values[0]    # Newest

        if first_value == 0:
            percentage_change = 0.0
        else:
            percentage_change = ((last_value - first_value) / abs(first_value)) * 100

        # Determine trend direction
        if abs(percentage_change) < 2:
            direction = TrendDirection.STABLE
        elif slope > 0:
            direction = TrendDirection.IMPROVING
        else:
            direction = TrendDirection.DECLINING

        # Check for volatility
        if len(recent_values) >= 3:
            std_dev = statistics.stdev(recent_values)
            mean_val = statistics.mean(recent_values)
            coefficient_of_variation = (std_dev / mean_val) if mean_val != 0 else 0

            if coefficient_of_variation > 0.2:  # 20% CV indicates volatility
                direction = TrendDirection.VOLATILE

        return direction, percentage_change

    def _determine_kpi_status(
        self,
        current_value: float,
        target_value: Optional[float],
        kpi_type: str
    ) -> KpiStatus:
        """Determine KPI status based on current value and targets."""
        if target_value is None:
            return KpiStatus.MEETING  # No target to compare against

        kpi_config = PENFOLD_KPI_TARGETS.get(kpi_type, {})
        improvement_positive = kpi_config.get("improvement_positive", True)
        critical_threshold = kpi_config.get("critical_threshold")

        if improvement_positive:
            # Higher values are better
            if current_value >= target_value * 1.1:  # 110% of target
                return KpiStatus.EXCEEDING
            elif current_value >= target_value * 0.9:  # 90% of target
                return KpiStatus.MEETING
            elif critical_threshold and current_value < critical_threshold:
                return KpiStatus.CRITICAL
            else:
                return KpiStatus.BELOW_TARGET
        else:
            # Lower values are better
            if current_value <= target_value * 0.9:  # 90% of target
                return KpiStatus.EXCEEDING
            elif current_value <= target_value * 1.1:  # 110% of target
                return KpiStatus.MEETING
            elif critical_threshold and current_value > critical_threshold:
                return KpiStatus.CRITICAL
            else:
                return KpiStatus.BELOW_TARGET

    async def _check_kpi_thresholds(
        self,
        kpi_type: str,
        value: float,
        context: Dict[str, Any]
    ) -> None:
        """Check if KPI value violates thresholds and log alerts."""
        kpi_config = PENFOLD_KPI_TARGETS.get(kpi_type, {})
        target = kpi_config.get("target")
        critical_threshold = kpi_config.get("critical_threshold")
        improvement_positive = kpi_config.get("improvement_positive", True)

        if target is None:
            return

        # Check for threshold violations
        if improvement_positive:
            if critical_threshold and value < critical_threshold:
                logger.warning(
                    f"CRITICAL: {kpi_type} = {value} is below critical threshold {critical_threshold}"
                )
            elif value < target * 0.9:
                logger.warning(
                    f"WARNING: {kpi_type} = {value} is below target {target}"
                )
        else:
            if critical_threshold and value > critical_threshold:
                logger.warning(
                    f"CRITICAL: {kpi_type} = {value} is above critical threshold {critical_threshold}"
                )
            elif value > target * 1.1:
                logger.warning(
                    f"WARNING: {kpi_type} = {value} is above target {target}"
                )
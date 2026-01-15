"""ROI Calculator Service for measuring business value and return on investment.

This module implements the ROICalculatorService that quantifies the business value
and return on investment delivered by the Penfold system, including time savings,
productivity gains, and efficiency improvements.

Features:
- Comprehensive ROI calculation methodology
- Time savings measurement and validation
- Productivity gain quantification
- Decision speed improvement tracking
- Cost-benefit analysis and reporting
- Value calculation audit trails
- Multi-dimensional value assessment
- Confidence scoring for calculations
- Business impact forecasting

Key Value Measurements:
- Time savings: Minutes/hours saved through automation and efficiency
- Productivity gains: Percentage improvement in task completion
- Decision speed: Reduced time to gather context and make decisions
- Information retrieval efficiency: Faster access to relevant information
- Context switch reduction: Less time switching between tools/sources
- Meeting preparation: Time saved preparing for meetings with complete context

Calculation Methodologies:
- Baseline vs. current performance comparison
- Before/after Penfold implementation analysis
- Task-level time tracking and aggregation
- User-reported time savings validation
- System performance metrics correlation
- Industry benchmark comparison

Example:
    from observability_lib.services.roi_calculator import ROICalculatorService
    from observability_lib.config import settings

    calculator = ROICalculatorService(settings)
    await calculator.start()

    # Calculate time savings ROI
    roi_id = await calculator.calculate_time_savings_roi(
        period_days=30,
        baseline_minutes_per_task=45,
        current_minutes_per_task=12,
        tasks_per_day=8,
        hourly_rate=75.0
    )

    # Get ROI summary
    summary = await calculator.get_roi_summary(days_back=30)

    # Calculate meeting preparation savings
    savings = await calculator.calculate_meeting_preparation_savings(
        meetings_per_week=10,
        avg_prep_time_before=30,  # minutes
        avg_prep_time_after=5     # minutes
    )

    await calculator.stop()
"""

import asyncio
import logging
import time
import uuid
from collections import defaultdict, deque
from dataclasses import dataclass, asdict
from datetime import datetime, timedelta, timezone
from decimal import Decimal, ROUND_HALF_UP
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
    RoiMetric, ValueCalculation, BusinessKpi, UsageMetric,
    RoiMetricType, ValueCalculationType
)
from observability_lib.models.agent_metrics import (
    AgentMetric, WorkflowEvent, DecisionTrace
)
from penf_lib.storage.validators import ValidationError


logger = logging.getLogger(__name__)


# ============================================================================
# ROI Calculation Data Types and Constants
# ============================================================================

class ValueCategory(str, Enum):
    """Categories of business value"""
    TIME_SAVINGS = "time_savings"
    COST_REDUCTION = "cost_reduction"
    PRODUCTIVITY_IMPROVEMENT = "productivity_improvement"
    QUALITY_ENHANCEMENT = "quality_enhancement"
    DECISION_ACCELERATION = "decision_acceleration"
    EFFICIENCY_GAIN = "efficiency_gain"


class CalculationMethod(str, Enum):
    """Methods for ROI calculation"""
    BASELINE_COMPARISON = "baseline_comparison"
    BEFORE_AFTER_ANALYSIS = "before_after_analysis"
    TASK_TIME_TRACKING = "task_time_tracking"
    USER_REPORTED = "user_reported"
    SYSTEM_METRICS = "system_metrics"
    INDUSTRY_BENCHMARK = "industry_benchmark"


@dataclass
class ROICalculationInput:
    """Input data for ROI calculations"""
    calculation_name: str
    period_start: datetime
    period_end: datetime
    baseline_value: Optional[Decimal]
    current_value: Decimal
    cost_value: Optional[Decimal]
    methodology: CalculationMethod
    assumptions: Dict[str, Any]
    supporting_data: Dict[str, Any]
    confidence_level: float


@dataclass
class ROIResult:
    """Result of an ROI calculation"""
    roi_id: str
    calculation_id: str
    roi_type: str
    period: str
    gross_value: Decimal
    net_value: Decimal
    roi_percentage: Optional[float]
    payback_period_months: Optional[float]
    confidence_score: float
    value_unit: str
    calculation_date: datetime


@dataclass
class TimeSavingsCalculation:
    """Specific calculation for time savings"""
    task_type: str
    baseline_time_minutes: float
    current_time_minutes: float
    frequency_per_period: int
    period_days: int
    hourly_rate: Decimal
    total_time_saved_minutes: float
    total_value_saved: Decimal
    confidence: float


@dataclass
class ProductivityGainCalculation:
    """Specific calculation for productivity gains"""
    process_name: str
    baseline_completion_rate: float
    current_completion_rate: float
    tasks_per_period: int
    value_per_task: Decimal
    productivity_improvement: float
    additional_value: Decimal
    confidence: float


@dataclass
class ROISummary:
    """Comprehensive ROI summary"""
    period_start: datetime
    period_end: datetime
    total_gross_value: Decimal
    total_net_value: Decimal
    overall_roi_percentage: float
    time_savings_value: Decimal
    productivity_gains_value: Decimal
    efficiency_improvements_value: Decimal
    cost_reductions_value: Decimal
    payback_achieved: bool
    average_confidence: float
    calculation_count: int
    top_value_drivers: List[Tuple[str, Decimal]]


# Default assumptions and benchmarks for calculations
DEFAULT_ASSUMPTIONS = {
    "knowledge_worker_hourly_rate": Decimal("75.00"),
    "context_switching_cost_minutes": 23,  # Average time to refocus after interruption
    "meeting_prep_baseline_minutes": 30,
    "search_baseline_minutes": 15,
    "decision_baseline_days": 2,
    "information_retrieval_baseline_minutes": 45,
    "working_days_per_month": 22,
    "working_hours_per_day": 8
}

# Industry benchmarks for validation
INDUSTRY_BENCHMARKS = {
    "knowledge_work_efficiency_improvement": {
        "typical_range": (10, 30),  # 10-30% improvement
        "best_in_class": 40,
        "unit": "percentage"
    },
    "information_retrieval_time_reduction": {
        "typical_range": (20, 50),  # 20-50% reduction
        "best_in_class": 70,
        "unit": "percentage"
    },
    "decision_making_speed_improvement": {
        "typical_range": (15, 40),  # 15-40% faster
        "best_in_class": 60,
        "unit": "percentage"
    }
}


# ============================================================================
# ROI Calculator Service
# ============================================================================

class ROICalculatorService:
    """Service for calculating and tracking return on investment metrics.

    Provides comprehensive ROI analysis capabilities including time savings,
    productivity gains, and overall business value measurement.
    """

    def __init__(self, settings: ObservabilitySettings):
        """Initialize the ROI calculator service.

        Args:
            settings: Observability configuration settings
        """
        self.settings = settings
        self.engine = None
        self.session_factory = None
        self.is_running = False
        self.tenant_id = settings.DEFAULT_TENANT_ID

        # Calculation settings
        self.default_assumptions = DEFAULT_ASSUMPTIONS.copy()
        self.industry_benchmarks = INDUSTRY_BENCHMARKS.copy()
        self.confidence_threshold = 0.7  # Minimum confidence for calculations

        # Caching for performance
        self.roi_cache: Dict[str, ROISummary] = {}
        self.cache_ttl_seconds = 600  # 10 minutes
        self.last_cache_update: Dict[str, datetime] = {}

        # Background tasks
        self.calculation_task: Optional[asyncio.Task] = None
        self.calculation_interval = 3600  # 1 hour

    async def start(self) -> None:
        """Start the ROI calculator service."""
        if self.is_running:
            logger.warning("ROICalculatorService is already running")
            return

        logger.info("Starting ROICalculatorService")

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

            # Start background calculation task
            self.calculation_task = asyncio.create_task(self._calculation_loop())

            logger.info("ROICalculatorService started successfully")

        except Exception as e:
            logger.error(f"Failed to start ROICalculatorService: {e}")
            await self.stop()
            raise

    async def stop(self) -> None:
        """Stop the ROI calculator service."""
        if not self.is_running:
            return

        logger.info("Stopping ROICalculatorService")
        self.is_running = False

        # Cancel background tasks
        if self.calculation_task:
            self.calculation_task.cancel()
            try:
                await self.calculation_task
            except asyncio.CancelledError:
                pass

        # Close database connection
        if self.engine:
            await self.engine.dispose()

        # Clear caches
        self.roi_cache.clear()
        self.last_cache_update.clear()

        logger.info("ROICalculatorService stopped")

    async def calculate_time_savings_roi(
        self,
        task_type: str,
        baseline_time_minutes: float,
        current_time_minutes: float,
        frequency_per_day: int,
        period_days: int = 30,
        hourly_rate: Optional[Decimal] = None,
        assumptions: Optional[Dict[str, Any]] = None
    ) -> str:
        """Calculate ROI from time savings for a specific task type.

        Args:
            task_type: Type of task being measured
            baseline_time_minutes: Original time per task (before Penfold)
            current_time_minutes: Current time per task (with Penfold)
            frequency_per_day: How many times task is performed daily
            period_days: Period for calculation (default 30 days)
            hourly_rate: Hourly rate for value calculation
            assumptions: Additional assumptions for calculation

        Returns:
            ID of the created ROI metric record
        """
        if not self.is_running:
            raise RuntimeError("ROICalculatorService is not running")

        if hourly_rate is None:
            hourly_rate = self.default_assumptions["knowledge_worker_hourly_rate"]

        # Validate inputs
        if baseline_time_minutes <= 0 or current_time_minutes < 0:
            raise ValidationError("Time values must be positive")
        if frequency_per_day <= 0:
            raise ValidationError("Frequency must be positive")

        time_savings_per_task = baseline_time_minutes - current_time_minutes
        if time_savings_per_task <= 0:
            logger.warning(f"No time savings detected for {task_type}")
            time_savings_per_task = 0

        # Calculate total time and value savings
        total_tasks = frequency_per_day * period_days
        total_time_saved_minutes = time_savings_per_task * total_tasks
        total_time_saved_hours = total_time_saved_minutes / 60
        total_value_saved = Decimal(str(total_time_saved_hours)) * hourly_rate

        # Calculate confidence based on data quality
        confidence = self._calculate_confidence_score(
            has_baseline=True,
            sample_size=total_tasks,
            measurement_method="task_time_tracking",
            validation_available=False
        )

        period_start = datetime.now(timezone.utc) - timedelta(days=period_days)
        period_end = datetime.now(timezone.utc)

        # Prepare calculation data
        calculation_input = ROICalculationInput(
            calculation_name=f"Time Savings - {task_type}",
            period_start=period_start,
            period_end=period_end,
            baseline_value=Decimal(str(baseline_time_minutes * total_tasks / 60 * float(hourly_rate))),
            current_value=total_value_saved,
            cost_value=None,  # Time savings typically have no direct cost
            methodology=CalculationMethod.TASK_TIME_TRACKING,
            assumptions={
                "hourly_rate": float(hourly_rate),
                "baseline_time_minutes": baseline_time_minutes,
                "current_time_minutes": current_time_minutes,
                "frequency_per_day": frequency_per_day,
                "period_days": period_days,
                **(assumptions or {})
            },
            supporting_data={
                "task_type": task_type,
                "time_savings_per_task_minutes": time_savings_per_task,
                "total_tasks": total_tasks,
                "total_time_saved_minutes": total_time_saved_minutes,
                "efficiency_improvement_percentage": (time_savings_per_task / baseline_time_minutes) * 100
            },
            confidence_level=confidence
        )

        # Create ROI and value calculation records
        roi_id = await self._create_roi_record(
            roi_type=RoiMetricType.TIME_SAVINGS_MINUTES.value,
            value_amount=total_value_saved,
            value_unit="dollars",
            calculation_input=calculation_input
        )

        logger.info(f"Calculated time savings ROI: ${total_value_saved:.2f} for {task_type}")
        return roi_id

    async def calculate_productivity_gain_roi(
        self,
        process_name: str,
        baseline_completion_rate: float,
        current_completion_rate: float,
        tasks_per_day: int,
        value_per_task: Decimal,
        period_days: int = 30,
        assumptions: Optional[Dict[str, Any]] = None
    ) -> str:
        """Calculate ROI from productivity improvements.

        Args:
            process_name: Name of the process being measured
            baseline_completion_rate: Original task completion rate (0.0-1.0)
            current_completion_rate: Current completion rate (0.0-1.0)
            tasks_per_day: Number of tasks attempted daily
            value_per_task: Business value per completed task
            period_days: Period for calculation
            assumptions: Additional assumptions

        Returns:
            ID of the created ROI metric record
        """
        if not self.is_running:
            raise RuntimeError("ROICalculatorService is not running")

        # Validate inputs
        if not (0 <= baseline_completion_rate <= 1):
            raise ValidationError("Baseline completion rate must be between 0 and 1")
        if not (0 <= current_completion_rate <= 1):
            raise ValidationError("Current completion rate must be between 0 and 1")
        if tasks_per_day <= 0:
            raise ValidationError("Tasks per day must be positive")
        if value_per_task <= 0:
            raise ValidationError("Value per task must be positive")

        # Calculate productivity improvement
        productivity_improvement = current_completion_rate - baseline_completion_rate
        improvement_percentage = (productivity_improvement / baseline_completion_rate * 100) if baseline_completion_rate > 0 else 0

        # Calculate additional value generated
        total_attempted_tasks = tasks_per_day * period_days
        baseline_completed_tasks = total_attempted_tasks * baseline_completion_rate
        current_completed_tasks = total_attempted_tasks * current_completion_rate
        additional_completed_tasks = current_completed_tasks - baseline_completed_tasks

        additional_value = Decimal(str(additional_completed_tasks)) * value_per_task

        # Calculate confidence
        confidence = self._calculate_confidence_score(
            has_baseline=True,
            sample_size=total_attempted_tasks,
            measurement_method="before_after_analysis",
            validation_available=False
        )

        period_start = datetime.now(timezone.utc) - timedelta(days=period_days)
        period_end = datetime.now(timezone.utc)

        calculation_input = ROICalculationInput(
            calculation_name=f"Productivity Gain - {process_name}",
            period_start=period_start,
            period_end=period_end,
            baseline_value=Decimal(str(baseline_completed_tasks)) * value_per_task,
            current_value=additional_value,
            cost_value=None,
            methodology=CalculationMethod.BEFORE_AFTER_ANALYSIS,
            assumptions={
                "baseline_completion_rate": baseline_completion_rate,
                "current_completion_rate": current_completion_rate,
                "tasks_per_day": tasks_per_day,
                "value_per_task": float(value_per_task),
                "period_days": period_days,
                **(assumptions or {})
            },
            supporting_data={
                "process_name": process_name,
                "productivity_improvement": productivity_improvement,
                "improvement_percentage": improvement_percentage,
                "additional_completed_tasks": additional_completed_tasks,
                "total_attempted_tasks": total_attempted_tasks
            },
            confidence_level=confidence
        )

        roi_id = await self._create_roi_record(
            roi_type=RoiMetricType.PRODUCTIVITY_GAIN_PERCENTAGE.value,
            value_amount=additional_value,
            value_unit="dollars",
            calculation_input=calculation_input
        )

        logger.info(f"Calculated productivity gain ROI: ${additional_value:.2f} for {process_name}")
        return roi_id

    async def calculate_decision_speed_improvement_roi(
        self,
        decision_type: str,
        baseline_days: float,
        current_days: float,
        decisions_per_month: int,
        cost_of_delay_per_day: Decimal,
        period_months: int = 1,
        assumptions: Optional[Dict[str, Any]] = None
    ) -> str:
        """Calculate ROI from faster decision making.

        Args:
            decision_type: Type of decision being analyzed
            baseline_days: Original time to make decision
            current_days: Current time to make decision
            decisions_per_month: Number of decisions per month
            cost_of_delay_per_day: Cost of each day of delay
            period_months: Period for calculation
            assumptions: Additional assumptions

        Returns:
            ID of the created ROI metric record
        """
        if not self.is_running:
            raise RuntimeError("ROICalculatorService is not running")

        # Validate inputs
        if baseline_days <= 0 or current_days < 0:
            raise ValidationError("Decision time values must be positive")
        if decisions_per_month <= 0:
            raise ValidationError("Decisions per month must be positive")

        time_savings_per_decision = baseline_days - current_days
        if time_savings_per_decision <= 0:
            logger.warning(f"No decision speed improvement detected for {decision_type}")
            time_savings_per_decision = 0

        # Calculate total value from faster decisions
        total_decisions = decisions_per_month * period_months
        total_time_saved_days = time_savings_per_decision * total_decisions
        total_value_saved = Decimal(str(total_time_saved_days)) * cost_of_delay_per_day

        improvement_percentage = (time_savings_per_decision / baseline_days * 100) if baseline_days > 0 else 0

        confidence = self._calculate_confidence_score(
            has_baseline=True,
            sample_size=total_decisions,
            measurement_method="baseline_comparison",
            validation_available=True
        )

        period_start = datetime.now(timezone.utc) - timedelta(days=period_months * 30)
        period_end = datetime.now(timezone.utc)

        calculation_input = ROICalculationInput(
            calculation_name=f"Decision Speed - {decision_type}",
            period_start=period_start,
            period_end=period_end,
            baseline_value=Decimal(str(baseline_days * total_decisions)) * cost_of_delay_per_day,
            current_value=total_value_saved,
            cost_value=None,
            methodology=CalculationMethod.BASELINE_COMPARISON,
            assumptions={
                "baseline_days": baseline_days,
                "current_days": current_days,
                "decisions_per_month": decisions_per_month,
                "cost_of_delay_per_day": float(cost_of_delay_per_day),
                "period_months": period_months,
                **(assumptions or {})
            },
            supporting_data={
                "decision_type": decision_type,
                "time_savings_per_decision_days": time_savings_per_decision,
                "total_decisions": total_decisions,
                "improvement_percentage": improvement_percentage,
                "total_time_saved_days": total_time_saved_days
            },
            confidence_level=confidence
        )

        roi_id = await self._create_roi_record(
            roi_type=RoiMetricType.DECISION_SPEED_IMPROVEMENT.value,
            value_amount=total_value_saved,
            value_unit="dollars",
            calculation_input=calculation_input
        )

        logger.info(f"Calculated decision speed ROI: ${total_value_saved:.2f} for {decision_type}")
        return roi_id

    async def calculate_meeting_preparation_savings(
        self,
        meetings_per_week: int,
        baseline_prep_minutes: float,
        current_prep_minutes: float,
        hourly_rate: Optional[Decimal] = None,
        period_weeks: int = 4,
        assumptions: Optional[Dict[str, Any]] = None
    ) -> str:
        """Calculate ROI from reduced meeting preparation time.

        Args:
            meetings_per_week: Number of meetings requiring preparation weekly
            baseline_prep_minutes: Original preparation time per meeting
            current_prep_minutes: Current preparation time per meeting
            hourly_rate: Hourly rate for value calculation
            period_weeks: Period for calculation
            assumptions: Additional assumptions

        Returns:
            ID of the created ROI metric record
        """
        if hourly_rate is None:
            hourly_rate = self.default_assumptions["knowledge_worker_hourly_rate"]

        prep_time_savings = baseline_prep_minutes - current_prep_minutes
        if prep_time_savings <= 0:
            logger.warning("No meeting preparation time savings detected")
            prep_time_savings = 0

        total_meetings = meetings_per_week * period_weeks
        total_time_saved_minutes = prep_time_savings * total_meetings
        total_time_saved_hours = total_time_saved_minutes / 60
        total_value_saved = Decimal(str(total_time_saved_hours)) * hourly_rate

        return await self.calculate_time_savings_roi(
            task_type="meeting_preparation",
            baseline_time_minutes=baseline_prep_minutes,
            current_time_minutes=current_prep_minutes,
            frequency_per_day=meetings_per_week / 7,  # Convert to daily frequency
            period_days=period_weeks * 7,
            hourly_rate=hourly_rate,
            assumptions={
                "context": "meeting_preparation_savings",
                "meetings_per_week": meetings_per_week,
                "period_weeks": period_weeks,
                **(assumptions or {})
            }
        )

    async def get_roi_summary(
        self,
        days_back: int = 30,
        use_cache: bool = True
    ) -> ROISummary:
        """Get comprehensive ROI summary for a time period.

        Args:
            days_back: Number of days to include in summary
            use_cache: Whether to use cached data if available

        Returns:
            Comprehensive ROI summary
        """
        if not self.is_running:
            raise RuntimeError("ROICalculatorService is not running")

        cache_key = f"roi_summary_{days_back}d"

        # Check cache first
        if use_cache and cache_key in self.roi_cache:
            cache_age = datetime.now(timezone.utc) - self.last_cache_update.get(cache_key, datetime.min.replace(tzinfo=timezone.utc))
            if cache_age.total_seconds() < self.cache_ttl_seconds:
                return self.roi_cache[cache_key]

        try:
            async with self.session_factory() as session:
                period_start = datetime.now(timezone.utc) - timedelta(days=days_back)
                period_end = datetime.now(timezone.utc)

                # Get all ROI metrics for the period
                roi_query = select(RoiMetric).where(
                    and_(
                        RoiMetric.tenant_id == self.tenant_id,
                        RoiMetric.timestamp >= period_start
                    )
                )

                result = await session.execute(roi_query)
                roi_metrics = result.scalars().all()

                if not roi_metrics:
                    # Return empty summary
                    return ROISummary(
                        period_start=period_start,
                        period_end=period_end,
                        total_gross_value=Decimal("0"),
                        total_net_value=Decimal("0"),
                        overall_roi_percentage=0.0,
                        time_savings_value=Decimal("0"),
                        productivity_gains_value=Decimal("0"),
                        efficiency_improvements_value=Decimal("0"),
                        cost_reductions_value=Decimal("0"),
                        payback_achieved=False,
                        average_confidence=0.0,
                        calculation_count=0,
                        top_value_drivers=[]
                    )

                # Calculate summary statistics
                total_gross_value = sum(metric.value_amount for metric in roi_metrics)
                total_net_value = sum(metric.net_value for metric in roi_metrics)

                # Calculate overall ROI percentage
                total_cost = sum(metric.cost_amount or Decimal("0") for metric in roi_metrics)
                overall_roi_percentage = float((total_net_value / total_cost * 100)) if total_cost > 0 else 0.0

                # Break down by value type
                value_by_type = defaultdict(lambda: Decimal("0"))
                confidence_scores = []

                for metric in roi_metrics:
                    confidence_scores.append(metric.confidence_level)

                    if "time_savings" in metric.roi_type:
                        value_by_type["time_savings"] += metric.value_amount
                    elif "productivity_gain" in metric.roi_type:
                        value_by_type["productivity_gains"] += metric.value_amount
                    elif "efficiency" in metric.roi_type:
                        value_by_type["efficiency_improvements"] += metric.value_amount
                    elif "cost" in metric.roi_type:
                        value_by_type["cost_reductions"] += metric.value_amount

                # Find top value drivers
                roi_by_type = defaultdict(lambda: Decimal("0"))
                for metric in roi_metrics:
                    roi_by_type[metric.roi_type] += metric.value_amount

                top_value_drivers = sorted(roi_by_type.items(), key=lambda x: x[1], reverse=True)[:5]

                # Calculate average confidence
                average_confidence = statistics.mean(confidence_scores) if confidence_scores else 0.0

                # Determine if payback achieved (simplified)
                payback_achieved = total_net_value > 0 and overall_roi_percentage > 0

                summary = ROISummary(
                    period_start=period_start,
                    period_end=period_end,
                    total_gross_value=total_gross_value,
                    total_net_value=total_net_value,
                    overall_roi_percentage=overall_roi_percentage,
                    time_savings_value=value_by_type["time_savings"],
                    productivity_gains_value=value_by_type["productivity_gains"],
                    efficiency_improvements_value=value_by_type["efficiency_improvements"],
                    cost_reductions_value=value_by_type["cost_reductions"],
                    payback_achieved=payback_achieved,
                    average_confidence=average_confidence,
                    calculation_count=len(roi_metrics),
                    top_value_drivers=top_value_drivers
                )

                # Cache the summary
                self.roi_cache[cache_key] = summary
                self.last_cache_update[cache_key] = datetime.now(timezone.utc)

                return summary

        except SQLAlchemyError as e:
            logger.error(f"Database error getting ROI summary: {e}")
            raise
        except Exception as e:
            logger.error(f"Error getting ROI summary: {e}")
            raise

    # ========================================================================
    # Private Methods
    # ========================================================================

    async def _calculation_loop(self) -> None:
        """Background loop for automated ROI calculations."""
        while self.is_running:
            try:
                # Perform automated calculations based on system metrics
                await self._calculate_system_generated_roi()

            except asyncio.CancelledError:
                break
            except Exception as e:
                logger.error(f"Error in calculation loop: {e}")

            try:
                await asyncio.sleep(self.calculation_interval)
            except asyncio.CancelledError:
                break

    async def _calculate_system_generated_roi(self) -> None:
        """Calculate ROI based on automatically collected system metrics."""
        try:
            # Calculate context reconstruction ROI
            await self._calculate_context_reconstruction_roi()

            # Calculate search efficiency ROI
            await self._calculate_search_efficiency_roi()

            # Calculate workflow automation ROI
            await self._calculate_workflow_automation_roi()

        except Exception as e:
            logger.error(f"Error calculating system-generated ROI: {e}")

    async def _calculate_context_reconstruction_roi(self) -> None:
        """Calculate ROI from context reconstruction time savings."""
        try:
            async with self.session_factory() as session:
                # Get recent context reconstruction KPI data
                since_time = datetime.now(timezone.utc) - timedelta(hours=24)

                query = select(BusinessKpi).where(
                    and_(
                        BusinessKpi.tenant_id == self.tenant_id,
                        BusinessKpi.kpi_type == "context_reconstruction_speed",
                        BusinessKpi.timestamp >= since_time
                    )
                ).order_by(desc(BusinessKpi.timestamp))

                result = await session.execute(query)
                kpi_records = result.scalars().all()

                if not kpi_records:
                    return

                # Calculate average reconstruction time
                avg_current_time = statistics.mean([record.current_value for record in kpi_records])
                baseline_time = 30.0  # Assume 30 minutes baseline (manual process)

                if avg_current_time < baseline_time:
                    # Calculate daily ROI from context reconstruction savings
                    await self.calculate_time_savings_roi(
                        task_type="context_reconstruction",
                        baseline_time_minutes=baseline_time,
                        current_time_minutes=avg_current_time,
                        frequency_per_day=3,  # Assume 3 context reconstructions per day
                        period_days=1,
                        assumptions={
                            "automated_calculation": True,
                            "source": "system_kpi_data",
                            "kpi_records_count": len(kpi_records)
                        }
                    )

        except Exception as e:
            logger.error(f"Error calculating context reconstruction ROI: {e}")

    async def _calculate_search_efficiency_roi(self) -> None:
        """Calculate ROI from improved search efficiency."""
        try:
            async with self.session_factory() as session:
                # Get recent search usage metrics
                since_time = datetime.now(timezone.utc) - timedelta(hours=24)

                query = select(UsageMetric).where(
                    and_(
                        UsageMetric.tenant_id == self.tenant_id,
                        UsageMetric.feature_name == "search_feature",
                        UsageMetric.timestamp >= since_time,
                        UsageMetric.duration_seconds.isnot(None)
                    )
                )

                result = await session.execute(query)
                usage_metrics = result.scalars().all()

                if not usage_metrics:
                    return

                # Calculate average search time
                search_times = [metric.duration_seconds for metric in usage_metrics if metric.duration_seconds]
                if not search_times:
                    return

                avg_search_time_seconds = statistics.mean(search_times)
                avg_search_time_minutes = avg_search_time_seconds / 60

                baseline_search_minutes = 15.0  # Assume 15 minutes baseline for manual search
                searches_per_day = len(usage_metrics)  # Approximate daily search count

                if avg_search_time_minutes < baseline_search_minutes:
                    await self.calculate_time_savings_roi(
                        task_type="information_search",
                        baseline_time_minutes=baseline_search_minutes,
                        current_time_minutes=avg_search_time_minutes,
                        frequency_per_day=searches_per_day,
                        period_days=1,
                        assumptions={
                            "automated_calculation": True,
                            "source": "usage_metrics",
                            "search_count": len(usage_metrics)
                        }
                    )

        except Exception as e:
            logger.error(f"Error calculating search efficiency ROI: {e}")

    async def _calculate_workflow_automation_roi(self) -> None:
        """Calculate ROI from workflow automation."""
        try:
            async with self.session_factory() as session:
                # Get recent workflow events
                since_time = datetime.now(timezone.utc) - timedelta(hours=24)

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

                if not events:
                    return

                # Calculate average workflow processing time
                processing_times_minutes = [event.processing_time_ms / 1000 / 60 for event in events]
                avg_processing_time = statistics.mean(processing_times_minutes)

                # Assume baseline manual processing time of 60 minutes per workflow
                baseline_time_minutes = 60.0
                workflows_per_day = len(events)

                if avg_processing_time < baseline_time_minutes:
                    await self.calculate_time_savings_roi(
                        task_type="workflow_automation",
                        baseline_time_minutes=baseline_time_minutes,
                        current_time_minutes=avg_processing_time,
                        frequency_per_day=workflows_per_day,
                        period_days=1,
                        assumptions={
                            "automated_calculation": True,
                            "source": "workflow_events",
                            "workflow_count": len(events)
                        }
                    )

        except Exception as e:
            logger.error(f"Error calculating workflow automation ROI: {e}")

    async def _create_roi_record(
        self,
        roi_type: str,
        value_amount: Decimal,
        value_unit: str,
        calculation_input: ROICalculationInput,
        cost_amount: Optional[Decimal] = None
    ) -> str:
        """Create ROI metric and value calculation records.

        Args:
            roi_type: Type of ROI metric
            value_amount: Calculated value amount
            value_unit: Unit of the value
            calculation_input: Input data for the calculation
            cost_amount: Associated cost (optional)

        Returns:
            ID of the created ROI metric record
        """
        try:
            async with self.session_factory() as session:
                # Determine measurement period
                period_delta = calculation_input.period_end - calculation_input.period_start
                if period_delta.days <= 1:
                    measurement_period = "daily"
                elif period_delta.days <= 7:
                    measurement_period = "weekly"
                elif period_delta.days <= 31:
                    measurement_period = "monthly"
                else:
                    measurement_period = "quarterly"

                # Create ROI metric record
                roi_metric = RoiMetric(
                    tenant_id=self.tenant_id,
                    roi_type=roi_type,
                    measurement_period=measurement_period,
                    value_amount=value_amount,
                    value_unit=value_unit,
                    baseline_amount=calculation_input.baseline_value,
                    cost_amount=cost_amount,
                    confidence_level=calculation_input.confidence_level,
                    calculation_method=calculation_input.methodology.value,
                    supporting_data=calculation_input.supporting_data
                )

                session.add(roi_metric)
                await session.commit()
                await session.refresh(roi_metric)

                # Create detailed value calculation record
                value_calc = ValueCalculation(
                    tenant_id=self.tenant_id,
                    calculation_name=calculation_input.calculation_name,
                    calculation_type=self._get_value_calculation_type(roi_type),
                    time_period_start=calculation_input.period_start,
                    time_period_end=calculation_input.period_end,
                    calculated_value=value_amount,
                    value_unit=value_unit,
                    methodology=self._get_methodology_description(calculation_input.methodology),
                    assumptions=calculation_input.assumptions,
                    input_data=calculation_input.supporting_data,
                    calculation_steps=self._generate_calculation_steps(calculation_input),
                    confidence_score=calculation_input.confidence_level,
                    related_roi_metric_id=roi_metric.id
                )

                session.add(value_calc)
                await session.commit()

                # Invalidate ROI cache
                self.roi_cache.clear()
                self.last_cache_update.clear()

                return str(roi_metric.id)

        except SQLAlchemyError as e:
            logger.error(f"Database error creating ROI record: {e}")
            raise
        except Exception as e:
            logger.error(f"Error creating ROI record: {e}")
            raise

    def _calculate_confidence_score(
        self,
        has_baseline: bool,
        sample_size: int,
        measurement_method: str,
        validation_available: bool
    ) -> float:
        """Calculate confidence score for ROI calculation."""
        confidence = 0.5  # Base confidence

        # Baseline data availability
        if has_baseline:
            confidence += 0.2

        # Sample size factor
        if sample_size > 100:
            confidence += 0.15
        elif sample_size > 10:
            confidence += 0.1
        elif sample_size > 1:
            confidence += 0.05

        # Measurement method reliability
        method_scores = {
            "task_time_tracking": 0.2,
            "system_metrics": 0.15,
            "before_after_analysis": 0.15,
            "baseline_comparison": 0.1,
            "user_reported": 0.05,
            "industry_benchmark": 0.1
        }
        confidence += method_scores.get(measurement_method, 0.05)

        # External validation
        if validation_available:
            confidence += 0.1

        return min(confidence, 1.0)  # Cap at 1.0

    def _get_value_calculation_type(self, roi_type: str) -> str:
        """Map ROI type to value calculation type."""
        if "time_savings" in roi_type:
            return ValueCalculationType.TIME_SAVINGS.value
        elif "productivity" in roi_type:
            return ValueCalculationType.PRODUCTIVITY_INCREASE.value
        elif "decision" in roi_type:
            return ValueCalculationType.DECISION_ACCELERATION.value
        elif "cost" in roi_type:
            return ValueCalculationType.COST_REDUCTION.value
        else:
            return ValueCalculationType.TIME_SAVINGS.value

    def _get_methodology_description(self, method: CalculationMethod) -> str:
        """Get human-readable methodology description."""
        descriptions = {
            CalculationMethod.BASELINE_COMPARISON: "Comparison against pre-implementation baseline measurements",
            CalculationMethod.BEFORE_AFTER_ANALYSIS: "Before and after implementation performance analysis",
            CalculationMethod.TASK_TIME_TRACKING: "Direct measurement of task completion times",
            CalculationMethod.USER_REPORTED: "User-reported time savings and efficiency improvements",
            CalculationMethod.SYSTEM_METRICS: "System-generated performance metrics analysis",
            CalculationMethod.INDUSTRY_BENCHMARK: "Comparison against industry standard benchmarks"
        }
        return descriptions.get(method, "Custom calculation methodology")

    def _generate_calculation_steps(self, calculation_input: ROICalculationInput) -> Dict[str, Any]:
        """Generate detailed calculation steps for audit trail."""
        steps = {
            "step_1": {
                "description": "Identify baseline and current values",
                "baseline_value": float(calculation_input.baseline_value) if calculation_input.baseline_value else None,
                "current_value": float(calculation_input.current_value),
                "improvement": float(calculation_input.current_value - (calculation_input.baseline_value or Decimal("0")))
            },
            "step_2": {
                "description": "Apply calculation methodology",
                "methodology": calculation_input.methodology.value,
                "period": f"{calculation_input.period_start.isoformat()} to {calculation_input.period_end.isoformat()}"
            },
            "step_3": {
                "description": "Apply assumptions and adjustments",
                "assumptions_applied": calculation_input.assumptions
            },
            "step_4": {
                "description": "Calculate final ROI value",
                "final_value": float(calculation_input.current_value),
                "value_unit": "dollars",
                "confidence_score": calculation_input.confidence_level
            }
        }
        return steps
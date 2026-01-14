"""Model performance tracking and learning system.

Implements User Story 4: Model Performance Learning
- Tracks model performance across different content types
- Learns optimal model selection strategies
- Adapts coordination based on historical data
- Provides performance analytics and recommendations
"""

import logging
import statistics
from datetime import datetime, timezone, timedelta
from typing import Dict, List, Optional, Any, Tuple
from dataclasses import dataclass

from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select, func, desc
from sqlalchemy.exc import SQLAlchemyError

logger = logging.getLogger(__name__)


@dataclass
class ModelPerformance:
    """Performance metrics for a specific model and content type."""
    model_id: str
    content_type: str
    total_processed: int
    success_count: int
    avg_confidence: float
    avg_processing_time_ms: Optional[float]
    confidence_variance: float
    success_rate: float
    last_updated: datetime
    performance_score: float  # Composite score for model selection
    trend_direction: str  # "improving", "declining", "stable"
    sample_size_confidence: float  # How confident we are in these metrics


class PerformanceTracker:
    """Tracks and analyzes AI model performance over time."""

    def __init__(self, session: AsyncSession):
        self.session = session
        self.performance_cache: Dict[str, ModelPerformance] = {}
        self.cache_expiry = timedelta(hours=1)
        self.last_cache_update = datetime.min

    async def record_performance(
        self,
        model_id: str,
        content_type: str,
        processing_time: Optional[float],
        confidence_score: Optional[float],
        success: bool,
        additional_metrics: Optional[Dict[str, Any]] = None
    ) -> None:
        """Record performance data for a model processing session.

        Args:
            model_id: Identifier of the model
            content_type: Type of content processed
            processing_time: Time taken in milliseconds (if successful)
            confidence_score: Model's confidence in result (if successful)
            success: Whether processing completed successfully
            additional_metrics: Optional additional performance data
        """
        try:
            from ..storage.models import ModelPerformanceRecord

            record = ModelPerformanceRecord(
                model_id=model_id,
                content_type=content_type,
                processing_time_ms=processing_time,
                confidence_score=confidence_score,
                success=success,
                additional_metrics=additional_metrics or {},
                timestamp=datetime.now(timezone.utc)
            )

            self.session.add(record)
            await self.session.commit()

            # Invalidate cache for this model/content type
            cache_key = f"{model_id}:{content_type}"
            if cache_key in self.performance_cache:
                del self.performance_cache[cache_key]

            logger.debug(
                f"Recorded performance for {model_id} on {content_type}: "
                f"success={success}, confidence={confidence_score}, time={processing_time}ms"
            )

        except Exception as e:
            logger.error(f"Failed to record performance for {model_id}: {e}")
            await self.session.rollback()

    async def get_model_performance(
        self,
        model_id: str,
        content_type: str,
        days_back: int = 30
    ) -> Optional[ModelPerformance]:
        """Get performance metrics for a specific model and content type.

        Args:
            model_id: Model identifier
            content_type: Content type
            days_back: Number of days of history to analyze

        Returns:
            ModelPerformance object or None if insufficient data
        """
        cache_key = f"{model_id}:{content_type}"

        # Check cache first
        if (cache_key in self.performance_cache and
            datetime.now(timezone.utc) - self.last_cache_update < self.cache_expiry):
            return self.performance_cache[cache_key]

        try:
            from ..storage.models import ModelPerformanceRecord

            cutoff_date = datetime.now(timezone.utc) - timedelta(days=days_back)

            # Query performance records
            query = (
                select(ModelPerformanceRecord)
                .where(
                    ModelPerformanceRecord.model_id == model_id,
                    ModelPerformanceRecord.content_type == content_type,
                    ModelPerformanceRecord.timestamp >= cutoff_date
                )
                .order_by(desc(ModelPerformanceRecord.timestamp))
                .limit(1000)  # Limit to prevent memory issues
            )

            result = await self.session.execute(query)
            records = result.scalars().all()

            if len(records) < 5:  # Need minimum samples for meaningful metrics
                return None

            # Calculate performance metrics
            performance = await self._calculate_performance_metrics(model_id, content_type, records)

            # Cache the result
            self.performance_cache[cache_key] = performance
            self.last_cache_update = datetime.now(timezone.utc)

            return performance

        except Exception as e:
            logger.error(f"Failed to get performance for {model_id} on {content_type}: {e}")
            return None

    async def optimize_model_selection(
        self,
        content_type: str,
        available_models: List[str],
        max_models: int = 5,
        diversity_weight: float = 0.2
    ) -> List[str]:
        """Select optimal models for content processing based on performance.

        Args:
            content_type: Type of content to process
            available_models: List of available model IDs
            max_models: Maximum number of models to select
            diversity_weight: Weight for model diversity (vs pure performance)

        Returns:
            List of selected model IDs ordered by preference
        """
        if len(available_models) <= max_models:
            return available_models

        # Get performance data for all available models
        model_performances = {}
        for model_id in available_models:
            performance = await self.get_model_performance(model_id, content_type)
            if performance:
                model_performances[model_id] = performance

        if not model_performances:
            # No performance data - use all available models
            return available_models[:max_models]

        # Calculate selection scores combining performance and diversity
        selection_scores = []

        for model_id, performance in model_performances.items():
            # Base score from performance
            performance_score = performance.performance_score

            # Adjust for sample size confidence
            confidence_adjusted_score = performance_score * performance.sample_size_confidence

            # Add diversity bonus (prefer models with different capabilities)
            diversity_score = self._calculate_diversity_score(
                model_id, list(model_performances.keys())
            )

            final_score = (
                confidence_adjusted_score * (1 - diversity_weight) +
                diversity_score * diversity_weight
            )

            selection_scores.append((model_id, final_score, performance))

        # Sort by final score and select top models
        selection_scores.sort(key=lambda x: x[1], reverse=True)
        selected_models = [model_id for model_id, _, _ in selection_scores[:max_models]]

        logger.info(
            f"Selected {len(selected_models)} models for {content_type}: "
            f"{[(m, f'{s:.3f}') for m, s, _ in selection_scores[:max_models]]}"
        )

        return selected_models

    async def get_performance_trends(
        self,
        model_ids: Optional[List[str]] = None,
        content_types: Optional[List[str]] = None,
        days_back: int = 30
    ) -> Dict[str, Any]:
        """Analyze performance trends across models and content types.

        Args:
            model_ids: Specific models to analyze (None for all)
            content_types: Specific content types (None for all)
            days_back: Days of history to analyze

        Returns:
            Performance trend analysis
        """
        try:
            from ..storage.models import ModelPerformanceRecord

            cutoff_date = datetime.now(timezone.utc) - timedelta(days=days_back)

            # Build query
            query = select(ModelPerformanceRecord).where(
                ModelPerformanceRecord.timestamp >= cutoff_date
            )

            if model_ids:
                query = query.where(ModelPerformanceRecord.model_id.in_(model_ids))
            if content_types:
                query = query.where(ModelPerformanceRecord.content_type.in_(content_types))

            result = await self.session.execute(query)
            records = result.scalars().all()

            if not records:
                return {"error": "No performance data available"}

            # Analyze trends by model
            model_trends = {}
            for model_id in set(r.model_id for r in records):
                model_records = [r for r in records if r.model_id == model_id]
                model_trends[model_id] = await self._analyze_model_trend(model_records)

            # Analyze trends by content type
            content_trends = {}
            for content_type in set(r.content_type for r in records):
                content_records = [r for r in records if r.content_type == content_type]
                content_trends[content_type] = await self._analyze_content_trend(content_records)

            # Overall system trends
            system_trends = await self._analyze_system_trends(records)

            return {
                "analysis_period": {
                    "start_date": cutoff_date.isoformat(),
                    "end_date": datetime.now(timezone.utc).isoformat(),
                    "total_records": len(records)
                },
                "model_trends": model_trends,
                "content_type_trends": content_trends,
                "system_trends": system_trends,
                "recommendations": await self._generate_recommendations(model_trends, content_trends)
            }

        except Exception as e:
            logger.error(f"Failed to analyze performance trends: {e}")
            return {"error": str(e)}

    async def _calculate_performance_metrics(
        self,
        model_id: str,
        content_type: str,
        records: List[Any]
    ) -> ModelPerformance:
        """Calculate performance metrics from raw records."""
        total_processed = len(records)
        successful_records = [r for r in records if r.success]
        success_count = len(successful_records)
        success_rate = success_count / total_processed if total_processed > 0 else 0.0

        # Calculate confidence metrics
        confidence_scores = [r.confidence_score for r in successful_records if r.confidence_score is not None]
        avg_confidence = statistics.mean(confidence_scores) if confidence_scores else 0.0
        confidence_variance = statistics.variance(confidence_scores) if len(confidence_scores) > 1 else 0.0

        # Calculate processing time metrics
        processing_times = [r.processing_time_ms for r in successful_records if r.processing_time_ms is not None]
        avg_processing_time = statistics.mean(processing_times) if processing_times else None

        # Calculate composite performance score
        performance_score = self._calculate_composite_score(
            success_rate, avg_confidence, confidence_variance, avg_processing_time
        )

        # Analyze trend direction
        trend_direction = self._analyze_trend_direction(records)

        # Calculate sample size confidence
        sample_size_confidence = self._calculate_sample_confidence(total_processed)

        return ModelPerformance(
            model_id=model_id,
            content_type=content_type,
            total_processed=total_processed,
            success_count=success_count,
            avg_confidence=avg_confidence,
            avg_processing_time_ms=avg_processing_time,
            confidence_variance=confidence_variance,
            success_rate=success_rate,
            last_updated=datetime.now(timezone.utc),
            performance_score=performance_score,
            trend_direction=trend_direction,
            sample_size_confidence=sample_size_confidence
        )

    def _calculate_composite_score(
        self,
        success_rate: float,
        avg_confidence: float,
        confidence_variance: float,
        avg_processing_time: Optional[float]
    ) -> float:
        """Calculate composite performance score for model ranking."""
        # Base score from success rate and confidence
        base_score = (success_rate * 0.4 + avg_confidence * 0.4)

        # Penalty for high variance (inconsistent results)
        variance_penalty = min(0.2, confidence_variance) * 0.2
        base_score -= variance_penalty

        # Speed bonus (faster is better, but cap the bonus)
        if avg_processing_time:
            # Normalize processing time (assume 10 seconds is baseline)
            normalized_time = min(2.0, avg_processing_time / 10000.0)  # Convert to seconds
            speed_bonus = max(0.0, (2.0 - normalized_time) / 2.0) * 0.2
            base_score += speed_bonus

        return max(0.0, min(1.0, base_score))

    def _analyze_trend_direction(self, records: List[Any]) -> str:
        """Analyze if performance is improving, declining, or stable."""
        if len(records) < 10:
            return "insufficient_data"

        # Sort by timestamp
        sorted_records = sorted(records, key=lambda r: r.timestamp)

        # Split into first and second half
        mid_point = len(sorted_records) // 2
        first_half = sorted_records[:mid_point]
        second_half = sorted_records[mid_point:]

        # Calculate average performance for each half
        first_performance = self._calculate_period_performance(first_half)
        second_performance = self._calculate_period_performance(second_half)

        performance_change = second_performance - first_performance

        if performance_change > 0.05:  # 5% improvement
            return "improving"
        elif performance_change < -0.05:  # 5% decline
            return "declining"
        else:
            return "stable"

    def _calculate_period_performance(self, records: List[Any]) -> float:
        """Calculate average performance for a period."""
        if not records:
            return 0.0

        successful_records = [r for r in records if r.success]
        success_rate = len(successful_records) / len(records)

        confidence_scores = [r.confidence_score for r in successful_records if r.confidence_score is not None]
        avg_confidence = statistics.mean(confidence_scores) if confidence_scores else 0.0

        return success_rate * 0.5 + avg_confidence * 0.5

    def _calculate_sample_confidence(self, sample_size: int) -> float:
        """Calculate confidence in metrics based on sample size."""
        # Simple logarithmic confidence curve
        if sample_size <= 1:
            return 0.1
        elif sample_size < 10:
            return 0.3
        elif sample_size < 50:
            return 0.6
        elif sample_size < 200:
            return 0.8
        else:
            return 1.0

    def _calculate_diversity_score(self, model_id: str, all_models: List[str]) -> float:
        """Calculate diversity bonus for model selection."""
        # Simple diversity scoring based on model names
        # In practice, this could use model capability matrices

        # Different model families get diversity bonus
        model_families = {
            "gpt": ["gpt-4", "gpt-3.5", "gpt-4-turbo"],
            "claude": ["claude-3-opus", "claude-3-sonnet", "claude-instant"],
            "llama": ["llama-3.1", "llama-3", "llama-2"],
            "local": ["local_", "ollama_"]
        }

        selected_families = set()
        for model in all_models:
            for family, variants in model_families.items():
                if any(variant in model.lower() for variant in variants):
                    selected_families.add(family)
                    break

        # Diversity bonus for models from underrepresented families
        current_family = None
        for family, variants in model_families.items():
            if any(variant in model_id.lower() for variant in variants):
                current_family = family
                break

        if current_family and len([m for m in all_models if current_family in m.lower()]) == 1:
            return 0.8  # High diversity bonus
        else:
            return 0.4  # Standard diversity score

    async def _analyze_model_trend(self, records: List[Any]) -> Dict[str, Any]:
        """Analyze performance trend for a specific model."""
        if len(records) < 5:
            return {"status": "insufficient_data", "sample_size": len(records)}

        # Group by week for trend analysis
        weekly_performance = {}
        for record in records:
            week_start = record.timestamp - timedelta(days=record.timestamp.weekday())
            week_key = week_start.strftime("%Y-%W")

            if week_key not in weekly_performance:
                weekly_performance[week_key] = []
            weekly_performance[week_key].append(record)

        # Calculate weekly averages
        weekly_scores = []
        for week, week_records in weekly_performance.items():
            week_performance = self._calculate_period_performance(week_records)
            weekly_scores.append((week, week_performance))

        weekly_scores.sort()

        # Calculate trend
        if len(weekly_scores) >= 2:
            recent_performance = statistics.mean([score for _, score in weekly_scores[-2:]])
            earlier_performance = statistics.mean([score for _, score in weekly_scores[:-2]]) if len(weekly_scores) > 2 else weekly_scores[0][1]
            trend_change = recent_performance - earlier_performance
        else:
            trend_change = 0.0

        return {
            "total_records": len(records),
            "weeks_analyzed": len(weekly_scores),
            "trend_change": trend_change,
            "trend_direction": "improving" if trend_change > 0.05 else "declining" if trend_change < -0.05 else "stable",
            "weekly_performance": weekly_scores[-4:],  # Last 4 weeks
            "current_performance": weekly_scores[-1][1] if weekly_scores else 0.0
        }

    async def _analyze_content_trend(self, records: List[Any]) -> Dict[str, Any]:
        """Analyze performance trend for a content type across all models."""
        models_analyzed = {}

        for model_id in set(r.model_id for r in records):
            model_records = [r for r in records if r.model_id == model_id]
            if len(model_records) >= 5:
                models_analyzed[model_id] = self._calculate_period_performance(model_records)

        best_model = max(models_analyzed.items(), key=lambda x: x[1]) if models_analyzed else None
        worst_model = min(models_analyzed.items(), key=lambda x: x[1]) if models_analyzed else None

        return {
            "models_with_data": len(models_analyzed),
            "best_performing_model": best_model[0] if best_model else None,
            "best_performance_score": best_model[1] if best_model else 0.0,
            "worst_performing_model": worst_model[0] if worst_model else None,
            "worst_performance_score": worst_model[1] if worst_model else 0.0,
            "performance_spread": (best_model[1] - worst_model[1]) if best_model and worst_model else 0.0,
            "avg_performance": statistics.mean(models_analyzed.values()) if models_analyzed else 0.0
        }

    async def _analyze_system_trends(self, records: List[Any]) -> Dict[str, Any]:
        """Analyze overall system performance trends."""
        # Daily processing volume
        daily_volumes = {}
        daily_success_rates = {}

        for record in records:
            day_key = record.timestamp.date()

            if day_key not in daily_volumes:
                daily_volumes[day_key] = 0
                daily_success_rates[day_key] = []

            daily_volumes[day_key] += 1
            daily_success_rates[day_key].append(1 if record.success else 0)

        # Calculate metrics
        avg_daily_volume = statistics.mean(daily_volumes.values()) if daily_volumes else 0

        daily_success_averages = {
            day: statistics.mean(successes)
            for day, successes in daily_success_rates.items()
        }

        overall_success_rate = statistics.mean(daily_success_averages.values()) if daily_success_averages else 0.0

        return {
            "total_processing_sessions": len(records),
            "unique_models": len(set(r.model_id for r in records)),
            "unique_content_types": len(set(r.content_type for r in records)),
            "avg_daily_volume": avg_daily_volume,
            "overall_success_rate": overall_success_rate,
            "analysis_days": len(daily_volumes)
        }

    async def _generate_recommendations(
        self,
        model_trends: Dict[str, Any],
        content_trends: Dict[str, Any]
    ) -> List[str]:
        """Generate actionable recommendations based on performance analysis."""
        recommendations = []

        # Check for declining models
        declining_models = [
            model_id for model_id, trend in model_trends.items()
            if trend.get("trend_direction") == "declining"
        ]

        if declining_models:
            recommendations.append(
                f"Models showing declining performance: {', '.join(declining_models[:3])}. "
                "Consider model retraining or replacement."
            )

        # Check for content types with poor performance
        poor_content_types = [
            content_type for content_type, trend in content_trends.items()
            if trend.get("avg_performance", 0) < 0.6
        ]

        if poor_content_types:
            recommendations.append(
                f"Content types with low performance: {', '.join(poor_content_types[:3])}. "
                "Consider specialized model training or additional preprocessing."
            )

        # Check for high performance spread
        high_spread_types = [
            content_type for content_type, trend in content_trends.items()
            if trend.get("performance_spread", 0) > 0.3
        ]

        if high_spread_types:
            recommendations.append(
                f"High performance variation in: {', '.join(high_spread_types[:3])}. "
                "Consider ensemble methods or model specialization."
            )

        if not recommendations:
            recommendations.append("System performance appears stable across all metrics.")

        return recommendations
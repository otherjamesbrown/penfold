"""Confidence-based escalation framework.

Implements User Story 3: Confidence-Based Escalation
- Escalates low-confidence local results to cloud models
- Manages processing costs and budget constraints
- Tracks escalation effectiveness and quality improvement
- Provides intelligent escalation decision-making
"""

import logging
from datetime import datetime, timezone, timedelta
from typing import Dict, List, Optional, Any, Tuple
from dataclasses import dataclass
from enum import Enum

from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select, func
from sqlalchemy.exc import SQLAlchemyError

from ..storage.models import ProcessingJob, ProcessingResult

logger = logging.getLogger(__name__)


class EscalationTrigger(Enum):
    """Triggers that can cause escalation to cloud processing."""
    LOW_CONFIDENCE = "low_confidence"
    HIGH_UNCERTAINTY = "high_uncertainty"  # High variance between models
    CONTENT_COMPLEXITY = "content_complexity"
    MODEL_DISAGREEMENT = "model_disagreement"
    HISTORICAL_PERFORMANCE = "historical_performance"
    USER_REQUEST = "user_request"


@dataclass
class EscalationRule:
    """Configuration for when and how to escalate processing.

    Defines thresholds and conditions for escalating local model results
    to higher-capability cloud models.
    """
    rule_id: str
    name: str
    content_types: List[str]
    trigger: EscalationTrigger
    confidence_threshold: Optional[float] = None
    variance_threshold: Optional[float] = None
    cost_limit_per_item: Optional[float] = None
    monthly_budget_limit: Optional[float] = None
    cloud_models: List[str] = None
    priority: int = 1  # Lower number = higher priority
    enabled: bool = True
    created_at: Optional[datetime] = None

    def __post_init__(self):
        if self.created_at is None:
            self.created_at = datetime.now(timezone.utc)
        if self.cloud_models is None:
            self.cloud_models = ["gpt-4", "claude-3-opus", "gemini-pro"]


@dataclass
class EscalationDecision:
    """Result of escalation decision-making process."""
    should_escalate: bool
    target_cloud_models: List[str]
    estimated_cost: float
    reasoning: str
    confidence_gap: float
    urgency_score: float
    budget_remaining: float


class BudgetManager:
    """Manages escalation costs and budget constraints."""

    def __init__(self, session: AsyncSession):
        self.session = session
        self.model_costs = {
            # Estimated costs per 1K tokens (can be updated from actual usage)
            "gpt-4": 0.03,
            "gpt-4-turbo": 0.01,
            "claude-3-opus": 0.015,
            "claude-3-sonnet": 0.003,
            "gemini-pro": 0.0005,
            "gemini-ultra": 0.002
        }

    async def estimate_cost(self, content_data: Dict[str, Any], model_names: List[str]) -> float:
        """Estimate cost for processing content with specified models."""
        # Rough token estimation based on content length
        content_text = str(content_data.get("content", ""))
        estimated_tokens = len(content_text.split()) * 1.3  # Words to tokens approximation

        total_cost = 0.0
        for model_name in model_names:
            cost_per_1k_tokens = self.model_costs.get(model_name, 0.01)  # Default cost
            model_cost = (estimated_tokens / 1000) * cost_per_1k_tokens
            total_cost += model_cost

        return total_cost

    async def check_budget_remaining(self, monthly_limit: float) -> float:
        """Check how much budget remains for the current month."""
        start_of_month = datetime.now(timezone.utc).replace(day=1, hour=0, minute=0, second=0, microsecond=0)

        try:
            # Query processing results with cloud models this month
            cloud_models = list(self.model_costs.keys())
            query = (
                select(func.sum(ProcessingResult.metadata["estimated_cost"].astext.cast(float)))
                .where(
                    ProcessingResult.created_at >= start_of_month,
                    ProcessingResult.model_name.in_(cloud_models)
                )
            )

            result = await self.session.execute(query)
            spent_this_month = result.scalar() or 0.0

            return max(0.0, monthly_limit - spent_this_month)

        except SQLAlchemyError as e:
            logger.error(f"Failed to check budget: {e}")
            return monthly_limit  # Conservative approach


class EscalationManager:
    """Manages confidence-based escalation to cloud models."""

    def __init__(self, session: AsyncSession, budget_manager: Optional[BudgetManager] = None):
        self.session = session
        self.budget_manager = budget_manager or BudgetManager(session)
        self.escalation_rules: List[EscalationRule] = []
        self.escalation_history: Dict[str, List[Dict]] = {}

    def add_escalation_rule(self, rule: EscalationRule) -> None:
        """Add escalation rule to the manager."""
        self.escalation_rules.append(rule)
        logger.info(f"Added escalation rule: {rule.name}")

    def remove_escalation_rule(self, rule_id: str) -> bool:
        """Remove escalation rule by ID."""
        original_count = len(self.escalation_rules)
        self.escalation_rules = [r for r in self.escalation_rules if r.rule_id != rule_id]
        removed = len(self.escalation_rules) < original_count
        if removed:
            logger.info(f"Removed escalation rule: {rule_id}")
        return removed

    async def should_escalate(
        self,
        content_type: str,
        content_data: Dict[str, Any],
        local_results: List[Dict[str, Any]],
        coordination_context: Optional[Dict[str, Any]] = None
    ) -> EscalationDecision:
        """Determine if processing should be escalated to cloud models.

        Args:
            content_type: Type of content being processed
            content_data: Original content data
            local_results: Results from local model processing
            coordination_context: Additional context from coordination

        Returns:
            EscalationDecision with recommendation and reasoning
        """
        if not local_results:
            return EscalationDecision(
                should_escalate=False,
                target_cloud_models=[],
                estimated_cost=0.0,
                reasoning="No local results to evaluate",
                confidence_gap=0.0,
                urgency_score=0.0,
                budget_remaining=0.0
            )

        # Find applicable escalation rules
        applicable_rules = [
            rule for rule in self.escalation_rules
            if rule.enabled and content_type in rule.content_types
        ]

        if not applicable_rules:
            return EscalationDecision(
                should_escalate=False,
                target_cloud_models=[],
                estimated_cost=0.0,
                reasoning="No applicable escalation rules",
                confidence_gap=0.0,
                urgency_score=0.0,
                budget_remaining=0.0
            )

        # Evaluate each rule
        escalation_triggers = []
        max_confidence = max(r.get("confidence_score", 0.0) for r in local_results)
        confidence_variance = self._calculate_confidence_variance(local_results)

        for rule in applicable_rules:
            trigger_result = await self._evaluate_rule(
                rule, content_type, content_data, local_results, max_confidence, confidence_variance
            )
            if trigger_result:
                escalation_triggers.append((rule, trigger_result))

        if not escalation_triggers:
            return EscalationDecision(
                should_escalate=False,
                target_cloud_models=[],
                estimated_cost=0.0,
                reasoning="No escalation triggers activated",
                confidence_gap=1.0 - max_confidence,
                urgency_score=0.0,
                budget_remaining=await self._get_budget_remaining(applicable_rules)
            )

        # Select best escalation rule (highest priority)
        best_rule, trigger_data = min(escalation_triggers, key=lambda x: x[0].priority)

        # Check budget constraints
        estimated_cost = await self.budget_manager.estimate_cost(content_data, best_rule.cloud_models)
        budget_remaining = await self._get_budget_remaining([best_rule])

        if best_rule.cost_limit_per_item and estimated_cost > best_rule.cost_limit_per_item:
            return EscalationDecision(
                should_escalate=False,
                target_cloud_models=[],
                estimated_cost=estimated_cost,
                reasoning=f"Cost ${estimated_cost:.4f} exceeds per-item limit ${best_rule.cost_limit_per_item:.4f}",
                confidence_gap=1.0 - max_confidence,
                urgency_score=trigger_data["urgency_score"],
                budget_remaining=budget_remaining
            )

        if estimated_cost > budget_remaining:
            return EscalationDecision(
                should_escalate=False,
                target_cloud_models=[],
                estimated_cost=estimated_cost,
                reasoning=f"Cost ${estimated_cost:.4f} exceeds remaining budget ${budget_remaining:.4f}",
                confidence_gap=1.0 - max_confidence,
                urgency_score=trigger_data["urgency_score"],
                budget_remaining=budget_remaining
            )

        return EscalationDecision(
            should_escalate=True,
            target_cloud_models=best_rule.cloud_models,
            estimated_cost=estimated_cost,
            reasoning=trigger_data["reasoning"],
            confidence_gap=1.0 - max_confidence,
            urgency_score=trigger_data["urgency_score"],
            budget_remaining=budget_remaining - estimated_cost
        )

    async def create_escalation_jobs(
        self,
        decision: EscalationDecision,
        coordination_id: str,
        content_id: str,
        content_type: str,
        content_data: Dict[str, Any],
        local_results: List[Dict[str, Any]],
        job_manager
    ) -> List[str]:
        """Create escalation jobs for cloud processing.

        Args:
            decision: Escalation decision from should_escalate()
            coordination_id: Original coordination ID
            content_id: Content identifier
            content_type: Type of content
            content_data: Original content data
            local_results: Local processing results for context
            job_manager: JobManager instance for creating jobs

        Returns:
            List of created job IDs
        """
        if not decision.should_escalate:
            return []

        escalation_jobs = []

        # Create escalation context with local results
        escalation_context = {
            "content_id": content_id,
            "content_type": content_type,
            "coordination_id": coordination_id,
            "escalation_trigger": decision.reasoning,
            "local_results_summary": self._summarize_local_results(local_results),
            "confidence_gap": decision.confidence_gap,
            "estimated_cost": decision.estimated_cost,
            **content_data
        }

        for model_name in decision.target_cloud_models:
            try:
                job_id = await job_manager.create_job(
                    event_id=f"escalation_{coordination_id}",
                    processor_name=f"cloud_{model_name}",
                    job_type=f"{content_type}_escalation",
                    input_data=escalation_context,
                    priority=0,  # High priority for escalated jobs
                    metadata={
                        "escalation": True,
                        "coordination_id": coordination_id,
                        "local_confidence": max(r.get("confidence_score", 0.0) for r in local_results),
                        "estimated_cost": decision.estimated_cost / len(decision.target_cloud_models)
                    }
                )
                escalation_jobs.append(job_id)

                logger.info(
                    f"Created escalation job {job_id} for {model_name} "
                    f"(content: {content_id}, cost: ${decision.estimated_cost:.4f})"
                )

            except Exception as e:
                logger.error(f"Failed to create escalation job for {model_name}: {e}")

        # Record escalation in history
        self.escalation_history[coordination_id] = {
            "timestamp": datetime.now(timezone.utc),
            "trigger": decision.reasoning,
            "target_models": decision.target_cloud_models,
            "estimated_cost": decision.estimated_cost,
            "job_ids": escalation_jobs,
            "local_results_count": len(local_results)
        }

        return escalation_jobs

    async def analyze_escalation_effectiveness(
        self,
        coordination_id: str,
        escalation_job_ids: List[str],
        job_manager
    ) -> Dict[str, Any]:
        """Analyze the effectiveness of escalation decisions.

        Compares cloud results with local results to measure quality improvement
        and cost-effectiveness of escalation.

        Args:
            coordination_id: Coordination ID
            escalation_job_ids: Job IDs for escalation processing
            job_manager: JobManager for retrieving results

        Returns:
            Analysis of escalation effectiveness
        """
        if coordination_id not in self.escalation_history:
            return {"error": "No escalation history found"}

        escalation_info = self.escalation_history[coordination_id]

        # Get escalation job results
        cloud_results = []
        total_actual_cost = 0.0

        for job_id in escalation_job_ids:
            job = await job_manager.get_job(job_id)
            if job and job.status.value == "completed":
                results = await job_manager.get_job_results(job_id)
                for result in results:
                    cloud_results.append({
                        "processor_name": job.processor_name,
                        "confidence_score": result.confidence_score,
                        "result_data": result.result_data,
                        "processing_time_ms": result.processing_time_ms,
                        "model_name": result.model_name
                    })
                    # Get actual cost from metadata if available
                    actual_cost = job.metadata.get("actual_cost", job.metadata.get("estimated_cost", 0.0))
                    total_actual_cost += float(actual_cost)

        if not cloud_results:
            return {
                "status": "pending",
                "message": "Cloud processing not yet complete",
                "jobs_completed": 0,
                "total_jobs": len(escalation_job_ids)
            }

        # Calculate quality improvement
        cloud_confidences = [r["confidence_score"] for r in cloud_results]
        max_cloud_confidence = max(cloud_confidences) if cloud_confidences else 0.0

        # Get local results confidence from escalation context
        local_confidence = escalation_info.get("local_confidence", 0.0)

        quality_improvement = max_cloud_confidence - local_confidence
        cost_per_improvement = total_actual_cost / max(0.001, quality_improvement)  # Avoid division by zero

        analysis = {
            "coordination_id": coordination_id,
            "escalation_timestamp": escalation_info["timestamp"],
            "trigger_reason": escalation_info["trigger"],
            "local_confidence": local_confidence,
            "cloud_confidence": max_cloud_confidence,
            "quality_improvement": quality_improvement,
            "improvement_percentage": (quality_improvement / max(0.001, local_confidence)) * 100,
            "estimated_cost": escalation_info["estimated_cost"],
            "actual_cost": total_actual_cost,
            "cost_accuracy": abs(total_actual_cost - escalation_info["estimated_cost"]) / max(0.001, escalation_info["estimated_cost"]),
            "cost_per_improvement": cost_per_improvement,
            "models_used": [r["model_name"] for r in cloud_results],
            "escalation_worthwhile": quality_improvement > 0.1,  # 10% improvement threshold
            "cost_effective": cost_per_improvement < 1.0,  # $1 per 100% improvement
            "processing_times": [r["processing_time_ms"] for r in cloud_results if r["processing_time_ms"]],
            "cloud_results_count": len(cloud_results)
        }

        return analysis

    async def _evaluate_rule(
        self,
        rule: EscalationRule,
        content_type: str,
        content_data: Dict[str, Any],
        local_results: List[Dict[str, Any]],
        max_confidence: float,
        confidence_variance: float
    ) -> Optional[Dict[str, Any]]:
        """Evaluate if a specific escalation rule should trigger."""
        urgency_score = 0.0
        reasoning_parts = []

        if rule.trigger == EscalationTrigger.LOW_CONFIDENCE:
            if rule.confidence_threshold and max_confidence < rule.confidence_threshold:
                confidence_gap = rule.confidence_threshold - max_confidence
                urgency_score = confidence_gap / rule.confidence_threshold
                reasoning_parts.append(f"Low confidence: {max_confidence:.2f} < {rule.confidence_threshold:.2f}")

        elif rule.trigger == EscalationTrigger.HIGH_UNCERTAINTY:
            if rule.variance_threshold and confidence_variance > rule.variance_threshold:
                urgency_score = min(1.0, confidence_variance / rule.variance_threshold)
                reasoning_parts.append(f"High uncertainty: variance {confidence_variance:.3f} > {rule.variance_threshold:.3f}")

        elif rule.trigger == EscalationTrigger.MODEL_DISAGREEMENT:
            if len(local_results) > 1:
                # Check if models disagree significantly
                confidences = [r.get("confidence_score", 0.0) for r in local_results]
                confidence_range = max(confidences) - min(confidences)
                if confidence_range > 0.3:  # 30% disagreement threshold
                    urgency_score = min(1.0, confidence_range / 0.5)
                    reasoning_parts.append(f"Model disagreement: confidence range {confidence_range:.2f}")

        elif rule.trigger == EscalationTrigger.CONTENT_COMPLEXITY:
            # Simple heuristics for content complexity
            content_text = str(content_data.get("content", ""))
            complexity_score = self._estimate_content_complexity(content_text, content_type)
            if complexity_score > 0.7:  # High complexity threshold
                urgency_score = complexity_score
                reasoning_parts.append(f"Complex content detected: score {complexity_score:.2f}")

        elif rule.trigger == EscalationTrigger.HISTORICAL_PERFORMANCE:
            # Check historical performance for this content type
            performance_score = await self._check_historical_performance(content_type, max_confidence)
            if performance_score < 0.6:  # Poor historical performance
                urgency_score = 1.0 - performance_score
                reasoning_parts.append(f"Poor historical performance: score {performance_score:.2f}")

        if reasoning_parts and urgency_score > 0.1:  # Minimum urgency threshold
            return {
                "urgency_score": urgency_score,
                "reasoning": f"Rule '{rule.name}': " + "; ".join(reasoning_parts)
            }

        return None

    def _calculate_confidence_variance(self, results: List[Dict[str, Any]]) -> float:
        """Calculate variance in confidence scores across results."""
        if len(results) < 2:
            return 0.0

        confidences = [r.get("confidence_score", 0.0) for r in results]
        mean_confidence = sum(confidences) / len(confidences)
        variance = sum((c - mean_confidence) ** 2 for c in confidences) / len(confidences)
        return variance

    def _estimate_content_complexity(self, content_text: str, content_type: str) -> float:
        """Estimate complexity of content for escalation decisions."""
        # Simple heuristics for content complexity
        complexity_score = 0.0

        # Length complexity
        word_count = len(content_text.split())
        if word_count > 1000:
            complexity_score += 0.3
        elif word_count > 500:
            complexity_score += 0.2

        # Technical language complexity
        technical_words = ["algorithm", "implementation", "architecture", "infrastructure", "deployment"]
        tech_word_ratio = sum(1 for word in content_text.lower().split() if word in technical_words) / max(1, word_count)
        complexity_score += tech_word_ratio * 0.4

        # Multiple languages or special characters
        non_ascii_ratio = sum(1 for char in content_text if ord(char) > 127) / max(1, len(content_text))
        complexity_score += non_ascii_ratio * 0.3

        return min(1.0, complexity_score)

    async def _check_historical_performance(self, content_type: str, current_confidence: float) -> float:
        """Check historical performance for similar content."""
        try:
            # Get recent results for this content type
            recent_cutoff = datetime.now(timezone.utc) - timedelta(days=30)

            query = (
                select(func.avg(ProcessingResult.confidence_score))
                .join(ProcessingJob)
                .where(
                    ProcessingJob.job_type.like(f"{content_type}%"),
                    ProcessingResult.created_at >= recent_cutoff
                )
            )

            result = await self.session.execute(query)
            avg_historical_confidence = result.scalar()

            if avg_historical_confidence is None:
                return 0.8  # Default to good performance if no history

            return float(avg_historical_confidence)

        except SQLAlchemyError as e:
            logger.error(f"Failed to check historical performance: {e}")
            return 0.8  # Conservative default

    def _summarize_local_results(self, local_results: List[Dict[str, Any]]) -> Dict[str, Any]:
        """Create summary of local results for escalation context."""
        if not local_results:
            return {}

        confidences = [r.get("confidence_score", 0.0) for r in local_results]

        return {
            "result_count": len(local_results),
            "max_confidence": max(confidences),
            "min_confidence": min(confidences),
            "avg_confidence": sum(confidences) / len(confidences),
            "confidence_variance": self._calculate_confidence_variance(local_results),
            "models": [r.get("processor_name", "unknown") for r in local_results]
        }

    async def _get_budget_remaining(self, rules: List[EscalationRule]) -> float:
        """Get remaining budget considering all applicable rules."""
        max_budget = 0.0
        for rule in rules:
            if rule.monthly_budget_limit:
                remaining = await self.budget_manager.check_budget_remaining(rule.monthly_budget_limit)
                max_budget = max(max_budget, remaining)
        return max_budget
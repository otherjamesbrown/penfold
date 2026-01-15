"""Core Automation Rules Engine.

Provides the main automation engine that evaluates content against
confidence thresholds and user-defined rules to determine automatic
processing or manual review queueing.

Reference: spec.md User Story 1 (Confidence-Based Automatic Processing)
Reference: research.md for algorithm decisions
"""

from dataclasses import dataclass, field
from datetime import datetime, timezone
from decimal import Decimal
from enum import Enum
from typing import Any, Dict, List, Optional, Tuple
from uuid import UUID, uuid4

from .conditions import evaluate_conditions


class DecisionType(Enum):
    """Types of automation decisions."""

    AUTO_PROCESSED = "auto_processed"
    QUEUED_REVIEW = "queued_review"
    CONFLICT_RESOLVED = "conflict_resolved"


@dataclass
class AutomationDecisionResult:
    """Result of an automation decision evaluation.

    Contains full audit trail information per FR-006.

    Attributes:
        content_id: ID of the content that was evaluated
        content_type: Type of content (email, meeting, etc.)
        decision_type: The decision made (auto_processed, queued_review, etc.)
        confidence_score: AI confidence score at decision time
        threshold_used: Confidence threshold that was applied
        rule_id: ID of the rule that was applied (if any)
        actions_taken: Actions that were or would be executed
        reasoning: Human-readable explanation of the decision
        processing_time_ms: Time taken to process (milliseconds)
        created_at: When the decision was made
    """

    content_id: str
    content_type: str
    decision_type: str
    confidence_score: float
    threshold_used: float
    rule_id: Optional[UUID] = None
    rule_version_id: Optional[int] = None
    actions_taken: Optional[Dict[str, Any]] = None
    reasoning: str = ""
    processing_time_ms: Optional[int] = None
    retry_count: int = 0
    created_at: datetime = field(default_factory=lambda: datetime.now(timezone.utc))


class AutomationEngine:
    """Core automation engine for content processing decisions.

    Evaluates content against confidence thresholds and user-defined
    rules to determine whether content should be automatically processed
    or queued for manual review.

    Attributes:
        default_threshold: Default confidence threshold (0.85 per Clarification #3)
        retry_delays: Retry delays in seconds [1, 4, 16] per Clarification #1
        max_retries: Maximum retry attempts (3 per Clarification #1)
    """

    def __init__(
        self,
        default_threshold: float = 0.85,
        retry_delays: Optional[List[int]] = None,
        max_retries: int = 3,
    ):
        """Initialize the automation engine.

        Args:
            default_threshold: Default confidence threshold for auto-processing
            retry_delays: List of retry delays in seconds
            max_retries: Maximum number of retry attempts
        """
        self.default_threshold = default_threshold
        self.retry_delays = retry_delays or [1, 4, 16]  # Exponential backoff
        self.max_retries = max_retries

    def should_auto_process(
        self, confidence: float, threshold: Optional[float] = None
    ) -> bool:
        """Determine if content should be automatically processed.

        Args:
            confidence: AI confidence score (0.0 to 1.0)
            threshold: Confidence threshold to use (defaults to default_threshold)

        Returns:
            True if confidence meets or exceeds threshold
        """
        effective_threshold = threshold if threshold is not None else self.default_threshold
        return confidence >= effective_threshold

    async def evaluate_content(
        self,
        content: Dict[str, Any],
        threshold: Optional[float] = None,
        rules: Optional[List[Dict[str, Any]]] = None,
    ) -> AutomationDecisionResult:
        """Evaluate content and return an automation decision.

        This is the main entry point for automation evaluation. It:
        1. Checks confidence against threshold
        2. Evaluates matching rules
        3. Returns a decision with full audit trail

        Args:
            content: Content dictionary with content_id, content_type, confidence_score
            threshold: Confidence threshold (defaults to default_threshold)
            rules: Optional list of rules to evaluate

        Returns:
            AutomationDecisionResult with decision and audit trail
        """
        import time

        start_time = time.monotonic()

        content_id = content.get("content_id", "unknown")
        content_type = content.get("content_type", "unknown")
        confidence_score = content.get("confidence_score", 0.0)

        effective_threshold = threshold if threshold is not None else self.default_threshold

        # Determine decision type based on confidence
        if self.should_auto_process(confidence_score, effective_threshold):
            decision_type = DecisionType.AUTO_PROCESSED.value
            reasoning = (
                f"Confidence score ({confidence_score:.2%}) meets or exceeds "
                f"threshold ({effective_threshold:.2%}). Content automatically processed."
            )
        else:
            decision_type = DecisionType.QUEUED_REVIEW.value
            reasoning = (
                f"Confidence score ({confidence_score:.2%}) below "
                f"threshold ({effective_threshold:.2%}). Queued for manual review."
            )

        # Calculate processing time
        processing_time_ms = int((time.monotonic() - start_time) * 1000)

        return AutomationDecisionResult(
            content_id=content_id,
            content_type=content_type,
            decision_type=decision_type,
            confidence_score=confidence_score,
            threshold_used=effective_threshold,
            reasoning=reasoning,
            processing_time_ms=processing_time_ms,
        )

    def rule_matches(
        self, rule_conditions: Dict[str, Any], content: Dict[str, Any]
    ) -> bool:
        """Check if a rule's conditions match the given content.

        Args:
            rule_conditions: Rule conditions in JSONB format
            content: Content dictionary to match against

        Returns:
            True if all conditions match (for AND) or any match (for OR)
        """
        return evaluate_conditions(rule_conditions, content)

    async def evaluate_rules(
        self,
        content: Dict[str, Any],
        rules: List[Dict[str, Any]],
    ) -> List[Tuple[Dict[str, Any], float]]:
        """Evaluate all rules against content and return matches.

        Args:
            content: Content dictionary to evaluate
            rules: List of rules to check

        Returns:
            List of (rule, score) tuples for matching rules
        """
        matches = []
        for rule in rules:
            conditions = rule.get("conditions", {})
            if self.rule_matches(conditions, content):
                # Calculate rule score based on historical accuracy
                # This will be populated from RuleEffectiveness in full implementation
                score = rule.get("accuracy_rate", 0.5)
                matches.append((rule, score))
        return matches

    def resolve_conflict(
        self, matching_rules: List[Tuple[Dict[str, Any], float]]
    ) -> Optional[Dict[str, Any]]:
        """Resolve conflicts when multiple rules match.

        Uses confidence-weighted scoring with priority tiebreaker per
        Clarification #2.

        Algorithm (from research.md):
        score = (historical_accuracy * 0.6) + (confidence_score * 0.3) + ((1 - priority/10) * 0.1)

        Args:
            matching_rules: List of (rule, score) tuples

        Returns:
            The winning rule, or None if no rules match
        """
        if not matching_rules:
            return None

        if len(matching_rules) == 1:
            return matching_rules[0][0]

        # Score each rule
        scored_rules = []
        for rule, base_score in matching_rules:
            accuracy = rule.get("accuracy_rate", 0.5)
            confidence = rule.get("confidence_score", base_score)
            priority = rule.get("priority", 5)

            # Weighted score per research.md algorithm
            score = (accuracy * 0.6) + (confidence * 0.3) + ((1.0 - priority / 10) * 0.1)
            scored_rules.append((rule, score))

        # Return highest scoring rule
        scored_rules.sort(key=lambda x: x[1], reverse=True)
        return scored_rules[0][0]

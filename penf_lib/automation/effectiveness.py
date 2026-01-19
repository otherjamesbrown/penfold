"""Rule effectiveness monitoring and optimization.

Tracks rule performance metrics for User Story 4 (FR-008, FR-013):
- Accuracy rate (correct decisions / total decisions)
- Coverage (matches per time period)
- Processing time metrics
- Conflict win rate

Implements degradation detection per SC-008:
- Identify underperforming rules within 7 days

Reference: data-model.md RuleEffectiveness schema
"""

from dataclasses import dataclass, field
from datetime import datetime, timezone, timedelta
from decimal import Decimal
from typing import Any, Dict, List, Optional, Tuple
from uuid import UUID


@dataclass
class RuleMetrics:
    """Aggregated metrics for a single rule.

    Attributes:
        rule_id: The rule being measured
        period_start: Start of measurement period
        period_end: End of measurement period
        total_matches: Times rule matched content
        total_applied: Times rule was actually applied
        correct_decisions: Decisions without user override
        incorrect_decisions: Decisions user overrode
        accuracy_rate: Calculated accuracy (0-1)
        avg_confidence: Average AI confidence when rule matched
        avg_processing_time_ms: Average processing time
        conflict_count: Times in conflict with other rules
        conflict_win_count: Times won conflicts
    """

    rule_id: UUID
    period_start: datetime
    period_end: datetime
    total_matches: int = 0
    total_applied: int = 0
    correct_decisions: int = 0
    incorrect_decisions: int = 0
    accuracy_rate: Optional[float] = None
    avg_confidence: Optional[float] = None
    avg_processing_time_ms: Optional[int] = None
    conflict_count: int = 0
    conflict_win_count: int = 0


@dataclass
class DegradationAlert:
    """Alert for rule performance degradation.

    Generated when rule accuracy drops below threshold
    per SC-008 (detect within 7 days).

    Attributes:
        rule_id: The degrading rule
        rule_name: Human-readable name
        current_accuracy: Current accuracy rate
        previous_accuracy: Previous accuracy rate
        threshold: The threshold that was crossed
        detected_at: When degradation was detected
        severity: low, medium, high
        recommendation: Suggested action
    """

    rule_id: UUID
    rule_name: str
    current_accuracy: float
    previous_accuracy: float
    threshold: float
    detected_at: datetime
    severity: str
    recommendation: str


def calculate_rule_metrics(
    rule_id: UUID,
    decisions: List[Dict[str, Any]],
    period_start: datetime,
    period_end: datetime,
) -> RuleMetrics:
    """Calculate metrics for a rule from its decisions.

    Args:
        rule_id: The rule to measure
        decisions: Decisions made by this rule
        period_start: Start of measurement period
        period_end: End of measurement period

    Returns:
        RuleMetrics with calculated values
    """
    # Filter decisions for this rule and period
    relevant = [
        d for d in decisions
        if d.get("rule_id") == rule_id
        and period_start <= d.get("created_at", datetime.min) <= period_end
    ]

    total_matches = len(relevant)
    total_applied = sum(1 for d in relevant if d.get("decision_type") == "auto_processed")
    correct = sum(1 for d in relevant if not d.get("user_override", False))
    incorrect = total_matches - correct

    # Calculate accuracy
    accuracy = correct / total_matches if total_matches > 0 else None

    # Calculate average confidence
    confidences = [d.get("confidence_score", 0) for d in relevant if d.get("confidence_score") is not None]
    avg_confidence = sum(confidences) / len(confidences) if confidences else None

    # Calculate average processing time
    times = [d.get("processing_time_ms", 0) for d in relevant if d.get("processing_time_ms") is not None]
    avg_time = int(sum(times) / len(times)) if times else None

    # Count conflicts
    conflict_count = sum(1 for d in relevant if d.get("decision_type") == "conflict_resolved")
    conflict_wins = sum(
        1 for d in relevant
        if d.get("decision_type") == "conflict_resolved" and d.get("rule_id") == rule_id
    )

    return RuleMetrics(
        rule_id=rule_id,
        period_start=period_start,
        period_end=period_end,
        total_matches=total_matches,
        total_applied=total_applied,
        correct_decisions=correct,
        incorrect_decisions=incorrect,
        accuracy_rate=round(accuracy, 3) if accuracy is not None else None,
        avg_confidence=round(avg_confidence, 3) if avg_confidence is not None else None,
        avg_processing_time_ms=avg_time,
        conflict_count=conflict_count,
        conflict_win_count=conflict_wins,
    )


def detect_degradation(
    current_metrics: RuleMetrics,
    previous_metrics: Optional[RuleMetrics],
    rule_name: str,
    accuracy_threshold: float = 0.85,
    min_samples: int = 10,
) -> Optional[DegradationAlert]:
    """Detect if a rule's performance has degraded.

    Per SC-008: Identify underperforming rules within 7 days.

    Args:
        current_metrics: Current period metrics
        previous_metrics: Previous period metrics (for comparison)
        rule_name: Human-readable rule name
        accuracy_threshold: Threshold below which to alert
        min_samples: Minimum samples required for valid measurement

    Returns:
        DegradationAlert if degradation detected, None otherwise
    """
    # Need minimum samples for valid measurement
    if current_metrics.total_matches < min_samples:
        return None

    # Need accuracy measurement
    if current_metrics.accuracy_rate is None:
        return None

    current_accuracy = current_metrics.accuracy_rate
    previous_accuracy = previous_metrics.accuracy_rate if previous_metrics else None

    # Check if below absolute threshold
    if current_accuracy < accuracy_threshold:
        # Determine severity
        if current_accuracy < 0.70:
            severity = "high"
            recommendation = "Consider disabling this rule immediately"
        elif current_accuracy < 0.80:
            severity = "medium"
            recommendation = "Review rule conditions for refinement"
        else:
            severity = "low"
            recommendation = "Monitor rule performance closely"

        return DegradationAlert(
            rule_id=current_metrics.rule_id,
            rule_name=rule_name,
            current_accuracy=current_accuracy,
            previous_accuracy=previous_accuracy or 0.0,
            threshold=accuracy_threshold,
            detected_at=datetime.now(timezone.utc),
            severity=severity,
            recommendation=recommendation,
        )

    # Check for significant drop from previous period
    if previous_accuracy is not None:
        drop = previous_accuracy - current_accuracy
        if drop >= 0.10:  # 10% drop
            return DegradationAlert(
                rule_id=current_metrics.rule_id,
                rule_name=rule_name,
                current_accuracy=current_accuracy,
                previous_accuracy=previous_accuracy,
                threshold=accuracy_threshold,
                detected_at=datetime.now(timezone.utc),
                severity="medium",
                recommendation=f"Accuracy dropped by {drop:.1%}. Investigate recent changes.",
            )

    return None


@dataclass
class AutomationStats:
    """Aggregated automation statistics.

    Provides overall view of automation performance for
    the stats command.

    Attributes:
        period_days: Number of days in measurement period
        total_processed: Total content processed
        auto_processed: Content automatically processed
        queued_review: Content queued for review
        automation_rate: % auto-processed
        overall_accuracy: Accuracy across all rules
        active_rules: Number of enabled rules
        underperforming_rules: Rules below threshold
        pending_patterns: Patterns awaiting review
        conflicts_resolved: Conflicts auto-resolved
    """

    period_days: int
    total_processed: int = 0
    auto_processed: int = 0
    queued_review: int = 0
    automation_rate: float = 0.0
    overall_accuracy: float = 0.0
    active_rules: int = 0
    underperforming_rules: int = 0
    pending_patterns: int = 0
    conflicts_resolved: int = 0


def calculate_automation_stats(
    decisions: List[Dict[str, Any]],
    rules: List[Dict[str, Any]],
    patterns: List[Dict[str, Any]],
    period_days: int = 30,
    accuracy_threshold: float = 0.85,
) -> AutomationStats:
    """Calculate aggregated automation statistics.

    Args:
        decisions: All decisions in period
        rules: All automation rules
        patterns: All detected patterns
        period_days: Number of days to measure
        accuracy_threshold: Threshold for underperforming

    Returns:
        AutomationStats with aggregated values
    """
    total = len(decisions)
    auto = sum(1 for d in decisions if d.get("decision_type") == "auto_processed")
    queued = sum(1 for d in decisions if d.get("decision_type") == "queued_review")
    correct = sum(1 for d in decisions if not d.get("user_override", False))
    conflicts = sum(1 for d in decisions if d.get("decision_type") == "conflict_resolved")

    automation_rate = auto / total if total > 0 else 0.0
    accuracy = correct / total if total > 0 else 0.0

    active = sum(1 for r in rules if r.get("is_enabled", False) and not r.get("is_deleted", False))

    # Count underperforming rules (simplified)
    underperforming = 0  # Would calculate from effectiveness metrics

    pending = sum(1 for p in patterns if p.get("status") == "pending")

    return AutomationStats(
        period_days=period_days,
        total_processed=total,
        auto_processed=auto,
        queued_review=queued,
        automation_rate=round(automation_rate, 3),
        overall_accuracy=round(accuracy, 3),
        active_rules=active,
        underperforming_rules=underperforming,
        pending_patterns=pending,
        conflicts_resolved=conflicts,
    )


def analyze_rule_impact(
    rule: Dict[str, Any],
    decisions: List[Dict[str, Any]],
    period_days: int = 30,
) -> Dict[str, Any]:
    """Analyze the impact of a specific rule.

    Calculates how much automation this rule provides
    and its accuracy compared to manual processing.

    Args:
        rule: The rule to analyze
        decisions: All decisions in period
        period_days: Number of days to analyze

    Returns:
        Impact analysis dictionary
    """
    rule_id = rule.get("id")

    # Get decisions for this rule
    rule_decisions = [d for d in decisions if d.get("rule_id") == rule_id]
    total_decisions = len(decisions)

    rule_total = len(rule_decisions)
    rule_auto = sum(1 for d in rule_decisions if d.get("decision_type") == "auto_processed")
    rule_correct = sum(1 for d in rule_decisions if not d.get("user_override", False))

    coverage = rule_total / total_decisions if total_decisions > 0 else 0.0
    accuracy = rule_correct / rule_total if rule_total > 0 else 0.0

    # Estimate time savings (assuming 30s per manual review)
    time_saved_seconds = rule_auto * 30

    return {
        "rule_id": rule_id,
        "rule_name": rule.get("name", "Unknown"),
        "period_days": period_days,
        "total_matches": rule_total,
        "auto_processed": rule_auto,
        "coverage_rate": round(coverage, 3),
        "accuracy_rate": round(accuracy, 3),
        "estimated_time_saved_minutes": round(time_saved_seconds / 60, 1),
        "status": "healthy" if accuracy >= 0.85 else "needs_attention",
    }

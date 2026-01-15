"""Progressive automation advancement logic.

Implements threshold auto-adjustment and automation rate management
per User Story 3 (FR-007).

Progressive advancement algorithm:
1. Track automation decisions over learning period (default 30 days)
2. Calculate accuracy rate from user overrides
3. If accuracy >= floor (95%), adjust threshold to increase automation
4. Respect user's aggressiveness level preference
5. Never drop accuracy below the floor

Reference: research.md Threshold Adjustment Algorithm
"""

from dataclasses import dataclass
from datetime import datetime, timezone, timedelta
from decimal import Decimal
from enum import Enum
from typing import Any, Dict, List, Optional, Tuple


class AggressivenessLevel(Enum):
    """User preference for automation aggressiveness.

    Conservative: Small adjustments, prioritize accuracy
    Moderate: Balanced adjustments (default)
    Aggressive: Larger adjustments, prioritize automation rate
    """

    CONSERVATIVE = "conservative"
    MODERATE = "moderate"
    AGGRESSIVE = "aggressive"


@dataclass
class ProgressiveAnalysis:
    """Results of progressive automation analysis.

    Attributes:
        current_automation_rate: Current % of auto-processed content
        current_accuracy: Current accuracy rate
        recommended_threshold: Recommended new threshold (or None)
        threshold_delta: Amount to adjust threshold by
        reasoning: Explanation of the recommendation
        can_adjust: Whether adjustment is allowed
    """

    current_automation_rate: float
    current_accuracy: float
    recommended_threshold: Optional[float]
    threshold_delta: float
    reasoning: str
    can_adjust: bool


# Adjustment parameters per aggressiveness level
ADJUSTMENT_PARAMS = {
    AggressivenessLevel.CONSERVATIVE: {
        "max_delta": 0.01,  # Max 1% change
        "min_accuracy_buffer": 0.02,  # Need 2% above floor
        "learning_multiplier": 1.5,  # Longer learning period
    },
    AggressivenessLevel.MODERATE: {
        "max_delta": 0.02,  # Max 2% change
        "min_accuracy_buffer": 0.01,  # Need 1% above floor
        "learning_multiplier": 1.0,  # Standard learning period
    },
    AggressivenessLevel.AGGRESSIVE: {
        "max_delta": 0.03,  # Max 3% change
        "min_accuracy_buffer": 0.005,  # Need 0.5% above floor
        "learning_multiplier": 0.75,  # Shorter learning period
    },
}


def calculate_automation_rate(decisions: List[Dict[str, Any]]) -> float:
    """Calculate the automation rate from decisions.

    Args:
        decisions: List of automation decision records

    Returns:
        Rate of auto-processed decisions (0-1)
    """
    if not decisions:
        return 0.0

    auto_count = sum(
        1 for d in decisions if d.get("decision_type") == "auto_processed"
    )
    return auto_count / len(decisions)


def calculate_accuracy_rate(decisions: List[Dict[str, Any]]) -> float:
    """Calculate accuracy rate from decisions.

    Accuracy is based on user overrides - if user didn't override,
    the decision was correct.

    Args:
        decisions: List of automation decision records

    Returns:
        Accuracy rate (0-1)
    """
    if not decisions:
        return 1.0  # No decisions = perfect accuracy

    correct_count = sum(
        1 for d in decisions if not d.get("user_override", False)
    )
    return correct_count / len(decisions)


def analyze_progressive_adjustment(
    decisions: List[Dict[str, Any]],
    current_threshold: float,
    settings: Dict[str, Any],
) -> ProgressiveAnalysis:
    """Analyze if threshold should be adjusted.

    Checks if learning period is complete and accuracy is sufficient
    to recommend a threshold adjustment.

    Args:
        decisions: Recent automation decisions
        current_threshold: Current confidence threshold
        settings: User's progressive settings

    Returns:
        ProgressiveAnalysis with recommendation
    """
    # Get settings
    aggressiveness = AggressivenessLevel(settings.get("aggressiveness_level", "moderate"))
    accuracy_floor = settings.get("accuracy_floor", 0.95)
    min_learning_days = settings.get("min_learning_period_days", 30)
    target_rate = settings.get("target_automation_rate", 0.60)
    auto_adjust = settings.get("auto_threshold_adjustment", True)

    params = ADJUSTMENT_PARAMS[aggressiveness]

    # Calculate current metrics
    automation_rate = calculate_automation_rate(decisions)
    accuracy_rate = calculate_accuracy_rate(decisions)

    # Check if we can adjust
    if not auto_adjust:
        return ProgressiveAnalysis(
            current_automation_rate=automation_rate,
            current_accuracy=accuracy_rate,
            recommended_threshold=None,
            threshold_delta=0.0,
            reasoning="Auto threshold adjustment is disabled",
            can_adjust=False,
        )

    # Check learning period
    effective_learning_days = int(min_learning_days * params["learning_multiplier"])
    if len(decisions) < effective_learning_days:  # Rough proxy for days
        return ProgressiveAnalysis(
            current_automation_rate=automation_rate,
            current_accuracy=accuracy_rate,
            recommended_threshold=None,
            threshold_delta=0.0,
            reasoning=f"Learning period not complete ({len(decisions)}/{effective_learning_days} decisions)",
            can_adjust=False,
        )

    # Check accuracy floor
    min_accuracy = accuracy_floor + params["min_accuracy_buffer"]
    if accuracy_rate < min_accuracy:
        return ProgressiveAnalysis(
            current_automation_rate=automation_rate,
            current_accuracy=accuracy_rate,
            recommended_threshold=None,
            threshold_delta=0.0,
            reasoning=f"Accuracy ({accuracy_rate:.1%}) below minimum ({min_accuracy:.1%})",
            can_adjust=False,
        )

    # Determine adjustment direction
    if automation_rate >= target_rate:
        # Already at target, no adjustment needed
        return ProgressiveAnalysis(
            current_automation_rate=automation_rate,
            current_accuracy=accuracy_rate,
            recommended_threshold=None,
            threshold_delta=0.0,
            reasoning=f"Automation rate ({automation_rate:.1%}) meets target ({target_rate:.1%})",
            can_adjust=False,
        )

    # Calculate adjustment (lower threshold = more automation)
    # Proportional to how far we are from target
    rate_gap = target_rate - automation_rate
    delta = min(rate_gap * 0.5, params["max_delta"])

    # Ensure we don't go below minimum threshold
    new_threshold = max(current_threshold - delta, 0.50)  # Never below 50%

    return ProgressiveAnalysis(
        current_automation_rate=automation_rate,
        current_accuracy=accuracy_rate,
        recommended_threshold=round(new_threshold, 3),
        threshold_delta=-delta,  # Negative = lowering threshold
        reasoning=(
            f"Accuracy ({accuracy_rate:.1%}) exceeds floor ({accuracy_floor:.1%}). "
            f"Lowering threshold by {delta:.1%} to increase automation."
        ),
        can_adjust=True,
    )


class ProgressiveAutomation:
    """Manages progressive automation advancement.

    Tracks automation decisions and recommends threshold adjustments
    to gradually increase automation rate while maintaining accuracy.
    """

    def __init__(
        self,
        default_accuracy_floor: float = 0.95,
        default_target_rate: float = 0.60,
        default_learning_days: int = 30,
    ):
        """Initialize progressive automation manager.

        Args:
            default_accuracy_floor: Minimum accuracy to maintain
            default_target_rate: Target automation rate
            default_learning_days: Days before adjustments allowed
        """
        self.default_accuracy_floor = default_accuracy_floor
        self.default_target_rate = default_target_rate
        self.default_learning_days = default_learning_days

    def get_default_settings(self, user_id: str, tenant_id: str) -> Dict[str, Any]:
        """Get default progressive settings for a user.

        Args:
            user_id: User identifier
            tenant_id: Tenant identifier

        Returns:
            Default settings dictionary
        """
        return {
            "tenant_id": tenant_id,
            "user_id": user_id,
            "aggressiveness_level": "moderate",
            "auto_threshold_adjustment": True,
            "min_learning_period_days": self.default_learning_days,
            "target_automation_rate": self.default_target_rate,
            "accuracy_floor": self.default_accuracy_floor,
            "pattern_suggestion_enabled": True,
            "notification_preferences": {},
        }

    def analyze(
        self,
        decisions: List[Dict[str, Any]],
        current_threshold: float,
        settings: Optional[Dict[str, Any]] = None,
    ) -> ProgressiveAnalysis:
        """Analyze decisions and recommend adjustments.

        Args:
            decisions: Recent automation decisions
            current_threshold: Current confidence threshold
            settings: User's progressive settings (uses defaults if None)

        Returns:
            Analysis with adjustment recommendation
        """
        if settings is None:
            settings = {
                "aggressiveness_level": "moderate",
                "accuracy_floor": self.default_accuracy_floor,
                "min_learning_period_days": self.default_learning_days,
                "target_automation_rate": self.default_target_rate,
                "auto_threshold_adjustment": True,
            }

        return analyze_progressive_adjustment(decisions, current_threshold, settings)

    def should_notify_adjustment(
        self,
        analysis: ProgressiveAnalysis,
        settings: Dict[str, Any],
    ) -> bool:
        """Check if user should be notified about adjustment.

        Args:
            analysis: The analysis results
            settings: User's notification preferences

        Returns:
            True if notification should be sent
        """
        if not analysis.can_adjust:
            return False

        prefs = settings.get("notification_preferences", {})
        notify_adjustments = prefs.get("threshold_adjustments", True)

        return notify_adjustments and analysis.recommended_threshold is not None

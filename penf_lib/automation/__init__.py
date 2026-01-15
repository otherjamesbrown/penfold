"""Automation Rules Engine.

Intelligent, progressive automation of content categorization based on
user feedback patterns and AI confidence scores.

This module provides:
- Confidence-based automatic processing (User Story 1)
- Rule creation and management (User Story 2)
- Progressive automation advancement (User Story 3)
- Rule effectiveness monitoring (User Story 4)
- Conflict resolution (User Story 5)

Integration Points:
- AI Coordination (003) for confidence scoring
- Event Processing (002) for event-driven automation
- Daily Review (006) for user feedback loop
"""

from .engine import AutomationEngine, AutomationDecisionResult
from .conditions import (
    evaluate_condition,
    evaluate_conditions,
    InvalidConditionError,
    RuleCondition,
)
from .models import (
    AutomationRule,
    AutomationRuleVersion,
    ConfidenceThreshold,
    AutomationDecision,
    RuleEffectiveness,
    AutomationPattern,
    RuleConflict,
    ProgressiveSettings,
)
from .repository import AutomationRepository
from .patterns import (
    PatternDetector,
    DetectedPattern,
    compute_pattern_signature,
    extract_pattern_conditions,
    calculate_pattern_confidence,
)
from .progressive import (
    ProgressiveAutomation,
    ProgressiveAnalysis,
    AggressivenessLevel,
    calculate_automation_rate,
    calculate_accuracy_rate,
)

__all__ = [
    # Engine
    "AutomationEngine",
    "AutomationDecisionResult",
    # Conditions
    "evaluate_condition",
    "evaluate_conditions",
    "InvalidConditionError",
    "RuleCondition",
    # Models
    "AutomationRule",
    "AutomationRuleVersion",
    "ConfidenceThreshold",
    "AutomationDecision",
    "RuleEffectiveness",
    "AutomationPattern",
    "RuleConflict",
    "ProgressiveSettings",
    # Repository
    "AutomationRepository",
    # Patterns
    "PatternDetector",
    "DetectedPattern",
    "compute_pattern_signature",
    "extract_pattern_conditions",
    "calculate_pattern_confidence",
    # Progressive
    "ProgressiveAutomation",
    "ProgressiveAnalysis",
    "AggressivenessLevel",
    "calculate_automation_rate",
    "calculate_accuracy_rate",
]

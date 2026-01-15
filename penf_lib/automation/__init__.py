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
from .repository import AutomationRepository

__all__ = [
    # Engine
    "AutomationEngine",
    "AutomationDecisionResult",
    # Conditions
    "evaluate_condition",
    "evaluate_conditions",
    "InvalidConditionError",
    "RuleCondition",
    # Repository
    "AutomationRepository",
]

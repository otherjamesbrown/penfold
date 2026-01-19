"""AI Coordination Framework.

Multi-model coordination system for parallel AI processing, ensemble combination,
confidence-based escalation, and performance learning.

This module builds on the Event Processing Framework (002) to provide intelligent
coordination between local and cloud AI models.
"""

from .coordinator import ModelCoordinator
from .ensemble import EnsembleResult, EnsembleCombiner
from .escalation import EscalationRule, EscalationManager
from .performance import ModelPerformance, PerformanceTracker
from .models import CoordinationDecision, QualityValidation, ModelProfile

__all__ = [
    "ModelCoordinator",
    "EnsembleResult",
    "EnsembleCombiner",
    "EscalationRule",
    "EscalationManager",
    "ModelPerformance",
    "PerformanceTracker",
    "CoordinationDecision",
    "QualityValidation",
    "ModelProfile",
]
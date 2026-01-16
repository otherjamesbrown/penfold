"""Confidence scoring module for relationship discovery.

Implements multi-factor confidence calculation including:
- AI extraction confidence weighting
- Evidence strength calculation with type weighting
- Temporal decay for stale evidence
- Interaction frequency boost
- Entity resolution confidence overlay
- Composite score algorithm
"""

from __future__ import annotations

import math
from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import Any
from uuid import UUID


@dataclass
class EvidenceItem:
    """A single piece of evidence supporting a relationship.

    Evidence is collected from content analysis and contributes
    to the overall confidence score of a relationship.
    """

    evidence_type: str  # email, meeting, mention, inference, user_input
    source_id: UUID
    observed_at: datetime
    strength_contribution: float  # 0.0 to 1.0
    content_snippet: str = ""
    metadata: dict[str, Any] = field(default_factory=dict)


@dataclass
class ConfidenceFactors:
    """All factors contributing to relationship confidence.

    These factors are combined using a weighted algorithm
    to produce the final confidence score.
    """

    ai_confidence: float
    evidence_strength: float
    temporal_decay: float = 1.0
    interaction_boost: float = 1.0
    entity_resolution_confidence: float = 1.0


# Evidence type weights - higher for more reliable evidence sources
EVIDENCE_TYPE_WEIGHTS: dict[str, float] = {
    "meeting": 1.2,  # Direct participation is strong evidence
    "email": 1.0,  # Direct communication
    "mention": 0.7,  # Indirect reference
    "inference": 0.5,  # AI-inferred relationship
    "user_input": 1.5,  # User validation is strongest
}

# Confidence factor weights for combining
FACTOR_WEIGHTS: dict[str, float] = {
    "ai_confidence": 0.30,
    "evidence_strength": 0.40,
    "entity_resolution": 0.15,
    "temporal": 0.15,
}

# Temporal decay parameters
DECAY_HALF_LIFE_DAYS = 180  # Confidence halves every 180 days without new evidence
DECAY_MINIMUM = 0.20  # Minimum decay multiplier


def calculate_ai_confidence_weight(ai_confidence: float) -> float:
    """Calculate weight from AI extraction confidence.

    Higher AI confidence maps to higher weight, with some
    non-linear scaling to reward high-confidence extractions.

    Args:
        ai_confidence: Raw AI confidence score (0.0 to 1.0)

    Returns:
        Weighted confidence contribution (0.0 to 1.0)
    """
    if ai_confidence <= 0.0:
        return 0.0

    # Apply slight exponential boost to high confidence scores
    # This rewards models that are more certain
    return min(1.0, ai_confidence * (1.0 + 0.1 * ai_confidence))


def calculate_evidence_strength(evidence_items: list[EvidenceItem]) -> float:
    """Calculate overall evidence strength from evidence items.

    Combines individual evidence contributions with type weighting
    and applies diminishing returns for additional evidence.

    Args:
        evidence_items: List of evidence items to evaluate

    Returns:
        Combined evidence strength (0.0 to 1.0)
    """
    if not evidence_items:
        return 0.0

    total_weighted_contribution = 0.0

    for evidence in evidence_items:
        type_weight = EVIDENCE_TYPE_WEIGHTS.get(evidence.evidence_type, 0.8)
        weighted_contribution = evidence.strength_contribution * type_weight
        total_weighted_contribution += weighted_contribution

    # Apply diminishing returns: each additional piece of evidence
    # contributes less to the total. Use logarithmic scaling.
    evidence_count = len(evidence_items)
    diminishing_factor = 1.0 + math.log10(evidence_count + 1) * 0.5

    # Normalize and cap at 1.0
    raw_strength = total_weighted_contribution / diminishing_factor
    return min(1.0, raw_strength)


def calculate_temporal_decay(last_evidence_at: datetime) -> float:
    """Calculate temporal decay based on time since last evidence.

    Relationships with no recent evidence have reduced confidence
    using exponential decay with a minimum floor.

    Args:
        last_evidence_at: Timestamp of most recent evidence

    Returns:
        Decay multiplier (DECAY_MINIMUM to 1.0)
    """
    now = datetime.now(timezone.utc)
    days_since = (now - last_evidence_at).total_seconds() / (24 * 3600)

    if days_since <= 0:
        return 1.0

    # Exponential decay: decay = 0.5^(days/half_life)
    decay = math.pow(0.5, days_since / DECAY_HALF_LIFE_DAYS)

    # Apply minimum floor
    return max(DECAY_MINIMUM, decay)


def calculate_interaction_boost(interaction_count: int) -> float:
    """Calculate boost from interaction frequency.

    Relationships with more interactions (evidence) get a boost
    to their confidence, with a maximum cap.

    Args:
        interaction_count: Number of interactions supporting relationship

    Returns:
        Boost multiplier (1.0 to 1.5)
    """
    if interaction_count <= 0:
        return 1.0

    # Logarithmic boost: more interactions = higher boost, but diminishing
    # boost = 1 + 0.1 * log2(count + 1), capped at 1.5
    boost = 1.0 + 0.1 * math.log2(interaction_count + 1)
    return min(1.5, boost)


def combine_confidence_factors(factors: ConfidenceFactors) -> float:
    """Combine all confidence factors into final score.

    Uses weighted combination with interaction boost as multiplier.

    Args:
        factors: ConfidenceFactors containing all factor values

    Returns:
        Final confidence score (0.0 to 1.0)
    """
    # Calculate weighted AI confidence
    ai_weight = calculate_ai_confidence_weight(factors.ai_confidence)

    # Combine base factors with weights
    base_score = (
        ai_weight * FACTOR_WEIGHTS["ai_confidence"]
        + factors.evidence_strength * FACTOR_WEIGHTS["evidence_strength"]
        + factors.entity_resolution_confidence * FACTOR_WEIGHTS["entity_resolution"]
        + factors.temporal_decay * FACTOR_WEIGHTS["temporal"]
    )

    # Apply interaction boost as multiplier
    boosted_score = base_score * factors.interaction_boost

    # Ensure score is in valid range
    return max(0.0, min(1.0, boosted_score))


class ConfidenceCalculator:
    """Calculator for relationship confidence scores.

    Provides methods to calculate confidence from various factors
    and convert scores to lifecycle states.
    """

    # Lifecycle state thresholds
    LIFECYCLE_THRESHOLDS = {
        "active": 0.70,  # High confidence = active
        "pending": 0.40,  # Medium confidence = pending review
        # Below 0.40 = experimental (stays pending)
    }

    def calculate_relationship_confidence(
        self,
        ai_confidence: float,
        evidence_items: list[EvidenceItem],
        last_evidence_at: datetime,
        interaction_count: int,
        entity_resolution_confidence: float = 1.0,
    ) -> float:
        """Calculate overall confidence for a relationship.

        Args:
            ai_confidence: AI extraction confidence
            evidence_items: List of supporting evidence
            last_evidence_at: Time of most recent evidence
            interaction_count: Number of interactions
            entity_resolution_confidence: Confidence in entity resolution

        Returns:
            Final confidence score (0.0 to 1.0)
        """
        factors = ConfidenceFactors(
            ai_confidence=ai_confidence,
            evidence_strength=calculate_evidence_strength(evidence_items),
            temporal_decay=calculate_temporal_decay(last_evidence_at),
            interaction_boost=calculate_interaction_boost(interaction_count),
            entity_resolution_confidence=entity_resolution_confidence,
        )

        return combine_confidence_factors(factors)

    def get_confidence_breakdown(
        self,
        ai_confidence: float,
        evidence_items: list[EvidenceItem],
        last_evidence_at: datetime,
        interaction_count: int,
        entity_resolution_confidence: float = 1.0,
    ) -> dict[str, float]:
        """Get detailed breakdown of confidence factors.

        Args:
            ai_confidence: AI extraction confidence
            evidence_items: List of supporting evidence
            last_evidence_at: Time of most recent evidence
            interaction_count: Number of interactions
            entity_resolution_confidence: Confidence in entity resolution

        Returns:
            Dictionary with all factor values and final score
        """
        factors = ConfidenceFactors(
            ai_confidence=ai_confidence,
            evidence_strength=calculate_evidence_strength(evidence_items),
            temporal_decay=calculate_temporal_decay(last_evidence_at),
            interaction_boost=calculate_interaction_boost(interaction_count),
            entity_resolution_confidence=entity_resolution_confidence,
        )

        return {
            "ai_confidence": factors.ai_confidence,
            "ai_confidence_weighted": calculate_ai_confidence_weight(
                factors.ai_confidence
            ),
            "evidence_strength": factors.evidence_strength,
            "temporal_decay": factors.temporal_decay,
            "interaction_boost": factors.interaction_boost,
            "entity_resolution_confidence": factors.entity_resolution_confidence,
            "final_score": combine_confidence_factors(factors),
        }

    def to_lifecycle_state(self, confidence_score: float) -> str:
        """Map confidence score to lifecycle state.

        Args:
            confidence_score: Calculated confidence (0.0 to 1.0)

        Returns:
            Lifecycle state string (active, pending)
        """
        if confidence_score >= self.LIFECYCLE_THRESHOLDS["active"]:
            return "active"
        return "pending"

    def recalculate_from_evidence(
        self,
        existing_confidence: float,
        new_evidence: EvidenceItem,
        existing_evidence_count: int,
    ) -> float:
        """Recalculate confidence after adding new evidence.

        Simplified recalculation when adding single evidence item.

        Args:
            existing_confidence: Current confidence score
            new_evidence: New evidence item being added
            existing_evidence_count: Current evidence count

        Returns:
            Updated confidence score
        """
        # New evidence provides boost based on its type
        type_weight = EVIDENCE_TYPE_WEIGHTS.get(new_evidence.evidence_type, 0.8)
        evidence_boost = new_evidence.strength_contribution * type_weight * 0.1

        # Fresh evidence resets temporal decay
        # This is a simplified calculation; full recalc uses all factors
        updated = existing_confidence + evidence_boost

        # Also factor in increased interaction count
        new_interaction_count = existing_evidence_count + 1
        interaction_boost = calculate_interaction_boost(new_interaction_count)

        # Normalize with boost
        final = min(1.0, updated * (interaction_boost / 1.0))
        return max(0.0, final)


__all__ = [
    "ConfidenceCalculator",
    "ConfidenceFactors",
    "EvidenceItem",
    "calculate_ai_confidence_weight",
    "calculate_evidence_strength",
    "calculate_interaction_boost",
    "calculate_temporal_decay",
    "combine_confidence_factors",
]

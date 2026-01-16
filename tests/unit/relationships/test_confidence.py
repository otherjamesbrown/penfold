"""Unit tests for confidence scoring module.

Tests the multi-factor confidence calculation algorithm including:
- AI extraction confidence weighting
- Evidence strength calculation
- Temporal decay
- Interaction frequency boost
- Entity resolution confidence overlay
- Composite score algorithm
"""

from __future__ import annotations

from datetime import datetime, timedelta, timezone
from decimal import Decimal
from typing import Any
from uuid import uuid4

import pytest

from penf_lib.relationships.confidence import (
    ConfidenceCalculator,
    ConfidenceFactors,
    EvidenceItem,
    calculate_ai_confidence_weight,
    calculate_evidence_strength,
    calculate_interaction_boost,
    calculate_temporal_decay,
    combine_confidence_factors,
)


class TestConfidenceFactors:
    """Tests for the ConfidenceFactors data model."""

    def test_create_confidence_factors(self) -> None:
        """Test creating ConfidenceFactors with all factors."""
        factors = ConfidenceFactors(
            ai_confidence=0.85,
            evidence_strength=0.70,
            temporal_decay=0.95,
            interaction_boost=1.15,
            entity_resolution_confidence=0.90,
        )

        assert factors.ai_confidence == 0.85
        assert factors.evidence_strength == 0.70
        assert factors.temporal_decay == 0.95
        assert factors.interaction_boost == 1.15
        assert factors.entity_resolution_confidence == 0.90

    def test_confidence_factors_defaults(self) -> None:
        """Test ConfidenceFactors with default values."""
        factors = ConfidenceFactors(
            ai_confidence=0.75,
            evidence_strength=0.60,
        )

        assert factors.ai_confidence == 0.75
        assert factors.evidence_strength == 0.60
        assert factors.temporal_decay == 1.0  # Default no decay
        assert factors.interaction_boost == 1.0  # Default no boost
        assert factors.entity_resolution_confidence == 1.0  # Default full confidence


class TestEvidenceItem:
    """Tests for the EvidenceItem data model."""

    def test_create_evidence_item(self) -> None:
        """Test creating an EvidenceItem."""
        evidence = EvidenceItem(
            evidence_type="email",
            source_id=uuid4(),
            observed_at=datetime.now(timezone.utc),
            strength_contribution=0.25,
        )

        assert evidence.evidence_type == "email"
        assert evidence.strength_contribution == 0.25

    def test_evidence_item_with_content(self) -> None:
        """Test EvidenceItem with optional content."""
        evidence = EvidenceItem(
            evidence_type="meeting",
            source_id=uuid4(),
            observed_at=datetime.now(timezone.utc),
            strength_contribution=0.35,
            content_snippet="Discussed project timeline",
            metadata={"meeting_type": "standup"},
        )

        assert evidence.content_snippet == "Discussed project timeline"
        assert evidence.metadata["meeting_type"] == "standup"


class TestCalculateAIConfidenceWeight:
    """Tests for AI confidence weighting."""

    def test_high_confidence_weight(self) -> None:
        """Test that high AI confidence receives proportional weight."""
        weight = calculate_ai_confidence_weight(0.95)
        assert 0.90 <= weight <= 1.0

    def test_medium_confidence_weight(self) -> None:
        """Test that medium AI confidence receives moderate weight."""
        weight = calculate_ai_confidence_weight(0.70)
        assert 0.60 <= weight <= 0.80

    def test_low_confidence_weight(self) -> None:
        """Test that low AI confidence receives reduced weight."""
        weight = calculate_ai_confidence_weight(0.40)
        assert 0.30 <= weight <= 0.50

    def test_zero_confidence_weight(self) -> None:
        """Test that zero AI confidence returns zero weight."""
        weight = calculate_ai_confidence_weight(0.0)
        assert weight == 0.0

    def test_confidence_weight_bounds(self) -> None:
        """Test that weight is always within [0, 1]."""
        for confidence in [0.0, 0.25, 0.50, 0.75, 1.0]:
            weight = calculate_ai_confidence_weight(confidence)
            assert 0.0 <= weight <= 1.0


class TestCalculateEvidenceStrength:
    """Tests for evidence strength calculation."""

    def test_single_evidence_strength(self) -> None:
        """Test strength calculation with single evidence item."""
        evidence_items = [
            EvidenceItem(
                evidence_type="email",
                source_id=uuid4(),
                observed_at=datetime.now(timezone.utc),
                strength_contribution=0.30,
            )
        ]

        strength = calculate_evidence_strength(evidence_items)
        assert 0.20 <= strength <= 0.40

    def test_multiple_evidence_strength(self) -> None:
        """Test strength calculation with multiple evidence items."""
        now = datetime.now(timezone.utc)
        evidence_items = [
            EvidenceItem(
                evidence_type="email",
                source_id=uuid4(),
                observed_at=now,
                strength_contribution=0.25,
            ),
            EvidenceItem(
                evidence_type="meeting",
                source_id=uuid4(),
                observed_at=now - timedelta(days=1),
                strength_contribution=0.35,
            ),
            EvidenceItem(
                evidence_type="mention",
                source_id=uuid4(),
                observed_at=now - timedelta(days=7),
                strength_contribution=0.15,
            ),
        ]

        strength = calculate_evidence_strength(evidence_items)
        # Multiple evidence should increase strength
        assert strength >= 0.40

    def test_empty_evidence_strength(self) -> None:
        """Test that no evidence returns zero strength."""
        strength = calculate_evidence_strength([])
        assert strength == 0.0

    def test_evidence_type_weighting(self) -> None:
        """Test that different evidence types have different weights."""
        now = datetime.now(timezone.utc)

        # Meeting evidence typically stronger than mention
        meeting_evidence = [
            EvidenceItem(
                evidence_type="meeting",
                source_id=uuid4(),
                observed_at=now,
                strength_contribution=0.35,
            )
        ]

        mention_evidence = [
            EvidenceItem(
                evidence_type="mention",
                source_id=uuid4(),
                observed_at=now,
                strength_contribution=0.15,
            )
        ]

        meeting_strength = calculate_evidence_strength(meeting_evidence)
        mention_strength = calculate_evidence_strength(mention_evidence)

        assert meeting_strength >= mention_strength

    def test_evidence_strength_diminishing_returns(self) -> None:
        """Test that adding more evidence has diminishing returns."""
        now = datetime.now(timezone.utc)

        # Create increasing numbers of evidence items
        def create_evidence_list(count: int) -> list[EvidenceItem]:
            return [
                EvidenceItem(
                    evidence_type="email",
                    source_id=uuid4(),
                    observed_at=now - timedelta(days=i),
                    strength_contribution=0.20,
                )
                for i in range(count)
            ]

        strength_1 = calculate_evidence_strength(create_evidence_list(1))
        strength_5 = calculate_evidence_strength(create_evidence_list(5))
        strength_10 = calculate_evidence_strength(create_evidence_list(10))

        # More evidence should increase strength but with diminishing returns
        assert strength_5 > strength_1
        assert strength_10 > strength_5
        # Check diminishing returns: increment from 5->10 < increment from 1->5
        assert (strength_10 - strength_5) <= (strength_5 - strength_1)


class TestCalculateTemporalDecay:
    """Tests for temporal decay calculation."""

    def test_recent_no_decay(self) -> None:
        """Test that recent activity has no decay."""
        last_evidence = datetime.now(timezone.utc) - timedelta(hours=1)
        decay = calculate_temporal_decay(last_evidence)
        assert decay >= 0.98

    def test_week_old_decay(self) -> None:
        """Test decay for week-old evidence."""
        last_evidence = datetime.now(timezone.utc) - timedelta(days=7)
        decay = calculate_temporal_decay(last_evidence)
        assert 0.85 <= decay <= 0.99

    def test_month_old_decay(self) -> None:
        """Test decay for month-old evidence."""
        last_evidence = datetime.now(timezone.utc) - timedelta(days=30)
        decay = calculate_temporal_decay(last_evidence)
        assert 0.70 <= decay <= 0.95

    def test_year_old_decay(self) -> None:
        """Test decay for year-old evidence."""
        last_evidence = datetime.now(timezone.utc) - timedelta(days=365)
        decay = calculate_temporal_decay(last_evidence)
        # With 180-day half-life, 365 days gives ~0.25 decay
        assert 0.20 <= decay <= 0.35

    def test_very_old_decay_minimum(self) -> None:
        """Test that very old evidence has minimum decay value."""
        last_evidence = datetime.now(timezone.utc) - timedelta(days=730)  # 2 years
        decay = calculate_temporal_decay(last_evidence)
        assert decay >= 0.20  # Should have some minimum floor


class TestCalculateInteractionBoost:
    """Tests for interaction frequency boost calculation."""

    def test_no_interactions_no_boost(self) -> None:
        """Test that zero interactions gives no boost."""
        boost = calculate_interaction_boost(0)
        assert boost == 1.0

    def test_few_interactions_small_boost(self) -> None:
        """Test that few interactions give small boost."""
        boost = calculate_interaction_boost(3)
        # log2(4) = 2, so boost = 1 + 0.1*2 = 1.2
        assert 1.0 <= boost <= 1.25

    def test_moderate_interactions_medium_boost(self) -> None:
        """Test that moderate interactions give medium boost."""
        boost = calculate_interaction_boost(10)
        # log2(11) ~= 3.46, so boost = 1 + 0.1*3.46 = 1.346
        assert 1.30 <= boost <= 1.40

    def test_many_interactions_large_boost(self) -> None:
        """Test that many interactions give larger boost."""
        boost = calculate_interaction_boost(50)
        # log2(51) ~= 5.67, boost = 1 + 0.1*5.67 = 1.567, capped at 1.5
        assert 1.45 <= boost <= 1.50

    def test_boost_has_maximum(self) -> None:
        """Test that boost has a maximum cap."""
        boost_high = calculate_interaction_boost(100)
        boost_very_high = calculate_interaction_boost(1000)
        # Boost should be capped
        assert boost_very_high <= 1.50


class TestCombineConfidenceFactors:
    """Tests for combining all confidence factors."""

    def test_combine_all_factors(self) -> None:
        """Test combining all confidence factors into final score."""
        factors = ConfidenceFactors(
            ai_confidence=0.85,
            evidence_strength=0.70,
            temporal_decay=0.95,
            interaction_boost=1.10,
            entity_resolution_confidence=0.90,
        )

        score = combine_confidence_factors(factors)

        # Score should be in valid range
        assert 0.0 <= score <= 1.0
        # With these good factors, should be reasonably high
        assert score >= 0.60

    def test_combine_low_factors(self) -> None:
        """Test combining low confidence factors."""
        factors = ConfidenceFactors(
            ai_confidence=0.40,
            evidence_strength=0.30,
            temporal_decay=0.60,
            interaction_boost=1.0,
            entity_resolution_confidence=0.50,
        )

        score = combine_confidence_factors(factors)

        assert 0.0 <= score <= 1.0
        assert score <= 0.50

    def test_combine_with_default_factors(self) -> None:
        """Test combining with default factor values."""
        factors = ConfidenceFactors(
            ai_confidence=0.75,
            evidence_strength=0.60,
        )

        score = combine_confidence_factors(factors)

        assert 0.0 <= score <= 1.0

    def test_combine_edge_cases(self) -> None:
        """Test edge cases in factor combination."""
        # All maximum factors
        max_factors = ConfidenceFactors(
            ai_confidence=1.0,
            evidence_strength=1.0,
            temporal_decay=1.0,
            interaction_boost=1.30,
            entity_resolution_confidence=1.0,
        )
        max_score = combine_confidence_factors(max_factors)
        assert max_score <= 1.0  # Should be capped at 1.0

        # All minimum factors
        min_factors = ConfidenceFactors(
            ai_confidence=0.0,
            evidence_strength=0.0,
            temporal_decay=0.2,
            interaction_boost=1.0,
            entity_resolution_confidence=0.0,
        )
        min_score = combine_confidence_factors(min_factors)
        assert min_score >= 0.0


class TestConfidenceCalculator:
    """Tests for the ConfidenceCalculator class."""

    @pytest.fixture
    def calculator(self) -> ConfidenceCalculator:
        """Create a ConfidenceCalculator instance."""
        return ConfidenceCalculator()

    def test_calculate_relationship_confidence(
        self, calculator: ConfidenceCalculator
    ) -> None:
        """Test calculating confidence for a relationship."""
        now = datetime.now(timezone.utc)
        evidence_items = [
            EvidenceItem(
                evidence_type="email",
                source_id=uuid4(),
                observed_at=now - timedelta(days=1),
                strength_contribution=0.30,
            ),
            EvidenceItem(
                evidence_type="meeting",
                source_id=uuid4(),
                observed_at=now,
                strength_contribution=0.40,
            ),
        ]

        score = calculator.calculate_relationship_confidence(
            ai_confidence=0.80,
            evidence_items=evidence_items,
            last_evidence_at=now,
            interaction_count=5,
            entity_resolution_confidence=0.95,
        )

        assert 0.0 <= score <= 1.0
        assert score >= 0.50  # Good factors should yield decent score

    def test_calculate_with_no_evidence(
        self, calculator: ConfidenceCalculator
    ) -> None:
        """Test calculation with no evidence items."""
        score = calculator.calculate_relationship_confidence(
            ai_confidence=0.75,
            evidence_items=[],
            last_evidence_at=datetime.now(timezone.utc),
            interaction_count=0,
            entity_resolution_confidence=1.0,
        )

        # Should still produce a score based on AI confidence
        assert 0.0 <= score <= 1.0
        # But should be lower than with evidence
        assert score <= 0.60

    def test_recalculate_on_new_evidence(
        self, calculator: ConfidenceCalculator
    ) -> None:
        """Test recalculating confidence when new evidence is added."""
        now = datetime.now(timezone.utc)

        initial_evidence = [
            EvidenceItem(
                evidence_type="mention",
                source_id=uuid4(),
                observed_at=now - timedelta(days=30),
                strength_contribution=0.15,
            )
        ]

        initial_score = calculator.calculate_relationship_confidence(
            ai_confidence=0.65,
            evidence_items=initial_evidence,
            last_evidence_at=now - timedelta(days=30),
            interaction_count=1,
            entity_resolution_confidence=0.90,
        )

        # Add new evidence
        updated_evidence = initial_evidence + [
            EvidenceItem(
                evidence_type="meeting",
                source_id=uuid4(),
                observed_at=now,
                strength_contribution=0.40,
            )
        ]

        updated_score = calculator.calculate_relationship_confidence(
            ai_confidence=0.75,  # AI confidence might improve
            evidence_items=updated_evidence,
            last_evidence_at=now,
            interaction_count=2,
            entity_resolution_confidence=0.90,
        )

        # New evidence should increase confidence
        assert updated_score > initial_score

    def test_get_confidence_breakdown(
        self, calculator: ConfidenceCalculator
    ) -> None:
        """Test getting a breakdown of confidence factors."""
        now = datetime.now(timezone.utc)
        evidence_items = [
            EvidenceItem(
                evidence_type="email",
                source_id=uuid4(),
                observed_at=now,
                strength_contribution=0.25,
            )
        ]

        breakdown = calculator.get_confidence_breakdown(
            ai_confidence=0.80,
            evidence_items=evidence_items,
            last_evidence_at=now,
            interaction_count=3,
            entity_resolution_confidence=0.95,
        )

        assert "ai_confidence" in breakdown
        assert "evidence_strength" in breakdown
        assert "temporal_decay" in breakdown
        assert "interaction_boost" in breakdown
        assert "entity_resolution_confidence" in breakdown
        assert "final_score" in breakdown

        assert 0.0 <= breakdown["final_score"] <= 1.0

    def test_to_lifecycle_state(self, calculator: ConfidenceCalculator) -> None:
        """Test mapping confidence score to lifecycle state."""
        # High confidence -> confirmed
        assert calculator.to_lifecycle_state(0.95) == "active"

        # Good confidence -> probable
        assert calculator.to_lifecycle_state(0.75) == "active"

        # Medium confidence -> tentative
        assert calculator.to_lifecycle_state(0.55) == "pending"

        # Low confidence -> experimental
        assert calculator.to_lifecycle_state(0.35) == "pending"

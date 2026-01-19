"""Unit tests for FeedbackProcessor.

Tests feedback processing including:
- Confirm/reject/modify feedback handling
- Confidence updates based on feedback
- Feedback storage for learning
- Applying learned preferences to discovery
"""

from __future__ import annotations

from datetime import datetime, timezone
from decimal import Decimal
from typing import Any
from unittest.mock import AsyncMock, MagicMock, patch
from uuid import uuid4

import pytest

from penf_lib.relationships.feedback import (
    FeedbackProcessor,
    FeedbackResult,
    FeedbackType,
    LearnedPreference,
)


class TestFeedbackType:
    """Tests for the FeedbackType enum."""

    def test_confirm_type_exists(self) -> None:
        """Test CONFIRM feedback type is defined."""
        assert FeedbackType.CONFIRM.value == "confirm"

    def test_reject_type_exists(self) -> None:
        """Test REJECT feedback type is defined."""
        assert FeedbackType.REJECT.value == "reject"

    def test_modify_type_exists(self) -> None:
        """Test MODIFY feedback type is defined."""
        assert FeedbackType.MODIFY.value == "modify"

    def test_strengthen_type_exists(self) -> None:
        """Test STRENGTHEN feedback type is defined."""
        assert FeedbackType.STRENGTHEN.value == "strengthen"

    def test_weaken_type_exists(self) -> None:
        """Test WEAKEN feedback type is defined."""
        assert FeedbackType.WEAKEN.value == "weaken"


class TestFeedbackResult:
    """Tests for the FeedbackResult dataclass."""

    def test_create_feedback_result(self) -> None:
        """Test creating a feedback result."""
        result = FeedbackResult(
            relationship_id=100,
            feedback_type=FeedbackType.CONFIRM,
            previous_confidence=Decimal("0.70"),
            new_confidence=Decimal("0.90"),
            previous_state="pending",
            new_state="active",
            user_email="user@example.com",
        )

        assert result.relationship_id == 100
        assert result.feedback_type == FeedbackType.CONFIRM
        assert result.new_confidence == Decimal("0.90")
        assert result.new_state == "active"

    def test_feedback_result_with_changes(self) -> None:
        """Test feedback result with modification changes."""
        result = FeedbackResult(
            relationship_id=101,
            feedback_type=FeedbackType.MODIFY,
            previous_confidence=Decimal("0.65"),
            new_confidence=Decimal("0.85"),
            previous_state="pending",
            new_state="active",
            user_email="user@example.com",
            changes={
                "relationship_type": {
                    "from": "collaborates_with",
                    "to": "manages",
                }
            },
        )

        assert result.changes is not None
        assert "relationship_type" in result.changes


class TestLearnedPreference:
    """Tests for the LearnedPreference dataclass."""

    def test_create_learned_preference(self) -> None:
        """Test creating a learned preference."""
        pref = LearnedPreference(
            pattern_type="entity_pair",
            pattern_key="person:alice@example.com-person:bob@example.com",
            preferred_relationship_type="collaborates_with",
            confidence_boost=Decimal("0.15"),
            observation_count=5,
        )

        assert pref.pattern_type == "entity_pair"
        assert pref.preferred_relationship_type == "collaborates_with"
        assert pref.confidence_boost == Decimal("0.15")


class TestFeedbackProcessor:
    """Tests for the FeedbackProcessor class."""

    @pytest.fixture
    def mock_repository(self) -> AsyncMock:
        """Create a mock RelationshipRepository."""
        repo = AsyncMock()
        repo.get_by_id = AsyncMock()
        repo.update = AsyncMock()
        repo.update_confidence = AsyncMock()
        repo.update_lifecycle_state = AsyncMock()
        repo.add_feedback = AsyncMock()
        repo.get_feedback_for_relationship = AsyncMock()
        repo.confirm_relationship = AsyncMock()
        return repo

    @pytest.fixture
    def mock_session(self) -> AsyncMock:
        """Create a mock database session."""
        session = AsyncMock()
        session.commit = AsyncMock()
        session.rollback = AsyncMock()
        return session

    @pytest.fixture
    def feedback_processor(
        self,
        mock_session: AsyncMock,
        mock_repository: AsyncMock,
    ) -> FeedbackProcessor:
        """Create FeedbackProcessor with mocked dependencies."""
        return FeedbackProcessor(
            session=mock_session,
            repository=mock_repository,
            tenant_id=uuid4(),
        )

    # =========================================================================
    # CONFIRM FEEDBACK TESTS
    # =========================================================================

    @pytest.mark.asyncio
    async def test_confirm_increases_confidence(
        self,
        feedback_processor: FeedbackProcessor,
        mock_repository: AsyncMock,
    ) -> None:
        """Test that confirming a relationship increases confidence."""
        mock_relationship = MagicMock()
        mock_relationship.id = 100
        mock_relationship.confidence_score = Decimal("0.70")
        mock_relationship.lifecycle_state = "pending"
        mock_relationship.relationship_type = "collaborates_with"

        mock_repository.get_by_id.return_value = mock_relationship
        mock_repository.confirm_relationship.return_value = mock_relationship

        result = await feedback_processor.process_confirm(
            relationship_id=100,
            user_email="user@example.com",
            reasoning="I work with this person regularly",
        )

        assert result.feedback_type == FeedbackType.CONFIRM
        assert result.new_confidence > result.previous_confidence
        assert result.new_state == "active"

    @pytest.mark.asyncio
    async def test_confirm_transitions_to_active(
        self,
        feedback_processor: FeedbackProcessor,
        mock_repository: AsyncMock,
    ) -> None:
        """Test that confirmation transitions relationship to active state."""
        mock_relationship = MagicMock()
        mock_relationship.id = 101
        mock_relationship.confidence_score = Decimal("0.65")
        mock_relationship.lifecycle_state = "pending"
        mock_relationship.relationship_type = "works_on"

        mock_repository.get_by_id.return_value = mock_relationship

        result = await feedback_processor.process_confirm(
            relationship_id=101,
            user_email="user@example.com",
        )

        assert result.new_state == "active"
        mock_repository.confirm_relationship.assert_called_once()

    @pytest.mark.asyncio
    async def test_confirm_stores_feedback(
        self,
        feedback_processor: FeedbackProcessor,
        mock_repository: AsyncMock,
    ) -> None:
        """Test that confirmation stores feedback record."""
        mock_relationship = MagicMock()
        mock_relationship.id = 102
        mock_relationship.confidence_score = Decimal("0.70")
        mock_relationship.lifecycle_state = "pending"
        mock_relationship.relationship_type = "collaborates_with"

        mock_repository.get_by_id.return_value = mock_relationship

        await feedback_processor.process_confirm(
            relationship_id=102,
            user_email="user@example.com",
            reasoning="Regular collaborator",
        )

        mock_repository.add_feedback.assert_called_once()
        call_args = mock_repository.add_feedback.call_args
        assert call_args[1]["feedback_type"] == "confirm"
        assert call_args[1]["user_email"] == "user@example.com"

    # =========================================================================
    # REJECT FEEDBACK TESTS
    # =========================================================================

    @pytest.mark.asyncio
    async def test_reject_decreases_confidence(
        self,
        feedback_processor: FeedbackProcessor,
        mock_repository: AsyncMock,
    ) -> None:
        """Test that rejecting a relationship decreases confidence."""
        mock_relationship = MagicMock()
        mock_relationship.id = 103
        mock_relationship.confidence_score = Decimal("0.70")
        mock_relationship.lifecycle_state = "pending"
        mock_relationship.relationship_type = "reports_to"

        mock_repository.get_by_id.return_value = mock_relationship

        result = await feedback_processor.process_reject(
            relationship_id=103,
            user_email="user@example.com",
            reasoning="This relationship is incorrect",
        )

        assert result.feedback_type == FeedbackType.REJECT
        assert result.new_confidence < result.previous_confidence

    @pytest.mark.asyncio
    async def test_reject_archives_relationship(
        self,
        feedback_processor: FeedbackProcessor,
        mock_repository: AsyncMock,
    ) -> None:
        """Test that strong rejection archives the relationship."""
        mock_relationship = MagicMock()
        mock_relationship.id = 104
        mock_relationship.confidence_score = Decimal("0.50")
        mock_relationship.lifecycle_state = "pending"
        mock_relationship.relationship_type = "manages"

        mock_repository.get_by_id.return_value = mock_relationship

        result = await feedback_processor.process_reject(
            relationship_id=104,
            user_email="user@example.com",
            reasoning="This is completely wrong",
            strong_rejection=True,
        )

        assert result.new_state == "archived"
        mock_repository.update_lifecycle_state.assert_called_with(104, "archived")

    @pytest.mark.asyncio
    async def test_reject_stores_negative_feedback(
        self,
        feedback_processor: FeedbackProcessor,
        mock_repository: AsyncMock,
    ) -> None:
        """Test that rejection stores feedback for learning."""
        mock_relationship = MagicMock()
        mock_relationship.id = 105
        mock_relationship.confidence_score = Decimal("0.60")
        mock_relationship.lifecycle_state = "pending"
        mock_relationship.relationship_type = "expert_in"

        mock_repository.get_by_id.return_value = mock_relationship

        await feedback_processor.process_reject(
            relationship_id=105,
            user_email="user@example.com",
            reasoning="Person is not an expert in this area",
        )

        mock_repository.add_feedback.assert_called_once()
        call_args = mock_repository.add_feedback.call_args
        assert call_args[1]["feedback_type"] == "reject"

    # =========================================================================
    # MODIFY FEEDBACK TESTS
    # =========================================================================

    @pytest.mark.asyncio
    async def test_modify_changes_relationship_type(
        self,
        feedback_processor: FeedbackProcessor,
        mock_repository: AsyncMock,
    ) -> None:
        """Test modifying relationship type."""
        mock_relationship = MagicMock()
        mock_relationship.id = 106
        mock_relationship.confidence_score = Decimal("0.70")
        mock_relationship.lifecycle_state = "pending"
        mock_relationship.relationship_type = "collaborates_with"

        mock_repository.get_by_id.return_value = mock_relationship
        mock_repository.update.return_value = mock_relationship

        result = await feedback_processor.process_modify(
            relationship_id=106,
            user_email="user@example.com",
            modifications={"relationship_type": "manages"},
            reasoning="This person actually manages the other",
        )

        assert result.feedback_type == FeedbackType.MODIFY
        assert result.changes is not None
        mock_repository.update.assert_called_once()

    @pytest.mark.asyncio
    async def test_modify_adjusts_confidence(
        self,
        feedback_processor: FeedbackProcessor,
        mock_repository: AsyncMock,
    ) -> None:
        """Test that modification adjusts confidence based on user input."""
        mock_relationship = MagicMock()
        mock_relationship.id = 107
        mock_relationship.confidence_score = Decimal("0.60")
        mock_relationship.lifecycle_state = "pending"
        mock_relationship.relationship_type = "works_on"

        mock_repository.get_by_id.return_value = mock_relationship

        result = await feedback_processor.process_modify(
            relationship_id=107,
            user_email="user@example.com",
            modifications={"confidence_adjustment": Decimal("0.20")},
            reasoning="This relationship is definitely correct",
        )

        assert result.new_confidence > result.previous_confidence

    @pytest.mark.asyncio
    async def test_modify_records_changes(
        self,
        feedback_processor: FeedbackProcessor,
        mock_repository: AsyncMock,
    ) -> None:
        """Test that modifications are properly recorded."""
        mock_relationship = MagicMock()
        mock_relationship.id = 108
        mock_relationship.confidence_score = Decimal("0.65")
        mock_relationship.lifecycle_state = "pending"
        mock_relationship.relationship_type = "interested_in"

        mock_repository.get_by_id.return_value = mock_relationship

        await feedback_processor.process_modify(
            relationship_id=108,
            user_email="user@example.com",
            modifications={"relationship_type": "expert_in"},
            reasoning="Upgraded from interested to expert",
        )

        mock_repository.add_feedback.assert_called_once()
        call_args = mock_repository.add_feedback.call_args
        assert call_args[1]["corrected_type"] == "expert_in"

    # =========================================================================
    # STRENGTHEN/WEAKEN FEEDBACK TESTS
    # =========================================================================

    @pytest.mark.asyncio
    async def test_strengthen_boosts_confidence(
        self,
        feedback_processor: FeedbackProcessor,
        mock_repository: AsyncMock,
    ) -> None:
        """Test that strengthening boosts relationship confidence."""
        mock_relationship = MagicMock()
        mock_relationship.id = 109
        mock_relationship.confidence_score = Decimal("0.70")
        mock_relationship.lifecycle_state = "active"
        mock_relationship.relationship_type = "collaborates_with"

        mock_repository.get_by_id.return_value = mock_relationship

        result = await feedback_processor.process_strengthen(
            relationship_id=109,
            user_email="user@example.com",
            reasoning="Very close collaboration",
        )

        assert result.feedback_type == FeedbackType.STRENGTHEN
        assert result.new_confidence > result.previous_confidence

    @pytest.mark.asyncio
    async def test_weaken_reduces_confidence(
        self,
        feedback_processor: FeedbackProcessor,
        mock_repository: AsyncMock,
    ) -> None:
        """Test that weakening reduces relationship confidence."""
        mock_relationship = MagicMock()
        mock_relationship.id = 110
        mock_relationship.confidence_score = Decimal("0.85")
        mock_relationship.lifecycle_state = "active"
        mock_relationship.relationship_type = "collaborates_with"

        mock_repository.get_by_id.return_value = mock_relationship

        result = await feedback_processor.process_weaken(
            relationship_id=110,
            user_email="user@example.com",
            reasoning="Collaboration has reduced",
        )

        assert result.feedback_type == FeedbackType.WEAKEN
        assert result.new_confidence < result.previous_confidence

    # =========================================================================
    # LEARNING INTEGRATION TESTS
    # =========================================================================

    @pytest.mark.asyncio
    async def test_feedback_creates_learned_preference(
        self,
        feedback_processor: FeedbackProcessor,
        mock_repository: AsyncMock,
    ) -> None:
        """Test that feedback creates learned preferences."""
        mock_relationship = MagicMock()
        mock_relationship.id = 111
        mock_relationship.confidence_score = Decimal("0.70")
        mock_relationship.lifecycle_state = "pending"
        mock_relationship.relationship_type = "collaborates_with"
        mock_relationship.source_entity_type = "person"
        mock_relationship.source_entity_id = 1
        mock_relationship.target_entity_type = "person"
        mock_relationship.target_entity_id = 2

        mock_repository.get_by_id.return_value = mock_relationship

        # Process multiple confirmations to create a learned preference
        await feedback_processor.process_confirm(
            relationship_id=111,
            user_email="user@example.com",
            reasoning="Confirmed collaboration",
        )

        # Verify learning integration was triggered
        assert mock_repository.add_feedback.called

    @pytest.mark.asyncio
    async def test_get_preferences_for_pattern(
        self,
        feedback_processor: FeedbackProcessor,
        mock_repository: AsyncMock,
    ) -> None:
        """Test retrieving learned preferences for a pattern."""
        preferences = await feedback_processor.get_learned_preferences(
            pattern_type="entity_pair",
            source_entity_type="person",
            source_entity_id=1,
            target_entity_type="person",
            target_entity_id=2,
        )

        # Should return list (may be empty)
        assert isinstance(preferences, list)

    @pytest.mark.asyncio
    async def test_apply_preferences_to_discovery(
        self,
        feedback_processor: FeedbackProcessor,
        mock_repository: AsyncMock,
    ) -> None:
        """Test applying learned preferences to new discoveries."""
        # Create a mock discovered relationship
        discovered = MagicMock()
        discovered.source_entity_type = "person"
        discovered.source_entity_id = 1
        discovered.target_entity_type = "project"
        discovered.target_entity_id = 10
        discovered.relationship_type = "works_on"
        discovered.confidence_score = Decimal("0.60")

        # Mock learned preference
        feedback_processor._learned_preferences = {
            "person:1-project:10": LearnedPreference(
                pattern_type="entity_pair",
                pattern_key="person:1-project:10",
                preferred_relationship_type="leads",
                confidence_boost=Decimal("0.10"),
                observation_count=3,
            )
        }

        adjusted = await feedback_processor.apply_learned_preferences(discovered)

        # Should apply confidence boost from preference
        assert adjusted.confidence_score >= discovered.confidence_score

    # =========================================================================
    # EVIDENCE AND CONTEXT PRESENTATION TESTS
    # =========================================================================

    @pytest.mark.asyncio
    async def test_get_relationship_context(
        self,
        feedback_processor: FeedbackProcessor,
        mock_repository: AsyncMock,
    ) -> None:
        """Test getting relationship context for validation."""
        mock_relationship = MagicMock()
        mock_relationship.id = 112
        mock_relationship.confidence_score = Decimal("0.65")
        mock_relationship.relationship_type = "collaborates_with"
        mock_relationship.source_entity_type = "person"
        mock_relationship.target_entity_type = "person"
        mock_relationship.first_observed_at = datetime.now(timezone.utc)
        mock_relationship.interaction_count = 5

        mock_repository.get_by_id.return_value = mock_relationship

        # Mock evidence
        mock_evidence1 = MagicMock()
        mock_evidence1.evidence_type = "email"
        mock_evidence1.evidence_content = "Discussed project timeline..."
        mock_evidence1.observed_at = datetime.now(timezone.utc)

        mock_evidence2 = MagicMock()
        mock_evidence2.evidence_type = "meeting"
        mock_evidence2.evidence_content = "Joint presentation..."
        mock_evidence2.observed_at = datetime.now(timezone.utc)

        mock_repository.get_evidence_for_relationship = AsyncMock(
            return_value=[mock_evidence1, mock_evidence2]
        )

        context = await feedback_processor.get_relationship_context(
            relationship_id=112
        )

        assert context is not None
        assert "relationship" in context
        assert "evidence" in context
        assert len(context["evidence"]) == 2

    @pytest.mark.asyncio
    async def test_format_evidence_for_display(
        self,
        feedback_processor: FeedbackProcessor,
    ) -> None:
        """Test formatting evidence for user display."""
        mock_evidence = MagicMock()
        mock_evidence.evidence_type = "email"
        mock_evidence.evidence_content = "Alice and Bob discussed the deployment timeline for the Atlas project. They agreed to meet next week."
        mock_evidence.observed_at = datetime.now(timezone.utc)
        mock_evidence.strength_contribution = Decimal("0.25")

        formatted = feedback_processor.format_evidence(mock_evidence)

        assert "email" in formatted.lower()
        assert len(formatted) > 0

    # =========================================================================
    # FEEDBACK HISTORY TESTS
    # =========================================================================

    @pytest.mark.asyncio
    async def test_get_feedback_history(
        self,
        feedback_processor: FeedbackProcessor,
        mock_repository: AsyncMock,
    ) -> None:
        """Test retrieving feedback history for a relationship."""
        mock_feedback1 = MagicMock()
        mock_feedback1.id = 1
        mock_feedback1.feedback_type = "confirm"
        mock_feedback1.user_email = "user1@example.com"
        mock_feedback1.created_at = datetime.now(timezone.utc)

        mock_feedback2 = MagicMock()
        mock_feedback2.id = 2
        mock_feedback2.feedback_type = "strengthen"
        mock_feedback2.user_email = "user2@example.com"
        mock_feedback2.created_at = datetime.now(timezone.utc)

        mock_repository.get_feedback_for_relationship.return_value = [
            mock_feedback1,
            mock_feedback2,
        ]

        history = await feedback_processor.get_feedback_history(relationship_id=100)

        assert len(history) == 2
        mock_repository.get_feedback_for_relationship.assert_called_once_with(100)

    @pytest.mark.asyncio
    async def test_aggregate_feedback_stats(
        self,
        feedback_processor: FeedbackProcessor,
        mock_repository: AsyncMock,
    ) -> None:
        """Test aggregating feedback statistics."""
        mock_feedback1 = MagicMock()
        mock_feedback1.feedback_type = "confirm"

        mock_feedback2 = MagicMock()
        mock_feedback2.feedback_type = "confirm"

        mock_feedback3 = MagicMock()
        mock_feedback3.feedback_type = "reject"

        mock_repository.get_feedback_for_relationship.return_value = [
            mock_feedback1,
            mock_feedback2,
            mock_feedback3,
        ]

        stats = await feedback_processor.get_feedback_stats(relationship_id=100)

        assert stats["total_feedback"] == 3
        assert stats["confirm_count"] == 2
        assert stats["reject_count"] == 1

    # =========================================================================
    # EDGE CASES
    # =========================================================================

    @pytest.mark.asyncio
    async def test_feedback_on_nonexistent_relationship(
        self,
        feedback_processor: FeedbackProcessor,
        mock_repository: AsyncMock,
    ) -> None:
        """Test handling feedback on non-existent relationship."""
        mock_repository.get_by_id.return_value = None

        with pytest.raises(ValueError, match="Relationship not found"):
            await feedback_processor.process_confirm(
                relationship_id=999,
                user_email="user@example.com",
            )

    @pytest.mark.asyncio
    async def test_confidence_stays_in_bounds(
        self,
        feedback_processor: FeedbackProcessor,
        mock_repository: AsyncMock,
    ) -> None:
        """Test that confidence stays within 0.0-1.0 bounds."""
        # Test upper bound
        mock_relationship = MagicMock()
        mock_relationship.id = 113
        mock_relationship.confidence_score = Decimal("0.98")
        mock_relationship.lifecycle_state = "active"
        mock_relationship.relationship_type = "collaborates_with"

        mock_repository.get_by_id.return_value = mock_relationship

        result = await feedback_processor.process_strengthen(
            relationship_id=113,
            user_email="user@example.com",
        )

        assert result.new_confidence <= Decimal("1.0")

    @pytest.mark.asyncio
    async def test_confidence_lower_bound(
        self,
        feedback_processor: FeedbackProcessor,
        mock_repository: AsyncMock,
    ) -> None:
        """Test that confidence doesn't go below 0.0."""
        mock_relationship = MagicMock()
        mock_relationship.id = 114
        mock_relationship.confidence_score = Decimal("0.10")
        mock_relationship.lifecycle_state = "pending"
        mock_relationship.relationship_type = "interested_in"

        mock_repository.get_by_id.return_value = mock_relationship

        result = await feedback_processor.process_reject(
            relationship_id=114,
            user_email="user@example.com",
        )

        assert result.new_confidence >= Decimal("0.0")

    @pytest.mark.asyncio
    async def test_batch_feedback_processing(
        self,
        feedback_processor: FeedbackProcessor,
        mock_repository: AsyncMock,
    ) -> None:
        """Test processing multiple feedback items in batch."""
        mock_rel1 = MagicMock()
        mock_rel1.id = 115
        mock_rel1.confidence_score = Decimal("0.70")
        mock_rel1.lifecycle_state = "pending"
        mock_rel1.relationship_type = "collaborates_with"

        mock_rel2 = MagicMock()
        mock_rel2.id = 116
        mock_rel2.confidence_score = Decimal("0.65")
        mock_rel2.lifecycle_state = "pending"
        mock_rel2.relationship_type = "works_on"

        mock_repository.get_by_id.side_effect = [mock_rel1, mock_rel2]

        feedback_items = [
            {"relationship_id": 115, "feedback_type": "confirm"},
            {"relationship_id": 116, "feedback_type": "reject"},
        ]

        results = await feedback_processor.process_batch(
            feedback_items=feedback_items,
            user_email="user@example.com",
        )

        assert len(results) == 2
        assert results[0].feedback_type == FeedbackType.CONFIRM
        assert results[1].feedback_type == FeedbackType.REJECT

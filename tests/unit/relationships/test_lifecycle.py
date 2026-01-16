"""Unit tests for LifecycleManager.

Tests lifecycle management including:
- State machine transitions (pending -> active -> historical -> archived)
- Transition rules and validations
- Evidence-based confidence decay
- Re-activation on new evidence
- Retention policy enforcement
- Inactivity detection
- Version history tracking
"""

from __future__ import annotations

from datetime import datetime, timedelta, timezone
from decimal import Decimal
from unittest.mock import AsyncMock, MagicMock
from uuid import uuid4

import pytest

from penf_lib.relationships.lifecycle import (
    INACTIVITY_THRESHOLD_DAYS,
    RETENTION_YEARS,
    LifecycleManager,
    LifecycleTransition,
    MaintenanceResult,
    StateTransitionError,
    TransitionReason,
)


class TestLifecycleTransition:
    """Tests for the LifecycleTransition dataclass."""

    def test_create_transition(self) -> None:
        """Test creating a lifecycle transition."""
        transition = LifecycleTransition(
            relationship_id=100,
            from_state="pending",
            to_state="active",
            reason=TransitionReason.USER_CONFIRMED,
            triggered_by="user@example.com",
        )

        assert transition.relationship_id == 100
        assert transition.from_state == "pending"
        assert transition.to_state == "active"
        assert transition.reason == TransitionReason.USER_CONFIRMED
        assert transition.triggered_by == "user@example.com"

    def test_transition_with_metadata(self) -> None:
        """Test creating a transition with metadata."""
        transition = LifecycleTransition(
            relationship_id=101,
            from_state="active",
            to_state="historical",
            reason=TransitionReason.INACTIVITY,
            triggered_by="system",
            metadata={"days_inactive": 95},
        )

        assert transition.metadata is not None
        assert transition.metadata.get("days_inactive") == 95


class TestMaintenanceResult:
    """Tests for the MaintenanceResult dataclass."""

    def test_create_maintenance_result(self) -> None:
        """Test creating a maintenance result."""
        result = MaintenanceResult(
            total_processed=100,
            transitions_applied=5,
            confidence_recalculated=20,
            archived_count=2,
            reactivated_count=1,
        )

        assert result.total_processed == 100
        assert result.transitions_applied == 5
        assert result.confidence_recalculated == 20
        assert result.archived_count == 2
        assert result.reactivated_count == 1


class TestLifecycleManager:
    """Tests for the LifecycleManager class."""

    @pytest.fixture
    def mock_repository(self) -> AsyncMock:
        """Create a mock RelationshipRepository."""
        repo = AsyncMock()
        repo.get_by_id = AsyncMock()
        repo.update = AsyncMock()
        repo.update_lifecycle_state = AsyncMock()
        repo.update_confidence = AsyncMock()
        repo.find_inactive = AsyncMock(return_value=[])
        repo.find_stale = AsyncMock(return_value=[])
        repo.find_by_state = AsyncMock(return_value=[])
        repo.add_version = AsyncMock()
        repo.get_evidence_for_relationship = AsyncMock(return_value=[])
        return repo

    @pytest.fixture
    def mock_session(self) -> AsyncMock:
        """Create a mock database session."""
        session = AsyncMock()
        session.commit = AsyncMock()
        session.rollback = AsyncMock()
        return session

    @pytest.fixture
    def lifecycle_manager(
        self,
        mock_session: AsyncMock,
        mock_repository: AsyncMock,
    ) -> LifecycleManager:
        """Create LifecycleManager with mocked dependencies."""
        return LifecycleManager(
            session=mock_session,
            repository=mock_repository,
            tenant_id=uuid4(),
        )

    # =========================================================================
    # STATE MACHINE TESTS - Valid Transitions
    # =========================================================================

    @pytest.mark.asyncio
    async def test_transition_pending_to_active(
        self,
        lifecycle_manager: LifecycleManager,
        mock_repository: AsyncMock,
    ) -> None:
        """Test transitioning from pending to active."""
        mock_relationship = MagicMock()
        mock_relationship.id = 100
        mock_relationship.lifecycle_state = "pending"
        mock_relationship.confidence_score = Decimal("0.75")

        mock_repository.get_by_id.return_value = mock_relationship

        result = await lifecycle_manager.transition_state(
            relationship_id=100,
            to_state="active",
            reason=TransitionReason.USER_CONFIRMED,
            triggered_by="user@example.com",
        )

        assert result.to_state == "active"
        mock_repository.update_lifecycle_state.assert_called_once_with(100, "active")

    @pytest.mark.asyncio
    async def test_transition_active_to_historical(
        self,
        lifecycle_manager: LifecycleManager,
        mock_repository: AsyncMock,
    ) -> None:
        """Test transitioning from active to historical."""
        mock_relationship = MagicMock()
        mock_relationship.id = 101
        mock_relationship.lifecycle_state = "active"
        mock_relationship.confidence_score = Decimal("0.60")

        mock_repository.get_by_id.return_value = mock_relationship

        result = await lifecycle_manager.transition_state(
            relationship_id=101,
            to_state="historical",
            reason=TransitionReason.ROLE_CHANGE,
            triggered_by="system",
        )

        assert result.to_state == "historical"

    @pytest.mark.asyncio
    async def test_transition_historical_to_archived(
        self,
        lifecycle_manager: LifecycleManager,
        mock_repository: AsyncMock,
    ) -> None:
        """Test transitioning from historical to archived."""
        mock_relationship = MagicMock()
        mock_relationship.id = 102
        mock_relationship.lifecycle_state = "historical"
        mock_relationship.confidence_score = Decimal("0.30")

        mock_repository.get_by_id.return_value = mock_relationship

        result = await lifecycle_manager.transition_state(
            relationship_id=102,
            to_state="archived",
            reason=TransitionReason.RETENTION_EXPIRED,
            triggered_by="system",
        )

        assert result.to_state == "archived"

    # =========================================================================
    # STATE MACHINE TESTS - Invalid Transitions
    # =========================================================================

    @pytest.mark.asyncio
    async def test_invalid_transition_archived_to_active(
        self,
        lifecycle_manager: LifecycleManager,
        mock_repository: AsyncMock,
    ) -> None:
        """Test that archived relationships cannot transition directly to active."""
        mock_relationship = MagicMock()
        mock_relationship.id = 103
        mock_relationship.lifecycle_state = "archived"

        mock_repository.get_by_id.return_value = mock_relationship

        with pytest.raises(StateTransitionError, match="Invalid transition"):
            await lifecycle_manager.transition_state(
                relationship_id=103,
                to_state="active",
                reason=TransitionReason.USER_CONFIRMED,
                triggered_by="user@example.com",
            )

    @pytest.mark.asyncio
    async def test_invalid_transition_pending_to_archived(
        self,
        lifecycle_manager: LifecycleManager,
        mock_repository: AsyncMock,
    ) -> None:
        """Test that pending relationships cannot transition directly to archived."""
        mock_relationship = MagicMock()
        mock_relationship.id = 104
        mock_relationship.lifecycle_state = "pending"

        mock_repository.get_by_id.return_value = mock_relationship

        with pytest.raises(StateTransitionError, match="Invalid transition"):
            await lifecycle_manager.transition_state(
                relationship_id=104,
                to_state="archived",
                reason=TransitionReason.RETENTION_EXPIRED,
                triggered_by="system",
            )

    # =========================================================================
    # RE-ACTIVATION TESTS
    # =========================================================================

    @pytest.mark.asyncio
    async def test_reactivate_historical_relationship(
        self,
        lifecycle_manager: LifecycleManager,
        mock_repository: AsyncMock,
    ) -> None:
        """Test re-activating a historical relationship with new evidence."""
        mock_relationship = MagicMock()
        mock_relationship.id = 105
        mock_relationship.lifecycle_state = "historical"
        mock_relationship.confidence_score = Decimal("0.50")

        mock_repository.get_by_id.return_value = mock_relationship

        result = await lifecycle_manager.reactivate(
            relationship_id=105,
            reason="New evidence discovered",
            triggered_by="system",
        )

        assert result.to_state == "active"
        assert result.reason == TransitionReason.NEW_EVIDENCE

    @pytest.mark.asyncio
    async def test_cannot_reactivate_archived_relationship(
        self,
        lifecycle_manager: LifecycleManager,
        mock_repository: AsyncMock,
    ) -> None:
        """Test that archived relationships cannot be reactivated directly."""
        mock_relationship = MagicMock()
        mock_relationship.id = 106
        mock_relationship.lifecycle_state = "archived"

        mock_repository.get_by_id.return_value = mock_relationship

        with pytest.raises(StateTransitionError, match="Cannot reactivate"):
            await lifecycle_manager.reactivate(
                relationship_id=106,
                reason="New evidence",
                triggered_by="system",
            )

    # =========================================================================
    # INACTIVITY DETECTION TESTS
    # =========================================================================

    @pytest.mark.asyncio
    async def test_detect_inactive_relationships(
        self,
        lifecycle_manager: LifecycleManager,
        mock_repository: AsyncMock,
    ) -> None:
        """Test detecting relationships inactive for 90+ days."""
        # Create mock inactive relationships
        inactive_rel1 = MagicMock()
        inactive_rel1.id = 107
        inactive_rel1.lifecycle_state = "active"
        inactive_rel1.last_observed_at = datetime.now(timezone.utc) - timedelta(
            days=95
        )

        inactive_rel2 = MagicMock()
        inactive_rel2.id = 108
        inactive_rel2.lifecycle_state = "active"
        inactive_rel2.last_observed_at = datetime.now(timezone.utc) - timedelta(
            days=120
        )

        mock_repository.find_inactive.return_value = [inactive_rel1, inactive_rel2]

        inactive = await lifecycle_manager.find_inactive_relationships(
            threshold_days=INACTIVITY_THRESHOLD_DAYS
        )

        assert len(inactive) == 2
        mock_repository.find_inactive.assert_called_once()

    @pytest.mark.asyncio
    async def test_transition_inactive_to_historical(
        self,
        lifecycle_manager: LifecycleManager,
        mock_repository: AsyncMock,
    ) -> None:
        """Test that inactive relationships are transitioned to historical."""
        mock_relationship = MagicMock()
        mock_relationship.id = 109
        mock_relationship.lifecycle_state = "active"
        mock_relationship.last_observed_at = datetime.now(timezone.utc) - timedelta(
            days=100
        )
        mock_relationship.confidence_score = Decimal("0.70")

        mock_repository.get_by_id.return_value = mock_relationship
        mock_repository.find_inactive.return_value = [mock_relationship]

        result = await lifecycle_manager.process_inactive_relationships()

        assert result.transitions_applied >= 1
        mock_repository.update_lifecycle_state.assert_called()

    # =========================================================================
    # RETENTION POLICY TESTS
    # =========================================================================

    @pytest.mark.asyncio
    async def test_detect_stale_for_archival(
        self,
        lifecycle_manager: LifecycleManager,
        mock_repository: AsyncMock,
    ) -> None:
        """Test detecting relationships past 2-year retention."""
        stale_rel = MagicMock()
        stale_rel.id = 110
        stale_rel.lifecycle_state = "historical"
        stale_rel.last_observed_at = datetime.now(timezone.utc) - timedelta(
            days=365 * 2 + 30
        )  # 2 years + 30 days

        mock_repository.find_stale.return_value = [stale_rel]

        stale = await lifecycle_manager.find_stale_relationships(
            retention_years=RETENTION_YEARS
        )

        assert len(stale) == 1
        mock_repository.find_stale.assert_called_once()

    @pytest.mark.asyncio
    async def test_archive_stale_relationships(
        self,
        lifecycle_manager: LifecycleManager,
        mock_repository: AsyncMock,
    ) -> None:
        """Test archiving relationships past retention period."""
        stale_rel = MagicMock()
        stale_rel.id = 111
        stale_rel.lifecycle_state = "historical"
        stale_rel.confidence_score = Decimal("0.30")
        stale_rel.last_observed_at = datetime.now(timezone.utc) - timedelta(
            days=365 * 2 + 30
        )

        mock_repository.get_by_id.return_value = stale_rel
        mock_repository.find_stale.return_value = [stale_rel]

        result = await lifecycle_manager.archive_stale_relationships()

        assert result.archived_count >= 1
        mock_repository.update_lifecycle_state.assert_called()

    # =========================================================================
    # CONFIDENCE DECAY TESTS
    # =========================================================================

    @pytest.mark.asyncio
    async def test_apply_temporal_decay(
        self,
        lifecycle_manager: LifecycleManager,
        mock_repository: AsyncMock,
    ) -> None:
        """Test applying temporal decay to relationship confidence."""
        mock_relationship = MagicMock()
        mock_relationship.id = 112
        mock_relationship.lifecycle_state = "active"
        mock_relationship.confidence_score = Decimal("0.80")
        mock_relationship.last_observed_at = datetime.now(timezone.utc) - timedelta(
            days=200
        )

        mock_repository.get_by_id.return_value = mock_relationship

        new_confidence = await lifecycle_manager.recalculate_confidence(
            relationship_id=112
        )

        # Confidence should decrease due to temporal decay
        assert new_confidence < Decimal("0.80")
        mock_repository.update_confidence.assert_called_once()

    @pytest.mark.asyncio
    async def test_confidence_decay_respects_minimum(
        self,
        lifecycle_manager: LifecycleManager,
        mock_repository: AsyncMock,
    ) -> None:
        """Test that confidence decay doesn't go below minimum threshold."""
        mock_relationship = MagicMock()
        mock_relationship.id = 113
        mock_relationship.lifecycle_state = "active"
        mock_relationship.confidence_score = Decimal("0.50")
        # Very old - 2 years without interaction
        mock_relationship.last_observed_at = datetime.now(timezone.utc) - timedelta(
            days=730
        )

        mock_repository.get_by_id.return_value = mock_relationship

        new_confidence = await lifecycle_manager.recalculate_confidence(
            relationship_id=113
        )

        # Confidence should not go below minimum
        assert new_confidence >= Decimal("0.05")

    @pytest.mark.asyncio
    async def test_confidence_boost_on_new_evidence(
        self,
        lifecycle_manager: LifecycleManager,
        mock_repository: AsyncMock,
    ) -> None:
        """Test that new evidence boosts confidence and resets decay."""
        mock_relationship = MagicMock()
        mock_relationship.id = 114
        mock_relationship.lifecycle_state = "active"
        mock_relationship.confidence_score = Decimal("0.60")
        mock_relationship.last_observed_at = datetime.now(timezone.utc) - timedelta(
            days=100
        )

        mock_repository.get_by_id.return_value = mock_relationship

        # Simulate adding new evidence
        new_evidence = MagicMock()
        new_evidence.evidence_type = "email"
        new_evidence.strength_contribution = Decimal("0.15")
        new_evidence.observed_at = datetime.now(timezone.utc)

        mock_repository.get_evidence_for_relationship.return_value = [new_evidence]

        new_confidence = await lifecycle_manager.recalculate_confidence_with_evidence(
            relationship_id=114,
            new_evidence=new_evidence,
        )

        # Confidence should increase due to new evidence
        assert new_confidence > Decimal("0.60")

    # =========================================================================
    # PROJECT COMPLETION TESTS
    # =========================================================================

    @pytest.mark.asyncio
    async def test_project_completion_transitions_relationships(
        self,
        lifecycle_manager: LifecycleManager,
        mock_repository: AsyncMock,
    ) -> None:
        """Test that project completion transitions related relationships."""
        project_id = uuid4()

        rel1 = MagicMock()
        rel1.id = 115
        rel1.lifecycle_state = "active"
        rel1.project_id = project_id
        rel1.confidence_score = Decimal("0.80")

        rel2 = MagicMock()
        rel2.id = 116
        rel2.lifecycle_state = "active"
        rel2.project_id = project_id
        rel2.confidence_score = Decimal("0.75")

        mock_repository.find_by_project.return_value = [rel1, rel2]
        mock_repository.get_by_id.side_effect = [rel1, rel2]

        result = await lifecycle_manager.handle_project_completion(project_id)

        assert result.transitions_applied == 2
        assert mock_repository.update_lifecycle_state.call_count == 2

    # =========================================================================
    # ROLE CHANGE TESTS
    # =========================================================================

    @pytest.mark.asyncio
    async def test_role_change_marks_relationships_historical(
        self,
        lifecycle_manager: LifecycleManager,
        mock_repository: AsyncMock,
    ) -> None:
        """Test that role changes mark related relationships as historical."""
        person_id = 42

        rel1 = MagicMock()
        rel1.id = 117
        rel1.lifecycle_state = "active"
        rel1.relationship_type = "reports_to"
        rel1.source_entity_id = person_id
        rel1.confidence_score = Decimal("0.85")

        mock_repository.find_by_entity.return_value = [rel1]
        mock_repository.get_by_id.return_value = rel1

        result = await lifecycle_manager.handle_role_change(
            entity_id=person_id,
            affected_relationship_types=["reports_to", "manages"],
        )

        assert result.transitions_applied >= 1

    # =========================================================================
    # VERSION HISTORY TESTS
    # =========================================================================

    @pytest.mark.asyncio
    async def test_transition_creates_version(
        self,
        lifecycle_manager: LifecycleManager,
        mock_repository: AsyncMock,
    ) -> None:
        """Test that state transitions create version records."""
        mock_relationship = MagicMock()
        mock_relationship.id = 118
        mock_relationship.lifecycle_state = "pending"
        mock_relationship.confidence_score = Decimal("0.70")
        mock_relationship.relationship_type = "collaborates_with"

        mock_repository.get_by_id.return_value = mock_relationship

        await lifecycle_manager.transition_state(
            relationship_id=118,
            to_state="active",
            reason=TransitionReason.USER_CONFIRMED,
            triggered_by="user@example.com",
        )

        # Verify version was created
        mock_repository.add_version.assert_called_once()
        call_args = mock_repository.add_version.call_args
        assert call_args[1]["relationship_id"] == 118
        assert call_args[1]["change_type"] == "state_transition"

    # =========================================================================
    # MAINTENANCE SCHEDULER TESTS
    # =========================================================================

    @pytest.mark.asyncio
    async def test_run_full_maintenance(
        self,
        lifecycle_manager: LifecycleManager,
        mock_repository: AsyncMock,
    ) -> None:
        """Test running full maintenance cycle."""
        # Setup mock returns
        mock_repository.find_inactive.return_value = []
        mock_repository.find_stale.return_value = []
        mock_repository.find_by_state.return_value = []

        result = await lifecycle_manager.run_maintenance()

        assert result is not None
        assert isinstance(result, MaintenanceResult)

    @pytest.mark.asyncio
    async def test_maintenance_handles_mixed_states(
        self,
        lifecycle_manager: LifecycleManager,
        mock_repository: AsyncMock,
    ) -> None:
        """Test maintenance correctly handles various relationship states."""
        inactive_rel = MagicMock()
        inactive_rel.id = 119
        inactive_rel.lifecycle_state = "active"
        inactive_rel.confidence_score = Decimal("0.60")
        inactive_rel.last_observed_at = datetime.now(timezone.utc) - timedelta(days=100)

        stale_rel = MagicMock()
        stale_rel.id = 120
        stale_rel.lifecycle_state = "historical"
        stale_rel.confidence_score = Decimal("0.30")
        stale_rel.last_observed_at = datetime.now(timezone.utc) - timedelta(
            days=365 * 2 + 30
        )

        mock_repository.find_inactive.return_value = [inactive_rel]
        mock_repository.find_stale.return_value = [stale_rel]
        mock_repository.get_by_id.side_effect = lambda x: (
            inactive_rel if x == 119 else stale_rel
        )

        result = await lifecycle_manager.run_maintenance()

        # Should have processed both types
        assert result.total_processed >= 2

    # =========================================================================
    # EDGE CASES
    # =========================================================================

    @pytest.mark.asyncio
    async def test_transition_nonexistent_relationship(
        self,
        lifecycle_manager: LifecycleManager,
        mock_repository: AsyncMock,
    ) -> None:
        """Test handling transition on non-existent relationship."""
        mock_repository.get_by_id.return_value = None

        with pytest.raises(ValueError, match="Relationship not found"):
            await lifecycle_manager.transition_state(
                relationship_id=999,
                to_state="active",
                reason=TransitionReason.USER_CONFIRMED,
                triggered_by="user@example.com",
            )

    @pytest.mark.asyncio
    async def test_transition_to_same_state(
        self,
        lifecycle_manager: LifecycleManager,
        mock_repository: AsyncMock,
    ) -> None:
        """Test handling transition to same state (no-op)."""
        mock_relationship = MagicMock()
        mock_relationship.id = 121
        mock_relationship.lifecycle_state = "active"
        mock_relationship.confidence_score = Decimal("0.75")

        mock_repository.get_by_id.return_value = mock_relationship

        result = await lifecycle_manager.transition_state(
            relationship_id=121,
            to_state="active",
            reason=TransitionReason.USER_CONFIRMED,
            triggered_by="user@example.com",
        )

        # Should return transition but not update
        assert result.from_state == result.to_state
        mock_repository.update_lifecycle_state.assert_not_called()

    # =========================================================================
    # VALIDATION HELPER TESTS
    # =========================================================================

    def test_is_valid_transition_pending_to_active(
        self,
        lifecycle_manager: LifecycleManager,
    ) -> None:
        """Test that pending -> active is a valid transition."""
        assert lifecycle_manager.is_valid_transition("pending", "active") is True

    def test_is_valid_transition_active_to_historical(
        self,
        lifecycle_manager: LifecycleManager,
    ) -> None:
        """Test that active -> historical is a valid transition."""
        assert lifecycle_manager.is_valid_transition("active", "historical") is True

    def test_is_valid_transition_historical_to_archived(
        self,
        lifecycle_manager: LifecycleManager,
    ) -> None:
        """Test that historical -> archived is a valid transition."""
        assert lifecycle_manager.is_valid_transition("historical", "archived") is True

    def test_is_invalid_transition_archived_to_active(
        self,
        lifecycle_manager: LifecycleManager,
    ) -> None:
        """Test that archived -> active is not a valid direct transition."""
        assert lifecycle_manager.is_valid_transition("archived", "active") is False

    def test_is_valid_transition_historical_to_active_reactivation(
        self,
        lifecycle_manager: LifecycleManager,
    ) -> None:
        """Test that historical -> active is valid (reactivation)."""
        assert lifecycle_manager.is_valid_transition("historical", "active") is True


class TestTransitionReason:
    """Tests for the TransitionReason enum."""

    def test_user_confirmed_reason(self) -> None:
        """Test USER_CONFIRMED transition reason."""
        assert TransitionReason.USER_CONFIRMED.value == "user_confirmed"

    def test_inactivity_reason(self) -> None:
        """Test INACTIVITY transition reason."""
        assert TransitionReason.INACTIVITY.value == "inactivity"

    def test_retention_expired_reason(self) -> None:
        """Test RETENTION_EXPIRED transition reason."""
        assert TransitionReason.RETENTION_EXPIRED.value == "retention_expired"

    def test_new_evidence_reason(self) -> None:
        """Test NEW_EVIDENCE transition reason."""
        assert TransitionReason.NEW_EVIDENCE.value == "new_evidence"

    def test_role_change_reason(self) -> None:
        """Test ROLE_CHANGE transition reason."""
        assert TransitionReason.ROLE_CHANGE.value == "role_change"

    def test_project_completed_reason(self) -> None:
        """Test PROJECT_COMPLETED transition reason."""
        assert TransitionReason.PROJECT_COMPLETED.value == "project_completed"

    def test_evidence_decay_reason(self) -> None:
        """Test EVIDENCE_DECAY transition reason."""
        assert TransitionReason.EVIDENCE_DECAY.value == "evidence_decay"

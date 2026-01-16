"""Unit tests for ConflictResolver.

Tests conflict resolution including:
- Auto-resolution when confidence gap > 30%
- User escalation for close conflicts
- Resolution tracking and audit trail
- Conflict type handling
"""

from __future__ import annotations

from datetime import datetime, timezone
from decimal import Decimal
from typing import Any
from unittest.mock import AsyncMock, MagicMock
from uuid import uuid4

import pytest

from penf_lib.relationships.conflict_resolver import (
    ConflictResolver,
    ConflictResolutionResult,
    ResolutionStrategy,
)


class TestResolutionStrategy:
    """Tests for the ResolutionStrategy enum."""

    def test_auto_resolve_strategy_exists(self) -> None:
        """Test AUTO_RESOLVE strategy is defined."""
        assert ResolutionStrategy.AUTO_RESOLVE.value == "auto_resolve"

    def test_user_resolve_strategy_exists(self) -> None:
        """Test USER_RESOLVE strategy is defined."""
        assert ResolutionStrategy.USER_RESOLVE.value == "user_resolve"

    def test_merge_strategy_exists(self) -> None:
        """Test MERGE strategy is defined."""
        assert ResolutionStrategy.MERGE.value == "merge"

    def test_coexist_strategy_exists(self) -> None:
        """Test COEXIST strategy is defined."""
        assert ResolutionStrategy.COEXIST.value == "coexist"


class TestConflictResolutionResult:
    """Tests for the ConflictResolutionResult dataclass."""

    def test_create_result(self) -> None:
        """Test creating a conflict resolution result."""
        result = ConflictResolutionResult(
            conflict_id=1,
            strategy=ResolutionStrategy.AUTO_RESOLVE,
            winning_relationship_id=100,
            losing_relationship_id=101,
            confidence_gap=Decimal("0.35"),
            reasoning="Confidence gap > 30%, auto-resolved",
            resolved_by="system",
        )

        assert result.conflict_id == 1
        assert result.strategy == ResolutionStrategy.AUTO_RESOLVE
        assert result.winning_relationship_id == 100
        assert result.confidence_gap == Decimal("0.35")

    def test_result_without_winner(self) -> None:
        """Test result for coexist strategy (no winner)."""
        result = ConflictResolutionResult(
            conflict_id=2,
            strategy=ResolutionStrategy.COEXIST,
            winning_relationship_id=None,
            losing_relationship_id=None,
            confidence_gap=Decimal("0.05"),
            reasoning="Both relationships are valid in different contexts",
            resolved_by="user@example.com",
        )

        assert result.winning_relationship_id is None
        assert result.strategy == ResolutionStrategy.COEXIST


class TestConflictResolver:
    """Tests for the ConflictResolver class."""

    @pytest.fixture
    def mock_repository(self) -> AsyncMock:
        """Create a mock RelationshipRepository."""
        repo = AsyncMock()
        repo.get_by_id = AsyncMock()
        repo.find_pending_conflicts = AsyncMock()
        repo.resolve_conflict = AsyncMock()
        repo.update_lifecycle_state = AsyncMock()
        repo.update_confidence = AsyncMock()
        return repo

    @pytest.fixture
    def mock_session(self) -> AsyncMock:
        """Create a mock database session."""
        session = AsyncMock()
        session.commit = AsyncMock()
        session.rollback = AsyncMock()
        return session

    @pytest.fixture
    def conflict_resolver(
        self,
        mock_session: AsyncMock,
        mock_repository: AsyncMock,
    ) -> ConflictResolver:
        """Create ConflictResolver with mocked dependencies."""
        return ConflictResolver(
            session=mock_session,
            repository=mock_repository,
            tenant_id=uuid4(),
        )

    # =========================================================================
    # AUTO-RESOLUTION TESTS (>30% confidence gap)
    # =========================================================================

    @pytest.mark.asyncio
    async def test_auto_resolve_high_confidence_gap(
        self,
        conflict_resolver: ConflictResolver,
        mock_repository: AsyncMock,
    ) -> None:
        """Test auto-resolution when confidence gap > 30%."""
        # Create conflict with 40% gap
        mock_conflict = MagicMock()
        mock_conflict.id = 1
        mock_conflict.relationship_id = 100
        mock_conflict.conflicting_relationship_id = 101
        mock_conflict.primary_confidence = Decimal("0.85")
        mock_conflict.secondary_confidence = Decimal("0.45")
        mock_conflict.confidence_gap = Decimal("0.40")
        mock_conflict.auto_resolvable = True
        mock_conflict.conflict_type = "duplicate_relationship"

        # Mock relationships
        mock_primary_rel = MagicMock()
        mock_primary_rel.id = 100
        mock_primary_rel.confidence_score = Decimal("0.85")

        mock_secondary_rel = MagicMock()
        mock_secondary_rel.id = 101
        mock_secondary_rel.confidence_score = Decimal("0.45")

        mock_repository.get_by_id.side_effect = [mock_primary_rel, mock_secondary_rel]

        result = await conflict_resolver.resolve_conflict(mock_conflict)

        assert result.strategy == ResolutionStrategy.AUTO_RESOLVE
        assert result.winning_relationship_id == 100
        assert result.losing_relationship_id == 101
        assert "auto" in result.reasoning.lower()

    @pytest.mark.asyncio
    async def test_auto_resolve_exactly_30_percent_gap(
        self,
        conflict_resolver: ConflictResolver,
        mock_repository: AsyncMock,
    ) -> None:
        """Test auto-resolution at exactly 30% gap threshold."""
        mock_conflict = MagicMock()
        mock_conflict.id = 2
        mock_conflict.relationship_id = 100
        mock_conflict.conflicting_relationship_id = 101
        mock_conflict.primary_confidence = Decimal("0.80")
        mock_conflict.secondary_confidence = Decimal("0.50")
        mock_conflict.confidence_gap = Decimal("0.30")
        mock_conflict.auto_resolvable = True
        mock_conflict.conflict_type = "duplicate_relationship"

        mock_primary_rel = MagicMock()
        mock_primary_rel.id = 100
        mock_primary_rel.confidence_score = Decimal("0.80")

        mock_secondary_rel = MagicMock()
        mock_secondary_rel.id = 101
        mock_secondary_rel.confidence_score = Decimal("0.50")

        mock_repository.get_by_id.side_effect = [mock_primary_rel, mock_secondary_rel]

        result = await conflict_resolver.resolve_conflict(mock_conflict)

        # At exactly 30%, should still auto-resolve
        assert result.strategy == ResolutionStrategy.AUTO_RESOLVE
        assert result.winning_relationship_id == 100

    @pytest.mark.asyncio
    async def test_auto_resolve_updates_losing_relationship(
        self,
        conflict_resolver: ConflictResolver,
        mock_repository: AsyncMock,
    ) -> None:
        """Test that auto-resolution archives the losing relationship."""
        mock_conflict = MagicMock()
        mock_conflict.id = 3
        mock_conflict.relationship_id = 100
        mock_conflict.conflicting_relationship_id = 101
        mock_conflict.primary_confidence = Decimal("0.90")
        mock_conflict.secondary_confidence = Decimal("0.40")
        mock_conflict.confidence_gap = Decimal("0.50")
        mock_conflict.auto_resolvable = True
        mock_conflict.conflict_type = "duplicate_relationship"

        mock_primary_rel = MagicMock()
        mock_primary_rel.id = 100
        mock_primary_rel.confidence_score = Decimal("0.90")

        mock_secondary_rel = MagicMock()
        mock_secondary_rel.id = 101
        mock_secondary_rel.confidence_score = Decimal("0.40")

        mock_repository.get_by_id.side_effect = [mock_primary_rel, mock_secondary_rel]

        await conflict_resolver.resolve_conflict(mock_conflict)

        # Verify losing relationship was archived
        mock_repository.update_lifecycle_state.assert_called_once_with(
            101, "archived"
        )

    # =========================================================================
    # USER ESCALATION TESTS (<30% confidence gap)
    # =========================================================================

    @pytest.mark.asyncio
    async def test_escalate_close_conflict(
        self,
        conflict_resolver: ConflictResolver,
        mock_repository: AsyncMock,
    ) -> None:
        """Test that close conflicts (<30% gap) require user resolution."""
        mock_conflict = MagicMock()
        mock_conflict.id = 4
        mock_conflict.relationship_id = 100
        mock_conflict.conflicting_relationship_id = 101
        mock_conflict.primary_confidence = Decimal("0.75")
        mock_conflict.secondary_confidence = Decimal("0.65")
        mock_conflict.confidence_gap = Decimal("0.10")
        mock_conflict.auto_resolvable = False
        mock_conflict.conflict_type = "duplicate_relationship"

        result = await conflict_resolver.check_auto_resolvable(mock_conflict)

        assert result is False

    @pytest.mark.asyncio
    async def test_get_conflicts_for_user_review(
        self,
        conflict_resolver: ConflictResolver,
        mock_repository: AsyncMock,
    ) -> None:
        """Test getting conflicts that need user review."""
        mock_conflict1 = MagicMock()
        mock_conflict1.id = 1
        mock_conflict1.auto_resolvable = False
        mock_conflict1.confidence_gap = Decimal("0.15")

        mock_conflict2 = MagicMock()
        mock_conflict2.id = 2
        mock_conflict2.auto_resolvable = False
        mock_conflict2.confidence_gap = Decimal("0.08")

        mock_repository.find_pending_conflicts.return_value = [
            mock_conflict1,
            mock_conflict2,
        ]

        conflicts = await conflict_resolver.get_conflicts_for_review()

        assert len(conflicts) == 2
        mock_repository.find_pending_conflicts.assert_called_once()

    @pytest.mark.asyncio
    async def test_user_resolve_conflict(
        self,
        conflict_resolver: ConflictResolver,
        mock_repository: AsyncMock,
    ) -> None:
        """Test user-driven conflict resolution."""
        mock_conflict = MagicMock()
        mock_conflict.id = 5
        mock_conflict.relationship_id = 100
        mock_conflict.conflicting_relationship_id = 101
        mock_conflict.primary_confidence = Decimal("0.70")
        mock_conflict.secondary_confidence = Decimal("0.65")
        mock_conflict.confidence_gap = Decimal("0.05")
        mock_conflict.auto_resolvable = False
        mock_conflict.conflict_type = "duplicate_relationship"

        mock_primary_rel = MagicMock()
        mock_primary_rel.id = 100
        mock_primary_rel.confidence_score = Decimal("0.70")

        mock_repository.get_by_id.return_value = mock_primary_rel

        result = await conflict_resolver.user_resolve(
            conflict=mock_conflict,
            winning_relationship_id=100,
            user_email="user@example.com",
            reasoning="I confirmed this is the correct relationship",
        )

        assert result.strategy == ResolutionStrategy.USER_RESOLVE
        assert result.winning_relationship_id == 100
        assert result.resolved_by == "user@example.com"

    # =========================================================================
    # CONFLICT TYPE HANDLING TESTS
    # =========================================================================

    @pytest.mark.asyncio
    async def test_handle_duplicate_relationship_conflict(
        self,
        conflict_resolver: ConflictResolver,
        mock_repository: AsyncMock,
    ) -> None:
        """Test handling duplicate relationship conflicts."""
        mock_conflict = MagicMock()
        mock_conflict.id = 6
        mock_conflict.relationship_id = 100
        mock_conflict.conflicting_relationship_id = 101
        mock_conflict.primary_confidence = Decimal("0.90")
        mock_conflict.secondary_confidence = Decimal("0.50")
        mock_conflict.confidence_gap = Decimal("0.40")
        mock_conflict.auto_resolvable = True
        mock_conflict.conflict_type = "duplicate_relationship"

        mock_primary_rel = MagicMock()
        mock_primary_rel.id = 100

        mock_repository.get_by_id.return_value = mock_primary_rel

        result = await conflict_resolver.resolve_conflict(mock_conflict)

        assert result.conflict_id == 6
        assert "duplicate" in result.reasoning.lower() or result.strategy == ResolutionStrategy.AUTO_RESOLVE

    @pytest.mark.asyncio
    async def test_handle_type_conflict(
        self,
        conflict_resolver: ConflictResolver,
        mock_repository: AsyncMock,
    ) -> None:
        """Test handling relationship type conflicts."""
        mock_conflict = MagicMock()
        mock_conflict.id = 7
        mock_conflict.relationship_id = 100
        mock_conflict.conflicting_relationship_id = 101
        mock_conflict.primary_confidence = Decimal("0.85")
        mock_conflict.secondary_confidence = Decimal("0.45")
        mock_conflict.confidence_gap = Decimal("0.40")
        mock_conflict.auto_resolvable = True
        mock_conflict.conflict_type = "type_mismatch"

        mock_primary_rel = MagicMock()
        mock_primary_rel.id = 100

        mock_repository.get_by_id.return_value = mock_primary_rel

        result = await conflict_resolver.resolve_conflict(mock_conflict)

        assert result.conflict_id == 7

    # =========================================================================
    # BATCH PROCESSING TESTS
    # =========================================================================

    @pytest.mark.asyncio
    async def test_process_all_auto_resolvable_conflicts(
        self,
        conflict_resolver: ConflictResolver,
        mock_repository: AsyncMock,
    ) -> None:
        """Test batch processing of auto-resolvable conflicts."""
        mock_conflict1 = MagicMock()
        mock_conflict1.id = 1
        mock_conflict1.relationship_id = 100
        mock_conflict1.conflicting_relationship_id = 101
        mock_conflict1.primary_confidence = Decimal("0.90")
        mock_conflict1.secondary_confidence = Decimal("0.40")
        mock_conflict1.confidence_gap = Decimal("0.50")
        mock_conflict1.auto_resolvable = True
        mock_conflict1.conflict_type = "duplicate_relationship"

        mock_conflict2 = MagicMock()
        mock_conflict2.id = 2
        mock_conflict2.relationship_id = 200
        mock_conflict2.conflicting_relationship_id = 201
        mock_conflict2.primary_confidence = Decimal("0.85")
        mock_conflict2.secondary_confidence = Decimal("0.45")
        mock_conflict2.confidence_gap = Decimal("0.40")
        mock_conflict2.auto_resolvable = True
        mock_conflict2.conflict_type = "duplicate_relationship"

        mock_repository.find_pending_conflicts.return_value = [
            mock_conflict1,
            mock_conflict2,
        ]

        # Mock relationship lookups
        mock_rel1 = MagicMock()
        mock_rel1.id = 100
        mock_rel2 = MagicMock()
        mock_rel2.id = 200

        mock_repository.get_by_id.side_effect = [
            mock_rel1, MagicMock(),  # For conflict 1
            mock_rel2, MagicMock(),  # For conflict 2
        ]

        results = await conflict_resolver.process_auto_resolvable()

        assert len(results) == 2
        assert all(r.strategy == ResolutionStrategy.AUTO_RESOLVE for r in results)

    # =========================================================================
    # RESOLUTION TRACKING AND AUDIT TESTS
    # =========================================================================

    @pytest.mark.asyncio
    async def test_resolution_creates_audit_trail(
        self,
        conflict_resolver: ConflictResolver,
        mock_repository: AsyncMock,
    ) -> None:
        """Test that resolution creates an audit trail."""
        mock_conflict = MagicMock()
        mock_conflict.id = 8
        mock_conflict.relationship_id = 100
        mock_conflict.conflicting_relationship_id = 101
        mock_conflict.primary_confidence = Decimal("0.90")
        mock_conflict.secondary_confidence = Decimal("0.40")
        mock_conflict.confidence_gap = Decimal("0.50")
        mock_conflict.auto_resolvable = True
        mock_conflict.conflict_type = "duplicate_relationship"

        mock_primary_rel = MagicMock()
        mock_primary_rel.id = 100

        mock_repository.get_by_id.return_value = mock_primary_rel

        await conflict_resolver.resolve_conflict(mock_conflict)

        # Verify resolve_conflict was called on repository with audit info
        mock_repository.resolve_conflict.assert_called_once()
        call_args = mock_repository.resolve_conflict.call_args
        assert call_args[1]["resolved_by"] == "system"
        assert call_args[1]["resolution_status"] == "auto_resolved"

    @pytest.mark.asyncio
    async def test_resolution_details_recorded(
        self,
        conflict_resolver: ConflictResolver,
        mock_repository: AsyncMock,
    ) -> None:
        """Test that resolution details are properly recorded."""
        mock_conflict = MagicMock()
        mock_conflict.id = 9
        mock_conflict.relationship_id = 100
        mock_conflict.conflicting_relationship_id = 101
        mock_conflict.primary_confidence = Decimal("0.90")
        mock_conflict.secondary_confidence = Decimal("0.40")
        mock_conflict.confidence_gap = Decimal("0.50")
        mock_conflict.auto_resolvable = True
        mock_conflict.conflict_type = "duplicate_relationship"

        mock_primary_rel = MagicMock()
        mock_primary_rel.id = 100

        mock_repository.get_by_id.return_value = mock_primary_rel

        result = await conflict_resolver.resolve_conflict(mock_conflict)

        # Verify result contains detailed information
        assert result.confidence_gap == Decimal("0.50")
        assert result.winning_relationship_id == 100
        assert result.losing_relationship_id == 101
        assert result.reasoning is not None
        assert len(result.reasoning) > 0

    # =========================================================================
    # EDGE CASES
    # =========================================================================

    @pytest.mark.asyncio
    async def test_conflict_with_missing_secondary_relationship(
        self,
        conflict_resolver: ConflictResolver,
        mock_repository: AsyncMock,
    ) -> None:
        """Test handling conflict where secondary relationship doesn't exist."""
        mock_conflict = MagicMock()
        mock_conflict.id = 10
        mock_conflict.relationship_id = 100
        mock_conflict.conflicting_relationship_id = None
        mock_conflict.primary_confidence = Decimal("0.80")
        mock_conflict.secondary_confidence = None
        mock_conflict.confidence_gap = Decimal("0.00")
        mock_conflict.auto_resolvable = False
        mock_conflict.conflict_type = "orphaned_conflict"

        mock_primary_rel = MagicMock()
        mock_primary_rel.id = 100

        mock_repository.get_by_id.return_value = mock_primary_rel

        result = await conflict_resolver.resolve_conflict(mock_conflict)

        # Should resolve in favor of the existing relationship
        assert result.winning_relationship_id == 100
        assert result.losing_relationship_id is None

    @pytest.mark.asyncio
    async def test_coexist_resolution(
        self,
        conflict_resolver: ConflictResolver,
        mock_repository: AsyncMock,
    ) -> None:
        """Test coexist resolution strategy."""
        mock_conflict = MagicMock()
        mock_conflict.id = 11
        mock_conflict.relationship_id = 100
        mock_conflict.conflicting_relationship_id = 101
        mock_conflict.primary_confidence = Decimal("0.70")
        mock_conflict.secondary_confidence = Decimal("0.68")
        mock_conflict.confidence_gap = Decimal("0.02")
        mock_conflict.auto_resolvable = False
        mock_conflict.conflict_type = "scope_overlap"

        result = await conflict_resolver.user_resolve(
            conflict=mock_conflict,
            winning_relationship_id=None,
            user_email="user@example.com",
            reasoning="Both relationships are valid in their contexts",
            strategy=ResolutionStrategy.COEXIST,
        )

        assert result.strategy == ResolutionStrategy.COEXIST
        assert result.winning_relationship_id is None
        assert result.losing_relationship_id is None

    @pytest.mark.asyncio
    async def test_get_resolution_history(
        self,
        conflict_resolver: ConflictResolver,
        mock_repository: AsyncMock,
    ) -> None:
        """Test retrieving resolution history for a relationship."""
        mock_conflict1 = MagicMock()
        mock_conflict1.id = 1
        mock_conflict1.resolution_status = "auto_resolved"
        mock_conflict1.resolved_at = datetime.now(timezone.utc)

        mock_conflict2 = MagicMock()
        mock_conflict2.id = 2
        mock_conflict2.resolution_status = "user_resolved"
        mock_conflict2.resolved_at = datetime.now(timezone.utc)

        mock_repository.find_by_relationship = AsyncMock(
            return_value=[mock_conflict1, mock_conflict2]
        )

        history = await conflict_resolver.get_resolution_history(relationship_id=100)

        assert len(history) == 2

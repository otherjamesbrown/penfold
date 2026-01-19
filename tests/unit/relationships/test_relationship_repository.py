"""Unit tests for RelationshipRepository operations.

Tests the repository layer for relationship CRUD operations,
queries by entity, lifecycle state, and confidence threshold.
"""

import pytest
import uuid
from datetime import datetime, timedelta, timezone
from decimal import Decimal

from penf_lib.storage.repositories import RelationshipRepository


@pytest.fixture
def tenant_id() -> uuid.UUID:
    """Test tenant ID."""
    return uuid.uuid4()


@pytest.fixture
def source_entity_id() -> int:
    """Test source entity ID."""
    return 1


@pytest.fixture
def target_entity_id() -> int:
    """Test target entity ID."""
    return 2


class TestRelationshipRepository:
    """Test RelationshipRepository operations."""

    @pytest.fixture
    async def repository(self, test_session):
        """Create relationship repository."""
        return RelationshipRepository(test_session)

    @pytest.mark.requires_db
    async def test_create_relationship(
        self, repository, test_session, tenant_id, source_entity_id, target_entity_id
    ):
        """Test creating a relationship."""
        relationship = await repository.create_relationship(
            tenant_id=tenant_id,
            source_entity_type="person",
            source_entity_id=source_entity_id,
            target_entity_type="project",
            target_entity_id=target_entity_id,
            relationship_type="works_on",
            confidence_score=Decimal("0.75"),
            ai_confidence=Decimal("0.70"),
        )

        assert relationship.id is not None
        assert relationship.tenant_id == tenant_id
        assert relationship.source_entity_type == "person"
        assert relationship.source_entity_id == source_entity_id
        assert relationship.target_entity_type == "project"
        assert relationship.target_entity_id == target_entity_id
        assert relationship.relationship_type == "works_on"
        assert relationship.confidence_score == Decimal("0.75")
        assert relationship.lifecycle_state == "pending"

    @pytest.mark.requires_db
    async def test_find_by_source_entity(
        self, repository, test_session, tenant_id, source_entity_id, target_entity_id
    ):
        """Test finding relationships by source entity."""
        # Create multiple relationships from same source
        await repository.create_relationship(
            tenant_id=tenant_id,
            source_entity_type="person",
            source_entity_id=source_entity_id,
            target_entity_type="project",
            target_entity_id=target_entity_id,
            relationship_type="works_on",
            confidence_score=Decimal("0.75"),
        )
        await repository.create_relationship(
            tenant_id=tenant_id,
            source_entity_type="person",
            source_entity_id=source_entity_id,
            target_entity_type="person",
            target_entity_id=target_entity_id + 1,
            relationship_type="collaborates_with",
            confidence_score=Decimal("0.80"),
        )
        await test_session.flush()

        # Find by source entity
        relationships = await repository.find_by_source_entity(
            tenant_id=tenant_id,
            entity_type="person",
            entity_id=source_entity_id,
        )

        assert len(relationships) == 2
        assert all(r.source_entity_id == source_entity_id for r in relationships)

    @pytest.mark.requires_db
    async def test_find_by_target_entity(
        self, repository, test_session, tenant_id, source_entity_id, target_entity_id
    ):
        """Test finding relationships by target entity."""
        # Create relationships to same target
        await repository.create_relationship(
            tenant_id=tenant_id,
            source_entity_type="person",
            source_entity_id=source_entity_id,
            target_entity_type="project",
            target_entity_id=target_entity_id,
            relationship_type="works_on",
            confidence_score=Decimal("0.75"),
        )
        await repository.create_relationship(
            tenant_id=tenant_id,
            source_entity_type="person",
            source_entity_id=source_entity_id + 1,
            target_entity_type="project",
            target_entity_id=target_entity_id,
            relationship_type="stakeholder_of",
            confidence_score=Decimal("0.65"),
        )
        await test_session.flush()

        # Find by target entity
        relationships = await repository.find_by_target_entity(
            tenant_id=tenant_id,
            entity_type="project",
            entity_id=target_entity_id,
        )

        assert len(relationships) == 2
        assert all(r.target_entity_id == target_entity_id for r in relationships)

    @pytest.mark.requires_db
    async def test_find_by_entity_pair(
        self, repository, test_session, tenant_id, source_entity_id, target_entity_id
    ):
        """Test finding relationships between specific entity pairs."""
        rel1 = await repository.create_relationship(
            tenant_id=tenant_id,
            source_entity_type="person",
            source_entity_id=source_entity_id,
            target_entity_type="person",
            target_entity_id=target_entity_id,
            relationship_type="collaborates_with",
            confidence_score=Decimal("0.80"),
        )
        await test_session.flush()

        # Find specific pair
        relationships = await repository.find_by_entity_pair(
            tenant_id=tenant_id,
            source_type="person",
            source_id=source_entity_id,
            target_type="person",
            target_id=target_entity_id,
        )

        assert len(relationships) == 1
        assert relationships[0].id == rel1.id

    @pytest.mark.requires_db
    async def test_find_by_lifecycle_state(
        self, repository, test_session, tenant_id, source_entity_id, target_entity_id
    ):
        """Test finding relationships by lifecycle state."""
        # Create relationships with different states
        rel_pending = await repository.create_relationship(
            tenant_id=tenant_id,
            source_entity_type="person",
            source_entity_id=source_entity_id,
            target_entity_type="project",
            target_entity_id=target_entity_id,
            relationship_type="works_on",
            confidence_score=Decimal("0.50"),
            lifecycle_state="pending",
        )
        rel_active = await repository.create_relationship(
            tenant_id=tenant_id,
            source_entity_type="person",
            source_entity_id=source_entity_id + 1,
            target_entity_type="project",
            target_entity_id=target_entity_id,
            relationship_type="stakeholder_of",
            confidence_score=Decimal("0.85"),
            lifecycle_state="active",
        )
        await test_session.flush()

        # Find active relationships
        active_rels = await repository.find_by_lifecycle_state(
            tenant_id=tenant_id,
            state="active",
        )

        assert len(active_rels) == 1
        assert active_rels[0].id == rel_active.id

        # Find pending relationships
        pending_rels = await repository.find_by_lifecycle_state(
            tenant_id=tenant_id,
            state="pending",
        )

        assert len(pending_rels) == 1
        assert pending_rels[0].id == rel_pending.id

    @pytest.mark.requires_db
    async def test_find_by_confidence_threshold(
        self, repository, test_session, tenant_id, source_entity_id, target_entity_id
    ):
        """Test finding relationships by confidence threshold."""
        # Create relationships with different confidence scores
        await repository.create_relationship(
            tenant_id=tenant_id,
            source_entity_type="person",
            source_entity_id=source_entity_id,
            target_entity_type="project",
            target_entity_id=target_entity_id,
            relationship_type="works_on",
            confidence_score=Decimal("0.40"),
        )
        high_conf = await repository.create_relationship(
            tenant_id=tenant_id,
            source_entity_type="person",
            source_entity_id=source_entity_id + 1,
            target_entity_type="project",
            target_entity_id=target_entity_id,
            relationship_type="leads",
            confidence_score=Decimal("0.90"),
        )
        await test_session.flush()

        # Find high confidence relationships
        high_confidence = await repository.find_by_confidence_threshold(
            tenant_id=tenant_id,
            min_confidence=Decimal("0.70"),
        )

        assert len(high_confidence) == 1
        assert high_confidence[0].id == high_conf.id

    @pytest.mark.requires_db
    async def test_update_confidence(
        self, repository, test_session, tenant_id, source_entity_id, target_entity_id
    ):
        """Test updating relationship confidence score."""
        rel = await repository.create_relationship(
            tenant_id=tenant_id,
            source_entity_type="person",
            source_entity_id=source_entity_id,
            target_entity_type="project",
            target_entity_id=target_entity_id,
            relationship_type="works_on",
            confidence_score=Decimal("0.50"),
        )
        await test_session.flush()

        # Update confidence
        updated = await repository.update_confidence(
            relationship_id=rel.id,
            new_confidence=Decimal("0.85"),
            evidence_strength=Decimal("0.80"),
        )

        assert updated is not None
        assert updated.confidence_score == Decimal("0.85")
        assert updated.evidence_strength == Decimal("0.80")

    @pytest.mark.requires_db
    async def test_update_lifecycle_state(
        self, repository, test_session, tenant_id, source_entity_id, target_entity_id
    ):
        """Test updating relationship lifecycle state."""
        rel = await repository.create_relationship(
            tenant_id=tenant_id,
            source_entity_type="person",
            source_entity_id=source_entity_id,
            target_entity_type="project",
            target_entity_id=target_entity_id,
            relationship_type="works_on",
            confidence_score=Decimal("0.85"),
        )
        await test_session.flush()

        # Transition to active
        updated = await repository.update_lifecycle_state(
            relationship_id=rel.id,
            new_state="active",
        )

        assert updated is not None
        assert updated.lifecycle_state == "active"

    @pytest.mark.requires_db
    async def test_confirm_relationship(
        self, repository, test_session, tenant_id, source_entity_id, target_entity_id
    ):
        """Test confirming a relationship via user validation."""
        rel = await repository.create_relationship(
            tenant_id=tenant_id,
            source_entity_type="person",
            source_entity_id=source_entity_id,
            target_entity_type="person",
            target_entity_id=target_entity_id,
            relationship_type="collaborates_with",
            confidence_score=Decimal("0.65"),
        )
        await test_session.flush()

        # User confirms relationship
        confirmed = await repository.confirm_relationship(
            relationship_id=rel.id,
            user_email="user@example.com",
        )

        assert confirmed is not None
        assert confirmed.user_confirmed is True
        assert confirmed.lifecycle_state == "active"

    @pytest.mark.requires_db
    async def test_increment_interaction_count(
        self, repository, test_session, tenant_id, source_entity_id, target_entity_id
    ):
        """Test incrementing interaction count."""
        rel = await repository.create_relationship(
            tenant_id=tenant_id,
            source_entity_type="person",
            source_entity_id=source_entity_id,
            target_entity_type="person",
            target_entity_id=target_entity_id,
            relationship_type="communicates_with",
            confidence_score=Decimal("0.70"),
        )
        await test_session.flush()

        assert rel.interaction_count == 0

        # Increment interaction count
        updated = await repository.increment_interaction_count(rel.id)

        assert updated is not None
        assert updated.interaction_count == 1

        # Increment again
        updated = await repository.increment_interaction_count(rel.id)
        assert updated.interaction_count == 2

    @pytest.mark.requires_db
    async def test_archive_relationship(
        self, repository, test_session, tenant_id, source_entity_id, target_entity_id
    ):
        """Test archiving a relationship."""
        rel = await repository.create_relationship(
            tenant_id=tenant_id,
            source_entity_type="person",
            source_entity_id=source_entity_id,
            target_entity_type="project",
            target_entity_id=target_entity_id,
            relationship_type="worked_on",
            confidence_score=Decimal("0.75"),
            lifecycle_state="historical",
        )
        await test_session.flush()

        # Archive relationship
        archived = await repository.archive_relationship(rel.id)

        assert archived is not None
        assert archived.lifecycle_state == "archived"
        assert archived.archived_at is not None

    @pytest.mark.requires_db
    async def test_find_stale_relationships(
        self, repository, test_session, tenant_id, source_entity_id, target_entity_id
    ):
        """Test finding stale relationships with no recent evidence."""
        old_evidence_date = datetime.now(timezone.utc) - timedelta(days=100)

        # Create stale relationship
        stale_rel = await repository.create_relationship(
            tenant_id=tenant_id,
            source_entity_type="person",
            source_entity_id=source_entity_id,
            target_entity_type="project",
            target_entity_id=target_entity_id,
            relationship_type="worked_on",
            confidence_score=Decimal("0.70"),
            lifecycle_state="active",
            last_evidence_at=old_evidence_date,
        )

        # Create recent relationship
        await repository.create_relationship(
            tenant_id=tenant_id,
            source_entity_type="person",
            source_entity_id=source_entity_id + 1,
            target_entity_type="project",
            target_entity_id=target_entity_id,
            relationship_type="works_on",
            confidence_score=Decimal("0.80"),
            lifecycle_state="active",
        )
        await test_session.flush()

        # Find stale relationships (no evidence for 90 days)
        stale_rels = await repository.find_stale_relationships(
            tenant_id=tenant_id,
            days_threshold=90,
        )

        assert len(stale_rels) == 1
        assert stale_rels[0].id == stale_rel.id

    @pytest.mark.requires_db
    async def test_get_relationship_stats(
        self, repository, test_session, tenant_id, source_entity_id, target_entity_id
    ):
        """Test getting relationship statistics for a tenant."""
        # Create relationships with different states
        await repository.create_relationship(
            tenant_id=tenant_id,
            source_entity_type="person",
            source_entity_id=source_entity_id,
            target_entity_type="project",
            target_entity_id=target_entity_id,
            relationship_type="works_on",
            confidence_score=Decimal("0.75"),
            lifecycle_state="active",
        )
        await repository.create_relationship(
            tenant_id=tenant_id,
            source_entity_type="person",
            source_entity_id=source_entity_id + 1,
            target_entity_type="project",
            target_entity_id=target_entity_id,
            relationship_type="leads",
            confidence_score=Decimal("0.90"),
            lifecycle_state="active",
        )
        await repository.create_relationship(
            tenant_id=tenant_id,
            source_entity_type="person",
            source_entity_id=source_entity_id,
            target_entity_type="person",
            target_entity_id=source_entity_id + 2,
            relationship_type="collaborates_with",
            confidence_score=Decimal("0.50"),
            lifecycle_state="pending",
        )
        await test_session.flush()

        # Get stats
        stats = await repository.get_relationship_stats(tenant_id)

        assert stats["total_relationships"] == 3
        assert stats["active_relationships"] == 2
        assert stats["pending_relationships"] == 1


class TestRelationshipEvidenceRepository:
    """Test RelationshipEvidenceRepository operations."""

    @pytest.fixture
    async def rel_repository(self, test_session):
        """Create relationship repository."""
        return RelationshipRepository(test_session)

    @pytest.mark.requires_db
    async def test_add_evidence(
        self, rel_repository, test_session, tenant_id, source_entity_id, target_entity_id
    ):
        """Test adding evidence to a relationship."""
        # Create relationship first
        rel = await rel_repository.create_relationship(
            tenant_id=tenant_id,
            source_entity_type="person",
            source_entity_id=source_entity_id,
            target_entity_type="project",
            target_entity_id=target_entity_id,
            relationship_type="works_on",
            confidence_score=Decimal("0.60"),
        )
        await test_session.flush()

        # Add evidence
        evidence = await rel_repository.add_evidence(
            tenant_id=tenant_id,
            relationship_id=rel.id,
            evidence_type="email",
            evidence_content="Discussion about project deliverables",
            strength_contribution=Decimal("0.20"),
        )

        assert evidence.id is not None
        assert evidence.relationship_id == rel.id
        assert evidence.evidence_type == "email"
        assert evidence.strength_contribution == Decimal("0.20")

    @pytest.mark.requires_db
    async def test_get_evidence_for_relationship(
        self, rel_repository, test_session, tenant_id, source_entity_id, target_entity_id
    ):
        """Test getting all evidence for a relationship."""
        # Create relationship
        rel = await rel_repository.create_relationship(
            tenant_id=tenant_id,
            source_entity_type="person",
            source_entity_id=source_entity_id,
            target_entity_type="project",
            target_entity_id=target_entity_id,
            relationship_type="works_on",
            confidence_score=Decimal("0.70"),
        )

        # Add multiple evidence items
        await rel_repository.add_evidence(
            tenant_id=tenant_id,
            relationship_id=rel.id,
            evidence_type="email",
            strength_contribution=Decimal("0.15"),
        )
        await rel_repository.add_evidence(
            tenant_id=tenant_id,
            relationship_id=rel.id,
            evidence_type="meeting",
            strength_contribution=Decimal("0.25"),
        )
        await test_session.flush()

        # Get all evidence
        evidence = await rel_repository.get_evidence_for_relationship(rel.id)

        assert len(evidence) == 2
        evidence_types = {e.evidence_type for e in evidence}
        assert "email" in evidence_types
        assert "meeting" in evidence_types


class TestRelationshipFeedbackRepository:
    """Test RelationshipFeedbackRepository operations."""

    @pytest.fixture
    async def rel_repository(self, test_session):
        """Create relationship repository."""
        return RelationshipRepository(test_session)

    @pytest.mark.requires_db
    async def test_add_feedback(
        self, rel_repository, test_session, tenant_id, source_entity_id, target_entity_id
    ):
        """Test adding user feedback for a relationship."""
        # Create relationship
        rel = await rel_repository.create_relationship(
            tenant_id=tenant_id,
            source_entity_type="person",
            source_entity_id=source_entity_id,
            target_entity_type="project",
            target_entity_id=target_entity_id,
            relationship_type="works_on",
            confidence_score=Decimal("0.65"),
        )
        await test_session.flush()

        # Add feedback
        feedback = await rel_repository.add_feedback(
            tenant_id=tenant_id,
            relationship_id=rel.id,
            feedback_type="confirm",
            user_email="user@example.com",
            reasoning="I work directly with this person on this project",
        )

        assert feedback.id is not None
        assert feedback.relationship_id == rel.id
        assert feedback.feedback_type == "confirm"
        assert feedback.user_email == "user@example.com"

    @pytest.mark.requires_db
    async def test_get_feedback_for_relationship(
        self, rel_repository, test_session, tenant_id, source_entity_id, target_entity_id
    ):
        """Test getting all feedback for a relationship."""
        # Create relationship
        rel = await rel_repository.create_relationship(
            tenant_id=tenant_id,
            source_entity_type="person",
            source_entity_id=source_entity_id,
            target_entity_type="person",
            target_entity_id=target_entity_id,
            relationship_type="collaborates_with",
            confidence_score=Decimal("0.55"),
        )

        # Add feedback
        await rel_repository.add_feedback(
            tenant_id=tenant_id,
            relationship_id=rel.id,
            feedback_type="modify",
            user_email="user1@example.com",
            corrected_type="manages",
            reasoning="This is actually a manager relationship",
        )
        await test_session.flush()

        # Get feedback
        feedback = await rel_repository.get_feedback_for_relationship(rel.id)

        assert len(feedback) == 1
        assert feedback[0].corrected_type == "manages"


class TestRelationshipConflictRepository:
    """Test RelationshipConflictRepository operations."""

    @pytest.fixture
    async def rel_repository(self, test_session):
        """Create relationship repository."""
        return RelationshipRepository(test_session)

    @pytest.mark.requires_db
    async def test_create_conflict(
        self, rel_repository, test_session, tenant_id, source_entity_id, target_entity_id
    ):
        """Test creating a relationship conflict."""
        # Create two potentially conflicting relationships
        rel1 = await rel_repository.create_relationship(
            tenant_id=tenant_id,
            source_entity_type="person",
            source_entity_id=source_entity_id,
            target_entity_type="person",
            target_entity_id=target_entity_id,
            relationship_type="manages",
            confidence_score=Decimal("0.70"),
        )
        rel2 = await rel_repository.create_relationship(
            tenant_id=tenant_id,
            source_entity_type="person",
            source_entity_id=source_entity_id,
            target_entity_type="person",
            target_entity_id=target_entity_id,
            relationship_type="reports_to",
            confidence_score=Decimal("0.60"),
        )
        await test_session.flush()

        # Create conflict
        conflict = await rel_repository.create_conflict(
            tenant_id=tenant_id,
            relationship_id=rel1.id,
            conflicting_relationship_id=rel2.id,
            conflict_type="contradictory",
            primary_confidence=Decimal("0.70"),
            secondary_confidence=Decimal("0.60"),
        )

        assert conflict.id is not None
        assert conflict.relationship_id == rel1.id
        assert conflict.conflicting_relationship_id == rel2.id
        assert conflict.conflict_type == "contradictory"
        assert conflict.confidence_gap == Decimal("0.10")
        assert conflict.auto_resolvable is False  # Gap < 30%

    @pytest.mark.requires_db
    async def test_find_pending_conflicts(
        self, rel_repository, test_session, tenant_id, source_entity_id, target_entity_id
    ):
        """Test finding pending conflicts."""
        # Create relationships and conflicts
        rel1 = await rel_repository.create_relationship(
            tenant_id=tenant_id,
            source_entity_type="person",
            source_entity_id=source_entity_id,
            target_entity_type="person",
            target_entity_id=target_entity_id,
            relationship_type="manages",
            confidence_score=Decimal("0.75"),
        )
        rel2 = await rel_repository.create_relationship(
            tenant_id=tenant_id,
            source_entity_type="person",
            source_entity_id=source_entity_id,
            target_entity_type="person",
            target_entity_id=target_entity_id,
            relationship_type="reports_to",
            confidence_score=Decimal("0.55"),
        )
        await test_session.flush()

        await rel_repository.create_conflict(
            tenant_id=tenant_id,
            relationship_id=rel1.id,
            conflicting_relationship_id=rel2.id,
            conflict_type="contradictory",
            primary_confidence=Decimal("0.75"),
            secondary_confidence=Decimal("0.55"),
        )
        await test_session.flush()

        # Find pending conflicts
        conflicts = await rel_repository.find_pending_conflicts(tenant_id)

        assert len(conflicts) == 1
        assert conflicts[0].resolution_status == "pending"

    @pytest.mark.requires_db
    async def test_resolve_conflict(
        self, rel_repository, test_session, tenant_id, source_entity_id, target_entity_id
    ):
        """Test resolving a conflict."""
        # Create relationships and conflict
        rel1 = await rel_repository.create_relationship(
            tenant_id=tenant_id,
            source_entity_type="person",
            source_entity_id=source_entity_id,
            target_entity_type="person",
            target_entity_id=target_entity_id,
            relationship_type="manages",
            confidence_score=Decimal("0.90"),
        )
        rel2 = await rel_repository.create_relationship(
            tenant_id=tenant_id,
            source_entity_type="person",
            source_entity_id=source_entity_id,
            target_entity_type="person",
            target_entity_id=target_entity_id,
            relationship_type="reports_to",
            confidence_score=Decimal("0.40"),
        )
        await test_session.flush()

        conflict = await rel_repository.create_conflict(
            tenant_id=tenant_id,
            relationship_id=rel1.id,
            conflicting_relationship_id=rel2.id,
            conflict_type="contradictory",
            primary_confidence=Decimal("0.90"),
            secondary_confidence=Decimal("0.40"),
        )
        await test_session.flush()

        # Resolve conflict (auto-resolvable since gap > 30%)
        resolved = await rel_repository.resolve_conflict(
            conflict_id=conflict.id,
            resolution_status="auto_resolved",
            resolved_by="system",
        )

        assert resolved is not None
        assert resolved.resolution_status == "auto_resolved"
        assert resolved.resolved_by == "system"
        assert resolved.resolved_at is not None

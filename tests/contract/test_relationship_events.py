"""Contract tests for relationship event schemas.

This module tests the event contracts for relationship-related events,
ensuring that event payloads conform to expected schemas for inter-service
communication.
"""

from __future__ import annotations

from datetime import datetime, timezone
from typing import Any
from uuid import uuid4

import pytest
from pydantic import ValidationError

from penf_lib.relationships import (
    ConflictResolution,
    EntityReference,
    EntityType,
    FeedbackType,
    LifecycleState,
    RelationshipConflict,
    RelationshipCreate,
    RelationshipEvidence,
    RelationshipFeedback,
    RelationshipNetworkEdge,
    RelationshipNetworkNode,
    RelationshipResponse,
    RelationshipScope,
    RelationshipType,
    RelationshipUpdate,
    RelationshipVersion,
)


# ============================================================================
# ENTITY REFERENCE CONTRACT TESTS
# ============================================================================


class TestEntityReferenceContract:
    """Test EntityReference schema contract."""

    def test_valid_entity_reference(self) -> None:
        """Test creating a valid entity reference."""
        ref = EntityReference(
            entity_id=uuid4(),
            entity_type=EntityType.PERSON,
            display_name="John Doe",
            external_ids={"email": "john@example.com"},
        )

        assert ref.entity_type == EntityType.PERSON
        assert ref.display_name == "John Doe"
        assert "email" in ref.external_ids

    def test_entity_reference_requires_id(self) -> None:
        """Test that entity_id is required."""
        with pytest.raises(ValidationError):
            EntityReference(
                entity_type=EntityType.PERSON,
                display_name="John Doe",
            )  # type: ignore

    def test_entity_reference_requires_type(self) -> None:
        """Test that entity_type is required."""
        with pytest.raises(ValidationError):
            EntityReference(
                entity_id=uuid4(),
                display_name="John Doe",
            )  # type: ignore

    def test_entity_reference_requires_display_name(self) -> None:
        """Test that display_name is required."""
        with pytest.raises(ValidationError):
            EntityReference(
                entity_id=uuid4(),
                entity_type=EntityType.PERSON,
            )  # type: ignore

    def test_all_entity_types_valid(self) -> None:
        """Test all entity types are valid."""
        valid_types = [
            EntityType.PERSON,
            EntityType.PROJECT,
            EntityType.TOPIC,
            EntityType.ORGANIZATION,
            EntityType.DOCUMENT,
            EntityType.MEETING,
            EntityType.EMAIL,
        ]

        for entity_type in valid_types:
            ref = EntityReference(
                entity_id=uuid4(),
                entity_type=entity_type,
                display_name="Test",
            )
            assert ref.entity_type == entity_type

    def test_external_ids_defaults_to_empty_dict(self) -> None:
        """Test external_ids defaults to empty dict."""
        ref = EntityReference(
            entity_id=uuid4(),
            entity_type=EntityType.PERSON,
            display_name="Test",
        )
        assert ref.external_ids == {}


# ============================================================================
# RELATIONSHIP EVIDENCE CONTRACT TESTS
# ============================================================================


class TestRelationshipEvidenceContract:
    """Test RelationshipEvidence schema contract."""

    def test_valid_evidence(self) -> None:
        """Test creating valid relationship evidence."""
        evidence = RelationshipEvidence(
            source_type="email",
            source_id=uuid4(),
            confidence_contribution=0.25,
            context_snippet="Discussion about the project",
        )

        assert evidence.source_type == "email"
        assert evidence.confidence_contribution == 0.25

    def test_confidence_contribution_bounds(self) -> None:
        """Test confidence_contribution is bounded 0-1."""
        # Valid at boundaries
        RelationshipEvidence(
            source_type="email",
            source_id=uuid4(),
            confidence_contribution=0.0,
        )
        RelationshipEvidence(
            source_type="email",
            source_id=uuid4(),
            confidence_contribution=1.0,
        )

        # Invalid below 0
        with pytest.raises(ValidationError):
            RelationshipEvidence(
                source_type="email",
                source_id=uuid4(),
                confidence_contribution=-0.1,
            )

        # Invalid above 1
        with pytest.raises(ValidationError):
            RelationshipEvidence(
                source_type="email",
                source_id=uuid4(),
                confidence_contribution=1.1,
            )

    def test_evidence_has_auto_generated_id(self) -> None:
        """Test evidence_id is auto-generated."""
        evidence = RelationshipEvidence(
            source_type="meeting",
            source_id=uuid4(),
            confidence_contribution=0.3,
        )
        assert evidence.evidence_id is not None

    def test_evidence_has_extracted_at_timestamp(self) -> None:
        """Test extracted_at is auto-populated."""
        evidence = RelationshipEvidence(
            source_type="document",
            source_id=uuid4(),
            confidence_contribution=0.2,
        )
        assert evidence.extracted_at is not None
        assert evidence.extracted_at.tzinfo is not None  # Should be timezone-aware


# ============================================================================
# RELATIONSHIP CREATE CONTRACT TESTS
# ============================================================================


class TestRelationshipCreateContract:
    """Test RelationshipCreate schema contract."""

    @pytest.fixture
    def valid_entities(self) -> tuple[EntityReference, EntityReference]:
        """Create valid source and target entities."""
        source = EntityReference(
            entity_id=uuid4(),
            entity_type=EntityType.PERSON,
            display_name="Alice",
        )
        target = EntityReference(
            entity_id=uuid4(),
            entity_type=EntityType.PERSON,
            display_name="Bob",
        )
        return source, target

    def test_valid_create_request(
        self, valid_entities: tuple[EntityReference, EntityReference]
    ) -> None:
        """Test creating a valid relationship creation request."""
        source, target = valid_entities

        create = RelationshipCreate(
            source_entity=source,
            target_entity=target,
            relationship_type=RelationshipType.COLLABORATES_WITH,
            scope=RelationshipScope.GLOBAL,
            initial_confidence=0.7,
        )

        assert create.relationship_type == RelationshipType.COLLABORATES_WITH
        assert create.initial_confidence == 0.7

    def test_create_defaults(
        self, valid_entities: tuple[EntityReference, EntityReference]
    ) -> None:
        """Test create request has correct defaults."""
        source, target = valid_entities

        create = RelationshipCreate(
            source_entity=source,
            target_entity=target,
            relationship_type=RelationshipType.COLLABORATES_WITH,
        )

        assert create.scope == RelationshipScope.TENTATIVE
        assert create.initial_confidence == 0.5
        assert create.evidence == []
        assert create.metadata == {}

    def test_all_relationship_types_valid(
        self, valid_entities: tuple[EntityReference, EntityReference]
    ) -> None:
        """Test all relationship types are valid."""
        source, target = valid_entities

        valid_types = [
            RelationshipType.COLLABORATES_WITH,
            RelationshipType.REPORTS_TO,
            RelationshipType.MANAGES,
            RelationshipType.COMMUNICATES_WITH,
            RelationshipType.WORKS_ON,
            RelationshipType.LEADS,
            RelationshipType.CONTRIBUTES_TO,
            RelationshipType.STAKEHOLDER_OF,
            RelationshipType.DEPENDS_ON,
            RelationshipType.BLOCKS,
            RelationshipType.RELATED_TO,
            RelationshipType.SUPERSEDES,
            RelationshipType.DISCUSSES,
            RelationshipType.EXPERT_IN,
            RelationshipType.INTERESTED_IN,
            RelationshipType.ASSOCIATED_WITH,
            RelationshipType.REFERENCED_IN,
        ]

        for rel_type in valid_types:
            create = RelationshipCreate(
                source_entity=source,
                target_entity=target,
                relationship_type=rel_type,
            )
            assert create.relationship_type == rel_type


# ============================================================================
# RELATIONSHIP RESPONSE CONTRACT TESTS
# ============================================================================


class TestRelationshipResponseContract:
    """Test RelationshipResponse schema contract."""

    @pytest.fixture
    def valid_response_data(self) -> dict[str, Any]:
        """Create valid response data."""
        now = datetime.now(timezone.utc)
        return {
            "relationship_id": uuid4(),
            "source_entity": EntityReference(
                entity_id=uuid4(),
                entity_type=EntityType.PERSON,
                display_name="Alice",
            ),
            "target_entity": EntityReference(
                entity_id=uuid4(),
                entity_type=EntityType.PERSON,
                display_name="Bob",
            ),
            "relationship_type": RelationshipType.COLLABORATES_WITH,
            "scope": RelationshipScope.GLOBAL,
            "lifecycle_state": LifecycleState.ACTIVE,
            "confidence_score": 0.85,
            "evidence_count": 5,
            "first_observed": now,
            "last_observed": now,
            "version": 1,
        }

    def test_valid_response(self, valid_response_data: dict[str, Any]) -> None:
        """Test creating a valid relationship response."""
        response = RelationshipResponse(**valid_response_data)

        assert response.confidence_score == 0.85
        assert response.lifecycle_state == LifecycleState.ACTIVE

    def test_confidence_score_bounds(self, valid_response_data: dict[str, Any]) -> None:
        """Test confidence_score is bounded 0-1."""
        # Valid at boundaries
        valid_response_data["confidence_score"] = 0.0
        RelationshipResponse(**valid_response_data)

        valid_response_data["confidence_score"] = 1.0
        RelationshipResponse(**valid_response_data)

        # Invalid below 0
        with pytest.raises(ValidationError):
            valid_response_data["confidence_score"] = -0.1
            RelationshipResponse(**valid_response_data)

        # Invalid above 1
        with pytest.raises(ValidationError):
            valid_response_data["confidence_score"] = 1.1
            RelationshipResponse(**valid_response_data)

    def test_all_lifecycle_states_valid(
        self, valid_response_data: dict[str, Any]
    ) -> None:
        """Test all lifecycle states are valid."""
        valid_states = [
            LifecycleState.EXPERIMENTAL,
            LifecycleState.TENTATIVE,
            LifecycleState.PROBABLE,
            LifecycleState.CONFIRMED,
            LifecycleState.ACTIVE,
            LifecycleState.HISTORICAL,
            LifecycleState.ARCHIVED,
            LifecycleState.PENDING_VALIDATION,
            LifecycleState.CONFLICTED,
        ]

        for state in valid_states:
            valid_response_data["lifecycle_state"] = state
            response = RelationshipResponse(**valid_response_data)
            assert response.lifecycle_state == state

    def test_all_scopes_valid(self, valid_response_data: dict[str, Any]) -> None:
        """Test all relationship scopes are valid."""
        valid_scopes = [
            RelationshipScope.GLOBAL,
            RelationshipScope.PROJECT,
            RelationshipScope.TEMPORAL,
            RelationshipScope.TENTATIVE,
        ]

        for scope in valid_scopes:
            valid_response_data["scope"] = scope
            response = RelationshipResponse(**valid_response_data)
            assert response.scope == scope


# ============================================================================
# RELATIONSHIP UPDATE CONTRACT TESTS
# ============================================================================


class TestRelationshipUpdateContract:
    """Test RelationshipUpdate schema contract."""

    def test_all_fields_optional(self) -> None:
        """Test all fields are optional."""
        update = RelationshipUpdate()
        assert update.relationship_type is None
        assert update.scope is None
        assert update.lifecycle_state is None
        assert update.confidence_adjustment is None

    def test_confidence_adjustment_bounds(self) -> None:
        """Test confidence_adjustment is bounded -1 to 1."""
        # Valid at boundaries
        RelationshipUpdate(confidence_adjustment=-1.0)
        RelationshipUpdate(confidence_adjustment=1.0)

        # Invalid below -1
        with pytest.raises(ValidationError):
            RelationshipUpdate(confidence_adjustment=-1.1)

        # Invalid above 1
        with pytest.raises(ValidationError):
            RelationshipUpdate(confidence_adjustment=1.1)


# ============================================================================
# RELATIONSHIP FEEDBACK CONTRACT TESTS
# ============================================================================


class TestRelationshipFeedbackContract:
    """Test RelationshipFeedback schema contract."""

    def test_valid_feedback(self) -> None:
        """Test creating valid feedback."""
        feedback = RelationshipFeedback(
            relationship_id=uuid4(),
            user_id="user@example.com",
            feedback_type=FeedbackType.CONFIRM,
            reasoning="I work with this person daily",
        )

        assert feedback.feedback_type == FeedbackType.CONFIRM

    def test_all_feedback_types_valid(self) -> None:
        """Test all feedback types are valid."""
        valid_types = [
            FeedbackType.CONFIRM,
            FeedbackType.REJECT,
            FeedbackType.MODIFY,
            FeedbackType.STRENGTHEN,
            FeedbackType.WEAKEN,
            FeedbackType.MERGE,
            FeedbackType.SPLIT,
        ]

        for fb_type in valid_types:
            feedback = RelationshipFeedback(
                relationship_id=uuid4(),
                user_id="user@example.com",
                feedback_type=fb_type,
            )
            assert feedback.feedback_type == fb_type

    def test_feedback_defaults(self) -> None:
        """Test feedback has correct defaults."""
        feedback = RelationshipFeedback(
            relationship_id=uuid4(),
            user_id="user@example.com",
            feedback_type=FeedbackType.CONFIRM,
        )

        assert feedback.confidence_in_feedback == 1.0
        assert feedback.reasoning == ""
        assert feedback.suggested_changes == {}

    def test_feedback_has_timestamp(self) -> None:
        """Test feedback has auto-generated timestamp."""
        feedback = RelationshipFeedback(
            relationship_id=uuid4(),
            user_id="user@example.com",
            feedback_type=FeedbackType.CONFIRM,
        )

        assert feedback.provided_at is not None


# ============================================================================
# RELATIONSHIP CONFLICT CONTRACT TESTS
# ============================================================================


class TestRelationshipConflictContract:
    """Test RelationshipConflict schema contract."""

    def test_valid_conflict(self) -> None:
        """Test creating valid conflict."""
        conflict = RelationshipConflict(
            relationship_ids=[uuid4(), uuid4()],
            conflict_type="type_mismatch",
            confidence_gap=0.25,
        )

        assert conflict.conflict_type == "type_mismatch"
        assert len(conflict.relationship_ids) == 2

    def test_resolved_conflict(self) -> None:
        """Test creating resolved conflict."""
        winning_id = uuid4()
        conflict = RelationshipConflict(
            relationship_ids=[winning_id, uuid4()],
            conflict_type="duplicate",
            confidence_gap=0.4,
            resolution=ConflictResolution.AUTO_RESOLVED,
            resolved_at=datetime.now(timezone.utc),
            winning_relationship_id=winning_id,
            resolution_reasoning="Auto-resolved due to confidence gap",
        )

        assert conflict.resolution == ConflictResolution.AUTO_RESOLVED
        assert conflict.winning_relationship_id == winning_id

    def test_all_resolutions_valid(self) -> None:
        """Test all resolution types are valid."""
        valid_resolutions = [
            ConflictResolution.AUTO_RESOLVED,
            ConflictResolution.USER_VALIDATED,
            ConflictResolution.MERGED,
            ConflictResolution.COEXIST,
        ]

        for resolution in valid_resolutions:
            conflict = RelationshipConflict(
                relationship_ids=[uuid4(), uuid4()],
                conflict_type="test",
                confidence_gap=0.1,
                resolution=resolution,
            )
            assert conflict.resolution == resolution


# ============================================================================
# RELATIONSHIP VERSION CONTRACT TESTS
# ============================================================================


class TestRelationshipVersionContract:
    """Test RelationshipVersion schema contract."""

    def test_valid_version(self) -> None:
        """Test creating valid version record."""
        version = RelationshipVersion(
            relationship_id=uuid4(),
            version_number=1,
            change_type="create",
            previous_state={},
            new_state={"confidence_score": 0.7},
        )

        assert version.version_number == 1
        assert version.change_type == "create"

    def test_version_has_auto_id_and_timestamp(self) -> None:
        """Test version has auto-generated id and timestamp."""
        version = RelationshipVersion(
            relationship_id=uuid4(),
            version_number=1,
            change_type="create",
            previous_state={},
            new_state={},
        )

        assert version.version_id is not None
        assert version.changed_at is not None


# ============================================================================
# NETWORK MODEL CONTRACT TESTS
# ============================================================================


class TestNetworkNodeContract:
    """Test RelationshipNetworkNode schema contract."""

    def test_valid_node(self) -> None:
        """Test creating valid network node."""
        entity = EntityReference(
            entity_id=uuid4(),
            entity_type=EntityType.PERSON,
            display_name="Alice",
        )

        node = RelationshipNetworkNode(
            entity=entity,
            relationship_count=5,
            centrality_score=0.75,
            is_hub=True,
            cluster_id="engineering",
        )

        assert node.relationship_count == 5
        assert node.is_hub is True


class TestNetworkEdgeContract:
    """Test RelationshipNetworkEdge schema contract."""

    def test_valid_edge(self) -> None:
        """Test creating valid network edge."""
        now = datetime.now(timezone.utc)

        edge = RelationshipNetworkEdge(
            source_entity_id=uuid4(),
            target_entity_id=uuid4(),
            relationship_types=[RelationshipType.COLLABORATES_WITH],
            combined_strength=0.8,
            interaction_count=10,
            first_interaction=now,
            last_interaction=now,
        )

        assert edge.combined_strength == 0.8
        assert len(edge.relationship_types) == 1

    def test_edge_multiple_relationship_types(self) -> None:
        """Test edge with multiple relationship types."""
        now = datetime.now(timezone.utc)

        edge = RelationshipNetworkEdge(
            source_entity_id=uuid4(),
            target_entity_id=uuid4(),
            relationship_types=[
                RelationshipType.COLLABORATES_WITH,
                RelationshipType.COMMUNICATES_WITH,
            ],
            combined_strength=0.9,
            interaction_count=25,
            first_interaction=now,
            last_interaction=now,
        )

        assert len(edge.relationship_types) == 2


# ============================================================================
# JSON SERIALIZATION CONTRACT TESTS
# ============================================================================


class TestJsonSerializationContract:
    """Test JSON serialization contracts for API compatibility."""

    def test_entity_reference_serialization(self) -> None:
        """Test EntityReference JSON serialization."""
        ref = EntityReference(
            entity_id=uuid4(),
            entity_type=EntityType.PERSON,
            display_name="Test",
        )

        json_data = ref.model_dump(mode="json")

        assert isinstance(json_data["entity_id"], str)  # UUID as string
        assert json_data["entity_type"] == "person"  # Enum as value

    def test_relationship_response_serialization(self) -> None:
        """Test RelationshipResponse JSON serialization."""
        now = datetime.now(timezone.utc)

        response = RelationshipResponse(
            relationship_id=uuid4(),
            source_entity=EntityReference(
                entity_id=uuid4(),
                entity_type=EntityType.PERSON,
                display_name="Alice",
            ),
            target_entity=EntityReference(
                entity_id=uuid4(),
                entity_type=EntityType.PERSON,
                display_name="Bob",
            ),
            relationship_type=RelationshipType.COLLABORATES_WITH,
            scope=RelationshipScope.GLOBAL,
            lifecycle_state=LifecycleState.ACTIVE,
            confidence_score=0.85,
            evidence_count=5,
            first_observed=now,
            last_observed=now,
            version=1,
        )

        json_data = response.model_dump(mode="json")

        # Verify serialization format
        assert isinstance(json_data["relationship_id"], str)
        assert json_data["relationship_type"] == "collaborates_with"
        assert json_data["lifecycle_state"] == "active"
        assert json_data["scope"] == "global"
        assert isinstance(json_data["first_observed"], str)  # ISO format

    def test_feedback_serialization(self) -> None:
        """Test RelationshipFeedback JSON serialization."""
        feedback = RelationshipFeedback(
            relationship_id=uuid4(),
            user_id="user@example.com",
            feedback_type=FeedbackType.CONFIRM,
        )

        json_data = feedback.model_dump(mode="json")

        assert json_data["feedback_type"] == "confirm"
        assert isinstance(json_data["feedback_id"], str)


if __name__ == "__main__":
    pytest.main([__file__, "-v"])

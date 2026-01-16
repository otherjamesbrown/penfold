"""Shared fixtures for relationship module tests.

This module provides test fixtures for creating relationship domain models,
evidence, feedback, and network structures used across relationship tests.
"""

from __future__ import annotations

from collections.abc import Callable
from datetime import datetime, timezone
from typing import Any, Optional
from uuid import uuid4

import pytest

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


@pytest.fixture
def sample_person_entity() -> EntityReference:
    """Create a sample person entity reference."""
    return EntityReference(
        entity_id=uuid4(),
        entity_type=EntityType.PERSON,
        display_name="Alice Johnson",
        external_ids={"email": "alice@example.com", "slack_id": "U12345"},
    )


@pytest.fixture
def sample_person_entity_bob() -> EntityReference:
    """Create a second sample person entity reference."""
    return EntityReference(
        entity_id=uuid4(),
        entity_type=EntityType.PERSON,
        display_name="Bob Smith",
        external_ids={"email": "bob@example.com"},
    )


@pytest.fixture
def sample_project_entity() -> EntityReference:
    """Create a sample project entity reference."""
    return EntityReference(
        entity_id=uuid4(),
        entity_type=EntityType.PROJECT,
        display_name="Atlas Deployment",
        external_ids={"jira_key": "ATLAS"},
    )


@pytest.fixture
def sample_topic_entity() -> EntityReference:
    """Create a sample topic entity reference."""
    return EntityReference(
        entity_id=uuid4(),
        entity_type=EntityType.TOPIC,
        display_name="Performance Optimization",
        external_ids={},
    )


@pytest.fixture
def sample_evidence() -> RelationshipEvidence:
    """Create a sample relationship evidence item."""
    return RelationshipEvidence(
        source_type="email",
        source_id=uuid4(),
        confidence_contribution=0.25,
        context_snippet="Alice and Bob discussed the deployment timeline...",
        metadata={"thread_id": "thread-123"},
    )


@pytest.fixture
def sample_evidence_meeting() -> RelationshipEvidence:
    """Create a sample meeting-based evidence item."""
    return RelationshipEvidence(
        source_type="meeting",
        source_id=uuid4(),
        confidence_contribution=0.35,
        context_snippet="Team standup discussing project progress",
        metadata={"meeting_type": "standup", "duration_minutes": 15},
    )


@pytest.fixture
def sample_relationship_create(
    sample_person_entity: EntityReference,
    sample_project_entity: EntityReference,
    sample_evidence: RelationshipEvidence,
) -> RelationshipCreate:
    """Create a sample relationship creation request."""
    return RelationshipCreate(
        source_entity=sample_person_entity,
        target_entity=sample_project_entity,
        relationship_type=RelationshipType.WORKS_ON,
        scope=RelationshipScope.PROJECT,
        project_id=uuid4(),
        initial_confidence=0.65,
        evidence=[sample_evidence],
        metadata={"role": "developer"},
    )


@pytest.fixture
def sample_relationship_response(
    sample_person_entity: EntityReference,
    sample_person_entity_bob: EntityReference,
) -> RelationshipResponse:
    """Create a sample relationship response."""
    now = datetime.now(timezone.utc)
    return RelationshipResponse(
        relationship_id=uuid4(),
        source_entity=sample_person_entity,
        target_entity=sample_person_entity_bob,
        relationship_type=RelationshipType.COLLABORATES_WITH,
        scope=RelationshipScope.GLOBAL,
        lifecycle_state=LifecycleState.ACTIVE,
        confidence_score=0.85,
        evidence_count=5,
        first_observed=now,
        last_observed=now,
        last_validated=now,
        version=1,
        metadata={"collaboration_type": "technical"},
    )


@pytest.fixture
def sample_relationship_update() -> RelationshipUpdate:
    """Create a sample relationship update request."""
    return RelationshipUpdate(
        lifecycle_state=LifecycleState.CONFIRMED,
        confidence_adjustment=0.1,
        metadata={"user_validated": True},
    )


@pytest.fixture
def sample_feedback(sample_relationship_response: RelationshipResponse) -> RelationshipFeedback:
    """Create a sample relationship feedback."""
    return RelationshipFeedback(
        relationship_id=sample_relationship_response.relationship_id,
        user_id="user-123",
        feedback_type=FeedbackType.CONFIRM,
        confidence_in_feedback=0.9,
        reasoning="I work closely with Bob on multiple projects",
        suggested_changes={},
    )


@pytest.fixture
def sample_conflict() -> RelationshipConflict:
    """Create a sample relationship conflict."""
    return RelationshipConflict(
        relationship_ids=[uuid4(), uuid4()],
        conflict_type="type_mismatch",
        confidence_gap=0.15,
        resolution=None,
        resolution_reasoning="",
    )


@pytest.fixture
def sample_resolved_conflict() -> RelationshipConflict:
    """Create a sample resolved relationship conflict."""
    rel_id = uuid4()
    return RelationshipConflict(
        relationship_ids=[rel_id, uuid4()],
        conflict_type="duplicate_entity",
        confidence_gap=0.45,
        resolution=ConflictResolution.AUTO_RESOLVED,
        resolved_at=datetime.now(timezone.utc),
        winning_relationship_id=rel_id,
        resolution_reasoning="Confidence gap > 30%, auto-resolved in favor of higher confidence relationship",
    )


@pytest.fixture
def sample_version(sample_relationship_response: RelationshipResponse) -> RelationshipVersion:
    """Create a sample relationship version."""
    return RelationshipVersion(
        relationship_id=sample_relationship_response.relationship_id,
        version_number=2,
        changed_by="user-123",
        change_type="feedback",
        previous_state={
            "confidence_score": 0.75,
            "lifecycle_state": "probable",
        },
        new_state={
            "confidence_score": 0.85,
            "lifecycle_state": "confirmed",
        },
        change_reasoning="User confirmed relationship based on direct experience",
    )


@pytest.fixture
def sample_network_node(sample_person_entity: EntityReference) -> RelationshipNetworkNode:
    """Create a sample network node."""
    return RelationshipNetworkNode(
        entity=sample_person_entity,
        relationship_count=12,
        centrality_score=0.75,
        is_hub=True,
        cluster_id="engineering-team",
    )


@pytest.fixture
def sample_network_edge(
    sample_person_entity: EntityReference,
    sample_person_entity_bob: EntityReference,
) -> RelationshipNetworkEdge:
    """Create a sample network edge."""
    now = datetime.now(timezone.utc)
    return RelationshipNetworkEdge(
        source_entity_id=sample_person_entity.entity_id,
        target_entity_id=sample_person_entity_bob.entity_id,
        relationship_types=[
            RelationshipType.COLLABORATES_WITH,
            RelationshipType.COMMUNICATES_WITH,
        ],
        combined_strength=0.82,
        interaction_count=47,
        first_interaction=now,
        last_interaction=now,
    )


# Type aliases for factory functions
EntityFactoryType = Callable[..., EntityReference]
EvidenceFactoryType = Callable[..., RelationshipEvidence]
RelationshipFactoryType = Callable[..., RelationshipResponse]


# Utility fixtures for generating test data
@pytest.fixture
def entity_factory() -> EntityFactoryType:
    """Factory for creating entity references with custom attributes."""

    def _create_entity(
        entity_type: EntityType = EntityType.PERSON,
        display_name: str = "Test Entity",
        external_ids: Optional[dict[str, Any]] = None,
    ) -> EntityReference:
        return EntityReference(
            entity_id=uuid4(),
            entity_type=entity_type,
            display_name=display_name,
            external_ids=external_ids or {},
        )

    return _create_entity


@pytest.fixture
def evidence_factory() -> EvidenceFactoryType:
    """Factory for creating relationship evidence with custom attributes."""

    def _create_evidence(
        source_type: str = "email",
        confidence_contribution: float = 0.2,
        context_snippet: str = "Sample context",
    ) -> RelationshipEvidence:
        return RelationshipEvidence(
            source_type=source_type,
            source_id=uuid4(),
            confidence_contribution=confidence_contribution,
            context_snippet=context_snippet,
        )

    return _create_evidence


@pytest.fixture
def relationship_factory(
    entity_factory: EntityFactoryType,
    evidence_factory: EvidenceFactoryType,
) -> RelationshipFactoryType:
    """Factory for creating complete relationship structures."""

    def _create_relationship(
        relationship_type: RelationshipType = RelationshipType.COLLABORATES_WITH,
        confidence: float = 0.7,
        lifecycle_state: LifecycleState = LifecycleState.ACTIVE,
        evidence_count: int = 3,
    ) -> RelationshipResponse:
        now = datetime.now(timezone.utc)
        return RelationshipResponse(
            relationship_id=uuid4(),
            source_entity=entity_factory(),
            target_entity=entity_factory(),
            relationship_type=relationship_type,
            scope=RelationshipScope.GLOBAL,
            lifecycle_state=lifecycle_state,
            confidence_score=confidence,
            evidence_count=evidence_count,
            first_observed=now,
            last_observed=now,
            version=1,
        )

    return _create_relationship

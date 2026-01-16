"""Integration tests for the complete relationship discovery and management workflow.

This module tests the full workflow from content extraction through discovery,
validation, lifecycle management, network analysis, and search integration.

The tests use mock components to avoid external dependencies while testing
the integration between all relationship module components.
"""

from __future__ import annotations

import asyncio
from datetime import datetime, timedelta, timezone
from decimal import Decimal
from typing import Any
from unittest.mock import AsyncMock, MagicMock
from uuid import UUID, uuid4

import pytest

from penf_lib.relationships import (
    ConfidenceCalculator,
    ConflictResolver,
    DiscoveredRelationship,
    DiscoveryResult,
    EntityReference,
    EntityType,
    ExtractedRelationship,
    ExtractionResult,
    FeedbackProcessor,
    FeedbackResult,
    FeedbackType,
    FeedbackTypeEnum,
    LearnedPreference,
    LifecycleManager,
    LifecycleState,
    LifecycleTransition,
    MaintenanceResult,
    NetworkAnalyzer,
    NetworkInsights,
    RelationshipConflict,
    RelationshipContextProvider,
    RelationshipCreate,
    RelationshipDiscoveryService,
    RelationshipEvidence,
    RelationshipExtractor,
    RelationshipFeedback,
    RelationshipResponse,
    RelationshipScope,
    RelationshipSearchIntegration,
    RelationshipType,
    RelationshipUpdate,
    StateTransitionError,
    TransitionReason,
)


# ============================================================================
# MOCK IMPLEMENTATIONS
# ============================================================================


class MockEntityResolver:
    """Mock entity resolver for testing without database."""

    def __init__(self) -> None:
        self.people: dict[str, dict[str, Any]] = {}
        self.projects: dict[str, dict[str, Any]] = {}
        self.topics: dict[str, dict[str, Any]] = {}

    def add_person(self, name: str, entity_id: int) -> None:
        """Register a person for resolution."""
        self.people[name.lower()] = {
            "entity_id": entity_id,
            "confidence": 1.0,
            "name": name,
        }

    def add_project(self, name: str, entity_id: int) -> None:
        """Register a project for resolution."""
        self.projects[name.lower()] = {
            "entity_id": entity_id,
            "confidence": 1.0,
            "name": name,
        }

    def add_topic(self, name: str, entity_id: int) -> None:
        """Register a topic for resolution."""
        self.topics[name.lower()] = {
            "entity_id": entity_id,
            "confidence": 1.0,
            "name": name,
        }

    async def resolve_person(
        self, name: str, context: dict[str, Any] | None = None
    ) -> dict[str, Any] | None:
        """Resolve person name to entity."""
        return self.people.get(name.lower())

    async def resolve_project(
        self, name: str, context: dict[str, Any] | None = None
    ) -> dict[str, Any] | None:
        """Resolve project name to entity."""
        return self.projects.get(name.lower())

    async def resolve_topic(
        self, name: str, context: dict[str, Any] | None = None
    ) -> dict[str, Any] | None:
        """Resolve topic name to entity."""
        return self.topics.get(name.lower())


class MockRelationshipRepository:
    """Mock relationship repository for testing without database."""

    def __init__(self) -> None:
        self.relationships: dict[int, MagicMock] = {}
        self.evidence: dict[int, list[MagicMock]] = {}
        self.versions: dict[int, list[MagicMock]] = {}
        self._next_id = 1

    async def create_relationship(
        self,
        tenant_id: UUID,
        source_entity_type: str,
        source_entity_id: int,
        target_entity_type: str,
        target_entity_id: int,
        relationship_type: str,
        confidence_score: Decimal,
        **kwargs: Any,
    ) -> MagicMock:
        """Create a mock relationship."""
        rel_id = self._next_id
        self._next_id += 1

        rel = MagicMock()
        rel.id = rel_id
        rel.tenant_id = tenant_id
        rel.source_entity_type = source_entity_type
        rel.source_entity_id = source_entity_id
        rel.target_entity_type = target_entity_type
        rel.target_entity_id = target_entity_id
        rel.relationship_type = relationship_type
        rel.confidence_score = confidence_score
        rel.lifecycle_state = kwargs.get("lifecycle_state", "tentative")
        rel.interaction_count = 1
        rel.last_observed_at = datetime.now(timezone.utc)
        rel.ai_confidence = kwargs.get("ai_confidence", confidence_score)
        rel.evidence_strength = kwargs.get("evidence_strength", Decimal("0.5"))

        self.relationships[rel_id] = rel
        self.evidence[rel_id] = []
        self.versions[rel_id] = []

        return rel

    async def find_by_entity_pair(
        self,
        tenant_id: UUID,
        source_type: str,
        source_id: int,
        target_type: str,
        target_id: int,
        relationship_type: str | None = None,
    ) -> list[MagicMock]:
        """Find relationships between entity pair."""
        results = []
        for rel in self.relationships.values():
            if (
                rel.source_entity_type == source_type
                and rel.source_entity_id == source_id
                and rel.target_entity_type == target_type
                and rel.target_entity_id == target_id
            ):
                if relationship_type is None or rel.relationship_type == relationship_type:
                    results.append(rel)
        return results

    async def update_confidence(
        self,
        relationship_id: int,
        new_confidence: Decimal,
        evidence_strength: Decimal | None = None,
    ) -> MagicMock | None:
        """Update relationship confidence."""
        if relationship_id in self.relationships:
            rel = self.relationships[relationship_id]
            rel.confidence_score = new_confidence
            if evidence_strength is not None:
                rel.evidence_strength = evidence_strength
            return rel
        return None

    async def add_evidence(
        self,
        tenant_id: UUID,
        relationship_id: int,
        evidence_type: str,
        strength_contribution: Decimal,
        evidence_content: str | None = None,
        source_id: int | None = None,
        evidence_context: dict[str, Any] | None = None,
    ) -> MagicMock:
        """Add evidence to a relationship."""
        evidence = MagicMock()
        evidence.relationship_id = relationship_id
        evidence.evidence_type = evidence_type
        evidence.strength_contribution = strength_contribution
        evidence.evidence_content = evidence_content
        evidence.source_id = source_id

        if relationship_id not in self.evidence:
            self.evidence[relationship_id] = []
        self.evidence[relationship_id].append(evidence)

        return evidence

    async def increment_interaction_count(
        self,
        relationship_id: int,
    ) -> MagicMock | None:
        """Increment relationship interaction count."""
        if relationship_id in self.relationships:
            rel = self.relationships[relationship_id]
            rel.interaction_count += 1
            return rel
        return None

    async def get_by_id(self, entity_id: int) -> MagicMock | None:
        """Get relationship by ID."""
        return self.relationships.get(entity_id)

    async def update(self, entity_id: int, **kwargs: Any) -> MagicMock | None:
        """Update relationship."""
        if entity_id in self.relationships:
            rel = self.relationships[entity_id]
            for key, value in kwargs.items():
                setattr(rel, key, value)
            return rel
        return None

    async def update_lifecycle_state(
        self,
        relationship_id: int,
        new_state: str,
    ) -> MagicMock | None:
        """Update relationship lifecycle state."""
        if relationship_id in self.relationships:
            rel = self.relationships[relationship_id]
            rel.lifecycle_state = new_state
            return rel
        return None

    async def find_inactive(
        self,
        threshold_days: int,
        states: list[str] | None = None,
    ) -> list[MagicMock]:
        """Find inactive relationships."""
        results = []
        cutoff = datetime.now(timezone.utc) - timedelta(days=threshold_days)

        for rel in self.relationships.values():
            if states is None or rel.lifecycle_state in states:
                if rel.last_observed_at < cutoff:
                    results.append(rel)
        return results

    async def find_stale(
        self,
        retention_days: int,
        states: list[str] | None = None,
    ) -> list[MagicMock]:
        """Find stale relationships past retention period."""
        results = []
        cutoff = datetime.now(timezone.utc) - timedelta(days=retention_days)

        for rel in self.relationships.values():
            if states is None or rel.lifecycle_state in states:
                if rel.last_observed_at < cutoff:
                    results.append(rel)
        return results

    async def find_by_state(
        self,
        states: list[str],
    ) -> list[MagicMock]:
        """Find relationships in given states."""
        return [
            rel
            for rel in self.relationships.values()
            if rel.lifecycle_state in states
        ]

    async def find_by_project(
        self,
        project_id: UUID,
    ) -> list[MagicMock]:
        """Find relationships associated with a project."""
        return []

    async def find_by_entity(
        self,
        entity_id: int,
        entity_type: str | None = None,
        relationship_types: list[str] | None = None,
    ) -> list[MagicMock]:
        """Find relationships involving an entity."""
        results = []
        for rel in self.relationships.values():
            matches_source = rel.source_entity_id == entity_id
            matches_target = rel.target_entity_id == entity_id

            if entity_type:
                matches_source = matches_source and rel.source_entity_type == entity_type
                matches_target = matches_target and rel.target_entity_type == entity_type

            if matches_source or matches_target:
                if relationship_types is None or rel.relationship_type in relationship_types:
                    results.append(rel)
        return results

    async def add_version(
        self,
        tenant_id: UUID,
        relationship_id: int,
        version_number: int,
        change_type: str,
        changed_by: str | None,
        previous_state: dict[str, Any],
        new_state: dict[str, Any],
        change_reasoning: str,
    ) -> MagicMock:
        """Add version record for relationship."""
        version = MagicMock()
        version.relationship_id = relationship_id
        version.version_number = version_number
        version.change_type = change_type
        version.changed_by = changed_by

        if relationship_id not in self.versions:
            self.versions[relationship_id] = []
        self.versions[relationship_id].append(version)

        return version

    async def get_evidence_for_relationship(
        self,
        relationship_id: int,
    ) -> list[MagicMock]:
        """Get evidence for a relationship."""
        return self.evidence.get(relationship_id, [])

    async def get_latest_version(
        self,
        relationship_id: int,
    ) -> MagicMock | None:
        """Get latest version record for relationship."""
        versions = self.versions.get(relationship_id, [])
        if versions:
            return max(versions, key=lambda v: v.version_number)
        return None


class MockExtractor:
    """Mock relationship extractor for testing."""

    def __init__(self) -> None:
        self.extraction_results: list[ExtractionResult] = []
        self._call_count = 0

    def set_extraction_result(self, result: ExtractionResult) -> None:
        """Set the result to return for the next extraction."""
        self.extraction_results.append(result)

    async def extract_relationships(
        self,
        content_id: UUID,
        content_type: str,
        content_data: dict[str, Any],
    ) -> ExtractionResult:
        """Return mock extraction result."""
        if self._call_count < len(self.extraction_results):
            result = self.extraction_results[self._call_count]
            self._call_count += 1
            return result

        # Default empty result
        return ExtractionResult(
            content_id=content_id,
            content_type=content_type,
            relationships=[],
            extraction_confidence=0.8,
            processing_time_ms=50,
        )


# ============================================================================
# TEST FIXTURES
# ============================================================================


@pytest.fixture
def mock_session() -> AsyncMock:
    """Create mock database session."""
    session = AsyncMock()
    return session


@pytest.fixture
def entity_resolver() -> MockEntityResolver:
    """Create mock entity resolver with sample entities."""
    resolver = MockEntityResolver()
    resolver.add_person("Alice Johnson", 1)
    resolver.add_person("Bob Smith", 2)
    resolver.add_person("Charlie Brown", 3)
    resolver.add_project("Atlas Deployment", 101)
    resolver.add_project("Phoenix Initiative", 102)
    resolver.add_topic("Performance", 201)
    resolver.add_topic("Security", 202)
    return resolver


@pytest.fixture
def repository() -> MockRelationshipRepository:
    """Create mock relationship repository."""
    return MockRelationshipRepository()


@pytest.fixture
def extractor() -> MockExtractor:
    """Create mock relationship extractor."""
    return MockExtractor()


@pytest.fixture
def tenant_id() -> UUID:
    """Create test tenant ID."""
    return uuid4()


@pytest.fixture
def discovery_service(
    mock_session: AsyncMock,
    repository: MockRelationshipRepository,
    extractor: MockExtractor,
    entity_resolver: MockEntityResolver,
    tenant_id: UUID,
) -> RelationshipDiscoveryService:
    """Create discovery service with mock dependencies."""
    return RelationshipDiscoveryService(
        session=mock_session,
        repository=repository,
        extractor=extractor,
        entity_resolver=entity_resolver,
        tenant_id=tenant_id,
    )


@pytest.fixture
def lifecycle_manager(
    mock_session: AsyncMock,
    repository: MockRelationshipRepository,
    tenant_id: UUID,
) -> LifecycleManager:
    """Create lifecycle manager with mock dependencies."""
    return LifecycleManager(
        session=mock_session,
        repository=repository,
        tenant_id=tenant_id,
    )


@pytest.fixture
def network_analyzer() -> NetworkAnalyzer:
    """Create network analyzer."""
    return NetworkAnalyzer()


@pytest.fixture
def sample_relationships() -> list[RelationshipResponse]:
    """Create sample relationships for network analysis."""
    now = datetime.now(timezone.utc)
    relationships = []

    # Create a network of relationships between people
    people = [
        EntityReference(
            entity_id=uuid4(),
            entity_type=EntityType.PERSON,
            display_name=f"Person {i}",
            external_ids={"email": f"person{i}@example.com"},
        )
        for i in range(5)
    ]

    # Create edges: 0-1, 0-2, 1-2, 2-3, 3-4
    edges = [(0, 1), (0, 2), (1, 2), (2, 3), (3, 4)]

    for i, (src, tgt) in enumerate(edges):
        rel = RelationshipResponse(
            relationship_id=uuid4(),
            source_entity=people[src],
            target_entity=people[tgt],
            relationship_type=RelationshipType.COLLABORATES_WITH,
            scope=RelationshipScope.GLOBAL,
            lifecycle_state=LifecycleState.ACTIVE,
            confidence_score=0.7 + (i * 0.05),
            evidence_count=i + 1,
            first_observed=now - timedelta(days=30),
            last_observed=now,
            version=1,
        )
        relationships.append(rel)

    return relationships


# ============================================================================
# INTEGRATION TEST: FULL DISCOVERY WORKFLOW
# ============================================================================


class TestDiscoveryWorkflow:
    """Test complete discovery workflow from content to relationships."""

    @pytest.mark.asyncio
    async def test_full_discovery_workflow(
        self,
        discovery_service: RelationshipDiscoveryService,
        extractor: MockExtractor,
        repository: MockRelationshipRepository,
    ) -> None:
        """Test complete workflow: extract -> resolve -> calculate confidence -> persist."""
        # Setup: Mock extraction result
        content_id = uuid4()
        extraction_result = ExtractionResult(
            content_id=content_id,
            content_type="email",
            relationships=[
                ExtractedRelationship(
                    source_entity_type="person",
                    source_entity_name="Alice Johnson",
                    target_entity_type="person",
                    target_entity_name="Bob Smith",
                    relationship_type="collaborates_with",
                    confidence=0.85,
                    context_snippet="Alice and Bob are working together on the project",
                ),
            ],
            model_used="llama-3.1-8b",
            extraction_confidence=0.9,
            processing_time_ms=100,
        )
        extractor.set_extraction_result(extraction_result)

        # Execute: Process content
        result = await discovery_service.process_content(
            content_id=content_id,
            content_type="email",
            source_id=1001,
            content_data={"body": "Alice and Bob discussed the project timeline"},
        )

        # Verify: Check discovery result
        assert result.content_id == content_id
        assert result.created_count == 1
        assert len(result.discovered_relationships) == 1
        assert len(result.errors) == 0

        # Verify: Check relationship was created
        discovered = result.discovered_relationships[0]
        assert discovered.source_entity_type == "person"
        assert discovered.target_entity_type == "person"
        assert discovered.relationship_type == "collaborates_with"
        assert discovered.confidence_score > Decimal("0")

        # Verify: Check repository state
        assert len(repository.relationships) == 1

    @pytest.mark.asyncio
    async def test_discovery_updates_existing_relationship(
        self,
        discovery_service: RelationshipDiscoveryService,
        extractor: MockExtractor,
        repository: MockRelationshipRepository,
    ) -> None:
        """Test that discovering same relationship updates confidence."""
        content_id_1 = uuid4()
        content_id_2 = uuid4()

        # First discovery
        extraction_result_1 = ExtractionResult(
            content_id=content_id_1,
            content_type="email",
            relationships=[
                ExtractedRelationship(
                    source_entity_type="person",
                    source_entity_name="Alice Johnson",
                    target_entity_type="person",
                    target_entity_name="Bob Smith",
                    relationship_type="collaborates_with",
                    confidence=0.7,
                    context_snippet="First interaction",
                ),
            ],
            model_used="llama-3.1-8b",
            extraction_confidence=0.85,
            processing_time_ms=50,
        )
        extractor.set_extraction_result(extraction_result_1)

        result_1 = await discovery_service.process_content(
            content_id=content_id_1,
            content_type="email",
            source_id=1001,
            content_data={"body": "First email"},
        )

        assert result_1.created_count == 1
        initial_confidence = result_1.discovered_relationships[0].confidence_score

        # Second discovery of same relationship
        extraction_result_2 = ExtractionResult(
            content_id=content_id_2,
            content_type="email",
            relationships=[
                ExtractedRelationship(
                    source_entity_type="person",
                    source_entity_name="Alice Johnson",
                    target_entity_type="person",
                    target_entity_name="Bob Smith",
                    relationship_type="collaborates_with",
                    confidence=0.8,
                    context_snippet="Second interaction",
                ),
            ],
            model_used="llama-3.1-8b",
            extraction_confidence=0.9,
            processing_time_ms=50,
        )
        extractor.set_extraction_result(extraction_result_2)

        result_2 = await discovery_service.process_content(
            content_id=content_id_2,
            content_type="email",
            source_id=1002,
            content_data={"body": "Second email"},
        )

        # Verify: Updated, not created
        assert result_2.created_count == 0
        assert result_2.updated_count == 1

        # Verify: Confidence increased
        updated_confidence = result_2.discovered_relationships[0].confidence_score
        assert updated_confidence >= initial_confidence

        # Verify: Still only one relationship in repository
        assert len(repository.relationships) == 1

        # Verify: Evidence was added
        rel_id = list(repository.relationships.keys())[0]
        assert len(repository.evidence[rel_id]) == 2

    @pytest.mark.asyncio
    async def test_discovery_handles_unresolved_entities(
        self,
        discovery_service: RelationshipDiscoveryService,
        extractor: MockExtractor,
    ) -> None:
        """Test that unresolved entities don't create relationships."""
        content_id = uuid4()

        extraction_result = ExtractionResult(
            content_id=content_id,
            content_type="email",
            relationships=[
                ExtractedRelationship(
                    source_entity_type="person",
                    source_entity_name="Unknown Person",  # Not in resolver
                    target_entity_type="person",
                    target_entity_name="Bob Smith",
                    relationship_type="collaborates_with",
                    confidence=0.9,
                    context_snippet="Context",
                ),
            ],
            model_used="llama-3.1-8b",
            extraction_confidence=0.85,
            processing_time_ms=50,
        )
        extractor.set_extraction_result(extraction_result)

        result = await discovery_service.process_content(
            content_id=content_id,
            content_type="email",
            source_id=1001,
            content_data={"body": "Content"},
        )

        # Verify: No relationship created due to unresolved entity
        assert result.created_count == 0
        assert len(result.discovered_relationships) == 0

    @pytest.mark.asyncio
    async def test_batch_processing(
        self,
        discovery_service: RelationshipDiscoveryService,
        extractor: MockExtractor,
    ) -> None:
        """Test batch processing of multiple content items."""
        content_items = []

        for i in range(3):
            content_id = uuid4()
            extraction_result = ExtractionResult(
                content_id=content_id,
                content_type="email",
                relationships=[
                    ExtractedRelationship(
                        source_entity_type="person",
                        source_entity_name="Alice Johnson",
                        target_entity_type="project",
                        target_entity_name="Atlas Deployment",
                        relationship_type="works_on",
                        confidence=0.75 + (i * 0.05),
                        context_snippet=f"Context {i}",
                    ),
                ],
                model_used="llama-3.1-8b",
                extraction_confidence=0.85,
                processing_time_ms=50,
            )
            extractor.set_extraction_result(extraction_result)

            content_items.append({
                "content_id": content_id,
                "content_type": "email",
                "source_id": 1001 + i,
                "content_data": {"body": f"Email {i}"},
            })

        results = await discovery_service.batch_process(content_items)

        assert len(results) == 3
        assert results[0].created_count == 1
        assert results[1].updated_count == 1  # Same relationship updated
        assert results[2].updated_count == 1


# ============================================================================
# INTEGRATION TEST: LIFECYCLE MANAGEMENT
# ============================================================================


class TestLifecycleWorkflow:
    """Test complete lifecycle management workflow."""

    @pytest.mark.asyncio
    async def test_state_transition_workflow(
        self,
        lifecycle_manager: LifecycleManager,
        repository: MockRelationshipRepository,
        tenant_id: UUID,
    ) -> None:
        """Test workflow: pending -> active -> historical -> archived."""
        # Create initial relationship in pending state
        rel = await repository.create_relationship(
            tenant_id=tenant_id,
            source_entity_type="person",
            source_entity_id=1,
            target_entity_type="person",
            target_entity_id=2,
            relationship_type="collaborates_with",
            confidence_score=Decimal("0.7"),
            lifecycle_state="pending",
        )

        # Transition: pending -> active (user confirmed)
        transition = await lifecycle_manager.transition_state(
            relationship_id=rel.id,
            to_state="active",
            reason=TransitionReason.USER_CONFIRMED,
            triggered_by="user@example.com",
        )

        assert transition.from_state == "pending"
        assert transition.to_state == "active"
        assert transition.reason == TransitionReason.USER_CONFIRMED

        # Verify state changed
        assert repository.relationships[rel.id].lifecycle_state == "active"

        # Transition: active -> historical (inactivity)
        transition = await lifecycle_manager.transition_state(
            relationship_id=rel.id,
            to_state="historical",
            reason=TransitionReason.INACTIVITY,
            triggered_by="system",
        )

        assert transition.from_state == "active"
        assert transition.to_state == "historical"

        # Transition: historical -> archived (retention expired)
        transition = await lifecycle_manager.transition_state(
            relationship_id=rel.id,
            to_state="archived",
            reason=TransitionReason.RETENTION_EXPIRED,
            triggered_by="system",
        )

        assert transition.to_state == "archived"

        # Verify: Can't transition out of archived
        with pytest.raises(StateTransitionError):
            await lifecycle_manager.transition_state(
                relationship_id=rel.id,
                to_state="active",
                reason=TransitionReason.NEW_EVIDENCE,
                triggered_by="system",
            )

    @pytest.mark.asyncio
    async def test_reactivation_workflow(
        self,
        lifecycle_manager: LifecycleManager,
        repository: MockRelationshipRepository,
        tenant_id: UUID,
    ) -> None:
        """Test reactivating a historical relationship."""
        # Create historical relationship
        rel = await repository.create_relationship(
            tenant_id=tenant_id,
            source_entity_type="person",
            source_entity_id=1,
            target_entity_type="person",
            target_entity_id=2,
            relationship_type="collaborates_with",
            confidence_score=Decimal("0.5"),
            lifecycle_state="historical",
        )

        # Reactivate
        transition = await lifecycle_manager.reactivate(
            relationship_id=rel.id,
            reason="New email exchange detected",
            triggered_by="system",
        )

        assert transition.to_state == "active"
        assert transition.reason == TransitionReason.NEW_EVIDENCE
        assert repository.relationships[rel.id].lifecycle_state == "active"

    @pytest.mark.asyncio
    async def test_cannot_reactivate_archived(
        self,
        lifecycle_manager: LifecycleManager,
        repository: MockRelationshipRepository,
        tenant_id: UUID,
    ) -> None:
        """Test that archived relationships cannot be reactivated."""
        rel = await repository.create_relationship(
            tenant_id=tenant_id,
            source_entity_type="person",
            source_entity_id=1,
            target_entity_type="person",
            target_entity_id=2,
            relationship_type="collaborates_with",
            confidence_score=Decimal("0.5"),
            lifecycle_state="archived",
        )

        with pytest.raises(StateTransitionError):
            await lifecycle_manager.reactivate(
                relationship_id=rel.id,
                reason="Should fail",
                triggered_by="system",
            )

    @pytest.mark.asyncio
    async def test_inactivity_detection(
        self,
        lifecycle_manager: LifecycleManager,
        repository: MockRelationshipRepository,
        tenant_id: UUID,
    ) -> None:
        """Test detecting and processing inactive relationships."""
        # Create an active relationship with old last_observed
        rel = await repository.create_relationship(
            tenant_id=tenant_id,
            source_entity_type="person",
            source_entity_id=1,
            target_entity_type="person",
            target_entity_id=2,
            relationship_type="collaborates_with",
            confidence_score=Decimal("0.7"),
            lifecycle_state="active",
        )

        # Make it inactive by setting old timestamp
        rel.last_observed_at = datetime.now(timezone.utc) - timedelta(days=100)

        # Find inactive relationships
        inactive = await lifecycle_manager.find_inactive_relationships(
            threshold_days=90
        )

        assert len(inactive) == 1
        assert inactive[0].id == rel.id

        # Process inactive relationships
        result = await lifecycle_manager.process_inactive_relationships(
            threshold_days=90
        )

        assert result.total_processed == 1
        assert result.transitions_applied == 1
        assert repository.relationships[rel.id].lifecycle_state == "historical"

    @pytest.mark.asyncio
    async def test_confidence_decay(
        self,
        lifecycle_manager: LifecycleManager,
        repository: MockRelationshipRepository,
        tenant_id: UUID,
    ) -> None:
        """Test confidence decay over time."""
        rel = await repository.create_relationship(
            tenant_id=tenant_id,
            source_entity_type="person",
            source_entity_id=1,
            target_entity_type="person",
            target_entity_id=2,
            relationship_type="collaborates_with",
            confidence_score=Decimal("0.9"),
            lifecycle_state="active",
        )

        # Set old observation time for decay
        rel.last_observed_at = datetime.now(timezone.utc) - timedelta(days=180)

        # Recalculate confidence
        new_confidence = await lifecycle_manager.recalculate_confidence(rel.id)

        # Confidence should have decayed
        assert new_confidence < Decimal("0.9")
        assert new_confidence >= Decimal("0.05")  # Minimum floor


# ============================================================================
# INTEGRATION TEST: NETWORK ANALYSIS
# ============================================================================


class TestNetworkAnalysisWorkflow:
    """Test network analysis workflow."""

    def test_full_network_analysis(
        self,
        network_analyzer: NetworkAnalyzer,
        sample_relationships: list[RelationshipResponse],
    ) -> None:
        """Test complete network analysis workflow."""
        insights = network_analyzer.analyze(sample_relationships)

        # Verify basic metrics
        assert insights.total_nodes == 5
        assert insights.total_edges == 5
        assert 0.0 <= insights.density <= 1.0

        # Verify hubs detection (Person 2 should be a hub - connected to 0, 1, 3)
        # With threshold 0.5, may or may not have hubs depending on graph size

        # Verify isolated detection (Person 4 only has 1 connection to 3)
        isolated = network_analyzer.detect_isolated(
            network_analyzer.build_network(sample_relationships),
            connection_threshold=2,
        )
        assert len(isolated) >= 1  # Person 0 and 4 have <= 2 connections

        # Verify community detection
        assert len(insights.communities) >= 1  # Should find at least one community

    def test_centrality_calculations(
        self,
        network_analyzer: NetworkAnalyzer,
        sample_relationships: list[RelationshipResponse],
    ) -> None:
        """Test centrality metric calculations."""
        graph = network_analyzer.build_network(sample_relationships)
        centrality = network_analyzer.calculate_centrality(graph)

        assert len(centrality) == 5

        # Verify all metrics are within valid range
        for entity_id, metrics in centrality.items():
            assert 0.0 <= metrics.degree_centrality <= 1.0
            assert 0.0 <= metrics.betweenness_centrality <= 1.0
            assert 0.0 <= metrics.closeness_centrality <= 1.0
            assert 0.0 <= metrics.eigenvector_centrality <= 1.0

    def test_collaboration_opportunities(
        self,
        network_analyzer: NetworkAnalyzer,
        sample_relationships: list[RelationshipResponse],
    ) -> None:
        """Test finding collaboration opportunities."""
        graph = network_analyzer.build_network(sample_relationships)
        opportunities = network_analyzer.find_collaboration_opportunities(
            graph,
            min_common_neighbors=1,
        )

        # Should find opportunities (e.g., Person 0 and Person 3 share Person 2)
        assert len(opportunities) >= 1

        for opp in opportunities:
            assert opp.common_neighbors >= 1
            assert 0.0 <= opp.opportunity_score <= 1.0

    def test_bottleneck_identification(
        self,
        network_analyzer: NetworkAnalyzer,
        sample_relationships: list[RelationshipResponse],
    ) -> None:
        """Test identifying communication bottlenecks."""
        graph = network_analyzer.build_network(sample_relationships)
        bottlenecks = network_analyzer.identify_bottlenecks(graph)

        # Person 2 and 3 are bridges in the chain 0-1-2-3-4
        for bottleneck in bottlenecks:
            assert 0.0 <= bottleneck.impact_score <= 1.0
            assert bottleneck.connected_components_affected > 0


# ============================================================================
# INTEGRATION TEST: FEEDBACK AND CONFLICT RESOLUTION
# ============================================================================


class TestFeedbackWorkflow:
    """Test feedback processing workflow."""

    @pytest.mark.asyncio
    async def test_feedback_application(
        self,
        mock_session: AsyncMock,
        repository: MockRelationshipRepository,
        tenant_id: UUID,
    ) -> None:
        """Test applying user feedback to a relationship."""
        # Create a relationship to provide feedback on
        rel = await repository.create_relationship(
            tenant_id=tenant_id,
            source_entity_type="person",
            source_entity_id=1,
            target_entity_type="person",
            target_entity_id=2,
            relationship_type="collaborates_with",
            confidence_score=Decimal("0.6"),
            lifecycle_state="tentative",
        )

        # Add mock methods for feedback processor
        async def mock_add_feedback(*args: Any, **kwargs: Any) -> MagicMock:
            return MagicMock()

        async def mock_confirm_relationship(rel_id: int, user: str) -> MagicMock:
            repository.relationships[rel_id].lifecycle_state = "active"
            return repository.relationships[rel_id]

        repository.add_feedback = mock_add_feedback
        repository.confirm_relationship = mock_confirm_relationship

        processor = FeedbackProcessor(
            session=mock_session,
            repository=repository,
            tenant_id=tenant_id,
        )

        # Process confirm feedback
        result = await processor.process_confirm(
            relationship_id=rel.id,
            user_email="user@example.com",
            reasoning="I work with this person daily",
        )

        # Verify result
        assert result.new_confidence > result.previous_confidence
        assert result.new_state == "active"

    @pytest.mark.asyncio
    async def test_feedback_rejection(
        self,
        mock_session: AsyncMock,
        repository: MockRelationshipRepository,
        tenant_id: UUID,
    ) -> None:
        """Test rejecting a relationship through feedback."""
        # Create a relationship to reject
        rel = await repository.create_relationship(
            tenant_id=tenant_id,
            source_entity_type="person",
            source_entity_id=1,
            target_entity_type="person",
            target_entity_id=2,
            relationship_type="collaborates_with",
            confidence_score=Decimal("0.7"),
            lifecycle_state="tentative",
        )

        # Add mock methods
        async def mock_add_feedback(*args: Any, **kwargs: Any) -> MagicMock:
            return MagicMock()

        repository.add_feedback = mock_add_feedback

        processor = FeedbackProcessor(
            session=mock_session,
            repository=repository,
            tenant_id=tenant_id,
        )

        # Process reject feedback
        result = await processor.process_reject(
            relationship_id=rel.id,
            user_email="user@example.com",
            reasoning="This is incorrect",
        )

        # Verify result
        assert result.new_confidence < result.previous_confidence


class TestConflictResolutionWorkflow:
    """Test conflict resolution workflow."""

    @pytest.mark.asyncio
    async def test_auto_resolution_large_gap(
        self,
        mock_session: AsyncMock,
        repository: MockRelationshipRepository,
        tenant_id: UUID,
    ) -> None:
        """Test automatic resolution when confidence gap is large."""
        # Create two conflicting relationships
        rel1 = await repository.create_relationship(
            tenant_id=tenant_id,
            source_entity_type="person",
            source_entity_id=1,
            target_entity_type="person",
            target_entity_id=2,
            relationship_type="collaborates_with",
            confidence_score=Decimal("0.9"),
            lifecycle_state="tentative",
        )
        rel2 = await repository.create_relationship(
            tenant_id=tenant_id,
            source_entity_type="person",
            source_entity_id=1,
            target_entity_type="person",
            target_entity_id=2,
            relationship_type="reports_to",  # Conflicting type
            confidence_score=Decimal("0.4"),
            lifecycle_state="tentative",
        )

        # Create mock conflict
        conflict = MagicMock()
        conflict.id = 1
        conflict.relationship_id = rel1.id
        conflict.conflicting_relationship_id = rel2.id
        conflict.confidence_gap = Decimal("0.5")
        conflict.primary_confidence = Decimal("0.9")
        conflict.secondary_confidence = Decimal("0.4")
        conflict.auto_resolvable = True

        # Add mock methods
        async def mock_find_pending(*args: Any, **kwargs: Any) -> list[MagicMock]:
            return [conflict]

        async def mock_resolve_conflict(*args: Any, **kwargs: Any) -> MagicMock:
            return conflict

        repository.find_pending_conflicts = mock_find_pending
        repository.resolve_conflict = mock_resolve_conflict

        resolver = ConflictResolver(
            session=mock_session,
            repository=repository,
            tenant_id=tenant_id,
        )

        # Resolve the conflict
        result = await resolver.resolve_conflict(conflict)

        # Verify auto-resolution occurred
        assert result.strategy.value == "auto_resolve"
        assert result.winning_relationship_id == rel1.id

    @pytest.mark.asyncio
    async def test_conflict_resolution_requires_repository(
        self,
        mock_session: AsyncMock,
        repository: MockRelationshipRepository,
        tenant_id: UUID,
    ) -> None:
        """Test that ConflictResolver requires proper initialization."""
        # Verify ConflictResolver needs session, repository, and tenant_id
        resolver = ConflictResolver(
            session=mock_session,
            repository=repository,
            tenant_id=tenant_id,
        )

        # Check attributes are set correctly
        assert resolver.session == mock_session
        assert resolver.repository == repository
        assert resolver.tenant_id == tenant_id


# ============================================================================
# INTEGRATION TEST: SEARCH INTEGRATION
# ============================================================================


class TestSearchIntegration:
    """Test search integration workflow."""

    @pytest.mark.asyncio
    async def test_context_provider_initialization(
        self,
        mock_session: AsyncMock,
        repository: MockRelationshipRepository,
        tenant_id: UUID,
    ) -> None:
        """Test RelationshipContextProvider initialization."""
        # Add mock methods
        async def mock_find_by_source(*args: Any, **kwargs: Any) -> list[MagicMock]:
            return []

        repository.find_by_source_entity = mock_find_by_source

        context_provider = RelationshipContextProvider(
            session=mock_session,
            repository=repository,
            tenant_id=tenant_id,
        )

        # Should initialize without error
        assert context_provider.session == mock_session
        assert context_provider.tenant_id == tenant_id

    @pytest.mark.asyncio
    async def test_search_integration_boost_calculation(
        self,
        mock_session: AsyncMock,
        repository: MockRelationshipRepository,
        tenant_id: UUID,
    ) -> None:
        """Test search boost calculation based on relationships."""
        # Add mock methods
        async def mock_find_by_source(*args: Any, **kwargs: Any) -> list[MagicMock]:
            return []

        repository.find_by_source_entity = mock_find_by_source

        context_provider = RelationshipContextProvider(
            session=mock_session,
            repository=repository,
            tenant_id=tenant_id,
        )

        integration = RelationshipSearchIntegration(
            context_provider=context_provider,
        )

        # Test boost calculation for related entity
        from penf_lib.relationships.search import RelatedEntity

        related = RelatedEntity(
            entity_id=1,
            entity_type="person",
            display_name="Alice",
            relationship_type="collaborates_with",
            relationship_strength=0.8,
            path_length=1,
        )

        boost = integration.calculate_relationship_boost(
            entity_id=1,
            entity_type="person",
            related_entities=[related],
        )

        # Should have boost > 1.0 for related entity
        assert boost > 1.0

        # Unrelated entity should have no boost
        boost_unrelated = integration.calculate_relationship_boost(
            entity_id=999,
            entity_type="person",
            related_entities=[related],
        )
        assert boost_unrelated == 1.0


# ============================================================================
# INTEGRATION TEST: END-TO-END WORKFLOW
# ============================================================================


class TestEndToEndWorkflow:
    """Test complete end-to-end relationship management workflow."""

    @pytest.mark.asyncio
    async def test_complete_workflow(
        self,
        discovery_service: RelationshipDiscoveryService,
        lifecycle_manager: LifecycleManager,
        extractor: MockExtractor,
        repository: MockRelationshipRepository,
        network_analyzer: NetworkAnalyzer,
    ) -> None:
        """Test complete workflow: Discovery -> Validation -> Lifecycle -> Analysis."""
        # Step 1: Discover relationships from content
        relationships_to_create = [
            ("Alice Johnson", "Bob Smith"),
            ("Bob Smith", "Charlie Brown"),
            ("Alice Johnson", "Charlie Brown"),
        ]

        for src, tgt in relationships_to_create:
            content_id = uuid4()
            extraction_result = ExtractionResult(
                content_id=content_id,
                content_type="email",
                relationships=[
                    ExtractedRelationship(
                        source_entity_type="person",
                        source_entity_name=src,
                        target_entity_type="person",
                        target_entity_name=tgt,
                        relationship_type="collaborates_with",
                        confidence=0.75,
                        context_snippet=f"{src} and {tgt} discussed the project",
                    ),
                ],
                model_used="llama-3.1-8b",
                extraction_confidence=0.85,
                processing_time_ms=50,
            )
            extractor.set_extraction_result(extraction_result)

            await discovery_service.process_content(
                content_id=content_id,
                content_type="email",
                source_id=1001,
                content_data={"body": "Content"},
            )

        # Verify: 3 relationships created
        assert len(repository.relationships) == 3

        # Step 2: User validates a relationship
        rel_id = list(repository.relationships.keys())[0]
        await lifecycle_manager.transition_state(
            relationship_id=rel_id,
            to_state="active",
            reason=TransitionReason.USER_CONFIRMED,
            triggered_by="user@example.com",
        )

        assert repository.relationships[rel_id].lifecycle_state == "active"

        # Step 3: Build and analyze network
        # Create RelationshipResponse objects for analysis
        now = datetime.now(timezone.utc)
        response_relationships = []

        for rel in repository.relationships.values():
            response = RelationshipResponse(
                relationship_id=uuid4(),
                source_entity=EntityReference(
                    entity_id=uuid4(),
                    entity_type=EntityType.PERSON,
                    display_name=f"Person {rel.source_entity_id}",
                    external_ids={},
                ),
                target_entity=EntityReference(
                    entity_id=uuid4(),
                    entity_type=EntityType.PERSON,
                    display_name=f"Person {rel.target_entity_id}",
                    external_ids={},
                ),
                relationship_type=RelationshipType.COLLABORATES_WITH,
                scope=RelationshipScope.GLOBAL,
                lifecycle_state=LifecycleState.ACTIVE,
                confidence_score=float(rel.confidence_score),
                evidence_count=rel.interaction_count,
                first_observed=now,
                last_observed=now,
                version=1,
            )
            response_relationships.append(response)

        insights = network_analyzer.analyze(response_relationships)

        # Verify: Network has correct structure
        assert insights.total_nodes == 3  # Alice, Bob, Charlie
        assert insights.total_edges == 3  # The three relationships

        # Step 4: Simulate inactivity and maintenance
        for rel in repository.relationships.values():
            rel.last_observed_at = datetime.now(timezone.utc) - timedelta(days=100)

        maintenance_result = await lifecycle_manager.process_inactive_relationships(
            threshold_days=90
        )

        # Some relationships should transition to historical
        assert maintenance_result.total_processed > 0


if __name__ == "__main__":
    pytest.main([__file__, "-v"])

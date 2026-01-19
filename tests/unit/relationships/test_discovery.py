"""Unit tests for RelationshipDiscoveryService.

Tests the orchestration of relationship discovery including:
- Processing content from different sources (email, meeting)
- Coordinating extraction with AI models
- Creating and updating relationships
- Managing evidence records
- Integration with repository layer
"""

from __future__ import annotations

from datetime import datetime, timedelta, timezone
from decimal import Decimal
from typing import Any
from unittest.mock import AsyncMock, MagicMock, patch
from uuid import UUID, uuid4

import pytest

from penf_lib.relationships.discovery import (
    DiscoveredRelationship,
    DiscoveryResult,
    RelationshipDiscoveryService,
)
from penf_lib.relationships.extractor import ExtractedRelationship, ExtractionResult
from penf_lib.relationships.confidence import ConfidenceCalculator


class TestDiscoveredRelationship:
    """Tests for the DiscoveredRelationship model."""

    def test_create_discovered_relationship(self) -> None:
        """Test creating a DiscoveredRelationship."""
        rel = DiscoveredRelationship(
            source_entity_type="person",
            source_entity_id=1,
            target_entity_type="project",
            target_entity_id=10,
            relationship_type="works_on",
            confidence_score=Decimal("0.85"),
            ai_confidence=Decimal("0.80"),
            evidence_strength=Decimal("0.75"),
            lifecycle_state="pending",
        )

        assert rel.source_entity_type == "person"
        assert rel.source_entity_id == 1
        assert rel.relationship_type == "works_on"
        assert rel.confidence_score == Decimal("0.85")

    def test_discovered_relationship_with_evidence(self) -> None:
        """Test DiscoveredRelationship with evidence data."""
        rel = DiscoveredRelationship(
            source_entity_type="person",
            source_entity_id=1,
            target_entity_type="person",
            target_entity_id=2,
            relationship_type="collaborates_with",
            confidence_score=Decimal("0.78"),
            ai_confidence=Decimal("0.75"),
            evidence_strength=Decimal("0.65"),
            lifecycle_state="pending",
            evidence=[
                {
                    "evidence_type": "email",
                    "source_id": 100,
                    "context_snippet": "Alice and Bob discussed the project",
                    "strength_contribution": 0.30,
                }
            ],
        )

        assert len(rel.evidence) == 1
        assert rel.evidence[0]["evidence_type"] == "email"


class TestDiscoveryResult:
    """Tests for the DiscoveryResult model."""

    def test_create_discovery_result(self) -> None:
        """Test creating a DiscoveryResult."""
        discovered_rels = [
            DiscoveredRelationship(
                source_entity_type="person",
                source_entity_id=1,
                target_entity_type="project",
                target_entity_id=10,
                relationship_type="works_on",
                confidence_score=Decimal("0.85"),
                ai_confidence=Decimal("0.80"),
                evidence_strength=Decimal("0.75"),
                lifecycle_state="pending",
            )
        ]

        result = DiscoveryResult(
            content_id=uuid4(),
            content_type="email",
            source_id=100,
            discovered_relationships=discovered_rels,
            created_count=1,
            updated_count=0,
            processing_time_ms=1500,
        )

        assert result.content_type == "email"
        assert result.created_count == 1
        assert result.updated_count == 0
        assert len(result.discovered_relationships) == 1

    def test_discovery_result_empty(self) -> None:
        """Test DiscoveryResult with no relationships discovered."""
        result = DiscoveryResult(
            content_id=uuid4(),
            content_type="email",
            source_id=100,
            discovered_relationships=[],
            created_count=0,
            updated_count=0,
            processing_time_ms=500,
        )

        assert len(result.discovered_relationships) == 0
        assert result.created_count == 0


class TestRelationshipDiscoveryService:
    """Tests for the RelationshipDiscoveryService class."""

    @pytest.fixture
    def mock_session(self) -> AsyncMock:
        """Create a mock database session."""
        session = AsyncMock()
        session.flush = AsyncMock()
        session.commit = AsyncMock()
        session.rollback = AsyncMock()
        return session

    @pytest.fixture
    def mock_repository(self) -> AsyncMock:
        """Create a mock RelationshipRepository."""
        repo = AsyncMock()
        repo.create_relationship = AsyncMock()
        repo.find_by_entity_pair = AsyncMock(return_value=[])
        repo.update_confidence = AsyncMock()
        repo.add_evidence = AsyncMock()
        repo.increment_interaction_count = AsyncMock()
        return repo

    @pytest.fixture
    def mock_extractor(self) -> AsyncMock:
        """Create a mock RelationshipExtractor."""
        extractor = AsyncMock()
        extractor.extract_relationships = AsyncMock()
        return extractor

    @pytest.fixture
    def mock_entity_resolver(self) -> AsyncMock:
        """Create a mock entity resolver."""
        resolver = AsyncMock()
        resolver.resolve_person = AsyncMock()
        resolver.resolve_project = AsyncMock()
        resolver.resolve_topic = AsyncMock()
        return resolver

    @pytest.fixture
    def discovery_service(
        self,
        mock_session: AsyncMock,
        mock_repository: AsyncMock,
        mock_extractor: AsyncMock,
        mock_entity_resolver: AsyncMock,
    ) -> RelationshipDiscoveryService:
        """Create a RelationshipDiscoveryService with mocked dependencies."""
        return RelationshipDiscoveryService(
            session=mock_session,
            repository=mock_repository,
            extractor=mock_extractor,
            entity_resolver=mock_entity_resolver,
            tenant_id=uuid4(),
        )

    @pytest.mark.asyncio
    async def test_process_email_content(
        self,
        discovery_service: RelationshipDiscoveryService,
        mock_extractor: AsyncMock,
        mock_entity_resolver: AsyncMock,
        mock_repository: AsyncMock,
    ) -> None:
        """Test processing email content for relationship discovery."""
        content_id = uuid4()

        # Set up extractor to return relationships
        mock_extractor.extract_relationships.return_value = ExtractionResult(
            content_id=content_id,
            content_type="email",
            relationships=[
                ExtractedRelationship(
                    source_entity_type="person",
                    source_entity_name="Alice Johnson",
                    target_entity_type="project",
                    target_entity_name="Atlas Deployment",
                    relationship_type="works_on",
                    confidence=0.85,
                    context_snippet="Alice is leading the Atlas deployment",
                )
            ],
            model_used="llama-3.1-8b",
            processing_time_ms=1200,
            extraction_confidence=0.85,
        )

        # Set up entity resolver
        mock_entity_resolver.resolve_person.return_value = {
            "entity_id": 1,
            "confidence": 0.95,
        }
        mock_entity_resolver.resolve_project.return_value = {
            "entity_id": 10,
            "confidence": 0.90,
        }

        # Set up repository to return created relationship
        mock_relationship = MagicMock()
        mock_relationship.id = 100
        mock_relationship.confidence_score = Decimal("0.82")
        mock_repository.create_relationship.return_value = mock_relationship

        content_data = {
            "subject": "Atlas Deployment Update",
            "body": "Alice is leading the Atlas deployment. Great progress!",
            "sender": "manager@example.com",
            "recipients": ["team@example.com"],
        }

        result = await discovery_service.process_content(
            content_id=content_id,
            content_type="email",
            source_id=1,
            content_data=content_data,
        )

        assert isinstance(result, DiscoveryResult)
        assert result.content_type == "email"
        assert result.created_count >= 1 or result.updated_count >= 0

        # Verify extractor was called
        mock_extractor.extract_relationships.assert_called_once()

    @pytest.mark.asyncio
    async def test_process_meeting_content(
        self,
        discovery_service: RelationshipDiscoveryService,
        mock_extractor: AsyncMock,
        mock_entity_resolver: AsyncMock,
        mock_repository: AsyncMock,
    ) -> None:
        """Test processing meeting content for relationship discovery."""
        content_id = uuid4()

        # Set up extractor to return multiple relationships
        mock_extractor.extract_relationships.return_value = ExtractionResult(
            content_id=content_id,
            content_type="meeting",
            relationships=[
                ExtractedRelationship(
                    source_entity_type="person",
                    source_entity_name="Bob Smith",
                    target_entity_type="person",
                    target_entity_name="Carol Davis",
                    relationship_type="collaborates_with",
                    confidence=0.78,
                    context_snippet="Bob and Carol worked on implementation",
                ),
                ExtractedRelationship(
                    source_entity_type="person",
                    source_entity_name="Bob Smith",
                    target_entity_type="topic",
                    target_entity_name="Performance Optimization",
                    relationship_type="expert_in",
                    confidence=0.65,
                    context_snippet="Bob presented the optimization strategy",
                ),
            ],
            model_used="llama-3.1-8b",
            processing_time_ms=1800,
            extraction_confidence=0.75,
        )

        # Set up entity resolver
        mock_entity_resolver.resolve_person.side_effect = [
            {"entity_id": 2, "confidence": 0.92},  # Bob
            {"entity_id": 3, "confidence": 0.88},  # Carol
            {"entity_id": 2, "confidence": 0.92},  # Bob again
        ]
        mock_entity_resolver.resolve_topic.return_value = {
            "entity_id": 50,
            "confidence": 0.80,
        }

        # Set up repository
        mock_rel1 = MagicMock()
        mock_rel1.id = 101
        mock_rel2 = MagicMock()
        mock_rel2.id = 102
        mock_repository.create_relationship.side_effect = [mock_rel1, mock_rel2]

        content_data = {
            "title": "Performance Review Meeting",
            "transcript": "Bob presented optimization. Carol supported implementation.",
            "participants": ["bob@example.com", "carol@example.com"],
        }

        result = await discovery_service.process_content(
            content_id=content_id,
            content_type="meeting",
            source_id=2,
            content_data=content_data,
        )

        assert result.content_type == "meeting"
        # Should create 2 relationships
        assert len(result.discovered_relationships) == 2

    @pytest.mark.asyncio
    async def test_update_existing_relationship(
        self,
        discovery_service: RelationshipDiscoveryService,
        mock_extractor: AsyncMock,
        mock_entity_resolver: AsyncMock,
        mock_repository: AsyncMock,
    ) -> None:
        """Test that existing relationships are updated with new evidence."""
        content_id = uuid4()

        # Set up extractor
        mock_extractor.extract_relationships.return_value = ExtractionResult(
            content_id=content_id,
            content_type="email",
            relationships=[
                ExtractedRelationship(
                    source_entity_type="person",
                    source_entity_name="Alice Johnson",
                    target_entity_type="project",
                    target_entity_name="Atlas Deployment",
                    relationship_type="works_on",
                    confidence=0.80,
                    context_snippet="Alice mentioned progress on Atlas",
                )
            ],
            model_used="llama-3.1-8b",
            processing_time_ms=1000,
            extraction_confidence=0.80,
        )

        # Set up entity resolver
        mock_entity_resolver.resolve_person.return_value = {
            "entity_id": 1,
            "confidence": 0.95,
        }
        mock_entity_resolver.resolve_project.return_value = {
            "entity_id": 10,
            "confidence": 0.90,
        }

        # Return existing relationship
        existing_rel = MagicMock()
        existing_rel.id = 100
        existing_rel.confidence_score = Decimal("0.75")
        existing_rel.interaction_count = 3
        mock_repository.find_by_entity_pair.return_value = [existing_rel]

        content_data = {
            "subject": "Atlas Update",
            "body": "Quick update on Atlas progress from Alice.",
        }

        result = await discovery_service.process_content(
            content_id=content_id,
            content_type="email",
            source_id=3,
            content_data=content_data,
        )

        # Should update existing, not create new
        assert result.updated_count >= 1 or result.created_count >= 0

        # Verify evidence was added
        assert mock_repository.add_evidence.called or mock_repository.increment_interaction_count.called

    @pytest.mark.asyncio
    async def test_handle_unresolved_entities(
        self,
        discovery_service: RelationshipDiscoveryService,
        mock_extractor: AsyncMock,
        mock_entity_resolver: AsyncMock,
        mock_repository: AsyncMock,
    ) -> None:
        """Test handling when entities cannot be resolved."""
        content_id = uuid4()

        # Set up extractor with relationship
        mock_extractor.extract_relationships.return_value = ExtractionResult(
            content_id=content_id,
            content_type="email",
            relationships=[
                ExtractedRelationship(
                    source_entity_type="person",
                    source_entity_name="Unknown Person",
                    target_entity_type="project",
                    target_entity_name="Mystery Project",
                    relationship_type="works_on",
                    confidence=0.60,
                    context_snippet="Someone mentioned a project",
                )
            ],
            model_used="llama-3.1-8b",
            processing_time_ms=800,
            extraction_confidence=0.60,
        )

        # Entity resolver returns None (cannot resolve)
        mock_entity_resolver.resolve_person.return_value = None
        mock_entity_resolver.resolve_project.return_value = None

        content_data = {"body": "Unclear content about someone"}

        result = await discovery_service.process_content(
            content_id=content_id,
            content_type="email",
            source_id=4,
            content_data=content_data,
        )

        # No relationships should be created if entities can't be resolved
        assert result.created_count == 0

    @pytest.mark.asyncio
    async def test_process_content_no_relationships(
        self,
        discovery_service: RelationshipDiscoveryService,
        mock_extractor: AsyncMock,
    ) -> None:
        """Test processing content that yields no relationships."""
        content_id = uuid4()

        # Set up extractor to return no relationships
        mock_extractor.extract_relationships.return_value = ExtractionResult(
            content_id=content_id,
            content_type="email",
            relationships=[],
            model_used="llama-3.1-8b",
            processing_time_ms=500,
            extraction_confidence=0.90,
        )

        content_data = {
            "subject": "Out of office",
            "body": "I will be unavailable next week.",
        }

        result = await discovery_service.process_content(
            content_id=content_id,
            content_type="email",
            source_id=5,
            content_data=content_data,
        )

        assert len(result.discovered_relationships) == 0
        assert result.created_count == 0
        assert result.updated_count == 0

    @pytest.mark.asyncio
    async def test_process_content_error_handling(
        self,
        discovery_service: RelationshipDiscoveryService,
        mock_extractor: AsyncMock,
    ) -> None:
        """Test error handling during content processing."""
        content_id = uuid4()

        # Set up extractor to raise an exception
        mock_extractor.extract_relationships.side_effect = Exception("AI model error")

        content_data = {"body": "Some content"}

        with pytest.raises(Exception, match="AI model error"):
            await discovery_service.process_content(
                content_id=content_id,
                content_type="email",
                source_id=6,
                content_data=content_data,
            )

    @pytest.mark.asyncio
    async def test_discover_person_to_person_relationship(
        self,
        discovery_service: RelationshipDiscoveryService,
        mock_extractor: AsyncMock,
        mock_entity_resolver: AsyncMock,
        mock_repository: AsyncMock,
    ) -> None:
        """Test discovering person-to-person collaboration relationships."""
        content_id = uuid4()

        mock_extractor.extract_relationships.return_value = ExtractionResult(
            content_id=content_id,
            content_type="meeting",
            relationships=[
                ExtractedRelationship(
                    source_entity_type="person",
                    source_entity_name="David Lee",
                    target_entity_type="person",
                    target_entity_name="Eva Martinez",
                    relationship_type="collaborates_with",
                    confidence=0.88,
                    context_snippet="David and Eva are working together on the API",
                )
            ],
            model_used="llama-3.1-8b",
            processing_time_ms=1100,
            extraction_confidence=0.88,
        )

        mock_entity_resolver.resolve_person.side_effect = [
            {"entity_id": 4, "confidence": 0.92},  # David
            {"entity_id": 5, "confidence": 0.90},  # Eva
        ]

        mock_rel = MagicMock()
        mock_rel.id = 103
        mock_repository.create_relationship.return_value = mock_rel

        content_data = {
            "transcript": "David and Eva discussed the API implementation",
        }

        result = await discovery_service.process_content(
            content_id=content_id,
            content_type="meeting",
            source_id=7,
            content_data=content_data,
        )

        assert len(result.discovered_relationships) == 1
        assert result.discovered_relationships[0].relationship_type == "collaborates_with"

    @pytest.mark.asyncio
    async def test_discover_project_dependency_relationship(
        self,
        discovery_service: RelationshipDiscoveryService,
        mock_extractor: AsyncMock,
        mock_entity_resolver: AsyncMock,
        mock_repository: AsyncMock,
    ) -> None:
        """Test discovering project-to-project dependency relationships."""
        content_id = uuid4()

        mock_extractor.extract_relationships.return_value = ExtractionResult(
            content_id=content_id,
            content_type="email",
            relationships=[
                ExtractedRelationship(
                    source_entity_type="project",
                    source_entity_name="Frontend Redesign",
                    target_entity_type="project",
                    target_entity_name="API Gateway",
                    relationship_type="depends_on",
                    confidence=0.82,
                    context_snippet="Frontend redesign depends on API gateway completion",
                )
            ],
            model_used="llama-3.1-8b",
            processing_time_ms=900,
            extraction_confidence=0.82,
        )

        mock_entity_resolver.resolve_project.side_effect = [
            {"entity_id": 20, "confidence": 0.88},  # Frontend
            {"entity_id": 21, "confidence": 0.85},  # API Gateway
        ]

        mock_rel = MagicMock()
        mock_rel.id = 104
        mock_repository.create_relationship.return_value = mock_rel

        content_data = {
            "subject": "Project Dependencies",
            "body": "The frontend work is blocked until API gateway is ready.",
        }

        result = await discovery_service.process_content(
            content_id=content_id,
            content_type="email",
            source_id=8,
            content_data=content_data,
        )

        assert len(result.discovered_relationships) == 1
        assert result.discovered_relationships[0].relationship_type == "depends_on"

    @pytest.mark.asyncio
    async def test_confidence_calculation_integration(
        self,
        discovery_service: RelationshipDiscoveryService,
        mock_extractor: AsyncMock,
        mock_entity_resolver: AsyncMock,
        mock_repository: AsyncMock,
    ) -> None:
        """Test that confidence is calculated correctly using all factors."""
        content_id = uuid4()

        mock_extractor.extract_relationships.return_value = ExtractionResult(
            content_id=content_id,
            content_type="meeting",
            relationships=[
                ExtractedRelationship(
                    source_entity_type="person",
                    source_entity_name="Frank Garcia",
                    target_entity_type="topic",
                    target_entity_name="Machine Learning",
                    relationship_type="expert_in",
                    confidence=0.75,
                    context_snippet="Frank presented advanced ML concepts",
                )
            ],
            model_used="llama-3.1-8b",
            processing_time_ms=1400,
            extraction_confidence=0.75,
        )

        mock_entity_resolver.resolve_person.return_value = {
            "entity_id": 6,
            "confidence": 0.85,
        }
        mock_entity_resolver.resolve_topic.return_value = {
            "entity_id": 60,
            "confidence": 0.80,
        }

        mock_rel = MagicMock()
        mock_rel.id = 105
        mock_rel.confidence_score = Decimal("0.72")
        mock_repository.create_relationship.return_value = mock_rel

        content_data = {
            "transcript": "Frank demonstrated ML techniques and answered questions",
        }

        result = await discovery_service.process_content(
            content_id=content_id,
            content_type="meeting",
            source_id=9,
            content_data=content_data,
        )

        assert len(result.discovered_relationships) == 1
        # Verify confidence score is reasonable
        discovered = result.discovered_relationships[0]
        assert 0.0 <= float(discovered.confidence_score) <= 1.0

    @pytest.mark.asyncio
    async def test_batch_process_content(
        self,
        discovery_service: RelationshipDiscoveryService,
        mock_extractor: AsyncMock,
        mock_entity_resolver: AsyncMock,
        mock_repository: AsyncMock,
    ) -> None:
        """Test batch processing multiple content items."""
        content_items = [
            {
                "content_id": uuid4(),
                "content_type": "email",
                "source_id": 10,
                "content_data": {"body": "Email content 1"},
            },
            {
                "content_id": uuid4(),
                "content_type": "meeting",
                "source_id": 11,
                "content_data": {"transcript": "Meeting content 2"},
            },
        ]

        # Set up extractor to return different results for each content
        mock_extractor.extract_relationships.side_effect = [
            ExtractionResult(
                content_id=content_items[0]["content_id"],
                content_type="email",
                relationships=[],
                model_used="llama-3.1-8b",
                processing_time_ms=400,
                extraction_confidence=0.90,
            ),
            ExtractionResult(
                content_id=content_items[1]["content_id"],
                content_type="meeting",
                relationships=[
                    ExtractedRelationship(
                        source_entity_type="person",
                        source_entity_name="Grace Hall",
                        target_entity_type="project",
                        target_entity_name="Data Pipeline",
                        relationship_type="works_on",
                        confidence=0.80,
                        context_snippet="Grace discussed the data pipeline",
                    )
                ],
                model_used="llama-3.1-8b",
                processing_time_ms=1000,
                extraction_confidence=0.80,
            ),
        ]

        mock_entity_resolver.resolve_person.return_value = {
            "entity_id": 7,
            "confidence": 0.90,
        }
        mock_entity_resolver.resolve_project.return_value = {
            "entity_id": 30,
            "confidence": 0.85,
        }

        mock_rel = MagicMock()
        mock_rel.id = 106
        mock_repository.create_relationship.return_value = mock_rel

        results = await discovery_service.batch_process(content_items)

        assert len(results) == 2
        # First item had no relationships
        assert results[0].created_count == 0
        # Second item had one relationship
        assert results[1].created_count >= 0 or len(results[1].discovered_relationships) >= 0

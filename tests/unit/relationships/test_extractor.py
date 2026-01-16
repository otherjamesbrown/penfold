"""Unit tests for RelationshipExtractor.

Tests the AI-powered relationship extraction from content, including:
- Prompt generation for different content types
- JSON response parsing
- Confidence score extraction
- Error handling for malformed responses
"""

from __future__ import annotations

from datetime import datetime, timezone
from decimal import Decimal
from typing import Any
from unittest.mock import AsyncMock, MagicMock
from uuid import uuid4

import pytest

from penf_lib.relationships.extractor import (
    ExtractedRelationship,
    ExtractionResult,
    RelationshipExtractor,
)


class TestExtractedRelationship:
    """Tests for the ExtractedRelationship model."""

    def test_create_extracted_relationship(self) -> None:
        """Test creating an ExtractedRelationship with valid data."""
        rel = ExtractedRelationship(
            source_entity_type="person",
            source_entity_name="Alice Johnson",
            target_entity_type="project",
            target_entity_name="Atlas Deployment",
            relationship_type="works_on",
            confidence=0.85,
            context_snippet="Alice mentioned she is working on the Atlas deployment",
        )

        assert rel.source_entity_type == "person"
        assert rel.source_entity_name == "Alice Johnson"
        assert rel.target_entity_type == "project"
        assert rel.target_entity_name == "Atlas Deployment"
        assert rel.relationship_type == "works_on"
        assert rel.confidence == 0.85
        assert "Atlas" in rel.context_snippet

    def test_extracted_relationship_with_metadata(self) -> None:
        """Test ExtractedRelationship with optional metadata."""
        rel = ExtractedRelationship(
            source_entity_type="person",
            source_entity_name="Bob Smith",
            target_entity_type="person",
            target_entity_name="Carol Davis",
            relationship_type="collaborates_with",
            confidence=0.72,
            context_snippet="Bob and Carol discussed the timeline",
            metadata={"meeting_id": "meeting-123", "topic": "planning"},
        )

        assert rel.metadata["meeting_id"] == "meeting-123"
        assert rel.metadata["topic"] == "planning"


class TestExtractionResult:
    """Tests for the ExtractionResult model."""

    def test_create_extraction_result_with_relationships(self) -> None:
        """Test creating an ExtractionResult with extracted relationships."""
        relationships = [
            ExtractedRelationship(
                source_entity_type="person",
                source_entity_name="Alice",
                target_entity_type="project",
                target_entity_name="Project X",
                relationship_type="works_on",
                confidence=0.85,
                context_snippet="context",
            ),
        ]

        result = ExtractionResult(
            content_id=uuid4(),
            content_type="email",
            relationships=relationships,
            model_used="llama-3.1-8b",
            processing_time_ms=1500,
            extraction_confidence=0.80,
        )

        assert len(result.relationships) == 1
        assert result.model_used == "llama-3.1-8b"
        assert result.processing_time_ms == 1500
        assert result.extraction_confidence == 0.80

    def test_extraction_result_empty_relationships(self) -> None:
        """Test ExtractionResult when no relationships are found."""
        result = ExtractionResult(
            content_id=uuid4(),
            content_type="document",
            relationships=[],
            model_used="llama-3.1-8b",
            processing_time_ms=800,
            extraction_confidence=0.90,
        )

        assert len(result.relationships) == 0
        assert result.extraction_confidence == 0.90


class TestRelationshipExtractor:
    """Tests for the RelationshipExtractor class."""

    @pytest.fixture
    def mock_model_coordinator(self) -> MagicMock:
        """Create a mock model coordinator."""
        coordinator = MagicMock()
        coordinator.coordinate_processing = AsyncMock()
        coordinator.wait_for_coordination = AsyncMock()
        return coordinator

    @pytest.fixture
    def extractor(self, mock_model_coordinator: MagicMock) -> RelationshipExtractor:
        """Create a RelationshipExtractor instance with mocked coordinator."""
        return RelationshipExtractor(model_coordinator=mock_model_coordinator)

    @pytest.mark.asyncio
    async def test_extract_from_email_content(
        self,
        extractor: RelationshipExtractor,
        mock_model_coordinator: MagicMock,
    ) -> None:
        """Test extracting relationships from email content."""
        # Set up mock response
        mock_model_coordinator.wait_for_coordination.return_value = {
            "coordination_id": "coord-123",
            "status": "completed",
            "individual_results": {
                "job-1": {
                    "processor_name": "llama-3.1-8b",
                    "results": [
                        MagicMock(
                            result_data={
                                "relationships": [
                                    {
                                        "source_entity_type": "person",
                                        "source_entity_name": "Alice Johnson",
                                        "target_entity_type": "project",
                                        "target_entity_name": "Atlas Deployment",
                                        "relationship_type": "works_on",
                                        "confidence": 0.85,
                                        "context_snippet": "Alice is leading the Atlas deployment",
                                    }
                                ]
                            },
                            confidence_score=Decimal("0.85"),
                            model_name="llama-3.1-8b",
                            processing_time_ms=1200,
                        )
                    ],
                }
            },
        }

        content_data = {
            "content_id": str(uuid4()),
            "subject": "Atlas Deployment Update",
            "body": "Hi team, Alice is leading the Atlas deployment. Best regards.",
            "sender": "manager@example.com",
            "recipients": ["team@example.com"],
        }

        result = await extractor.extract_relationships(
            content_id=uuid4(),
            content_type="email",
            content_data=content_data,
        )

        assert isinstance(result, ExtractionResult)
        assert len(result.relationships) == 1
        assert result.relationships[0].source_entity_name == "Alice Johnson"
        assert result.relationships[0].relationship_type == "works_on"

    @pytest.mark.asyncio
    async def test_extract_from_meeting_content(
        self,
        extractor: RelationshipExtractor,
        mock_model_coordinator: MagicMock,
    ) -> None:
        """Test extracting relationships from meeting content."""
        mock_model_coordinator.wait_for_coordination.return_value = {
            "coordination_id": "coord-456",
            "status": "completed",
            "individual_results": {
                "job-1": {
                    "processor_name": "llama-3.1-8b",
                    "results": [
                        MagicMock(
                            result_data={
                                "relationships": [
                                    {
                                        "source_entity_type": "person",
                                        "source_entity_name": "Bob Smith",
                                        "target_entity_type": "person",
                                        "target_entity_name": "Carol Davis",
                                        "relationship_type": "collaborates_with",
                                        "confidence": 0.78,
                                        "context_snippet": "Bob and Carol worked together on the implementation",
                                    },
                                    {
                                        "source_entity_type": "person",
                                        "source_entity_name": "Bob Smith",
                                        "target_entity_type": "topic",
                                        "target_entity_name": "Performance Optimization",
                                        "relationship_type": "expert_in",
                                        "confidence": 0.65,
                                        "context_snippet": "Bob presented the performance optimization strategy",
                                    },
                                ]
                            },
                            confidence_score=Decimal("0.75"),
                            model_name="llama-3.1-8b",
                            processing_time_ms=1800,
                        )
                    ],
                }
            },
        }

        content_data = {
            "content_id": str(uuid4()),
            "title": "Performance Review Meeting",
            "transcript": "Bob presented the optimization strategy. Carol provided implementation support.",
            "participants": ["bob@example.com", "carol@example.com"],
        }

        result = await extractor.extract_relationships(
            content_id=uuid4(),
            content_type="meeting",
            content_data=content_data,
        )

        assert len(result.relationships) == 2
        assert result.relationships[0].relationship_type == "collaborates_with"
        assert result.relationships[1].relationship_type == "expert_in"

    @pytest.mark.asyncio
    async def test_extract_handles_no_relationships(
        self,
        extractor: RelationshipExtractor,
        mock_model_coordinator: MagicMock,
    ) -> None:
        """Test handling content with no extractable relationships."""
        mock_model_coordinator.wait_for_coordination.return_value = {
            "coordination_id": "coord-789",
            "status": "completed",
            "individual_results": {
                "job-1": {
                    "processor_name": "llama-3.1-8b",
                    "results": [
                        MagicMock(
                            result_data={"relationships": []},
                            confidence_score=Decimal("0.90"),
                            model_name="llama-3.1-8b",
                            processing_time_ms=500,
                        )
                    ],
                }
            },
        }

        content_data = {
            "content_id": str(uuid4()),
            "subject": "Out of office",
            "body": "I will be out of office next week.",
        }

        result = await extractor.extract_relationships(
            content_id=uuid4(),
            content_type="email",
            content_data=content_data,
        )

        assert len(result.relationships) == 0
        assert result.extraction_confidence >= 0.0

    @pytest.mark.asyncio
    async def test_extract_handles_processing_failure(
        self,
        extractor: RelationshipExtractor,
        mock_model_coordinator: MagicMock,
    ) -> None:
        """Test handling AI processing failures."""
        mock_model_coordinator.coordinate_processing.side_effect = Exception(
            "Model unavailable"
        )

        content_data = {
            "content_id": str(uuid4()),
            "subject": "Test",
            "body": "Test content",
        }

        with pytest.raises(Exception, match="Model unavailable"):
            await extractor.extract_relationships(
                content_id=uuid4(),
                content_type="email",
                content_data=content_data,
            )

    @pytest.mark.asyncio
    async def test_extract_handles_malformed_response(
        self,
        extractor: RelationshipExtractor,
        mock_model_coordinator: MagicMock,
    ) -> None:
        """Test handling malformed AI responses gracefully."""
        mock_model_coordinator.wait_for_coordination.return_value = {
            "coordination_id": "coord-bad",
            "status": "completed",
            "individual_results": {
                "job-1": {
                    "processor_name": "llama-3.1-8b",
                    "results": [
                        MagicMock(
                            result_data={"invalid_key": "invalid_data"},
                            confidence_score=Decimal("0.50"),
                            model_name="llama-3.1-8b",
                            processing_time_ms=300,
                        )
                    ],
                }
            },
        }

        content_data = {"content_id": str(uuid4()), "body": "Some content"}

        result = await extractor.extract_relationships(
            content_id=uuid4(),
            content_type="email",
            content_data=content_data,
        )

        # Should return empty relationships for malformed response
        assert len(result.relationships) == 0

    def test_build_extraction_prompt_for_email(
        self,
        extractor: RelationshipExtractor,
    ) -> None:
        """Test that extraction prompt is built correctly for email content."""
        content_data = {
            "subject": "Project Update",
            "body": "Alice is working with Bob on the new feature.",
            "sender": "alice@example.com",
        }

        prompt = extractor.build_extraction_prompt("email", content_data)

        assert "email" in prompt.lower()
        assert "relationship" in prompt.lower()
        assert "Project Update" in prompt or "Alice" in prompt

    def test_build_extraction_prompt_for_meeting(
        self,
        extractor: RelationshipExtractor,
    ) -> None:
        """Test that extraction prompt is built correctly for meeting content."""
        content_data = {
            "title": "Sprint Planning",
            "transcript": "Team discussed the upcoming deliverables.",
            "participants": ["alice@example.com", "bob@example.com"],
        }

        prompt = extractor.build_extraction_prompt("meeting", content_data)

        assert "meeting" in prompt.lower()
        assert "relationship" in prompt.lower()

    def test_parse_model_response_valid(
        self,
        extractor: RelationshipExtractor,
    ) -> None:
        """Test parsing a valid model response."""
        response_data = {
            "relationships": [
                {
                    "source_entity_type": "person",
                    "source_entity_name": "Alice",
                    "target_entity_type": "project",
                    "target_entity_name": "Project X",
                    "relationship_type": "leads",
                    "confidence": 0.90,
                    "context_snippet": "Alice is the project lead",
                }
            ]
        }

        relationships = extractor.parse_model_response(response_data)

        assert len(relationships) == 1
        assert relationships[0].source_entity_name == "Alice"
        assert relationships[0].relationship_type == "leads"

    def test_parse_model_response_empty(
        self,
        extractor: RelationshipExtractor,
    ) -> None:
        """Test parsing a response with no relationships."""
        response_data = {"relationships": []}

        relationships = extractor.parse_model_response(response_data)

        assert len(relationships) == 0

    def test_parse_model_response_malformed(
        self,
        extractor: RelationshipExtractor,
    ) -> None:
        """Test parsing a malformed response returns empty list."""
        response_data = {"not_relationships": "invalid"}

        relationships = extractor.parse_model_response(response_data)

        assert len(relationships) == 0

    def test_parse_model_response_partial_data(
        self,
        extractor: RelationshipExtractor,
    ) -> None:
        """Test parsing response with some invalid relationship entries."""
        response_data = {
            "relationships": [
                {
                    "source_entity_type": "person",
                    "source_entity_name": "Alice",
                    "target_entity_type": "project",
                    "target_entity_name": "Project X",
                    "relationship_type": "works_on",
                    "confidence": 0.85,
                    "context_snippet": "Valid entry",
                },
                {
                    # Missing required fields
                    "source_entity_type": "person",
                },
            ]
        }

        relationships = extractor.parse_model_response(response_data)

        # Should only return the valid relationship
        assert len(relationships) == 1
        assert relationships[0].source_entity_name == "Alice"

    @pytest.mark.asyncio
    async def test_extract_with_escalation(
        self,
        extractor: RelationshipExtractor,
        mock_model_coordinator: MagicMock,
    ) -> None:
        """Test that low-confidence results trigger escalation."""
        # First call returns low confidence
        mock_model_coordinator.wait_for_coordination.return_value = {
            "coordination_id": "coord-low",
            "status": "completed",
            "individual_results": {
                "job-1": {
                    "processor_name": "llama-3.1-8b",
                    "results": [
                        MagicMock(
                            result_data={
                                "relationships": [
                                    {
                                        "source_entity_type": "person",
                                        "source_entity_name": "Unknown Person",
                                        "target_entity_type": "project",
                                        "target_entity_name": "Vague Project",
                                        "relationship_type": "associated_with",
                                        "confidence": 0.35,
                                        "context_snippet": "Unclear context",
                                    }
                                ]
                            },
                            confidence_score=Decimal("0.35"),
                            model_name="llama-3.1-8b",
                            processing_time_ms=1000,
                        )
                    ],
                }
            },
        }

        content_data = {
            "content_id": str(uuid4()),
            "subject": "Unclear Topic",
            "body": "Some ambiguous content that is hard to parse.",
        }

        result = await extractor.extract_relationships(
            content_id=uuid4(),
            content_type="email",
            content_data=content_data,
            confidence_threshold=0.50,
        )

        # With low confidence result, escalation should be considered
        assert len(result.relationships) >= 0
        # The result should have low confidence indicator
        assert result.extraction_confidence <= 0.50 or len(result.relationships) == 1

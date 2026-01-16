"""RelationshipExtractor - AI-powered relationship extraction from content.

This module provides relationship extraction using AI models coordinated
through the ModelCoordinator framework. It generates structured prompts
for different content types and parses model responses into relationship data.
"""

from __future__ import annotations

import logging
import time
from dataclasses import dataclass, field
from decimal import Decimal
from typing import Any
from uuid import UUID

logger = logging.getLogger(__name__)


@dataclass
class ExtractedRelationship:
    """A relationship extracted from content by AI analysis.

    Represents a raw extraction result before entity resolution
    and confidence calculation.
    """

    source_entity_type: str
    source_entity_name: str
    target_entity_type: str
    target_entity_name: str
    relationship_type: str
    confidence: float
    context_snippet: str
    metadata: dict[str, Any] = field(default_factory=dict)


@dataclass
class ExtractionResult:
    """Result of relationship extraction from a single content item.

    Contains all extracted relationships along with processing metadata.
    """

    content_id: UUID
    content_type: str
    relationships: list[ExtractedRelationship]
    model_used: str
    processing_time_ms: int
    extraction_confidence: float
    metadata: dict[str, Any] = field(default_factory=dict)


class RelationshipExtractor:
    """Extracts relationships from content using AI model coordination.

    Integrates with the ModelCoordinator framework to process content
    through AI models and extract structured relationship information.

    Attributes:
        model_coordinator: The AI model coordinator for processing
        default_model: Default model to use for extraction
        confidence_threshold: Minimum confidence for escalation consideration
    """

    # Prompt template for relationship extraction
    EXTRACTION_PROMPT_TEMPLATE = """Analyze the following {content_type} content and extract all relationships between entities.

For each relationship found, identify:
1. Source entity (person, project, or topic) with their name
2. Target entity (person, project, or topic) with their name
3. Type of relationship (works_on, collaborates_with, leads, reports_to, manages, contributes_to, stakeholder_of, depends_on, blocks, related_to, discusses, expert_in, interested_in, associated_with)
4. Your confidence in this relationship (0.0 to 1.0)
5. A brief context snippet from the content supporting this relationship

Content:
{content}

{additional_context}

Respond in JSON format:
{{
    "relationships": [
        {{
            "source_entity_type": "person|project|topic",
            "source_entity_name": "entity name",
            "target_entity_type": "person|project|topic",
            "target_entity_name": "entity name",
            "relationship_type": "type from list above",
            "confidence": 0.0-1.0,
            "context_snippet": "brief supporting text"
        }}
    ]
}}

If no relationships are found, return {{"relationships": []}}"""

    # Content-specific context builders
    CONTENT_CONTEXT_BUILDERS = {
        "email": lambda data: f"""
Email Subject: {data.get('subject', 'N/A')}
From: {data.get('sender', 'N/A')}
To: {', '.join(data.get('recipients', []))}
""",
        "meeting": lambda data: f"""
Meeting Title: {data.get('title', 'N/A')}
Participants: {', '.join(data.get('participants', []))}
""",
        "document": lambda data: f"""
Document Title: {data.get('title', 'N/A')}
Author: {data.get('author', 'N/A')}
""",
    }

    def __init__(
        self,
        model_coordinator: Any,
        default_model: str = "llama-3.1-8b",
        confidence_threshold: float = 0.50,
    ) -> None:
        """Initialize the RelationshipExtractor.

        Args:
            model_coordinator: ModelCoordinator instance for AI processing
            default_model: Default model ID for extraction
            confidence_threshold: Minimum confidence before considering escalation
        """
        self.model_coordinator = model_coordinator
        self.default_model = default_model
        self.confidence_threshold = confidence_threshold

    async def extract_relationships(
        self,
        content_id: UUID,
        content_type: str,
        content_data: dict[str, Any],
        confidence_threshold: float | None = None,
    ) -> ExtractionResult:
        """Extract relationships from content using AI coordination.

        Args:
            content_id: Unique identifier for the content
            content_type: Type of content (email, meeting, document)
            content_data: Content data including body/transcript
            confidence_threshold: Override default confidence threshold

        Returns:
            ExtractionResult containing extracted relationships

        Raises:
            Exception: If AI processing fails
        """
        start_time = time.time()
        # Note: confidence_threshold can be used for escalation decisions in future
        _ = confidence_threshold or self.confidence_threshold

        # Build extraction prompt
        prompt = self.build_extraction_prompt(content_type, content_data)

        # Coordinate AI processing
        coordination_id = await self.model_coordinator.coordinate_processing(
            content_id=str(content_id),
            content_type=content_type,
            content_data={
                "prompt": prompt,
                "extraction_type": "relationship",
                **content_data,
            },
            target_models=[self.default_model],
        )

        # Wait for results
        coordination_result = await self.model_coordinator.wait_for_coordination(
            coordination_id=coordination_id,
            min_results=1,
        )

        # Process results
        relationships: list[ExtractedRelationship] = []
        model_used = self.default_model
        extraction_confidence = 0.0

        individual_results = coordination_result.get("individual_results", {})
        for job_data in individual_results.values():
            for result in job_data.get("results", []):
                result_data = getattr(result, "result_data", {})
                if isinstance(result_data, dict):
                    parsed = self.parse_model_response(result_data)
                    relationships.extend(parsed)

                    model_used = getattr(result, "model_name", self.default_model)
                    conf_score = getattr(result, "confidence_score", Decimal("0.5"))
                    extraction_confidence = max(
                        extraction_confidence, float(conf_score)
                    )

        processing_time_ms = int((time.time() - start_time) * 1000)

        return ExtractionResult(
            content_id=content_id,
            content_type=content_type,
            relationships=relationships,
            model_used=model_used,
            processing_time_ms=processing_time_ms,
            extraction_confidence=extraction_confidence,
        )

    def build_extraction_prompt(
        self,
        content_type: str,
        content_data: dict[str, Any],
    ) -> str:
        """Build the extraction prompt for a specific content type.

        Args:
            content_type: Type of content (email, meeting, document)
            content_data: Content data to include in prompt

        Returns:
            Formatted prompt string for AI model
        """
        # Get content body/text
        content_text = content_data.get(
            "body",
            content_data.get(
                "transcript",
                content_data.get("content", str(content_data)),
            ),
        )

        # Build additional context based on content type
        context_builder = self.CONTENT_CONTEXT_BUILDERS.get(
            content_type, lambda _: ""
        )
        additional_context = context_builder(content_data)

        return self.EXTRACTION_PROMPT_TEMPLATE.format(
            content_type=content_type,
            content=content_text,
            additional_context=additional_context,
        )

    def parse_model_response(
        self,
        response_data: dict[str, Any],
    ) -> list[ExtractedRelationship]:
        """Parse AI model response into ExtractedRelationship objects.

        Args:
            response_data: Raw response data from AI model

        Returns:
            List of parsed ExtractedRelationship objects
        """
        relationships: list[ExtractedRelationship] = []

        raw_relationships = response_data.get("relationships", [])
        if not isinstance(raw_relationships, list):
            logger.warning("Invalid response format: 'relationships' is not a list")
            return relationships

        for rel_data in raw_relationships:
            if not isinstance(rel_data, dict):
                continue

            # Validate required fields
            required_fields = [
                "source_entity_type",
                "source_entity_name",
                "target_entity_type",
                "target_entity_name",
                "relationship_type",
                "confidence",
                "context_snippet",
            ]

            if not all(field in rel_data for field in required_fields):
                logger.debug(f"Skipping incomplete relationship data: {rel_data}")
                continue

            try:
                relationship = ExtractedRelationship(
                    source_entity_type=str(rel_data["source_entity_type"]).lower(),
                    source_entity_name=str(rel_data["source_entity_name"]),
                    target_entity_type=str(rel_data["target_entity_type"]).lower(),
                    target_entity_name=str(rel_data["target_entity_name"]),
                    relationship_type=str(rel_data["relationship_type"]).lower(),
                    confidence=float(rel_data["confidence"]),
                    context_snippet=str(rel_data["context_snippet"]),
                    metadata=rel_data.get("metadata", {}),
                )
                relationships.append(relationship)
            except (ValueError, TypeError) as e:
                logger.warning(f"Failed to parse relationship data: {e}")
                continue

        return relationships


__all__ = [
    "ExtractedRelationship",
    "ExtractionResult",
    "RelationshipExtractor",
]

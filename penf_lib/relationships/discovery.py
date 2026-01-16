"""RelationshipDiscoveryService - Orchestrates relationship discovery from content.

This module provides the main service for processing content and discovering
relationships. It coordinates extraction, entity resolution, confidence
calculation, and persistence to the database.
"""

from __future__ import annotations

import logging
import time
from dataclasses import dataclass, field
from datetime import datetime, timezone
from decimal import Decimal
from typing import Any, Protocol
from uuid import UUID

from sqlalchemy.ext.asyncio import AsyncSession

from .confidence import ConfidenceCalculator, EvidenceItem
from .extractor import ExtractedRelationship, RelationshipExtractor

logger = logging.getLogger(__name__)


class EntityResolver(Protocol):
    """Protocol for entity resolution services."""

    async def resolve_person(
        self, name: str, context: dict[str, Any] | None = None
    ) -> dict[str, Any] | None:
        """Resolve a person name to an entity ID."""
        ...

    async def resolve_project(
        self, name: str, context: dict[str, Any] | None = None
    ) -> dict[str, Any] | None:
        """Resolve a project name to an entity ID."""
        ...

    async def resolve_topic(
        self, name: str, context: dict[str, Any] | None = None
    ) -> dict[str, Any] | None:
        """Resolve a topic name to an entity ID."""
        ...


class RelationshipRepository(Protocol):
    """Protocol for relationship repository operations."""

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
    ) -> Any:
        """Create a new relationship."""
        ...

    async def find_by_entity_pair(
        self,
        tenant_id: UUID,
        source_type: str,
        source_id: int,
        target_type: str,
        target_id: int,
        relationship_type: str | None = None,
    ) -> list[Any]:
        """Find relationships between entity pair."""
        ...

    async def update_confidence(
        self,
        relationship_id: int,
        new_confidence: Decimal,
        evidence_strength: Decimal | None = None,
    ) -> Any | None:
        """Update relationship confidence."""
        ...

    async def add_evidence(
        self,
        tenant_id: UUID,
        relationship_id: int,
        evidence_type: str,
        strength_contribution: Decimal,
        evidence_content: str | None = None,
        source_id: int | None = None,
        evidence_context: dict[str, Any] | None = None,
    ) -> Any:
        """Add evidence to a relationship."""
        ...

    async def increment_interaction_count(
        self,
        relationship_id: int,
    ) -> Any | None:
        """Increment relationship interaction count."""
        ...


@dataclass
class DiscoveredRelationship:
    """A relationship discovered through content analysis.

    Contains the full relationship data with entity IDs
    and confidence information.
    """

    source_entity_type: str
    source_entity_id: int
    target_entity_type: str
    target_entity_id: int
    relationship_type: str
    confidence_score: Decimal
    ai_confidence: Decimal
    evidence_strength: Decimal
    lifecycle_state: str
    evidence: list[dict[str, Any]] = field(default_factory=list)
    metadata: dict[str, Any] = field(default_factory=dict)


@dataclass
class DiscoveryResult:
    """Result of relationship discovery for a content item.

    Tracks discovered relationships and processing statistics.
    """

    content_id: UUID
    content_type: str
    source_id: int
    discovered_relationships: list[DiscoveredRelationship]
    created_count: int
    updated_count: int
    processing_time_ms: int
    errors: list[str] = field(default_factory=list)


class RelationshipDiscoveryService:
    """Orchestrates relationship discovery from content.

    Coordinates extraction, entity resolution, confidence calculation,
    and persistence of relationships.

    Attributes:
        session: Database session for persistence
        repository: Relationship repository for CRUD operations
        extractor: AI-powered relationship extractor
        entity_resolver: Service for resolving entity names to IDs
        tenant_id: Current tenant ID for isolation
        confidence_calculator: Calculator for confidence scores
    """

    def __init__(
        self,
        session: AsyncSession,
        repository: RelationshipRepository,
        extractor: RelationshipExtractor,
        entity_resolver: EntityResolver,
        tenant_id: UUID,
        confidence_calculator: ConfidenceCalculator | None = None,
    ) -> None:
        """Initialize the discovery service.

        Args:
            session: Async database session
            repository: Relationship repository instance
            extractor: Relationship extractor instance
            entity_resolver: Entity resolution service
            tenant_id: Tenant ID for isolation
            confidence_calculator: Optional confidence calculator
        """
        self.session = session
        self.repository = repository
        self.extractor = extractor
        self.entity_resolver = entity_resolver
        self.tenant_id = tenant_id
        self.confidence_calculator = confidence_calculator or ConfidenceCalculator()

    async def process_content(
        self,
        content_id: UUID,
        content_type: str,
        source_id: int,
        content_data: dict[str, Any],
    ) -> DiscoveryResult:
        """Process content to discover relationships.

        Main entry point for relationship discovery. Extracts relationships
        using AI, resolves entities, calculates confidence, and persists.

        Args:
            content_id: Unique identifier for the content
            content_type: Type of content (email, meeting, document)
            source_id: Database ID of the source record
            content_data: Content data including body/transcript

        Returns:
            DiscoveryResult with discovered relationships and statistics

        Raises:
            Exception: If extraction fails
        """
        start_time = time.time()
        discovered: list[DiscoveredRelationship] = []
        created_count = 0
        updated_count = 0
        errors: list[str] = []

        # Extract relationships using AI
        extraction_result = await self.extractor.extract_relationships(
            content_id=content_id,
            content_type=content_type,
            content_data=content_data,
        )

        # Process each extracted relationship
        for extracted in extraction_result.relationships:
            try:
                result = await self._process_extracted_relationship(
                    extracted=extracted,
                    content_type=content_type,
                    source_id=source_id,
                    extraction_confidence=extraction_result.extraction_confidence,
                )

                if result is not None:
                    discovered_rel, is_new = result
                    discovered.append(discovered_rel)
                    if is_new:
                        created_count += 1
                    else:
                        updated_count += 1

            except Exception as e:
                logger.warning(f"Failed to process relationship: {e}")
                errors.append(str(e))

        processing_time_ms = int((time.time() - start_time) * 1000)

        return DiscoveryResult(
            content_id=content_id,
            content_type=content_type,
            source_id=source_id,
            discovered_relationships=discovered,
            created_count=created_count,
            updated_count=updated_count,
            processing_time_ms=processing_time_ms,
            errors=errors,
        )

    async def _process_extracted_relationship(
        self,
        extracted: ExtractedRelationship,
        content_type: str,
        source_id: int,
        extraction_confidence: float,
    ) -> tuple[DiscoveredRelationship, bool] | None:
        """Process a single extracted relationship.

        Resolves entities, calculates confidence, and creates or updates
        the relationship in the database.

        Args:
            extracted: The extracted relationship data
            content_type: Type of source content
            source_id: Database ID of source
            extraction_confidence: Overall extraction confidence

        Returns:
            Tuple of (DiscoveredRelationship, is_new) or None if cannot process
        """
        # Resolve source entity
        source_resolved = await self._resolve_entity(
            extracted.source_entity_type,
            extracted.source_entity_name,
        )
        if source_resolved is None:
            logger.debug(
                f"Could not resolve source entity: {extracted.source_entity_name}"
            )
            return None

        # Resolve target entity
        target_resolved = await self._resolve_entity(
            extracted.target_entity_type,
            extracted.target_entity_name,
        )
        if target_resolved is None:
            logger.debug(
                f"Could not resolve target entity: {extracted.target_entity_name}"
            )
            return None

        # Calculate confidence
        entity_resolution_confidence = min(
            source_resolved.get("confidence", 1.0),
            target_resolved.get("confidence", 1.0),
        )

        # Create evidence item for confidence calculation
        now = datetime.now(timezone.utc)
        evidence_item = EvidenceItem(
            evidence_type=content_type,
            source_id=source_id,  # type: ignore
            observed_at=now,
            strength_contribution=extracted.confidence * 0.5,  # Scale down
            content_snippet=extracted.context_snippet,
        )

        final_confidence = self.confidence_calculator.calculate_relationship_confidence(
            ai_confidence=extracted.confidence,
            evidence_items=[evidence_item],
            last_evidence_at=now,
            interaction_count=1,
            entity_resolution_confidence=entity_resolution_confidence,
        )

        lifecycle_state = self.confidence_calculator.to_lifecycle_state(
            final_confidence
        )

        # Check for existing relationship
        existing = await self.repository.find_by_entity_pair(
            tenant_id=self.tenant_id,
            source_type=extracted.source_entity_type,
            source_id=source_resolved["entity_id"],
            target_type=extracted.target_entity_type,
            target_id=target_resolved["entity_id"],
            relationship_type=extracted.relationship_type,
        )

        evidence_data = {
            "evidence_type": content_type,
            "source_id": source_id,
            "context_snippet": extracted.context_snippet,
            "strength_contribution": extracted.confidence * 0.5,
        }

        if existing:
            # Update existing relationship
            existing_rel = existing[0]
            await self.repository.add_evidence(
                tenant_id=self.tenant_id,
                relationship_id=existing_rel.id,
                evidence_type=content_type,
                strength_contribution=Decimal(str(extracted.confidence * 0.5)),
                evidence_content=extracted.context_snippet,
                source_id=source_id,
            )
            await self.repository.increment_interaction_count(existing_rel.id)

            # Recalculate confidence with new evidence
            new_confidence = self.confidence_calculator.recalculate_from_evidence(
                existing_confidence=float(existing_rel.confidence_score),
                new_evidence=evidence_item,
                existing_evidence_count=existing_rel.interaction_count,
            )
            await self.repository.update_confidence(
                relationship_id=existing_rel.id,
                new_confidence=Decimal(str(new_confidence)),
            )

            discovered = DiscoveredRelationship(
                source_entity_type=extracted.source_entity_type,
                source_entity_id=source_resolved["entity_id"],
                target_entity_type=extracted.target_entity_type,
                target_entity_id=target_resolved["entity_id"],
                relationship_type=extracted.relationship_type,
                confidence_score=Decimal(str(new_confidence)),
                ai_confidence=Decimal(str(extracted.confidence)),
                evidence_strength=Decimal(str(extracted.confidence * 0.5)),
                lifecycle_state=lifecycle_state,
                evidence=[evidence_data],
            )
            return (discovered, False)

        else:
            # Create new relationship
            new_rel = await self.repository.create_relationship(
                tenant_id=self.tenant_id,
                source_entity_type=extracted.source_entity_type,
                source_entity_id=source_resolved["entity_id"],
                target_entity_type=extracted.target_entity_type,
                target_entity_id=target_resolved["entity_id"],
                relationship_type=extracted.relationship_type,
                confidence_score=Decimal(str(final_confidence)),
                ai_confidence=Decimal(str(extracted.confidence)),
                evidence_strength=Decimal(str(extracted.confidence * 0.5)),
                lifecycle_state=lifecycle_state,
            )

            # Add initial evidence
            await self.repository.add_evidence(
                tenant_id=self.tenant_id,
                relationship_id=new_rel.id,
                evidence_type=content_type,
                strength_contribution=Decimal(str(extracted.confidence * 0.5)),
                evidence_content=extracted.context_snippet,
                source_id=source_id,
            )

            discovered = DiscoveredRelationship(
                source_entity_type=extracted.source_entity_type,
                source_entity_id=source_resolved["entity_id"],
                target_entity_type=extracted.target_entity_type,
                target_entity_id=target_resolved["entity_id"],
                relationship_type=extracted.relationship_type,
                confidence_score=Decimal(str(final_confidence)),
                ai_confidence=Decimal(str(extracted.confidence)),
                evidence_strength=Decimal(str(extracted.confidence * 0.5)),
                lifecycle_state=lifecycle_state,
                evidence=[evidence_data],
            )
            return (discovered, True)

    async def _resolve_entity(
        self,
        entity_type: str,
        entity_name: str,
    ) -> dict[str, Any] | None:
        """Resolve an entity name to its database ID.

        Args:
            entity_type: Type of entity (person, project, topic)
            entity_name: Name to resolve

        Returns:
            Resolution result with entity_id and confidence, or None
        """
        resolver_map = {
            "person": self.entity_resolver.resolve_person,
            "project": self.entity_resolver.resolve_project,
            "topic": self.entity_resolver.resolve_topic,
        }

        resolver = resolver_map.get(entity_type)
        if resolver is None:
            logger.warning(f"Unknown entity type: {entity_type}")
            return None

        return await resolver(entity_name)

    async def batch_process(
        self,
        content_items: list[dict[str, Any]],
    ) -> list[DiscoveryResult]:
        """Process multiple content items for relationship discovery.

        Args:
            content_items: List of content item dictionaries with keys:
                - content_id: UUID
                - content_type: str
                - source_id: int
                - content_data: dict

        Returns:
            List of DiscoveryResult for each content item
        """
        results = []
        for item in content_items:
            try:
                result = await self.process_content(
                    content_id=item["content_id"],
                    content_type=item["content_type"],
                    source_id=item["source_id"],
                    content_data=item["content_data"],
                )
                results.append(result)
            except Exception as e:
                logger.error(f"Failed to process content {item.get('content_id')}: {e}")
                # Create error result
                results.append(
                    DiscoveryResult(
                        content_id=item["content_id"],
                        content_type=item.get("content_type", "unknown"),
                        source_id=item.get("source_id", 0),
                        discovered_relationships=[],
                        created_count=0,
                        updated_count=0,
                        processing_time_ms=0,
                        errors=[str(e)],
                    )
                )

        return results


__all__ = [
    "DiscoveredRelationship",
    "DiscoveryResult",
    "EntityResolver",
    "RelationshipDiscoveryService",
    "RelationshipRepository",
]

"""Relationship Search Integration Module.

This module provides integration between the relationship discovery system
and search functionality, enabling relationship-enhanced queries.

Key Features:
- RelationshipContextProvider for gathering relationship context
- Query expansion using relationship pathways
- Result ranking boost based on relationship strength
- Pathway traversal for indirect connections
"""

from __future__ import annotations

import logging
from dataclasses import dataclass, field
from decimal import Decimal
from typing import Any, Protocol
from uuid import UUID

from sqlalchemy.ext.asyncio import AsyncSession

logger = logging.getLogger(__name__)


# ============================================================================
# DATA MODELS
# ============================================================================


@dataclass
class RelatedEntity:
    """An entity related to the search context through relationships.

    Represents a node in the relationship graph that is connected to
    the entity being searched. Used to expand search scope and boost
    results from related entities.

    Attributes:
        entity_id: Database ID of the related entity
        entity_type: Type of entity (person, project, topic)
        display_name: Human-readable name for display
        relationship_type: Type of relationship connecting this entity
        relationship_strength: Combined strength of the relationship (0.0-1.0)
        path_length: Number of hops from the source entity (1 = direct)
        via_entities: Names of intermediate entities for indirect connections
        context: Additional context about the relationship
    """

    entity_id: int
    entity_type: str
    display_name: str
    relationship_type: str
    relationship_strength: float
    path_length: int = 1
    via_entities: list[str] = field(default_factory=list)
    context: dict[str, Any] = field(default_factory=dict)


@dataclass
class PathwayStep:
    """A single step in a relationship pathway.

    Represents one hop in a path between two entities,
    including the relationship type and strength.

    Attributes:
        from_entity_id: Starting entity ID for this step
        from_entity_type: Type of starting entity
        from_entity_name: Display name of starting entity
        to_entity_id: Ending entity ID for this step
        to_entity_type: Type of ending entity
        to_entity_name: Display name of ending entity
        relationship_type: Type of relationship for this step
        strength: Relationship confidence score
    """

    from_entity_id: int
    from_entity_type: str
    from_entity_name: str
    to_entity_id: int
    to_entity_type: str
    to_entity_name: str
    relationship_type: str
    strength: float


@dataclass
class PathwayResult:
    """Result of pathway discovery between two entities.

    Contains the complete path from source to target entity,
    including all intermediate steps and combined strength.

    Attributes:
        source_entity_id: Starting entity ID
        target_entity_id: Target entity ID
        path_length: Number of steps in the path (0 if no path)
        combined_strength: Product of all step strengths
        steps: List of PathwayStep objects forming the path
        path_found: Whether a path was found
    """

    source_entity_id: int
    target_entity_id: int
    path_length: int
    combined_strength: float
    steps: list[PathwayStep] = field(default_factory=list)
    path_found: bool = True


@dataclass
class QueryExpansion:
    """Query expansion based on relationship context.

    Contains additional search terms and entity IDs derived
    from relationships to expand the search scope.

    Attributes:
        original_query: The original user query
        expanded_terms: Additional terms derived from relationships
        related_entity_ids: IDs of entities to include in search
        relationship_context: Descriptions of relevant relationships
        expansion_confidence: Confidence in the expansion (0.0-1.0)
    """

    original_query: str
    expanded_terms: list[str]
    related_entity_ids: list[int]
    relationship_context: list[str]
    expansion_confidence: float


@dataclass
class SearchContextEnhancement:
    """Complete enhancement of search context with relationships.

    Bundles all relationship-derived enhancements for a search query.

    Attributes:
        original_query: The original user query
        related_entities: Entities related to the search context
        query_expansion: Expanded query terms and context
        relationship_filters: Filters to apply based on relationships
        boost_factors: Boost factors for result ranking by entity
    """

    original_query: str
    related_entities: list[RelatedEntity]
    query_expansion: QueryExpansion | None = None
    relationship_filters: dict[str, Any] = field(default_factory=dict)
    boost_factors: dict[str, float] = field(default_factory=dict)


# ============================================================================
# PROTOCOLS
# ============================================================================


class RelationshipRepositoryProtocol(Protocol):
    """Protocol for relationship repository operations."""

    async def find_by_source_entity(
        self,
        tenant_id: UUID,
        entity_type: str,
        entity_id: int,
        relationship_type: str | None = None,
    ) -> list[Any]:
        """Find relationships by source entity."""
        ...

    async def find_by_target_entity(
        self,
        tenant_id: UUID,
        entity_type: str,
        entity_id: int,
        relationship_type: str | None = None,
    ) -> list[Any]:
        """Find relationships by target entity."""
        ...

    async def find_by_confidence_threshold(
        self,
        tenant_id: UUID,
        min_confidence: Decimal,
        lifecycle_state: str | None = None,
        limit: int = 100,
    ) -> list[Any]:
        """Find relationships meeting confidence threshold."""
        ...


# ============================================================================
# RELATIONSHIP CONTEXT PROVIDER
# ============================================================================


class RelationshipContextProvider:
    """Provides relationship context for search enhancement.

    Gathers related entities, expands queries, and discovers pathways
    between entities to enhance search functionality.

    Attributes:
        session: Database session for queries
        repository: Relationship repository for data access
        tenant_id: Tenant ID for multi-tenant isolation
    """

    def __init__(
        self,
        session: AsyncSession,
        repository: RelationshipRepositoryProtocol,
        tenant_id: UUID,
        entity_name_resolver: Any | None = None,
    ) -> None:
        """Initialize the context provider.

        Args:
            session: Async database session
            repository: Relationship repository instance
            tenant_id: Tenant ID for isolation
            entity_name_resolver: Optional resolver for entity display names
        """
        self.session = session
        self.repository = repository
        self.tenant_id = tenant_id
        self.entity_name_resolver = entity_name_resolver

    async def get_related_entities(
        self,
        entity_type: str,
        entity_id: int,
        max_depth: int = 1,
        min_confidence: float = 0.5,
        relationship_types: list[str] | None = None,
    ) -> list[RelatedEntity]:
        """Get entities related to the given entity.

        Traverses the relationship graph to find connected entities
        up to the specified depth.

        Args:
            entity_type: Type of the source entity
            entity_id: ID of the source entity
            max_depth: Maximum relationship hops to traverse
            min_confidence: Minimum confidence threshold
            relationship_types: Optional filter for specific relationship types

        Returns:
            List of related entities with relationship information
        """
        related: list[RelatedEntity] = []
        visited: set[tuple[str, int]] = {(entity_type, entity_id)}
        current_level: list[tuple[str, int, int, list[str]]] = [
            (entity_type, entity_id, 0, [])
        ]

        while current_level:
            next_level: list[tuple[str, int, int, list[str]]] = []

            for curr_type, curr_id, depth, via in current_level:
                if depth >= max_depth:
                    continue

                # Find outgoing relationships from this entity
                try:
                    relationships = await self.repository.find_by_source_entity(
                        tenant_id=self.tenant_id,
                        entity_type=curr_type,
                        entity_id=curr_id,
                        relationship_type=relationship_types[0]
                        if relationship_types and len(relationship_types) == 1
                        else None,
                    )
                except Exception as e:
                    logger.warning(
                        f"Error finding relationships for {curr_type}/{curr_id}: {e}"
                    )
                    continue

                for rel in relationships:
                    target_key = (rel.target_entity_type, rel.target_entity_id)

                    # Skip if already visited
                    if target_key in visited:
                        continue

                    # Check confidence threshold
                    confidence = float(rel.confidence_score)
                    if confidence < min_confidence:
                        continue

                    # Check relationship type filter
                    if relationship_types and rel.relationship_type not in relationship_types:
                        continue

                    visited.add(target_key)

                    # Create related entity entry
                    related.append(
                        RelatedEntity(
                            entity_id=rel.target_entity_id,
                            entity_type=rel.target_entity_type,
                            display_name=await self._get_entity_name(
                                rel.target_entity_type, rel.target_entity_id
                            ),
                            relationship_type=rel.relationship_type,
                            relationship_strength=confidence,
                            path_length=depth + 1,
                            via_entities=via.copy(),
                        )
                    )

                    # Queue for next level traversal
                    new_via = via + [
                        await self._get_entity_name(curr_type, curr_id)
                    ]
                    next_level.append(
                        (
                            rel.target_entity_type,
                            rel.target_entity_id,
                            depth + 1,
                            new_via,
                        )
                    )

            current_level = next_level

        # Sort by relationship strength (descending)
        related.sort(key=lambda x: x.relationship_strength, reverse=True)

        return related

    async def expand_query(
        self,
        query: str,
        context_entity_type: str,
        context_entity_id: int,
        max_related: int = 10,
    ) -> QueryExpansion:
        """Expand a query using relationship context.

        Adds related entity IDs and context descriptions to
        help broaden search results.

        Args:
            query: Original search query
            context_entity_type: Type of context entity
            context_entity_id: ID of context entity
            max_related: Maximum number of related entities to include

        Returns:
            QueryExpansion with additional terms and context
        """
        related = await self.get_related_entities(
            entity_type=context_entity_type,
            entity_id=context_entity_id,
            max_depth=1,
            min_confidence=0.5,
        )

        # Limit to max_related
        related = related[:max_related]

        # Collect entity IDs for search expansion
        related_ids = [r.entity_id for r in related]

        # Build relationship context descriptions
        context_descriptions = []
        for r in related:
            context_descriptions.append(
                f"{await self._get_entity_name(context_entity_type, context_entity_id)} "
                f"{r.relationship_type.replace('_', ' ')} {r.display_name}"
            )

        # Extract potential terms from related entity names
        expanded_terms = [query]
        for r in related:
            terms = r.display_name.split()
            expanded_terms.extend(terms)

        # Calculate confidence based on number and strength of relationships
        if related:
            avg_strength = sum(r.relationship_strength for r in related) / len(related)
            confidence = avg_strength * min(1.0, len(related) / max_related)
        else:
            confidence = 0.0

        return QueryExpansion(
            original_query=query,
            expanded_terms=list(set(expanded_terms)),  # Deduplicate
            related_entity_ids=related_ids,
            relationship_context=context_descriptions,
            expansion_confidence=confidence,
        )

    async def find_pathway(
        self,
        source_entity_type: str,
        source_entity_id: int,
        target_entity_type: str,
        target_entity_id: int,
        max_depth: int = 3,
    ) -> PathwayResult:
        """Find the shortest pathway between two entities.

        Uses BFS to find the shortest path in the relationship graph
        connecting the source and target entities.

        Args:
            source_entity_type: Type of source entity
            source_entity_id: ID of source entity
            target_entity_type: Type of target entity
            target_entity_id: ID of target entity
            max_depth: Maximum path length to search

        Returns:
            PathwayResult with the path or indication of no path
        """
        if source_entity_type == target_entity_type and source_entity_id == target_entity_id:
            return PathwayResult(
                source_entity_id=source_entity_id,
                target_entity_id=target_entity_id,
                path_length=0,
                combined_strength=1.0,
                steps=[],
                path_found=True,
            )

        # BFS for shortest path
        visited: set[tuple[str, int]] = {(source_entity_type, source_entity_id)}
        # Queue: (current_type, current_id, path_so_far, strength_product)
        queue: list[tuple[str, int, list[PathwayStep], float]] = [
            (source_entity_type, source_entity_id, [], 1.0)
        ]

        while queue:
            curr_type, curr_id, path, strength = queue.pop(0)

            if len(path) >= max_depth:
                continue

            # Find outgoing relationships
            try:
                relationships = await self.repository.find_by_source_entity(
                    tenant_id=self.tenant_id,
                    entity_type=curr_type,
                    entity_id=curr_id,
                )
            except Exception as e:
                logger.warning(f"Error finding pathway relationships: {e}")
                continue

            for rel in relationships:
                target_key = (rel.target_entity_type, rel.target_entity_id)

                if target_key in visited:
                    continue

                visited.add(target_key)

                # Create step
                step = PathwayStep(
                    from_entity_id=curr_id,
                    from_entity_type=curr_type,
                    from_entity_name=await self._get_entity_name(curr_type, curr_id),
                    to_entity_id=rel.target_entity_id,
                    to_entity_type=rel.target_entity_type,
                    to_entity_name=await self._get_entity_name(
                        rel.target_entity_type, rel.target_entity_id
                    ),
                    relationship_type=rel.relationship_type,
                    strength=float(rel.confidence_score),
                )

                new_path = path + [step]
                new_strength = strength * float(rel.confidence_score)

                # Check if we found the target
                if (
                    rel.target_entity_type == target_entity_type
                    and rel.target_entity_id == target_entity_id
                ):
                    return PathwayResult(
                        source_entity_id=source_entity_id,
                        target_entity_id=target_entity_id,
                        path_length=len(new_path),
                        combined_strength=new_strength,
                        steps=new_path,
                        path_found=True,
                    )

                # Queue for further exploration
                queue.append(
                    (
                        rel.target_entity_type,
                        rel.target_entity_id,
                        new_path,
                        new_strength,
                    )
                )

        # No path found
        return PathwayResult(
            source_entity_id=source_entity_id,
            target_entity_id=target_entity_id,
            path_length=0,
            combined_strength=0.0,
            steps=[],
            path_found=False,
        )

    async def _get_entity_name(self, entity_type: str, entity_id: int) -> str:
        """Get display name for an entity.

        Args:
            entity_type: Type of entity
            entity_id: ID of entity

        Returns:
            Display name or placeholder if resolver not available
        """
        if self.entity_name_resolver:
            try:
                name: str | None = await self.entity_name_resolver.get_name(
                    entity_type, entity_id
                )
                if name:
                    return name
            except Exception:
                pass

        return f"{entity_type}_{entity_id}"


# ============================================================================
# SEARCH INTEGRATION SERVICE
# ============================================================================


class RelationshipSearchIntegration:
    """Integrates relationship context into search operations.

    Provides methods to enhance searches with relationship data,
    calculate boost factors, and rerank results.

    Attributes:
        context_provider: Provider for relationship context
    """

    def __init__(
        self,
        context_provider: RelationshipContextProvider,
    ) -> None:
        """Initialize the search integration.

        Args:
            context_provider: RelationshipContextProvider instance
        """
        self.context_provider = context_provider

    async def enhance_search_context(
        self,
        query: str,
        context_entity_type: str | None = None,
        context_entity_id: int | None = None,
        include_expansion: bool = True,
    ) -> SearchContextEnhancement:
        """Enhance search context with relationship data.

        Gathers related entities and optionally expands the query
        based on relationship context.

        Args:
            query: Original search query
            context_entity_type: Optional entity type for context
            context_entity_id: Optional entity ID for context
            include_expansion: Whether to include query expansion

        Returns:
            SearchContextEnhancement with all enhancement data
        """
        related: list[RelatedEntity] = []
        expansion: QueryExpansion | None = None
        boost_factors: dict[str, float] = {}

        if context_entity_type and context_entity_id:
            # Get related entities
            related = await self.context_provider.get_related_entities(
                entity_type=context_entity_type,
                entity_id=context_entity_id,
                max_depth=2,
            )

            # Expand query if requested
            if include_expansion:
                expansion = await self.context_provider.expand_query(
                    query=query,
                    context_entity_type=context_entity_type,
                    context_entity_id=context_entity_id,
                )

            # Calculate boost factors for related entities
            for r in related:
                key = f"{r.entity_type}_{r.entity_id}"
                boost_factors[key] = self._calculate_boost(r)

        return SearchContextEnhancement(
            original_query=query,
            related_entities=related,
            query_expansion=expansion,
            relationship_filters={
                "include_participants": [r.entity_id for r in related]
            },
            boost_factors=boost_factors,
        )

    def calculate_relationship_boost(
        self,
        entity_id: int,
        entity_type: str,
        related_entities: list[RelatedEntity],
    ) -> float:
        """Calculate boost factor for an entity based on relationships.

        Args:
            entity_id: ID of entity to boost
            entity_type: Type of entity
            related_entities: List of related entities for context

        Returns:
            Boost factor (1.0 = no boost, >1.0 = boosted)
        """
        for related in related_entities:
            if related.entity_id == entity_id and related.entity_type == entity_type:
                return self._calculate_boost(related)

        return 1.0  # No boost for unrelated entities

    def _calculate_boost(self, related: RelatedEntity) -> float:
        """Calculate boost factor from a related entity.

        Args:
            related: Related entity information

        Returns:
            Boost factor based on relationship strength and path length
        """
        # Base boost from relationship strength
        base_boost = 1.0 + (related.relationship_strength * 0.5)

        # Decay by path length
        path_decay = 1.0 / related.path_length

        return base_boost * path_decay

    async def filter_by_relationship(
        self,
        results: list[dict[str, Any]],
        context_entity_type: str,
        context_entity_id: int,
        include_unrelated: bool = True,
    ) -> list[dict[str, Any]]:
        """Filter search results by relationship to context entity.

        Args:
            results: Search results to filter
            context_entity_type: Type of context entity
            context_entity_id: ID of context entity
            include_unrelated: Whether to include unrelated results

        Returns:
            Filtered results
        """
        related = await self.context_provider.get_related_entities(
            entity_type=context_entity_type,
            entity_id=context_entity_id,
        )

        related_ids = {(r.entity_type, r.entity_id) for r in related}

        if include_unrelated:
            return results

        # Filter to only related entities
        return [
            r
            for r in results
            if (r.get("entity_type"), r.get("entity_id")) in related_ids
        ]

    async def rerank_by_relationship(
        self,
        results: list[dict[str, Any]],
        context_entity_type: str,
        context_entity_id: int,
        relationship_weight: float = 0.3,
    ) -> list[dict[str, Any]]:
        """Rerank search results using relationship context.

        Combines original scores with relationship boost factors
        to rerank results.

        Args:
            results: Search results to rerank
            context_entity_type: Type of context entity
            context_entity_id: ID of context entity
            relationship_weight: Weight for relationship boost (0.0-1.0)

        Returns:
            Reranked results
        """
        related = await self.context_provider.get_related_entities(
            entity_type=context_entity_type,
            entity_id=context_entity_id,
        )

        # Calculate boosted scores
        reranked = []
        for result in results:
            original_score = result.get("score", 0.0)

            boost = self.calculate_relationship_boost(
                entity_id=result.get("entity_id", 0),
                entity_type=result.get("entity_type", ""),
                related_entities=related,
            )

            # Combine scores
            # Original score contributes (1 - weight), relationship boost contributes weight
            boosted_score = (
                original_score * (1 - relationship_weight)
                + (original_score * boost) * relationship_weight
            )

            reranked.append({**result, "score": boosted_score, "relationship_boost": boost})

        # Sort by new score
        reranked.sort(key=lambda x: x["score"], reverse=True)

        return reranked


# ============================================================================
# EXPORTS
# ============================================================================


__all__ = [
    # Data models
    "PathwayResult",
    "PathwayStep",
    "QueryExpansion",
    "RelatedEntity",
    "SearchContextEnhancement",
    # Services
    "RelationshipContextProvider",
    "RelationshipSearchIntegration",
]

"""Pydantic domain models for relationship discovery and management.

This module defines the core domain models for relationships between entities
in the Penfold system. These models support automatic discovery, user validation,
lifecycle management, and multi-dimensional analysis.

Design Philosophy:
- Immutable content + Flexible relationship layer
- Relationships evolve dynamically with different scopes, strengths, and validation states
"""

from __future__ import annotations

from datetime import datetime, timezone
from enum import Enum
from typing import Any
from uuid import UUID, uuid4

from pydantic import BaseModel, Field


class EntityType(str, Enum):
    """Types of entities that can participate in relationships.

    Entities are the nodes in the relationship graph. Each entity type
    represents a distinct category of information that can be connected
    through relationships.
    """

    PERSON = "person"
    PROJECT = "project"
    TOPIC = "topic"
    # Note: Additional types (organization, document, meeting, email) can be
    # added in a future migration when needed


class RelationshipType(str, Enum):
    """Classification of relationship types between entities.

    Relationships describe how entities are connected. Different types
    carry different semantic meanings and affect how the relationship
    is used in queries and analysis.
    """

    # Person-to-person relationships
    COLLABORATES_WITH = "collaborates_with"
    REPORTS_TO = "reports_to"
    MANAGES = "manages"
    COMMUNICATES_WITH = "communicates_with"

    # Person-to-project relationships
    WORKS_ON = "works_on"
    LEADS = "leads"
    CONTRIBUTES_TO = "contributes_to"
    STAKEHOLDER_OF = "stakeholder_of"

    # Project-to-project relationships
    DEPENDS_ON = "depends_on"
    BLOCKS = "blocks"
    RELATED_TO = "related_to"
    SUPERSEDES = "supersedes"

    # Topic relationships
    DISCUSSES = "discusses"
    EXPERT_IN = "expert_in"
    INTERESTED_IN = "interested_in"

    # Generic relationships
    ASSOCIATED_WITH = "associated_with"
    REFERENCED_IN = "referenced_in"


class LifecycleState(str, Enum):
    """Lifecycle states for relationships.

    Relationships evolve over time. This state tracks where the relationship
    is in its lifecycle, from discovery through validation to archival.
    """

    # Discovery states
    EXPERIMENTAL = "experimental"  # AI suggested, <50% confidence
    TENTATIVE = "tentative"  # Pattern observed, 50-69% confidence
    PROBABLE = "probable"  # Domain logic validated, 70-89% confidence
    CONFIRMED = "confirmed"  # User verified, 90-100% confidence

    # Lifecycle states
    ACTIVE = "active"  # Currently relevant relationship
    HISTORICAL = "historical"  # No longer active but preserved
    ARCHIVED = "archived"  # Archived due to inactivity (2-year default)

    # Special states
    PENDING_VALIDATION = "pending_validation"  # Flagged for user review
    CONFLICTED = "conflicted"  # Conflicting relationships detected


class RelationshipScope(str, Enum):
    """Scope of relationship applicability.

    Relationships can apply globally or be scoped to specific contexts.
    This affects when and how the relationship is used in queries.
    """

    GLOBAL = "global"  # Permanent business knowledge
    PROJECT = "project"  # Exists only within project context
    TEMPORAL = "temporal"  # Time-bound relationship
    TENTATIVE = "tentative"  # Unvalidated hypothesis


class FeedbackType(str, Enum):
    """Types of user feedback on relationships.

    User feedback is essential for relationship quality. These types
    classify the nature of feedback provided.
    """

    CONFIRM = "confirm"  # User confirms relationship is accurate
    REJECT = "reject"  # User rejects relationship as incorrect
    MODIFY = "modify"  # User modifies relationship details
    STRENGTHEN = "strengthen"  # User indicates relationship is stronger
    WEAKEN = "weaken"  # User indicates relationship is weaker
    MERGE = "merge"  # User merges with another relationship
    SPLIT = "split"  # User splits into multiple relationships


class ConflictResolution(str, Enum):
    """Strategies for resolving relationship conflicts.

    When conflicting relationships are discovered, they must be resolved
    using one of these strategies.
    """

    AUTO_RESOLVED = "auto_resolved"  # Confidence gap > 30%, auto-resolved
    USER_VALIDATED = "user_validated"  # User chose winning relationship
    MERGED = "merged"  # Relationships were merged
    COEXIST = "coexist"  # Both relationships valid in different contexts


class EntityReference(BaseModel):
    """Reference to an entity participating in a relationship.

    This is a lightweight reference that identifies an entity
    without requiring the full entity data.
    """

    entity_id: int = Field(description="Unique identifier of the entity")
    entity_type: EntityType = Field(description="Type of the entity")
    display_name: str = Field(description="Human-readable name for display")
    external_ids: dict[str, str] = Field(
        default_factory=dict,
        description="External identifiers (email, slack_id, etc.)",
    )


class RelationshipEvidence(BaseModel):
    """Evidence supporting a relationship's existence and confidence.

    Evidence is collected from content analysis and user actions to
    justify the relationship's confidence score.
    """

    evidence_id: UUID = Field(default_factory=uuid4, description="Unique evidence ID")
    source_type: str = Field(description="Type of source (email, meeting, document)")
    source_id: int = Field(description="ID of the source content")
    extracted_at: datetime = Field(
        default_factory=lambda: datetime.now(timezone.utc),
        description="When this evidence was extracted",
    )
    confidence_contribution: float = Field(
        ge=0.0,
        le=1.0,
        description="How much this evidence contributes to confidence",
    )
    context_snippet: str = Field(
        default="",
        description="Text snippet providing context for this evidence",
    )
    metadata: dict[str, Any] = Field(
        default_factory=dict,
        description="Additional evidence metadata",
    )


class RelationshipCreate(BaseModel):
    """Request model for creating a new relationship.

    Used when creating relationships through the API or internal
    discovery processes.
    """

    source_entity: EntityReference = Field(description="Source entity of relationship")
    target_entity: EntityReference = Field(description="Target entity of relationship")
    relationship_type: RelationshipType = Field(description="Type of relationship")
    scope: RelationshipScope = Field(
        default=RelationshipScope.TENTATIVE,
        description="Scope of the relationship",
    )
    project_id: UUID | None = Field(
        default=None,
        description="Project context if scope is PROJECT",
    )
    initial_confidence: float = Field(
        default=0.5,
        ge=0.0,
        le=1.0,
        description="Initial confidence score",
    )
    evidence: list[RelationshipEvidence] = Field(
        default_factory=list,
        description="Initial evidence for this relationship",
    )
    metadata: dict[str, Any] = Field(
        default_factory=dict,
        description="Additional relationship metadata",
    )


class RelationshipResponse(BaseModel):
    """Response model for relationship data.

    Full relationship data returned from queries and API responses.
    """

    relationship_id: UUID = Field(description="Unique relationship identifier")
    source_entity: EntityReference = Field(description="Source entity of relationship")
    target_entity: EntityReference = Field(description="Target entity of relationship")
    relationship_type: RelationshipType = Field(description="Type of relationship")
    scope: RelationshipScope = Field(description="Scope of the relationship")
    project_id: UUID | None = Field(
        default=None,
        description="Project context if scope is PROJECT",
    )
    lifecycle_state: LifecycleState = Field(description="Current lifecycle state")
    confidence_score: float = Field(
        ge=0.0,
        le=1.0,
        description="Current confidence score (0.0-1.0)",
    )
    evidence_count: int = Field(description="Number of supporting evidence items")
    first_observed: datetime = Field(description="When relationship was first observed")
    last_observed: datetime = Field(description="When relationship was last observed")
    last_validated: datetime | None = Field(
        default=None,
        description="When relationship was last validated by user",
    )
    version: int = Field(default=1, description="Relationship version for tracking")
    metadata: dict[str, Any] = Field(
        default_factory=dict,
        description="Additional relationship metadata",
    )

    model_config = {"from_attributes": True}


class RelationshipUpdate(BaseModel):
    """Request model for updating a relationship.

    Used when modifying relationship properties through the API.
    """

    relationship_type: RelationshipType | None = Field(
        default=None,
        description="New relationship type",
    )
    scope: RelationshipScope | None = Field(
        default=None,
        description="New relationship scope",
    )
    project_id: UUID | None = Field(
        default=None,
        description="New project context",
    )
    lifecycle_state: LifecycleState | None = Field(
        default=None,
        description="New lifecycle state",
    )
    confidence_adjustment: float | None = Field(
        default=None,
        ge=-1.0,
        le=1.0,
        description="Adjustment to confidence score",
    )
    metadata: dict[str, Any] | None = Field(
        default=None,
        description="Updated metadata (merged with existing)",
    )


class RelationshipFeedback(BaseModel):
    """User feedback on a relationship.

    Captures user validation input to improve relationship quality
    and train discovery algorithms.
    """

    feedback_id: UUID = Field(default_factory=uuid4, description="Unique feedback ID")
    relationship_id: UUID = Field(description="Relationship being evaluated")
    user_id: str = Field(description="User providing feedback")
    feedback_type: FeedbackType = Field(description="Type of feedback")
    confidence_in_feedback: float = Field(
        default=1.0,
        ge=0.0,
        le=1.0,
        description="User's confidence in their feedback",
    )
    reasoning: str = Field(
        default="",
        description="User's reasoning for the feedback",
    )
    suggested_changes: dict[str, Any] = Field(
        default_factory=dict,
        description="Suggested modifications to the relationship",
    )
    provided_at: datetime = Field(
        default_factory=lambda: datetime.now(timezone.utc),
        description="When feedback was provided",
    )


class RelationshipConflict(BaseModel):
    """Conflict between discovered relationships.

    When multiple incompatible relationships are discovered, this model
    tracks the conflict and resolution status.
    """

    conflict_id: UUID = Field(default_factory=uuid4, description="Unique conflict ID")
    relationship_ids: list[UUID] = Field(
        description="IDs of conflicting relationships",
    )
    conflict_type: str = Field(description="Nature of the conflict")
    confidence_gap: float = Field(
        description="Confidence gap between relationships",
    )
    detected_at: datetime = Field(
        default_factory=lambda: datetime.now(timezone.utc),
        description="When conflict was detected",
    )
    resolution: ConflictResolution | None = Field(
        default=None,
        description="How conflict was resolved",
    )
    resolved_at: datetime | None = Field(
        default=None,
        description="When conflict was resolved",
    )
    winning_relationship_id: UUID | None = Field(
        default=None,
        description="ID of relationship chosen if resolved",
    )
    resolution_reasoning: str = Field(
        default="",
        description="Explanation of resolution decision",
    )


class RelationshipVersion(BaseModel):
    """Historical version of a relationship.

    Tracks relationship changes over time to maintain historical context
    and enable relationship evolution analysis.
    """

    version_id: UUID = Field(default_factory=uuid4, description="Unique version ID")
    relationship_id: UUID = Field(description="ID of the versioned relationship")
    version_number: int = Field(description="Sequential version number")
    changed_at: datetime = Field(
        default_factory=lambda: datetime.now(timezone.utc),
        description="When this version was created",
    )
    changed_by: str | None = Field(
        default=None,
        description="User or system that made the change",
    )
    change_type: str = Field(description="Type of change (create, update, feedback)")
    previous_state: dict[str, Any] = Field(
        description="Relationship state before change",
    )
    new_state: dict[str, Any] = Field(
        description="Relationship state after change",
    )
    change_reasoning: str = Field(
        default="",
        description="Explanation of why change occurred",
    )


class RelationshipNetworkNode(BaseModel):
    """Node in a relationship network graph.

    Used for network analysis and visualization of relationship patterns.
    """

    entity: EntityReference = Field(description="The entity represented by this node")
    relationship_count: int = Field(
        description="Total relationships involving this entity",
    )
    centrality_score: float = Field(
        default=0.0,
        description="Network centrality measure",
    )
    is_hub: bool = Field(
        default=False,
        description="Whether this is a communication hub",
    )
    cluster_id: str | None = Field(
        default=None,
        description="Cluster this node belongs to",
    )


class RelationshipNetworkEdge(BaseModel):
    """Edge in a relationship network graph.

    Represents a connection between two nodes with aggregated
    relationship information.
    """

    source_entity_id: UUID = Field(description="Source node entity ID")
    target_entity_id: UUID = Field(description="Target node entity ID")
    relationship_types: list[RelationshipType] = Field(
        description="Types of relationships in this edge",
    )
    combined_strength: float = Field(
        ge=0.0,
        le=1.0,
        description="Combined strength of all relationships",
    )
    interaction_count: int = Field(
        description="Number of interactions supporting this edge",
    )
    first_interaction: datetime = Field(description="First observed interaction")
    last_interaction: datetime = Field(description="Most recent interaction")


# Export all public models
__all__ = [
    # Enums
    "EntityType",
    "RelationshipType",
    "LifecycleState",
    "RelationshipScope",
    "FeedbackType",
    "ConflictResolution",
    # Core models
    "EntityReference",
    "RelationshipEvidence",
    "RelationshipCreate",
    "RelationshipResponse",
    "RelationshipUpdate",
    "RelationshipFeedback",
    "RelationshipConflict",
    "RelationshipVersion",
    # Network models
    "RelationshipNetworkNode",
    "RelationshipNetworkEdge",
]

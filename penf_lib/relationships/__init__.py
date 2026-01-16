"""Relationship Discovery and Management Framework.

This module provides automatic relationship discovery, user validation,
lifecycle management, and multi-dimensional analysis for relationships
between people, projects, topics, and other entities.

Design Philosophy:
- Immutable content + Flexible relationship layer
- Relationships evolve dynamically with different scopes, strengths, and validation states
- User feedback improves discovery accuracy over time

Key Features:
- Automatic relationship discovery from content processing
- Confidence-based relationship scoring with evidence tracking
- User validation and feedback integration
- Relationship lifecycle management (active, historical, archived)
- Network analysis for communication patterns and insights
"""

from .models import (
    # Enums
    ConflictResolution,
    # Core models
    EntityReference,
    EntityType,
    FeedbackType,
    LifecycleState,
    RelationshipConflict,
    RelationshipCreate,
    RelationshipEvidence,
    RelationshipFeedback,
    # Network models
    RelationshipNetworkEdge,
    RelationshipNetworkNode,
    RelationshipResponse,
    RelationshipScope,
    RelationshipType,
    RelationshipUpdate,
    RelationshipVersion,
)

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

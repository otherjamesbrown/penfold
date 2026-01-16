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

from .confidence import (
    ConfidenceCalculator,
    ConfidenceFactors,
    EvidenceItem,
    calculate_ai_confidence_weight,
    calculate_evidence_strength,
    calculate_interaction_boost,
    calculate_temporal_decay,
    combine_confidence_factors,
)
from .conflict_resolver import (
    ConflictResolutionResult,
    ConflictResolver,
    ResolutionStrategy,
)
from .discovery import (
    DiscoveredRelationship,
    DiscoveryResult,
    RelationshipDiscoveryService,
)
from .extractor import (
    ExtractedRelationship,
    ExtractionResult,
    RelationshipExtractor,
)
from .feedback import (
    FeedbackProcessor,
    FeedbackResult,
    LearnedPreference,
)
from .feedback import (
    FeedbackType as FeedbackTypeEnum,
)
from .lifecycle import (
    INACTIVITY_THRESHOLD_DAYS,
    RETENTION_YEARS,
    LifecycleManager,
    LifecycleTransition,
    MaintenanceResult,
    StateTransitionError,
    TransitionReason,
)
from .models import (
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
from .network import (
    BottleneckInfo,
    CentralityMetrics,
    CollaborationOpportunity,
    CommunityInfo,
    ExportFormat,
    NetworkAnalyzer,
    NetworkGraph,
    NetworkInsights,
)
from .search import (
    PathwayResult,
    PathwayStep,
    QueryExpansion,
    RelatedEntity,
    RelationshipContextProvider,
    RelationshipSearchIntegration,
    SearchContextEnhancement,
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
    # Extractor
    "ExtractedRelationship",
    "ExtractionResult",
    "RelationshipExtractor",
    # Confidence
    "ConfidenceCalculator",
    "ConfidenceFactors",
    "EvidenceItem",
    "calculate_ai_confidence_weight",
    "calculate_evidence_strength",
    "calculate_interaction_boost",
    "calculate_temporal_decay",
    "combine_confidence_factors",
    # Discovery
    "DiscoveredRelationship",
    "DiscoveryResult",
    "RelationshipDiscoveryService",
    # Network Analysis
    "NetworkAnalyzer",
    "NetworkGraph",
    "NetworkInsights",
    "CentralityMetrics",
    "CommunityInfo",
    "BottleneckInfo",
    "CollaborationOpportunity",
    "ExportFormat",
    # Search Integration
    "RelatedEntity",
    "PathwayStep",
    "PathwayResult",
    "QueryExpansion",
    "SearchContextEnhancement",
    "RelationshipContextProvider",
    "RelationshipSearchIntegration",
    # Conflict Resolution
    "ConflictResolver",
    "ConflictResolutionResult",
    "ResolutionStrategy",
    # Feedback Processing
    "FeedbackProcessor",
    "FeedbackResult",
    "FeedbackTypeEnum",
    "LearnedPreference",
    # Lifecycle Management
    "LifecycleManager",
    "LifecycleTransition",
    "MaintenanceResult",
    "StateTransitionError",
    "TransitionReason",
    "INACTIVITY_THRESHOLD_DAYS",
    "RETENTION_YEARS",
]

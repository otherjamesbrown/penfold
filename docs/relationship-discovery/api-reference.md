# Relationship Discovery API Reference

Programmatic access to relationship discovery and management.

## Core Classes

### RelationshipDiscoveryService

Main entry point for relationship discovery operations.

```python
from penf_lib.relationships import RelationshipDiscoveryService

async with async_session() as session:
    service = RelationshipDiscoveryService(
        session=session,
        repository=relationship_repo,
        extractor=ai_extractor,
        tenant_id=tenant_id,
    )

    # Process new content
    result = await service.process_content(
        content_id=123,
        content_type="email",
    )

    print(f"Discovered: {result.discovered_count}")
    print(f"Updated: {result.updated_count}")
```

### ConfidenceCalculator

Calculate and manage relationship confidence scores.

```python
from penf_lib.relationships import ConfidenceCalculator, EvidenceItem

calculator = ConfidenceCalculator()

# Full confidence calculation
score = calculator.calculate_relationship_confidence(
    ai_confidence=0.85,
    evidence_items=[
        EvidenceItem(
            evidence_type="email",
            source_id=1,
            observed_at=datetime.now(timezone.utc),
            strength_contribution=0.8,
        ),
    ],
    last_evidence_at=datetime.now(timezone.utc),
    interaction_count=5,
)

# Get detailed breakdown
breakdown = calculator.get_confidence_breakdown(
    ai_confidence=0.85,
    evidence_items=evidence_items,
    last_evidence_at=last_observed,
    interaction_count=5,
)
print(breakdown)
# {
#     'ai_confidence': 0.85,
#     'ai_confidence_weighted': 0.92,
#     'evidence_strength': 0.72,
#     'temporal_decay': 1.0,
#     'interaction_boost': 1.26,
#     'final_score': 0.89
# }
```

### NetworkAnalyzer

Analyze relationship networks for insights.

```python
from penf_lib.relationships import NetworkAnalyzer, ExportFormat

analyzer = NetworkAnalyzer()

# Build network from relationships
graph = analyzer.build_network(relationships)

# Calculate centrality metrics
centrality = analyzer.calculate_centrality(graph)

# Get comprehensive insights
insights = analyzer.get_network_insights(graph)
print(f"Hubs: {len(insights.hubs)}")
print(f"Communities: {len(insights.communities)}")
print(f"Bottlenecks: {len(insights.bottlenecks)}")

# Export for visualization
json_export = analyzer.export_graph(graph, ExportFormat.JSON)
dot_export = analyzer.export_graph(graph, ExportFormat.DOT)
```

### ConflictResolver

Handle relationship conflicts.

```python
from penf_lib.relationships import ConflictResolver, ResolutionStrategy

resolver = ConflictResolver(
    session=session,
    repository=conflict_repo,
    tenant_id=tenant_id,
)

# Auto-resolve high-confidence-gap conflicts
results = await resolver.process_auto_resolvable()

# Get conflicts needing user review
pending = await resolver.get_conflicts_for_review()

# User resolution
result = await resolver.user_resolve(
    conflict=conflict,
    winning_relationship_id=rel_id,
    user_email="user@example.com",
    reasoning="These are the same person",
    strategy=ResolutionStrategy.USER_RESOLVE,
)
```

### LifecycleManager

Manage relationship lifecycle transitions.

```python
from penf_lib.relationships import LifecycleManager, TransitionReason

manager = LifecycleManager(
    session=session,
    repository=lifecycle_repo,
    tenant_id=tenant_id,
)

# Transition state
transition = await manager.transition_state(
    relationship_id=rel_id,
    to_state="active",
    reason=TransitionReason.USER_CONFIRMED,
    triggered_by="user@example.com",
)

# Process inactive relationships
result = await manager.process_inactive_relationships(threshold_days=90)
print(f"Transitioned to historical: {result.transitions_applied}")

# Archive stale relationships
result = await manager.archive_stale_relationships(retention_years=2)
print(f"Archived: {result.archived_count}")
```

### FeedbackProcessor

Process user feedback on relationships.

```python
from penf_lib.relationships import FeedbackProcessor
from penf_lib.relationships.models import FeedbackType

processor = FeedbackProcessor(
    session=session,
    repository=feedback_repo,
    tenant_id=tenant_id,
)

# Submit feedback
result = await processor.submit_feedback(
    relationship_id=rel_id,
    feedback_type=FeedbackType.CONFIRM,
    user_email="user@example.com",
    reasoning="Accurate relationship",
)

# Get learned preferences
preferences = await processor.get_learned_preferences()
```

## Data Models

### RelationshipCreate

```python
from penf_lib.relationships.models import (
    RelationshipCreate,
    EntityReference,
    EntityType,
    RelationshipType,
)

new_relationship = RelationshipCreate(
    source_entity=EntityReference(
        entity_id=1,
        entity_type=EntityType.PERSON,
        display_name="Alice Smith",
    ),
    target_entity=EntityReference(
        entity_id=2,
        entity_type=EntityType.PERSON,
        display_name="Bob Jones",
    ),
    relationship_type=RelationshipType.COLLABORATES_WITH,
    confidence_score=0.85,
)
```

### Enums

```python
from penf_lib.relationships.models import (
    EntityType,
    RelationshipType,
    LifecycleState,
    FeedbackType,
)

# Entity types
EntityType.PERSON
EntityType.PROJECT
EntityType.TOPIC

# Relationship types
RelationshipType.COLLABORATES_WITH
RelationshipType.REPORTS_TO
RelationshipType.WORKS_ON
RelationshipType.EXPERT_IN

# Lifecycle states
LifecycleState.PENDING_VALIDATION
LifecycleState.ACTIVE
LifecycleState.HISTORICAL
LifecycleState.ARCHIVED

# Feedback types
FeedbackType.CONFIRM
FeedbackType.REJECT
FeedbackType.MODIFY
```

## Events

Relationship events for pub-sub integration.

```python
# Published events
RelationshipDiscoveredEvent(relationship_id, tenant_id, confidence)
RelationshipUpdatedEvent(relationship_id, tenant_id, changes)
RelationshipConflictDetectedEvent(conflict_id, tenant_id, relationships)
RelationshipValidatedEvent(relationship_id, tenant_id, feedback_type)
```

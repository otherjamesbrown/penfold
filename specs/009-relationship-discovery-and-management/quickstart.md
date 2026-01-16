# Quickstart: Relationship Discovery and Management

**Feature**: 009-relationship-discovery-and-management
**Date**: 2026-01-15

## Overview

The Relationship Discovery and Management system automatically discovers and tracks connections between people, projects, and topics from your email and meeting content.

## Prerequisites

- Penfold installed and configured
- Database migrations applied
- At least one tenant configured
- Gmail or meeting content ingested

## Quick Start

### 1. View Discovered Relationships

```bash
# List all active relationships for current tenant
penf relationships list

# Filter by entity
penf relationships list --person "John Smith"
penf relationships list --project "Alpha Project"

# Filter by confidence
penf relationships list --min-confidence 0.7

# Show pending validation
penf relationships pending
```

### 2. Validate Relationships

```bash
# Review a specific relationship
penf relationships show <relationship-id>

# Confirm a relationship
penf relationships confirm <relationship-id>

# Reject a relationship
penf relationships reject <relationship-id> --reason "Not a real connection"

# Modify relationship type
penf relationships modify <relationship-id> --type "colleague" --subtype "manager"
```

### 3. Resolve Conflicts

```bash
# List pending conflicts
penf relationships conflicts

# View conflict details
penf relationships conflict <conflict-id>

# Resolve manually
penf relationships resolve <conflict-id> --keep-primary
penf relationships resolve <conflict-id> --keep-secondary
penf relationships resolve <conflict-id> --merge
```

### 4. Trigger Discovery

```bash
# Run discovery on new content
penf relationships discover

# Run maintenance (archive stale relationships)
penf relationships maintain

# Recalculate confidence scores
penf relationships recalculate
```

## Python API

### Basic Usage

```python
from penf_lib.relationships import RelationshipDiscoveryService
from penf_lib.storage import get_async_session

async def example():
    async with get_async_session() as session:
        service = RelationshipDiscoveryService(session, tenant_id)

        # Get relationships for a person
        relationships = await service.get_relationships_for_entity(
            entity_type="person",
            entity_id=123,
            min_confidence=0.7
        )

        # Provide feedback
        await service.submit_feedback(
            relationship_id=456,
            feedback_type="confirm",
            user_email="user@example.com"
        )
```

### Discovery Pipeline

```python
from penf_lib.relationships import RelationshipExtractor
from penf_lib.ai_coordination import ModelCoordinator

async def discover_relationships(content: str, source_id: int):
    # Extract relationships using AI
    extractor = RelationshipExtractor(coordinator)
    relationships = await extractor.extract(content, source_id)

    # Each relationship has:
    # - source_entity (type, id)
    # - target_entity (type, id)
    # - relationship_type
    # - confidence_score
    # - evidence

    return relationships
```

### Conflict Resolution

```python
from penf_lib.relationships import ConflictResolver

resolver = ConflictResolver(session, tenant_id)

# Check if auto-resolvable (>30% confidence gap)
if conflict.auto_resolvable:
    await resolver.auto_resolve(conflict.id)
else:
    # Requires user decision
    await resolver.resolve_with_user_input(
        conflict.id,
        keep_primary=True,
        user_email="user@example.com"
    )
```

## Event Integration

### Subscribe to Relationship Events

```python
from penf_lib.events import subscribe

@subscribe("relationship.discovered")
async def on_relationship_discovered(event):
    relationship = event.payload
    print(f"New relationship: {relationship['source']} -> {relationship['target']}")

@subscribe("relationship.conflict")
async def on_conflict(event):
    conflict = event.payload
    if not conflict['auto_resolvable']:
        # Queue for user review
        await queue_for_review(conflict)
```

## Configuration

### Environment Variables

```bash
# Confidence thresholds
RELATIONSHIP_HIGH_CONFIDENCE=0.7      # Auto-activate threshold
RELATIONSHIP_CONFLICT_GAP=0.3         # Auto-resolve gap threshold

# Retention
RELATIONSHIP_RETENTION_DAYS=730       # 2 years default

# Performance
RELATIONSHIP_BATCH_SIZE=100           # Batch processing size
RELATIONSHIP_PROCESSING_TIMEOUT=60    # Seconds per item
```

### Tenant Settings

```python
# In tenant settings JSONB
{
    "relationships": {
        "auto_discovery": true,
        "confidence_threshold": 0.7,
        "retention_days": 730,
        "notify_on_conflict": true
    }
}
```

## Testing

### Run Tests

```bash
# Unit tests
pytest tests/unit/relationships/ -v

# Integration tests
pytest tests/integration/test_relationship_workflow.py -v

# Contract tests
pytest tests/contract/test_relationship_events.py -v
```

### Test Fixtures

```python
import pytest
from penf_lib.relationships.models import RelationshipCreate

@pytest.fixture
def sample_relationship():
    return RelationshipCreate(
        source_entity_type="person",
        source_entity_id=1,
        target_entity_type="person",
        target_entity_id=2,
        relationship_type="person_to_person",
        relationship_subtype="colleague",
        ai_confidence=Decimal("0.85")
    )
```

## Troubleshooting

### Common Issues

**No relationships discovered**
- Check that content has been ingested
- Verify AI coordination service is running
- Check logs for extraction errors

**Low confidence scores**
- Review evidence quality
- Consider providing feedback to improve learning
- Check entity resolution accuracy

**Conflicts not auto-resolving**
- Verify confidence gap exceeds 30%
- Check conflict resolution status
- Review resolution_details in conflict record

### Debug Logging

```bash
# Enable debug logging
export PENF_LOG_LEVEL=DEBUG
penf relationships discover --verbose
```

## Next Steps

1. Configure discovery thresholds for your use case
2. Set up daily review workflow for pending validations
3. Monitor relationship quality metrics
4. Integrate with search for relationship-enhanced queries

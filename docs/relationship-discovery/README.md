# Relationship Discovery and Management

Automatic discovery, validation, and analysis of relationships between people, projects, and topics from email and meeting content.

## Overview

The relationship discovery system automatically identifies connections between entities in your information ecosystem:

- **People-to-People**: Collaborations, reporting structures, communication patterns
- **People-to-Projects**: Work assignments, leadership, stakeholder relationships
- **Project-to-Project**: Dependencies, blockers, related work
- **Topic Relationships**: Expertise, interests, discussion patterns

## Key Features

### Automatic Discovery
- AI-powered extraction from email and meeting content
- Evidence-based confidence scoring
- Incremental updates as new content arrives

### User Validation
- Review low-confidence relationships in daily digest
- Confirm, reject, or modify discovered relationships
- Feedback improves future discovery accuracy

### Network Analysis
- Identify communication hubs and bottlenecks
- Detect communities and collaboration clusters
- Find potential collaboration opportunities

### Lifecycle Management
- Automatic aging of inactive relationships
- 2-year retention with configurable policies
- Reactivation on new evidence

## Quick Start

### View Relationships

```bash
# Analyze your relationship network
penf relationships analyze

# View relationships for a specific person
penf relationships list --entity "John Smith"

# Show network insights
penf relationships insights
```

### Validate Relationships

```bash
# Review pending relationships
penf relationships pending

# Validate a specific relationship
penf relationships validate <relationship-id>
```

### Export Data

```bash
# Export to JSON for visualization tools
penf relationships export --format json > network.json

# Export to DOT for Graphviz
penf relationships export --format dot > network.dot

# Export to CSV for spreadsheets
penf relationships export --format csv > network.csv
```

## Architecture

### Confidence Scoring

Relationships are scored using multiple factors:

| Factor | Weight | Description |
|--------|--------|-------------|
| AI Confidence | 30% | Extraction model confidence |
| Evidence Strength | 40% | Quality and quantity of evidence |
| Entity Resolution | 15% | Confidence in entity identification |
| Temporal Freshness | 15% | Recency of supporting evidence |

Additional boost applied for interaction frequency.

### Lifecycle States

```
pending → active → historical → archived
            ↑           |
            └───────────┘ (reactivation on new evidence)
```

- **Pending**: Newly discovered, awaiting validation
- **Active**: Confirmed and currently relevant
- **Historical**: No recent activity (90+ days)
- **Archived**: Past retention period (2+ years)

### Conflict Resolution

When conflicting relationships are discovered:

- **Auto-resolve**: Confidence gap ≥30% → higher confidence wins
- **User escalation**: Close confidence → presented for review
- **Coexistence**: Both valid in different contexts

## Configuration

### Environment Variables

```bash
# Confidence thresholds
RELATIONSHIP_MIN_CONFIDENCE=0.40    # Minimum for active state
RELATIONSHIP_HIGH_CONFIDENCE=0.70   # Auto-approve threshold

# Lifecycle settings
RELATIONSHIP_INACTIVITY_DAYS=90     # Days before historical
RELATIONSHIP_RETENTION_YEARS=2      # Years before archival

# Decay parameters
RELATIONSHIP_DECAY_HALF_LIFE=180    # Days for confidence to halve
RELATIONSHIP_DECAY_MINIMUM=0.20     # Floor for decayed confidence
```

## See Also

- [API Reference](./api-reference.md) - Programmatic access
- [Data Model](../../specs/009-relationship-discovery-and-management/data-model.md) - Entity schemas
- [Architecture Patterns](../../context/ARCHITECTURE.md#relationship-discovery-patterns) - Implementation details

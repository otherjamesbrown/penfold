# Relationship Discovery Patterns

> **Note**: Code examples are from the original Python implementation for reference. Go implementations use similar algorithms.

## 23. Multi-Factor Confidence Scoring

**Pattern**: Combine multiple signals into a weighted confidence score for relationship validity

**Implementation Details**:
- AI extraction confidence weighted (30%)
- Evidence strength with type weighting (40%)
- Entity resolution confidence (15%)
- Temporal decay for stale evidence (15%)
- Interaction frequency boost (multiplier)

**Evidence Type Weights**:
| Type | Weight |
|------|--------|
| Meeting | 1.2 (direct participation) |
| Email | 1.0 (direct communication) |
| Mention | 0.7 (indirect reference) |
| Inference | 0.5 (AI-inferred) |
| User input | 1.5 (user validation - strongest) |

## 24. Temporal Decay with Minimum Floor

**Pattern**: Exponential decay for relationship confidence with minimum threshold

**Implementation Details**:
- Half-life based decay (180 days default)
- Minimum floor prevents complete decay (20%)
- New evidence resets decay
- Supports different decay rates per relationship type

## 25. Conflict Resolution with Auto-Resolution Threshold

**Pattern**: Automatic conflict resolution based on confidence gap, user escalation for close conflicts

**Implementation Details**:
- Auto-resolve when confidence gap >= 30%
- User escalation for conflicts within 30%
- Full audit trail for all resolutions
- Support for coexistence (both valid in different contexts)

**Resolution Strategies**:
| Strategy | When Used |
|----------|-----------|
| Auto-resolve | Confidence gap >= 30% |
| User-resolve | User chose winner |
| Merge | Combined into one |
| Coexist | Both valid |

## 26. Relationship Lifecycle State Machine

**Pattern**: Defined state transitions with validation and audit trail

**States**: `pending` → `active` → `historical` → `archived`

**Valid Transitions**:
| From | To |
|------|-----|
| pending | active, historical |
| active | historical, archived |
| historical | archived, active (reactivation) |
| archived | (terminal state) |

**Transition Reasons**:
- User confirmed
- Inactivity (90 days default)
- Retention expired (2 years default)
- New evidence
- Role change
- Project completed

## 27. Evidence-Based Relationship Discovery

**Pattern**: Content-driven relationship extraction with evidence tracking

**Implementation Details**:
- AI extracts relationships from email/meeting content
- Each extraction creates evidence record
- Evidence accumulates to strengthen relationships
- Source tracking enables provenance queries

## 28. Network Analysis with Graph Algorithms

**Pattern**: Use graph algorithms for centrality calculations and community detection

**Implementation Details**:
- Degree, betweenness, closeness, eigenvector centrality
- Label propagation for community detection
- Hub and bottleneck identification

**Go Implementation**: `cmd/penf/cmd/relationship.go`

---

## Relationship Discovery Performance

### Confidence Calculation
- **Scoring Time**: <10ms per relationship evaluation
- **Batch Processing**: 100+ relationships per second
- **Decay Calculation**: Constant time O(1)

### Discovery Performance
- **Content Processing**: <60 seconds per item
- **Evidence Accumulation**: Real-time updates
- **Conflict Detection**: <100ms per check

### Network Analysis
- **Centrality Calculation**: <1s for 10,000 nodes
- **Community Detection**: <5s for large networks
- **Graph Export**: <500ms for JSON/DOT/CSV formats

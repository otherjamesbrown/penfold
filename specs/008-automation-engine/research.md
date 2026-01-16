# Research: Automation Rules Engine

**Feature**: 008-automation-engine
**Date**: 2026-01-15
**Status**: Complete

## Research Tasks

### 1. Rule Engine Architecture Patterns

**Decision**: Event-driven rule evaluation with condition trees

**Rationale**:
- Condition trees allow flexible, composable rule logic without hardcoded patterns
- Event-driven evaluation integrates naturally with existing event processing framework
- Supports both immediate evaluation and batch processing scenarios

**Alternatives Considered**:
- **Drools-style RETE algorithm**: Too complex for initial scope, better for 1000+ rules
- **Simple pattern matching**: Insufficient for compound conditions
- **ML-based rule learning**: Deferred - requires training data we don't have yet

**Implementation Notes**:
- Use Python dataclasses for condition representation
- Leverage existing JSONB storage for flexible condition schemas
- Implement visitor pattern for condition evaluation

### 2. Confidence Threshold Management

**Decision**: Per-content-type thresholds with global default

**Rationale**:
- Different content types (email, meetings, documents) have varying AI accuracy levels
- Global default of 85% provides conservative starting point (Clarification #3)
- Per-type overrides enable optimization as system learns

**Alternatives Considered**:
- **Single global threshold**: Too inflexible for varied content
- **Per-rule thresholds**: Adds complexity without clear benefit initially
- **Dynamic thresholds**: Deferred to progressive automation phase

**Implementation Notes**:
```python
class ConfidenceThreshold:
    global_default: float = 0.85
    content_type_overrides: Dict[str, float]  # e.g., {"email": 0.80, "meeting": 0.90}
    user_id: str
```

### 3. Rule Versioning Strategy

**Decision**: Immutable version records with active version pointer

**Rationale**:
- Full history required per Clarification #4
- Immutable records simplify audit trail
- Active version pointer enables instant rollback

**Alternatives Considered**:
- **Git-style branching**: Over-engineered for rule versioning
- **Copy-on-write**: Similar approach, less explicit versioning
- **Event sourcing**: Adds significant complexity

**Implementation Notes**:
```sql
CREATE TABLE automation_rule_versions (
    id BIGINT PRIMARY KEY,
    rule_id UUID NOT NULL,
    version_number INT NOT NULL,
    is_active BOOLEAN DEFAULT FALSE,
    conditions JSONB NOT NULL,
    actions JSONB NOT NULL,
    created_at TIMESTAMP,
    created_by VARCHAR(100),
    change_description TEXT
);
```

### 4. Conflict Resolution Algorithm

**Decision**: Confidence-weighted scoring with priority tiebreaker

**Rationale**:
- Historical accuracy naturally promotes effective rules (Clarification #2)
- Self-optimizing reduces manual maintenance burden
- User-assigned priorities provide override capability

**Algorithm**:
```python
def resolve_conflict(matching_rules: List[Rule], content: Content) -> Rule:
    scored_rules = []
    for rule in matching_rules:
        score = (
            rule.historical_accuracy * 0.6 +  # Primary factor
            rule.confidence_score * 0.3 +      # Current match confidence
            (1.0 - rule.priority / 10) * 0.1   # Priority tiebreaker
        )
        scored_rules.append((score, rule))
    return max(scored_rules, key=lambda x: x[0])[1]
```

**Alternatives Considered**:
- **First match wins**: Non-deterministic, hard to reason about
- **Most specific wins**: Difficult to define "specificity"
- **User prompt each time**: Defeats automation purpose

### 5. Pattern Detection Approach

**Decision**: Sliding window with frequency analysis

**Rationale**:
- Detects recurring categorization patterns over time
- Window size balances responsiveness vs. false positives
- Integrates with user feedback loop

**Algorithm**:
```python
@dataclass
class PatternCandidate:
    condition_signature: str  # Hash of conditions
    occurrence_count: int
    first_seen: datetime
    last_seen: datetime
    user_decisions: List[str]  # What user did each time

def detect_patterns(decisions: List[Decision], window_days: int = 14) -> List[PatternCandidate]:
    """Find repeated categorization patterns in recent decisions."""
    # Group by condition signature
    # Filter by minimum occurrence threshold (e.g., 3+)
    # Return candidates for rule suggestion
```

**Alternatives Considered**:
- **ML clustering**: Requires labeled data, too complex initially
- **Exact match only**: Misses variations, too rigid
- **Real-time streaming**: Unnecessary for daily pattern analysis

### 6. Retry and Recovery Strategy

**Decision**: Exponential backoff with dead letter queue

**Rationale**:
- Per Clarification #1: 3 attempts with exponential backoff
- Failed items queue for manual review
- Aligns with existing event processing patterns

**Implementation**:
```python
RETRY_DELAYS = [1, 4, 16]  # Seconds: 1s, 4s, 16s

async def process_with_retry(content_id: str, rule: Rule) -> ProcessingResult:
    for attempt, delay in enumerate(RETRY_DELAYS):
        try:
            return await apply_rule(content_id, rule)
        except TransientError:
            if attempt < len(RETRY_DELAYS) - 1:
                await asyncio.sleep(delay)
            else:
                await queue_for_review(content_id, rule, "max_retries_exceeded")
                raise
```

### 7. Performance Optimization

**Decision**: Rule caching with invalidation on update

**Rationale**:
- Rules change infrequently, reads are frequent
- Cache enables <500ms processing target (SC-009)
- Invalidation on update prevents stale data

**Implementation Notes**:
- Use Redis for distributed caching
- Cache key: `automation:rules:{user_id}:{content_type}`
- TTL: 5 minutes with event-driven invalidation
- Precompute condition indexes for fast matching

### 8. Integration with AI Coordination

**Decision**: Subscribe to ProcessingResult events

**Rationale**:
- AI coordination already produces confidence scores
- Event subscription enables loose coupling
- Allows automation to process results asynchronously

**Integration Point**:
```python
# In automation engine
@event_handler("ai.processing.completed")
async def on_processing_complete(event: ProcessingEvent):
    result = event.payload
    if result.confidence_score >= get_threshold(result.content_type):
        await apply_automation(result)
    else:
        await queue_for_review(result)
```

## Technology Decisions Summary

| Component | Technology | Rationale |
|-----------|------------|-----------|
| Rule storage | PostgreSQL + JSONB | Flexible conditions, existing infrastructure |
| Rule caching | Redis | Fast reads, existing infrastructure |
| Condition evaluation | Python dataclasses + visitor | Type-safe, testable |
| Event handling | Redis pub/sub | Existing event framework |
| Version history | Immutable records | Simple audit trail |
| Performance | Index + cache | Meet <500ms target |

## Open Questions (Resolved)

All NEEDS CLARIFICATION items have been resolved through spec clarifications:

1. Failure recovery -> Exponential backoff (Clarification #1)
2. Conflict resolution -> Confidence-based (Clarification #2)
3. Default thresholds -> 85% (Clarification #3)
4. Version history -> Full history with rollback (Clarification #4)
5. Multi-user -> User-scoped rules (Clarification #5)

## References

- [Event Processing Spec](../002-event-processing/spec.md) - Event framework patterns
- [AI Coordination Spec](../003-ai-coordination/spec.md) - Confidence scoring integration
- [Daily Review Spec](../006-daily-review/spec.md) - User feedback workflow
- Martin Fowler's "Rules Engine" pattern catalog
- Drools documentation for RETE algorithm concepts (deferred)

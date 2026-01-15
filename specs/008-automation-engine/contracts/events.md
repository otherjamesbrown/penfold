# Automation Engine Events Contract

**Feature**: 008-automation-engine
**Date**: 2026-01-15

## Event Overview

The Automation Engine integrates with the existing event processing framework (002-event-processing) to:
1. Subscribe to AI processing completion events
2. Publish automation decision events
3. Trigger pattern detection on user feedback

## Subscribed Events

### `ai.processing.completed`

**Source**: AI Coordination (003-ai-coordination)
**Purpose**: Evaluate automation rules against processed content

```json
{
  "event_type": "ai.processing.completed",
  "event_id": "uuid",
  "tenant_id": "uuid",
  "timestamp": "2026-01-15T10:30:00Z",
  "payload": {
    "content_id": "string",
    "content_type": "email|meeting|document",
    "source_id": "bigint",
    "processing_result": {
      "confidence_score": 0.92,
      "extracted_entities": {},
      "classifications": [],
      "model_used": "string"
    }
  }
}
```

**Handler**: `automation.engine.on_processing_completed`
- Check confidence against user threshold
- Evaluate matching automation rules
- Apply automation or queue for review

---

### `user.feedback.submitted`

**Source**: Daily Review (006-daily-review)
**Purpose**: Learn from user corrections for pattern detection

```json
{
  "event_type": "user.feedback.submitted",
  "event_id": "uuid",
  "tenant_id": "uuid",
  "timestamp": "2026-01-15T10:30:00Z",
  "payload": {
    "user_id": "string",
    "content_id": "string",
    "content_type": "string",
    "original_suggestion": {
      "action_type": "string",
      "parameters": {}
    },
    "user_decision": {
      "action_type": "string",
      "parameters": {}
    },
    "feedback_type": "approve|modify|reject"
  }
}
```

**Handler**: `automation.patterns.on_user_feedback`
- Record decision for pattern analysis
- Update rule effectiveness metrics
- Trigger pattern detection if threshold met

---

### `content.ingested`

**Source**: Gmail Integration (004-gmail-integration), Meeting Pipeline (005-meeting-pipeline)
**Purpose**: Trigger automation evaluation for new content

```json
{
  "event_type": "content.ingested",
  "event_id": "uuid",
  "tenant_id": "uuid",
  "timestamp": "2026-01-15T10:30:00Z",
  "payload": {
    "source_id": "bigint",
    "content_type": "email|meeting|document",
    "external_id": "string",
    "metadata": {}
  }
}
```

**Handler**: `automation.engine.on_content_ingested`
- Check for pre-AI automation rules (sender-based, keyword-based)
- Route content for AI processing if no immediate rule matches

---

## Published Events

### `automation.decision.made`

**Purpose**: Notify system of automation decisions for audit and learning

```json
{
  "event_type": "automation.decision.made",
  "event_id": "uuid",
  "tenant_id": "uuid",
  "timestamp": "2026-01-15T10:30:00Z",
  "payload": {
    "decision_id": "bigint",
    "user_id": "string",
    "content_id": "string",
    "content_type": "string",
    "decision_type": "auto_processed|queued_review|conflict_resolved",
    "rule_id": "uuid|null",
    "confidence_score": 0.92,
    "threshold_used": 0.85,
    "actions_taken": {
      "action_type": "categorize",
      "parameters": {}
    },
    "reasoning": "string"
  }
}
```

**Subscribers**:
- Daily Review: Update review queue
- Observability: Log for metrics
- Progressive: Update automation stats

---

### `automation.pattern.detected`

**Purpose**: Notify user of new automation pattern suggestion

```json
{
  "event_type": "automation.pattern.detected",
  "event_id": "uuid",
  "tenant_id": "uuid",
  "timestamp": "2026-01-15T10:30:00Z",
  "payload": {
    "pattern_id": "bigint",
    "user_id": "string",
    "pattern_signature": "string",
    "detected_conditions": {},
    "suggested_actions": {},
    "occurrence_count": 5,
    "confidence_score": 0.85
  }
}
```

**Subscribers**:
- Daily Review: Show pattern suggestion to user
- Notifications: Send pattern alert

---

### `automation.rule.applied`

**Purpose**: Detailed tracking of rule execution

```json
{
  "event_type": "automation.rule.applied",
  "event_id": "uuid",
  "tenant_id": "uuid",
  "timestamp": "2026-01-15T10:30:00Z",
  "payload": {
    "rule_id": "uuid",
    "rule_version": "integer",
    "content_id": "string",
    "matched_conditions": ["condition1", "condition2"],
    "actions_executed": {},
    "execution_time_ms": 45,
    "success": true
  }
}
```

**Subscribers**:
- RuleEffectiveness: Update metrics
- Observability: Performance tracking

---

### `automation.conflict.detected`

**Purpose**: Record rule conflicts for analysis

```json
{
  "event_type": "automation.conflict.detected",
  "event_id": "uuid",
  "tenant_id": "uuid",
  "timestamp": "2026-01-15T10:30:00Z",
  "payload": {
    "conflict_id": "bigint",
    "content_id": "string",
    "conflicting_rules": [
      {"rule_id": "uuid", "confidence": 0.92},
      {"rule_id": "uuid", "confidence": 0.88}
    ],
    "resolution_method": "confidence",
    "winning_rule_id": "uuid",
    "user_notification_required": false
  }
}
```

**Subscribers**:
- Daily Review: Show conflict if notification required
- Analytics: Track conflict patterns

---

### `automation.threshold.adjusted`

**Purpose**: Track progressive automation changes

```json
{
  "event_type": "automation.threshold.adjusted",
  "event_id": "uuid",
  "tenant_id": "uuid",
  "timestamp": "2026-01-15T10:30:00Z",
  "payload": {
    "user_id": "string",
    "content_type": "string",
    "previous_threshold": 0.85,
    "new_threshold": 0.82,
    "adjustment_reason": "accuracy_target_met",
    "metrics": {
      "accuracy_rate": 0.97,
      "automation_rate": 0.55,
      "sample_size": 150
    }
  }
}
```

**Subscribers**:
- Notifications: Inform user of change
- Analytics: Track progression

---

## Event Channels

| Channel | Purpose | Events |
|---------|---------|--------|
| `automation.decisions` | Audit trail | decision.made |
| `automation.rules` | Rule lifecycle | rule.applied, conflict.detected |
| `automation.learning` | Progressive automation | pattern.detected, threshold.adjusted |

## Subscription Configuration

```python
# Automation Engine Subscriptions
AUTOMATION_SUBSCRIPTIONS = [
    {
        "subscriber_id": "automation-engine",
        "subscription_name": "ai-results",
        "event_channels": ["ai.processing"],
        "event_types": ["ai.processing.completed"],
        "priority": 3,
        "batch_size": 10
    },
    {
        "subscriber_id": "automation-engine",
        "subscription_name": "user-feedback",
        "event_channels": ["user.feedback"],
        "event_types": ["user.feedback.submitted"],
        "priority": 5,
        "batch_size": 1
    },
    {
        "subscriber_id": "automation-engine",
        "subscription_name": "content-ingestion",
        "event_channels": ["content"],
        "event_types": ["content.ingested"],
        "priority": 4,
        "batch_size": 20
    }
]
```

## Error Handling

Per Clarification #1 (Failure Recovery):

```python
RETRY_CONFIG = {
    "max_retries": 3,
    "retry_delays": [1, 4, 16],  # seconds, exponential backoff
    "dead_letter_channel": "automation.failed",
    "on_max_retries": "queue_for_review"
}
```

Events that fail after max retries are:
1. Published to dead letter channel
2. Content queued for manual review
3. Failure recorded in AutomationDecision audit

## Integration Points

```
┌─────────────────────┐     ai.processing.completed    ┌─────────────────────┐
│  AI Coordination    ├───────────────────────────────▶│  Automation Engine  │
│  (003)              │                                │  (008)              │
└─────────────────────┘                                └──────────┬──────────┘
                                                                  │
┌─────────────────────┐     user.feedback.submitted              │
│  Daily Review       │◀────────────────────────────────────────+│
│  (006)              ├────────────────────────────────────────▶│
└─────────────────────┘     automation.decision.made             │
                                                                  │
┌─────────────────────┐     content.ingested                     │
│  Gmail/Meeting      ├─────────────────────────────────────────▶│
│  (004, 005)         │                                          │
└─────────────────────┘                                          ▼
                                                       ┌─────────────────────┐
                                                       │  Event Bus (Redis)  │
                                                       │  (002)              │
                                                       └─────────────────────┘
```

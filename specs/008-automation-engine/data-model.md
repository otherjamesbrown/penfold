# Data Model: Automation Rules Engine

**Feature**: 008-automation-engine
**Date**: 2026-01-15
**Status**: Complete

## Entity Overview

```
┌─────────────────────┐       ┌─────────────────────┐
│  AutomationRule     │       │  ConfidenceThreshold│
│  (user rule defs)   │       │  (processing gates) │
└─────────┬───────────┘       └─────────────────────┘
          │
          │ 1:N
          ▼
┌─────────────────────┐       ┌─────────────────────┐
│ AutomationRuleVersion│       │  AutomationPattern  │
│  (version history)  │       │  (detected patterns)│
└─────────────────────┘       └─────────────────────┘
          │
          │ 1:N
          ▼
┌─────────────────────┐       ┌─────────────────────┐
│  AutomationDecision │       │  RuleEffectiveness  │
│  (execution audit)  │       │  (performance stats)│
└─────────────────────┘       └─────────────────────┘
          │
          │ N:1
          ▼
┌─────────────────────┐
│  RuleConflict       │
│  (conflict records) │
└─────────────────────┘
```

## Entity Definitions

### AutomationRule

User-defined automation rule with conditions and actions.

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | UUID | PK | Unique rule identifier |
| tenant_id | UUID | FK, NOT NULL, INDEX | Tenant isolation |
| user_id | VARCHAR(254) | NOT NULL, INDEX | Rule owner (user-scoped) |
| name | VARCHAR(200) | NOT NULL | Human-readable rule name |
| description | TEXT | | Rule purpose description |
| is_enabled | BOOLEAN | NOT NULL, DEFAULT TRUE | Whether rule is active |
| priority | INTEGER | DEFAULT 5, CHECK 1-10 | Manual priority (1=highest) |
| conditions | JSONB | NOT NULL | Rule conditions (see schema below) |
| actions | JSONB | NOT NULL | Actions to execute (see schema below) |
| current_version_id | BIGINT | FK | Active version reference |
| created_at | TIMESTAMP | NOT NULL | Creation timestamp |
| updated_at | TIMESTAMP | NOT NULL | Last update timestamp |
| is_deleted | BOOLEAN | NOT NULL, DEFAULT FALSE | Soft delete flag |
| deleted_at | TIMESTAMP | | Deletion timestamp |

**Indexes**:
- `idx_rules_tenant_user`: (tenant_id, user_id, is_enabled)
- `idx_rules_conditions`: (conditions) using GIN
- `idx_rules_priority`: (priority, is_enabled)

**Conditions JSONB Schema**:
```json
{
  "operator": "AND|OR",
  "conditions": [
    {
      "field": "content_type|sender|subject|project|keywords|confidence",
      "operator": "equals|contains|greater_than|less_than|matches|in",
      "value": "string|number|array"
    }
  ]
}
```

**Actions JSONB Schema**:
```json
{
  "action_type": "categorize|tag|assign_project|archive|flag",
  "parameters": {
    "project_id": "uuid",
    "tags": ["string"],
    "priority": "string"
  }
}
```

---

### AutomationRuleVersion

Immutable version record for rule history and rollback.

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | BIGINT | PK | Version record identifier |
| rule_id | UUID | FK, NOT NULL, INDEX | Parent rule reference |
| version_number | INTEGER | NOT NULL | Sequential version number |
| is_active | BOOLEAN | NOT NULL, DEFAULT FALSE | Currently active version |
| conditions | JSONB | NOT NULL | Conditions at this version |
| actions | JSONB | NOT NULL | Actions at this version |
| change_description | TEXT | | Description of changes |
| created_at | TIMESTAMP | NOT NULL | Version creation time |
| created_by | VARCHAR(254) | NOT NULL | User who created version |

**Indexes**:
- `idx_versions_rule_active`: (rule_id, is_active)
- `idx_versions_rule_number`: (rule_id, version_number)

**Constraints**:
- UNIQUE (rule_id, version_number)
- Only one active version per rule (enforced via trigger)

---

### ConfidenceThreshold

Per-user, per-content-type automation thresholds.

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | BIGINT | PK | Threshold record identifier |
| tenant_id | UUID | FK, NOT NULL | Tenant isolation |
| user_id | VARCHAR(254) | NOT NULL | Threshold owner |
| content_type | VARCHAR(50) | NOT NULL | Content type (email, meeting, document, *) |
| threshold_value | DECIMAL(4,3) | NOT NULL, CHECK 0-1 | Confidence threshold (0.000-1.000) |
| is_enabled | BOOLEAN | NOT NULL, DEFAULT TRUE | Whether threshold is active |
| created_at | TIMESTAMP | NOT NULL | Creation timestamp |
| updated_at | TIMESTAMP | NOT NULL | Last update timestamp |

**Indexes**:
- `idx_thresholds_user_type`: (tenant_id, user_id, content_type)

**Constraints**:
- UNIQUE (tenant_id, user_id, content_type)
- DEFAULT threshold_value = 0.850

---

### AutomationDecision

Audit trail for all automated processing decisions.

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | BIGINT | PK | Decision record identifier |
| tenant_id | UUID | FK, NOT NULL | Tenant isolation |
| user_id | VARCHAR(254) | NOT NULL | Decision context user |
| content_id | VARCHAR(255) | NOT NULL, INDEX | Processed content reference |
| content_type | VARCHAR(50) | NOT NULL | Content type |
| rule_id | UUID | FK | Applied rule (null if threshold-based) |
| rule_version_id | BIGINT | FK | Specific version applied |
| decision_type | VARCHAR(50) | NOT NULL | auto_processed, queued_review, conflict_resolved |
| confidence_score | DECIMAL(4,3) | NOT NULL | AI confidence at decision time |
| threshold_used | DECIMAL(4,3) | NOT NULL | Threshold applied |
| actions_taken | JSONB | NOT NULL | Actions executed |
| reasoning | TEXT | | Decision explanation |
| processing_time_ms | INTEGER | | Time to process |
| retry_count | INTEGER | DEFAULT 0 | Number of retry attempts |
| error_details | JSONB | | Error info if failed |
| user_override | BOOLEAN | DEFAULT FALSE | Whether user overrode decision |
| user_feedback | JSONB | | User correction if provided |
| created_at | TIMESTAMP | NOT NULL | Decision timestamp |

**Indexes**:
- `idx_decisions_user_date`: (tenant_id, user_id, created_at)
- `idx_decisions_rule`: (rule_id, created_at)
- `idx_decisions_content`: (content_id, content_type)
- `idx_decisions_type`: (decision_type, created_at)

---

### RuleEffectiveness

Performance metrics for automation rules.

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | BIGINT | PK | Metrics record identifier |
| rule_id | UUID | FK, NOT NULL | Rule being measured |
| period_start | TIMESTAMP | NOT NULL | Metrics period start |
| period_end | TIMESTAMP | NOT NULL | Metrics period end |
| total_matches | INTEGER | NOT NULL, DEFAULT 0 | Times rule matched |
| total_applied | INTEGER | NOT NULL, DEFAULT 0 | Times rule was applied |
| correct_decisions | INTEGER | NOT NULL, DEFAULT 0 | Decisions without override |
| incorrect_decisions | INTEGER | NOT NULL, DEFAULT 0 | Decisions user overrode |
| accuracy_rate | DECIMAL(4,3) | | Calculated accuracy |
| avg_confidence | DECIMAL(4,3) | | Average confidence score |
| avg_processing_time_ms | INTEGER | | Average processing time |
| conflict_count | INTEGER | DEFAULT 0 | Times in conflict |
| conflict_win_count | INTEGER | DEFAULT 0 | Times won conflict |
| created_at | TIMESTAMP | NOT NULL | Record creation time |

**Indexes**:
- `idx_effectiveness_rule_period`: (rule_id, period_start, period_end)
- `idx_effectiveness_accuracy`: (accuracy_rate, total_applied)

**Constraints**:
- UNIQUE (rule_id, period_start, period_end)

---

### AutomationPattern

Detected user behavior patterns for rule suggestions.

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | BIGINT | PK | Pattern record identifier |
| tenant_id | UUID | FK, NOT NULL | Tenant isolation |
| user_id | VARCHAR(254) | NOT NULL | Pattern owner |
| pattern_signature | VARCHAR(64) | NOT NULL, INDEX | Hash of pattern conditions |
| detected_conditions | JSONB | NOT NULL | Conditions forming pattern |
| suggested_actions | JSONB | NOT NULL | Suggested automation actions |
| occurrence_count | INTEGER | NOT NULL | Times pattern observed |
| first_seen_at | TIMESTAMP | NOT NULL | First occurrence |
| last_seen_at | TIMESTAMP | NOT NULL | Most recent occurrence |
| confidence_score | DECIMAL(4,3) | NOT NULL | Pattern reliability score |
| status | VARCHAR(20) | NOT NULL, DEFAULT 'pending' | pending, accepted, rejected, expired |
| converted_rule_id | UUID | FK | Rule if pattern was accepted |
| created_at | TIMESTAMP | NOT NULL | Detection timestamp |
| updated_at | TIMESTAMP | NOT NULL | Last update timestamp |

**Indexes**:
- `idx_patterns_user_status`: (tenant_id, user_id, status)
- `idx_patterns_signature`: (pattern_signature)
- `idx_patterns_confidence`: (confidence_score, occurrence_count)

---

### RuleConflict

Records of rule conflicts for analysis and learning.

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | BIGINT | PK | Conflict record identifier |
| tenant_id | UUID | FK, NOT NULL | Tenant isolation |
| content_id | VARCHAR(255) | NOT NULL | Content causing conflict |
| conflicting_rule_ids | UUID[] | NOT NULL | Rules that matched |
| winning_rule_id | UUID | FK | Rule that was applied |
| resolution_method | VARCHAR(50) | NOT NULL | confidence, priority, user |
| resolution_reasoning | TEXT | | Why winner was chosen |
| confidence_scores | JSONB | NOT NULL | Scores per rule |
| user_notified | BOOLEAN | DEFAULT FALSE | Whether user was notified |
| user_resolution | UUID | FK | User's chosen rule if different |
| created_at | TIMESTAMP | NOT NULL | Conflict timestamp |

**Indexes**:
- `idx_conflicts_content`: (content_id, created_at)
- `idx_conflicts_rules`: (conflicting_rule_ids) using GIN
- `idx_conflicts_winner`: (winning_rule_id)

---

### ProgressiveSettings

User preferences for automation advancement.

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | BIGINT | PK | Settings record identifier |
| tenant_id | UUID | FK, NOT NULL | Tenant isolation |
| user_id | VARCHAR(254) | NOT NULL | Settings owner |
| aggressiveness_level | VARCHAR(20) | NOT NULL, DEFAULT 'moderate' | conservative, moderate, aggressive |
| auto_threshold_adjustment | BOOLEAN | DEFAULT TRUE | Allow system to adjust thresholds |
| min_learning_period_days | INTEGER | DEFAULT 30 | Days before auto-adjustment |
| target_automation_rate | DECIMAL(4,3) | DEFAULT 0.60 | Target % auto-processed |
| accuracy_floor | DECIMAL(4,3) | DEFAULT 0.95 | Minimum acceptable accuracy |
| pattern_suggestion_enabled | BOOLEAN | DEFAULT TRUE | Show pattern suggestions |
| notification_preferences | JSONB | DEFAULT '{}' | Notification settings |
| created_at | TIMESTAMP | NOT NULL | Creation timestamp |
| updated_at | TIMESTAMP | NOT NULL | Last update timestamp |

**Indexes**:
- `idx_settings_user`: (tenant_id, user_id) UNIQUE

---

## Relationships

```
Tenant (1) ──────────────────┬──── (N) AutomationRule
                             ├──── (N) ConfidenceThreshold
                             ├──── (N) AutomationDecision
                             ├──── (N) AutomationPattern
                             ├──── (N) RuleConflict
                             └──── (1) ProgressiveSettings

AutomationRule (1) ──────────┬──── (N) AutomationRuleVersion
                             ├──── (N) AutomationDecision
                             ├──── (N) RuleEffectiveness
                             └──── (N) RuleConflict

AutomationPattern (1) ───────┴──── (0-1) AutomationRule (converted)

Source/ProcessingResult ─────────── AutomationDecision (via content_id)
```

## Migration Strategy

1. Create tables in order: rules -> versions -> thresholds -> decisions -> patterns -> conflicts -> settings -> effectiveness
2. Add foreign key constraints after all tables exist
3. Create indexes for query optimization
4. Add trigger for single active version enforcement
5. Insert default thresholds for existing users

## Validation Rules

| Entity | Rule | Implementation |
|--------|------|----------------|
| AutomationRule | Conditions must be valid JSON schema | Check constraint + app validation |
| ConfidenceThreshold | Value must be 0.000-1.000 | Check constraint |
| AutomationDecision | Must reference valid content | App-level validation |
| RuleEffectiveness | accuracy_rate = correct / (correct + incorrect) | Computed column or trigger |
| AutomationPattern | occurrence_count >= 3 for suggestion | App-level filter |

## State Transitions

### AutomationRule States
```
┌──────────┐     enable      ┌──────────┐
│ disabled ├─────────────────▶│ enabled  │
└────┬─────┘◀────────────────┴────┬─────┘
     │         disable            │
     │                            │
     └──────────┬─────────────────┘
                │ soft_delete
                ▼
         ┌──────────┐
         │ deleted  │
         └──────────┘
```

### AutomationPattern States
```
┌─────────┐    accept    ┌──────────┐
│ pending ├──────────────▶│ accepted │
└────┬────┘              └──────────┘
     │
     ├─────reject────────▶┌──────────┐
     │                    │ rejected │
     │                    └──────────┘
     │
     └─────expire─────────▶┌─────────┐
                          │ expired │
                          └─────────┘
```

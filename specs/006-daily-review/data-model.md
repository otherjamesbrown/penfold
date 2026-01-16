# Data Model: Daily Review Workflow Interface

**Feature**: 006-daily-review
**Date**: 2026-01-15
**Status**: Complete

## Entity Overview

```
ReviewSession 1──* ReviewItem
ReviewItem *──1 Source
ReviewItem 1──* UserFeedback
UserFeedback *──1 LearningRule (optional)
ReviewSession 1──* BatchOperation
BatchOperation 1──* ReviewItem
```

## Entities

### ReviewSession

Represents a user's active review workflow session with state management and progress tracking.

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | BIGINT | PK, AUTO | Session identifier |
| session_uuid | UUID | UNIQUE, NOT NULL | External session reference |
| tenant_id | UUID | FK(tenants.id), NOT NULL | Tenant isolation |
| user_email | VARCHAR(254) | NOT NULL | User performing review |
| status | VARCHAR(20) | NOT NULL, DEFAULT 'active' | active, paused, completed, abandoned |
| started_at | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Session start time |
| last_activity_at | TIMESTAMPTZ | NOT NULL | Last interaction time |
| completed_at | TIMESTAMPTZ | NULL | Session completion time |
| expires_at | TIMESTAMPTZ | NOT NULL | Auto-expiration time (24h) |
| current_position | INTEGER | DEFAULT 0 | Current queue position |
| total_items | INTEGER | NOT NULL | Total items in queue at start |
| items_reviewed | INTEGER | DEFAULT 0 | Count of reviewed items |
| items_accepted | INTEGER | DEFAULT 0 | Count of accepted items |
| items_rejected | INTEGER | DEFAULT 0 | Count of rejected items |
| items_modified | INTEGER | DEFAULT 0 | Count of modified items |
| items_skipped | INTEGER | DEFAULT 0 | Count of skipped items |
| review_mode | VARCHAR(30) | NOT NULL, DEFAULT 'standard' | quick, standard, detailed |
| priority_mode | VARCHAR(30) | NOT NULL, DEFAULT 'mixed' | confidence, importance, recency, mixed |
| filter_criteria | JSONB | DEFAULT '{}' | Active filters for queue |
| session_metadata | JSONB | DEFAULT '{}' | Additional session state |
| created_at | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Record creation |
| updated_at | TIMESTAMPTZ | NOT NULL | Last update |

**Indexes**:
- `idx_review_session_tenant_user` ON (tenant_id, user_email, status)
- `idx_review_session_active` ON (status, expires_at) WHERE status = 'active'
- `idx_review_session_uuid` ON (session_uuid)

**Constraints**:
- CHECK: status IN ('active', 'paused', 'completed', 'abandoned')
- CHECK: review_mode IN ('quick', 'standard', 'detailed')
- CHECK: priority_mode IN ('confidence', 'importance', 'recency', 'mixed')
- CHECK: items_reviewed = items_accepted + items_rejected + items_modified + items_skipped

---

### ReviewItem

Individual item in the review queue linking AI processing results to review workflow state.

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | BIGINT | PK, AUTO | Item identifier |
| tenant_id | UUID | FK(tenants.id), NOT NULL | Tenant isolation |
| session_id | BIGINT | FK(review_sessions.id), NULL | Associated session (NULL if unqueued) |
| source_id | BIGINT | FK(sources.id), NOT NULL | Source content reference |
| processing_result_id | UUID | FK(processing_results.id), NULL | AI processing result |
| queue_position | INTEGER | NULL | Position in review queue |
| status | VARCHAR(20) | NOT NULL, DEFAULT 'pending' | pending, in_review, accepted, rejected, modified, skipped |
| content_type | VARCHAR(50) | NOT NULL | email, meeting, document |
| content_preview | TEXT | NOT NULL | Truncated content for display |
| ai_suggestion | JSONB | NOT NULL | Suggested categorization |
| ai_confidence | DECIMAL(4,3) | NOT NULL | AI confidence score 0.000-1.000 |
| ai_model | VARCHAR(100) | NOT NULL | Model that generated suggestion |
| business_importance | INTEGER | DEFAULT 5 | Importance score 1-10 |
| source_timestamp | TIMESTAMPTZ | NOT NULL | Original content timestamp |
| reviewed_at | TIMESTAMPTZ | NULL | When item was reviewed |
| review_duration_ms | INTEGER | NULL | Time spent on review |
| user_decision | VARCHAR(20) | NULL | accept, reject, modify, skip |
| user_correction | JSONB | NULL | User's correction if modified |
| batch_id | UUID | NULL | If part of batch operation |
| undo_eligible | BOOLEAN | DEFAULT TRUE | Can be undone |
| undo_deadline | TIMESTAMPTZ | NULL | Deadline for undo (5 min window) |
| created_at | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Record creation |
| updated_at | TIMESTAMPTZ | NOT NULL | Last update |

**Indexes**:
- `idx_review_item_session` ON (session_id, queue_position)
- `idx_review_item_status` ON (tenant_id, status, ai_confidence)
- `idx_review_item_source` ON (source_id)
- `idx_review_item_batch` ON (batch_id) WHERE batch_id IS NOT NULL
- `idx_review_item_undo` ON (undo_eligible, undo_deadline) WHERE undo_eligible = TRUE

**Constraints**:
- CHECK: status IN ('pending', 'in_review', 'accepted', 'rejected', 'modified', 'skipped')
- CHECK: content_type IN ('email', 'meeting', 'document')
- CHECK: ai_confidence BETWEEN 0.000 AND 1.000
- CHECK: business_importance BETWEEN 1 AND 10
- CHECK: user_decision IN ('accept', 'reject', 'modify', 'skip') OR user_decision IS NULL

---

### UserFeedback

Captures user validation decisions for learning system integration.

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | BIGINT | PK, AUTO | Feedback identifier |
| feedback_uuid | UUID | UNIQUE, NOT NULL | External feedback reference |
| tenant_id | UUID | FK(tenants.id), NOT NULL | Tenant isolation |
| review_item_id | BIGINT | FK(review_items.id), NOT NULL | Associated review item |
| session_id | BIGINT | FK(review_sessions.id), NOT NULL | Session context |
| decision_type | VARCHAR(20) | NOT NULL | accept, reject, modify |
| original_suggestion | JSONB | NOT NULL | AI's original suggestion |
| user_correction | JSONB | NULL | User's correction (for modify) |
| correction_reason | VARCHAR(100) | NULL | Reason code for correction |
| correction_notes | TEXT | NULL | Free-form explanation |
| confidence_feedback | VARCHAR(20) | NULL | too_high, accurate, too_low |
| time_spent_ms | INTEGER | NOT NULL | Time user spent on decision |
| was_batch_decision | BOOLEAN | DEFAULT FALSE | Part of batch operation |
| batch_id | UUID | NULL | Batch operation reference |
| learning_rule_id | BIGINT | FK(learning_rules.id), NULL | Generated learning rule |
| learning_signal_quality | DECIMAL(3,2) | NULL | Quality score 0.00-1.00 |
| created_at | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Record creation |

**Indexes**:
- `idx_user_feedback_session` ON (session_id, created_at)
- `idx_user_feedback_item` ON (review_item_id)
- `idx_user_feedback_learning` ON (tenant_id, decision_type, correction_reason)
- `idx_user_feedback_batch` ON (batch_id) WHERE batch_id IS NOT NULL

**Constraints**:
- CHECK: decision_type IN ('accept', 'reject', 'modify')
- CHECK: confidence_feedback IN ('too_high', 'accurate', 'too_low') OR confidence_feedback IS NULL
- CHECK: learning_signal_quality BETWEEN 0.00 AND 1.00 OR learning_signal_quality IS NULL

---

### LearningRule

Automated decision patterns derived from user feedback.

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | BIGINT | PK, AUTO | Rule identifier |
| rule_uuid | UUID | UNIQUE, NOT NULL | External rule reference |
| tenant_id | UUID | FK(tenants.id), NOT NULL | Tenant isolation |
| name | VARCHAR(200) | NOT NULL | Human-readable rule name |
| description | TEXT | NULL | Rule description |
| status | VARCHAR(20) | NOT NULL, DEFAULT 'active' | active, disabled, expired |
| rule_type | VARCHAR(50) | NOT NULL | sender_match, content_pattern, category_override |
| match_criteria | JSONB | NOT NULL | Conditions for rule match |
| action | JSONB | NOT NULL | Action to apply when matched |
| confidence_threshold | DECIMAL(4,3) | DEFAULT 0.800 | Min confidence to apply |
| priority | INTEGER | DEFAULT 100 | Lower = higher priority |
| derived_from_count | INTEGER | DEFAULT 0 | Number of feedback items that created rule |
| times_applied | INTEGER | DEFAULT 0 | Application count |
| times_overridden | INTEGER | DEFAULT 0 | User override count |
| effectiveness_score | DECIMAL(4,3) | NULL | Calculated effectiveness |
| last_applied_at | TIMESTAMPTZ | NULL | Last application time |
| expires_at | TIMESTAMPTZ | NULL | Optional expiration |
| created_by | VARCHAR(254) | NOT NULL | User who created rule |
| created_at | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Record creation |
| updated_at | TIMESTAMPTZ | NOT NULL | Last update |
| is_deleted | BOOLEAN | DEFAULT FALSE | Soft delete flag |
| deleted_at | TIMESTAMPTZ | NULL | Deletion timestamp |

**Indexes**:
- `idx_learning_rule_tenant_status` ON (tenant_id, status, priority)
- `idx_learning_rule_type` ON (rule_type, status)
- `idx_learning_rule_effectiveness` ON (effectiveness_score DESC) WHERE status = 'active'
- `idx_learning_rule_soft_delete` ON (is_deleted, deleted_at)

**Constraints**:
- CHECK: status IN ('active', 'disabled', 'expired')
- CHECK: rule_type IN ('sender_match', 'content_pattern', 'category_override', 'participant_mapping')
- CHECK: confidence_threshold BETWEEN 0.000 AND 1.000
- CHECK: priority BETWEEN 1 AND 1000
- CHECK: effectiveness_score BETWEEN 0.000 AND 1.000 OR effectiveness_score IS NULL

---

### BatchOperation

Groups related review items for bulk actions.

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | BIGINT | PK, AUTO | Batch identifier |
| batch_uuid | UUID | UNIQUE, NOT NULL | External batch reference |
| tenant_id | UUID | FK(tenants.id), NOT NULL | Tenant isolation |
| session_id | BIGINT | FK(review_sessions.id), NOT NULL | Parent session |
| batch_type | VARCHAR(50) | NOT NULL | Grouping type |
| group_criteria | JSONB | NOT NULL | How items were grouped |
| action_type | VARCHAR(20) | NOT NULL | accept_all, reject_all, apply_category |
| action_details | JSONB | NOT NULL | Action parameters |
| item_count | INTEGER | NOT NULL | Number of items in batch |
| status | VARCHAR(20) | NOT NULL, DEFAULT 'pending' | pending, confirmed, applied, undone |
| confirmed_at | TIMESTAMPTZ | NULL | User confirmation time |
| applied_at | TIMESTAMPTZ | NULL | Action application time |
| undone_at | TIMESTAMPTZ | NULL | Undo time if reversed |
| undo_eligible | BOOLEAN | DEFAULT TRUE | Can be undone |
| undo_deadline | TIMESTAMPTZ | NULL | Deadline for undo |
| created_at | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Record creation |
| updated_at | TIMESTAMPTZ | NOT NULL | Last update |

**Indexes**:
- `idx_batch_session` ON (session_id, created_at)
- `idx_batch_status` ON (tenant_id, status, created_at)
- `idx_batch_undo` ON (undo_eligible, undo_deadline) WHERE undo_eligible = TRUE

**Constraints**:
- CHECK: batch_type IN ('thread', 'sender', 'category', 'time_window', 'custom')
- CHECK: action_type IN ('accept_all', 'reject_all', 'apply_category', 'skip_all')
- CHECK: status IN ('pending', 'confirmed', 'applied', 'undone')
- CHECK: item_count > 0

---

### ReviewAnalytics (P3 - Deferred)

Aggregated metrics for review patterns and system learning progress.

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | BIGINT | PK, AUTO | Analytics identifier |
| tenant_id | UUID | FK(tenants.id), NOT NULL | Tenant isolation |
| period_start | DATE | NOT NULL | Analytics period start |
| period_end | DATE | NOT NULL | Analytics period end |
| period_type | VARCHAR(20) | NOT NULL | daily, weekly, monthly |
| total_sessions | INTEGER | NOT NULL | Session count |
| total_items_reviewed | INTEGER | NOT NULL | Items reviewed |
| avg_session_duration_min | DECIMAL(6,2) | NOT NULL | Average session length |
| avg_items_per_minute | DECIMAL(5,2) | NOT NULL | Review velocity |
| acceptance_rate | DECIMAL(5,4) | NOT NULL | Accept percentage |
| modification_rate | DECIMAL(5,4) | NOT NULL | Modify percentage |
| rejection_rate | DECIMAL(5,4) | NOT NULL | Reject percentage |
| skip_rate | DECIMAL(5,4) | NOT NULL | Skip percentage |
| avg_ai_confidence | DECIMAL(4,3) | NOT NULL | Mean confidence score |
| ai_accuracy | DECIMAL(5,4) | NULL | Calculated AI accuracy |
| batch_usage_rate | DECIMAL(5,4) | NOT NULL | Batch operation frequency |
| learning_rules_created | INTEGER | NOT NULL | New rules count |
| learning_rules_applied | INTEGER | NOT NULL | Rule applications |
| content_type_breakdown | JSONB | NOT NULL | By content type metrics |
| correction_type_breakdown | JSONB | NOT NULL | By correction type |
| created_at | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Record creation |

**Indexes**:
- `idx_analytics_tenant_period` ON (tenant_id, period_type, period_start DESC)

**Constraints**:
- CHECK: period_type IN ('daily', 'weekly', 'monthly')
- CHECK: period_end >= period_start

---

## State Transitions

### ReviewSession States

```
created -> active -> paused -> active -> completed
                  -> abandoned (on expiration)
```

### ReviewItem States

```
pending -> in_review -> accepted
                     -> rejected
                     -> modified
                     -> skipped
                     -> pending (on undo)
```

### BatchOperation States

```
pending -> confirmed -> applied -> undone
                     -> pending (on cancel)
```

## Relationships to Existing Entities

| New Entity | Existing Entity | Relationship | Notes |
|------------|-----------------|--------------|-------|
| ReviewItem | Source | Many-to-One | Each review item references one source |
| ReviewItem | ProcessingResult | Many-to-One | Links to AI processing output |
| ReviewSession | Tenant | Many-to-One | Tenant isolation |
| UserFeedback | QualityValidation | Conceptual | Feedback informs coordination decisions |
| LearningRule | CoordinationDecision | Conceptual | Rules influence future decisions |

## Migration Strategy

1. Create tables in order: ReviewSession, ReviewItem, UserFeedback, LearningRule, BatchOperation
2. Add foreign keys after all tables exist
3. Create indexes for query performance
4. No data migration needed (new feature)

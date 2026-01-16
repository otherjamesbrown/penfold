# Data Model: Relationship Discovery and Management

**Feature**: 009-relationship-discovery-and-management
**Date**: 2026-01-15
**Status**: Complete

## Entity Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│                     Relationship Data Model                          │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  ┌──────────────┐         ┌─────────────────────┐                   │
│  │    Source    │────────▶│ RelationshipEvidence │                   │
│  │   (email,    │         │   (supporting data)  │                   │
│  │   meeting)   │         └──────────┬──────────┘                   │
│  └──────────────┘                    │                               │
│                                      ▼                               │
│  ┌──────────────┐         ┌─────────────────────┐                   │
│  │    Person    │◀───────▶│    Relationship     │◀──────────────┐   │
│  │   Project    │         │  (core connection)  │               │   │
│  │    Topic     │         └──────────┬──────────┘               │   │
│  └──────────────┘                    │                          │   │
│                                      ▼                          │   │
│                          ┌─────────────────────┐                │   │
│                          │ RelationshipVersion │                │   │
│                          │  (change history)   │                │   │
│                          └─────────────────────┘                │   │
│                                      │                          │   │
│                                      ▼                          │   │
│  ┌──────────────────┐    ┌─────────────────────┐               │   │
│  │ RelationshipFeed │◀───│ RelationshipConflict│───────────────┘   │
│  │    back          │    │  (pending review)   │                   │
│  └──────────────────┘    └─────────────────────┘                   │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

## Core Entities

### Relationship

The primary entity representing a discovered connection between two entities.

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | BIGINT | PK, AUTO | Unique identifier |
| tenant_id | UUID | FK(tenants), NOT NULL, INDEX | Tenant isolation |
| source_entity_type | VARCHAR(50) | NOT NULL | Type: person, project, topic |
| source_entity_id | BIGINT | NOT NULL, INDEX | ID of source entity |
| target_entity_type | VARCHAR(50) | NOT NULL | Type: person, project, topic |
| target_entity_id | BIGINT | NOT NULL, INDEX | ID of target entity |
| relationship_type | VARCHAR(100) | NOT NULL, INDEX | Type classification |
| relationship_subtype | VARCHAR(100) | NULL | Optional subtype |
| confidence_score | DECIMAL(4,3) | NOT NULL, CHECK(0-1) | Overall confidence |
| ai_confidence | DECIMAL(4,3) | NULL | AI extraction confidence |
| evidence_strength | DECIMAL(4,3) | NULL | Evidence-based score |
| lifecycle_state | VARCHAR(20) | NOT NULL, DEFAULT 'pending' | State: pending/active/historical/archived |
| first_observed_at | TIMESTAMP WITH TZ | NOT NULL | When first discovered |
| last_evidence_at | TIMESTAMP WITH TZ | NOT NULL | Most recent evidence |
| interaction_count | INTEGER | DEFAULT 0 | Number of interactions |
| user_confirmed | BOOLEAN | DEFAULT FALSE | User validation flag |
| created_at | TIMESTAMP WITH TZ | NOT NULL | Record creation |
| updated_at | TIMESTAMP WITH TZ | NOT NULL | Last update |
| archived_at | TIMESTAMP WITH TZ | NULL | When archived |

**Indexes**:
- `idx_rel_tenant_entities` ON (tenant_id, source_entity_type, source_entity_id, target_entity_type, target_entity_id)
- `idx_rel_type` ON (relationship_type, relationship_subtype)
- `idx_rel_confidence` ON (confidence_score DESC)
- `idx_rel_lifecycle` ON (lifecycle_state, last_evidence_at)
- `idx_rel_temporal` ON (first_observed_at, last_evidence_at)

**Constraints**:
- `ck_rel_confidence_range`: confidence_score BETWEEN 0.000 AND 1.000
- `ck_rel_lifecycle_valid`: lifecycle_state IN ('pending', 'active', 'historical', 'archived')
- `ck_rel_entity_types`: source_entity_type IN ('person', 'project', 'topic')
- `uq_rel_pair`: UNIQUE (tenant_id, source_entity_type, source_entity_id, target_entity_type, target_entity_id, relationship_type)

### RelationshipEvidence

Supporting evidence that justifies relationship existence.

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | BIGINT | PK, AUTO | Unique identifier |
| tenant_id | UUID | FK(tenants), NOT NULL | Tenant isolation |
| relationship_id | BIGINT | FK(relationships), NOT NULL, INDEX | Parent relationship |
| source_id | BIGINT | FK(sources), NULL | Link to source content |
| evidence_type | VARCHAR(50) | NOT NULL | Type: email, meeting, mention, inference |
| evidence_content | TEXT | NULL | Extracted evidence text |
| evidence_context | JSONB | DEFAULT '{}' | Additional context |
| strength_contribution | DECIMAL(4,3) | NOT NULL | Contribution to confidence |
| observed_at | TIMESTAMP WITH TZ | NOT NULL | When evidence occurred |
| expires_at | TIMESTAMP WITH TZ | NOT NULL | Retention expiry (2 years) |
| created_at | TIMESTAMP WITH TZ | NOT NULL | Record creation |

**Indexes**:
- `idx_evidence_relationship` ON (relationship_id, observed_at DESC)
- `idx_evidence_source` ON (source_id)
- `idx_evidence_expiry` ON (expires_at)
- `idx_evidence_type` ON (evidence_type, observed_at)

**Constraints**:
- `ck_evidence_strength_range`: strength_contribution BETWEEN 0.000 AND 1.000
- `ck_evidence_type_valid`: evidence_type IN ('email', 'meeting', 'mention', 'inference', 'user_input')

### RelationshipFeedback

User validation and feedback for learning.

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | BIGINT | PK, AUTO | Unique identifier |
| tenant_id | UUID | FK(tenants), NOT NULL | Tenant isolation |
| relationship_id | BIGINT | FK(relationships), NOT NULL, INDEX | Target relationship |
| feedback_type | VARCHAR(20) | NOT NULL | Type: confirm, reject, modify |
| original_type | VARCHAR(100) | NULL | Original relationship type |
| corrected_type | VARCHAR(100) | NULL | User-corrected type |
| original_confidence | DECIMAL(4,3) | NULL | Original confidence |
| reasoning | TEXT | NULL | User explanation |
| feedback_metadata | JSONB | DEFAULT '{}' | Additional feedback data |
| user_email | VARCHAR(254) | NOT NULL | Feedback provider |
| created_at | TIMESTAMP WITH TZ | NOT NULL | Feedback timestamp |

**Indexes**:
- `idx_feedback_relationship` ON (relationship_id, created_at DESC)
- `idx_feedback_type` ON (feedback_type, created_at)
- `idx_feedback_user` ON (user_email, created_at)

**Constraints**:
- `ck_feedback_type_valid`: feedback_type IN ('confirm', 'reject', 'modify')

### RelationshipVersion

Historical tracking of relationship changes.

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | BIGINT | PK, AUTO | Unique identifier |
| tenant_id | UUID | FK(tenants), NOT NULL | Tenant isolation |
| relationship_id | BIGINT | FK(relationships), NOT NULL, INDEX | Target relationship |
| version_number | INTEGER | NOT NULL | Sequential version |
| relationship_type | VARCHAR(100) | NOT NULL | Type at this version |
| relationship_subtype | VARCHAR(100) | NULL | Subtype at this version |
| confidence_score | DECIMAL(4,3) | NOT NULL | Confidence at this version |
| lifecycle_state | VARCHAR(20) | NOT NULL | State at this version |
| change_reason | VARCHAR(100) | NOT NULL | What triggered change |
| change_details | JSONB | DEFAULT '{}' | Detailed change info |
| valid_from | TIMESTAMP WITH TZ | NOT NULL | Version start |
| valid_to | TIMESTAMP WITH TZ | NULL | Version end (NULL = current) |
| created_at | TIMESTAMP WITH TZ | NOT NULL | Record creation |

**Indexes**:
- `idx_version_relationship` ON (relationship_id, version_number DESC)
- `idx_version_temporal` ON (relationship_id, valid_from, valid_to)
- `idx_version_reason` ON (change_reason)

**Constraints**:
- `uq_version_number`: UNIQUE (relationship_id, version_number)
- `ck_version_change_reason`: change_reason IN ('discovery', 'evidence_update', 'user_feedback', 'confidence_recalc', 'lifecycle_transition', 'conflict_resolution')

### RelationshipConflict

Pending conflicts requiring resolution.

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | BIGINT | PK, AUTO | Unique identifier |
| tenant_id | UUID | FK(tenants), NOT NULL | Tenant isolation |
| relationship_id | BIGINT | FK(relationships), NOT NULL, INDEX | Primary relationship |
| conflicting_relationship_id | BIGINT | FK(relationships), NULL | Competing relationship |
| conflict_type | VARCHAR(50) | NOT NULL | Type: type_mismatch, duplicate, contradictory |
| primary_confidence | DECIMAL(4,3) | NOT NULL | Primary relationship confidence |
| secondary_confidence | DECIMAL(4,3) | NULL | Conflicting relationship confidence |
| confidence_gap | DECIMAL(4,3) | NOT NULL | Absolute difference |
| auto_resolvable | BOOLEAN | NOT NULL | Gap > 30% |
| resolution_status | VARCHAR(20) | NOT NULL, DEFAULT 'pending' | Status: pending, auto_resolved, user_resolved, deferred |
| resolution_details | JSONB | DEFAULT '{}' | Resolution information |
| resolved_at | TIMESTAMP WITH TZ | NULL | When resolved |
| resolved_by | VARCHAR(254) | NULL | Who resolved (user or 'system') |
| created_at | TIMESTAMP WITH TZ | NOT NULL | Conflict detection time |

**Indexes**:
- `idx_conflict_status` ON (resolution_status, created_at)
- `idx_conflict_relationship` ON (relationship_id)
- `idx_conflict_resolvable` ON (auto_resolvable, resolution_status)

**Constraints**:
- `ck_conflict_type_valid`: conflict_type IN ('type_mismatch', 'duplicate', 'contradictory', 'circular')
- `ck_conflict_status_valid`: resolution_status IN ('pending', 'auto_resolved', 'user_resolved', 'deferred')
- `ck_conflict_gap_range`: confidence_gap BETWEEN 0.000 AND 1.000

## Relationship Types

### Type Hierarchy

```python
RELATIONSHIP_TYPES = {
    "person_to_person": {
        "professional": ["colleague", "manager", "report", "mentor", "peer"],
        "communication": ["frequent_contact", "cc_only", "meeting_attendee"],
    },
    "person_to_project": {
        "role": ["owner", "contributor", "stakeholder", "reviewer", "approver"],
        "involvement": ["active", "historical", "mentioned"],
    },
    "project_to_project": {
        "dependency": ["blocks", "blocked_by", "related", "successor", "predecessor"],
        "organizational": ["parent", "child", "sibling"],
    },
    "topic_to_entity": {
        "association": ["primary", "secondary", "mentioned"],
    },
}
```

## State Transitions

### Lifecycle State Machine

```
                    ┌──────────────────────────────────────┐
                    │                                      │
                    ▼                                      │
┌─────────┐    ┌────────┐    ┌────────────┐    ┌─────────┐│
│ pending │───▶│ active │───▶│ historical │───▶│archived ││
└─────────┘    └────────┘    └────────────┘    └─────────┘│
     │              │              │                 ▲     │
     │              │              │                 │     │
     │              └──────────────┼─────────────────┘     │
     │                             │                       │
     └─────────────────────────────┴───────────────────────┘
                        (user rejection)

Transitions:
- pending -> active: High confidence (>70%) OR user confirmation
- pending -> archived: User rejection OR low confidence after 30 days
- active -> historical: No evidence for 90 days OR project completion
- historical -> archived: 2 years since last activity
- historical -> active: New evidence discovered (re-activation)
- active -> archived: User explicit rejection
```

## Pydantic Domain Models

```python
from datetime import datetime
from decimal import Decimal
from enum import Enum
from typing import Optional, List
from pydantic import BaseModel, Field, validator
from uuid import UUID

class EntityType(str, Enum):
    PERSON = "person"
    PROJECT = "project"
    TOPIC = "topic"

class LifecycleState(str, Enum):
    PENDING = "pending"
    ACTIVE = "active"
    HISTORICAL = "historical"
    ARCHIVED = "archived"

class FeedbackType(str, Enum):
    CONFIRM = "confirm"
    REJECT = "reject"
    MODIFY = "modify"

class RelationshipCreate(BaseModel):
    """Input model for creating a new relationship."""
    source_entity_type: EntityType
    source_entity_id: int
    target_entity_type: EntityType
    target_entity_id: int
    relationship_type: str
    relationship_subtype: Optional[str] = None
    ai_confidence: Decimal = Field(ge=0, le=1)
    evidence_content: Optional[str] = None
    evidence_source_id: Optional[int] = None

class RelationshipResponse(BaseModel):
    """Output model for relationship queries."""
    id: int
    source_entity_type: EntityType
    source_entity_id: int
    target_entity_type: EntityType
    target_entity_id: int
    relationship_type: str
    relationship_subtype: Optional[str]
    confidence_score: Decimal
    lifecycle_state: LifecycleState
    interaction_count: int
    user_confirmed: bool
    first_observed_at: datetime
    last_evidence_at: datetime
    created_at: datetime
    updated_at: datetime

class RelationshipFeedbackCreate(BaseModel):
    """Input model for user feedback."""
    relationship_id: int
    feedback_type: FeedbackType
    corrected_type: Optional[str] = None
    reasoning: Optional[str] = None

class RelationshipConflictResponse(BaseModel):
    """Output model for conflicts requiring review."""
    id: int
    relationship_id: int
    conflicting_relationship_id: Optional[int]
    conflict_type: str
    primary_confidence: Decimal
    secondary_confidence: Optional[Decimal]
    confidence_gap: Decimal
    auto_resolvable: bool
    resolution_status: str
    created_at: datetime
```

## Migration Strategy

1. **Phase 1**: Create new tables with foreign keys to existing entities
2. **Phase 2**: Add indexes after initial data load
3. **Phase 3**: Enable RLS policies consistent with existing tenant isolation
4. **Rollback**: Drop tables in reverse order

```sql
-- Migration: add_relationship_tables
-- Up
CREATE TABLE relationships (...);
CREATE TABLE relationship_evidence (...);
CREATE TABLE relationship_feedback (...);
CREATE TABLE relationship_versions (...);
CREATE TABLE relationship_conflicts (...);

-- Enable RLS
ALTER TABLE relationships ENABLE ROW LEVEL SECURITY;
-- ... (same for other tables)

-- Down
DROP TABLE IF EXISTS relationship_conflicts;
DROP TABLE IF EXISTS relationship_versions;
DROP TABLE IF EXISTS relationship_feedback;
DROP TABLE IF EXISTS relationship_evidence;
DROP TABLE IF EXISTS relationships;
```

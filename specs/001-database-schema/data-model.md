# Data Model: Database Schema and Storage Layer

**Feature**: Database Schema and Storage Layer
**Date**: 2026-01-12
**Phase**: 1 - Design & Contracts

## Schema Organization

### Logical Schema Structure
```sql
-- Single database with logical table organization
-- All tables include tenant_id for Row-Level Security (RLS)
-- Temporal-first design with created_at/updated_at on all entities
```

## Core Entities

### Source
Raw content from external systems with metadata and integrity verification.

```sql
CREATE TABLE sources (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL,

    -- Content identification
    source_system VARCHAR(50) NOT NULL,     -- 'gmail', 'slack', 'meetings'
    external_id VARCHAR(255) NOT NULL,      -- System-specific identifier
    content_hash VARCHAR(64) NOT NULL,      -- SHA-256 for deduplication

    -- Content storage
    raw_content TEXT,                       -- Original content
    content_type VARCHAR(100),              -- MIME type or format
    content_size INTEGER,                   -- Size in bytes

    -- Metadata
    ingestion_metadata JSONB,               -- System-specific metadata
    processing_status VARCHAR(20) DEFAULT 'pending', -- 'pending', 'processing', 'completed', 'failed'

    -- Temporal tracking
    source_timestamp TIMESTAMP WITH TIME ZONE, -- When content was created in source system
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    UNIQUE(tenant_id, source_system, external_id),
    INDEX idx_sources_tenant_created (tenant_id, created_at),
    INDEX idx_sources_status (processing_status, created_at),
    INDEX idx_sources_hash (content_hash)
);
```

**Validation Rules**:
- `source_system` must be one of: 'gmail', 'slack', 'meetings', 'documents'
- `content_hash` must be unique within tenant for deduplication
- `external_id` must be unique within (tenant_id, source_system)
- `processing_status` follows state machine: pending → processing → completed/failed

### Assertion
Extracted meaningful information with confidence scores and categorization.

```sql
CREATE TABLE assertions (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL,
    source_id BIGINT NOT NULL REFERENCES sources(id) ON DELETE CASCADE,

    -- Assertion classification
    assertion_type VARCHAR(50) NOT NULL,    -- 'decision', 'commitment', 'risk', etc.
    content TEXT NOT NULL,                  -- Extracted assertion text
    context TEXT,                          -- Surrounding context

    -- AI processing metadata
    confidence_score DECIMAL(4,3),         -- 0.000 to 1.000
    extraction_model VARCHAR(100),         -- Model used for extraction
    processing_metadata JSONB,             -- Model-specific metadata

    -- Relationships
    related_entities JSONB,                -- References to people, projects, etc.
    tags TEXT[],                          -- User or AI-assigned tags

    -- Validation and feedback
    user_validated BOOLEAN DEFAULT FALSE,
    validation_feedback JSONB,             -- User corrections and feedback

    -- Temporal tracking
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    INDEX idx_assertions_tenant_type (tenant_id, assertion_type, created_at),
    INDEX idx_assertions_confidence (confidence_score DESC),
    INDEX idx_assertions_source (source_id, created_at),
    INDEX idx_assertions_tags USING gin(tags)
);
```

**State Transitions**:
- Created with AI confidence score
- User validation updates `user_validated` and `validation_feedback`
- Confidence scores inform automation decisions

### Person
Canonical person records with aliases and organizational relationships.

```sql
CREATE TABLE people (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL,

    -- Identity
    canonical_name VARCHAR(255) NOT NULL,
    display_name VARCHAR(255),

    -- Contact methods (flexible JSON structure)
    contact_methods JSONB DEFAULT '{}',    -- emails, phone, slack, etc.
    aliases TEXT[],                       -- Known name variations

    -- Organizational context
    organization VARCHAR(255),
    job_title VARCHAR(255),
    department VARCHAR(255),

    -- Resolution metadata
    confidence_score DECIMAL(4,3),        -- Entity resolution confidence
    resolution_metadata JSONB,            -- Resolution algorithm details
    manual_override BOOLEAN DEFAULT FALSE, -- User manually verified

    -- Activity tracking
    last_seen_at TIMESTAMP WITH TIME ZONE,
    interaction_count INTEGER DEFAULT 0,

    -- Temporal tracking
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    UNIQUE(tenant_id, canonical_name),
    INDEX idx_people_tenant_name (tenant_id, canonical_name),
    INDEX idx_people_aliases USING gin(aliases),
    INDEX idx_people_contact USING gin(contact_methods),
    INDEX idx_people_activity (last_seen_at DESC, interaction_count DESC)
);
```

**Entity Resolution Logic**:
- Multiple contact methods may resolve to same person
- `aliases` array captures name variations found in content
- `confidence_score` indicates resolution algorithm certainty
- Manual overrides take precedence over automated resolution

### Project
Hierarchical project structure with timeline and team membership.

```sql
CREATE TABLE projects (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL,
    parent_id BIGINT REFERENCES projects(id) ON DELETE SET NULL,

    -- Project identity
    name VARCHAR(255) NOT NULL,
    description TEXT,
    project_code VARCHAR(50),              -- Optional short code

    -- Hierarchy and organization
    path LTREE,                           -- Hierarchical path for queries
    level INTEGER DEFAULT 0,              -- Depth in hierarchy

    -- Timeline
    start_date DATE,
    end_date DATE,
    estimated_completion DATE,
    actual_completion DATE,

    -- Status and metadata
    status VARCHAR(50) DEFAULT 'active',   -- 'active', 'completed', 'on_hold', 'cancelled'
    priority VARCHAR(20),                  -- 'high', 'medium', 'low'
    project_metadata JSONB DEFAULT '{}',   -- Flexible project-specific data

    -- Activity tracking
    last_activity_at TIMESTAMP WITH TIME ZONE,
    activity_count INTEGER DEFAULT 0,

    -- Temporal tracking
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    UNIQUE(tenant_id, name, parent_id),
    INDEX idx_projects_tenant_name (tenant_id, name),
    INDEX idx_projects_hierarchy (path),
    INDEX idx_projects_timeline (start_date, end_date),
    INDEX idx_projects_status_priority (status, priority, last_activity_at)
);
```

**Hierarchical Design**:
- Self-referential hierarchy with parent_id
- LTREE for efficient hierarchical queries
- Path materialization for fast descendant lookups

### Team
Organizational units with member relationships and project associations.

```sql
CREATE TABLE teams (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL,

    -- Team identity
    name VARCHAR(255) NOT NULL,
    description TEXT,
    team_type VARCHAR(50),                 -- 'department', 'project_team', 'working_group'

    -- Organizational structure
    parent_team_id BIGINT REFERENCES teams(id) ON DELETE SET NULL,
    reporting_structure JSONB,             -- Flexible org chart data

    -- Team metadata
    formation_date DATE,
    dissolution_date DATE,
    team_metadata JSONB DEFAULT '{}',

    -- Activity tracking
    member_count INTEGER DEFAULT 0,
    active_projects INTEGER DEFAULT 0,
    last_activity_at TIMESTAMP WITH TIME ZONE,

    -- Temporal tracking
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    UNIQUE(tenant_id, name),
    INDEX idx_teams_tenant_name (tenant_id, name),
    INDEX idx_teams_type (team_type, formation_date),
    INDEX idx_teams_hierarchy (parent_team_id)
);
```

## Event Processing Entities

### ProcessingEvent
Published events for content processing with subscriber tracking.

```sql
CREATE TABLE processing_events (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL,

    -- Event identification
    event_type VARCHAR(100) NOT NULL,      -- 'content.ingested', 'model.updated', etc.
    event_id UUID UNIQUE DEFAULT gen_random_uuid(),

    -- Event payload
    payload JSONB NOT NULL,               -- Event data (MessagePack in JSONB for queryability)
    payload_size INTEGER,                 -- Size tracking for monitoring

    -- Source and context
    source_id BIGINT REFERENCES sources(id) ON DELETE SET NULL,
    correlation_id UUID,                  -- For event tracing

    -- Processing metadata
    publisher VARCHAR(100),               -- Which component published
    subscriber_count INTEGER DEFAULT 0,   -- How many processors subscribed
    retry_count INTEGER DEFAULT 0,        -- Retry attempts

    -- State tracking
    processing_status VARCHAR(20) DEFAULT 'published', -- 'published', 'processing', 'completed', 'failed'
    error_message TEXT,                   -- Error details if failed

    -- Temporal tracking with retention
    published_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    completed_at TIMESTAMP WITH TIME ZONE,
    expires_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() + INTERVAL '30 days',

    INDEX idx_events_tenant_type (tenant_id, event_type, published_at),
    INDEX idx_events_status (processing_status, published_at),
    INDEX idx_events_correlation (correlation_id),
    INDEX idx_events_expiry (expires_at)  -- For cleanup automation
);
```

### ProcessingJob
Individual processing tasks with state and execution metadata.

```sql
CREATE TABLE processing_jobs (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL,
    event_id UUID NOT NULL REFERENCES processing_events(event_id) ON DELETE CASCADE,

    -- Job identification
    job_id UUID UNIQUE DEFAULT gen_random_uuid(),
    processor_id VARCHAR(100) NOT NULL,   -- Which AI model/processor
    processor_version VARCHAR(50),        -- Model version for tracking

    -- Job configuration
    job_config JSONB DEFAULT '{}',        -- Processor-specific config
    input_data JSONB,                     -- Input parameters

    -- State management
    job_status VARCHAR(20) DEFAULT 'queued', -- 'queued', 'in_progress', 'completed', 'failed', 'timeout'
    retry_count INTEGER DEFAULT 0,
    max_retries INTEGER DEFAULT 3,
    priority INTEGER DEFAULT 5,           -- 1-10, higher = more important

    -- Execution tracking
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    execution_time_ms INTEGER,            -- Performance tracking

    -- Error handling
    error_code VARCHAR(50),
    error_message TEXT,
    error_metadata JSONB,

    -- Resource usage
    memory_usage_mb INTEGER,
    cpu_time_ms INTEGER,

    -- Temporal tracking with retention
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    expires_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() + INTERVAL '30 days',

    INDEX idx_jobs_tenant_processor (tenant_id, processor_id, created_at),
    INDEX idx_jobs_status_priority (job_status, priority DESC, created_at),
    INDEX idx_jobs_event (event_id, created_at),
    INDEX idx_jobs_expiry (expires_at)
);
```

### ProcessingResult
Output from AI processors with confidence scores and comparison data.

```sql
CREATE TABLE processing_results (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL,
    job_id UUID NOT NULL REFERENCES processing_jobs(job_id) ON DELETE CASCADE,

    -- Result identification
    result_type VARCHAR(100) NOT NULL,    -- 'extraction', 'classification', 'embedding'
    result_format VARCHAR(50),            -- 'json', 'text', 'binary'

    -- Result data
    result_data JSONB,                    -- Structured results
    raw_output TEXT,                      -- Raw processor output if needed

    -- Quality metrics
    confidence_score DECIMAL(4,3),       -- Processor confidence
    quality_metrics JSONB,               -- Additional quality indicators

    -- Comparison and validation
    human_validated BOOLEAN DEFAULT FALSE,
    validation_feedback JSONB,           -- Human feedback on results
    alternative_results JSONB,           -- Other processor outputs for comparison

    -- Performance metadata
    processing_time_ms INTEGER,
    model_version VARCHAR(50),

    -- Temporal tracking with retention
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    expires_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() + INTERVAL '30 days',

    INDEX idx_results_tenant_type (tenant_id, result_type, created_at),
    INDEX idx_results_job (job_id, created_at),
    INDEX idx_results_confidence (confidence_score DESC),
    INDEX idx_results_expiry (expires_at)
);
```

## Vector Storage Entities

### Embedding
Vector representations linked to content with model version and similarity indexes.

```sql
CREATE TABLE embeddings (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL,

    -- Source entity linkage
    source_id BIGINT REFERENCES sources(id) ON DELETE CASCADE,
    assertion_id BIGINT REFERENCES assertions(id) ON DELETE CASCADE,
    entity_type VARCHAR(50) NOT NULL,     -- 'source', 'assertion', 'person', 'project'
    entity_id BIGINT NOT NULL,           -- Polymorphic reference

    -- Vector data
    embedding VECTOR(768) NOT NULL,      -- 768-dimensional for nomic-embed-text
    embedding_model VARCHAR(100) NOT NULL, -- Model used for generation
    model_version VARCHAR(50),           -- Version tracking

    -- Content reference
    text_content TEXT,                   -- Original text that was embedded
    content_hash VARCHAR(64),            -- For deduplication

    -- Quality and metadata
    generation_confidence DECIMAL(4,3),  -- Model confidence in embedding quality
    generation_metadata JSONB,           -- Model-specific generation data

    -- Usage tracking
    search_count INTEGER DEFAULT 0,      -- How often used in searches
    last_searched_at TIMESTAMP WITH TIME ZONE,

    -- Temporal tracking
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    INDEX idx_embeddings_tenant_entity (tenant_id, entity_type, entity_id),
    INDEX idx_embeddings_model (embedding_model, model_version),
    INDEX idx_embeddings_content_hash (content_hash),
    INDEX idx_embeddings_usage (search_count DESC, last_searched_at DESC)
);

-- HNSW index for vector similarity search
CREATE INDEX embeddings_vector_idx ON embeddings
USING hnsw (embedding vector_l2_ops)
WITH (m = 24, ef_construction = 100);
```

### Subscription
Configuration for which processors handle which event types.

```sql
CREATE TABLE subscriptions (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL,

    -- Subscription identity
    subscription_name VARCHAR(255) NOT NULL,
    processor_id VARCHAR(100) NOT NULL,   -- Subscribing processor

    -- Event filtering
    event_types TEXT[] NOT NULL,          -- Array of subscribed event types
    event_filters JSONB DEFAULT '{}',     -- Additional filtering criteria

    -- Processing configuration
    processing_config JSONB DEFAULT '{}', -- Processor-specific settings
    batch_size INTEGER DEFAULT 1,         -- How many events to batch
    max_retries INTEGER DEFAULT 3,
    timeout_seconds INTEGER DEFAULT 300,

    -- State and control
    subscription_status VARCHAR(20) DEFAULT 'active', -- 'active', 'paused', 'disabled'
    last_processed_at TIMESTAMP WITH TIME ZONE,
    events_processed INTEGER DEFAULT 0,

    -- Error tracking
    error_count INTEGER DEFAULT 0,
    last_error_at TIMESTAMP WITH TIME ZONE,
    last_error_message TEXT,

    -- Temporal tracking
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    UNIQUE(tenant_id, subscription_name),
    INDEX idx_subscriptions_tenant_processor (tenant_id, processor_id),
    INDEX idx_subscriptions_event_types USING gin(event_types),
    INDEX idx_subscriptions_status (subscription_status, last_processed_at)
);
```

## Relationship Tables

### Team Memberships
Many-to-many relationship between people and teams.

```sql
CREATE TABLE team_memberships (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL,
    person_id BIGINT NOT NULL REFERENCES people(id) ON DELETE CASCADE,
    team_id BIGINT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,

    -- Membership details
    role VARCHAR(100),                    -- 'member', 'lead', 'manager'
    start_date DATE,
    end_date DATE,
    membership_type VARCHAR(50),          -- 'permanent', 'contractor', 'temporary'

    -- Status
    is_active BOOLEAN DEFAULT TRUE,

    -- Temporal tracking
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    UNIQUE(tenant_id, person_id, team_id, start_date),
    INDEX idx_memberships_person (person_id, is_active, end_date),
    INDEX idx_memberships_team (team_id, is_active, start_date)
);
```

### Project Assignments
Many-to-many relationship between people and projects.

```sql
CREATE TABLE project_assignments (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL,
    person_id BIGINT NOT NULL REFERENCES people(id) ON DELETE CASCADE,
    project_id BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,

    -- Assignment details
    role VARCHAR(100),                    -- 'contributor', 'lead', 'stakeholder'
    allocation_percentage INTEGER,        -- 0-100, time allocation
    start_date DATE,
    end_date DATE,

    -- Status
    assignment_status VARCHAR(20) DEFAULT 'active', -- 'active', 'completed', 'on_hold'

    -- Temporal tracking
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    UNIQUE(tenant_id, person_id, project_id, start_date),
    INDEX idx_assignments_person (person_id, assignment_status, end_date),
    INDEX idx_assignments_project (project_id, assignment_status, start_date)
);
```

## Data Validation Rules

### Constraint Definitions
```sql
-- Source validation
ALTER TABLE sources ADD CONSTRAINT chk_source_system
    CHECK (source_system IN ('gmail', 'slack', 'meetings', 'documents'));
ALTER TABLE sources ADD CONSTRAINT chk_processing_status
    CHECK (processing_status IN ('pending', 'processing', 'completed', 'failed'));

-- Assertion validation
ALTER TABLE assertions ADD CONSTRAINT chk_confidence_score
    CHECK (confidence_score >= 0.0 AND confidence_score <= 1.0);
ALTER TABLE assertions ADD CONSTRAINT chk_assertion_type
    CHECK (assertion_type IN ('decision', 'commitment', 'risk', 'milestone', 'question', 'action_item', 'deadline', 'dependency'));

-- People validation
ALTER TABLE people ADD CONSTRAINT chk_person_confidence
    CHECK (confidence_score >= 0.0 AND confidence_score <= 1.0);

-- Project validation
ALTER TABLE projects ADD CONSTRAINT chk_project_status
    CHECK (status IN ('active', 'completed', 'on_hold', 'cancelled'));
ALTER TABLE projects ADD CONSTRAINT chk_project_priority
    CHECK (priority IN ('high', 'medium', 'low'));
ALTER TABLE projects ADD CONSTRAINT chk_project_dates
    CHECK (end_date IS NULL OR start_date IS NULL OR end_date >= start_date);

-- Event processing validation
ALTER TABLE processing_jobs ADD CONSTRAINT chk_job_status
    CHECK (job_status IN ('queued', 'in_progress', 'completed', 'failed', 'timeout'));
ALTER TABLE processing_jobs ADD CONSTRAINT chk_priority_range
    CHECK (priority >= 1 AND priority <= 10);

-- Vector validation
ALTER TABLE embeddings ADD CONSTRAINT chk_embedding_confidence
    CHECK (generation_confidence >= 0.0 AND generation_confidence <= 1.0);
```

## Row-Level Security (RLS)

### Security Policies
```sql
-- Enable RLS on all tables
ALTER TABLE sources ENABLE ROW LEVEL SECURITY;
ALTER TABLE assertions ENABLE ROW LEVEL SECURITY;
ALTER TABLE people ENABLE ROW LEVEL SECURITY;
ALTER TABLE projects ENABLE ROW LEVEL SECURITY;
ALTER TABLE teams ENABLE ROW LEVEL SECURITY;
ALTER TABLE processing_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE processing_jobs ENABLE ROW LEVEL SECURITY;
ALTER TABLE processing_results ENABLE ROW LEVEL SECURITY;
ALTER TABLE embeddings ENABLE ROW LEVEL SECURITY;
ALTER TABLE subscriptions ENABLE ROW LEVEL SECURITY;

-- Tenant isolation policies
CREATE POLICY tenant_isolation_sources ON sources
    FOR ALL TO application_user
    USING (tenant_id = current_setting('app.tenant_id')::UUID);

CREATE POLICY tenant_isolation_assertions ON assertions
    FOR ALL TO application_user
    USING (tenant_id = current_setting('app.tenant_id')::UUID);

-- Repeat for all tables with tenant_id...
```

## Archive and Audit Strategy

### Archive Tables
```sql
-- Archive structure mirrors main tables with additional tracking
CREATE TABLE sources_archive (
    LIKE sources INCLUDING ALL,
    deleted_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_by VARCHAR(255),
    deletion_reason TEXT
);

CREATE TABLE assertions_archive (
    LIKE assertions INCLUDING ALL,
    deleted_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_by VARCHAR(255),
    deletion_reason TEXT
);

-- Soft delete triggers
CREATE OR REPLACE FUNCTION archive_source()
RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO sources_archive SELECT OLD.*, NOW(), current_user, 'soft_delete';
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER sources_archive_trigger
    BEFORE DELETE ON sources
    FOR EACH ROW EXECUTE FUNCTION archive_source();
```
# Penfold Database Schema Documentation

> **Status**: Production Ready - Implementation complete with comprehensive testing
> **Last Updated**: 2026-01-23
> **Version**: 2.0

## Overview

The Penfold database schema provides a multi-tenant storage layer for personal AI-powered information management. Built on PostgreSQL 16+ with pgvector extension, it enables hybrid relational and vector storage with complete tenant isolation.

## Quick Start

### Prerequisites
- PostgreSQL 16+ with pgvector extension
- Go 1.22+

### Setup Database
```bash
# Run migrations (from project root)
penf db migrate

# Verify setup
go test ./pkg/db/... -v
```

## Architecture

### Multi-Tenant Isolation
- **Row-Level Security (RLS)**: Automatic tenant filtering via PostgreSQL policies
- **Session Context**: Persistent tenant selection via `current_setting('app.current_tenant')`
- **Shared Entities**: People can be linked across tenants while maintaining data isolation
- **Complete Separation**: All other entities are tenant-isolated

### Migration Management
- Migrations are located in `/migrations/` directory
- Files are numbered sequentially: `001_ingest_tables.sql`, `002_content_enrichment.sql`, etc.
- Migrations are tracked in `schema_migrations` table

## Database Schema

### Core Tables Summary

| Table | Purpose | Migration |
|-------|---------|-----------|
| sources | Raw content from external systems | 001 (spec) |
| tenants | Tenant organizations | 007 |
| people | Canonical person records | 003 |
| projects | Project organization | 003 |
| teams | Team structures | 003 |
| products | Business products with hierarchy | 015 |
| glossary | Acronyms and terminology | 013 |

### Content Processing Tables

| Table | Purpose | Migration |
|-------|---------|-----------|
| content_enrichment | Classification and enrichment results | 002 |
| enrichment_stages | Processing stage tracking | 002 |
| assertions | AI-extracted information (RAID+) | 005 |
| content_sentiment | Sentiment analysis results | 005 |
| extraction_runs | AI extraction audit trail | 005 |
| extraction_feedback | User corrections on extractions | 005 |
| prompt_templates | Configurable extraction prompts | 005 |

### Ingest and Queue Infrastructure

| Table | Purpose | Migration |
|-------|---------|-----------|
| ingest_jobs | Batch import job tracking | 001 |
| ingest_errors | Per-file failure records | 001 |
| dead_letter_items | Failed queue messages | 006 |
| queue_stats | Hourly queue metrics | 006 |
| processing_batches | Batch processing progress | 006 |
| workers | Worker registration and health | 006 |

### Entity Resolution Tables

| Table | Purpose | Migration |
|-------|---------|-----------|
| person_aliases | Email, Slack, name aliases for people | 003 |
| team_members | Team membership relationships | 003 |
| project_members | Project membership (people or teams) | 003 |
| content_mentions | Unified mention tracking | 017 |
| mention_patterns | Resolution pattern history | 017 |
| entity_project_affinity | Entity-project relevance scores | 017 |

### Communication Tables

| Table | Purpose | Migration |
|-------|---------|-----------|
| email_threads | Thread grouping for emails | 004 |
| thread_messages | Thread-message membership | 004 |
| meetings | Calendar meeting records | 004, 010 |
| meeting_attendees | Meeting attendance | 004 |
| meeting_events | Meeting lifecycle events | 004 |
| meeting_participants | Resolved meeting participants | 011 |
| meeting_mentions | People mentioned in meetings | 012 |

### Link and Document Tables

| Table | Purpose | Migration |
|-------|---------|-----------|
| extracted_links | Deduplicated URLs from content | 004 |
| link_sources | Link-to-source junction | 004 |
| link_enrichment | Enriched link metadata | 004 |
| email_attachments | Attachment metadata | 004 |
| attachment_enrichment | Processed attachment data | 004 |
| source_attachments | Parent-child source links | 008 |

### Jira Integration Tables

| Table | Purpose | Migration |
|-------|---------|-----------|
| jira_tickets | Current Jira ticket state | 004 |
| jira_ticket_changes | Ticket change history | 004 |
| tenant_jira_mappings | Jira-Penfold project mapping | 007 |

### Tenant Configuration Tables

| Table | Purpose | Migration |
|-------|---------|-----------|
| tenant_domains | Internal/external domain classification | 007 |
| tenant_email_patterns | Bot/distribution list detection | 007 |
| tenant_integrations | External system integrations | 007 |
| tenant_processing_rules | Content classification overrides | 007 |

### Product Management Tables

| Table | Purpose | Migration |
|-------|---------|-----------|
| products | Products with 3-level hierarchy | 015 |
| product_aliases | Alternative product names | 015 |
| product_teams | Team-product associations | 015 |
| product_team_roles | Scoped role assignments | 015 |
| product_events | Product timeline events | 015 |
| product_event_links | Event-to-entity links | 015 |

### Review and AI Tables

| Table | Purpose | Migration |
|-------|---------|-----------|
| review_queue | Questions requiring human input | 014 |
| extraction_experiments | A/B test configurations | 005 |
| extraction_experiment_results | Per-extraction A/B results | 005 |
| resolution_traces | Resolution process audit | 017 |
| resolution_trace_stages | Stage-level trace data | 017 |
| resolution_llm_calls | LLM request/response logs | 017 |
| resolution_decisions | Decision reasoning records | 017 |
| resolution_comparisons | Model comparison runs | 018 |
| resolution_comparison_decisions | Per-mention model comparisons | 018 |

---

## Detailed Schema Reference

### sources
Raw content from external systems (email, Slack, documents, meetings).

```sql
CREATE TABLE sources (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL,
    source_system VARCHAR(50) NOT NULL,     -- 'gmail', 'slack', 'meetings'
    external_id VARCHAR(255) NOT NULL,
    content_hash VARCHAR(64) NOT NULL,
    raw_content TEXT,
    content_type VARCHAR(100),
    content_size INTEGER,
    ingestion_metadata JSONB,
    processing_status VARCHAR(20) DEFAULT 'pending',
    source_timestamp TIMESTAMPTZ,
    meeting_id BIGINT,                      -- FK to meetings (migration 010)
    participant_emails TEXT[],              -- All participant emails (migration 020)
    content_id VARCHAR(12),                 -- Human-readable tracing ID (migration 021)
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(tenant_id, source_system, external_id)
);
```

**Key Indexes**: tenant_created, status, content_hash, meeting_id, participant_emails (GIN), content_id (unique partial)

### tenants
Tenant organizations for multi-tenant support.

```sql
CREATE TABLE tenants (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(63) NOT NULL UNIQUE,
    description TEXT,
    is_active BOOLEAN NOT NULL DEFAULT true,
    settings JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### people
Canonical person records resolved from various identifiers.

```sql
CREATE TABLE people (
    id BIGSERIAL PRIMARY KEY,
    tenant_id VARCHAR(255) NOT NULL,
    canonical_name VARCHAR(500) NOT NULL,
    primary_email VARCHAR(500) NOT NULL,
    title VARCHAR(255),
    department VARCHAR(255),
    company VARCHAR(255),                   -- Added migration 019
    country VARCHAR(100),                   -- Added migration 015
    is_internal BOOLEAN DEFAULT FALSE,
    account_type account_type NOT NULL DEFAULT 'person',  -- person, role, distribution, bot, external_service
    confidence REAL NOT NULL DEFAULT 0.6,
    needs_review BOOLEAN DEFAULT TRUE,
    auto_created BOOLEAN DEFAULT TRUE,
    reviewed_at TIMESTAMPTZ,
    reviewed_by VARCHAR(255),
    potential_duplicates BIGINT[] DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (tenant_id, primary_email)
);
```

### products
Business products with 3-level hierarchy (product -> sub_product -> feature).

```sql
CREATE TABLE products (
    id BIGSERIAL PRIMARY KEY,
    tenant_id VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    parent_id BIGINT REFERENCES products(id),
    product_type product_type NOT NULL DEFAULT 'product',  -- product, sub_product, feature
    status product_status NOT NULL DEFAULT 'active',       -- active, beta, sunset, deprecated
    keywords TEXT[] DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, name)
);
```

### glossary
Domain terminology and acronym definitions for query expansion.

```sql
CREATE TABLE glossary (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL,
    term VARCHAR(100) NOT NULL,
    expansion VARCHAR(500) NOT NULL,
    definition TEXT,
    context JSONB DEFAULT '[]'::jsonb,
    aliases JSONB DEFAULT '[]'::jsonb,
    expand_in_search BOOLEAN DEFAULT true,
    source VARCHAR(50) DEFAULT 'manual',
    linked_entity_type VARCHAR(50),         -- product, project, company (migration 016)
    linked_entity_id BIGINT,                -- FK to linked entity (migration 016)
    embedding vector(1024),                 -- For semantic search (migration 021)
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by VARCHAR(255),
    UNIQUE (tenant_id, term)
);
```

**Vector Index**: HNSW index on embedding column for fast semantic search.

### content_enrichment
Stores classification and enrichment results for each ingested source.

```sql
CREATE TABLE content_enrichment (
    id BIGSERIAL PRIMARY KEY,
    source_id BIGINT NOT NULL,
    tenant_id VARCHAR(255) NOT NULL,
    content_type content_type NOT NULL,     -- email, calendar, document, attachment
    content_subtype VARCHAR(100) NOT NULL,
    processing_profile processing_profile NOT NULL,
    classification_confidence REAL DEFAULT 1.0,
    classification_reason TEXT,
    status enrichment_status NOT NULL DEFAULT 'pending',
    current_stage VARCHAR(50),
    error_message TEXT,
    retry_count INTEGER DEFAULT 0,
    participants JSONB DEFAULT '[]'::jsonb,
    resolved_participants JSONB DEFAULT '[]'::jsonb,
    extracted_links JSONB DEFAULT '[]'::jsonb,
    thread_id VARCHAR(255),
    project_id BIGINT,
    extracted_data JSONB DEFAULT '{}'::jsonb,
    ai_processed BOOLEAN DEFAULT FALSE,
    ai_skip_reason TEXT,
    ai_processed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    UNIQUE (source_id)
);
```

**Processing Profiles**: full_ai, full_ai_chunked, metadata_only, state_tracking, structure_only, ocr_if_text

### assertions
Extracted assertions (RAID+) from content, grounded to entities.

```sql
CREATE TABLE assertions (
    id BIGSERIAL PRIMARY KEY,
    tenant_id VARCHAR(255) NOT NULL,
    source_id BIGINT NOT NULL,
    thread_id BIGINT REFERENCES email_threads(id),
    extraction_run_id BIGINT,
    assertion_type assertion_type NOT NULL,  -- risk, action, issue, decision, commitment, question
    description TEXT NOT NULL,
    source_quote TEXT,
    confidence REAL DEFAULT 0.8,
    owner_person_id BIGINT REFERENCES people(id),
    assignee_person_id BIGINT REFERENCES people(id),
    target_person_id BIGINT REFERENCES people(id),
    decision_maker_person_id BIGINT REFERENCES people(id),
    committer_person_id BIGINT REFERENCES people(id),
    committed_to_person_id BIGINT REFERENCES people(id),
    project_id BIGINT REFERENCES projects(id),
    ticket_id BIGINT REFERENCES jira_tickets(id),
    severity assertion_severity,             -- low, medium, high, critical
    status action_status DEFAULT 'open',     -- open, in_progress, completed, cancelled
    due_date TIMESTAMPTZ,
    due_date_source TEXT,
    rationale TEXT,
    answered BOOLEAN DEFAULT FALSE,
    is_current BOOLEAN DEFAULT TRUE,
    superseded_by BIGINT REFERENCES assertions(id),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
```

### content_mentions
Unified table for all entity mentions extracted from content.

```sql
CREATE TABLE content_mentions (
    id BIGSERIAL PRIMARY KEY,
    tenant_id VARCHAR(255) NOT NULL,
    content_id BIGINT NOT NULL,
    entity_type mention_entity_type NOT NULL,  -- person, term, product, company, project
    mentioned_text VARCHAR(500) NOT NULL,
    position INT,
    context_snippet TEXT,
    resolved_entity_id BIGINT,
    resolution_confidence REAL,
    resolution_source VARCHAR(100),            -- exact_match, alias, fuzzy, project_context, prior_link, user_confirmed
    resolved_expansion VARCHAR(500),
    candidates JSONB DEFAULT '[]'::jsonb,
    status mention_status NOT NULL DEFAULT 'pending',  -- pending, auto_resolved, user_resolved, dismissed
    resolved_at TIMESTAMPTZ,
    resolved_by VARCHAR(255),
    project_context_id BIGINT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
```

### review_queue
Queue for AI questions requiring human review.

```sql
CREATE TABLE review_queue (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL,
    question_type VARCHAR(50) NOT NULL,     -- acronym, person, entity, duplicate, other
    priority VARCHAR(20) NOT NULL DEFAULT 'medium',  -- high, medium, low
    question TEXT NOT NULL,
    context TEXT,
    source_type VARCHAR(50),
    source_id BIGINT,
    source_reference VARCHAR(255),
    suggested_term VARCHAR(100),
    suggested_expansion VARCHAR(500),
    candidate_person_ids BIGINT[],
    matched_text VARCHAR(255),
    status VARCHAR(20) NOT NULL DEFAULT 'pending',  -- pending, resolved, dismissed, deferred
    resolution TEXT,
    resolved_at TIMESTAMPTZ,
    resolved_by VARCHAR(255),
    confidence DECIMAL(3,2) DEFAULT 0.5,
    metadata JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

---

## Enum Types

The schema uses PostgreSQL enums for type safety:

| Enum | Values | Used In |
|------|--------|---------|
| content_type | email, calendar, document, attachment | content_enrichment |
| processing_profile | full_ai, full_ai_chunked, metadata_only, state_tracking, structure_only, ocr_if_text | content_enrichment |
| enrichment_status | pending, classifying, enriching, extracting, ai_processing, completed, failed, skipped | content_enrichment |
| account_type | person, role, distribution, bot, external_service | people |
| alias_type | email, slack_id, name, display_name | person_aliases |
| link_category | google_doc, google_sheet, google_slides, google_drive, jira_ticket, jira_board, confluence, webex_recording, zoom_recording, sharepoint, onedrive, github, gitlab, bitbucket, slack, teams, generic_url | extracted_links |
| assertion_type | risk, action, issue, decision, commitment, question | assertions |
| assertion_severity | low, medium, high, critical | assertions |
| action_status | open, in_progress, completed, cancelled | assertions |
| meeting_status | active, cancelled, updated | meetings |
| attendee_response | accepted, declined, tentative, none | meeting_attendees |
| product_type | product, sub_product, feature | products |
| product_status | active, beta, sunset, deprecated | products |
| mention_entity_type | person, term, product, company, project | content_mentions |
| mention_status | pending, auto_resolved, user_resolved, dismissed | content_mentions |
| error_category | transient, permanent, partial, dependency | dead_letter_items |
| batch_status | pending, processing, completed, partial, failed | processing_batches |
| worker_status | starting, healthy, unhealthy, draining, stopped | workers |

---

## Vector Storage

### pgvector Configuration
- **Extension**: pgvector for vector similarity search
- **Default Dimensions**: 768 (nomic-embed-text compatible) or 1024 (mxbai-embed-large-v1)
- **Index Type**: HNSW for fast approximate nearest neighbor search

### Vector-Enabled Tables

| Table | Column | Dimensions | Index |
|-------|--------|------------|-------|
| glossary | embedding | 1024 | HNSW (m=16, ef_construction=64) |

### Vector Search Example
```sql
-- Find similar glossary terms
SELECT term, expansion,
       embedding <=> '[query_vector]'::vector AS distance
FROM glossary
WHERE tenant_id = :tenant_id
ORDER BY embedding <=> '[query_vector]'::vector
LIMIT 10;
```

---

## Performance Characteristics

| Operation | Target | Notes |
|-----------|--------|-------|
| CRUD Operations | <100ms | 10K records/tenant |
| Vector Similarity Search | <500ms | 100K vectors/tenant |
| Queue Processing | <50ms | Pub/sub operations |
| Migration Time | <15 minutes | Full schema |
| Concurrent Connections | 50+ | Connection pooling |

---

## Key Indexes

### Full-Text Search Indexes
- `idx_glossary_search` - GIN index on term + expansion + definition

### GIN Indexes (JSONB/Array)
- `idx_glossary_aliases` - Alias lookups
- `idx_glossary_context` - Context filtering
- `idx_projects_keywords` - Project keyword search
- `idx_products_keywords` - Product keyword search
- `idx_sources_participant_emails` - Email participant queries

### Partial Indexes (Conditional)
- `idx_content_enrichment_pending` - Only pending records
- `idx_content_enrichment_failed` - Only failed records with retry count
- `idx_people_needs_review` - Only records needing review
- `idx_review_queue_term` - Only records with suggested_term

---

## Row-Level Security

All tables with `tenant_id` have RLS policies enabled:

```sql
-- Example policy pattern
ALTER TABLE table_name ENABLE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON table_name
    USING (tenant_id = current_setting('app.current_tenant', true));
```

### Setting Tenant Context
```go
_, err := db.Exec(ctx, "SET app.current_tenant = $1", tenantID)
```

---

## Views

### v_tenant_config
Aggregated tenant configuration for quick lookups.

```sql
SELECT
    tenant_id,
    tenant_slug,
    tenant_name,
    internal_domains,
    bot_patterns,
    distribution_patterns,
    role_account_patterns,
    active_integrations,
    active_rules
FROM v_tenant_config;
```

---

## Triggers

### Updated Timestamp Triggers
Most tables have automatic `updated_at` timestamp management:
- `trg_glossary_updated_at`
- `trg_review_queue_updated_at`
- `trg_products_updated_at`
- `trg_product_teams_updated_at`
- `trg_product_team_roles_updated_at`
- `trg_product_events_updated_at`
- `trg_entity_affinity_updated_at`

---

## Migration Files

| File | Description |
|------|-------------|
| 001_ingest_tables.sql | Batch email ingest job tracking |
| 002_content_enrichment.sql | Content classification and enrichment pipeline |
| 003_entity_resolution.sql | People, teams, projects, and aliases |
| 004_type_handlers.sql | Links, Jira, meetings, threads, attachments |
| 005_ai_extraction.sql | Assertions, templates, extraction audit, A/B testing |
| 006_queue_infrastructure.sql | Dead letter queue, stats, batches, workers |
| 007_tenant_configuration.sql | Tenants, domains, patterns, integrations, rules |
| 008_source_attachments.sql | Parent-child source attachment links |
| 009_drop_sources_fulltext_index.sql | Remove unused tsvector index |
| 010_meetings.sql | Meetings table with sources.meeting_id FK |
| 011_meeting_participants.sql | Meeting-person junction table |
| 012_meeting_mentions.sql | People mentioned in meetings |
| 013_glossary.sql | Acronym and terminology definitions |
| 014_review_queue.sql | AI questions requiring human input |
| 015_products.sql | Products, aliases, teams, roles, events |
| 016_glossary_linked_entity.sql | Link glossary terms to entities |
| 017_mention_resolution.sql | Unified mention resolution and tracing |
| 018_resolution_comparisons.sql | Model comparison infrastructure |
| 019_people_company.sql | Add company field to people |
| 020_sources_participant_emails.sql | Add participant_emails array to sources |
| 021_glossary_embeddings.sql | Add embedding column for semantic search |

---

## Troubleshooting

### Check Migration Status
```bash
penf db status
```

### View Applied Migrations
```sql
SELECT version, applied_at FROM schema_migrations ORDER BY version;
```

### Verify RLS Policies
```sql
SELECT schemaname, tablename, policyname, cmd, qual
FROM pg_policies
WHERE schemaname = 'public';
```

### Check Index Usage
```sql
SELECT schemaname, tablename, indexname, idx_scan, idx_tup_read
FROM pg_stat_user_indexes
WHERE schemaname = 'public'
ORDER BY idx_scan DESC;
```

---

## Support

For issues or questions:
1. Check the troubleshooting section above
2. Review migration files for schema details
3. Consult `context/database-dev/agents.md` for database agent documentation
4. Create a bead for database-related issues

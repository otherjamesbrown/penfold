# Data Model: Unified Mention Resolution System

**Date**: 2026-01-21 | **Branch**: `013-content-enrichment`

## Entity Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                            MENTION RESOLUTION                                │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌─────────────────┐      ┌─────────────────┐      ┌─────────────────┐      │
│  │ content_mentions│──────│ mention_patterns│      │entity_project_  │      │
│  │ (EXISTING)      │      │ (EXISTING)      │      │affinity(EXISTING│      │
│  └────────┬────────┘      └─────────────────┘      └─────────────────┘      │
│           │                                                                  │
│           │ resolution                                                       │
│           ▼                                                                  │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                     AUDIT & TRACING (NEW)                            │    │
│  ├─────────────────────────────────────────────────────────────────────┤    │
│  │                                                                       │    │
│  │  ┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐   │    │
│  │  │resolution_traces│───►│resolution_trace_│───►│resolution_llm_  │   │    │
│  │  │                 │    │stages           │    │calls            │   │    │
│  │  └────────┬────────┘    └─────────────────┘    └─────────────────┘   │    │
│  │           │                                                           │    │
│  │           │                                                           │    │
│  │           ▼                                                           │    │
│  │  ┌─────────────────┐                                                  │    │
│  │  │resolution_      │ (individual decisions with reasoning)           │    │
│  │  │decisions        │                                                  │    │
│  │  └─────────────────┘                                                  │    │
│  │                                                                       │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                     MODEL COMPARISON (NEW)                           │    │
│  ├─────────────────────────────────────────────────────────────────────┤    │
│  │                                                                       │    │
│  │  ┌─────────────────┐    ┌─────────────────┐                          │    │
│  │  │resolution_      │───►│resolution_      │                          │    │
│  │  │comparisons      │    │comparison_      │                          │    │
│  │  │                 │    │decisions        │                          │    │
│  │  └─────────────────┘    └─────────────────┘                          │    │
│  │                                                                       │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Existing Entities (Reference)

### content_mentions

Already defined in `pkg/mentions/types.go`. Key fields:

| Field | Type | Description |
|-------|------|-------------|
| id | BIGSERIAL | Primary key |
| tenant_id | TEXT | Multi-tenant isolation |
| content_id | BIGINT | FK to source content |
| entity_type | TEXT | person, term, product, company, project |
| mentioned_text | TEXT | The text that was mentioned |
| position | INT | Character offset in content |
| context_snippet | TEXT | Surrounding text |
| resolved_entity_id | BIGINT | FK to resolved entity (nullable) |
| resolution_confidence | DECIMAL(3,2) | 0.00-1.00 |
| resolution_source | TEXT | exact_match, alias, fuzzy, etc. |
| status | TEXT | pending, auto_resolved, user_resolved, dismissed |
| created_at | TIMESTAMPTZ | Creation timestamp |

### glossary_terms (Extended)

Add linked entity support:

| Field | Type | Description | NEW |
|-------|------|-------------|-----|
| linked_entity_type | TEXT | product, project, company | ✓ |
| linked_entity_id | BIGINT | FK to linked entity | ✓ |

## New Entities

### resolution_traces

Top-level trace record for each resolution run.

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | TEXT | PRIMARY KEY | trace_abc123 format |
| tenant_id | TEXT | NOT NULL | Multi-tenant isolation |
| content_id | BIGINT | NOT NULL | FK to content being processed |
| content_type | TEXT | | email, meeting, document |
| content_summary | TEXT | | Brief description |
| started_at | TIMESTAMPTZ | NOT NULL | When processing started |
| completed_at | TIMESTAMPTZ | | When processing finished |
| duration_ms | INT | | Total processing time |
| mentions_found | INT | | Count of mentions extracted |
| auto_resolved | INT | | Count auto-resolved |
| queued_for_review | INT | | Count needing human review |
| new_entities_suggested | INT | | Count of new entity suggestions |
| status | TEXT | DEFAULT 'in_progress' | in_progress, completed, failed |
| error_message | TEXT | | Error details if failed |
| model_used | TEXT | | mlx-mistral-7b, claude-sonnet |
| trace_level | TEXT | | minimal, standard, full, debug |
| config_snapshot | JSONB | | Configuration at time of run |
| created_at | TIMESTAMPTZ | DEFAULT NOW() | Record creation |

**Indexes**:
- `idx_traces_content` ON (content_id)
- `idx_traces_tenant_time` ON (tenant_id, started_at DESC)
- `idx_traces_status` ON (tenant_id, status) WHERE status != 'completed'

### resolution_trace_stages

Individual stage records within a trace.

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | BIGSERIAL | PRIMARY KEY | Auto-increment |
| trace_id | TEXT | FK resolution_traces | Parent trace |
| stage_number | INT | NOT NULL | 1, 2, 3, 4 |
| stage_name | TEXT | NOT NULL | understanding, cross_mention, matching, verification |
| started_at | TIMESTAMPTZ | NOT NULL | Stage start time |
| completed_at | TIMESTAMPTZ | | Stage end time |
| duration_ms | INT | | Stage processing time |
| input_summary | TEXT | | Human-readable input description |
| input_data | JSONB | | Full structured input |
| output_summary | TEXT | | Human-readable output description |
| output_data | JSONB | | Full structured output |
| status | TEXT | DEFAULT 'in_progress' | in_progress, completed, failed, skipped |
| skipped | BOOLEAN | DEFAULT false | Whether stage was skipped |
| skip_reason | TEXT | | Why stage was skipped |
| error_message | TEXT | | Error details if failed |
| created_at | TIMESTAMPTZ | DEFAULT NOW() | Record creation |

**Indexes**:
- `idx_stages_trace` ON (trace_id, stage_number)

### resolution_llm_calls

LLM request/response logs (for full/debug trace levels).

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | BIGSERIAL | PRIMARY KEY | Auto-increment |
| trace_id | TEXT | FK resolution_traces | Parent trace |
| stage_id | BIGINT | FK resolution_trace_stages | Parent stage |
| model | TEXT | NOT NULL | mlx-mistral-7b, claude-3-sonnet |
| prompt_template | TEXT | | Template name used |
| prompt_text | TEXT | | Full prompt (can be large) |
| prompt_tokens | INT | | Token count estimate |
| response_text | TEXT | | Full response (can be large) |
| response_tokens | INT | | Token count estimate |
| parsed_output | JSONB | | Structured data extracted |
| parse_errors | TEXT[] | | Any parsing issues |
| latency_ms | INT | | Response time |
| attempt_number | INT | DEFAULT 1 | Retry attempt |
| is_fallback | BOOLEAN | DEFAULT false | Whether this was fallback provider |
| fallback_reason | TEXT | | Why fallback was used |
| created_at | TIMESTAMPTZ | DEFAULT NOW() | Record creation |

**Indexes**:
- `idx_llm_calls_trace` ON (trace_id)
- `idx_llm_calls_stage` ON (stage_id)

### resolution_decisions

Individual resolution decisions with reasoning (kept for 1 year).

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | BIGSERIAL | PRIMARY KEY | Auto-increment |
| trace_id | TEXT | FK resolution_traces | Parent trace |
| stage_id | BIGINT | FK resolution_trace_stages | Parent stage |
| decision_type | TEXT | NOT NULL | resolve, queue_review, suggest_new_entity, skip_verification |
| mention_id | BIGINT | | FK to content_mentions |
| mentioned_text | TEXT | | The text being resolved |
| chosen_option | TEXT | | What was decided |
| alternatives | JSONB | | Other options considered |
| confidence | DECIMAL(3,2) | | 0.00-1.00 |
| reasoning | TEXT | | LLM's reasoning |
| factors | JSONB | | Structured factors |
| was_correct | BOOLEAN | | NULL until human reviews |
| correction_notes | TEXT | | Notes on correction |
| created_at | TIMESTAMPTZ | DEFAULT NOW() | Record creation |

**Indexes**:
- `idx_decisions_trace` ON (trace_id)
- `idx_decisions_mention` ON (mention_id)
- `idx_decisions_corrections` ON (trace_id) WHERE was_correct = false

### resolution_comparisons

Model comparison run records.

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | TEXT | PRIMARY KEY | comp_xyz789 format |
| tenant_id | TEXT | NOT NULL | Multi-tenant isolation |
| content_id | BIGINT | NOT NULL | Content being compared |
| content_type | TEXT | | email, meeting, document |
| content_summary | TEXT | | Brief description |
| models | TEXT[] | NOT NULL | Models compared |
| trace_ids | TEXT[] | NOT NULL | Linked traces (one per model) |
| initiated_by | TEXT | | user, scheduled, ci |
| purpose | TEXT | | model_evaluation, regression_test, debugging |
| started_at | TIMESTAMPTZ | NOT NULL | Comparison start |
| completed_at | TIMESTAMPTZ | | Comparison end |
| total_decisions | INT | | Total decisions compared |
| unanimous_decisions | INT | | All models agreed |
| divergent_decisions | INT | | Models disagreed |
| divergence_summary | JSONB | | Summary of differences |
| created_at | TIMESTAMPTZ | DEFAULT NOW() | Record creation |

**Indexes**:
- `idx_comparisons_content` ON (content_id)
- `idx_comparisons_tenant` ON (tenant_id, created_at DESC)

### resolution_comparison_decisions

Per-mention comparison across models.

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | BIGSERIAL | PRIMARY KEY | Auto-increment |
| comparison_id | TEXT | FK resolution_comparisons | Parent comparison |
| mentioned_text | TEXT | NOT NULL | The mention being compared |
| mention_index | INT | | Position in content |
| model_decisions | JSONB | NOT NULL | Decision from each model |
| is_unanimous | BOOLEAN | | All models agreed |
| divergence_type | TEXT | | different_entity, confidence_gap, new_vs_existing |
| confidence_spread | DECIMAL(3,2) | | Max - min confidence |
| ground_truth_entity_id | BIGINT | | Known correct answer |
| models_correct | TEXT[] | | Which models got it right |
| created_at | TIMESTAMPTZ | DEFAULT NOW() | Record creation |

**Indexes**:
- `idx_comp_decisions_comparison` ON (comparison_id)
- `idx_comp_decisions_divergent` ON (comparison_id) WHERE is_unanimous = false

## State Transitions

### Trace Status

```
in_progress ──────► completed
     │
     └───────────► failed
```

### Stage Status

```
in_progress ──────► completed
     │                  │
     │                  └──► (proceeds to next stage)
     │
     └───────────► failed ──► (trace fails)
     │
     └───────────► skipped ──► (proceeds to next stage)
```

### Decision Outcome Tracking

```
(created) ──► was_correct = NULL
                    │
        ┌───────────┼───────────┐
        ▼           │           ▼
was_correct = true  │  was_correct = false
(confirmed)         │  (corrected)
                    │
                    ▼
        (no review needed - auto-resolved high confidence)
```

## Validation Rules

### resolution_traces

- `status` must be one of: in_progress, completed, failed
- `trace_level` must be one of: minimal, standard, full, debug
- `completed_at` must be >= `started_at` when set
- `duration_ms` = `completed_at` - `started_at` (computed)

### resolution_trace_stages

- `stage_number` must be 1, 2, 3, or 4
- `stage_name` must match expected name for stage_number
- `skipped` = true requires `skip_reason`

### resolution_decisions

- `confidence` must be between 0.00 and 1.00
- `decision_type` must be one of: resolve, queue_review, suggest_new_entity, skip_verification
- `reasoning` is required (NOT NULL equivalent via application logic)

## Migration Files

### 016_glossary_linked_entity.sql

```sql
-- Add linked entity support to glossary terms
ALTER TABLE glossary_terms
    ADD COLUMN linked_entity_type TEXT,
    ADD COLUMN linked_entity_id BIGINT;

-- Index for lookups
CREATE INDEX idx_glossary_linked
    ON glossary_terms(linked_entity_type, linked_entity_id)
    WHERE linked_entity_type IS NOT NULL;

-- Check constraint
ALTER TABLE glossary_terms
    ADD CONSTRAINT chk_linked_entity
    CHECK (
        (linked_entity_type IS NULL AND linked_entity_id IS NULL) OR
        (linked_entity_type IS NOT NULL AND linked_entity_id IS NOT NULL)
    );
```

### 017_mention_resolution.sql

```sql
-- Resolution traces
CREATE TABLE resolution_traces (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    content_id BIGINT NOT NULL,
    content_type TEXT,
    content_summary TEXT,
    started_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,
    duration_ms INT,
    mentions_found INT,
    auto_resolved INT,
    queued_for_review INT,
    new_entities_suggested INT,
    status TEXT DEFAULT 'in_progress',
    error_message TEXT,
    model_used TEXT,
    trace_level TEXT,
    config_snapshot JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_traces_content ON resolution_traces(content_id);
CREATE INDEX idx_traces_tenant_time ON resolution_traces(tenant_id, started_at DESC);
CREATE INDEX idx_traces_status ON resolution_traces(tenant_id, status)
    WHERE status != 'completed';

-- Trace stages
CREATE TABLE resolution_trace_stages (
    id BIGSERIAL PRIMARY KEY,
    trace_id TEXT NOT NULL REFERENCES resolution_traces(id) ON DELETE CASCADE,
    stage_number INT NOT NULL,
    stage_name TEXT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,
    duration_ms INT,
    input_summary TEXT,
    input_data JSONB,
    output_summary TEXT,
    output_data JSONB,
    status TEXT DEFAULT 'in_progress',
    skipped BOOLEAN DEFAULT false,
    skip_reason TEXT,
    error_message TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_stages_trace ON resolution_trace_stages(trace_id, stage_number);

-- LLM calls
CREATE TABLE resolution_llm_calls (
    id BIGSERIAL PRIMARY KEY,
    trace_id TEXT NOT NULL REFERENCES resolution_traces(id) ON DELETE CASCADE,
    stage_id BIGINT REFERENCES resolution_trace_stages(id) ON DELETE CASCADE,
    model TEXT NOT NULL,
    prompt_template TEXT,
    prompt_text TEXT,
    prompt_tokens INT,
    response_text TEXT,
    response_tokens INT,
    parsed_output JSONB,
    parse_errors TEXT[],
    latency_ms INT,
    attempt_number INT DEFAULT 1,
    is_fallback BOOLEAN DEFAULT false,
    fallback_reason TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_llm_calls_trace ON resolution_llm_calls(trace_id);
CREATE INDEX idx_llm_calls_stage ON resolution_llm_calls(stage_id);

-- Decisions
CREATE TABLE resolution_decisions (
    id BIGSERIAL PRIMARY KEY,
    trace_id TEXT NOT NULL REFERENCES resolution_traces(id) ON DELETE CASCADE,
    stage_id BIGINT REFERENCES resolution_trace_stages(id) ON DELETE CASCADE,
    decision_type TEXT NOT NULL,
    mention_id BIGINT,
    mentioned_text TEXT,
    chosen_option TEXT,
    alternatives JSONB,
    confidence DECIMAL(3,2),
    reasoning TEXT,
    factors JSONB,
    was_correct BOOLEAN,
    correction_notes TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_decisions_trace ON resolution_decisions(trace_id);
CREATE INDEX idx_decisions_mention ON resolution_decisions(mention_id);
CREATE INDEX idx_decisions_corrections ON resolution_decisions(trace_id)
    WHERE was_correct = false;
```

### 018_resolution_comparisons.sql

```sql
-- Comparisons
CREATE TABLE resolution_comparisons (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    content_id BIGINT NOT NULL,
    content_type TEXT,
    content_summary TEXT,
    models TEXT[] NOT NULL,
    trace_ids TEXT[] NOT NULL,
    initiated_by TEXT,
    purpose TEXT,
    started_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,
    total_decisions INT,
    unanimous_decisions INT,
    divergent_decisions INT,
    divergence_summary JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_comparisons_content ON resolution_comparisons(content_id);
CREATE INDEX idx_comparisons_tenant ON resolution_comparisons(tenant_id, created_at DESC);

-- Comparison decisions
CREATE TABLE resolution_comparison_decisions (
    id BIGSERIAL PRIMARY KEY,
    comparison_id TEXT NOT NULL REFERENCES resolution_comparisons(id) ON DELETE CASCADE,
    mentioned_text TEXT NOT NULL,
    mention_index INT,
    model_decisions JSONB NOT NULL,
    is_unanimous BOOLEAN,
    divergence_type TEXT,
    confidence_spread DECIMAL(3,2),
    ground_truth_entity_id BIGINT,
    models_correct TEXT[],
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_comp_decisions_comparison ON resolution_comparison_decisions(comparison_id);
CREATE INDEX idx_comp_decisions_divergent ON resolution_comparison_decisions(comparison_id)
    WHERE is_unanimous = false;
```

## Data Retention

Automated cleanup (daily scheduled job):

```sql
-- Full traces: 90 days
DELETE FROM resolution_llm_calls WHERE created_at < NOW() - INTERVAL '90 days';
DELETE FROM resolution_trace_stages WHERE created_at < NOW() - INTERVAL '90 days';
DELETE FROM resolution_traces WHERE created_at < NOW() - INTERVAL '90 days';

-- Decisions: 1 year
DELETE FROM resolution_decisions WHERE created_at < NOW() - INTERVAL '1 year';

-- Comparisons: 1 year
DELETE FROM resolution_comparison_decisions WHERE created_at < NOW() - INTERVAL '1 year';
DELETE FROM resolution_comparisons WHERE created_at < NOW() - INTERVAL '1 year';
```

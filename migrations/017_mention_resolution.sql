-- =====================================================
-- Unified Mention Resolution Tables Migration
-- Created: 2026-01-21
-- Description: Tables for unified mention resolution across all entity types
-- Specification: specs/013-content-enrichment/mention-resolution.md
-- Bead: pe-gp11, pe-0w39
-- =====================================================

-- Enum for entity types in mentions
DO $$ BEGIN
    CREATE TYPE mention_entity_type AS ENUM ('person', 'term', 'product', 'company', 'project');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Enum for resolution status
DO $$ BEGIN
    CREATE TYPE mention_status AS ENUM ('pending', 'auto_resolved', 'user_resolved', 'dismissed');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Enum for trace status
DO $$ BEGIN
    CREATE TYPE trace_status AS ENUM ('in_progress', 'completed', 'failed');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Enum for trace level
DO $$ BEGIN
    CREATE TYPE trace_level AS ENUM ('minimal', 'standard', 'full', 'debug');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Enum for stage status
DO $$ BEGIN
    CREATE TYPE stage_status AS ENUM ('in_progress', 'completed', 'failed', 'skipped');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Enum for decision type
DO $$ BEGIN
    CREATE TYPE decision_type AS ENUM ('resolve', 'queue_review', 'suggest_new_entity', 'skip_verification');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- =====================================================
-- Content Mentions Table
-- =====================================================
-- Unified table for all mentions extracted from content

CREATE TABLE IF NOT EXISTS content_mentions (
    id BIGSERIAL PRIMARY KEY,
    tenant_id VARCHAR(255) NOT NULL,
    content_id BIGINT NOT NULL,

    -- What was mentioned
    entity_type mention_entity_type NOT NULL,
    mentioned_text VARCHAR(500) NOT NULL,
    position INT,                              -- Character offset in content
    context_snippet TEXT,                      -- Surrounding text for disambiguation

    -- Resolution
    resolved_entity_id BIGINT,                 -- FK to appropriate table based on entity_type
    resolution_confidence REAL CHECK (resolution_confidence IS NULL OR (resolution_confidence >= 0.0 AND resolution_confidence <= 1.0)),
    resolution_source VARCHAR(100),            -- exact_match, alias, fuzzy, project_context, prior_link, user_confirmed

    -- For terms: also track expansion used
    resolved_expansion VARCHAR(500),           -- "Linode Kubernetes Engine" (for term type)

    -- Candidates at extraction time (for review UI)
    candidates JSONB DEFAULT '[]'::jsonb,      -- [{entity_id, name, confidence, reasons}]

    -- Review status
    status mention_status NOT NULL DEFAULT 'pending',
    resolved_at TIMESTAMPTZ,
    resolved_by VARCHAR(255),

    -- Project context at mention time
    project_context_id BIGINT,

    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,

    -- Constraints
    CONSTRAINT content_mentions_text_not_empty CHECK (length(trim(mentioned_text)) > 0)
);

-- Indexes for content_mentions
CREATE INDEX IF NOT EXISTS idx_content_mentions_content
    ON content_mentions (content_id);

CREATE INDEX IF NOT EXISTS idx_content_mentions_pending
    ON content_mentions (tenant_id, status)
    WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS idx_content_mentions_entity
    ON content_mentions (tenant_id, entity_type, resolved_entity_id)
    WHERE resolved_entity_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_content_mentions_text
    ON content_mentions (tenant_id, entity_type, LOWER(mentioned_text));

CREATE INDEX IF NOT EXISTS idx_content_mentions_project_context
    ON content_mentions (tenant_id, project_context_id)
    WHERE project_context_id IS NOT NULL;

-- =====================================================
-- Mention Patterns Table
-- =====================================================
-- Tracks resolution patterns for suggestions (soft links + history)

CREATE TABLE IF NOT EXISTS mention_patterns (
    id BIGSERIAL PRIMARY KEY,
    tenant_id VARCHAR(255) NOT NULL,

    -- Pattern definition
    entity_type mention_entity_type NOT NULL,
    pattern_text VARCHAR(500) NOT NULL,        -- "Alan", "OKEE", "the database"

    -- Resolution target
    resolved_entity_id BIGINT,                 -- NULL if pattern exists but unresolved
    resolved_expansion VARCHAR(500),           -- For terms without entity link

    -- Scope
    project_id BIGINT,                         -- NULL = global pattern
    is_permanent BOOLEAN DEFAULT FALSE,        -- true = glossary/entity alias, false = soft pattern

    -- Usage tracking
    times_seen INT DEFAULT 1,
    times_linked INT DEFAULT 0,
    last_seen_at TIMESTAMPTZ DEFAULT NOW(),
    last_linked_at TIMESTAMPTZ,

    -- Source tracking
    first_content_id BIGINT,                   -- Where pattern was first seen

    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,

    -- Constraints
    CONSTRAINT mention_patterns_text_not_empty CHECK (length(trim(pattern_text)) > 0),
    CONSTRAINT mention_patterns_unique UNIQUE (tenant_id, entity_type, pattern_text, project_id)
);

-- Indexes for mention_patterns
CREATE INDEX IF NOT EXISTS idx_mention_patterns_lookup
    ON mention_patterns (tenant_id, entity_type, LOWER(pattern_text));

CREATE INDEX IF NOT EXISTS idx_mention_patterns_project
    ON mention_patterns (tenant_id, project_id)
    WHERE project_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_mention_patterns_resolved
    ON mention_patterns (tenant_id, entity_type, resolved_entity_id)
    WHERE resolved_entity_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_mention_patterns_frequent
    ON mention_patterns (tenant_id, entity_type, times_linked DESC)
    WHERE times_linked > 0;

-- =====================================================
-- Entity Project Affinity Table
-- =====================================================
-- Tracks entity-project relationships for ranking

CREATE TABLE IF NOT EXISTS entity_project_affinity (
    id BIGSERIAL PRIMARY KEY,
    tenant_id VARCHAR(255) NOT NULL,

    -- Entity reference
    entity_type mention_entity_type NOT NULL,
    entity_id BIGINT NOT NULL,
    project_id BIGINT NOT NULL,

    -- Affinity metrics
    mention_count INT DEFAULT 0,
    last_mentioned_at TIMESTAMPTZ,
    is_member BOOLEAN DEFAULT FALSE,           -- Explicit project member (for persons)
    role VARCHAR(100),                         -- Role in project context

    -- Computed affinity score (0.0 - 1.0)
    affinity_score REAL DEFAULT 0.5 CHECK (affinity_score >= 0.0 AND affinity_score <= 1.0),

    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,

    -- Constraints
    CONSTRAINT entity_project_affinity_unique UNIQUE (tenant_id, entity_type, entity_id, project_id)
);

-- Indexes for entity_project_affinity
CREATE INDEX IF NOT EXISTS idx_entity_affinity_project
    ON entity_project_affinity (tenant_id, project_id);

CREATE INDEX IF NOT EXISTS idx_entity_affinity_entity
    ON entity_project_affinity (tenant_id, entity_type, entity_id);

CREATE INDEX IF NOT EXISTS idx_entity_affinity_high_score
    ON entity_project_affinity (tenant_id, project_id, affinity_score DESC)
    WHERE affinity_score > 0.5;

-- Trigger to update updated_at
CREATE OR REPLACE FUNCTION update_entity_affinity_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_entity_affinity_updated_at ON entity_project_affinity;
CREATE TRIGGER trg_entity_affinity_updated_at
    BEFORE UPDATE ON entity_project_affinity
    FOR EACH ROW
    EXECUTE FUNCTION update_entity_affinity_updated_at();

-- =====================================================
-- Resolution Traces Table
-- =====================================================
-- Top-level trace record for each resolution run

CREATE TABLE IF NOT EXISTS resolution_traces (
    id TEXT PRIMARY KEY,                       -- trace_abc123 format
    tenant_id VARCHAR(255) NOT NULL,

    -- What was processed
    content_id BIGINT NOT NULL,
    content_type VARCHAR(50),                  -- email, meeting, document
    content_summary TEXT,                      -- Brief description

    -- Timing
    started_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,
    duration_ms INT,

    -- Outcome summary
    mentions_found INT,
    auto_resolved INT,
    queued_for_review INT,
    new_entities_suggested INT,

    -- Status
    status trace_status NOT NULL DEFAULT 'in_progress',
    error_message TEXT,

    -- Configuration at time of run
    model_used VARCHAR(100),                   -- mlx-mistral-7b, claude-sonnet
    trace_level trace_level NOT NULL DEFAULT 'standard',
    config_snapshot JSONB,                     -- Thresholds, etc.

    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL
);

-- Indexes for resolution_traces
CREATE INDEX IF NOT EXISTS idx_traces_content
    ON resolution_traces (content_id);

CREATE INDEX IF NOT EXISTS idx_traces_tenant_time
    ON resolution_traces (tenant_id, started_at DESC);

CREATE INDEX IF NOT EXISTS idx_traces_status
    ON resolution_traces (tenant_id, status)
    WHERE status != 'completed';

-- =====================================================
-- Resolution Trace Stages Table
-- =====================================================
-- Individual stage records within a trace

CREATE TABLE IF NOT EXISTS resolution_trace_stages (
    id BIGSERIAL PRIMARY KEY,
    trace_id TEXT NOT NULL REFERENCES resolution_traces(id) ON DELETE CASCADE,

    -- Stage identification
    stage_number INT NOT NULL CHECK (stage_number >= 1 AND stage_number <= 4),
    stage_name VARCHAR(50) NOT NULL,           -- understanding, cross_mention, matching, verification

    -- Timing
    started_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,
    duration_ms INT,

    -- Input snapshot
    input_summary TEXT,                        -- Human-readable: "4 mentions, 12 candidates"
    input_data JSONB,                          -- Full structured input

    -- Output snapshot
    output_summary TEXT,                       -- Human-readable: "3 resolved, 1 uncertain"
    output_data JSONB,                         -- Full structured output

    -- Status
    status stage_status NOT NULL DEFAULT 'in_progress',
    skipped BOOLEAN NOT NULL DEFAULT FALSE,
    skip_reason TEXT,
    error_message TEXT,

    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL
);

-- Indexes for resolution_trace_stages
CREATE INDEX IF NOT EXISTS idx_stages_trace
    ON resolution_trace_stages (trace_id, stage_number);

-- =====================================================
-- Resolution LLM Calls Table
-- =====================================================
-- LLM request/response logs (for full/debug trace levels)

CREATE TABLE IF NOT EXISTS resolution_llm_calls (
    id BIGSERIAL PRIMARY KEY,
    trace_id TEXT NOT NULL REFERENCES resolution_traces(id) ON DELETE CASCADE,
    stage_id BIGINT REFERENCES resolution_trace_stages(id) ON DELETE CASCADE,

    -- Request
    model VARCHAR(100) NOT NULL,               -- mlx-mistral-7b, claude-3-sonnet
    prompt_template VARCHAR(100),              -- Template name used
    prompt_text TEXT,                          -- Full prompt (can be large)
    prompt_tokens INT,                         -- Token count estimate

    -- Response
    response_text TEXT,                        -- Full response (can be large)
    response_tokens INT,                       -- Token count estimate

    -- Structured extraction from response
    parsed_output JSONB,                       -- Structured data extracted
    parse_errors TEXT[],                       -- Any issues parsing response

    -- Performance
    latency_ms INT,

    -- Retry/fallback tracking
    attempt_number INT NOT NULL DEFAULT 1,
    is_fallback BOOLEAN NOT NULL DEFAULT FALSE,
    fallback_reason TEXT,                      -- "local model failed, fell back to Claude"

    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL
);

-- Indexes for resolution_llm_calls
CREATE INDEX IF NOT EXISTS idx_llm_calls_trace
    ON resolution_llm_calls (trace_id);

CREATE INDEX IF NOT EXISTS idx_llm_calls_stage
    ON resolution_llm_calls (stage_id);

-- =====================================================
-- Resolution Decisions Table
-- =====================================================
-- Individual decisions with reasoning (kept for 1 year)

CREATE TABLE IF NOT EXISTS resolution_decisions (
    id BIGSERIAL PRIMARY KEY,
    trace_id TEXT NOT NULL REFERENCES resolution_traces(id) ON DELETE CASCADE,
    stage_id BIGINT REFERENCES resolution_trace_stages(id) ON DELETE CASCADE,

    -- What decision was made
    decision_type decision_type NOT NULL,

    -- Context
    mention_id BIGINT,                         -- FK to content_mentions
    mentioned_text VARCHAR(500),               -- The text being resolved

    -- Decision details
    chosen_option TEXT,                        -- What was decided
    alternatives JSONB,                        -- Other options considered
    confidence REAL CHECK (confidence IS NULL OR (confidence >= 0.0 AND confidence <= 1.0)),

    -- Reasoning
    reasoning TEXT,                            -- LLM's reasoning for this decision
    factors JSONB,                             -- Structured factors that influenced decision

    -- Outcome tracking (filled in later)
    was_correct BOOLEAN,                       -- NULL until human reviews
    correction_notes TEXT,

    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL
);

-- Indexes for resolution_decisions
CREATE INDEX IF NOT EXISTS idx_decisions_trace
    ON resolution_decisions (trace_id);

CREATE INDEX IF NOT EXISTS idx_decisions_mention
    ON resolution_decisions (mention_id);

CREATE INDEX IF NOT EXISTS idx_decisions_corrections
    ON resolution_decisions (trace_id)
    WHERE was_correct = false;

-- =====================================================
-- Comments
-- =====================================================

COMMENT ON TABLE content_mentions IS 'Unified table for all entity mentions extracted from content';
COMMENT ON COLUMN content_mentions.entity_type IS 'Type of entity: person, term, product, company, project';
COMMENT ON COLUMN content_mentions.mentioned_text IS 'The text as it appeared in content';
COMMENT ON COLUMN content_mentions.resolved_entity_id IS 'ID in the appropriate entity table (people, glossary, products, projects)';
COMMENT ON COLUMN content_mentions.resolution_source IS 'How resolution was determined: exact_match, alias, fuzzy, project_context, prior_link, user_confirmed';
COMMENT ON COLUMN content_mentions.candidates IS 'JSON array of resolution candidates with confidence scores';
COMMENT ON COLUMN content_mentions.project_context_id IS 'Project context active when mention was extracted';

COMMENT ON TABLE mention_patterns IS 'Tracks resolution patterns for suggestion and soft links';
COMMENT ON COLUMN mention_patterns.pattern_text IS 'The text pattern (e.g., "Alan", "OKEE")';
COMMENT ON COLUMN mention_patterns.project_id IS 'Project scope for pattern (NULL = global)';
COMMENT ON COLUMN mention_patterns.is_permanent IS 'True if promoted to permanent alias/glossary entry';
COMMENT ON COLUMN mention_patterns.times_linked IS 'Number of times this pattern was linked to resolved_entity_id';

COMMENT ON TABLE entity_project_affinity IS 'Tracks entity relevance within project contexts for ranking';
COMMENT ON COLUMN entity_project_affinity.affinity_score IS 'Computed relevance score 0.0-1.0 for ranking candidates';
COMMENT ON COLUMN entity_project_affinity.is_member IS 'True if entity is explicit project member (for persons)';

COMMENT ON TABLE resolution_traces IS 'Top-level trace for each resolution process run';
COMMENT ON COLUMN resolution_traces.id IS 'Unique trace ID (trace_abc123 format)';
COMMENT ON COLUMN resolution_traces.trace_level IS 'Detail level: minimal, standard, full, debug';
COMMENT ON COLUMN resolution_traces.model_used IS 'LLM model used for resolution (e.g., mlx-mistral-7b)';

COMMENT ON TABLE resolution_trace_stages IS 'Individual stages within a resolution trace (1-4)';
COMMENT ON COLUMN resolution_trace_stages.stage_name IS 'Stage name: understanding, cross_mention, matching, verification';
COMMENT ON COLUMN resolution_trace_stages.input_data IS 'Full structured input to this stage';
COMMENT ON COLUMN resolution_trace_stages.output_data IS 'Full structured output from this stage';

COMMENT ON TABLE resolution_llm_calls IS 'LLM request/response logs for full/debug trace levels';
COMMENT ON COLUMN resolution_llm_calls.prompt_text IS 'Complete prompt sent to LLM (can be large)';
COMMENT ON COLUMN resolution_llm_calls.response_text IS 'Complete response from LLM (can be large)';
COMMENT ON COLUMN resolution_llm_calls.parsed_output IS 'Structured JSON extracted from response';

COMMENT ON TABLE resolution_decisions IS 'Individual resolution decisions with reasoning (1 year retention)';
COMMENT ON COLUMN resolution_decisions.decision_type IS 'Type: resolve, queue_review, suggest_new_entity, skip_verification';
COMMENT ON COLUMN resolution_decisions.reasoning IS 'LLM reasoning for this decision';
COMMENT ON COLUMN resolution_decisions.was_correct IS 'NULL until reviewed; false if human corrected';

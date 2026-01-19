-- =====================================================
-- AI Extraction Tables Migration
-- Created: 2026-01-19
-- Description: Tables for assertions, templates, extraction audit trail, and A/B testing
-- Specification: specs/013-content-enrichment/extraction.md
-- =====================================================

-- =====================================================
-- Assertion Tables
-- =====================================================

-- Enum for assertion types
DO $$ BEGIN
    CREATE TYPE assertion_type AS ENUM (
        'risk', 'action', 'issue', 'decision', 'commitment', 'question'
    );
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Enum for assertion severity
DO $$ BEGIN
    CREATE TYPE assertion_severity AS ENUM ('low', 'medium', 'high', 'critical');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Enum for action status
DO $$ BEGIN
    CREATE TYPE action_status AS ENUM ('open', 'in_progress', 'completed', 'cancelled');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Main assertions table
CREATE TABLE IF NOT EXISTS assertions (
    id BIGSERIAL PRIMARY KEY,
    tenant_id VARCHAR(255) NOT NULL,

    -- Source linkage
    source_id BIGINT NOT NULL, -- Content this was extracted from
    thread_id BIGINT REFERENCES email_threads(id) ON DELETE SET NULL, -- If part of thread
    extraction_run_id BIGINT, -- Link to audit trail (FK added after extraction_runs created)

    -- Assertion classification
    assertion_type assertion_type NOT NULL,
    description TEXT NOT NULL,
    source_quote TEXT, -- Original text that triggered extraction
    confidence REAL DEFAULT 0.8 CHECK (confidence >= 0.0 AND confidence <= 1.0),

    -- Grounded references to entities
    owner_person_id BIGINT REFERENCES people(id) ON DELETE SET NULL, -- Who owns/is responsible
    assignee_person_id BIGINT REFERENCES people(id) ON DELETE SET NULL, -- For actions
    target_person_id BIGINT REFERENCES people(id) ON DELETE SET NULL, -- Who it's directed at
    decision_maker_person_id BIGINT REFERENCES people(id) ON DELETE SET NULL, -- For decisions
    committer_person_id BIGINT REFERENCES people(id) ON DELETE SET NULL, -- For commitments
    committed_to_person_id BIGINT REFERENCES people(id) ON DELETE SET NULL, -- For commitments
    project_id BIGINT REFERENCES projects(id) ON DELETE SET NULL,
    ticket_id BIGINT REFERENCES jira_tickets(id) ON DELETE SET NULL,

    -- Type-specific fields
    severity assertion_severity, -- For risks/issues
    status action_status DEFAULT 'open', -- For actions
    due_date TIMESTAMPTZ, -- For actions/commitments
    due_date_source TEXT, -- Original text ("by Friday")
    rationale TEXT, -- For decisions
    answered BOOLEAN DEFAULT FALSE, -- For questions

    -- Versioning
    is_current BOOLEAN DEFAULT TRUE,
    superseded_by BIGINT REFERENCES assertions(id) ON DELETE SET NULL,

    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,

    -- Constraints
    CONSTRAINT assertions_description_not_empty CHECK (length(trim(description)) > 0)
);

-- Indexes for assertions
CREATE INDEX IF NOT EXISTS idx_assertions_tenant_id ON assertions (tenant_id);
CREATE INDEX IF NOT EXISTS idx_assertions_source_id ON assertions (source_id);
CREATE INDEX IF NOT EXISTS idx_assertions_thread_id ON assertions (thread_id) WHERE thread_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_assertions_type ON assertions (tenant_id, assertion_type);
CREATE INDEX IF NOT EXISTS idx_assertions_project ON assertions (project_id) WHERE project_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_assertions_current ON assertions (tenant_id, is_current) WHERE is_current = TRUE;
CREATE INDEX IF NOT EXISTS idx_assertions_assignee ON assertions (assignee_person_id) WHERE assignee_person_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_assertions_due_date ON assertions (tenant_id, due_date) WHERE due_date IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_assertions_status ON assertions (tenant_id, status) WHERE assertion_type = 'action';

-- Content sentiment table (extracted alongside assertions)
CREATE TABLE IF NOT EXISTS content_sentiment (
    id BIGSERIAL PRIMARY KEY,
    source_id BIGINT NOT NULL,
    extraction_run_id BIGINT, -- FK added after extraction_runs created

    -- Sentiment analysis
    overall_sentiment VARCHAR(20) NOT NULL, -- positive, neutral, negative
    urgency VARCHAR(20), -- low, medium, high, critical
    confidence_in_outcome VARCHAR(20), -- low, medium, high
    tone VARCHAR(50), -- collaborative, confrontational, informational

    -- Topics and entities
    topics JSONB DEFAULT '[]', -- Array of {topic, relevance}
    key_entities JSONB DEFAULT '[]', -- Array of {type, name, context}

    -- Thread contribution (for thread messages)
    advances_discussion BOOLEAN,
    new_information BOOLEAN,
    is_decision_point BOOLEAN,
    contribution_summary TEXT,

    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL
);

-- Indexes for content_sentiment
CREATE INDEX IF NOT EXISTS idx_content_sentiment_source_id ON content_sentiment (source_id);
CREATE INDEX IF NOT EXISTS idx_content_sentiment_sentiment ON content_sentiment (overall_sentiment);
CREATE INDEX IF NOT EXISTS idx_content_sentiment_urgency ON content_sentiment (urgency) WHERE urgency IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_content_sentiment_topics ON content_sentiment USING GIN (topics);

-- =====================================================
-- Prompt Template Tables
-- =====================================================

-- Enum for template types
DO $$ BEGIN
    CREATE TYPE template_type AS ENUM ('system_default', 'tenant_default', 'project');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Prompt templates table
CREATE TABLE IF NOT EXISTS prompt_templates (
    id BIGSERIAL PRIMARY KEY,
    tenant_id VARCHAR(255), -- NULL for system templates

    -- Template identity
    name VARCHAR(255) NOT NULL,
    template_type template_type NOT NULL,
    version VARCHAR(20) NOT NULL DEFAULT '1.0.0', -- Semantic versioning

    -- Content
    template_text TEXT NOT NULL,
    extraction_schema JSONB, -- JSON schema for expected output
    context_config JSONB DEFAULT '{}', -- Config for context injection

    -- Project associations (for project type templates)
    project_ids BIGINT[] DEFAULT '{}',

    -- Status
    active BOOLEAN DEFAULT TRUE,

    -- Audit
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    created_by VARCHAR(255),

    -- Constraints
    CONSTRAINT prompt_templates_name_not_empty CHECK (length(trim(name)) > 0),
    CONSTRAINT prompt_templates_template_not_empty CHECK (length(trim(template_text)) > 0)
);

-- Indexes for prompt_templates
CREATE INDEX IF NOT EXISTS idx_prompt_templates_tenant ON prompt_templates (tenant_id) WHERE tenant_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_prompt_templates_type ON prompt_templates (template_type);
CREATE INDEX IF NOT EXISTS idx_prompt_templates_active ON prompt_templates (tenant_id, active) WHERE active = TRUE;
CREATE INDEX IF NOT EXISTS idx_prompt_templates_projects ON prompt_templates USING GIN (project_ids) WHERE array_length(project_ids, 1) > 0;

-- Unique constraint for system_default (only one)
CREATE UNIQUE INDEX IF NOT EXISTS idx_prompt_templates_system_default
    ON prompt_templates (template_type)
    WHERE template_type = 'system_default' AND active = TRUE;

-- Unique constraint for tenant_default (one per tenant)
CREATE UNIQUE INDEX IF NOT EXISTS idx_prompt_templates_tenant_default
    ON prompt_templates (tenant_id, template_type)
    WHERE template_type = 'tenant_default' AND active = TRUE;

-- =====================================================
-- Extraction Audit Trail Tables
-- =====================================================

-- Extraction runs table (complete audit trail)
CREATE TABLE IF NOT EXISTS extraction_runs (
    id BIGSERIAL PRIMARY KEY,
    tenant_id VARCHAR(255) NOT NULL,

    -- Source linkage
    source_id BIGINT NOT NULL,
    thread_id BIGINT REFERENCES email_threads(id) ON DELETE SET NULL,

    -- Template used
    template_id BIGINT REFERENCES prompt_templates(id) ON DELETE SET NULL,
    template_version VARCHAR(20), -- Snapshot at run time

    -- Model info
    model_id VARCHAR(100) NOT NULL, -- "gpt-4", "claude-3", etc.
    model_version VARCHAR(100), -- Specific version

    -- Context
    context_injected JSONB DEFAULT '{}', -- What context was provided

    -- Request/Response
    full_prompt TEXT NOT NULL, -- Complete prompt sent to LLM
    input_tokens INT,
    output_tokens INT,
    latency_ms INT,
    raw_response TEXT, -- Exact LLM response
    parsed_response JSONB, -- Structured extraction result
    parse_errors TEXT[], -- Any parsing issues

    -- Status
    status VARCHAR(20) NOT NULL DEFAULT 'completed', -- pending, completed, failed

    -- Experiment linkage (if part of A/B test)
    experiment_id BIGINT, -- FK added after experiments table created

    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL
);

-- Indexes for extraction_runs
CREATE INDEX IF NOT EXISTS idx_extraction_runs_tenant ON extraction_runs (tenant_id);
CREATE INDEX IF NOT EXISTS idx_extraction_runs_source ON extraction_runs (source_id);
CREATE INDEX IF NOT EXISTS idx_extraction_runs_thread ON extraction_runs (thread_id) WHERE thread_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_extraction_runs_template ON extraction_runs (template_id);
CREATE INDEX IF NOT EXISTS idx_extraction_runs_model ON extraction_runs (model_id);
CREATE INDEX IF NOT EXISTS idx_extraction_runs_created ON extraction_runs (tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_extraction_runs_experiment ON extraction_runs (experiment_id) WHERE experiment_id IS NOT NULL;

-- Extraction feedback table (user corrections)
CREATE TABLE IF NOT EXISTS extraction_feedback (
    id BIGSERIAL PRIMARY KEY,
    extraction_run_id BIGINT NOT NULL REFERENCES extraction_runs(id) ON DELETE CASCADE,

    -- Feedback details
    feedback_type VARCHAR(50) NOT NULL, -- correction, validation, rating
    field_path TEXT NOT NULL, -- "actions[0].assignee_person_id"
    original_value JSONB,
    corrected_value JSONB,
    rating INT CHECK (rating >= 1 AND rating <= 5), -- For rating feedback

    -- Audit
    feedback_by VARCHAR(255) NOT NULL,
    notes TEXT,

    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL
);

-- Indexes for extraction_feedback
CREATE INDEX IF NOT EXISTS idx_extraction_feedback_run ON extraction_feedback (extraction_run_id);
CREATE INDEX IF NOT EXISTS idx_extraction_feedback_type ON extraction_feedback (feedback_type);
CREATE INDEX IF NOT EXISTS idx_extraction_feedback_by ON extraction_feedback (feedback_by);

-- =====================================================
-- A/B Testing Tables
-- =====================================================

-- Enum for experiment status
DO $$ BEGIN
    CREATE TYPE experiment_status AS ENUM ('draft', 'running', 'completed', 'analyzed', 'cancelled');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Extraction experiments table
CREATE TABLE IF NOT EXISTS extraction_experiments (
    id BIGSERIAL PRIMARY KEY,
    tenant_id VARCHAR(255) NOT NULL,

    -- Experiment identity
    name VARCHAR(255) NOT NULL,
    description TEXT,

    -- Status
    status experiment_status NOT NULL DEFAULT 'draft',

    -- Configuration
    variants JSONB NOT NULL, -- [{name, template_id, model_id, weight}, ...]
    sample_size INT NOT NULL DEFAULT 100, -- Target number of extractions

    -- Progress
    current_count INT DEFAULT 0,

    -- Timestamps
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,

    -- Constraints
    CONSTRAINT extraction_experiments_name_not_empty CHECK (length(trim(name)) > 0),
    CONSTRAINT extraction_experiments_sample_positive CHECK (sample_size > 0)
);

-- Indexes for extraction_experiments
CREATE INDEX IF NOT EXISTS idx_extraction_experiments_tenant ON extraction_experiments (tenant_id);
CREATE INDEX IF NOT EXISTS idx_extraction_experiments_status ON extraction_experiments (tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_extraction_experiments_running ON extraction_experiments (tenant_id) WHERE status = 'running';

-- Experiment results table
CREATE TABLE IF NOT EXISTS extraction_experiment_results (
    id BIGSERIAL PRIMARY KEY,
    experiment_id BIGINT NOT NULL REFERENCES extraction_experiments(id) ON DELETE CASCADE,
    extraction_run_id BIGINT NOT NULL REFERENCES extraction_runs(id) ON DELETE CASCADE,

    -- Variant assignment
    variant_name VARCHAR(100) NOT NULL,

    -- Metrics
    metrics JSONB NOT NULL DEFAULT '{}', -- {latency, tokens, parse_success, etc.}

    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,

    -- Constraints
    CONSTRAINT experiment_results_unique UNIQUE (experiment_id, extraction_run_id)
);

-- Indexes for experiment results
CREATE INDEX IF NOT EXISTS idx_experiment_results_experiment ON extraction_experiment_results (experiment_id);
CREATE INDEX IF NOT EXISTS idx_experiment_results_variant ON extraction_experiment_results (experiment_id, variant_name);
CREATE INDEX IF NOT EXISTS idx_experiment_results_run ON extraction_experiment_results (extraction_run_id);

-- =====================================================
-- Add Foreign Keys
-- =====================================================

-- Add FK from assertions to extraction_runs
ALTER TABLE assertions
    ADD CONSTRAINT fk_assertions_extraction_run
    FOREIGN KEY (extraction_run_id) REFERENCES extraction_runs(id) ON DELETE SET NULL;

-- Add FK from content_sentiment to extraction_runs
ALTER TABLE content_sentiment
    ADD CONSTRAINT fk_content_sentiment_extraction_run
    FOREIGN KEY (extraction_run_id) REFERENCES extraction_runs(id) ON DELETE SET NULL;

-- Add FK from extraction_runs to experiments
ALTER TABLE extraction_runs
    ADD CONSTRAINT fk_extraction_runs_experiment
    FOREIGN KEY (experiment_id) REFERENCES extraction_experiments(id) ON DELETE SET NULL;

-- =====================================================
-- Insert System Default Template
-- =====================================================

INSERT INTO prompt_templates (tenant_id, name, template_type, version, template_text, extraction_schema)
VALUES (
    NULL,
    'system-default',
    'system_default',
    '1.0.0',
    E'You are extracting structured information from a business email.\n\n{{#if context.participants}}\nPARTICIPANTS:\n{{#each context.participants}}\n- {{name}} <{{email}}>{{#if role}} - {{role}}{{/if}}\n{{/each}}\n{{/if}}\n\n{{#if context.project}}\nPROJECT: {{context.project.name}}\n{{#if context.project.goal}}Goal: {{context.project.goal}}{{/if}}\n{{/if}}\n\n{{#if context.thread}}\nTHREAD CONTEXT (message {{context.thread.position}} of {{context.thread.total}}):\n{{#each context.thread.prior_messages}}\n- Msg {{position}}: {{preview}}\n{{/each}}\n{{/if}}\n\nINSTRUCTIONS:\nExtract the following from the email below:\n1. RISKS: Potential problems or uncertainties mentioned\n2. ACTIONS: Tasks to be done, with assignee if mentioned\n3. ISSUES: Current blockers or problems\n4. DECISIONS: Choices that were made\n5. COMMITMENTS: Promises or pledges made\n6. QUESTIONS: Unanswered questions\n\nFor each item, include:\n- A clear description\n- The source quote from the email\n- Any referenced people, dates, or projects\n\nEMAIL:\n{{content}}',
    '{"type": "object", "properties": {"risks": {"type": "array"}, "actions": {"type": "array"}, "issues": {"type": "array"}, "decisions": {"type": "array"}, "commitments": {"type": "array"}, "questions": {"type": "array"}, "sentiment": {"type": "object"}}}'
) ON CONFLICT DO NOTHING;

-- =====================================================
-- Comments
-- =====================================================

COMMENT ON TABLE assertions IS 'Extracted assertions (RAID+) from content, grounded to entities';
COMMENT ON COLUMN assertions.source_quote IS 'Original text that triggered this extraction';
COMMENT ON COLUMN assertions.is_current IS 'True if this is the latest version of this assertion';
COMMENT ON COLUMN assertions.superseded_by IS 'Points to newer assertion that replaced this one';

COMMENT ON TABLE content_sentiment IS 'Sentiment analysis extracted alongside assertions';
COMMENT ON COLUMN content_sentiment.contribution_summary IS 'How this message advances the thread discussion';

COMMENT ON TABLE prompt_templates IS 'Prompt templates for AI extraction with fallback chain';
COMMENT ON COLUMN prompt_templates.template_type IS 'system_default (global), tenant_default (per-tenant), project (specific projects)';
COMMENT ON COLUMN prompt_templates.extraction_schema IS 'JSON schema defining expected extraction output structure';

COMMENT ON TABLE extraction_runs IS 'Complete audit trail for AI extractions';
COMMENT ON COLUMN extraction_runs.context_injected IS 'Structured context provided to the LLM';
COMMENT ON COLUMN extraction_runs.full_prompt IS 'Complete prompt sent to LLM (for replay/debugging)';

COMMENT ON TABLE extraction_feedback IS 'User corrections and ratings on extraction results';
COMMENT ON COLUMN extraction_feedback.field_path IS 'JSONPath to the corrected field, e.g., actions[0].assignee';

COMMENT ON TABLE extraction_experiments IS 'A/B test configurations for extraction quality comparison';
COMMENT ON COLUMN extraction_experiments.variants IS 'Array of {name, template_id, model_id, weight} objects';

COMMENT ON TABLE extraction_experiment_results IS 'Per-extraction results for A/B test analysis';

-- Migration: 029_pipeline_registry
-- Description: Create pipeline stage registry, recreate prompt_templates for pipeline-based design, and add pipeline_runs provenance
-- Author: Claude Code (data-dev agent)
-- Date: 2026-02-04

-- UP: Create pipeline infrastructure
BEGIN;

-- Create pipeline_stages registry table
CREATE TABLE IF NOT EXISTS pipeline_stages (
    stage TEXT PRIMARY KEY,          -- 'parse', 'segment', 'triage', 'extract_ner', 'extract_semantic', 'resolve', 'analyze', 'persist', 'embed'
    display_name TEXT NOT NULL,      -- 'Stage 2b: Semantic Extraction'
    description TEXT NOT NULL,       -- what this stage does, in plain language
    stage_type TEXT NOT NULL,        -- 'code_only', 'slm', 'llm', 'embedding'
    model_dependent BOOLEAN NOT NULL,-- true if output changes when model/prompt changes
    has_prompt BOOLEAN NOT NULL,     -- true if this stage uses a prompt template
    depends_on TEXT[],               -- stages that must complete before this one
    downstream TEXT[],               -- stages that consume this stage's output
    updated_at TIMESTAMPTZ DEFAULT now()
);

-- Seed data for pipeline stages
INSERT INTO pipeline_stages (stage, display_name, description, stage_type, model_dependent, has_prompt, depends_on, downstream) VALUES
('parse', 'Stage 0: Parse', 'Parse raw content and extract structured metadata (headers, body, attachments) using code-only parsing. No model dependency.', 'code_only', false, false, '{}', '{segment,triage}'),
('segment', 'Stage 0.5: Topic Segmentation', 'Segment long content into coherent topic chunks using SLM. Each chunk is processed independently in downstream stages.', 'slm', true, true, '{parse}', '{triage}'),
('triage', 'Stage 1: Triage', 'Classify content into categories (PROJECT_UPDATE, CUSTOMER, RISK_ISSUE, etc.) using SLM. Determines which content needs deep extraction.', 'slm', true, true, '{parse}', '{extract_ner,extract_semantic}'),
('extract_ner', 'Stage 2a: NER Extraction', 'Extract named entities (people, projects, products, dates) using SLM. Output is preliminary and refined in Stage 3.', 'slm', true, true, '{triage}', '{resolve}'),
('extract_semantic', 'Stage 2b: Semantic Extraction', 'Extract action items, decisions, and risks using SLM. Runs only on content that passed triage. Output is preliminary and verified in Stage 4.', 'slm', true, true, '{triage}', '{resolve}'),
('resolve', 'Stage 3: Entity Resolution', 'Resolve extracted entity names to canonical database records (people, projects, products) using code-only matching. No model dependency.', 'code_only', false, false, '{extract_ner,extract_semantic}', '{analyze}'),
('analyze', 'Stage 4: Deep Analysis', 'Verify and refine high-importance extractions using LLM. Checks consistency, fills missing fields, and improves quality.', 'llm', true, true, '{resolve}', '{persist}'),
('persist', 'Stage 4.5: Persist Results', 'Write final assertions and entities to the database using code-only persistence. No model dependency.', 'code_only', false, false, '{analyze}', '{embed}'),
('embed', 'Stage 5: Embed and Index', 'Generate vector embeddings and update search indexes using embedding model. Final stage in the pipeline.', 'embedding', true, false, '{persist}', '{}')
ON CONFLICT (stage) DO NOTHING;

-- Drop old prompt_templates dependencies before recreating the table
-- This is a dev system, so we can drop and recreate with the new schema

-- First, drop the FK constraint from extraction_runs to prompt_templates
ALTER TABLE extraction_runs DROP CONSTRAINT IF EXISTS fk_extraction_runs_template;
ALTER TABLE extraction_runs DROP CONSTRAINT IF EXISTS extraction_runs_template_id_fkey;

-- Drop the unique indexes on prompt_templates
DROP INDEX IF EXISTS idx_prompt_templates_system_default;
DROP INDEX IF EXISTS idx_prompt_templates_tenant_default;

-- Drop other indexes on prompt_templates
DROP INDEX IF EXISTS idx_prompt_templates_projects;
DROP INDEX IF EXISTS idx_prompt_templates_active;
DROP INDEX IF EXISTS idx_prompt_templates_type;
DROP INDEX IF EXISTS idx_prompt_templates_tenant;

-- Drop the template_type enum (no longer used)
DROP TYPE IF EXISTS template_type CASCADE;

-- Drop the old prompt_templates table
DROP TABLE IF EXISTS prompt_templates;

-- Create new prompt_templates table with pipeline-based schema
CREATE TABLE IF NOT EXISTS prompt_templates (
    id BIGSERIAL PRIMARY KEY,
    stage TEXT NOT NULL REFERENCES pipeline_stages(stage),
    version INT NOT NULL,
    content TEXT NOT NULL,            -- the prompt template, with {placeholders}
    description TEXT,                 -- what changed in this version and why
    is_active BOOLEAN DEFAULT false,
    created_by TEXT,                  -- 'agent-mycroft', 'james'
    created_at TIMESTAMPTZ DEFAULT now(),
    UNIQUE(stage, version)
);

-- Create index on prompt_templates
CREATE INDEX IF NOT EXISTS idx_prompt_templates_stage ON prompt_templates(stage);
CREATE INDEX IF NOT EXISTS idx_prompt_templates_active ON prompt_templates(stage, is_active) WHERE is_active = true;

-- Create pipeline_runs table for provenance tracking
CREATE TABLE IF NOT EXISTS pipeline_runs (
    id BIGSERIAL PRIMARY KEY,
    source_id BIGINT NOT NULL,       -- which content item
    stage TEXT NOT NULL REFERENCES pipeline_stages(stage),
    model_id TEXT,                    -- 'qwen2.5-7b', 'gemini-2.0-flash', null for code-only
    prompt_version INT,              -- which prompt_templates.version was used (null for no-prompt stages)
    config_hash TEXT,                -- hash of relevant config (glossary version, entity list, etc.)
    status TEXT NOT NULL,            -- 'completed', 'failed', 'superseded'
    created_at TIMESTAMPTZ DEFAULT now(),
    duration_ms INT
);

-- Indexes for pipeline_runs
CREATE INDEX IF NOT EXISTS idx_pipeline_runs_source ON pipeline_runs(source_id);
CREATE INDEX IF NOT EXISTS idx_pipeline_runs_stage ON pipeline_runs(stage);
CREATE INDEX IF NOT EXISTS idx_pipeline_runs_status ON pipeline_runs(status);

COMMIT;

-- Comments for documentation
COMMENT ON TABLE pipeline_stages IS 'Registry of pipeline stages with dependencies and metadata';
COMMENT ON COLUMN pipeline_stages.stage IS 'Unique stage identifier (parse, segment, triage, extract_ner, extract_semantic, resolve, analyze, persist, embed)';
COMMENT ON COLUMN pipeline_stages.stage_type IS 'Stage type: code_only, slm, llm, embedding';
COMMENT ON COLUMN pipeline_stages.model_dependent IS 'True if output changes when model/prompt changes';
COMMENT ON COLUMN pipeline_stages.has_prompt IS 'True if this stage uses a prompt template';
COMMENT ON COLUMN pipeline_stages.depends_on IS 'Array of stage names that must complete before this stage';
COMMENT ON COLUMN pipeline_stages.downstream IS 'Array of stage names that consume this stage output';

COMMENT ON TABLE prompt_templates IS 'Versioned prompt templates for pipeline stages';
COMMENT ON COLUMN prompt_templates.stage IS 'Pipeline stage this prompt is for (FK to pipeline_stages)';
COMMENT ON COLUMN prompt_templates.version IS 'Version number (increments for each change)';
COMMENT ON COLUMN prompt_templates.content IS 'Prompt template text with {placeholders}';
COMMENT ON COLUMN prompt_templates.is_active IS 'True if this is the active version for this stage';

COMMENT ON TABLE pipeline_runs IS 'Provenance tracking for pipeline executions';
COMMENT ON COLUMN pipeline_runs.source_id IS 'Content item that was processed';
COMMENT ON COLUMN pipeline_runs.stage IS 'Pipeline stage that ran';
COMMENT ON COLUMN pipeline_runs.model_id IS 'Model used (null for code-only stages)';
COMMENT ON COLUMN pipeline_runs.prompt_version IS 'Prompt template version used (null for no-prompt stages)';
COMMENT ON COLUMN pipeline_runs.config_hash IS 'Hash of relevant config (glossary, entity list, etc.)';
COMMENT ON COLUMN pipeline_runs.status IS 'Run status: completed, failed, superseded';

-- DOWN: Rollback all changes
-- BEGIN;
-- DROP INDEX IF EXISTS idx_pipeline_runs_status;
-- DROP INDEX IF EXISTS idx_pipeline_runs_stage;
-- DROP INDEX IF EXISTS idx_pipeline_runs_source;
-- DROP TABLE IF EXISTS pipeline_runs;
-- DROP INDEX IF EXISTS idx_prompt_templates_active;
-- DROP INDEX IF EXISTS idx_prompt_templates_stage;
-- DROP TABLE IF EXISTS prompt_templates;
-- DROP TABLE IF EXISTS pipeline_stages;
-- COMMIT;

-- +goose Up

-- Migration 159: Tenant context store — personal reference context with LLM-assisted triggers
-- Parent: pf-e8b173
-- Task: pf-53faa5

-- 1. Tenant context entries
CREATE TABLE IF NOT EXISTS tenant_context (
    id SERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    category VARCHAR(50) NOT NULL,
    label VARCHAR(200) NOT NULL,
    details JSONB NOT NULL DEFAULT '{}',
    always_inject BOOLEAN NOT NULL DEFAULT false,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, category, label)
);

-- 2. Trigger conditions for tenant context entries
CREATE TABLE IF NOT EXISTS tenant_context_conditions (
    id SERIAL PRIMARY KEY,
    context_id INTEGER NOT NULL REFERENCES tenant_context(id) ON DELETE CASCADE,
    field VARCHAR(100) NOT NULL,
    match_type VARCHAR(20) NOT NULL,
    value VARCHAR(255) NOT NULL,
    case_sensitive BOOLEAN NOT NULL DEFAULT false
);

CREATE INDEX IF NOT EXISTS idx_tenant_context_tenant ON tenant_context(tenant_id) WHERE active = true;
CREATE INDEX IF NOT EXISTS idx_tenant_context_conditions_ctx ON tenant_context_conditions(context_id);

-- 3. Register context_trigger_suggest stage (used for LLM-assisted trigger generation, not a pipeline stage)
INSERT INTO pipeline_stages (stage, display_name, description, stage_type, model_dependent, has_prompt, depends_on, downstream)
VALUES (
    'context_trigger_suggest',
    'Context Trigger Suggest',
    'LLM-assisted trigger condition generation for tenant context entries. Called at context entry creation time, not per-email.',
    'llm',
    true,
    true,
    ARRAY[]::TEXT[],
    ARRAY[]::TEXT[]
) ON CONFLICT (stage) DO NOTHING;

-- 4. Seed prompt template for LLM-assisted trigger generation
INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES (
    'context_trigger_suggest',
    1,
    'Given this personal context entry ({category}: {label}, details: {details}), suggest trigger conditions for detecting relevant emails. Consider likely sender domains, subject keywords, and content patterns. Return as a JSON array of objects with fields: field (one of: content, subject, sender_email, to_address), match_type (one of: contains, prefix, suffix, glob, exact), value (the match string). Aim for 5-10 diverse conditions covering content keywords, sender domains, and subject patterns.',
    'Initial prompt for LLM-assisted trigger condition generation for tenant context entries',
    true,
    'agent-mycroft'
) ON CONFLICT (stage, version) DO NOTHING;

-- +goose Down

DELETE FROM prompt_templates WHERE stage = 'context_trigger_suggest' AND version = 1;
DELETE FROM pipeline_stages WHERE stage = 'context_trigger_suggest';
DROP TABLE IF EXISTS tenant_context_conditions;
DROP TABLE IF EXISTS tenant_context;

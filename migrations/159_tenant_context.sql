-- 159_tenant_context.sql
-- Creates tenant_context and tenant_context_conditions tables for personal reference context.
-- Also seeds the context_trigger_suggest prompt template.

CREATE TABLE tenant_context (
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

CREATE TABLE tenant_context_conditions (
    id SERIAL PRIMARY KEY,
    context_id INTEGER NOT NULL REFERENCES tenant_context(id) ON DELETE CASCADE,
    field VARCHAR(100) NOT NULL,
    match_type VARCHAR(20) NOT NULL,
    value VARCHAR(255) NOT NULL,
    case_sensitive BOOLEAN NOT NULL DEFAULT false
);

CREATE INDEX idx_tenant_context_tenant ON tenant_context(tenant_id) WHERE active = true;
CREATE INDEX idx_tenant_context_conditions_ctx ON tenant_context_conditions(context_id);

-- Register the context_trigger_suggest stage in pipeline_stages.
INSERT INTO pipeline_stages (stage, display_name, description, stage_type, model_dependent, has_prompt, depends_on, downstream)
VALUES (
    'context_trigger_suggest',
    'Context Trigger Suggest',
    'LLM-assisted generation of trigger conditions for a personal context entry.',
    'llm',
    true,
    true,
    '{}',
    '{}'
)
ON CONFLICT (stage) DO NOTHING;

-- Seed the prompt template for LLM-assisted trigger generation.
-- Stage: context_trigger_suggest
-- Used by SuggestTenantContextTriggers RPC to generate trigger conditions for a new context entry.
INSERT INTO prompt_templates (stage, version, content, is_active, created_by)
VALUES (
    'context_trigger_suggest',
    1,
    'Given this personal context entry, suggest trigger conditions for detecting relevant emails.

Category: {category}
Label: {label}
Details: {details}

Suggest trigger conditions that identify emails likely related to this context entry.
Consider:
- Content keywords (words that would appear in the email body)
- Likely sender email domains or patterns
- Subject line patterns

Return ONLY a JSON array of condition objects. Each object must have exactly these fields:
- "field": one of "content", "sender_email", "subject", "to_address"
- "match_type": one of "contains", "prefix", "suffix", "exact", "glob"
- "value": the value to match (lowercase)

Example output:
[
  {"field": "content", "match_type": "contains", "value": "porsche"},
  {"field": "sender_email", "match_type": "contains", "value": "porsche.com"},
  {"field": "subject", "match_type": "contains", "value": "service due"}
]

Return only the JSON array, no explanation or markdown.',
    true,
    'mycroft'
)
ON CONFLICT (stage, version) DO NOTHING;

-- Tenant context store: personal reference context with LLM-assisted triggers
-- See design pf-e8b173

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

-- Enable tenant_context provider for james-personal pipeline stages that use analyze context.
-- This adds the provider to the analyze and extract_ner stages for the james-personal tenant.
UPDATE pipeline_definitions
SET context_providers = array_append(context_providers, 'tenant_context')
WHERE tenant_id = 'b857823a-362d-469a-bf44-df3f16fbf9c1'
  AND stage IN ('analyze', 'extract_ner')
  AND NOT ('tenant_context' = ANY(context_providers));

-- Prompt template for LLM-assisted trigger generation
INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by)
VALUES (
    'context_trigger_suggest',
    1,
    'You are helping configure a personal context system for email analysis. Given a personal context entry, suggest trigger conditions for detecting relevant emails.

Entry:
- Category: {category}
- Label: {label}
- Details: {details}

Suggest trigger conditions that would identify emails related to this context entry. Consider:
- Content keywords (words likely to appear in relevant emails)
- Sender email patterns (domains or addresses of likely senders)
- Subject line patterns

Return ONLY a JSON array of condition objects with this structure:
[
  {"field": "content", "match_type": "contains", "value": "keyword"},
  {"field": "sender_email", "match_type": "contains", "value": "domain.com"},
  {"field": "subject", "match_type": "contains", "value": "pattern"}
]

Valid field values: content, sender_email, subject, to_address
Valid match_type values: contains, prefix, suffix, exact, glob

Return 5-10 diverse conditions covering different signal types. Return ONLY the JSON array, no other text.',
    'LLM-assisted trigger condition suggestions for tenant context entries',
    true,
    'migration'
)
ON CONFLICT DO NOTHING;

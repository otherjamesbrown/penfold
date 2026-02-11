-- Migration 042: Fix Tenant Configuration Tables
-- Recreates 5 tenant config tables from migration 007 with UUID foreign keys
-- Migration 007 failed silently because it defined BIGINT FKs but tenants.id is UUID

-- =============================================================================
-- DOMAIN CONFIGURATION
-- =============================================================================

-- Tenant domains for internal/external classification
CREATE TABLE IF NOT EXISTS tenant_domains (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    domain VARCHAR(255) NOT NULL,
    domain_type VARCHAR(50) NOT NULL DEFAULT 'external_unknown',
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT valid_domain_type CHECK (domain_type IN ('internal', 'external_known', 'external_unknown')),
    CONSTRAINT unique_tenant_domain UNIQUE (tenant_id, domain)
);

CREATE INDEX IF NOT EXISTS idx_tenant_domains_tenant ON tenant_domains(tenant_id);
CREATE INDEX IF NOT EXISTS idx_tenant_domains_domain ON tenant_domains(domain);

COMMENT ON TABLE tenant_domains IS 'Known domains for tenant with internal/external classification';
COMMENT ON COLUMN tenant_domains.domain_type IS 'internal=company domain, external_known=known partner, external_unknown=unclassified';

-- =============================================================================
-- EMAIL PATTERN CONFIGURATION
-- =============================================================================

-- Email patterns for bot/distribution list detection
CREATE TABLE IF NOT EXISTS tenant_email_patterns (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    pattern VARCHAR(255) NOT NULL,
    pattern_type VARCHAR(50) NOT NULL,
    notes TEXT,
    priority INTEGER NOT NULL DEFAULT 100,
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT valid_pattern_type CHECK (pattern_type IN ('bot', 'distribution_list', 'role_account', 'ignore')),
    CONSTRAINT unique_tenant_pattern UNIQUE (tenant_id, pattern)
);

CREATE INDEX IF NOT EXISTS idx_tenant_email_patterns_tenant ON tenant_email_patterns(tenant_id);
CREATE INDEX IF NOT EXISTS idx_tenant_email_patterns_enabled ON tenant_email_patterns(tenant_id, enabled) WHERE enabled = true;

COMMENT ON TABLE tenant_email_patterns IS 'Glob patterns for detecting bots, distribution lists, role accounts';
COMMENT ON COLUMN tenant_email_patterns.pattern IS 'Glob pattern like *-jira@* or team-*@*';
COMMENT ON COLUMN tenant_email_patterns.priority IS 'Lower number = higher priority (checked first)';

-- =============================================================================
-- INTEGRATION CONFIGURATION
-- =============================================================================

-- External integrations (Jira, Google Workspace, Slack, etc.)
CREATE TABLE IF NOT EXISTS tenant_integrations (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    integration_type VARCHAR(50) NOT NULL,
    name VARCHAR(255) NOT NULL,
    instance_url VARCHAR(500),
    config JSONB DEFAULT '{}',
    credentials_key VARCHAR(255),
    enabled BOOLEAN NOT NULL DEFAULT true,
    last_sync_at TIMESTAMPTZ,
    sync_status VARCHAR(50) DEFAULT 'never_synced',
    sync_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT valid_integration_type CHECK (integration_type IN ('jira', 'google_workspace', 'slack', 'confluence', 'github', 'linear')),
    CONSTRAINT valid_sync_status CHECK (sync_status IN ('healthy', 'error', 'syncing', 'never_synced')),
    CONSTRAINT unique_tenant_integration UNIQUE (tenant_id, integration_type, name)
);

CREATE INDEX IF NOT EXISTS idx_tenant_integrations_tenant ON tenant_integrations(tenant_id);
CREATE INDEX IF NOT EXISTS idx_tenant_integrations_type ON tenant_integrations(integration_type);
CREATE INDEX IF NOT EXISTS idx_tenant_integrations_enabled ON tenant_integrations(tenant_id, enabled) WHERE enabled = true;

COMMENT ON TABLE tenant_integrations IS 'External system integrations per tenant';
COMMENT ON COLUMN tenant_integrations.credentials_key IS 'Reference to secrets store (not actual credentials)';
COMMENT ON COLUMN tenant_integrations.config IS 'Non-sensitive configuration (JSON)';

-- =============================================================================
-- JIRA PROJECT MAPPINGS
-- =============================================================================

-- Map Jira projects to Penfold projects
CREATE TABLE IF NOT EXISTS tenant_jira_mappings (
    id BIGSERIAL PRIMARY KEY,
    integration_id BIGINT NOT NULL REFERENCES tenant_integrations(id) ON DELETE CASCADE,
    jira_project_key VARCHAR(50) NOT NULL,
    penfold_project_id BIGINT REFERENCES projects(id) ON DELETE SET NULL,
    sync_tickets BOOLEAN NOT NULL DEFAULT true,
    sync_comments BOOLEAN NOT NULL DEFAULT false,
    last_sync_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT unique_integration_jira_project UNIQUE (integration_id, jira_project_key)
);

CREATE INDEX IF NOT EXISTS idx_tenant_jira_mappings_integration ON tenant_jira_mappings(integration_id);
CREATE INDEX IF NOT EXISTS idx_tenant_jira_mappings_project ON tenant_jira_mappings(penfold_project_id);

COMMENT ON TABLE tenant_jira_mappings IS 'Mapping between Jira projects and Penfold projects';
COMMENT ON COLUMN tenant_jira_mappings.sync_tickets IS 'Whether to fetch full ticket details from Jira API';

-- =============================================================================
-- PROCESSING RULES
-- =============================================================================

-- Custom processing rules for classification overrides
CREATE TABLE IF NOT EXISTS tenant_processing_rules (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    rule_name VARCHAR(255) NOT NULL,
    priority INTEGER NOT NULL DEFAULT 100,
    match_conditions JSONB NOT NULL,
    classification_override JSONB NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT unique_tenant_rule UNIQUE (tenant_id, rule_name)
);

CREATE INDEX IF NOT EXISTS idx_tenant_processing_rules_tenant ON tenant_processing_rules(tenant_id);
CREATE INDEX IF NOT EXISTS idx_tenant_processing_rules_enabled ON tenant_processing_rules(tenant_id, enabled, priority) WHERE enabled = true;

COMMENT ON TABLE tenant_processing_rules IS 'Custom rules for overriding content classification';
COMMENT ON COLUMN tenant_processing_rules.match_conditions IS 'JSON conditions: {from_contains, subject_contains, has_header, etc.}';
COMMENT ON COLUMN tenant_processing_rules.classification_override IS 'JSON overrides: {content_subtype, processing_profile}';
COMMENT ON COLUMN tenant_processing_rules.priority IS 'Lower number = higher priority (checked first)';

-- =============================================================================
-- SEED DATA
-- =============================================================================

-- Seed Akamai internal domain
INSERT INTO tenant_domains (tenant_id, domain, domain_type)
VALUES ('c3170310-78bd-409c-b186-126f40bfa6ad', 'akamai.com', 'internal')
ON CONFLICT DO NOTHING;

-- =============================================================================
-- HELPER VIEWS
-- =============================================================================

-- Active tenant configuration view
CREATE OR REPLACE VIEW v_tenant_config AS
SELECT
    t.id AS tenant_id,
    t.slug AS tenant_slug,
    t.name AS tenant_name,
    COALESCE(
        (SELECT array_agg(domain) FROM tenant_domains WHERE tenant_id = t.id AND domain_type = 'internal'),
        ARRAY[]::VARCHAR[]
    ) AS internal_domains,
    COALESCE(
        (SELECT array_agg(pattern) FROM tenant_email_patterns WHERE tenant_id = t.id AND pattern_type = 'bot' AND enabled),
        ARRAY[]::VARCHAR[]
    ) AS bot_patterns,
    COALESCE(
        (SELECT array_agg(pattern) FROM tenant_email_patterns WHERE tenant_id = t.id AND pattern_type = 'distribution_list' AND enabled),
        ARRAY[]::VARCHAR[]
    ) AS distribution_patterns,
    COALESCE(
        (SELECT array_agg(pattern) FROM tenant_email_patterns WHERE tenant_id = t.id AND pattern_type = 'role_account' AND enabled),
        ARRAY[]::VARCHAR[]
    ) AS role_account_patterns,
    (SELECT COUNT(*) FROM tenant_integrations WHERE tenant_id = t.id AND enabled) AS active_integrations,
    (SELECT COUNT(*) FROM tenant_processing_rules WHERE tenant_id = t.id AND enabled) AS active_rules
FROM tenants t
WHERE t.is_active = true;

COMMENT ON VIEW v_tenant_config IS 'Aggregated tenant configuration for quick lookups';

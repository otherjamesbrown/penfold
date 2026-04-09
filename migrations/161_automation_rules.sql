-- +goose Up

-- Migration 160: Automation rules — composable trigger → selector → skill → output framework
-- Parent: pf-f397ab
-- Task: pf-5c9a41

-- 1. automation_rules: rule definitions
CREATE TABLE IF NOT EXISTS automation_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    name TEXT NOT NULL,
    description TEXT,
    enabled BOOLEAN NOT NULL DEFAULT true,
    trigger_type TEXT NOT NULL,          -- 'cron', 'event', 'manual'
    trigger_config JSONB NOT NULL,
    selector_config JSONB NOT NULL,
    skill_name TEXT NOT NULL,
    output_config JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by TEXT NOT NULL,
    UNIQUE(tenant_id, name)
);

CREATE INDEX IF NOT EXISTS idx_automation_rules_tenant ON automation_rules(tenant_id) WHERE enabled = true;
CREATE INDEX IF NOT EXISTS idx_automation_rules_trigger ON automation_rules(tenant_id, trigger_type) WHERE enabled = true;

-- 2. automation_rule_executions: execution history
CREATE TABLE IF NOT EXISTS automation_rule_executions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_id UUID NOT NULL REFERENCES automation_rules(id),
    trigger_type TEXT NOT NULL,
    trigger_source TEXT,
    started_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,
    status TEXT NOT NULL,             -- 'running', 'completed', 'failed'
    items_selected INT,
    skill_tokens_used INT,
    delivery_status JSONB,
    chain_executions JSONB,
    error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_rule_executions_rule_id ON automation_rule_executions(rule_id);
CREATE INDEX IF NOT EXISTS idx_rule_executions_started_at ON automation_rule_executions(started_at);

-- 3. Seed operational config entries
INSERT INTO pipeline_operational_config (tenant_id, key, value, description)
SELECT id, 'automation.chain_depth_max', '3', 'Maximum chain depth for automation rule chaining (prevents loops)'
FROM tenants
ON CONFLICT DO NOTHING;

INSERT INTO pipeline_operational_config (tenant_id, key, value, description)
SELECT id, 'automation.default_selector_limit', '50', 'Default item limit for automation rule query selectors'
FROM tenants
ON CONFLICT DO NOTHING;

-- +goose Down

DELETE FROM pipeline_operational_config WHERE key IN (
    'automation.chain_depth_max',
    'automation.default_selector_limit'
);

DROP TABLE IF EXISTS automation_rule_executions;
DROP TABLE IF EXISTS automation_rules;

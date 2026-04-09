-- +goose Up

-- Migration 160: Automation rules — composable trigger → selector → skill → output framework
-- Parent: pf-f397ab
-- Task: pf-5c9a41

-- 1. Automation rules table
CREATE TABLE automation_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    name TEXT NOT NULL,
    description TEXT,
    enabled BOOLEAN NOT NULL DEFAULT true,

    -- Trigger
    trigger_type TEXT NOT NULL,          -- 'cron', 'event', 'manual'
    trigger_config JSONB NOT NULL,       -- type-specific config

    -- Selector
    selector_config JSONB NOT NULL,      -- what data to gather

    -- Skill
    skill_name TEXT NOT NULL,            -- references skills/<name>.md in repo

    -- Output
    output_config JSONB NOT NULL,        -- delivery + optional chaining

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by TEXT NOT NULL,

    UNIQUE(tenant_id, name)
);

CREATE INDEX idx_automation_rules_tenant ON automation_rules(tenant_id);
CREATE INDEX idx_automation_rules_trigger_type ON automation_rules(tenant_id, trigger_type) WHERE enabled = true;

-- 2. Automation rule executions table
CREATE TABLE automation_rule_executions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_id UUID NOT NULL REFERENCES automation_rules(id),
    trigger_type TEXT NOT NULL,
    trigger_source TEXT,              -- source_id for events, 'cron' for scheduled, 'manual' for CLI
    started_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,
    status TEXT NOT NULL,             -- 'running', 'completed', 'failed'
    items_selected INT,
    skill_tokens_used INT,
    delivery_status JSONB,            -- per-channel status
    chain_executions JSONB,           -- results of chained rules
    error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_rule_executions_rule_id ON automation_rule_executions(rule_id);
CREATE INDEX idx_rule_executions_started_at ON automation_rule_executions(started_at);

-- +goose Down

DROP INDEX IF EXISTS idx_rule_executions_started_at;
DROP INDEX IF EXISTS idx_rule_executions_rule_id;
DROP TABLE IF EXISTS automation_rule_executions;
DROP INDEX IF EXISTS idx_automation_rules_trigger_type;
DROP INDEX IF EXISTS idx_automation_rules_tenant;
DROP TABLE IF EXISTS automation_rules;

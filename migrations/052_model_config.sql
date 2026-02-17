-- 052_model_config.sql
-- Creates model_config table for dynamic AI model configuration management

BEGIN;

-- Create model_config table
CREATE TABLE IF NOT EXISTS model_config (
    id SERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    config_key VARCHAR(255) NOT NULL,
    model_name VARCHAR(255) NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_by VARCHAR(255) DEFAULT 'system',
    UNIQUE(tenant_id, config_key)
);

COMMENT ON TABLE model_config IS 'Runtime configuration for AI model selection per tenant';
COMMENT ON COLUMN model_config.config_key IS 'Configuration key (e.g., stage.triage, default.llm, default.embedding)';
COMMENT ON COLUMN model_config.model_name IS 'Model name to use for this configuration';
COMMENT ON COLUMN model_config.updated_at IS 'Timestamp of last update';
COMMENT ON COLUMN model_config.updated_by IS 'User or service that last updated this config';

-- Create index for tenant lookups
CREATE INDEX idx_model_config_tenant ON model_config(tenant_id);

COMMIT;

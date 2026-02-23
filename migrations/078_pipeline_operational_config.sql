-- Migration 078: Create pipeline_operational_config and drop pipeline_config.
-- Replaces pipeline_config with a cleaner tenant-scoped table.
-- Drops unused columns: value_type, min_value, max_value, default_value, updated_by.
-- Author: agent-mycroft
-- Date: 2026-02-23
-- Shard: pf-135e14

BEGIN;

CREATE TABLE pipeline_operational_config (
    tenant_id   UUID NOT NULL REFERENCES tenants(id),
    key         TEXT NOT NULL,
    value       TEXT NOT NULL,
    description TEXT,
    updated_at  TIMESTAMPTZ DEFAULT now(),
    PRIMARY KEY (tenant_id, key)
);

COMMENT ON TABLE pipeline_operational_config IS 'Operational config knobs per tenant. Replaces pipeline_config.';
COMMENT ON COLUMN pipeline_operational_config.tenant_id IS 'Tenant this config row belongs to.';
COMMENT ON COLUMN pipeline_operational_config.key IS 'Config key (e.g. pipeline.max_concurrent, timeout.ai_client.request).';
COMMENT ON COLUMN pipeline_operational_config.value IS 'Config value as text (durations use Go duration strings, e.g. 120s).';

-- Copy all surviving rows from pipeline_config into the new table.
-- Columns dropped: value_type, min_value, max_value, default_value, updated_by.
INSERT INTO pipeline_operational_config (tenant_id, key, value, description)
SELECT 'c3170310-78bd-409c-b186-126f40bfa6ad'::uuid, key, value, description
FROM pipeline_config;

DROP TABLE pipeline_config;

COMMIT;

-- Rollback (manual):
-- CREATE TABLE pipeline_config ... (restore from migration 035 and subsequent seeds)
-- DROP TABLE pipeline_operational_config;

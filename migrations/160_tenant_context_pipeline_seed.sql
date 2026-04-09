-- +goose Up

-- Migration 160: Seed tenant_context pipeline definitions and operational config defaults
-- Parent: pf-e8b173
-- Task: pf-eb6736

-- Enable tenant_context provider for james-personal pipeline
UPDATE pipeline_definitions
SET context_providers = array_append(context_providers, 'tenant_context')
WHERE tenant_id = 'b857823a-362d-469a-bf44-df3f16fbf9c1'
  AND NOT ('tenant_context' = ANY(context_providers));

-- Seed operational config defaults for the tenant context provider
INSERT INTO pipeline_operational_config (tenant_id, key, value)
VALUES
    ('b857823a-362d-469a-bf44-df3f16fbf9c1', 'context.tenant_context.max_entries_per_email', '10'),
    ('b857823a-362d-469a-bf44-df3f16fbf9c1', 'context.tenant_context.content_scan_length', '5000')
ON CONFLICT (tenant_id, key) DO NOTHING;

-- +goose Down

UPDATE pipeline_definitions
SET context_providers = array_remove(context_providers, 'tenant_context')
WHERE tenant_id = 'b857823a-362d-469a-bf44-df3f16fbf9c1';

DELETE FROM pipeline_operational_config
WHERE tenant_id = 'b857823a-362d-469a-bf44-df3f16fbf9c1'
  AND key IN (
      'context.tenant_context.max_entries_per_email',
      'context.tenant_context.content_scan_length'
  );

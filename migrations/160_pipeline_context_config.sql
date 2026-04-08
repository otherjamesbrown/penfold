-- +goose Up

-- Migration 160: Add tenant_context provider to james-personal pipeline definitions
-- and seed operational config thresholds.

-- Add tenant_context provider to the standard analyze stage for james-personal.
-- This enables personal reference context injection during deep email analysis.
UPDATE pipeline_definitions
SET context_providers = array_append(context_providers, 'tenant_context')
WHERE tenant_id = 'b857823a-362d-469a-bf44-df3f16fbf9c1'
  AND stage = 'analyze'
  AND NOT (context_providers @> ARRAY['tenant_context']);

-- Seed operational config thresholds for tenant context provider.
-- These control how many entries are injected and how much content is scanned per email.
INSERT INTO pipeline_operational_config (tenant_id, key, value, description)
VALUES
    (
        'b857823a-362d-469a-bf44-df3f16fbf9c1',
        'context.tenant_context.max_entries_per_email',
        '10',
        'Maximum number of personal context entries injected per email analysis'
    ),
    (
        'b857823a-362d-469a-bf44-df3f16fbf9c1',
        'context.tenant_context.content_scan_length',
        '5000',
        'Maximum characters of email content scanned for trigger condition matching'
    )
ON CONFLICT (tenant_id, key) DO NOTHING;

-- +goose Down

UPDATE pipeline_definitions
SET context_providers = array_remove(context_providers, 'tenant_context')
WHERE tenant_id = 'b857823a-362d-469a-bf44-df3f16fbf9c1'
  AND stage = 'analyze';

DELETE FROM pipeline_operational_config
WHERE tenant_id = 'b857823a-362d-469a-bf44-df3f16fbf9c1'
  AND key IN (
      'context.tenant_context.max_entries_per_email',
      'context.tenant_context.content_scan_length'
  );

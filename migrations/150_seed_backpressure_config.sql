-- Migration 150: Seed queue backpressure configuration defaults
-- Adds queue.backpressure.* keys to pipeline_operational_config for all tenants.
-- These control the dynamic ScheduleToClose timeout in KickProcessing.

INSERT INTO pipeline_operational_config (tenant_id, key, value, description)
SELECT id, 'queue.backpressure.tier1_threshold', '50',
    'Pending queue depth above which tier1 workflow execution timeout applies'
FROM tenants
ON CONFLICT DO NOTHING;

INSERT INTO pipeline_operational_config (tenant_id, key, value, description)
SELECT id, 'queue.backpressure.tier1_timeout_hours', '2',
    'Workflow execution timeout in hours when pending queue exceeds tier1 threshold'
FROM tenants
ON CONFLICT DO NOTHING;

INSERT INTO pipeline_operational_config (tenant_id, key, value, description)
SELECT id, 'queue.backpressure.tier2_threshold', '100',
    'Pending queue depth above which tier2 workflow execution timeout applies'
FROM tenants
ON CONFLICT DO NOTHING;

INSERT INTO pipeline_operational_config (tenant_id, key, value, description)
SELECT id, 'queue.backpressure.tier2_timeout_hours', '4',
    'Workflow execution timeout in hours when pending queue exceeds tier2 threshold'
FROM tenants
ON CONFLICT DO NOTHING;

INSERT INTO pipeline_operational_config (tenant_id, key, value, description)
SELECT id, 'queue.backpressure.default_timeout_hours', '1',
    'Default workflow execution timeout in hours when pending queue is below tier1 threshold'
FROM tenants
ON CONFLICT DO NOTHING;

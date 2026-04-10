-- +goose Up

-- For both james-personal and akamai tenants, add classify_project to stage_config
-- and disable attribute_project (kept for rollback capability).
UPDATE pipeline_definitions
SET stage_config = jsonb_set(
    stage_config,
    '{classify_project}',
    '{"enabled": true, "skip_when_low": false, "order": 47}'::jsonb
)
WHERE tenant_id IN (
    'b857823a-362d-469a-bf44-df3f16fbf9c1'::uuid,  -- james-personal
    (SELECT id FROM tenants WHERE slug = 'akamai')
);

-- Disable attribute_project (keep for rollback)
UPDATE pipeline_definitions
SET stage_config = jsonb_set(
    stage_config,
    '{attribute_project,enabled}',
    'false'::jsonb
)
WHERE tenant_id IN (
    'b857823a-362d-469a-bf44-df3f16fbf9c1'::uuid,
    (SELECT id FROM tenants WHERE slug = 'akamai')
);

-- +goose Down

UPDATE pipeline_definitions
SET stage_config = stage_config - 'classify_project'
WHERE tenant_id IN (
    'b857823a-362d-469a-bf44-df3f16fbf9c1'::uuid,
    (SELECT id FROM tenants WHERE slug = 'akamai')
);

UPDATE pipeline_definitions
SET stage_config = jsonb_set(
    stage_config,
    '{attribute_project,enabled}',
    'true'::jsonb
)
WHERE tenant_id IN (
    'b857823a-362d-469a-bf44-df3f16fbf9c1'::uuid,
    (SELECT id FROM tenants WHERE slug = 'akamai')
);

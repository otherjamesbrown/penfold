-- +goose Up

-- Add classify_project stage rows for both tenants (james-personal and akamai)
-- in both 'standard' and 'transcript' pipelines where attribute_project already runs.
-- classify_project replaces attribute_project at the same stage_order.
-- attribute_project is disabled (kept for rollback).

INSERT INTO pipeline_definitions (tenant_id, pipeline, stage, stage_order, enabled, skip_when_low, optional, content_type, stage_kind)
SELECT tenant_id, pipeline, 'classify_project', stage_order, true, false, true, content_type, stage_kind
FROM pipeline_definitions
WHERE stage = 'attribute_project'
  AND tenant_id IN (
      'b857823a-362d-469a-bf44-df3f16fbf9c1'::uuid,
      (SELECT id FROM tenants WHERE slug = 'akamai')
  )
ON CONFLICT DO NOTHING;

-- Disable attribute_project (keep for rollback)
UPDATE pipeline_definitions
SET enabled = false
WHERE stage = 'attribute_project'
  AND tenant_id IN (
      'b857823a-362d-469a-bf44-df3f16fbf9c1'::uuid,
      (SELECT id FROM tenants WHERE slug = 'akamai')
  );

-- +goose Down

-- Re-enable attribute_project
UPDATE pipeline_definitions
SET enabled = true
WHERE stage = 'attribute_project'
  AND tenant_id IN (
      'b857823a-362d-469a-bf44-df3f16fbf9c1'::uuid,
      (SELECT id FROM tenants WHERE slug = 'akamai')
  );

-- Remove classify_project rows
DELETE FROM pipeline_definitions
WHERE stage = 'classify_project'
  AND tenant_id IN (
      'b857823a-362d-469a-bf44-df3f16fbf9c1'::uuid,
      (SELECT id FROM tenants WHERE slug = 'akamai')
  );

-- +goose Up

-- Migration 150: Seed operational config rows for pipeline concurrency (pf-5feb30)
-- KickNextPending limit: 0 = fill all available slots
INSERT INTO pipeline_operational_config (tenant_id, key, value)
SELECT id, 'pipeline.kick_next_limit', '0'
FROM tenants
ON CONFLICT DO NOTHING;

-- Model concurrency limits (used by Phase 3 semaphores)
INSERT INTO pipeline_operational_config (tenant_id, key, value)
SELECT id, 'model.concurrency.ollama', '3' FROM tenants
ON CONFLICT DO NOTHING;
INSERT INTO pipeline_operational_config (tenant_id, key, value)
SELECT id, 'model.concurrency.gemini', '50' FROM tenants
ON CONFLICT DO NOTHING;
INSERT INTO pipeline_operational_config (tenant_id, key, value)
SELECT id, 'model.concurrency.anthropic', '20' FROM tenants
ON CONFLICT DO NOTHING;
INSERT INTO pipeline_operational_config (tenant_id, key, value)
SELECT id, 'model.concurrency.mlx', '3' FROM tenants
ON CONFLICT DO NOTHING;
INSERT INTO pipeline_operational_config (tenant_id, key, value)
SELECT id, 'model.concurrency.default', '10' FROM tenants
ON CONFLICT DO NOTHING;

-- +goose Down

DELETE FROM pipeline_operational_config WHERE key IN (
  'pipeline.kick_next_limit',
  'model.concurrency.ollama',
  'model.concurrency.gemini',
  'model.concurrency.anthropic',
  'model.concurrency.mlx',
  'model.concurrency.default'
);

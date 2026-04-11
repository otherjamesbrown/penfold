-- +goose Up

-- KB Drift Scan: nightly at 03:00
-- Re-verify all KB article facts against current system state (Layer 3 defence — cp-cb935b).
INSERT INTO schedules (tenant_id, name, description, schedule_type, schedule_expr, workflow_type, workflow_params, overlap_policy, enabled)
SELECT
    t.id,
    'kb-drift-scan',
    'Nightly KB drift detection — re-verify all KB article facts against current system state',
    'cron',
    '0 3 * * *',
    'KBDriftScanWorkflow',
    '{}',
    'skip',
    true
FROM tenants t WHERE t.is_primary = true
ON CONFLICT (tenant_id, name) DO NOTHING;

-- KB Canary: daily at 06:00
-- Run canary questions against KB search to verify end-to-end retrieval quality.
INSERT INTO schedules (tenant_id, name, description, schedule_type, schedule_expr, workflow_type, workflow_params, overlap_policy, enabled)
SELECT
    t.id,
    'kb-canary',
    'Daily KB retrieval quality check — run canary questions against KB search',
    'cron',
    '0 6 * * *',
    'KBCanaryWorkflow',
    '{}',
    'skip',
    true
FROM tenants t WHERE t.is_primary = true
ON CONFLICT (tenant_id, name) DO NOTHING;

-- KB Triage: weekly Monday at 05:00
-- Review gaps shard, propose actions, escalate unresolvable items.
INSERT INTO schedules (tenant_id, name, description, schedule_type, schedule_expr, workflow_type, workflow_params, overlap_policy, enabled)
SELECT
    t.id,
    'kb-triage',
    'Weekly KB gap triage — review gaps shard, propose actions, escalate unresolvable items',
    'cron',
    '0 5 * * MON',
    'KBTriageWorkflow',
    '{}',
    'skip',
    true
FROM tenants t WHERE t.is_primary = true
ON CONFLICT (tenant_id, name) DO NOTHING;

-- +goose Down
DELETE FROM schedules WHERE name IN ('kb-drift-scan', 'kb-canary', 'kb-triage');

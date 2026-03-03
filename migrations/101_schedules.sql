-- Migration 101: Scheduling Infrastructure (Phase 1)
-- Creates the schedules table for DB-driven schedule management.
-- Part of Epic 0: Scheduling Infrastructure (pf-99e0b8).
-- Date: 2026-03-03

BEGIN;

CREATE TABLE IF NOT EXISTS schedules (
    id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    name            TEXT NOT NULL,
    description     TEXT,
    schedule_type   VARCHAR(50) NOT NULL DEFAULT 'cron',
    schedule_expr   TEXT NOT NULL,
    workflow_type   TEXT NOT NULL,
    workflow_params JSONB DEFAULT '{}',
    enabled         BOOLEAN NOT NULL DEFAULT true,
    overlap_policy  VARCHAR(50) NOT NULL DEFAULT 'skip',
    last_run_at     TIMESTAMPTZ,
    next_run_at     TIMESTAMPTZ,
    last_status     VARCHAR(50),
    last_error      TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ,
    UNIQUE(tenant_id, name)
);

CREATE INDEX IF NOT EXISTS idx_schedules_tenant_enabled
    ON schedules(tenant_id, enabled) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_schedules_tenant_type
    ON schedules(tenant_id, schedule_type) WHERE deleted_at IS NULL;

COMMENT ON TABLE schedules IS 'DB-driven schedule definitions. Source of truth — Temporal is the execution engine.';
COMMENT ON COLUMN schedules.schedule_type IS 'VARCHAR not enum — extensible without migration. Values: cron, interval, heartbeat.';
COMMENT ON COLUMN schedules.overlap_policy IS 'Maps to Temporal ScheduleOverlapPolicy: skip, buffer_one, cancel_other, allow_all.';

-- Seed: migrate existing hardcoded schedules
INSERT INTO schedules (tenant_id, name, description, schedule_type, schedule_expr, workflow_type, workflow_params, overlap_policy)
SELECT
    t.id,
    'conversation-stale-check',
    'Daily stale conversation detection — marks conversations inactive after 14 days of no activity',
    'cron',
    '0 3 * * *',
    'ConversationMaintenanceWorkflow',
    jsonb_build_object('tenant_id', t.id::TEXT, 'stale_days', 14, 'limit', 100),
    'skip'
FROM tenants t
ON CONFLICT (tenant_id, name) DO NOTHING;

INSERT INTO schedules (tenant_id, name, description, schedule_type, schedule_expr, workflow_type, workflow_params, overlap_policy)
SELECT
    t.id,
    'ledger-consolidation',
    'Daily session ledger consolidation — merges ledger entries older than 48h with 3+ entries into narratives',
    'cron',
    '0 3 * * *',
    'SessionLedgerConsolidationWorkflow',
    jsonb_build_object('tenant_id', t.id::TEXT, 'cutoff_hours', 48, 'min_entries', 3),
    'skip'
FROM tenants t
ON CONFLICT (tenant_id, name) DO NOTHING;

COMMIT;

-- Rollback:
-- DROP TABLE IF EXISTS schedules;

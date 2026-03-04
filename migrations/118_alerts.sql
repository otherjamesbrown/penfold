-- Migration 118: Alert model for instruction-triggered notifications
-- Alerts are created when watch instruction matches exceed confidence thresholds.

CREATE TABLE IF NOT EXISTS alerts (
    id VARCHAR(20) PRIMARY KEY DEFAULT ('al-' || left(replace(gen_random_uuid()::text, '-', ''), 12)),
    tenant_id UUID NOT NULL,
    project_id BIGINT REFERENCES projects(id),
    instruction_id BIGINT REFERENCES watch_instructions(id),
    severity VARCHAR(20) NOT NULL DEFAULT 'medium',
    title VARCHAR(500) NOT NULL,
    body TEXT,
    source_id BIGINT,
    acknowledged BOOLEAN NOT NULL DEFAULT false,
    acknowledged_at TIMESTAMPTZ,
    acknowledged_by VARCHAR(200),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_alerts_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id)
);

CREATE INDEX IF NOT EXISTS idx_alerts_tenant_project ON alerts(tenant_id, project_id);
CREATE INDEX IF NOT EXISTS idx_alerts_unacked ON alerts(tenant_id, acknowledged) WHERE acknowledged = false;

-- +goose Up
CREATE TABLE schedule_execution_history (
    id            BIGSERIAL PRIMARY KEY,
    schedule_id   UUID NOT NULL REFERENCES schedules(id),
    tenant_id     UUID NOT NULL,
    workflow_run_id TEXT,
    status        VARCHAR(20) NOT NULL DEFAULT 'running',
    started_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at  TIMESTAMPTZ,
    error         TEXT,
    result_metadata JSONB DEFAULT '{}',
    created_at    TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_seh_schedule_id ON schedule_execution_history(schedule_id);
CREATE INDEX idx_seh_tenant_id ON schedule_execution_history(tenant_id);

-- +goose Down
DROP TABLE IF EXISTS schedule_execution_history;

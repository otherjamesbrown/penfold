-- 058_pipeline_batches.sql
-- Add pipeline_batches table for tracking batch processing jobs with progress and status

CREATE TABLE IF NOT EXISTS pipeline_batches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    workflow_id VARCHAR(255) NOT NULL,
    total_items INT NOT NULL,
    completed_items INT NOT NULL DEFAULT 0,
    failed_items INT NOT NULL DEFAULT 0,
    status VARCHAR(50) NOT NULL DEFAULT 'running',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_pipeline_batches_tenant ON pipeline_batches(tenant_id);
CREATE INDEX IF NOT EXISTS idx_pipeline_batches_status ON pipeline_batches(status);

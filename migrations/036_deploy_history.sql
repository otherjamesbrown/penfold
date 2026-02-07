-- 036_deploy_history.sql
-- Creates deploy_history table for tracking service deployments
-- Supports deployment audit trail, rollback context, and change attribution

BEGIN;

CREATE TABLE IF NOT EXISTS deploy_history (
    id SERIAL PRIMARY KEY,
    service_name TEXT NOT NULL,
    commit TEXT NOT NULL,
    previous_commit TEXT,
    version TEXT,
    deployed_at TIMESTAMPTZ DEFAULT now(),
    deployed_by TEXT,
    changes TEXT,
    nomad_job_version INT,
    shard_ids TEXT[]
);

-- Index for efficient queries by service and time
CREATE INDEX IF NOT EXISTS idx_deploy_history_service
    ON deploy_history(service_name, deployed_at DESC);

-- Comments for documentation
COMMENT ON TABLE deploy_history IS 'Tracks deployment history for each Penfold service';
COMMENT ON COLUMN deploy_history.id IS 'Primary key';
COMMENT ON COLUMN deploy_history.service_name IS 'Name of the deployed service (e.g., penfold-gateway, penfold-worker)';
COMMENT ON COLUMN deploy_history.commit IS 'Git commit hash for this deployment';
COMMENT ON COLUMN deploy_history.previous_commit IS 'Git commit hash of the previous deployment';
COMMENT ON COLUMN deploy_history.version IS 'Semantic version or build identifier';
COMMENT ON COLUMN deploy_history.deployed_at IS 'Timestamp when deployment completed';
COMMENT ON COLUMN deploy_history.deployed_by IS 'User or system that triggered deployment';
COMMENT ON COLUMN deploy_history.changes IS 'Human-readable summary of changes in this deployment';
COMMENT ON COLUMN deploy_history.nomad_job_version IS 'Nomad job version number for correlation';
COMMENT ON COLUMN deploy_history.shard_ids IS 'Array of Context-Palace shard IDs associated with this deployment';

COMMIT;

-- Migration 110: Add attribution columns for project attribution pipeline stage
-- Parent: Epic 3 pf-740c68, Phase 2: pf-974c93

-- Add attribution metadata columns to assertions
ALTER TABLE assertions
    ADD COLUMN IF NOT EXISTS attribution_source VARCHAR(30),
    ADD COLUMN IF NOT EXISTS attribution_confidence REAL;

COMMENT ON COLUMN assertions.attribution_source IS 'How the project was attributed: channel_mapping, keyword, participant_affinity, llm';
COMMENT ON COLUMN assertions.attribution_confidence IS 'Confidence score (0.0–1.0) of the project attribution';

-- Add attributed project IDs to sources for content-level attribution
ALTER TABLE sources
    ADD COLUMN IF NOT EXISTS attributed_project_ids BIGINT[] NOT NULL DEFAULT '{}';

CREATE INDEX IF NOT EXISTS idx_sources_attributed_projects
    ON sources USING GIN (attributed_project_ids);

COMMENT ON COLUMN sources.attributed_project_ids IS 'Array of project IDs attributed to this content via the attribution pipeline stage';

-- Seed attribution_min_confidence into pipeline_operational_config for all tenants
INSERT INTO pipeline_operational_config (tenant_id, key, value, description)
SELECT id, 'attribution_min_confidence', '0.7', 'Minimum confidence threshold for project attribution'
FROM tenants
ON CONFLICT (tenant_id, key) DO NOTHING;

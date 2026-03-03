-- Migration 109: Project source mappings
-- Maps external sources (channels, meeting series, email lists, etc.) to projects

CREATE TABLE IF NOT EXISTS project_source_mappings (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   VARCHAR(255) NOT NULL,
    project_id  BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    source_type VARCHAR(50) NOT NULL,
    source_identifier VARCHAR(500) NOT NULL,
    match_type  VARCHAR(20) NOT NULL DEFAULT 'exact',
    confidence  REAL NOT NULL DEFAULT 0.95,
    notes       TEXT,
    enabled     BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, source_type, source_identifier)
);

CREATE INDEX idx_psm_tenant_project ON project_source_mappings (tenant_id, project_id);
CREATE INDEX idx_psm_lookup ON project_source_mappings (tenant_id, source_type, source_identifier) WHERE enabled = true;

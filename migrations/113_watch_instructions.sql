-- Migration 113: Watch instructions tables, pipeline stage, and operational config
-- Parent: Epic 5 pf-bc08ad, Phase 1a: pf-3119f8

-- 1. Watch instructions table
CREATE TABLE IF NOT EXISTS watch_instructions (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    project_id BIGINT REFERENCES projects(id) ON DELETE SET NULL,
    name VARCHAR(255) NOT NULL,
    instruction TEXT NOT NULL,
    priority VARCHAR(20) NOT NULL DEFAULT 'normal',
    enabled BOOLEAN NOT NULL DEFAULT true,
    model_hint VARCHAR(20) NOT NULL DEFAULT 'fast',
    created_by VARCHAR(255),
    version INT NOT NULL DEFAULT 1,
    last_matched_at TIMESTAMPTZ,
    match_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, name)
);

CREATE INDEX IF NOT EXISTS idx_watch_instructions_tenant ON watch_instructions(tenant_id);
CREATE INDEX IF NOT EXISTS idx_watch_instructions_tenant_enabled ON watch_instructions(tenant_id, enabled);
CREATE INDEX IF NOT EXISTS idx_watch_instructions_project ON watch_instructions(tenant_id, project_id) WHERE project_id IS NOT NULL;

-- 2. Instruction matches table
CREATE TABLE IF NOT EXISTS instruction_matches (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    instruction_id BIGINT NOT NULL REFERENCES watch_instructions(id) ON DELETE CASCADE,
    content_id VARCHAR(255) NOT NULL,
    source_id BIGINT NOT NULL,
    confidence REAL NOT NULL DEFAULT 0.0,
    explanation TEXT,
    matched_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_instruction_matches_tenant ON instruction_matches(tenant_id);
CREATE INDEX IF NOT EXISTS idx_instruction_matches_instruction ON instruction_matches(tenant_id, instruction_id);
CREATE INDEX IF NOT EXISTS idx_instruction_matches_source ON instruction_matches(tenant_id, source_id);

-- 3. Constraints
ALTER TABLE watch_instructions ADD CONSTRAINT chk_instruction_priority
    CHECK (priority IN ('critical', 'high', 'normal', 'low'));

ALTER TABLE watch_instructions ADD CONSTRAINT chk_instruction_model_hint
    CHECK (model_hint IN ('fast', 'standard', 'premium'));

-- 4. Register instruction_evaluate pipeline stage (used by Phase 1b)
INSERT INTO pipeline_stages (stage, display_name, description, stage_type, model_dependent, has_prompt, depends_on, downstream)
VALUES (
    'instruction_evaluate',
    'Instruction Evaluation',
    'Evaluates content against tenant watch instructions. Batched — one LLM call per content item evaluates all enabled instructions.',
    'llm', true, true,
    ARRAY['attribute_project'],
    ARRAY['analyze']
) ON CONFLICT (stage) DO NOTHING;

-- 5. Operational config
INSERT INTO pipeline_operational_config (tenant_id, key, value, description)
SELECT id, 'max_instructions_per_evaluation', '30', 'Maximum number of instructions evaluated per content item.'
FROM tenants
ON CONFLICT (tenant_id, key) DO NOTHING;

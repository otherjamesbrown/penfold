-- =====================================================
-- Resolution Comparisons Migration
-- Created: 2026-01-21
-- Description: Tables for model comparison runs
-- Specification: specs/013-content-enrichment/mention-resolution.md
-- Bead: pe-0w39
-- =====================================================

-- =====================================================
-- Resolution Comparisons Table
-- =====================================================
-- Model comparison run records

CREATE TABLE IF NOT EXISTS resolution_comparisons (
    id TEXT PRIMARY KEY,                       -- comp_xyz789 format
    tenant_id VARCHAR(255) NOT NULL,

    -- What was compared
    content_id BIGINT NOT NULL,
    content_type VARCHAR(50),                  -- email, meeting, document
    content_summary TEXT,                      -- Brief description

    -- Models compared
    models TEXT[] NOT NULL,                    -- ['mlx-mistral-7b', 'mlx-llama-3', 'claude-sonnet']
    trace_ids TEXT[] NOT NULL,                 -- Linked traces (one per model)

    -- Comparison metadata
    initiated_by VARCHAR(100),                 -- user, scheduled, ci
    purpose VARCHAR(100),                      -- model_evaluation, regression_test, debugging

    -- Timing
    started_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,

    -- Summary stats
    total_decisions INT,
    unanimous_decisions INT,                   -- All models agreed
    divergent_decisions INT,                   -- Models disagreed

    -- Analysis
    divergence_summary JSONB,                  -- Quick summary of where models differed

    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL
);

-- Indexes for resolution_comparisons
CREATE INDEX IF NOT EXISTS idx_comparisons_content
    ON resolution_comparisons (content_id);

CREATE INDEX IF NOT EXISTS idx_comparisons_tenant
    ON resolution_comparisons (tenant_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_comparisons_purpose
    ON resolution_comparisons (tenant_id, purpose);

-- =====================================================
-- Resolution Comparison Decisions Table
-- =====================================================
-- Per-mention comparison across models

CREATE TABLE IF NOT EXISTS resolution_comparison_decisions (
    id BIGSERIAL PRIMARY KEY,
    comparison_id TEXT NOT NULL REFERENCES resolution_comparisons(id) ON DELETE CASCADE,

    -- What mention
    mentioned_text VARCHAR(500) NOT NULL,
    mention_index INT,                         -- Position in content

    -- Decision by each model (JSONB array, one per model)
    model_decisions JSONB NOT NULL,
    -- Example:
    -- [
    --   {"model": "mistral", "entity_id": 101, "confidence": 0.88, "reasoning": "..."},
    --   {"model": "llama", "entity_id": 101, "confidence": 0.82, "reasoning": "..."},
    --   {"model": "claude", "entity_id": 101, "confidence": 0.94, "reasoning": "..."}
    -- ]

    -- Comparison analysis
    is_unanimous BOOLEAN,                      -- All models chose same entity
    divergence_type VARCHAR(50),               -- NULL, 'different_entity', 'confidence_gap', 'new_vs_existing'
    confidence_spread REAL CHECK (confidence_spread IS NULL OR (confidence_spread >= 0.0 AND confidence_spread <= 1.0)),

    -- If there's a known correct answer
    ground_truth_entity_id BIGINT,
    models_correct TEXT[],                     -- Which models got it right

    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL
);

-- Indexes for resolution_comparison_decisions
CREATE INDEX IF NOT EXISTS idx_comp_decisions_comparison
    ON resolution_comparison_decisions (comparison_id);

CREATE INDEX IF NOT EXISTS idx_comp_decisions_divergent
    ON resolution_comparison_decisions (comparison_id)
    WHERE is_unanimous = false;

CREATE INDEX IF NOT EXISTS idx_comp_decisions_ground_truth
    ON resolution_comparison_decisions (comparison_id)
    WHERE ground_truth_entity_id IS NOT NULL;

-- =====================================================
-- Comments
-- =====================================================

COMMENT ON TABLE resolution_comparisons IS 'Model comparison run records for evaluating multiple LLMs';
COMMENT ON COLUMN resolution_comparisons.id IS 'Unique comparison ID (comp_xyz789 format)';
COMMENT ON COLUMN resolution_comparisons.models IS 'Array of models compared (e.g., mlx-mistral-7b, claude-sonnet)';
COMMENT ON COLUMN resolution_comparisons.trace_ids IS 'Array of trace IDs, one per model in same order';
COMMENT ON COLUMN resolution_comparisons.initiated_by IS 'Who started comparison: user, scheduled, ci';
COMMENT ON COLUMN resolution_comparisons.purpose IS 'Why comparison was run: model_evaluation, regression_test, debugging';
COMMENT ON COLUMN resolution_comparisons.divergence_summary IS 'JSON summary of where and how models differed';

COMMENT ON TABLE resolution_comparison_decisions IS 'Per-mention decision comparison across models';
COMMENT ON COLUMN resolution_comparison_decisions.model_decisions IS 'Array of decisions from each model with confidence and reasoning';
COMMENT ON COLUMN resolution_comparison_decisions.is_unanimous IS 'True if all models resolved to same entity';
COMMENT ON COLUMN resolution_comparison_decisions.divergence_type IS 'Type of divergence: different_entity, confidence_gap, new_vs_existing';
COMMENT ON COLUMN resolution_comparison_decisions.confidence_spread IS 'Max confidence minus min confidence across models';
COMMENT ON COLUMN resolution_comparison_decisions.ground_truth_entity_id IS 'Known correct answer if available';
COMMENT ON COLUMN resolution_comparison_decisions.models_correct IS 'Array of model names that matched ground truth';

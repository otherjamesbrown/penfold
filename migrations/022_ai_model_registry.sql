-- =====================================================
-- AI Model Registry Migration
-- Created: 2026-01-25
-- Bead: pe-61qv - Database migration for ai_models, ai_routing_rules, ai_model_health tables
-- Description: Tables for multi-model architecture supporting local and remote
--              AI model management, task-based routing, and health tracking.
-- =====================================================

-- +goose Up

-- =====================================================
-- AI Models Registry Table
-- =====================================================
-- Central registry of all available AI models (local and remote).
-- Stores model capabilities, costs, and configuration.
-- Model ID format: "provider/model-name" (e.g., "gemini/gemini-2.0-flash")

CREATE TABLE IF NOT EXISTS ai_models (
    id TEXT PRIMARY KEY,                              -- Format: "provider/model-name"
    name TEXT NOT NULL,                               -- Human-readable display name
    provider TEXT NOT NULL,                           -- ollama, gemini, openai, anthropic, mlx
    model_name TEXT NOT NULL,                         -- Provider-specific model identifier
    endpoint TEXT,                                    -- Custom endpoint URL (NULL for default)
    capabilities TEXT[] NOT NULL DEFAULT '{}',        -- Array of capabilities: embedding, summarization, extraction, classification
    context_window INTEGER DEFAULT 4096,              -- Maximum input tokens
    max_output_tokens INTEGER DEFAULT 2048,           -- Maximum output tokens
    input_cost_per_1k DECIMAL(12,8) DEFAULT 0,        -- Cost per 1K input tokens (USD)
    output_cost_per_1k DECIMAL(12,8) DEFAULT 0,       -- Cost per 1K output tokens (USD)
    is_local BOOLEAN DEFAULT false,                   -- True for locally-running models (ollama, mlx)
    is_enabled BOOLEAN DEFAULT true,                  -- Administrative enable/disable
    priority INTEGER DEFAULT 50,                      -- Default priority (lower = preferred)
    metadata JSONB DEFAULT '{}',                      -- Additional provider-specific config
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,

    -- Constraints
    CONSTRAINT ai_models_provider_check CHECK (provider IN ('ollama', 'gemini', 'openai', 'anthropic', 'mlx')),
    CONSTRAINT ai_models_priority_check CHECK (priority >= 0 AND priority <= 100),
    CONSTRAINT ai_models_context_window_check CHECK (context_window > 0),
    CONSTRAINT ai_models_max_output_check CHECK (max_output_tokens > 0)
);

-- Indexes for ai_models
CREATE INDEX IF NOT EXISTS idx_ai_models_provider ON ai_models (provider);
CREATE INDEX IF NOT EXISTS idx_ai_models_is_enabled ON ai_models (is_enabled);
CREATE INDEX IF NOT EXISTS idx_ai_models_is_local ON ai_models (is_local) WHERE is_local = true;
CREATE INDEX IF NOT EXISTS idx_ai_models_capabilities ON ai_models USING GIN (capabilities);

-- =====================================================
-- AI Routing Rules Table
-- =====================================================
-- Task-based routing configuration for intelligent model selection.
-- Maps task types to preferred and fallback model chains.

CREATE TABLE IF NOT EXISTS ai_routing_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL UNIQUE,                        -- Rule identifier (e.g., 'embeddings-local')
    task_type TEXT NOT NULL,                          -- embedding, summarization, extraction, classification
    preferred_models TEXT[] NOT NULL,                 -- Ordered list of preferred model IDs
    fallback_models TEXT[] DEFAULT '{}',              -- Fallback chain when preferred unavailable
    require_local BOOLEAN DEFAULT false,              -- Enforce local-only model selection
    max_cost_per_request DECIMAL(12,8),               -- Cost ceiling per request (NULL = unlimited)
    optimization_mode TEXT DEFAULT 'balanced',        -- latency, quality, cost, balanced
    priority INTEGER DEFAULT 0,                       -- Rule priority (higher = checked first)
    is_enabled BOOLEAN DEFAULT true,                  -- Administrative enable/disable
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,

    -- Constraints
    CONSTRAINT ai_routing_rules_task_type_check CHECK (task_type IN ('embedding', 'summarization', 'extraction', 'classification')),
    CONSTRAINT ai_routing_rules_optimization_check CHECK (optimization_mode IN ('latency', 'quality', 'cost', 'balanced')),
    CONSTRAINT ai_routing_rules_priority_check CHECK (priority >= 0)
);

-- Indexes for ai_routing_rules
CREATE INDEX IF NOT EXISTS idx_ai_routing_rules_task_type ON ai_routing_rules (task_type);
CREATE INDEX IF NOT EXISTS idx_ai_routing_rules_is_enabled ON ai_routing_rules (is_enabled);
CREATE INDEX IF NOT EXISTS idx_ai_routing_rules_priority ON ai_routing_rules (priority DESC);

-- =====================================================
-- AI Model Health Table
-- =====================================================
-- Real-time health tracking for proactive routing decisions.
-- Updated by health check workflows and model requests.

CREATE TABLE IF NOT EXISTS ai_model_health (
    model_id TEXT PRIMARY KEY REFERENCES ai_models(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'unknown',           -- healthy, degraded, unhealthy, unknown
    last_check TIMESTAMPTZ,                           -- Last health check timestamp
    avg_latency_ms DECIMAL(10,2),                     -- Moving average response latency
    error_count BIGINT DEFAULT 0,                     -- Cumulative error counter
    success_count BIGINT DEFAULT 0,                   -- Cumulative success counter
    last_error TEXT,                                  -- Most recent error message
    last_error_at TIMESTAMPTZ,                        -- When last error occurred
    updated_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,

    -- Constraints
    CONSTRAINT ai_model_health_status_check CHECK (status IN ('healthy', 'degraded', 'unhealthy', 'unknown'))
);

-- =====================================================
-- Trigger for updated_at
-- =====================================================

CREATE OR REPLACE FUNCTION update_ai_models_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_ai_models_updated_at ON ai_models;
CREATE TRIGGER trg_ai_models_updated_at
    BEFORE UPDATE ON ai_models
    FOR EACH ROW
    EXECUTE FUNCTION update_ai_models_updated_at();

DROP TRIGGER IF EXISTS trg_ai_model_health_updated_at ON ai_model_health;
CREATE TRIGGER trg_ai_model_health_updated_at
    BEFORE UPDATE ON ai_model_health
    FOR EACH ROW
    EXECUTE FUNCTION update_ai_models_updated_at();

-- =====================================================
-- Seed Default Routing Rules
-- =====================================================
-- Default routing configuration for common task types.
-- Prioritizes local models with remote fallbacks.

INSERT INTO ai_routing_rules (name, task_type, preferred_models, fallback_models, optimization_mode, priority)
VALUES
    -- Embeddings: Local-first for low latency and cost
    (
        'embeddings-local',
        'embedding',
        ARRAY['ollama/nomic-embed-text'],
        ARRAY['gemini/text-embedding-004'],
        'latency',
        10
    ),
    -- Simple extraction: Fast local models for routine extraction
    (
        'simple-extraction',
        'extraction',
        ARRAY['ollama/llama3.2'],
        ARRAY['gemini/gemini-2.0-flash'],
        'latency',
        10
    ),
    -- Complex extraction: Quality-optimized for nuanced understanding
    (
        'complex-extraction',
        'extraction',
        ARRAY['gemini/gemini-2.0-pro'],
        ARRAY['ollama/qwen2.5-32b'],
        'quality',
        20
    ),
    -- Summarization: Balanced for general use
    (
        'summarization',
        'summarization',
        ARRAY['ollama/phi-3.5-mini'],
        ARRAY['gemini/gemini-2.0-flash'],
        'balanced',
        10
    )
ON CONFLICT (name) DO NOTHING;

-- =====================================================
-- Comments
-- =====================================================

COMMENT ON TABLE ai_models IS 'Registry of AI models (local and remote) with capabilities and costs';
COMMENT ON COLUMN ai_models.id IS 'Unique identifier in format: provider/model-name';
COMMENT ON COLUMN ai_models.capabilities IS 'Array of supported tasks: embedding, summarization, extraction, classification';
COMMENT ON COLUMN ai_models.is_local IS 'True for locally-running models (ollama, mlx) - prefer for cost/privacy';
COMMENT ON COLUMN ai_models.priority IS 'Default selection priority (0-100, lower = preferred)';
COMMENT ON COLUMN ai_models.metadata IS 'Provider-specific configuration (temperature, top_p, etc.)';

COMMENT ON TABLE ai_routing_rules IS 'Task-based routing rules for intelligent model selection';
COMMENT ON COLUMN ai_routing_rules.task_type IS 'Task category: embedding, summarization, extraction, classification';
COMMENT ON COLUMN ai_routing_rules.preferred_models IS 'Ordered list of model IDs to try first';
COMMENT ON COLUMN ai_routing_rules.fallback_models IS 'Models to use when preferred unavailable';
COMMENT ON COLUMN ai_routing_rules.optimization_mode IS 'Optimization target: latency, quality, cost, balanced';
COMMENT ON COLUMN ai_routing_rules.require_local IS 'If true, only local models (ollama, mlx) are considered';

COMMENT ON TABLE ai_model_health IS 'Real-time health metrics for proactive routing decisions';
COMMENT ON COLUMN ai_model_health.status IS 'Current health: healthy, degraded, unhealthy, unknown';
COMMENT ON COLUMN ai_model_health.avg_latency_ms IS 'Moving average response time in milliseconds';
COMMENT ON COLUMN ai_model_health.error_count IS 'Cumulative errors since model registration';

-- +goose Down

-- Drop triggers first
DROP TRIGGER IF EXISTS trg_ai_model_health_updated_at ON ai_model_health;
DROP TRIGGER IF EXISTS trg_ai_models_updated_at ON ai_models;
DROP FUNCTION IF EXISTS update_ai_models_updated_at();

-- Drop tables in reverse dependency order
DROP TABLE IF EXISTS ai_model_health;
DROP TABLE IF EXISTS ai_routing_rules;
DROP TABLE IF EXISTS ai_models;

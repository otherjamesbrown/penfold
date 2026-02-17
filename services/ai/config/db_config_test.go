// Package config provides unit tests for database-backed dynamic model config resolution.
package config

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Mock Implementation
// =============================================================================

// mockModelConfigQuerier is a test implementation of ModelConfigQuerier.
type mockModelConfigQuerier struct {
	configs map[string]string // configKey -> modelName
	err     error             // error to return (if non-nil, overrides configs)
}

func (m *mockModelConfigQuerier) GetModelConfig(ctx context.Context, tenantID uuid.UUID, configKey string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	if m.configs == nil {
		return "", errors.New("model config not found")
	}
	modelName, ok := m.configs[configKey]
	if !ok {
		return "", errors.New("model config not found")
	}
	return modelName, nil
}

// =============================================================================
// DBConfigResolver Tests - Test DB-backed model resolution with 5-level priority
// =============================================================================

// TestDBConfigResolver_DBOverrideTakesPriority tests that DB stage override takes precedence over env vars.
// Priority level 1: DB override (stage.<stage>) should win over level 2: per-stage env var.
//
// Acceptance criteria:
// - When DB has "stage.triage=custom-db-model"
// - And env has AI_MODEL_TRIAGE=env-model
// - Then resolver returns "custom-db-model" (DB wins)
func TestDBConfigResolver_DBOverrideTakesPriority(t *testing.T) {
	// Setup: DB has stage.triage configured
	mockDB := &mockModelConfigQuerier{
		configs: map[string]string{
			"stage.triage": "custom-db-model",
		},
	}

	// Setup: env var also configured (should be ignored)
	t.Setenv("AI_MODEL_TRIAGE", "env-model")
	t.Setenv("AI_DEFAULT_LLM_MODEL", "default-model")

	baseCfg, err := Load()
	require.NoError(t, err)

	tenantID := uuid.New()
	resolver := NewDBConfigResolver(mockDB, baseCfg, tenantID)

	ctx := context.Background()
	got, err := resolver.ModelForStage(ctx, "triage")
	require.NoError(t, err)

	// DB override should take priority over env var
	assert.Equal(t, "custom-db-model", got, "DB stage override should take priority over env var")
}

// TestDBConfigResolver_FallbackToEnvWhenNoDBOverride tests fallback to env var when DB has no stage override.
// Priority level 2: per-stage env var should be used when level 1 (DB override) is not present.
//
// Acceptance criteria:
// - When DB has no "stage.triage" entry (returns error or not found)
// - And env has AI_MODEL_TRIAGE=env-model
// - Then resolver returns "env-model"
func TestDBConfigResolver_FallbackToEnvWhenNoDBOverride(t *testing.T) {
	// Setup: DB has no stage.triage configured
	mockDB := &mockModelConfigQuerier{
		configs: map[string]string{
			// No "stage.triage" entry
		},
	}

	// Setup: env var configured
	t.Setenv("AI_MODEL_TRIAGE", "env-triage-model")
	t.Setenv("AI_DEFAULT_LLM_MODEL", "default-model")

	baseCfg, err := Load()
	require.NoError(t, err)

	tenantID := uuid.New()
	resolver := NewDBConfigResolver(mockDB, baseCfg, tenantID)

	ctx := context.Background()
	got, err := resolver.ModelForStage(ctx, "triage")
	require.NoError(t, err)

	// Should fall back to env var
	assert.Equal(t, "env-triage-model", got, "Should fall back to per-stage env var when DB has no override")
}

// TestDBConfigResolver_DBDefaultUsedWhenNoStageConfig tests DB default.llm fallback.
// Priority level 3: DB default.llm should be used when levels 1 and 2 are not present.
//
// Acceptance criteria:
// - When DB has no "stage.triage" entry
// - And env has no AI_MODEL_TRIAGE
// - And DB has "default.llm=db-default-llm"
// - Then resolver returns "db-default-llm"
func TestDBConfigResolver_DBDefaultUsedWhenNoStageConfig(t *testing.T) {
	// Setup: DB has default.llm but no stage override
	mockDB := &mockModelConfigQuerier{
		configs: map[string]string{
			"default.llm": "db-default-llm",
			// No "stage.triage"
		},
	}

	// Setup: no stage-specific env var, but global default exists
	t.Setenv("AI_DEFAULT_LLM_MODEL", "env-default-llm")

	baseCfg, err := Load()
	require.NoError(t, err)

	tenantID := uuid.New()
	resolver := NewDBConfigResolver(mockDB, baseCfg, tenantID)

	ctx := context.Background()
	got, err := resolver.ModelForStage(ctx, "triage")
	require.NoError(t, err)

	// DB default should take priority over env var default
	assert.Equal(t, "db-default-llm", got, "Should use DB default.llm when no stage-specific config exists")
}

// TestDBConfigResolver_EmbeddingUsesDBDefaultEmbedding tests that embedding stage uses default.embedding.
// Priority level 3: Embedding stage should check "default.embedding" NOT "default.llm".
//
// Acceptance criteria:
// - When DB has "default.embedding=db-embed-model"
// - And DB has "default.llm=db-llm-model"
// - And no stage.embedding or AI_MODEL_EMBEDDING
// - Then embedding stage returns "db-embed-model" (NOT db-llm-model)
func TestDBConfigResolver_EmbeddingUsesDBDefaultEmbedding(t *testing.T) {
	// Setup: DB has both default.llm and default.embedding
	mockDB := &mockModelConfigQuerier{
		configs: map[string]string{
			"default.llm":       "db-llm-model",
			"default.embedding": "db-embed-model",
			// No stage.embedding
		},
	}

	// Setup: no stage-specific env vars
	t.Setenv("AI_DEFAULT_LLM_MODEL", "env-llm")
	t.Setenv("AI_DEFAULT_EMBEDDING_MODEL", "env-embed")

	baseCfg, err := Load()
	require.NoError(t, err)

	tenantID := uuid.New()
	resolver := NewDBConfigResolver(mockDB, baseCfg, tenantID)

	ctx := context.Background()
	got, err := resolver.ModelForStage(ctx, "embedding")
	require.NoError(t, err)

	// Embedding stage should use default.embedding, NOT default.llm
	assert.Equal(t, "db-embed-model", got, "Embedding stage should use default.embedding from DB")
}

// TestDBConfigResolver_FallbackChain tests the full 5-level priority chain.
// Tests all 5 levels in order of priority:
// 1. DB override (stage.<stage>)
// 2. Per-stage env var
// 3. DB default (default.llm or default.embedding)
// 4. Global env var default
// 5. Hardcoded fallback
func TestDBConfigResolver_FallbackChain(t *testing.T) {
	tests := []struct {
		name        string
		stage       string
		dbConfigs   map[string]string
		envVars     map[string]string
		wantModel   string
		description string
	}{
		{
			name:  "level 1: DB stage override wins",
			stage: "triage",
			dbConfigs: map[string]string{
				"stage.triage": "db-stage-override",
				"default.llm":  "db-default",
			},
			envVars: map[string]string{
				"AI_MODEL_TRIAGE":      "env-stage",
				"AI_DEFAULT_LLM_MODEL": "env-default",
			},
			wantModel:   "db-stage-override",
			description: "DB stage override takes priority over everything",
		},
		{
			name:  "level 2: per-stage env var used when no DB override",
			stage: "triage",
			dbConfigs: map[string]string{
				"default.llm": "db-default",
			},
			envVars: map[string]string{
				"AI_MODEL_TRIAGE":      "env-stage",
				"AI_DEFAULT_LLM_MODEL": "env-default",
			},
			wantModel:   "env-stage",
			description: "Per-stage env var used when DB has no stage override",
		},
		{
			name:  "level 3: DB default used when no stage config",
			stage: "triage",
			dbConfigs: map[string]string{
				"default.llm": "db-default",
			},
			envVars: map[string]string{
				"AI_DEFAULT_LLM_MODEL": "env-default",
			},
			wantModel:   "db-default",
			description: "DB default.llm used when no stage-specific config exists",
		},
		{
			name:      "level 4: global env var default used when no DB config",
			stage:     "triage",
			dbConfigs: map[string]string{},
			envVars: map[string]string{
				"AI_DEFAULT_LLM_MODEL": "env-default",
			},
			wantModel:   "env-default",
			description: "Global env var default used when DB has no config",
		},
		{
			name:        "level 5: hardcoded fallback when nothing configured",
			stage:       "triage",
			dbConfigs:   map[string]string{},
			envVars:     map[string]string{},
			wantModel:   DefaultLLMModel,
			description: "Hardcoded fallback used when nothing else is configured",
		},
		{
			name:  "embedding stage: uses default.embedding not default.llm",
			stage: "embedding",
			dbConfigs: map[string]string{
				"default.embedding": "db-embed",
				"default.llm":       "db-llm",
			},
			envVars: map[string]string{
				"AI_DEFAULT_EMBEDDING_MODEL": "env-embed",
				"AI_DEFAULT_LLM_MODEL":       "env-llm",
			},
			wantModel:   "db-embed",
			description: "Embedding stage checks default.embedding, not default.llm",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDB := &mockModelConfigQuerier{
				configs: tt.dbConfigs,
			}

			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			baseCfg, err := Load()
			require.NoError(t, err)

			tenantID := uuid.New()
			resolver := NewDBConfigResolver(mockDB, baseCfg, tenantID)

			ctx := context.Background()
			got, err := resolver.ModelForStage(ctx, tt.stage)
			require.NoError(t, err)

			assert.Equal(t, tt.wantModel, got, tt.description)
		})
	}
}

// TestDBConfigResolver_CacheTTL tests that resolver caches results and respects TTL.
// Acceptance criteria:
// - First call queries DB and caches result
// - Subsequent calls within TTL return cached value (no DB query)
// - After TTL expires, next call queries DB again
func TestDBConfigResolver_CacheTTL(t *testing.T) {
	callCount := 0
	mockDB := &mockModelConfigQuerier{
		configs: map[string]string{
			"stage.triage": "model-v1",
		},
	}

	// Wrap the mock to count calls
	countingMock := &countingModelConfigQuerier{
		delegate:  mockDB,
		callCount: &callCount,
	}

	baseCfg, err := Load()
	require.NoError(t, err)

	tenantID := uuid.New()

	// Create resolver with short TTL (1 second for testing)
	resolver := NewDBConfigResolverWithTTL(countingMock, baseCfg, tenantID, 1*time.Second)

	ctx := context.Background()

	// First call should query DB
	got1, err := resolver.ModelForStage(ctx, "triage")
	require.NoError(t, err)
	assert.Equal(t, "model-v1", got1)
	assert.Equal(t, 1, callCount, "First call should query DB")

	// Second call within TTL should use cache (no DB query)
	got2, err := resolver.ModelForStage(ctx, "triage")
	require.NoError(t, err)
	assert.Equal(t, "model-v1", got2)
	assert.Equal(t, 1, callCount, "Second call within TTL should use cache")

	// Update DB value (to verify cache is being used)
	mockDB.configs["stage.triage"] = "model-v2"

	// Third call within TTL should still return cached value
	got3, err := resolver.ModelForStage(ctx, "triage")
	require.NoError(t, err)
	assert.Equal(t, "model-v1", got3, "Should return cached value, not new DB value")
	assert.Equal(t, 1, callCount, "Third call within TTL should use cache")

	// Wait for TTL to expire
	time.Sleep(1100 * time.Millisecond)

	// Fourth call after TTL should query DB again
	got4, err := resolver.ModelForStage(ctx, "triage")
	require.NoError(t, err)
	assert.Equal(t, "model-v2", got4, "After TTL expires, should query DB and get new value")
	assert.Equal(t, 2, callCount, "Fourth call after TTL should query DB again")
}

// TestDBConfigResolver_DBErrorFallsBackToEnv tests that DB errors don't break resolution.
// Acceptance criteria:
// - When DB query returns error
// - Resolver should fall back to env var resolution
// - Error should be logged but not returned to caller
func TestDBConfigResolver_DBErrorFallsBackToEnv(t *testing.T) {
	// Setup: DB returns error
	mockDB := &mockModelConfigQuerier{
		err: errors.New("database connection failed"),
	}

	// Setup: env var configured as fallback
	t.Setenv("AI_MODEL_TRIAGE", "env-fallback-model")

	baseCfg, err := Load()
	require.NoError(t, err)

	tenantID := uuid.New()
	resolver := NewDBConfigResolver(mockDB, baseCfg, tenantID)

	ctx := context.Background()
	got, err := resolver.ModelForStage(ctx, "triage")
	require.NoError(t, err, "DB error should not propagate to caller")

	// Should fall back to env var
	assert.Equal(t, "env-fallback-model", got, "Should fall back to env var when DB query fails")
}

// TestDBConfigResolver_AllStages tests resolution for all pipeline stages.
// Acceptance criteria:
// - All pipeline stages (triage, extract_entities, extract_assertions, deep_analyze, embedding)
// - Each can have DB overrides
// - LLM stages fall back to default.llm
// - Embedding stage falls back to default.embedding
func TestDBConfigResolver_AllStages(t *testing.T) {
	mockDB := &mockModelConfigQuerier{
		configs: map[string]string{
			"stage.triage":              "db-triage",
			"stage.extract_entities":    "db-entities",
			"stage.extract_assertions":  "db-assertions",
			"stage.deep_analyze":        "db-analyze",
			"stage.embedding":           "db-embedding",
		},
	}

	baseCfg, err := Load()
	require.NoError(t, err)

	tenantID := uuid.New()
	resolver := NewDBConfigResolver(mockDB, baseCfg, tenantID)

	ctx := context.Background()

	stages := []struct {
		name     string
		expected string
	}{
		{"triage", "db-triage"},
		{"extract_entities", "db-entities"},
		{"extract_assertions", "db-assertions"},
		{"deep_analyze", "db-analyze"},
		{"embedding", "db-embedding"},
	}

	for _, stage := range stages {
		t.Run(stage.name, func(t *testing.T) {
			got, err := resolver.ModelForStage(ctx, stage.name)
			require.NoError(t, err)
			assert.Equal(t, stage.expected, got)
		})
	}
}

// =============================================================================
// Helper Types
// =============================================================================

// countingModelConfigQuerier wraps a ModelConfigQuerier and counts calls.
type countingModelConfigQuerier struct {
	delegate  ModelConfigQuerier
	callCount *int
}

func (c *countingModelConfigQuerier) GetModelConfig(ctx context.Context, tenantID uuid.UUID, configKey string) (string, error) {
	*c.callCount++
	return c.delegate.GetModelConfig(ctx, tenantID, configKey)
}

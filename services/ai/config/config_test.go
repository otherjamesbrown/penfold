// Package config provides unit tests for service configuration.
package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// ModelForStage Tests - Test per-stage model configuration loading
// =============================================================================

// TestModelForStage_ExplicitStageConfig tests that stage-specific env vars are used.
// Acceptance criteria: Each pipeline stage uses its configured model from AI_MODEL_* env vars.
func TestModelForStage_ExplicitStageConfig(t *testing.T) {
	tests := []struct {
		name      string
		stage     string
		envVars   map[string]string
		wantModel string
	}{
		{
			name:  "triage stage uses AI_MODEL_TRIAGE",
			stage: "triage",
			envVars: map[string]string{
				"AI_MODEL_TRIAGE": "gemini-2.0-flash-thinking",
			},
			wantModel: "gemini-2.0-flash-thinking",
		},
		{
			name:  "extract_ner stage uses AI_MODEL_EXTRACT_ENTITIES",
			stage: "extract_ner",
			envVars: map[string]string{
				"AI_MODEL_EXTRACT_ENTITIES": "qwen2.5:7b",
			},
			wantModel: "qwen2.5:7b",
		},
		{
			name:  "extract_assertions stage uses AI_MODEL_EXTRACT_ASSERTIONS",
			stage: "extract_assertions",
			envVars: map[string]string{
				"AI_MODEL_EXTRACT_ASSERTIONS": "llama3.2",
			},
			wantModel: "llama3.2",
		},
		{
			name:  "analyze stage uses AI_MODEL_DEEP_ANALYZE",
			stage: "analyze",
			envVars: map[string]string{
				"AI_MODEL_DEEP_ANALYZE": "gemini-2.5-pro",
			},
			wantModel: "gemini-2.5-pro",
		},
		{
			name:  "embed stage uses AI_MODEL_EMBEDDING",
			stage: "embed",
			envVars: map[string]string{
				"AI_MODEL_EMBEDDING": "mxbai-embed-large-v1",
			},
			wantModel: "mxbai-embed-large-v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set env vars
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			cfg, err := Load()
			require.NoError(t, err)

			got := cfg.ModelForStage(tt.stage)
			assert.Equal(t, tt.wantModel, got, "ModelForStage(%q) should return stage-specific model", tt.stage)
		})
	}
}

// TestModelForStage_FallbackToDefaultLLM tests fallback to AI_DEFAULT_LLM_MODEL.
// Acceptance criteria: Missing/empty stage config falls back to AI_DEFAULT_LLM_MODEL for LLM stages,
// and AI_DEFAULT_EMBEDDING_MODEL for embedding stage.
func TestModelForStage_FallbackToDefaultLLM(t *testing.T) {
	tests := []struct {
		name      string
		stage     string
		envVars   map[string]string
		wantModel string
	}{
		{
			name:  "triage falls back to default LLM when not configured",
			stage: "triage",
			envVars: map[string]string{
				"AI_DEFAULT_LLM_MODEL": "gemini-2.0-flash",
			},
			wantModel: "gemini-2.0-flash",
		},
		{
			name:  "extract_ner falls back to default LLM",
			stage: "extract_ner",
			envVars: map[string]string{
				"AI_DEFAULT_LLM_MODEL": "custom-default-model",
			},
			wantModel: "custom-default-model",
		},
		{
			name:  "analyze falls back to default LLM",
			stage: "analyze",
			envVars: map[string]string{
				"AI_DEFAULT_LLM_MODEL": "gemini-2.5-pro",
			},
			wantModel: "gemini-2.5-pro",
		},
		{
			name:  "embed falls back to default embedding model",
			stage: "embed",
			envVars: map[string]string{
				"AI_DEFAULT_EMBEDDING_MODEL": "custom-embedding-model",
			},
			wantModel: "custom-embedding-model",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set env vars (no stage-specific var set)
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			cfg, err := Load()
			require.NoError(t, err)

			got := cfg.ModelForStage(tt.stage)
			assert.Equal(t, tt.wantModel, got, "ModelForStage(%q) should fall back to DefaultLLMModel", tt.stage)
		})
	}
}

// TestModelForStage_FallbackToHardcodedDefault tests final fallback.
// Acceptance criteria: If no env vars set, LLM stages fall back to "gemini-2.5-flash",
// embedding stage falls back to "mxbai-embed-large".
func TestModelForStage_FallbackToHardcodedDefault(t *testing.T) {
	tests := []struct {
		stage    string
		expected string
	}{
		{"triage", DefaultLLMModel},
		{"extract_ner", DefaultLLMModel},
		{"extract_assertions", DefaultLLMModel},
		{"analyze", DefaultLLMModel},
		{"embed", DefaultEmbeddingModel},
	}

	for _, tt := range tests {
		t.Run(tt.stage, func(t *testing.T) {
			// No env vars set - should use hardcoded defaults from Load()
			cfg, err := Load()
			require.NoError(t, err)

			got := cfg.ModelForStage(tt.stage)
			assert.Equal(t, tt.expected, got, "ModelForStage(%q) should fall back to hardcoded default", tt.stage)
		})
	}
}

// TestModelForStage_EmbeddingFallbackToEmbeddingModel tests that embedding stage
// falls back to DefaultEmbeddingModel, NOT DefaultLLMModel.
// This is the core fix for pf-783168.
func TestModelForStage_EmbeddingFallbackToEmbeddingModel(t *testing.T) {
	tests := []struct {
		name      string
		envVars   map[string]string
		wantModel string
	}{
		{
			name: "embedding stage falls back to DefaultEmbeddingModel constant when no config",
			envVars: map[string]string{
				// No AI_MODEL_EMBEDDING, no AI_DEFAULT_EMBEDDING_MODEL
			},
			wantModel: DefaultEmbeddingModel, // Should be "mxbai-embed-large"
		},
		{
			name: "embedding stage falls back to AI_DEFAULT_EMBEDDING_MODEL when set",
			envVars: map[string]string{
				"AI_DEFAULT_EMBEDDING_MODEL": "custom-embedding",
				// No AI_MODEL_EMBEDDING
			},
			wantModel: "custom-embedding",
		},
		{
			name: "embedding stage DOES NOT fall back to AI_DEFAULT_LLM_MODEL",
			envVars: map[string]string{
				"AI_DEFAULT_LLM_MODEL": "gemini-2.0-flash",
				// No AI_MODEL_EMBEDDING, no AI_DEFAULT_EMBEDDING_MODEL
			},
			wantModel: DefaultEmbeddingModel, // Should use embedding default, NOT LLM default
		},
		{
			name: "embedding stage prefers AI_DEFAULT_EMBEDDING_MODEL over AI_DEFAULT_LLM_MODEL",
			envVars: map[string]string{
				"AI_DEFAULT_LLM_MODEL":       "gemini-2.0-flash",
				"AI_DEFAULT_EMBEDDING_MODEL": "custom-embedding",
				// No AI_MODEL_EMBEDDING
			},
			wantModel: "custom-embedding", // Should use embedding default, NOT LLM default
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			cfg, err := Load()
			require.NoError(t, err)

			got := cfg.ModelForStage("embed")
			assert.Equal(t, tt.wantModel, got, "ModelForStage(\"embed\") should fall back to embedding model defaults")
		})
	}
}

// TestModelForStage_EmptyStageConfig tests that empty string in env var falls back.
// Acceptance criteria: Empty stage-specific env var should fall back to default, not use empty string.
func TestModelForStage_EmptyStageConfig(t *testing.T) {
	t.Setenv("AI_MODEL_TRIAGE", "")
	t.Setenv("AI_DEFAULT_LLM_MODEL", "gemini-2.0-flash")

	cfg, err := Load()
	require.NoError(t, err)

	got := cfg.ModelForStage("triage")
	assert.Equal(t, "gemini-2.0-flash", got, "Empty AI_MODEL_TRIAGE should fall back to AI_DEFAULT_LLM_MODEL")
}

// TestModelForStage_UnknownStage tests behavior for unrecognized stage names.
// Should fall back to default model gracefully.
func TestModelForStage_UnknownStage(t *testing.T) {
	t.Setenv("AI_DEFAULT_LLM_MODEL", "gemini-2.0-flash")

	cfg, err := Load()
	require.NoError(t, err)

	got := cfg.ModelForStage("unknown_stage")
	assert.Equal(t, "gemini-2.0-flash", got, "Unknown stage should fall back to DefaultLLMModel")
}

// TestModelForStage_CaseInsensitive tests that stage names are case-insensitive.
func TestModelForStage_CaseInsensitive(t *testing.T) {
	t.Setenv("AI_MODEL_TRIAGE", "custom-triage-model")

	cfg, err := Load()
	require.NoError(t, err)

	tests := []string{"triage", "TRIAGE", "Triage", "TrIaGe"}
	for _, stage := range tests {
		got := cfg.ModelForStage(stage)
		assert.Equal(t, "custom-triage-model", got, "ModelForStage should be case-insensitive for stage=%q", stage)
	}
}

// =============================================================================
// Canonical Env Var Name Tests
// =============================================================================

// TestModelForStage_CanonicalEnvVarNames tests that canonical env var names work
// and take precedence over legacy names.
func TestModelForStage_CanonicalEnvVarNames(t *testing.T) {
	tests := []struct {
		name      string
		stage     string
		envVars   map[string]string
		wantModel string
	}{
		{
			name:  "AI_MODEL_EXTRACT_NER canonical name works",
			stage: "extract_ner",
			envVars: map[string]string{
				"AI_MODEL_EXTRACT_NER": "canonical-model",
			},
			wantModel: "canonical-model",
		},
		{
			name:  "AI_MODEL_EXTRACT_NER takes precedence over legacy AI_MODEL_EXTRACT_ENTITIES",
			stage: "extract_ner",
			envVars: map[string]string{
				"AI_MODEL_EXTRACT_NER":      "canonical-model",
				"AI_MODEL_EXTRACT_ENTITIES": "legacy-model",
			},
			wantModel: "canonical-model",
		},
		{
			name:  "legacy AI_MODEL_EXTRACT_ENTITIES still works as fallback",
			stage: "extract_ner",
			envVars: map[string]string{
				"AI_MODEL_EXTRACT_ENTITIES": "legacy-model",
			},
			wantModel: "legacy-model",
		},
		{
			name:  "AI_MODEL_ANALYZE canonical name works",
			stage: "analyze",
			envVars: map[string]string{
				"AI_MODEL_ANALYZE": "canonical-analyze",
			},
			wantModel: "canonical-analyze",
		},
		{
			name:  "AI_MODEL_ANALYZE takes precedence over legacy AI_MODEL_DEEP_ANALYZE",
			stage: "analyze",
			envVars: map[string]string{
				"AI_MODEL_ANALYZE":      "canonical-analyze",
				"AI_MODEL_DEEP_ANALYZE": "legacy-analyze",
			},
			wantModel: "canonical-analyze",
		},
		{
			name:  "legacy AI_MODEL_DEEP_ANALYZE still works as fallback",
			stage: "analyze",
			envVars: map[string]string{
				"AI_MODEL_DEEP_ANALYZE": "legacy-analyze",
			},
			wantModel: "legacy-analyze",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			cfg, err := Load()
			require.NoError(t, err)

			got := cfg.ModelForStage(tt.stage)
			assert.Equal(t, tt.wantModel, got)
		})
	}
}

// =============================================================================
// Ollama URL Configuration Tests
// =============================================================================

// TestLoad_OllamaURL tests that AI_OLLAMA_URL is loaded correctly.
// Acceptance criteria: OllamaURL loaded from AI_OLLAMA_URL env var.
func TestLoad_OllamaURL(t *testing.T) {
	tests := []struct {
		name        string
		envVar      string
		envValue    string
		wantOllama  string
	}{
		{
			name:       "explicit AI_OLLAMA_URL",
			envVar:     "AI_OLLAMA_URL",
			envValue:   "http://dev01.brown.chat:11434",
			wantOllama: "http://dev01.brown.chat:11434",
		},
		{
			name:       "localhost Ollama URL",
			envVar:     "AI_OLLAMA_URL",
			envValue:   "http://localhost:11434",
			wantOllama: "http://localhost:11434",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.envVar, tt.envValue)

			cfg, err := Load()
			require.NoError(t, err)
			assert.Equal(t, tt.wantOllama, cfg.OllamaURL, "OllamaURL should be loaded from %s", tt.envVar)
		})
	}
}

// TestLoad_OllamaURL_DefaultFallback tests default Ollama URL when not configured.
func TestLoad_OllamaURL_DefaultFallback(t *testing.T) {
	// No AI_OLLAMA_URL set
	cfg, err := Load()
	require.NoError(t, err)

	// Should use default from constant (need to check what default is appropriate)
	// For now, test that it's not empty
	assert.NotEmpty(t, cfg.OllamaURL, "OllamaURL should have a default value")
}

// =============================================================================
// Backward Compatibility Tests
// =============================================================================

// TestLoad_BackwardCompatMLXURLs tests that existing AI_MLX_* env vars still work.
// Acceptance criteria: Backward compatible with AI_MLX_EMBEDDINGS_URL and AI_MLX_LLM_URL.
func TestLoad_BackwardCompatMLXURLs(t *testing.T) {
	tests := []struct {
		name        string
		envVars     map[string]string
		wantEmbeddings string
		wantLLM        string
	}{
		{
			name: "AI_MLX_EMBEDDINGS_URL still works",
			envVars: map[string]string{
				"AI_MLX_EMBEDDINGS_URL": "http://dev01:8081",
			},
			wantEmbeddings: "http://dev01:8081",
		},
		{
			name: "AI_MLX_LLM_URL still works",
			envVars: map[string]string{
				"AI_MLX_LLM_URL": "http://dev01:8080",
			},
			wantLLM: "http://dev01:8080",
		},
		{
			name: "both MLX URLs work together",
			envVars: map[string]string{
				"AI_MLX_EMBEDDINGS_URL": "http://mlx-embed:8081",
				"AI_MLX_LLM_URL":        "http://mlx-llm:8080",
			},
			wantEmbeddings: "http://mlx-embed:8081",
			wantLLM:        "http://mlx-llm:8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			cfg, err := Load()
			require.NoError(t, err)

			if tt.wantEmbeddings != "" {
				assert.Equal(t, tt.wantEmbeddings, cfg.MLXEmbeddingsURL, "MLXEmbeddingsURL should be loaded from AI_MLX_EMBEDDINGS_URL")
			}
			if tt.wantLLM != "" {
				assert.Equal(t, tt.wantLLM, cfg.MLXLLMURL, "MLXLLMURL should be loaded from AI_MLX_LLM_URL")
			}
		})
	}
}

// =============================================================================
// Stage Model Map Population Tests
// =============================================================================

// TestLoad_StageModelsPopulated tests that the StageModels map is correctly populated.
// Acceptance criteria: StageModels map populated from AI_MODEL_* env vars.
func TestLoad_StageModelsPopulated(t *testing.T) {
	envVars := map[string]string{
		"AI_MODEL_TRIAGE":              "triage-model",
		"AI_MODEL_EXTRACT_ENTITIES":    "entity-model",
		"AI_MODEL_EXTRACT_ASSERTIONS":  "assertion-model",
		"AI_MODEL_DEEP_ANALYZE":        "analyze-model",
		"AI_MODEL_EMBEDDING":           "embed-model",
	}

	for k, v := range envVars {
		t.Setenv(k, v)
	}

	cfg, err := Load()
	require.NoError(t, err)

	// Verify StageModels map has all entries
	expectedStages := map[string]string{
		"triage":              "triage-model",
		"extract_ner":         "entity-model",
		"extract_assertions":  "assertion-model",
		"analyze":             "analyze-model",
		"embed":               "embed-model",
	}

	for stage, expectedModel := range expectedStages {
		got := cfg.ModelForStage(stage)
		assert.Equal(t, expectedModel, got, "StageModels[%q] should be %q", stage, expectedModel)
	}
}

// TestLoad_PartialStageModelsConfig tests mixed explicit and default stage config.
// Some stages configured explicitly, others fall back to default.
func TestLoad_PartialStageModelsConfig(t *testing.T) {
	t.Setenv("AI_MODEL_TRIAGE", "custom-triage")
	t.Setenv("AI_MODEL_DEEP_ANALYZE", "custom-analyze")
	t.Setenv("AI_DEFAULT_LLM_MODEL", "default-model")
	t.Setenv("AI_DEFAULT_EMBEDDING_MODEL", "default-embedding")

	cfg, err := Load()
	require.NoError(t, err)

	// Explicit configs should use their values
	assert.Equal(t, "custom-triage", cfg.ModelForStage("triage"))
	assert.Equal(t, "custom-analyze", cfg.ModelForStage("analyze"))

	// Non-configured LLM stages should fall back to default LLM model
	assert.Equal(t, "default-model", cfg.ModelForStage("extract_ner"))
	assert.Equal(t, "default-model", cfg.ModelForStage("extract_assertions"))

	// Non-configured embed stage should fall back to default embedding model, NOT LLM model
	assert.Equal(t, "default-embedding", cfg.ModelForStage("embed"))
}

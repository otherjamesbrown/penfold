package pipelineservice

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pipelinev1 "github.com/otherjamesbrown/penfold/api/proto/pipeline/v1"
)

// TestListModels_NilDB returns results even when no DB is configured.
// The hardcoded known-model set must always be returned.
func TestListModels_NilDB(t *testing.T) {
	ctx := context.Background()
	svc := NewService(nil, testLogger(), nil, nil, "default")

	resp, err := svc.ListModels(ctx, &pipelinev1.ListModelsRequest{})

	require.NoError(t, err)
	require.NotNil(t, resp)
	// Must contain at least the 4 hardcoded known models.
	assert.GreaterOrEqual(t, len(resp.Models), 4)
}

// TestListModels_KnownModelsPresent verifies all 4 hardcoded models appear.
func TestListModels_KnownModelsPresent(t *testing.T) {
	ctx := context.Background()
	svc := NewService(nil, testLogger(), nil, nil, "default")

	resp, err := svc.ListModels(ctx, &pipelinev1.ListModelsRequest{})
	require.NoError(t, err)

	byName := make(map[string]*pipelinev1.ModelInfo)
	for _, m := range resp.Models {
		byName[m.Name] = m
	}

	for _, expected := range []string{
		"qwen2.5:7b",
		"mxbai-embed-large",
		"gemini-2.0-flash",
		"gemini-2.5-pro",
	} {
		m, ok := byName[expected]
		require.True(t, ok, "expected model %q to be present", expected)
		assert.Equal(t, "available", m.Status, "model %q should be available", expected)
	}
}

// TestListModels_BackendDetection verifies that backend assignment matches the
// selectBackend logic: "gemini" in name → "gemini", everything else → "ollama".
func TestListModels_BackendDetection(t *testing.T) {
	ctx := context.Background()
	svc := NewService(nil, testLogger(), nil, nil, "default")

	resp, err := svc.ListModels(ctx, &pipelinev1.ListModelsRequest{})
	require.NoError(t, err)

	byName := make(map[string]*pipelinev1.ModelInfo)
	for _, m := range resp.Models {
		byName[m.Name] = m
	}

	assert.Equal(t, "ollama", byName["qwen2.5:7b"].Backend, "qwen2.5:7b should use ollama backend")
	assert.Equal(t, "ollama", byName["mxbai-embed-large"].Backend, "mxbai-embed-large should use ollama backend")
	assert.Equal(t, "gemini", byName["gemini-2.0-flash"].Backend, "gemini-2.0-flash should use gemini backend")
	assert.Equal(t, "gemini", byName["gemini-2.5-pro"].Backend, "gemini-2.5-pro should use gemini backend")
}

// TestListModels_DefaultModel verifies that exactly one model is marked is_default.
func TestListModels_DefaultModel(t *testing.T) {
	ctx := context.Background()
	svc := NewService(nil, testLogger(), nil, nil, "default")

	resp, err := svc.ListModels(ctx, &pipelinev1.ListModelsRequest{})
	require.NoError(t, err)

	var defaults []string
	for _, m := range resp.Models {
		if m.IsDefault {
			defaults = append(defaults, m.Name)
		}
	}
	require.Len(t, defaults, 1, "exactly one model should be marked as default")
	// Without any DB config, the fallback is qwen2.5:7b.
	assert.Equal(t, "qwen2.5:7b", defaults[0])
}

// TestListModels_EmbeddingModelHasEmbeddingStage verifies mxbai-embed-large
// is associated with the embedding stage.
func TestListModels_EmbeddingModelHasEmbeddingStage(t *testing.T) {
	ctx := context.Background()
	svc := NewService(nil, testLogger(), nil, nil, "default")

	resp, err := svc.ListModels(ctx, &pipelinev1.ListModelsRequest{})
	require.NoError(t, err)

	byName := make(map[string]*pipelinev1.ModelInfo)
	for _, m := range resp.Models {
		byName[m.Name] = m
	}

	embed := byName["mxbai-embed-large"]
	require.NotNil(t, embed)
	assert.Contains(t, embed.Stages, "embedding", "mxbai-embed-large should be assigned to embedding stage")
}

// TestListModels_DefaultLLMHasLLMStages verifies the default LLM model is
// assigned to all non-embedding LLM stages.
func TestListModels_DefaultLLMHasLLMStages(t *testing.T) {
	ctx := context.Background()
	svc := NewService(nil, testLogger(), nil, nil, "default")

	resp, err := svc.ListModels(ctx, &pipelinev1.ListModelsRequest{})
	require.NoError(t, err)

	byName := make(map[string]*pipelinev1.ModelInfo)
	for _, m := range resp.Models {
		byName[m.Name] = m
	}

	llm := byName["qwen2.5:7b"]
	require.NotNil(t, llm)
	for _, stage := range []string{"triage", "extract_entities", "extract_assertions", "deep_analyze"} {
		assert.Contains(t, llm.Stages, stage, "qwen2.5:7b should be assigned to stage %q", stage)
	}
}

// TestListModels_ResponseStructure verifies the proto response has the expected shape.
func TestListModels_ResponseStructure(t *testing.T) {
	resp := &pipelinev1.ListModelsResponse{
		Models: []*pipelinev1.ModelInfo{
			{
				Name:      "qwen2.5:7b",
				Backend:   "ollama",
				Stages:    []string{"triage", "extract_entities"},
				IsDefault: true,
				Status:    "available",
			},
			{
				Name:      "gemini-2.0-flash",
				Backend:   "gemini",
				Stages:    []string{},
				IsDefault: false,
				Status:    "available",
			},
		},
	}

	assert.Len(t, resp.Models, 2)
	assert.Equal(t, "qwen2.5:7b", resp.Models[0].Name)
	assert.Equal(t, "ollama", resp.Models[0].Backend)
	assert.Equal(t, []string{"triage", "extract_entities"}, resp.Models[0].Stages)
	assert.True(t, resp.Models[0].IsDefault)
	assert.Equal(t, "available", resp.Models[0].Status)

	assert.Equal(t, "gemini-2.0-flash", resp.Models[1].Name)
	assert.Equal(t, "gemini", resp.Models[1].Backend)
	assert.False(t, resp.Models[1].IsDefault)
}

// TestSelectBackend verifies the backend detection helper function.
func TestSelectBackend(t *testing.T) {
	testCases := []struct {
		model           string
		expectedBackend string
	}{
		{"gemini-2.0-flash", "gemini"},
		{"gemini-2.5-pro", "gemini"},
		{"GEMINI-PRO", "gemini"}, // case-insensitive
		{"qwen2.5:7b", "ollama"},
		{"mxbai-embed-large", "ollama"},
		{"llama3:8b", "ollama"},
		{"mistral:7b", "ollama"},
	}

	for _, tc := range testCases {
		t.Run(tc.model, func(t *testing.T) {
			got := selectBackend(tc.model)
			assert.Equal(t, tc.expectedBackend, got)
		})
	}
}

// TestSortModelInfos verifies that the default model sorts first, then alphabetically.
func TestSortModelInfos(t *testing.T) {
	models := []*pipelinev1.ModelInfo{
		{Name: "zzz-model", IsDefault: false},
		{Name: "aaa-model", IsDefault: false},
		{Name: "default-model", IsDefault: true},
		{Name: "mmm-model", IsDefault: false},
	}

	sortModelInfos(models)

	assert.Equal(t, "default-model", models[0].Name, "default model should be first")
	assert.Equal(t, "aaa-model", models[1].Name)
	assert.Equal(t, "mmm-model", models[2].Name)
	assert.Equal(t, "zzz-model", models[3].Name)
}

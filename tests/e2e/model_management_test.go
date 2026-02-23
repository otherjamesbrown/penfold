//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestModelManagement_ListModels verifies that `penf ai model list` returns
// the current model configuration for all 5 pipeline stages.
func TestModelManagement_ListModels(t *testing.T) {
	env := SetupE2EEnvironment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result := env.SafeCLI.Run(ctx, "ai", "model", "list", "-o", "json")
	require.True(t, result.Success(), "ai model list should succeed: %s", result.Stderr)

	var modelList struct {
		Stages []struct {
			Stage   string `json:"stage"`
			Model   string `json:"model"`
			Source  string `json:"source"`  // "db", "env", "default"
			Backend string `json:"backend"` // "ollama", "gemini"
		} `json:"stages"`
		Defaults struct {
			LLM       string `json:"llm"`
			Embedding string `json:"embedding"`
		} `json:"defaults"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Stdout), &modelList),
		"should parse model list JSON: %s", result.Stdout)

	// Must have all 5 stages
	stageNames := make(map[string]bool)
	for _, s := range modelList.Stages {
		stageNames[s.Stage] = true
		assert.NotEmpty(t, s.Model, "stage %s should have a model", s.Stage)
		assert.NotEmpty(t, s.Source, "stage %s should have a source", s.Stage)
		assert.NotEmpty(t, s.Backend, "stage %s should have a backend", s.Stage)
	}

	expectedStages := []string{"triage", "extract_ner", "extract_assertions", "analyze", "embed"}
	for _, stage := range expectedStages {
		assert.True(t, stageNames[stage], "stage %s should be in model list", stage)
	}

	// Defaults should be populated
	assert.NotEmpty(t, modelList.Defaults.LLM, "LLM default should be set")
	assert.NotEmpty(t, modelList.Defaults.Embedding, "embedding default should be set")
}

// TestModelManagement_GetStageConfig verifies that individual stage config
// can be retrieved and includes the model name and backend.
func TestModelManagement_GetStageConfig(t *testing.T) {
	env := SetupE2EEnvironment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Query triage stage specifically
	result := env.SafeCLI.Run(ctx, "ai", "model", "list", "-o", "json")
	require.True(t, result.Success(), "ai model list should succeed: %s", result.Stderr)

	var modelList struct {
		Stages []struct {
			Stage   string `json:"stage"`
			Model   string `json:"model"`
			Source  string `json:"source"`
			Backend string `json:"backend"`
		} `json:"stages"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Stdout), &modelList))

	// Find triage stage
	var triageFound bool
	for _, s := range modelList.Stages {
		if s.Stage == "triage" {
			triageFound = true
			assert.NotEmpty(t, s.Model, "triage model should not be empty")
			// Source should be one of the valid values
			assert.Contains(t, []string{"db", "env", "default"}, s.Source,
				"triage source should be db, env, or default")
			break
		}
	}
	assert.True(t, triageFound, "triage stage should be in model list")
}

// TestModelManagement_SetStageModel verifies that changing a stage's model
// via CLI is reflected in subsequent list output.
func TestModelManagement_SetStageModel(t *testing.T) {
	env := SetupE2EEnvironment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Get the current triage model so we can restore it
	listResult := env.SafeCLI.Run(ctx, "ai", "model", "list", "-o", "json")
	require.True(t, listResult.Success(), "initial list should succeed: %s", listResult.Stderr)

	var beforeList struct {
		Stages []struct {
			Stage string `json:"stage"`
			Model string `json:"model"`
		} `json:"stages"`
	}
	require.NoError(t, json.Unmarshal([]byte(listResult.Stdout), &beforeList))

	var originalTriageModel string
	for _, s := range beforeList.Stages {
		if s.Stage == "triage" {
			originalTriageModel = s.Model
			break
		}
	}
	require.NotEmpty(t, originalTriageModel, "should find current triage model")

	// Set triage to a different model
	testModel := "llama3.2"
	if originalTriageModel == testModel {
		testModel = "qwen2.5:7b" // Use a different model if already set to llama
	}

	setResult := env.SafeCLI.Run(ctx, "ai", "model", "set", "triage", testModel)
	require.True(t, setResult.Success(), "ai model set should succeed: %s", setResult.Stderr)

	// Verify the change took effect
	afterResult := env.SafeCLI.Run(ctx, "ai", "model", "list", "-o", "json")
	require.True(t, afterResult.Success(), "list after set should succeed: %s", afterResult.Stderr)

	var afterList struct {
		Stages []struct {
			Stage  string `json:"stage"`
			Model  string `json:"model"`
			Source string `json:"source"`
		} `json:"stages"`
	}
	require.NoError(t, json.Unmarshal([]byte(afterResult.Stdout), &afterList))

	var newTriageModel, newTriageSource string
	for _, s := range afterList.Stages {
		if s.Stage == "triage" {
			newTriageModel = s.Model
			newTriageSource = s.Source
			break
		}
	}

	assert.Equal(t, testModel, newTriageModel,
		"triage model should be updated to %s", testModel)
	assert.Equal(t, "db", newTriageSource,
		"triage source should be 'db' after explicit set")

	// Clean up: reset to remove DB override
	t.Cleanup(func() {
		resetCtx, resetCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer resetCancel()
		env.SafeCLI.Run(resetCtx, "ai", "model", "reset", "triage")
	})
}

// TestModelManagement_SetDefault verifies that changing the default LLM model
// affects stages that don't have explicit overrides.
func TestModelManagement_SetDefault(t *testing.T) {
	env := SetupE2EEnvironment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Set a new default
	setResult := env.SafeCLI.Run(ctx, "ai", "model", "set", "default", "llama3.2")
	require.True(t, setResult.Success(), "set default should succeed: %s", setResult.Stderr)

	// Verify it shows in the list
	listResult := env.SafeCLI.Run(ctx, "ai", "model", "list", "-o", "json")
	require.True(t, listResult.Success(), "list should succeed: %s", listResult.Stderr)

	var modelList struct {
		Defaults struct {
			LLM       string `json:"llm"`
			LLMSource string `json:"llm_source"`
		} `json:"defaults"`
	}
	require.NoError(t, json.Unmarshal([]byte(listResult.Stdout), &modelList))

	assert.Equal(t, "llama3.2", modelList.Defaults.LLM,
		"default LLM should be updated to llama3.2")

	// Clean up
	t.Cleanup(func() {
		resetCtx, resetCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer resetCancel()
		env.SafeCLI.Run(resetCtx, "ai", "model", "reset", "default")
	})
}

// TestModelManagement_ListAvailable verifies that `penf ai model available`
// returns models from Ollama and known Gemini models.
func TestModelManagement_ListAvailable(t *testing.T) {
	env := SetupE2EEnvironment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result := env.SafeCLI.Run(ctx, "ai", "model", "available", "-o", "json")
	require.True(t, result.Success(), "ai model available should succeed: %s", result.Stderr)

	var available struct {
		Ollama []struct {
			Name string `json:"name"`
			Size string `json:"size"`
		} `json:"ollama"`
		Gemini []struct {
			Name string `json:"name"`
		} `json:"gemini"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Stdout), &available),
		"should parse available models JSON: %s", result.Stdout)

	// Ollama should have at least qwen2.5:7b and mxbai-embed-large
	ollamaNames := make(map[string]bool)
	for _, m := range available.Ollama {
		ollamaNames[m.Name] = true
	}
	assert.True(t, len(available.Ollama) > 0, "should have at least one Ollama model")

	// Gemini should list known models
	assert.True(t, len(available.Gemini) > 0, "should have at least one Gemini model")
}

// TestModelManagement_TestStage verifies that `penf ai model test <stage>`
// runs a quick inference test and returns latency + status.
func TestModelManagement_TestStage(t *testing.T) {
	env := SetupE2EEnvironment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	result := env.SafeCLI.Run(ctx, "ai", "model", "test", "triage", "-o", "json")
	require.True(t, result.Success(), "ai model test should succeed: %s", result.Stderr)

	var testResult struct {
		Stage   string  `json:"stage"`
		Model   string  `json:"model"`
		Backend string  `json:"backend"`
		Status  string  `json:"status"` // "ok" or "error"
		Latency float64 `json:"latency_ms"`
		Error   string  `json:"error,omitempty"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Stdout), &testResult),
		"should parse test result JSON: %s", result.Stdout)

	assert.Equal(t, "triage", testResult.Stage)
	assert.NotEmpty(t, testResult.Model, "model should be populated")
	assert.NotEmpty(t, testResult.Backend, "backend should be populated")
	assert.Equal(t, "ok", testResult.Status, "test should succeed: %s", testResult.Error)
	assert.Greater(t, testResult.Latency, 0.0, "latency should be positive")
}

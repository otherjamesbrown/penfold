// Package server provides unit tests for per-stage model selection and backend routing.
package server

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/ai/v1"
	"github.com/otherjamesbrown/penfold/services/ai/backend"
)

// =============================================================================
// Backend Routing Tests
// =============================================================================

// TestSelectBackend_GeminiModels tests that gemini-* models route to Gemini backend.
// Acceptance criteria: Backend routing - "gemini-*" → Gemini backend.
func TestSelectBackend_GeminiModels(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		wantBackendType string
	}{
		{
			name:      "gemini-2.0-flash routes to Gemini",
			model:     "gemini-2.0-flash",
			wantBackendType: "gemini",
		},
		{
			name:      "gemini-2.5-pro routes to Gemini",
			model:     "gemini-2.5-pro",
			wantBackendType: "gemini",
		},
		{
			name:      "gemini-2.0-flash-thinking routes to Gemini",
			model:     "gemini-2.0-flash-thinking",
			wantBackendType: "gemini",
		},
		{
			name:      "GEMINI-2.0-FLASH case-insensitive",
			model:     "GEMINI-2.0-FLASH",
			wantBackendType: "gemini",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// selectBackend should return "gemini" for gemini-* models
			backendType := selectBackend(tt.model)
			assert.Equal(t, tt.wantBackendType, backendType, "Model %q should route to Gemini backend", tt.model)
		})
	}
}

// TestSelectBackend_LocalModels tests that non-gemini models route to Ollama/local backend.
// Acceptance criteria: Backend routing - everything else → local/Ollama backend.
func TestSelectBackend_LocalModels(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		wantBackendType string
	}{
		{
			name:      "qwen2.5:7b routes to Ollama",
			model:     "qwen2.5:7b",
			wantBackendType: "ollama",
		},
		{
			name:      "llama3.2 routes to Ollama",
			model:     "llama3.2",
			wantBackendType: "ollama",
		},
		{
			name:      "mxbai-embed-large-v1 routes to Ollama",
			model:     "mxbai-embed-large-v1",
			wantBackendType: "ollama",
		},
		{
			name:      "custom-model routes to Ollama",
			model:     "custom-model",
			wantBackendType: "ollama",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// selectBackend should return "ollama" for non-gemini models
			backendType := selectBackend(tt.model)
			assert.Equal(t, tt.wantBackendType, backendType, "Model %q should route to Ollama backend", tt.model)
		})
	}
}

// =============================================================================
// Model Resolution in Handlers Tests
// =============================================================================

// TestTriageContent_ModelResolution tests model resolution order in TriageContent handler.
// Acceptance criteria: Model resolution - req.GetModel() → config stage default → global default → hardcoded fallback.
func TestTriageContent_ModelResolution(t *testing.T) {
	tests := []struct {
		name          string
		requestModel  *string
		configStage   string
		expectedModel string
		description   string
	}{
		{
			name:          "explicit request model takes precedence",
			requestModel:  stringPtr("custom-triage-model"),
			configStage:   "triage",
			expectedModel: "custom-triage-model",
			description:   "Request model should override config",
		},
		{
			name:          "falls back to config stage default when no request model",
			requestModel:  nil,
			configStage:   "triage",
			expectedModel: "", // Will be determined by config in actual implementation
			description:   "Should use config.ModelForStage('triage') when request model is nil",
		},
		{
			name:          "empty request model falls back to config",
			requestModel:  stringPtr(""),
			configStage:   "triage",
			expectedModel: "", // Will be determined by config in actual implementation
			description:   "Empty string request model should fall back to config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock backend that captures which model was used
			var capturedModel string
			be := &mockBackend{
				chatCompletionFunc: func(ctx context.Context, messages []backend.Message, opts backend.CompletionOptions) (*backend.CompletionResult, error) {
					capturedModel = opts.Model
					return &backend.CompletionResult{
						Content: `{"category": "PROJECT_UPDATE", "importance": "MEDIUM", "reason": "test"}`,
						Model:   opts.Model,
					}, nil
				},
			}
			server := newTestServer(be)

			req := &aiv1.TriageContentRequest{
				Content:  "Test content for triage",
				TenantId: stringPtr("tenant-1"),
			}
			if tt.requestModel != nil {
				req.Model = tt.requestModel
			}

			_, err := server.TriageContent(context.Background(), req)
			require.NoError(t, err)

			// Verify the backend received the expected model
			if tt.requestModel != nil && *tt.requestModel != "" {
				assert.Equal(t, *tt.requestModel, capturedModel, "Backend should receive explicit request model")
			} else {
				// Should have received model from config.ModelForStage("triage")
				assert.NotEmpty(t, capturedModel, "Backend should receive a model from config fallback")
			}
		})
	}
}

// TestExtractEntities_ModelResolution tests model resolution in ExtractEntities handler.
// Acceptance criteria: Each handler resolves model as: req.GetModel() → config stage default → global default → hardcoded fallback.
func TestExtractEntities_ModelResolution(t *testing.T) {
	var capturedModel string
	be := &mockBackend{
		chatCompletionFunc: func(ctx context.Context, messages []backend.Message, opts backend.CompletionOptions) (*backend.CompletionResult, error) {
			capturedModel = opts.Model
			return &backend.CompletionResult{
				Content: `{"entities": []}`,
				Model:   opts.Model,
			}, nil
		},
	}
	server := newTestServer(be)

	tests := []struct {
		name         string
		requestModel *string
	}{
		{
			name:         "explicit model in request",
			requestModel: stringPtr("custom-entity-model"),
		},
		{
			name:         "nil model falls back to config",
			requestModel: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capturedModel = "" // Reset

			req := &aiv1.ExtractEntitiesRequest{
				Content:  "Extract entities from this text",
				TenantId: stringPtr("tenant-1"),
			}
			if tt.requestModel != nil {
				req.Model = tt.requestModel
			}

			_, err := server.ExtractEntities(context.Background(), req)
			require.NoError(t, err)

			if tt.requestModel != nil {
				assert.Equal(t, *tt.requestModel, capturedModel, "Backend should receive explicit request model")
			} else {
				// Should receive model from config.ModelForStage("extract_entities")
				assert.NotEmpty(t, capturedModel, "Backend should receive model from config")
			}
		})
	}
}

// TestExtractAssertions_ModelResolution tests model resolution in ExtractAssertions handler.
func TestExtractAssertions_ModelResolution(t *testing.T) {
	var capturedModel string
	be := &mockBackend{
		chatCompletionFunc: func(ctx context.Context, messages []backend.Message, opts backend.CompletionOptions) (*backend.CompletionResult, error) {
			capturedModel = opts.Model
			return &backend.CompletionResult{
				Content: `{"assertions": []}`,
				Model:   opts.Model,
			}, nil
		},
	}
	server := newTestServer(be)

	req := &aiv1.AssertionRequest{
		Content:  "Content for assertion extraction",
		TenantId: stringPtr("tenant-1"),
		Model:    stringPtr("custom-assertion-model"),
	}

	_, err := server.ExtractAssertions(context.Background(), req)
	require.NoError(t, err)

	assert.Equal(t, "custom-assertion-model", capturedModel, "Backend should receive request model")
}

// TestGenerateEmbedding_ModelResolution tests model resolution in GenerateEmbedding handler.
// Acceptance criteria: Embedding handler uses config.ModelForStage("embedding").
func TestGenerateEmbedding_ModelResolution(t *testing.T) {
	var capturedModel string
	be := &mockBackend{
		generateEmbeddingFunc: func(ctx context.Context, text string, model string) (*backend.EmbeddingResult, error) {
			capturedModel = model
			return &backend.EmbeddingResult{
				Vector:     []float32{0.1, 0.2, 0.3},
				Dimensions: 3,
				Model:      model,
			}, nil
		},
	}
	server := newTestServer(be)

	tests := []struct {
		name         string
		requestModel *string
	}{
		{
			name:         "explicit embedding model",
			requestModel: stringPtr("custom-embed-model"),
		},
		{
			name:         "nil model falls back to config",
			requestModel: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capturedModel = "" // Reset

			req := &aiv1.EmbeddingRequest{
				Text:     "Text to embed",
				TenantId: stringPtr("tenant-1"),
			}
			if tt.requestModel != nil {
				req.Model = tt.requestModel
			}

			_, err := server.GenerateEmbedding(context.Background(), req)
			require.NoError(t, err)

			if tt.requestModel != nil {
				assert.Equal(t, *tt.requestModel, capturedModel, "Backend should receive explicit request model")
			} else {
				// Should receive model from config.ModelForStage("embedding")
				assert.NotEmpty(t, capturedModel, "Backend should receive model from config")
			}
		})
	}
}

// =============================================================================
// Deep Analysis Model Selection Integration Tests
// =============================================================================

// TestDeepAnalyze_ModelResolutionWithTriageMetadata tests integration with existing selectModelForDeepAnalysis.
// Acceptance criteria: selectModelForDeepAnalysis() still works but falls back to config stage default when no triage metadata.
func TestDeepAnalyze_ModelResolutionWithTriageMetadata(t *testing.T) {
	tests := []struct {
		name             string
		triageCategory   string
		triageImportance string
		requestModel     *string
		expectedModel    string
	}{
		{
			name:             "RISK_ISSUE upgrades to Pro",
			triageCategory:   "RISK_ISSUE",
			triageImportance: "MEDIUM",
			requestModel:     nil,
			expectedModel:    "gemini-2.5-pro",
		},
		{
			name:             "CUSTOMER + HIGH upgrades to Pro",
			triageCategory:   "CUSTOMER",
			triageImportance: "HIGH",
			requestModel:     nil,
			expectedModel:    "gemini-2.5-pro",
		},
		{
			name:             "PROJECT_UPDATE + MEDIUM uses Flash",
			triageCategory:   "PROJECT_UPDATE",
			triageImportance: "MEDIUM",
			requestModel:     nil,
			expectedModel:    "gemini-2.0-flash",
		},
		{
			name:             "explicit request model overrides triage",
			triageCategory:   "RISK_ISSUE",
			triageImportance: "HIGH",
			requestModel:     stringPtr("custom-override-model"),
			expectedModel:    "custom-override-model",
		},
		{
			name:             "no triage metadata falls back to config default",
			triageCategory:   "",
			triageImportance: "",
			requestModel:     nil,
			expectedModel:    "", // Should fall back to config.ModelForStage("deep_analyze")
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedModel string
			be := &mockBackend{
				chatCompletionFunc: func(ctx context.Context, messages []backend.Message, opts backend.CompletionOptions) (*backend.CompletionResult, error) {
					capturedModel = opts.Model
					return &backend.CompletionResult{
						Content: `{"summary": "test", "sentiment": {"score": 0.5, "label": "neutral", "confidence": 0.8}}`,
						Model:   opts.Model,
					}, nil
				},
			}
			server := newTestServer(be)

			req := &aiv1.DeepAnalyzeRequest{
				Content:          "Content to analyze",
				TenantId:         stringPtr("tenant-1"),
				TriageCategory:   tt.triageCategory,
				TriageImportance: tt.triageImportance,
			}
			if tt.requestModel != nil {
				req.Model = tt.requestModel
			}

			_, err := server.DeepAnalyze(context.Background(), req)
			require.NoError(t, err)

			if tt.expectedModel != "" {
				assert.Equal(t, tt.expectedModel, capturedModel, "Backend should receive expected model")
			} else {
				// Should have received config.ModelForStage("deep_analyze")
				assert.NotEmpty(t, capturedModel, "Backend should receive model from config when no triage metadata")
			}
		})
	}
}

// TestSelectModelForDeepAnalysis_FallbackToConfig tests that selectModelForDeepAnalysis
// falls back to config stage default when triage metadata is missing.
// Acceptance criteria: selectModelForDeepAnalysis() falls back to config stage default instead of hardcoded when no triage metadata.
func TestSelectModelForDeepAnalysis_FallbackToConfig(t *testing.T) {
	tests := []struct {
		name             string
		category         string
		importance       string
		requestedModel   string
		configDefault    string
		expectedModel    string
	}{
		{
			name:           "no triage metadata with config default",
			category:       "",
			importance:     "",
			requestedModel: "",
			configDefault:  "gemini-2.0-flash-exp",
			expectedModel:  "gemini-2.0-flash-exp",
		},
		{
			name:           "empty category/importance with config default",
			category:       "",
			importance:     "",
			requestedModel: "",
			configDefault:  "qwen2.5:7b",
			expectedModel:  "qwen2.5:7b",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When no category/importance and no requested model, should use config default
			result := selectModelForDeepAnalysis(tt.category, tt.importance, tt.requestedModel, tt.configDefault)

			// Verify it returns the config default when no triage metadata
			if tt.configDefault != "" && tt.category == "" && tt.importance == "" && tt.requestedModel == "" {
				assert.Equal(t, tt.expectedModel, result,
					"selectModelForDeepAnalysis should fall back to config stage default, not hardcoded default")
			}
		})
	}
}

// =============================================================================
// Integration Tests - End-to-End Model Selection Flow
// =============================================================================

// TestModelSelectionFlow_TriageStage tests complete flow for triage stage.
// Acceptance criteria: Changing AI_MODEL_TRIAGE in env and restarting switches triage to new model.
func TestModelSelectionFlow_TriageStage(t *testing.T) {
	t.Setenv("AI_MODEL_TRIAGE", "qwen2.5:7b")

	var capturedModel string
	be := &mockBackend{
		chatCompletionFunc: func(ctx context.Context, messages []backend.Message, opts backend.CompletionOptions) (*backend.CompletionResult, error) {
			capturedModel = opts.Model
			return &backend.CompletionResult{
				Content: `{"category": "PROJECT_UPDATE", "importance": "MEDIUM", "reason": "test"}`,
				Model:   opts.Model,
			}, nil
		},
	}
	server := newTestServerWithEnvConfig(t, be)

	req := &aiv1.TriageContentRequest{
		Content:  "Content to triage",
		TenantId: stringPtr("tenant-1"),
		// No explicit model - should use config
	}

	_, err := server.TriageContent(context.Background(), req)
	require.NoError(t, err)

	// Should use model from AI_MODEL_TRIAGE env var
	assert.Equal(t, "qwen2.5:7b", capturedModel,
		"Triage should use model from AI_MODEL_TRIAGE after config reload")
}

// TestModelSelectionFlow_MultipleStages tests that different stages use different models.
// Acceptance criteria: Each pipeline stage uses its configured model.
func TestModelSelectionFlow_MultipleStages(t *testing.T) {
	t.Setenv("AI_MODEL_TRIAGE", "triage-model")
	t.Setenv("AI_MODEL_EXTRACT_ENTITIES", "entity-model")
	t.Setenv("AI_MODEL_EXTRACT_ASSERTIONS", "assertion-model")
	t.Setenv("AI_MODEL_EMBEDDING", "embed-model")

	capturedModels := make(map[string]string)

	be := &mockBackend{
		chatCompletionFunc: func(ctx context.Context, messages []backend.Message, opts backend.CompletionOptions) (*backend.CompletionResult, error) {
			// Determine which handler based on response format expected
			content := messages[0].Content
			// Triage has specific JSON structure with category/importance/reason
			if strings.Contains(content, "content_contribution") || (strings.Contains(content, "category") && strings.Contains(content, "importance")) {
				capturedModels["triage"] = opts.Model
				return &backend.CompletionResult{
					Content: `{"category": "PROJECT_UPDATE", "importance": "MEDIUM", "reason": "test"}`,
					Model:   opts.Model,
				}, nil
			}
			// Entities extraction asks for people/dates/projects/organisations
			if strings.Contains(content, "people") && strings.Contains(content, "dates") && strings.Contains(content, "projects") {
				capturedModels["entities"] = opts.Model
				return &backend.CompletionResult{
					Content: `{"people": [], "dates": [], "projects": [], "organisations": []}`,
					Model:   opts.Model,
				}, nil
			}
			// Assertions have subject/predicate/object structure
			if strings.Contains(content, "subject") && strings.Contains(content, "predicate") && strings.Contains(content, "object") {
				capturedModels["assertions"] = opts.Model
				return &backend.CompletionResult{
					Content: `{"assertions": []}`,
					Model:   opts.Model,
				}, nil
			}
			return &backend.CompletionResult{Content: "{}", Model: opts.Model}, nil
		},
		generateEmbeddingFunc: func(ctx context.Context, text string, model string) (*backend.EmbeddingResult, error) {
			capturedModels["embedding"] = model
			return &backend.EmbeddingResult{
				Vector:     []float32{0.1, 0.2, 0.3},
				Dimensions: 3,
				Model:      model,
			}, nil
		},
	}
	server := newTestServerWithEnvConfig(t, be)

	// Call each handler
	server.TriageContent(context.Background(), &aiv1.TriageContentRequest{
		Content:  "triage content",
		TenantId: stringPtr("tenant-1"),
	})
	server.ExtractEntities(context.Background(), &aiv1.ExtractEntitiesRequest{
		Content:  "extract entities",
		TenantId: stringPtr("tenant-1"),
	})
	server.ExtractAssertions(context.Background(), &aiv1.AssertionRequest{
		Content:  "extract assertions",
		TenantId: stringPtr("tenant-1"),
	})
	server.GenerateEmbedding(context.Background(), &aiv1.EmbeddingRequest{
		Text:     "embed this",
		TenantId: stringPtr("tenant-1"),
	})

	// Verify each stage used its configured model
	assert.Equal(t, "triage-model", capturedModels["triage"], "Triage stage should use AI_MODEL_TRIAGE")
	assert.Equal(t, "entity-model", capturedModels["entities"], "Entity extraction should use AI_MODEL_EXTRACT_ENTITIES")
	assert.Equal(t, "assertion-model", capturedModels["assertions"], "Assertion extraction should use AI_MODEL_EXTRACT_ASSERTIONS")
	assert.Equal(t, "embed-model", capturedModels["embedding"], "Embedding should use AI_MODEL_EMBEDDING")
}

// TestBackendRoutingFlow_MixedModels tests that different models route to correct backends.
// Acceptance criteria: Local model names go to Ollama URL, gemini-* goes to Gemini API.
func TestBackendRoutingFlow_MixedModels(t *testing.T) {
	t.Setenv("AI_MODEL_TRIAGE", "gemini-2.0-flash")
	t.Setenv("AI_MODEL_EXTRACT_ENTITIES", "qwen2.5:7b")
	t.Setenv("AI_OLLAMA_URL", "http://dev01.brown.chat:11434")

	// In actual implementation, backend selection would happen in a composite backend
	// or router. This test verifies the selectBackend logic.

	triageBackend := selectBackend("gemini-2.0-flash")
	assert.Equal(t, "gemini", triageBackend, "gemini-2.0-flash should route to Gemini backend")

	entitiesBackend := selectBackend("qwen2.5:7b")
	assert.Equal(t, "ollama", entitiesBackend, "qwen2.5:7b should route to Ollama backend")
}

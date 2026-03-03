package server

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/ai/v1"
	"github.com/otherjamesbrown/penfold/services/ai/backend"
	"github.com/otherjamesbrown/penfold/services/ai/registry"
)

// mockRoutingRepo implements registry.Repository with only routing rule support.
// All non-routing methods return errors (they're not needed for these tests).
type mockRoutingRepo struct {
	rulesByTask map[string][]*registry.RoutingRule
	queryErr    error
}

func (m *mockRoutingRepo) GetRoutingRulesByTask(_ context.Context, taskType string) ([]*registry.RoutingRule, error) {
	if m.queryErr != nil {
		return nil, m.queryErr
	}
	return m.rulesByTask[taskType], nil
}

// Unused repository methods — return errors to fail fast if called unexpectedly.
func (m *mockRoutingRepo) CreateModel(context.Context, *registry.ModelConfig) error       { return errors.New("not implemented") }
func (m *mockRoutingRepo) GetModel(context.Context, string) (*registry.ModelConfig, error) { return nil, errors.New("not implemented") }
func (m *mockRoutingRepo) UpdateModel(context.Context, *registry.ModelConfig) error       { return errors.New("not implemented") }
func (m *mockRoutingRepo) DeleteModel(context.Context, string) error                      { return errors.New("not implemented") }
func (m *mockRoutingRepo) ListModels(context.Context, *registry.ModelFilter) ([]*registry.ModelConfig, error) { return nil, nil }
func (m *mockRoutingRepo) GetHealth(context.Context, string) (*registry.ModelHealth, error) { return nil, errors.New("not implemented") }
func (m *mockRoutingRepo) UpdateHealth(context.Context, string, *registry.ModelHealth) error { return errors.New("not implemented") }
func (m *mockRoutingRepo) GetModelContextWindowByName(context.Context, string) (int, error) { return 0, errors.New("not implemented") }
func (m *mockRoutingRepo) GetModelProviderByName(context.Context, string) (string, error) { return "", errors.New("not implemented") }
func (m *mockRoutingRepo) GetRoutingRules(context.Context) ([]*registry.RoutingRule, error) { return nil, errors.New("not implemented") }
func (m *mockRoutingRepo) GetRoutingRuleByName(context.Context, string) (*registry.RoutingRule, error) { return nil, errors.New("not implemented") }
func (m *mockRoutingRepo) CreateRoutingRule(context.Context, *registry.RoutingRule) error { return errors.New("not implemented") }
func (m *mockRoutingRepo) UpdateRoutingRule(context.Context, *registry.RoutingRule) error { return errors.New("not implemented") }
func (m *mockRoutingRepo) DeleteRoutingRule(context.Context, string) error               { return errors.New("not implemented") }

// newTestServerWithRegistry creates a test server with a mock registry containing routing rules.
func newTestServerWithRegistry(be backend.Backend, rulesByTask map[string][]*registry.RoutingRule) *AIServer {
	cfg := testConfig()
	logger := testLogger()
	repo := &mockRoutingRepo{rulesByTask: rulesByTask}
	reg := registry.NewDBRegistry(repo, nil)
	return NewAIServerWithAll(cfg, logger, be, nil, reg)
}

// =============================================================================
// resolveEmbeddingModel Tests
// =============================================================================

func TestResolveEmbeddingModel_RoutingRuleFound(t *testing.T) {
	server := newTestServerWithRegistry(&mockBackend{}, map[string][]*registry.RoutingRule{
		"embedding": {
			{
				Name:            "embeddings-local",
				TaskType:        "embedding",
				PreferredModels: []string{"ollama/mxbai-embed-large"},
				FallbackModels:  []string{"gemini/text-embedding-004"},
				Priority:        10,
				IsEnabled:       true,
			},
		},
	})

	model, fallbacks, ruleName := server.resolveEmbeddingModel(context.Background())

	assert.Equal(t, "mxbai-embed-large", model, "should use preferred model from routing rule")
	assert.Equal(t, []string{"text-embedding-004"}, fallbacks, "should return fallback models")
	assert.Equal(t, "embeddings-local", ruleName, "should return rule name")
}

func TestResolveEmbeddingModel_NoRoutingRule_FallsBackToConfig(t *testing.T) {
	// Server with registry but no embedding rules
	server := newTestServerWithRegistry(&mockBackend{}, map[string][]*registry.RoutingRule{})

	model, fallbacks, ruleName := server.resolveEmbeddingModel(context.Background())

	// Should fall back to config.ModelForStage("embed") which returns DefaultEmbeddingModel
	assert.NotEmpty(t, model, "should return a model from config fallback")
	assert.Nil(t, fallbacks, "should have no fallbacks when using config chain")
	assert.Empty(t, ruleName, "should have no rule name when using config chain")
}

func TestResolveEmbeddingModel_NilRegistry_FallsBackToConfig(t *testing.T) {
	// Server without registry
	server := newTestServer(&mockBackend{})

	model, fallbacks, ruleName := server.resolveEmbeddingModel(context.Background())

	assert.NotEmpty(t, model, "should return a model from config fallback")
	assert.Nil(t, fallbacks, "should have no fallbacks")
	assert.Empty(t, ruleName, "should have no rule name")
}

func TestResolveEmbeddingModel_RegistryQueryError_FallsBackToConfig(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	repo := &mockRoutingRepo{queryErr: errors.New("connection refused")}
	reg := registry.NewDBRegistry(repo, nil)
	server := NewAIServerWithAll(cfg, logger, &mockBackend{}, nil, reg)

	model, fallbacks, ruleName := server.resolveEmbeddingModel(context.Background())

	assert.NotEmpty(t, model, "should fall back to config on DB error")
	assert.Nil(t, fallbacks)
	assert.Empty(t, ruleName)
}

func TestResolveEmbeddingModel_StripsProviderPrefix(t *testing.T) {
	server := newTestServerWithRegistry(&mockBackend{}, map[string][]*registry.RoutingRule{
		"embedding": {
			{
				Name:            "test-rule",
				TaskType:        "embedding",
				PreferredModels: []string{"ollama/mxbai-embed-large"},
				FallbackModels:  []string{"gemini/text-embedding-004", "openai/text-embedding-3-small"},
				Priority:        10,
				IsEnabled:       true,
			},
		},
	})

	model, fallbacks, _ := server.resolveEmbeddingModel(context.Background())

	assert.Equal(t, "mxbai-embed-large", model, "should strip provider prefix from preferred model")
	assert.Equal(t, []string{"text-embedding-004", "text-embedding-3-small"}, fallbacks,
		"should strip provider prefix from all fallback models")
}

// =============================================================================
// GenerateEmbedding with Routing Rules Tests
// =============================================================================

func TestGenerateEmbedding_UsesRoutingRule(t *testing.T) {
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
	server := newTestServerWithRegistry(be, map[string][]*registry.RoutingRule{
		"embedding": {
			{
				Name:            "embeddings-local",
				TaskType:        "embedding",
				PreferredModels: []string{"ollama/mxbai-embed-large"},
				FallbackModels:  []string{"gemini/text-embedding-004"},
				Priority:        10,
				IsEnabled:       true,
			},
		},
	})

	resp, err := server.GenerateEmbedding(context.Background(), &aiv1.EmbeddingRequest{
		Text:     "test text",
		TenantId: stringPtr("tenant-1"),
	})

	require.NoError(t, err)
	assert.Equal(t, "mxbai-embed-large", capturedModel,
		"should use preferred model from routing rule, not config default")
	assert.Equal(t, "mxbai-embed-large", resp.ModelUsed)
}

func TestGenerateEmbedding_ExplicitModelOverridesRoutingRule(t *testing.T) {
	var capturedModel string
	be := &mockBackend{
		generateEmbeddingFunc: func(ctx context.Context, text string, model string) (*backend.EmbeddingResult, error) {
			capturedModel = model
			return &backend.EmbeddingResult{
				Vector:     []float32{0.1},
				Dimensions: 1,
				Model:      model,
			}, nil
		},
	}
	server := newTestServerWithRegistry(be, map[string][]*registry.RoutingRule{
		"embedding": {
			{
				Name:            "embeddings-local",
				TaskType:        "embedding",
				PreferredModels: []string{"ollama/mxbai-embed-large"},
				Priority:        10,
				IsEnabled:       true,
			},
		},
	})

	_, err := server.GenerateEmbedding(context.Background(), &aiv1.EmbeddingRequest{
		Text:     "test text",
		TenantId: stringPtr("tenant-1"),
		Model:    stringPtr("custom-embed-model"),
	})

	require.NoError(t, err)
	assert.Equal(t, "custom-embed-model", capturedModel,
		"explicit request model should override routing rule")
}

func TestGenerateEmbedding_FallbackOnPreferredModelFailure(t *testing.T) {
	callCount := 0
	be := &mockBackend{
		generateEmbeddingFunc: func(ctx context.Context, text string, model string) (*backend.EmbeddingResult, error) {
			callCount++
			if model == "mxbai-embed-large" {
				return nil, backend.ErrServiceUnavailable
			}
			// Fallback model succeeds
			return &backend.EmbeddingResult{
				Vector:     []float32{0.1, 0.2},
				Dimensions: 2,
				Model:      model,
			}, nil
		},
	}
	server := newTestServerWithRegistry(be, map[string][]*registry.RoutingRule{
		"embedding": {
			{
				Name:            "embeddings-local",
				TaskType:        "embedding",
				PreferredModels: []string{"ollama/mxbai-embed-large"},
				FallbackModels:  []string{"gemini/text-embedding-004"},
				Priority:        10,
				IsEnabled:       true,
			},
		},
	})

	resp, err := server.GenerateEmbedding(context.Background(), &aiv1.EmbeddingRequest{
		Text:     "test text",
		TenantId: stringPtr("tenant-1"),
	})

	require.NoError(t, err)
	assert.Equal(t, 2, callCount, "should have tried preferred then fallback")
	assert.Equal(t, "text-embedding-004", resp.ModelUsed,
		"should use fallback model when preferred fails")
}

func TestGenerateEmbedding_AllModelsFailReturnsError(t *testing.T) {
	be := &mockBackend{
		generateEmbeddingFunc: func(ctx context.Context, text string, model string) (*backend.EmbeddingResult, error) {
			return nil, backend.ErrServiceUnavailable
		},
	}
	server := newTestServerWithRegistry(be, map[string][]*registry.RoutingRule{
		"embedding": {
			{
				Name:            "embeddings-local",
				TaskType:        "embedding",
				PreferredModels: []string{"ollama/mxbai-embed-large"},
				FallbackModels:  []string{"gemini/text-embedding-004"},
				Priority:        10,
				IsEnabled:       true,
			},
		},
	})

	_, err := server.GenerateEmbedding(context.Background(), &aiv1.EmbeddingRequest{
		Text:     "test text",
		TenantId: stringPtr("tenant-1"),
	})

	require.Error(t, err, "should return error when all models fail")
}

// =============================================================================
// stripProviderPrefix Tests
// =============================================================================

func TestStripProviderPrefix(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"ollama/mxbai-embed-large", "mxbai-embed-large"},
		{"gemini/text-embedding-004", "text-embedding-004"},
		{"openai/text-embedding-3-small", "text-embedding-3-small"},
		{"mxbai-embed-large", "mxbai-embed-large"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, stripProviderPrefix(tt.input))
		})
	}
}

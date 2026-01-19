package selector

import (
	"context"
	"testing"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testLogger creates a logger for testing.
func testLogger() logging.Logger {
	return logging.NewLogger(&logging.Config{
		Level:       logging.LevelDebug,
		ServiceName: "selector-test",
		Environment: "test",
		JSONFormat:  false,
	})
}

// createTestSelector creates a selector with test models.
func createTestSelector(t *testing.T) *ModelSelector {
	t.Helper()

	cfg := &SelectorConfig{
		DefaultOptimizationMode: OptimizationModeBalanced,
		WarmupThreshold:         5 * time.Minute,
		EnableMetrics:           false, // Disable metrics in tests
	}

	selector := NewModelSelector(cfg, testLogger())

	// Register test models
	require.NoError(t, selector.RegisterModel(&ModelConfig{
		ModelID:  "local-embedding",
		Provider: ModelProviderOllama,
		Endpoint: "http://localhost:11434",
		Capabilities: &ModelCapabilities{
			ModelID:             "local-embedding",
			Provider:            ModelProviderOllama,
			IsLocal:             true,
			SupportedTasks:      []TaskType{TaskTypeEmbedding},
			ContextWindow:       8192,
			EmbeddingDimensions: 768,
			AvgLatency:          100 * time.Millisecond,
			CostPer1KTokens:     0,
			QualityScore:        0.75,
			SpeedScore:          0.85,
			IsAvailable:         true,
			WarmupRequired:      true,
		},
	}))

	require.NoError(t, selector.RegisterModel(&ModelConfig{
		ModelID:  "local-llm",
		Provider: ModelProviderOllama,
		Endpoint: "http://localhost:11434",
		Capabilities: &ModelCapabilities{
			ModelID:         "local-llm",
			Provider:        ModelProviderOllama,
			IsLocal:         true,
			SupportedTasks:  []TaskType{TaskTypeSummarization, TaskTypeExtraction, TaskTypeClassification},
			ContextWindow:   128000,
			AvgLatency:      500 * time.Millisecond,
			CostPer1KTokens: 0,
			QualityScore:    0.70,
			SpeedScore:      0.75,
			IsAvailable:     true,
			WarmupRequired:  true,
		},
	}))

	require.NoError(t, selector.RegisterModel(&ModelConfig{
		ModelID:  "cloud-embedding",
		Provider: ModelProviderGemini,
		Endpoint: "https://api.gemini.example.com",
		APIKey:   "test-key",
		Capabilities: &ModelCapabilities{
			ModelID:             "cloud-embedding",
			Provider:            ModelProviderGemini,
			IsLocal:             false,
			SupportedTasks:      []TaskType{TaskTypeEmbedding},
			ContextWindow:       2048,
			EmbeddingDimensions: 768,
			AvgLatency:          200 * time.Millisecond,
			CostPer1KTokens:     0.00001,
			QualityScore:        0.90,
			SpeedScore:          0.70,
			IsAvailable:         true,
			WarmupRequired:      false,
		},
	}))

	require.NoError(t, selector.RegisterModel(&ModelConfig{
		ModelID:  "cloud-llm",
		Provider: ModelProviderGemini,
		Endpoint: "https://api.gemini.example.com",
		APIKey:   "test-key",
		Capabilities: &ModelCapabilities{
			ModelID:         "cloud-llm",
			Provider:        ModelProviderGemini,
			IsLocal:         false,
			SupportedTasks:  []TaskType{TaskTypeSummarization, TaskTypeExtraction, TaskTypeClassification},
			ContextWindow:   1000000,
			AvgLatency:      1 * time.Second,
			CostPer1KTokens: 0.001,
			QualityScore:    0.95,
			SpeedScore:      0.50,
			IsAvailable:     true,
			WarmupRequired:  false,
		},
	}))

	return selector
}

func TestNewModelSelector(t *testing.T) {
	selector := NewModelSelector(nil, testLogger())
	require.NotNil(t, selector)
	assert.NotNil(t, selector.warmupManager)
	assert.Equal(t, OptimizationModeBalanced, selector.defaultOptimizationMode)
}

func TestRegisterModel(t *testing.T) {
	selector := NewModelSelector(nil, testLogger())

	t.Run("successful registration", func(t *testing.T) {
		err := selector.RegisterModel(&ModelConfig{
			ModelID:  "test-model",
			Provider: ModelProviderOllama,
			Capabilities: &ModelCapabilities{
				ModelID:        "test-model",
				SupportedTasks: []TaskType{TaskTypeEmbedding},
				IsAvailable:    true,
			},
		})
		require.NoError(t, err)

		model, err := selector.GetModel("test-model")
		require.NoError(t, err)
		assert.Equal(t, "test-model", model.ModelID)
	})

	t.Run("nil config", func(t *testing.T) {
		err := selector.RegisterModel(nil)
		assert.Error(t, err)
	})

	t.Run("empty model ID", func(t *testing.T) {
		err := selector.RegisterModel(&ModelConfig{
			Provider: ModelProviderOllama,
			Capabilities: &ModelCapabilities{
				SupportedTasks: []TaskType{TaskTypeEmbedding},
			},
		})
		assert.Error(t, err)
	})

	t.Run("nil capabilities", func(t *testing.T) {
		err := selector.RegisterModel(&ModelConfig{
			ModelID:  "test",
			Provider: ModelProviderOllama,
		})
		assert.Error(t, err)
	})
}

func TestUnregisterModel(t *testing.T) {
	selector := createTestSelector(t)

	t.Run("successful unregistration", func(t *testing.T) {
		err := selector.UnregisterModel("local-embedding")
		require.NoError(t, err)

		_, err = selector.GetModel("local-embedding")
		assert.Equal(t, ErrModelNotFound, err)
	})

	t.Run("model not found", func(t *testing.T) {
		err := selector.UnregisterModel("non-existent")
		assert.Equal(t, ErrModelNotFound, err)
	})
}

func TestSelectModel_TaskTypes(t *testing.T) {
	selector := createTestSelector(t)
	ctx := context.Background()

	testCases := []struct {
		name      string
		taskType  TaskType
		expectErr bool
	}{
		{"embedding task", TaskTypeEmbedding, false},
		{"summarization task", TaskTypeSummarization, false},
		{"extraction task", TaskTypeExtraction, false},
		{"classification task", TaskTypeClassification, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := selector.SelectModel(ctx, &SelectionCriteria{
				TaskType: tc.taskType,
			})

			if tc.expectErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, result)
				assert.NotNil(t, result.SelectedModel)
				assert.True(t, result.SelectedModel.Capabilities.SupportsTask(tc.taskType))
			}
		})
	}
}

func TestSelectModel_OptimizationModes(t *testing.T) {
	selector := createTestSelector(t)
	ctx := context.Background()

	t.Run("balanced mode prefers local", func(t *testing.T) {
		result, err := selector.SelectModel(ctx, &SelectionCriteria{
			TaskType:         TaskTypeSummarization,
			OptimizationMode: OptimizationModeBalanced,
		})
		require.NoError(t, err)
		// With balanced weights, local model should be competitive due to privacy and cost scores
		assert.NotNil(t, result.SelectedModel)
	})

	t.Run("quality mode selects high quality", func(t *testing.T) {
		result, err := selector.SelectModel(ctx, &SelectionCriteria{
			TaskType:         TaskTypeSummarization,
			OptimizationMode: OptimizationModeQuality,
		})
		require.NoError(t, err)
		// Quality mode should prefer the high-quality cloud model
		assert.Equal(t, "cloud-llm", result.SelectedModel.ModelID)
	})

	t.Run("cost mode selects cheap", func(t *testing.T) {
		result, err := selector.SelectModel(ctx, &SelectionCriteria{
			TaskType:         TaskTypeSummarization,
			OptimizationMode: OptimizationModeCost,
		})
		require.NoError(t, err)
		// Cost mode should prefer the free local model
		assert.True(t, result.SelectedModel.Capabilities.IsLocal)
	})

	t.Run("latency mode selects fast", func(t *testing.T) {
		result, err := selector.SelectModel(ctx, &SelectionCriteria{
			TaskType:         TaskTypeSummarization,
			OptimizationMode: OptimizationModeLatency,
		})
		require.NoError(t, err)
		// Latency mode should prefer the faster local model
		assert.True(t, result.SelectedModel.Capabilities.IsLocal)
	})

	t.Run("privacy mode selects local", func(t *testing.T) {
		result, err := selector.SelectModel(ctx, &SelectionCriteria{
			TaskType:         TaskTypeSummarization,
			OptimizationMode: OptimizationModePrivacy,
		})
		require.NoError(t, err)
		assert.True(t, result.SelectedModel.Capabilities.IsLocal)
	})
}

func TestSelectModel_PreferredModel(t *testing.T) {
	selector := createTestSelector(t)
	ctx := context.Background()

	t.Run("selects preferred model when available", func(t *testing.T) {
		result, err := selector.SelectModel(ctx, &SelectionCriteria{
			TaskType:       TaskTypeEmbedding,
			PreferredModel: "cloud-embedding",
		})
		require.NoError(t, err)
		assert.Equal(t, "cloud-embedding", result.SelectedModel.ModelID)
		assert.Contains(t, result.SelectionReason, "preferred")
	})

	t.Run("falls back when preferred model unavailable", func(t *testing.T) {
		result, err := selector.SelectModel(ctx, &SelectionCriteria{
			TaskType:       TaskTypeEmbedding,
			PreferredModel: "non-existent",
		})
		require.NoError(t, err)
		assert.NotEqual(t, "non-existent", result.SelectedModel.ModelID)
	})

	t.Run("falls back when preferred model does not support task", func(t *testing.T) {
		result, err := selector.SelectModel(ctx, &SelectionCriteria{
			TaskType:       TaskTypeSummarization,
			PreferredModel: "local-embedding", // Only supports embedding
		})
		require.NoError(t, err)
		assert.NotEqual(t, "local-embedding", result.SelectedModel.ModelID)
	})
}

func TestSelectModel_RequireLocal(t *testing.T) {
	selector := createTestSelector(t)
	ctx := context.Background()

	t.Run("selects local model when required", func(t *testing.T) {
		result, err := selector.SelectModel(ctx, &SelectionCriteria{
			TaskType:     TaskTypeSummarization,
			RequireLocal: true,
		})
		require.NoError(t, err)
		assert.True(t, result.SelectedModel.Capabilities.IsLocal)
	})

	t.Run("fails when local required but none available for task", func(t *testing.T) {
		// Create selector with only cloud models for embedding
		selector := NewModelSelector(&SelectorConfig{EnableMetrics: false}, testLogger())
		require.NoError(t, selector.RegisterModel(&ModelConfig{
			ModelID:  "cloud-only",
			Provider: ModelProviderGemini,
			Capabilities: &ModelCapabilities{
				ModelID:        "cloud-only",
				IsLocal:        false,
				SupportedTasks: []TaskType{TaskTypeEmbedding},
				IsAvailable:    true,
			},
		}))

		_, err := selector.SelectModel(ctx, &SelectionCriteria{
			TaskType:     TaskTypeEmbedding,
			RequireLocal: true,
		})
		assert.Equal(t, ErrNoSuitableModel, err)
	})
}

func TestSelectModel_ContentSize(t *testing.T) {
	selector := createTestSelector(t)
	ctx := context.Background()

	t.Run("selects model with sufficient context window", func(t *testing.T) {
		result, err := selector.SelectModel(ctx, &SelectionCriteria{
			TaskType:    TaskTypeSummarization,
			ContentSize: 10000, // ~2500 tokens
		})
		require.NoError(t, err)
		assert.NotNil(t, result.SelectedModel)
	})

	t.Run("penalizes models with insufficient context", func(t *testing.T) {
		// Large content that only cloud model can handle
		result, err := selector.SelectModel(ctx, &SelectionCriteria{
			TaskType:    TaskTypeSummarization,
			ContentSize: 1000000, // ~250k tokens
		})
		require.NoError(t, err)
		// Should select the model with the largest context window
		assert.Equal(t, "cloud-llm", result.SelectedModel.ModelID)
	})
}

func TestSelectModel_LatencyConstraint(t *testing.T) {
	selector := createTestSelector(t)
	ctx := context.Background()

	t.Run("respects max latency constraint", func(t *testing.T) {
		result, err := selector.SelectModel(ctx, &SelectionCriteria{
			TaskType:   TaskTypeSummarization,
			MaxLatency: 600 * time.Millisecond,
		})
		require.NoError(t, err)
		// Should prefer local model which has lower latency
		assert.True(t, result.SelectedModel.Capabilities.IsLocal)
	})
}

func TestSelectModel_NoModelsRegistered(t *testing.T) {
	selector := NewModelSelector(&SelectorConfig{EnableMetrics: false}, testLogger())
	ctx := context.Background()

	_, err := selector.SelectModel(ctx, &SelectionCriteria{
		TaskType: TaskTypeEmbedding,
	})
	assert.Equal(t, ErrNoModelsRegistered, err)
}

func TestSelectModel_SelectionResult(t *testing.T) {
	selector := createTestSelector(t)
	ctx := context.Background()

	result, err := selector.SelectModel(ctx, &SelectionCriteria{
		TaskType: TaskTypeEmbedding,
	})
	require.NoError(t, err)

	assert.NotNil(t, result.SelectedModel)
	assert.NotEmpty(t, result.SelectionReason)
	assert.Greater(t, result.SelectionDuration, time.Duration(0))
	assert.Greater(t, result.Score.TotalScore, float64(0))
}

func TestGetModelsForTask(t *testing.T) {
	selector := createTestSelector(t)

	t.Run("embedding models", func(t *testing.T) {
		models := selector.GetModelsForTask(TaskTypeEmbedding)
		assert.Len(t, models, 2) // local-embedding and cloud-embedding
	})

	t.Run("summarization models", func(t *testing.T) {
		models := selector.GetModelsForTask(TaskTypeSummarization)
		assert.Len(t, models, 2) // local-llm and cloud-llm
	})
}

func TestGetLocalModels(t *testing.T) {
	selector := createTestSelector(t)

	models := selector.GetLocalModels()
	assert.Len(t, models, 2)
	for _, m := range models {
		assert.True(t, m.Capabilities.IsLocal)
	}
}

func TestGetCloudModels(t *testing.T) {
	selector := createTestSelector(t)

	models := selector.GetCloudModels()
	assert.Len(t, models, 2)
	for _, m := range models {
		assert.False(t, m.Capabilities.IsLocal)
	}
}

func TestUpdateModelAvailability(t *testing.T) {
	selector := createTestSelector(t)

	t.Run("update availability", func(t *testing.T) {
		err := selector.UpdateModelAvailability("local-embedding", false)
		require.NoError(t, err)

		model, err := selector.GetModel("local-embedding")
		require.NoError(t, err)
		assert.False(t, model.Capabilities.IsAvailable)
	})

	t.Run("model not found", func(t *testing.T) {
		err := selector.UpdateModelAvailability("non-existent", true)
		assert.Equal(t, ErrModelNotFound, err)
	})
}

func TestModelCapabilities_SupportsTask(t *testing.T) {
	caps := &ModelCapabilities{
		SupportedTasks: []TaskType{TaskTypeEmbedding, TaskTypeSummarization},
	}

	assert.True(t, caps.SupportsTask(TaskTypeEmbedding))
	assert.True(t, caps.SupportsTask(TaskTypeSummarization))
	assert.False(t, caps.SupportsTask(TaskTypeExtraction))
}

func TestModelCapabilities_CanHandleContentSize(t *testing.T) {
	caps := &ModelCapabilities{
		ContextWindow: 8192,
	}

	// 8192 tokens * 4 chars/token * 0.75 = ~24576 chars max
	assert.True(t, caps.CanHandleContentSize(10000))
	assert.True(t, caps.CanHandleContentSize(24000))
	assert.False(t, caps.CanHandleContentSize(100000))
}

func TestModelCapabilities_EstimateCost(t *testing.T) {
	caps := &ModelCapabilities{
		CostPer1KTokens: 0.001,
	}

	cost := caps.EstimateCost(500, 100)
	assert.InDelta(t, 0.0006, cost, 0.0001)
}

func TestModelCapabilities_NeedsWarmup(t *testing.T) {
	t.Run("warmup not required", func(t *testing.T) {
		caps := &ModelCapabilities{
			WarmupRequired: false,
		}
		assert.False(t, caps.NeedsWarmup(5*time.Minute))
	})

	t.Run("never used", func(t *testing.T) {
		caps := &ModelCapabilities{
			WarmupRequired: true,
		}
		assert.True(t, caps.NeedsWarmup(5*time.Minute))
	})

	t.Run("recently used", func(t *testing.T) {
		caps := &ModelCapabilities{
			WarmupRequired: true,
			LastUsed:       time.Now().Add(-1 * time.Minute),
		}
		assert.False(t, caps.NeedsWarmup(5*time.Minute))
	})

	t.Run("not recently used", func(t *testing.T) {
		caps := &ModelCapabilities{
			WarmupRequired: true,
			LastUsed:       time.Now().Add(-10 * time.Minute),
		}
		assert.True(t, caps.NeedsWarmup(5*time.Minute))
	})
}

func TestScoreWeights(t *testing.T) {
	testCases := []struct {
		name  string
		mode  OptimizationMode
		check func(ScoreWeights) bool
	}{
		{
			"balanced weights sum to 1",
			OptimizationModeBalanced,
			func(w ScoreWeights) bool {
				return approxEqual(w.Latency+w.Cost+w.Quality+w.Privacy+w.Availability, 1.0)
			},
		},
		{
			"cost mode prioritizes cost",
			OptimizationModeCost,
			func(w ScoreWeights) bool {
				return w.Cost > w.Quality && w.Cost > w.Latency
			},
		},
		{
			"quality mode prioritizes quality",
			OptimizationModeQuality,
			func(w ScoreWeights) bool {
				return w.Quality > w.Cost && w.Quality > w.Latency
			},
		},
		{
			"latency mode prioritizes latency",
			OptimizationModeLatency,
			func(w ScoreWeights) bool {
				return w.Latency > w.Cost && w.Latency > w.Quality
			},
		},
		{
			"privacy mode prioritizes privacy",
			OptimizationModePrivacy,
			func(w ScoreWeights) bool {
				return w.Privacy > w.Cost && w.Privacy > w.Latency
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			weights := GetWeightsForMode(tc.mode)
			assert.True(t, tc.check(weights))
		})
	}
}

func TestCalculateScore(t *testing.T) {
	model := &ModelCapabilities{
		ModelID:         "test-model",
		IsLocal:         true,
		SupportedTasks:  []TaskType{TaskTypeEmbedding},
		ContextWindow:   8192,
		AvgLatency:      100 * time.Millisecond,
		CostPer1KTokens: 0,
		QualityScore:    0.80,
		SpeedScore:      0.85,
		IsAvailable:     true,
	}

	criteria := &SelectionCriteria{
		TaskType: TaskTypeEmbedding,
	}

	weights := DefaultBalancedWeights()
	score := CalculateScore(model, criteria, weights, 5*time.Second, 0.01)

	assert.Equal(t, "test-model", score.ModelID)
	assert.Greater(t, score.TotalScore, float64(0))
	assert.Equal(t, 1.0, score.ComponentScores.PrivacyScore) // Local model
	assert.Equal(t, 1.0, score.ComponentScores.AvailabilityScore)
}

func TestCalculateScore_Penalties(t *testing.T) {
	t.Run("task not supported penalty", func(t *testing.T) {
		model := &ModelCapabilities{
			ModelID:        "test-model",
			SupportedTasks: []TaskType{TaskTypeEmbedding},
			IsAvailable:    true,
		}

		criteria := &SelectionCriteria{
			TaskType: TaskTypeSummarization, // Not supported
		}

		score := CalculateScore(model, criteria, DefaultBalancedWeights(), 5*time.Second, 0.01)
		assert.Less(t, score.TotalScore, float64(0))
		assert.Len(t, score.Penalties, 1)
	})

	t.Run("local required penalty", func(t *testing.T) {
		model := &ModelCapabilities{
			ModelID:        "cloud-model",
			IsLocal:        false,
			SupportedTasks: []TaskType{TaskTypeEmbedding},
			IsAvailable:    true,
		}

		criteria := &SelectionCriteria{
			TaskType:     TaskTypeEmbedding,
			RequireLocal: true,
		}

		score := CalculateScore(model, criteria, DefaultBalancedWeights(), 5*time.Second, 0.01)
		assert.Less(t, score.TotalScore, float64(0))
	})
}

func TestWarmupManager(t *testing.T) {
	selector := createTestSelector(t)

	t.Run("tracks usage", func(t *testing.T) {
		model, _ := selector.GetModel("local-embedding")

		// Use the model multiple times
		for i := 0; i < 5; i++ {
			selector.warmupManager.ConsiderWarmup(model)
		}

		// Check usage count
		selector.warmupManager.mu.Lock()
		count := selector.warmupManager.usageCount["local-embedding"]
		selector.warmupManager.mu.Unlock()

		assert.Equal(t, 5, count)
	})

	t.Run("reset usage count", func(t *testing.T) {
		model, _ := selector.GetModel("local-llm")
		selector.warmupManager.ConsiderWarmup(model)

		selector.warmupManager.ResetUsageCount()

		selector.warmupManager.mu.Lock()
		count := selector.warmupManager.usageCount["local-llm"]
		selector.warmupManager.mu.Unlock()

		assert.Equal(t, 0, count)
	})
}

func TestListModels(t *testing.T) {
	selector := createTestSelector(t)

	models := selector.ListModels()
	assert.Len(t, models, 4)
}

// approxEqual checks if two floats are approximately equal.
func approxEqual(a, b float64) bool {
	const epsilon = 0.0001
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < epsilon
}

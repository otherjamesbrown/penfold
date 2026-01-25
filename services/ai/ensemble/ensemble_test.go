package ensemble

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// mockModelBackend implements ModelBackend for testing.
type mockModelBackend struct {
	name         string
	provider     string
	isLocal      bool
	costPer1K    float64
	qualityScore float64
	delay        time.Duration
	response     interface{}
	confidence   float64
	err          error
	callCount    int
	mu           sync.Mutex
}

func newMockModel(name, provider string, isLocal bool) *mockModelBackend {
	return &mockModelBackend{
		name:         name,
		provider:     provider,
		isLocal:      isLocal,
		costPer1K:    0.001,
		qualityScore: 0.8,
		response:     "test response",
		confidence:   0.9,
	}
}

func (m *mockModelBackend) Name() string                { return m.name }
func (m *mockModelBackend) Provider() string            { return m.provider }
func (m *mockModelBackend) IsLocal() bool               { return m.isLocal }
func (m *mockModelBackend) CostPer1KTokens() float64    { return m.costPer1K }
func (m *mockModelBackend) QualityScore() float64       { return m.qualityScore }

func (m *mockModelBackend) Process(ctx context.Context, content string, reqType RequestType, options map[string]interface{}) (*ModelResult, error) {
	m.mu.Lock()
	m.callCount++
	m.mu.Unlock()

	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if m.err != nil {
		return nil, m.err
	}

	return &ModelResult{
		ModelID:    m.name,
		Provider:   m.provider,
		Response:   m.response,
		Confidence: m.confidence,
		TokensUsed: 100,
	}, nil
}

func (m *mockModelBackend) getCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

func (m *mockModelBackend) withResponse(response interface{}) *mockModelBackend {
	m.response = response
	return m
}

func (m *mockModelBackend) withConfidence(confidence float64) *mockModelBackend {
	m.confidence = confidence
	return m
}

func (m *mockModelBackend) withDelay(delay time.Duration) *mockModelBackend {
	m.delay = delay
	return m
}

func (m *mockModelBackend) withError(err error) *mockModelBackend {
	m.err = err
	return m
}

func (m *mockModelBackend) withQuality(quality float64) *mockModelBackend {
	m.qualityScore = quality
	return m
}

func (m *mockModelBackend) withCost(cost float64) *mockModelBackend {
	m.costPer1K = cost
	return m
}

// Test helpers

func createTestProcessor(models ...ModelBackend) *EnsembleProcessor {
	cfg := DefaultProcessorConfig()
	cfg.EnableMetrics = false // Disable metrics for tests

	ep := NewEnsembleProcessor(cfg, nil)

	for _, m := range models {
		_ = ep.AddModel(m)
	}

	return ep
}

func createTestRequest() *EnsembleRequest {
	return &EnsembleRequest{
		ID:           "test-request-1",
		Type:         RequestTypeClassification,
		Content:      "Test content for processing",
		Strategy:     StrategyParallel,
		Timeout:      5 * time.Second,
		MinResponses: 1,
	}
}

// Tests

func TestNewEnsembleProcessor(t *testing.T) {
	t.Run("creates with default config", func(t *testing.T) {
		ep := NewEnsembleProcessor(nil, nil)
		if ep == nil {
			t.Fatal("expected non-nil processor")
		}

		strategies := ep.GetSupportedStrategies()
		if len(strategies) == 0 {
			t.Error("expected built-in strategies to be registered")
		}
	})

	t.Run("creates with custom config", func(t *testing.T) {
		cfg := &ProcessorConfig{
			DefaultTimeout:      10 * time.Second,
			DefaultStrategy:     StrategyCascade,
			DefaultMinResponses: 2,
			EnableMetrics:       false,
		}

		ep := NewEnsembleProcessor(cfg, nil)
		if ep.GetConfig().DefaultStrategy != StrategyCascade {
			t.Error("expected cascade strategy")
		}
	})
}

func TestAddRemoveModel(t *testing.T) {
	ep := createTestProcessor()

	t.Run("adds model", func(t *testing.T) {
		model := newMockModel("test-model", "ollama", true)
		err := ep.AddModel(model)
		if err != nil {
			t.Fatalf("failed to add model: %v", err)
		}

		models := ep.GetModels()
		if len(models) != 1 {
			t.Errorf("expected 1 model, got %d", len(models))
		}
	})

	t.Run("removes model", func(t *testing.T) {
		err := ep.RemoveModel("test-model")
		if err != nil {
			t.Fatalf("failed to remove model: %v", err)
		}

		models := ep.GetModels()
		if len(models) != 0 {
			t.Errorf("expected 0 models, got %d", len(models))
		}
	})

	t.Run("returns error for missing model", func(t *testing.T) {
		err := ep.RemoveModel("nonexistent")
		if !errors.Is(err, ErrModelNotFound) {
			t.Errorf("expected ErrModelNotFound, got %v", err)
		}
	})
}

func TestProcessParallelStrategy(t *testing.T) {
	model1 := newMockModel("model1", "ollama", true).withResponse("answer A")
	model2 := newMockModel("model2", "gemini", false).withResponse("answer A")
	model3 := newMockModel("model3", "openai", false).withResponse("answer B")

	ep := createTestProcessor(model1, model2, model3)

	req := createTestRequest()
	req.Strategy = StrategyParallel

	ctx := context.Background()
	result, err := ep.Process(ctx, req)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if len(result.IndividualResults) != 3 {
		t.Errorf("expected 3 individual results, got %d", len(result.IndividualResults))
	}

	if result.ConsensusScore < 0.5 {
		t.Errorf("expected consensus score >= 0.5, got %f", result.ConsensusScore)
	}
}

func TestProcessMajorityVotingStrategy(t *testing.T) {
	// 3 models: 2 agree, 1 disagrees
	model1 := newMockModel("model1", "ollama", true).withResponse("correct answer").withConfidence(0.9)
	model2 := newMockModel("model2", "gemini", false).withResponse("correct answer").withConfidence(0.85)
	model3 := newMockModel("model3", "openai", false).withResponse("wrong answer").withConfidence(0.7)

	ep := createTestProcessor(model1, model2, model3)

	req := createTestRequest()
	req.Strategy = StrategyMajorityVoting
	req.MinResponses = 2

	ctx := context.Background()
	result, err := ep.Process(ctx, req)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Consensus should be ~0.67 (2 out of 3)
	if result.ConsensusScore < 0.6 || result.ConsensusScore > 0.7 {
		t.Errorf("expected consensus score around 0.67, got %f", result.ConsensusScore)
	}

	if result.FinalResponse != "correct answer" {
		t.Errorf("expected 'correct answer', got %v", result.FinalResponse)
	}
}

func TestProcessWeightedVotingStrategy(t *testing.T) {
	// Higher quality model's answer should win
	model1 := newMockModel("model1", "ollama", true).
		withResponse("low quality answer").
		withConfidence(0.9).
		withQuality(0.5)
	model2 := newMockModel("model2", "gemini", false).
		withResponse("high quality answer").
		withConfidence(0.9).
		withQuality(0.95)

	ep := createTestProcessor(model1, model2)

	req := createTestRequest()
	req.Strategy = StrategyWeightedVoting
	req.MinResponses = 1

	ctx := context.Background()
	result, err := ep.Process(ctx, req)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The high-quality model should be selected
	if result.SelectedModel != "model2" {
		t.Errorf("expected model2 to be selected, got %s", result.SelectedModel)
	}
}

func TestProcessCascadeStrategy(t *testing.T) {
	// First model fails, second succeeds
	model1 := newMockModel("model1", "ollama", true).
		withError(errors.New("model unavailable")).
		withQuality(0.95)
	model2 := newMockModel("model2", "gemini", false).
		withResponse("fallback answer").
		withConfidence(0.85).
		withQuality(0.9)

	ep := createTestProcessor(model1, model2)

	req := createTestRequest()
	req.Strategy = StrategyCascade
	req.Options = map[string]interface{}{
		"min_confidence": 0.7,
	}

	ctx := context.Background()
	result, err := ep.Process(ctx, req)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.SelectedModel != "model2" {
		t.Errorf("expected model2 to be selected, got %s", result.SelectedModel)
	}

	if result.FinalResponse != "fallback answer" {
		t.Errorf("expected 'fallback answer', got %v", result.FinalResponse)
	}
}

func TestProcessDebateStrategy(t *testing.T) {
	// Use different initial answers to force multiple debate rounds
	model1 := newMockModel("model1", "ollama", true).withResponse("answer A").withConfidence(0.6)
	model2 := newMockModel("model2", "gemini", false).withResponse("answer B").withConfidence(0.6)

	ep := createTestProcessor(model1, model2)

	req := createTestRequest()
	req.Strategy = StrategyDebate
	req.Options = map[string]interface{}{
		"max_rounds": 2,
	}
	req.MinResponses = 2
	req.ConsensusThreshold = 0.9 // High threshold to force multiple rounds

	ctx := context.Background()
	result, err := ep.Process(ctx, req)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Both models should have been queried at least once
	if model1.getCallCount() < 1 || model2.getCallCount() < 1 {
		t.Errorf("expected models to be called, got model1=%d model2=%d calls",
			model1.getCallCount(), model2.getCallCount())
	}

	if result.Strategy != StrategyDebate {
		t.Errorf("expected debate strategy, got %s", result.Strategy)
	}

	// Debate should have individual results from all rounds
	if len(result.IndividualResults) < 2 {
		t.Errorf("expected at least 2 individual results, got %d", len(result.IndividualResults))
	}
}

func TestProcessTimeout(t *testing.T) {
	// Model that takes too long
	model := newMockModel("slow-model", "ollama", true).withDelay(2 * time.Second)

	ep := createTestProcessor(model)

	req := createTestRequest()
	req.Timeout = 100 * time.Millisecond

	ctx := context.Background()
	_, err := ep.Process(ctx, req)

	if err == nil {
		t.Error("expected error due to timeout")
	}
}

func TestProcessInsufficientQuorum(t *testing.T) {
	// Both models fail
	model1 := newMockModel("model1", "ollama", true).withError(errors.New("error 1"))
	model2 := newMockModel("model2", "gemini", false).withError(errors.New("error 2"))

	ep := createTestProcessor(model1, model2)

	req := createTestRequest()
	req.MinResponses = 1

	ctx := context.Background()
	_, err := ep.Process(ctx, req)

	if err == nil {
		t.Error("expected error due to insufficient quorum")
	}
}

func TestAggregatorMajorityVoting(t *testing.T) {
	agg := NewAggregator(nil, nil)

	results := []ModelResult{
		{ModelID: "m1", Response: "answer A", Confidence: 0.9},
		{ModelID: "m2", Response: "answer A", Confidence: 0.8},
		{ModelID: "m3", Response: "answer B", Confidence: 0.7},
	}

	result := agg.AggregateMajorityVoting(results)

	if result.FinalResponse != "answer A" {
		t.Errorf("expected 'answer A', got %v", result.FinalResponse)
	}

	// 2/3 = 0.666...
	if result.ConsensusScore < 0.66 || result.ConsensusScore > 0.67 {
		t.Errorf("expected consensus ~0.67, got %f", result.ConsensusScore)
	}
}

func TestAggregatorWeightedVoting(t *testing.T) {
	agg := NewAggregator(nil, nil)

	results := []ModelResult{
		{ModelID: "m1", Response: "answer A", Confidence: 0.5},
		{ModelID: "m2", Response: "answer B", Confidence: 0.9},
	}

	qualities := map[string]float64{
		"m1": 0.5,
		"m2": 0.9,
	}

	result := agg.AggregateWeightedVoting(results, qualities)

	// Higher weight model should win
	if result.FinalResponse != "answer B" {
		t.Errorf("expected 'answer B', got %v", result.FinalResponse)
	}
}

func TestAggregatorConsensusScore(t *testing.T) {
	agg := NewAggregator(nil, nil)

	tests := []struct {
		name     string
		results  []ModelResult
		expected float64
	}{
		{
			name:     "single result",
			results:  []ModelResult{{Response: "A"}},
			expected: 1.0,
		},
		{
			name: "all agree",
			results: []ModelResult{
				{Response: "A"},
				{Response: "A"},
				{Response: "A"},
			},
			expected: 1.0,
		},
		{
			name: "majority agree",
			results: []ModelResult{
				{Response: "A"},
				{Response: "A"},
				{Response: "B"},
			},
			expected: 0.666,
		},
		{
			name: "split",
			results: []ModelResult{
				{Response: "A"},
				{Response: "B"},
			},
			expected: 0.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := agg.CalculateConsensusScore(tt.results)
			// Allow small tolerance for floating point
			if score < tt.expected-0.01 || score > tt.expected+0.01 {
				t.Errorf("expected %f, got %f", tt.expected, score)
			}
		})
	}
}

func TestOrchestratorParallel(t *testing.T) {
	cfg := DefaultOrchestratorConfig()
	cfg.MaxConcurrentModels = 3
	orch := NewOrchestrator(cfg, nil)

	models := []ModelBackend{
		newMockModel("m1", "ollama", true).withDelay(50 * time.Millisecond),
		newMockModel("m2", "gemini", false).withDelay(50 * time.Millisecond),
		newMockModel("m3", "openai", false).withDelay(50 * time.Millisecond),
	}

	req := createTestRequest()
	ctx := context.Background()

	start := time.Now()
	results, err := orch.ExecuteParallel(ctx, req, models)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}

	// Should complete faster than sequential (3 * 50ms = 150ms)
	if duration > 150*time.Millisecond {
		t.Errorf("expected parallel execution, took %v", duration)
	}
}

func TestOrchestratorSequential(t *testing.T) {
	cfg := DefaultOrchestratorConfig()
	orch := NewOrchestrator(cfg, nil)

	models := []ModelBackend{
		newMockModel("m1", "ollama", true).withDelay(30 * time.Millisecond),
		newMockModel("m2", "gemini", false).withDelay(30 * time.Millisecond),
	}

	req := createTestRequest()
	ctx := context.Background()

	start := time.Now()
	results, err := orch.ExecuteSequential(ctx, req, models)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}

	// Should take at least 60ms (sequential)
	if duration < 60*time.Millisecond {
		t.Errorf("expected sequential execution, took %v", duration)
	}
}

// flakeyModelBackend fails for the first N calls, then succeeds.
type flakeyModelBackend struct {
	name          string
	failCount     int
	maxFailures   int
	mu            sync.Mutex
}

func newFlakeyModel(name string, maxFailures int) *flakeyModelBackend {
	return &flakeyModelBackend{
		name:        name,
		maxFailures: maxFailures,
	}
}

func (m *flakeyModelBackend) Name() string             { return m.name }
func (m *flakeyModelBackend) Provider() string         { return "test" }
func (m *flakeyModelBackend) IsLocal() bool            { return true }
func (m *flakeyModelBackend) CostPer1KTokens() float64 { return 0 }
func (m *flakeyModelBackend) QualityScore() float64    { return 0.8 }

func (m *flakeyModelBackend) Process(ctx context.Context, content string, reqType RequestType, options map[string]interface{}) (*ModelResult, error) {
	m.mu.Lock()
	m.failCount++
	currentCount := m.failCount
	m.mu.Unlock()

	if currentCount <= m.maxFailures {
		return nil, errors.New("temporary failure")
	}

	return &ModelResult{
		ModelID:    m.name,
		Provider:   "test",
		Response:   "success after retry",
		Confidence: 0.9,
		TokensUsed: 100,
	}, nil
}

func (m *flakeyModelBackend) getFailCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.failCount
}

func TestOrchestratorRetry(t *testing.T) {
	cfg := DefaultOrchestratorConfig()
	cfg.RetryFailedModels = true
	cfg.MaxRetries = 2
	cfg.RetryDelay = 10 * time.Millisecond
	orch := NewOrchestrator(cfg, nil)

	// Model that fails once, then succeeds on retry
	model := newFlakeyModel("m1", 1)

	req := createTestRequest()
	ctx := context.Background()

	result, err := orch.ExecuteSingle(ctx, req, model)

	if err != nil {
		t.Fatalf("unexpected error after retry: %v", err)
	}

	if result.Response != "success after retry" {
		t.Errorf("expected success response, got %v", result.Response)
	}

	// Should have been called twice (initial + 1 retry)
	if model.getFailCount() != 2 {
		t.Errorf("expected 2 calls, got %d", model.getFailCount())
	}
}

func TestConfigBuilder(t *testing.T) {
	cfg := NewConfigBuilder().
		WithName("test-ensemble").
		WithDescription("Test ensemble configuration").
		WithModels("model1", "model2").
		WithStrategy(StrategyMajorityVoting).
		WithTimeout(15 * time.Second).
		WithMinResponses(2).
		WithConsensusThreshold(0.8).
		WithRequireConsensus(true).
		WithOption("custom_option", "value").
		WithTags("test", "ensemble").
		Build()

	if cfg.Name != "test-ensemble" {
		t.Errorf("expected 'test-ensemble', got '%s'", cfg.Name)
	}

	if len(cfg.Models) != 2 {
		t.Errorf("expected 2 models, got %d", len(cfg.Models))
	}

	if cfg.Strategy != StrategyMajorityVoting {
		t.Errorf("expected majority_voting, got %s", cfg.Strategy)
	}

	if cfg.StrategyOptions["custom_option"] != "value" {
		t.Error("expected custom option to be set")
	}
}

func TestTaskEnsembleConfigs(t *testing.T) {
	tests := []struct {
		taskType RequestType
		expected StrategyType
	}{
		{RequestTypeClassification, StrategyMajorityVoting},
		{RequestTypeSummary, StrategyWeightedVoting},
		{RequestTypeExtraction, StrategyParallel},
		{RequestTypeEmbedding, StrategyCascade},
		{RequestTypeChat, StrategyDebate},
	}

	for _, tt := range tests {
		t.Run(string(tt.taskType), func(t *testing.T) {
			cfg := GetTaskEnsembleConfig(tt.taskType)
			if cfg.Strategy != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, cfg.Strategy)
			}
		})
	}
}

func TestModelPrioritizer(t *testing.T) {
	models := []ModelBackend{
		newMockModel("m1", "ollama", true).withQuality(0.7).withCost(0),
		newMockModel("m2", "gemini", false).withQuality(0.9).withCost(0.01),
		newMockModel("m3", "openai", false).withQuality(0.8).withCost(0.005),
	}

	t.Run("sort by quality", func(t *testing.T) {
		p := NewModelPrioritizer(SortByQuality)
		sorted := p.Prioritize(models)

		if sorted[0].Name() != "m2" {
			t.Errorf("expected m2 first (highest quality), got %s", sorted[0].Name())
		}
	})

	t.Run("sort by cost", func(t *testing.T) {
		p := NewModelPrioritizer(SortByCost)
		sorted := p.Prioritize(models)

		if sorted[0].Name() != "m1" {
			t.Errorf("expected m1 first (lowest cost), got %s", sorted[0].Name())
		}
	})

	t.Run("sort by local", func(t *testing.T) {
		p := NewModelPrioritizer(SortByLocal)
		sorted := p.Prioritize(models)

		if !sorted[0].IsLocal() {
			t.Error("expected local model first")
		}
	})
}

func TestResponseClassifier(t *testing.T) {
	classifier := NewResponseClassifier(0.8)

	results := []ModelResult{
		{ModelID: "m1", Response: "The answer is 42"},
		{ModelID: "m2", Response: "The answer is 42"},
		{ModelID: "m3", Response: "I don't know"},
	}

	groups := classifier.ClassifyResponses(results)

	if len(groups) != 2 {
		t.Errorf("expected 2 groups, got %d", len(groups))
	}
}

func TestExecutionPlanner(t *testing.T) {
	planner := NewExecutionPlanner(nil)

	models := []ModelBackend{
		newMockModel("m1", "ollama", true),
		newMockModel("m2", "gemini", false),
	}

	req := createTestRequest()

	t.Run("cascade plan", func(t *testing.T) {
		plan := planner.PlanExecution(req, models, StrategyCascade)

		if len(plan.Phases) != 2 {
			t.Errorf("expected 2 phases for cascade, got %d", len(plan.Phases))
		}

		if !plan.Phases[0].StopOnSuccess {
			t.Error("expected StopOnSuccess for cascade phases")
		}
	})

	t.Run("parallel plan", func(t *testing.T) {
		plan := planner.PlanExecution(req, models, StrategyParallel)

		if len(plan.Phases) != 1 {
			t.Errorf("expected 1 phase for parallel, got %d", len(plan.Phases))
		}

		if !plan.Phases[0].Parallel {
			t.Error("expected parallel execution")
		}
	})
}

func TestDisagreementDetection(t *testing.T) {
	agg := NewAggregator(&AggregatorConfig{
		DisagreementThreshold: 0.3,
	}, nil)

	results := []ModelResult{
		{ModelID: "m1", Response: "answer A", Confidence: 0.95},
		{ModelID: "m2", Response: "answer B", Confidence: 0.5},
	}

	result := agg.AggregateMajorityVoting(results)

	if len(result.Disagreements) == 0 {
		t.Error("expected disagreements to be detected")
	}
}

func BenchmarkParallelExecution(b *testing.B) {
	models := make([]ModelBackend, 5)
	for i := 0; i < 5; i++ {
		models[i] = newMockModel("model"+string(rune('0'+i)), "test", true)
	}

	ep := createTestProcessor(models...)
	req := createTestRequest()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ep.Process(ctx, req)
	}
}

func BenchmarkMajorityVotingAggregation(b *testing.B) {
	agg := NewAggregator(nil, nil)

	results := make([]ModelResult, 10)
	for i := 0; i < 10; i++ {
		results[i] = ModelResult{
			ModelID:    "model" + string(rune('0'+i)),
			Response:   "answer A",
			Confidence: 0.8,
		}
	}
	// Make some disagree
	results[7].Response = "answer B"
	results[8].Response = "answer B"
	results[9].Response = "answer C"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = agg.AggregateMajorityVoting(results)
	}
}

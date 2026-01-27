// Package integration provides integration tests for the AI Coordinator service.
// These tests verify the interaction between different components:
// - Model selector, router, registry, cost tracking, ensemble, and escalation
package integration

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/ai/cost"
	"github.com/otherjamesbrown/penfold/services/ai/ensemble"
	"github.com/otherjamesbrown/penfold/services/ai/escalation"
	"github.com/otherjamesbrown/penfold/services/ai/registry"
	"github.com/otherjamesbrown/penfold/services/ai/router"
	"github.com/otherjamesbrown/penfold/services/ai/selector"
)

// testLogger creates a logger for testing.
func testLogger() logging.Logger {
	return logging.NewLogger(&logging.Config{
		Level:       logging.LevelDebug,
		ServiceName: "ai-integration-test",
		Environment: "test",
		JSONFormat:  false,
	})
}

// ============================================================================
// Mock Backend for Integration Tests
// ============================================================================

// mockBackend implements router.ModelBackend for testing.
type mockBackend struct {
	name         string
	provider     string
	isLocal      bool
	capabilities []string
	mu           sync.Mutex
	healthy      bool
	processFunc  func(ctx context.Context, req *router.Request) (*router.Response, error)
	processCount int64
	processDelay time.Duration
}

func newMockBackend(name, provider string, isLocal bool, caps []string) *mockBackend {
	return &mockBackend{
		name:         name,
		provider:     provider,
		isLocal:      isLocal,
		capabilities: caps,
		healthy:      true,
	}
}

func (m *mockBackend) Name() string           { return m.name }
func (m *mockBackend) Provider() string       { return m.provider }
func (m *mockBackend) IsLocal() bool          { return m.isLocal }
func (m *mockBackend) Capabilities() []string { return m.capabilities }

func (m *mockBackend) HealthCheck(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.healthy {
		return errors.New("backend unhealthy")
	}
	return nil
}

func (m *mockBackend) Process(ctx context.Context, req *router.Request) (*router.Response, error) {
	m.mu.Lock()
	m.processCount++
	delay := m.processDelay
	fn := m.processFunc
	m.mu.Unlock()

	if delay > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}

	if fn != nil {
		return fn(ctx, req)
	}

	return &router.Response{
		RequestID:      req.ID,
		ModelUsed:      m.name,
		ProviderUsed:   m.provider,
		Data:           "mock response",
		TokensUsed:     100,
		ProcessingTime: 50 * time.Millisecond,
	}, nil
}

func (m *mockBackend) SetHealthy(healthy bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.healthy = healthy
}

func (m *mockBackend) SetProcessDelay(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.processDelay = d
}

func (m *mockBackend) GetProcessCount() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.processCount
}

// ============================================================================
// Full AI Pipeline Integration Tests
// ============================================================================

func TestFullAIPipeline(t *testing.T) {
	logger := testLogger()
	ctx := context.Background()

	// 1. Set up model selector
	selectorCfg := &selector.SelectorConfig{
		DefaultOptimizationMode: selector.OptimizationModeBalanced,
		WarmupThreshold:         5 * time.Minute,
		EnableMetrics:           false,
	}
	modelSelector := selector.NewModelSelector(selectorCfg, logger)

	// Register test models
	err := modelSelector.RegisterModel(&selector.ModelConfig{
		ModelID:  "local-llm",
		Provider: selector.ModelProviderMLX,
		Endpoint: "http://localhost:11434",
		Capabilities: &selector.ModelCapabilities{
			ModelID:        "local-llm",
			Provider:       selector.ModelProviderMLX,
			IsLocal:        true,
			SupportedTasks: []selector.TaskType{selector.TaskTypeSummarization, selector.TaskTypeExtraction},
			ContextWindow:  128000,
			AvgLatency:     500 * time.Millisecond,
			QualityScore:   0.70,
			SpeedScore:     0.85,
			IsAvailable:    true,
		},
	})
	if err != nil {
		t.Fatalf("Failed to register local model: %v", err)
	}

	err = modelSelector.RegisterModel(&selector.ModelConfig{
		ModelID:  "cloud-llm",
		Provider: selector.ModelProviderGemini,
		Endpoint: "https://api.gemini.example.com",
		Capabilities: &selector.ModelCapabilities{
			ModelID:         "cloud-llm",
			Provider:        selector.ModelProviderGemini,
			IsLocal:         false,
			SupportedTasks:  []selector.TaskType{selector.TaskTypeSummarization, selector.TaskTypeExtraction},
			ContextWindow:   1000000,
			AvgLatency:      1 * time.Second,
			CostPer1KTokens: 0.001,
			QualityScore:    0.95,
			SpeedScore:      0.50,
			IsAvailable:     true,
		},
	})
	if err != nil {
		t.Fatalf("Failed to register cloud model: %v", err)
	}

	// 2. Set up router
	routerCfg := &router.RouterConfig{
		DefaultTimeout:       30 * time.Second,
		MaxQueueSize:         100,
		EnableMetrics:        false,
		CircuitBreakerConfig: router.DefaultCircuitBreakerConfig(),
		FallbackOrder:        []string{"local-llm", "cloud-llm"},
		Logger:               logger,
	}
	modelRouter := router.NewModelRouter(routerCfg)
	defer func() { _ = modelRouter.Shutdown(ctx) }()

	// Register backends
	localBackend := newMockBackend("local-llm", "mlx", true, []string{"summarization", "extraction"})
	cloudBackend := newMockBackend("cloud-llm", "gemini", false, []string{"summarization", "extraction"})

	if err := modelRouter.RegisterBackend(localBackend); err != nil {
		t.Fatalf("Failed to register local backend: %v", err)
	}
	if err := modelRouter.RegisterBackend(cloudBackend); err != nil {
		t.Fatalf("Failed to register cloud backend: %v", err)
	}

	// 3. Set up cost tracker
	costCfg := &cost.TrackerConfig{
		EnablePersistence: false,
		EnableMetrics:     false,
		EnableAlerts:      true,
		EnforceBudgets:    false,
	}
	costTracker := cost.NewCostTracker(costCfg, logger)

	// 4. Test model selection
	t.Run("ModelSelection", func(t *testing.T) {
		// Cost optimization should prefer local (free)
		result, err := modelSelector.SelectModel(ctx, &selector.SelectionCriteria{
			TaskType:         selector.TaskTypeSummarization,
			OptimizationMode: selector.OptimizationModeCost,
		})
		if err != nil {
			t.Fatalf("Failed to select model: %v", err)
		}
		if !result.SelectedModel.Capabilities.IsLocal {
			t.Error("Expected local model for cost optimization")
		}

		// Quality optimization should prefer cloud (higher quality)
		result, err = modelSelector.SelectModel(ctx, &selector.SelectionCriteria{
			TaskType:         selector.TaskTypeSummarization,
			OptimizationMode: selector.OptimizationModeQuality,
		})
		if err != nil {
			t.Fatalf("Failed to select model: %v", err)
		}
		if result.SelectedModel.ModelID != "cloud-llm" {
			t.Errorf("Expected cloud-llm for quality optimization, got %s", result.SelectedModel.ModelID)
		}
	})

	// 5. Test routing
	t.Run("Routing", func(t *testing.T) {
		req := &router.Request{
			ID:       "test-req-1",
			Type:     router.RequestTypeSummary,
			Content:  "Test content for summarization",
			TenantID: "test-tenant",
		}

		resp, err := modelRouter.Route(ctx, req)
		if err != nil {
			t.Fatalf("Failed to route request: %v", err)
		}

		if resp.RequestID != req.ID {
			t.Errorf("Expected request ID %s, got %s", req.ID, resp.RequestID)
		}
	})

	// 6. Test cost tracking
	t.Run("CostTracking", func(t *testing.T) {
		req := &router.Request{
			ID:       "test-req-2",
			Type:     router.RequestTypeSummary,
			Content:  "Test content for cost tracking",
			TenantID: "test-tenant",
		}

		resp := &router.Response{
			RequestID:      req.ID,
			ModelUsed:      "gemini-1.5-flash",
			ProviderUsed:   "gemini",
			TokensUsed:     500,
			ProcessingTime: 100 * time.Millisecond,
		}

		err := costTracker.TrackRequest(ctx, req, resp, "test-operation")
		if err != nil {
			t.Fatalf("Failed to track request: %v", err)
		}

		// Verify cost estimation
		estimate := costTracker.EstimateCost("test-tenant", "gemini-1.5-flash", 1000, 500)
		if estimate.EstimatedCost <= 0 {
			t.Error("Expected positive estimated cost for cloud model")
		}
	})
}

// TestCostAwareRouting tests the integration of cost tracking with routing decisions.
func TestCostAwareRouting(t *testing.T) {
	logger := testLogger()
	ctx := context.Background()

	// Set up cost tracker with budget enforcement
	costCfg := &cost.TrackerConfig{
		EnablePersistence: false,
		EnableMetrics:     false,
		EnableAlerts:      true,
		EnforceBudgets:    true,
	}
	costTracker := cost.NewCostTracker(costCfg, logger)

	// Set up mock storage with budget
	mockStorage := &mockCostStorage{
		budgets: map[string]*cost.Budget{
			"test-tenant": cost.NewBudget("test-tenant", 0.10, 1.0), // Very small budget
		},
	}
	costTracker.SetStorage(mockStorage)

	// Test budget checking
	t.Run("BudgetCheck_WithinBudget", func(t *testing.T) {
		estimate := &cost.EstimatedCost{EstimatedCost: 0.05}
		ok, err := costTracker.CheckBudget(ctx, "test-tenant", estimate)
		if err != nil {
			t.Fatalf("CheckBudget failed: %v", err)
		}
		if !ok {
			t.Error("Expected budget check to pass for cost within budget")
		}
	})

	t.Run("BudgetCheck_ExceedsBudget", func(t *testing.T) {
		estimate := &cost.EstimatedCost{EstimatedCost: 0.15} // Exceeds $0.10 daily budget
		ok, err := costTracker.CheckBudget(ctx, "test-tenant", estimate)
		if err == nil {
			t.Error("Expected error when exceeding budget with enforcement enabled")
		}
		if ok {
			t.Error("Expected budget check to fail for cost exceeding budget")
		}
	})

	t.Run("BudgetCheck_NoLimit", func(t *testing.T) {
		estimate := &cost.EstimatedCost{EstimatedCost: 100.0}
		ok, err := costTracker.CheckBudget(ctx, "unlimited-tenant", estimate)
		if err != nil {
			t.Fatalf("CheckBudget failed: %v", err)
		}
		if !ok {
			t.Error("Expected budget check to pass for tenant without budget")
		}
	})
}

// TestEscalationFlow tests the integration of escalation with model selection.
func TestEscalationFlow(t *testing.T) {
	logger := testLogger()
	ctx := context.Background()

	// Set up escalation manager
	escalationCfg := escalation.DefaultManagerConfig()
	escalationCfg.EnableMetrics = false
	escalationCfg.MaxEscalations = 2
	manager := escalation.NewEscalationManager(escalationCfg, logger)
	defer func() { _ = manager.Close() }()

	// Set up mock processor
	callCount := 0
	processor := &mockEscalationProcessor{
		processFunc: func(ctx context.Context, req *escalation.EscalationRequest, model string, tier escalation.TierLevel) (*escalation.EscalationResult, error) {
			callCount++
			// Simulate escalation path: local (low) -> cloud-standard (high)
			// The tier in the request reflects the current tier being processed
			confidence := 0.5
			if tier >= escalation.TierLevelCloudStandard {
				confidence = 0.95 // High enough to stop further escalation
			}
			return &escalation.EscalationResult{
				RequestID:         req.ID,
				ModelUsed:         model,
				TierUsed:          tier,
				Confidence:        confidence,
				ProcessingTime:    100 * time.Millisecond,
				ModelQualityScore: 0.85,
			}, nil
		},
	}
	manager.SetProcessor(processor)

	t.Run("AutoEscalation", func(t *testing.T) {
		request := &escalation.EscalationRequest{
			ID:          "test-escalation-1",
			CurrentTier: escalation.TierLevelLocal,
			TaskType:    "summarization",
		}

		result, err := manager.ProcessWithEscalation(ctx, request)
		if err != nil {
			t.Fatalf("ProcessWithEscalation failed: %v", err)
		}

		// Should have processed (at least 1 call occurred)
		if callCount < 1 {
			t.Error("Expected at least one processing call")
		}

		// Result should be returned
		if result == nil {
			t.Fatal("Expected non-nil result")
		}
	})

	t.Run("EscalationPath", func(t *testing.T) {
		path := manager.GetEscalationPath("summarization")
		if len(path) == 0 {
			t.Error("Expected non-empty escalation path")
		}
	})
}

// TestRegistryWithRouter tests the integration of model registry with router.
func TestRegistryWithRouter(t *testing.T) {
	logger := testLogger()
	ctx := context.Background()

	// Create registry with default models
	regConfig := &registry.RegistryConfig{
		MLXLLMURL:           "http://localhost:8080",
		MLXEmbeddingsURL:    "http://localhost:8081",
		GeminiEndpoint:      "https://api.example.com",
		HealthCheckInterval: 0, // Disable for tests
		EnableAutoDiscovery: false,
	}
	reg := registry.NewRegistry(regConfig)

	// Register models
	localModel := &registry.ModelConfig{
		ID:        "mlx/qwen2.5-32b",
		Name:      "Llama 3",
		Provider:  registry.ProviderMLX,
		ModelName: "llama3",
		Capabilities: registry.ModelCapabilities{
			Capabilities: []registry.Capability{registry.CapabilityChat, registry.CapabilitySummarization},
		},
		Health:   registry.ModelHealth{Status: registry.HealthStatusHealthy},
		IsLocal:  true,
		Priority: 100,
	}

	cloudModel := &registry.ModelConfig{
		ID:        "gemini/gemini-pro",
		Name:      "Gemini Pro",
		Provider:  registry.ProviderGemini,
		ModelName: "gemini-pro",
		Capabilities: registry.ModelCapabilities{
			Capabilities: []registry.Capability{registry.CapabilityChat, registry.CapabilitySummarization},
		},
		Health:   registry.ModelHealth{Status: registry.HealthStatusHealthy},
		IsLocal:  false,
		Priority: 90,
	}

	if err := reg.Register(localModel); err != nil {
		t.Fatalf("Failed to register local model: %v", err)
	}
	if err := reg.Register(cloudModel); err != nil {
		t.Fatalf("Failed to register cloud model: %v", err)
	}

	t.Run("SelectLocalPreferred", func(t *testing.T) {
		selected, err := reg.SelectModel(registry.CapabilityChat, true)
		if err != nil {
			t.Fatalf("SelectModel failed: %v", err)
		}
		if !selected.IsLocal {
			t.Error("Expected local model when preferLocal=true")
		}
	})

	t.Run("SelectByCapability", func(t *testing.T) {
		models := reg.ListByCapability(registry.CapabilitySummarization)
		if len(models) != 2 {
			t.Errorf("Expected 2 models with summarization capability, got %d", len(models))
		}
	})

	t.Run("ListAvailable", func(t *testing.T) {
		available := reg.ListAvailable()
		if len(available) != 2 {
			t.Errorf("Expected 2 available models, got %d", len(available))
		}
	})

	t.Run("UpdateHealth", func(t *testing.T) {
		err := reg.UpdateHealth("mlx/qwen2.5-32b", registry.ModelHealth{
			Status:    registry.HealthStatusUnhealthy,
			LastCheck: time.Now(),
		})
		if err != nil {
			t.Fatalf("UpdateHealth failed: %v", err)
		}

		available := reg.ListAvailable()
		if len(available) != 1 {
			t.Errorf("Expected 1 available model after unhealthy update, got %d", len(available))
		}
	})

	// 6. Test with router (conceptual integration)
	t.Run("RouterIntegration", func(t *testing.T) {
		routerCfg := &router.RouterConfig{
			DefaultTimeout: 5 * time.Second,
			EnableMetrics:  false,
			Logger:         logger,
		}
		r := router.NewModelRouter(routerCfg)
		defer func() { _ = r.Shutdown(ctx) }()

		// In a real integration, we'd map registry models to router backends
		// For this test, we verify the registration flow
		mockBackend := newMockBackend("test-backend", "test", true, []string{"chat"})
		if err := r.RegisterBackend(mockBackend); err != nil {
			t.Fatalf("Failed to register backend: %v", err)
		}

		backends := r.ListBackends()
		if len(backends) != 1 {
			t.Errorf("Expected 1 backend, got %d", len(backends))
		}
	})
}

// TestEnsembleIntegration tests the ensemble processor integration.
func TestEnsembleIntegration(t *testing.T) {
	cfg := ensemble.DefaultProcessorConfig()
	cfg.EnableMetrics = false
	ep := ensemble.NewEnsembleProcessor(cfg, nil)

	// Add mock models
	model1 := &mockEnsembleModel{
		name:       "model1",
		provider:   "mlx",
		isLocal:    true,
		response:   "answer A",
		confidence: 0.9,
	}
	model2 := &mockEnsembleModel{
		name:       "model2",
		provider:   "gemini",
		isLocal:    false,
		response:   "answer A",
		confidence: 0.85,
	}
	model3 := &mockEnsembleModel{
		name:       "model3",
		provider:   "openai",
		isLocal:    false,
		response:   "answer B",
		confidence: 0.7,
	}

	_ = ep.AddModel(model1)
	_ = ep.AddModel(model2)
	_ = ep.AddModel(model3)

	t.Run("ParallelStrategy", func(t *testing.T) {
		req := &ensemble.EnsembleRequest{
			ID:           "test-1",
			Type:         ensemble.RequestTypeClassification,
			Content:      "Test content",
			Strategy:     ensemble.StrategyParallel,
			Timeout:      5 * time.Second,
			MinResponses: 1,
		}

		result, err := ep.Process(context.Background(), req)
		if err != nil {
			t.Fatalf("Process failed: %v", err)
		}

		if len(result.IndividualResults) != 3 {
			t.Errorf("Expected 3 individual results, got %d", len(result.IndividualResults))
		}
	})

	t.Run("MajorityVotingStrategy", func(t *testing.T) {
		req := &ensemble.EnsembleRequest{
			ID:           "test-2",
			Type:         ensemble.RequestTypeClassification,
			Content:      "Test content",
			Strategy:     ensemble.StrategyMajorityVoting,
			Timeout:      5 * time.Second,
			MinResponses: 2,
		}

		result, err := ep.Process(context.Background(), req)
		if err != nil {
			t.Fatalf("Process failed: %v", err)
		}

		// answer A should win (2 votes vs 1)
		if result.FinalResponse != "answer A" {
			t.Errorf("Expected 'answer A', got %v", result.FinalResponse)
		}

		// Consensus should be ~0.67
		if result.ConsensusScore < 0.6 || result.ConsensusScore > 0.7 {
			t.Errorf("Expected consensus score around 0.67, got %f", result.ConsensusScore)
		}
	})

	t.Run("SupportedStrategies", func(t *testing.T) {
		strategies := ep.GetSupportedStrategies()
		if len(strategies) == 0 {
			t.Error("Expected built-in strategies to be registered")
		}
	})
}

// TestCircuitBreakerIntegration tests circuit breaker behavior in the router.
func TestCircuitBreakerIntegration(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()

	routerCfg := &router.RouterConfig{
		DefaultTimeout: 5 * time.Second,
		CircuitBreakerConfig: &router.CircuitBreakerConfig{
			FailureThreshold: 2,
			SuccessThreshold: 1,
			ResetTimeout:     50 * time.Millisecond,
		},
		EnableMetrics: false,
		Logger:        logger,
	}
	r := router.NewModelRouter(routerCfg)
	defer func() { _ = r.Shutdown(ctx) }()

	// Create failing backend
	failingBackend := newMockBackend("failing", "test", true, []string{"chat"})
	failingBackend.processFunc = func(ctx context.Context, req *router.Request) (*router.Response, error) {
		return nil, errors.New("backend error")
	}

	if err := r.RegisterBackend(failingBackend); err != nil {
		t.Fatalf("Failed to register backend: %v", err)
	}

	// Trigger circuit breaker by failing twice
	req := &router.Request{
		ID:      "test",
		Type:    router.RequestTypeEmbedding,
		Content: "test",
	}

	_, _ = r.Route(ctx, req)
	_, _ = r.Route(ctx, req)

	// Check circuit is open
	stats := r.GetCircuitStats("failing")
	if stats == nil {
		t.Fatal("Expected circuit stats")
	}
	if stats.State != router.CircuitOpen {
		t.Errorf("Expected circuit open, got %s", stats.State.String())
	}

	// Wait for reset timeout
	time.Sleep(60 * time.Millisecond)

	// Fix the backend
	failingBackend.processFunc = nil

	// Should be half-open now and allow a test request
	resp, err := r.Route(ctx, req)
	if err != nil {
		t.Fatalf("Expected request to succeed after reset timeout: %v", err)
	}
	if resp == nil {
		t.Error("Expected response after circuit recovery")
	}
}

// ============================================================================
// Helper Types
// ============================================================================

// mockCostStorage implements cost.Storage for testing.
type mockCostStorage struct {
	records []*cost.CostRecord
	budgets map[string]*cost.Budget
}

func (m *mockCostStorage) SaveRecord(ctx context.Context, record *cost.CostRecord) error {
	m.records = append(m.records, record)
	return nil
}

func (m *mockCostStorage) GetRecord(ctx context.Context, id string) (*cost.CostRecord, error) {
	for _, r := range m.records {
		if r.ID == id {
			return r, nil
		}
	}
	return nil, cost.ErrRecordNotFound
}

func (m *mockCostStorage) QueryRecords(ctx context.Context, filter *cost.CostFilter) ([]*cost.CostRecord, error) {
	return m.records, nil
}

func (m *mockCostStorage) GetCostReport(ctx context.Context, filter *cost.CostFilter, granularity cost.TimeRange) (*cost.CostReport, error) {
	return &cost.CostReport{TenantID: filter.TenantID, GeneratedAt: time.Now()}, nil
}

func (m *mockCostStorage) SaveBudget(ctx context.Context, budget *cost.Budget) error {
	if m.budgets == nil {
		m.budgets = make(map[string]*cost.Budget)
	}
	m.budgets[budget.TenantID] = budget
	return nil
}

func (m *mockCostStorage) GetBudget(ctx context.Context, tenantID string) (*cost.Budget, error) {
	if m.budgets == nil {
		return nil, cost.ErrBudgetNotFound
	}
	budget, ok := m.budgets[tenantID]
	if !ok {
		return nil, cost.ErrBudgetNotFound
	}
	return budget, nil
}

func (m *mockCostStorage) UpdateBudgetSpending(ctx context.Context, tenantID string, amount float64) (*cost.Budget, error) {
	budget, err := m.GetBudget(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	budget.UpdateSpending(amount)
	return budget, nil
}

func (m *mockCostStorage) GetDailySpending(ctx context.Context, tenantID string) (float64, error) {
	var total float64
	for _, r := range m.records {
		if r.TenantID == tenantID {
			total += r.TotalCost
		}
	}
	return total, nil
}

func (m *mockCostStorage) GetMonthlySpending(ctx context.Context, tenantID string) (float64, error) {
	return m.GetDailySpending(ctx, tenantID)
}

func (m *mockCostStorage) Close() error {
	return nil
}

// mockEscalationProcessor implements escalation.ModelProcessor for testing.
type mockEscalationProcessor struct {
	processFunc func(ctx context.Context, req *escalation.EscalationRequest, model string, tier escalation.TierLevel) (*escalation.EscalationResult, error)
	callCount   int
}

func (m *mockEscalationProcessor) Process(ctx context.Context, req *escalation.EscalationRequest, model string, tier escalation.TierLevel) (*escalation.EscalationResult, error) {
	m.callCount++
	if m.processFunc != nil {
		return m.processFunc(ctx, req, model, tier)
	}
	return &escalation.EscalationResult{
		RequestID:      req.ID,
		ModelUsed:      model,
		TierUsed:       tier,
		Confidence:     0.85,
		ProcessingTime: 100 * time.Millisecond,
	}, nil
}

// mockEnsembleModel implements ensemble.ModelBackend for testing.
type mockEnsembleModel struct {
	name       string
	provider   string
	isLocal    bool
	response   interface{}
	confidence float64
	err        error
}

func (m *mockEnsembleModel) Name() string             { return m.name }
func (m *mockEnsembleModel) Provider() string         { return m.provider }
func (m *mockEnsembleModel) IsLocal() bool            { return m.isLocal }
func (m *mockEnsembleModel) CostPer1KTokens() float64 { return 0.001 }
func (m *mockEnsembleModel) QualityScore() float64    { return 0.8 }

func (m *mockEnsembleModel) Process(ctx context.Context, content string, reqType ensemble.RequestType, options map[string]interface{}) (*ensemble.ModelResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &ensemble.ModelResult{
		ModelID:    m.name,
		Provider:   m.provider,
		Response:   m.response,
		Confidence: m.confidence,
		TokensUsed: 100,
	}, nil
}

// ============================================================================
// Benchmark Tests
// ============================================================================

func BenchmarkFullPipeline(b *testing.B) {
	logger := testLogger()
	ctx := context.Background()

	// Set up router
	routerCfg := &router.RouterConfig{
		DefaultTimeout: 5 * time.Second,
		EnableMetrics:  false,
		Logger:         logger,
	}
	r := router.NewModelRouter(routerCfg)
	defer func() { _ = r.Shutdown(ctx) }()

	backend := newMockBackend("bench", "test", true, []string{"chat"})
	_ = r.RegisterBackend(backend)

	req := &router.Request{
		ID:       "bench",
		Type:     router.RequestTypeEmbedding,
		Content:  "benchmark content",
		TenantID: "benchmark",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = r.Route(ctx, req)
	}
}

func BenchmarkModelSelection(b *testing.B) {
	logger := testLogger()
	ctx := context.Background()

	selectorCfg := &selector.SelectorConfig{
		EnableMetrics: false,
	}
	s := selector.NewModelSelector(selectorCfg, logger)

	// Register multiple models
	for i := 0; i < 10; i++ {
		_ = s.RegisterModel(&selector.ModelConfig{
			ModelID:  "model-" + string(rune('a'+i)),
			Provider: selector.ModelProviderMLX,
			Capabilities: &selector.ModelCapabilities{
				ModelID:        "model-" + string(rune('a'+i)),
				SupportedTasks: []selector.TaskType{selector.TaskTypeSummarization},
				IsAvailable:    true,
				QualityScore:   0.7 + float64(i)*0.02,
			},
		})
	}

	criteria := &selector.SelectionCriteria{
		TaskType: selector.TaskTypeSummarization,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = s.SelectModel(ctx, criteria)
	}
}

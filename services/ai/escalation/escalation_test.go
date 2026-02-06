package escalation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/otherjamesbrown/penfold/services/ai/testutil"
)

// MockModelProcessor is a mock implementation of ModelProcessor for testing.
type MockModelProcessor struct {
	ProcessFunc func(ctx context.Context, req *EscalationRequest, model string, tier TierLevel) (*EscalationResult, error)
	CallCount   int
}

func (m *MockModelProcessor) Process(ctx context.Context, req *EscalationRequest, model string, tier TierLevel) (*EscalationResult, error) {
	m.CallCount++
	if m.ProcessFunc != nil {
		return m.ProcessFunc(ctx, req, model, tier)
	}
	return &EscalationResult{
		RequestID:      req.ID,
		ModelUsed:      model,
		TierUsed:       tier,
		Confidence:     0.85,
		ProcessingTime: 100 * time.Millisecond,
	}, nil
}

func TestTierRegistry(t *testing.T) {
	t.Run("NewTierRegistry with default config", func(t *testing.T) {
		registry := NewTierRegistry(nil)
		if registry == nil {
			t.Fatal("expected registry to be created")
		}

		// Should have all three tiers
		tiers := registry.GetAllTiers()
		if len(tiers) != 3 {
			t.Errorf("expected 3 tiers, got %d", len(tiers))
		}
	})

	t.Run("GetTier returns correct tier", func(t *testing.T) {
		registry := NewTierRegistry(nil)

		tier, err := registry.GetTier(TierLevelLocal)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if tier.Level != TierLevelLocal {
			t.Errorf("expected local tier, got %v", tier.Level)
		}
		if !tier.IsLocal {
			t.Error("expected tier to be local")
		}
	})

	t.Run("GetTierForModel returns correct tier", func(t *testing.T) {
		// Discover available MLX models dynamically
		mlxModels := testutil.DiscoverMLXModels()
		if !mlxModels.Available() {
			t.Skip("MLX server unavailable, skipping GetTierForModel test")
		}

		// Create a custom tier config with discovered MLX models
		config := DefaultTierConfig()
		// Add discovered MLX models to the local tier
		localTier := config.Tiers[0]
		localTier.Models = append(localTier.Models, mlxModels.Models...)
		if len(mlxModels.Models) > 0 {
			localTier.PreferredModel = mlxModels.Models[0]
		}

		registry := NewTierRegistry(config)

		// Test that discovered MLX model is in local tier
		firstModel := mlxModels.GetFirst()
		level, err := registry.GetTierForModel(firstModel)
		if err != nil {
			t.Fatalf("unexpected error for MLX model %q: %v", firstModel, err)
		}
		if level != TierLevelLocal {
			t.Errorf("expected local tier for MLX model %q, got %v", firstModel, level)
		}

		// Test that cloud premium model is still in premium tier
		level, err = registry.GetTierForModel("gemini-2.5-pro")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if level != TierLevelCloudPremium {
			t.Errorf("expected premium tier, got %v", level)
		}
	})

	t.Run("GetNextTier returns next tier", func(t *testing.T) {
		registry := NewTierRegistry(nil)

		tier, err := registry.GetNextTier(TierLevelLocal)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if tier.Level != TierLevelCloudStandard {
			t.Errorf("expected cloud standard, got %v", tier.Level)
		}
	})

	t.Run("GetNextTier returns error at max tier", func(t *testing.T) {
		registry := NewTierRegistry(nil)

		_, err := registry.GetNextTier(TierLevelCloudPremium)
		if !errors.Is(err, ErrAlreadyAtMaxTier) {
			t.Errorf("expected ErrAlreadyAtMaxTier, got %v", err)
		}
	})

	t.Run("GetEscalationPath returns full path", func(t *testing.T) {
		registry := NewTierRegistry(nil)

		path, err := registry.GetEscalationPath(TierLevelLocal)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(path) != 3 {
			t.Errorf("expected 3 tiers in path, got %d", len(path))
		}
	})

	t.Run("EstimateEscalationCost calculates correctly", func(t *testing.T) {
		registry := NewTierRegistry(nil)

		cost, err := registry.EstimateEscalationCost(TierLevelLocal, TierLevelCloudStandard)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// From local (0) to standard (1.0), should return 1.0
		if cost != 1.0 {
			t.Errorf("expected cost 1.0, got %f", cost)
		}
	})
}

func TestModelTier(t *testing.T) {
	t.Run("CanHandle with matching capabilities", func(t *testing.T) {
		tier := &ModelTier{
			Capabilities: []string{"embedding", "summarization"},
		}

		if !tier.CanHandle([]string{"embedding"}) {
			t.Error("expected tier to handle embedding")
		}
		if !tier.CanHandle([]string{}) {
			t.Error("expected tier to handle empty capabilities")
		}
	})

	t.Run("CanHandle with missing capability", func(t *testing.T) {
		tier := &ModelTier{
			Capabilities: []string{"embedding"},
		}

		if tier.CanHandle([]string{"reasoning"}) {
			t.Error("expected tier to not handle reasoning")
		}
	})

	t.Run("HasModel finds model", func(t *testing.T) {
		tier := &ModelTier{
			Models: []string{"model1", "model2"},
		}

		if !tier.HasModel("model1") {
			t.Error("expected tier to have model1")
		}
		if tier.HasModel("model3") {
			t.Error("expected tier to not have model3")
		}
	})
}

func TestConfidenceTrigger(t *testing.T) {
	t.Run("does not trigger above threshold", func(t *testing.T) {
		trigger := NewConfidenceTrigger(0.7, 0.4)
		request := &EscalationRequest{ID: "test"}
		result := &EscalationResult{Confidence: 0.8}

		tr := trigger.Evaluate(context.Background(), request, result)
		if tr.ShouldEscalate {
			t.Error("expected no escalation above threshold")
		}
	})

	t.Run("triggers below threshold", func(t *testing.T) {
		trigger := NewConfidenceTrigger(0.7, 0.4)
		request := &EscalationRequest{ID: "test"}
		result := &EscalationResult{Confidence: 0.5}

		tr := trigger.Evaluate(context.Background(), request, result)
		if !tr.ShouldEscalate {
			t.Error("expected escalation below threshold")
		}
		if tr.RecommendedTier != TierLevelCloudStandard {
			t.Errorf("expected cloud standard tier, got %v", tr.RecommendedTier)
		}
	})

	t.Run("triggers critical below critical threshold", func(t *testing.T) {
		trigger := NewConfidenceTrigger(0.7, 0.4)
		request := &EscalationRequest{ID: "test"}
		result := &EscalationResult{Confidence: 0.3}

		tr := trigger.Evaluate(context.Background(), request, result)
		if !tr.ShouldEscalate {
			t.Error("expected escalation below critical threshold")
		}
		if tr.RecommendedTier != TierLevelCloudPremium {
			t.Errorf("expected cloud premium tier, got %v", tr.RecommendedTier)
		}
		if tr.Severity != 1.0 {
			t.Errorf("expected severity 1.0, got %f", tr.Severity)
		}
	})
}

func TestErrorTrigger(t *testing.T) {
	t.Run("does not trigger without error", func(t *testing.T) {
		trigger := NewErrorTrigger(2)
		request := &EscalationRequest{ID: "test"}
		result := &EscalationResult{Error: nil}

		tr := trigger.Evaluate(context.Background(), request, result)
		if tr.ShouldEscalate {
			t.Error("expected no escalation without error")
		}
	})

	t.Run("triggers with error", func(t *testing.T) {
		trigger := NewErrorTrigger(2)
		request := &EscalationRequest{ID: "test"}
		result := &EscalationResult{Error: errors.New("model error")}

		tr := trigger.Evaluate(context.Background(), request, result)
		if !tr.ShouldEscalate {
			t.Error("expected escalation with error")
		}
	})

	t.Run("triggers premium for context length error", func(t *testing.T) {
		trigger := NewErrorTrigger(2)
		request := &EscalationRequest{ID: "test"}
		result := &EscalationResult{Error: errors.New("context length exceeded")}

		tr := trigger.Evaluate(context.Background(), request, result)
		if tr.RecommendedTier != TierLevelCloudPremium {
			t.Errorf("expected cloud premium tier for context error, got %v", tr.RecommendedTier)
		}
	})
}

func TestContentTrigger(t *testing.T) {
	t.Run("does not trigger for normal content", func(t *testing.T) {
		trigger := NewContentTrigger(0.8, 50000)
		request := &EscalationRequest{
			ID:          "test",
			Content:     "short content",
			ContentType: "general",
			TaskType:    "summarization",
		}
		result := &EscalationResult{}

		tr := trigger.Evaluate(context.Background(), request, result)
		if tr.ShouldEscalate {
			t.Error("expected no escalation for normal content")
		}
	})

	t.Run("triggers for long content", func(t *testing.T) {
		trigger := NewContentTrigger(0.8, 100)
		request := &EscalationRequest{
			ID:      "test",
			Content: string(make([]byte, 200)),
		}
		result := &EscalationResult{}

		tr := trigger.Evaluate(context.Background(), request, result)
		if !tr.ShouldEscalate {
			t.Error("expected escalation for long content")
		}
	})

	t.Run("triggers premium for sensitive content", func(t *testing.T) {
		trigger := NewContentTrigger(0.8, 50000)
		request := &EscalationRequest{
			ID:          "test",
			Content:     "some content",
			ContentType: "legal",
		}
		result := &EscalationResult{}

		tr := trigger.Evaluate(context.Background(), request, result)
		if !tr.ShouldEscalate {
			t.Error("expected escalation for sensitive content")
		}
		if tr.RecommendedTier != TierLevelCloudPremium {
			t.Errorf("expected cloud premium tier, got %v", tr.RecommendedTier)
		}
	})
}

func TestUserTrigger(t *testing.T) {
	t.Run("does not trigger without user request", func(t *testing.T) {
		trigger := NewUserTrigger()
		request := &EscalationRequest{
			ID:                   "test",
			UserRequestedUpgrade: false,
		}
		result := &EscalationResult{}

		tr := trigger.Evaluate(context.Background(), request, result)
		if tr.ShouldEscalate {
			t.Error("expected no escalation without user request")
		}
	})

	t.Run("triggers with user request", func(t *testing.T) {
		trigger := NewUserTrigger()
		request := &EscalationRequest{
			ID:                   "test",
			UserRequestedUpgrade: true,
			RequestedTier:        TierLevelCloudPremium,
		}
		result := &EscalationResult{}

		tr := trigger.Evaluate(context.Background(), request, result)
		if !tr.ShouldEscalate {
			t.Error("expected escalation with user request")
		}
		if tr.RecommendedTier != TierLevelCloudPremium {
			t.Errorf("expected cloud premium tier, got %v", tr.RecommendedTier)
		}
	})
}

func TestTriggerChain(t *testing.T) {
	t.Run("returns highest severity trigger", func(t *testing.T) {
		chain := NewTriggerChain(ChainModeHighestSeverity)
		chain.Add(NewConfidenceTrigger(0.8, 0.5))
		chain.Add(NewErrorTrigger(2))

		request := &EscalationRequest{ID: "test"}
		result := &EscalationResult{
			Confidence: 0.6,
			Error:      errors.New("some error"),
		}

		results := chain.Evaluate(context.Background(), request, result)
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		// Error trigger should have higher severity
		if results[0].TriggerType != TriggerTypeError {
			t.Errorf("expected error trigger (highest severity), got %v", results[0].TriggerType)
		}
	})

	t.Run("returns first match in first_match mode", func(t *testing.T) {
		chain := NewTriggerChain(ChainModeFirstMatch)
		chain.Add(NewConfidenceTrigger(0.9, 0.5))
		chain.Add(NewErrorTrigger(2))

		request := &EscalationRequest{ID: "test"}
		result := &EscalationResult{
			Confidence: 0.6,
			Error:      errors.New("some error"),
		}

		results := chain.Evaluate(context.Background(), request, result)
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		// Confidence trigger is first
		if results[0].TriggerType != TriggerTypeConfidence {
			t.Errorf("expected confidence trigger (first), got %v", results[0].TriggerType)
		}
	})
}

func TestLowConfidencePolicy(t *testing.T) {
	t.Run("allows escalation for low confidence", func(t *testing.T) {
		policy := NewLowConfidencePolicy()
		request := &EscalationRequest{
			ID:          "test",
			CurrentTier: TierLevelLocal,
		}
		trigger := &TriggerResult{
			Confidence:      0.5,
			RecommendedTier: TierLevelCloudStandard,
		}

		decision := policy.Evaluate(context.Background(), request, trigger)
		if !decision.Allow {
			t.Error("expected policy to allow escalation")
		}
	})

	t.Run("does not allow escalation for high confidence", func(t *testing.T) {
		policy := NewLowConfidencePolicy()
		request := &EscalationRequest{
			ID:          "test",
			CurrentTier: TierLevelLocal,
		}
		trigger := &TriggerResult{
			Confidence:      0.9,
			RecommendedTier: TierLevelCloudStandard,
		}

		decision := policy.Evaluate(context.Background(), request, trigger)
		if decision.Allow {
			t.Error("expected policy to not allow escalation for high confidence")
		}
	})
}

func TestCriticalTaskPolicy(t *testing.T) {
	t.Run("escalates critical task types", func(t *testing.T) {
		policy := NewCriticalTaskPolicy()
		request := &EscalationRequest{
			ID:       "test",
			TaskType: "legal_analysis",
		}
		trigger := &TriggerResult{}

		decision := policy.Evaluate(context.Background(), request, trigger)
		if !decision.Allow {
			t.Error("expected policy to allow escalation for critical task")
		}
		if decision.RecommendedTier != TierLevelCloudPremium {
			t.Errorf("expected cloud premium tier, got %v", decision.RecommendedTier)
		}
	})

	t.Run("escalates critical content types", func(t *testing.T) {
		policy := NewCriticalTaskPolicy()
		request := &EscalationRequest{
			ID:          "test",
			ContentType: "pii",
		}
		trigger := &TriggerResult{}

		decision := policy.Evaluate(context.Background(), request, trigger)
		if !decision.Allow {
			t.Error("expected policy to allow escalation for PII content")
		}
	})
}

func TestCostAwarePolicy(t *testing.T) {
	t.Run("allows escalation within budget", func(t *testing.T) {
		policy := NewCostAwarePolicy(100.0)
		request := &EscalationRequest{
			ID:      "test",
			Content: "short content",
		}
		trigger := &TriggerResult{
			RecommendedTier: TierLevelCloudStandard,
		}

		decision := policy.Evaluate(context.Background(), request, trigger)
		if !decision.Allow {
			t.Error("expected policy to allow escalation within budget")
		}
	})

	t.Run("denies escalation exceeding budget", func(t *testing.T) {
		policy := NewCostAwarePolicy(0.0001) // Very small budget
		policy.DailySpent = 0.0001           // Already spent
		request := &EscalationRequest{
			ID:      "test",
			Content: string(make([]byte, 100000)), // Large content
		}
		trigger := &TriggerResult{
			RecommendedTier: TierLevelCloudPremium,
		}

		decision := policy.Evaluate(context.Background(), request, trigger)
		if !decision.Deny {
			t.Error("expected policy to deny escalation exceeding budget")
		}
	})

	t.Run("records spending correctly", func(t *testing.T) {
		policy := NewCostAwarePolicy(100.0)
		policy.RecordSpending(10.0)
		policy.RecordSpending(5.0)

		spent, remaining := policy.GetBudgetStatus()
		if spent != 15.0 {
			t.Errorf("expected spent 15.0, got %f", spent)
		}
		if remaining != 85.0 {
			t.Errorf("expected remaining 85.0, got %f", remaining)
		}
	})
}

func TestTenantPolicy(t *testing.T) {
	t.Run("applies tenant-specific config", func(t *testing.T) {
		policy := NewTenantPolicy()
		policy.SetTenantConfig("tenant1", &TenantEscalationConfig{
			MaxTier:      TierLevelCloudStandard,
			AutoEscalate: true,
		})

		request := &EscalationRequest{
			ID:       "test",
			TenantID: "tenant1",
		}
		trigger := &TriggerResult{
			RecommendedTier: TierLevelCloudPremium,
		}

		decision := policy.Evaluate(context.Background(), request, trigger)
		if !decision.Allow {
			t.Error("expected policy to allow escalation")
		}
		if decision.RecommendedTier != TierLevelCloudStandard {
			t.Errorf("expected max tier cloud standard, got %v", decision.RecommendedTier)
		}
	})

	t.Run("denies when auto-escalate disabled", func(t *testing.T) {
		policy := NewTenantPolicy()
		policy.SetTenantConfig("tenant1", &TenantEscalationConfig{
			MaxTier:      TierLevelCloudPremium,
			AutoEscalate: false,
		})

		request := &EscalationRequest{
			ID:                   "test",
			TenantID:             "tenant1",
			UserRequestedUpgrade: false,
		}
		trigger := &TriggerResult{
			RecommendedTier: TierLevelCloudStandard,
		}

		decision := policy.Evaluate(context.Background(), request, trigger)
		if !decision.Deny {
			t.Error("expected policy to deny auto-escalation when disabled")
		}
	})
}

func TestPolicyChain(t *testing.T) {
	t.Run("deny decision takes precedence", func(t *testing.T) {
		chain := NewPolicyChain()
		chain.Add(NewLowConfidencePolicy())
		chain.Add(NewCostAwarePolicy(0.0)) // Zero budget - will deny

		request := &EscalationRequest{
			ID:          "test",
			CurrentTier: TierLevelLocal,
			Content:     string(make([]byte, 100000)),
		}
		trigger := &TriggerResult{
			Confidence:      0.5,
			RecommendedTier: TierLevelCloudPremium,
		}

		decision := chain.Evaluate(context.Background(), request, trigger)
		if !decision.Deny {
			t.Error("expected deny decision to take precedence")
		}
	})
}

func TestEscalationManager(t *testing.T) {
	t.Run("NewEscalationManager creates valid manager", func(t *testing.T) {
		manager := NewEscalationManager(nil, nil)
		if manager == nil {
			t.Fatal("expected manager to be created")
		}
		defer func() { _ = manager.Close() }()

		config := manager.GetConfig()
		if config == nil {
			t.Error("expected config to be set")
		}
	})

	t.Run("Evaluate returns correct decision", func(t *testing.T) {
		config := DefaultManagerConfig()
		config.EnableMetrics = false
		manager := NewEscalationManager(config, nil)
		defer func() { _ = manager.Close() }()

		request := &EscalationRequest{
			ID:          "test-1",
			CurrentTier: TierLevelLocal,
			TaskType:    "summarization",
		}
		result := &EscalationResult{
			RequestID:  "test-1",
			Confidence: 0.5, // Below threshold
		}

		decision := manager.Evaluate(request, result)
		if !decision.ShouldEscalate {
			t.Error("expected escalation decision for low confidence")
		}
	})

	t.Run("Evaluate does not escalate for high confidence", func(t *testing.T) {
		config := DefaultManagerConfig()
		config.EnableMetrics = false
		manager := NewEscalationManager(config, nil)
		defer func() { _ = manager.Close() }()

		request := &EscalationRequest{
			ID:          "test-1",
			CurrentTier: TierLevelLocal,
			TaskType:    "embedding", // Task with lower quality requirement
		}
		result := &EscalationResult{
			RequestID:         "test-1",
			Confidence:        0.9, // Above threshold
			ModelQualityScore: 0.8, // Above quality threshold for embedding tasks
		}

		decision := manager.Evaluate(request, result)
		if decision.ShouldEscalate {
			t.Errorf("expected no escalation for high confidence, got reason: %s", decision.Reason)
		}
	})

	t.Run("Escalate processes request at higher tier", func(t *testing.T) {
		config := DefaultManagerConfig()
		config.EnableMetrics = false
		manager := NewEscalationManager(config, nil)
		defer func() { _ = manager.Close() }()

		processor := &MockModelProcessor{}
		manager.SetProcessor(processor)

		request := &EscalationRequest{
			ID:          "test-1",
			CurrentTier: TierLevelLocal,
		}

		result, err := manager.Escalate(context.Background(), request, "low confidence")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.TierUsed != TierLevelCloudStandard {
			t.Errorf("expected cloud standard tier, got %v", result.TierUsed)
		}
		if processor.CallCount != 1 {
			t.Errorf("expected 1 process call, got %d", processor.CallCount)
		}
	})

	t.Run("GetEscalationPath returns valid path", func(t *testing.T) {
		config := DefaultManagerConfig()
		config.EnableMetrics = false
		manager := NewEscalationManager(config, nil)
		defer func() { _ = manager.Close() }()

		path := manager.GetEscalationPath("summarization")
		if len(path) == 0 {
			t.Error("expected non-empty escalation path")
		}
	})

	t.Run("Audit logs are recorded", func(t *testing.T) {
		config := DefaultManagerConfig()
		config.EnableMetrics = false
		config.EnableAuditLog = true
		manager := NewEscalationManager(config, nil)
		defer func() { _ = manager.Close() }()

		processor := &MockModelProcessor{}
		manager.SetProcessor(processor)

		request := &EscalationRequest{
			ID:          "test-1",
			TenantID:    "tenant1",
			CurrentTier: TierLevelLocal,
		}

		_, err := manager.Escalate(context.Background(), request, "test reason")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		logs := manager.GetAuditLogs(10)
		if len(logs) != 1 {
			t.Errorf("expected 1 log entry, got %d", len(logs))
		}
		if logs[0].Reason != "test reason" {
			t.Errorf("expected reason 'test reason', got '%s'", logs[0].Reason)
		}
	})

	t.Run("ProcessWithEscalation auto-escalates", func(t *testing.T) {
		config := DefaultManagerConfig()
		config.EnableMetrics = false
		config.MaxEscalations = 2
		manager := NewEscalationManager(config, nil)
		defer func() { _ = manager.Close() }()

		callCount := 0
		processor := &MockModelProcessor{
			ProcessFunc: func(ctx context.Context, req *EscalationRequest, model string, tier TierLevel) (*EscalationResult, error) {
				callCount++
				// First call returns low confidence, subsequent calls return high
				confidence := 0.5
				if callCount > 1 {
					confidence = 0.9
				}
				return &EscalationResult{
					RequestID:         req.ID,
					ModelUsed:         model,
					TierUsed:          tier,
					Confidence:        confidence,
					ProcessingTime:    100 * time.Millisecond,
					ModelQualityScore: 0.8,
				}, nil
			},
		}
		manager.SetProcessor(processor)

		request := &EscalationRequest{
			ID:          "test-1",
			CurrentTier: TierLevelLocal,
		}

		result, err := manager.ProcessWithEscalation(context.Background(), request)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should have escalated at least once
		if callCount < 2 {
			t.Errorf("expected at least 2 process calls for escalation, got %d", callCount)
		}
		if result.Confidence != 0.9 {
			t.Errorf("expected final confidence 0.9, got %f", result.Confidence)
		}
	})
}

func TestEscalationStats(t *testing.T) {
	t.Run("RecordEscalation tracks counts", func(t *testing.T) {
		stats := NewEscalationStats()
		stats.RecordEscalation(TierLevelLocal, TierLevelCloudStandard, "test", "tenant1")
		stats.RecordEscalation(TierLevelCloudStandard, TierLevelCloudPremium, "test", "tenant1")

		snapshot := stats.Snapshot()
		if snapshot.EscalationCounts[TierLevelCloudStandard] != 1 {
			t.Errorf("expected 1 cloud standard escalation, got %d", snapshot.EscalationCounts[TierLevelCloudStandard])
		}
		if snapshot.EscalationCounts[TierLevelCloudPremium] != 1 {
			t.Errorf("expected 1 cloud premium escalation, got %d", snapshot.EscalationCounts[TierLevelCloudPremium])
		}
	})

	t.Run("GetSuccessRate calculates correctly", func(t *testing.T) {
		stats := NewEscalationStats()
		stats.RecordSuccess(TierLevelLocal)
		stats.RecordSuccess(TierLevelLocal)
		stats.RecordFailure(TierLevelLocal)

		rate := stats.GetSuccessRate(TierLevelLocal)
		expected := 2.0 / 3.0
		if rate < expected-0.01 || rate > expected+0.01 {
			t.Errorf("expected success rate ~%f, got %f", expected, rate)
		}
	})

	t.Run("GetTierDistribution calculates correctly", func(t *testing.T) {
		stats := NewEscalationStats()
		stats.RecordEscalation(TierLevelLocal, TierLevelCloudStandard, "test", "tenant1")
		stats.RecordEscalation(TierLevelLocal, TierLevelCloudStandard, "test", "tenant1")
		stats.RecordEscalation(TierLevelCloudStandard, TierLevelCloudPremium, "test", "tenant1")

		distribution := stats.GetTierDistribution()
		// 2 to standard, 1 to premium
		if distribution[TierLevelCloudStandard] < 0.65 || distribution[TierLevelCloudStandard] > 0.68 {
			t.Errorf("expected ~0.66 for cloud standard, got %f", distribution[TierLevelCloudStandard])
		}
	})

	t.Run("Reset clears all stats", func(t *testing.T) {
		stats := NewEscalationStats()
		stats.RecordEscalation(TierLevelLocal, TierLevelCloudStandard, "test", "tenant1")
		stats.RecordCost(10.0)

		stats.Reset()

		snapshot := stats.Snapshot()
		if len(snapshot.EscalationCounts) != 0 {
			t.Error("expected escalation counts to be cleared")
		}
		if snapshot.TotalCost != 0 {
			t.Error("expected total cost to be cleared")
		}
	})
}

func TestTierLevelString(t *testing.T) {
	tests := []struct {
		level    TierLevel
		expected string
	}{
		{TierLevelLocal, "local"},
		{TierLevelCloudStandard, "cloud-standard"},
		{TierLevelCloudPremium, "cloud-premium"},
		{TierLevel(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if tt.level.String() != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, tt.level.String())
			}
		})
	}
}

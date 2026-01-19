package cost

import (
	"context"
	"testing"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/ai/router"
)

func TestNewCostTracker(t *testing.T) {
	logger := logging.NewLogger(&logging.Config{
		Level:       logging.LevelInfo,
		ServiceName: "test",
		Environment: "test",
	})

	tracker := NewCostTracker(nil, logger)
	if tracker == nil {
		t.Fatal("Expected tracker to be created")
	}

	if tracker.pricing == nil {
		t.Error("Expected pricing table to be initialized")
	}

	if tracker.alerts == nil {
		t.Error("Expected alerts manager to be initialized")
	}
}

func TestCostTracker_TrackRequestWithTokens(t *testing.T) {
	logger := logging.NewLogger(&logging.Config{
		Level:       logging.LevelInfo,
		ServiceName: "test",
		Environment: "test",
	})

	cfg := DefaultTrackerConfig()
	cfg.EnablePersistence = false // Disable persistence for unit tests
	cfg.EnableAlerts = false

	tracker := NewCostTracker(cfg, logger)

	ctx := context.Background()
	err := tracker.TrackRequestWithTokens(
		ctx,
		"tenant-1",
		"req-123",
		"gemini-1.5-flash",
		"gemini",
		"summary",
		"daily-review",
		1000,  // input tokens
		500,   // output tokens
		100*time.Millisecond,
	)

	if err != nil {
		t.Errorf("TrackRequestWithTokens failed: %v", err)
	}
}

func TestCostTracker_EstimateCost(t *testing.T) {
	logger := logging.NewLogger(&logging.Config{
		Level:       logging.LevelInfo,
		ServiceName: "test",
		Environment: "test",
	})

	tracker := NewCostTracker(nil, logger)

	// Test cloud model
	estimate := tracker.EstimateCost("tenant-1", "gemini-1.5-flash", 1000, 500)
	if estimate == nil {
		t.Fatal("Expected estimate to be returned")
	}

	if estimate.EstimatedInputTokens != 1000 {
		t.Errorf("Expected 1000 input tokens, got %d", estimate.EstimatedInputTokens)
	}

	if estimate.EstimatedOutputTokens != 500 {
		t.Errorf("Expected 500 output tokens, got %d", estimate.EstimatedOutputTokens)
	}

	// Gemini Flash: $0.075/1M input, $0.30/1M output
	// Expected: (1000 * 0.000075 / 1000) + (500 * 0.0003 / 1000)
	//         = 0.000075 + 0.00015 = 0.000225
	expectedCost := 0.000225
	if estimate.EstimatedCost < expectedCost*0.9 || estimate.EstimatedCost > expectedCost*1.1 {
		t.Errorf("Expected cost around %f, got %f", expectedCost, estimate.EstimatedCost)
	}

	// Test local model (should be free)
	localEstimate := tracker.EstimateCost("tenant-1", "llama3.2", 1000, 500)
	if localEstimate.EstimatedCost != 0 {
		t.Errorf("Expected 0 cost for local model, got %f", localEstimate.EstimatedCost)
	}
}

func TestCostTracker_CheckBudget(t *testing.T) {
	logger := logging.NewLogger(&logging.Config{
		Level:       logging.LevelInfo,
		ServiceName: "test",
		Environment: "test",
	})

	cfg := DefaultTrackerConfig()
	cfg.EnablePersistence = false
	cfg.EnforceBudgets = true

	tracker := NewCostTracker(cfg, logger)

	// Use mock storage
	mockStorage := &mockStorage{
		budgets: map[string]*Budget{
			"tenant-1": NewBudget("tenant-1", 1.0, 10.0),
		},
	}
	tracker.SetStorage(mockStorage)

	ctx := context.Background()

	// Small cost should pass
	estimate := &EstimatedCost{EstimatedCost: 0.50}
	ok, err := tracker.CheckBudget(ctx, "tenant-1", estimate)
	if err != nil {
		t.Errorf("CheckBudget failed: %v", err)
	}
	if !ok {
		t.Error("Expected budget check to pass for small cost")
	}

	// Cost that would exceed daily budget should fail
	largeEstimate := &EstimatedCost{EstimatedCost: 1.50}
	ok, err = tracker.CheckBudget(ctx, "tenant-1", largeEstimate)
	if err == nil {
		t.Error("Expected error for exceeding budget")
	}
	if ok {
		t.Error("Expected budget check to fail for large cost")
	}

	// Unknown tenant should pass (no budget = unlimited)
	ok, err = tracker.CheckBudget(ctx, "unknown-tenant", largeEstimate)
	if err != nil {
		t.Errorf("CheckBudget failed for unknown tenant: %v", err)
	}
	if !ok {
		t.Error("Expected budget check to pass for unknown tenant")
	}
}

func TestCostTracker_TrackRequest(t *testing.T) {
	logger := logging.NewLogger(&logging.Config{
		Level:       logging.LevelInfo,
		ServiceName: "test",
		Environment: "test",
	})

	cfg := DefaultTrackerConfig()
	cfg.EnablePersistence = false
	cfg.EnableAlerts = false

	tracker := NewCostTracker(cfg, logger)

	ctx := context.Background()
	req := &router.Request{
		ID:       "req-123",
		Type:     router.RequestTypeSummary,
		TenantID: "tenant-1",
		Content:  "This is a test content for summarization.",
	}

	resp := &router.Response{
		RequestID:      "req-123",
		ModelUsed:      "gemini-1.5-flash",
		ProviderUsed:   "gemini",
		TokensUsed:     100,
		ProcessingTime: 500 * time.Millisecond,
	}

	err := tracker.TrackRequest(ctx, req, resp, "test-operation")
	if err != nil {
		t.Errorf("TrackRequest failed: %v", err)
	}
}

// mockStorage implements Storage for testing.
type mockStorage struct {
	records []*CostRecord
	budgets map[string]*Budget
}

func (m *mockStorage) SaveRecord(ctx context.Context, record *CostRecord) error {
	m.records = append(m.records, record)
	return nil
}

func (m *mockStorage) GetRecord(ctx context.Context, id string) (*CostRecord, error) {
	for _, r := range m.records {
		if r.ID == id {
			return r, nil
		}
	}
	return nil, ErrRecordNotFound
}

func (m *mockStorage) QueryRecords(ctx context.Context, filter *CostFilter) ([]*CostRecord, error) {
	var results []*CostRecord
	for _, r := range m.records {
		if r.TenantID == filter.TenantID {
			results = append(results, r)
		}
	}
	return results, nil
}

func (m *mockStorage) GetCostReport(ctx context.Context, filter *CostFilter, granularity TimeRange) (*CostReport, error) {
	return &CostReport{
		TenantID:    filter.TenantID,
		StartTime:   filter.StartTime,
		EndTime:     filter.EndTime,
		GeneratedAt: time.Now(),
	}, nil
}

func (m *mockStorage) SaveBudget(ctx context.Context, budget *Budget) error {
	if m.budgets == nil {
		m.budgets = make(map[string]*Budget)
	}
	m.budgets[budget.TenantID] = budget
	return nil
}

func (m *mockStorage) GetBudget(ctx context.Context, tenantID string) (*Budget, error) {
	if m.budgets == nil {
		return nil, ErrBudgetNotFound
	}
	budget, ok := m.budgets[tenantID]
	if !ok {
		return nil, ErrBudgetNotFound
	}
	return budget, nil
}

func (m *mockStorage) UpdateBudgetSpending(ctx context.Context, tenantID string, amount float64) (*Budget, error) {
	budget, err := m.GetBudget(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	budget.UpdateSpending(amount)
	return budget, nil
}

func (m *mockStorage) GetDailySpending(ctx context.Context, tenantID string) (float64, error) {
	var total float64
	for _, r := range m.records {
		if r.TenantID == tenantID {
			total += r.TotalCost
		}
	}
	return total, nil
}

func (m *mockStorage) GetMonthlySpending(ctx context.Context, tenantID string) (float64, error) {
	return m.GetDailySpending(ctx, tenantID) // Same for mock
}

func (m *mockStorage) Close() error {
	return nil
}

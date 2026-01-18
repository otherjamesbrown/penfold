package cost

import (
	"testing"
	"time"
)

func TestNewCost(t *testing.T) {
	pricing := &ModelPricing{
		Model:           "test-model",
		Provider:        "test-provider",
		InputCostPer1K:  0.001,  // $1 per 1M tokens
		OutputCostPer1K: 0.002,  // $2 per 1M tokens
		IsLocal:         false,
	}

	cost := NewCost(1000, 500, "test-model", "test-provider", pricing)

	if cost.InputTokens != 1000 {
		t.Errorf("Expected 1000 input tokens, got %d", cost.InputTokens)
	}

	if cost.OutputTokens != 500 {
		t.Errorf("Expected 500 output tokens, got %d", cost.OutputTokens)
	}

	if cost.TotalTokens != 1500 {
		t.Errorf("Expected 1500 total tokens, got %d", cost.TotalTokens)
	}

	expectedInputCost := 0.001 // 1000 tokens * 0.001 / 1000
	if cost.InputCost != expectedInputCost {
		t.Errorf("Expected input cost %f, got %f", expectedInputCost, cost.InputCost)
	}

	expectedOutputCost := 0.001 // 500 tokens * 0.002 / 1000
	if cost.OutputCost != expectedOutputCost {
		t.Errorf("Expected output cost %f, got %f", expectedOutputCost, cost.OutputCost)
	}

	expectedTotalCost := 0.002
	if cost.TotalCost != expectedTotalCost {
		t.Errorf("Expected total cost %f, got %f", expectedTotalCost, cost.TotalCost)
	}

	if cost.Currency != "USD" {
		t.Errorf("Expected currency 'USD', got '%s'", cost.Currency)
	}
}

func TestNewBudget(t *testing.T) {
	budget := NewBudget("tenant-1", 10.0, 100.0)

	if budget.TenantID != "tenant-1" {
		t.Errorf("Expected tenant ID 'tenant-1', got '%s'", budget.TenantID)
	}

	if budget.DailyLimit != 10.0 {
		t.Errorf("Expected daily limit 10.0, got %f", budget.DailyLimit)
	}

	if budget.MonthlyLimit != 100.0 {
		t.Errorf("Expected monthly limit 100.0, got %f", budget.MonthlyLimit)
	}

	if budget.DailySpent != 0 {
		t.Errorf("Expected 0 daily spent, got %f", budget.DailySpent)
	}

	if budget.DailyRemaining != 10.0 {
		t.Errorf("Expected 10.0 daily remaining, got %f", budget.DailyRemaining)
	}

	if len(budget.AlertThresholds) != 3 {
		t.Errorf("Expected 3 alert thresholds, got %d", len(budget.AlertThresholds))
	}
}

func TestBudget_UpdateSpending(t *testing.T) {
	budget := NewBudget("tenant-1", 10.0, 100.0)

	budget.UpdateSpending(3.0)

	if budget.DailySpent != 3.0 {
		t.Errorf("Expected 3.0 daily spent, got %f", budget.DailySpent)
	}

	if budget.MonthlySpent != 3.0 {
		t.Errorf("Expected 3.0 monthly spent, got %f", budget.MonthlySpent)
	}

	if budget.DailyRemaining != 7.0 {
		t.Errorf("Expected 7.0 daily remaining, got %f", budget.DailyRemaining)
	}

	if budget.MonthlyRemaining != 97.0 {
		t.Errorf("Expected 97.0 monthly remaining, got %f", budget.MonthlyRemaining)
	}

	// Update again
	budget.UpdateSpending(8.0)

	if budget.DailyRemaining != 0 {
		t.Errorf("Expected 0 daily remaining (exceeded), got %f", budget.DailyRemaining)
	}
}

func TestBudget_UsagePercent(t *testing.T) {
	budget := NewBudget("tenant-1", 10.0, 100.0)
	budget.UpdateSpending(5.0)

	dailyPercent := budget.DailyUsagePercent()
	if dailyPercent != 50.0 {
		t.Errorf("Expected 50%% daily usage, got %f%%", dailyPercent)
	}

	monthlyPercent := budget.MonthlyUsagePercent()
	if monthlyPercent != 5.0 {
		t.Errorf("Expected 5%% monthly usage, got %f%%", monthlyPercent)
	}
}

func TestBudget_IsOverBudget(t *testing.T) {
	budget := NewBudget("tenant-1", 10.0, 100.0)

	if budget.IsOverDailyBudget() {
		t.Error("Should not be over daily budget initially")
	}

	budget.UpdateSpending(10.0)

	if !budget.IsOverDailyBudget() {
		t.Error("Should be over daily budget after spending limit")
	}

	if budget.IsOverMonthlyBudget() {
		t.Error("Should not be over monthly budget yet")
	}
}

func TestBudget_CheckedAlertThresholds(t *testing.T) {
	budget := NewBudget("tenant-1", 10.0, 100.0)

	// No thresholds crossed at 0%
	triggered := budget.CheckedAlertThresholds(0)
	if len(triggered) != 0 {
		t.Errorf("Expected 0 triggered thresholds at 0%%, got %d", len(triggered))
	}

	// 50% threshold crossed
	triggered = budget.CheckedAlertThresholds(50)
	if len(triggered) != 1 || triggered[0] != 0.50 {
		t.Errorf("Expected 50%% threshold triggered, got %v", triggered)
	}

	// Mark it as sent
	budget.MarkAlertSent(0.50)

	// Check again - should not trigger again
	triggered = budget.CheckedAlertThresholds(50)
	if len(triggered) != 0 {
		t.Errorf("Expected 0 triggered after marking sent, got %d", len(triggered))
	}

	// 80% threshold crossed
	triggered = budget.CheckedAlertThresholds(80)
	if len(triggered) != 1 || triggered[0] != 0.80 {
		t.Errorf("Expected 80%% threshold triggered, got %v", triggered)
	}

	// 100% crosses all remaining thresholds
	triggered = budget.CheckedAlertThresholds(100)
	if len(triggered) != 2 { // 80% and 100%
		t.Errorf("Expected 2 triggered thresholds at 100%%, got %d", len(triggered))
	}
}

func TestBudget_Reset(t *testing.T) {
	budget := NewBudget("tenant-1", 10.0, 100.0)
	budget.UpdateSpending(5.0)
	budget.MarkAlertSent(0.50)

	// Reset daily
	budget.ResetDaily()

	if budget.DailySpent != 0 {
		t.Errorf("Expected 0 daily spent after reset, got %f", budget.DailySpent)
	}

	if budget.DailyRemaining != 10.0 {
		t.Errorf("Expected 10.0 daily remaining after reset, got %f", budget.DailyRemaining)
	}

	// Monthly should still have spending
	if budget.MonthlySpent != 5.0 {
		t.Errorf("Expected 5.0 monthly spent after daily reset, got %f", budget.MonthlySpent)
	}

	// Reset monthly
	budget.ResetMonthly()

	if budget.MonthlySpent != 0 {
		t.Errorf("Expected 0 monthly spent after monthly reset, got %f", budget.MonthlySpent)
	}

	if budget.MonthlyRemaining != 100.0 {
		t.Errorf("Expected 100.0 monthly remaining after reset, got %f", budget.MonthlyRemaining)
	}

	// Alerts should be cleared
	if len(budget.AlertsSent) != 0 {
		t.Errorf("Expected alerts to be cleared after monthly reset, got %d", len(budget.AlertsSent))
	}
}

func TestDefaultAlertThresholds(t *testing.T) {
	thresholds := DefaultAlertThresholds()

	if len(thresholds) != 3 {
		t.Fatalf("Expected 3 default thresholds, got %d", len(thresholds))
	}

	expected := []float64{0.50, 0.80, 1.00}
	for i, threshold := range thresholds {
		if threshold != expected[i] {
			t.Errorf("Expected threshold[%d] = %f, got %f", i, expected[i], threshold)
		}
	}
}

func TestCostFilter(t *testing.T) {
	now := time.Now()
	filter := &CostFilter{
		TenantID:     "tenant-1",
		StartTime:    now.Add(-24 * time.Hour),
		EndTime:      now,
		Models:       []string{"gemini-1.5-pro"},
		Providers:    []string{"gemini"},
		RequestTypes: []string{"summary"},
		Operations:   []string{"daily-review"},
		Limit:        100,
		Offset:       0,
	}

	if filter.TenantID != "tenant-1" {
		t.Errorf("Expected tenant ID 'tenant-1', got '%s'", filter.TenantID)
	}

	if len(filter.Models) != 1 {
		t.Errorf("Expected 1 model filter, got %d", len(filter.Models))
	}
}

func TestTimeRange(t *testing.T) {
	ranges := []TimeRange{
		TimeRangeHour,
		TimeRangeDay,
		TimeRangeWeek,
		TimeRangeMonth,
	}

	for _, r := range ranges {
		if r == "" {
			t.Error("TimeRange constant should not be empty")
		}
	}
}

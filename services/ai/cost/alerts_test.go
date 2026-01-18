package cost

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

func TestNewAlertManager(t *testing.T) {
	logger := logging.NewLogger(&logging.Config{
		Level:       logging.LevelInfo,
		ServiceName: "test",
		Environment: "test",
	})

	manager := NewAlertManager(nil, logger)
	if manager == nil {
		t.Fatal("Expected alert manager to be created")
	}

	if manager.anomalyDetector == nil {
		t.Error("Expected anomaly detector to be initialized")
	}
}

func TestAlertManager_RegisterHandler(t *testing.T) {
	logger := logging.NewLogger(&logging.Config{
		Level:       logging.LevelInfo,
		ServiceName: "test",
		Environment: "test",
	})

	manager := NewAlertManager(nil, logger)

	var receivedAlert *Alert
	var mu sync.Mutex

	handler := func(ctx context.Context, alert *Alert) error {
		mu.Lock()
		receivedAlert = alert
		mu.Unlock()
		return nil
	}

	manager.RegisterHandler(handler)

	// Create and send a test alert
	ctx := context.Background()
	budget := NewBudget("test-tenant", 10.0, 100.0)
	budget.UpdateSpending(6.0) // 60% of daily

	alerts, err := manager.CheckBudgetThresholds(ctx, budget)
	if err != nil {
		t.Errorf("CheckBudgetThresholds failed: %v", err)
	}

	if len(alerts) != 1 {
		t.Errorf("Expected 1 alert (50%% threshold), got %d", len(alerts))
	}

	// Give time for async alert processing
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	if receivedAlert == nil {
		t.Error("Expected handler to receive alert")
	} else if receivedAlert.Type != AlertTypeBudgetThreshold {
		t.Errorf("Expected budget threshold alert, got %s", receivedAlert.Type)
	}
	mu.Unlock()
}

func TestAlertManager_CheckBudgetThresholds(t *testing.T) {
	logger := logging.NewLogger(&logging.Config{
		Level:       logging.LevelInfo,
		ServiceName: "test",
		Environment: "test",
	})

	ctx := context.Background()
	_ = logger // Used to create managers in subtests

	tests := []struct {
		name           string
		dailySpent     float64
		dailyLimit     float64
		expectedAlerts int
	}{
		{
			name:           "no threshold crossed",
			dailySpent:     3.0,
			dailyLimit:     10.0,
			expectedAlerts: 0,
		},
		{
			name:           "50% threshold crossed",
			dailySpent:     5.0,
			dailyLimit:     10.0,
			expectedAlerts: 1,
		},
		{
			name:           "80% threshold crossed",
			dailySpent:     8.0,
			dailyLimit:     10.0,
			expectedAlerts: 2, // 50% + 80%
		},
		{
			name:           "100% threshold crossed (exceeded)",
			dailySpent:     10.0,
			dailyLimit:     10.0,
			expectedAlerts: 4, // 50% + 80% + 100% + exceeded
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fresh manager to avoid deduplication
			mgr := NewAlertManager(&AlertManagerConfig{HistoryTTL: 1 * time.Millisecond}, logger)

			budget := NewBudget("test-tenant", tt.dailyLimit, 100.0)
			budget.UpdateSpending(tt.dailySpent)

			alerts, err := mgr.CheckBudgetThresholds(ctx, budget)
			if err != nil {
				t.Errorf("CheckBudgetThresholds failed: %v", err)
			}

			if len(alerts) != tt.expectedAlerts {
				t.Errorf("Expected %d alerts, got %d", tt.expectedAlerts, len(alerts))
			}
		})
	}
}

func TestAlertManager_DuplicatePrevention(t *testing.T) {
	logger := logging.NewLogger(&logging.Config{
		Level:       logging.LevelInfo,
		ServiceName: "test",
		Environment: "test",
	})

	manager := NewAlertManager(&AlertManagerConfig{
		HistoryTTL: 1 * time.Hour, // Long TTL to ensure deduplication
	}, logger)

	ctx := context.Background()

	budget := NewBudget("test-tenant", 10.0, 100.0)
	budget.UpdateSpending(5.0) // 50% threshold

	// First check should produce an alert
	alerts1, _ := manager.CheckBudgetThresholds(ctx, budget)
	if len(alerts1) != 1 {
		t.Errorf("Expected 1 alert on first check, got %d", len(alerts1))
	}

	// Second check should not produce the same alert (duplicate)
	alerts2, _ := manager.CheckBudgetThresholds(ctx, budget)
	if len(alerts2) != 0 {
		t.Errorf("Expected 0 alerts on duplicate check, got %d", len(alerts2))
	}
}

func TestAlertManager_CheckAnomaly(t *testing.T) {
	logger := logging.NewLogger(&logging.Config{
		Level:       logging.LevelInfo,
		ServiceName: "test",
		Environment: "test",
	})

	manager := NewAlertManager(&AlertManagerConfig{
		HistoryTTL:       1 * time.Millisecond,
		AnomalyThreshold: 2.0,
	}, logger)

	ctx := context.Background()

	// Normal spending (below 2x average) - no anomaly
	alert, err := manager.CheckAnomaly(ctx, "test-tenant", 1.5, 1.0)
	if err != nil {
		t.Errorf("CheckAnomaly failed: %v", err)
	}
	if alert != nil {
		t.Error("Expected no anomaly alert for normal spending")
	}

	// Anomalous spending (above 2x average)
	alert, err = manager.CheckAnomaly(ctx, "test-tenant", 3.0, 1.0)
	if err != nil {
		t.Errorf("CheckAnomaly failed: %v", err)
	}
	if alert == nil {
		t.Error("Expected anomaly alert for high spending")
	} else {
		if alert.Type != AlertTypeAnomaly {
			t.Errorf("Expected anomaly alert type, got %s", alert.Type)
		}
		if alert.Severity != SeverityWarning {
			t.Errorf("Expected warning severity, got %s", alert.Severity)
		}
	}
}

func TestAlertManager_CreateDailyDigest(t *testing.T) {
	logger := logging.NewLogger(&logging.Config{
		Level:       logging.LevelInfo,
		ServiceName: "test",
		Environment: "test",
	})

	manager := NewAlertManager(nil, logger)
	ctx := context.Background()

	report := &CostReport{
		TenantID:          "test-tenant",
		StartTime:         time.Now().Add(-24 * time.Hour),
		EndTime:           time.Now(),
		TotalCost:         5.50,
		TotalRequests:     150,
		TotalInputTokens:  50000,
		TotalOutputTokens: 25000,
	}

	alert, err := manager.CreateDailyDigest(ctx, "test-tenant", report)
	if err != nil {
		t.Errorf("CreateDailyDigest failed: %v", err)
	}

	if alert == nil {
		t.Fatal("Expected digest alert to be created")
	}

	if alert.Type != AlertTypeDailyDigest {
		t.Errorf("Expected daily digest type, got %s", alert.Type)
	}

	if alert.Severity != SeverityInfo {
		t.Errorf("Expected info severity, got %s", alert.Severity)
	}

	if alert.CurrentValue != 5.50 {
		t.Errorf("Expected current value 5.50, got %f", alert.CurrentValue)
	}
}

func TestAnomalyDetector(t *testing.T) {
	tests := []struct {
		name      string
		threshold float64
		current   float64
		average   float64
		expected  bool
	}{
		{
			name:      "normal spending",
			threshold: 2.0,
			current:   1.5,
			average:   1.0,
			expected:  false,
		},
		{
			name:      "exactly at threshold",
			threshold: 2.0,
			current:   2.0,
			average:   1.0,
			expected:  false, // Not greater than
		},
		{
			name:      "above threshold",
			threshold: 2.0,
			current:   2.5,
			average:   1.0,
			expected:  true,
		},
		{
			name:      "zero average",
			threshold: 2.0,
			current:   5.0,
			average:   0,
			expected:  false, // Can't detect without baseline
		},
		{
			name:      "high threshold",
			threshold: 5.0,
			current:   4.0,
			average:   1.0,
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detector := NewAnomalyDetector(tt.threshold)
			result := detector.IsAnomaly(tt.current, tt.average)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestAlertSeverity(t *testing.T) {
	logger := logging.NewLogger(&logging.Config{
		Level:       logging.LevelInfo,
		ServiceName: "test",
		Environment: "test",
	})

	manager := NewAlertManager(nil, logger)

	tests := []struct {
		threshold float64
		expected  AlertSeverity
	}{
		{0.30, SeverityInfo},
		{0.50, SeverityInfo},
		{0.80, SeverityWarning},
		{0.90, SeverityWarning},
		{1.00, SeverityCritical},
		{1.50, SeverityCritical},
	}

	for _, tt := range tests {
		name := fmt.Sprintf("%s_threshold_%.0f", tt.expected, tt.threshold*100)
		t.Run(name, func(t *testing.T) {
			severity := manager.getSeverityForThreshold(tt.threshold)
			if severity != tt.expected {
				t.Errorf("For threshold %f, expected %s, got %s", tt.threshold, tt.expected, severity)
			}
		})
	}
}

func TestLogAlertHandler(t *testing.T) {
	logger := logging.NewLogger(&logging.Config{
		Level:       logging.LevelInfo,
		ServiceName: "test",
		Environment: "test",
	})

	handler := LogAlertHandler(logger)

	alert := &Alert{
		ID:       "test-alert-1",
		Type:     AlertTypeBudgetThreshold,
		Severity: SeverityWarning,
		TenantID: "test-tenant",
		Title:    "Test Alert",
		Message:  "This is a test alert",
	}

	err := handler(context.Background(), alert)
	if err != nil {
		t.Errorf("LogAlertHandler failed: %v", err)
	}
}

func TestCapitalize(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"daily", "Daily"},
		{"monthly", "Monthly"},
		{"", ""},
		{"ALREADY", "ALREADY"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := capitalize(tt.input)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

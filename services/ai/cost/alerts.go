package cost

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// AlertType identifies the type of cost alert.
type AlertType string

const (
	// AlertTypeBudgetThreshold is triggered when a budget threshold is crossed.
	AlertTypeBudgetThreshold AlertType = "budget_threshold"

	// AlertTypeDailyBudgetExceeded is triggered when daily budget is exceeded.
	AlertTypeDailyBudgetExceeded AlertType = "daily_budget_exceeded"

	// AlertTypeMonthlyBudgetExceeded is triggered when monthly budget is exceeded.
	AlertTypeMonthlyBudgetExceeded AlertType = "monthly_budget_exceeded"

	// AlertTypeAnomaly is triggered when unusual spending is detected.
	AlertTypeAnomaly AlertType = "anomaly"

	// AlertTypeDailyDigest is a daily summary of spending.
	AlertTypeDailyDigest AlertType = "daily_digest"
)

// AlertSeverity indicates the urgency of an alert.
type AlertSeverity string

const (
	SeverityInfo     AlertSeverity = "info"
	SeverityWarning  AlertSeverity = "warning"
	SeverityCritical AlertSeverity = "critical"
)

// Alert represents a cost-related alert.
type Alert struct {
	// ID is the unique identifier for this alert.
	ID string `json:"id"`

	// Type is the type of alert.
	Type AlertType `json:"type"`

	// Severity is the severity level.
	Severity AlertSeverity `json:"severity"`

	// TenantID is the tenant this alert is for.
	TenantID string `json:"tenant_id"`

	// Title is a short summary of the alert.
	Title string `json:"title"`

	// Message is the detailed alert message.
	Message string `json:"message"`

	// Threshold is the threshold that was crossed (for threshold alerts).
	Threshold float64 `json:"threshold,omitempty"`

	// CurrentValue is the current spending value.
	CurrentValue float64 `json:"current_value,omitempty"`

	// Limit is the budget limit.
	Limit float64 `json:"limit,omitempty"`

	// Metadata contains additional alert data.
	Metadata map[string]interface{} `json:"metadata,omitempty"`

	// CreatedAt is when the alert was created.
	CreatedAt time.Time `json:"created_at"`

	// AcknowledgedAt is when the alert was acknowledged (nil if not).
	AcknowledgedAt *time.Time `json:"acknowledged_at,omitempty"`
}

// AlertHandler is a function that processes alerts.
type AlertHandler func(ctx context.Context, alert *Alert) error

// AlertManager manages cost alerts and notifications.
type AlertManager struct {
	mu sync.RWMutex

	// handlers stores registered alert handlers.
	handlers []AlertHandler

	// logger for alert operations.
	logger logging.Logger

	// alertHistory stores recent alerts to prevent duplicates.
	alertHistory map[string]time.Time

	// historyTTL is how long to keep alert history.
	historyTTL time.Duration

	// anomalyDetector detects unusual spending patterns.
	anomalyDetector *AnomalyDetector
}

// AlertManagerConfig configures the alert manager.
type AlertManagerConfig struct {
	// HistoryTTL is how long to keep alert history for deduplication.
	HistoryTTL time.Duration

	// AnomalyThreshold is the multiplier for anomaly detection.
	// Spending above (average * threshold) triggers an anomaly alert.
	AnomalyThreshold float64
}

// DefaultAlertManagerConfig returns default configuration.
func DefaultAlertManagerConfig() *AlertManagerConfig {
	return &AlertManagerConfig{
		HistoryTTL:       24 * time.Hour,
		AnomalyThreshold: 2.0, // 2x average spending triggers anomaly
	}
}

// NewAlertManager creates a new alert manager.
func NewAlertManager(cfg *AlertManagerConfig, logger logging.Logger) *AlertManager {
	if cfg == nil {
		cfg = DefaultAlertManagerConfig()
	}

	return &AlertManager{
		handlers:        make([]AlertHandler, 0),
		logger:          logger.With(logging.F("component", "alert_manager")),
		alertHistory:    make(map[string]time.Time),
		historyTTL:      cfg.HistoryTTL,
		anomalyDetector: NewAnomalyDetector(cfg.AnomalyThreshold),
	}
}

// RegisterHandler adds an alert handler.
func (m *AlertManager) RegisterHandler(handler AlertHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers = append(m.handlers, handler)
}

// CheckBudgetThresholds checks if any budget thresholds have been crossed.
func (m *AlertManager) CheckBudgetThresholds(ctx context.Context, budget *Budget) ([]*Alert, error) {
	var alerts []*Alert

	// Check daily thresholds
	if budget.DailyLimit > 0 {
		dailyPercent := budget.DailyUsagePercent()
		triggered := budget.CheckedAlertThresholds(dailyPercent)

		for _, threshold := range triggered {
			alert := m.createThresholdAlert(budget, threshold, dailyPercent, "daily")
			if !m.isDuplicate(alert) {
				alerts = append(alerts, alert)
				budget.MarkAlertSent(threshold)
			}
		}

		// Check for budget exceeded
		if budget.IsOverDailyBudget() {
			alert := m.createBudgetExceededAlert(budget, "daily")
			if !m.isDuplicate(alert) {
				alerts = append(alerts, alert)
			}
		}
	}

	// Check monthly thresholds
	if budget.MonthlyLimit > 0 {
		monthlyPercent := budget.MonthlyUsagePercent()
		triggered := budget.CheckedAlertThresholds(monthlyPercent)

		for _, threshold := range triggered {
			alert := m.createThresholdAlert(budget, threshold, monthlyPercent, "monthly")
			if !m.isDuplicate(alert) {
				alerts = append(alerts, alert)
				budget.MarkAlertSent(threshold)
			}
		}

		// Check for budget exceeded
		if budget.IsOverMonthlyBudget() {
			alert := m.createBudgetExceededAlert(budget, "monthly")
			if !m.isDuplicate(alert) {
				alerts = append(alerts, alert)
			}
		}
	}

	// Send alerts through handlers
	for _, alert := range alerts {
		if err := m.sendAlert(ctx, alert); err != nil {
			m.logger.Error("Failed to send alert",
				logging.F("alert_type", string(alert.Type)),
				logging.F("tenant_id", alert.TenantID),
				logging.Err(err),
			)
		}
	}

	return alerts, nil
}

// createThresholdAlert creates a budget threshold alert.
func (m *AlertManager) createThresholdAlert(budget *Budget, threshold, currentPercent float64, period string) *Alert {
	thresholdPercent := threshold * 100
	severity := m.getSeverityForThreshold(threshold)

	var limit, spent float64
	if period == "daily" {
		limit = budget.DailyLimit
		spent = budget.DailySpent
	} else {
		limit = budget.MonthlyLimit
		spent = budget.MonthlySpent
	}

	return &Alert{
		ID:           fmt.Sprintf("%s-%s-%.0f-%d", budget.TenantID, period, thresholdPercent, time.Now().Unix()),
		Type:         AlertTypeBudgetThreshold,
		Severity:     severity,
		TenantID:     budget.TenantID,
		Title:        fmt.Sprintf("%s Budget %.0f%% Threshold Reached", capitalize(period), thresholdPercent),
		Message:      fmt.Sprintf("Your %s AI spending has reached %.1f%% of your $%.2f budget. Current spend: $%.4f", period, currentPercent, limit, spent),
		Threshold:    threshold,
		CurrentValue: spent,
		Limit:        limit,
		Metadata: map[string]interface{}{
			"period":          period,
			"usage_percent":   currentPercent,
			"remaining":       limit - spent,
		},
		CreatedAt: time.Now(),
	}
}

// createBudgetExceededAlert creates a budget exceeded alert.
func (m *AlertManager) createBudgetExceededAlert(budget *Budget, period string) *Alert {
	var alertType AlertType
	var limit, spent float64

	if period == "daily" {
		alertType = AlertTypeDailyBudgetExceeded
		limit = budget.DailyLimit
		spent = budget.DailySpent
	} else {
		alertType = AlertTypeMonthlyBudgetExceeded
		limit = budget.MonthlyLimit
		spent = budget.MonthlySpent
	}

	return &Alert{
		ID:           fmt.Sprintf("%s-%s-exceeded-%d", budget.TenantID, period, time.Now().Unix()),
		Type:         alertType,
		Severity:     SeverityCritical,
		TenantID:     budget.TenantID,
		Title:        fmt.Sprintf("%s Budget Exceeded", capitalize(period)),
		Message:      fmt.Sprintf("Your %s AI spending of $%.4f has exceeded your $%.2f budget!", period, spent, limit),
		CurrentValue: spent,
		Limit:        limit,
		Metadata: map[string]interface{}{
			"period":    period,
			"overage":   spent - limit,
		},
		CreatedAt: time.Now(),
	}
}

// CheckAnomaly checks for unusual spending patterns.
func (m *AlertManager) CheckAnomaly(ctx context.Context, tenantID string, currentSpending float64, historicalAverage float64) (*Alert, error) {
	if !m.anomalyDetector.IsAnomaly(currentSpending, historicalAverage) {
		return nil, nil
	}

	alert := &Alert{
		ID:           fmt.Sprintf("%s-anomaly-%d", tenantID, time.Now().Unix()),
		Type:         AlertTypeAnomaly,
		Severity:     SeverityWarning,
		TenantID:     tenantID,
		Title:        "Unusual AI Spending Detected",
		Message:      fmt.Sprintf("Current spending ($%.4f) is significantly higher than your average ($%.4f)", currentSpending, historicalAverage),
		CurrentValue: currentSpending,
		Metadata: map[string]interface{}{
			"historical_average": historicalAverage,
			"multiplier":         currentSpending / historicalAverage,
		},
		CreatedAt: time.Now(),
	}

	if m.isDuplicate(alert) {
		return nil, nil
	}

	if err := m.sendAlert(ctx, alert); err != nil {
		return nil, err
	}

	return alert, nil
}

// CreateDailyDigest creates a daily spending summary alert.
func (m *AlertManager) CreateDailyDigest(ctx context.Context, tenantID string, report *CostReport) (*Alert, error) {
	alert := &Alert{
		ID:       fmt.Sprintf("%s-digest-%s", tenantID, report.StartTime.Format("2006-01-02")),
		Type:     AlertTypeDailyDigest,
		Severity: SeverityInfo,
		TenantID: tenantID,
		Title:    "Daily AI Spending Summary",
		Message: fmt.Sprintf(
			"Today's AI spending: $%.4f across %d requests. Input tokens: %d, Output tokens: %d",
			report.TotalCost, report.TotalRequests,
			report.TotalInputTokens, report.TotalOutputTokens,
		),
		CurrentValue: report.TotalCost,
		Metadata: map[string]interface{}{
			"date":              report.StartTime.Format("2006-01-02"),
			"total_requests":    report.TotalRequests,
			"total_input_tokens": report.TotalInputTokens,
			"total_output_tokens": report.TotalOutputTokens,
			"by_model":          report.ByModel,
		},
		CreatedAt: time.Now(),
	}

	if err := m.sendAlert(ctx, alert); err != nil {
		return nil, err
	}

	return alert, nil
}

// sendAlert sends an alert through all registered handlers.
func (m *AlertManager) sendAlert(ctx context.Context, alert *Alert) error {
	m.mu.RLock()
	handlers := make([]AlertHandler, len(m.handlers))
	copy(handlers, m.handlers)
	m.mu.RUnlock()

	m.markSent(alert)

	m.logger.Info("Sending alert",
		logging.F("alert_id", alert.ID),
		logging.F("type", string(alert.Type)),
		logging.F("severity", string(alert.Severity)),
		logging.F("tenant_id", alert.TenantID),
	)

	var lastErr error
	for _, handler := range handlers {
		if err := handler(ctx, alert); err != nil {
			lastErr = err
			m.logger.Error("Alert handler failed",
				logging.F("alert_id", alert.ID),
				logging.Err(err),
			)
		}
	}

	return lastErr
}

// isDuplicate checks if an alert was recently sent.
func (m *AlertManager) isDuplicate(alert *Alert) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := m.alertKey(alert)
	if lastSent, ok := m.alertHistory[key]; ok {
		return time.Since(lastSent) < m.historyTTL
	}
	return false
}

// markSent marks an alert as sent.
func (m *AlertManager) markSent(alert *Alert) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := m.alertKey(alert)
	m.alertHistory[key] = time.Now()

	// Cleanup old entries
	for k, v := range m.alertHistory {
		if time.Since(v) > m.historyTTL {
			delete(m.alertHistory, k)
		}
	}
}

// alertKey generates a deduplication key for an alert.
func (m *AlertManager) alertKey(alert *Alert) string {
	return fmt.Sprintf("%s-%s-%s", alert.TenantID, alert.Type, alert.Title)
}

// getSeverityForThreshold returns the severity for a given threshold.
func (m *AlertManager) getSeverityForThreshold(threshold float64) AlertSeverity {
	switch {
	case threshold >= 1.0:
		return SeverityCritical
	case threshold >= 0.8:
		return SeverityWarning
	default:
		return SeverityInfo
	}
}

// AnomalyDetector detects unusual spending patterns.
type AnomalyDetector struct {
	threshold float64
}

// NewAnomalyDetector creates a new anomaly detector.
func NewAnomalyDetector(threshold float64) *AnomalyDetector {
	if threshold <= 0 {
		threshold = 2.0
	}
	return &AnomalyDetector{threshold: threshold}
}

// IsAnomaly returns true if current spending is anomalous.
func (d *AnomalyDetector) IsAnomaly(current, average float64) bool {
	if average <= 0 {
		return false // Can't detect anomaly without baseline
	}
	return current > (average * d.threshold)
}

// capitalize capitalizes the first letter of a string.
func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	// Only capitalize if first char is lowercase a-z
	if s[0] >= 'a' && s[0] <= 'z' {
		return string(s[0]-32) + s[1:]
	}
	return s
}

// LogAlertHandler is a simple handler that logs alerts.
func LogAlertHandler(logger logging.Logger) AlertHandler {
	return func(ctx context.Context, alert *Alert) error {
		logFn := logger.Info
		switch alert.Severity {
		case SeverityWarning:
			logFn = logger.Warn
		case SeverityCritical:
			logFn = logger.Error
		}

		logFn("Cost Alert",
			logging.F("alert_id", alert.ID),
			logging.F("type", string(alert.Type)),
			logging.F("severity", string(alert.Severity)),
			logging.F("tenant_id", alert.TenantID),
			logging.F("title", alert.Title),
			logging.F("message", alert.Message),
		)

		return nil
	}
}

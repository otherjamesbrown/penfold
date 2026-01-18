package cost

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/ai/router"
	"github.com/prometheus/client_golang/prometheus"
)

// Tracker errors.
var (
	ErrBudgetExceeded     = errors.New("budget exceeded")
	ErrDailyBudgetExceeded = errors.New("daily budget exceeded")
	ErrMonthlyBudgetExceeded = errors.New("monthly budget exceeded")
	ErrNoStorage          = errors.New("no storage configured")
)

// TrackerConfig configures the cost tracker.
type TrackerConfig struct {
	// EnablePersistence enables database storage of cost records.
	EnablePersistence bool

	// EnableMetrics enables Prometheus metrics.
	EnableMetrics bool

	// EnableAlerts enables budget alerts.
	EnableAlerts bool

	// EnforceBudgets blocks requests that would exceed budgets.
	EnforceBudgets bool

	// AlertConfig configures the alert manager.
	AlertConfig *AlertManagerConfig
}

// DefaultTrackerConfig returns default configuration.
func DefaultTrackerConfig() *TrackerConfig {
	return &TrackerConfig{
		EnablePersistence: true,
		EnableMetrics:     true,
		EnableAlerts:      true,
		EnforceBudgets:    false, // Default to non-blocking
		AlertConfig:       DefaultAlertManagerConfig(),
	}
}

// CostTracker tracks AI operation costs and manages budgets.
type CostTracker struct {
	mu sync.RWMutex

	config  *TrackerConfig
	logger  logging.Logger
	pricing *PricingTable
	storage Storage
	alerts  *AlertManager

	// In-memory budget cache for fast lookups.
	budgetCache map[string]*Budget
	budgetTTL   time.Duration

	// Metrics
	metrics *trackerMetrics
}

// trackerMetrics holds Prometheus metrics for the cost tracker.
type trackerMetrics struct {
	requestsTotal     *prometheus.CounterVec
	tokensTotal       *prometheus.CounterVec
	costTotal         *prometheus.CounterVec
	budgetUsage       *prometheus.GaugeVec
	budgetRemaining   *prometheus.GaugeVec
	alertsTriggered   *prometheus.CounterVec
}

// NewCostTracker creates a new cost tracker.
func NewCostTracker(cfg *TrackerConfig, logger logging.Logger) *CostTracker {
	if cfg == nil {
		cfg = DefaultTrackerConfig()
	}

	tracker := &CostTracker{
		config:      cfg,
		logger:      logger.With(logging.F("component", "cost_tracker")),
		pricing:     NewPricingTable(),
		budgetCache: make(map[string]*Budget),
		budgetTTL:   5 * time.Minute,
	}

	if cfg.EnableAlerts {
		tracker.alerts = NewAlertManager(cfg.AlertConfig, logger)
		// Register default log handler
		tracker.alerts.RegisterHandler(LogAlertHandler(logger))
	}

	if cfg.EnableMetrics {
		tracker.initMetrics()
	}

	return tracker
}

// SetStorage sets the storage backend.
func (t *CostTracker) SetStorage(storage Storage) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.storage = storage
}

// GetPricingTable returns the pricing table for configuration.
func (t *CostTracker) GetPricingTable() *PricingTable {
	return t.pricing
}

// GetAlertManager returns the alert manager for configuration.
func (t *CostTracker) GetAlertManager() *AlertManager {
	return t.alerts
}

// initMetrics initializes Prometheus metrics.
func (t *CostTracker) initMetrics() {
	t.metrics = &trackerMetrics{
		requestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "penfold",
				Subsystem: "ai_cost",
				Name:      "requests_total",
				Help:      "Total number of AI requests tracked",
			},
			[]string{"tenant_id", "model", "provider", "request_type"},
		),
		tokensTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "penfold",
				Subsystem: "ai_cost",
				Name:      "tokens_total",
				Help:      "Total tokens used",
			},
			[]string{"tenant_id", "model", "token_type"},
		),
		costTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "penfold",
				Subsystem: "ai_cost",
				Name:      "usd_total",
				Help:      "Total cost in USD",
			},
			[]string{"tenant_id", "model", "provider"},
		),
		budgetUsage: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "penfold",
				Subsystem: "ai_cost",
				Name:      "budget_usage_percent",
				Help:      "Current budget usage percentage",
			},
			[]string{"tenant_id", "period"},
		),
		budgetRemaining: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "penfold",
				Subsystem: "ai_cost",
				Name:      "budget_remaining_usd",
				Help:      "Remaining budget in USD",
			},
			[]string{"tenant_id", "period"},
		),
		alertsTriggered: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "penfold",
				Subsystem: "ai_cost",
				Name:      "alerts_triggered_total",
				Help:      "Total alerts triggered",
			},
			[]string{"tenant_id", "alert_type", "severity"},
		),
	}

	// Register metrics
	collectors := []prometheus.Collector{
		t.metrics.requestsTotal,
		t.metrics.tokensTotal,
		t.metrics.costTotal,
		t.metrics.budgetUsage,
		t.metrics.budgetRemaining,
		t.metrics.alertsTriggered,
	}

	for _, c := range collectors {
		if err := prometheus.Register(c); err != nil {
			if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
				t.logger.Warn("Failed to register metric", logging.Err(err))
			}
		}
	}
}

// TrackRequest records the cost of an AI request.
func (t *CostTracker) TrackRequest(ctx context.Context, req *router.Request, resp *router.Response, operation string) error {
	if resp == nil {
		return nil
	}

	tenantID := req.TenantID
	if tenantID == "" {
		tenantID = "default"
	}

	// Get pricing for the model
	pricing := t.pricing.GetPricing(resp.ModelUsed, tenantID)

	// Calculate cost (resp.TokensUsed is typically total; we estimate split)
	// In practice, the response should provide separate input/output counts
	inputTokens := len(req.Content) / 4   // Rough estimate: ~4 chars per token
	outputTokens := resp.TokensUsed - inputTokens
	if outputTokens < 0 {
		outputTokens = resp.TokensUsed / 2
		inputTokens = resp.TokensUsed / 2
	}

	cost := NewCost(inputTokens, outputTokens, resp.ModelUsed, resp.ProviderUsed, pricing)

	// Create cost record
	record := &CostRecord{
		ID:               uuid.New().String(),
		TenantID:         tenantID,
		RequestID:        req.ID,
		Model:            resp.ModelUsed,
		Provider:         resp.ProviderUsed,
		RequestType:      string(req.Type),
		Operation:        operation,
		InputTokens:      cost.InputTokens,
		OutputTokens:     cost.OutputTokens,
		TotalTokens:      cost.TotalTokens,
		InputCost:        cost.InputCost,
		OutputCost:       cost.OutputCost,
		TotalCost:        cost.TotalCost,
		ProcessingTimeMs: resp.ProcessingTime.Milliseconds(),
		CreatedAt:        time.Now(),
	}

	// Update metrics
	t.recordMetrics(record)

	// Persist to storage
	if t.config.EnablePersistence && t.storage != nil {
		if err := t.storage.SaveRecord(ctx, record); err != nil {
			t.logger.Error("Failed to save cost record",
				logging.F("request_id", req.ID),
				logging.Err(err),
			)
			// Don't fail the request due to cost tracking failure
		}
	}

	// Update budget and check alerts
	if t.config.EnableAlerts && cost.TotalCost > 0 {
		if err := t.updateBudgetAndAlert(ctx, tenantID, cost.TotalCost); err != nil {
			t.logger.Warn("Failed to update budget",
				logging.F("tenant_id", tenantID),
				logging.Err(err),
			)
		}
	}

	t.logger.Debug("Tracked request cost",
		logging.F("request_id", req.ID),
		logging.F("tenant_id", tenantID),
		logging.F("model", resp.ModelUsed),
		logging.F("total_tokens", cost.TotalTokens),
		logging.F("total_cost", cost.TotalCost),
	)

	return nil
}

// TrackRequestWithTokens records the cost with explicit token counts.
func (t *CostTracker) TrackRequestWithTokens(ctx context.Context, tenantID, requestID, model, provider, requestType, operation string, inputTokens, outputTokens int, processingTime time.Duration) error {
	if tenantID == "" {
		tenantID = "default"
	}

	// Get pricing for the model
	pricing := t.pricing.GetPricing(model, tenantID)
	cost := NewCost(inputTokens, outputTokens, model, provider, pricing)

	// Create cost record
	record := &CostRecord{
		ID:               uuid.New().String(),
		TenantID:         tenantID,
		RequestID:        requestID,
		Model:            model,
		Provider:         provider,
		RequestType:      requestType,
		Operation:        operation,
		InputTokens:      inputTokens,
		OutputTokens:     outputTokens,
		TotalTokens:      inputTokens + outputTokens,
		InputCost:        cost.InputCost,
		OutputCost:       cost.OutputCost,
		TotalCost:        cost.TotalCost,
		ProcessingTimeMs: processingTime.Milliseconds(),
		CreatedAt:        time.Now(),
	}

	// Update metrics
	t.recordMetrics(record)

	// Persist to storage
	if t.config.EnablePersistence && t.storage != nil {
		if err := t.storage.SaveRecord(ctx, record); err != nil {
			return err
		}
	}

	// Update budget and check alerts
	if t.config.EnableAlerts && cost.TotalCost > 0 {
		if err := t.updateBudgetAndAlert(ctx, tenantID, cost.TotalCost); err != nil {
			t.logger.Warn("Failed to update budget",
				logging.F("tenant_id", tenantID),
				logging.Err(err),
			)
		}
	}

	return nil
}

// recordMetrics updates Prometheus metrics.
func (t *CostTracker) recordMetrics(record *CostRecord) {
	if t.metrics == nil {
		return
	}

	t.metrics.requestsTotal.WithLabelValues(
		record.TenantID, record.Model, record.Provider, record.RequestType,
	).Inc()

	t.metrics.tokensTotal.WithLabelValues(
		record.TenantID, record.Model, "input",
	).Add(float64(record.InputTokens))

	t.metrics.tokensTotal.WithLabelValues(
		record.TenantID, record.Model, "output",
	).Add(float64(record.OutputTokens))

	t.metrics.costTotal.WithLabelValues(
		record.TenantID, record.Model, record.Provider,
	).Add(record.TotalCost)
}

// updateBudgetAndAlert updates budget spending and checks for alerts.
func (t *CostTracker) updateBudgetAndAlert(ctx context.Context, tenantID string, amount float64) error {
	if t.storage == nil {
		return nil
	}

	budget, err := t.storage.UpdateBudgetSpending(ctx, tenantID, amount)
	if errors.Is(err, ErrBudgetNotFound) {
		// No budget configured for this tenant
		return nil
	}
	if err != nil {
		return err
	}

	// Update budget cache
	t.mu.Lock()
	t.budgetCache[tenantID] = budget
	t.mu.Unlock()

	// Update metrics
	if t.metrics != nil {
		if budget.DailyLimit > 0 {
			t.metrics.budgetUsage.WithLabelValues(tenantID, "daily").Set(budget.DailyUsagePercent())
			t.metrics.budgetRemaining.WithLabelValues(tenantID, "daily").Set(budget.DailyRemaining)
		}
		if budget.MonthlyLimit > 0 {
			t.metrics.budgetUsage.WithLabelValues(tenantID, "monthly").Set(budget.MonthlyUsagePercent())
			t.metrics.budgetRemaining.WithLabelValues(tenantID, "monthly").Set(budget.MonthlyRemaining)
		}
	}

	// Check for alerts
	if t.alerts != nil {
		alerts, err := t.alerts.CheckBudgetThresholds(ctx, budget)
		if err != nil {
			t.logger.Error("Failed to check budget thresholds",
				logging.F("tenant_id", tenantID),
				logging.Err(err),
			)
		}

		// Record alert metrics
		if t.metrics != nil {
			for _, alert := range alerts {
				t.metrics.alertsTriggered.WithLabelValues(
					tenantID, string(alert.Type), string(alert.Severity),
				).Inc()
			}
		}
	}

	return nil
}

// GetCosts retrieves cost records matching the filter.
func (t *CostTracker) GetCosts(ctx context.Context, filter *CostFilter) ([]*CostRecord, error) {
	if t.storage == nil {
		return nil, ErrNoStorage
	}
	return t.storage.QueryRecords(ctx, filter)
}

// GetCostReport generates a cost report for the specified filter.
func (t *CostTracker) GetCostReport(ctx context.Context, filter *CostFilter, granularity TimeRange) (*CostReport, error) {
	if t.storage == nil {
		return nil, ErrNoStorage
	}
	return t.storage.GetCostReport(ctx, filter, granularity)
}

// GetBudget retrieves the budget for a tenant.
func (t *CostTracker) GetBudget(ctx context.Context, tenantID string) (*Budget, error) {
	// Check cache first
	t.mu.RLock()
	if budget, ok := t.budgetCache[tenantID]; ok {
		t.mu.RUnlock()
		return budget, nil
	}
	t.mu.RUnlock()

	if t.storage == nil {
		return nil, ErrNoStorage
	}

	budget, err := t.storage.GetBudget(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	// Update cache
	t.mu.Lock()
	t.budgetCache[tenantID] = budget
	t.mu.Unlock()

	return budget, nil
}

// SetBudget creates or updates a budget for a tenant.
func (t *CostTracker) SetBudget(ctx context.Context, budget *Budget) error {
	if t.storage == nil {
		return ErrNoStorage
	}

	if err := t.storage.SaveBudget(ctx, budget); err != nil {
		return err
	}

	// Update cache
	t.mu.Lock()
	t.budgetCache[budget.TenantID] = budget
	t.mu.Unlock()

	return nil
}

// CheckBudget checks if an estimated cost would exceed the budget.
// Returns true if the request should proceed, false if it would exceed budget.
func (t *CostTracker) CheckBudget(ctx context.Context, tenantID string, estimated *EstimatedCost) (bool, error) {
	budget, err := t.GetBudget(ctx, tenantID)
	if errors.Is(err, ErrBudgetNotFound) {
		// No budget = unlimited
		return true, nil
	}
	if err != nil {
		return true, err // Allow on error
	}

	// Check daily budget
	if budget.DailyLimit > 0 {
		if budget.DailySpent+estimated.EstimatedCost > budget.DailyLimit {
			if t.config.EnforceBudgets {
				return false, ErrDailyBudgetExceeded
			}
			t.logger.Warn("Request would exceed daily budget",
				logging.F("tenant_id", tenantID),
				logging.F("daily_spent", budget.DailySpent),
				logging.F("estimated_cost", estimated.EstimatedCost),
				logging.F("daily_limit", budget.DailyLimit),
			)
		}
	}

	// Check monthly budget
	if budget.MonthlyLimit > 0 {
		if budget.MonthlySpent+estimated.EstimatedCost > budget.MonthlyLimit {
			if t.config.EnforceBudgets {
				return false, ErrMonthlyBudgetExceeded
			}
			t.logger.Warn("Request would exceed monthly budget",
				logging.F("tenant_id", tenantID),
				logging.F("monthly_spent", budget.MonthlySpent),
				logging.F("estimated_cost", estimated.EstimatedCost),
				logging.F("monthly_limit", budget.MonthlyLimit),
			)
		}
	}

	return true, nil
}

// EstimateCost estimates the cost for a potential request.
func (t *CostTracker) EstimateCost(tenantID, model string, estimatedInputTokens, estimatedOutputTokens int) *EstimatedCost {
	return t.pricing.EstimateCost(model, tenantID, estimatedInputTokens, estimatedOutputTokens)
}

// GetDailySpending returns the current day's spending for a tenant.
func (t *CostTracker) GetDailySpending(ctx context.Context, tenantID string) (float64, error) {
	if t.storage == nil {
		return 0, ErrNoStorage
	}
	return t.storage.GetDailySpending(ctx, tenantID)
}

// GetMonthlySpending returns the current month's spending for a tenant.
func (t *CostTracker) GetMonthlySpending(ctx context.Context, tenantID string) (float64, error) {
	if t.storage == nil {
		return 0, ErrNoStorage
	}
	return t.storage.GetMonthlySpending(ctx, tenantID)
}

// InvalidateBudgetCache clears the budget cache for a tenant.
func (t *CostTracker) InvalidateBudgetCache(tenantID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.budgetCache, tenantID)
}

// Close closes the cost tracker and its resources.
func (t *CostTracker) Close() error {
	if t.storage != nil {
		return t.storage.Close()
	}
	return nil
}

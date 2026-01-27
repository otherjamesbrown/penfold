// Package cost provides cost tracking and budget management for AI operations.
// It tracks token usage, calculates costs per model, and enforces budget limits.
package cost

import (
	"time"
)

// Cost represents the cost of a single AI operation.
type Cost struct {
	// InputTokens is the number of input/prompt tokens used.
	InputTokens int `json:"input_tokens"`

	// OutputTokens is the number of output/completion tokens used.
	OutputTokens int `json:"output_tokens"`

	// TotalTokens is the sum of input and output tokens.
	TotalTokens int `json:"total_tokens"`

	// InputCost is the cost for input tokens in USD.
	InputCost float64 `json:"input_cost"`

	// OutputCost is the cost for output tokens in USD.
	OutputCost float64 `json:"output_cost"`

	// TotalCost is the total cost in USD.
	TotalCost float64 `json:"total_cost"`

	// Model is the model that incurred this cost.
	Model string `json:"model"`

	// Provider is the provider (mlx, gemini, openai, anthropic).
	Provider string `json:"provider"`

	// Currency is always "USD".
	Currency string `json:"currency"`
}

// NewCost creates a Cost from token counts and pricing.
func NewCost(inputTokens, outputTokens int, model, provider string, pricing *ModelPricing) *Cost {
	inputCost := float64(inputTokens) * pricing.InputCostPer1K / 1000.0
	outputCost := float64(outputTokens) * pricing.OutputCostPer1K / 1000.0

	return &Cost{
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		TotalTokens:  inputTokens + outputTokens,
		InputCost:    inputCost,
		OutputCost:   outputCost,
		TotalCost:    inputCost + outputCost,
		Model:        model,
		Provider:     provider,
		Currency:     "USD",
	}
}

// CostRecord represents a persisted cost record in the database.
type CostRecord struct {
	// ID is the unique identifier for this record.
	ID string `json:"id"`

	// TenantID is the tenant that incurred this cost.
	TenantID string `json:"tenant_id"`

	// RequestID links to the original AI request.
	RequestID string `json:"request_id"`

	// Model is the model used.
	Model string `json:"model"`

	// Provider is the provider used.
	Provider string `json:"provider"`

	// RequestType is the type of request (embedding, summary, etc.).
	RequestType string `json:"request_type"`

	// Operation is the specific operation or workflow (e.g., "daily-review").
	Operation string `json:"operation,omitempty"`

	// InputTokens is the number of input tokens.
	InputTokens int `json:"input_tokens"`

	// OutputTokens is the number of output tokens.
	OutputTokens int `json:"output_tokens"`

	// TotalTokens is the sum of input and output tokens.
	TotalTokens int `json:"total_tokens"`

	// InputCost is the cost for input tokens.
	InputCost float64 `json:"input_cost"`

	// OutputCost is the cost for output tokens.
	OutputCost float64 `json:"output_cost"`

	// TotalCost is the total cost.
	TotalCost float64 `json:"total_cost"`

	// ProcessingTimeMs is how long the request took in milliseconds.
	ProcessingTimeMs int64 `json:"processing_time_ms"`

	// CreatedAt is when this record was created.
	CreatedAt time.Time `json:"created_at"`
}

// CostFilter specifies criteria for querying cost records.
type CostFilter struct {
	// TenantID filters by tenant (required for security).
	TenantID string

	// StartTime is the beginning of the time range (inclusive).
	StartTime time.Time

	// EndTime is the end of the time range (exclusive).
	EndTime time.Time

	// Models filters by specific models (empty means all).
	Models []string

	// Providers filters by specific providers (empty means all).
	Providers []string

	// RequestTypes filters by request types (empty means all).
	RequestTypes []string

	// Operations filters by operations (empty means all).
	Operations []string

	// Limit is the maximum number of records to return.
	Limit int

	// Offset is the number of records to skip.
	Offset int
}

// TimeRange represents a time period for cost aggregation.
type TimeRange string

const (
	TimeRangeHour  TimeRange = "hour"
	TimeRangeDay   TimeRange = "day"
	TimeRangeWeek  TimeRange = "week"
	TimeRangeMonth TimeRange = "month"
)

// ModelCostSummary provides cost breakdown for a single model.
type ModelCostSummary struct {
	// Model is the model name.
	Model string `json:"model"`

	// Provider is the provider name.
	Provider string `json:"provider"`

	// RequestCount is the number of requests.
	RequestCount int64 `json:"request_count"`

	// TotalInputTokens is the total input tokens used.
	TotalInputTokens int64 `json:"total_input_tokens"`

	// TotalOutputTokens is the total output tokens used.
	TotalOutputTokens int64 `json:"total_output_tokens"`

	// TotalCost is the total cost for this model.
	TotalCost float64 `json:"total_cost"`

	// AvgCostPerRequest is the average cost per request.
	AvgCostPerRequest float64 `json:"avg_cost_per_request"`
}

// OperationCostSummary provides cost breakdown for a workflow/operation.
type OperationCostSummary struct {
	// Operation is the operation/workflow name.
	Operation string `json:"operation"`

	// RequestCount is the number of requests.
	RequestCount int64 `json:"request_count"`

	// TotalCost is the total cost for this operation.
	TotalCost float64 `json:"total_cost"`

	// AvgCostPerRequest is the average cost per request.
	AvgCostPerRequest float64 `json:"avg_cost_per_request"`
}

// TimePeriodCost provides cost for a specific time period.
type TimePeriodCost struct {
	// Period is the start of the time period.
	Period time.Time `json:"period"`

	// TotalCost is the total cost for this period.
	TotalCost float64 `json:"total_cost"`

	// RequestCount is the number of requests in this period.
	RequestCount int64 `json:"request_count"`
}

// CostReport aggregates costs across multiple dimensions.
type CostReport struct {
	// TenantID is the tenant this report is for.
	TenantID string `json:"tenant_id"`

	// StartTime is the beginning of the report period.
	StartTime time.Time `json:"start_time"`

	// EndTime is the end of the report period.
	EndTime time.Time `json:"end_time"`

	// TotalCost is the total cost for the period.
	TotalCost float64 `json:"total_cost"`

	// TotalRequests is the total number of requests.
	TotalRequests int64 `json:"total_requests"`

	// TotalInputTokens is the total input tokens used.
	TotalInputTokens int64 `json:"total_input_tokens"`

	// TotalOutputTokens is the total output tokens used.
	TotalOutputTokens int64 `json:"total_output_tokens"`

	// ByModel is the cost breakdown by model.
	ByModel []ModelCostSummary `json:"by_model"`

	// ByOperation is the cost breakdown by operation.
	ByOperation []OperationCostSummary `json:"by_operation"`

	// ByTimeRange is the cost breakdown by time period.
	ByTimeRange []TimePeriodCost `json:"by_time_range,omitempty"`

	// TimeRangeGranularity is the granularity of ByTimeRange.
	TimeRangeGranularity TimeRange `json:"time_range_granularity,omitempty"`

	// GeneratedAt is when this report was generated.
	GeneratedAt time.Time `json:"generated_at"`
}

// Budget defines spending limits for a tenant.
type Budget struct {
	// TenantID is the tenant this budget is for.
	TenantID string `json:"tenant_id"`

	// DailyLimit is the maximum daily spend in USD (0 means unlimited).
	DailyLimit float64 `json:"daily_limit"`

	// MonthlyLimit is the maximum monthly spend in USD (0 means unlimited).
	MonthlyLimit float64 `json:"monthly_limit"`

	// DailySpent is the amount spent today.
	DailySpent float64 `json:"daily_spent"`

	// MonthlySpent is the amount spent this month.
	MonthlySpent float64 `json:"monthly_spent"`

	// DailyRemaining is the remaining daily budget.
	DailyRemaining float64 `json:"daily_remaining"`

	// MonthlyRemaining is the remaining monthly budget.
	MonthlyRemaining float64 `json:"monthly_remaining"`

	// AlertThresholds are the percentages at which to trigger alerts.
	AlertThresholds []float64 `json:"alert_thresholds"`

	// AlertsSent tracks which alerts have been sent (threshold -> sent).
	AlertsSent map[float64]bool `json:"alerts_sent"`

	// LastUpdated is when this budget was last updated.
	LastUpdated time.Time `json:"last_updated"`

	// CreatedAt is when this budget was created.
	CreatedAt time.Time `json:"created_at"`
}

// DefaultAlertThresholds returns the default budget alert thresholds.
func DefaultAlertThresholds() []float64 {
	return []float64{0.50, 0.80, 1.00} // 50%, 80%, 100%
}

// NewBudget creates a new Budget with default settings.
func NewBudget(tenantID string, dailyLimit, monthlyLimit float64) *Budget {
	now := time.Now()
	return &Budget{
		TenantID:         tenantID,
		DailyLimit:       dailyLimit,
		MonthlyLimit:     monthlyLimit,
		DailySpent:       0,
		MonthlySpent:     0,
		DailyRemaining:   dailyLimit,
		MonthlyRemaining: monthlyLimit,
		AlertThresholds:  DefaultAlertThresholds(),
		AlertsSent:       make(map[float64]bool),
		LastUpdated:      now,
		CreatedAt:        now,
	}
}

// UpdateSpending updates the budget with new spending.
func (b *Budget) UpdateSpending(amount float64) {
	b.DailySpent += amount
	b.MonthlySpent += amount

	if b.DailyLimit > 0 {
		b.DailyRemaining = b.DailyLimit - b.DailySpent
		if b.DailyRemaining < 0 {
			b.DailyRemaining = 0
		}
	}

	if b.MonthlyLimit > 0 {
		b.MonthlyRemaining = b.MonthlyLimit - b.MonthlySpent
		if b.MonthlyRemaining < 0 {
			b.MonthlyRemaining = 0
		}
	}

	b.LastUpdated = time.Now()
}

// DailyUsagePercent returns the percentage of daily budget used.
func (b *Budget) DailyUsagePercent() float64 {
	if b.DailyLimit <= 0 {
		return 0
	}
	return (b.DailySpent / b.DailyLimit) * 100
}

// MonthlyUsagePercent returns the percentage of monthly budget used.
func (b *Budget) MonthlyUsagePercent() float64 {
	if b.MonthlyLimit <= 0 {
		return 0
	}
	return (b.MonthlySpent / b.MonthlyLimit) * 100
}

// IsOverDailyBudget returns true if daily spending exceeds the limit.
func (b *Budget) IsOverDailyBudget() bool {
	if b.DailyLimit <= 0 {
		return false
	}
	return b.DailySpent >= b.DailyLimit
}

// IsOverMonthlyBudget returns true if monthly spending exceeds the limit.
func (b *Budget) IsOverMonthlyBudget() bool {
	if b.MonthlyLimit <= 0 {
		return false
	}
	return b.MonthlySpent >= b.MonthlyLimit
}

// CheckedAlertThresholds returns thresholds that have been crossed but not alerted.
func (b *Budget) CheckedAlertThresholds(usagePercent float64) []float64 {
	var triggered []float64
	for _, threshold := range b.AlertThresholds {
		thresholdPercent := threshold * 100
		if usagePercent >= thresholdPercent && !b.AlertsSent[threshold] {
			triggered = append(triggered, threshold)
		}
	}
	return triggered
}

// MarkAlertSent marks a threshold as having been alerted.
func (b *Budget) MarkAlertSent(threshold float64) {
	if b.AlertsSent == nil {
		b.AlertsSent = make(map[float64]bool)
	}
	b.AlertsSent[threshold] = true
}

// ResetDaily resets the daily spending counters.
// Note: Alert state is preserved - only spending counters are reset.
// Monthly alerts persist until month reset.
func (b *Budget) ResetDaily() {
	b.DailySpent = 0
	b.DailyRemaining = b.DailyLimit
	b.LastUpdated = time.Now()
}

// ResetMonthly resets both daily and monthly spending counters.
func (b *Budget) ResetMonthly() {
	b.DailySpent = 0
	b.MonthlySpent = 0
	b.DailyRemaining = b.DailyLimit
	b.MonthlyRemaining = b.MonthlyLimit
	b.AlertsSent = make(map[float64]bool)
	b.LastUpdated = time.Now()
}

// EstimatedCost represents a pre-request cost estimate.
type EstimatedCost struct {
	// Model is the model to be used.
	Model string `json:"model"`

	// EstimatedInputTokens is the estimated input tokens.
	EstimatedInputTokens int `json:"estimated_input_tokens"`

	// EstimatedOutputTokens is the estimated output tokens.
	EstimatedOutputTokens int `json:"estimated_output_tokens"`

	// EstimatedCost is the estimated total cost.
	EstimatedCost float64 `json:"estimated_cost"`

	// Confidence is the confidence level (0-1) in this estimate.
	Confidence float64 `json:"confidence"`
}

package cost

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Storage errors.
var (
	ErrRecordNotFound = errors.New("cost record not found")
	ErrBudgetNotFound = errors.New("budget not found")
)

// Storage provides persistence for cost records and budgets.
type Storage interface {
	// SaveRecord persists a cost record.
	SaveRecord(ctx context.Context, record *CostRecord) error

	// GetRecord retrieves a cost record by ID.
	GetRecord(ctx context.Context, id string) (*CostRecord, error)

	// QueryRecords retrieves cost records matching the filter.
	QueryRecords(ctx context.Context, filter *CostFilter) ([]*CostRecord, error)

	// GetCostReport generates an aggregated cost report.
	GetCostReport(ctx context.Context, filter *CostFilter, granularity TimeRange) (*CostReport, error)

	// SaveBudget persists a budget.
	SaveBudget(ctx context.Context, budget *Budget) error

	// GetBudget retrieves a budget for a tenant.
	GetBudget(ctx context.Context, tenantID string) (*Budget, error)

	// UpdateBudgetSpending atomically updates budget spending.
	UpdateBudgetSpending(ctx context.Context, tenantID string, amount float64) (*Budget, error)

	// GetDailySpending returns total spending for a tenant for today.
	GetDailySpending(ctx context.Context, tenantID string) (float64, error)

	// GetMonthlySpending returns total spending for a tenant for this month.
	GetMonthlySpending(ctx context.Context, tenantID string) (float64, error)

	// Close closes the storage connection.
	Close() error
}

// PostgresStorage implements Storage using PostgreSQL.
type PostgresStorage struct {
	pool *pgxpool.Pool
}

// NewPostgresStorage creates a new PostgreSQL-backed storage.
func NewPostgresStorage(pool *pgxpool.Pool) *PostgresStorage {
	return &PostgresStorage{pool: pool}
}

// EnsureSchema creates the required database tables if they don't exist.
func (s *PostgresStorage) EnsureSchema(ctx context.Context) error {
	schema := `
		-- Cost records table
		CREATE TABLE IF NOT EXISTS ai_cost_records (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			tenant_id TEXT NOT NULL,
			request_id TEXT,
			model TEXT NOT NULL,
			provider TEXT NOT NULL,
			request_type TEXT NOT NULL,
			operation TEXT,
			input_tokens INTEGER NOT NULL DEFAULT 0,
			output_tokens INTEGER NOT NULL DEFAULT 0,
			total_tokens INTEGER NOT NULL DEFAULT 0,
			input_cost DECIMAL(12,8) NOT NULL DEFAULT 0,
			output_cost DECIMAL(12,8) NOT NULL DEFAULT 0,
			total_cost DECIMAL(12,8) NOT NULL DEFAULT 0,
			processing_time_ms BIGINT DEFAULT 0,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		-- Indexes for efficient querying
		CREATE INDEX IF NOT EXISTS idx_ai_cost_records_tenant_created
			ON ai_cost_records(tenant_id, created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_ai_cost_records_tenant_model
			ON ai_cost_records(tenant_id, model);
		CREATE INDEX IF NOT EXISTS idx_ai_cost_records_tenant_operation
			ON ai_cost_records(tenant_id, operation) WHERE operation IS NOT NULL;
		CREATE INDEX IF NOT EXISTS idx_ai_cost_records_created_at
			ON ai_cost_records(created_at DESC);

		-- Budgets table
		CREATE TABLE IF NOT EXISTS ai_budgets (
			tenant_id TEXT PRIMARY KEY,
			daily_limit DECIMAL(12,4) NOT NULL DEFAULT 0,
			monthly_limit DECIMAL(12,4) NOT NULL DEFAULT 0,
			daily_spent DECIMAL(12,8) NOT NULL DEFAULT 0,
			monthly_spent DECIMAL(12,8) NOT NULL DEFAULT 0,
			alert_thresholds DECIMAL(4,2)[] DEFAULT ARRAY[0.50, 0.80, 1.00],
			alerts_sent JSONB DEFAULT '{}',
			last_updated TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		-- Daily aggregation table for fast reporting
		CREATE TABLE IF NOT EXISTS ai_cost_daily_agg (
			tenant_id TEXT NOT NULL,
			date DATE NOT NULL,
			model TEXT NOT NULL,
			provider TEXT NOT NULL,
			request_count BIGINT NOT NULL DEFAULT 0,
			total_input_tokens BIGINT NOT NULL DEFAULT 0,
			total_output_tokens BIGINT NOT NULL DEFAULT 0,
			total_cost DECIMAL(12,8) NOT NULL DEFAULT 0,
			PRIMARY KEY (tenant_id, date, model)
		);

		CREATE INDEX IF NOT EXISTS idx_ai_cost_daily_agg_tenant_date
			ON ai_cost_daily_agg(tenant_id, date DESC);
	`

	_, err := s.pool.Exec(ctx, schema)
	return err
}

// SaveRecord persists a cost record.
func (s *PostgresStorage) SaveRecord(ctx context.Context, record *CostRecord) error {
	if record.ID == "" {
		record.ID = uuid.New().String()
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now()
	}

	query := `
		INSERT INTO ai_cost_records (
			id, tenant_id, request_id, model, provider, request_type, operation,
			input_tokens, output_tokens, total_tokens,
			input_cost, output_cost, total_cost,
			processing_time_ms, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`

	_, err := s.pool.Exec(ctx, query,
		record.ID, record.TenantID, record.RequestID,
		record.Model, record.Provider, record.RequestType, record.Operation,
		record.InputTokens, record.OutputTokens, record.TotalTokens,
		record.InputCost, record.OutputCost, record.TotalCost,
		record.ProcessingTimeMs, record.CreatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to save cost record: %w", err)
	}

	// Update daily aggregation
	return s.updateDailyAggregation(ctx, record)
}

// updateDailyAggregation updates the daily aggregation table.
func (s *PostgresStorage) updateDailyAggregation(ctx context.Context, record *CostRecord) error {
	query := `
		INSERT INTO ai_cost_daily_agg (
			tenant_id, date, model, provider,
			request_count, total_input_tokens, total_output_tokens, total_cost
		) VALUES ($1, $2, $3, $4, 1, $5, $6, $7)
		ON CONFLICT (tenant_id, date, model) DO UPDATE SET
			request_count = ai_cost_daily_agg.request_count + 1,
			total_input_tokens = ai_cost_daily_agg.total_input_tokens + EXCLUDED.total_input_tokens,
			total_output_tokens = ai_cost_daily_agg.total_output_tokens + EXCLUDED.total_output_tokens,
			total_cost = ai_cost_daily_agg.total_cost + EXCLUDED.total_cost
	`

	_, err := s.pool.Exec(ctx, query,
		record.TenantID, record.CreatedAt.UTC().Truncate(24*time.Hour),
		record.Model, record.Provider,
		record.InputTokens, record.OutputTokens, record.TotalCost,
	)

	return err
}

// GetRecord retrieves a cost record by ID.
func (s *PostgresStorage) GetRecord(ctx context.Context, id string) (*CostRecord, error) {
	query := `
		SELECT id, tenant_id, request_id, model, provider, request_type, operation,
			   input_tokens, output_tokens, total_tokens,
			   input_cost, output_cost, total_cost,
			   processing_time_ms, created_at
		FROM ai_cost_records
		WHERE id = $1
	`

	record := &CostRecord{}
	err := s.pool.QueryRow(ctx, query, id).Scan(
		&record.ID, &record.TenantID, &record.RequestID,
		&record.Model, &record.Provider, &record.RequestType, &record.Operation,
		&record.InputTokens, &record.OutputTokens, &record.TotalTokens,
		&record.InputCost, &record.OutputCost, &record.TotalCost,
		&record.ProcessingTimeMs, &record.CreatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRecordNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get cost record: %w", err)
	}

	return record, nil
}

// QueryRecords retrieves cost records matching the filter.
func (s *PostgresStorage) QueryRecords(ctx context.Context, filter *CostFilter) ([]*CostRecord, error) {
	var conditions []string
	var args []interface{}
	argNum := 1

	// Always filter by tenant (security requirement)
	conditions = append(conditions, fmt.Sprintf("tenant_id = $%d", argNum))
	args = append(args, filter.TenantID)
	argNum++

	if !filter.StartTime.IsZero() {
		conditions = append(conditions, fmt.Sprintf("created_at >= $%d", argNum))
		args = append(args, filter.StartTime)
		argNum++
	}

	if !filter.EndTime.IsZero() {
		conditions = append(conditions, fmt.Sprintf("created_at < $%d", argNum))
		args = append(args, filter.EndTime)
		argNum++
	}

	if len(filter.Models) > 0 {
		conditions = append(conditions, fmt.Sprintf("model = ANY($%d)", argNum))
		args = append(args, filter.Models)
		argNum++
	}

	if len(filter.Providers) > 0 {
		conditions = append(conditions, fmt.Sprintf("provider = ANY($%d)", argNum))
		args = append(args, filter.Providers)
		argNum++
	}

	if len(filter.RequestTypes) > 0 {
		conditions = append(conditions, fmt.Sprintf("request_type = ANY($%d)", argNum))
		args = append(args, filter.RequestTypes)
		argNum++
	}

	if len(filter.Operations) > 0 {
		conditions = append(conditions, fmt.Sprintf("operation = ANY($%d)", argNum))
		args = append(args, filter.Operations)
		argNum++
	}

	query := `
		SELECT id, tenant_id, request_id, model, provider, request_type, operation,
			   input_tokens, output_tokens, total_tokens,
			   input_cost, output_cost, total_cost,
			   processing_time_ms, created_at
		FROM ai_cost_records
		WHERE ` + strings.Join(conditions, " AND ") + `
		ORDER BY created_at DESC
	`

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}
	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", filter.Offset)
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query cost records: %w", err)
	}
	defer rows.Close()

	var records []*CostRecord
	for rows.Next() {
		record := &CostRecord{}
		err := rows.Scan(
			&record.ID, &record.TenantID, &record.RequestID,
			&record.Model, &record.Provider, &record.RequestType, &record.Operation,
			&record.InputTokens, &record.OutputTokens, &record.TotalTokens,
			&record.InputCost, &record.OutputCost, &record.TotalCost,
			&record.ProcessingTimeMs, &record.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan cost record: %w", err)
		}
		records = append(records, record)
	}

	return records, rows.Err()
}

// GetCostReport generates an aggregated cost report.
func (s *PostgresStorage) GetCostReport(ctx context.Context, filter *CostFilter, granularity TimeRange) (*CostReport, error) {
	report := &CostReport{
		TenantID:             filter.TenantID,
		StartTime:            filter.StartTime,
		EndTime:              filter.EndTime,
		TimeRangeGranularity: granularity,
		GeneratedAt:          time.Now(),
	}

	// Get totals
	totalsQuery := `
		SELECT
			COALESCE(SUM(total_cost), 0),
			COALESCE(COUNT(*), 0),
			COALESCE(SUM(input_tokens), 0),
			COALESCE(SUM(output_tokens), 0)
		FROM ai_cost_records
		WHERE tenant_id = $1 AND created_at >= $2 AND created_at < $3
	`
	err := s.pool.QueryRow(ctx, totalsQuery, filter.TenantID, filter.StartTime, filter.EndTime).Scan(
		&report.TotalCost, &report.TotalRequests,
		&report.TotalInputTokens, &report.TotalOutputTokens,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get cost totals: %w", err)
	}

	// Get breakdown by model
	modelQuery := `
		SELECT model, provider,
			   COUNT(*),
			   SUM(input_tokens),
			   SUM(output_tokens),
			   SUM(total_cost)
		FROM ai_cost_records
		WHERE tenant_id = $1 AND created_at >= $2 AND created_at < $3
		GROUP BY model, provider
		ORDER BY SUM(total_cost) DESC
	`
	rows, err := s.pool.Query(ctx, modelQuery, filter.TenantID, filter.StartTime, filter.EndTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get model breakdown: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var summary ModelCostSummary
		err := rows.Scan(
			&summary.Model, &summary.Provider,
			&summary.RequestCount,
			&summary.TotalInputTokens, &summary.TotalOutputTokens,
			&summary.TotalCost,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan model summary: %w", err)
		}
		if summary.RequestCount > 0 {
			summary.AvgCostPerRequest = summary.TotalCost / float64(summary.RequestCount)
		}
		report.ByModel = append(report.ByModel, summary)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	// Get breakdown by operation
	operationQuery := `
		SELECT COALESCE(operation, 'unknown'),
			   COUNT(*),
			   SUM(total_cost)
		FROM ai_cost_records
		WHERE tenant_id = $1 AND created_at >= $2 AND created_at < $3
		GROUP BY operation
		ORDER BY SUM(total_cost) DESC
	`
	rows, err = s.pool.Query(ctx, operationQuery, filter.TenantID, filter.StartTime, filter.EndTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get operation breakdown: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var summary OperationCostSummary
		err := rows.Scan(&summary.Operation, &summary.RequestCount, &summary.TotalCost)
		if err != nil {
			return nil, fmt.Errorf("failed to scan operation summary: %w", err)
		}
		if summary.RequestCount > 0 {
			summary.AvgCostPerRequest = summary.TotalCost / float64(summary.RequestCount)
		}
		report.ByOperation = append(report.ByOperation, summary)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	// Get time series if granularity is specified
	if granularity != "" {
		report.ByTimeRange, err = s.getTimeSeries(ctx, filter, granularity)
		if err != nil {
			return nil, err
		}
	}

	return report, nil
}

// getTimeSeries returns cost data aggregated by time periods.
func (s *PostgresStorage) getTimeSeries(ctx context.Context, filter *CostFilter, granularity TimeRange) ([]TimePeriodCost, error) {
	var truncExpr string
	switch granularity {
	case TimeRangeHour:
		truncExpr = "date_trunc('hour', created_at)"
	case TimeRangeDay:
		truncExpr = "date_trunc('day', created_at)"
	case TimeRangeWeek:
		truncExpr = "date_trunc('week', created_at)"
	case TimeRangeMonth:
		truncExpr = "date_trunc('month', created_at)"
	default:
		truncExpr = "date_trunc('day', created_at)"
	}

	query := fmt.Sprintf(`
		SELECT %s as period,
			   SUM(total_cost),
			   COUNT(*)
		FROM ai_cost_records
		WHERE tenant_id = $1 AND created_at >= $2 AND created_at < $3
		GROUP BY period
		ORDER BY period
	`, truncExpr)

	rows, err := s.pool.Query(ctx, query, filter.TenantID, filter.StartTime, filter.EndTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get time series: %w", err)
	}
	defer rows.Close()

	var series []TimePeriodCost
	for rows.Next() {
		var period TimePeriodCost
		err := rows.Scan(&period.Period, &period.TotalCost, &period.RequestCount)
		if err != nil {
			return nil, fmt.Errorf("failed to scan time period: %w", err)
		}
		series = append(series, period)
	}

	return series, rows.Err()
}

// SaveBudget persists a budget.
func (s *PostgresStorage) SaveBudget(ctx context.Context, budget *Budget) error {
	query := `
		INSERT INTO ai_budgets (
			tenant_id, daily_limit, monthly_limit,
			daily_spent, monthly_spent,
			alert_thresholds, alerts_sent,
			last_updated, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (tenant_id) DO UPDATE SET
			daily_limit = EXCLUDED.daily_limit,
			monthly_limit = EXCLUDED.monthly_limit,
			daily_spent = EXCLUDED.daily_spent,
			monthly_spent = EXCLUDED.monthly_spent,
			alert_thresholds = EXCLUDED.alert_thresholds,
			alerts_sent = EXCLUDED.alerts_sent,
			last_updated = EXCLUDED.last_updated
	`

	_, err := s.pool.Exec(ctx, query,
		budget.TenantID, budget.DailyLimit, budget.MonthlyLimit,
		budget.DailySpent, budget.MonthlySpent,
		budget.AlertThresholds, budget.AlertsSent,
		budget.LastUpdated, budget.CreatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to save budget: %w", err)
	}

	return nil
}

// GetBudget retrieves a budget for a tenant.
func (s *PostgresStorage) GetBudget(ctx context.Context, tenantID string) (*Budget, error) {
	query := `
		SELECT tenant_id, daily_limit, monthly_limit,
			   daily_spent, monthly_spent,
			   alert_thresholds, alerts_sent,
			   last_updated, created_at
		FROM ai_budgets
		WHERE tenant_id = $1
	`

	budget := &Budget{}
	err := s.pool.QueryRow(ctx, query, tenantID).Scan(
		&budget.TenantID, &budget.DailyLimit, &budget.MonthlyLimit,
		&budget.DailySpent, &budget.MonthlySpent,
		&budget.AlertThresholds, &budget.AlertsSent,
		&budget.LastUpdated, &budget.CreatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrBudgetNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get budget: %w", err)
	}

	// Calculate remaining
	if budget.DailyLimit > 0 {
		budget.DailyRemaining = budget.DailyLimit - budget.DailySpent
		if budget.DailyRemaining < 0 {
			budget.DailyRemaining = 0
		}
	}
	if budget.MonthlyLimit > 0 {
		budget.MonthlyRemaining = budget.MonthlyLimit - budget.MonthlySpent
		if budget.MonthlyRemaining < 0 {
			budget.MonthlyRemaining = 0
		}
	}

	return budget, nil
}

// UpdateBudgetSpending atomically updates budget spending.
func (s *PostgresStorage) UpdateBudgetSpending(ctx context.Context, tenantID string, amount float64) (*Budget, error) {
	query := `
		UPDATE ai_budgets
		SET daily_spent = daily_spent + $2,
			monthly_spent = monthly_spent + $2,
			last_updated = NOW()
		WHERE tenant_id = $1
		RETURNING tenant_id, daily_limit, monthly_limit,
				  daily_spent, monthly_spent,
				  alert_thresholds, alerts_sent,
				  last_updated, created_at
	`

	budget := &Budget{}
	err := s.pool.QueryRow(ctx, query, tenantID, amount).Scan(
		&budget.TenantID, &budget.DailyLimit, &budget.MonthlyLimit,
		&budget.DailySpent, &budget.MonthlySpent,
		&budget.AlertThresholds, &budget.AlertsSent,
		&budget.LastUpdated, &budget.CreatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrBudgetNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to update budget spending: %w", err)
	}

	// Calculate remaining
	if budget.DailyLimit > 0 {
		budget.DailyRemaining = budget.DailyLimit - budget.DailySpent
		if budget.DailyRemaining < 0 {
			budget.DailyRemaining = 0
		}
	}
	if budget.MonthlyLimit > 0 {
		budget.MonthlyRemaining = budget.MonthlyLimit - budget.MonthlySpent
		if budget.MonthlyRemaining < 0 {
			budget.MonthlyRemaining = 0
		}
	}

	return budget, nil
}

// GetDailySpending returns total spending for a tenant for today.
func (s *PostgresStorage) GetDailySpending(ctx context.Context, tenantID string) (float64, error) {
	query := `
		SELECT COALESCE(SUM(total_cost), 0)
		FROM ai_cost_records
		WHERE tenant_id = $1
		  AND created_at >= date_trunc('day', NOW())
		  AND created_at < date_trunc('day', NOW()) + INTERVAL '1 day'
	`

	var total float64
	err := s.pool.QueryRow(ctx, query, tenantID).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("failed to get daily spending: %w", err)
	}

	return total, nil
}

// GetMonthlySpending returns total spending for a tenant for this month.
func (s *PostgresStorage) GetMonthlySpending(ctx context.Context, tenantID string) (float64, error) {
	query := `
		SELECT COALESCE(SUM(total_cost), 0)
		FROM ai_cost_records
		WHERE tenant_id = $1
		  AND created_at >= date_trunc('month', NOW())
		  AND created_at < date_trunc('month', NOW()) + INTERVAL '1 month'
	`

	var total float64
	err := s.pool.QueryRow(ctx, query, tenantID).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("failed to get monthly spending: %w", err)
	}

	return total, nil
}

// Close closes the storage connection.
func (s *PostgresStorage) Close() error {
	s.pool.Close()
	return nil
}

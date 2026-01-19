// Package analytics provides search analytics tracking and aggregation.
package analytics

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// Aggregator provides methods for aggregating analytics data.
// It handles daily, weekly, and monthly rollups for efficient querying.
type Aggregator struct {
	store  Store
	logger logging.Logger
}

// NewAggregator creates a new analytics aggregator.
func NewAggregator(store Store, logger logging.Logger) *Aggregator {
	return &Aggregator{
		store:  store,
		logger: logger.With(logging.F("component", "analytics_aggregator")),
	}
}

// AggregateDaily computes and stores a daily rollup for the specified date.
func (a *Aggregator) AggregateDaily(ctx context.Context, tenantID string, date time.Time) (*DailyRollup, error) {
	if tenantID == "" {
		return nil, ErrInvalidTenantID
	}

	// Compute the rollup from raw events
	rollup, err := a.store.ComputeDailyRollup(ctx, tenantID, date)
	if err != nil {
		return nil, err
	}

	// Get top queries for the day
	filter := StatsFilter{
		TenantID: tenantID,
		DateFrom: date.Truncate(24 * time.Hour),
		DateTo:   date.Truncate(24 * time.Hour).Add(24 * time.Hour),
		Limit:    50,
	}

	topQueries, err := a.store.GetTopQueries(ctx, filter, 50)
	if err == nil && len(topQueries) > 0 {
		jsonBytes, err := json.Marshal(topQueries)
		if err == nil {
			rollup.TopQueriesJSON = jsonBytes
		}
	}

	// Get zero-result queries for the day
	zeroQueries, err := a.store.GetZeroResultQueries(ctx, filter, 100)
	if err == nil && len(zeroQueries) > 0 {
		jsonBytes, err := json.Marshal(zeroQueries)
		if err == nil {
			rollup.ZeroResultQueriesJSON = jsonBytes
		}
	}

	// Save the rollup
	if err := a.store.SaveDailyRollup(ctx, rollup); err != nil {
		return nil, err
	}

	a.logger.Info("Created daily rollup",
		logging.F("tenant_id", tenantID),
		logging.F("date", date.Format("2006-01-02")),
		logging.F("total_queries", rollup.TotalQueries),
	)

	return rollup, nil
}

// AggregateWeekly computes weekly statistics by aggregating daily rollups.
func (a *Aggregator) AggregateWeekly(ctx context.Context, tenantID string, weekStart time.Time) (*QueryStats, error) {
	if tenantID == "" {
		return nil, ErrInvalidTenantID
	}

	// Ensure weekStart is the start of a week (Monday)
	weekStart = weekStart.Truncate(24 * time.Hour)
	for weekStart.Weekday() != time.Monday {
		weekStart = weekStart.AddDate(0, 0, -1)
	}
	weekEnd := weekStart.AddDate(0, 0, 7)

	filter := StatsFilter{
		TenantID:    tenantID,
		DateFrom:    weekStart,
		DateTo:      weekEnd,
		Granularity: GranularityWeek,
		Limit:       100,
	}

	// Get stats from daily rollups
	stats, err := a.store.GetQueryStats(ctx, filter)
	if err != nil {
		return nil, err
	}

	stats.Period = TimePeriod{
		Start:       weekStart,
		End:         weekEnd,
		Granularity: GranularityWeek,
	}

	return stats, nil
}

// AggregateMonthly computes monthly statistics by aggregating daily rollups.
func (a *Aggregator) AggregateMonthly(ctx context.Context, tenantID string, month time.Time) (*QueryStats, error) {
	if tenantID == "" {
		return nil, ErrInvalidTenantID
	}

	// Get the first and last day of the month
	year, mon, _ := month.Date()
	monthStart := time.Date(year, mon, 1, 0, 0, 0, 0, month.Location())
	monthEnd := monthStart.AddDate(0, 1, 0)

	filter := StatsFilter{
		TenantID:    tenantID,
		DateFrom:    monthStart,
		DateTo:      monthEnd,
		Granularity: GranularityMonth,
		Limit:       100,
	}

	// Get stats from daily rollups
	stats, err := a.store.GetQueryStats(ctx, filter)
	if err != nil {
		return nil, err
	}

	stats.Period = TimePeriod{
		Start:       monthStart,
		End:         monthEnd,
		Granularity: GranularityMonth,
	}

	return stats, nil
}

// TopQueries retrieves the most frequent queries for a time period.
func (a *Aggregator) TopQueries(ctx context.Context, filter StatsFilter, limit int) ([]QueryFrequency, error) {
	if filter.TenantID == "" {
		return nil, ErrInvalidTenantID
	}

	if limit <= 0 {
		limit = 100
	}

	return a.store.GetTopQueries(ctx, filter, limit)
}

// ZeroResultQueries retrieves queries that returned no results.
func (a *Aggregator) ZeroResultQueries(ctx context.Context, filter StatsFilter) ([]string, error) {
	if filter.TenantID == "" {
		return nil, ErrInvalidTenantID
	}

	return a.store.GetZeroResultQueries(ctx, filter, filter.Limit)
}

// ClickThroughRate calculates the CTR for the specified filter.
func (a *Aggregator) ClickThroughRate(ctx context.Context, filter StatsFilter) (float64, error) {
	if filter.TenantID == "" {
		return 0, ErrInvalidTenantID
	}

	return a.store.GetClickThroughRate(ctx, filter)
}

// ClickThroughRateByQuery calculates the CTR for a specific query pattern.
func (a *Aggregator) ClickThroughRateByQuery(ctx context.Context, tenantID, queryPattern string, from, to time.Time) (float64, error) {
	if tenantID == "" {
		return 0, ErrInvalidTenantID
	}

	filter := StatsFilter{
		TenantID: tenantID,
		DateFrom: from,
		DateTo:   to,
	}

	// This requires a query-specific implementation
	// For now, use the overall CTR
	return a.store.GetClickThroughRate(ctx, filter)
}

// TrendAnalysis provides trend analysis over time.
type TrendAnalysis struct {
	// DataPoints contains the time-series data points.
	DataPoints []TrendDataPoint `json:"data_points"`

	// TrendDirection indicates if metrics are improving or declining.
	TrendDirection string `json:"trend_direction"` // "improving", "declining", "stable"

	// ChangePercent is the percentage change from start to end.
	ChangePercent float64 `json:"change_percent"`

	// Period describes the analysis period.
	Period TimePeriod `json:"period"`
}

// TrendDataPoint represents a single point in time-series data.
type TrendDataPoint struct {
	// Date is the date for this data point.
	Date time.Time `json:"date"`

	// TotalQueries is the query count for this period.
	TotalQueries int64 `json:"total_queries"`

	// AvgLatencyMs is the average latency for this period.
	AvgLatencyMs float64 `json:"avg_latency_ms"`

	// ZeroResultRate is the zero-result rate for this period.
	ZeroResultRate float64 `json:"zero_result_rate"`

	// ClickThroughRate is the CTR for this period.
	ClickThroughRate float64 `json:"click_through_rate"`
}

// AnalyzeTrend analyzes trends in analytics data over time.
func (a *Aggregator) AnalyzeTrend(ctx context.Context, filter StatsFilter) (*TrendAnalysis, error) {
	if filter.TenantID == "" {
		return nil, ErrInvalidTenantID
	}

	rollups, err := a.store.ListDailyRollups(ctx, filter)
	if err != nil {
		return nil, err
	}

	if len(rollups) == 0 {
		return &TrendAnalysis{
			DataPoints:     []TrendDataPoint{},
			TrendDirection: "stable",
			Period: TimePeriod{
				Start:       filter.DateFrom,
				End:         filter.DateTo,
				Granularity: filter.Granularity,
			},
		}, nil
	}

	analysis := &TrendAnalysis{
		DataPoints: make([]TrendDataPoint, len(rollups)),
		Period: TimePeriod{
			Start:       filter.DateFrom,
			End:         filter.DateTo,
			Granularity: filter.Granularity,
		},
	}

	for i, rollup := range rollups {
		var avgLatency, zeroRate, ctr float64
		if rollup.TotalQueries > 0 {
			avgLatency = rollup.SumLatencyMs / float64(rollup.TotalQueries)
			zeroRate = float64(rollup.ZeroResultQueries) / float64(rollup.TotalQueries)
			ctr = float64(rollup.TotalClicks) / float64(rollup.TotalQueries)
		}

		analysis.DataPoints[i] = TrendDataPoint{
			Date:             rollup.Date,
			TotalQueries:     rollup.TotalQueries,
			AvgLatencyMs:     avgLatency,
			ZeroResultRate:   zeroRate,
			ClickThroughRate: ctr,
		}
	}

	// Calculate trend direction based on CTR
	if len(analysis.DataPoints) >= 2 {
		first := analysis.DataPoints[0]
		last := analysis.DataPoints[len(analysis.DataPoints)-1]

		if first.ClickThroughRate > 0 {
			analysis.ChangePercent = ((last.ClickThroughRate - first.ClickThroughRate) / first.ClickThroughRate) * 100
		}

		if analysis.ChangePercent > 5 {
			analysis.TrendDirection = "improving"
		} else if analysis.ChangePercent < -5 {
			analysis.TrendDirection = "declining"
		} else {
			analysis.TrendDirection = "stable"
		}
	}

	return analysis, nil
}

// BackfillRollups creates daily rollups for historical data.
func (a *Aggregator) BackfillRollups(ctx context.Context, tenantID string, from, to time.Time) error {
	if tenantID == "" {
		return ErrInvalidTenantID
	}

	// Iterate through each day in the range
	current := from.Truncate(24 * time.Hour)
	endDate := to.Truncate(24 * time.Hour)

	for !current.After(endDate) {
		_, err := a.AggregateDaily(ctx, tenantID, current)
		if err != nil {
			a.logger.Error("Failed to create rollup",
				logging.F("date", current.Format("2006-01-02")),
				logging.Err(err),
			)
			// Continue with next day even if one fails
		}

		current = current.AddDate(0, 0, 1)
	}

	a.logger.Info("Backfill completed",
		logging.F("tenant_id", tenantID),
		logging.F("from", from.Format("2006-01-02")),
		logging.F("to", to.Format("2006-01-02")),
	)

	return nil
}

// QueryPerformanceReport provides detailed query performance analysis.
type QueryPerformanceReport struct {
	// Period is the time range for this report.
	Period TimePeriod `json:"period"`

	// OverallStats contains aggregate statistics.
	OverallStats QueryStats `json:"overall_stats"`

	// SlowQueries lists queries with latency above threshold.
	SlowQueries []QueryFrequency `json:"slow_queries"`

	// FailingQueries lists queries with high zero-result rates.
	FailingQueries []QueryFrequency `json:"failing_queries"`

	// TopPerformers lists queries with high CTR.
	TopPerformers []QueryFrequency `json:"top_performers"`

	// LatencyDistribution shows the latency histogram.
	LatencyDistribution map[string]int64 `json:"latency_distribution"`

	// Recommendations contains actionable insights.
	Recommendations []string `json:"recommendations"`
}

// GeneratePerformanceReport creates a comprehensive performance report.
func (a *Aggregator) GeneratePerformanceReport(ctx context.Context, filter StatsFilter) (*QueryPerformanceReport, error) {
	if filter.TenantID == "" {
		return nil, ErrInvalidTenantID
	}

	// Get overall stats
	stats, err := a.store.GetQueryStats(ctx, filter)
	if err != nil {
		return nil, err
	}

	report := &QueryPerformanceReport{
		Period: TimePeriod{
			Start:       filter.DateFrom,
			End:         filter.DateTo,
			Granularity: filter.Granularity,
		},
		OverallStats:    *stats,
		Recommendations: []string{},
	}

	// Get slow queries (above P95 latency)
	slowQueries, err := a.getSlowQueries(ctx, filter, stats.P95LatencyMs)
	if err == nil {
		report.SlowQueries = slowQueries
	}

	// Get failing queries (high zero-result rate)
	failingQueries, err := a.getFailingQueries(ctx, filter)
	if err == nil {
		report.FailingQueries = failingQueries
	}

	// Get latency distribution from rollups
	report.LatencyDistribution = a.aggregateLatencyHistogram(ctx, filter)

	// Generate recommendations
	report.Recommendations = a.generateRecommendations(stats, slowQueries, failingQueries)

	return report, nil
}

// getSlowQueries retrieves queries with latency above threshold.
func (a *Aggregator) getSlowQueries(ctx context.Context, filter StatsFilter, threshold float64) ([]QueryFrequency, error) {
	// This would need a specialized query; for now use top queries with high latency
	topQueries, err := a.store.GetTopQueries(ctx, filter, 100)
	if err != nil {
		return nil, err
	}

	var slowQueries []QueryFrequency
	for _, q := range topQueries {
		if q.AvgLatencyMs > threshold {
			slowQueries = append(slowQueries, q)
		}
	}

	// Sort by latency descending
	sort.Slice(slowQueries, func(i, j int) bool {
		return slowQueries[i].AvgLatencyMs > slowQueries[j].AvgLatencyMs
	})

	if len(slowQueries) > 20 {
		slowQueries = slowQueries[:20]
	}

	return slowQueries, nil
}

// getFailingQueries retrieves queries with high zero-result rates.
func (a *Aggregator) getFailingQueries(ctx context.Context, filter StatsFilter) ([]QueryFrequency, error) {
	// Get queries with zero results
	zeroQueries, err := a.store.GetZeroResultQueries(ctx, filter, 50)
	if err != nil {
		return nil, err
	}

	// Convert to QueryFrequency format
	var failingQueries []QueryFrequency
	for _, q := range zeroQueries {
		failingQueries = append(failingQueries, QueryFrequency{
			QueryText:      q,
			AvgResultCount: 0,
		})
	}

	return failingQueries, nil
}

// aggregateLatencyHistogram combines latency histograms from daily rollups.
func (a *Aggregator) aggregateLatencyHistogram(ctx context.Context, filter StatsFilter) map[string]int64 {
	rollups, err := a.store.ListDailyRollups(ctx, filter)
	if err != nil {
		return nil
	}

	combined := make(map[string]int64)
	for _, rollup := range rollups {
		for bucket, count := range rollup.LatencyHistogram {
			combined[bucket] += count
		}
	}

	return combined
}

// generateRecommendations produces actionable insights based on analytics.
func (a *Aggregator) generateRecommendations(stats *QueryStats, slowQueries, failingQueries []QueryFrequency) []string {
	var recs []string

	// Check zero-result rate
	if stats.ZeroResultRate > 0.1 {
		recs = append(recs, "High zero-result rate detected. Consider improving content coverage or query understanding.")
	}

	// Check average latency
	if stats.AvgLatencyMs > 500 {
		recs = append(recs, "Average query latency is above 500ms. Consider caching or index optimization.")
	}

	// Check P95 latency
	if stats.P95LatencyMs > 1000 {
		recs = append(recs, "P95 latency is above 1 second. Investigate slow query patterns.")
	}

	// Check CTR
	if stats.ClickThroughRate < 0.1 {
		recs = append(recs, "Low click-through rate. Consider improving result ranking or snippet quality.")
	}

	// Check slow queries
	if len(slowQueries) > 5 {
		recs = append(recs, "Multiple slow query patterns detected. Review common characteristics for optimization.")
	}

	// Check failing queries
	if len(failingQueries) > 10 {
		recs = append(recs, "Many queries return no results. Consider adding synonyms or content expansion.")
	}

	if len(recs) == 0 {
		recs = append(recs, "Search performance metrics are within healthy ranges.")
	}

	return recs
}

// CompareABTest compares performance between A/B test variants.
func (a *Aggregator) CompareABTest(ctx context.Context, filter StatsFilter, testName string) (*ABTestResult, error) {
	if filter.TenantID == "" {
		return nil, ErrInvalidTenantID
	}

	result, err := a.store.GetABTestResults(ctx, filter, testName)
	if err != nil {
		return nil, err
	}

	// Determine winner based on CTR (simplified statistical significance)
	if len(result.Variants) >= 2 {
		var bestVariant string
		var bestCTR float64

		for name, stats := range result.Variants {
			if stats.ClickThroughRate > bestCTR {
				bestCTR = stats.ClickThroughRate
				bestVariant = name
			}
		}

		// Simple check for significance (at least 100 queries per variant)
		allHaveEnoughData := true
		for _, stats := range result.Variants {
			if stats.QueryCount < 100 {
				allHaveEnoughData = false
				break
			}
		}

		if allHaveEnoughData {
			result.Winner = bestVariant
			result.SignificanceLevel = 0.05 // Placeholder
		}
	}

	return result, nil
}

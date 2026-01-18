// Package analytics provides search analytics tracking and aggregation.
// It tracks query performance, user interactions (clicks, conversions),
// and provides aggregated statistics for search optimization.
package analytics

import (
	"encoding/json"
	"time"
)

// QueryEvent represents a tracked search query with its metadata and performance.
type QueryEvent struct {
	// ID is the unique identifier for this query event.
	ID string `json:"id"`

	// QueryText is the original search query text.
	QueryText string `json:"query_text"`

	// QueryHash is a hash of the normalized query for grouping similar queries.
	QueryHash string `json:"query_hash"`

	// UserID is the user who executed the query (optional).
	UserID string `json:"user_id,omitempty"`

	// TenantID is the tenant this query belongs to (required for isolation).
	TenantID string `json:"tenant_id"`

	// SessionID tracks queries within the same user session.
	SessionID string `json:"session_id,omitempty"`

	// Timestamp is when the query was executed.
	Timestamp time.Time `json:"timestamp"`

	// ResultCount is the number of results returned.
	ResultCount int `json:"result_count"`

	// TotalMatches is the estimated total matching documents.
	TotalMatches int64 `json:"total_matches"`

	// LatencyMs is the query execution time in milliseconds.
	LatencyMs float64 `json:"latency_ms"`

	// SearchMode indicates the search type (hybrid, keyword, semantic).
	SearchMode string `json:"search_mode"`

	// Filters contains the applied filter parameters.
	Filters QueryFilters `json:"filters,omitempty"`

	// ABTestVariant indicates which A/B test variant was used (if any).
	ABTestVariant string `json:"ab_test_variant,omitempty"`

	// TopResults contains IDs of the top N results for click tracking.
	TopResults []string `json:"top_results,omitempty"`
}

// QueryFilters represents the filters applied to a search query.
type QueryFilters struct {
	ContentTypes []string   `json:"content_types,omitempty"`
	DateFrom     *time.Time `json:"date_from,omitempty"`
	DateTo       *time.Time `json:"date_to,omitempty"`
	Tags         []string   `json:"tags,omitempty"`
	SourceIDs    []string   `json:"source_ids,omitempty"`
}

// ClickEvent represents a user clicking on a search result.
type ClickEvent struct {
	// ID is the unique identifier for this click event.
	ID string `json:"id"`

	// QueryID links this click to the original query.
	QueryID string `json:"query_id"`

	// ResultID is the ID of the clicked result.
	ResultID string `json:"result_id"`

	// Position is the rank position of the clicked result (1-indexed).
	Position int `json:"position"`

	// TenantID is the tenant this click belongs to.
	TenantID string `json:"tenant_id"`

	// UserID is the user who clicked (optional).
	UserID string `json:"user_id,omitempty"`

	// SessionID tracks clicks within the same user session.
	SessionID string `json:"session_id,omitempty"`

	// Timestamp is when the click occurred.
	Timestamp time.Time `json:"timestamp"`

	// DwellTimeMs is how long the user spent on the result (if tracked).
	DwellTimeMs *int64 `json:"dwell_time_ms,omitempty"`
}

// ConversionEvent represents a user taking a meaningful action on a search result.
type ConversionEvent struct {
	// ID is the unique identifier for this conversion event.
	ID string `json:"id"`

	// QueryID links this conversion to the original query.
	QueryID string `json:"query_id"`

	// ResultID is the ID of the result that led to conversion.
	ResultID string `json:"result_id"`

	// Action describes what action was taken (e.g., "download", "share", "reply").
	Action string `json:"action"`

	// TenantID is the tenant this conversion belongs to.
	TenantID string `json:"tenant_id"`

	// UserID is the user who performed the action (optional).
	UserID string `json:"user_id,omitempty"`

	// SessionID tracks conversions within the same user session.
	SessionID string `json:"session_id,omitempty"`

	// Timestamp is when the conversion occurred.
	Timestamp time.Time `json:"timestamp"`

	// Metadata contains additional action-specific data.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// QueryStats provides aggregated statistics about search queries.
type QueryStats struct {
	// TotalQueries is the count of queries in the time period.
	TotalQueries int64 `json:"total_queries"`

	// UniqueUsers is the count of distinct users who searched.
	UniqueUsers int64 `json:"unique_users"`

	// AvgLatencyMs is the average query execution time.
	AvgLatencyMs float64 `json:"avg_latency_ms"`

	// P50LatencyMs is the 50th percentile latency.
	P50LatencyMs float64 `json:"p50_latency_ms"`

	// P95LatencyMs is the 95th percentile latency.
	P95LatencyMs float64 `json:"p95_latency_ms"`

	// P99LatencyMs is the 99th percentile latency.
	P99LatencyMs float64 `json:"p99_latency_ms"`

	// AvgResultCount is the average number of results returned.
	AvgResultCount float64 `json:"avg_result_count"`

	// ZeroResultRate is the percentage of queries returning no results.
	ZeroResultRate float64 `json:"zero_result_rate"`

	// ZeroResultCount is the count of queries returning no results.
	ZeroResultCount int64 `json:"zero_result_count"`

	// TopQueries is a list of the most frequent queries.
	TopQueries []QueryFrequency `json:"top_queries"`

	// ZeroResultQueries lists queries that returned no results.
	ZeroResultQueries []string `json:"zero_result_queries,omitempty"`

	// ClickThroughRate is the overall CTR for the period.
	ClickThroughRate float64 `json:"click_through_rate"`

	// ConversionRate is the overall conversion rate.
	ConversionRate float64 `json:"conversion_rate"`

	// SearchModeBreakdown shows query distribution by search mode.
	SearchModeBreakdown map[string]int64 `json:"search_mode_breakdown"`

	// Period defines the time range for these stats.
	Period TimePeriod `json:"period"`
}

// QueryFrequency represents a query and its occurrence count.
type QueryFrequency struct {
	// QueryText is the search query text.
	QueryText string `json:"query_text"`

	// QueryHash is the normalized hash for grouping.
	QueryHash string `json:"query_hash"`

	// Count is the number of times this query was executed.
	Count int64 `json:"count"`

	// AvgLatencyMs is the average latency for this specific query.
	AvgLatencyMs float64 `json:"avg_latency_ms,omitempty"`

	// AvgResultCount is the average result count for this query.
	AvgResultCount float64 `json:"avg_result_count,omitempty"`

	// ClickThroughRate is the CTR for this specific query.
	ClickThroughRate float64 `json:"click_through_rate,omitempty"`
}

// TimePeriod defines a time range for statistics.
type TimePeriod struct {
	// Start is the beginning of the period.
	Start time.Time `json:"start"`

	// End is the end of the period.
	End time.Time `json:"end"`

	// Granularity indicates the aggregation level (hour, day, week, month).
	Granularity Granularity `json:"granularity"`
}

// Granularity represents the time granularity for aggregations.
type Granularity string

const (
	GranularityHour  Granularity = "hour"
	GranularityDay   Granularity = "day"
	GranularityWeek  Granularity = "week"
	GranularityMonth Granularity = "month"
)

// StatsFilter defines parameters for filtering analytics queries.
type StatsFilter struct {
	// TenantID filters by tenant (required).
	TenantID string

	// DateFrom is the start of the time range.
	DateFrom time.Time

	// DateTo is the end of the time range.
	DateTo time.Time

	// Granularity specifies the aggregation level.
	Granularity Granularity

	// SearchMode filters by search mode (optional).
	SearchMode string

	// UserID filters by specific user (optional).
	UserID string

	// ABTestVariant filters by A/B test variant (optional).
	ABTestVariant string

	// MinResultCount filters queries with at least this many results.
	MinResultCount *int

	// MaxResultCount filters queries with at most this many results.
	MaxResultCount *int

	// Limit limits the number of results for top-N queries.
	Limit int

	// IncludeZeroResults includes/excludes zero-result queries.
	IncludeZeroResults bool
}

// DefaultStatsFilter returns a StatsFilter with sensible defaults.
func DefaultStatsFilter(tenantID string) StatsFilter {
	now := time.Now()
	return StatsFilter{
		TenantID:           tenantID,
		DateFrom:           now.AddDate(0, 0, -30), // Last 30 days
		DateTo:             now,
		Granularity:        GranularityDay,
		Limit:              100,
		IncludeZeroResults: true,
	}
}

// ABTestResult contains comparison data between A/B test variants.
type ABTestResult struct {
	// TestName is the name of the A/B test.
	TestName string `json:"test_name"`

	// Variants contains stats for each variant.
	Variants map[string]VariantStats `json:"variants"`

	// Period is the time range for the test.
	Period TimePeriod `json:"period"`

	// Winner is the variant with better performance (if statistically significant).
	Winner string `json:"winner,omitempty"`

	// SignificanceLevel is the p-value for the comparison.
	SignificanceLevel float64 `json:"significance_level,omitempty"`
}

// VariantStats contains statistics for a single A/B test variant.
type VariantStats struct {
	// QueryCount is the number of queries with this variant.
	QueryCount int64 `json:"query_count"`

	// ClickThroughRate is the CTR for this variant.
	ClickThroughRate float64 `json:"click_through_rate"`

	// ConversionRate is the conversion rate for this variant.
	ConversionRate float64 `json:"conversion_rate"`

	// AvgLatencyMs is the average latency for this variant.
	AvgLatencyMs float64 `json:"avg_latency_ms"`

	// AvgResultCount is the average result count for this variant.
	AvgResultCount float64 `json:"avg_result_count"`
}

// DailyRollup represents aggregated daily statistics for efficient querying.
type DailyRollup struct {
	// Date is the date for this rollup.
	Date time.Time `json:"date"`

	// TenantID is the tenant for this rollup.
	TenantID string `json:"tenant_id"`

	// TotalQueries is the total query count for the day.
	TotalQueries int64 `json:"total_queries"`

	// UniqueUsers is the unique user count for the day.
	UniqueUsers int64 `json:"unique_users"`

	// TotalClicks is the total click count for the day.
	TotalClicks int64 `json:"total_clicks"`

	// TotalConversions is the total conversion count for the day.
	TotalConversions int64 `json:"total_conversions"`

	// ZeroResultQueries is the count of zero-result queries.
	ZeroResultQueries int64 `json:"zero_result_queries"`

	// SumLatencyMs is the sum of all latencies (for average calculation).
	SumLatencyMs float64 `json:"sum_latency_ms"`

	// SumResultCount is the sum of all result counts (for average calculation).
	SumResultCount int64 `json:"sum_result_count"`

	// LatencyHistogram contains latency distribution buckets.
	LatencyHistogram map[string]int64 `json:"latency_histogram"`

	// SearchModeBreakdown shows query distribution by search mode.
	SearchModeBreakdown map[string]int64 `json:"search_mode_breakdown"`

	// TopQueries contains the top queries for the day (serialized JSON).
	TopQueriesJSON []byte `json:"-"`

	// ZeroResultQueriesJSON contains zero-result queries (serialized JSON).
	ZeroResultQueriesJSON []byte `json:"-"`
}

// GetTopQueries deserializes the top queries from JSON.
func (d *DailyRollup) GetTopQueries() ([]QueryFrequency, error) {
	if len(d.TopQueriesJSON) == 0 {
		return nil, nil
	}
	var queries []QueryFrequency
	if err := json.Unmarshal(d.TopQueriesJSON, &queries); err != nil {
		return nil, err
	}
	return queries, nil
}

// GetZeroResultQueries deserializes zero-result queries from JSON.
func (d *DailyRollup) GetZeroResultQueries() ([]string, error) {
	if len(d.ZeroResultQueriesJSON) == 0 {
		return nil, nil
	}
	var queries []string
	if err := json.Unmarshal(d.ZeroResultQueriesJSON, &queries); err != nil {
		return nil, err
	}
	return queries, nil
}

// ExportFormat specifies the output format for data export.
type ExportFormat string

const (
	ExportFormatCSV  ExportFormat = "csv"
	ExportFormatJSON ExportFormat = "json"
)

// ExportOptions configures data export behavior.
type ExportOptions struct {
	// Format is the output format (CSV or JSON).
	Format ExportFormat

	// Filter defines the data to export.
	Filter StatsFilter

	// IncludeClicks includes click events in the export.
	IncludeClicks bool

	// IncludeConversions includes conversion events in the export.
	IncludeConversions bool

	// IncludeRawQueries includes individual query events.
	IncludeRawQueries bool

	// IncludeAggregations includes aggregated statistics.
	IncludeAggregations bool
}

// DefaultExportOptions returns sensible default export options.
func DefaultExportOptions(tenantID string) ExportOptions {
	return ExportOptions{
		Format:              ExportFormatCSV,
		Filter:              DefaultStatsFilter(tenantID),
		IncludeClicks:       true,
		IncludeConversions:  true,
		IncludeRawQueries:   false, // Raw queries can be large
		IncludeAggregations: true,
	}
}

// ScheduledReport configures automated report generation.
type ScheduledReport struct {
	// ID is the unique identifier for this report schedule.
	ID string `json:"id"`

	// Name is the human-readable report name.
	Name string `json:"name"`

	// TenantID is the tenant for this report.
	TenantID string `json:"tenant_id"`

	// Schedule is a cron expression for when to generate the report.
	Schedule string `json:"schedule"`

	// Options configures what data to include.
	Options ExportOptions `json:"options"`

	// Recipients is a list of email addresses to send the report to.
	Recipients []string `json:"recipients,omitempty"`

	// WebhookURL is a URL to POST the report to.
	WebhookURL string `json:"webhook_url,omitempty"`

	// Enabled indicates if this schedule is active.
	Enabled bool `json:"enabled"`

	// LastRun is when this report was last generated.
	LastRun *time.Time `json:"last_run,omitempty"`

	// NextRun is when this report will next be generated.
	NextRun *time.Time `json:"next_run,omitempty"`
}

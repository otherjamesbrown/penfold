// Package analytics provides search analytics tracking and aggregation.
package analytics

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// Exporter provides data export functionality for analytics.
type Exporter struct {
	store  Store
	logger logging.Logger
}

// NewExporter creates a new analytics exporter.
func NewExporter(store Store, logger logging.Logger) *Exporter {
	return &Exporter{
		store:  store,
		logger: logger.With(logging.F("component", "analytics_exporter")),
	}
}

// ExportResult contains the exported data and metadata.
type ExportResult struct {
	// Data contains the exported content.
	Data []byte

	// Format is the export format used.
	Format ExportFormat

	// RecordCount is the number of records exported.
	RecordCount int

	// GeneratedAt is when the export was created.
	GeneratedAt time.Time

	// Period describes the time range of the data.
	Period TimePeriod
}

// ExportCSV exports analytics data in CSV format.
func (e *Exporter) ExportCSV(ctx context.Context, options ExportOptions) (*ExportResult, error) {
	if options.Filter.TenantID == "" {
		return nil, ErrInvalidTenantID
	}

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	recordCount := 0

	// Export query events if requested
	if options.IncludeRawQueries {
		count, err := e.exportQueryEventsCSV(ctx, writer, options.Filter)
		if err != nil {
			return nil, fmt.Errorf("exporting query events: %w", err)
		}
		recordCount += count
	}

	// Export click events if requested
	if options.IncludeClicks {
		count, err := e.exportClickEventsCSV(ctx, writer, options.Filter)
		if err != nil {
			return nil, fmt.Errorf("exporting click events: %w", err)
		}
		recordCount += count
	}

	// Export conversion events if requested
	if options.IncludeConversions {
		count, err := e.exportConversionEventsCSV(ctx, writer, options.Filter)
		if err != nil {
			return nil, fmt.Errorf("exporting conversion events: %w", err)
		}
		recordCount += count
	}

	// Export aggregations if requested
	if options.IncludeAggregations {
		count, err := e.exportAggregationsCSV(ctx, writer, options.Filter)
		if err != nil {
			return nil, fmt.Errorf("exporting aggregations: %w", err)
		}
		recordCount += count
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("csv writer error: %w", err)
	}

	e.logger.Info("CSV export completed",
		logging.F("tenant_id", options.Filter.TenantID),
		logging.F("record_count", recordCount),
	)

	return &ExportResult{
		Data:        buf.Bytes(),
		Format:      ExportFormatCSV,
		RecordCount: recordCount,
		GeneratedAt: time.Now().UTC(),
		Period: TimePeriod{
			Start:       options.Filter.DateFrom,
			End:         options.Filter.DateTo,
			Granularity: options.Filter.Granularity,
		},
	}, nil
}

// exportQueryEventsCSV writes query events to CSV.
func (e *Exporter) exportQueryEventsCSV(ctx context.Context, writer *csv.Writer, filter StatsFilter) (int, error) {
	// Write header
	header := []string{
		"event_type", "id", "query_text", "query_hash", "user_id", "tenant_id",
		"session_id", "timestamp", "result_count", "total_matches", "latency_ms",
		"search_mode", "ab_test_variant",
	}
	if err := writer.Write(header); err != nil {
		return 0, err
	}

	events, err := e.store.ListQueryEvents(ctx, filter)
	if err != nil {
		return 0, err
	}

	for _, event := range events {
		row := []string{
			"query",
			event.ID,
			event.QueryText,
			event.QueryHash,
			event.UserID,
			event.TenantID,
			event.SessionID,
			event.Timestamp.Format(time.RFC3339),
			fmt.Sprintf("%d", event.ResultCount),
			fmt.Sprintf("%d", event.TotalMatches),
			fmt.Sprintf("%.2f", event.LatencyMs),
			event.SearchMode,
			event.ABTestVariant,
		}
		if err := writer.Write(row); err != nil {
			return 0, err
		}
	}

	return len(events), nil
}

// exportClickEventsCSV writes click events to CSV.
func (e *Exporter) exportClickEventsCSV(ctx context.Context, writer *csv.Writer, filter StatsFilter) (int, error) {
	// Write header
	header := []string{
		"event_type", "id", "query_id", "result_id", "position",
		"tenant_id", "user_id", "session_id", "timestamp", "dwell_time_ms",
	}
	if err := writer.Write(header); err != nil {
		return 0, err
	}

	events, err := e.store.ListClickEvents(ctx, filter)
	if err != nil {
		return 0, err
	}

	for _, event := range events {
		dwellTime := ""
		if event.DwellTimeMs != nil {
			dwellTime = fmt.Sprintf("%d", *event.DwellTimeMs)
		}

		row := []string{
			"click",
			event.ID,
			event.QueryID,
			event.ResultID,
			fmt.Sprintf("%d", event.Position),
			event.TenantID,
			event.UserID,
			event.SessionID,
			event.Timestamp.Format(time.RFC3339),
			dwellTime,
		}
		if err := writer.Write(row); err != nil {
			return 0, err
		}
	}

	return len(events), nil
}

// exportConversionEventsCSV writes conversion events to CSV.
func (e *Exporter) exportConversionEventsCSV(ctx context.Context, writer *csv.Writer, filter StatsFilter) (int, error) {
	// Write header
	header := []string{
		"event_type", "id", "query_id", "result_id", "action",
		"tenant_id", "user_id", "session_id", "timestamp",
	}
	if err := writer.Write(header); err != nil {
		return 0, err
	}

	events, err := e.store.ListConversionEvents(ctx, filter)
	if err != nil {
		return 0, err
	}

	for _, event := range events {
		row := []string{
			"conversion",
			event.ID,
			event.QueryID,
			event.ResultID,
			event.Action,
			event.TenantID,
			event.UserID,
			event.SessionID,
			event.Timestamp.Format(time.RFC3339),
		}
		if err := writer.Write(row); err != nil {
			return 0, err
		}
	}

	return len(events), nil
}

// exportAggregationsCSV writes daily rollups to CSV.
func (e *Exporter) exportAggregationsCSV(ctx context.Context, writer *csv.Writer, filter StatsFilter) (int, error) {
	// Write header
	header := []string{
		"record_type", "date", "tenant_id", "total_queries", "unique_users",
		"total_clicks", "total_conversions", "zero_result_queries",
		"avg_latency_ms", "avg_result_count", "ctr", "conversion_rate",
	}
	if err := writer.Write(header); err != nil {
		return 0, err
	}

	rollups, err := e.store.ListDailyRollups(ctx, filter)
	if err != nil {
		return 0, err
	}

	for _, rollup := range rollups {
		var avgLatency, avgResultCount, ctr, convRate float64
		if rollup.TotalQueries > 0 {
			avgLatency = rollup.SumLatencyMs / float64(rollup.TotalQueries)
			avgResultCount = float64(rollup.SumResultCount) / float64(rollup.TotalQueries)
			ctr = float64(rollup.TotalClicks) / float64(rollup.TotalQueries)
		}
		if rollup.TotalClicks > 0 {
			convRate = float64(rollup.TotalConversions) / float64(rollup.TotalClicks)
		}

		row := []string{
			"daily_rollup",
			rollup.Date.Format("2006-01-02"),
			rollup.TenantID,
			fmt.Sprintf("%d", rollup.TotalQueries),
			fmt.Sprintf("%d", rollup.UniqueUsers),
			fmt.Sprintf("%d", rollup.TotalClicks),
			fmt.Sprintf("%d", rollup.TotalConversions),
			fmt.Sprintf("%d", rollup.ZeroResultQueries),
			fmt.Sprintf("%.2f", avgLatency),
			fmt.Sprintf("%.2f", avgResultCount),
			fmt.Sprintf("%.4f", ctr),
			fmt.Sprintf("%.4f", convRate),
		}
		if err := writer.Write(row); err != nil {
			return 0, err
		}
	}

	return len(rollups), nil
}

// ExportJSON exports analytics data in JSON format.
func (e *Exporter) ExportJSON(ctx context.Context, options ExportOptions) (*ExportResult, error) {
	if options.Filter.TenantID == "" {
		return nil, ErrInvalidTenantID
	}

	export := &JSONExport{
		Metadata: JSONExportMetadata{
			TenantID:    options.Filter.TenantID,
			ExportedAt:  time.Now().UTC(),
			DateFrom:    options.Filter.DateFrom,
			DateTo:      options.Filter.DateTo,
			Granularity: string(options.Filter.Granularity),
		},
	}

	recordCount := 0

	// Export query events if requested
	if options.IncludeRawQueries {
		events, err := e.store.ListQueryEvents(ctx, options.Filter)
		if err != nil {
			return nil, fmt.Errorf("fetching query events: %w", err)
		}
		export.QueryEvents = events
		recordCount += len(events)
	}

	// Export click events if requested
	if options.IncludeClicks {
		events, err := e.store.ListClickEvents(ctx, options.Filter)
		if err != nil {
			return nil, fmt.Errorf("fetching click events: %w", err)
		}
		export.ClickEvents = events
		recordCount += len(events)
	}

	// Export conversion events if requested
	if options.IncludeConversions {
		events, err := e.store.ListConversionEvents(ctx, options.Filter)
		if err != nil {
			return nil, fmt.Errorf("fetching conversion events: %w", err)
		}
		export.ConversionEvents = events
		recordCount += len(events)
	}

	// Export aggregations if requested
	if options.IncludeAggregations {
		stats, err := e.store.GetQueryStats(ctx, options.Filter)
		if err == nil {
			export.AggregatedStats = stats
		}

		rollups, err := e.store.ListDailyRollups(ctx, options.Filter)
		if err == nil {
			export.DailyRollups = rollups
			recordCount += len(rollups)
		}
	}

	export.Metadata.RecordCount = recordCount

	data, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling JSON: %w", err)
	}

	e.logger.Info("JSON export completed",
		logging.F("tenant_id", options.Filter.TenantID),
		logging.F("record_count", recordCount),
	)

	return &ExportResult{
		Data:        data,
		Format:      ExportFormatJSON,
		RecordCount: recordCount,
		GeneratedAt: time.Now().UTC(),
		Period: TimePeriod{
			Start:       options.Filter.DateFrom,
			End:         options.Filter.DateTo,
			Granularity: options.Filter.Granularity,
		},
	}, nil
}

// JSONExport represents the structure of a JSON export.
type JSONExport struct {
	Metadata         JSONExportMetadata `json:"metadata"`
	QueryEvents      []QueryEvent       `json:"query_events,omitempty"`
	ClickEvents      []ClickEvent       `json:"click_events,omitempty"`
	ConversionEvents []ConversionEvent  `json:"conversion_events,omitempty"`
	AggregatedStats  *QueryStats        `json:"aggregated_stats,omitempty"`
	DailyRollups     []DailyRollup      `json:"daily_rollups,omitempty"`
}

// JSONExportMetadata contains metadata about the export.
type JSONExportMetadata struct {
	TenantID    string    `json:"tenant_id"`
	ExportedAt  time.Time `json:"exported_at"`
	DateFrom    time.Time `json:"date_from"`
	DateTo      time.Time `json:"date_to"`
	Granularity string    `json:"granularity"`
	RecordCount int       `json:"record_count"`
}

// Export exports data in the format specified in options.
func (e *Exporter) Export(ctx context.Context, options ExportOptions) (*ExportResult, error) {
	switch options.Format {
	case ExportFormatCSV:
		return e.ExportCSV(ctx, options)
	case ExportFormatJSON:
		return e.ExportJSON(ctx, options)
	default:
		return nil, fmt.Errorf("unsupported export format: %s", options.Format)
	}
}

// ExportToWriter exports data directly to a writer.
func (e *Exporter) ExportToWriter(ctx context.Context, w io.Writer, options ExportOptions) (int, error) {
	result, err := e.Export(ctx, options)
	if err != nil {
		return 0, err
	}

	n, err := w.Write(result.Data)
	if err != nil {
		return n, err
	}

	return result.RecordCount, nil
}

// ReportScheduler manages scheduled report generation.
type ReportScheduler struct {
	store    Store
	exporter *Exporter
	logger   logging.Logger

	// Callbacks for report delivery
	onReportGenerated func(report *ScheduledReport, result *ExportResult) error

	stopChan chan struct{}
}

// NewReportScheduler creates a new report scheduler.
func NewReportScheduler(store Store, exporter *Exporter, logger logging.Logger) *ReportScheduler {
	return &ReportScheduler{
		store:    store,
		exporter: exporter,
		logger:   logger.With(logging.F("component", "report_scheduler")),
		stopChan: make(chan struct{}),
	}
}

// SetReportCallback sets the callback for when a report is generated.
func (s *ReportScheduler) SetReportCallback(fn func(*ScheduledReport, *ExportResult) error) {
	s.onReportGenerated = fn
}

// GenerateScheduledReport generates a report for a schedule.
func (s *ReportScheduler) GenerateScheduledReport(ctx context.Context, schedule *ScheduledReport) (*ExportResult, error) {
	if !schedule.Enabled {
		return nil, fmt.Errorf("schedule is disabled")
	}

	result, err := s.exporter.Export(ctx, schedule.Options)
	if err != nil {
		return nil, err
	}

	// Update last run time
	now := time.Now()
	schedule.LastRun = &now

	s.logger.Info("Generated scheduled report",
		logging.F("schedule_id", schedule.ID),
		logging.F("name", schedule.Name),
		logging.F("record_count", result.RecordCount),
	)

	// Call the callback if set
	if s.onReportGenerated != nil {
		if err := s.onReportGenerated(schedule, result); err != nil {
			s.logger.Error("Report callback failed",
				logging.F("schedule_id", schedule.ID),
				logging.Err(err),
			)
		}
	}

	return result, nil
}

// PerformanceReportGenerator generates structured performance reports.
type PerformanceReportGenerator struct {
	aggregator *Aggregator
	exporter   *Exporter
	logger     logging.Logger
}

// NewPerformanceReportGenerator creates a new performance report generator.
func NewPerformanceReportGenerator(aggregator *Aggregator, exporter *Exporter, logger logging.Logger) *PerformanceReportGenerator {
	return &PerformanceReportGenerator{
		aggregator: aggregator,
		exporter:   exporter,
		logger:     logger.With(logging.F("component", "performance_report_generator")),
	}
}

// DailyReport generates a daily performance report.
func (g *PerformanceReportGenerator) DailyReport(ctx context.Context, tenantID string, date time.Time) (*QueryPerformanceReport, error) {
	startOfDay := date.Truncate(24 * time.Hour)
	endOfDay := startOfDay.Add(24 * time.Hour)

	filter := StatsFilter{
		TenantID:    tenantID,
		DateFrom:    startOfDay,
		DateTo:      endOfDay,
		Granularity: GranularityDay,
		Limit:       50,
	}

	return g.aggregator.GeneratePerformanceReport(ctx, filter)
}

// WeeklyReport generates a weekly performance report.
func (g *PerformanceReportGenerator) WeeklyReport(ctx context.Context, tenantID string, weekStart time.Time) (*QueryPerformanceReport, error) {
	// Ensure weekStart is Monday
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

	return g.aggregator.GeneratePerformanceReport(ctx, filter)
}

// MonthlyReport generates a monthly performance report.
func (g *PerformanceReportGenerator) MonthlyReport(ctx context.Context, tenantID string, month time.Time) (*QueryPerformanceReport, error) {
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

	return g.aggregator.GeneratePerformanceReport(ctx, filter)
}

// GenerateReportJSON generates a performance report as JSON.
func (g *PerformanceReportGenerator) GenerateReportJSON(ctx context.Context, filter StatsFilter) ([]byte, error) {
	report, err := g.aggregator.GeneratePerformanceReport(ctx, filter)
	if err != nil {
		return nil, err
	}

	return json.MarshalIndent(report, "", "  ")
}

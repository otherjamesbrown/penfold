package analytics

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"testing"
	"time"
)

func TestExporter_ExportCSV(t *testing.T) {
	store := newMockStore()
	logger := testLogger()
	exporter := NewExporter(store, logger)

	ctx := context.Background()

	// Add test data
	store.queryEvents = []QueryEvent{
		{
			ID:          "q1",
			QueryText:   "test query 1",
			QueryHash:   HashQuery("test query 1"),
			TenantID:    "tenant-1",
			Timestamp:   time.Now(),
			ResultCount: 5,
			LatencyMs:   100.5,
			SearchMode:  "hybrid",
		},
		{
			ID:          "q2",
			QueryText:   "test query 2",
			QueryHash:   HashQuery("test query 2"),
			TenantID:    "tenant-1",
			Timestamp:   time.Now(),
			ResultCount: 3,
			LatencyMs:   50.2,
			SearchMode:  "keyword",
		},
	}

	// Test query events only export (uniform columns)
	options := ExportOptions{
		Format:             ExportFormatCSV,
		Filter:             DefaultStatsFilter("tenant-1"),
		IncludeClicks:      false,
		IncludeRawQueries:  true,
		IncludeConversions: false,
	}

	result, err := exporter.ExportCSV(ctx, options)
	if err != nil {
		t.Fatalf("ExportCSV failed: %v", err)
	}

	if result.Format != ExportFormatCSV {
		t.Errorf("Expected format CSV, got %s", result.Format)
	}

	if result.RecordCount != 2 { // 2 queries
		t.Errorf("Expected 2 records, got %d", result.RecordCount)
	}

	// Verify CSV can be parsed
	reader := csv.NewReader(bytes.NewReader(result.Data))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("Failed to parse CSV: %v", err)
	}

	// Should have 1 header row + 2 data rows
	if len(records) != 3 {
		t.Errorf("Expected 3 rows, got %d", len(records))
	}

	// Verify header contains expected columns
	header := records[0]
	if header[0] != "event_type" {
		t.Errorf("Expected first column to be 'event_type', got '%s'", header[0])
	}
}

func TestExporter_ExportCSV_ClicksOnly(t *testing.T) {
	store := newMockStore()
	logger := testLogger()
	exporter := NewExporter(store, logger)

	ctx := context.Background()

	store.clickEvents = []ClickEvent{
		{
			ID:        "c1",
			QueryID:   "q1",
			ResultID:  "doc-1",
			Position:  1,
			TenantID:  "tenant-1",
			Timestamp: time.Now(),
		},
	}

	options := ExportOptions{
		Format:             ExportFormatCSV,
		Filter:             DefaultStatsFilter("tenant-1"),
		IncludeClicks:      true,
		IncludeRawQueries:  false,
		IncludeConversions: false,
	}

	result, err := exporter.ExportCSV(ctx, options)
	if err != nil {
		t.Fatalf("ExportCSV failed: %v", err)
	}

	if result.RecordCount != 1 {
		t.Errorf("Expected 1 record, got %d", result.RecordCount)
	}

	// Verify CSV can be parsed
	reader := csv.NewReader(bytes.NewReader(result.Data))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("Failed to parse CSV: %v", err)
	}

	// Should have 1 header row + 1 data row
	if len(records) != 2 {
		t.Errorf("Expected 2 rows, got %d", len(records))
	}
}

func TestExporter_ExportJSON(t *testing.T) {
	store := newMockStore()
	logger := testLogger()
	exporter := NewExporter(store, logger)

	ctx := context.Background()

	// Add test data
	store.queryEvents = []QueryEvent{
		{
			ID:          "q1",
			QueryText:   "test query 1",
			QueryHash:   HashQuery("test query 1"),
			TenantID:    "tenant-1",
			Timestamp:   time.Now(),
			ResultCount: 5,
			LatencyMs:   100.5,
			SearchMode:  "hybrid",
		},
	}

	store.clickEvents = []ClickEvent{
		{
			ID:        "c1",
			QueryID:   "q1",
			ResultID:  "doc-1",
			Position:  1,
			TenantID:  "tenant-1",
			Timestamp: time.Now(),
		},
	}

	options := ExportOptions{
		Format:              ExportFormatJSON,
		Filter:              DefaultStatsFilter("tenant-1"),
		IncludeClicks:       true,
		IncludeRawQueries:   true,
		IncludeAggregations: true,
	}

	result, err := exporter.ExportJSON(ctx, options)
	if err != nil {
		t.Fatalf("ExportJSON failed: %v", err)
	}

	if result.Format != ExportFormatJSON {
		t.Errorf("Expected format JSON, got %s", result.Format)
	}

	// Verify JSON can be parsed
	var export JSONExport
	if err := json.Unmarshal(result.Data, &export); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if export.Metadata.TenantID != "tenant-1" {
		t.Errorf("Expected tenant ID 'tenant-1', got '%s'", export.Metadata.TenantID)
	}

	if len(export.QueryEvents) != 1 {
		t.Errorf("Expected 1 query event, got %d", len(export.QueryEvents))
	}

	if len(export.ClickEvents) != 1 {
		t.Errorf("Expected 1 click event, got %d", len(export.ClickEvents))
	}
}

func TestExporter_Export_InvalidTenantID(t *testing.T) {
	store := newMockStore()
	logger := testLogger()
	exporter := NewExporter(store, logger)

	ctx := context.Background()

	options := ExportOptions{
		Format: ExportFormatCSV,
		Filter: StatsFilter{
			// TenantID is missing
		},
	}

	_, err := exporter.ExportCSV(ctx, options)
	if err != ErrInvalidTenantID {
		t.Errorf("Expected ErrInvalidTenantID, got %v", err)
	}
}

func TestExporter_ExportToWriter(t *testing.T) {
	store := newMockStore()
	logger := testLogger()
	exporter := NewExporter(store, logger)

	ctx := context.Background()

	store.queryEvents = []QueryEvent{
		{
			ID:          "q1",
			QueryText:   "test query",
			QueryHash:   HashQuery("test query"),
			TenantID:    "tenant-1",
			Timestamp:   time.Now(),
			ResultCount: 5,
		},
	}

	options := ExportOptions{
		Format:            ExportFormatJSON,
		Filter:            DefaultStatsFilter("tenant-1"),
		IncludeRawQueries: true,
	}

	var buf bytes.Buffer
	count, err := exporter.ExportToWriter(ctx, &buf, options)
	if err != nil {
		t.Fatalf("ExportToWriter failed: %v", err)
	}

	if count != 1 {
		t.Errorf("Expected 1 record, got %d", count)
	}

	if buf.Len() == 0 {
		t.Error("Expected non-empty output")
	}
}

func TestPerformanceReportGenerator_DailyReport(t *testing.T) {
	store := newMockStore()
	logger := testLogger()
	aggregator := NewAggregator(store, logger)
	exporter := NewExporter(store, logger)
	generator := NewPerformanceReportGenerator(aggregator, exporter, logger)

	ctx := context.Background()

	// Add test data
	now := time.Now()
	store.queryEvents = []QueryEvent{
		{
			ID:          "q1",
			QueryText:   "test query 1",
			QueryHash:   HashQuery("test query 1"),
			TenantID:    "tenant-1",
			Timestamp:   now,
			ResultCount: 5,
			LatencyMs:   100.0,
		},
		{
			ID:          "q2",
			QueryText:   "test query 2",
			QueryHash:   HashQuery("test query 2"),
			TenantID:    "tenant-1",
			Timestamp:   now,
			ResultCount: 0, // Zero result query
			LatencyMs:   50.0,
		},
	}

	store.clickEvents = []ClickEvent{
		{
			ID:        "c1",
			QueryID:   "q1",
			ResultID:  "doc-1",
			Position:  1,
			TenantID:  "tenant-1",
			Timestamp: now,
		},
	}

	report, err := generator.DailyReport(ctx, "tenant-1", now)
	if err != nil {
		t.Fatalf("DailyReport failed: %v", err)
	}

	if report.OverallStats.TotalQueries != 2 {
		t.Errorf("Expected 2 total queries, got %d", report.OverallStats.TotalQueries)
	}

	if report.OverallStats.ZeroResultCount != 1 {
		t.Errorf("Expected 1 zero result query, got %d", report.OverallStats.ZeroResultCount)
	}

	if len(report.Recommendations) == 0 {
		t.Error("Expected at least one recommendation")
	}
}

func TestDefaultExportOptions(t *testing.T) {
	options := DefaultExportOptions("tenant-1")

	if options.Format != ExportFormatCSV {
		t.Errorf("Expected CSV format, got %s", options.Format)
	}

	if options.Filter.TenantID != "tenant-1" {
		t.Errorf("Expected tenant ID 'tenant-1', got '%s'", options.Filter.TenantID)
	}

	if !options.IncludeClicks {
		t.Error("Expected IncludeClicks to be true")
	}

	if !options.IncludeConversions {
		t.Error("Expected IncludeConversions to be true")
	}

	if options.IncludeRawQueries {
		t.Error("Expected IncludeRawQueries to be false by default")
	}

	if !options.IncludeAggregations {
		t.Error("Expected IncludeAggregations to be true")
	}
}

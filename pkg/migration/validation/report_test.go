package validation

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNewValidationReport(t *testing.T) {
	report := NewValidationReport()

	if report == nil {
		t.Fatal("NewValidationReport returned nil")
	}
	if report.Title != "Penfold Go Migration Validation Report" {
		t.Errorf("Expected default title, got %s", report.Title)
	}
	if report.Version != "1.0.0" {
		t.Errorf("Expected version 1.0.0, got %s", report.Version)
	}
	if report.Results == nil {
		t.Error("Results map should be initialized")
	}
	if report.Summary.ByCategory == nil {
		t.Error("ByCategory map should be initialized")
	}
}

func TestValidationReport_AddResult(t *testing.T) {
	report := NewValidationReport()

	result := ValidationResult{
		Name:      "test_check",
		Status:    StatusPass,
		Message:   "Test passed",
		Timestamp: time.Now(),
	}

	report.AddResult("services", result)

	// Verify result was added
	results := report.GetResults("services")
	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}
	if results[0].Name != "test_check" {
		t.Errorf("Expected name test_check, got %s", results[0].Name)
	}

	// Add another result to same category
	report.AddResult("services", ValidationResult{Name: "test_check_2", Status: StatusFail})
	results = report.GetResults("services")
	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}
}

func TestValidationReport_GetResults(t *testing.T) {
	report := NewValidationReport()

	// Empty category
	results := report.GetResults("nonexistent")
	if results != nil {
		t.Error("Expected nil for nonexistent category")
	}

	// Add results and verify
	report.AddResult("services", ValidationResult{Name: "svc1"})
	report.AddResult("endpoints", ValidationResult{Name: "ep1"})

	svcResults := report.GetResults("services")
	if len(svcResults) != 1 {
		t.Errorf("Expected 1 service result, got %d", len(svcResults))
	}

	epResults := report.GetResults("endpoints")
	if len(epResults) != 1 {
		t.Errorf("Expected 1 endpoint result, got %d", len(epResults))
	}
}

func TestValidationReport_GetResults_ReturnsCopy(t *testing.T) {
	report := NewValidationReport()
	report.AddResult("services", ValidationResult{Name: "original"})

	results1 := report.GetResults("services")
	results1[0].Name = "modified"

	results2 := report.GetResults("services")
	if results2[0].Name != "original" {
		t.Error("GetResults should return a copy")
	}
}

func TestValidationReport_GetAllResults(t *testing.T) {
	report := NewValidationReport()

	report.AddResult("services", ValidationResult{Name: "svc1"})
	report.AddResult("endpoints", ValidationResult{Name: "ep1"})
	report.AddResult("database", ValidationResult{Name: "db1"})

	allResults := report.GetAllResults()

	if len(allResults) != 3 {
		t.Errorf("Expected 3 categories, got %d", len(allResults))
	}
	if _, ok := allResults["services"]; !ok {
		t.Error("Missing services category")
	}
	if _, ok := allResults["endpoints"]; !ok {
		t.Error("Missing endpoints category")
	}
	if _, ok := allResults["database"]; !ok {
		t.Error("Missing database category")
	}
}

func TestValidationReport_GetFailedResults(t *testing.T) {
	report := NewValidationReport()

	report.AddResult("services", ValidationResult{Name: "svc1", Status: StatusPass})
	report.AddResult("services", ValidationResult{Name: "svc2", Status: StatusFail})
	report.AddResult("endpoints", ValidationResult{Name: "ep1", Status: StatusWarn})
	report.AddResult("database", ValidationResult{Name: "db1", Status: StatusFail})

	failed := report.GetFailedResults()

	if len(failed) != 2 {
		t.Errorf("Expected 2 failed results, got %d", len(failed))
	}

	// Verify only failed results
	for _, r := range failed {
		if r.Status != StatusFail {
			t.Errorf("Expected only FAIL status, got %s", r.Status)
		}
	}
}

func TestValidationReport_calculateSummary(t *testing.T) {
	report := NewValidationReport()
	report.StartTime = time.Now().Add(-5 * time.Second)
	report.EndTime = time.Now()

	report.AddResult("services", ValidationResult{Status: StatusPass})
	report.AddResult("services", ValidationResult{Status: StatusPass})
	report.AddResult("services", ValidationResult{Status: StatusFail})
	report.AddResult("endpoints", ValidationResult{Status: StatusWarn})
	report.AddResult("database", ValidationResult{Status: StatusSkip})

	report.calculateSummary()

	// Verify totals
	if report.Summary.TotalChecks != 5 {
		t.Errorf("Expected total 5, got %d", report.Summary.TotalChecks)
	}
	if report.Summary.PassedChecks != 2 {
		t.Errorf("Expected passed 2, got %d", report.Summary.PassedChecks)
	}
	if report.Summary.FailedChecks != 1 {
		t.Errorf("Expected failed 1, got %d", report.Summary.FailedChecks)
	}
	if report.Summary.WarningChecks != 1 {
		t.Errorf("Expected warning 1, got %d", report.Summary.WarningChecks)
	}
	if report.Summary.SkippedChecks != 1 {
		t.Errorf("Expected skipped 1, got %d", report.Summary.SkippedChecks)
	}

	// Verify overall status (FAIL because there's a failure)
	if report.Summary.OverallStatus != StatusFail {
		t.Errorf("Expected overall FAIL, got %s", report.Summary.OverallStatus)
	}

	// Verify category stats
	svcStats := report.Summary.ByCategory["services"]
	if svcStats.Total != 3 {
		t.Errorf("Expected services total 3, got %d", svcStats.Total)
	}
	if svcStats.Status != StatusFail {
		t.Errorf("Expected services status FAIL, got %s", svcStats.Status)
	}
}

func TestValidationReport_calculateSummary_OverallStatus(t *testing.T) {
	tests := []struct {
		name           string
		results        []ValidationResult
		expectedStatus CheckStatus
	}{
		{
			name: "all pass",
			results: []ValidationResult{
				{Status: StatusPass},
				{Status: StatusPass},
			},
			expectedStatus: StatusPass,
		},
		{
			name: "has failure",
			results: []ValidationResult{
				{Status: StatusPass},
				{Status: StatusFail},
			},
			expectedStatus: StatusFail,
		},
		{
			name: "has warning no failure",
			results: []ValidationResult{
				{Status: StatusPass},
				{Status: StatusWarn},
			},
			expectedStatus: StatusWarn,
		},
		{
			name: "all skipped",
			results: []ValidationResult{
				{Status: StatusSkip},
				{Status: StatusSkip},
			},
			expectedStatus: StatusSkip,
		},
		{
			name:           "empty",
			results:        []ValidationResult{},
			expectedStatus: StatusSkip,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := NewValidationReport()
			for _, r := range tt.results {
				report.AddResult("test", r)
			}
			report.calculateSummary()

			if report.Summary.OverallStatus != tt.expectedStatus {
				t.Errorf("Expected overall status %s, got %s", tt.expectedStatus, report.Summary.OverallStatus)
			}
		})
	}
}

func TestValidationReport_GenerateReport_JSON(t *testing.T) {
	report := NewValidationReport()
	report.Title = "Test Report"
	report.Environment = "test"
	report.StartTime = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	report.EndTime = time.Date(2024, 1, 1, 0, 0, 5, 0, time.UTC)

	report.AddResult("services", ValidationResult{
		Name:    "test_service",
		Status:  StatusPass,
		Message: "Service is healthy",
	})

	report.calculateSummary()

	var buf bytes.Buffer
	err := report.GenerateReport(&buf, FormatJSON)
	if err != nil {
		t.Fatalf("GenerateReport JSON failed: %v", err)
	}

	// Verify valid JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("Invalid JSON output: %v", err)
	}

	// Verify structure
	if parsed["title"] != "Test Report" {
		t.Errorf("Expected title 'Test Report', got %v", parsed["title"])
	}
}

func TestValidationReport_GenerateReport_Text(t *testing.T) {
	report := NewValidationReport()
	report.Title = "Test Migration Report"
	report.Version = "1.0.0"
	report.Environment = "testing"
	report.StartTime = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	report.EndTime = time.Date(2024, 1, 1, 0, 0, 5, 0, time.UTC)

	report.AddResult("services", ValidationResult{
		Name:    "test_service",
		Status:  StatusPass,
		Message: "Service is healthy",
	})
	report.AddResult("services", ValidationResult{
		Name:   "failed_service",
		Status: StatusFail,
		Error:  "Connection refused",
	})

	report.calculateSummary()

	var buf bytes.Buffer
	err := report.GenerateReport(&buf, FormatText)
	if err != nil {
		t.Fatalf("GenerateReport Text failed: %v", err)
	}

	output := buf.String()

	// Verify key sections are present
	if !strings.Contains(output, "Test Migration Report") {
		t.Error("Missing title in text output")
	}
	if !strings.Contains(output, "OVERALL STATUS:") {
		t.Error("Missing overall status in text output")
	}
	if !strings.Contains(output, "SUMMARY") {
		t.Error("Missing summary section in text output")
	}
	if !strings.Contains(output, "RESULTS BY CATEGORY") {
		t.Error("Missing results by category in text output")
	}
	if !strings.Contains(output, "FAILED CHECKS DETAIL") {
		t.Error("Missing failed checks detail in text output")
	}
	if !strings.Contains(output, "Connection refused") {
		t.Error("Missing error message in text output")
	}
}

func TestValidationReport_GenerateReport_HTML(t *testing.T) {
	report := NewValidationReport()
	report.Title = "HTML Test Report"
	report.StartTime = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	report.EndTime = time.Date(2024, 1, 1, 0, 0, 5, 0, time.UTC)

	report.AddResult("services", ValidationResult{
		Name:     "test_service",
		Status:   StatusPass,
		Message:  "Service is healthy",
		Duration: 100 * time.Millisecond,
	})

	report.calculateSummary()

	var buf bytes.Buffer
	err := report.GenerateReport(&buf, FormatHTML)
	if err != nil {
		t.Fatalf("GenerateReport HTML failed: %v", err)
	}

	output := buf.String()

	// Verify HTML structure
	if !strings.Contains(output, "<!DOCTYPE html>") {
		t.Error("Missing DOCTYPE in HTML output")
	}
	if !strings.Contains(output, "<title>HTML Test Report</title>") {
		t.Error("Missing title in HTML output")
	}
	if !strings.Contains(output, "class=\"status-badge") {
		t.Error("Missing status badge in HTML output")
	}
	if !strings.Contains(output, "test_service") {
		t.Error("Missing test_service in HTML output")
	}
}

func TestValidationReport_GenerateReport_UnsupportedFormat(t *testing.T) {
	report := NewValidationReport()

	var buf bytes.Buffer
	err := report.GenerateReport(&buf, ReportFormat("xml"))
	if err == nil {
		t.Error("Expected error for unsupported format")
	}
	if !strings.Contains(err.Error(), "unsupported report format") {
		t.Errorf("Expected unsupported format error, got: %v", err)
	}
}

func TestValidationReport_ExportFunctions(t *testing.T) {
	report := NewValidationReport()
	report.AddResult("test", ValidationResult{Name: "test", Status: StatusPass})
	report.calculateSummary()

	t.Run("ExportJSON", func(t *testing.T) {
		var buf bytes.Buffer
		err := report.ExportJSON(&buf)
		if err != nil {
			t.Errorf("ExportJSON failed: %v", err)
		}
		if buf.Len() == 0 {
			t.Error("ExportJSON produced empty output")
		}
	})

	t.Run("ExportHTML", func(t *testing.T) {
		var buf bytes.Buffer
		err := report.ExportHTML(&buf)
		if err != nil {
			t.Errorf("ExportHTML failed: %v", err)
		}
		if buf.Len() == 0 {
			t.Error("ExportHTML produced empty output")
		}
	})

	t.Run("ExportText", func(t *testing.T) {
		var buf bytes.Buffer
		err := report.ExportText(&buf)
		if err != nil {
			t.Errorf("ExportText failed: %v", err)
		}
		if buf.Len() == 0 {
			t.Error("ExportText produced empty output")
		}
	})
}

func TestValidationReport_PassRate(t *testing.T) {
	tests := []struct {
		name     string
		results  []ValidationResult
		expected float64
	}{
		{
			name:     "empty",
			results:  []ValidationResult{},
			expected: 0,
		},
		{
			name: "all pass",
			results: []ValidationResult{
				{Status: StatusPass},
				{Status: StatusPass},
			},
			expected: 100,
		},
		{
			name: "half pass",
			results: []ValidationResult{
				{Status: StatusPass},
				{Status: StatusFail},
			},
			expected: 50,
		},
		{
			name: "one third pass",
			results: []ValidationResult{
				{Status: StatusPass},
				{Status: StatusFail},
				{Status: StatusWarn},
			},
			expected: 100.0 / 3.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := NewValidationReport()
			for _, r := range tt.results {
				report.AddResult("test", r)
			}
			report.calculateSummary()

			rate := report.PassRate()
			// Use tolerance for floating point comparison
			diff := rate - tt.expected
			if diff < 0 {
				diff = -diff
			}
			if diff > 0.0001 {
				t.Errorf("Expected pass rate %f, got %f", tt.expected, rate)
			}
		})
	}
}

func TestValidationReport_IsPassing(t *testing.T) {
	tests := []struct {
		name     string
		results  []ValidationResult
		expected bool
	}{
		{
			name: "all pass",
			results: []ValidationResult{
				{Status: StatusPass},
			},
			expected: true,
		},
		{
			name: "has failure",
			results: []ValidationResult{
				{Status: StatusPass},
				{Status: StatusFail},
			},
			expected: false,
		},
		{
			name: "has warning only",
			results: []ValidationResult{
				{Status: StatusPass},
				{Status: StatusWarn},
			},
			expected: true, // WARN is considered passing
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := NewValidationReport()
			for _, r := range tt.results {
				report.AddResult("test", r)
			}
			report.calculateSummary()

			if report.IsPassing() != tt.expected {
				t.Errorf("Expected IsPassing %v, got %v", tt.expected, report.IsPassing())
			}
		})
	}
}

func TestMigrationStatus(t *testing.T) {
	status := MigrationStatus{
		Phase:            "migration_in_progress",
		Progress:         75.0,
		ServicesReady:    5,
		TotalServices:    7,
		ValidationPassed: false,
		LastValidation:   time.Now(),
		Issues:           []string{"service gateway failed", "database connection issue"},
	}

	if status.Phase != "migration_in_progress" {
		t.Errorf("Expected phase migration_in_progress, got %s", status.Phase)
	}
	if len(status.Issues) != 2 {
		t.Errorf("Expected 2 issues, got %d", len(status.Issues))
	}
}

func TestValidationReport_GetMigrationStatus(t *testing.T) {
	t.Run("no services", func(t *testing.T) {
		report := NewValidationReport()
		report.EndTime = time.Now()
		report.calculateSummary()

		status := report.GetMigrationStatus()

		if status.TotalServices != 0 {
			t.Errorf("Expected 0 total services, got %d", status.TotalServices)
		}
		// With 0/0 services, the phase logic falls through to default
		// (NaN comparisons return false)
		if status.Phase != "validation_pending" {
			t.Errorf("Expected phase validation_pending (0/0 edge case), got %s", status.Phase)
		}
	})

	t.Run("half services ready", func(t *testing.T) {
		report := NewValidationReport()
		report.EndTime = time.Now()

		report.AddResult("services", ValidationResult{Name: "svc1", Status: StatusPass})
		report.AddResult("services", ValidationResult{Name: "svc2", Status: StatusPass})
		report.AddResult("services", ValidationResult{Name: "svc3", Status: StatusFail, Error: "Connection refused"})
		report.AddResult("services", ValidationResult{Name: "svc4", Status: StatusFail, Error: "Timeout"})

		report.calculateSummary()

		status := report.GetMigrationStatus()

		if status.TotalServices != 4 {
			t.Errorf("Expected 4 total services, got %d", status.TotalServices)
		}
		if status.ServicesReady != 2 {
			t.Errorf("Expected 2 services ready, got %d", status.ServicesReady)
		}
		// 50% ready means servicePercent = 50, which is not < 50, so goes to "migration_in_progress"
		if status.Phase != "migration_in_progress" {
			t.Errorf("Expected phase migration_in_progress (50%%), got %s", status.Phase)
		}
		if len(status.Issues) != 2 {
			t.Errorf("Expected 2 issues, got %d", len(status.Issues))
		}
	})

	t.Run("all services ready passing", func(t *testing.T) {
		report := NewValidationReport()
		report.EndTime = time.Now()

		report.AddResult("services", ValidationResult{Name: "svc1", Status: StatusPass})
		report.AddResult("services", ValidationResult{Name: "svc2", Status: StatusPass})

		report.calculateSummary()

		status := report.GetMigrationStatus()

		if status.TotalServices != 2 {
			t.Errorf("Expected 2 total services, got %d", status.TotalServices)
		}
		if status.ServicesReady != 2 {
			t.Errorf("Expected 2 services ready, got %d", status.ServicesReady)
		}
		if status.Phase != "migration_complete" {
			t.Errorf("Expected phase migration_complete, got %s", status.Phase)
		}
		if !status.ValidationPassed {
			t.Error("Expected ValidationPassed to be true")
		}
		if status.Progress != 100 {
			t.Errorf("Expected progress 100, got %f", status.Progress)
		}
	})

	t.Run("all services ready but validation fails", func(t *testing.T) {
		report := NewValidationReport()
		report.EndTime = time.Now()

		report.AddResult("services", ValidationResult{Name: "svc1", Status: StatusPass})
		report.AddResult("services", ValidationResult{Name: "svc2", Status: StatusPass})
		// Add a failure in another category
		report.AddResult("database", ValidationResult{Name: "db1", Status: StatusFail})

		report.calculateSummary()

		status := report.GetMigrationStatus()

		if status.Phase != "validation_pending" {
			t.Errorf("Expected phase validation_pending, got %s", status.Phase)
		}
		if status.ValidationPassed {
			t.Error("Expected ValidationPassed to be false")
		}
	})
}

func TestReportFormat(t *testing.T) {
	formats := []ReportFormat{FormatJSON, FormatHTML, FormatText}
	expected := []string{"json", "html", "text"}

	for i, f := range formats {
		if string(f) != expected[i] {
			t.Errorf("Format %d: got %s, want %s", i, f, expected[i])
		}
	}
}

func TestCategoryStats(t *testing.T) {
	stats := CategoryStats{
		Total:   10,
		Passed:  7,
		Failed:  1,
		Warning: 1,
		Skipped: 1,
		Status:  StatusWarn,
	}

	if stats.Total != 10 {
		t.Errorf("Expected total 10, got %d", stats.Total)
	}
	if stats.Status != StatusWarn {
		t.Errorf("Expected status WARN, got %s", stats.Status)
	}
}

func TestReportSummary(t *testing.T) {
	summary := ReportSummary{
		TotalChecks:    20,
		PassedChecks:   15,
		FailedChecks:   2,
		WarningChecks:  2,
		SkippedChecks:  1,
		CriticalFailed: 1,
		OverallStatus:  StatusFail,
		Duration:       5 * time.Second,
		ByCategory:     map[string]CategoryStats{},
	}

	if summary.TotalChecks != 20 {
		t.Errorf("Expected total 20, got %d", summary.TotalChecks)
	}
	if summary.Duration != 5*time.Second {
		t.Errorf("Expected duration 5s, got %v", summary.Duration)
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"short", 10, "short"},
		{"exactly10c", 10, "exactly10c"},
		{"this is a longer string", 10, "this is..."},
		{"", 10, ""},
		{"abc", 3, "abc"},
		{"abcd", 3, "..."},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := truncateString(tt.input, tt.maxLen)
			if result != tt.expected {
				t.Errorf("truncateString(%q, %d) = %q, want %q", tt.input, tt.maxLen, result, tt.expected)
			}
		})
	}
}

func TestValidationReport_ConcurrentAccess(t *testing.T) {
	report := NewValidationReport()

	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			report.AddResult("services", ValidationResult{Name: "svc"})
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for i := 0; i < 100; i++ {
			_ = report.GetResults("services")
			_ = report.GetAllResults()
			_ = report.GetFailedResults()
		}
		done <- true
	}()

	<-done
	<-done
}

func TestValidationReport_TextOutput_LongMessage(t *testing.T) {
	report := NewValidationReport()
	report.StartTime = time.Now()
	report.EndTime = time.Now()

	// Add result with very long message
	longMessage := strings.Repeat("a", 100)
	report.AddResult("services", ValidationResult{
		Name:    "test",
		Status:  StatusPass,
		Message: longMessage,
	})

	report.calculateSummary()

	var buf bytes.Buffer
	err := report.GenerateReport(&buf, FormatText)
	if err != nil {
		t.Fatalf("GenerateReport failed: %v", err)
	}

	// Verify output doesn't contain full 100 char message (should be truncated)
	output := buf.String()
	if strings.Contains(output, longMessage) {
		t.Error("Long message should be truncated in text output")
	}
}

func TestValidationReport_HTMLTemplate_NoResults(t *testing.T) {
	report := NewValidationReport()
	report.StartTime = time.Now()
	report.EndTime = time.Now()
	report.calculateSummary()

	var buf bytes.Buffer
	err := report.GenerateReport(&buf, FormatHTML)
	if err != nil {
		t.Fatalf("GenerateReport HTML failed with no results: %v", err)
	}

	// Should still produce valid HTML
	if !strings.Contains(buf.String(), "<!DOCTYPE html>") {
		t.Error("Should produce valid HTML even with no results")
	}
}

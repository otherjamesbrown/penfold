// Package cmd provides CLI commands for the penf tool.
package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/otherjamesbrown/penfold/cmd/penf/client"
	"github.com/otherjamesbrown/penfold/cmd/penf/config"
	"github.com/otherjamesbrown/penfold/services/search/query"
)

// createSearchTestDeps creates test dependencies with mock implementations.
func createSearchTestDeps(cfg *config.CLIConfig) *SearchCommandDeps {
	return &SearchCommandDeps{
		Config:       cfg,
		OutputFormat: cfg.OutputFormat,
		LoadConfig: func() (*config.CLIConfig, error) {
			return cfg, nil
		},
		InitClient: func(c *config.CLIConfig) (*client.GRPCClient, error) {
			return nil, nil
		},
	}
}

func TestNewSearchCommand(t *testing.T) {
	deps := createSearchTestDeps(mockConfig())
	cmd := NewSearchCommand(deps)

	if cmd == nil {
		t.Fatal("NewSearchCommand returned nil")
	}

	if cmd.Use != "search <query>" {
		t.Errorf("expected Use to be 'search <query>', got %q", cmd.Use)
	}

	// Check flags exist.
	flags := []string{"type", "after", "before", "mode", "limit", "offset", "sort", "verbose", "output"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q to exist", flag)
		}
	}
}

func TestNewSearchCommand_WithNilDeps(t *testing.T) {
	cmd := NewSearchCommand(nil)

	if cmd == nil {
		t.Fatal("NewSearchCommand with nil deps returned nil")
	}
}

func TestSearchMode_Validation(t *testing.T) {
	tests := []struct {
		mode  string
		valid bool
	}{
		{"hybrid", true},
		{"keyword", true},
		{"semantic", true},
		{"invalid", false},
		{"", false},
		{"Hybrid", false}, // Case sensitive.
	}

	for _, tt := range tests {
		result := ValidateSearchMode(tt.mode)
		if result != tt.valid {
			t.Errorf("ValidateSearchMode(%q) = %v, want %v", tt.mode, result, tt.valid)
		}
	}
}

func TestSortOrder_Validation(t *testing.T) {
	tests := []struct {
		sort  string
		valid bool
	}{
		{"relevance", true},
		{"date", true},
		{"date_asc", true},
		{"invalid", false},
		{"", false},
		{"Date", false}, // Case sensitive.
	}

	for _, tt := range tests {
		result := ValidateSortOrder(tt.sort)
		if result != tt.valid {
			t.Errorf("ValidateSortOrder(%q) = %v, want %v", tt.sort, result, tt.valid)
		}
	}
}

func TestParseSearchDate(t *testing.T) {
	parser := query.NewParser()
	now := time.Now()

	tests := []struct {
		input       string
		expectError bool
		checkFunc   func(time.Time) bool
	}{
		{
			input:       "today",
			expectError: false,
			checkFunc: func(t time.Time) bool {
				return t.Year() == now.Year() && t.Month() == now.Month() && t.Day() == now.Day()
			},
		},
		{
			input:       "yesterday",
			expectError: false,
			checkFunc: func(t time.Time) bool {
				yesterday := now.AddDate(0, 0, -1)
				return t.Year() == yesterday.Year() && t.Month() == yesterday.Month() && t.Day() == yesterday.Day()
			},
		},
		{
			input:       "2024-01-15",
			expectError: false,
			checkFunc: func(t time.Time) bool {
				return t.Year() == 2024 && t.Month() == time.January && t.Day() == 15
			},
		},
		{
			input:       "2024/06/30",
			expectError: false,
			checkFunc: func(t time.Time) bool {
				return t.Year() == 2024 && t.Month() == time.June && t.Day() == 30
			},
		},
		{
			input:       "lastweek",
			expectError: false,
			checkFunc: func(t time.Time) bool {
				expected := now.AddDate(0, 0, -7)
				return t.Before(now) && t.After(expected.AddDate(0, 0, -1))
			},
		},
		{
			input:       "lastmonth",
			expectError: false,
			checkFunc: func(t time.Time) bool {
				expected := now.AddDate(0, -1, 0)
				return t.Before(now) && t.After(expected.AddDate(0, 0, -1))
			},
		},
		{
			input:       "invalid-date",
			expectError: true,
			checkFunc:   nil,
		},
		{
			input:       "not a date",
			expectError: true,
			checkFunc:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := parseSearchDate(parser, tt.input)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error for input %q, got nil", tt.input)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error for input %q: %v", tt.input, err)
				} else if !tt.checkFunc(result) {
					t.Errorf("date check failed for input %q, got %v", tt.input, result)
				}
			}
		})
	}
}

func TestExecuteSearch_NoFilters(t *testing.T) {
	filters := SearchFilters{}
	results := executeSearch("test query", SearchModeHybrid, filters, 10, 0, false)

	if len(results) == 0 {
		t.Error("expected non-empty results")
	}
	if len(results) > 10 {
		t.Errorf("expected at most 10 results, got %d", len(results))
	}
}

func TestExecuteSearch_ContentTypeFilter(t *testing.T) {
	filters := SearchFilters{
		ContentTypes: []string{"email"},
	}
	results := executeSearch("test query", SearchModeHybrid, filters, 10, 0, false)

	for _, r := range results {
		if r.ContentType != "email" {
			t.Errorf("expected content type 'email', got %q", r.ContentType)
		}
	}
}

func TestExecuteSearch_MultipleContentTypes(t *testing.T) {
	filters := SearchFilters{
		ContentTypes: []string{"email", "meeting"},
	}
	results := executeSearch("test query", SearchModeHybrid, filters, 10, 0, false)

	validTypes := map[string]bool{"email": true, "meeting": true}
	for _, r := range results {
		if !validTypes[r.ContentType] {
			t.Errorf("unexpected content type %q", r.ContentType)
		}
	}
}

func TestExecuteSearch_DateFilters(t *testing.T) {
	now := time.Now()
	weekAgo := now.AddDate(0, 0, -7)

	filters := SearchFilters{
		DateFrom: &weekAgo,
	}
	results := executeSearch("test query", SearchModeHybrid, filters, 10, 0, false)

	for _, r := range results {
		if r.CreatedAt.Before(weekAgo) {
			t.Errorf("result date %v is before filter date %v", r.CreatedAt, weekAgo)
		}
	}
}

func TestExecuteSearch_Pagination(t *testing.T) {
	filters := SearchFilters{}

	// Get all results.
	allResults := executeSearch("test query", SearchModeHybrid, filters, 100, 0, false)

	if len(allResults) < 3 {
		t.Skip("not enough mock results for pagination test")
	}

	// Get first page.
	page1 := executeSearch("test query", SearchModeHybrid, filters, 2, 0, false)
	if len(page1) > 2 {
		t.Errorf("expected at most 2 results for page 1, got %d", len(page1))
	}

	// Get second page.
	page2 := executeSearch("test query", SearchModeHybrid, filters, 2, 2, false)

	// Ensure pages are different.
	if len(page1) > 0 && len(page2) > 0 && page1[0].ID == page2[0].ID {
		t.Error("expected different results for different offsets")
	}
}

func TestExecuteSearch_OffsetBeyondResults(t *testing.T) {
	filters := SearchFilters{}
	results := executeSearch("test query", SearchModeHybrid, filters, 10, 1000, false)

	if len(results) != 0 {
		t.Errorf("expected 0 results for offset beyond data, got %d", len(results))
	}
}

func TestSearchResult_JSONSerialization(t *testing.T) {
	result := SearchResult{
		ID:          "test-001",
		Title:       "Test Result",
		ContentType: "email",
		Source:      "gmail",
		Snippet:     "Test snippet with <em>highlight</em>",
		Highlights:  []string{"<em>highlight</em>"},
		Score:       0.85,
		TextScore:   0.82,
		VectorScore: 0.88,
		CreatedAt:   time.Now(),
		Metadata: map[string]string{
			"from": "test@example.com",
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal SearchResult: %v", err)
	}

	var decoded SearchResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal SearchResult: %v", err)
	}

	if decoded.ID != result.ID {
		t.Errorf("expected ID %q, got %q", result.ID, decoded.ID)
	}
	if decoded.Title != result.Title {
		t.Errorf("expected Title %q, got %q", result.Title, decoded.Title)
	}
	if decoded.Score != result.Score {
		t.Errorf("expected Score %v, got %v", result.Score, decoded.Score)
	}
}

func TestSearchResult_YAMLSerialization(t *testing.T) {
	result := SearchResult{
		ID:          "test-001",
		Title:       "Test Result",
		ContentType: "email",
		Source:      "gmail",
		Snippet:     "Test snippet",
		Score:       0.85,
		CreatedAt:   time.Now(),
	}

	data, err := yaml.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal SearchResult: %v", err)
	}

	var decoded SearchResult
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal SearchResult: %v", err)
	}

	if decoded.ID != result.ID {
		t.Errorf("expected ID %q, got %q", result.ID, decoded.ID)
	}
	if decoded.Title != result.Title {
		t.Errorf("expected Title %q, got %q", result.Title, decoded.Title)
	}
}

func TestSearchResponse_JSONSerialization(t *testing.T) {
	now := time.Now()
	dateFrom := now.AddDate(0, 0, -7)

	response := SearchResponse{
		Query: "test query",
		Mode:  SearchModeHybrid,
		Results: []SearchResult{
			{ID: "test-001", Title: "Result 1", Score: 0.9, CreatedAt: now},
			{ID: "test-002", Title: "Result 2", Score: 0.8, CreatedAt: now},
		},
		TotalCount:  100,
		QueryTimeMs: 15.5,
		Limit:       10,
		Offset:      0,
		Filters: SearchFilters{
			ContentTypes: []string{"email", "document"},
			DateFrom:     &dateFrom,
		},
		SearchedAt: now,
	}

	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("failed to marshal SearchResponse: %v", err)
	}

	var decoded SearchResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal SearchResponse: %v", err)
	}

	if decoded.Query != response.Query {
		t.Errorf("expected Query %q, got %q", response.Query, decoded.Query)
	}
	if len(decoded.Results) != len(response.Results) {
		t.Errorf("expected %d results, got %d", len(response.Results), len(decoded.Results))
	}
	if decoded.TotalCount != response.TotalCount {
		t.Errorf("expected TotalCount %d, got %d", response.TotalCount, decoded.TotalCount)
	}
	if len(decoded.Filters.ContentTypes) != len(response.Filters.ContentTypes) {
		t.Errorf("expected %d content types, got %d",
			len(response.Filters.ContentTypes), len(decoded.Filters.ContentTypes))
	}
}

func TestOutputSearchResults_JSON(t *testing.T) {
	response := SearchResponse{
		Query: "test query",
		Mode:  SearchModeHybrid,
		Results: []SearchResult{
			{ID: "test-001", Title: "Test Result", Score: 0.9, CreatedAt: time.Now()},
		},
		TotalCount:  1,
		QueryTimeMs: 10.0,
		Limit:       10,
		Offset:      0,
		SearchedAt:  time.Now(),
	}

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := outputSearchResults(config.OutputFormatJSON, response, false)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("outputSearchResults failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Verify it's valid JSON.
	var decoded SearchResponse
	if err := json.Unmarshal([]byte(output), &decoded); err != nil {
		t.Errorf("output is not valid JSON: %v", err)
	}

	if decoded.Query != "test query" {
		t.Errorf("expected query 'test query', got %q", decoded.Query)
	}
}

func TestOutputSearchResults_YAML(t *testing.T) {
	response := SearchResponse{
		Query: "test query",
		Mode:  SearchModeHybrid,
		Results: []SearchResult{
			{ID: "test-001", Title: "Test Result", Score: 0.9, CreatedAt: time.Now()},
		},
		TotalCount:  1,
		QueryTimeMs: 10.0,
		Limit:       10,
		Offset:      0,
		SearchedAt:  time.Now(),
	}

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := outputSearchResults(config.OutputFormatYAML, response, false)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("outputSearchResults failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Verify it's valid YAML.
	var decoded SearchResponse
	if err := yaml.Unmarshal([]byte(output), &decoded); err != nil {
		t.Errorf("output is not valid YAML: %v", err)
	}

	if decoded.Query != "test query" {
		t.Errorf("expected query 'test query', got %q", decoded.Query)
	}
}

func TestOutputSearchResults_Text(t *testing.T) {
	response := SearchResponse{
		Query: "project update",
		Mode:  SearchModeHybrid,
		Results: []SearchResult{
			{
				ID:          "test-001",
				Title:       "Test Result",
				ContentType: "email",
				Source:      "gmail",
				Snippet:     "Test snippet with <em>highlight</em>",
				Score:       0.9,
				CreatedAt:   time.Now().AddDate(0, 0, -1),
			},
		},
		TotalCount:  1,
		QueryTimeMs: 10.0,
		Limit:       10,
		Offset:      0,
		SearchedAt:  time.Now(),
	}

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := outputSearchResults(config.OutputFormatText, response, false)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("outputSearchResults failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Check for expected content.
	if !strings.Contains(output, "project update") {
		t.Error("output should contain query")
	}
	if !strings.Contains(output, "Test Result") {
		t.Error("output should contain result title")
	}
	if !strings.Contains(output, "email") {
		t.Error("output should contain content type")
	}
	if !strings.Contains(output, "0.9") || !strings.Contains(output, "0.90") {
		t.Error("output should contain score")
	}
}

func TestOutputSearchResults_TextEmpty(t *testing.T) {
	response := SearchResponse{
		Query:       "no results query",
		Mode:        SearchModeHybrid,
		Results:     []SearchResult{},
		TotalCount:  0,
		QueryTimeMs: 5.0,
		Limit:       10,
		Offset:      0,
		SearchedAt:  time.Now(),
	}

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := outputSearchResults(config.OutputFormatText, response, false)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("outputSearchResults failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "No results found") {
		t.Error("output should indicate no results found")
	}
}

func TestOutputSearchResults_TextVerbose(t *testing.T) {
	response := SearchResponse{
		Query: "verbose test",
		Mode:  SearchModeHybrid,
		Results: []SearchResult{
			{
				ID:          "test-001",
				Title:       "Test Result",
				ContentType: "email",
				Source:      "gmail",
				Score:       0.9,
				TextScore:   0.85,
				VectorScore: 0.95,
				CreatedAt:   time.Now(),
				Metadata: map[string]string{
					"from": "test@example.com",
				},
			},
		},
		TotalCount:  1,
		QueryTimeMs: 10.0,
		Limit:       10,
		Offset:      0,
		SearchedAt:  time.Now(),
	}

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := outputSearchResults(config.OutputFormatText, response, true) // verbose=true

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("outputSearchResults failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Verbose should include text/vector scores.
	if !strings.Contains(output, "text:") {
		t.Error("verbose output should contain text score")
	}
	if !strings.Contains(output, "vector:") {
		t.Error("verbose output should contain vector score")
	}
	// Verbose should include metadata.
	if !strings.Contains(output, "from=") {
		t.Error("verbose output should contain metadata")
	}
}

func TestFormatRelativeTime(t *testing.T) {
	now := time.Now()

	tests := []struct {
		input    time.Time
		contains string
	}{
		{now.Add(-30 * time.Second), "just now"},
		{now.Add(-5 * time.Minute), "5 minutes ago"},
		{now.Add(-1 * time.Minute), "1 minute ago"},
		{now.Add(-2 * time.Hour), "2 hours ago"},
		{now.Add(-1 * time.Hour), "1 hour ago"},
		{now.Add(-24 * time.Hour), "yesterday"},
		{now.Add(-3 * 24 * time.Hour), "3 days ago"},
		{now.Add(-14 * 24 * time.Hour), "2 weeks ago"},
		{now.Add(-45 * 24 * time.Hour), "1 month ago"},
	}

	for _, tt := range tests {
		result := formatRelativeTime(tt.input)
		if !strings.Contains(result, tt.contains) {
			t.Errorf("formatRelativeTime(%v) = %q, expected to contain %q",
				tt.input, result, tt.contains)
		}
	}
}

func TestFormatSnippet(t *testing.T) {
	tests := []struct {
		input    string
		contains string
	}{
		{
			"Test with <em>highlight</em> text",
			"\033[1;33m", // ANSI yellow bold.
		},
		{
			"No highlights here",
			"No highlights here",
		},
	}

	for _, tt := range tests {
		result := formatSnippet(tt.input)
		if !strings.Contains(result, tt.contains) {
			t.Errorf("formatSnippet(%q) = %q, expected to contain %q",
				tt.input, result, tt.contains)
		}
	}

	// Test truncation.
	longSnippet := strings.Repeat("a", 300)
	result := formatSnippet(longSnippet)
	if len(result) > 200 {
		t.Errorf("expected truncated snippet to be <= 200 chars, got %d", len(result))
	}
	if !strings.HasSuffix(result, "...") {
		t.Error("truncated snippet should end with '...'")
	}
}

func TestGetScoreColor(t *testing.T) {
	tests := []struct {
		score    float64
		expected string
	}{
		{0.9, "\033[32m"},  // Green for high.
		{0.8, "\033[32m"},  // Green for high.
		{0.7, "\033[33m"},  // Yellow for medium.
		{0.6, "\033[33m"},  // Yellow for medium.
		{0.5, "\033[31m"},  // Red for low.
		{0.3, "\033[31m"},  // Red for low.
	}

	for _, tt := range tests {
		result := getScoreColor(tt.score)
		if result != tt.expected {
			t.Errorf("getScoreColor(%v) = %q, expected %q", tt.score, result, tt.expected)
		}
	}
}

func TestRunSearch_Basic(t *testing.T) {
	cfg := mockConfig()
	deps := createSearchTestDeps(cfg)

	// Reset global flags.
	searchTypes = nil
	searchAfter = ""
	searchBefore = ""
	searchMode = "hybrid"
	searchLimit = 10
	searchOffset = 0
	searchSort = "relevance"
	searchVerbose = false
	searchOutput = ""

	// Capture stdout.
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	err := runSearch(context.Background(), deps, "test query")

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runSearch failed: %v", err)
	}
}

func TestRunSearch_WithTypeFilter(t *testing.T) {
	cfg := mockConfig()
	deps := createSearchTestDeps(cfg)

	// Set type filter.
	searchTypes = []string{"email", "meeting"}
	searchAfter = ""
	searchBefore = ""
	searchMode = "hybrid"
	searchLimit = 10
	searchOffset = 0
	searchSort = "relevance"
	searchVerbose = false
	searchOutput = ""

	// Capture stdout.
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	err := runSearch(context.Background(), deps, "test query")

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runSearch with type filter failed: %v", err)
	}

	// Reset.
	searchTypes = nil
}

func TestRunSearch_WithDateFilters(t *testing.T) {
	cfg := mockConfig()
	deps := createSearchTestDeps(cfg)

	// Set date filters.
	searchTypes = nil
	searchAfter = "2024-01-01"
	searchBefore = "2024-12-31"
	searchMode = "hybrid"
	searchLimit = 10
	searchOffset = 0
	searchSort = "relevance"
	searchVerbose = false
	searchOutput = ""

	// Capture stdout.
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	err := runSearch(context.Background(), deps, "test query")

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runSearch with date filters failed: %v", err)
	}

	// Reset.
	searchAfter = ""
	searchBefore = ""
}

func TestRunSearch_InvalidMode(t *testing.T) {
	cfg := mockConfig()
	deps := createSearchTestDeps(cfg)

	searchTypes = nil
	searchAfter = ""
	searchBefore = ""
	searchMode = "invalid_mode"
	searchLimit = 10
	searchOffset = 0
	searchSort = "relevance"
	searchVerbose = false
	searchOutput = ""

	// Capture stdout.
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	err := runSearch(context.Background(), deps, "test query")

	w.Close()
	os.Stdout = oldStdout

	if err == nil {
		t.Error("expected error for invalid mode")
	}
	if !strings.Contains(err.Error(), "invalid search mode") {
		t.Errorf("expected 'invalid search mode' error, got: %v", err)
	}

	// Reset.
	searchMode = "hybrid"
}

func TestRunSearch_InvalidSortOrder(t *testing.T) {
	cfg := mockConfig()
	deps := createSearchTestDeps(cfg)

	searchTypes = nil
	searchAfter = ""
	searchBefore = ""
	searchMode = "hybrid"
	searchLimit = 10
	searchOffset = 0
	searchSort = "invalid_sort"
	searchVerbose = false
	searchOutput = ""

	// Capture stdout.
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	err := runSearch(context.Background(), deps, "test query")

	w.Close()
	os.Stdout = oldStdout

	if err == nil {
		t.Error("expected error for invalid sort order")
	}
	if !strings.Contains(err.Error(), "invalid sort order") {
		t.Errorf("expected 'invalid sort order' error, got: %v", err)
	}

	// Reset.
	searchSort = "relevance"
}

func TestRunSearch_InvalidDateFormat(t *testing.T) {
	cfg := mockConfig()
	deps := createSearchTestDeps(cfg)

	searchTypes = nil
	searchAfter = "not-a-date"
	searchBefore = ""
	searchMode = "hybrid"
	searchLimit = 10
	searchOffset = 0
	searchSort = "relevance"
	searchVerbose = false
	searchOutput = ""

	// Capture stdout.
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	err := runSearch(context.Background(), deps, "test query")

	w.Close()
	os.Stdout = oldStdout

	if err == nil {
		t.Error("expected error for invalid date format")
	}
	if !strings.Contains(err.Error(), "invalid --after date") {
		t.Errorf("expected 'invalid --after date' error, got: %v", err)
	}

	// Reset.
	searchAfter = ""
}

func TestRunSearch_InvalidOutputFormat(t *testing.T) {
	cfg := mockConfig()
	deps := createSearchTestDeps(cfg)

	searchTypes = nil
	searchAfter = ""
	searchBefore = ""
	searchMode = "hybrid"
	searchLimit = 10
	searchOffset = 0
	searchSort = "relevance"
	searchVerbose = false
	searchOutput = "invalid_format"

	// Capture stdout.
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	err := runSearch(context.Background(), deps, "test query")

	w.Close()
	os.Stdout = oldStdout

	if err == nil {
		t.Error("expected error for invalid output format")
	}
	if !strings.Contains(err.Error(), "invalid output format") {
		t.Errorf("expected 'invalid output format' error, got: %v", err)
	}

	// Reset.
	searchOutput = ""
}

func TestRunSearch_LimitClamping(t *testing.T) {
	cfg := mockConfig()
	deps := createSearchTestDeps(cfg)

	// Test limit clamping - too high.
	searchTypes = nil
	searchAfter = ""
	searchBefore = ""
	searchMode = "hybrid"
	searchLimit = 500 // Should be clamped to 100.
	searchOffset = 0
	searchSort = "relevance"
	searchVerbose = false
	searchOutput = ""

	// Capture stdout.
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	err := runSearch(context.Background(), deps, "test query")

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runSearch with high limit should not fail: %v", err)
	}

	// Reset.
	searchLimit = 10
}

func TestRunSearch_SemanticMode(t *testing.T) {
	cfg := mockConfig()
	deps := createSearchTestDeps(cfg)

	searchTypes = nil
	searchAfter = ""
	searchBefore = ""
	searchMode = "semantic"
	searchLimit = 10
	searchOffset = 0
	searchSort = "relevance"
	searchVerbose = false
	searchOutput = ""

	// Capture stdout.
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	err := runSearch(context.Background(), deps, "conceptual search query")

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runSearch with semantic mode failed: %v", err)
	}

	// Reset.
	searchMode = "hybrid"
}

func TestRunSearch_KeywordMode(t *testing.T) {
	cfg := mockConfig()
	deps := createSearchTestDeps(cfg)

	searchTypes = nil
	searchAfter = ""
	searchBefore = ""
	searchMode = "keyword"
	searchLimit = 10
	searchOffset = 0
	searchSort = "relevance"
	searchVerbose = false
	searchOutput = ""

	// Capture stdout.
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	err := runSearch(context.Background(), deps, "exact phrase search")

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runSearch with keyword mode failed: %v", err)
	}

	// Reset.
	searchMode = "hybrid"
}

func TestRunSearch_JSONOutput(t *testing.T) {
	cfg := mockConfig()
	deps := createSearchTestDeps(cfg)

	searchTypes = nil
	searchAfter = ""
	searchBefore = ""
	searchMode = "hybrid"
	searchLimit = 5
	searchOffset = 0
	searchSort = "relevance"
	searchVerbose = false
	searchOutput = "json"

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runSearch(context.Background(), deps, "test query")

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runSearch with JSON output failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Verify it's valid JSON.
	var decoded SearchResponse
	if err := json.Unmarshal([]byte(output), &decoded); err != nil {
		t.Errorf("output is not valid JSON: %v", err)
	}

	// Reset.
	searchOutput = ""
}

func TestRunSearch_YAMLOutput(t *testing.T) {
	cfg := mockConfig()
	deps := createSearchTestDeps(cfg)

	searchTypes = nil
	searchAfter = ""
	searchBefore = ""
	searchMode = "hybrid"
	searchLimit = 5
	searchOffset = 0
	searchSort = "relevance"
	searchVerbose = false
	searchOutput = "yaml"

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runSearch(context.Background(), deps, "test query")

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runSearch with YAML output failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Verify it's valid YAML.
	var decoded SearchResponse
	if err := yaml.Unmarshal([]byte(output), &decoded); err != nil {
		t.Errorf("output is not valid YAML: %v", err)
	}

	// Reset.
	searchOutput = ""
}

func TestSearchFilters_JSONSerialization(t *testing.T) {
	now := time.Now()
	dateFrom := now.AddDate(0, 0, -7)
	dateTo := now

	filters := SearchFilters{
		ContentTypes: []string{"email", "document"},
		DateFrom:     &dateFrom,
		DateTo:       &dateTo,
		Sort:         SortOrderRelevance,
	}

	data, err := json.Marshal(filters)
	if err != nil {
		t.Fatalf("failed to marshal SearchFilters: %v", err)
	}

	var decoded SearchFilters
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal SearchFilters: %v", err)
	}

	if len(decoded.ContentTypes) != 2 {
		t.Errorf("expected 2 content types, got %d", len(decoded.ContentTypes))
	}
	if decoded.DateFrom == nil {
		t.Error("expected DateFrom to be set")
	}
	if decoded.DateTo == nil {
		t.Error("expected DateTo to be set")
	}
	if decoded.Sort != SortOrderRelevance {
		t.Errorf("expected Sort to be relevance, got %v", decoded.Sort)
	}
}

func TestDefaultSearchDeps(t *testing.T) {
	deps := DefaultSearchDeps()

	if deps == nil {
		t.Fatal("DefaultSearchDeps returned nil")
	}
	if deps.LoadConfig == nil {
		t.Error("expected LoadConfig to be set")
	}
	if deps.InitClient == nil {
		t.Error("expected InitClient to be set")
	}
}

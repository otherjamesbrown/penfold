package engine

import (
	"context"
	"testing"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// testLogger returns a mock logger for testing.
// Note: mockLogger type is defined in vector_test.go
func testLogger() logging.Logger {
	return newMockLogger()
}

func TestDefaultFieldWeights(t *testing.T) {
	weights := DefaultFieldWeights()

	if weights.Title != 1.0 {
		t.Errorf("expected title weight 1.0, got %f", weights.Title)
	}
	if weights.Body != 0.5 {
		t.Errorf("expected body weight 0.5, got %f", weights.Body)
	}
}

func TestNewBM25Engine(t *testing.T) {
	logger := testLogger()

	t.Run("with nil config uses defaults", func(t *testing.T) {
		engine := NewBM25Engine(nil, logger, nil)

		if engine == nil {
			t.Fatal("expected engine to be created")
		}

		weights := engine.GetWeights()
		if weights.Title != 1.0 {
			t.Errorf("expected default title weight 1.0, got %f", weights.Title)
		}
		if weights.Body != 0.5 {
			t.Errorf("expected default body weight 0.5, got %f", weights.Body)
		}
	})

	t.Run("with custom config", func(t *testing.T) {
		cfg := &BM25Config{
			Weights: FieldWeights{
				Title: 2.0,
				Body:  0.8,
			},
		}

		engine := NewBM25Engine(nil, logger, cfg)

		weights := engine.GetWeights()
		if weights.Title != 2.0 {
			t.Errorf("expected custom title weight 2.0, got %f", weights.Title)
		}
		if weights.Body != 0.8 {
			t.Errorf("expected custom body weight 0.8, got %f", weights.Body)
		}
	})
}

func TestSetWeights(t *testing.T) {
	logger := testLogger()
	engine := NewBM25Engine(nil, logger, nil)

	newWeights := FieldWeights{
		Title: 1.5,
		Body:  0.3,
	}

	engine.SetWeights(newWeights)

	weights := engine.GetWeights()
	if weights.Title != 1.5 {
		t.Errorf("expected title weight 1.5, got %f", weights.Title)
	}
	if weights.Body != 0.3 {
		t.Errorf("expected body weight 0.3, got %f", weights.Body)
	}
}

func TestPreprocessQuery(t *testing.T) {
	logger := testLogger()
	engine := NewBM25Engine(nil, logger, nil)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty query",
			input:    "",
			expected: "",
		},
		{
			name:     "whitespace only",
			input:    "   ",
			expected: "",
		},
		{
			name:     "single word",
			input:    "hello",
			expected: "hello:*",
		},
		{
			name:     "multiple words",
			input:    "hello world",
			expected: "hello:* & world:*",
		},
		{
			name:     "with special characters",
			input:    "hello, world! How are you?",
			expected: "hello:* & world:* & how:* & are:* & you:*",
		},
		{
			name:     "mixed case",
			input:    "Hello World TEST",
			expected: "hello:* & world:* & test:*",
		},
		{
			name:     "single character words filtered",
			input:    "a hello b world c",
			expected: "hello:* & world:*",
		},
		{
			name:     "with query expansion - email",
			input:    "email message",
			expected: "email:* & mail:* & message:*",
		},
		{
			name:     "with query expansion - doc",
			input:    "doc file",
			expected: "doc:* & document:* & file:*",
		},
		{
			name:     "hyphenated words",
			input:    "self-contained",
			expected: "self-contained:*",
		},
		{
			name:     "underscore words",
			input:    "user_name",
			expected: "user_name:*",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.preprocessQuery(tt.input)
			if result != tt.expected {
				t.Errorf("preprocessQuery(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestExpandQuery(t *testing.T) {
	logger := testLogger()
	engine := NewBM25Engine(nil, logger, nil)

	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "no expansion needed",
			input:    []string{"hello", "world"},
			expected: []string{"hello", "world"},
		},
		{
			name:     "email expansion",
			input:    []string{"email"},
			expected: []string{"email", "mail", "message"},
		},
		{
			name:     "document expansion",
			input:    []string{"document"},
			expected: []string{"document", "doc", "file"},
		},
		{
			name:     "meeting expansion",
			input:    []string{"meeting"},
			expected: []string{"meeting", "call", "session"},
		},
		{
			name:     "no duplicate expansions",
			input:    []string{"email", "mail"},
			expected: []string{"email", "mail", "message"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.expandQuery(tt.input)

			if len(result) != len(tt.expected) {
				t.Errorf("expandQuery(%v) returned %d items, expected %d: got %v", tt.input, len(result), len(tt.expected), result)
				return
			}

			// Check all expected words are present
			resultSet := make(map[string]bool)
			for _, w := range result {
				resultSet[w] = true
			}

			for _, exp := range tt.expected {
				if !resultSet[exp] {
					t.Errorf("expandQuery(%v) missing expected word %q, got %v", tt.input, exp, result)
				}
			}
		})
	}
}

func TestSearchValidation(t *testing.T) {
	logger := testLogger()
	engine := NewBM25Engine(nil, logger, nil)

	ctx := context.Background()

	t.Run("requires tenant_id", func(t *testing.T) {
		filters := SearchFilters{
			TenantID: "", // Empty tenant ID
		}

		_, err := engine.Search(ctx, "test query", filters, 10, 0)

		if err == nil {
			t.Error("expected error for missing tenant_id")
		}

		if err.Error() != "tenant_id is required for search (security: tenant isolation)" {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("empty query returns empty results", func(t *testing.T) {
		filters := SearchFilters{
			TenantID: "tenant-123",
		}

		result, err := engine.Search(ctx, "", filters, 10, 0)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if result == nil {
			t.Fatal("expected result, got nil")
		}

		if len(result.Results) != 0 {
			t.Errorf("expected 0 results for empty query, got %d", len(result.Results))
		}

		if result.TotalCount != 0 {
			t.Errorf("expected total count 0, got %d", result.TotalCount)
		}
	})
}

func TestBuildSearchQuery(t *testing.T) {
	logger := testLogger()
	engine := NewBM25Engine(nil, logger, nil)

	t.Run("basic query with tenant", func(t *testing.T) {
		filters := SearchFilters{
			TenantID: "tenant-123",
		}

		sql, args := engine.buildSearchQuery("test:*", filters, 10, 0)

		// Verify tenant_id is included in args
		if len(args) < 2 {
			t.Fatalf("expected at least 2 args, got %d", len(args))
		}

		if args[0] != "test:*" {
			t.Errorf("expected first arg to be query, got %v", args[0])
		}

		if args[1] != "tenant-123" {
			t.Errorf("expected second arg to be tenant_id, got %v", args[1])
		}

		// Verify SQL contains tenant isolation
		if !containsString(sql, "tenant_id") {
			t.Error("SQL should contain tenant_id filter")
		}

		// Verify SQL contains window function for total count
		if !containsString(sql, "COUNT(*) OVER()") {
			t.Error("SQL should contain COUNT(*) OVER() window function for total count")
		}
	})

	t.Run("with content type filter", func(t *testing.T) {
		filters := SearchFilters{
			TenantID:     "tenant-123",
			ContentTypes: []string{"email", "document"},
		}

		sql, args := engine.buildSearchQuery("test:*", filters, 10, 0)

		if !containsString(sql, "content_type = ANY") {
			t.Error("SQL should contain content_type filter")
		}

		// Verify content types are in args
		found := false
		for _, arg := range args {
			if types, ok := arg.([]string); ok {
				if len(types) == 2 && types[0] == "email" && types[1] == "document" {
					found = true
					break
				}
			}
		}

		if !found {
			t.Error("content types not found in args")
		}
	})

	t.Run("with date range filter", func(t *testing.T) {
		dateFrom := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		dateTo := time.Date(2024, 12, 31, 23, 59, 59, 0, time.UTC)

		filters := SearchFilters{
			TenantID: "tenant-123",
			DateFrom: &dateFrom,
			DateTo:   &dateTo,
		}

		sql, args := engine.buildSearchQuery("test:*", filters, 10, 0)

		if !containsString(sql, "created_at >=") {
			t.Error("SQL should contain date_from filter")
		}

		if !containsString(sql, "created_at <=") {
			t.Error("SQL should contain date_to filter")
		}

		// Verify dates are in args
		foundFrom := false
		foundTo := false
		for _, arg := range args {
			if ts, ok := arg.(time.Time); ok {
				if ts.Equal(dateFrom) {
					foundFrom = true
				}
				if ts.Equal(dateTo) {
					foundTo = true
				}
			}
		}

		if !foundFrom {
			t.Error("date_from not found in args")
		}
		if !foundTo {
			t.Error("date_to not found in args")
		}
	})

	t.Run("with exclude IDs filter", func(t *testing.T) {
		filters := SearchFilters{
			TenantID:   "tenant-123",
			ExcludeIDs: []string{"doc-1", "doc-2"},
		}

		sql, args := engine.buildSearchQuery("test:*", filters, 10, 0)

		if !containsString(sql, "id != ALL") {
			t.Error("SQL should contain exclude IDs filter")
		}

		found := false
		for _, arg := range args {
			if ids, ok := arg.([]string); ok {
				if len(ids) == 2 && ids[0] == "doc-1" {
					found = true
					break
				}
			}
		}

		if !found {
			t.Error("exclude IDs not found in args")
		}
	})

	t.Run("with pagination", func(t *testing.T) {
		filters := SearchFilters{
			TenantID: "tenant-123",
		}

		sql, args := engine.buildSearchQuery("test:*", filters, 25, 50)

		if !containsString(sql, "LIMIT") {
			t.Error("SQL should contain LIMIT")
		}

		if !containsString(sql, "OFFSET") {
			t.Error("SQL should contain OFFSET")
		}

		// Check limit and offset are in args
		limitFound := false
		offsetFound := false
		for _, arg := range args {
			if v, ok := arg.(int); ok {
				if v == 25 {
					limitFound = true
				}
				if v == 50 {
					offsetFound = true
				}
			}
		}

		if !limitFound {
			t.Error("limit not found in args")
		}
		if !offsetFound {
			t.Error("offset not found in args")
		}
	})
}

func TestNormalizeScores(t *testing.T) {
	logger := testLogger()
	engine := NewBM25Engine(nil, logger, nil)

	t.Run("normalizes to 0-1 range", func(t *testing.T) {
		results := []SearchResult{
			{DocumentID: "1", RawScore: 1.0},
			{DocumentID: "2", RawScore: 0.5},
			{DocumentID: "3", RawScore: 0.1},
		}

		engine.normalizeScores(results, 1.0)

		for _, r := range results {
			if r.Score < 0 || r.Score > 1 {
				t.Errorf("score %f is outside 0-1 range", r.Score)
			}
		}

		// Highest raw score should have highest normalized score
		if results[0].Score <= results[1].Score {
			t.Error("first result should have highest score")
		}

		if results[1].Score <= results[2].Score {
			t.Error("second result should have higher score than third")
		}
	})

	t.Run("handles zero max score", func(t *testing.T) {
		results := []SearchResult{
			{DocumentID: "1", RawScore: 0},
			{DocumentID: "2", RawScore: 0},
		}

		engine.normalizeScores(results, 0)

		for _, r := range results {
			if r.Score != 0 {
				t.Errorf("expected score 0 for zero max, got %f", r.Score)
			}
		}
	})

	t.Run("handles empty results", func(t *testing.T) {
		results := []SearchResult{}
		// Should not panic
		engine.normalizeScores(results, 1.0)
	})
}

func TestParseHighlights(t *testing.T) {
	logger := testLogger()
	engine := NewBM25Engine(nil, logger, nil)

	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "single highlight",
			input:    "This is a <em>test</em> highlight",
			expected: 1,
		},
		{
			name:     "multiple highlights",
			input:    "First <em>match</em> ... Second <em>match</em> ... Third <em>match</em>",
			expected: 3,
		},
		{
			name:     "no highlights",
			input:    "No matches here",
			expected: 0,
		},
		{
			name:     "empty string",
			input:    "",
			expected: 0,
		},
		{
			name:     "fragment without em tag filtered",
			input:    "Has <em>tag</em> ... No tag here ... Another <em>tag</em>",
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.parseHighlights(tt.input)

			if len(result) != tt.expected {
				t.Errorf("parseHighlights(%q) returned %d items, expected %d: %v", tt.input, len(result), tt.expected, result)
			}
		})
	}
}

func TestSearchFilters(t *testing.T) {
	t.Run("empty filters", func(t *testing.T) {
		filters := SearchFilters{}

		if filters.TenantID != "" {
			t.Error("expected empty tenant_id")
		}

		if len(filters.ContentTypes) != 0 {
			t.Error("expected empty content types")
		}

		if filters.DateFrom != nil {
			t.Error("expected nil date_from")
		}
	})

	t.Run("populated filters", func(t *testing.T) {
		now := time.Now()
		filters := SearchFilters{
			TenantID:     "tenant-1",
			ContentTypes: []string{"email", "document"},
			DateFrom:     &now,
			DateTo:       &now,
			ExcludeIDs:   []string{"id-1", "id-2"},
			Tags:         []string{"important"},
			SourceIDs:    []string{"gmail"},
		}

		if filters.TenantID != "tenant-1" {
			t.Error("tenant_id not set correctly")
		}

		if len(filters.ContentTypes) != 2 {
			t.Error("content types not set correctly")
		}

		if filters.DateFrom == nil || filters.DateTo == nil {
			t.Error("date range not set correctly")
		}

		if len(filters.ExcludeIDs) != 2 {
			t.Error("exclude IDs not set correctly")
		}

		if len(filters.Tags) != 1 {
			t.Error("tags not set correctly")
		}

		if len(filters.SourceIDs) != 1 {
			t.Error("source IDs not set correctly")
		}
	})
}

func TestSearchResult(t *testing.T) {
	t.Run("result fields", func(t *testing.T) {
		title := "Test Title"
		now := time.Now()

		result := SearchResult{
			DocumentID:  "doc-123",
			ContentType: "email",
			SourceID:    "gmail-456",
			Title:       &title,
			Snippet:     "This is a test snippet",
			Highlights:  []string{"<em>test</em> highlight"},
			Score:       0.85,
			RawScore:    1.5,
			CreatedAt:   now,
			UpdatedAt:   now,
			Metadata:    map[string]string{"key": "value"},
		}

		if result.DocumentID != "doc-123" {
			t.Error("document_id not set correctly")
		}

		if result.ContentType != "email" {
			t.Error("content_type not set correctly")
		}

		if result.Title == nil || *result.Title != "Test Title" {
			t.Error("title not set correctly")
		}

		if result.Score != 0.85 {
			t.Error("score not set correctly")
		}

		if len(result.Metadata) != 1 {
			t.Error("metadata not set correctly")
		}
	})

	t.Run("nil title", func(t *testing.T) {
		result := SearchResult{
			DocumentID: "doc-123",
			Title:      nil,
		}

		if result.Title != nil {
			t.Error("expected nil title")
		}
	})
}

func TestSearchResponse(t *testing.T) {
	results := []SearchResult{
		{DocumentID: "1", Score: 0.9},
		{DocumentID: "2", Score: 0.8},
	}

	response := SearchResponse{
		Results:     results,
		TotalCount:  100,
		QueryTimeMs: 15.5,
	}

	if len(response.Results) != 2 {
		t.Error("results not set correctly")
	}

	if response.TotalCount != 100 {
		t.Error("total count not set correctly")
	}

	if response.QueryTimeMs != 15.5 {
		t.Error("query time not set correctly")
	}
}

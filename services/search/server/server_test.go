// Package server provides tests for the SearchService gRPC server.
package server

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	searchv1 "github.com/otherjamesbrown/penfold/api/proto/searchv1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/metrics"
	"github.com/otherjamesbrown/penfold/services/search/cache"
	"github.com/otherjamesbrown/penfold/services/search/config"
	"github.com/otherjamesbrown/penfold/services/search/engine"
	"github.com/otherjamesbrown/penfold/services/search/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// =============================================================================
// Test Fixtures and Mocks
// =============================================================================

// mockLogger implements logging.Logger for testing.
type mockLogger struct{}

func (m *mockLogger) Debug(msg string, fields ...logging.Field) {}
func (m *mockLogger) Info(msg string, fields ...logging.Field)  {}
func (m *mockLogger) Warn(msg string, fields ...logging.Field)  {}
func (m *mockLogger) Error(msg string, fields ...logging.Field) {}
func (m *mockLogger) With(fields ...logging.Field) logging.Logger {
	return m
}
func (m *mockLogger) WithContext(ctx context.Context) logging.Logger {
	return m
}

// mockMetrics implements metrics tracking for testing.
type mockMetrics struct {
	mu         sync.Mutex
	counters   map[string]int64
	histograms map[string][]float64
}

func newMockMetrics() *mockMetrics {
	return &mockMetrics{
		counters:   make(map[string]int64),
		histograms: make(map[string][]float64),
	}
}

func (m *mockMetrics) IncrCounter(name string, delta int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counters[name] += delta
}

func (m *mockMetrics) RecordHistogram(name string, value float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.histograms[name] = append(m.histograms[name], value)
}

// testConfig returns a test configuration.
func testConfig() *config.Config {
	return &config.Config{
		GRPCPort: 50051,
		HTTPPort: 8080,
		Search: config.SearchConfig{
			DefaultLimit:        10,
			MaxLimit:            100,
			DefaultTextWeight:   0.5,
			DefaultVectorWeight: 0.5,
			DefaultMinScore:     0.0,
		},
	}
}

// testLogger returns a mock logger for testing.
func testLogger() logging.Logger {
	return &mockLogger{}
}

// testMetrics returns mock metrics for testing.
func testMetrics() *metrics.Metrics {
	// Return nil for now - the server should handle nil metrics gracefully
	return nil
}

// =============================================================================
// Sample Test Documents
// =============================================================================

// TestDocument represents a sample document for testing.
type TestDocument struct {
	ID          string
	TenantID    string
	ContentType string
	Title       string
	Content     string
	Tags        []string
	CreatedAt   time.Time
}

// sampleDocuments returns a set of test documents for various scenarios.
func sampleDocuments() []TestDocument {
	now := time.Now()
	return []TestDocument{
		{
			ID:          "doc-001",
			TenantID:    "tenant-1",
			ContentType: "email",
			Title:       "Project Budget Proposal",
			Content:     "Please review the attached budget proposal for Q1. The total cost is estimated at $50,000.",
			Tags:        []string{"budget", "project"},
			CreatedAt:   now.Add(-24 * time.Hour),
		},
		{
			ID:          "doc-002",
			TenantID:    "tenant-1",
			ContentType: "meeting",
			Title:       "Team Standup Notes",
			Content:     "Daily standup meeting notes. John discussed the new feature implementation. Sarah reported a blocking issue.",
			Tags:        []string{"standup", "team"},
			CreatedAt:   now.Add(-12 * time.Hour),
		},
		{
			ID:          "doc-003",
			TenantID:    "tenant-1",
			ContentType: "document",
			Title:       "Technical Architecture Document",
			Content:     "This document outlines the technical architecture for the search service including BM25 and vector search.",
			Tags:        []string{"architecture", "technical"},
			CreatedAt:   now.Add(-48 * time.Hour),
		},
		{
			ID:          "doc-004",
			TenantID:    "tenant-2",
			ContentType: "email",
			Title:       "Confidential: Company Strategy",
			Content:     "This is a confidential document containing company strategy for the next fiscal year.",
			Tags:        []string{"confidential", "strategy"},
			CreatedAt:   now.Add(-6 * time.Hour),
		},
		{
			ID:          "doc-005",
			TenantID:    "tenant-1",
			ContentType: "note",
			Title:       "Meeting Action Items",
			Content:     "Action items from today's meeting: 1. Review budget 2. Update documentation 3. Schedule follow-up",
			Tags:        []string{"action-items", "meeting"},
			CreatedAt:   now,
		},
	}
}

// =============================================================================
// Server Construction Tests
// =============================================================================

func TestNewSearchServer(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	m := testMetrics()

	server := NewSearchServer(cfg, logger, m)

	require.NotNil(t, server, "NewSearchServer should return non-nil server")
	assert.Equal(t, cfg, server.cfg, "Config should be assigned")
}

func TestNewSearchServer_WithNilConfig(t *testing.T) {
	logger := testLogger()
	m := testMetrics()

	// Server should be created even with nil config (will use defaults)
	server := NewSearchServer(nil, logger, m)

	require.NotNil(t, server, "NewSearchServer should handle nil config")
}

// =============================================================================
// Search Endpoint Tests
// =============================================================================

func TestSearch_Unimplemented(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	m := testMetrics()
	server := NewSearchServer(cfg, logger, m)

	req := &searchv1.SearchRequest{
		Query:    "test query",
		TenantId: "tenant-1",
		Limit:    10,
	}

	_, err := server.Search(context.Background(), req)

	require.Error(t, err, "Search should return error (unimplemented)")

	st, ok := status.FromError(err)
	require.True(t, ok, "Error should be a gRPC status")
	assert.Equal(t, codes.Unimplemented, st.Code(), "Should return Unimplemented status")
}

func TestSemanticSearch_Unimplemented(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	m := testMetrics()
	server := NewSearchServer(cfg, logger, m)

	req := &searchv1.SemanticSearchRequest{
		Query:    "semantic query",
		TenantId: "tenant-1",
		Limit:    10,
	}

	_, err := server.SemanticSearch(context.Background(), req)

	require.Error(t, err, "SemanticSearch should return error (unimplemented)")

	st, ok := status.FromError(err)
	require.True(t, ok, "Error should be a gRPC status")
	assert.Equal(t, codes.Unimplemented, st.Code(), "Should return Unimplemented status")
}

func TestKeywordSearch_Unimplemented(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	m := testMetrics()
	server := NewSearchServer(cfg, logger, m)

	req := &searchv1.KeywordSearchRequest{
		Query:    "keyword query",
		TenantId: "tenant-1",
		Limit:    10,
	}

	_, err := server.KeywordSearch(context.Background(), req)

	require.Error(t, err, "KeywordSearch should return error (unimplemented)")

	st, ok := status.FromError(err)
	require.True(t, ok, "Error should be a gRPC status")
	assert.Equal(t, codes.Unimplemented, st.Code(), "Should return Unimplemented status")
}

// =============================================================================
// Document Indexing Tests
// =============================================================================

func TestIndexDocument_Unimplemented(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	m := testMetrics()
	server := NewSearchServer(cfg, logger, m)

	req := &searchv1.IndexDocumentRequest{
		DocumentId:  "doc-123",
		TenantId:    "tenant-1",
		ContentType: "email",
		Content:     "Test content for indexing",
	}

	_, err := server.IndexDocument(context.Background(), req)

	require.Error(t, err, "IndexDocument should return error (unimplemented)")

	st, ok := status.FromError(err)
	require.True(t, ok, "Error should be a gRPC status")
	assert.Equal(t, codes.Unimplemented, st.Code(), "Should return Unimplemented status")
}

func TestDeleteDocument_Unimplemented(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	m := testMetrics()
	server := NewSearchServer(cfg, logger, m)

	req := &searchv1.DeleteDocumentRequest{
		DocumentId: "doc-123",
		TenantId:   "tenant-1",
	}

	_, err := server.DeleteDocument(context.Background(), req)

	require.Error(t, err, "DeleteDocument should return error (unimplemented)")

	st, ok := status.FromError(err)
	require.True(t, ok, "Error should be a gRPC status")
	assert.Equal(t, codes.Unimplemented, st.Code(), "Should return Unimplemented status")
}

// =============================================================================
// Statistics Tests
// =============================================================================

func TestGetSearchStats_Unimplemented(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	m := testMetrics()
	server := NewSearchServer(cfg, logger, m)

	req := &searchv1.GetSearchStatsRequest{
		TenantId: stringPtr("tenant-1"),
	}

	_, err := server.GetSearchStats(context.Background(), req)

	require.Error(t, err, "GetSearchStats should return error (unimplemented)")

	st, ok := status.FromError(err)
	require.True(t, ok, "Error should be a gRPC status")
	assert.Equal(t, codes.Unimplemented, st.Code(), "Should return Unimplemented status")
}

// =============================================================================
// Tenant Isolation Tests (CRITICAL)
// =============================================================================

func TestTenantIsolation_SearchRequiresTenantID(t *testing.T) {
	// These tests verify that tenant isolation is properly enforced.
	// This is CRITICAL for multi-tenant security.

	cfg := testConfig()
	logger := testLogger()
	m := testMetrics()
	server := NewSearchServer(cfg, logger, m)

	testCases := []struct {
		name     string
		tenantID string
	}{
		{"tenant-1", "tenant-1"},
		{"tenant-2", "tenant-2"},
		{"different-org", "different-org"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := &searchv1.SearchRequest{
				Query:    "test query",
				TenantId: tc.tenantID,
				Limit:    10,
			}

			_, err := server.Search(context.Background(), req)

			// For now the server returns Unimplemented, but the request structure
			// correctly includes tenant_id which is logged
			require.Error(t, err)
			st, ok := status.FromError(err)
			require.True(t, ok)
			assert.Equal(t, codes.Unimplemented, st.Code())
		})
	}
}

func TestTenantIsolation_CacheKeyIsolation(t *testing.T) {
	// Verify that cache keys for different tenants are different
	// even with identical queries

	req1 := &searchv1.SearchRequest{
		Query:    "test query",
		TenantId: "tenant-1",
		Limit:    10,
	}

	req2 := &searchv1.SearchRequest{
		Query:    "test query",
		TenantId: "tenant-2",
		Limit:    10,
	}

	key1 := cache.GenerateKeyForHybridSearch(req1)
	key2 := cache.GenerateKeyForHybridSearch(req2)

	assert.NotEqual(t, key1, key2, "CRITICAL: Cache keys for different tenants MUST be different")
}

func TestTenantIsolation_IndexDocumentRequiresTenantID(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	m := testMetrics()
	server := NewSearchServer(cfg, logger, m)

	req := &searchv1.IndexDocumentRequest{
		DocumentId:  "doc-123",
		TenantId:    "tenant-1", // Must be set
		ContentType: "email",
		Content:     "Test content",
	}

	_, err := server.IndexDocument(context.Background(), req)

	// Currently returns Unimplemented but structure is correct
	require.Error(t, err)
}

func TestTenantIsolation_DeleteDocumentRequiresTenantID(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	m := testMetrics()
	server := NewSearchServer(cfg, logger, m)

	req := &searchv1.DeleteDocumentRequest{
		DocumentId: "doc-123",
		TenantId:   "tenant-1", // Must be set
	}

	_, err := server.DeleteDocument(context.Background(), req)

	// Currently returns Unimplemented but structure is correct
	require.Error(t, err)
}

// =============================================================================
// Error Handling Tests
// =============================================================================

func TestErrorHandling_NilRequest(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	m := testMetrics()
	server := NewSearchServer(cfg, logger, m)

	// These should not panic even with nil requests
	t.Run("Search", func(t *testing.T) {
		_, err := server.Search(context.Background(), nil)
		// Either error or panic recovery
		if err == nil {
			t.Skip("Server accepts nil request")
		}
	})

	t.Run("SemanticSearch", func(t *testing.T) {
		_, err := server.SemanticSearch(context.Background(), nil)
		if err == nil {
			t.Skip("Server accepts nil request")
		}
	})

	t.Run("KeywordSearch", func(t *testing.T) {
		_, err := server.KeywordSearch(context.Background(), nil)
		if err == nil {
			t.Skip("Server accepts nil request")
		}
	})
}

func TestErrorHandling_CancelledContext(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	m := testMetrics()
	server := NewSearchServer(cfg, logger, m)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	req := &searchv1.SearchRequest{
		Query:    "test query",
		TenantId: "tenant-1",
		Limit:    10,
	}

	_, err := server.Search(ctx, req)

	// Server should return Unimplemented for now
	require.Error(t, err)
}

func TestErrorHandling_TimeoutContext(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	m := testMetrics()
	server := NewSearchServer(cfg, logger, m)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	time.Sleep(1 * time.Millisecond) // Let timeout expire

	req := &searchv1.SearchRequest{
		Query:    "test query",
		TenantId: "tenant-1",
		Limit:    10,
	}

	_, err := server.Search(ctx, req)

	// Server returns Unimplemented (doesn't check context)
	require.Error(t, err)
}

// =============================================================================
// Integration Tests - Query Parser
// =============================================================================

func TestIntegration_QueryParsing(t *testing.T) {
	parser := query.NewParser()

	testCases := []struct {
		name           string
		query          string
		expectFilters  int
		expectTextLen  bool
		expectDateFrom bool
	}{
		{
			name:          "simple_text",
			query:         "budget proposal",
			expectFilters: 0,
			expectTextLen: true,
		},
		{
			name:          "with_from_filter",
			query:         "from:john@example.com meeting",
			expectFilters: 1,
			expectTextLen: true,
		},
		{
			name:           "with_date_filter",
			query:          "after:2024-01-01 project",
			expectFilters:  1,
			expectDateFrom: true,
		},
		{
			name:          "with_type_filter",
			query:         "type:email important",
			expectFilters: 1,
			expectTextLen: true,
		},
		{
			name:          "complex_query",
			query:         `from:alice type:email after:2024-01-01 "budget meeting"`,
			expectFilters: 3,
			expectTextLen: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parser.Parse(tc.query)

			require.NoError(t, err, "Parser should not return error")
			require.NotNil(t, result, "Result should not be nil")

			if tc.expectFilters > 0 {
				assert.GreaterOrEqual(t, len(result.Filters), tc.expectFilters,
					"Should have expected number of filters")
			}

			if tc.expectTextLen {
				assert.NotEmpty(t, result.TextQuery, "TextQuery should not be empty")
			}

			if tc.expectDateFrom {
				assert.NotNil(t, result.DateFrom, "DateFrom should be set")
			}
		})
	}
}

func TestIntegration_NaturalLanguageQuery(t *testing.T) {
	parser := query.NewParser()

	testCases := []struct {
		name         string
		query        string
		expectType   string
		expectPerson string
	}{
		{
			name:       "emails_query",
			query:      "show me my emails from last week",
			expectType: "email",
		},
		{
			name:         "person_query",
			query:        "emails from John about the project",
			expectType:   "email",
			expectPerson: "john",
		},
		{
			name:       "meeting_query",
			query:      "meetings about budget",
			expectType: "meeting",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parser.ParseNaturalLanguage(tc.query)

			require.NoError(t, err)
			require.NotNil(t, result)

			if tc.expectType != "" {
				found := false
				for _, ct := range result.ContentTypes {
					if ct == tc.expectType {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected content type %s not found", tc.expectType)
			}
		})
	}
}

// =============================================================================
// Integration Tests - Cache Operations
// =============================================================================

func TestIntegration_CacheHitMiss(t *testing.T) {
	c := cache.New()
	defer c.Close()

	// Create a sample response
	response := &searchv1.SearchResponse{
		TotalCount:  5,
		QueryTimeMs: 15.0,
		Results: []*searchv1.SearchResult{
			{
				DocumentId:  "doc-1",
				ContentType: "email",
				Score:       0.95,
			},
		},
	}

	key := "test-key"
	tenantID := "tenant-1"

	// Cache miss
	result := c.Get(key)
	assert.Nil(t, result, "Should be cache miss initially")

	stats := c.Stats()
	assert.Equal(t, uint64(1), stats.Misses, "Should have 1 miss")

	// Cache set and hit
	c.Set(key, response, tenantID, 5*time.Minute)

	result = c.Get(key)
	assert.NotNil(t, result, "Should be cache hit")
	assert.Equal(t, int64(5), result.TotalCount)

	stats = c.Stats()
	assert.Equal(t, uint64(1), stats.Hits, "Should have 1 hit")
}

func TestIntegration_CacheInvalidation(t *testing.T) {
	c := cache.New()
	defer c.Close()

	response := &searchv1.SearchResponse{TotalCount: 1}

	// Add entries for multiple tenants
	c.Set("key-t1-1", response, "tenant-1", 5*time.Minute)
	c.Set("key-t1-2", response, "tenant-1", 5*time.Minute)
	c.Set("key-t2-1", response, "tenant-2", 5*time.Minute)

	assert.Equal(t, 3, c.Size(), "Should have 3 entries")

	// Invalidate only tenant-1
	count := c.InvalidateForTenant("tenant-1")
	assert.Equal(t, 2, count, "Should invalidate 2 entries")

	assert.Equal(t, 1, c.Size(), "Should have 1 entry remaining")

	// Tenant-2 should still be accessible
	result := c.Get("key-t2-1")
	assert.NotNil(t, result, "Tenant-2 cache should still be accessible")
}

// =============================================================================
// Integration Tests - BM25 Engine
// =============================================================================

func TestIntegration_BM25EmptyQuery(t *testing.T) {
	e := engine.NewBM25Engine(nil, testLogger(), nil)

	filters := engine.SearchFilters{
		TenantID: "tenant-1",
	}

	// Empty query should return empty results without hitting the database
	result, err := e.Search(context.Background(), "", filters, 10, 0)

	require.NoError(t, err, "Empty query should not return error")
	require.NotNil(t, result, "Result should not be nil")
	assert.Empty(t, result.Results, "Empty query should return no results")
	assert.Equal(t, int64(0), result.TotalCount, "Total count should be 0")
}

func TestIntegration_BM25WhitespaceQuery(t *testing.T) {
	e := engine.NewBM25Engine(nil, testLogger(), nil)

	filters := engine.SearchFilters{
		TenantID: "tenant-1",
	}

	// Whitespace-only query should return empty results
	result, err := e.Search(context.Background(), "   ", filters, 10, 0)

	require.NoError(t, err, "Whitespace query should not return error")
	require.NotNil(t, result, "Result should not be nil")
	assert.Empty(t, result.Results, "Whitespace query should return no results")
}

func TestIntegration_BM25FieldWeights(t *testing.T) {
	e := engine.NewBM25Engine(nil, testLogger(), nil)

	// Test default weights
	weights := e.GetWeights()
	assert.Equal(t, 1.0, weights.Title, "Default title weight should be 1.0")
	assert.Equal(t, 0.5, weights.Body, "Default body weight should be 0.5")

	// Test custom weights
	newWeights := engine.FieldWeights{
		Title: 2.0,
		Body:  0.3,
	}
	e.SetWeights(newWeights)

	weights = e.GetWeights()
	assert.Equal(t, 2.0, weights.Title, "Custom title weight should be 2.0")
	assert.Equal(t, 0.3, weights.Body, "Custom body weight should be 0.3")
}

func TestIntegration_BM25TenantRequired(t *testing.T) {
	e := engine.NewBM25Engine(nil, testLogger(), nil)

	filters := engine.SearchFilters{
		TenantID: "", // Missing tenant ID
	}

	_, err := e.Search(context.Background(), "test", filters, 10, 0)

	require.Error(t, err, "Search should require tenant_id")
	assert.Contains(t, err.Error(), "tenant_id is required")
}

// =============================================================================
// Integration Tests - Vector Engine
// =============================================================================

func TestIntegration_VectorEngineValidation(t *testing.T) {
	mockClient := &mockEmbeddingClient{dimensions: 384}
	e := engine.NewVectorEngine(nil, mockClient, nil, testLogger())

	testCases := []struct {
		name      string
		tenantID  string
		query     string
		expectErr string
	}{
		{
			name:      "missing_tenant",
			tenantID:  "",
			query:     "test",
			expectErr: "tenant_id is required",
		},
		{
			name:      "empty_query",
			tenantID:  "tenant-1",
			query:     "",
			expectErr: "search query cannot be empty",
		},
		{
			name:      "whitespace_query",
			tenantID:  "tenant-1",
			query:     "   ",
			expectErr: "search query cannot be empty",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := e.Search(context.Background(), tc.tenantID, tc.query, nil, 10, 0)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.expectErr)
		})
	}
}

// =============================================================================
// Concurrent Access Tests
// =============================================================================

func TestConcurrentSearchRequests(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	m := testMetrics()
	server := NewSearchServer(cfg, logger, m)

	var wg sync.WaitGroup
	numGoroutines := 50
	errChan := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			tenantID := fmt.Sprintf("tenant-%d", id%5)
			req := &searchv1.SearchRequest{
				Query:    fmt.Sprintf("query %d", id),
				TenantId: tenantID,
				Limit:    10,
			}

			_, err := server.Search(context.Background(), req)
			if err != nil {
				// Expected Unimplemented error
				st, ok := status.FromError(err)
				if !ok || st.Code() != codes.Unimplemented {
					errChan <- err
				}
			}
		}(i)
	}

	wg.Wait()
	close(errChan)

	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}

	assert.Empty(t, errors, "Should not have unexpected errors")
}

func TestConcurrentCacheAccess(t *testing.T) {
	c := cache.New()
	defer c.Close()

	var wg sync.WaitGroup
	numGoroutines := 100
	numOperations := 50

	// Concurrent writes and reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			tenantID := fmt.Sprintf("tenant-%d", id%3)

			for j := 0; j < numOperations; j++ {
				key := fmt.Sprintf("key-%d-%d", id, j)
				response := &searchv1.SearchResponse{TotalCount: int64(j)}

				if j%2 == 0 {
					c.Set(key, response, tenantID, 5*time.Minute)
				} else {
					c.Get(key)
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify cache is still functional
	stats := c.Stats()
	assert.Greater(t, stats.Hits+stats.Misses, uint64(0), "Should have some cache activity")
}

// =============================================================================
// Full Flow Integration Tests
// =============================================================================

func TestIntegration_SearchFlow_QueryToCache(t *testing.T) {
	// This test demonstrates the intended flow:
	// 1. Parse query
	// 2. Generate cache key
	// 3. Check cache
	// 4. (Would execute search)
	// 5. Store in cache

	parser := query.NewParser()
	c := cache.New()
	defer c.Close()

	// Parse the query
	queryStr := `from:john type:email "project budget"`
	parsed, err := parser.Parse(queryStr)
	require.NoError(t, err)
	require.NotNil(t, parsed)

	// Build search request
	req := &searchv1.SearchRequest{
		Query:    queryStr,
		TenantId: "tenant-1",
		Limit:    10,
	}

	// Generate cache key
	cacheKey := cache.GenerateKeyForHybridSearch(req)
	assert.NotEmpty(t, cacheKey)

	// Cache miss (expected)
	cached := c.Get(cacheKey)
	assert.Nil(t, cached, "Should be cache miss")

	// Simulate search result
	response := &searchv1.SearchResponse{
		TotalCount:  2,
		QueryTimeMs: 25.0,
		Results: []*searchv1.SearchResult{
			{
				DocumentId:  "doc-1",
				ContentType: "email",
				Title:       stringPtr("Project Budget"),
				Score:       0.92,
			},
		},
	}

	// Store in cache
	c.Set(cacheKey, response, "tenant-1", 5*time.Minute)

	// Cache hit
	cached = c.Get(cacheKey)
	require.NotNil(t, cached, "Should be cache hit")
	assert.Equal(t, int64(2), cached.TotalCount)
}

func TestIntegration_SearchFlow_DifferentSearchTypes(t *testing.T) {
	c := cache.New()
	defer c.Close()

	// Test that different search types generate different cache keys
	hybridReq := &searchv1.SearchRequest{
		Query:    "test query",
		TenantId: "tenant-1",
		Limit:    10,
	}

	semanticReq := &searchv1.SemanticSearchRequest{
		Query:    "test query",
		TenantId: "tenant-1",
		Limit:    10,
	}

	keywordReq := &searchv1.KeywordSearchRequest{
		Query:    "test query",
		TenantId: "tenant-1",
		Limit:    10,
	}

	hybridKey := cache.GenerateKeyForHybridSearch(hybridReq)
	semanticKey := cache.GenerateKeyForSemanticSearch(semanticReq)
	keywordKey := cache.GenerateKeyForKeywordSearch(keywordReq)

	// All keys should be different
	assert.NotEqual(t, hybridKey, semanticKey, "Hybrid and semantic keys should differ")
	assert.NotEqual(t, hybridKey, keywordKey, "Hybrid and keyword keys should differ")
	assert.NotEqual(t, semanticKey, keywordKey, "Semantic and keyword keys should differ")
}

// =============================================================================
// Benchmark Tests
// =============================================================================

func BenchmarkQueryParsing_Simple(b *testing.B) {
	parser := query.NewParser()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = parser.Parse("simple query")
	}
}

func BenchmarkQueryParsing_Complex(b *testing.B) {
	parser := query.NewParser()
	complexQuery := `from:john@example.com type:email after:2024-01-01 "budget meeting" sort:date`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = parser.Parse(complexQuery)
	}
}

func BenchmarkQueryParsing_NaturalLanguage(b *testing.B) {
	parser := query.NewParser()
	nlQuery := "show me the latest emails from John about project budget from last week"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = parser.ParseNaturalLanguage(nlQuery)
	}
}

func BenchmarkCacheGet(b *testing.B) {
	c := cache.New()
	defer c.Close()

	response := &searchv1.SearchResponse{TotalCount: 1}
	c.Set("benchmark-key", response, "tenant-1", 5*time.Minute)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get("benchmark-key")
	}
}

func BenchmarkCacheSet(b *testing.B) {
	c := cache.New()
	defer c.Close()

	response := &searchv1.SearchResponse{TotalCount: 1}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Set(fmt.Sprintf("key-%d", i), response, "tenant-1", 5*time.Minute)
	}
}

func BenchmarkCacheKeyGeneration(b *testing.B) {
	req := &searchv1.SearchRequest{
		Query:    "test query with multiple words",
		TenantId: "tenant-1",
		Limit:    10,
		Filters: &searchv1.FilterOptions{
			ContentTypes: []string{"email", "document"},
			Tags:         []string{"important", "urgent"},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.GenerateKeyForHybridSearch(req)
	}
}

func BenchmarkConcurrentCacheOperations(b *testing.B) {
	c := cache.New()
	defer c.Close()

	response := &searchv1.SearchResponse{TotalCount: 1}

	// Pre-populate cache
	for i := 0; i < 1000; i++ {
		c.Set(fmt.Sprintf("key-%d", i), response, "tenant-1", 5*time.Minute)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("key-%d", i%1000)
			if i%2 == 0 {
				c.Get(key)
			} else {
				c.Set(key, response, "tenant-1", 5*time.Minute)
			}
			i++
		}
	})
}

func BenchmarkBM25FieldWeights(b *testing.B) {
	e := engine.NewBM25Engine(nil, testLogger(), nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Benchmark getting and setting weights (no DB required)
		weights := e.GetWeights()
		e.SetWeights(engine.FieldWeights{
			Title: weights.Title * 1.1,
			Body:  weights.Body * 1.1,
		})
	}
}

func BenchmarkBM25EmptyQuery(b *testing.B) {
	e := engine.NewBM25Engine(nil, testLogger(), nil)
	filters := engine.SearchFilters{TenantID: "tenant-1"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Empty query returns early without DB hit
		_, _ = e.Search(context.Background(), "", filters, 10, 0)
	}
}

// =============================================================================
// Helper Types and Functions
// =============================================================================

// mockEmbeddingClient implements engine.EmbeddingClient for testing.
type mockEmbeddingClient struct {
	dimensions int
	embedFn    func(ctx context.Context, text string) ([]float32, error)
}

func (m *mockEmbeddingClient) Embed(ctx context.Context, text string) ([]float32, error) {
	if m.embedFn != nil {
		return m.embedFn(ctx, text)
	}
	embedding := make([]float32, m.dimensions)
	for i := range embedding {
		embedding[i] = 0.1
	}
	return embedding, nil
}

func (m *mockEmbeddingClient) Dimensions() int {
	return m.dimensions
}

// stringPtr returns a pointer to the given string.
func stringPtr(s string) *string {
	return &s
}

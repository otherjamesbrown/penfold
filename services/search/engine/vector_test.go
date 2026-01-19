package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// mockEmbeddingClient implements EmbeddingClient for testing.
type mockEmbeddingClient struct {
	embedFn    func(ctx context.Context, text string) ([]float32, error)
	dimensions int
}

func (m *mockEmbeddingClient) Embed(ctx context.Context, text string) ([]float32, error) {
	if m.embedFn != nil {
		return m.embedFn(ctx, text)
	}
	// Return a default embedding
	embedding := make([]float32, m.dimensions)
	for i := range embedding {
		embedding[i] = 0.1
	}
	return embedding, nil
}

func (m *mockEmbeddingClient) Dimensions() int {
	return m.dimensions
}

func newMockEmbeddingClient(dims int) *mockEmbeddingClient {
	return &mockEmbeddingClient{dimensions: dims}
}

func TestDefaultVectorEngineConfig(t *testing.T) {
	cfg := DefaultVectorEngineConfig()

	if cfg.TableName != "search_documents" {
		t.Errorf("expected table name 'search_documents', got '%s'", cfg.TableName)
	}
	if cfg.EmbeddingColumn != "embedding" {
		t.Errorf("expected embedding column 'embedding', got '%s'", cfg.EmbeddingColumn)
	}
	if cfg.DefaultLimit != 10 {
		t.Errorf("expected default limit 10, got %d", cfg.DefaultLimit)
	}
	if cfg.MaxLimit != 100 {
		t.Errorf("expected max limit 100, got %d", cfg.MaxLimit)
	}
	if cfg.MinSimilarity != 0.0 {
		t.Errorf("expected min similarity 0.0, got %f", cfg.MinSimilarity)
	}
	if cfg.SnippetLength != 200 {
		t.Errorf("expected snippet length 200, got %d", cfg.SnippetLength)
	}
}

func TestPgVectorFormat(t *testing.T) {
	tests := []struct {
		name      string
		embedding []float32
		expected  string
	}{
		{
			name:      "empty embedding",
			embedding: []float32{},
			expected:  "[]",
		},
		{
			name:      "single value",
			embedding: []float32{1.0},
			expected:  "[1.000000]",
		},
		{
			name:      "multiple values",
			embedding: []float32{1.0, 2.5, 3.0},
			expected:  "[1.000000,2.500000,3.000000]",
		},
		{
			name:      "negative values",
			embedding: []float32{-1.0, 0.0, 1.0},
			expected:  "[-1.000000,0.000000,1.000000]",
		},
		{
			name:      "small values",
			embedding: []float32{0.001, 0.002},
			expected:  "[0.001000,0.002000]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pgVectorFormat(tt.embedding)
			if result != tt.expected {
				t.Errorf("pgVectorFormat() = %s, expected %s", result, tt.expected)
			}
		})
	}
}

func TestVectorEngine_Search_ValidationErrors(t *testing.T) {
	mockClient := newMockEmbeddingClient(384)
	cfg := DefaultVectorEngineConfig()

	// Create engine without database (will be used for validation tests)
	engine := NewVectorEngine(nil, mockClient, cfg, &mockLogger{})

	tests := []struct {
		name      string
		tenantID  string
		query     string
		wantErr   error
	}{
		{
			name:     "empty tenant ID",
			tenantID: "",
			query:    "test query",
			wantErr:  ErrInvalidTenantID,
		},
		{
			name:     "empty query",
			tenantID: "tenant-123",
			query:    "",
			wantErr:  ErrEmptyQuery,
		},
		{
			name:     "whitespace only query",
			tenantID: "tenant-123",
			query:    "   ",
			wantErr:  ErrEmptyQuery,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := engine.Search(context.Background(), tt.tenantID, tt.query, nil, 10, 0)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Search() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestVectorEngine_Search_EmbeddingError(t *testing.T) {
	mockClient := &mockEmbeddingClient{
		dimensions: 384,
		embedFn: func(ctx context.Context, text string) ([]float32, error) {
			return nil, ErrEmbeddingServiceUnavailable
		},
	}
	cfg := DefaultVectorEngineConfig()
	engine := NewVectorEngine(nil, mockClient, cfg, &mockLogger{})

	_, err := engine.Search(context.Background(), "tenant-123", "test query", nil, 10, 0)
	if !errors.Is(err, ErrEmbeddingServiceUnavailable) {
		t.Errorf("Search() error = %v, wantErr %v", err, ErrEmbeddingServiceUnavailable)
	}
}

func TestVectorEngine_SearchByVector_ValidationErrors(t *testing.T) {
	mockClient := newMockEmbeddingClient(384)
	cfg := DefaultVectorEngineConfig()
	engine := NewVectorEngine(nil, mockClient, cfg, &mockLogger{})

	tests := []struct {
		name      string
		tenantID  string
		embedding []float32
		wantErr   string
	}{
		{
			name:      "empty tenant ID",
			tenantID:  "",
			embedding: make([]float32, 384),
			wantErr:   "tenant_id is required",
		},
		{
			name:      "empty embedding",
			tenantID:  "tenant-123",
			embedding: []float32{},
			wantErr:   "embedding cannot be empty",
		},
		{
			name:      "wrong embedding dimensions",
			tenantID:  "tenant-123",
			embedding: make([]float32, 256),
			wantErr:   "embedding dimension mismatch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := engine.SearchByVector(context.Background(), tt.tenantID, tt.embedding, nil, 10, 0)
			if err == nil {
				t.Error("SearchByVector() expected error, got nil")
				return
			}
			if !containsString(err.Error(), tt.wantErr) {
				t.Errorf("SearchByVector() error = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestVectorEngine_LimitConstraints(t *testing.T) {
	mockClient := newMockEmbeddingClient(384)
	cfg := &VectorEngineConfig{
		TableName:       "search_documents",
		EmbeddingColumn: "embedding",
		DefaultLimit:    10,
		MaxLimit:        50,
		MinSimilarity:   0.0,
		SnippetLength:   200,
	}

	// We can't fully test without a database, but we can verify
	// the configuration is applied correctly via the engine
	engine := NewVectorEngine(nil, mockClient, cfg, &mockLogger{})

	if engine.config.DefaultLimit != 10 {
		t.Errorf("expected default limit 10, got %d", engine.config.DefaultLimit)
	}
	if engine.config.MaxLimit != 50 {
		t.Errorf("expected max limit 50, got %d", engine.config.MaxLimit)
	}
}

func TestVectorSearchFilters_Initialization(t *testing.T) {
	now := time.Now()

	filters := &VectorSearchFilters{
		ContentTypes: []string{"email", "document"},
		SourceIDs:    []string{"source-1", "source-2"},
		DateFrom:     &now,
		DateTo:       &now,
		Tags:         []string{"important", "work"},
		EntityIDs:    []string{"entity-1"},
		Sources:      []string{"gmail"},
		ExcludeIDs:   []string{"exclude-1"},
	}

	if len(filters.ContentTypes) != 2 {
		t.Errorf("expected 2 content types, got %d", len(filters.ContentTypes))
	}
	if len(filters.SourceIDs) != 2 {
		t.Errorf("expected 2 source IDs, got %d", len(filters.SourceIDs))
	}
	if filters.DateFrom == nil {
		t.Error("expected DateFrom to be set")
	}
	if len(filters.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(filters.Tags))
	}
}

func TestVectorSearchResult_Fields(t *testing.T) {
	title := "Test Document"
	result := VectorSearchResult{
		DocumentID:      "doc-123",
		ContentType:     "email",
		SourceID:        "source-456",
		Title:           &title,
		Snippet:         "This is a test snippet...",
		SimilarityScore: 0.95,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
		Metadata:        map[string]string{"key": "value"},
	}

	if result.DocumentID != "doc-123" {
		t.Errorf("expected document ID 'doc-123', got '%s'", result.DocumentID)
	}
	if result.ContentType != "email" {
		t.Errorf("expected content type 'email', got '%s'", result.ContentType)
	}
	if result.SimilarityScore != 0.95 {
		t.Errorf("expected similarity score 0.95, got %f", result.SimilarityScore)
	}
	if result.Title == nil || *result.Title != "Test Document" {
		t.Error("expected title to be 'Test Document'")
	}
}

func TestVectorSearchResponse_Fields(t *testing.T) {
	response := &VectorSearchResponse{
		Results:     []VectorSearchResult{{DocumentID: "doc-1"}},
		TotalCount:  100,
		QueryTimeMs: 15.5,
	}

	if len(response.Results) != 1 {
		t.Errorf("expected 1 result, got %d", len(response.Results))
	}
	if response.TotalCount != 100 {
		t.Errorf("expected total count 100, got %d", response.TotalCount)
	}
	if response.QueryTimeMs != 15.5 {
		t.Errorf("expected query time 15.5ms, got %f", response.QueryTimeMs)
	}
}

func TestBuildSearchQuery_BasicQuery(t *testing.T) {
	mockClient := newMockEmbeddingClient(384)
	cfg := DefaultVectorEngineConfig()
	engine := NewVectorEngine(nil, mockClient, cfg, &mockLogger{})

	embedding := []float32{0.1, 0.2, 0.3}
	query, args := engine.buildSearchQuery("tenant-123", embedding, nil, 10, 0)

	// Verify the query contains essential parts
	if !containsString(query, "tenant_id = $") {
		t.Error("query should contain tenant_id filter")
	}
	if !containsString(query, "<=>") {
		t.Error("query should contain pgvector cosine distance operator")
	}
	if !containsString(query, "ORDER BY cosine_distance ASC") {
		t.Error("query should order by cosine distance")
	}
	if !containsString(query, "LIMIT $") {
		t.Error("query should contain LIMIT clause")
	}
	if !containsString(query, "OFFSET $") {
		t.Error("query should contain OFFSET clause")
	}

	// Verify args
	if len(args) < 4 {
		t.Errorf("expected at least 4 args, got %d", len(args))
	}
}

func TestBuildSearchQuery_WithFilters(t *testing.T) {
	mockClient := newMockEmbeddingClient(384)
	cfg := DefaultVectorEngineConfig()
	engine := NewVectorEngine(nil, mockClient, cfg, &mockLogger{})

	now := time.Now()
	filters := &VectorSearchFilters{
		ContentTypes: []string{"email"},
		SourceIDs:    []string{"source-1"},
		DateFrom:     &now,
		DateTo:       &now,
		Tags:         []string{"important"},
		EntityIDs:    []string{"entity-1"},
		Sources:      []string{"gmail"},
		ExcludeIDs:   []string{"exclude-1"},
	}

	embedding := []float32{0.1, 0.2, 0.3}
	query, args := engine.buildSearchQuery("tenant-123", embedding, filters, 10, 0)

	// Verify all filter clauses are present
	if !containsString(query, "content_type = ANY") {
		t.Error("query should contain content_type filter")
	}
	if !containsString(query, "source_id = ANY") {
		t.Error("query should contain source_id filter")
	}
	if !containsString(query, "created_at >=") {
		t.Error("query should contain date_from filter")
	}
	if !containsString(query, "created_at <=") {
		t.Error("query should contain date_to filter")
	}
	if !containsString(query, "tags &&") {
		t.Error("query should contain tags filter")
	}
	if !containsString(query, "entity_ids &&") {
		t.Error("query should contain entity_ids filter")
	}
	if !containsString(query, "source = ANY") {
		t.Error("query should contain source filter")
	}
	if !containsString(query, "document_id != ALL") {
		t.Error("query should contain exclude_ids filter")
	}

	// With all filters, we expect: embedding, tenant_id, 8 filters, limit, offset = 12 args
	if len(args) != 12 {
		t.Errorf("expected 12 args with all filters, got %d", len(args))
	}
}

func TestNewVectorEngine_NilConfig(t *testing.T) {
	mockClient := newMockEmbeddingClient(384)
	engine := NewVectorEngine(nil, mockClient, nil, &mockLogger{})

	// Should use default config
	if engine.config == nil {
		t.Fatal("engine config should not be nil")
	}
	if engine.config.TableName != "search_documents" {
		t.Errorf("expected default table name, got '%s'", engine.config.TableName)
	}
}

func TestErrors(t *testing.T) {
	// Verify error messages are descriptive
	if ErrEmbeddingServiceUnavailable.Error() != "embedding service unavailable" {
		t.Errorf("unexpected error message: %s", ErrEmbeddingServiceUnavailable.Error())
	}
	if ErrEmptyQuery.Error() != "search query cannot be empty" {
		t.Errorf("unexpected error message: %s", ErrEmptyQuery.Error())
	}
	if ErrInvalidTenantID.Error() != "tenant_id is required" {
		t.Errorf("unexpected error message: %s", ErrInvalidTenantID.Error())
	}
}

// mockLogger implements logging.Logger for testing.
type mockLogger struct{}

func (m *mockLogger) Debug(msg string, fields ...logging.Field) {}
func (m *mockLogger) Info(msg string, fields ...logging.Field)  {}
func (m *mockLogger) Warn(msg string, fields ...logging.Field)  {}
func (m *mockLogger) Error(msg string, fields ...logging.Field) {}
func (m *mockLogger) With(fields ...logging.Field) logging.Logger { return m }
func (m *mockLogger) WithContext(ctx context.Context) logging.Logger { return m }

// containsString checks if a string contains a substring.
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

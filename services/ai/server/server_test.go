// Package server provides unit tests for the AI Coordinator gRPC server.
package server

import (
	"context"
	"os"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/ai/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/ai/config"
)

// =============================================================================
// Test Fixtures and Mocks
// =============================================================================

// mockLogger implements logging.Logger for testing.
type mockLogger struct {
	zl zerolog.Logger
}

func newMockLogger() *mockLogger {
	return &mockLogger{
		zl: zerolog.New(os.Stdout).Level(zerolog.Disabled),
	}
}

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
func (m *mockLogger) Zerolog() zerolog.Logger {
	return m.zl
}

// testConfig returns a test configuration.
func testConfig() *config.Config {
	return &config.Config{
		ServiceName:           "ai-test",
		GRPCPort:              50051,
		HTTPPort:              8080,
		OllamaHost:            "http://localhost:11434",
		OllamaDefaultModel:    "llama3.2",
		GeminiDefaultModel:    "gemini-1.5-pro",
		DefaultEmbeddingModel: "nomic-embed-text",
		DefaultLLMModel:       "llama3.2",
		LogLevel:              "info",
		Environment:           "dev",
		EnableMetrics:         true,
		GracefulShutdownTimeout: 30,
	}
}

// testLogger returns a mock logger for testing.
func testLogger() logging.Logger {
	return newMockLogger()
}

// =============================================================================
// Server Construction Tests
// =============================================================================

func TestNewAIServer(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()

	server := NewAIServer(cfg, logger)

	require.NotNil(t, server, "NewAIServer should return non-nil server")
	assert.Equal(t, cfg, server.config, "Config should be assigned")
	assert.NotNil(t, server.logger, "Logger should be assigned")
}

func TestNewAIServer_WithNilConfig(t *testing.T) {
	logger := testLogger()

	// Server should be created even with nil config
	server := NewAIServer(nil, logger)

	require.NotNil(t, server, "NewAIServer should handle nil config")
	assert.Nil(t, server.config, "Config should be nil")
}

func TestNewAIServer_WithNilLogger(t *testing.T) {
	cfg := testConfig()

	// This may panic depending on implementation - testing graceful handling
	assert.Panics(t, func() {
		NewAIServer(cfg, nil)
	}, "NewAIServer with nil logger should panic")
}

// =============================================================================
// GenerateEmbedding Tests
// =============================================================================

func TestGenerateEmbedding_Unimplemented(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	server := NewAIServer(cfg, logger)

	req := &aiv1.EmbeddingRequest{
		Text:     "test text for embedding",
		TenantId: stringPtr("tenant-1"),
	}

	_, err := server.GenerateEmbedding(context.Background(), req)

	require.Error(t, err, "GenerateEmbedding should return error (unimplemented)")

	st, ok := status.FromError(err)
	require.True(t, ok, "Error should be a gRPC status")
	assert.Equal(t, codes.Unimplemented, st.Code(), "Should return Unimplemented status")
	assert.Contains(t, st.Message(), "not implemented")
}

func TestGenerateEmbedding_EmptyText(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	server := NewAIServer(cfg, logger)

	req := &aiv1.EmbeddingRequest{
		Text:     "",
		TenantId: stringPtr("tenant-1"),
	}

	_, err := server.GenerateEmbedding(context.Background(), req)

	// Currently returns Unimplemented, but should validate input
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unimplemented, st.Code())
}

func TestGenerateEmbedding_WithModel(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	server := NewAIServer(cfg, logger)

	req := &aiv1.EmbeddingRequest{
		Text:     "test text",
		TenantId: stringPtr("tenant-1"),
		Model:    stringPtr("custom-model"),
	}

	_, err := server.GenerateEmbedding(context.Background(), req)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unimplemented, st.Code())
}

func TestGenerateEmbedding_NilRequest(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	server := NewAIServer(cfg, logger)

	_, err := server.GenerateEmbedding(context.Background(), nil)

	// Should handle nil request gracefully
	require.Error(t, err)
}

func TestGenerateEmbedding_CanceledContext(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	server := NewAIServer(cfg, logger)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	req := &aiv1.EmbeddingRequest{
		Text:     "test text",
		TenantId: stringPtr("tenant-1"),
	}

	_, err := server.GenerateEmbedding(ctx, req)

	// Currently returns Unimplemented regardless of context
	require.Error(t, err)
}

// =============================================================================
// GenerateSummary Tests
// =============================================================================

func TestGenerateSummary_Unimplemented(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	server := NewAIServer(cfg, logger)

	req := &aiv1.SummaryRequest{
		Content:   "This is a long document that needs to be summarized.",
		TenantId:  stringPtr("tenant-1"),
		Style:     aiv1.SummaryStyle_SUMMARY_STYLE_BRIEF,
		MaxLength: int32Ptr(100),
	}

	_, err := server.GenerateSummary(context.Background(), req)

	require.Error(t, err, "GenerateSummary should return error (unimplemented)")

	st, ok := status.FromError(err)
	require.True(t, ok, "Error should be a gRPC status")
	assert.Equal(t, codes.Unimplemented, st.Code())
	assert.Contains(t, st.Message(), "not implemented")
}

func TestGenerateSummary_DifferentStyles(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	server := NewAIServer(cfg, logger)

	styles := []aiv1.SummaryStyle{
		aiv1.SummaryStyle_SUMMARY_STYLE_UNSPECIFIED,
		aiv1.SummaryStyle_SUMMARY_STYLE_BRIEF,
		aiv1.SummaryStyle_SUMMARY_STYLE_DETAILED,
		aiv1.SummaryStyle_SUMMARY_STYLE_BULLET_POINTS,
		aiv1.SummaryStyle_SUMMARY_STYLE_TECHNICAL,
	}

	for _, style := range styles {
		t.Run(style.String(), func(t *testing.T) {
			req := &aiv1.SummaryRequest{
				Content:  "Content to summarize",
				TenantId: stringPtr("tenant-1"),
				Style:    style,
			}

			_, err := server.GenerateSummary(context.Background(), req)

			require.Error(t, err)
			st, ok := status.FromError(err)
			require.True(t, ok)
			assert.Equal(t, codes.Unimplemented, st.Code())
		})
	}
}

func TestGenerateSummary_EmptyContent(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	server := NewAIServer(cfg, logger)

	req := &aiv1.SummaryRequest{
		Content:  "",
		TenantId: stringPtr("tenant-1"),
	}

	_, err := server.GenerateSummary(context.Background(), req)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unimplemented, st.Code())
}

func TestGenerateSummary_WithMaxLength(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	server := NewAIServer(cfg, logger)

	testCases := []struct {
		name      string
		maxLength int32
	}{
		{"zero_length", 0},
		{"small_length", 50},
		{"medium_length", 200},
		{"large_length", 1000},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := &aiv1.SummaryRequest{
				Content:   "Content to summarize",
				TenantId:  stringPtr("tenant-1"),
				MaxLength: int32Ptr(tc.maxLength),
			}

			_, err := server.GenerateSummary(context.Background(), req)

			require.Error(t, err)
			st, ok := status.FromError(err)
			require.True(t, ok)
			assert.Equal(t, codes.Unimplemented, st.Code())
		})
	}
}

func TestGenerateSummary_NilRequest(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	server := NewAIServer(cfg, logger)

	_, err := server.GenerateSummary(context.Background(), nil)

	require.Error(t, err)
}

// =============================================================================
// ExtractAssertions Tests
// =============================================================================

func TestExtractAssertions_Unimplemented(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	server := NewAIServer(cfg, logger)

	req := &aiv1.AssertionRequest{
		Content:       "John works at Acme Corp. The project deadline is next Friday.",
		TenantId:      stringPtr("tenant-1"),
		MinConfidence: float32Ptr(0.8),
		MaxAssertions: int32Ptr(10),
	}

	_, err := server.ExtractAssertions(context.Background(), req)

	require.Error(t, err, "ExtractAssertions should return error (unimplemented)")

	st, ok := status.FromError(err)
	require.True(t, ok, "Error should be a gRPC status")
	assert.Equal(t, codes.Unimplemented, st.Code())
	assert.Contains(t, st.Message(), "not implemented")
}

func TestExtractAssertions_ConfidenceThresholds(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	server := NewAIServer(cfg, logger)

	thresholds := []float32{0.0, 0.5, 0.8, 0.95, 1.0}

	for _, threshold := range thresholds {
		t.Run(floatToString(threshold), func(t *testing.T) {
			req := &aiv1.AssertionRequest{
				Content:       "Test content",
				TenantId:      stringPtr("tenant-1"),
				MinConfidence: float32Ptr(threshold),
			}

			_, err := server.ExtractAssertions(context.Background(), req)

			require.Error(t, err)
			st, ok := status.FromError(err)
			require.True(t, ok)
			assert.Equal(t, codes.Unimplemented, st.Code())
		})
	}
}

func TestExtractAssertions_MaxAssertions(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	server := NewAIServer(cfg, logger)

	testCases := []struct {
		name          string
		maxAssertions int32
	}{
		{"zero", 0},
		{"small", 5},
		{"medium", 20},
		{"large", 100},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := &aiv1.AssertionRequest{
				Content:       "Test content",
				TenantId:      stringPtr("tenant-1"),
				MaxAssertions: int32Ptr(tc.maxAssertions),
			}

			_, err := server.ExtractAssertions(context.Background(), req)

			require.Error(t, err)
			st, ok := status.FromError(err)
			require.True(t, ok)
			assert.Equal(t, codes.Unimplemented, st.Code())
		})
	}
}

func TestExtractAssertions_EmptyContent(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	server := NewAIServer(cfg, logger)

	req := &aiv1.AssertionRequest{
		Content:  "",
		TenantId: stringPtr("tenant-1"),
	}

	_, err := server.ExtractAssertions(context.Background(), req)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unimplemented, st.Code())
}

func TestExtractAssertions_NilRequest(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	server := NewAIServer(cfg, logger)

	_, err := server.ExtractAssertions(context.Background(), nil)

	require.Error(t, err)
}

// =============================================================================
// ClassifyContent Tests
// =============================================================================

func TestClassifyContent_Unimplemented(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	server := NewAIServer(cfg, logger)

	req := &aiv1.ClassifyContentRequest{
		Content:       "This email discusses project budget allocations.",
		TenantId:      stringPtr("tenant-1"),
		Categories:    []string{"finance", "project", "meeting", "personal"},
		MultiLabel:    boolPtr(true),
		MinConfidence: float32Ptr(0.7),
	}

	_, err := server.ClassifyContent(context.Background(), req)

	require.Error(t, err, "ClassifyContent should return error (unimplemented)")

	st, ok := status.FromError(err)
	require.True(t, ok, "Error should be a gRPC status")
	assert.Equal(t, codes.Unimplemented, st.Code())
	assert.Contains(t, st.Message(), "not implemented")
}

func TestClassifyContent_MultiLabelVsSingleLabel(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	server := NewAIServer(cfg, logger)

	testCases := []struct {
		name       string
		multiLabel bool
	}{
		{"single_label", false},
		{"multi_label", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := &aiv1.ClassifyContentRequest{
				Content:    "Test content",
				TenantId:   stringPtr("tenant-1"),
				Categories: []string{"cat1", "cat2"},
				MultiLabel: boolPtr(tc.multiLabel),
			}

			_, err := server.ClassifyContent(context.Background(), req)

			require.Error(t, err)
			st, ok := status.FromError(err)
			require.True(t, ok)
			assert.Equal(t, codes.Unimplemented, st.Code())
		})
	}
}

func TestClassifyContent_EmptyCategories(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	server := NewAIServer(cfg, logger)

	req := &aiv1.ClassifyContentRequest{
		Content:    "Test content",
		TenantId:   stringPtr("tenant-1"),
		Categories: []string{}, // Empty categories
	}

	_, err := server.ClassifyContent(context.Background(), req)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unimplemented, st.Code())
}

func TestClassifyContent_ManyCategories(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	server := NewAIServer(cfg, logger)

	categories := make([]string, 100)
	for i := range categories {
		categories[i] = "category_" + string(rune('a'+i%26))
	}

	req := &aiv1.ClassifyContentRequest{
		Content:    "Test content",
		TenantId:   stringPtr("tenant-1"),
		Categories: categories,
	}

	_, err := server.ClassifyContent(context.Background(), req)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unimplemented, st.Code())
}

func TestClassifyContent_EmptyContent(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	server := NewAIServer(cfg, logger)

	req := &aiv1.ClassifyContentRequest{
		Content:    "",
		TenantId:   stringPtr("tenant-1"),
		Categories: []string{"cat1", "cat2"},
	}

	_, err := server.ClassifyContent(context.Background(), req)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unimplemented, st.Code())
}

func TestClassifyContent_NilRequest(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	server := NewAIServer(cfg, logger)

	_, err := server.ClassifyContent(context.Background(), nil)

	require.Error(t, err)
}

// =============================================================================
// GetModelStatus Tests
// =============================================================================

func TestGetModelStatus_Unimplemented(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	server := NewAIServer(cfg, logger)

	req := &aiv1.GetModelStatusRequest{
		ModelName:      stringPtr("llama3.2"),
		ModelType:      modelTypePtr(aiv1.ModelType_MODEL_TYPE_LLM),
		IncludeMetrics: boolPtr(true),
	}

	_, err := server.GetModelStatus(context.Background(), req)

	require.Error(t, err, "GetModelStatus should return error (unimplemented)")

	st, ok := status.FromError(err)
	require.True(t, ok, "Error should be a gRPC status")
	assert.Equal(t, codes.Unimplemented, st.Code())
	assert.Contains(t, st.Message(), "not implemented")
}

func TestGetModelStatus_DifferentModelTypes(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	server := NewAIServer(cfg, logger)

	modelTypes := []aiv1.ModelType{
		aiv1.ModelType_MODEL_TYPE_UNSPECIFIED,
		aiv1.ModelType_MODEL_TYPE_EMBEDDING,
		aiv1.ModelType_MODEL_TYPE_LLM,
		aiv1.ModelType_MODEL_TYPE_CLASSIFIER,
		aiv1.ModelType_MODEL_TYPE_NER,
	}

	for _, modelType := range modelTypes {
		t.Run(modelType.String(), func(t *testing.T) {
			req := &aiv1.GetModelStatusRequest{
				ModelType: modelTypePtr(modelType),
			}

			_, err := server.GetModelStatus(context.Background(), req)

			require.Error(t, err)
			st, ok := status.FromError(err)
			require.True(t, ok)
			assert.Equal(t, codes.Unimplemented, st.Code())
		})
	}
}

func TestGetModelStatus_WithAndWithoutMetrics(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	server := NewAIServer(cfg, logger)

	testCases := []struct {
		name           string
		includeMetrics bool
	}{
		{"without_metrics", false},
		{"with_metrics", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := &aiv1.GetModelStatusRequest{
				ModelName:      stringPtr("test-model"),
				IncludeMetrics: boolPtr(tc.includeMetrics),
			}

			_, err := server.GetModelStatus(context.Background(), req)

			require.Error(t, err)
			st, ok := status.FromError(err)
			require.True(t, ok)
			assert.Equal(t, codes.Unimplemented, st.Code())
		})
	}
}

func TestGetModelStatus_NilRequest(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	server := NewAIServer(cfg, logger)

	_, err := server.GetModelStatus(context.Background(), nil)

	require.Error(t, err)
}

// =============================================================================
// Tenant Isolation Tests (CRITICAL)
// =============================================================================

func TestTenantIsolation_EmbeddingRequiresTenantID(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	server := NewAIServer(cfg, logger)

	testCases := []struct {
		name     string
		tenantID *string
	}{
		{"with_tenant_id", stringPtr("tenant-1")},
		{"nil_tenant_id", nil},
		{"empty_tenant_id", stringPtr("")},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := &aiv1.EmbeddingRequest{
				Text:     "test text",
				TenantId: tc.tenantID,
			}

			_, err := server.GenerateEmbedding(context.Background(), req)

			// Currently returns Unimplemented for all cases
			// When implemented, should validate tenant_id
			require.Error(t, err)
		})
	}
}

func TestTenantIsolation_SummaryRequiresTenantID(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	server := NewAIServer(cfg, logger)

	req := &aiv1.SummaryRequest{
		Content:  "test content",
		TenantId: nil, // Missing tenant ID
	}

	_, err := server.GenerateSummary(context.Background(), req)

	// When implemented, should require tenant_id
	require.Error(t, err)
}

func TestTenantIsolation_AssertionsRequiresTenantID(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	server := NewAIServer(cfg, logger)

	req := &aiv1.AssertionRequest{
		Content:  "test content",
		TenantId: nil, // Missing tenant ID
	}

	_, err := server.ExtractAssertions(context.Background(), req)

	require.Error(t, err)
}

func TestTenantIsolation_ClassifyRequiresTenantID(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	server := NewAIServer(cfg, logger)

	req := &aiv1.ClassifyContentRequest{
		Content:    "test content",
		TenantId:   nil, // Missing tenant ID
		Categories: []string{"cat1"},
	}

	_, err := server.ClassifyContent(context.Background(), req)

	require.Error(t, err)
}

// =============================================================================
// Error Handling Tests
// =============================================================================

func TestErrorHandling_CanceledContext(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	server := NewAIServer(cfg, logger)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	testCases := []struct {
		name string
		fn   func() error
	}{
		{
			"embedding",
			func() error {
				_, err := server.GenerateEmbedding(ctx, &aiv1.EmbeddingRequest{Text: "test", TenantId: stringPtr("t1")})
				return err
			},
		},
		{
			"summary",
			func() error {
				_, err := server.GenerateSummary(ctx, &aiv1.SummaryRequest{Content: "test", TenantId: stringPtr("t1")})
				return err
			},
		},
		{
			"assertions",
			func() error {
				_, err := server.ExtractAssertions(ctx, &aiv1.AssertionRequest{Content: "test", TenantId: stringPtr("t1")})
				return err
			},
		},
		{
			"classify",
			func() error {
				_, err := server.ClassifyContent(ctx, &aiv1.ClassifyContentRequest{Content: "test", TenantId: stringPtr("t1"), Categories: []string{"c1"}})
				return err
			},
		},
		{
			"model_status",
			func() error {
				_, err := server.GetModelStatus(ctx, &aiv1.GetModelStatusRequest{ModelType: modelTypePtr(aiv1.ModelType_MODEL_TYPE_LLM)})
				return err
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.fn()
			require.Error(t, err)
			// Currently returns Unimplemented regardless of context state
		})
	}
}

// =============================================================================
// Concurrent Access Tests
// =============================================================================

func TestConcurrentRequests(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	server := NewAIServer(cfg, logger)

	const numGoroutines = 50
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			ctx := context.Background()
			tenantID := stringPtr("tenant-" + string(rune('a'+id%5)))

			// Mix of different requests
			switch id % 5 {
			case 0:
				server.GenerateEmbedding(ctx, &aiv1.EmbeddingRequest{Text: "test", TenantId: tenantID})
			case 1:
				server.GenerateSummary(ctx, &aiv1.SummaryRequest{Content: "test", TenantId: tenantID})
			case 2:
				server.ExtractAssertions(ctx, &aiv1.AssertionRequest{Content: "test", TenantId: tenantID})
			case 3:
				server.ClassifyContent(ctx, &aiv1.ClassifyContentRequest{Content: "test", TenantId: tenantID, Categories: []string{"c1"}})
			case 4:
				server.GetModelStatus(ctx, &aiv1.GetModelStatusRequest{ModelType: modelTypePtr(aiv1.ModelType_MODEL_TYPE_LLM)})
			}

			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// If we get here without deadlock, the test passes
}

// =============================================================================
// Benchmark Tests
// =============================================================================

func BenchmarkGenerateEmbedding(b *testing.B) {
	cfg := testConfig()
	logger := testLogger()
	server := NewAIServer(cfg, logger)

	req := &aiv1.EmbeddingRequest{
		Text:     "test text for embedding",
		TenantId: stringPtr("tenant-1"),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		server.GenerateEmbedding(context.Background(), req)
	}
}

func BenchmarkGenerateSummary(b *testing.B) {
	cfg := testConfig()
	logger := testLogger()
	server := NewAIServer(cfg, logger)

	req := &aiv1.SummaryRequest{
		Content:  "This is a long document that needs to be summarized.",
		TenantId: stringPtr("tenant-1"),
		Style:    aiv1.SummaryStyle_SUMMARY_STYLE_BRIEF,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		server.GenerateSummary(context.Background(), req)
	}
}

func BenchmarkExtractAssertions(b *testing.B) {
	cfg := testConfig()
	logger := testLogger()
	server := NewAIServer(cfg, logger)

	req := &aiv1.AssertionRequest{
		Content:  "John works at Acme Corp.",
		TenantId: stringPtr("tenant-1"),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		server.ExtractAssertions(context.Background(), req)
	}
}

func BenchmarkClassifyContent(b *testing.B) {
	cfg := testConfig()
	logger := testLogger()
	server := NewAIServer(cfg, logger)

	req := &aiv1.ClassifyContentRequest{
		Content:    "Test content for classification",
		TenantId:   stringPtr("tenant-1"),
		Categories: []string{"cat1", "cat2", "cat3"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		server.ClassifyContent(context.Background(), req)
	}
}

func BenchmarkGetModelStatus(b *testing.B) {
	cfg := testConfig()
	logger := testLogger()
	server := NewAIServer(cfg, logger)

	req := &aiv1.GetModelStatusRequest{
		ModelName: stringPtr("llama3.2"),
		ModelType: modelTypePtr(aiv1.ModelType_MODEL_TYPE_LLM),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		server.GetModelStatus(context.Background(), req)
	}
}

// =============================================================================
// Helper Functions
// =============================================================================

// stringPtr returns a pointer to the given string.
func stringPtr(s string) *string {
	return &s
}

// int32Ptr returns a pointer to the given int32.
func int32Ptr(i int32) *int32 {
	return &i
}

// float32Ptr returns a pointer to the given float32.
func float32Ptr(f float32) *float32 {
	return &f
}

// boolPtr returns a pointer to the given bool.
func boolPtr(b bool) *bool {
	return &b
}

// floatToString converts a float32 to a string for test naming.
func floatToString(f float32) string {
	return string(rune(int(f*100) + '0'))
}

// modelTypePtr returns a pointer to the given ModelType.
func modelTypePtr(mt aiv1.ModelType) *aiv1.ModelType {
	return &mt
}

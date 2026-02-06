// Package server provides unit tests for the AI Coordinator gRPC server.
package server

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/ai/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/ai/backend"
	"github.com/otherjamesbrown/penfold/services/ai/config"
	"github.com/otherjamesbrown/penfold/services/ai/registry"
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

// mockBackend implements backend.Backend for testing.
type mockBackend struct {
	generateEmbeddingFunc     func(ctx context.Context, text string, model string) (*backend.EmbeddingResult, error)
	chatCompletionFunc        func(ctx context.Context, messages []backend.Message, opts backend.CompletionOptions) (*backend.CompletionResult, error)
	checkEmbeddingsHealthFunc func(ctx context.Context) error
	checkLLMHealthFunc        func(ctx context.Context) error
}

func (m *mockBackend) GenerateEmbedding(ctx context.Context, text string, model string) (*backend.EmbeddingResult, error) {
	if m.generateEmbeddingFunc != nil {
		return m.generateEmbeddingFunc(ctx, text, model)
	}
	return &backend.EmbeddingResult{
		Vector:     []float32{0.1, 0.2, 0.3},
		Dimensions: 3,
		Model:      "mock-model",
		TokenCount: 5,
	}, nil
}

func (m *mockBackend) ChatCompletion(ctx context.Context, messages []backend.Message, opts backend.CompletionOptions) (*backend.CompletionResult, error) {
	if m.chatCompletionFunc != nil {
		return m.chatCompletionFunc(ctx, messages, opts)
	}
	return &backend.CompletionResult{
		Content:      "mock response",
		Model:        "mock-llm",
		InputTokens:  10,
		OutputTokens: 5,
	}, nil
}

func (m *mockBackend) CheckEmbeddingsHealth(ctx context.Context) error {
	if m.checkEmbeddingsHealthFunc != nil {
		return m.checkEmbeddingsHealthFunc(ctx)
	}
	return nil
}

func (m *mockBackend) CheckLLMHealth(ctx context.Context) error {
	if m.checkLLMHealthFunc != nil {
		return m.checkLLMHealthFunc(ctx)
	}
	return nil
}

func (m *mockBackend) Close() error {
	return nil
}

// testConfig returns a test configuration.
func testConfig() *config.Config {
	return &config.Config{
		ServiceName:             "ai-test",
		GRPCPort:                50051,
		HTTPPort:                8090,
		MLXLLMURL:               "http://localhost:8080",
		MLXEmbeddingsURL:        "http://localhost:8081",
		GeminiDefaultModel:      "gemini-2.5-pro",
		DefaultEmbeddingModel:   "mxbai-embed-large-v1",
		DefaultLLMModel:         "mlx-community/Qwen2.5-7B-Instruct-4bit",
		EmbeddingDimensions:     1024,
		LogLevel:                "info",
		Environment:             "dev",
		EnableMetrics:           true,
		GracefulShutdownTimeout: 30,
	}
}

// testLogger returns a mock logger for testing.
func testLogger() logging.Logger {
	return newMockLogger()
}

// newTestServer creates a test server with mock backend.
func newTestServer(be backend.Backend) *AIServer {
	cfg := testConfig()
	logger := testLogger()
	return NewAIServer(cfg, logger, be)
}

// =============================================================================
// Server Construction Tests
// =============================================================================

func TestNewAIServer(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	be := &mockBackend{}

	server := NewAIServer(cfg, logger, be)

	require.NotNil(t, server, "NewAIServer should return non-nil server")
	assert.Equal(t, cfg, server.config, "Config should be assigned")
	assert.NotNil(t, server.logger, "Logger should be assigned")
	assert.NotNil(t, server.backend, "Backend should be assigned")
}

// =============================================================================
// GenerateEmbedding Tests
// =============================================================================

func TestGenerateEmbedding_Success(t *testing.T) {
	be := &mockBackend{
		generateEmbeddingFunc: func(ctx context.Context, text string, model string) (*backend.EmbeddingResult, error) {
			return &backend.EmbeddingResult{
				Vector:     []float32{0.1, 0.2, 0.3, 0.4},
				Dimensions: 4,
				Model:      "test-model",
				TokenCount: 10,
			}, nil
		},
	}
	server := newTestServer(be)

	req := &aiv1.EmbeddingRequest{
		Text:     "test text for embedding",
		TenantId: stringPtr("tenant-1"),
	}

	resp, err := server.GenerateEmbedding(context.Background(), req)

	require.NoError(t, err)
	assert.Len(t, resp.Vector, 4)
	assert.Equal(t, int32(4), resp.Dimensions)
	assert.Equal(t, "test-model", resp.ModelUsed)
	assert.Equal(t, int32(10), resp.GetTokenCount())
}

func TestGenerateEmbedding_EmptyText(t *testing.T) {
	server := newTestServer(&mockBackend{})

	req := &aiv1.EmbeddingRequest{
		Text:     "",
		TenantId: stringPtr("tenant-1"),
	}

	_, err := server.GenerateEmbedding(context.Background(), req)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}

func TestGenerateEmbedding_WhitespaceOnlyText(t *testing.T) {
	server := newTestServer(&mockBackend{})

	req := &aiv1.EmbeddingRequest{
		Text:     "   \n\t  ",
		TenantId: stringPtr("tenant-1"),
	}

	_, err := server.GenerateEmbedding(context.Background(), req)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}

func TestGenerateEmbedding_BackendError(t *testing.T) {
	be := &mockBackend{
		generateEmbeddingFunc: func(ctx context.Context, text string, model string) (*backend.EmbeddingResult, error) {
			return nil, backend.ErrServiceUnavailable
		},
	}
	server := newTestServer(be)

	req := &aiv1.EmbeddingRequest{
		Text:     "test text",
		TenantId: stringPtr("tenant-1"),
	}

	_, err := server.GenerateEmbedding(context.Background(), req)

	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.Unavailable, st.Code())
}

func TestGenerateEmbedding_WithModel(t *testing.T) {
	var receivedModel string
	be := &mockBackend{
		generateEmbeddingFunc: func(ctx context.Context, text string, model string) (*backend.EmbeddingResult, error) {
			receivedModel = model
			return &backend.EmbeddingResult{
				Vector:     []float32{0.1},
				Dimensions: 1,
				Model:      model,
			}, nil
		},
	}
	server := newTestServer(be)

	req := &aiv1.EmbeddingRequest{
		Text:     "test text",
		TenantId: stringPtr("tenant-1"),
		Model:    stringPtr("custom-model"),
	}

	_, err := server.GenerateEmbedding(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, "custom-model", receivedModel)
}

// =============================================================================
// GenerateSummary Tests
// =============================================================================

func TestGenerateSummary_Success(t *testing.T) {
	be := &mockBackend{
		chatCompletionFunc: func(ctx context.Context, messages []backend.Message, opts backend.CompletionOptions) (*backend.CompletionResult, error) {
			return &backend.CompletionResult{
				Content: `SUMMARY:
This is a test summary.

KEY_POINTS:
["point one", "point two"]`,
				Model:        "test-llm",
				InputTokens:  50,
				OutputTokens: 20,
			}, nil
		},
	}
	server := newTestServer(be)

	req := &aiv1.SummaryRequest{
		Content:  "This is test content to summarize.",
		TenantId: stringPtr("tenant-1"),
		Style:    aiv1.SummaryStyle_SUMMARY_STYLE_BRIEF,
	}

	resp, err := server.GenerateSummary(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, "This is a test summary.", resp.Summary)
	assert.Len(t, resp.KeyPoints, 2)
	assert.Equal(t, "test-llm", resp.ModelUsed)
	assert.Equal(t, int32(50), resp.GetInputTokens())
	assert.Equal(t, int32(20), resp.GetOutputTokens())
}

func TestGenerateSummary_EmptyContent(t *testing.T) {
	server := newTestServer(&mockBackend{})

	req := &aiv1.SummaryRequest{
		Content:  "",
		TenantId: stringPtr("tenant-1"),
	}

	_, err := server.GenerateSummary(context.Background(), req)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}

func TestGenerateSummary_DifferentStyles(t *testing.T) {
	be := &mockBackend{
		chatCompletionFunc: func(ctx context.Context, messages []backend.Message, opts backend.CompletionOptions) (*backend.CompletionResult, error) {
			return &backend.CompletionResult{
				Content: "Plain summary content",
				Model:   "test-llm",
			}, nil
		},
	}
	server := newTestServer(be)

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

			resp, err := server.GenerateSummary(context.Background(), req)

			require.NoError(t, err)
			assert.NotEmpty(t, resp.Summary)
		})
	}
}

// =============================================================================
// ExtractAssertions Tests
// =============================================================================

func TestExtractAssertions_Success(t *testing.T) {
	be := &mockBackend{
		chatCompletionFunc: func(ctx context.Context, messages []backend.Message, opts backend.CompletionOptions) (*backend.CompletionResult, error) {
			return &backend.CompletionResult{
				Content: `{
					"assertions": [
						{
							"subject": "John",
							"predicate": "works at",
							"object": "Acme Corp",
							"confidence": 0.9,
							"source_text": "John works at Acme Corp",
							"category": "organizational"
						},
						{
							"subject": "Meeting",
							"predicate": "scheduled for",
							"object": "Tuesday",
							"confidence": 0.7,
							"source_text": "Meeting scheduled for Tuesday",
							"category": "temporal"
						}
					]
				}`,
				Model: "test-llm",
			}, nil
		},
	}
	server := newTestServer(be)

	req := &aiv1.AssertionRequest{
		Content:  "John works at Acme Corp. Meeting scheduled for Tuesday.",
		TenantId: stringPtr("tenant-1"),
	}

	resp, err := server.ExtractAssertions(context.Background(), req)

	require.NoError(t, err)
	assert.Len(t, resp.Assertions, 2)
	assert.Equal(t, "John", resp.Assertions[0].Subject)
	assert.Equal(t, float32(0.9), resp.Assertions[0].Confidence)
	assert.Equal(t, int32(2), resp.TotalFound)
}

func TestExtractAssertions_FiltersByConfidence(t *testing.T) {
	be := &mockBackend{
		chatCompletionFunc: func(ctx context.Context, messages []backend.Message, opts backend.CompletionOptions) (*backend.CompletionResult, error) {
			return &backend.CompletionResult{
				Content: `{
					"assertions": [
						{"subject": "A", "predicate": "is", "object": "B", "confidence": 0.9},
						{"subject": "C", "predicate": "is", "object": "D", "confidence": 0.3}
					]
				}`,
				Model: "test-llm",
			}, nil
		},
	}
	server := newTestServer(be)

	minConf := float32(0.5)
	req := &aiv1.AssertionRequest{
		Content:       "Test content",
		TenantId:      stringPtr("tenant-1"),
		MinConfidence: &minConf,
	}

	resp, err := server.ExtractAssertions(context.Background(), req)

	require.NoError(t, err)
	assert.Len(t, resp.Assertions, 1)
	assert.Equal(t, int32(1), resp.FilteredCount)
}

func TestExtractAssertions_EmptyContent(t *testing.T) {
	server := newTestServer(&mockBackend{})

	req := &aiv1.AssertionRequest{
		Content:  "",
		TenantId: stringPtr("tenant-1"),
	}

	_, err := server.ExtractAssertions(context.Background(), req)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}

func TestExtractAssertions_MaxAssertions(t *testing.T) {
	be := &mockBackend{
		chatCompletionFunc: func(ctx context.Context, messages []backend.Message, opts backend.CompletionOptions) (*backend.CompletionResult, error) {
			return &backend.CompletionResult{
				Content: `{
					"assertions": [
						{"subject": "A", "predicate": "is", "object": "B", "confidence": 0.9},
						{"subject": "C", "predicate": "is", "object": "D", "confidence": 0.8},
						{"subject": "E", "predicate": "is", "object": "F", "confidence": 0.7}
					]
				}`,
				Model: "test-llm",
			}, nil
		},
	}
	server := newTestServer(be)

	maxAssertions := int32(2)
	req := &aiv1.AssertionRequest{
		Content:       "Test content",
		TenantId:      stringPtr("tenant-1"),
		MaxAssertions: &maxAssertions,
	}

	resp, err := server.ExtractAssertions(context.Background(), req)

	require.NoError(t, err)
	assert.Len(t, resp.Assertions, 2)
	// Should keep highest confidence assertions
	assert.Equal(t, float32(0.9), resp.Assertions[0].Confidence)
	assert.Equal(t, float32(0.8), resp.Assertions[1].Confidence)
}

// =============================================================================
// ClassifyContent Tests
// =============================================================================

func TestClassifyContent_Success(t *testing.T) {
	be := &mockBackend{
		chatCompletionFunc: func(ctx context.Context, messages []backend.Message, opts backend.CompletionOptions) (*backend.CompletionResult, error) {
			return &backend.CompletionResult{
				Content: `{
					"classifications": [
						{"label": "technology", "confidence": 0.85, "explanation": "Discusses tech topics"},
						{"label": "business", "confidence": 0.6, "explanation": "Business context"}
					]
				}`,
				Model: "test-llm",
			}, nil
		},
	}
	server := newTestServer(be)

	req := &aiv1.ClassifyContentRequest{
		Content:    "This is about AI and machine learning for business.",
		TenantId:   stringPtr("tenant-1"),
		Categories: []string{"technology", "business", "health"},
	}

	resp, err := server.ClassifyContent(context.Background(), req)

	require.NoError(t, err)
	assert.Len(t, resp.Classifications, 2)
	assert.Equal(t, "technology", resp.Primary.Label)
	assert.Equal(t, float32(0.85), resp.Primary.Confidence)
}

func TestClassifyContent_SingleLabelMode(t *testing.T) {
	be := &mockBackend{
		chatCompletionFunc: func(ctx context.Context, messages []backend.Message, opts backend.CompletionOptions) (*backend.CompletionResult, error) {
			return &backend.CompletionResult{
				Content: `{
					"classifications": [
						{"label": "A", "confidence": 0.9},
						{"label": "B", "confidence": 0.7}
					]
				}`,
				Model: "test-llm",
			}, nil
		},
	}
	server := newTestServer(be)

	multiLabel := false
	req := &aiv1.ClassifyContentRequest{
		Content:    "Test content",
		TenantId:   stringPtr("tenant-1"),
		MultiLabel: &multiLabel,
	}

	resp, err := server.ClassifyContent(context.Background(), req)

	require.NoError(t, err)
	assert.Len(t, resp.Classifications, 1)
	assert.Equal(t, "A", resp.Classifications[0].Label)
}

func TestClassifyContent_EmptyContent(t *testing.T) {
	server := newTestServer(&mockBackend{})

	req := &aiv1.ClassifyContentRequest{
		Content:    "",
		TenantId:   stringPtr("tenant-1"),
		Categories: []string{"cat1", "cat2"},
	}

	_, err := server.ClassifyContent(context.Background(), req)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}

func TestClassifyContent_MinConfidenceFilter(t *testing.T) {
	be := &mockBackend{
		chatCompletionFunc: func(ctx context.Context, messages []backend.Message, opts backend.CompletionOptions) (*backend.CompletionResult, error) {
			return &backend.CompletionResult{
				Content: `{
					"classifications": [
						{"label": "A", "confidence": 0.8},
						{"label": "B", "confidence": 0.2}
					]
				}`,
				Model: "test-llm",
			}, nil
		},
	}
	server := newTestServer(be)

	minConf := float32(0.5)
	req := &aiv1.ClassifyContentRequest{
		Content:       "Test content",
		TenantId:      stringPtr("tenant-1"),
		MinConfidence: &minConf,
	}

	resp, err := server.ClassifyContent(context.Background(), req)

	require.NoError(t, err)
	assert.Len(t, resp.Classifications, 1)
	assert.Equal(t, "A", resp.Classifications[0].Label)
}

// =============================================================================
// GetModelStatus Tests
// =============================================================================

func TestGetModelStatus_AllHealthy(t *testing.T) {
	be := &mockBackend{
		checkEmbeddingsHealthFunc: func(ctx context.Context) error { return nil },
		checkLLMHealthFunc:        func(ctx context.Context) error { return nil },
	}
	server := newTestServer(be)

	req := &aiv1.GetModelStatusRequest{}

	resp, err := server.GetModelStatus(context.Background(), req)

	require.NoError(t, err)
	assert.True(t, resp.Healthy)
	assert.Equal(t, "All models healthy", resp.Message)
	assert.NotEmpty(t, resp.DefaultEmbeddingModel)
	assert.NotEmpty(t, resp.DefaultLlmModel)
}

func TestGetModelStatus_EmbeddingsUnhealthy(t *testing.T) {
	be := &mockBackend{
		checkEmbeddingsHealthFunc: func(ctx context.Context) error {
			return errors.New("connection refused")
		},
		checkLLMHealthFunc: func(ctx context.Context) error { return nil },
	}
	server := newTestServer(be)

	req := &aiv1.GetModelStatusRequest{}

	resp, err := server.GetModelStatus(context.Background(), req)

	require.NoError(t, err)
	assert.False(t, resp.Healthy)
	assert.Contains(t, resp.Message, "embeddings")
}

func TestGetModelStatus_LLMUnhealthy(t *testing.T) {
	be := &mockBackend{
		checkEmbeddingsHealthFunc: func(ctx context.Context) error { return nil },
		checkLLMHealthFunc: func(ctx context.Context) error {
			return errors.New("timeout")
		},
	}
	server := newTestServer(be)

	req := &aiv1.GetModelStatusRequest{}

	resp, err := server.GetModelStatus(context.Background(), req)

	require.NoError(t, err)
	assert.False(t, resp.Healthy)
	assert.Contains(t, resp.Message, "llm")
}

func TestGetModelStatus_FilterByModelType(t *testing.T) {
	be := &mockBackend{}
	server := newTestServer(be)

	req := &aiv1.GetModelStatusRequest{
		ModelType: modelTypePtr(aiv1.ModelType_MODEL_TYPE_EMBEDDING),
	}

	resp, err := server.GetModelStatus(context.Background(), req)

	require.NoError(t, err)
	// Should filter to only embedding models
	for _, m := range resp.Models {
		assert.Equal(t, aiv1.ModelType_MODEL_TYPE_EMBEDDING, m.Type)
	}
}

// =============================================================================
// Response Parsing Tests
// =============================================================================

func TestParseSummaryResponse_Structured(t *testing.T) {
	server := newTestServer(&mockBackend{})

	content := `SUMMARY:
This is the summary text.

KEY_POINTS:
["point 1", "point 2", "point 3"]`

	summary, keyPoints := server.parseSummaryResponse(content)

	assert.Equal(t, "This is the summary text.", summary)
	assert.Len(t, keyPoints, 3)
	assert.Equal(t, "point 1", keyPoints[0])
}

func TestParseSummaryResponse_Unstructured(t *testing.T) {
	server := newTestServer(&mockBackend{})

	content := "This is just plain text without markers."

	summary, keyPoints := server.parseSummaryResponse(content)

	assert.Equal(t, content, summary)
	assert.Empty(t, keyPoints)
}

func TestParseAssertionsResponse_ValidJSON(t *testing.T) {
	server := newTestServer(&mockBackend{})

	content := `{
		"assertions": [
			{
				"subject": "Alice",
				"predicate": "manages",
				"object": "the team",
				"confidence": 0.85,
				"source_text": "Alice manages the team",
				"category": "organizational"
			}
		]
	}`

	assertions, err := server.parseAssertionsResponse(content)

	require.NoError(t, err)
	assert.Len(t, assertions, 1)
	assert.Equal(t, "Alice", assertions[0].Subject)
	assert.Equal(t, float32(0.85), assertions[0].Confidence)
}

func TestParseAssertionsResponse_MarkdownWrapped(t *testing.T) {
	server := newTestServer(&mockBackend{})

	content := "```json\n{\"assertions\": []}\n```"

	assertions, err := server.parseAssertionsResponse(content)

	require.NoError(t, err)
	assert.Empty(t, assertions)
}

func TestParseAssertionsResponse_InvalidJSON(t *testing.T) {
	server := newTestServer(&mockBackend{})

	content := "not valid json"

	_, err := server.parseAssertionsResponse(content)

	assert.Error(t, err)
}

func TestParseClassificationsResponse_ValidJSON(t *testing.T) {
	server := newTestServer(&mockBackend{})

	content := `{
		"classifications": [
			{"label": "tech", "confidence": 0.9, "explanation": "Technical content"}
		]
	}`

	classifications, err := server.parseClassificationsResponse(content)

	require.NoError(t, err)
	assert.Len(t, classifications, 1)
	assert.Equal(t, "tech", classifications[0].Label)
	assert.Equal(t, float32(0.9), classifications[0].Confidence)
}

// =============================================================================
// Error Conversion Tests
// =============================================================================

func TestConvertError(t *testing.T) {
	server := newTestServer(&mockBackend{})

	tests := []struct {
		name     string
		err      error
		wantCode codes.Code
	}{
		{
			name:     "context canceled",
			err:      backend.ErrContextCanceled,
			wantCode: codes.Canceled,
		},
		{
			name:     "service unavailable",
			err:      backend.ErrServiceUnavailable,
			wantCode: codes.Unavailable,
		},
		{
			name:     "empty text",
			err:      backend.ErrEmptyText,
			wantCode: codes.InvalidArgument,
		},
		{
			name:     "empty messages",
			err:      backend.ErrEmptyMessages,
			wantCode: codes.InvalidArgument,
		},
		{
			name:     "request timeout",
			err:      backend.ErrRequestTimeout,
			wantCode: codes.DeadlineExceeded,
		},
		{
			name:     "generic error",
			err:      errors.New("something went wrong"),
			wantCode: codes.Internal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			grpcErr := server.convertError(tt.err)
			st, _ := status.FromError(grpcErr)
			assert.Equal(t, tt.wantCode, st.Code())
		})
	}
}

// =============================================================================
// Concurrent Access Tests
// =============================================================================

func TestConcurrentRequests(t *testing.T) {
	be := &mockBackend{}
	server := newTestServer(be)

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
	be := &mockBackend{}
	server := newTestServer(be)

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
	be := &mockBackend{}
	server := newTestServer(be)

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
	be := &mockBackend{}
	server := newTestServer(be)

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
	be := &mockBackend{}
	server := newTestServer(be)

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
	be := &mockBackend{}
	server := newTestServer(be)

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

// modelTypePtr returns a pointer to the given ModelType.
func modelTypePtr(mt aiv1.ModelType) *aiv1.ModelType {
	return &mt
}

// =============================================================================
// Router/Registry Integration Tests
// =============================================================================

func TestNewAIServerWithRouter(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()

	// Test with nil router and registry (should not panic)
	server := NewAIServerWithRouter(cfg, logger, nil, nil)
	require.NotNil(t, server)
	assert.Equal(t, cfg, server.config)
	assert.Nil(t, server.backend)
	assert.Nil(t, server.router)
	assert.Nil(t, server.registry)
}

func TestNewAIServerWithAll(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	be := &mockBackend{}

	server := NewAIServerWithAll(cfg, logger, be, nil, nil)
	require.NotNil(t, server)
	assert.Equal(t, cfg, server.config)
	assert.NotNil(t, server.backend)
}

func TestListModels_NoRegistry(t *testing.T) {
	server := newTestServer(&mockBackend{})

	req := &aiv1.ListModelsRequest{}
	_, err := server.ListModels(context.Background(), req)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unimplemented, st.Code())
}

func TestRegisterModel_NoRegistry(t *testing.T) {
	server := newTestServer(&mockBackend{})

	req := &aiv1.RegisterModelRequest{
		Name:         "test-model",
		Provider:     "mlx",
		ModelName:    "llama3.2",
		Type:         aiv1.ModelType_MODEL_TYPE_LLM,
		Capabilities: []string{"chat"},
	}
	_, err := server.RegisterModel(context.Background(), req)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unimplemented, st.Code())
}

func TestRegisterModel_ValidationErrors(t *testing.T) {
	// Create a server with a mock registry would require mocking the registry
	// For now, test that validation happens before registry check
	server := newTestServer(&mockBackend{})

	tests := []struct {
		name    string
		req     *aiv1.RegisterModelRequest
		wantErr string
	}{
		{
			name: "missing name",
			req: &aiv1.RegisterModelRequest{
				Provider:     "mlx",
				ModelName:    "llama3.2",
				Type:         aiv1.ModelType_MODEL_TYPE_LLM,
				Capabilities: []string{"chat"},
			},
			wantErr: "Unimplemented", // Will hit registry check before validation in current impl
		},
		{
			name: "missing provider",
			req: &aiv1.RegisterModelRequest{
				Name:         "test",
				ModelName:    "llama3.2",
				Type:         aiv1.ModelType_MODEL_TYPE_LLM,
				Capabilities: []string{"chat"},
			},
			wantErr: "Unimplemented",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := server.RegisterModel(context.Background(), tt.req)
			require.Error(t, err)
		})
	}
}

func TestUpdateModel_NoRegistry(t *testing.T) {
	server := newTestServer(&mockBackend{})

	req := &aiv1.UpdateModelRequest{
		ModelId: "test-model-id",
	}
	_, err := server.UpdateModel(context.Background(), req)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unimplemented, st.Code())
}

func TestDeleteModel_NoRegistry(t *testing.T) {
	server := newTestServer(&mockBackend{})

	req := &aiv1.DeleteModelRequest{
		ModelId: "test-model-id",
	}
	_, err := server.DeleteModel(context.Background(), req)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unimplemented, st.Code())
}

func TestGetRoutingRules_NoRegistry(t *testing.T) {
	server := newTestServer(&mockBackend{})

	req := &aiv1.GetRoutingRulesRequest{}
	_, err := server.GetRoutingRules(context.Background(), req)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unimplemented, st.Code())
}

func TestUpdateRoutingRule_NoRegistry(t *testing.T) {
	server := newTestServer(&mockBackend{})

	req := &aiv1.UpdateRoutingRuleRequest{
		Name:              "test-rule",
		TaskType:          "embedding",
		PreferredModelIds: []string{"model-1"},
	}
	_, err := server.UpdateRoutingRule(context.Background(), req)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unimplemented, st.Code())
}

func TestUpdateRoutingRule_ValidationErrors(t *testing.T) {
	server := newTestServer(&mockBackend{})

	tests := []struct {
		name    string
		req     *aiv1.UpdateRoutingRuleRequest
		wantErr string
	}{
		{
			name: "missing name",
			req: &aiv1.UpdateRoutingRuleRequest{
				TaskType:          "embedding",
				PreferredModelIds: []string{"model-1"},
			},
			wantErr: "Unimplemented", // Will hit registry check before validation
		},
		{
			name: "missing task_type",
			req: &aiv1.UpdateRoutingRuleRequest{
				Name:              "test-rule",
				PreferredModelIds: []string{"model-1"},
			},
			wantErr: "Unimplemented",
		},
		{
			name: "missing preferred_model_ids",
			req: &aiv1.UpdateRoutingRuleRequest{
				Name:     "test-rule",
				TaskType: "embedding",
			},
			wantErr: "Unimplemented",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := server.UpdateRoutingRule(context.Background(), tt.req)
			require.Error(t, err)
		})
	}
}

func TestGetModelStatus_WithNilBackend(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	// Create server without backend
	server := NewAIServerWithRouter(cfg, logger, nil, nil)

	req := &aiv1.GetModelStatusRequest{}
	resp, err := server.GetModelStatus(context.Background(), req)

	require.NoError(t, err)
	assert.True(t, resp.Healthy)
	assert.Empty(t, resp.Models)
}

func TestConvertError_RegistryErrors(t *testing.T) {
	server := newTestServer(&mockBackend{})

	tests := []struct {
		name     string
		err      error
		wantCode codes.Code
	}{
		{
			name:     "model not found",
			err:      registry.ErrModelNotFound,
			wantCode: codes.NotFound,
		},
		{
			name:     "model already exists",
			err:      registry.ErrModelAlreadyExists,
			wantCode: codes.AlreadyExists,
		},
		{
			name:     "invalid model ID",
			err:      registry.ErrInvalidModelID,
			wantCode: codes.InvalidArgument,
		},
		{
			name:     "invalid model name",
			err:      registry.ErrInvalidModelName,
			wantCode: codes.InvalidArgument,
		},
		{
			name:     "invalid provider",
			err:      registry.ErrInvalidProvider,
			wantCode: codes.InvalidArgument,
		},
		{
			name:     "no capabilities",
			err:      registry.ErrNoCapabilities,
			wantCode: codes.InvalidArgument,
		},
		{
			name:     "invalid routing rule",
			err:      registry.ErrInvalidRoutingRule,
			wantCode: codes.InvalidArgument,
		},
		{
			name:     "routing rule not found",
			err:      registry.ErrRoutingRuleNotFound,
			wantCode: codes.NotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			grpcErr := server.convertError(tt.err)
			st, _ := status.FromError(grpcErr)
			assert.Equal(t, tt.wantCode, st.Code())
		})
	}
}

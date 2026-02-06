package ai

import (
	"context"
	"net"
	"testing"
	"time"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// mockAIServer implements the AICoordinatorServiceServer for testing.
type mockAIServer struct {
	aiv1.UnimplementedAICoordinatorServiceServer

	// Response handlers
	generateEmbeddingFunc  func(ctx context.Context, req *aiv1.EmbeddingRequest) (*aiv1.EmbeddingResponse, error)
	generateSummaryFunc    func(ctx context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error)
	extractAssertionsFunc  func(ctx context.Context, req *aiv1.AssertionRequest) (*aiv1.AssertionResponse, error)
	classifyContentFunc    func(ctx context.Context, req *aiv1.ClassifyContentRequest) (*aiv1.ClassifyContentResponse, error)
	triageContentFunc      func(ctx context.Context, req *aiv1.TriageContentRequest) (*aiv1.TriageContentResponse, error)
	extractEntitiesFunc    func(ctx context.Context, req *aiv1.ExtractEntitiesRequest) (*aiv1.ExtractEntitiesResponse, error)
	deepAnalyzeFunc        func(ctx context.Context, req *aiv1.DeepAnalyzeRequest) (*aiv1.DeepAnalyzeResponse, error)
	getModelStatusFunc     func(ctx context.Context, req *aiv1.GetModelStatusRequest) (*aiv1.GetModelStatusResponse, error)

	// Call counters for retry testing
	embeddingCalls int
	summaryCalls   int
	assertionCalls int
}

func (m *mockAIServer) GenerateEmbedding(ctx context.Context, req *aiv1.EmbeddingRequest) (*aiv1.EmbeddingResponse, error) {
	m.embeddingCalls++
	if m.generateEmbeddingFunc != nil {
		return m.generateEmbeddingFunc(ctx, req)
	}
	return &aiv1.EmbeddingResponse{
		Vector:     []float32{0.1, 0.2, 0.3, 0.4, 0.5},
		Dimensions: 5,
		ModelUsed:  "test-model",
	}, nil
}

func (m *mockAIServer) GenerateSummary(ctx context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
	m.summaryCalls++
	if m.generateSummaryFunc != nil {
		return m.generateSummaryFunc(ctx, req)
	}
	return &aiv1.SummaryResponse{
		Summary:   "This is a test summary.",
		KeyPoints: []string{"Point 1", "Point 2"},
		ModelUsed: "test-model",
	}, nil
}

func (m *mockAIServer) ExtractAssertions(ctx context.Context, req *aiv1.AssertionRequest) (*aiv1.AssertionResponse, error) {
	m.assertionCalls++
	if m.extractAssertionsFunc != nil {
		return m.extractAssertionsFunc(ctx, req)
	}
	return &aiv1.AssertionResponse{
		Assertions: []*aiv1.Assertion{
			{
				Subject:    "Test",
				Predicate:  "is",
				Object:     "passing",
				Confidence: 0.95,
			},
		},
		ModelUsed:  "test-model",
		TotalFound: 1,
	}, nil
}

func (m *mockAIServer) ClassifyContent(ctx context.Context, req *aiv1.ClassifyContentRequest) (*aiv1.ClassifyContentResponse, error) {
	if m.classifyContentFunc != nil {
		return m.classifyContentFunc(ctx, req)
	}
	return &aiv1.ClassifyContentResponse{
		Classifications: []*aiv1.Classification{
			{Label: "work", Confidence: 0.9},
		},
		Primary:   &aiv1.Classification{Label: "work", Confidence: 0.9},
		ModelUsed: "test-model",
	}, nil
}

func (m *mockAIServer) TriageContent(ctx context.Context, req *aiv1.TriageContentRequest) (*aiv1.TriageContentResponse, error) {
	if m.triageContentFunc != nil {
		return m.triageContentFunc(ctx, req)
	}
	inputTokens := int32(50)
	outputTokens := int32(20)
	return &aiv1.TriageContentResponse{
		Category:     "PROJECT_UPDATE",
		Importance:   "HIGH",
		Reason:       "Content contains project status information",
		ModelUsed:    "test-model",
		InputTokens:  &inputTokens,
		OutputTokens: &outputTokens,
		Retries:      0,
	}, nil
}

func (m *mockAIServer) ExtractEntities(ctx context.Context, req *aiv1.ExtractEntitiesRequest) (*aiv1.ExtractEntitiesResponse, error) {
	if m.extractEntitiesFunc != nil {
		return m.extractEntitiesFunc(ctx, req)
	}
	inputTokens := int32(100)
	outputTokens := int32(80)
	return &aiv1.ExtractEntitiesResponse{
		People: []*aiv1.PersonEntity{
			{Name: "Alice Smith", Role: "Engineer"},
		},
		Dates: []*aiv1.DateEntity{
			{Date: "January 15th", Context: "project deadline"},
		},
		Projects:      []string{"Project Alpha"},
		Organisations: []string{"Acme Corp"},
		ActionItems: []*aiv1.ActionItemEntity{
			{Assignee: "Bob", Action: "Review PR", Due: "next week"},
		},
		Decisions:            []string{"Approved budget"},
		Risks:                []string{"Potential delay"},
		DetailedRisks:        []*aiv1.RiskEntity{},
		QualityGateTriggered: false,
		ModelUsed:            "test-model",
		InputTokens:          &inputTokens,
		OutputTokens:         &outputTokens,
		Retries:              0,
	}, nil
}

func (m *mockAIServer) DeepAnalyze(ctx context.Context, req *aiv1.DeepAnalyzeRequest) (*aiv1.DeepAnalyzeResponse, error) {
	if m.deepAnalyzeFunc != nil {
		return m.deepAnalyzeFunc(ctx, req)
	}
	inputTokens := int32(500)
	outputTokens := int32(300)
	return &aiv1.DeepAnalyzeResponse{
		Summary: "This is a deep analysis summary",
		Sentiment: &aiv1.DeepSentiment{
			Score:       0.3,
			Label:       "neutral",
			Confidence:  0.85,
			Indicators:  []string{"balanced tone", "factual content"},
			Explanation: "Content maintains a professional and balanced tone",
		},
		TopicMappings: []*aiv1.TopicMapping{
			{
				Topic:          "project status",
				RelatedProject: "Project Alpha",
				Relationship:   "provides update on progress",
				Confidence:     0.9,
			},
		},
		VerifiedActionItems: []*aiv1.VerifiedActionItem{
			{
				Description:    "Review the PR",
				Assignee:       "Bob",
				Due:            "next week",
				Priority:       "high",
				ContextExcerpt: "Bob should review the PR next week",
				Status:         "confirmed",
			},
		},
		VerifiedDecisions: []*aiv1.VerifiedDecision{
			{
				Description:    "Approved budget",
				ContextExcerpt: "We approved the budget for Q1",
				Status:         "confirmed",
			},
		},
		RiskReferences: []*aiv1.RiskReference{
			{
				Description:    "Potential delay in delivery",
				Significance:   "primary",
				ContextExcerpt: "There is a potential delay in delivery",
				IsNew:          true,
			},
		},
		StrategicInsights: []string{"Project is on track but requires attention to timeline"},
		ImplicitActionItems: []*aiv1.ImplicitActionItem{
			{
				Description:    "Update project timeline",
				Reasoning:      "Delay mentioned requires timeline adjustment",
				ContextExcerpt: "potential delay in delivery",
			},
		},
		ModelUsed:    "gemini-2.5-pro",
		InputTokens:  &inputTokens,
		OutputTokens: &outputTokens,
	}, nil
}

func (m *mockAIServer) GetModelStatus(ctx context.Context, req *aiv1.GetModelStatusRequest) (*aiv1.GetModelStatusResponse, error) {
	if m.getModelStatusFunc != nil {
		return m.getModelStatusFunc(ctx, req)
	}
	return &aiv1.GetModelStatusResponse{
		Models: []*aiv1.ModelInfo{
			{
				Name:     "test-model",
				Type:     aiv1.ModelType_MODEL_TYPE_EMBEDDING,
				Status:   aiv1.ModelStatus_MODEL_STATUS_READY,
				IsLocal:  true,
				Provider: "test",
			},
		},
		DefaultEmbeddingModel: "test-model",
		DefaultLlmModel:       "test-llm",
		Healthy:               true,
		Message:               "All systems operational",
	}, nil
}

// startTestServer starts a gRPC server with the mock implementation.
func startTestServer(t *testing.T, mock *mockAIServer) (*grpc.Server, string) {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := grpc.NewServer()
	aiv1.RegisterAICoordinatorServiceServer(server, mock)

	go func() {
		if err := server.Serve(lis); err != nil && err != grpc.ErrServerStopped {
			t.Logf("Server error: %v", err)
		}
	}()

	return server, lis.Addr().String()
}

func TestNewClient(t *testing.T) {
	t.Run("empty address returns error", func(t *testing.T) {
		_, err := NewClient("")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "address is required")
	})

	t.Run("invalid address returns error", func(t *testing.T) {
		// Use a very short timeout to fail quickly on invalid address
		_, err := NewClient("invalid-host:99999",
			WithConnectTimeout(100*time.Millisecond),
			WithDialOptions(grpc.WithBlock()),
		)
		assert.Error(t, err)
	})

	t.Run("valid connection succeeds", func(t *testing.T) {
		mock := &mockAIServer{}
		server, addr := startTestServer(t, mock)
		defer server.Stop()

		client, err := NewClient(addr,
			WithConnectTimeout(5*time.Second),
		)
		require.NoError(t, err)
		defer client.Close()

		assert.NotNil(t, client)
		assert.NotNil(t, client.conn)
		assert.NotNil(t, client.client)
	})
}

func TestClient_GenerateEmbedding(t *testing.T) {
	mock := &mockAIServer{}
	server, addr := startTestServer(t, mock)
	defer server.Stop()

	client, err := NewClient(addr)
	require.NoError(t, err)
	defer client.Close()

	t.Run("nil request returns error", func(t *testing.T) {
		_, err := client.GenerateEmbedding(context.Background(), nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "request is required")
	})

	t.Run("successful embedding generation", func(t *testing.T) {
		resp, err := client.GenerateEmbedding(context.Background(), &aiv1.EmbeddingRequest{
			Text: "Hello, world!",
		})
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, int32(5), resp.Dimensions)
		assert.Equal(t, "test-model", resp.ModelUsed)
		assert.Len(t, resp.Vector, 5)
	})

	t.Run("cancelled context returns error", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := client.GenerateEmbedding(ctx, &aiv1.EmbeddingRequest{
			Text: "Hello, world!",
		})
		assert.Error(t, err)
	})
}

func TestClient_GenerateSummary(t *testing.T) {
	mock := &mockAIServer{}
	server, addr := startTestServer(t, mock)
	defer server.Stop()

	client, err := NewClient(addr)
	require.NoError(t, err)
	defer client.Close()

	t.Run("nil request returns error", func(t *testing.T) {
		_, err := client.GenerateSummary(context.Background(), nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "request is required")
	})

	t.Run("successful summary generation", func(t *testing.T) {
		resp, err := client.GenerateSummary(context.Background(), &aiv1.SummaryRequest{
			Content: "This is a long document that needs summarization.",
		})
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.NotEmpty(t, resp.Summary)
		assert.NotEmpty(t, resp.KeyPoints)
		assert.Equal(t, "test-model", resp.ModelUsed)
	})
}

func TestClient_ExtractAssertions(t *testing.T) {
	mock := &mockAIServer{}
	server, addr := startTestServer(t, mock)
	defer server.Stop()

	client, err := NewClient(addr)
	require.NoError(t, err)
	defer client.Close()

	t.Run("nil request returns error", func(t *testing.T) {
		_, err := client.ExtractAssertions(context.Background(), nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "request is required")
	})

	t.Run("successful assertion extraction", func(t *testing.T) {
		resp, err := client.ExtractAssertions(context.Background(), &aiv1.AssertionRequest{
			Content: "John works at Acme Corp. The project is due next week.",
		})
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.NotEmpty(t, resp.Assertions)
		assert.Equal(t, "test-model", resp.ModelUsed)

		assertion := resp.Assertions[0]
		assert.Equal(t, "Test", assertion.Subject)
		assert.Equal(t, "is", assertion.Predicate)
		assert.Equal(t, "passing", assertion.Object)
	})
}

func TestClient_HealthCheck(t *testing.T) {
	t.Run("healthy server passes health check", func(t *testing.T) {
		mock := &mockAIServer{}
		server, addr := startTestServer(t, mock)
		defer server.Stop()

		client, err := NewClient(addr)
		require.NoError(t, err)
		defer client.Close()

		err = client.HealthCheck(context.Background())
		assert.NoError(t, err)
	})

	t.Run("unhealthy server fails health check", func(t *testing.T) {
		mock := &mockAIServer{
			getModelStatusFunc: func(ctx context.Context, req *aiv1.GetModelStatusRequest) (*aiv1.GetModelStatusResponse, error) {
				return nil, status.Error(codes.Unavailable, "service unavailable")
			},
		}
		server, addr := startTestServer(t, mock)
		defer server.Stop()

		client, err := NewClient(addr, WithMaxRetries(0))
		require.NoError(t, err)
		defer client.Close()

		err = client.HealthCheck(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "health check failed")
	})
}

func TestClient_Close(t *testing.T) {
	mock := &mockAIServer{}
	server, addr := startTestServer(t, mock)
	defer server.Stop()

	client, err := NewClient(addr)
	require.NoError(t, err)

	// Close should succeed
	err = client.Close()
	assert.NoError(t, err)

	// Double close should also succeed
	err = client.Close()
	assert.NoError(t, err)
}

func TestClient_RetryBehavior(t *testing.T) {
	t.Run("retries on unavailable error", func(t *testing.T) {
		callCount := 0
		mock := &mockAIServer{
			generateEmbeddingFunc: func(ctx context.Context, req *aiv1.EmbeddingRequest) (*aiv1.EmbeddingResponse, error) {
				callCount++
				if callCount < 3 {
					return nil, status.Error(codes.Unavailable, "temporarily unavailable")
				}
				return &aiv1.EmbeddingResponse{
					Vector:     []float32{0.1, 0.2, 0.3},
					Dimensions: 3,
					ModelUsed:  "test-model",
				}, nil
			},
		}
		server, addr := startTestServer(t, mock)
		defer server.Stop()

		client, err := NewClient(addr,
			WithMaxRetries(3),
			WithRetryBackoff(10*time.Millisecond),
		)
		require.NoError(t, err)
		defer client.Close()

		resp, err := client.GenerateEmbedding(context.Background(), &aiv1.EmbeddingRequest{
			Text: "test",
		})
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, 3, callCount) // Should have retried twice before succeeding
	})

	t.Run("does not retry on permanent error", func(t *testing.T) {
		callCount := 0
		mock := &mockAIServer{
			generateEmbeddingFunc: func(ctx context.Context, req *aiv1.EmbeddingRequest) (*aiv1.EmbeddingResponse, error) {
				callCount++
				return nil, status.Error(codes.InvalidArgument, "invalid request")
			},
		}
		server, addr := startTestServer(t, mock)
		defer server.Stop()

		client, err := NewClient(addr,
			WithMaxRetries(3),
			WithRetryBackoff(10*time.Millisecond),
		)
		require.NoError(t, err)
		defer client.Close()

		_, err = client.GenerateEmbedding(context.Background(), &aiv1.EmbeddingRequest{
			Text: "test",
		})
		assert.Error(t, err)
		assert.Equal(t, 1, callCount) // Should not have retried
	})

	t.Run("stops retrying on context cancellation", func(t *testing.T) {
		callCount := 0
		mock := &mockAIServer{
			generateEmbeddingFunc: func(ctx context.Context, req *aiv1.EmbeddingRequest) (*aiv1.EmbeddingResponse, error) {
				callCount++
				return nil, status.Error(codes.Unavailable, "temporarily unavailable")
			},
		}
		server, addr := startTestServer(t, mock)
		defer server.Stop()

		client, err := NewClient(addr,
			WithMaxRetries(10),
			WithRetryBackoff(100*time.Millisecond),
		)
		require.NoError(t, err)
		defer client.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		_, err = client.GenerateEmbedding(ctx, &aiv1.EmbeddingRequest{
			Text: "test",
		})
		assert.Error(t, err)
		// Should have made at least 1 call but not all 10 retries
		assert.GreaterOrEqual(t, callCount, 1)
		assert.Less(t, callCount, 10)
	})
}

func TestClientOptions(t *testing.T) {
	t.Run("default options", func(t *testing.T) {
		opts := defaultOptions()
		assert.Equal(t, 10*time.Second, opts.connectTimeout)
		assert.Equal(t, 120*time.Second, opts.requestTimeout)
		assert.Equal(t, 3, opts.maxRetries)
		assert.Equal(t, 100*time.Millisecond, opts.retryBackoff)
		assert.False(t, opts.useTLS)
	})

	t.Run("custom options", func(t *testing.T) {
		opts := defaultOptions()

		WithConnectTimeout(5 * time.Second)(opts)
		assert.Equal(t, 5*time.Second, opts.connectTimeout)

		WithRequestTimeout(60 * time.Second)(opts)
		assert.Equal(t, 60*time.Second, opts.requestTimeout)

		WithMaxRetries(5)(opts)
		assert.Equal(t, 5, opts.maxRetries)

		WithRetryBackoff(200 * time.Millisecond)(opts)
		assert.Equal(t, 200*time.Millisecond, opts.retryBackoff)

		WithTLS()(opts)
		assert.True(t, opts.useTLS)

		WithInsecure()(opts)
		assert.False(t, opts.useTLS)
	})

	t.Run("dial options are appended", func(t *testing.T) {
		opts := defaultOptions()
		opt1 := grpc.WithTransportCredentials(insecure.NewCredentials())
		opt2 := grpc.WithBlock()

		WithDialOptions(opt1)(opts)
		assert.Len(t, opts.dialOptions, 1)

		WithDialOptions(opt2)(opts)
		assert.Len(t, opts.dialOptions, 2)
	})
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name      string
		code      codes.Code
		retryable bool
	}{
		{"OK", codes.OK, false},
		{"Cancelled", codes.Canceled, false},
		{"Unknown", codes.Unknown, false},
		{"InvalidArgument", codes.InvalidArgument, false},
		{"DeadlineExceeded", codes.DeadlineExceeded, true},
		{"NotFound", codes.NotFound, false},
		{"AlreadyExists", codes.AlreadyExists, false},
		{"PermissionDenied", codes.PermissionDenied, false},
		{"ResourceExhausted", codes.ResourceExhausted, true},
		{"FailedPrecondition", codes.FailedPrecondition, false},
		{"Aborted", codes.Aborted, true},
		{"OutOfRange", codes.OutOfRange, false},
		{"Unimplemented", codes.Unimplemented, false},
		{"Internal", codes.Internal, true},
		{"Unavailable", codes.Unavailable, true},
		{"DataLoss", codes.DataLoss, false},
		{"Unauthenticated", codes.Unauthenticated, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := status.Error(tt.code, "test error")
			assert.Equal(t, tt.retryable, isRetryable(err))
		})
	}

	t.Run("nil error is not retryable", func(t *testing.T) {
		assert.False(t, isRetryable(nil))
	})
}

func TestClient_GetModelStatus(t *testing.T) {
	mock := &mockAIServer{}
	server, addr := startTestServer(t, mock)
	defer server.Stop()

	client, err := NewClient(addr)
	require.NoError(t, err)
	defer client.Close()

	t.Run("nil request uses defaults", func(t *testing.T) {
		resp, err := client.GetModelStatus(context.Background(), nil)
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.True(t, resp.Healthy)
	})

	t.Run("successful status retrieval", func(t *testing.T) {
		resp, err := client.GetModelStatus(context.Background(), &aiv1.GetModelStatusRequest{})
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.NotEmpty(t, resp.Models)
		assert.Equal(t, "test-model", resp.DefaultEmbeddingModel)
	})
}

func TestClient_ClassifyContent(t *testing.T) {
	mock := &mockAIServer{}
	server, addr := startTestServer(t, mock)
	defer server.Stop()

	client, err := NewClient(addr)
	require.NoError(t, err)
	defer client.Close()

	t.Run("nil request returns error", func(t *testing.T) {
		_, err := client.ClassifyContent(context.Background(), nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "request is required")
	})

	t.Run("successful classification", func(t *testing.T) {
		resp, err := client.ClassifyContent(context.Background(), &aiv1.ClassifyContentRequest{
			Content:    "Meeting notes from the quarterly review",
			Categories: []string{"work", "personal"},
		})
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.NotEmpty(t, resp.Classifications)
		assert.Equal(t, "work", resp.Primary.Label)
	})
}

func TestClient_TriageContent(t *testing.T) {
	mock := &mockAIServer{}
	server, addr := startTestServer(t, mock)
	defer server.Stop()

	client, err := NewClient(addr)
	require.NoError(t, err)
	defer client.Close()

	t.Run("nil request returns error", func(t *testing.T) {
		_, err := client.TriageContent(context.Background(), nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "request is required")
	})

	t.Run("successful triage", func(t *testing.T) {
		subject := "Q4 Project Status"
		sender := "john@example.com"
		sourceID := int64(123)

		resp, err := client.TriageContent(context.Background(), &aiv1.TriageContentRequest{
			Content:  "The project is on track and we've completed all major milestones for Q4.",
			Subject:  &subject,
			Sender:   &sender,
			SourceId: &sourceID,
		})
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, "PROJECT_UPDATE", resp.Category)
		assert.Equal(t, "HIGH", resp.Importance)
		assert.NotEmpty(t, resp.Reason)
		assert.Equal(t, "test-model", resp.ModelUsed)
		assert.NotNil(t, resp.InputTokens)
		assert.NotNil(t, resp.OutputTokens)
		assert.Equal(t, int32(0), resp.Retries)
	})

	t.Run("triage with minimal context", func(t *testing.T) {
		resp, err := client.TriageContent(context.Background(), &aiv1.TriageContentRequest{
			Content: "Quick update on the project status.",
		})
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.NotEmpty(t, resp.Category)
		assert.NotEmpty(t, resp.Importance)
	})
}

func TestClient_ExtractEntities(t *testing.T) {
	mock := &mockAIServer{}
	server, addr := startTestServer(t, mock)
	defer server.Stop()

	client, err := NewClient(addr)
	require.NoError(t, err)
	defer client.Close()

	t.Run("nil request returns error", func(t *testing.T) {
		_, err := client.ExtractEntities(context.Background(), nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "request is required")
	})

	t.Run("successful entity extraction", func(t *testing.T) {
		triageCategory := "PROJECT_UPDATE"
		sourceID := int64(456)

		resp, err := client.ExtractEntities(context.Background(), &aiv1.ExtractEntitiesRequest{
			Content:        "Alice Smith is the engineer working on Project Alpha at Acme Corp. The deadline is January 15th. Bob should review the PR next week. We approved the budget. There is a potential delay.",
			TriageCategory: &triageCategory,
			SourceId:       &sourceID,
		})
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.NotEmpty(t, resp.People)
		assert.NotEmpty(t, resp.Dates)
		assert.NotEmpty(t, resp.Projects)
		assert.NotEmpty(t, resp.Organisations)
		assert.NotEmpty(t, resp.ActionItems)
		assert.NotEmpty(t, resp.Decisions)
		assert.NotEmpty(t, resp.Risks)
		assert.Equal(t, "test-model", resp.ModelUsed)
		assert.NotNil(t, resp.InputTokens)
		assert.NotNil(t, resp.OutputTokens)
		assert.Equal(t, int32(0), resp.Retries)
	})

	t.Run("extraction with minimal content", func(t *testing.T) {
		resp, err := client.ExtractEntities(context.Background(), &aiv1.ExtractEntitiesRequest{
			Content: "Simple content with no entities.",
		})
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.NotEmpty(t, resp.ModelUsed)
	})
}

func TestClient_DeepAnalyze(t *testing.T) {
	mock := &mockAIServer{}
	server, addr := startTestServer(t, mock)
	defer server.Stop()

	client, err := NewClient(addr)
	require.NoError(t, err)
	defer client.Close()

	t.Run("nil request returns error", func(t *testing.T) {
		_, err := client.DeepAnalyze(context.Background(), nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "request is required")
	})

	t.Run("successful deep analysis", func(t *testing.T) {
		resp, err := client.DeepAnalyze(context.Background(), &aiv1.DeepAnalyzeRequest{
			Content: "Alice Smith is working on Project Alpha at Acme Corp. Bob should review the PR next week. We approved the budget for Q1. There is a potential delay in delivery.",
			VerifiedPeople: []*aiv1.PersonEntity{
				{Name: "Alice Smith", Role: "Engineer"},
				{Name: "Bob", Role: "Reviewer"},
			},
			VerifiedProjects: []string{"Project Alpha"},
			PreliminaryActionItems: []*aiv1.ActionItemEntity{
				{Assignee: "Bob", Action: "Review PR", Due: "next week"},
			},
			BackgroundContext: "Project Alpha is a critical initiative for Q1 2024",
			TriageCategory:    "PROJECT_UPDATE",
			TriageImportance:  "HIGH",
		})
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.NotEmpty(t, resp.Summary)
		assert.NotNil(t, resp.Sentiment)
		assert.NotEmpty(t, resp.TopicMappings)
		assert.NotEmpty(t, resp.VerifiedActionItems)
		assert.NotEmpty(t, resp.VerifiedDecisions)
		assert.NotEmpty(t, resp.RiskReferences)
		assert.NotEmpty(t, resp.StrategicInsights)
		assert.NotEmpty(t, resp.ImplicitActionItems)
		assert.Equal(t, "gemini-2.5-pro", resp.ModelUsed)
		assert.NotNil(t, resp.InputTokens)
		assert.NotNil(t, resp.OutputTokens)

		// Verify sentiment structure
		assert.Equal(t, float32(0.3), resp.Sentiment.Score)
		assert.Equal(t, "neutral", resp.Sentiment.Label)
		assert.Greater(t, resp.Sentiment.Confidence, float32(0))

		// Verify action item has required context_excerpt
		assert.NotEmpty(t, resp.VerifiedActionItems[0].ContextExcerpt)

		// Verify decision has required context_excerpt
		assert.NotEmpty(t, resp.VerifiedDecisions[0].ContextExcerpt)

		// Verify risk has required context_excerpt
		assert.NotEmpty(t, resp.RiskReferences[0].ContextExcerpt)

		// Verify implicit action item has required context_excerpt
		assert.NotEmpty(t, resp.ImplicitActionItems[0].ContextExcerpt)
	})

	t.Run("deep analysis with minimal content", func(t *testing.T) {
		resp, err := client.DeepAnalyze(context.Background(), &aiv1.DeepAnalyzeRequest{
			Content:          "Simple project update.",
			TriageCategory:   "OTHER",
			TriageImportance: "LOW",
		})
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.NotEmpty(t, resp.ModelUsed)
	})
}

// Package server provides comprehensive tests for the API Gateway server.
package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/ai/v1"
	contentv1 "github.com/otherjamesbrown/penfold/api/proto/contentv1"
	gmailv1 "github.com/otherjamesbrown/penfold/api/proto/gmail/v1"
	relationshipv1 "github.com/otherjamesbrown/penfold/api/proto/relationship/v1"
	reviewv1 "github.com/otherjamesbrown/penfold/api/proto/reviewv1"
	searchv1 "github.com/otherjamesbrown/penfold/api/proto/searchv1"
	"github.com/otherjamesbrown/penfold/pkg/auth"
	pkghealth "github.com/otherjamesbrown/penfold/pkg/health"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/metrics"
	"github.com/otherjamesbrown/penfold/services/gateway/config"
	"github.com/otherjamesbrown/penfold/services/gateway/health"
	"github.com/otherjamesbrown/penfold/services/gateway/middleware"
	"github.com/otherjamesbrown/penfold/services/gateway/ratelimit"
	"github.com/otherjamesbrown/penfold/services/gateway/router"
)

// =============================================================================
// Test Fixtures and Setup
// =============================================================================

// testContext provides common test infrastructure.
type testContext struct {
	logger           logging.Logger
	metrics          *metrics.Metrics
	healthAggregator *health.Aggregator
	config           *config.GatewayConfig
}

// newTestContext creates a test context with initialized components.
func newTestContext(t *testing.T) *testContext {
	t.Helper()

	logger := logging.NewLogger(&logging.Config{
		Level:       logging.LevelDebug,
		ServiceName: "gateway-test",
		Environment: "test",
	})
	logging.SetGlobal(logger)

	m := metrics.NewMetrics("gateway-test", "penfold")
	// Ignore registration errors in tests as metrics may already be registered

	healthAggregator := health.NewAggregator(Version)
	healthAggregator.SetDefaultTimeout(1 * time.Second)

	cfg := &config.GatewayConfig{
		GRPCPort:         0, // Use random port
		HTTPPort:         0,
		AuthEnabled:      false,
		RateLimitEnabled: false,
	}

	return &testContext{
		logger:           logger,
		metrics:          m,
		healthAggregator: healthAggregator,
		config:           cfg,
	}
}

// =============================================================================
// Mock Backend Services
// =============================================================================

// mockSearchService implements searchv1.SearchServiceServer for testing.
type mockSearchService struct {
	searchv1.UnimplementedSearchServiceServer
	mu               sync.RWMutex
	searchCalls      int
	searchDelay      time.Duration
	searchError      error
	searchResponse   *searchv1.SearchResponse
	receivedMetadata metadata.MD
	receivedTenantID string
}

func (m *mockSearchService) Search(ctx context.Context, req *searchv1.SearchRequest) (*searchv1.SearchResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.searchCalls++

	// Capture metadata for verification
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		m.receivedMetadata = md
		if tenants := md.Get("x-tenant-id"); len(tenants) > 0 {
			m.receivedTenantID = tenants[0]
		}
	}

	// Simulate processing delay
	if m.searchDelay > 0 {
		select {
		case <-time.After(m.searchDelay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if m.searchError != nil {
		return nil, m.searchError
	}

	if m.searchResponse != nil {
		return m.searchResponse, nil
	}

	title := "Test Document"
	return &searchv1.SearchResponse{
		Results: []*searchv1.SearchResult{
			{DocumentId: "doc-1", Score: 0.95, Title: &title},
		},
		TotalCount: 1,
	}, nil
}

func (m *mockSearchService) SemanticSearch(ctx context.Context, req *searchv1.SemanticSearchRequest) (*searchv1.SearchResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.searchCalls++

	if m.searchError != nil {
		return nil, m.searchError
	}

	return &searchv1.SearchResponse{
		Results:    []*searchv1.SearchResult{},
		TotalCount: 0,
	}, nil
}

func (m *mockSearchService) getSearchCalls() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.searchCalls
}

// mockAIService implements aiv1.AICoordinatorServiceServer for testing.
type mockAIService struct {
	aiv1.UnimplementedAICoordinatorServiceServer
	mu             sync.RWMutex
	embeddingCalls int
	summaryCalls   int
	embeddingError error
	summaryError   error
}

func (m *mockAIService) GenerateEmbedding(ctx context.Context, req *aiv1.EmbeddingRequest) (*aiv1.EmbeddingResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.embeddingCalls++

	if m.embeddingError != nil {
		return nil, m.embeddingError
	}

	return &aiv1.EmbeddingResponse{
		Vector: []float32{0.1, 0.2, 0.3, 0.4, 0.5},
	}, nil
}

func (m *mockAIService) GenerateSummary(ctx context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.summaryCalls++

	if m.summaryError != nil {
		return nil, m.summaryError
	}

	return &aiv1.SummaryResponse{
		Summary: "This is a test summary",
	}, nil
}

// mockReviewService implements reviewv1.ReviewServiceServer for testing.
type mockReviewService struct {
	reviewv1.UnimplementedReviewServiceServer
	mu             sync.RWMutex
	dailyCalls     int
	approveCalls   int
	dailyError     error
	approveError   error
	dailyResponse  *reviewv1.GetDailyReviewResponse
}

func (m *mockReviewService) GetDailyReview(ctx context.Context, req *reviewv1.GetDailyReviewRequest) (*reviewv1.GetDailyReviewResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dailyCalls++

	if m.dailyError != nil {
		return nil, m.dailyError
	}

	if m.dailyResponse != nil {
		return m.dailyResponse, nil
	}

	return &reviewv1.GetDailyReviewResponse{}, nil
}

func (m *mockReviewService) ApproveItem(ctx context.Context, req *reviewv1.ApproveItemRequest) (*reviewv1.ApproveItemResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.approveCalls++

	if m.approveError != nil {
		return nil, m.approveError
	}

	return &reviewv1.ApproveItemResponse{}, nil
}

// mockContentService implements contentv1.ContentProcessorServiceServer for testing.
type mockContentService struct {
	contentv1.UnimplementedContentProcessorServiceServer
	mu           sync.RWMutex
	processCalls int
	getCalls     int
	processError error
	getError     error
}

func (m *mockContentService) ProcessContent(ctx context.Context, req *contentv1.ProcessContentRequest) (*contentv1.ProcessContentResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.processCalls++

	if m.processError != nil {
		return nil, m.processError
	}

	return &contentv1.ProcessContentResponse{
		JobId: "job-123",
	}, nil
}

func (m *mockContentService) GetContentItem(ctx context.Context, req *contentv1.GetContentItemRequest) (*contentv1.ContentItem, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getCalls++

	if m.getError != nil {
		return nil, m.getError
	}

	return &contentv1.ContentItem{
		Id:         req.ContentId,
		SourceType: "test",
	}, nil
}

// mockGmailService implements gmailv1.GmailConnectorServiceServer for testing.
type mockGmailService struct {
	gmailv1.UnimplementedGmailConnectorServiceServer
	mu        sync.RWMutex
	syncCalls int
	getCalls  int
	syncError error
	getError  error
}

func (m *mockGmailService) SyncEmails(ctx context.Context, req *gmailv1.SyncEmailsRequest) (*gmailv1.SyncEmailsResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.syncCalls++

	if m.syncError != nil {
		return nil, m.syncError
	}

	return &gmailv1.SyncEmailsResponse{
		SyncId: "sync-123",
	}, nil
}

func (m *mockGmailService) GetEmail(ctx context.Context, req *gmailv1.GetEmailRequest) (*gmailv1.Email, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getCalls++

	if m.getError != nil {
		return nil, m.getError
	}

	return &gmailv1.Email{
		Id:      req.EmailId,
		Subject: "Test Email",
	}, nil
}

// mockRelationshipService implements relationshipv1.RelationshipServiceServer for testing.
type mockRelationshipService struct {
	relationshipv1.UnimplementedRelationshipServiceServer
	mu            sync.RWMutex
	discoverCalls int
	listCalls     int
	discoverError error
	listError     error
}

func (m *mockRelationshipService) DiscoverRelationships(ctx context.Context, req *relationshipv1.DiscoverRelationshipsRequest) (*relationshipv1.DiscoverRelationshipsResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.discoverCalls++

	if m.discoverError != nil {
		return nil, m.discoverError
	}

	return &relationshipv1.DiscoverRelationshipsResponse{
		TotalDiscovered: 5,
	}, nil
}

func (m *mockRelationshipService) ListRelationships(ctx context.Context, req *relationshipv1.ListRelationshipsRequest) (*relationshipv1.ListRelationshipsResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listCalls++

	if m.listError != nil {
		return nil, m.listError
	}

	return &relationshipv1.ListRelationshipsResponse{}, nil
}

// mockHealthClient implements health.ServiceClient for testing.
type mockHealthClient struct {
	healthy bool
	delay   time.Duration
}

func (m *mockHealthClient) HealthCheck(ctx context.Context) error {
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if !m.healthy {
		return errors.New("service unhealthy")
	}
	return nil
}

// =============================================================================
// Test Helpers
// =============================================================================

// startMockServer starts a gRPC server with registered mock services.
func startMockServer(t *testing.T, registerFunc func(*grpc.Server)) (string, func()) {
	t.Helper()

	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err, "Failed to listen")

	server := grpc.NewServer()
	registerFunc(server)

	go func() {
		if err := server.Serve(lis); err != nil && err != grpc.ErrServerStopped {
			t.Logf("Server error: %v", err)
		}
	}()

	return lis.Addr().String(), func() {
		server.Stop()
		lis.Close()
	}
}

// dialMockServer creates a gRPC client connection to a mock server.
func dialMockServer(t *testing.T, addr string) *grpc.ClientConn {
	t.Helper()

	conn, err := grpc.Dial(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithTimeout(5*time.Second),
	)
	require.NoError(t, err, "Failed to dial mock server")

	return conn
}

// =============================================================================
// Server Tests
// =============================================================================

func TestNewGatewayServer(t *testing.T) {
	tc := newTestContext(t)

	server := NewGatewayServer(tc.config, tc.logger, tc.metrics, tc.healthAggregator)

	assert.NotNil(t, server)
	assert.NotNil(t, server.config)
	assert.NotNil(t, server.logger)
	assert.NotNil(t, server.metrics)
	assert.NotNil(t, server.healthAggregator)
	assert.False(t, server.startTime.IsZero())
}

func TestGatewayServer_Uptime(t *testing.T) {
	tc := newTestContext(t)

	server := NewGatewayServer(tc.config, tc.logger, tc.metrics, tc.healthAggregator)

	// Wait a bit to ensure uptime is measurable
	time.Sleep(10 * time.Millisecond)

	uptime := server.Uptime()
	assert.True(t, uptime >= 10*time.Millisecond, "Expected uptime >= 10ms, got %v", uptime)
}

func TestGatewayServer_HealthCheck_AllHealthy(t *testing.T) {
	tc := newTestContext(t)

	// Register healthy backend services
	tc.healthAggregator.RegisterService(health.ServiceConfig{
		Name:     "search",
		Client:   &mockHealthClient{healthy: true},
		Critical: true,
	})
	tc.healthAggregator.RegisterService(health.ServiceConfig{
		Name:     "ai",
		Client:   &mockHealthClient{healthy: true},
		Critical: false,
	})

	server := NewGatewayServer(tc.config, tc.logger, tc.metrics, tc.healthAggregator)

	resp, err := server.HealthCheck(context.Background(), &HealthCheckRequest{
		IncludeDependencies: true,
	})

	require.NoError(t, err)
	assert.True(t, resp.Healthy)
	assert.Equal(t, "Gateway is healthy", resp.Message)
	assert.Equal(t, Version, resp.Version)
	assert.NotNil(t, resp.Timestamp)
	assert.True(t, resp.UptimeSeconds >= 0)
	assert.Len(t, resp.Dependencies, 2)
}

func TestGatewayServer_HealthCheck_CriticalFailure(t *testing.T) {
	tc := newTestContext(t)

	// Register one healthy and one unhealthy critical service
	tc.healthAggregator.RegisterService(health.ServiceConfig{
		Name:     "healthy-service",
		Client:   &mockHealthClient{healthy: true},
		Critical: false,
	})
	tc.healthAggregator.RegisterService(health.ServiceConfig{
		Name:     "critical-service",
		Client:   &mockHealthClient{healthy: false},
		Critical: true,
	})

	server := NewGatewayServer(tc.config, tc.logger, tc.metrics, tc.healthAggregator)

	resp, err := server.HealthCheck(context.Background(), &HealthCheckRequest{
		IncludeDependencies: true,
	})

	require.NoError(t, err)
	assert.False(t, resp.Healthy)
	assert.Contains(t, resp.Message, "unhealthy")
}

func TestGatewayServer_HealthCheck_Degraded(t *testing.T) {
	tc := newTestContext(t)

	// Register healthy critical and unhealthy non-critical
	tc.healthAggregator.RegisterService(health.ServiceConfig{
		Name:     "critical-service",
		Client:   &mockHealthClient{healthy: true},
		Critical: true,
	})
	tc.healthAggregator.RegisterService(health.ServiceConfig{
		Name:     "non-critical-service",
		Client:   &mockHealthClient{healthy: false},
		Critical: false,
	})

	server := NewGatewayServer(tc.config, tc.logger, tc.metrics, tc.healthAggregator)

	resp, err := server.HealthCheck(context.Background(), &HealthCheckRequest{
		IncludeDependencies: true,
	})

	require.NoError(t, err)
	assert.True(t, resp.Healthy) // Degraded is still considered "healthy" for the gateway
	assert.Contains(t, resp.Message, "degraded")
}

func TestGatewayServer_HealthCheck_NoDependencies(t *testing.T) {
	tc := newTestContext(t)

	tc.healthAggregator.RegisterService(health.ServiceConfig{
		Name:     "service",
		Client:   &mockHealthClient{healthy: true},
		Critical: true,
	})

	server := NewGatewayServer(tc.config, tc.logger, tc.metrics, tc.healthAggregator)

	resp, err := server.HealthCheck(context.Background(), &HealthCheckRequest{
		IncludeDependencies: false,
	})

	require.NoError(t, err)
	assert.Empty(t, resp.Dependencies)
}

func TestGatewayServer_ProcessEmail_Unimplemented(t *testing.T) {
	tc := newTestContext(t)
	server := NewGatewayServer(tc.config, tc.logger, tc.metrics, tc.healthAggregator)

	_, err := server.ProcessEmail(context.Background(), &ProcessEmailRequest{
		TenantID:  "tenant-1",
		MessageID: "msg-1",
	})

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unimplemented, st.Code())
}

func TestGatewayServer_Search_Unimplemented(t *testing.T) {
	tc := newTestContext(t)
	server := NewGatewayServer(tc.config, tc.logger, tc.metrics, tc.healthAggregator)

	_, err := server.Search(context.Background(), &SearchRequest{
		TenantID: "tenant-1",
		Query:    "test query",
	})

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unimplemented, st.Code())
}

func TestGatewayServer_GetDailyReview_Unimplemented(t *testing.T) {
	tc := newTestContext(t)
	server := NewGatewayServer(tc.config, tc.logger, tc.metrics, tc.healthAggregator)

	_, err := server.GetDailyReview(context.Background(), &GetDailyReviewRequest{
		TenantID: "tenant-1",
		UserID:   "user-1",
	})

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unimplemented, st.Code())
}

// =============================================================================
// Auth Middleware Integration Tests
// =============================================================================

func TestAuthMiddleware_Integration_JWT(t *testing.T) {
	const testSecret = "test-secret-key-for-jwt-testing"

	jwtValidator := auth.NewJWTValidator(testSecret)
	authMiddleware := middleware.NewAuthMiddleware(&middleware.AuthConfig{
		JWTValidator: jwtValidator,
		SkipMethods:  []string{"/grpc.health.v1.Health/Check"},
	})

	// Generate a valid token
	claims := &auth.Claims{
		Subject: "test-user",
		Roles:   []string{"admin"},
		Custom: map[string]interface{}{
			"tenant_id": "tenant-123",
		},
	}
	token, err := jwtValidator.GenerateToken(claims, time.Hour)
	require.NoError(t, err)

	tests := []struct {
		name         string
		authHeader   string
		method       string
		expectError  bool
		expectedCode codes.Code
	}{
		{
			name:        "valid JWT",
			authHeader:  "Bearer " + token,
			method:      "/test.Service/Method",
			expectError: false,
		},
		{
			name:         "missing auth",
			authHeader:   "",
			method:       "/test.Service/Method",
			expectError:  true,
			expectedCode: codes.Unauthenticated,
		},
		{
			name:         "invalid token",
			authHeader:   "Bearer invalid-token",
			method:       "/test.Service/Method",
			expectError:  true,
			expectedCode: codes.Unauthenticated,
		},
		{
			name:        "skipped method",
			authHeader:  "",
			method:      "/grpc.health.v1.Health/Check",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interceptor := authMiddleware.UnaryInterceptor()

			md := metadata.New(map[string]string{})
			if tt.authHeader != "" {
				md.Set("authorization", tt.authHeader)
			}
			ctx := metadata.NewIncomingContext(context.Background(), md)

			info := &grpc.UnaryServerInfo{FullMethod: tt.method}
			handlerCalled := false
			handler := func(ctx context.Context, req interface{}) (interface{}, error) {
				handlerCalled = true
				return "success", nil
			}

			_, err := interceptor(ctx, nil, info, handler)

			if tt.expectError {
				require.Error(t, err)
				st, ok := status.FromError(err)
				require.True(t, ok)
				assert.Equal(t, tt.expectedCode, st.Code())
				assert.False(t, handlerCalled)
			} else {
				require.NoError(t, err)
				assert.True(t, handlerCalled)
			}
		})
	}
}

func TestAuthMiddleware_Integration_APIKey(t *testing.T) {
	apiKeyValidator := auth.NewAPIKeyValidator()
	apiKeyValidator.RegisterKey("valid-api-key", &auth.APIKeyInfo{
		ID:          "key-001",
		ServiceName: "test-service",
		Roles:       []string{"service"},
	})

	authMiddleware := middleware.NewAuthMiddleware(&middleware.AuthConfig{
		APIKeyValidator: apiKeyValidator,
	})

	tests := []struct {
		name         string
		apiKey       string
		expectError  bool
		expectedCode codes.Code
	}{
		{
			name:        "valid API key",
			apiKey:      "valid-api-key",
			expectError: false,
		},
		{
			name:         "invalid API key",
			apiKey:       "invalid-key",
			expectError:  true,
			expectedCode: codes.Unauthenticated,
		},
		{
			name:         "missing API key",
			apiKey:       "",
			expectError:  true,
			expectedCode: codes.Unauthenticated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interceptor := authMiddleware.UnaryInterceptor()

			md := metadata.New(map[string]string{})
			if tt.apiKey != "" {
				md.Set("x-api-key", tt.apiKey)
			}
			ctx := metadata.NewIncomingContext(context.Background(), md)

			info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}
			handlerCalled := false
			handler := func(ctx context.Context, req interface{}) (interface{}, error) {
				handlerCalled = true
				return "success", nil
			}

			_, err := interceptor(ctx, nil, info, handler)

			if tt.expectError {
				require.Error(t, err)
				st, ok := status.FromError(err)
				require.True(t, ok)
				assert.Equal(t, tt.expectedCode, st.Code())
			} else {
				require.NoError(t, err)
				assert.True(t, handlerCalled)
			}
		})
	}
}

// =============================================================================
// Rate Limiting Integration Tests
// =============================================================================

func TestRateLimiter_Integration_gRPC(t *testing.T) {
	rlConfig := ratelimit.Config{
		DefaultRPS:      10.0,
		DefaultBurst:    5,
		CleanupInterval: 1 * time.Minute,
		BucketTTL:       5 * time.Minute,
		TenantLimits:    make(map[string]ratelimit.TenantLimit),
		EndpointLimits:  make(map[string]ratelimit.EndpointLimit),
	}
	limiter := ratelimit.NewRateLimiter(rlConfig)
	defer limiter.Close()

	interceptor := ratelimit.UnaryServerInterceptor(limiter)

	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}
	handlerCalls := 0
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		handlerCalls++
		return "success", nil
	}

	// First 5 requests should succeed (burst capacity)
	for i := 0; i < 5; i++ {
		md := metadata.New(map[string]string{"x-tenant-id": "tenant-1"})
		ctx := metadata.NewIncomingContext(context.Background(), md)

		_, err := interceptor(ctx, nil, info, handler)
		assert.NoError(t, err, "Request %d should succeed", i+1)
	}

	assert.Equal(t, 5, handlerCalls)

	// Next request should be rate limited
	md := metadata.New(map[string]string{"x-tenant-id": "tenant-1"})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	_, err := interceptor(ctx, nil, info, handler)
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.ResourceExhausted, st.Code())
}

func TestRateLimiter_Integration_TenantIsolation(t *testing.T) {
	rlConfig := ratelimit.Config{
		DefaultRPS:      10.0,
		DefaultBurst:    3,
		CleanupInterval: 1 * time.Minute,
		BucketTTL:       5 * time.Minute,
		TenantLimits:    make(map[string]ratelimit.TenantLimit),
		EndpointLimits:  make(map[string]ratelimit.EndpointLimit),
	}
	limiter := ratelimit.NewRateLimiter(rlConfig)
	defer limiter.Close()

	interceptor := ratelimit.UnaryServerInterceptor(limiter)
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "success", nil
	}

	// Exhaust tenant-1's rate limit
	for i := 0; i < 3; i++ {
		md := metadata.New(map[string]string{"x-tenant-id": "tenant-1"})
		ctx := metadata.NewIncomingContext(context.Background(), md)
		interceptor(ctx, nil, info, handler)
	}

	// tenant-1 should be rate limited
	md1 := metadata.New(map[string]string{"x-tenant-id": "tenant-1"})
	ctx1 := metadata.NewIncomingContext(context.Background(), md1)
	_, err := interceptor(ctx1, nil, info, handler)
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.ResourceExhausted, st.Code())

	// tenant-2 should still have capacity
	md2 := metadata.New(map[string]string{"x-tenant-id": "tenant-2"})
	ctx2 := metadata.NewIncomingContext(context.Background(), md2)
	_, err = interceptor(ctx2, nil, info, handler)
	assert.NoError(t, err, "tenant-2 should not be rate limited")
}

func TestRateLimiter_Integration_HTTP(t *testing.T) {
	rlConfig := ratelimit.Config{
		DefaultRPS:      10.0,
		DefaultBurst:    3,
		CleanupInterval: 1 * time.Minute,
		BucketTTL:       5 * time.Minute,
		TenantLimits:    make(map[string]ratelimit.TenantLimit),
		EndpointLimits:  make(map[string]ratelimit.EndpointLimit),
	}
	limiter := ratelimit.NewRateLimiter(rlConfig)
	defer limiter.Close()

	middleware := ratelimit.HTTPMiddleware(limiter)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First 3 requests should succeed
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Header.Set("X-Tenant-ID", "tenant-http")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	}

	// Next request should be rate limited
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("X-Tenant-ID", "tenant-http")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("Retry-After"))
}

func TestRateLimiter_Integration_SkipMethods(t *testing.T) {
	rlConfig := ratelimit.Config{
		DefaultRPS:      10.0,
		DefaultBurst:    1, // Very low to trigger limit easily
		CleanupInterval: 1 * time.Minute,
		BucketTTL:       5 * time.Minute,
		TenantLimits:    make(map[string]ratelimit.TenantLimit),
		EndpointLimits:  make(map[string]ratelimit.EndpointLimit),
	}
	limiter := ratelimit.NewRateLimiter(rlConfig)
	defer limiter.Close()

	interceptor := ratelimit.UnaryServerInterceptorWithConfig(&ratelimit.GRPCInterceptorConfig{
		Limiter:     limiter,
		SkipMethods: []string{"/grpc.health.v1.Health/Check"},
	})

	healthInfo := &grpc.UnaryServerInfo{FullMethod: "/grpc.health.v1.Health/Check"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "success", nil
	}

	// Health checks should never be rate limited
	for i := 0; i < 10; i++ {
		md := metadata.New(map[string]string{"x-tenant-id": "tenant-1"})
		ctx := metadata.NewIncomingContext(context.Background(), md)
		_, err := interceptor(ctx, nil, healthInfo, handler)
		assert.NoError(t, err, "Health check %d should not be rate limited", i+1)
	}
}

// =============================================================================
// Router Integration Tests
// =============================================================================

func TestRouter_Integration_SearchRouting(t *testing.T) {
	mockSearch := &mockSearchService{}

	addr, cleanup := startMockServer(t, func(s *grpc.Server) {
		searchv1.RegisterSearchServiceServer(s, mockSearch)
	})
	defer cleanup()

	r := router.NewRouter()
	err := r.RegisterBackend(router.BackendSearch, addr)
	require.NoError(t, err)

	// Allow connection to establish
	time.Sleep(100 * time.Millisecond)

	// Route a search request
	ctx := context.Background()
	ctx = router.WithTenantID(ctx, "test-tenant")
	ctx = router.WithTraceID(ctx, "trace-123")

	req := &searchv1.SearchRequest{
		Query:    "test query",
		TenantId: "test-tenant",
	}

	resp, err := r.RouteSearch(ctx, req)
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Len(t, resp.Results, 1)

	// Verify context was propagated
	mockSearch.mu.RLock()
	defer mockSearch.mu.RUnlock()
	assert.Equal(t, 1, mockSearch.searchCalls)
	if len(mockSearch.receivedMetadata.Get("x-tenant-id")) > 0 {
		assert.Equal(t, "test-tenant", mockSearch.receivedMetadata.Get("x-tenant-id")[0])
	}
}

func TestRouter_Integration_AIRouting(t *testing.T) {
	mockAI := &mockAIService{}

	addr, cleanup := startMockServer(t, func(s *grpc.Server) {
		aiv1.RegisterAICoordinatorServiceServer(s, mockAI)
	})
	defer cleanup()

	r := router.NewRouter()
	err := r.RegisterBackend(router.BackendAI, addr)
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	ctx := context.Background()
	tenantID := "tenant-1"
	req := &aiv1.EmbeddingRequest{
		Text:     "test text for embedding",
		TenantId: &tenantID,
	}

	resp, err := r.RouteAIGenerateEmbedding(ctx, req)
	require.NoError(t, err)
	assert.Len(t, resp.Vector, 5)

	mockAI.mu.RLock()
	assert.Equal(t, 1, mockAI.embeddingCalls)
	mockAI.mu.RUnlock()
}

func TestRouter_Integration_BackendUnavailable(t *testing.T) {
	r := router.NewRouter()

	// Try to route without registering backend
	ctx := context.Background()
	req := &searchv1.SearchRequest{Query: "test"}

	_, err := r.RouteSearch(ctx, req)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	// Can be either NotFound (not registered) or Unavailable (registered but can't connect)
	assert.True(t, st.Code() == codes.NotFound || st.Code() == codes.Unavailable)
}

func TestRouter_Integration_MultipleBackends(t *testing.T) {
	mockSearch := &mockSearchService{}
	mockAI := &mockAIService{}
	mockReview := &mockReviewService{}

	searchAddr, searchCleanup := startMockServer(t, func(s *grpc.Server) {
		searchv1.RegisterSearchServiceServer(s, mockSearch)
	})
	defer searchCleanup()

	aiAddr, aiCleanup := startMockServer(t, func(s *grpc.Server) {
		aiv1.RegisterAICoordinatorServiceServer(s, mockAI)
	})
	defer aiCleanup()

	reviewAddr, reviewCleanup := startMockServer(t, func(s *grpc.Server) {
		reviewv1.RegisterReviewServiceServer(s, mockReview)
	})
	defer reviewCleanup()

	r := router.NewRouter()
	require.NoError(t, r.RegisterBackend(router.BackendSearch, searchAddr))
	require.NoError(t, r.RegisterBackend(router.BackendAI, aiAddr))
	require.NoError(t, r.RegisterBackend(router.BackendReview, reviewAddr))

	time.Sleep(100 * time.Millisecond)

	ctx := context.Background()

	// Test search routing
	searchResp, err := r.RouteSearch(ctx, &searchv1.SearchRequest{Query: "test"})
	require.NoError(t, err)
	assert.NotNil(t, searchResp)

	// Test AI routing
	tenantID := "tenant-1"
	aiResp, err := r.RouteAIGenerateEmbedding(ctx, &aiv1.EmbeddingRequest{Text: "test", TenantId: &tenantID})
	require.NoError(t, err)
	assert.NotNil(t, aiResp)

	// Test review routing
	reviewResp, err := r.RouteReviewGetDaily(ctx, &reviewv1.GetDailyReviewRequest{TenantId: &tenantID})
	require.NoError(t, err)
	assert.NotNil(t, reviewResp)

	// Verify all backends were called
	assert.Equal(t, 1, mockSearch.getSearchCalls())
	mockAI.mu.RLock()
	assert.Equal(t, 1, mockAI.embeddingCalls)
	mockAI.mu.RUnlock()
	mockReview.mu.RLock()
	assert.Equal(t, 1, mockReview.dailyCalls)
	mockReview.mu.RUnlock()
}

// =============================================================================
// End-to-End Tests
// =============================================================================

func TestE2E_FullRequestFlow(t *testing.T) {
	// Set up mock backend
	e2eTitle := "E2E Test Result"
	mockSearch := &mockSearchService{
		searchResponse: &searchv1.SearchResponse{
			Results: []*searchv1.SearchResult{
				{DocumentId: "e2e-doc-1", Score: 0.99, Title: &e2eTitle},
			},
			TotalCount: 1,
		},
	}

	searchAddr, searchCleanup := startMockServer(t, func(s *grpc.Server) {
		searchv1.RegisterSearchServiceServer(s, mockSearch)
	})
	defer searchCleanup()

	// Set up router
	r := router.NewRouter()
	require.NoError(t, r.RegisterBackend(router.BackendSearch, searchAddr))
	time.Sleep(100 * time.Millisecond)

	// Simulate a full request flow
	ctx := context.Background()
	ctx = router.WithTenantID(ctx, "e2e-tenant")
	ctx = router.WithTraceID(ctx, "e2e-trace-123")
	ctx = router.WithRequestID(ctx, "e2e-req-456")

	req := &searchv1.SearchRequest{
		Query:    "end to end test",
		TenantId: "e2e-tenant",
		Limit:    10,
	}

	resp, err := r.RouteSearch(ctx, req)
	require.NoError(t, err)

	// Verify response
	assert.Equal(t, int64(1), resp.TotalCount)
	assert.Len(t, resp.Results, 1)
	assert.Equal(t, "e2e-doc-1", resp.Results[0].DocumentId)

	// Verify metadata propagation
	mockSearch.mu.RLock()
	defer mockSearch.mu.RUnlock()

	assert.Equal(t, "e2e-tenant", mockSearch.receivedTenantID)
	if len(mockSearch.receivedMetadata.Get("x-trace-id")) > 0 {
		assert.Equal(t, "e2e-trace-123", mockSearch.receivedMetadata.Get("x-trace-id")[0])
	}
}

func TestE2E_ErrorPropagation(t *testing.T) {
	// Set up mock backend that returns errors
	mockSearch := &mockSearchService{
		searchError: status.Error(codes.InvalidArgument, "invalid query format"),
	}

	searchAddr, searchCleanup := startMockServer(t, func(s *grpc.Server) {
		searchv1.RegisterSearchServiceServer(s, mockSearch)
	})
	defer searchCleanup()

	r := router.NewRouter()
	require.NoError(t, r.RegisterBackend(router.BackendSearch, searchAddr))
	time.Sleep(100 * time.Millisecond)

	ctx := context.Background()
	req := &searchv1.SearchRequest{Query: "bad query"}

	_, err := r.RouteSearch(ctx, req)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "invalid query format")
}

func TestE2E_ContextPropagation(t *testing.T) {
	mockSearch := &mockSearchService{}

	searchAddr, searchCleanup := startMockServer(t, func(s *grpc.Server) {
		searchv1.RegisterSearchServiceServer(s, mockSearch)
	})
	defer searchCleanup()

	r := router.NewRouter()
	require.NoError(t, r.RegisterBackend(router.BackendSearch, searchAddr))
	time.Sleep(100 * time.Millisecond)

	// Create context with all propagated values
	ctx := context.Background()
	ctx = router.WithTenantID(ctx, "ctx-tenant")
	ctx = router.WithTraceID(ctx, "ctx-trace")
	ctx = router.WithRequestID(ctx, "ctx-request")

	req := &searchv1.SearchRequest{Query: "context test"}
	_, err := r.RouteSearch(ctx, req)
	require.NoError(t, err)

	// Verify all context values were propagated
	mockSearch.mu.RLock()
	defer mockSearch.mu.RUnlock()

	md := mockSearch.receivedMetadata
	assert.Equal(t, "ctx-tenant", getMetadataValue(md, "x-tenant-id"))
	assert.Equal(t, "ctx-trace", getMetadataValue(md, "x-trace-id"))
	assert.Equal(t, "ctx-request", getMetadataValue(md, "x-request-id"))
}

func getMetadataValue(md metadata.MD, key string) string {
	values := md.Get(key)
	if len(values) > 0 {
		return values[0]
	}
	return ""
}

// =============================================================================
// Load Tests
// =============================================================================

func TestLoad_ConcurrentRequests(t *testing.T) {
	mockSearch := &mockSearchService{}

	searchAddr, searchCleanup := startMockServer(t, func(s *grpc.Server) {
		searchv1.RegisterSearchServiceServer(s, mockSearch)
	})
	defer searchCleanup()

	r := router.NewRouter()
	require.NoError(t, r.RegisterBackend(router.BackendSearch, searchAddr))
	time.Sleep(100 * time.Millisecond)

	const numRequests = 100
	var wg sync.WaitGroup
	errors := make(chan error, numRequests)

	start := time.Now()

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()

			ctx := context.Background()
			ctx = router.WithTenantID(ctx, "load-tenant")

			req := &searchv1.SearchRequest{
				Query:    "concurrent test",
				TenantId: "load-tenant",
			}

			_, err := r.RouteSearch(ctx, req)
			if err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	elapsed := time.Since(start)
	t.Logf("Processed %d requests in %v (%.2f req/s)", numRequests, elapsed, float64(numRequests)/elapsed.Seconds())

	var errorCount int
	for err := range errors {
		errorCount++
		t.Logf("Request error: %v", err)
	}

	assert.Equal(t, 0, errorCount, "Expected no errors in concurrent requests")
	assert.Equal(t, numRequests, mockSearch.getSearchCalls())
}

func TestLoad_RateLimiterUnderLoad(t *testing.T) {
	rlConfig := ratelimit.Config{
		DefaultRPS:      100.0,
		DefaultBurst:    50,
		CleanupInterval: 1 * time.Minute,
		BucketTTL:       5 * time.Minute,
		TenantLimits:    make(map[string]ratelimit.TenantLimit),
		EndpointLimits:  make(map[string]ratelimit.EndpointLimit),
	}
	limiter := ratelimit.NewRateLimiter(rlConfig)
	defer limiter.Close()

	interceptor := ratelimit.UnaryServerInterceptor(limiter)
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "success", nil
	}

	const numRequests = 200
	var wg sync.WaitGroup
	var successCount, rateLimitedCount atomic.Int64

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			md := metadata.New(map[string]string{"x-tenant-id": "load-tenant"})
			ctx := metadata.NewIncomingContext(context.Background(), md)

			_, err := interceptor(ctx, nil, info, handler)
			if err == nil {
				successCount.Add(1)
			} else {
				st, ok := status.FromError(err)
				if ok && st.Code() == codes.ResourceExhausted {
					rateLimitedCount.Add(1)
				}
			}
		}()
	}

	wg.Wait()

	t.Logf("Success: %d, Rate Limited: %d", successCount.Load(), rateLimitedCount.Load())

	// Should have some successful and some rate-limited requests
	assert.True(t, successCount.Load() > 0, "Should have some successful requests")
	// With burst of 50 and RPS of 100, and instant requests, we expect some rate limiting
	assert.True(t, successCount.Load()+rateLimitedCount.Load() == int64(numRequests), "All requests should be accounted for")
}

func TestLoad_ConnectionPoolBehavior(t *testing.T) {
	mockSearch := &mockSearchService{
		searchDelay: 5 * time.Millisecond, // Simulate some processing time
	}

	searchAddr, searchCleanup := startMockServer(t, func(s *grpc.Server) {
		searchv1.RegisterSearchServiceServer(s, mockSearch)
	})
	defer searchCleanup()

	r := router.NewRouter()
	require.NoError(t, r.RegisterBackend(router.BackendSearch, searchAddr))
	time.Sleep(100 * time.Millisecond)

	const numRequests = 20
	var wg sync.WaitGroup
	var successCount atomic.Int64

	start := time.Now()

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			ctx := context.Background()
			ctx = router.WithTenantID(ctx, "pool-tenant")

			req := &searchv1.SearchRequest{Query: "pool test"}
			_, err := r.RouteSearch(ctx, req)
			if err == nil {
				successCount.Add(1)
			}
		}()
	}

	wg.Wait()

	elapsed := time.Since(start)
	t.Logf("Processed %d requests in %v with connection pooling", numRequests, elapsed)

	assert.Equal(t, int64(numRequests), successCount.Load())
	// With 5ms delay per request and connection pooling, 20 requests should complete
	// much faster than 100ms (20 * 5ms serial) - but allow some overhead
	assert.Less(t, elapsed, 200*time.Millisecond, "Connection pooling should enable parallel processing")
}

// =============================================================================
// Graceful Degradation Tests
// =============================================================================

func TestGracefulDegradation_BackendFailure(t *testing.T) {
	mockSearch := &mockSearchService{
		searchError: status.Error(codes.Unavailable, "backend temporarily unavailable"),
	}

	searchAddr, searchCleanup := startMockServer(t, func(s *grpc.Server) {
		searchv1.RegisterSearchServiceServer(s, mockSearch)
	})
	defer searchCleanup()

	r := router.NewRouter()
	require.NoError(t, r.RegisterBackend(router.BackendSearch, searchAddr))
	time.Sleep(100 * time.Millisecond)

	ctx := context.Background()
	req := &searchv1.SearchRequest{Query: "test"}

	_, err := r.RouteSearch(ctx, req)
	require.Error(t, err)

	// Should propagate the backend error
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unavailable, st.Code())
}

func TestGracefulDegradation_BackendTimeout(t *testing.T) {
	mockSearch := &mockSearchService{
		searchDelay: 5 * time.Second, // Long delay to trigger timeout
	}

	searchAddr, searchCleanup := startMockServer(t, func(s *grpc.Server) {
		searchv1.RegisterSearchServiceServer(s, mockSearch)
	})
	defer searchCleanup()

	r := router.NewRouter()
	require.NoError(t, r.RegisterBackend(router.BackendSearch, searchAddr))
	time.Sleep(100 * time.Millisecond)

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	req := &searchv1.SearchRequest{Query: "timeout test"}

	start := time.Now()
	_, err := r.RouteSearch(ctx, req)
	elapsed := time.Since(start)

	require.Error(t, err)
	// Should timeout quickly, not wait for full 5 seconds
	assert.Less(t, elapsed, 500*time.Millisecond)
}

func TestGracefulDegradation_PartialBackendFailure(t *testing.T) {
	// One healthy backend
	mockSearch := &mockSearchService{}
	searchAddr, searchCleanup := startMockServer(t, func(s *grpc.Server) {
		searchv1.RegisterSearchServiceServer(s, mockSearch)
	})
	defer searchCleanup()

	// One failing backend
	mockAI := &mockAIService{
		embeddingError: status.Error(codes.Unavailable, "AI service down"),
	}
	aiAddr, aiCleanup := startMockServer(t, func(s *grpc.Server) {
		aiv1.RegisterAICoordinatorServiceServer(s, mockAI)
	})
	defer aiCleanup()

	r := router.NewRouter()
	require.NoError(t, r.RegisterBackend(router.BackendSearch, searchAddr))
	require.NoError(t, r.RegisterBackend(router.BackendAI, aiAddr))
	time.Sleep(100 * time.Millisecond)

	ctx := context.Background()

	// Search should still work
	searchResp, err := r.RouteSearch(ctx, &searchv1.SearchRequest{Query: "test"})
	require.NoError(t, err)
	assert.NotNil(t, searchResp)

	// AI should fail gracefully
	tenantID := "tenant-1"
	_, err = r.RouteAIGenerateEmbedding(ctx, &aiv1.EmbeddingRequest{Text: "test", TenantId: &tenantID})
	require.Error(t, err)

	// Verify search wasn't affected
	assert.Equal(t, 1, mockSearch.getSearchCalls())
}

func TestGracefulDegradation_HealthAggregation(t *testing.T) {
	healthAggregator := health.NewAggregator("1.0.0")
	healthAggregator.SetDefaultTimeout(100 * time.Millisecond)

	// Register healthy and unhealthy services
	healthAggregator.RegisterService(health.ServiceConfig{
		Name:     "healthy-critical",
		Client:   &mockHealthClient{healthy: true},
		Critical: true,
	})
	healthAggregator.RegisterService(health.ServiceConfig{
		Name:     "unhealthy-optional",
		Client:   &mockHealthClient{healthy: false},
		Critical: false,
	})

	// Check aggregated health
	aggHealth := healthAggregator.CheckAll(context.Background())

	// Should be degraded (not unhealthy) since only non-critical service failed
	assert.Equal(t, pkghealth.StatusDegraded, aggHealth.Status)
	assert.Len(t, aggHealth.Services, 2)

	// Verify individual service statuses
	var healthyFound, unhealthyFound bool
	for _, svc := range aggHealth.Services {
		if svc.Name == "healthy-critical" {
			healthyFound = true
			assert.Equal(t, pkghealth.StatusHealthy, svc.Status)
		}
		if svc.Name == "unhealthy-optional" {
			unhealthyFound = true
			assert.Equal(t, pkghealth.StatusUnhealthy, svc.Status)
		}
	}
	assert.True(t, healthyFound && unhealthyFound)
}

func TestGracefulDegradation_CircuitBreaker(t *testing.T) {
	healthAggregator := health.NewAggregator("1.0.0")
	healthAggregator.SetDefaultTimeout(50 * time.Millisecond)

	// Register a service that always fails
	healthAggregator.RegisterService(health.ServiceConfig{
		Name:     "flaky-service",
		Client:   &mockHealthClient{healthy: false},
		Critical: true,
	})

	// Trigger circuit breaker by making multiple failed checks
	for i := 0; i < 10; i++ {
		healthAggregator.CheckAll(context.Background())
	}

	// Circuit should be open - check health should reflect this
	aggHealth := healthAggregator.CheckAll(context.Background())
	assert.Equal(t, pkghealth.StatusUnhealthy, aggHealth.Status)
}

// =============================================================================
// Integration Test Helpers
// =============================================================================

func TestHTTPHealthEndpoints(t *testing.T) {
	healthAggregator := health.NewAggregator("1.0.0")
	healthAggregator.RegisterService(health.ServiceConfig{
		Name:     "test-service",
		Client:   &mockHealthClient{healthy: true},
		Critical: true,
	})

	tests := []struct {
		name           string
		endpoint       string
		handler        http.Handler
		expectedStatus int
	}{
		{
			name:           "health endpoint - healthy",
			endpoint:       "/health",
			handler:        healthAggregator.Handler(),
			expectedStatus: http.StatusOK,
		},
		{
			name:           "ready endpoint - ready",
			endpoint:       "/ready",
			handler:        healthAggregator.ReadyHandler(),
			expectedStatus: http.StatusOK,
		},
		{
			name:           "live endpoint",
			endpoint:       "/live",
			handler:        healthAggregator.LiveHandler(),
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.endpoint, nil)
			rec := httptest.NewRecorder()

			tt.handler.ServeHTTP(rec, req)

			assert.Equal(t, tt.expectedStatus, rec.Code)
			assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
		})
	}
}

func TestHTTPHealthEndpoints_Unhealthy(t *testing.T) {
	healthAggregator := health.NewAggregator("1.0.0")
	healthAggregator.RegisterService(health.ServiceConfig{
		Name:     "failing-service",
		Client:   &mockHealthClient{healthy: false},
		Critical: true,
	})

	// Health endpoint should return 503
	healthReq := httptest.NewRequest(http.MethodGet, "/health", nil)
	healthRec := httptest.NewRecorder()
	healthAggregator.Handler().ServeHTTP(healthRec, healthReq)
	assert.Equal(t, http.StatusServiceUnavailable, healthRec.Code)

	// Ready endpoint should return 503
	readyReq := httptest.NewRequest(http.MethodGet, "/ready", nil)
	readyRec := httptest.NewRecorder()
	healthAggregator.ReadyHandler().ServeHTTP(readyRec, readyReq)
	assert.Equal(t, http.StatusServiceUnavailable, readyRec.Code)

	// Live endpoint should still return 200 (gateway is running)
	liveReq := httptest.NewRequest(http.MethodGet, "/live", nil)
	liveRec := httptest.NewRecorder()
	healthAggregator.LiveHandler().ServeHTTP(liveRec, liveReq)
	assert.Equal(t, http.StatusOK, liveRec.Code)
}

// =============================================================================
// Benchmark Tests
// =============================================================================

func BenchmarkRouter_RouteSearch(b *testing.B) {
	mockSearch := &mockSearchService{}

	addr, cleanup := startMockServer(&testing.T{}, func(s *grpc.Server) {
		searchv1.RegisterSearchServiceServer(s, mockSearch)
	})
	defer cleanup()

	r := router.NewRouter()
	r.RegisterBackend(router.BackendSearch, addr)
	time.Sleep(100 * time.Millisecond)

	ctx := context.Background()
	ctx = router.WithTenantID(ctx, "bench-tenant")

	req := &searchv1.SearchRequest{Query: "benchmark test"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.RouteSearch(ctx, req)
	}
}

func BenchmarkRateLimiter_Allow(b *testing.B) {
	rlConfig := ratelimit.Config{
		DefaultRPS:      1000000.0, // Very high to avoid limiting during benchmark
		DefaultBurst:    1000000,
		CleanupInterval: 1 * time.Hour,
		BucketTTL:       1 * time.Hour,
		TenantLimits:    make(map[string]ratelimit.TenantLimit),
		EndpointLimits:  make(map[string]ratelimit.EndpointLimit),
	}
	limiter := ratelimit.NewRateLimiter(rlConfig)
	defer limiter.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.Allow(ctx, "bench-tenant", "/test.Service/Method")
	}
}

func BenchmarkContextPropagation(b *testing.B) {
	ctx := context.Background()
	ctx = router.WithTenantID(ctx, "bench-tenant")
	ctx = router.WithTraceID(ctx, "bench-trace")
	ctx = router.WithRequestID(ctx, "bench-request")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		router.PropagateContext(ctx)
	}
}

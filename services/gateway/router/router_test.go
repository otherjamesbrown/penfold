package router

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	contentv1 "github.com/otherjamesbrown/penfold/api/proto/content/v1"
	gmailv1 "github.com/otherjamesbrown/penfold/api/proto/gmail/v1"
	relationshipv1 "github.com/otherjamesbrown/penfold/api/proto/relationship/v1"
	reviewv1 "github.com/otherjamesbrown/penfold/api/proto/review/v1"
	searchv1 "github.com/otherjamesbrown/penfold/api/proto/search/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// mockSearchServer implements the SearchServiceServer interface for testing.
type mockSearchServer struct {
	searchv1.UnimplementedSearchServiceServer
	searchCalled     bool
	lastSearchReq    *searchv1.SearchRequest
	searchResponse   *searchv1.SearchResponse
	searchError      error
	mu               sync.Mutex
	receivedMetadata metadata.MD
}

func (m *mockSearchServer) Search(ctx context.Context, req *searchv1.SearchRequest) (*searchv1.SearchResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.searchCalled = true
	m.lastSearchReq = req

	// Capture incoming metadata.
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		m.receivedMetadata = md
	}

	if m.searchError != nil {
		return nil, m.searchError
	}
	if m.searchResponse != nil {
		return m.searchResponse, nil
	}
	return &searchv1.SearchResponse{
		Results: []*searchv1.SearchResult{
			{
				DocumentId: "test-doc-1",
				Score:      0.95,
			},
		},
	}, nil
}

// mockReviewServer implements the ReviewServiceServer interface for testing.
type mockReviewServer struct {
	reviewv1.UnimplementedReviewServiceServer
	getDailyReviewCalled bool
	lastDailyReviewReq   *reviewv1.GetDailyReviewRequest
	dailyReviewResponse  *reviewv1.GetDailyReviewResponse
	dailyReviewError     error
	mu                   sync.Mutex
	receivedMetadata     metadata.MD
}

func (m *mockReviewServer) GetDailyReview(ctx context.Context, req *reviewv1.GetDailyReviewRequest) (*reviewv1.GetDailyReviewResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getDailyReviewCalled = true
	m.lastDailyReviewReq = req

	// Capture incoming metadata.
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		m.receivedMetadata = md
	}

	if m.dailyReviewError != nil {
		return nil, m.dailyReviewError
	}
	if m.dailyReviewResponse != nil {
		return m.dailyReviewResponse, nil
	}
	return &reviewv1.GetDailyReviewResponse{}, nil
}

// startTestServer starts a gRPC server for testing and returns the address and cleanup function.
func startTestServer(t *testing.T, registerFunc func(*grpc.Server)) (string, func()) {
	t.Helper()

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}

	server := grpc.NewServer()
	registerFunc(server)

	go func() {
		if err := server.Serve(lis); err != nil && err != grpc.ErrServerStopped {
			t.Logf("Server error: %v", err)
		}
	}()

	return lis.Addr().String(), func() {
		server.Stop()
		_ = lis.Close()
	}
}

func TestNewRouter(t *testing.T) {
	r := NewRouter()
	if r == nil {
		t.Fatal("Expected non-nil router")
	}
	if r.backends == nil {
		t.Fatal("Expected backends map to be initialized")
	}
	if len(r.dialOpts) == 0 {
		t.Fatal("Expected default dial options to be set")
	}
}

func TestNewRouterWithOptions(t *testing.T) {
	logger := logging.NewLogger(&logging.Config{
		Level:       logging.LevelDebug,
		ServiceName: "test",
	})

	r := NewRouter(
		WithLogger(logger),
	)

	if r == nil {
		t.Fatal("Expected non-nil router")
	}
}

func TestRegisterBackend(t *testing.T) {
	r := NewRouter()

	// Register a backend (will fail to connect but should still register).
	err := r.RegisterBackend("test-backend", "localhost:99999")
	if err != nil {
		t.Fatalf("RegisterBackend should not return error even with unreachable address: %v", err)
	}

	// Check backend is registered.
	r.mu.RLock()
	_, exists := r.backends["test-backend"]
	r.mu.RUnlock()

	if !exists {
		t.Fatal("Expected backend to be registered")
	}

	// Try to register the same backend again - should fail.
	err = r.RegisterBackend("test-backend", "localhost:99999")
	if err == nil {
		t.Fatal("Expected error when registering duplicate backend")
	}
}

func TestRegisterBackendWithConfig(t *testing.T) {
	r := NewRouter()

	config := &BackendConfig{
		Address:           "localhost:99999",
		MaxRetries:        1,
		RetryDelay:        100 * time.Millisecond,
		ConnectionTimeout: 500 * time.Millisecond,
		RequestTimeout:    5 * time.Second,
	}

	err := r.RegisterBackendWithConfig("custom-backend", config)
	if err != nil {
		t.Fatalf("RegisterBackendWithConfig should not return error: %v", err)
	}

	r.mu.RLock()
	backend, exists := r.backends["custom-backend"]
	r.mu.RUnlock()

	if !exists {
		t.Fatal("Expected backend to be registered")
	}

	if backend.config.MaxRetries != 1 {
		t.Errorf("Expected MaxRetries=1, got %d", backend.config.MaxRetries)
	}
}

func TestDefaultBackendConfig(t *testing.T) {
	config := DefaultBackendConfig("localhost:50051")

	if config.Address != "localhost:50051" {
		t.Errorf("Expected address localhost:50051, got %s", config.Address)
	}
	if config.MaxRetries != 3 {
		t.Errorf("Expected MaxRetries=3, got %d", config.MaxRetries)
	}
	if config.RetryDelay != time.Second {
		t.Errorf("Expected RetryDelay=1s, got %v", config.RetryDelay)
	}
	if config.ConnectionTimeout != 10*time.Second {
		t.Errorf("Expected ConnectionTimeout=10s, got %v", config.ConnectionTimeout)
	}
	if config.RequestTimeout != 30*time.Second {
		t.Errorf("Expected RequestTimeout=30s, got %v", config.RequestTimeout)
	}
}

func TestSearchClientWithMockServer(t *testing.T) {
	mockServer := &mockSearchServer{}

	addr, cleanup := startTestServer(t, func(s *grpc.Server) {
		searchv1.RegisterSearchServiceServer(s, mockServer)
	})
	defer cleanup()

	r := NewRouter()
	err := r.RegisterBackend(BackendSearch, addr)
	if err != nil {
		t.Fatalf("Failed to register backend: %v", err)
	}

	// Give the connection time to establish.
	time.Sleep(100 * time.Millisecond)

	client, err := r.SearchClient()
	if err != nil {
		t.Fatalf("Failed to get search client: %v", err)
	}

	if client == nil {
		t.Fatal("Expected non-nil client")
	}
}

func TestRouteSearchWithMockServer(t *testing.T) {
	mockServer := &mockSearchServer{
		searchResponse: &searchv1.SearchResponse{
			Results: []*searchv1.SearchResult{
				{DocumentId: "doc-123", Score: 0.9},
			},
		},
	}

	addr, cleanup := startTestServer(t, func(s *grpc.Server) {
		searchv1.RegisterSearchServiceServer(s, mockServer)
	})
	defer cleanup()

	r := NewRouter()
	err := r.RegisterBackend(BackendSearch, addr)
	if err != nil {
		t.Fatalf("Failed to register backend: %v", err)
	}

	// Give the connection time to establish.
	time.Sleep(100 * time.Millisecond)

	// Create context with tenant_id and trace_id.
	ctx := context.Background()
	ctx = WithTenantID(ctx, "test-tenant")
	ctx = WithTraceID(ctx, "trace-123")
	ctx = WithRequestID(ctx, "req-456")

	req := &searchv1.SearchRequest{
		Query:    "test query",
		TenantId: "test-tenant",
	}

	resp, err := r.RouteSearch(ctx, req)
	if err != nil {
		t.Fatalf("RouteSearch failed: %v", err)
	}

	if resp == nil {
		t.Fatal("Expected non-nil response")
	}

	if len(resp.Results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(resp.Results))
	}

	if resp.Results[0].DocumentId != "doc-123" {
		t.Errorf("Expected document_id=doc-123, got %s", resp.Results[0].DocumentId)
	}

	// Verify the mock was called.
	mockServer.mu.Lock()
	if !mockServer.searchCalled {
		t.Error("Expected Search to be called on mock server")
	}

	// Verify context propagation.
	if tenantID := mockServer.receivedMetadata.Get(MetadataTenantID); len(tenantID) == 0 || tenantID[0] != "test-tenant" {
		t.Errorf("Expected tenant_id=test-tenant in metadata, got %v", tenantID)
	}
	if traceID := mockServer.receivedMetadata.Get(MetadataTraceID); len(traceID) == 0 || traceID[0] != "trace-123" {
		t.Errorf("Expected trace_id=trace-123 in metadata, got %v", traceID)
	}
	if requestID := mockServer.receivedMetadata.Get(MetadataRequestID); len(requestID) == 0 || requestID[0] != "req-456" {
		t.Errorf("Expected request_id=req-456 in metadata, got %v", requestID)
	}
	mockServer.mu.Unlock()
}

func TestRouteSearchBackendUnavailable(t *testing.T) {
	r := NewRouter()

	// Don't register any backend.
	ctx := context.Background()
	req := &searchv1.SearchRequest{
		Query: "test query",
	}

	_, err := r.RouteSearch(ctx, req)
	if err == nil {
		t.Fatal("Expected error when backend not registered")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("Expected gRPC status error")
	}

	// Gateway returns Unavailable when backend is not reachable (even if not registered).
	if st.Code() != codes.Unavailable {
		t.Errorf("Expected Unavailable code, got %v", st.Code())
	}
}

func TestRouteReviewGetDailyWithMockServer(t *testing.T) {
	mockServer := &mockReviewServer{
		dailyReviewResponse: &reviewv1.GetDailyReviewResponse{},
	}

	addr, cleanup := startTestServer(t, func(s *grpc.Server) {
		reviewv1.RegisterReviewServiceServer(s, mockServer)
	})
	defer cleanup()

	r := NewRouter()
	err := r.RegisterBackend(BackendReview, addr)
	if err != nil {
		t.Fatalf("Failed to register backend: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	ctx := context.Background()
	ctx = WithTenantID(ctx, "tenant-1")

	tenantID := "tenant-1"
	req := &reviewv1.GetDailyReviewRequest{
		TenantId: &tenantID,
	}

	resp, err := r.RouteReviewGetDaily(ctx, req)
	if err != nil {
		t.Fatalf("RouteReviewGetDaily failed: %v", err)
	}

	if resp == nil {
		t.Fatal("Expected non-nil response")
	}

	mockServer.mu.Lock()
	if !mockServer.getDailyReviewCalled {
		t.Error("Expected GetDailyReview to be called on mock server")
	}
	mockServer.mu.Unlock()
}

func TestContextPropagation(t *testing.T) {
	ctx := context.Background()

	// Set values using helper functions.
	ctx = WithTenantID(ctx, "tenant-123")
	ctx = WithTraceID(ctx, "trace-456")
	ctx = WithRequestID(ctx, "request-789")

	// Verify values can be retrieved.
	if got := GetTenantID(ctx); got != "tenant-123" {
		t.Errorf("Expected tenant_id=tenant-123, got %s", got)
	}
	if got := GetTraceID(ctx); got != "trace-456" {
		t.Errorf("Expected trace_id=trace-456, got %s", got)
	}

	// Test PropagateContext creates proper metadata.
	outCtx := PropagateContext(ctx)
	md, ok := metadata.FromOutgoingContext(outCtx)
	if !ok {
		t.Fatal("Expected outgoing metadata")
	}

	if values := md.Get(MetadataTenantID); len(values) == 0 || values[0] != "tenant-123" {
		t.Errorf("Expected x-tenant-id=tenant-123, got %v", values)
	}
	if values := md.Get(MetadataTraceID); len(values) == 0 || values[0] != "trace-456" {
		t.Errorf("Expected x-trace-id=trace-456, got %v", values)
	}
	if values := md.Get(MetadataRequestID); len(values) == 0 || values[0] != "request-789" {
		t.Errorf("Expected x-request-id=request-789, got %v", values)
	}
}

func TestGetTenantIDFromMetadata(t *testing.T) {
	// Create a context with incoming gRPC metadata.
	md := metadata.Pairs(MetadataTenantID, "tenant-from-metadata")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	if got := GetTenantID(ctx); got != "tenant-from-metadata" {
		t.Errorf("Expected tenant_id=tenant-from-metadata, got %s", got)
	}
}

func TestGetTraceIDFromMetadata(t *testing.T) {
	md := metadata.Pairs(MetadataTraceID, "trace-from-metadata")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	if got := GetTraceID(ctx); got != "trace-from-metadata" {
		t.Errorf("Expected trace_id=trace-from-metadata, got %s", got)
	}
}

func TestBackendStatuses(t *testing.T) {
	r := NewRouter()

	// Register backends (will fail to connect).
	err := r.RegisterBackend("backend-1", "localhost:99991")
	if err != nil {
		t.Fatalf("Failed to register backend-1: %v", err)
	}
	err = r.RegisterBackend("backend-2", "localhost:99992")
	if err != nil {
		t.Fatalf("Failed to register backend-2: %v", err)
	}

	statuses := r.BackendStatuses()
	if len(statuses) != 2 {
		t.Errorf("Expected 2 backend statuses, got %d", len(statuses))
	}

	// Verify both backends are in the list.
	names := make(map[string]bool)
	for _, s := range statuses {
		names[s.Name] = true
	}

	if !names["backend-1"] || !names["backend-2"] {
		t.Error("Expected both backends in status list")
	}
}

func TestIsBackendHealthy(t *testing.T) {
	mockServer := &mockSearchServer{}

	addr, cleanup := startTestServer(t, func(s *grpc.Server) {
		searchv1.RegisterSearchServiceServer(s, mockServer)
	})
	defer cleanup()

	r := NewRouter()
	err := r.RegisterBackend(BackendSearch, addr)
	if err != nil {
		t.Fatalf("Failed to register backend: %v", err)
	}

	// Give time for connection to establish.
	time.Sleep(100 * time.Millisecond)

	// Check non-existent backend.
	if r.IsBackendHealthy("nonexistent") {
		t.Error("Expected IsBackendHealthy to return false for non-existent backend")
	}

	// Check registered backend - it should be connected.
	healthy := r.IsBackendHealthy(BackendSearch)
	// May or may not be healthy depending on timing, but should not panic.
	t.Logf("Search backend healthy: %v", healthy)
}

func TestUnregisterBackend(t *testing.T) {
	r := NewRouter()

	err := r.RegisterBackend("temp-backend", "localhost:99999")
	if err != nil {
		t.Fatalf("Failed to register backend: %v", err)
	}

	// Verify it exists.
	r.mu.RLock()
	_, exists := r.backends["temp-backend"]
	r.mu.RUnlock()
	if !exists {
		t.Fatal("Expected backend to be registered")
	}

	// Unregister it.
	err = r.UnregisterBackend("temp-backend")
	if err != nil {
		t.Fatalf("Failed to unregister backend: %v", err)
	}

	// Verify it's gone.
	r.mu.RLock()
	_, exists = r.backends["temp-backend"]
	r.mu.RUnlock()
	if exists {
		t.Fatal("Expected backend to be unregistered")
	}

	// Try to unregister non-existent backend.
	err = r.UnregisterBackend("nonexistent")
	if err == nil {
		t.Error("Expected error when unregistering non-existent backend")
	}
}

func TestClose(t *testing.T) {
	r := NewRouter()

	err := r.RegisterBackend("backend-1", "localhost:99991")
	if err != nil {
		t.Fatalf("Failed to register backend-1: %v", err)
	}
	err = r.RegisterBackend("backend-2", "localhost:99992")
	if err != nil {
		t.Fatalf("Failed to register backend-2: %v", err)
	}

	// Close all connections.
	err = r.Close()
	if err != nil {
		t.Fatalf("Failed to close router: %v", err)
	}

	// Verify connections are closed.
	r.mu.RLock()
	for name, backend := range r.backends {
		backend.mu.RLock()
		if backend.conn != nil {
			t.Errorf("Expected connection for %s to be nil after close", name)
		}
		backend.mu.RUnlock()
	}
	r.mu.RUnlock()
}

func TestConcurrentAccess(t *testing.T) {
	mockServer := &mockSearchServer{
		searchResponse: &searchv1.SearchResponse{
			Results: []*searchv1.SearchResult{
				{DocumentId: "doc-1", Score: 0.9},
			},
		},
	}

	addr, cleanup := startTestServer(t, func(s *grpc.Server) {
		searchv1.RegisterSearchServiceServer(s, mockServer)
	})
	defer cleanup()

	r := NewRouter()
	err := r.RegisterBackend(BackendSearch, addr)
	if err != nil {
		t.Fatalf("Failed to register backend: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Run concurrent searches.
	var wg sync.WaitGroup
	errCh := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()

			ctx := context.Background()
			ctx = WithTenantID(ctx, "tenant-concurrent")

			req := &searchv1.SearchRequest{
				Query:    "concurrent test",
				TenantId: "tenant-concurrent",
			}

			_, err := r.RouteSearch(ctx, req)
			if err != nil {
				errCh <- err
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("Concurrent search failed: %v", err)
	}
}

func TestGracefulDegradation(t *testing.T) {
	// Register backend with unreachable address.
	r := NewRouter()
	config := &BackendConfig{
		Address:           "localhost:1", // Unreachable port.
		MaxRetries:        0,
		RetryDelay:        10 * time.Millisecond,
		ConnectionTimeout: 100 * time.Millisecond,
		RequestTimeout:    100 * time.Millisecond,
	}

	err := r.RegisterBackendWithConfig(BackendSearch, config)
	if err != nil {
		t.Fatalf("Failed to register backend: %v", err)
	}

	ctx := context.Background()
	req := &searchv1.SearchRequest{
		Query: "test",
	}

	// Should return Unavailable error, not panic.
	_, err = r.RouteSearch(ctx, req)
	if err == nil {
		t.Fatal("Expected error when backend unavailable")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("Expected gRPC status error")
	}

	if st.Code() != codes.Unavailable {
		t.Errorf("Expected Unavailable code, got %v", st.Code())
	}
}

func TestBackendReconnection(t *testing.T) {
	mockServer := &mockSearchServer{}

	addr, cleanup := startTestServer(t, func(s *grpc.Server) {
		searchv1.RegisterSearchServiceServer(s, mockServer)
	})

	r := NewRouter()
	config := &BackendConfig{
		Address:           addr,
		MaxRetries:        2,
		RetryDelay:        50 * time.Millisecond,
		ConnectionTimeout: 500 * time.Millisecond,
		RequestTimeout:    500 * time.Millisecond,
	}

	err := r.RegisterBackendWithConfig(BackendSearch, config)
	if err != nil {
		t.Fatalf("Failed to register backend: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// First request should work.
	ctx := context.Background()
	req := &searchv1.SearchRequest{Query: "test"}

	_, err = r.RouteSearch(ctx, req)
	if err != nil {
		t.Fatalf("First search failed: %v", err)
	}

	// Stop the server.
	cleanup()

	// Request should fail with unavailable.
	_, err = r.RouteSearch(ctx, req)
	if err == nil {
		t.Log("Request after server stop succeeded (may happen if cached)")
	}

	// Restart the server on a new port - can't reconnect to same address,
	// but this tests that the router handles connection failures gracefully.
}

// mockAIServer implements AICoordinatorServiceServer for testing.
type mockAIServer struct {
	aiv1.UnimplementedAICoordinatorServiceServer
	generateEmbeddingCalled bool
	embeddingResponse       *aiv1.EmbeddingResponse
	embeddingError          error
	mu                      sync.Mutex
}

func (m *mockAIServer) GenerateEmbedding(ctx context.Context, req *aiv1.EmbeddingRequest) (*aiv1.EmbeddingResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.generateEmbeddingCalled = true

	if m.embeddingError != nil {
		return nil, m.embeddingError
	}
	if m.embeddingResponse != nil {
		return m.embeddingResponse, nil
	}
	return &aiv1.EmbeddingResponse{
		Vector: []float32{0.1, 0.2, 0.3},
	}, nil
}

func TestRouteAIGenerateEmbedding(t *testing.T) {
	mockServer := &mockAIServer{}

	addr, cleanup := startTestServer(t, func(s *grpc.Server) {
		aiv1.RegisterAICoordinatorServiceServer(s, mockServer)
	})
	defer cleanup()

	r := NewRouter()
	err := r.RegisterBackend(BackendAI, addr)
	if err != nil {
		t.Fatalf("Failed to register backend: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	ctx := context.Background()
	tenantID := "tenant-1"
	req := &aiv1.EmbeddingRequest{
		Text:     "test text",
		TenantId: &tenantID,
	}

	resp, err := r.RouteAIGenerateEmbedding(ctx, req)
	if err != nil {
		t.Fatalf("RouteAIGenerateEmbedding failed: %v", err)
	}

	if resp == nil {
		t.Fatal("Expected non-nil response")
	}

	if len(resp.Vector) != 3 {
		t.Errorf("Expected 3 embedding values, got %d", len(resp.Vector))
	}

	mockServer.mu.Lock()
	if !mockServer.generateEmbeddingCalled {
		t.Error("Expected GenerateEmbedding to be called")
	}
	mockServer.mu.Unlock()
}

// mockContentServer implements ContentProcessorServiceServer for testing.
type mockContentServer struct {
	contentv1.UnimplementedContentProcessorServiceServer
	processContentCalled bool
	processResponse      *contentv1.ProcessContentResponse
	processError         error
	mu                   sync.Mutex
}

func (m *mockContentServer) ProcessContent(ctx context.Context, req *contentv1.ProcessContentRequest) (*contentv1.ProcessContentResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.processContentCalled = true

	if m.processError != nil {
		return nil, m.processError
	}
	if m.processResponse != nil {
		return m.processResponse, nil
	}
	return &contentv1.ProcessContentResponse{
		JobId: "job-123",
	}, nil
}

func TestRouteContentProcess(t *testing.T) {
	mockServer := &mockContentServer{}

	addr, cleanup := startTestServer(t, func(s *grpc.Server) {
		contentv1.RegisterContentProcessorServiceServer(s, mockServer)
	})
	defer cleanup()

	r := NewRouter()
	err := r.RegisterBackend(BackendContent, addr)
	if err != nil {
		t.Fatalf("Failed to register backend: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	ctx := context.Background()
	req := &contentv1.ProcessContentRequest{
		ContentId: "content-1",
	}

	resp, err := r.RouteContentProcess(ctx, req)
	if err != nil {
		t.Fatalf("RouteContentProcess failed: %v", err)
	}

	if resp == nil {
		t.Fatal("Expected non-nil response")
	}

	if resp.JobId != "job-123" {
		t.Errorf("Expected job_id=job-123, got %s", resp.JobId)
	}

	mockServer.mu.Lock()
	if !mockServer.processContentCalled {
		t.Error("Expected ProcessContent to be called")
	}
	mockServer.mu.Unlock()
}

// mockGmailServer implements GmailConnectorServiceServer for testing.
type mockGmailServer struct {
	gmailv1.UnimplementedGmailConnectorServiceServer
	syncEmailsCalled bool
	syncResponse     *gmailv1.SyncEmailsResponse
	syncError        error
	mu               sync.Mutex
}

func (m *mockGmailServer) SyncEmails(ctx context.Context, req *gmailv1.SyncEmailsRequest) (*gmailv1.SyncEmailsResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.syncEmailsCalled = true

	if m.syncError != nil {
		return nil, m.syncError
	}
	if m.syncResponse != nil {
		return m.syncResponse, nil
	}
	return &gmailv1.SyncEmailsResponse{
		SyncId: "sync-123",
	}, nil
}

func TestRouteGmailSync(t *testing.T) {
	mockServer := &mockGmailServer{}

	addr, cleanup := startTestServer(t, func(s *grpc.Server) {
		gmailv1.RegisterGmailConnectorServiceServer(s, mockServer)
	})
	defer cleanup()

	r := NewRouter()
	err := r.RegisterBackend(BackendGmail, addr)
	if err != nil {
		t.Fatalf("Failed to register backend: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	ctx := context.Background()
	req := &gmailv1.SyncEmailsRequest{
		TenantId: "tenant-1",
	}

	resp, err := r.RouteGmailSync(ctx, req)
	if err != nil {
		t.Fatalf("RouteGmailSync failed: %v", err)
	}

	if resp == nil {
		t.Fatal("Expected non-nil response")
	}

	if resp.SyncId != "sync-123" {
		t.Errorf("Expected sync_id=sync-123, got %s", resp.SyncId)
	}

	mockServer.mu.Lock()
	if !mockServer.syncEmailsCalled {
		t.Error("Expected SyncEmails to be called")
	}
	mockServer.mu.Unlock()
}

// mockRelationshipServer implements RelationshipServiceServer for testing.
type mockRelationshipServer struct {
	relationshipv1.UnimplementedRelationshipServiceServer
	discoverCalled   bool
	discoverResponse *relationshipv1.DiscoverRelationshipsResponse
	discoverError    error
	mu               sync.Mutex
}

func (m *mockRelationshipServer) DiscoverRelationships(ctx context.Context, req *relationshipv1.DiscoverRelationshipsRequest) (*relationshipv1.DiscoverRelationshipsResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.discoverCalled = true

	if m.discoverError != nil {
		return nil, m.discoverError
	}
	if m.discoverResponse != nil {
		return m.discoverResponse, nil
	}
	return &relationshipv1.DiscoverRelationshipsResponse{
		TotalDiscovered: 5,
	}, nil
}

func TestRouteRelationshipDiscover(t *testing.T) {
	mockServer := &mockRelationshipServer{}

	addr, cleanup := startTestServer(t, func(s *grpc.Server) {
		relationshipv1.RegisterRelationshipServiceServer(s, mockServer)
	})
	defer cleanup()

	r := NewRouter()
	err := r.RegisterBackend(BackendRelationship, addr)
	if err != nil {
		t.Fatalf("Failed to register backend: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	ctx := context.Background()
	req := &relationshipv1.DiscoverRelationshipsRequest{
		TenantId: "tenant-1",
	}

	resp, err := r.RouteRelationshipDiscover(ctx, req)
	if err != nil {
		t.Fatalf("RouteRelationshipDiscover failed: %v", err)
	}

	if resp == nil {
		t.Fatal("Expected non-nil response")
	}

	if resp.TotalDiscovered != 5 {
		t.Errorf("Expected total_discovered=5, got %d", resp.TotalDiscovered)
	}

	mockServer.mu.Lock()
	if !mockServer.discoverCalled {
		t.Error("Expected DiscoverRelationships to be called")
	}
	mockServer.mu.Unlock()
}

func TestBackendStatusConnectivityState(t *testing.T) {
	mockServer := &mockSearchServer{}

	addr, cleanup := startTestServer(t, func(s *grpc.Server) {
		searchv1.RegisterSearchServiceServer(s, mockServer)
	})
	defer cleanup()

	r := NewRouter()
	err := r.RegisterBackend(BackendSearch, addr)
	if err != nil {
		t.Fatalf("Failed to register backend: %v", err)
	}

	// Give connection time to establish.
	time.Sleep(200 * time.Millisecond)

	statuses := r.BackendStatuses()
	if len(statuses) != 1 {
		t.Fatalf("Expected 1 status, got %d", len(statuses))
	}

	status := statuses[0]
	if status.Name != BackendSearch {
		t.Errorf("Expected name=search, got %s", status.Name)
	}
	if status.Address != addr {
		t.Errorf("Expected address=%s, got %s", addr, status.Address)
	}

	// State should be Ready or Idle for a working connection.
	validStates := map[connectivity.State]bool{
		connectivity.Ready: true,
		connectivity.Idle:  true,
	}
	if status.Connected && !validStates[status.State] {
		t.Errorf("Expected Ready or Idle state, got %v", status.State)
	}
}

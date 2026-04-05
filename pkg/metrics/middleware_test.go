package metrics

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestResponseWriter(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := newResponseWriter(rec)

	// Default status should be 200
	if rw.statusCode != http.StatusOK {
		t.Errorf("expected default status 200, got %d", rw.statusCode)
	}

	// Write header
	rw.WriteHeader(http.StatusNotFound)
	if rw.statusCode != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rw.statusCode)
	}

	// Write header again should not change status
	rw.WriteHeader(http.StatusOK)
	if rw.statusCode != http.StatusNotFound {
		t.Errorf("status should remain 404, got %d", rw.statusCode)
	}
}

func TestResponseWriter_Write(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := newResponseWriter(rec)

	// Write without WriteHeader should default to 200
	n, err := rw.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != 5 {
		t.Errorf("expected 5 bytes written, got %d", n)
	}
	if rw.statusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", rw.statusCode)
	}
}

func TestHTTPMiddleware(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics("test-service", "test")
	_ = m.RegisterWithRegistry(reg)

	// Create test handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Wrap with middleware
	wrapped := HTTPMiddleware(m)(handler)

	// Make request
	req := httptest.NewRequest("GET", "/api/test", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	// Verify response
	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	// Verify metrics recorded
	count := testutil.ToFloat64(m.RequestsTotal.WithLabelValues("GET", "/api/test", "200"))
	if count != 1 {
		t.Errorf("expected requests_total count 1, got %f", count)
	}
}

func TestHTTPMiddleware_5xxError(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics("test-service", "test")
	_ = m.RegisterWithRegistry(reg)

	// Create handler that returns 500
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	wrapped := HTTPMiddleware(m)(handler)

	req := httptest.NewRequest("POST", "/api/fail", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	// Verify error recorded
	errCount := testutil.ToFloat64(m.ErrorsTotal.WithLabelValues("http_5xx"))
	if errCount != 1 {
		t.Errorf("expected http_5xx error count 1, got %f", errCount)
	}
}

func TestHTTPMiddleware_4xxError(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics("test-service", "test")
	_ = m.RegisterWithRegistry(reg)

	// Create handler that returns 400
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})

	wrapped := HTTPMiddleware(m)(handler)

	req := httptest.NewRequest("POST", "/api/bad", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	// Verify error recorded
	errCount := testutil.ToFloat64(m.ErrorsTotal.WithLabelValues("http_4xx"))
	if errCount != 1 {
		t.Errorf("expected http_4xx error count 1, got %f", errCount)
	}
}

func TestHTTPMiddlewareWithConfig_SkipPaths(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics("test-service", "test")
	_ = m.RegisterWithRegistry(reg)

	cfg := &MiddlewareConfig{
		Metrics:   m,
		SkipPaths: []string{"/health", "/metrics"},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := HTTPMiddlewareWithConfig(cfg)(handler)

	// Request to skipped path
	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	// Verify no metrics recorded for /health
	count := testutil.ToFloat64(m.RequestsTotal.WithLabelValues("GET", "/health", "200"))
	if count != 0 {
		t.Errorf("expected no metrics for skipped path, got count %f", count)
	}

	// Request to non-skipped path
	req = httptest.NewRequest("GET", "/api/users", nil)
	rec = httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	count = testutil.ToFloat64(m.RequestsTotal.WithLabelValues("GET", "/api/users", "200"))
	if count != 1 {
		t.Errorf("expected metrics for non-skipped path, got count %f", count)
	}
}

func TestHTTPMiddlewareWithConfig_PathExtractor(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics("test-service", "test")
	_ = m.RegisterWithRegistry(reg)

	// Custom path extractor that normalizes user IDs
	extractor := func(r *http.Request) string {
		path := r.URL.Path
		if path == "/api/users/123" || path == "/api/users/456" {
			return "/api/users/{id}"
		}
		return path
	}

	cfg := &MiddlewareConfig{
		Metrics:       m,
		PathExtractor: extractor,
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := HTTPMiddlewareWithConfig(cfg)(handler)

	// Make requests to different user IDs
	req1 := httptest.NewRequest("GET", "/api/users/123", nil)
	rec1 := httptest.NewRecorder()
	wrapped.ServeHTTP(rec1, req1)

	req2 := httptest.NewRequest("GET", "/api/users/456", nil)
	rec2 := httptest.NewRecorder()
	wrapped.ServeHTTP(rec2, req2)

	// Both should be grouped under the same normalized path
	count := testutil.ToFloat64(m.RequestsTotal.WithLabelValues("GET", "/api/users/{id}", "200"))
	if count != 2 {
		t.Errorf("expected count 2 for normalized path, got %f", count)
	}
}

func TestUnaryServerInterceptor(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics("test-service", "test")
	_ = m.RegisterWithRegistry(reg)

	interceptor := UnaryServerInterceptor(m)

	// Mock handler that succeeds
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "success", nil
	}

	info := &grpc.UnaryServerInfo{
		FullMethod: "/test.Service/Method",
	}

	// Call interceptor
	resp, err := interceptor(context.Background(), "request", info, handler)
	if err != nil {
		t.Fatalf("interceptor failed: %v", err)
	}
	if resp != "success" {
		t.Errorf("expected response 'success', got %v", resp)
	}

	// Verify metrics
	count := testutil.ToFloat64(m.RequestsTotal.WithLabelValues("GRPC", "/test.Service/Method", "OK"))
	if count != 1 {
		t.Errorf("expected gRPC requests_total count 1, got %f", count)
	}
}

func TestUnaryServerInterceptor_Error(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics("test-service", "test")
	_ = m.RegisterWithRegistry(reg)

	interceptor := UnaryServerInterceptor(m)

	// Mock handler that fails
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, status.Error(codes.NotFound, "not found")
	}

	info := &grpc.UnaryServerInfo{
		FullMethod: "/test.Service/Get",
	}

	// Call interceptor
	_, err := interceptor(context.Background(), "request", info, handler)
	if err == nil {
		t.Fatal("expected error from interceptor")
	}

	// Verify metrics
	count := testutil.ToFloat64(m.RequestsTotal.WithLabelValues("GRPC", "/test.Service/Get", "NotFound"))
	if count != 1 {
		t.Errorf("expected gRPC requests_total count 1, got %f", count)
	}

	errCount := testutil.ToFloat64(m.ErrorsTotal.WithLabelValues("grpc_NotFound"))
	if errCount != 1 {
		t.Errorf("expected grpc_NotFound error count 1, got %f", errCount)
	}
}

func TestStatusCodeFromError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: "OK",
		},
		{
			name:     "gRPC NotFound",
			err:      status.Error(codes.NotFound, "not found"),
			expected: "NotFound",
		},
		{
			name:     "gRPC Internal",
			err:      status.Error(codes.Internal, "internal error"),
			expected: "Internal",
		},
		{
			name:     "non-gRPC error",
			err:      errors.New("plain error"),
			expected: "UNKNOWN",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code := statusCodeFromError(tt.err)
			if code != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, code)
			}
		})
	}
}

// mockServerStream implements grpc.ServerStream for testing
type mockServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (m *mockServerStream) Context() context.Context {
	return m.ctx
}

func TestStreamServerInterceptor(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics("test-service", "test")
	_ = m.RegisterWithRegistry(reg)

	interceptor := StreamServerInterceptor(m)

	// Mock handler that succeeds
	handler := func(srv interface{}, stream grpc.ServerStream) error {
		return nil
	}

	info := &grpc.StreamServerInfo{
		FullMethod: "/test.Service/StreamMethod",
	}

	stream := &mockServerStream{ctx: context.Background()}

	// Call interceptor
	err := interceptor("server", stream, info, handler)
	if err != nil {
		t.Fatalf("interceptor failed: %v", err)
	}

	// Verify metrics
	count := testutil.ToFloat64(m.RequestsTotal.WithLabelValues("GRPC_STREAM", "/test.Service/StreamMethod", "OK"))
	if count != 1 {
		t.Errorf("expected gRPC stream requests_total count 1, got %f", count)
	}
}

func TestStreamServerInterceptor_Error(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics("test-service", "test")
	_ = m.RegisterWithRegistry(reg)

	interceptor := StreamServerInterceptor(m)

	// Mock handler that fails
	handler := func(srv interface{}, stream grpc.ServerStream) error {
		return status.Error(codes.Unavailable, "service unavailable")
	}

	info := &grpc.StreamServerInfo{
		FullMethod: "/test.Service/StreamFail",
	}

	stream := &mockServerStream{ctx: context.Background()}

	// Call interceptor
	err := interceptor("server", stream, info, handler)
	if err == nil {
		t.Fatal("expected error from interceptor")
	}

	// Verify metrics
	count := testutil.ToFloat64(m.RequestsTotal.WithLabelValues("GRPC_STREAM", "/test.Service/StreamFail", "Unavailable"))
	if count != 1 {
		t.Errorf("expected gRPC stream requests_total count 1, got %f", count)
	}

	errCount := testutil.ToFloat64(m.ErrorsTotal.WithLabelValues("grpc_stream_Unavailable"))
	if errCount != 1 {
		t.Errorf("expected grpc_stream_Unavailable error count 1, got %f", errCount)
	}
}

func TestHTTPMiddleware_ConnectionTracking(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics("test-service", "test")
	_ = m.RegisterWithRegistry(reg)

	// Verify initial connections is 0
	initial := testutil.ToFloat64(m.ActiveConnections)
	if initial != 0 {
		t.Errorf("expected initial connections 0, got %f", initial)
	}

	// Channel to signal when handler is running
	handlerStarted := make(chan struct{})
	handlerDone := make(chan struct{})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(handlerStarted)
		<-handlerDone
		w.WriteHeader(http.StatusOK)
	})

	wrapped := HTTPMiddleware(m)(handler)

	// Start request in goroutine
	go func() {
		req := httptest.NewRequest("GET", "/api/slow", nil)
		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, req)
	}()

	// Wait for handler to start
	<-handlerStarted

	// While handler is running, connections should be 1
	duringCount := testutil.ToFloat64(m.ActiveConnections)
	if duringCount != 1 {
		t.Errorf("expected connections 1 during request, got %f", duringCount)
	}

	// Let handler finish
	close(handlerDone)

	// Give time for defer to run
	// Note: In real tests we'd use proper synchronization
}

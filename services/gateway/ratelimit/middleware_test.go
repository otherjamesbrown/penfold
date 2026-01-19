package ratelimit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestHTTPMiddleware(t *testing.T) {
	t.Run("allows requests within limit", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.DefaultBurst = 5
		limiter := NewRateLimiter(cfg)
		defer limiter.Close()

		handler := HTTPMiddleware(limiter)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		for i := 0; i < 5; i++ {
			req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
			req.Header.Set("X-Tenant-ID", "tenant1")
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("request %d: expected status 200, got %d", i, rr.Code)
			}
		}
	})

	t.Run("returns 429 when rate limited", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.DefaultBurst = 2
		limiter := NewRateLimiter(cfg)
		defer limiter.Close()

		handler := HTTPMiddleware(limiter)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		// Exhaust limit
		for i := 0; i < 2; i++ {
			req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
			req.Header.Set("X-Tenant-ID", "tenant1")
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
		}

		// Next request should be rate limited
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Header.Set("X-Tenant-ID", "tenant1")
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusTooManyRequests {
			t.Errorf("expected status 429, got %d", rr.Code)
		}
	})

	t.Run("includes Retry-After header when rate limited", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.DefaultRPS = 10.0
		cfg.DefaultBurst = 1
		limiter := NewRateLimiter(cfg)
		defer limiter.Close()

		handler := HTTPMiddleware(limiter)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		// Exhaust limit
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Header.Set("X-Tenant-ID", "tenant1")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		// Rate limited request
		req = httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Header.Set("X-Tenant-ID", "tenant1")
		rr = httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		retryAfter := rr.Header().Get(HeaderRetryAfter)
		if retryAfter == "" {
			t.Error("expected Retry-After header to be set")
		}

		retrySeconds, err := strconv.Atoi(retryAfter)
		if err != nil {
			t.Errorf("expected Retry-After to be integer, got %s", retryAfter)
		}
		if retrySeconds < 1 {
			t.Errorf("expected Retry-After >= 1, got %d", retrySeconds)
		}
	})

	t.Run("includes rate limit headers", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.DefaultBurst = 10
		limiter := NewRateLimiter(cfg)
		defer limiter.Close()

		handler := HTTPMiddleware(limiter)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Header.Set("X-Tenant-ID", "tenant1")
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Header().Get(HeaderRateLimitLimit) == "" {
			t.Error("expected X-RateLimit-Limit header to be set")
		}
		if rr.Header().Get(HeaderRateLimitRemaining) == "" {
			t.Error("expected X-RateLimit-Remaining header to be set")
		}
	})

	t.Run("skips configured paths", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.DefaultBurst = 1
		limiter := NewRateLimiter(cfg)
		defer limiter.Close()

		handler := HTTPMiddlewareWithConfig(&HTTPMiddlewareConfig{
			Limiter:        limiter,
			SkipPaths:      []string{"/health", "/metrics"},
			IncludeHeaders: true,
		})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		// Health endpoint should never be rate limited
		for i := 0; i < 10; i++ {
			req := httptest.NewRequest(http.MethodGet, "/health", nil)
			req.Header.Set("X-Tenant-ID", "tenant1")
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("health request %d should not be rate limited", i)
			}
		}
	})

	t.Run("uses custom tenant extractor", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.DefaultBurst = 2
		limiter := NewRateLimiter(cfg)
		defer limiter.Close()

		customExtractor := func(r *http.Request) string {
			return r.Header.Get("X-Custom-Tenant")
		}

		handler := HTTPMiddlewareWithConfig(&HTTPMiddlewareConfig{
			Limiter:         limiter,
			TenantExtractor: customExtractor,
			IncludeHeaders:  true,
		})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		// Use custom header
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Header.Set("X-Custom-Tenant", "custom-tenant")
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rr.Code)
		}
	})
}

func TestDefaultTenantExtractor(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("X-Tenant-ID", "test-tenant")

	tenantID := DefaultTenantExtractor(req)
	if tenantID != "test-tenant" {
		t.Errorf("expected test-tenant, got %s", tenantID)
	}
}

func TestDefaultGRPCTenantExtractor(t *testing.T) {
	t.Run("extracts tenant from metadata", func(t *testing.T) {
		md := metadata.Pairs("x-tenant-id", "grpc-tenant")
		ctx := metadata.NewIncomingContext(context.Background(), md)

		tenantID := DefaultGRPCTenantExtractor(ctx)
		if tenantID != "grpc-tenant" {
			t.Errorf("expected grpc-tenant, got %s", tenantID)
		}
	})

	t.Run("returns empty when no metadata", func(t *testing.T) {
		ctx := context.Background()

		tenantID := DefaultGRPCTenantExtractor(ctx)
		if tenantID != "" {
			t.Errorf("expected empty string, got %s", tenantID)
		}
	})

	t.Run("returns empty when header missing", func(t *testing.T) {
		md := metadata.Pairs("other-header", "value")
		ctx := metadata.NewIncomingContext(context.Background(), md)

		tenantID := DefaultGRPCTenantExtractor(ctx)
		if tenantID != "" {
			t.Errorf("expected empty string, got %s", tenantID)
		}
	})
}

func TestUnaryServerInterceptor(t *testing.T) {
	t.Run("allows requests within limit", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.DefaultBurst = 5
		limiter := NewRateLimiter(cfg)
		defer limiter.Close()

		interceptor := UnaryServerInterceptor(limiter)

		md := metadata.Pairs("x-tenant-id", "tenant1")
		ctx := metadata.NewIncomingContext(context.Background(), md)

		info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}
		handler := func(ctx context.Context, req interface{}) (interface{}, error) {
			return "response", nil
		}

		for i := 0; i < 5; i++ {
			resp, err := interceptor(ctx, "request", info, handler)
			if err != nil {
				t.Errorf("request %d should have been allowed: %v", i, err)
			}
			if resp != "response" {
				t.Errorf("expected response, got %v", resp)
			}
		}
	})

	t.Run("returns ResourceExhausted when rate limited", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.DefaultBurst = 1
		limiter := NewRateLimiter(cfg)
		defer limiter.Close()

		interceptor := UnaryServerInterceptor(limiter)

		md := metadata.Pairs("x-tenant-id", "tenant1")
		ctx := metadata.NewIncomingContext(context.Background(), md)

		info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}
		handler := func(ctx context.Context, req interface{}) (interface{}, error) {
			return "response", nil
		}

		// First request should succeed
		_, err := interceptor(ctx, "request", info, handler)
		if err != nil {
			t.Errorf("first request should have been allowed: %v", err)
		}

		// Second request should be rate limited
		_, err = interceptor(ctx, "request", info, handler)
		if err == nil {
			t.Error("second request should have been rate limited")
		}

		st, ok := status.FromError(err)
		if !ok {
			t.Error("expected gRPC status error")
		}
		if st.Code() != codes.ResourceExhausted {
			t.Errorf("expected ResourceExhausted, got %v", st.Code())
		}
	})

	t.Run("skips configured methods", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.DefaultBurst = 1
		limiter := NewRateLimiter(cfg)
		defer limiter.Close()

		interceptor := UnaryServerInterceptorWithConfig(&GRPCInterceptorConfig{
			Limiter:     limiter,
			SkipMethods: []string{"/grpc.health.v1.Health/Check"},
		})

		md := metadata.Pairs("x-tenant-id", "tenant1")
		ctx := metadata.NewIncomingContext(context.Background(), md)

		info := &grpc.UnaryServerInfo{FullMethod: "/grpc.health.v1.Health/Check"}
		handler := func(ctx context.Context, req interface{}) (interface{}, error) {
			return "response", nil
		}

		// Health check should never be rate limited
		for i := 0; i < 10; i++ {
			_, err := interceptor(ctx, "request", info, handler)
			if err != nil {
				t.Errorf("health check %d should not be rate limited: %v", i, err)
			}
		}
	})

	t.Run("uses custom tenant extractor", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.DefaultBurst = 2
		limiter := NewRateLimiter(cfg)
		defer limiter.Close()

		customExtractor := func(ctx context.Context) string {
			md, ok := metadata.FromIncomingContext(ctx)
			if !ok {
				return ""
			}
			values := md.Get("custom-tenant")
			if len(values) == 0 {
				return ""
			}
			return values[0]
		}

		interceptor := UnaryServerInterceptorWithConfig(&GRPCInterceptorConfig{
			Limiter:         limiter,
			TenantExtractor: customExtractor,
		})

		md := metadata.Pairs("custom-tenant", "custom-tenant-value")
		ctx := metadata.NewIncomingContext(context.Background(), md)

		info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}
		handler := func(ctx context.Context, req interface{}) (interface{}, error) {
			return "response", nil
		}

		_, err := interceptor(ctx, "request", info, handler)
		if err != nil {
			t.Errorf("request should have been allowed: %v", err)
		}
	})
}

// mockServerStream implements grpc.ServerStream for testing.
type mockServerStream struct {
	grpc.ServerStream
	ctx     context.Context
	trailer metadata.MD
}

func (m *mockServerStream) Context() context.Context {
	return m.ctx
}

func (m *mockServerStream) SetTrailer(md metadata.MD) {
	m.trailer = md
}

func TestStreamServerInterceptor(t *testing.T) {
	t.Run("allows streams within limit", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.DefaultBurst = 5
		limiter := NewRateLimiter(cfg)
		defer limiter.Close()

		interceptor := StreamServerInterceptor(limiter)

		md := metadata.Pairs("x-tenant-id", "tenant1")
		ctx := metadata.NewIncomingContext(context.Background(), md)

		info := &grpc.StreamServerInfo{FullMethod: "/test.Service/StreamMethod"}
		handler := func(srv interface{}, stream grpc.ServerStream) error {
			return nil
		}

		for i := 0; i < 5; i++ {
			stream := &mockServerStream{ctx: ctx}
			err := interceptor(nil, stream, info, handler)
			if err != nil {
				t.Errorf("stream %d should have been allowed: %v", i, err)
			}
		}
	})

	t.Run("returns ResourceExhausted when rate limited", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.DefaultBurst = 1
		limiter := NewRateLimiter(cfg)
		defer limiter.Close()

		interceptor := StreamServerInterceptor(limiter)

		md := metadata.Pairs("x-tenant-id", "tenant1")
		ctx := metadata.NewIncomingContext(context.Background(), md)

		info := &grpc.StreamServerInfo{FullMethod: "/test.Service/StreamMethod"}
		handler := func(srv interface{}, stream grpc.ServerStream) error {
			return nil
		}

		// First stream should succeed
		stream := &mockServerStream{ctx: ctx}
		err := interceptor(nil, stream, info, handler)
		if err != nil {
			t.Errorf("first stream should have been allowed: %v", err)
		}

		// Second stream should be rate limited
		stream = &mockServerStream{ctx: ctx}
		err = interceptor(nil, stream, info, handler)
		if err == nil {
			t.Error("second stream should have been rate limited")
		}

		st, ok := status.FromError(err)
		if !ok {
			t.Error("expected gRPC status error")
		}
		if st.Code() != codes.ResourceExhausted {
			t.Errorf("expected ResourceExhausted, got %v", st.Code())
		}
	})

	t.Run("skips configured methods", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.DefaultBurst = 1
		limiter := NewRateLimiter(cfg)
		defer limiter.Close()

		interceptor := StreamServerInterceptorWithConfig(&GRPCInterceptorConfig{
			Limiter:     limiter,
			SkipMethods: []string{"/test.Service/SkippedStream"},
		})

		md := metadata.Pairs("x-tenant-id", "tenant1")
		ctx := metadata.NewIncomingContext(context.Background(), md)

		info := &grpc.StreamServerInfo{FullMethod: "/test.Service/SkippedStream"}
		handler := func(srv interface{}, stream grpc.ServerStream) error {
			return nil
		}

		// Skipped method should never be rate limited
		for i := 0; i < 10; i++ {
			stream := &mockServerStream{ctx: ctx}
			err := interceptor(nil, stream, info, handler)
			if err != nil {
				t.Errorf("skipped stream %d should not be rate limited: %v", i, err)
			}
		}
	})
}

package metrics

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

// newResponseWriter creates a wrapped ResponseWriter with default status 200.
func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}
}

// WriteHeader captures the status code before writing.
func (rw *responseWriter) WriteHeader(code int) {
	if !rw.written {
		rw.statusCode = code
		rw.written = true
	}
	rw.ResponseWriter.WriteHeader(code)
}

// Write implements http.ResponseWriter and ensures status code is captured.
func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.written {
		rw.statusCode = http.StatusOK
		rw.written = true
	}
	return rw.ResponseWriter.Write(b)
}

// PathExtractor is a function that extracts the path pattern from a request.
// This allows for normalizing paths (e.g., "/users/123" -> "/users/{id}").
type PathExtractor func(r *http.Request) string

// DefaultPathExtractor returns the request URL path as-is.
func DefaultPathExtractor(r *http.Request) string {
	return r.URL.Path
}

// MiddlewareConfig configures the HTTP metrics middleware.
type MiddlewareConfig struct {
	// Metrics is the metrics instance to record to.
	Metrics *Metrics

	// PathExtractor extracts the path pattern from requests.
	// If nil, DefaultPathExtractor is used.
	PathExtractor PathExtractor

	// SkipPaths is a list of paths to skip metrics collection for.
	// Useful for health checks and metrics endpoints.
	SkipPaths []string
}

// HTTPMiddleware creates HTTP middleware that automatically records request metrics.
// It tracks requests_total, request_duration_seconds, and active_connections.
func HTTPMiddleware(m *Metrics) func(http.Handler) http.Handler {
	return HTTPMiddlewareWithConfig(&MiddlewareConfig{
		Metrics: m,
	})
}

// HTTPMiddlewareWithConfig creates HTTP middleware with custom configuration.
func HTTPMiddlewareWithConfig(cfg *MiddlewareConfig) func(http.Handler) http.Handler {
	if cfg.PathExtractor == nil {
		cfg.PathExtractor = DefaultPathExtractor
	}

	skipPaths := make(map[string]bool)
	for _, p := range cfg.SkipPaths {
		skipPaths[p] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := cfg.PathExtractor(r)

			// Skip metrics for certain paths
			if skipPaths[path] {
				next.ServeHTTP(w, r)
				return
			}

			// Track active connections
			cfg.Metrics.IncrementConnections()
			defer cfg.Metrics.DecrementConnections()

			// Wrap response writer to capture status code
			wrapped := newResponseWriter(w)

			// Record start time
			start := time.Now()

			// Process request
			next.ServeHTTP(wrapped, r)

			// Record metrics
			duration := time.Since(start).Seconds()
			status := strconv.Itoa(wrapped.statusCode)
			cfg.Metrics.RecordRequest(r.Method, path, status, duration)

			// Record errors for 5xx status codes
			if wrapped.statusCode >= 500 {
				cfg.Metrics.RecordError("http_5xx")
			} else if wrapped.statusCode >= 400 {
				cfg.Metrics.RecordError("http_4xx")
			}
		})
	}
}

// UnaryServerInterceptor creates a gRPC unary server interceptor for metrics.
// It records request counts, durations, and errors for unary RPC calls.
func UnaryServerInterceptor(m *Metrics) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// Track active connections
		m.IncrementConnections()
		defer m.DecrementConnections()

		// Record start time
		start := time.Now()

		// Process request
		resp, err := handler(ctx, req)

		// Record metrics
		duration := time.Since(start).Seconds()
		statusCode := statusCodeFromError(err)
		m.RecordRequest("GRPC", info.FullMethod, statusCode, duration)

		// Record errors
		if err != nil {
			m.RecordError("grpc_" + statusCode)
		}

		return resp, err
	}
}

// StreamServerInterceptor creates a gRPC stream server interceptor for metrics.
// It tracks active connections and records stream start/completion.
func StreamServerInterceptor(m *Metrics) grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		// Track active connections
		m.IncrementConnections()
		defer m.DecrementConnections()

		// Record start time
		start := time.Now()

		// Process stream
		err := handler(srv, ss)

		// Record metrics
		duration := time.Since(start).Seconds()
		statusCode := statusCodeFromError(err)
		m.RecordRequest("GRPC_STREAM", info.FullMethod, statusCode, duration)

		// Record errors
		if err != nil {
			m.RecordError("grpc_stream_" + statusCode)
		}

		return err
	}
}

// statusCodeFromError extracts the gRPC status code from an error.
func statusCodeFromError(err error) string {
	if err == nil {
		return "OK"
	}
	st, ok := status.FromError(err)
	if !ok {
		return "UNKNOWN"
	}
	return st.Code().String()
}

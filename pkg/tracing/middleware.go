package tracing

import (
	"context"
	"fmt"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// HTTPMiddlewareConfig configures the HTTP tracing middleware.
type HTTPMiddlewareConfig struct {
	// TracerName is the name of the tracer to use.
	// If empty, defaults to "http-server".
	TracerName string

	// SkipPaths is a list of paths to skip tracing for.
	// Useful for health checks and metrics endpoints.
	SkipPaths []string

	// PathExtractor extracts the path pattern from requests.
	// If nil, uses the raw URL path.
	PathExtractor func(r *http.Request) string
}

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

// HTTPMiddleware creates HTTP middleware that automatically traces requests.
// It extracts trace context from incoming headers and creates a span for each request.
func HTTPMiddleware(next http.Handler) http.Handler {
	return HTTPMiddlewareWithConfig(&HTTPMiddlewareConfig{})(next)
}

// HTTPMiddlewareWithConfig creates HTTP middleware with custom configuration.
func HTTPMiddlewareWithConfig(cfg *HTTPMiddlewareConfig) func(http.Handler) http.Handler {
	if cfg == nil {
		cfg = &HTTPMiddlewareConfig{}
	}

	tracerName := cfg.TracerName
	if tracerName == "" {
		tracerName = "http-server"
	}

	tracer := otel.Tracer(tracerName)
	propagator := otel.GetTextMapPropagator()

	skipPaths := make(map[string]bool)
	for _, p := range cfg.SkipPaths {
		skipPaths[p] = true
	}

	pathExtractor := cfg.PathExtractor
	if pathExtractor == nil {
		pathExtractor = func(r *http.Request) string {
			return r.URL.Path
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := pathExtractor(r)

			// Skip tracing for certain paths
			if skipPaths[path] {
				next.ServeHTTP(w, r)
				return
			}

			// Extract trace context from headers
			ctx := propagator.Extract(r.Context(), propagation.HeaderCarrier(r.Header))

			// Start span
			spanName := fmt.Sprintf("%s %s", r.Method, path)
			ctx, span := tracer.Start(ctx, spanName,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(
					attribute.String("http.request.method", r.Method),
					semconv.URLScheme(schemeFromRequest(r)),
					semconv.URLPath(r.URL.Path),
					semconv.URLQuery(r.URL.RawQuery),
					semconv.URLFull(r.URL.String()),
					attribute.String("server.address", r.Host),
					attribute.String("user_agent.original", r.UserAgent()),
				),
			)
			defer span.End()

			// Wrap response writer to capture status code
			wrapped := newResponseWriter(w)

			// Process request
			next.ServeHTTP(wrapped, r.WithContext(ctx))

			// Record response attributes
			span.SetAttributes(
				semconv.HTTPResponseStatusCode(wrapped.statusCode),
			)

			// Set span status based on HTTP status code
			if wrapped.statusCode >= 400 {
				span.SetStatus(codes.Error, http.StatusText(wrapped.statusCode))
			} else {
				span.SetStatus(codes.Ok, "")
			}
		})
	}
}

// schemeFromRequest determines the request scheme (http or https).
func schemeFromRequest(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	if scheme := r.Header.Get("X-Forwarded-Proto"); scheme != "" {
		return scheme
	}
	return "http"
}

// InjectHTTPHeaders injects trace context into outgoing HTTP request headers.
// Use this when making outgoing HTTP calls to propagate the trace.
func InjectHTTPHeaders(ctx context.Context, req *http.Request) {
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
}

// grpcMetadataCarrier implements the TextMapCarrier interface for gRPC metadata.
type grpcMetadataCarrier struct {
	md *metadata.MD
}

func (c grpcMetadataCarrier) Get(key string) string {
	vals := c.md.Get(key)
	if len(vals) > 0 {
		return vals[0]
	}
	return ""
}

func (c grpcMetadataCarrier) Set(key, value string) {
	c.md.Set(key, value)
}

func (c grpcMetadataCarrier) Keys() []string {
	keys := make([]string, 0, len(*c.md))
	for k := range *c.md {
		keys = append(keys, k)
	}
	return keys
}

// UnaryServerInterceptor creates a gRPC unary server interceptor for tracing.
// It extracts trace context from incoming metadata and creates a span for each RPC.
func UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return UnaryServerInterceptorWithTracer("grpc-server")
}

// UnaryServerInterceptorWithTracer creates a unary interceptor with a custom tracer name.
func UnaryServerInterceptorWithTracer(tracerName string) grpc.UnaryServerInterceptor {
	tracer := otel.Tracer(tracerName)
	propagator := otel.GetTextMapPropagator()

	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// Extract trace context from metadata
		md, ok := metadata.FromIncomingContext(ctx)
		if ok {
			ctx = propagator.Extract(ctx, grpcMetadataCarrier{md: &md})
		}

		// Start span
		ctx, span := tracer.Start(ctx, info.FullMethod,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				semconv.RPCSystemGRPC,
				semconv.RPCService(serviceFromMethod(info.FullMethod)),
				semconv.RPCMethod(methodFromMethod(info.FullMethod)),
			),
		)
		defer span.End()

		// Process request
		resp, err := handler(ctx, req)

		// Set status based on error
		if err != nil {
			st, _ := status.FromError(err)
			span.SetAttributes(
				attribute.String("rpc.grpc.status_code", st.Code().String()),
			)
			span.RecordError(err)
			span.SetStatus(codes.Error, st.Message())
		} else {
			span.SetAttributes(
				attribute.String("rpc.grpc.status_code", "OK"),
			)
			span.SetStatus(codes.Ok, "")
		}

		return resp, err
	}
}

// StreamServerInterceptor creates a gRPC stream server interceptor for tracing.
func StreamServerInterceptor() grpc.StreamServerInterceptor {
	return StreamServerInterceptorWithTracer("grpc-server")
}

// StreamServerInterceptorWithTracer creates a stream interceptor with a custom tracer name.
func StreamServerInterceptorWithTracer(tracerName string) grpc.StreamServerInterceptor {
	tracer := otel.Tracer(tracerName)
	propagator := otel.GetTextMapPropagator()

	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		ctx := ss.Context()

		// Extract trace context from metadata
		md, ok := metadata.FromIncomingContext(ctx)
		if ok {
			ctx = propagator.Extract(ctx, grpcMetadataCarrier{md: &md})
		}

		// Start span
		ctx, span := tracer.Start(ctx, info.FullMethod,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				semconv.RPCSystemGRPC,
				semconv.RPCService(serviceFromMethod(info.FullMethod)),
				semconv.RPCMethod(methodFromMethod(info.FullMethod)),
				attribute.Bool("rpc.grpc.is_client_stream", info.IsClientStream),
				attribute.Bool("rpc.grpc.is_server_stream", info.IsServerStream),
			),
		)
		defer span.End()

		// Wrap stream with new context
		wrapped := &wrappedServerStream{
			ServerStream: ss,
			ctx:          ctx,
		}

		// Process stream
		err := handler(srv, wrapped)

		// Set status based on error
		if err != nil {
			st, _ := status.FromError(err)
			span.SetAttributes(
				attribute.String("rpc.grpc.status_code", st.Code().String()),
			)
			span.RecordError(err)
			span.SetStatus(codes.Error, st.Message())
		} else {
			span.SetAttributes(
				attribute.String("rpc.grpc.status_code", "OK"),
			)
			span.SetStatus(codes.Ok, "")
		}

		return err
	}
}

// wrappedServerStream wraps grpc.ServerStream with a custom context.
type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
}

// UnaryClientInterceptor creates a gRPC unary client interceptor for tracing.
// It injects trace context into outgoing metadata.
func UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return UnaryClientInterceptorWithTracer("grpc-client")
}

// UnaryClientInterceptorWithTracer creates a unary client interceptor with a custom tracer name.
func UnaryClientInterceptorWithTracer(tracerName string) grpc.UnaryClientInterceptor {
	tracer := otel.Tracer(tracerName)
	propagator := otel.GetTextMapPropagator()

	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		// Start span
		ctx, span := tracer.Start(ctx, method,
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(
				semconv.RPCSystemGRPC,
				semconv.RPCService(serviceFromMethod(method)),
				semconv.RPCMethod(methodFromMethod(method)),
			),
		)
		defer span.End()

		// Inject trace context into metadata
		md, ok := metadata.FromOutgoingContext(ctx)
		if !ok {
			md = metadata.New(nil)
		} else {
			md = md.Copy()
		}
		propagator.Inject(ctx, grpcMetadataCarrier{md: &md})
		ctx = metadata.NewOutgoingContext(ctx, md)

		// Invoke RPC
		err := invoker(ctx, method, req, reply, cc, opts...)

		// Set status based on error
		if err != nil {
			st, _ := status.FromError(err)
			span.SetAttributes(
				attribute.String("rpc.grpc.status_code", st.Code().String()),
			)
			span.RecordError(err)
			span.SetStatus(codes.Error, st.Message())
		} else {
			span.SetAttributes(
				attribute.String("rpc.grpc.status_code", "OK"),
			)
			span.SetStatus(codes.Ok, "")
		}

		return err
	}
}

// StreamClientInterceptor creates a gRPC stream client interceptor for tracing.
func StreamClientInterceptor() grpc.StreamClientInterceptor {
	return StreamClientInterceptorWithTracer("grpc-client")
}

// StreamClientInterceptorWithTracer creates a stream client interceptor with a custom tracer name.
func StreamClientInterceptorWithTracer(tracerName string) grpc.StreamClientInterceptor {
	tracer := otel.Tracer(tracerName)
	propagator := otel.GetTextMapPropagator()

	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		// Start span
		ctx, span := tracer.Start(ctx, method,
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(
				semconv.RPCSystemGRPC,
				semconv.RPCService(serviceFromMethod(method)),
				semconv.RPCMethod(methodFromMethod(method)),
				attribute.Bool("rpc.grpc.is_client_stream", desc.ClientStreams),
				attribute.Bool("rpc.grpc.is_server_stream", desc.ServerStreams),
			),
		)

		// Inject trace context into metadata
		md, ok := metadata.FromOutgoingContext(ctx)
		if !ok {
			md = metadata.New(nil)
		} else {
			md = md.Copy()
		}
		propagator.Inject(ctx, grpcMetadataCarrier{md: &md})
		ctx = metadata.NewOutgoingContext(ctx, md)

		// Create stream
		stream, err := streamer(ctx, desc, cc, method, opts...)

		if err != nil {
			st, _ := status.FromError(err)
			span.SetAttributes(
				attribute.String("rpc.grpc.status_code", st.Code().String()),
			)
			span.RecordError(err)
			span.SetStatus(codes.Error, st.Message())
			span.End()
			return nil, err
		}

		// Wrap stream to end span when stream closes
		return &wrappedClientStream{
			ClientStream: stream,
			span:         span,
		}, nil
	}
}

// wrappedClientStream wraps grpc.ClientStream to end the span when the stream closes.
type wrappedClientStream struct {
	grpc.ClientStream
	span trace.Span
}

func (w *wrappedClientStream) CloseSend() error {
	err := w.ClientStream.CloseSend()
	if err != nil {
		st, _ := status.FromError(err)
		w.span.SetAttributes(
			attribute.String("rpc.grpc.status_code", st.Code().String()),
		)
		w.span.RecordError(err)
		w.span.SetStatus(codes.Error, st.Message())
	} else {
		w.span.SetAttributes(
			attribute.String("rpc.grpc.status_code", "OK"),
		)
		w.span.SetStatus(codes.Ok, "")
	}
	w.span.End()
	return err
}

// serviceFromMethod extracts the service name from a gRPC method string.
// Method format: /package.service/method
func serviceFromMethod(method string) string {
	if len(method) < 2 {
		return method
	}
	// Remove leading /
	if method[0] == '/' {
		method = method[1:]
	}
	// Find the service part
	for i := len(method) - 1; i >= 0; i-- {
		if method[i] == '/' {
			return method[:i]
		}
	}
	return method
}

// methodFromMethod extracts the method name from a gRPC method string.
// Method format: /package.service/method
func methodFromMethod(method string) string {
	for i := len(method) - 1; i >= 0; i-- {
		if method[i] == '/' {
			return method[i+1:]
		}
	}
	return method
}

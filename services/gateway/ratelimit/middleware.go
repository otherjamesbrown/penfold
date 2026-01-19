package ratelimit

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/otherjamesbrown/penfold/pkg/auth"
)

// HTTP Headers for rate limit information.
const (
	// HeaderRateLimitLimit is the maximum number of requests allowed in the window.
	HeaderRateLimitLimit = "X-RateLimit-Limit"
	// HeaderRateLimitRemaining is the number of remaining requests in the current window.
	HeaderRateLimitRemaining = "X-RateLimit-Remaining"
	// HeaderRetryAfter is the time to wait before retrying (in seconds).
	HeaderRetryAfter = "Retry-After"
)

// TenantExtractor is a function that extracts the tenant ID from a request.
type TenantExtractor func(r *http.Request) string

// DefaultTenantExtractor extracts tenant ID from the X-Tenant-ID header.
func DefaultTenantExtractor(r *http.Request) string {
	return r.Header.Get("X-Tenant-ID")
}

// AuthTenantExtractor extracts tenant ID from authentication claims.
func AuthTenantExtractor(r *http.Request) string {
	claims := auth.GetClaims(r.Context())
	if claims != nil {
		// Check for tenant_id in custom claims
		if tenantID, ok := claims.Custom["tenant_id"].(string); ok {
			return tenantID
		}
		// Fall back to subject as tenant ID
		return claims.Subject
	}
	return ""
}

// EndpointExtractor is a function that extracts the endpoint pattern from a request.
type EndpointExtractor func(r *http.Request) string

// DefaultEndpointExtractor returns the request URL path.
func DefaultEndpointExtractor(r *http.Request) string {
	return r.URL.Path
}

// HTTPMiddlewareConfig configures the HTTP rate limiting middleware.
type HTTPMiddlewareConfig struct {
	// Limiter is the rate limiter instance.
	Limiter *RateLimiter

	// TenantExtractor extracts the tenant ID from requests.
	// If nil, DefaultTenantExtractor is used.
	TenantExtractor TenantExtractor

	// EndpointExtractor extracts the endpoint pattern from requests.
	// If nil, DefaultEndpointExtractor is used.
	EndpointExtractor EndpointExtractor

	// SkipPaths is a list of paths to skip rate limiting for.
	// Useful for health checks and metrics endpoints.
	SkipPaths []string

	// IncludeHeaders indicates whether to include rate limit headers in responses.
	IncludeHeaders bool
}

// HTTPMiddleware creates HTTP middleware that applies rate limiting.
// It returns 429 Too Many Requests when the rate limit is exceeded.
func HTTPMiddleware(limiter *RateLimiter) func(http.Handler) http.Handler {
	return HTTPMiddlewareWithConfig(&HTTPMiddlewareConfig{
		Limiter:        limiter,
		IncludeHeaders: true,
	})
}

// HTTPMiddlewareWithConfig creates HTTP middleware with custom configuration.
func HTTPMiddlewareWithConfig(cfg *HTTPMiddlewareConfig) func(http.Handler) http.Handler {
	if cfg.TenantExtractor == nil {
		cfg.TenantExtractor = DefaultTenantExtractor
	}
	if cfg.EndpointExtractor == nil {
		cfg.EndpointExtractor = DefaultEndpointExtractor
	}

	skipPaths := make(map[string]bool)
	for _, p := range cfg.SkipPaths {
		skipPaths[p] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			endpoint := cfg.EndpointExtractor(r)

			// Skip rate limiting for certain paths
			if skipPaths[endpoint] {
				next.ServeHTTP(w, r)
				return
			}

			tenantID := cfg.TenantExtractor(r)
			ctx := WithTenantID(r.Context(), tenantID)

			info, err := cfg.Limiter.AllowWithInfo(ctx, tenantID, endpoint)

			// Add rate limit headers if configured
			if cfg.IncludeHeaders && info != nil {
				if info.Limit > 0 {
					w.Header().Set(HeaderRateLimitLimit, strconv.Itoa(info.Limit))
				}
				if info.Remaining >= 0 {
					w.Header().Set(HeaderRateLimitRemaining, strconv.Itoa(info.Remaining))
				}
			}

			if err != nil {
				// Add Retry-After header
				retryAfter := int(info.RetryAfter.Seconds())
				if retryAfter < 1 {
					retryAfter = 1
				}
				w.Header().Set(HeaderRetryAfter, strconv.Itoa(retryAfter))

				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}

			// Store tenant ID in context for downstream use
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GRPCTenantExtractor extracts tenant ID from gRPC metadata.
type GRPCTenantExtractor func(ctx context.Context) string

// DefaultGRPCTenantExtractor extracts tenant ID from x-tenant-id metadata.
func DefaultGRPCTenantExtractor(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	values := md.Get("x-tenant-id")
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// AuthGRPCTenantExtractor extracts tenant ID from authentication claims.
func AuthGRPCTenantExtractor(ctx context.Context) string {
	claims := auth.GetClaims(ctx)
	if claims != nil {
		// Check for tenant_id in custom claims
		if tenantID, ok := claims.Custom["tenant_id"].(string); ok {
			return tenantID
		}
		// Fall back to subject as tenant ID
		return claims.Subject
	}
	return ""
}

// GRPCInterceptorConfig configures the gRPC rate limiting interceptor.
type GRPCInterceptorConfig struct {
	// Limiter is the rate limiter instance.
	Limiter *RateLimiter

	// TenantExtractor extracts the tenant ID from context.
	// If nil, DefaultGRPCTenantExtractor is used.
	TenantExtractor GRPCTenantExtractor

	// SkipMethods is a list of gRPC methods to skip rate limiting for.
	// Methods should be full method names like "/package.Service/Method".
	SkipMethods []string
}

// UnaryServerInterceptor creates a gRPC unary server interceptor for rate limiting.
// It returns codes.ResourceExhausted when the rate limit is exceeded.
func UnaryServerInterceptor(limiter *RateLimiter) grpc.UnaryServerInterceptor {
	return UnaryServerInterceptorWithConfig(&GRPCInterceptorConfig{
		Limiter: limiter,
	})
}

// UnaryServerInterceptorWithConfig creates a gRPC unary interceptor with custom configuration.
func UnaryServerInterceptorWithConfig(cfg *GRPCInterceptorConfig) grpc.UnaryServerInterceptor {
	if cfg.TenantExtractor == nil {
		cfg.TenantExtractor = DefaultGRPCTenantExtractor
	}

	skipMethods := make(map[string]bool)
	for _, m := range cfg.SkipMethods {
		skipMethods[m] = true
	}

	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// Skip rate limiting for certain methods
		if skipMethods[info.FullMethod] {
			return handler(ctx, req)
		}

		tenantID := cfg.TenantExtractor(ctx)
		ctx = WithTenantID(ctx, tenantID)

		limitInfo, err := cfg.Limiter.AllowWithInfo(ctx, tenantID, info.FullMethod)
		if err != nil {
			// Create status with retry-after in details
			retryAfter := limitInfo.RetryAfter
			if retryAfter < time.Second {
				retryAfter = time.Second
			}

			// Set trailer with rate limit info
			trailer := metadata.Pairs(
				"retry-after", strconv.Itoa(int(retryAfter.Seconds())),
				"x-ratelimit-limit", strconv.Itoa(limitInfo.Limit),
				"x-ratelimit-remaining", strconv.Itoa(limitInfo.Remaining),
			)
			grpc.SetTrailer(ctx, trailer)

			return nil, status.Errorf(
				codes.ResourceExhausted,
				"rate limit exceeded, retry after %v",
				retryAfter,
			)
		}

		// Set trailer with rate limit info for successful requests too
		if limitInfo.Limit > 0 {
			trailer := metadata.Pairs(
				"x-ratelimit-limit", strconv.Itoa(limitInfo.Limit),
				"x-ratelimit-remaining", strconv.Itoa(limitInfo.Remaining),
			)
			grpc.SetTrailer(ctx, trailer)
		}

		return handler(ctx, req)
	}
}

// StreamServerInterceptor creates a gRPC stream server interceptor for rate limiting.
func StreamServerInterceptor(limiter *RateLimiter) grpc.StreamServerInterceptor {
	return StreamServerInterceptorWithConfig(&GRPCInterceptorConfig{
		Limiter: limiter,
	})
}

// StreamServerInterceptorWithConfig creates a gRPC stream interceptor with custom configuration.
func StreamServerInterceptorWithConfig(cfg *GRPCInterceptorConfig) grpc.StreamServerInterceptor {
	if cfg.TenantExtractor == nil {
		cfg.TenantExtractor = DefaultGRPCTenantExtractor
	}

	skipMethods := make(map[string]bool)
	for _, m := range cfg.SkipMethods {
		skipMethods[m] = true
	}

	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		// Skip rate limiting for certain methods
		if skipMethods[info.FullMethod] {
			return handler(srv, ss)
		}

		ctx := ss.Context()
		tenantID := cfg.TenantExtractor(ctx)
		ctx = WithTenantID(ctx, tenantID)

		limitInfo, err := cfg.Limiter.AllowWithInfo(ctx, tenantID, info.FullMethod)
		if err != nil {
			// Create status with retry-after in details
			retryAfter := limitInfo.RetryAfter
			if retryAfter < time.Second {
				retryAfter = time.Second
			}

			// Set trailer with rate limit info
			trailer := metadata.Pairs(
				"retry-after", strconv.Itoa(int(retryAfter.Seconds())),
				"x-ratelimit-limit", strconv.Itoa(limitInfo.Limit),
				"x-ratelimit-remaining", strconv.Itoa(limitInfo.Remaining),
			)
			ss.SetTrailer(trailer)

			return status.Errorf(
				codes.ResourceExhausted,
				"rate limit exceeded, retry after %v",
				retryAfter,
			)
		}

		// Set trailer with rate limit info for successful requests too
		if limitInfo.Limit > 0 {
			trailer := metadata.Pairs(
				"x-ratelimit-limit", strconv.Itoa(limitInfo.Limit),
				"x-ratelimit-remaining", strconv.Itoa(limitInfo.Remaining),
			)
			ss.SetTrailer(trailer)
		}

		// Wrap the stream with the updated context
		wrappedStream := &rateLimitedServerStream{
			ServerStream: ss,
			ctx:          ctx,
		}

		return handler(srv, wrappedStream)
	}
}

// rateLimitedServerStream wraps a gRPC server stream with a rate-limited context.
type rateLimitedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

// Context returns the rate-limited context.
func (s *rateLimitedServerStream) Context() context.Context {
	return s.ctx
}

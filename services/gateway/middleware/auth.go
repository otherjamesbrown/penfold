// Package middleware provides gRPC interceptors for the API Gateway.
// It implements authentication, authorization, and tenant extraction middleware.
package middleware

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/otherjamesbrown/penfold/pkg/auth"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// Context keys for tenant information.
const (
	// TenantIDKey is the context key for storing the tenant ID.
	TenantIDKey auth.ContextKey = "tenant_id"
)

// Header/metadata keys for authentication.
const (
	// AuthorizationKey is the metadata key for bearer tokens.
	AuthorizationKey = "authorization"
	// APIKeyKey is the metadata key for API keys.
	APIKeyKey = "x-api-key"
	// TenantIDHeaderKey is the metadata key for tenant ID.
	TenantIDHeaderKey = "x-tenant-id"
)

// AuthConfig holds configuration for the authentication middleware.
type AuthConfig struct {
	// JWTValidator validates JWT tokens.
	JWTValidator auth.TokenValidator
	// APIKeyValidator validates API keys.
	APIKeyValidator *auth.APIKeyValidator
	// Logger for authentication events.
	Logger logging.Logger
	// SkipMethods is a list of gRPC methods to skip authentication for.
	// Format: "/package.service/method"
	SkipMethods []string
	// RequireTenant when true, requires a tenant ID to be present.
	RequireTenant bool
}

// AuthMiddleware provides combined authentication middleware that supports
// both JWT and API key authentication with tenant extraction.
type AuthMiddleware struct {
	config      *AuthConfig
	skipMethods map[string]bool
}

// NewAuthMiddleware creates a new authentication middleware instance.
func NewAuthMiddleware(cfg *AuthConfig) *AuthMiddleware {
	if cfg.Logger == nil {
		cfg.Logger = logging.Global()
	}

	skipMethods := make(map[string]bool)
	for _, m := range cfg.SkipMethods {
		skipMethods[m] = true
	}

	return &AuthMiddleware{
		config:      cfg,
		skipMethods: skipMethods,
	}
}

// UnaryInterceptor returns a gRPC unary server interceptor that performs authentication.
// It tries JWT authentication first, then falls back to API key authentication.
// The authenticated claims and tenant ID are stored in the context.
func (m *AuthMiddleware) UnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// Skip authentication for certain methods
		if m.skipMethods[info.FullMethod] {
			return handler(ctx, req)
		}

		// Perform authentication
		ctx, err := m.authenticate(ctx)
		if err != nil {
			m.config.Logger.Warn("Authentication failed",
				logging.F("method", info.FullMethod),
				logging.Err(err),
			)
			return nil, err
		}

		// Extract and validate tenant
		ctx, err = m.extractTenant(ctx)
		if err != nil {
			m.config.Logger.Warn("Tenant extraction failed",
				logging.F("method", info.FullMethod),
				logging.Err(err),
			)
			return nil, err
		}

		return handler(ctx, req)
	}
}

// StreamInterceptor returns a gRPC stream server interceptor that performs authentication.
func (m *AuthMiddleware) StreamInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		// Skip authentication for certain methods
		if m.skipMethods[info.FullMethod] {
			return handler(srv, ss)
		}

		// Perform authentication
		ctx, err := m.authenticate(ss.Context())
		if err != nil {
			m.config.Logger.Warn("Stream authentication failed",
				logging.F("method", info.FullMethod),
				logging.Err(err),
			)
			return err
		}

		// Extract and validate tenant
		ctx, err = m.extractTenant(ctx)
		if err != nil {
			m.config.Logger.Warn("Stream tenant extraction failed",
				logging.F("method", info.FullMethod),
				logging.Err(err),
			)
			return err
		}

		// Wrap the stream with the authenticated context
		wrapped := &authenticatedServerStream{
			ServerStream: ss,
			ctx:          ctx,
		}

		return handler(srv, wrapped)
	}
}

// authenticate performs authentication using JWT or API key.
// Returns the context with claims attached or an error.
func (m *AuthMiddleware) authenticate(ctx context.Context) (context.Context, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing metadata")
	}

	// Try JWT authentication first
	if m.config.JWTValidator != nil {
		ctx, err := m.authenticateJWT(ctx, md)
		if err == nil {
			return ctx, nil
		}
		// Only log non-missing token errors
		if err != auth.ErrMissingToken {
			m.config.Logger.Debug("JWT authentication failed, trying API key",
				logging.Err(err),
			)
		}
	}

	// Try API key authentication
	if m.config.APIKeyValidator != nil {
		ctx, err := m.authenticateAPIKey(ctx, md)
		if err == nil {
			return ctx, nil
		}
		// Only log non-missing key errors
		if err != auth.ErrMissingAPIKey {
			m.config.Logger.Debug("API key authentication failed",
				logging.Err(err),
			)
		}
	}

	return nil, status.Error(codes.Unauthenticated, "authentication required")
}

// authenticateJWT validates a JWT token from the authorization header.
func (m *AuthMiddleware) authenticateJWT(ctx context.Context, md metadata.MD) (context.Context, error) {
	values := md.Get(AuthorizationKey)
	if len(values) == 0 {
		return nil, auth.ErrMissingToken
	}

	authHeader := values[0]
	var token string

	// Check for "Bearer " prefix (case-insensitive)
	if strings.HasPrefix(authHeader, auth.BearerPrefix) {
		token = strings.TrimPrefix(authHeader, auth.BearerPrefix)
	} else if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		token = authHeader[7:]
	} else {
		return nil, auth.ErrInvalidToken
	}

	claims, err := m.config.JWTValidator.ValidateToken(token)
	if err != nil {
		return nil, convertAuthError(err)
	}

	// Mark auth method
	if claims.Custom == nil {
		claims.Custom = make(map[string]interface{})
	}
	claims.Custom["auth_method"] = "jwt"

	// Store claims in context
	ctx = context.WithValue(ctx, auth.ClaimsKey, claims)
	return ctx, nil
}

// authenticateAPIKey validates an API key from the x-api-key header.
func (m *AuthMiddleware) authenticateAPIKey(ctx context.Context, md metadata.MD) (context.Context, error) {
	values := md.Get(APIKeyKey)
	if len(values) == 0 {
		return nil, auth.ErrMissingAPIKey
	}

	apiKey := values[0]
	info, err := m.config.APIKeyValidator.ValidateAPIKey(apiKey)
	if err != nil {
		return nil, convertAuthError(err)
	}

	// Create claims from API key info
	claims := &auth.Claims{
		Subject: info.ServiceName,
		Roles:   info.Roles,
		Custom:  make(map[string]interface{}),
	}
	claims.Custom["api_key_id"] = info.ID
	claims.Custom["auth_method"] = "api_key"
	for k, v := range info.Metadata {
		claims.Custom[k] = v
	}

	// Store claims in context
	ctx = context.WithValue(ctx, auth.ClaimsKey, claims)
	return ctx, nil
}

// extractTenant extracts the tenant ID from metadata or claims.
func (m *AuthMiddleware) extractTenant(ctx context.Context) (context.Context, error) {
	md, _ := metadata.FromIncomingContext(ctx)

	// First, try to get tenant from explicit header
	if values := md.Get(TenantIDHeaderKey); len(values) > 0 && values[0] != "" {
		ctx = context.WithValue(ctx, TenantIDKey, values[0])
		return ctx, nil
	}

	// Try to get tenant from claims
	claims := auth.GetClaims(ctx)
	if claims != nil && claims.Custom != nil {
		if tenantID, ok := claims.Custom["tenant_id"].(string); ok && tenantID != "" {
			ctx = context.WithValue(ctx, TenantIDKey, tenantID)
			return ctx, nil
		}
	}

	// If tenant is required but not found, return error
	if m.config.RequireTenant {
		return nil, status.Error(codes.InvalidArgument, "tenant ID required")
	}

	return ctx, nil
}

// convertAuthError converts auth package errors to gRPC status errors.
func convertAuthError(err error) error {
	switch err {
	case auth.ErrMissingToken, auth.ErrMissingAPIKey:
		return status.Error(codes.Unauthenticated, err.Error())
	case auth.ErrTokenExpired:
		return status.Error(codes.Unauthenticated, "token expired")
	case auth.ErrInvalidToken, auth.ErrInvalidAPIKey:
		return status.Error(codes.Unauthenticated, "invalid credentials")
	case auth.ErrInvalidClaims:
		return status.Error(codes.Unauthenticated, "invalid token claims")
	case auth.ErrInsufficientPermissions:
		return status.Error(codes.PermissionDenied, "insufficient permissions")
	default:
		return status.Error(codes.Unauthenticated, "authentication failed")
	}
}

// authenticatedServerStream wraps a gRPC server stream with an authenticated context.
type authenticatedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

// Context returns the authenticated context.
func (s *authenticatedServerStream) Context() context.Context {
	return s.ctx
}

// JWTAuthMiddleware creates a gRPC unary interceptor that only validates JWT tokens.
// For more flexible authentication, use AuthMiddleware instead.
func JWTAuthMiddleware(validator auth.TokenValidator, skipMethods ...string) grpc.UnaryServerInterceptor {
	m := NewAuthMiddleware(&AuthConfig{
		JWTValidator: validator,
		SkipMethods:  skipMethods,
	})
	return m.UnaryInterceptor()
}

// APIKeyAuthMiddleware creates a gRPC unary interceptor that only validates API keys.
// For more flexible authentication, use AuthMiddleware instead.
func APIKeyAuthMiddleware(validator *auth.APIKeyValidator, skipMethods ...string) grpc.UnaryServerInterceptor {
	m := NewAuthMiddleware(&AuthConfig{
		APIKeyValidator: validator,
		SkipMethods:     skipMethods,
	})
	return m.UnaryInterceptor()
}

// GetTenantID extracts the tenant ID from the context.
// Returns an empty string if no tenant ID is present.
func GetTenantID(ctx context.Context) string {
	tenantID, ok := ctx.Value(TenantIDKey).(string)
	if !ok {
		return ""
	}
	return tenantID
}

// RequireTenantID extracts the tenant ID from the context and returns an error if not present.
func RequireTenantID(ctx context.Context) (string, error) {
	tenantID := GetTenantID(ctx)
	if tenantID == "" {
		return "", status.Error(codes.InvalidArgument, "tenant ID required")
	}
	return tenantID, nil
}

// TenantExtractor creates a gRPC unary interceptor that extracts tenant ID from metadata.
// This can be used independently or chained with authentication middleware.
func TenantExtractor(requireTenant bool) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok && requireTenant {
			return nil, status.Error(codes.InvalidArgument, "missing metadata")
		}

		// Try to get tenant from explicit header
		if ok {
			if values := md.Get(TenantIDHeaderKey); len(values) > 0 && values[0] != "" {
				ctx = context.WithValue(ctx, TenantIDKey, values[0])
				return handler(ctx, req)
			}
		}

		// Try to get tenant from claims
		claims := auth.GetClaims(ctx)
		if claims != nil && claims.Custom != nil {
			if tenantID, ok := claims.Custom["tenant_id"].(string); ok && tenantID != "" {
				ctx = context.WithValue(ctx, TenantIDKey, tenantID)
				return handler(ctx, req)
			}
		}

		// If tenant is required but not found, return error
		if requireTenant {
			return nil, status.Error(codes.InvalidArgument, "tenant ID required")
		}

		return handler(ctx, req)
	}
}

// ChainUnaryInterceptors chains multiple unary interceptors into a single interceptor.
// Interceptors are executed in the order they are provided.
func ChainUnaryInterceptors(interceptors ...grpc.UnaryServerInterceptor) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// Build the chain from the innermost handler outward
		chain := handler
		for i := len(interceptors) - 1; i >= 0; i-- {
			interceptor := interceptors[i]
			next := chain
			chain = func(ctx context.Context, req interface{}) (interface{}, error) {
				return interceptor(ctx, req, info, next)
			}
		}
		return chain(ctx, req)
	}
}

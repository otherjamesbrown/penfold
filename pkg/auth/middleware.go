package auth

import (
	"context"
	"net/http"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// ContextKey type for context values to avoid collisions.
type ContextKey string

// Context keys for authentication.
const (
	// ClaimsKey is the context key for storing authenticated claims.
	ClaimsKey ContextKey = "auth_claims"
)

// Header constants for authentication.
const (
	// AuthorizationHeader is the standard HTTP authorization header.
	AuthorizationHeader = "Authorization"
	// BearerPrefix is the prefix for bearer tokens in the Authorization header.
	BearerPrefix = "Bearer "
)

// HTTPMiddleware returns HTTP middleware that validates JWT tokens.
// It extracts the token from the Authorization header and validates it.
// If validation succeeds, the claims are stored in the request context.
func HTTPMiddleware(validator TokenValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, err := extractBearerToken(r)
			if err != nil {
				http.Error(w, err.Error(), http.StatusUnauthorized)
				return
			}

			claims, err := validator.ValidateToken(token)
			if err != nil {
				switch err {
				case ErrTokenExpired:
					http.Error(w, "token expired", http.StatusUnauthorized)
				case ErrMissingToken:
					http.Error(w, "missing token", http.StatusUnauthorized)
				default:
					http.Error(w, "invalid token", http.StatusUnauthorized)
				}
				return
			}

			// Store claims in context
			ctx := context.WithValue(r.Context(), ClaimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// HTTPMiddlewareOptional returns HTTP middleware that validates JWT tokens but allows unauthenticated requests.
// If a valid token is provided, claims are added to the context.
// If no token or an invalid token is provided, the request continues without claims.
func HTTPMiddlewareOptional(validator TokenValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, err := extractBearerToken(r)
			if err == nil && token != "" {
				claims, err := validator.ValidateToken(token)
				if err == nil {
					ctx := context.WithValue(r.Context(), ClaimsKey, claims)
					r = r.WithContext(ctx)
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// extractBearerToken extracts the bearer token from the Authorization header.
func extractBearerToken(r *http.Request) (string, error) {
	authHeader := r.Header.Get(AuthorizationHeader)
	if authHeader == "" {
		return "", ErrMissingToken
	}

	if !strings.HasPrefix(authHeader, BearerPrefix) {
		return "", ErrInvalidToken
	}

	return strings.TrimPrefix(authHeader, BearerPrefix), nil
}

// GRPCUnaryInterceptor returns a gRPC unary server interceptor for authentication.
// It extracts the token from the "authorization" metadata and validates it.
func GRPCUnaryInterceptor(validator TokenValidator) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		token, err := extractGRPCToken(ctx)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, err.Error())
		}

		claims, err := validator.ValidateToken(token)
		if err != nil {
			switch err {
			case ErrTokenExpired:
				return nil, status.Errorf(codes.Unauthenticated, "token expired")
			case ErrMissingToken:
				return nil, status.Errorf(codes.Unauthenticated, "missing token")
			default:
				return nil, status.Errorf(codes.Unauthenticated, "invalid token")
			}
		}

		// Store claims in context
		ctx = context.WithValue(ctx, ClaimsKey, claims)
		return handler(ctx, req)
	}
}

// GRPCStreamInterceptor returns a gRPC stream server interceptor for authentication.
func GRPCStreamInterceptor(validator TokenValidator) grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		token, err := extractGRPCToken(ss.Context())
		if err != nil {
			return status.Error(codes.Unauthenticated, err.Error())
		}

		claims, err := validator.ValidateToken(token)
		if err != nil {
			switch err {
			case ErrTokenExpired:
				return status.Errorf(codes.Unauthenticated, "token expired")
			case ErrMissingToken:
				return status.Errorf(codes.Unauthenticated, "missing token")
			default:
				return status.Errorf(codes.Unauthenticated, "invalid token")
			}
		}

		// Wrap the stream with authenticated context
		wrappedStream := &authenticatedServerStream{
			ServerStream: ss,
			ctx:          context.WithValue(ss.Context(), ClaimsKey, claims),
		}

		return handler(srv, wrappedStream)
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

// extractGRPCToken extracts the bearer token from gRPC metadata.
func extractGRPCToken(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", ErrMissingToken
	}

	values := md.Get("authorization")
	if len(values) == 0 {
		return "", ErrMissingToken
	}

	authHeader := values[0]
	if !strings.HasPrefix(authHeader, BearerPrefix) {
		// Also check lowercase "bearer " for compatibility
		if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
			return authHeader[7:], nil
		}
		return "", ErrInvalidToken
	}

	return strings.TrimPrefix(authHeader, BearerPrefix), nil
}

// GetClaims extracts the authentication claims from the context.
// Returns nil if no claims are present (request not authenticated).
func GetClaims(ctx context.Context) *Claims {
	claims, ok := ctx.Value(ClaimsKey).(*Claims)
	if !ok {
		return nil
	}
	return claims
}

// RequireClaims extracts claims from context and returns an error if not present.
func RequireClaims(ctx context.Context) (*Claims, error) {
	claims := GetClaims(ctx)
	if claims == nil {
		return nil, ErrMissingToken
	}
	return claims, nil
}

// RequireRole checks if the authenticated user has the specified role.
// Returns an error if not authenticated or if the role is missing.
func RequireRole(ctx context.Context, role string) error {
	claims := GetClaims(ctx)
	if claims == nil {
		return ErrMissingToken
	}
	if !claims.HasRole(role) {
		return ErrInsufficientPermissions
	}
	return nil
}

// RequireAnyRole checks if the authenticated user has any of the specified roles.
func RequireAnyRole(ctx context.Context, roles ...string) error {
	claims := GetClaims(ctx)
	if claims == nil {
		return ErrMissingToken
	}
	for _, role := range roles {
		if claims.HasRole(role) {
			return nil
		}
	}
	return ErrInsufficientPermissions
}

// RequireAllRoles checks if the authenticated user has all of the specified roles.
func RequireAllRoles(ctx context.Context, roles ...string) error {
	claims := GetClaims(ctx)
	if claims == nil {
		return ErrMissingToken
	}
	for _, role := range roles {
		if !claims.HasRole(role) {
			return ErrInsufficientPermissions
		}
	}
	return nil
}

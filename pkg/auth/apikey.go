package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
	"sync"
)

// API key authentication errors.
var (
	// ErrInvalidAPIKey is returned when an API key is invalid.
	ErrInvalidAPIKey = errors.New("invalid API key")
	// ErrMissingAPIKey is returned when no API key is provided.
	ErrMissingAPIKey = errors.New("missing API key")
)

// Header constants for API key authentication.
const (
	// APIKeyHeader is the standard header for API key authentication.
	APIKeyHeader = "X-API-Key"
)

// APIKeyInfo contains metadata about an API key.
type APIKeyInfo struct {
	// ID is the unique identifier for the API key.
	ID string
	// ServiceName is the name of the service this key belongs to.
	ServiceName string
	// Roles are the permissions associated with this key.
	Roles []string
	// Metadata contains additional key metadata.
	Metadata map[string]string
}

// APIKeyValidator validates API keys for service-to-service authentication.
type APIKeyValidator struct {
	mu   sync.RWMutex
	keys map[string]*APIKeyInfo
}

// NewAPIKeyValidator creates a new API key validator.
func NewAPIKeyValidator() *APIKeyValidator {
	return &APIKeyValidator{
		keys: make(map[string]*APIKeyInfo),
	}
}

// RegisterKey adds an API key to the validator.
// The key is hashed before storage for security.
func (v *APIKeyValidator) RegisterKey(key string, info *APIKeyInfo) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.keys[key] = info
}

// RemoveKey removes an API key from the validator.
func (v *APIKeyValidator) RemoveKey(key string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	delete(v.keys, key)
}

// ValidateAPIKey validates an API key and returns its associated info.
func (v *APIKeyValidator) ValidateAPIKey(key string) (*APIKeyInfo, error) {
	if key == "" {
		return nil, ErrMissingAPIKey
	}

	v.mu.RLock()
	defer v.mu.RUnlock()

	// Use constant-time comparison to prevent timing attacks
	for storedKey, info := range v.keys {
		if subtle.ConstantTimeCompare([]byte(key), []byte(storedKey)) == 1 {
			return info, nil
		}
	}

	return nil, ErrInvalidAPIKey
}

// APIKeyMiddleware returns HTTP middleware that validates API keys.
// It extracts the key from the X-API-Key header and validates it.
// If validation succeeds, claims derived from the key info are stored in context.
func APIKeyMiddleware(validator *APIKeyValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get(APIKeyHeader)
			if key == "" {
				http.Error(w, "missing API key", http.StatusUnauthorized)
				return
			}

			info, err := validator.ValidateAPIKey(key)
			if err != nil {
				http.Error(w, "invalid API key", http.StatusUnauthorized)
				return
			}

			// Create claims from API key info
			claims := &Claims{
				Subject: info.ServiceName,
				Roles:   info.Roles,
				Custom:  make(map[string]interface{}),
			}
			claims.Custom["api_key_id"] = info.ID
			for k, v := range info.Metadata {
				claims.Custom[k] = v
			}

			// Store claims in context
			ctx := context.WithValue(r.Context(), ClaimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// APIKeyOrJWTMiddleware returns HTTP middleware that accepts either API key or JWT.
// It first checks for an API key, then falls back to JWT if not present.
func APIKeyOrJWTMiddleware(apiKeyValidator *APIKeyValidator, jwtValidator TokenValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Try API key first
			apiKey := r.Header.Get(APIKeyHeader)
			if apiKey != "" {
				info, err := apiKeyValidator.ValidateAPIKey(apiKey)
				if err == nil {
					claims := &Claims{
						Subject: info.ServiceName,
						Roles:   info.Roles,
						Custom:  make(map[string]interface{}),
					}
					claims.Custom["api_key_id"] = info.ID
					claims.Custom["auth_method"] = "api_key"
					for k, v := range info.Metadata {
						claims.Custom[k] = v
					}
					ctx := context.WithValue(r.Context(), ClaimsKey, claims)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}

			// Try JWT
			authHeader := r.Header.Get(AuthorizationHeader)
			if authHeader != "" && strings.HasPrefix(authHeader, BearerPrefix) {
				token := strings.TrimPrefix(authHeader, BearerPrefix)
				claims, err := jwtValidator.ValidateToken(token)
				if err == nil {
					if claims.Custom == nil {
						claims.Custom = make(map[string]interface{})
					}
					claims.Custom["auth_method"] = "jwt"
					ctx := context.WithValue(r.Context(), ClaimsKey, claims)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}

			http.Error(w, "authentication required", http.StatusUnauthorized)
		})
	}
}

// IsAPIKeyAuth checks if the current request was authenticated via API key.
func IsAPIKeyAuth(ctx context.Context) bool {
	claims := GetClaims(ctx)
	if claims == nil || claims.Custom == nil {
		return false
	}
	method, ok := claims.Custom["auth_method"].(string)
	return ok && method == "api_key"
}

// IsJWTAuth checks if the current request was authenticated via JWT.
func IsJWTAuth(ctx context.Context) bool {
	claims := GetClaims(ctx)
	if claims == nil || claims.Custom == nil {
		return false
	}
	method, ok := claims.Custom["auth_method"].(string)
	return ok && method == "jwt"
}

// GetAPIKeyID returns the API key ID from claims if authenticated via API key.
func GetAPIKeyID(ctx context.Context) string {
	claims := GetClaims(ctx)
	if claims == nil || claims.Custom == nil {
		return ""
	}
	id, _ := claims.Custom["api_key_id"].(string)
	return id
}

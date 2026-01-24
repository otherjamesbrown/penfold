// Package middleware provides HTTP middleware for the API Gateway.
// This file implements CSRF protection middleware for browser-based requests.
package middleware

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/gorilla/csrf"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// CSRFConfig holds configuration for the CSRF protection middleware.
type CSRFConfig struct {
	// SecretKey is a 32-byte secret key used for CSRF token generation.
	// This should be kept secret and consistent across server restarts.
	SecretKey []byte

	// CookieName is the name of the CSRF cookie (default: "_csrf").
	CookieName string

	// HeaderName is the name of the header that should contain the CSRF token
	// for AJAX requests (default: "X-CSRF-Token").
	HeaderName string

	// FieldName is the name of the form field that should contain the CSRF token
	// for form submissions (default: "csrf_token").
	FieldName string

	// Secure sets the Secure flag on the CSRF cookie.
	// Should be true in production (HTTPS only).
	Secure bool

	// SameSite controls the SameSite attribute of the CSRF cookie.
	// Use csrf.SameSiteStrictMode for maximum protection.
	SameSite csrf.SameSiteMode

	// Path sets the cookie path (default: "/").
	Path string

	// Domain sets the cookie domain (empty for current domain).
	Domain string

	// MaxAge sets the cookie max age in seconds (default: 12 hours).
	MaxAge int

	// ExemptPaths is a list of path prefixes to exempt from CSRF protection.
	// Typically used for health checks, metrics, and webhook endpoints.
	ExemptPaths []string

	// ExemptMethods is a list of HTTP methods exempt from CSRF checks.
	// By default, GET, HEAD, OPTIONS, and TRACE are safe methods.
	ExemptMethods []string

	// TrustedOrigins is a list of trusted origins for referer checking.
	TrustedOrigins []string

	// Logger for CSRF events.
	Logger logging.Logger
}

// CSRFMiddleware provides CSRF protection for HTTP endpoints.
// It wraps gorilla/csrf with additional configuration for API-first services.
type CSRFMiddleware struct {
	config     *CSRFConfig
	protect    func(http.Handler) http.Handler
	exemptSet  map[string]bool
	methodSet  map[string]bool
	logger     logging.Logger
}

// DefaultCSRFConfig returns a CSRFConfig with secure defaults.
// The secret key must be set before use.
func DefaultCSRFConfig() *CSRFConfig {
	return &CSRFConfig{
		CookieName:    "_csrf",
		HeaderName:    "X-CSRF-Token",
		FieldName:     "csrf_token",
		Secure:        true,
		SameSite:      csrf.SameSiteStrictMode,
		Path:          "/",
		MaxAge:        43200, // 12 hours
		ExemptPaths:   []string{"/health", "/ready", "/live", "/metrics"},
		ExemptMethods: []string{"GET", "HEAD", "OPTIONS", "TRACE"},
	}
}

// NewCSRFMiddleware creates a new CSRF protection middleware.
// If cfg.SecretKey is not provided, a random key will be generated.
// Note: Random keys won't persist across restarts, which may affect user experience.
func NewCSRFMiddleware(cfg *CSRFConfig) (*CSRFMiddleware, error) {
	if cfg == nil {
		cfg = DefaultCSRFConfig()
	}

	// Generate random secret if not provided
	if len(cfg.SecretKey) == 0 {
		cfg.SecretKey = make([]byte, 32)
		if _, err := rand.Read(cfg.SecretKey); err != nil {
			return nil, err
		}
	}

	// Ensure secret is exactly 32 bytes
	if len(cfg.SecretKey) != 32 {
		// Pad or truncate to 32 bytes
		key := make([]byte, 32)
		copy(key, cfg.SecretKey)
		cfg.SecretKey = key
	}

	logger := cfg.Logger
	if logger == nil {
		logger = logging.Global()
	}

	// Build exempt path set for fast lookup
	exemptSet := make(map[string]bool)
	for _, p := range cfg.ExemptPaths {
		exemptSet[p] = true
	}

	// Build exempt method set for fast lookup
	methodSet := make(map[string]bool)
	for _, m := range cfg.ExemptMethods {
		methodSet[strings.ToUpper(m)] = true
	}

	// Build gorilla/csrf options
	opts := []csrf.Option{
		csrf.CookieName(cfg.CookieName),
		csrf.RequestHeader(cfg.HeaderName),
		csrf.FieldName(cfg.FieldName),
		csrf.Secure(cfg.Secure),
		csrf.SameSite(cfg.SameSite),
		csrf.Path(cfg.Path),
		csrf.MaxAge(cfg.MaxAge),
		csrf.HttpOnly(true), // Always set HttpOnly for security
		csrf.ErrorHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			logger.Warn("CSRF validation failed",
				logging.F("path", r.URL.Path),
				logging.F("method", r.Method),
				logging.F("remote_addr", r.RemoteAddr),
			)
			http.Error(w, "CSRF token validation failed", http.StatusForbidden)
		})),
	}

	if cfg.Domain != "" {
		opts = append(opts, csrf.Domain(cfg.Domain))
	}

	if len(cfg.TrustedOrigins) > 0 {
		opts = append(opts, csrf.TrustedOrigins(cfg.TrustedOrigins))
	}

	// Create the gorilla/csrf protection function
	protect := csrf.Protect(cfg.SecretKey, opts...)

	return &CSRFMiddleware{
		config:    cfg,
		protect:   protect,
		exemptSet: exemptSet,
		methodSet: methodSet,
		logger:    logger,
	}, nil
}

// Handler returns an http.Handler that wraps the given handler with CSRF protection.
// Paths in ExemptPaths are not protected, allowing webhooks and API endpoints to work.
func (m *CSRFMiddleware) Handler(next http.Handler) http.Handler {
	// Wrap the handler with CSRF protection
	protected := m.protect(next)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if path is exempt
		if m.isExemptPath(r.URL.Path) {
			m.logger.Debug("CSRF check skipped (exempt path)",
				logging.F("path", r.URL.Path),
			)
			next.ServeHTTP(w, r)
			return
		}

		// Check for API authentication headers that bypass CSRF
		// JWT Bearer token or API key indicates a programmatic client
		if m.hasAPIAuthentication(r) {
			m.logger.Debug("CSRF check skipped (API auth present)",
				logging.F("path", r.URL.Path),
				logging.F("auth_type", m.getAuthType(r)),
			)
			next.ServeHTTP(w, r)
			return
		}

		// Apply CSRF protection
		protected.ServeHTTP(w, r)
	})
}

// isExemptPath checks if the request path matches any exempt path.
// Paths ending with "/" are treated as prefix patterns.
// Paths not ending with "/" require exact matches.
func (m *CSRFMiddleware) isExemptPath(path string) bool {
	// Direct match
	if m.exemptSet[path] {
		return true
	}

	// Prefix match only for paths ending with "/" (explicit prefix patterns)
	for exemptPath := range m.exemptSet {
		if strings.HasSuffix(exemptPath, "/") && strings.HasPrefix(path, exemptPath) {
			return true
		}
	}

	return false
}

// hasAPIAuthentication checks if the request contains API authentication headers.
// Requests with JWT Bearer tokens or API keys are considered programmatic
// and should bypass CSRF checks (they have their own authentication).
func (m *CSRFMiddleware) hasAPIAuthentication(r *http.Request) bool {
	// Check for Bearer token (JWT)
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		if strings.HasPrefix(authHeader, "Bearer ") ||
			strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
			return true
		}
	}

	// Check for API key header
	if r.Header.Get("X-API-Key") != "" {
		return true
	}

	return false
}

// getAuthType returns the type of API authentication present.
func (m *CSRFMiddleware) getAuthType(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") ||
		strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		return "bearer"
	}
	if r.Header.Get("X-API-Key") != "" {
		return "api_key"
	}
	return "none"
}

// Token returns the CSRF token for the current request.
// This should be called within a handler wrapped by the CSRF middleware.
// The token can be rendered in forms or included in AJAX request headers.
func Token(r *http.Request) string {
	return csrf.Token(r)
}

// TemplateField returns a hidden form field containing the CSRF token.
// This is useful for rendering in HTML templates.
func TemplateField(r *http.Request) string {
	return string(csrf.TemplateField(r))
}

// GenerateSecretKey generates a cryptographically secure 32-byte secret key
// suitable for CSRF token generation. The returned string is base64-encoded.
func GenerateSecretKey() (string, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(key), nil
}

// DecodeSecretKey decodes a base64-encoded secret key to bytes.
func DecodeSecretKey(encoded string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(encoded)
}

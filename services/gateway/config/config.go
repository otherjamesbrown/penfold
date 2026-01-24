// Package config provides gateway service-specific configuration.
// It extends the shared pkg/config with gateway-specific settings.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/config"
)

// GatewayConfig holds configuration specific to the API Gateway service.
type GatewayConfig struct {
	// Base contains shared configuration from pkg/config.
	Base *config.Config

	// GRPCPort is the port for the gRPC server (default: 50051).
	GRPCPort int

	// HTTPPort is the port for the HTTP server (health checks, metrics) (default: 8080).
	HTTPPort int

	// AuthEnabled enables authentication middleware when true.
	AuthEnabled bool

	// Auth contains authentication-specific configuration.
	Auth AuthConfig

	// CSRFEnabled enables CSRF protection for HTTP endpoints.
	CSRFEnabled bool

	// CSRF contains CSRF protection configuration.
	CSRF CSRFConfig

	// RateLimitEnabled enables rate limiting when true.
	RateLimitEnabled bool

	// RateLimitRPS is the maximum requests per second when rate limiting is enabled.
	// Deprecated: Use RateLimit.DefaultRPS instead.
	RateLimitRPS int

	// RateLimit contains detailed rate limiting configuration.
	RateLimit RateLimitConfig

	// OrchestratorAddress is the gRPC address of the orchestrator service.
	OrchestratorAddress string

	// SearchAddress is the gRPC address of the search service.
	SearchAddress string

	// DailyReviewAddress is the gRPC address of the daily review service.
	DailyReviewAddress string

	// WorkerHealthURL is the URL for the worker health endpoint (on dev01).
	WorkerHealthURL string

	// EmbeddingsURL is the URL for the MLX embeddings service (on dev01).
	EmbeddingsURL string

	// LLMURL is the URL for the MLX LLM server (on dev01).
	LLMURL string
}

// AuthConfig holds authentication configuration for the gateway.
type AuthConfig struct {
	// JWTSecretKey is the secret key used to validate JWT tokens.
	JWTSecretKey string

	// JWTIssuer is the expected issuer for JWT tokens (optional).
	JWTIssuer string

	// RequireTenant when true, requires a tenant ID to be present in requests.
	RequireTenant bool

	// SkipAuthMethods is a list of gRPC methods to skip authentication for.
	// Format: "/package.service/method"
	SkipAuthMethods []string
}

// CSRFConfig holds CSRF protection configuration for the gateway.
type CSRFConfig struct {
	// Enabled enables CSRF protection for HTTP endpoints.
	Enabled bool

	// SecretKey is the base64-encoded 32-byte secret key for CSRF tokens.
	// If not provided, a random key will be generated (not recommended for production).
	SecretKey string

	// Secure sets the Secure flag on cookies (should be true for HTTPS).
	Secure bool

	// Domain sets the cookie domain (empty for current domain).
	Domain string

	// ExemptPaths is a list of path prefixes to exempt from CSRF protection.
	// Paths ending with "/" are treated as prefix patterns.
	ExemptPaths []string

	// TrustedOrigins is a list of trusted origins for referer checking.
	TrustedOrigins []string
}

// RateLimitConfig holds rate limiting configuration for the gateway.
type RateLimitConfig struct {
	// DefaultRPS is the default requests per second allowed per tenant.
	DefaultRPS float64

	// DefaultBurst is the default burst capacity (maximum tokens).
	DefaultBurst int

	// CleanupInterval is how often to clean up expired rate limit buckets.
	CleanupInterval time.Duration

	// BucketTTL is how long to keep inactive rate limit buckets.
	BucketTTL time.Duration

	// SkipMethods is a list of gRPC methods to skip rate limiting for.
	// Format: "/package.service/method"
	SkipMethods []string

	// SkipPaths is a list of HTTP paths to skip rate limiting for.
	SkipPaths []string

	// IncludeHeaders when true, includes rate limit headers in HTTP responses.
	IncludeHeaders bool
}

// Default configuration values for the gateway.
const (
	DefaultGRPCPort             = 50051
	DefaultHTTPPort             = 8080
	DefaultAuthEnabled          = false
	DefaultCSRFEnabled          = false
	DefaultRateLimitEnabled     = false
	DefaultRateLimitRPS         = 100
	DefaultOrchestratorAddress  = "localhost:50052"
	DefaultSearchAddress        = "localhost:50053"
	DefaultDailyReviewAddress   = "localhost:50054"

	// Rate limit defaults.
	DefaultRateLimitDefaultRPS   = 100.0
	DefaultRateLimitDefaultBurst = 150
	DefaultRateLimitCleanup      = 5 * time.Minute
	DefaultRateLimitBucketTTL    = 10 * time.Minute
)

// Load loads the gateway configuration from environment variables.
// It first loads the base configuration, then overlays gateway-specific settings.
func Load() (*GatewayConfig, error) {
	// Load base configuration from pkg/config.
	// Set service name if not already set.
	if os.Getenv("PENFOLD_SERVICE_NAME") == "" {
		os.Setenv("PENFOLD_SERVICE_NAME", "gateway")
	}

	baseCfg, err := config.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("loading base config: %w", err)
	}

	cfg := &GatewayConfig{
		Base:                baseCfg,
		GRPCPort:            DefaultGRPCPort,
		HTTPPort:            DefaultHTTPPort,
		AuthEnabled:         DefaultAuthEnabled,
		CSRFEnabled:         DefaultCSRFEnabled,
		RateLimitEnabled:    DefaultRateLimitEnabled,
		RateLimitRPS:        DefaultRateLimitRPS,
		OrchestratorAddress: DefaultOrchestratorAddress,
		SearchAddress:       DefaultSearchAddress,
		DailyReviewAddress:  DefaultDailyReviewAddress,
		CSRF: CSRFConfig{
			Enabled:     DefaultCSRFEnabled,
			Secure:      true,
			ExemptPaths: []string{"/health", "/ready", "/live", "/metrics", "/api/webhooks/"},
		},
		RateLimit: RateLimitConfig{
			DefaultRPS:      DefaultRateLimitDefaultRPS,
			DefaultBurst:    DefaultRateLimitDefaultBurst,
			CleanupInterval: DefaultRateLimitCleanup,
			BucketTTL:       DefaultRateLimitBucketTTL,
			SkipMethods:     []string{},
			SkipPaths:       []string{"/health", "/ready", "/live", "/metrics"},
			IncludeHeaders:  true,
		},
	}

	// Override from environment variables.
	loadGatewayEnv(cfg)

	return cfg, nil
}

// loadGatewayEnv loads gateway-specific configuration from environment variables.
func loadGatewayEnv(cfg *GatewayConfig) {
	if v := os.Getenv("GATEWAY_GRPC_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil && port > 0 && port <= 65535 {
			cfg.GRPCPort = port
		}
	}

	if v := os.Getenv("GATEWAY_HTTP_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil && port > 0 && port <= 65535 {
			cfg.HTTPPort = port
		}
	}

	if v := os.Getenv("GATEWAY_AUTH_ENABLED"); v != "" {
		cfg.AuthEnabled = v == "true" || v == "1"
	}

	// Auth configuration
	if v := os.Getenv("GATEWAY_JWT_SECRET_KEY"); v != "" {
		cfg.Auth.JWTSecretKey = v
	}

	if v := os.Getenv("GATEWAY_JWT_ISSUER"); v != "" {
		cfg.Auth.JWTIssuer = v
	}

	if v := os.Getenv("GATEWAY_REQUIRE_TENANT"); v != "" {
		cfg.Auth.RequireTenant = v == "true" || v == "1"
	}

	if v := os.Getenv("GATEWAY_SKIP_AUTH_METHODS"); v != "" {
		// Comma-separated list of methods to skip
		methods := strings.Split(v, ",")
		for i := range methods {
			methods[i] = strings.TrimSpace(methods[i])
		}
		cfg.Auth.SkipAuthMethods = methods
	}

	// CSRF configuration
	if v := os.Getenv("GATEWAY_CSRF_ENABLED"); v != "" {
		enabled := v == "true" || v == "1"
		cfg.CSRFEnabled = enabled
		cfg.CSRF.Enabled = enabled
	}

	if v := os.Getenv("GATEWAY_CSRF_SECRET_KEY"); v != "" {
		cfg.CSRF.SecretKey = v
	}

	if v := os.Getenv("GATEWAY_CSRF_SECURE"); v != "" {
		cfg.CSRF.Secure = v == "true" || v == "1"
	}

	if v := os.Getenv("GATEWAY_CSRF_DOMAIN"); v != "" {
		cfg.CSRF.Domain = v
	}

	if v := os.Getenv("GATEWAY_CSRF_EXEMPT_PATHS"); v != "" {
		// Comma-separated list of paths to exempt
		paths := strings.Split(v, ",")
		for i := range paths {
			paths[i] = strings.TrimSpace(paths[i])
		}
		cfg.CSRF.ExemptPaths = paths
	}

	if v := os.Getenv("GATEWAY_CSRF_TRUSTED_ORIGINS"); v != "" {
		// Comma-separated list of trusted origins
		origins := strings.Split(v, ",")
		for i := range origins {
			origins[i] = strings.TrimSpace(origins[i])
		}
		cfg.CSRF.TrustedOrigins = origins
	}

	if v := os.Getenv("GATEWAY_RATE_LIMIT_ENABLED"); v != "" {
		cfg.RateLimitEnabled = v == "true" || v == "1"
	}

	if v := os.Getenv("GATEWAY_RATE_LIMIT_RPS"); v != "" {
		if rps, err := strconv.Atoi(v); err == nil && rps > 0 {
			cfg.RateLimitRPS = rps
			// Also update new config structure for backwards compatibility
			cfg.RateLimit.DefaultRPS = float64(rps)
		}
	}

	// Enhanced rate limit configuration
	if v := os.Getenv("GATEWAY_RATE_LIMIT_DEFAULT_RPS"); v != "" {
		if rps, err := strconv.ParseFloat(v, 64); err == nil && rps > 0 {
			cfg.RateLimit.DefaultRPS = rps
		}
	}

	if v := os.Getenv("GATEWAY_RATE_LIMIT_DEFAULT_BURST"); v != "" {
		if burst, err := strconv.Atoi(v); err == nil && burst > 0 {
			cfg.RateLimit.DefaultBurst = burst
		}
	}

	if v := os.Getenv("GATEWAY_RATE_LIMIT_SKIP_METHODS"); v != "" {
		// Comma-separated list of gRPC methods to skip
		methods := strings.Split(v, ",")
		for i := range methods {
			methods[i] = strings.TrimSpace(methods[i])
		}
		cfg.RateLimit.SkipMethods = methods
	}

	if v := os.Getenv("GATEWAY_RATE_LIMIT_SKIP_PATHS"); v != "" {
		// Comma-separated list of HTTP paths to skip
		paths := strings.Split(v, ",")
		for i := range paths {
			paths[i] = strings.TrimSpace(paths[i])
		}
		cfg.RateLimit.SkipPaths = paths
	}

	if v := os.Getenv("GATEWAY_RATE_LIMIT_INCLUDE_HEADERS"); v != "" {
		cfg.RateLimit.IncludeHeaders = v == "true" || v == "1"
	}

	if v := os.Getenv("GATEWAY_ORCHESTRATOR_ADDRESS"); v != "" {
		cfg.OrchestratorAddress = v
	}

	if v := os.Getenv("GATEWAY_SEARCH_ADDRESS"); v != "" {
		cfg.SearchAddress = v
	}

	if v := os.Getenv("GATEWAY_DAILY_REVIEW_ADDRESS"); v != "" {
		cfg.DailyReviewAddress = v
	}

	// ML service URLs (on dev01)
	if v := os.Getenv("GATEWAY_WORKER_HEALTH_URL"); v != "" {
		cfg.WorkerHealthURL = v
	}

	if v := os.Getenv("GATEWAY_EMBEDDINGS_URL"); v != "" {
		cfg.EmbeddingsURL = v
	}

	if v := os.Getenv("GATEWAY_LLM_URL"); v != "" {
		cfg.LLMURL = v
	}
}

// GRPCAddress returns the address for the gRPC server.
func (c *GatewayConfig) GRPCAddress() string {
	return fmt.Sprintf(":%d", c.GRPCPort)
}

// HTTPAddress returns the address for the HTTP server.
func (c *GatewayConfig) HTTPAddress() string {
	return fmt.Sprintf(":%d", c.HTTPPort)
}

// Package config provides gateway service-specific configuration.
// It extends the shared pkg/config with gateway-specific settings.
package config

import (
	"fmt"
	"os"
	"strconv"

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

	// RateLimitEnabled enables rate limiting when true.
	RateLimitEnabled bool

	// RateLimitRPS is the maximum requests per second when rate limiting is enabled.
	RateLimitRPS int

	// OrchestratorAddress is the gRPC address of the orchestrator service.
	OrchestratorAddress string

	// SearchAddress is the gRPC address of the search service.
	SearchAddress string

	// DailyReviewAddress is the gRPC address of the daily review service.
	DailyReviewAddress string
}

// Default configuration values for the gateway.
const (
	DefaultGRPCPort             = 50051
	DefaultHTTPPort             = 8080
	DefaultAuthEnabled          = false
	DefaultRateLimitEnabled     = false
	DefaultRateLimitRPS         = 100
	DefaultOrchestratorAddress  = "localhost:50052"
	DefaultSearchAddress        = "localhost:50053"
	DefaultDailyReviewAddress   = "localhost:50054"
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
		RateLimitEnabled:    DefaultRateLimitEnabled,
		RateLimitRPS:        DefaultRateLimitRPS,
		OrchestratorAddress: DefaultOrchestratorAddress,
		SearchAddress:       DefaultSearchAddress,
		DailyReviewAddress:  DefaultDailyReviewAddress,
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

	if v := os.Getenv("GATEWAY_RATE_LIMIT_ENABLED"); v != "" {
		cfg.RateLimitEnabled = v == "true" || v == "1"
	}

	if v := os.Getenv("GATEWAY_RATE_LIMIT_RPS"); v != "" {
		if rps, err := strconv.Atoi(v); err == nil && rps > 0 {
			cfg.RateLimitRPS = rps
		}
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
}

// GRPCAddress returns the address for the gRPC server.
func (c *GatewayConfig) GRPCAddress() string {
	return fmt.Sprintf(":%d", c.GRPCPort)
}

// HTTPAddress returns the address for the HTTP server.
func (c *GatewayConfig) HTTPAddress() string {
	return fmt.Sprintf(":%d", c.HTTPPort)
}

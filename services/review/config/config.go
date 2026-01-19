// Package config provides service-specific configuration for the Review service.
package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/otherjamesbrown/penfold/pkg/config"
)

// Default configuration values for the Review service.
const (
	DefaultGRPCPort       = 50051
	DefaultHTTPPort       = 8080
	DefaultShutdownTimeout = 30 // seconds
)

// ServiceConfig holds Review service-specific configuration.
type ServiceConfig struct {
	// Base configuration from shared pkg/config.
	*config.Config

	// GRPCPort is the port the gRPC server listens on.
	GRPCPort int

	// HTTPPort is the port for health checks and metrics HTTP endpoints.
	HTTPPort int

	// ShutdownTimeoutSeconds is the graceful shutdown timeout.
	ShutdownTimeoutSeconds int
}

// LoadServiceConfig loads both base and service-specific configuration.
func LoadServiceConfig() (*ServiceConfig, error) {
	// Load base configuration.
	baseCfg, err := config.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("loading base config: %w", err)
	}

	cfg := &ServiceConfig{
		Config:                 baseCfg,
		GRPCPort:               DefaultGRPCPort,
		HTTPPort:               DefaultHTTPPort,
		ShutdownTimeoutSeconds: DefaultShutdownTimeout,
	}

	// Override with environment variables if present.
	if v := os.Getenv("REVIEW_GRPC_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil && port > 0 && port <= 65535 {
			cfg.GRPCPort = port
		}
	}

	if v := os.Getenv("REVIEW_HTTP_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil && port > 0 && port <= 65535 {
			cfg.HTTPPort = port
		}
	}

	if v := os.Getenv("REVIEW_SHUTDOWN_TIMEOUT"); v != "" {
		if timeout, err := strconv.Atoi(v); err == nil && timeout > 0 {
			cfg.ShutdownTimeoutSeconds = timeout
		}
	}

	return cfg, nil
}

// GRPCAddress returns the gRPC server listen address.
func (c *ServiceConfig) GRPCAddress() string {
	return fmt.Sprintf(":%d", c.GRPCPort)
}

// HTTPAddress returns the HTTP server listen address.
func (c *ServiceConfig) HTTPAddress() string {
	return fmt.Sprintf(":%d", c.HTTPPort)
}

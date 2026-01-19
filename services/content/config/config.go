// Package config provides service-specific configuration for the Content Processor.
package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/otherjamesbrown/penfold/pkg/config"
)

// ServiceConfig holds configuration specific to the Content Processor service.
type ServiceConfig struct {
	// Embedded common configuration.
	*config.Config

	// GRPCPort is the port for the gRPC server.
	GRPCPort int

	// HTTPPort is the port for the HTTP server (health, metrics).
	HTTPPort int

	// StoragePath is the path for local content storage.
	StoragePath string

	// MaxConcurrentProcessing is the maximum number of concurrent processing tasks.
	MaxConcurrentProcessing int

	// ProcessingTimeoutSeconds is the default timeout for processing operations.
	ProcessingTimeoutSeconds int
}

// Default configuration values for the Content Processor service.
const (
	DefaultGRPCPort                 = 50051
	DefaultHTTPPort                 = 8081
	DefaultStoragePath              = "/var/lib/penfold/content"
	DefaultMaxConcurrentProcessing  = 10
	DefaultProcessingTimeoutSeconds = 300
)

// LoadServiceConfig loads the Content Processor service configuration.
// It loads the common config first, then overlays service-specific settings.
func LoadServiceConfig() (*ServiceConfig, error) {
	// Load common configuration.
	baseCfg, err := config.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("loading base config: %w", err)
	}

	cfg := &ServiceConfig{
		Config:                   baseCfg,
		GRPCPort:                 DefaultGRPCPort,
		HTTPPort:                 DefaultHTTPPort,
		StoragePath:              DefaultStoragePath,
		MaxConcurrentProcessing:  DefaultMaxConcurrentProcessing,
		ProcessingTimeoutSeconds: DefaultProcessingTimeoutSeconds,
	}

	// Override with environment variables.
	loadServiceEnv(cfg)

	return cfg, nil
}

// loadServiceEnv loads service-specific configuration from environment variables.
func loadServiceEnv(cfg *ServiceConfig) {
	if v := os.Getenv("PENFOLD_CONTENT_GRPC_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.GRPCPort = port
		}
	}

	if v := os.Getenv("PENFOLD_CONTENT_HTTP_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.HTTPPort = port
		}
	}

	if v := os.Getenv("PENFOLD_CONTENT_STORAGE_PATH"); v != "" {
		cfg.StoragePath = v
	}

	if v := os.Getenv("PENFOLD_CONTENT_MAX_CONCURRENT"); v != "" {
		if max, err := strconv.Atoi(v); err == nil {
			cfg.MaxConcurrentProcessing = max
		}
	}

	if v := os.Getenv("PENFOLD_CONTENT_PROCESSING_TIMEOUT"); v != "" {
		if timeout, err := strconv.Atoi(v); err == nil {
			cfg.ProcessingTimeoutSeconds = timeout
		}
	}
}

// GRPCAddress returns the gRPC server address.
func (c *ServiceConfig) GRPCAddress() string {
	return fmt.Sprintf(":%d", c.GRPCPort)
}

// HTTPAddress returns the HTTP server address.
func (c *ServiceConfig) HTTPAddress() string {
	return fmt.Sprintf(":%d", c.HTTPPort)
}

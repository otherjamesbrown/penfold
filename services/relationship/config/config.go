// Package config provides configuration loading for the Relationship Discovery service.
package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/otherjamesbrown/penfold/pkg/config"
)

// ServiceConfig holds configuration specific to the Relationship Discovery service.
type ServiceConfig struct {
	// Base configuration from the shared pkg/config
	*config.Config

	// GRPCPort is the port on which the gRPC server listens.
	GRPCPort int

	// HTTPPort is the port on which the HTTP server listens (health/metrics).
	HTTPPort int

	// GraphDBHost is the hostname of the graph database (e.g., Neo4j, ArangoDB).
	GraphDBHost string

	// GraphDBPort is the port of the graph database.
	GraphDBPort int

	// GraphDBName is the database name in the graph database.
	GraphDBName string

	// GraphDBUser is the username for graph database authentication.
	GraphDBUser string

	// GraphDBPassword is the password for graph database authentication.
	GraphDBPassword string

	// AIServiceURL is the URL of the AI service for relationship extraction.
	AIServiceURL string

	// MaxConcurrentDiscoveries limits concurrent discovery operations.
	MaxConcurrentDiscoveries int
}

// Default configuration values for the Relationship service.
const (
	DefaultGRPCPort                 = 50051
	DefaultHTTPPort                 = 8080
	DefaultGraphDBHost              = "localhost"
	DefaultGraphDBPort              = 7687
	DefaultGraphDBName              = "penfold"
	DefaultGraphDBUser              = "neo4j"
	DefaultAIServiceURL             = "http://localhost:8000"
	DefaultMaxConcurrentDiscoveries = 10
)

// Load loads the service configuration from environment variables,
// overlaying service-specific settings on the base configuration.
func Load() (*ServiceConfig, error) {
	// Load base configuration
	baseCfg, err := config.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("loading base config: %w", err)
	}

	// Set service name if not already set
	if baseCfg.ServiceName == "" {
		baseCfg.ServiceName = "relationship-discovery"
	}

	cfg := &ServiceConfig{
		Config:                   baseCfg,
		GRPCPort:                 DefaultGRPCPort,
		HTTPPort:                 DefaultHTTPPort,
		GraphDBHost:              DefaultGraphDBHost,
		GraphDBPort:              DefaultGraphDBPort,
		GraphDBName:              DefaultGraphDBName,
		GraphDBUser:              DefaultGraphDBUser,
		AIServiceURL:             DefaultAIServiceURL,
		MaxConcurrentDiscoveries: DefaultMaxConcurrentDiscoveries,
	}

	// Load service-specific environment variables
	loadServiceEnv(cfg)

	// Validate service-specific configuration
	if err := cfg.ValidateService(); err != nil {
		return nil, fmt.Errorf("validating service config: %w", err)
	}

	return cfg, nil
}

// loadServiceEnv loads service-specific environment variables.
func loadServiceEnv(cfg *ServiceConfig) {
	if v := os.Getenv("RELATIONSHIP_GRPC_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.GRPCPort = port
		}
	}

	if v := os.Getenv("RELATIONSHIP_HTTP_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.HTTPPort = port
		}
	}

	if v := os.Getenv("RELATIONSHIP_GRAPHDB_HOST"); v != "" {
		cfg.GraphDBHost = v
	}

	if v := os.Getenv("RELATIONSHIP_GRAPHDB_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.GraphDBPort = port
		}
	}

	if v := os.Getenv("RELATIONSHIP_GRAPHDB_NAME"); v != "" {
		cfg.GraphDBName = v
	}

	if v := os.Getenv("RELATIONSHIP_GRAPHDB_USER"); v != "" {
		cfg.GraphDBUser = v
	}

	if v := os.Getenv("RELATIONSHIP_GRAPHDB_PASSWORD"); v != "" {
		cfg.GraphDBPassword = v
	}

	if v := os.Getenv("RELATIONSHIP_AI_SERVICE_URL"); v != "" {
		cfg.AIServiceURL = v
	}

	if v := os.Getenv("RELATIONSHIP_MAX_CONCURRENT_DISCOVERIES"); v != "" {
		if max, err := strconv.Atoi(v); err == nil {
			cfg.MaxConcurrentDiscoveries = max
		}
	}
}

// ValidateService validates service-specific configuration.
func (c *ServiceConfig) ValidateService() error {
	if c.GRPCPort <= 0 || c.GRPCPort > 65535 {
		return fmt.Errorf("invalid grpc_port: %d", c.GRPCPort)
	}

	if c.HTTPPort <= 0 || c.HTTPPort > 65535 {
		return fmt.Errorf("invalid http_port: %d", c.HTTPPort)
	}

	if c.GraphDBHost == "" {
		return fmt.Errorf("graphdb_host is required")
	}

	if c.GraphDBPort <= 0 || c.GraphDBPort > 65535 {
		return fmt.Errorf("invalid graphdb_port: %d", c.GraphDBPort)
	}

	if c.MaxConcurrentDiscoveries <= 0 {
		return fmt.Errorf("max_concurrent_discoveries must be positive")
	}

	return nil
}

// GRPCAddress returns the address for the gRPC server.
func (c *ServiceConfig) GRPCAddress() string {
	return fmt.Sprintf(":%d", c.GRPCPort)
}

// HTTPAddress returns the address for the HTTP server.
func (c *ServiceConfig) HTTPAddress() string {
	return fmt.Sprintf(":%d", c.HTTPPort)
}

// GraphDBAddress returns the graph database connection address.
func (c *ServiceConfig) GraphDBAddress() string {
	return fmt.Sprintf("%s:%d", c.GraphDBHost, c.GraphDBPort)
}

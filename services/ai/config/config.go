// Package config provides service-specific configuration for the AI Coordinator service.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds the AI Coordinator service configuration.
type Config struct {
	// ServiceName identifies this service instance.
	ServiceName string

	// GRPCPort is the port for the gRPC server.
	GRPCPort int

	// HTTPPort is the port for HTTP endpoints (health, metrics).
	HTTPPort int

	// OllamaHost is the address of the Ollama server for local models.
	OllamaHost string

	// OllamaDefaultModel is the default model to use with Ollama.
	OllamaDefaultModel string

	// GeminiAPIKey is the API key for Google Gemini.
	GeminiAPIKey string

	// GeminiDefaultModel is the default Gemini model to use.
	GeminiDefaultModel string

	// DefaultEmbeddingModel is the default model for embeddings.
	DefaultEmbeddingModel string

	// DefaultLLMModel is the default model for text generation.
	DefaultLLMModel string

	// LogLevel controls logging verbosity (debug, info, warn, error).
	LogLevel string

	// Environment is the deployment environment (dev, staging, prod).
	Environment string

	// EnableMetrics enables Prometheus metrics collection.
	EnableMetrics bool

	// GracefulShutdownTimeout is the timeout in seconds for graceful shutdown.
	GracefulShutdownTimeout int
}

// Default configuration values.
const (
	DefaultServiceName              = "ai-coordinator"
	DefaultGRPCPort                 = 50051
	DefaultHTTPPort                 = 8080
	DefaultOllamaHost               = "http://localhost:11434"
	DefaultOllamaDefaultModel       = "llama3.2"
	DefaultGeminiDefaultModel       = "gemini-1.5-pro"
	DefaultEmbeddingModel           = "nomic-embed-text"
	DefaultLLMModel                 = "llama3.2"
	DefaultLogLevel                 = "info"
	DefaultEnvironment              = "dev"
	DefaultGracefulShutdownTimeout  = 30
)

// Load loads the configuration from environment variables.
func Load() (*Config, error) {
	cfg := &Config{
		ServiceName:             DefaultServiceName,
		GRPCPort:                DefaultGRPCPort,
		HTTPPort:                DefaultHTTPPort,
		OllamaHost:              DefaultOllamaHost,
		OllamaDefaultModel:      DefaultOllamaDefaultModel,
		GeminiDefaultModel:      DefaultGeminiDefaultModel,
		DefaultEmbeddingModel:   DefaultEmbeddingModel,
		DefaultLLMModel:         DefaultLLMModel,
		LogLevel:                DefaultLogLevel,
		Environment:             DefaultEnvironment,
		EnableMetrics:           true,
		GracefulShutdownTimeout: DefaultGracefulShutdownTimeout,
	}

	// Load from environment variables
	if v := os.Getenv("AI_SERVICE_NAME"); v != "" {
		cfg.ServiceName = v
	}

	if v := os.Getenv("AI_GRPC_PORT"); v != "" {
		port, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid AI_GRPC_PORT: %w", err)
		}
		cfg.GRPCPort = port
	}

	if v := os.Getenv("AI_HTTP_PORT"); v != "" {
		port, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid AI_HTTP_PORT: %w", err)
		}
		cfg.HTTPPort = port
	}

	if v := os.Getenv("AI_OLLAMA_HOST"); v != "" {
		cfg.OllamaHost = v
	}

	if v := os.Getenv("AI_OLLAMA_DEFAULT_MODEL"); v != "" {
		cfg.OllamaDefaultModel = v
	}

	if v := os.Getenv("AI_GEMINI_API_KEY"); v != "" {
		cfg.GeminiAPIKey = v
	}

	if v := os.Getenv("AI_GEMINI_DEFAULT_MODEL"); v != "" {
		cfg.GeminiDefaultModel = v
	}

	if v := os.Getenv("AI_DEFAULT_EMBEDDING_MODEL"); v != "" {
		cfg.DefaultEmbeddingModel = v
	}

	if v := os.Getenv("AI_DEFAULT_LLM_MODEL"); v != "" {
		cfg.DefaultLLMModel = v
	}

	if v := os.Getenv("AI_LOG_LEVEL"); v != "" {
		cfg.LogLevel = strings.ToLower(v)
	}

	if v := os.Getenv("AI_ENVIRONMENT"); v != "" {
		cfg.Environment = strings.ToLower(v)
	}

	if v := os.Getenv("AI_ENABLE_METRICS"); v != "" {
		cfg.EnableMetrics = strings.ToLower(v) == "true" || v == "1"
	}

	if v := os.Getenv("AI_GRACEFUL_SHUTDOWN_TIMEOUT"); v != "" {
		timeout, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid AI_GRACEFUL_SHUTDOWN_TIMEOUT: %w", err)
		}
		cfg.GracefulShutdownTimeout = timeout
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// Validate checks that the configuration is valid.
func (c *Config) Validate() error {
	var errs []string

	if c.ServiceName == "" {
		errs = append(errs, "service_name is required")
	}

	if c.GRPCPort <= 0 || c.GRPCPort > 65535 {
		errs = append(errs, fmt.Sprintf("invalid grpc_port: %d", c.GRPCPort))
	}

	if c.HTTPPort <= 0 || c.HTTPPort > 65535 {
		errs = append(errs, fmt.Sprintf("invalid http_port: %d", c.HTTPPort))
	}

	if c.GRPCPort == c.HTTPPort {
		errs = append(errs, "grpc_port and http_port must be different")
	}

	if c.OllamaHost == "" {
		errs = append(errs, "ollama_host is required")
	}

	validLogLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLogLevels[c.LogLevel] {
		errs = append(errs, fmt.Sprintf("invalid log_level: %q (must be debug, info, warn, or error)", c.LogLevel))
	}

	validEnvironments := map[string]bool{"dev": true, "staging": true, "prod": true}
	if !validEnvironments[c.Environment] {
		errs = append(errs, fmt.Sprintf("invalid environment: %q (must be dev, staging, or prod)", c.Environment))
	}

	if c.GracefulShutdownTimeout <= 0 {
		errs = append(errs, fmt.Sprintf("invalid graceful_shutdown_timeout: %d", c.GracefulShutdownTimeout))
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}

	return nil
}

// IsDevelopment returns true if running in development environment.
func (c *Config) IsDevelopment() bool {
	return c.Environment == "dev"
}

// IsProduction returns true if running in production environment.
func (c *Config) IsProduction() bool {
	return c.Environment == "prod"
}

// GRPCAddr returns the gRPC server address.
func (c *Config) GRPCAddr() string {
	return fmt.Sprintf(":%d", c.GRPCPort)
}

// HTTPAddr returns the HTTP server address.
func (c *Config) HTTPAddr() string {
	return fmt.Sprintf(":%d", c.HTTPPort)
}

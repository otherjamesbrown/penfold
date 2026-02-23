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

	// GeminiAPIKey is the API key for Google Gemini.
	GeminiAPIKey string

	// GeminiDefaultModel is the default Gemini model to use.
	GeminiDefaultModel string

	// DefaultEmbeddingModel is the default model for embeddings.
	DefaultEmbeddingModel string

	// DefaultLLMModel is the default model for text generation.
	DefaultLLMModel string

	// MLXEmbeddingsURL is the URL of the MLX embeddings server.
	MLXEmbeddingsURL string

	// MLXLLMURL is the URL of the MLX LLM server.
	MLXLLMURL string

	// OllamaURL is the URL for Ollama/local models (OpenAI-compatible).
	// Backward compatible: falls back to MLXLLMURL if not set.
	OllamaURL string

	// StageModels maps pipeline stage names to their configured models.
	// Keys: triage, extract_ner, extract_assertions, analyze, embed
	StageModels map[string]string

	// EmbeddingDimensions is the expected dimension of embedding vectors.
	EmbeddingDimensions int

	// LogLevel controls logging verbosity (debug, info, warn, error).
	LogLevel string

	// Environment is the deployment environment (dev, staging, prod).
	Environment string

	// EnableMetrics enables Prometheus metrics collection.
	EnableMetrics bool

	// GracefulShutdownTimeout is the timeout in seconds for graceful shutdown.
	GracefulShutdownTimeout int

	// DBURL is the PostgreSQL connection string for dynamic model config (optional).
	// If not set, model config resolution falls back to env vars only.
	DBURL string
}

// Default configuration values.
const (
	DefaultServiceName              = "ai-coordinator"
	DefaultGRPCPort                 = 50051
	DefaultHTTPPort                 = 8090
	DefaultGeminiDefaultModel       = "gemini-2.5-pro"
	DefaultEmbeddingModel           = "mxbai-embed-large"
	DefaultLLMModel                 = "gemini-2.0-flash"
	DefaultMLXEmbeddingsURL         = "http://localhost:11434"
	DefaultMLXLLMURL                = "http://localhost:8080"
	DefaultEmbeddingDimensions      = 1024
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
		GeminiDefaultModel:      DefaultGeminiDefaultModel,
		DefaultEmbeddingModel:   DefaultEmbeddingModel,
		DefaultLLMModel:         DefaultLLMModel,
		MLXEmbeddingsURL:        DefaultMLXEmbeddingsURL,
		MLXLLMURL:               DefaultMLXLLMURL,
		OllamaURL:               "", // Will be set below with fallback
		StageModels:             make(map[string]string),
		EmbeddingDimensions:     DefaultEmbeddingDimensions,
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

	if v := os.Getenv("AI_MLX_EMBEDDINGS_URL"); v != "" {
		cfg.MLXEmbeddingsURL = v
	}

	if v := os.Getenv("AI_MLX_LLM_URL"); v != "" {
		cfg.MLXLLMURL = v
	}

	// OllamaURL with backward compatibility: try AI_OLLAMA_URL first, then fall back to AI_MLX_LLM_URL
	if v := os.Getenv("AI_OLLAMA_URL"); v != "" {
		cfg.OllamaURL = v
	} else if cfg.MLXLLMURL != "" {
		cfg.OllamaURL = cfg.MLXLLMURL
	} else {
		cfg.OllamaURL = "http://localhost:11434"
	}

	// Load per-stage model configuration.
	// Canonical env var names match stage names (e.g., AI_MODEL_EXTRACT_NER for extract_ner).
	// Legacy names (AI_MODEL_EXTRACT_ENTITIES, AI_MODEL_DEEP_ANALYZE) are kept as fallback
	// for backwards compatibility with deployed env files.
	if v := os.Getenv("AI_MODEL_TRIAGE"); v != "" {
		cfg.StageModels["triage"] = v
	}
	if v := os.Getenv("AI_MODEL_EXTRACT_NER"); v != "" {
		cfg.StageModels["extract_ner"] = v
	} else if v := os.Getenv("AI_MODEL_EXTRACT_ENTITIES"); v != "" {
		cfg.StageModels["extract_ner"] = v
	}
	if v := os.Getenv("AI_MODEL_EXTRACT_ASSERTIONS"); v != "" {
		cfg.StageModels["extract_assertions"] = v
	}
	if v := os.Getenv("AI_MODEL_ANALYZE"); v != "" {
		cfg.StageModels["analyze"] = v
	} else if v := os.Getenv("AI_MODEL_DEEP_ANALYZE"); v != "" {
		cfg.StageModels["analyze"] = v
	}
	if v := os.Getenv("AI_MODEL_EMBEDDING"); v != "" {
		cfg.StageModels["embed"] = v
	}

	if v := os.Getenv("AI_EMBEDDING_DIMENSIONS"); v != "" {
		dims, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid AI_EMBEDDING_DIMENSIONS: %w", err)
		}
		cfg.EmbeddingDimensions = dims
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

	// Load optional DB URL for dynamic model config
	if v := os.Getenv("AI_DB_URL"); v != "" {
		cfg.DBURL = v
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

// ModelForStage returns the configured model for a given pipeline stage.
// Resolution order for embedding stage:
// 1. Stage-specific config (AI_MODEL_EMBEDDING)
// 2. Global default embedding model (AI_DEFAULT_EMBEDDING_MODEL)
// 3. Hardcoded default (mxbai-embed-large)
//
// Resolution order for other stages:
// 1. Stage-specific config (e.g., AI_MODEL_TRIAGE)
// 2. Global default LLM model (AI_DEFAULT_LLM_MODEL)
// 3. Hardcoded default (gemini-2.0-flash)
//
// Stage names are case-insensitive.
func (c *Config) ModelForStage(stage string) string {
	// Normalize stage name to lowercase
	stage = strings.ToLower(strings.TrimSpace(stage))

	// Check stage-specific config
	if model, ok := c.StageModels[stage]; ok && model != "" {
		return model
	}

	// For embed stage, fall back to embedding model defaults
	if stage == "embed" {
		if c.DefaultEmbeddingModel != "" {
			return c.DefaultEmbeddingModel
		}
		return DefaultEmbeddingModel
	}

	// For all other stages, fall back to LLM model defaults
	if c.DefaultLLMModel != "" {
		return c.DefaultLLMModel
	}

	// Final fallback to hardcoded default
	return DefaultLLMModel
}

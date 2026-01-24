// Package embeddings provides MLX embedding generation for semantic search.
// It integrates with the mxbai-embed-large-v1 model running on Apple Silicon via MLX.
package embeddings

import (
	"os"
	"strconv"
	"time"
)

// Default configuration values for the MLX embedding service.
const (
	// DefaultServerURL is the default MLX server URL (Ollama-compatible API).
	DefaultServerURL = "http://localhost:11434"

	// DefaultModel is the default embedding model to use.
	// mxbai-embed-large-v1 provides 1024-dimension embeddings optimized for semantic search.
	DefaultModel = "mxbai-embed-large-v1"

	// DefaultBatchSize is the default number of texts to embed in a single request.
	DefaultBatchSize = 32

	// DefaultTimeout is the default timeout for embedding requests.
	DefaultTimeout = 30 * time.Second

	// DefaultDimensions is the embedding dimension for mxbai-embed-large-v1.
	DefaultDimensions = 1024

	// DefaultMaxRetries is the default number of retries for failed requests.
	DefaultMaxRetries = 3

	// DefaultRetryDelay is the default delay between retries.
	DefaultRetryDelay = 100 * time.Millisecond
)

// Environment variable names for embedding configuration.
const (
	EnvServerURL  = "PENFOLD_EMBEDDING_SERVER_URL"
	EnvModel      = "PENFOLD_EMBEDDING_MODEL"
	EnvBatchSize  = "PENFOLD_EMBEDDING_BATCH_SIZE"
	EnvTimeout    = "PENFOLD_EMBEDDING_TIMEOUT_SECONDS"
	EnvDimensions = "PENFOLD_EMBEDDING_DIMENSIONS"
	EnvMaxRetries = "PENFOLD_EMBEDDING_MAX_RETRIES"
)

// Config holds configuration for the embedding client.
type Config struct {
	// ServerURL is the URL of the embedding server (optional for gRPC-backed clients).
	// For direct HTTP clients, this should be the MLX/Ollama-compatible server URL.
	// Deprecated: Use AIClient-backed client instead.
	ServerURL string

	// Model is the name of the embedding model to use.
	// Default is mxbai-embed-large-v1 which provides 1024-dimension vectors.
	Model string

	// BatchSize is the maximum number of texts to embed in a single batch request.
	// Larger batch sizes are more efficient but use more memory.
	BatchSize int

	// Timeout is the maximum time to wait for an embedding request to complete.
	Timeout time.Duration

	// Dimensions is the expected dimension of the embedding vectors.
	// This is used for validation and must match the model's output.
	Dimensions int

	// MaxRetries is the number of times to retry failed requests.
	MaxRetries int

	// RetryDelay is the initial delay between retries (uses exponential backoff).
	RetryDelay time.Duration
}

// DefaultConfig returns a Config with sensible defaults for local MLX deployment.
func DefaultConfig() *Config {
	return &Config{
		ServerURL:  DefaultServerURL,
		Model:      DefaultModel,
		BatchSize:  DefaultBatchSize,
		Timeout:    DefaultTimeout,
		Dimensions: DefaultDimensions,
		MaxRetries: DefaultMaxRetries,
		RetryDelay: DefaultRetryDelay,
	}
}

// LoadFromEnv loads configuration from environment variables.
// It starts with defaults and overrides with any environment variables that are set.
func LoadFromEnv() *Config {
	cfg := DefaultConfig()

	if v := os.Getenv(EnvServerURL); v != "" {
		cfg.ServerURL = v
	}

	if v := os.Getenv(EnvModel); v != "" {
		cfg.Model = v
	}

	if v := os.Getenv(EnvBatchSize); v != "" {
		if batchSize, err := strconv.Atoi(v); err == nil && batchSize > 0 {
			cfg.BatchSize = batchSize
		}
	}

	if v := os.Getenv(EnvTimeout); v != "" {
		if seconds, err := strconv.Atoi(v); err == nil && seconds > 0 {
			cfg.Timeout = time.Duration(seconds) * time.Second
		}
	}

	if v := os.Getenv(EnvDimensions); v != "" {
		if dims, err := strconv.Atoi(v); err == nil && dims > 0 {
			cfg.Dimensions = dims
		}
	}

	if v := os.Getenv(EnvMaxRetries); v != "" {
		if retries, err := strconv.Atoi(v); err == nil && retries >= 0 {
			cfg.MaxRetries = retries
		}
	}

	return cfg
}

// Validate checks if the configuration is valid.
// Note: ServerURL is no longer required for AIClient-backed clients.
func (c *Config) Validate() error {
	if c.Model == "" {
		return ErrInvalidModel
	}
	if c.BatchSize <= 0 {
		return ErrInvalidBatchSize
	}
	if c.Dimensions <= 0 {
		return ErrInvalidDimensions
	}
	if c.Timeout <= 0 {
		return ErrInvalidTimeout
	}
	return nil
}

// ModelInfo contains information about the embedding model.
type ModelInfo struct {
	// Name is the model identifier.
	Name string

	// Dimensions is the number of dimensions in the embedding vectors.
	Dimensions int

	// MaxTokens is the maximum input length in tokens.
	MaxTokens int

	// Provider is the provider name (e.g., "mlx", "ollama").
	Provider string

	// IsLocal indicates whether this is a locally-running model.
	IsLocal bool

	// Description provides a brief description of the model.
	Description string
}

// GetModelInfo returns information about the configured embedding model.
// This is useful for display and validation purposes.
func (c *Config) GetModelInfo() *ModelInfo {
	return &ModelInfo{
		Name:        c.Model,
		Dimensions:  c.Dimensions,
		MaxTokens:   512, // mxbai-embed-large-v1 typical max
		Provider:    "mlx",
		IsLocal:     true,
		Description: "Mixed Bread AI embedding model optimized for semantic search (1024 dims)",
	}
}

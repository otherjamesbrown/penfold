// Package config provides service-specific configuration for the Search service.
// It extends the shared configuration with search-specific settings.
package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/otherjamesbrown/penfold/pkg/config"
)

// Config holds the complete configuration for the Search service.
type Config struct {
	// Base contains shared configuration from pkg/config.
	Base *config.Config

	// GRPCPort is the port for the gRPC server.
	GRPCPort int

	// HTTPPort is the port for HTTP endpoints (health, metrics).
	HTTPPort int

	// VectorDB holds vector database connection settings.
	VectorDB VectorDBConfig

	// Embedding holds embedding service settings.
	Embedding EmbeddingConfig

	// Search holds search-specific tuning parameters.
	Search SearchConfig
}

// VectorDBConfig holds vector database connection settings.
type VectorDBConfig struct {
	// Host is the vector database hostname.
	Host string

	// Port is the vector database port.
	Port int

	// CollectionName is the name of the vector collection/index.
	CollectionName string
}

// Address returns the vector database address in host:port format.
func (v *VectorDBConfig) Address() string {
	return fmt.Sprintf("%s:%d", v.Host, v.Port)
}

// EmbeddingConfig holds embedding service settings.
type EmbeddingConfig struct {
	// Model is the embedding model identifier.
	Model string

	// Dimensions is the embedding vector dimension.
	Dimensions int
}

// SearchConfig holds search-specific tuning parameters.
type SearchConfig struct {
	// DefaultLimit is the default number of results to return.
	DefaultLimit int

	// MaxLimit is the maximum number of results allowed.
	MaxLimit int

	// DefaultTextWeight is the default weight for full-text search (0.0 to 1.0).
	DefaultTextWeight float64

	// DefaultVectorWeight is the default weight for vector search (0.0 to 1.0).
	DefaultVectorWeight float64

	// DefaultMinScore is the default minimum score threshold.
	DefaultMinScore float64
}

// Default configuration values for the Search service.
const (
	DefaultGRPCPort          = 50051
	DefaultHTTPPort          = 8080
	DefaultVectorDBHost      = "localhost"
	DefaultVectorDBPort      = 6333
	DefaultCollectionName    = "penfold_documents"
	DefaultEmbeddingModel    = "all-MiniLM-L6-v2"
	DefaultEmbeddingDims     = 384
	DefaultSearchLimit       = 10
	DefaultSearchMaxLimit    = 100
	DefaultTextWeight        = 0.5
	DefaultVectorWeight      = 0.5
	DefaultMinScore          = 0.0
)

// Load loads the Search service configuration.
// It first loads the base configuration, then overlays search-specific settings.
func Load() (*Config, error) {
	// Load base configuration
	baseCfg, err := config.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("loading base config: %w", err)
	}

	cfg := &Config{
		Base:     baseCfg,
		GRPCPort: DefaultGRPCPort,
		HTTPPort: DefaultHTTPPort,
		VectorDB: VectorDBConfig{
			Host:           DefaultVectorDBHost,
			Port:           DefaultVectorDBPort,
			CollectionName: DefaultCollectionName,
		},
		Embedding: EmbeddingConfig{
			Model:      DefaultEmbeddingModel,
			Dimensions: DefaultEmbeddingDims,
		},
		Search: SearchConfig{
			DefaultLimit:        DefaultSearchLimit,
			MaxLimit:            DefaultSearchMaxLimit,
			DefaultTextWeight:   DefaultTextWeight,
			DefaultVectorWeight: DefaultVectorWeight,
			DefaultMinScore:     DefaultMinScore,
		},
	}

	// Load search-specific environment variables
	loadSearchEnv(cfg)

	return cfg, nil
}

// loadSearchEnv overlays environment variables onto the search configuration.
func loadSearchEnv(cfg *Config) {
	// gRPC and HTTP ports
	if v := os.Getenv("PENFOLD_SEARCH_GRPC_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.GRPCPort = port
		}
	}
	if v := os.Getenv("PENFOLD_SEARCH_HTTP_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.HTTPPort = port
		}
	}

	// Vector database configuration
	if v := os.Getenv("PENFOLD_VECTORDB_HOST"); v != "" {
		cfg.VectorDB.Host = v
	}
	if v := os.Getenv("PENFOLD_VECTORDB_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.VectorDB.Port = port
		}
	}
	if v := os.Getenv("PENFOLD_VECTORDB_COLLECTION"); v != "" {
		cfg.VectorDB.CollectionName = v
	}

	// Embedding configuration
	if v := os.Getenv("PENFOLD_EMBEDDING_MODEL"); v != "" {
		cfg.Embedding.Model = v
	}
	if v := os.Getenv("PENFOLD_EMBEDDING_DIMENSIONS"); v != "" {
		if dims, err := strconv.Atoi(v); err == nil {
			cfg.Embedding.Dimensions = dims
		}
	}

	// Search tuning parameters
	if v := os.Getenv("PENFOLD_SEARCH_DEFAULT_LIMIT"); v != "" {
		if limit, err := strconv.Atoi(v); err == nil {
			cfg.Search.DefaultLimit = limit
		}
	}
	if v := os.Getenv("PENFOLD_SEARCH_MAX_LIMIT"); v != "" {
		if limit, err := strconv.Atoi(v); err == nil {
			cfg.Search.MaxLimit = limit
		}
	}
	if v := os.Getenv("PENFOLD_SEARCH_TEXT_WEIGHT"); v != "" {
		if weight, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.Search.DefaultTextWeight = weight
		}
	}
	if v := os.Getenv("PENFOLD_SEARCH_VECTOR_WEIGHT"); v != "" {
		if weight, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.Search.DefaultVectorWeight = weight
		}
	}
	if v := os.Getenv("PENFOLD_SEARCH_MIN_SCORE"); v != "" {
		if score, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.Search.DefaultMinScore = score
		}
	}
}

// GRPCAddress returns the address for the gRPC server.
func (c *Config) GRPCAddress() string {
	return fmt.Sprintf(":%d", c.GRPCPort)
}

// HTTPAddress returns the address for the HTTP server.
func (c *Config) HTTPAddress() string {
	return fmt.Sprintf(":%d", c.HTTPPort)
}

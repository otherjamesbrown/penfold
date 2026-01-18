// Package config provides environment-based configuration for the penfold-go-pipeline.
package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v10"
)

// Config holds all configuration for the pipeline service.
type Config struct {
	Database   DatabaseConfig
	Redis      RedisConfig
	AI         AIConfig
	Processing ProcessingConfig
	Server     ServerConfig
	Temporal   TemporalConfig
}

// TemporalConfig holds Temporal server connection settings.
type TemporalConfig struct {
	HostPort               string `env:"TEMPORAL_HOST_PORT" envDefault:"localhost:7233"`
	Namespace              string `env:"TEMPORAL_NAMESPACE" envDefault:"default"`
	TaskQueue              string `env:"TEMPORAL_TASK_QUEUE" envDefault:"penfold-ai-processing"`
	MaxConcurrentActivities int    `env:"TEMPORAL_MAX_CONCURRENT_ACTIVITIES" envDefault:"4"`
	WorkerEnabled          bool   `env:"TEMPORAL_WORKER_ENABLED" envDefault:"true"`
}

// DatabaseConfig holds PostgreSQL connection settings.
type DatabaseConfig struct {
	Host     string `env:"DB_HOST" envDefault:"10.0.10.253"`
	Port     int    `env:"DB_PORT" envDefault:"5432"`
	Name     string `env:"DB_NAME" envDefault:"penfold_dev"`
	User     string `env:"DB_USER" envDefault:"penfold"`
	Password string `env:"DB_PASSWORD" envDefault:""`

	// Connection pool settings
	MaxOpenConns    int           `env:"DB_MAX_OPEN_CONNS" envDefault:"25"`
	MaxIdleConns    int           `env:"DB_MAX_IDLE_CONNS" envDefault:"5"`
	ConnMaxLifetime time.Duration `env:"DB_CONN_MAX_LIFETIME" envDefault:"5m"`
	ConnMaxIdleTime time.Duration `env:"DB_CONN_MAX_IDLE_TIME" envDefault:"1m"`
}

// DSN returns the PostgreSQL connection string.
func (c DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=disable",
		c.User, c.Password, c.Host, c.Port, c.Name,
	)
}

// RedisConfig holds Redis connection settings.
type RedisConfig struct {
	Host     string `env:"REDIS_HOST" envDefault:"10.0.10.253"`
	Port     int    `env:"REDIS_PORT" envDefault:"6379"`
	Password string `env:"REDIS_PASSWORD" envDefault:""`
	DB       int    `env:"REDIS_DB" envDefault:"0"`

	// Connection settings
	DialTimeout  time.Duration `env:"REDIS_DIAL_TIMEOUT" envDefault:"5s"`
	ReadTimeout  time.Duration `env:"REDIS_READ_TIMEOUT" envDefault:"3s"`
	WriteTimeout time.Duration `env:"REDIS_WRITE_TIMEOUT" envDefault:"3s"`
	PoolSize     int           `env:"REDIS_POOL_SIZE" envDefault:"10"`
}

// Addr returns the Redis address in host:port format.
func (c RedisConfig) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// AIConfig holds AI service connection settings.
type AIConfig struct {
	// Embeddings service (local mxbai-embed-large-v1)
	EmbeddingsURL   string        `env:"EMBEDDINGS_URL" envDefault:"http://localhost:8001"`
	EmbeddingsModel string        `env:"EMBEDDINGS_MODEL" envDefault:"mxbai-embed-large-v1"`
	EmbeddingsTimeout time.Duration `env:"EMBEDDINGS_TIMEOUT" envDefault:"30s"`

	// LLM service (local Qwen2.5 or cloud fallback)
	LLMURL     string        `env:"LLM_URL" envDefault:"http://localhost:8000"`
	LLMModel   string        `env:"LLM_MODEL" envDefault:"Qwen2.5-32B-4bit"`
	LLMTimeout time.Duration `env:"LLM_TIMEOUT" envDefault:"120s"`

	// Cloud fallback (Gemini)
	CloudAPIKey  string `env:"CLOUD_API_KEY" envDefault:""`
	CloudEnabled bool   `env:"CLOUD_ENABLED" envDefault:"false"`
}

// ProcessingConfig holds pipeline processing settings.
type ProcessingConfig struct {
	WorkerCount int `env:"WORKER_COUNT" envDefault:"4"`
	MaxRetries  int `env:"MAX_RETRIES" envDefault:"3"`
	QueueSize   int `env:"QUEUE_SIZE" envDefault:"1000"`

	// Batch processing
	BatchSize     int           `env:"BATCH_SIZE" envDefault:"100"`
	BatchTimeout  time.Duration `env:"BATCH_TIMEOUT" envDefault:"5s"`
	FlushInterval time.Duration `env:"FLUSH_INTERVAL" envDefault:"10s"`

	// Circuit breaker settings
	CircuitBreakerThreshold   int           `env:"CB_THRESHOLD" envDefault:"5"`
	CircuitBreakerTimeout     time.Duration `env:"CB_TIMEOUT" envDefault:"30s"`
	CircuitBreakerMaxRequests uint32        `env:"CB_MAX_REQUESTS" envDefault:"3"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	HealthPort   int           `env:"HEALTH_PORT" envDefault:"8080"`
	ReadTimeout  time.Duration `env:"SERVER_READ_TIMEOUT" envDefault:"5s"`
	WriteTimeout time.Duration `env:"SERVER_WRITE_TIMEOUT" envDefault:"10s"`
	IdleTimeout  time.Duration `env:"SERVER_IDLE_TIMEOUT" envDefault:"120s"`
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config from environment: %w", err)
	}
	return cfg, nil
}

// MustLoad reads configuration from environment variables and panics on error.
func MustLoad() *Config {
	cfg, err := Load()
	if err != nil {
		panic(err)
	}
	return cfg
}

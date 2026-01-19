// Package config provides shared configuration loading for Penfold Go microservices.
// It supports loading configuration from environment variables with optional YAML file fallback.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Environment represents the deployment environment.
type Environment string

const (
	EnvDev     Environment = "dev"
	EnvStaging Environment = "staging"
	EnvProd    Environment = "prod"
)

// LogLevel represents logging verbosity.
type LogLevel string

const (
	LogDebug LogLevel = "debug"
	LogInfo  LogLevel = "info"
	LogWarn  LogLevel = "warn"
	LogError LogLevel = "error"
)

// Config holds the common configuration for Penfold microservices.
type Config struct {
	// ServiceName identifies this service instance.
	ServiceName string `yaml:"service_name"`

	// Environment is the deployment environment (dev, staging, prod).
	Environment Environment `yaml:"environment"`

	// LogLevel controls logging verbosity (debug, info, warn, error).
	LogLevel LogLevel `yaml:"log_level"`

	// Database configuration.
	Database DatabaseConfig `yaml:"database"`

	// Redis configuration.
	Redis RedisConfig `yaml:"redis"`

	// Temporal configuration.
	Temporal TemporalConfig `yaml:"temporal"`
}

// DatabaseConfig holds PostgreSQL connection settings.
type DatabaseConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Name     string `yaml:"name"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
}

// ConnectionString returns a PostgreSQL connection string.
func (d *DatabaseConfig) ConnectionString() string {
	return fmt.Sprintf("host=%s port=%d dbname=%s user=%s password=%s sslmode=disable",
		d.Host, d.Port, d.Name, d.User, d.Password)
}

// DSN returns a PostgreSQL DSN suitable for database/sql.
func (d *DatabaseConfig) DSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		d.User, d.Password, d.Host, d.Port, d.Name)
}

// RedisConfig holds Redis connection settings.
type RedisConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

// Address returns the Redis address in host:port format.
func (r *RedisConfig) Address() string {
	return fmt.Sprintf("%s:%d", r.Host, r.Port)
}

// TemporalConfig holds Temporal workflow engine settings.
type TemporalConfig struct {
	Host      string `yaml:"host"`
	Namespace string `yaml:"namespace"`
}

// Address returns the Temporal server address.
func (t *TemporalConfig) Address() string {
	return t.Host
}

// Default configuration values.
const (
	DefaultEnvironment      = EnvDev
	DefaultLogLevel         = LogInfo
	DefaultDBHost           = "localhost"
	DefaultDBPort           = 5432
	DefaultDBName           = "penfold"
	DefaultDBUser           = "penfold"
	DefaultRedisHost        = "localhost"
	DefaultRedisPort        = 6379
	DefaultTemporalHost     = "localhost:7233"
	DefaultTemporalNamespace = "default"
)

// LoadConfig loads configuration from environment variables with optional YAML file fallback.
// It first checks for a config file path in PENFOLD_CONFIG_FILE, loads that if present,
// then overlays any environment variables that are set.
func LoadConfig() (*Config, error) {
	cfg := &Config{
		Environment: DefaultEnvironment,
		LogLevel:    DefaultLogLevel,
		Database: DatabaseConfig{
			Host: DefaultDBHost,
			Port: DefaultDBPort,
			Name: DefaultDBName,
			User: DefaultDBUser,
		},
		Redis: RedisConfig{
			Host: DefaultRedisHost,
			Port: DefaultRedisPort,
		},
		Temporal: TemporalConfig{
			Host:      DefaultTemporalHost,
			Namespace: DefaultTemporalNamespace,
		},
	}

	// Load from YAML file if specified.
	if configFile := os.Getenv("PENFOLD_CONFIG_FILE"); configFile != "" {
		if err := loadFromYAML(cfg, configFile); err != nil {
			return nil, fmt.Errorf("loading config file: %w", err)
		}
	}

	// Overlay environment variables.
	loadFromEnv(cfg)

	// Validate the configuration.
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return cfg, nil
}

// loadFromYAML loads configuration from a YAML file.
func loadFromYAML(cfg *Config, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading config file: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("parsing config file: %w", err)
	}

	return nil
}

// loadFromEnv overlays environment variables onto the configuration.
func loadFromEnv(cfg *Config) {
	if v := os.Getenv("PENFOLD_SERVICE_NAME"); v != "" {
		cfg.ServiceName = v
	}

	if v := os.Getenv("PENFOLD_ENVIRONMENT"); v != "" {
		cfg.Environment = Environment(strings.ToLower(v))
	}

	if v := os.Getenv("PENFOLD_LOG_LEVEL"); v != "" {
		cfg.LogLevel = LogLevel(strings.ToLower(v))
	}

	// Database configuration.
	if v := os.Getenv("PENFOLD_DB_HOST"); v != "" {
		cfg.Database.Host = v
	}
	if v := os.Getenv("PENFOLD_DB_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Database.Port = port
		}
	}
	if v := os.Getenv("PENFOLD_DB_NAME"); v != "" {
		cfg.Database.Name = v
	}
	if v := os.Getenv("PENFOLD_DB_USER"); v != "" {
		cfg.Database.User = v
	}
	if v := os.Getenv("PENFOLD_DB_PASSWORD"); v != "" {
		cfg.Database.Password = v
	}

	// Redis configuration.
	if v := os.Getenv("PENFOLD_REDIS_HOST"); v != "" {
		cfg.Redis.Host = v
	}
	if v := os.Getenv("PENFOLD_REDIS_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Redis.Port = port
		}
	}

	// Temporal configuration.
	if v := os.Getenv("PENFOLD_TEMPORAL_HOST"); v != "" {
		cfg.Temporal.Host = v
	}
	if v := os.Getenv("PENFOLD_TEMPORAL_NAMESPACE"); v != "" {
		cfg.Temporal.Namespace = v
	}
}

// Validate checks that the configuration is valid.
func (c *Config) Validate() error {
	var errs []string

	if c.ServiceName == "" {
		errs = append(errs, "service_name is required")
	}

	if !c.Environment.IsValid() {
		errs = append(errs, fmt.Sprintf("invalid environment: %q (must be dev, staging, or prod)", c.Environment))
	}

	if !c.LogLevel.IsValid() {
		errs = append(errs, fmt.Sprintf("invalid log_level: %q (must be debug, info, warn, or error)", c.LogLevel))
	}

	if c.Database.Host == "" {
		errs = append(errs, "database.host is required")
	}
	if c.Database.Port <= 0 || c.Database.Port > 65535 {
		errs = append(errs, fmt.Sprintf("invalid database.port: %d", c.Database.Port))
	}
	if c.Database.Name == "" {
		errs = append(errs, "database.name is required")
	}
	if c.Database.User == "" {
		errs = append(errs, "database.user is required")
	}

	if c.Redis.Host == "" {
		errs = append(errs, "redis.host is required")
	}
	if c.Redis.Port <= 0 || c.Redis.Port > 65535 {
		errs = append(errs, fmt.Sprintf("invalid redis.port: %d", c.Redis.Port))
	}

	if c.Temporal.Host == "" {
		errs = append(errs, "temporal.host is required")
	}
	if c.Temporal.Namespace == "" {
		errs = append(errs, "temporal.namespace is required")
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}

	return nil
}

// IsValid checks if the environment value is valid.
func (e Environment) IsValid() bool {
	switch e {
	case EnvDev, EnvStaging, EnvProd:
		return true
	default:
		return false
	}
}

// String returns the string representation of the environment.
func (e Environment) String() string {
	return string(e)
}

// IsValid checks if the log level value is valid.
func (l LogLevel) IsValid() bool {
	switch l {
	case LogDebug, LogInfo, LogWarn, LogError:
		return true
	default:
		return false
	}
}

// String returns the string representation of the log level.
func (l LogLevel) String() string {
	return string(l)
}

// IsDevelopment returns true if running in development environment.
func (c *Config) IsDevelopment() bool {
	return c.Environment == EnvDev
}

// IsProduction returns true if running in production environment.
func (c *Config) IsProduction() bool {
	return c.Environment == EnvProd
}

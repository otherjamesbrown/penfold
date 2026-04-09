// Package config provides configuration for the Temporal worker service.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// TaskQueueNames defines the task queue names used by Penfold.
const (
	// MainTaskQueue handles general workflows.
	MainTaskQueue = "penfold-main"

	// AITaskQueue handles AI-intensive workflows (embeddings, LLM calls).
	AITaskQueue = "penfold-ai"

	// EmailTaskQueue handles email processing workflows.
	EmailTaskQueue = "penfold-email"
)

// AllTaskQueues returns all available task queues.
func AllTaskQueues() []string {
	return []string{MainTaskQueue, AITaskQueue, EmailTaskQueue}
}

// Config holds the Temporal worker service configuration.
type Config struct {
	// ServiceName identifies this worker instance.
	ServiceName string

	// HTTPPort is the port for HTTP endpoints (health, metrics).
	HTTPPort int

	// Temporal connection settings.
	TemporalHostPort  string
	TemporalNamespace string

	// TaskQueues specifies which task queues this worker should poll.
	// Comma-separated list: "penfold-main,penfold-ai"
	TaskQueues []string

	// Worker concurrency settings.
	MaxConcurrentActivities int
	MaxConcurrentWorkflows  int

	// Graceful shutdown timeout in seconds.
	GracefulShutdownTimeout int

	// LogLevel controls logging verbosity (debug, info, warn, error).
	LogLevel string

	// Environment is the deployment environment (dev, staging, prod).
	Environment string

	// EnableMetrics enables Prometheus metrics collection.
	EnableMetrics bool

	// DatabaseURL is the PostgreSQL connection string.
	DatabaseURL string

	// AIServiceURL is the URL for the AI/embeddings service (deprecated, use AIServiceAddr for gRPC).
	AIServiceURL string

	// AIServiceAddr is the gRPC address for the AI Coordinator service.
	AIServiceAddr string

	// GatewayAddr is the gRPC address for the Gateway service (pipeline service).
	GatewayAddr string

	// SkillsPath is the directory containing automation rule skill .md files.
	// Defaults to "./skills".
	SkillsPath string
}

// Default configuration values.
const (
	DefaultServiceName              = "penfold-worker"
	DefaultHTTPPort                 = 8085
	DefaultTemporalHostPort         = "localhost:7233"
	DefaultTemporalNamespace        = "default"
	DefaultMaxConcurrentActivities  = 10
	DefaultMaxConcurrentWorkflows   = 10
	DefaultGracefulShutdownTimeout  = 30
	DefaultLogLevel                 = "info"
	DefaultEnvironment              = "dev"
	DefaultSkillsPath               = "./skills"
)

// Load loads the configuration from environment variables.
func Load() (*Config, error) {
	cfg := &Config{
		ServiceName:             DefaultServiceName,
		HTTPPort:                DefaultHTTPPort,
		TemporalHostPort:        DefaultTemporalHostPort,
		TemporalNamespace:       DefaultTemporalNamespace,
		TaskQueues:              AllTaskQueues(), // Default to all queues
		MaxConcurrentActivities: DefaultMaxConcurrentActivities,
		MaxConcurrentWorkflows:  DefaultMaxConcurrentWorkflows,
		GracefulShutdownTimeout: DefaultGracefulShutdownTimeout,
		LogLevel:                DefaultLogLevel,
		Environment:             DefaultEnvironment,
		EnableMetrics:           true,
	}

	// Load from environment variables
	if v := os.Getenv("WORKER_SERVICE_NAME"); v != "" {
		cfg.ServiceName = v
	}

	if v := os.Getenv("WORKER_HTTP_PORT"); v != "" {
		port, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid WORKER_HTTP_PORT: %w", err)
		}
		cfg.HTTPPort = port
	}

	if v := os.Getenv("TEMPORAL_HOST_PORT"); v != "" {
		cfg.TemporalHostPort = v
	}

	if v := os.Getenv("TEMPORAL_NAMESPACE"); v != "" {
		cfg.TemporalNamespace = v
	}

	if v := os.Getenv("WORKER_TASK_QUEUES"); v != "" {
		cfg.TaskQueues = parseTaskQueues(v)
	}

	if v := os.Getenv("WORKER_MAX_CONCURRENT_ACTIVITIES"); v != "" {
		max, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid WORKER_MAX_CONCURRENT_ACTIVITIES: %w", err)
		}
		cfg.MaxConcurrentActivities = max
	}

	if v := os.Getenv("WORKER_MAX_CONCURRENT_WORKFLOWS"); v != "" {
		max, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid WORKER_MAX_CONCURRENT_WORKFLOWS: %w", err)
		}
		cfg.MaxConcurrentWorkflows = max
	}

	if v := os.Getenv("WORKER_GRACEFUL_SHUTDOWN_TIMEOUT"); v != "" {
		timeout, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid WORKER_GRACEFUL_SHUTDOWN_TIMEOUT: %w", err)
		}
		cfg.GracefulShutdownTimeout = timeout
	}

	if v := os.Getenv("WORKER_LOG_LEVEL"); v != "" {
		cfg.LogLevel = strings.ToLower(v)
	}

	if v := os.Getenv("WORKER_ENVIRONMENT"); v != "" {
		cfg.Environment = strings.ToLower(v)
	}

	if v := os.Getenv("WORKER_ENABLE_METRICS"); v != "" {
		cfg.EnableMetrics = strings.ToLower(v) == "true" || v == "1"
	}

	if v := os.Getenv("DATABASE_URL"); v != "" {
		cfg.DatabaseURL = v
	}

	if v := os.Getenv("AI_SERVICE_URL"); v != "" {
		cfg.AIServiceURL = v
	} else {
		cfg.AIServiceURL = "http://localhost:11434"
	}

	if v := os.Getenv("AI_SERVICE_ADDR"); v != "" {
		cfg.AIServiceAddr = v
	} else {
		cfg.AIServiceAddr = "localhost:50055"
	}

	if v := os.Getenv("GATEWAY_ADDR"); v != "" {
		cfg.GatewayAddr = v
	} else {
		cfg.GatewayAddr = "dev02.brown.chat:50051"
	}

	if v := os.Getenv("WORKER_SKILLS_PATH"); v != "" {
		cfg.SkillsPath = v
	} else {
		cfg.SkillsPath = DefaultSkillsPath
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// parseTaskQueues parses a comma-separated list of task queue names.
func parseTaskQueues(s string) []string {
	parts := strings.Split(s, ",")
	queues := make([]string, 0, len(parts))
	for _, part := range parts {
		q := strings.TrimSpace(part)
		if q != "" {
			queues = append(queues, q)
		}
	}
	return queues
}

// Validate checks that the configuration is valid.
func (c *Config) Validate() error {
	var errs []string

	if c.ServiceName == "" {
		errs = append(errs, "service_name is required")
	}

	if c.HTTPPort <= 0 || c.HTTPPort > 65535 {
		errs = append(errs, fmt.Sprintf("invalid http_port: %d", c.HTTPPort))
	}

	if c.TemporalHostPort == "" {
		errs = append(errs, "temporal_host_port is required")
	}

	if c.TemporalNamespace == "" {
		errs = append(errs, "temporal_namespace is required")
	}

	if len(c.TaskQueues) == 0 {
		errs = append(errs, "at least one task_queue is required")
	}

	if c.MaxConcurrentActivities <= 0 {
		errs = append(errs, fmt.Sprintf("invalid max_concurrent_activities: %d", c.MaxConcurrentActivities))
	}

	if c.MaxConcurrentWorkflows <= 0 {
		errs = append(errs, fmt.Sprintf("invalid max_concurrent_workflows: %d", c.MaxConcurrentWorkflows))
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

// HTTPAddr returns the HTTP server address.
func (c *Config) HTTPAddr() string {
	return fmt.Sprintf(":%d", c.HTTPPort)
}

// HasTaskQueue returns true if the worker is configured to poll the given queue.
func (c *Config) HasTaskQueue(queue string) bool {
	for _, q := range c.TaskQueues {
		if q == queue {
			return true
		}
	}
	return false
}

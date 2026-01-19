package temporal

import (
	"os"
	"strconv"
)

// DefaultConfig values for Temporal connection.
const (
	DefaultHostPort                = "localhost:7233"
	DefaultNamespace               = "default"
	DefaultTaskQueue               = "penfold-ai-processing"
	DefaultMaxConcurrentActivities = 10
	DefaultMaxConcurrentWorkflows  = 10
)

// LoadConfigFromEnv loads Temporal configuration from environment variables.
// Environment variables:
//   - TEMPORAL_HOST_PORT: Temporal server address (default: localhost:7233)
//   - TEMPORAL_NAMESPACE: Temporal namespace (default: default)
//   - TEMPORAL_TASK_QUEUE: Task queue name (default: penfold-ai-processing)
//   - TEMPORAL_MAX_CONCURRENT_ACTIVITIES: Max concurrent activities (default: 10)
//   - TEMPORAL_MAX_CONCURRENT_WORKFLOWS: Max concurrent workflows (default: 10)
func LoadConfigFromEnv() *Config {
	return &Config{
		HostPort:                getEnvOrDefault("TEMPORAL_HOST_PORT", DefaultHostPort),
		Namespace:               getEnvOrDefault("TEMPORAL_NAMESPACE", DefaultNamespace),
		TaskQueue:               getEnvOrDefault("TEMPORAL_TASK_QUEUE", DefaultTaskQueue),
		MaxConcurrentActivities: getEnvIntOrDefault("TEMPORAL_MAX_CONCURRENT_ACTIVITIES", DefaultMaxConcurrentActivities),
		MaxConcurrentWorkflows:  getEnvIntOrDefault("TEMPORAL_MAX_CONCURRENT_WORKFLOWS", DefaultMaxConcurrentWorkflows),
	}
}

// getEnvOrDefault returns the value of the environment variable or the default value.
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvIntOrDefault returns the integer value of the environment variable or the default value.
func getEnvIntOrDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

package temporal

import "os"

// DefaultConfig values for Temporal connection.
const (
	DefaultHostPort  = "localhost:7233"
	DefaultNamespace = "default"
	DefaultTaskQueue = "penfold-ai-processing"
)

// LoadConfigFromEnv loads Temporal configuration from environment variables.
// Environment variables:
//   - TEMPORAL_HOST_PORT: Temporal server address (default: localhost:7233)
//   - TEMPORAL_NAMESPACE: Temporal namespace (default: default)
//   - TEMPORAL_TASK_QUEUE: Task queue name (default: penfold-ai-processing)
func LoadConfigFromEnv() *Config {
	return &Config{
		HostPort:  getEnvOrDefault("TEMPORAL_HOST_PORT", DefaultHostPort),
		Namespace: getEnvOrDefault("TEMPORAL_NAMESPACE", DefaultNamespace),
		TaskQueue: getEnvOrDefault("TEMPORAL_TASK_QUEUE", DefaultTaskQueue),
	}
}

// getEnvOrDefault returns the value of the environment variable or the default value.
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

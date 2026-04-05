package temporal

import (
	"os"
	"testing"
)

func TestLoadConfigFromEnv(t *testing.T) {
	// Save original env vars
	origHostPort := os.Getenv("TEMPORAL_HOST_PORT")
	origNamespace := os.Getenv("TEMPORAL_NAMESPACE")
	origTaskQueue := os.Getenv("TEMPORAL_TASK_QUEUE")
	origMaxActivities := os.Getenv("TEMPORAL_MAX_CONCURRENT_ACTIVITIES")
	origMaxWorkflows := os.Getenv("TEMPORAL_MAX_CONCURRENT_WORKFLOWS")

	// Clean up after test
	defer func() {
		setEnvOrUnset("TEMPORAL_HOST_PORT", origHostPort)
		setEnvOrUnset("TEMPORAL_NAMESPACE", origNamespace)
		setEnvOrUnset("TEMPORAL_TASK_QUEUE", origTaskQueue)
		setEnvOrUnset("TEMPORAL_MAX_CONCURRENT_ACTIVITIES", origMaxActivities)
		setEnvOrUnset("TEMPORAL_MAX_CONCURRENT_WORKFLOWS", origMaxWorkflows)
	}()

	t.Run("uses defaults when env vars not set", func(t *testing.T) {
		_ = os.Unsetenv("TEMPORAL_HOST_PORT")
		_ = os.Unsetenv("TEMPORAL_NAMESPACE")
		_ = os.Unsetenv("TEMPORAL_TASK_QUEUE")
		_ = os.Unsetenv("TEMPORAL_MAX_CONCURRENT_ACTIVITIES")
		_ = os.Unsetenv("TEMPORAL_MAX_CONCURRENT_WORKFLOWS")

		cfg := LoadConfigFromEnv()

		if cfg.HostPort != DefaultHostPort {
			t.Errorf("expected HostPort %s, got %s", DefaultHostPort, cfg.HostPort)
		}
		if cfg.Namespace != DefaultNamespace {
			t.Errorf("expected Namespace %s, got %s", DefaultNamespace, cfg.Namespace)
		}
		if cfg.TaskQueue != DefaultTaskQueue {
			t.Errorf("expected TaskQueue %s, got %s", DefaultTaskQueue, cfg.TaskQueue)
		}
		if cfg.MaxConcurrentActivities != DefaultMaxConcurrentActivities {
			t.Errorf("expected MaxConcurrentActivities %d, got %d", DefaultMaxConcurrentActivities, cfg.MaxConcurrentActivities)
		}
		if cfg.MaxConcurrentWorkflows != DefaultMaxConcurrentWorkflows {
			t.Errorf("expected MaxConcurrentWorkflows %d, got %d", DefaultMaxConcurrentWorkflows, cfg.MaxConcurrentWorkflows)
		}
	})

	t.Run("uses env vars when set", func(t *testing.T) {
		_ = os.Setenv("TEMPORAL_HOST_PORT", "temporal:7233")
		_ = os.Setenv("TEMPORAL_NAMESPACE", "penfold")
		_ = os.Setenv("TEMPORAL_TASK_QUEUE", "custom-queue")
		_ = os.Setenv("TEMPORAL_MAX_CONCURRENT_ACTIVITIES", "20")
		_ = os.Setenv("TEMPORAL_MAX_CONCURRENT_WORKFLOWS", "15")

		cfg := LoadConfigFromEnv()

		if cfg.HostPort != "temporal:7233" {
			t.Errorf("expected HostPort temporal:7233, got %s", cfg.HostPort)
		}
		if cfg.Namespace != "penfold" {
			t.Errorf("expected Namespace penfold, got %s", cfg.Namespace)
		}
		if cfg.TaskQueue != "custom-queue" {
			t.Errorf("expected TaskQueue custom-queue, got %s", cfg.TaskQueue)
		}
		if cfg.MaxConcurrentActivities != 20 {
			t.Errorf("expected MaxConcurrentActivities 20, got %d", cfg.MaxConcurrentActivities)
		}
		if cfg.MaxConcurrentWorkflows != 15 {
			t.Errorf("expected MaxConcurrentWorkflows 15, got %d", cfg.MaxConcurrentWorkflows)
		}
	})

	t.Run("uses defaults for invalid int values", func(t *testing.T) {
		_ = os.Setenv("TEMPORAL_MAX_CONCURRENT_ACTIVITIES", "invalid")
		_ = os.Setenv("TEMPORAL_MAX_CONCURRENT_WORKFLOWS", "not-a-number")

		cfg := LoadConfigFromEnv()

		if cfg.MaxConcurrentActivities != DefaultMaxConcurrentActivities {
			t.Errorf("expected MaxConcurrentActivities %d, got %d", DefaultMaxConcurrentActivities, cfg.MaxConcurrentActivities)
		}
		if cfg.MaxConcurrentWorkflows != DefaultMaxConcurrentWorkflows {
			t.Errorf("expected MaxConcurrentWorkflows %d, got %d", DefaultMaxConcurrentWorkflows, cfg.MaxConcurrentWorkflows)
		}
	})
}

func TestGetEnvOrDefault(t *testing.T) {
	const testKey = "TEST_TEMPORAL_VAR"

	defer os.Unsetenv(testKey) //nolint:errcheck

	t.Run("returns default when not set", func(t *testing.T) {
		_ = os.Unsetenv(testKey)
		result := getEnvOrDefault(testKey, "default-value")
		if result != "default-value" {
			t.Errorf("expected default-value, got %s", result)
		}
	})

	t.Run("returns env value when set", func(t *testing.T) {
		_ = os.Setenv(testKey, "custom-value")
		result := getEnvOrDefault(testKey, "default-value")
		if result != "custom-value" {
			t.Errorf("expected custom-value, got %s", result)
		}
	})

	t.Run("returns default for empty string", func(t *testing.T) {
		_ = os.Setenv(testKey, "")
		result := getEnvOrDefault(testKey, "default-value")
		if result != "default-value" {
			t.Errorf("expected default-value, got %s", result)
		}
	})
}

func TestGetEnvIntOrDefault(t *testing.T) {
	const testKey = "TEST_TEMPORAL_INT_VAR"

	defer os.Unsetenv(testKey) //nolint:errcheck

	t.Run("returns default when not set", func(t *testing.T) {
		_ = os.Unsetenv(testKey)
		result := getEnvIntOrDefault(testKey, 42)
		if result != 42 {
			t.Errorf("expected 42, got %d", result)
		}
	})

	t.Run("returns parsed int when valid", func(t *testing.T) {
		_ = os.Setenv(testKey, "100")
		result := getEnvIntOrDefault(testKey, 42)
		if result != 100 {
			t.Errorf("expected 100, got %d", result)
		}
	})

	t.Run("returns default for invalid int", func(t *testing.T) {
		_ = os.Setenv(testKey, "not-a-number")
		result := getEnvIntOrDefault(testKey, 42)
		if result != 42 {
			t.Errorf("expected 42, got %d", result)
		}
	})

	t.Run("handles negative numbers", func(t *testing.T) {
		_ = os.Setenv(testKey, "-5")
		result := getEnvIntOrDefault(testKey, 42)
		if result != -5 {
			t.Errorf("expected -5, got %d", result)
		}
	})

	t.Run("handles zero", func(t *testing.T) {
		_ = os.Setenv(testKey, "0")
		result := getEnvIntOrDefault(testKey, 42)
		if result != 0 {
			t.Errorf("expected 0, got %d", result)
		}
	})
}

// setEnvOrUnset sets an environment variable or unsets it if the value is empty.
func setEnvOrUnset(key, value string) {
	if value == "" {
		_ = os.Unsetenv(key)
	} else {
		_ = os.Setenv(key, value)
	}
}

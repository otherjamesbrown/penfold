package main

import (
	"testing"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/timeout"
	"github.com/stretchr/testify/require"
)

// TestTimeoutConfigWithNilDB verifies that timeout config works with nil database.
// This ensures the worker can start with defaults even if DB is unavailable.
func TestTimeoutConfigWithNilDB(t *testing.T) {
	// Create config with nil DB (simulates DB unavailable at startup)
	cfg := timeout.New(nil)

	// Should still provide default values
	aiTimeout := cfg.Get("timeout.ai_client.request")
	require.Equal(t, 120*time.Second, aiTimeout, "ai_client.request should default to 120s")

	fastTimeout := cfg.Get("timeout.activity.fast.start_to_close")
	require.Equal(t, 30*time.Second, fastTimeout, "activity.fast.start_to_close should default to 30s")

	llmTimeout := cfg.Get("timeout.activity.llm.start_to_close")
	require.Equal(t, 600*time.Second, llmTimeout, "activity.llm.start_to_close should default to 600s")

	// Unknown keys should return 0
	unknownTimeout := cfg.Get("timeout.unknown.key")
	require.Equal(t, time.Duration(0), unknownTimeout, "unknown keys should return 0")
}

// TestTimeoutConfigDefaults verifies all expected timeout keys have defaults.
func TestTimeoutConfigDefaults(t *testing.T) {
	cfg := timeout.New(nil)

	expectedKeys := []string{
		"timeout.ai_client.request",
		"timeout.activity.fast.start_to_close",
		"timeout.activity.fast.heartbeat",
		"timeout.activity.embedding.start_to_close",
		"timeout.activity.embedding.heartbeat",
		"timeout.activity.llm.start_to_close",
		"timeout.activity.llm.heartbeat",
		"timeout.activity.batch.start_to_close",
		"timeout.activity.batch.heartbeat",
		"timeout.http.backend.gemini",
		"timeout.http.backend.mlx",
		"timeout.schedule_to_close.default",
	}

	for _, key := range expectedKeys {
		value := cfg.Get(key)
		require.NotZero(t, value, "key %s should have a non-zero default", key)
		require.Greater(t, value, time.Duration(0), "key %s should have a positive timeout", key)
	}
}

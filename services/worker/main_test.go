package main

import (
	"testing"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/timeout"
	"github.com/stretchr/testify/require"
)

const testTenantID = "c3170310-78bd-409c-b186-126f40bfa6ad"

// TestTimeoutConfigWithNilDB verifies that timeout config works with nil database.
// This ensures the worker can start with defaults even if DB is unavailable.
func TestTimeoutConfigWithNilDB(t *testing.T) {
	// Create config with nil DB (simulates DB unavailable at startup)
	cfg := timeout.New(nil, testTenantID)

	// Should still provide default values for active keys
	aiTimeout := cfg.Get("timeout.ai_client.request")
	require.Equal(t, 120*time.Second, aiTimeout, "ai_client.request should default to 120s")

	geminiTimeout := cfg.Get("timeout.http.backend.gemini")
	require.Equal(t, 120*time.Second, geminiTimeout, "http.backend.gemini should default to 120s")

	mlxTimeout := cfg.Get("timeout.http.backend.mlx")
	require.Equal(t, 60*time.Second, mlxTimeout, "http.backend.mlx should default to 60s")

	scheduleDefault := cfg.Get("timeout.schedule_to_close.default")
	require.Equal(t, 3600*time.Second, scheduleDefault, "schedule_to_close.default should default to 3600s")

	// Unknown keys should return 0
	unknownTimeout := cfg.Get("timeout.unknown.key")
	require.Equal(t, time.Duration(0), unknownTimeout, "unknown keys should return 0")

	// Dead keys should return 0 (no longer in defaults)
	deadKey := cfg.Get("timeout.activity.fast.start_to_close")
	require.Equal(t, time.Duration(0), deadKey, "dead activity keys should return 0")
}

// TestTimeoutConfigDefaults verifies all active timeout keys have defaults.
func TestTimeoutConfigDefaults(t *testing.T) {
	cfg := timeout.New(nil, testTenantID)

	expectedKeys := []string{
		"timeout.ai_client.request",
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

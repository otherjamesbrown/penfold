//go:build integration

// Package integration contains integration tests that require real services.
package integration

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCLI_Logs tests the logs command with default output.
func TestCLI_Logs(t *testing.T) {
	stdout, stderr, err := runCLI(t, "logs")

	require.NoError(t, err, "logs command should succeed. stderr: %s", stderr)
	assert.NotEmpty(t, stdout, "should have log output")

	t.Logf("Logs output: %s", stdout)
}

// TestCLI_Logs_JSONOutput tests JSON output format.
func TestCLI_Logs_JSONOutput(t *testing.T) {
	stdout, stderr, err := runCLI(t, "logs", "--output", "json")

	require.NoError(t, err, "logs command with JSON output should succeed. stderr: %s", stderr)

	// JSON output may be empty array or contain log entries
	if strings.TrimSpace(stdout) != "" {
		assert.True(t,
			strings.Contains(stdout, "{") || strings.Contains(stdout, "["),
			"output should be JSON formatted")
	}

	t.Logf("Logs JSON output: %s", stdout)
}

// TestCLI_Logs_YAMLOutput tests YAML output format.
func TestCLI_Logs_YAMLOutput(t *testing.T) {
	stdout, stderr, err := runCLI(t, "logs", "--output", "yaml")

	require.NoError(t, err, "logs command with YAML output should succeed. stderr: %s", stderr)

	// YAML output may be empty or contain log entries
	// We just verify it doesn't error
	t.Logf("Logs YAML output: %s", stdout)
}

// TestCLI_Logs_ServiceFilter tests filtering logs by service.
func TestCLI_Logs_ServiceFilter(t *testing.T) {
	stdout, stderr, err := runCLI(t, "logs", "--service", "gateway")

	require.NoError(t, err, "logs command with service filter should succeed. stderr: %s", stderr)

	// Output may be empty if no logs for this service
	t.Logf("Logs for gateway service: %s", stdout)
}

// TestCLI_Logs_LevelFilter tests filtering logs by level.
func TestCLI_Logs_LevelFilter(t *testing.T) {
	stdout, stderr, err := runCLI(t, "logs", "--level", "error")

	require.NoError(t, err, "logs command with level filter should succeed. stderr: %s", stderr)

	// Output may be empty if no error logs
	t.Logf("Error level logs: %s", stdout)
}

// TestCLI_Logs_MultipleFilters tests combining service and level filters.
func TestCLI_Logs_MultipleFilters(t *testing.T) {
	stdout, stderr, err := runCLI(t, "logs", "--service", "gateway", "--level", "info")

	require.NoError(t, err, "logs command with multiple filters should succeed. stderr: %s", stderr)

	t.Logf("Filtered logs: %s", stdout)
}

// TestCLI_Logs_InvalidLevel tests that invalid log level is handled gracefully.
func TestCLI_Logs_InvalidLevel(t *testing.T) {
	_, stderr, err := runCLI(t, "logs", "--level", "invalid_level")

	// Should either error or handle gracefully
	if err != nil {
		assert.Contains(t, strings.ToLower(stderr), "invalid",
			"error message should mention invalid level")
	}

	t.Logf("Invalid level error: %s", stderr)
}

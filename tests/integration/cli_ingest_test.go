//go:build integration

// Package integration contains integration tests that require real services.
package integration

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCLI_IngestStatus_Overall tests the ingest status command.
func TestCLI_IngestStatus_Overall(t *testing.T) {
	stdout, stderr, err := runCLI(t, "ingest", "status")

	require.NoError(t, err, "ingest status should succeed. stderr: %s", stderr)
	assert.NotEmpty(t, stdout, "should have status output")

	t.Logf("Ingest status output: %s", stdout)
}

// TestCLI_IngestStatus_SpecificJob tests the ingest status command for a specific job.
func TestCLI_IngestStatus_SpecificJob(t *testing.T) {
	t.Skip("Requires seeded job data - implement with test fixtures")

	// This test would check status for a specific job ID
	// stdout, stderr, err := runCLI(t, "ingest", "status", "--job", "job-123")
	// require.NoError(t, err)
	// assert.Contains(t, stdout, "job-123")
}

// TestCLI_IngestQueue tests the ingest queue command.
func TestCLI_IngestQueue(t *testing.T) {
	stdout, stderr, err := runCLI(t, "ingest", "queue")

	require.NoError(t, err, "ingest queue should succeed. stderr: %s", stderr)
	assert.NotEmpty(t, stdout, "should have queue output")

	t.Logf("Ingest queue output: %s", stdout)
}

// TestCLI_IngestQueue_JSONOutput tests JSON output format for queue.
func TestCLI_IngestQueue_JSONOutput(t *testing.T) {
	stdout, stderr, err := runCLI(t, "ingest", "queue", "--output", "json")

	require.NoError(t, err, "ingest queue with JSON output should succeed. stderr: %s", stderr)
	// Queue may return an empty array [] or an object {} depending on whether there are items
	assert.True(t, strings.Contains(stdout, "{") || strings.Contains(stdout, "["),
		"output should be JSON (object or array)")

	t.Logf("Queue JSON output: %s", stdout)
}

// TestCLI_IngestGmailStatus tests the Gmail status subcommand.
func TestCLI_IngestGmailStatus(t *testing.T) {
	stdout, stderr, err := runCLI(t, "ingest", "gmail", "status")

	// This may fail if Gmail integration is not configured
	if err != nil {
		t.Logf("Gmail status returned error (may not be configured): %s", stderr)
		return
	}

	assert.NotEmpty(t, stdout, "should have Gmail status output")
	t.Logf("Gmail status output: %s", stdout)
}

// TestCLI_IngestGmailHistory tests the Gmail history subcommand.
func TestCLI_IngestGmailHistory(t *testing.T) {
	stdout, stderr, err := runCLI(t, "ingest", "gmail", "history")

	// This may fail if Gmail integration is not configured
	if err != nil {
		t.Logf("Gmail history returned error (may not be configured): %s", stderr)
		return
	}

	assert.NotEmpty(t, stdout, "should have Gmail history output")
	t.Logf("Gmail history output: %s", stdout)
}

// TestCLI_IngestGmailSync tests the Gmail sync subcommand (incremental).
func TestCLI_IngestGmailSync(t *testing.T) {
	t.Skip("Gmail sync requires OAuth setup and may trigger actual sync - implement with proper fixtures")

	// This test would trigger an actual Gmail sync
	// stdout, stderr, err := runCLI(t, "ingest", "gmail", "sync")
	// require.NoError(t, err, "Gmail sync should succeed. stderr: %s", stderr)
}

// TestCLI_IngestGmailSync_Full tests the Gmail full sync subcommand.
func TestCLI_IngestGmailSync_Full(t *testing.T) {
	t.Skip("Full Gmail sync requires OAuth setup and may trigger actual sync - implement with proper fixtures")

	// This test would trigger a full Gmail sync
	// stdout, stderr, err := runCLI(t, "ingest", "gmail", "sync", "--full")
	// require.NoError(t, err, "Gmail full sync should succeed. stderr: %s", stderr)
}

// TestCLI_IngestConfigShow tests the config show subcommand.
func TestCLI_IngestConfigShow(t *testing.T) {
	stdout, stderr, err := runCLI(t, "ingest", "config", "show")

	require.NoError(t, err, "ingest config show should succeed. stderr: %s", stderr)
	assert.NotEmpty(t, stdout, "should have config output")

	t.Logf("Ingest config output: %s", stdout)
}

// TestCLI_IngestConfigSet_ValidKey tests setting a valid configuration key.
func TestCLI_IngestConfigSet_ValidKey(t *testing.T) {
	t.Skip("Config set requires proper setup and may modify server state - implement with proper fixtures")

	// This test would set a config value
	// stdout, stderr, err := runCLI(t, "ingest", "config", "set", "some_key", "some_value")
	// require.NoError(t, err, "ingest config set should succeed. stderr: %s", stderr)
}

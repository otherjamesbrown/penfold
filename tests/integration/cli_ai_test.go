//go:build integration

// Package integration contains integration tests that require real services.
package integration

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCLI_AIQuery_Success tests the full AI query flow through the CLI.
func TestCLI_AIQuery_Success(t *testing.T) {
	stdout, stderr, err := runCLI(t, "ai", "query", "What is 2+2?")

	require.NoError(t, err, "AI query should succeed. stderr: %s", stderr)
	assert.NotEmpty(t, stdout, "should have output")

	// The response should contain something related to the answer
	// (AI responses vary, so we just check for non-empty response)
	t.Logf("AI query response: %s", stdout)
}

// TestCLI_AIQuery_JSONOutput tests JSON output format.
func TestCLI_AIQuery_JSONOutput(t *testing.T) {
	stdout, stderr, err := runCLI(t, "ai", "query", "--output", "json", "What is the capital of France?")

	require.NoError(t, err, "AI query with JSON output should succeed. stderr: %s", stderr)
	assert.Contains(t, stdout, "{", "output should be JSON")
	assert.Contains(t, stdout, "response", "JSON should contain response field")
}

// TestCLI_AIQuery_YAMLOutput tests YAML output format.
func TestCLI_AIQuery_YAMLOutput(t *testing.T) {
	stdout, stderr, err := runCLI(t, "ai", "query", "--output", "yaml", "What is 1+1?")

	require.NoError(t, err, "AI query with YAML output should succeed. stderr: %s", stderr)
	// YAML output typically has key: value format
	assert.Contains(t, stdout, ":", "output should be YAML formatted")
}

// TestCLI_AISummarize_Brief tests brief summary generation.
func TestCLI_AISummarize_Brief(t *testing.T) {
	// First, we need a content ID to summarize
	// This test requires seeded data or a known content ID
	t.Skip("Requires seeded content - implement with test fixtures")
}

// TestCLI_AISummarize_Standard tests standard summary generation.
func TestCLI_AISummarize_Standard(t *testing.T) {
	t.Skip("Requires seeded content - implement with test fixtures")
}

// TestCLI_AISummarize_Detailed tests detailed summary generation.
func TestCLI_AISummarize_Detailed(t *testing.T) {
	t.Skip("Requires seeded content - implement with test fixtures")
}

// TestCLI_AIAnalyze_Sentiment tests sentiment analysis.
func TestCLI_AIAnalyze_Sentiment(t *testing.T) {
	t.Skip("Requires seeded content - implement with test fixtures")
}

// TestCLI_AIAnalyze_Entities tests entity extraction.
func TestCLI_AIAnalyze_Entities(t *testing.T) {
	t.Skip("Requires seeded content - implement with test fixtures")
}

// TestCLI_AIAnalyze_Topics tests topic identification.
func TestCLI_AIAnalyze_Topics(t *testing.T) {
	t.Skip("Requires seeded content - implement with test fixtures")
}

// TestCLI_AIAnalyze_Actions tests action item extraction.
func TestCLI_AIAnalyze_Actions(t *testing.T) {
	t.Skip("Requires seeded content - implement with test fixtures")
}

// TestCLI_AIAnalyze_Full tests full analysis.
func TestCLI_AIAnalyze_Full(t *testing.T) {
	t.Skip("Requires seeded content - implement with test fixtures")
}

// TestCLI_AIQuery_ConnectionFailure tests that connection failures are reported properly.
// This test should be run when the gateway is intentionally down to verify error handling.
func TestCLI_AIQuery_ConnectionFailure(t *testing.T) {
	// Override to a non-existent address
	os.Setenv("PENF_SERVER_ADDRESS", "localhost:59999")
	defer os.Unsetenv("PENF_SERVER_ADDRESS")

	_, stderr, err := runCLI(t, "ai", "query", "test")

	require.Error(t, err, "should fail when gateway is unavailable")
	assert.True(t,
		strings.Contains(stderr, "connection") ||
			strings.Contains(stderr, "connect") ||
			strings.Contains(stderr, "refused") ||
			strings.Contains(stderr, "unavailable"),
		"error should indicate connection failure: %s", stderr)
}

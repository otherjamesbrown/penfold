//go:build integration

package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCLI_ProcessMentionsContext tests getting mentions context.
func TestCLI_ProcessMentionsContext(t *testing.T) {
	db := SetupTestDB(t)
	EnsureAcmeCorpFixtures(t, db)

	stdout, stderr, err := runCLI(t, "process", "mentions", "context")

	require.NoError(t, err, "process mentions context should succeed. stderr: %s", stderr)
	// Output may be empty if no pending mentions - this tests that the command runs
	t.Logf("Process mentions context output: %s", stdout)
}

// TestCLI_ProcessMentionsContext_JSONOutput tests JSON output format.
func TestCLI_ProcessMentionsContext_JSONOutput(t *testing.T) {
	db := SetupTestDB(t)
	EnsureAcmeCorpFixtures(t, db)

	stdout, stderr, err := runCLI(t, "process", "mentions", "context", "-o", "json")

	require.NoError(t, err, "process mentions context JSON should succeed. stderr: %s", stderr)
	// Even with no pending mentions, JSON output should be valid
	assert.Contains(t, stdout, "{", "output should be JSON")
}

// TestCLI_ProcessMentionsContext_Limit tests the --limit flag.
func TestCLI_ProcessMentionsContext_Limit(t *testing.T) {
	db := SetupTestDB(t)
	EnsureAcmeCorpFixtures(t, db)

	stdout, stderr, err := runCLI(t, "process", "mentions", "context", "--limit", "10")

	require.NoError(t, err, "process mentions context with limit should succeed. stderr: %s", stderr)
	t.Logf("Process mentions context with limit output: %s", stdout)
}

// TestCLI_ProcessAcronymsContext tests getting acronyms context.
func TestCLI_ProcessAcronymsContext(t *testing.T) {
	db := SetupTestDB(t)
	EnsureAcmeCorpFixtures(t, db)

	stdout, stderr, err := runCLI(t, "process", "acronyms", "context")

	require.NoError(t, err, "process acronyms context should succeed. stderr: %s", stderr)
	// Output may be empty if no pending acronyms - this tests that the command runs
	t.Logf("Process acronyms context output: %s", stdout)
}

// TestCLI_ProcessAcronymsContext_JSONOutput tests JSON output format.
func TestCLI_ProcessAcronymsContext_JSONOutput(t *testing.T) {
	db := SetupTestDB(t)
	EnsureAcmeCorpFixtures(t, db)

	stdout, stderr, err := runCLI(t, "process", "acronyms", "context", "-o", "json")

	require.NoError(t, err, "process acronyms context JSON should succeed. stderr: %s", stderr)
	// Even with no pending acronyms, JSON output should be valid
	assert.Contains(t, stdout, "{", "output should be JSON")
}

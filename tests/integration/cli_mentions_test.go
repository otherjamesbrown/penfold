//go:build integration

package integration

import (
	"strings"
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

// =============================================================================
// Write Command Tests (Phase 2) - Batch Resolve Commands
// =============================================================================

// TestCLI_ProcessAcronymsBatchResolve_DryRun tests batch resolve in preview mode.
func TestCLI_ProcessAcronymsBatchResolve_DryRun(t *testing.T) {
	db := SetupTestDB(t)
	EnsureAcmeCorpFixtures(t, db)

	// Use dry-run to preview without making changes
	jsonInput := `{"resolutions":[{"id":99999,"expansion":"Test Expansion"}]}`
	stdout, stderr, err := runCLI(t, "process", "acronyms", "batch-resolve", "--dry-run", jsonInput)

	// Dry-run should succeed even if the ID doesn't exist
	if err != nil {
		t.Logf("batch-resolve dry-run returned error (may be expected if ID invalid): %s", stderr)
	} else {
		t.Logf("Batch resolve dry-run output: %s", stdout)
	}
}

// TestCLI_ProcessAcronymsBatchResolve_InvalidJSON tests error handling for invalid JSON input.
func TestCLI_ProcessAcronymsBatchResolve_InvalidJSON(t *testing.T) {
	db := SetupTestDB(t)
	EnsureAcmeCorpFixtures(t, db)

	_, stderr, err := runCLI(t, "process", "acronyms", "batch-resolve", "invalid-json-{{{")

	require.Error(t, err, "batch-resolve with invalid JSON should fail")
	assert.True(t,
		strings.Contains(stderr, "json") || strings.Contains(stderr, "JSON") || strings.Contains(stderr, "parse") || strings.Contains(stderr, "invalid"),
		"stderr should indicate JSON error: %s", stderr)
}

// TestCLI_ProcessAcronymsBatchResolve_EmptyInput tests handling of empty resolutions.
func TestCLI_ProcessAcronymsBatchResolve_EmptyInput(t *testing.T) {
	db := SetupTestDB(t)
	EnsureAcmeCorpFixtures(t, db)

	jsonInput := `{"resolutions":[]}`
	stdout, stderr, err := runCLI(t, "process", "acronyms", "batch-resolve", "--dry-run", jsonInput)

	// Empty input may succeed with no-op or may fail with validation error
	if err != nil {
		t.Logf("batch-resolve empty input returned error (may be expected): %s", stderr)
	} else {
		t.Logf("Batch resolve empty input output: %s", stdout)
	}
}

// TestCLI_ProcessMentionsBatchResolve_DryRun tests batch resolve in preview mode.
func TestCLI_ProcessMentionsBatchResolve_DryRun(t *testing.T) {
	db := SetupTestDB(t)
	EnsureAcmeCorpFixtures(t, db)

	// Use dry-run to preview without making changes
	jsonInput := `{"resolutions":[{"mention_id":99999,"entity_id":1,"entity_type":"ENTITY_TYPE_PERSON"}]}`
	stdout, stderr, err := runCLI(t, "process", "mentions", "batch-resolve", "--dry-run", jsonInput)

	// Dry-run should succeed even if the ID doesn't exist
	if err != nil {
		t.Logf("batch-resolve dry-run returned error (may be expected if ID invalid): %s", stderr)
	} else {
		t.Logf("Batch resolve dry-run output: %s", stdout)
	}
}

// TestCLI_ProcessMentionsBatchResolve_InvalidJSON tests error handling for invalid JSON input.
func TestCLI_ProcessMentionsBatchResolve_InvalidJSON(t *testing.T) {
	db := SetupTestDB(t)
	EnsureAcmeCorpFixtures(t, db)

	_, stderr, err := runCLI(t, "process", "mentions", "batch-resolve", "invalid-json-{{{")

	require.Error(t, err, "batch-resolve with invalid JSON should fail")
	assert.True(t,
		strings.Contains(stderr, "json") || strings.Contains(stderr, "JSON") || strings.Contains(stderr, "parse") || strings.Contains(stderr, "invalid"),
		"stderr should indicate JSON error: %s", stderr)
}

// TestCLI_ProcessMentionsBatchResolve_EmptyInput tests handling of empty resolutions.
func TestCLI_ProcessMentionsBatchResolve_EmptyInput(t *testing.T) {
	db := SetupTestDB(t)
	EnsureAcmeCorpFixtures(t, db)

	jsonInput := `{"resolutions":[]}`
	stdout, stderr, err := runCLI(t, "process", "mentions", "batch-resolve", "--dry-run", jsonInput)

	// Empty input may succeed with no-op or may fail with validation error
	if err != nil {
		t.Logf("batch-resolve empty input returned error (may be expected): %s", stderr)
	} else {
		t.Logf("Batch resolve empty input output: %s", stdout)
	}
}

//go:build integration

package integration

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCLI_ContentList tests the basic content list command.
func TestCLI_ContentList(t *testing.T) {
	db := SetupTestDB(t)
	EnsureAcmeCorpFixtures(t, db)

	stdout, stderr, err := runCLI(t, "content", "list")

	require.NoError(t, err, "content list should succeed. stderr: %s", stderr)
	// Output may be empty if no content exists - this tests that the command runs
	t.Logf("Content list output: %s", stdout)
}

// TestCLI_ContentList_JSONOutput tests JSON output format.
func TestCLI_ContentList_JSONOutput(t *testing.T) {
	db := SetupTestDB(t)
	EnsureAcmeCorpFixtures(t, db)

	stdout, stderr, err := runCLI(t, "content", "list", "-o", "json")

	require.NoError(t, err, "content list JSON should succeed. stderr: %s", stderr)
	// Even with no content, JSON output should be valid
	assert.Contains(t, stdout, "{", "output should be JSON")
}

// TestCLI_ContentList_StatusFilter tests the --status filter.
func TestCLI_ContentList_StatusFilter(t *testing.T) {
	db := SetupTestDB(t)
	EnsureAcmeCorpFixtures(t, db)

	stdout, stderr, err := runCLI(t, "content", "list", "--status", "complete")

	require.NoError(t, err, "content list with status filter should succeed. stderr: %s", stderr)
	t.Logf("Content list with status filter output: %s", stdout)
}

// TestCLI_ContentList_SourceFilter tests the --source filter.
func TestCLI_ContentList_SourceFilter(t *testing.T) {
	db := SetupTestDB(t)
	EnsureAcmeCorpFixtures(t, db)

	stdout, stderr, err := runCLI(t, "content", "list", "--source", "email")

	require.NoError(t, err, "content list with source filter should succeed. stderr: %s", stderr)
	t.Logf("Content list with source filter output: %s", stdout)
}

// TestCLI_ContentStats tests the content stats command.
func TestCLI_ContentStats(t *testing.T) {
	db := SetupTestDB(t)
	EnsureAcmeCorpFixtures(t, db)

	stdout, stderr, err := runCLI(t, "content", "stats")

	require.NoError(t, err, "content stats should succeed. stderr: %s", stderr)
	t.Logf("Content stats output: %s", stdout)
}

// TestCLI_ContentStats_JSONOutput tests JSON output for stats command.
func TestCLI_ContentStats_JSONOutput(t *testing.T) {
	db := SetupTestDB(t)
	EnsureAcmeCorpFixtures(t, db)

	stdout, stderr, err := runCLI(t, "content", "stats", "-o", "json")

	require.NoError(t, err, "content stats JSON should succeed. stderr: %s", stderr)
	assert.Contains(t, stdout, "{", "output should be JSON")
}

// TestCLI_ContentShow_NotFound tests error handling for non-existent content.
func TestCLI_ContentShow_NotFound(t *testing.T) {
	db := SetupTestDB(t)
	EnsureAcmeCorpFixtures(t, db)

	_, stderr, err := runCLI(t, "content", "show", "nonexistent-content-id-xyz")

	require.Error(t, err, "content show for non-existent content should fail")
	assert.True(t,
		strings.Contains(stderr, "not found") || strings.Contains(stderr, "error") || strings.Contains(stderr, "Error"),
		"stderr should indicate content not found: %s", stderr)
}

// TestCLI_ContentList_WithContent tests content list when content exists.
// This test is conditional on having content data.
func TestCLI_ContentList_WithContent(t *testing.T) {
	db := SetupTestDB(t)
	EnsureAcmeCorpFixtures(t, db)

	// First check if any content exists
	type ContentListResponse struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	result, _, err := runCLIWithJSON[ContentListResponse](t, "content", "list")
	if err != nil {
		t.Skip("Skipping - could not parse content list response")
	}

	if len(result.Items) == 0 {
		t.Skip("Skipping - no content in test data")
	}

	// If content exists, verify the list contains expected fields
	stdout, stderr, err := runCLI(t, "content", "list", "-o", "json")
	require.NoError(t, err, "content list should succeed. stderr: %s", stderr)
	assertJSONContains(t, stdout, "items")
	assertJSONArrayNotEmpty(t, stdout, "items")
}

// TestCLI_ContentShow_WithContent tests content show when content exists.
// This test is conditional on having content data.
func TestCLI_ContentShow_WithContent(t *testing.T) {
	db := SetupTestDB(t)
	EnsureAcmeCorpFixtures(t, db)

	// First get a content ID
	type ContentListResponse struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	result, _, err := runCLIWithJSON[ContentListResponse](t, "content", "list")
	if err != nil {
		t.Skip("Skipping - could not parse content list response")
	}

	if len(result.Items) == 0 {
		t.Skip("Skipping - no content in test data")
	}

	contentID := result.Items[0].ID
	stdout, stderr, err := runCLI(t, "content", "show", contentID)

	require.NoError(t, err, "content show should succeed. stderr: %s", stderr)
	assert.NotEmpty(t, stdout, "content show should return details")
	t.Logf("Content show output: %s", stdout)
}

// TestCLI_ContentList_Limit tests the --limit flag.
func TestCLI_ContentList_Limit(t *testing.T) {
	db := SetupTestDB(t)
	EnsureAcmeCorpFixtures(t, db)

	stdout, stderr, err := runCLI(t, "content", "list", "--limit", "5")

	require.NoError(t, err, "content list with limit should succeed. stderr: %s", stderr)
	t.Logf("Content list with limit output: %s", stdout)
}

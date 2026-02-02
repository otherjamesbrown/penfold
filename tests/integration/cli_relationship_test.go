//go:build integration

// Package integration contains integration tests that require real services.
package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCLI_RelationshipList tests the relationship list command.
func TestCLI_RelationshipList(t *testing.T) {
	stdout, stderr, err := runCLI(t, "relationship", "list")

	require.NoError(t, err, "relationship list should succeed. stderr: %s", stderr)
	assert.NotEmpty(t, stdout, "should have output")

	t.Logf("Relationship list output: %s", stdout)
}

// TestCLI_RelationshipList_JSONOutput tests JSON output format.
func TestCLI_RelationshipList_JSONOutput(t *testing.T) {
	stdout, stderr, err := runCLI(t, "relationship", "list", "--output", "json")

	require.NoError(t, err, "relationship list with JSON output should succeed. stderr: %s", stderr)

	// Output may be empty array or contain relationships
	t.Logf("Relationship list JSON: %s", stdout)
}

// TestCLI_RelationshipList_WithFilters tests filtering relationships.
func TestCLI_RelationshipList_WithFilters(t *testing.T) {
	stdout, stderr, err := runCLI(t, "relationship", "list",
		"--type", "mentions",
		"--limit", "10",
		"--min-confidence", "0.5")

	require.NoError(t, err, "relationship list with filters should succeed. stderr: %s", stderr)

	t.Logf("Filtered relationships: %s", stdout)
}

// TestCLI_RelationshipShow tests showing a specific relationship.
func TestCLI_RelationshipShow(t *testing.T) {
	t.Skip("Requires seeded relationship data - implement with test fixtures")

	// This test would show a specific relationship by ID
	// stdout, stderr, err := runCLI(t, "relationship", "show", "rel-001")
	// require.NoError(t, err, "relationship show should succeed. stderr: %s", stderr)
	// assert.Contains(t, stdout, "rel-001")
}

// TestCLI_RelationshipShow_JSONOutput tests JSON output for show command.
func TestCLI_RelationshipShow_JSONOutput(t *testing.T) {
	t.Skip("Requires seeded relationship data - implement with test fixtures")

	// stdout, stderr, err := runCLI(t, "relationship", "show", "rel-001", "--output", "json")
	// require.NoError(t, err)
	// assert.Contains(t, stdout, "{")
	// assert.Contains(t, stdout, "rel-001")
}

// TestCLI_RelationshipSearch tests searching relationships.
func TestCLI_RelationshipSearch(t *testing.T) {
	t.Skip("Requires seeded relationship data - implement with test fixtures")

	// This test would search for relationships by entity
	// stdout, stderr, err := runCLI(t, "relationship", "search", "Alice")
	// require.NoError(t, err, "relationship search should succeed. stderr: %s", stderr)
}

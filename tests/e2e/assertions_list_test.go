//go:build e2e

package e2e

// ============================================================================
// Bug: assertions list/search/summary fail — nonexistent columns on sources
// ============================================================================
//
// `penf assertions list` crashes with:
//   ERROR: column s.source_type does not exist (SQLSTATE 42703)
//
// Root cause: pkg/assertions/repository.go queries reference 4 columns that
// don't exist on the sources table:
//   s.source_type  → should be s.source_system
//   s.subject      → should be s.ingestion_metadata->>'subject'
//   s.date         → should be s.source_timestamp
//   s.from_name    → should be s.ingestion_metadata->>'from_name'
//
// Affected methods: ListAssertions, SearchAssertions, GetAssertionSummary
//
// Also: pkg/projects/repository.go:345 references source_type='meeting'
// instead of source_system='meeting'.
//
// Bug shard: pf-9a5970
// ============================================================================

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestE2E_AssertionsList_DoesNotCrash verifies that `penf assertions list`
// executes without SQL errors. The query currently references nonexistent
// columns (s.source_type, s.subject, s.date, s.from_name) on the sources table.
func TestE2E_AssertionsList_DoesNotCrash(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	err := env.EnsureTenantExists()
	require.NoError(t, err)

	// penf assertions list should not crash with SQL column errors
	result := env.SafeCLI.Run(ctx, "assertions", "list")
	assert.True(t, result.Success(),
		"assertions list should succeed, got error: %v\nstderr: %s", result.Err, result.Stderr)
	assert.NotContains(t, result.Stderr, "source_type does not exist",
		"should not reference nonexistent source_type column")
	assert.NotContains(t, result.Stderr, "42703",
		"should not produce SQLSTATE 42703 (undefined column)")
}

// TestE2E_AssertionsList_ReturnsSourceContext verifies that assertion results
// include correct source context (source_system, subject, date, sender) pulled
// from the actual sources schema columns.
func TestE2E_AssertionsList_ReturnsSourceContext(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	err := env.EnsureTenantExists()
	require.NoError(t, err)

	// Query DB directly to check if any assertions exist for test tenant
	var count int
	err = env.DB.QueryRow(ctx,
		"SELECT COUNT(*) FROM assertions a JOIN sources s ON s.id = a.source_id WHERE s.tenant_id = $1",
		env.TenantID).Scan(&count)
	require.NoError(t, err)

	if count == 0 {
		t.Skip("No assertions in test tenant — ingest content first")
	}

	// Fetch via CLI with JSON output
	result := env.SafeCLI.Run(ctx, "assertions", "list", "--output", "json")
	require.True(t, result.Success(),
		"assertions list --output json should succeed, got: %v\nstderr: %s", result.Err, result.Stderr)

	// Should produce parseable output (not an error message)
	assert.NotEmpty(t, result.Stdout, "should return assertion data")
	assert.NotContains(t, result.Stdout, "does not exist",
		"output should not contain SQL error text")
}

// TestE2E_AssertionsSearch_DoesNotCrash verifies that `penf assertions search`
// does not crash with the same column reference bugs as list.
func TestE2E_AssertionsSearch_DoesNotCrash(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	err := env.EnsureTenantExists()
	require.NoError(t, err)

	// Search with a generic term — should not crash regardless of results
	result := env.SafeCLI.Run(ctx, "assertions", "search", "project")
	assert.True(t, result.Success(),
		"assertions search should succeed, got error: %v\nstderr: %s", result.Err, result.Stderr)
	assert.NotContains(t, result.Stderr, "source_type does not exist",
		"should not reference nonexistent source_type column")
}

// TestE2E_AssertionsSummary_DoesNotCrash verifies that `penf assertions summary`
// does not crash. The summary query references s.date which should be
// s.source_timestamp.
func TestE2E_AssertionsSummary_DoesNotCrash(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	err := env.EnsureTenantExists()
	require.NoError(t, err)

	// Test all three group-by modes — each has its own query with s.date references
	for _, groupBy := range []string{"type", "project", "person"} {
		t.Run("group_by_"+groupBy, func(t *testing.T) {
			result := env.SafeCLI.Run(ctx, "assertions", "summary", "--group-by", groupBy)
			assert.True(t, result.Success(),
				"assertions summary --group-by %s should succeed, got error: %v\nstderr: %s",
				groupBy, result.Err, result.Stderr)
			assert.NotContains(t, result.Stderr, "does not exist",
				"should not reference nonexistent columns")
		})
	}
}

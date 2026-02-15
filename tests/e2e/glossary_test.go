//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCLI_GlossaryRemoveByID verifies that `penf glossary remove --id <id>` removes exactly that entry.
func TestCLI_GlossaryRemoveByID(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	// Cleanup before test
	err := env.CleanupTestTenant()
	require.NoError(t, err)

	err = env.EnsureTenantExists()
	require.NoError(t, err)

	// Step 1: Add a term
	uniqueTerm := fmt.Sprintf("TEST_TERM_%d", time.Now().UnixNano())
	addResult := env.CLI.Run(ctx, "glossary", "add", uniqueTerm, "Test Expansion")
	require.Equal(t, 0, addResult.ExitCode, "Failed to add term: %s", addResult.Stderr)
	t.Logf("Added term %q", uniqueTerm)

	// Step 1b: Get the term ID by listing terms and finding our newly added term
	listResult := env.CLI.Run(ctx, "glossary", "list", "--output", "json")
	require.Equal(t, 0, listResult.ExitCode, "Failed to list terms: %s", listResult.Stderr)

	var terms []struct {
		ID        int64  `json:"id"`
		Term      string `json:"term"`
		Expansion string `json:"expansion"`
	}
	err = json.Unmarshal([]byte(listResult.Stdout), &terms)
	require.NoError(t, err, "Failed to parse list terms JSON output")

	// Find our term in the list to get its ID
	var termID int64
	for _, term := range terms {
		if term.Term == uniqueTerm {
			termID = term.ID
			break
		}
	}
	require.NotZero(t, termID, "Failed to find term %q in list", uniqueTerm)
	t.Logf("Found term %q with ID %d", uniqueTerm, termID)

	// Step 2: Remove by ID
	removeResult := env.CLI.Run(ctx, "glossary", "remove", "--id", fmt.Sprintf("%d", termID))
	require.Equal(t, 0, removeResult.ExitCode, "Failed to remove term by ID: %s", removeResult.Stderr)

	t.Logf("Remove output: %s", removeResult.Stdout)

	// Step 3: Verify it's gone via list
	listAfterResult := env.CLI.Run(ctx, "glossary", "list", "--output", "json")
	require.Equal(t, 0, listAfterResult.ExitCode, "Failed to list terms after removal: %s", listAfterResult.Stderr)

	var termsAfter []struct {
		ID        int64  `json:"id"`
		Term      string `json:"term"`
		Expansion string `json:"expansion"`
	}
	err = json.Unmarshal([]byte(listAfterResult.Stdout), &termsAfter)
	require.NoError(t, err, "Failed to parse list terms JSON output after removal")

	// Verify the term is not in the list
	for _, term := range termsAfter {
		assert.NotEqual(t, termID, term.ID, "Term with ID %d should not exist after removal", termID)
	}

	// Step 4: Verify by trying to show the term (should fail)
	showResult := env.CLI.Run(ctx, "glossary", "show", uniqueTerm)
	assert.NotEqual(t, 0, showResult.ExitCode, "Show command should fail for removed term")
	assert.Contains(t, showResult.Stderr, "not found", "Error should mention term not found")
}

// TestCLI_GlossaryRemoveByID_NotFound verifies that `penf glossary remove --id 99999999` returns clear error.
func TestCLI_GlossaryRemoveByID_NotFound(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	// Cleanup before test
	err := env.CleanupTestTenant()
	require.NoError(t, err)

	err = env.EnsureTenantExists()
	require.NoError(t, err)

	// Try to remove a term with a non-existent ID
	nonExistentID := int64(99999999)
	removeResult := env.CLI.Run(ctx, "glossary", "remove", "--id", fmt.Sprintf("%d", nonExistentID))

	// Should fail with non-zero exit code
	assert.NotEqual(t, 0, removeResult.ExitCode, "Remove command should fail for non-existent ID")

	// Error message should be clear and mention "not found" or similar
	combinedOutput := removeResult.Stdout + removeResult.Stderr
	assert.True(t,
		strings.Contains(combinedOutput, "not found") ||
			strings.Contains(combinedOutput, "does not exist") ||
			strings.Contains(combinedOutput, "invalid"),
		"Error message should indicate ID not found. Got: %s", combinedOutput)

	t.Logf("Error output for non-existent ID: %s", combinedOutput)
}

// TestCLI_GlossaryAddDuplicate verifies that adding a duplicate term returns a clear error.
func TestCLI_GlossaryAddDuplicate(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	// Cleanup before test
	err := env.CleanupTestTenant()
	require.NoError(t, err)

	err = env.EnsureTenantExists()
	require.NoError(t, err)

	// Step 1: Add a term
	uniqueTerm := fmt.Sprintf("DUP_TEST_%d", time.Now().UnixNano())
	addResult := env.CLI.Run(ctx, "glossary", "add", uniqueTerm, "Original Expansion")
	require.Equal(t, 0, addResult.ExitCode, "Failed to add term: %s", addResult.Stderr)

	t.Logf("Added term: %s", uniqueTerm)

	// Step 2: Try to add the same term again (should fail)
	dupResult := env.CLI.Run(ctx, "glossary", "add", uniqueTerm, "Duplicate Expansion")

	// Should fail with non-zero exit code
	assert.NotEqual(t, 0, dupResult.ExitCode, "Adding duplicate term should fail")

	// Error message should mention "already exists" or "duplicate"
	combinedOutput := dupResult.Stdout + dupResult.Stderr
	assert.True(t,
		strings.Contains(combinedOutput, "already exists") ||
			strings.Contains(combinedOutput, "duplicate") ||
			strings.Contains(combinedOutput, "exists"),
		"Error message should mention duplicate or already exists. Got: %s", combinedOutput)

	t.Logf("Duplicate add error output: %s", combinedOutput)

	// Step 3: List terms and verify only one copy exists
	listResult := env.CLI.Run(ctx, "glossary", "list", "--output", "json")
	require.Equal(t, 0, listResult.ExitCode, "Failed to list terms: %s", listResult.Stderr)

	var terms []struct {
		ID        int64  `json:"id"`
		Term      string `json:"term"`
		Expansion string `json:"expansion"`
	}
	err = json.Unmarshal([]byte(listResult.Stdout), &terms)
	require.NoError(t, err, "Failed to parse list terms JSON output")

	// Count how many times our term appears
	count := 0
	for _, term := range terms {
		if term.Term == uniqueTerm {
			count++
			// Should have the original expansion
			assert.Equal(t, "Original Expansion", term.Expansion,
				"Term should keep original expansion")
		}
	}

	assert.Equal(t, 1, count, "Should have exactly one copy of the term, got %d", count)
}

// TestCLI_GlossaryRemoveByID_vs_ByName verifies both removal methods work and are independent.
func TestCLI_GlossaryRemoveByID_vs_ByName(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	// Cleanup before test
	err := env.CleanupTestTenant()
	require.NoError(t, err)

	err = env.EnsureTenantExists()
	require.NoError(t, err)

	// Add two terms
	term1 := fmt.Sprintf("TERM1_%d", time.Now().UnixNano())
	term2 := fmt.Sprintf("TERM2_%d", time.Now().UnixNano())

	add1Result := env.CLI.Run(ctx, "glossary", "add", term1, "Expansion 1")
	require.Equal(t, 0, add1Result.ExitCode, "Failed to add term1: %s", add1Result.Stderr)

	add2Result := env.CLI.Run(ctx, "glossary", "add", term2, "Expansion 2")
	require.Equal(t, 0, add2Result.ExitCode, "Failed to add term2: %s", add2Result.Stderr)

	// Get term1 ID from list
	listInitResult := env.CLI.Run(ctx, "glossary", "list", "--output", "json")
	require.Equal(t, 0, listInitResult.ExitCode, "Failed to list terms: %s", listInitResult.Stderr)

	var termsInit []struct {
		ID   int64  `json:"id"`
		Term string `json:"term"`
	}
	err = json.Unmarshal([]byte(listInitResult.Stdout), &termsInit)
	require.NoError(t, err)

	var term1ID int64
	for _, term := range termsInit {
		if term.Term == term1 {
			term1ID = term.ID
			break
		}
	}
	require.NotZero(t, term1ID, "Failed to find term1 ID")

	// Remove term1 by ID
	removeByIDResult := env.CLI.Run(ctx, "glossary", "remove", "--id", fmt.Sprintf("%d", term1ID))
	require.Equal(t, 0, removeByIDResult.ExitCode, "Failed to remove by ID: %s", removeByIDResult.Stderr)

	// Remove term2 by name
	removeByNameResult := env.CLI.Run(ctx, "glossary", "remove", term2)
	require.Equal(t, 0, removeByNameResult.ExitCode, "Failed to remove by name: %s", removeByNameResult.Stderr)

	// Verify both are gone
	listResult := env.CLI.Run(ctx, "glossary", "list", "--output", "json")
	require.Equal(t, 0, listResult.ExitCode, "Failed to list terms: %s", listResult.Stderr)

	var termsAfter []struct {
		ID   int64  `json:"id"`
		Term string `json:"term"`
	}
	err = json.Unmarshal([]byte(listResult.Stdout), &termsAfter)
	require.NoError(t, err)

	for _, term := range termsAfter {
		assert.NotEqual(t, term1, term.Term, "term1 should be removed")
		assert.NotEqual(t, term2, term.Term, "term2 should be removed")
		assert.NotEqual(t, term1ID, term.ID, "term1 ID should be removed")
	}
}

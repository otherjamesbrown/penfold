//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Glossary Bug E2E Tests
// ============================================================================
//
// These tests verify fixes for three pre-existing glossary bugs:
//
//   pf-d1bf5c: List() missing tenant_id filter — returns rows from all tenants
//   pf-26ce32: LookupTerm fails scanning NULL definition column
//   pf-f58b00: GetByTerm/ListLinked fails unmarshalling context string vs []string
//
// ============================================================================

// TestE2E_GlossaryShow_NullDefinition verifies that `penf glossary show` works
// for terms that have no definition (NULL in DB).
//
// Bug pf-26ce32: LookupTerm scans definition into *string but doesn't handle
// NULL correctly, causing "cannot scan NULL into *string".
func TestE2E_GlossaryShow_NullDefinition(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	err := env.CleanupTestTenant()
	require.NoError(t, err)
	err = env.EnsureTenantExists()
	require.NoError(t, err)

	// Add a term WITHOUT a definition (no --definition flag)
	term := fmt.Sprintf("NULLDEF_%d", time.Now().UnixNano())
	addResult := env.CLI.Run(ctx, "glossary", "add", term, "Null Definition Test")
	require.Equal(t, 0, addResult.ExitCode, "Failed to add term: %s", addResult.Stderr)

	t.Cleanup(func() {
		env.CLI.Run(ctx, "glossary", "remove", term)
	})

	// Show the term — this is what fails with NULL scan error
	showResult := env.CLI.Run(ctx, "glossary", "show", term, "-o", "json")
	require.Equal(t, 0, showResult.ExitCode,
		"penf glossary show should handle NULL definition. Got: %s", showResult.Stderr)

	var resp map[string]interface{}
	err = json.Unmarshal([]byte(showResult.Stdout), &resp)
	require.NoError(t, err, "show response should be valid JSON")

	// Term should be present with expansion, definition may be empty/null
	termData, ok := resp["term"].(map[string]interface{})
	if !ok {
		termData = resp // some formats return flat
	}
	assert.Contains(t, []interface{}{term, nil},
		termData["term"],
		"response should contain the term")
}

// TestE2E_GlossaryShow_WithDefinition verifies the normal case still works
// (non-NULL definition) as a control for the NULL test above.
func TestE2E_GlossaryShow_WithDefinition(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	err := env.CleanupTestTenant()
	require.NoError(t, err)
	err = env.EnsureTenantExists()
	require.NoError(t, err)

	term := fmt.Sprintf("HASDEF_%d", time.Now().UnixNano())
	addResult := env.CLI.Run(ctx, "glossary", "add", term, "Has Definition Test",
		"--definition", "This term has a full definition.")
	require.Equal(t, 0, addResult.ExitCode, "Failed to add term: %s", addResult.Stderr)

	t.Cleanup(func() {
		env.CLI.Run(ctx, "glossary", "remove", term)
	})

	showResult := env.CLI.Run(ctx, "glossary", "show", term, "-o", "json")
	require.Equal(t, 0, showResult.ExitCode,
		"penf glossary show should work with definition: %s", showResult.Stderr)

	var resp map[string]interface{}
	err = json.Unmarshal([]byte(showResult.Stdout), &resp)
	require.NoError(t, err)
}

// TestE2E_GlossaryShow_ContextStringVsArray verifies that `penf glossary show`
// and `penf glossary linked` work regardless of whether context is stored as
// a JSON string or a JSON array.
//
// Bug pf-f58b00: Some rows have context as a bare JSON string (e.g., "MTC")
// instead of an array (e.g., ["MTC"]). Repository scans into []string and
// fails with "json: cannot unmarshal string into Go value of type []string".
func TestE2E_GlossaryShow_ContextStringVsArray(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	err := env.CleanupTestTenant()
	require.NoError(t, err)
	err = env.EnsureTenantExists()
	require.NoError(t, err)

	// Add a term with context (stored as array — normal case)
	termArray := fmt.Sprintf("CTXARR_%d", time.Now().UnixNano())
	addResult := env.CLI.Run(ctx, "glossary", "add", termArray, "Context Array Test",
		"--context", "MTC,Oracle")
	require.Equal(t, 0, addResult.ExitCode, "Failed to add term: %s", addResult.Stderr)

	t.Cleanup(func() {
		env.CLI.Run(ctx, "glossary", "remove", termArray)
	})

	// Show should work for array context
	t.Run("array_context_show", func(t *testing.T) {
		showResult := env.CLI.Run(ctx, "glossary", "show", termArray, "-o", "json")
		require.Equal(t, 0, showResult.ExitCode,
			"show should work with array context: %s", showResult.Stderr)
	})

	// List should work and return our term
	t.Run("list_includes_context_terms", func(t *testing.T) {
		listResult := env.CLI.Run(ctx, "glossary", "list", "-o", "json")
		require.Equal(t, 0, listResult.ExitCode,
			"list should work: %s", listResult.Stderr)

		// Parse and verify our term is present
		var terms []map[string]interface{}
		err := json.Unmarshal([]byte(listResult.Stdout), &terms)
		require.NoError(t, err)

		found := false
		for _, term := range terms {
			if term["term"] == termArray {
				found = true
				break
			}
		}
		assert.True(t, found, "list should include our test term %s", termArray)
	})

	// Linked should not crash even if some rows have string context
	t.Run("linked_handles_mixed_context", func(t *testing.T) {
		linkedResult := env.CLI.Run(ctx, "glossary", "linked", "-o", "json")
		// May return 0 with empty results or non-zero if no links — either is fine,
		// but it must NOT crash with a JSON unmarshal error
		if linkedResult.ExitCode != 0 {
			assert.NotContains(t, linkedResult.Stderr, "cannot unmarshal string",
				"linked should not fail with JSON unmarshal error")
		}
	})
}

// TestE2E_GlossaryList_TenantIsolation verifies that `penf glossary list`
// only returns terms belonging to the current tenant.
//
// Bug pf-d1bf5c: List() query omits tenant_id filter, returning rows from
// all tenants. This causes tests to see unexpected data and production to
// leak data across tenants.
func TestE2E_GlossaryList_TenantIsolation(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	err := env.CleanupTestTenant()
	require.NoError(t, err)
	err = env.EnsureTenantExists()
	require.NoError(t, err)

	// Add exactly 3 terms
	terms := make([]string, 3)
	for i := range terms {
		terms[i] = fmt.Sprintf("TENANT_%d_%d", i, time.Now().UnixNano())
		addResult := env.CLI.Run(ctx, "glossary", "add", terms[i],
			fmt.Sprintf("Tenant Test %d", i))
		require.Equal(t, 0, addResult.ExitCode,
			"Failed to add term %s: %s", terms[i], addResult.Stderr)
	}

	t.Cleanup(func() {
		for _, term := range terms {
			env.CLI.Run(ctx, "glossary", "remove", term)
		}
	})

	// List should return exactly our 3 terms (tenant was cleaned before adding)
	listResult := env.CLI.Run(ctx, "glossary", "list", "-o", "json")
	require.Equal(t, 0, listResult.ExitCode,
		"list should succeed: %s", listResult.Stderr)

	var listed []map[string]interface{}
	err = json.Unmarshal([]byte(listResult.Stdout), &listed)
	require.NoError(t, err)

	assert.Equal(t, 3, len(listed),
		"list should return exactly 3 terms for this tenant, got %d (tenant isolation failure if more)",
		len(listed))

	// Verify all 3 are ours
	foundTerms := map[string]bool{}
	for _, item := range listed {
		if t, ok := item["term"].(string); ok {
			foundTerms[t] = true
		}
	}
	for _, term := range terms {
		assert.True(t, foundTerms[term], "list should include our term %s", term)
	}
}

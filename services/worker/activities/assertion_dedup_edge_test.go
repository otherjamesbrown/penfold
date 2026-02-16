//go:build integration

package activities

import (
	"context"
	"testing"

	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStoreAssertions_SameDescription_DifferentQuote_BugRepro reproduces the
// actual bug: the unique constraint is on (source_id, assertion_type, description)
// but it SHOULD be on (source_id, assertion_type, source_quote).
//
// This test inserts two assertions with:
// - Same source_id
// - Same assertion_type
// - Same description (SPO triple)
// - DIFFERENT source_quote (context)
//
// Current behavior (BUG):
//   The second insert will be rejected as a duplicate because the unique
//   constraint only checks description, not source_quote.
//
// Expected behavior (AFTER FIX):
//   Both assertions should be created because they have different source_quotes.
//   The same decision/action/risk can appear in different parts of an email
//   with different context, and we want to capture both references.
func TestStoreAssertions_SameDescription_DifferentQuote_BugRepro(t *testing.T) {
	ctx := context.Background()

	// Setup database connection
	pool := setupTestDB(t)
	defer pool.Close()

	tenantID := IntegrationTestTenantID
	logger := logging.MustGlobal()

	// Create repository
	repo := NewPostgresAssertionRepository(pool, logger)

	// Create a test source record
	testSourceID := createTestSource(t, ctx, pool, tenantID)
	defer cleanupTestSource(t, ctx, pool, testSourceID)

	// Two assertions with the SAME description but DIFFERENT source_quotes
	// This represents the same action mentioned in two different parts of the email
	assertion1 := []*Assertion{
		{
			Subject:    "Budget",
			Predicate:  "is",
			Object:     "approved",
			Confidence: 0.90,
			// First mention in email header
			SourceText:   "In the meeting, we discussed the budget and it is approved",
			Category:     "decision",
			Attributions: []Attribution{},
		},
	}

	assertion2 := []*Assertion{
		{
			Subject:    "Budget",
			Predicate:  "is",
			Object:     "approved",
			Confidence: 0.95,
			// Second mention in email footer
			SourceText:   "Just to confirm: the budget is approved, moving forward",
			Category:     "decision",
			Attributions: []Attribution{},
		},
	}

	// Insert first assertion
	count1, err := repo.StoreAssertions(ctx, tenantID, testSourceID, assertion1, "test-model-v1")
	require.NoError(t, err, "First insert should succeed")
	t.Logf("First insert: %d assertions created", count1)

	// Insert second assertion (same description, different source_quote)
	count2, err := repo.StoreAssertions(ctx, tenantID, testSourceID, assertion2, "test-model-v1")
	require.NoError(t, err, "Second insert should succeed")
	t.Logf("Second insert: %d assertions created", count2)

	// Query the database to see what we actually have
	var totalCount int
	countQuery := `
		SELECT COUNT(*)
		FROM assertions
		WHERE tenant_id = $1::uuid AND source_id = $2
	`
	err = pool.QueryRow(ctx, countQuery, tenantID, testSourceID).Scan(&totalCount)
	require.NoError(t, err)
	t.Logf("Total assertions in DB: %d", totalCount)

	// Query to see the actual assertions
	detailQuery := `
		SELECT id, description, source_quote
		FROM assertions
		WHERE tenant_id = $1::uuid AND source_id = $2
		ORDER BY id
	`
	rows, err := pool.Query(ctx, detailQuery, tenantID, testSourceID)
	require.NoError(t, err)
	defer rows.Close()

	var assertions []struct {
		ID          int64
		Description string
		SourceQuote string
	}
	for rows.Next() {
		var a struct {
			ID          int64
			Description string
			SourceQuote string
		}
		err := rows.Scan(&a.ID, &a.Description, &a.SourceQuote)
		require.NoError(t, err)
		assertions = append(assertions, a)
		t.Logf("Assertion %d: description=%q, source_quote=%q",
			a.ID, a.Description, a.SourceQuote)
	}

	// BUG: Current unique constraint is on (source_id, assertion_type, description)
	// So the second insert will be treated as a duplicate even though source_quote differs.
	// This test will FAIL until the constraint is changed to include source_quote.

	if count2 == 0 {
		t.Errorf("BUG REPRODUCED: Second assertion was rejected as duplicate")
		t.Errorf("Current unique constraint uses description, not source_quote")
		t.Errorf("Expected: 2 assertions (different context quotes)")
		t.Errorf("Actual: %d assertions (second insert returned count=0)", totalCount)
	}

	// After fix, we should have 2 assertions with the same description but different quotes
	assert.Equal(t, 1, count1, "First insert should create 1 assertion")
	assert.Equal(t, 1, count2, "Second insert should create 1 assertion (different source_quote)")
	assert.Equal(t, 2, totalCount, "Should have 2 assertions with different source_quotes")

	// Verify both source_quotes are present
	if len(assertions) >= 1 {
		assert.Contains(t,
			[]string{assertions[0].SourceQuote},
			"In the meeting, we discussed the budget and it is approved",
			"Should have first source_quote")
	}
	if len(assertions) >= 2 {
		assert.Contains(t,
			[]string{assertions[1].SourceQuote},
			"Just to confirm: the budget is approved, moving forward",
			"Should have second source_quote")
	}
}

// TestStoreAssertions_RealWorldScenario_EmPgUz3mLM simulates the real bug case:
// Reprocessing em-PgUz3mLM should not create duplicate assertions.
//
// The bug manifests when:
// 1. Email is processed once -> creates assertions
// 2. Email is reprocessed -> should NOT create duplicates
//
// The dedup key should be: (source_id, assertion_type, source_quote)
// NOT: (source_id, assertion_type, description)
func TestStoreAssertions_RealWorldScenario_EmPgUz3mLM(t *testing.T) {
	ctx := context.Background()

	// Setup database connection
	pool := setupTestDB(t)
	defer pool.Close()

	tenantID := IntegrationTestTenantID
	logger := logging.MustGlobal()

	// Create repository
	repo := NewPostgresAssertionRepository(pool, logger)

	// Create a test source record representing em-PgUz3mLM
	testSourceID := createTestSource(t, ctx, pool, tenantID)
	defer cleanupTestSource(t, ctx, pool, testSourceID)

	// Simulate assertions extracted from em-PgUz3mLM (Tim's reply)
	// These would be extracted during normal processing
	assertionsFromProcessing := []*Assertion{
		{
			Subject:      "Tim",
			Predicate:    "replied to",
			Object:       "proposal",
			Confidence:   0.85,
			SourceText:   "Tim replied with concerns about the timeline",
			Category:     "issue",
			Attributions: []Attribution{},
		},
		{
			Subject:      "Timeline",
			Predicate:    "is at",
			Object:       "risk",
			Confidence:   0.80,
			SourceText:   "The timeline is at risk due to resource constraints",
			Category:     "risk",
			Attributions: []Attribution{},
		},
	}

	// FIRST PROCESSING: Extract and store assertions
	t.Log("=== FIRST PROCESSING ===")
	count1, err := repo.StoreAssertions(ctx, tenantID, testSourceID, assertionsFromProcessing, "gpt-4o")
	require.NoError(t, err, "First processing should succeed")
	t.Logf("Assertions created: %d", count1)
	assert.Equal(t, 2, count1, "First processing should create 2 assertions")

	// REPROCESSING: User runs `penf reprocess em-PgUz3mLM`
	// The AI extracts the SAME assertions again (same content, same quotes)
	t.Log("=== REPROCESSING (user runs: penf reprocess em-PgUz3mLM) ===")
	count2, err := repo.StoreAssertions(ctx, tenantID, testSourceID, assertionsFromProcessing, "gpt-4o")
	require.NoError(t, err, "Reprocessing should succeed")
	t.Logf("Assertions created: %d", count2)

	// On reprocessing, count should be 0 (all duplicates)
	assert.Equal(t, 0, count2, "Reprocessing should create 0 assertions (all are duplicates)")

	// Verify total count
	var totalCount int
	countQuery := `
		SELECT COUNT(*)
		FROM assertions
		WHERE tenant_id = $1::uuid AND source_id = $2
	`
	err = pool.QueryRow(ctx, countQuery, tenantID, testSourceID).Scan(&totalCount)
	require.NoError(t, err)
	t.Logf("Total assertions in DB: %d", totalCount)

	// Should have exactly 2 assertions (no duplicates from reprocessing)
	assert.Equal(t, 2, totalCount, "Should have exactly 2 assertions (no duplicates)")

	// Verify no duplicate tuples exist
	duplicateQuery := `
		SELECT assertion_type, description, source_quote, COUNT(*) as count
		FROM assertions
		WHERE tenant_id = $1::uuid AND source_id = $2
		GROUP BY assertion_type, description, source_quote
		HAVING COUNT(*) > 1
	`
	rows, err := pool.Query(ctx, duplicateQuery, tenantID, testSourceID)
	require.NoError(t, err)
	defer rows.Close()

	var duplicateFound bool
	for rows.Next() {
		var assertionType, description, sourceQuote string
		var count int
		err := rows.Scan(&assertionType, &description, &sourceQuote, &count)
		require.NoError(t, err)
		t.Errorf("DUPLICATE FOUND: type=%s, desc=%s, quote=%s, count=%d",
			assertionType, description, sourceQuote, count)
		duplicateFound = true
	}

	assert.False(t, duplicateFound, "Should have no duplicate (type, description, quote) tuples")
}

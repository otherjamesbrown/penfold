//go:build integration

package activities

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testRunTenantID is generated once per test binary execution.
// Using a per-run UUID prevents data collisions when multiple developers or
// CI pipelines run integration tests concurrently against the shared live database.
var testRunTenantID = uuid.New().String()

// TestStoreAssertions_Duplicates_Integration reproduces bug pf-ab90cb:
// StoreAssertions uses INSERT with ON CONFLICT DO NOTHING but no unique constraint
// exists on assertions table, so duplicates are never detected.
//
// This test:
// 1. Calls StoreAssertions twice with the same assertions for the same source_id
// 2. Queries the assertions table to count duplicates
// 3. Asserts that only one copy of each assertion exists (no duplicates)
// 4. This test should FAIL because without a unique constraint, duplicates ARE inserted
//
// Example: em-goTacbq8 has "Rishi pushed back" appearing 10 times.
//
// After the bug is fixed (by adding a unique constraint), this test should PASS.
func TestStoreAssertions_Duplicates_Integration(t *testing.T) {
	ctx := context.Background()

	// Setup database connection
	pool := testutil.OpenDB(t)
	defer pool.Close()

	tenantID := testRunTenantID
	logger := logging.MustGlobal()

	// Create repository
	repo := NewPostgresAssertionRepository(pool, logger)

	// Create a test source record (required for foreign key constraint)
	testSourceID := createTestSource(t, ctx, pool, tenantID)

	// Clean up: delete test source (cascades to assertions)
	defer cleanupTestSource(t, ctx, pool, testSourceID)

	// Create test assertions
	assertions := []*Assertion{
		{
			Subject:      "Rishi",
			Predicate:    "pushed back",
			Object:       "on timeline",
			Confidence:   0.85,
			SourceText:   "Rishi pushed back on the aggressive timeline",
			Category:     "risk",
			Attributions: []Attribution{},
		},
		{
			Subject:      "Team",
			Predicate:    "needs",
			Object:       "more resources",
			Confidence:   0.90,
			SourceText:   "The team needs more resources to complete this",
			Category:     "issue",
			Attributions: []Attribution{},
		},
	}

	// STEP 1: Insert assertions first time
	count1, err := repo.StoreAssertions(ctx, tenantID, testSourceID, assertions, "test-model-v1")
	require.NoError(t, err, "First StoreAssertions call should succeed")
	assert.Equal(t, 2, count1, "Should insert 2 assertions on first call")

	// STEP 2: Insert the SAME assertions again
	count2, err := repo.StoreAssertions(ctx, tenantID, testSourceID, assertions, "test-model-v1")
	require.NoError(t, err, "Second StoreAssertions call should succeed")

	// With ON CONFLICT DO NOTHING working correctly, count2 should be 0
	// But without a unique constraint, count2 will be 2 (duplicates inserted)
	t.Logf("First insert: %d assertions, Second insert: %d assertions", count1, count2)

	// STEP 3: Query the database to count total assertions for this source
	query := `
		SELECT COUNT(*)
		FROM assertions
		WHERE tenant_id = $1::uuid AND source_id = $2
	`
	var totalCount int
	err = pool.QueryRow(ctx, query, tenantID, testSourceID).Scan(&totalCount)
	require.NoError(t, err, "Should be able to query assertion count")

	t.Logf("Total assertions in database for source %d: %d", testSourceID, totalCount)

	// STEP 4: Check for duplicates by description
	duplicateQuery := `
		SELECT description, COUNT(*) as count
		FROM assertions
		WHERE tenant_id = $1::uuid AND source_id = $2
		GROUP BY description
		HAVING COUNT(*) > 1
	`
	rows, err := pool.Query(ctx, duplicateQuery, tenantID, testSourceID)
	require.NoError(t, err, "Should be able to query for duplicates")
	defer rows.Close()

	duplicates := make(map[string]int)
	for rows.Next() {
		var description string
		var count int
		err := rows.Scan(&description, &count)
		require.NoError(t, err)
		duplicates[description] = count
	}

	// ASSERT: There should be NO duplicates (each assertion should appear exactly once)
	if len(duplicates) > 0 {
		t.Errorf("BUG REPRODUCED: Found duplicate assertions:")
		for desc, count := range duplicates {
			t.Errorf("  - %q appears %d times (expected 1)", desc, count)
		}
		t.Errorf("Total assertions: %d (expected 2)", totalCount)
		t.Errorf("This happens because assertions table has no unique constraint for ON CONFLICT DO NOTHING to match against")
	} else {
		t.Logf("PASS: No duplicates found. Each assertion appears exactly once.")
	}

	// The test MUST fail if duplicates exist
	assert.Equal(t, 2, totalCount, "Should have exactly 2 assertions (no duplicates)")
	assert.Empty(t, duplicates, "Should have no duplicate assertions")
}

// createTestSource creates a test source record in the database.
func createTestSource(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID string) int64 {
	t.Helper()

	var sourceID int64
	query := `
		INSERT INTO sources (
			tenant_id,
			source_system,
			external_id,
			content_hash,
			content_type,
			processing_status
		) VALUES (
			$1::uuid,
			'gmail',
			'test-assertion-duplicates-' || gen_random_uuid()::text,
			encode(sha256('test content'::bytea), 'hex'),
			'email',
			'completed'
		)
		RETURNING id
	`
	err := pool.QueryRow(ctx, query, tenantID).Scan(&sourceID)
	require.NoError(t, err, "Should be able to create test source")
	t.Logf("Created test source with ID: %d", sourceID)
	return sourceID
}

// cleanupTestSource removes a test source and its assertions.
func cleanupTestSource(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sourceID int64) {
	t.Helper()

	// Delete assertions first (foreign key constraint)
	_, err := pool.Exec(ctx, `DELETE FROM assertions WHERE source_id = $1`, sourceID)
	if err != nil {
		t.Logf("Warning: failed to cleanup assertions: %v", err)
	}

	// Then delete the source
	query := `DELETE FROM sources WHERE id = $1`
	_, err = pool.Exec(ctx, query, sourceID)
	if err != nil {
		t.Logf("Warning: failed to cleanup test source: %v", err)
	}
}


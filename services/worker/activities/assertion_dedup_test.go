//go:build integration

package activities

import (
	"context"
	"testing"

	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStoreAssertions_PreventsDuplicates_ExactMatch tests that reprocessing
// the same email does NOT create duplicate assertions when the source_id,
// assertion_type, and source_quote are identical.
//
// This maps to acceptance criterion:
// - [ ] Reprocess em-PgUz3mLM (Tim reply) — no duplicate assertion pairs created
// - [ ] Reprocessing any email does not create duplicate assertions
//
// Expected behavior:
// - First insert: 2 assertions created
// - Second insert (identical data): 0 assertions created (deduplicated)
// - Total in DB: 2 assertions (no duplicates)
func TestStoreAssertions_PreventsDuplicates_ExactMatch(t *testing.T) {
	ctx := context.Background()

	// Setup database connection
	pool := testutil.OpenDB(t)
	defer pool.Close()

	tenantID := testRunTenantID
	logger := logging.MustGlobal()

	// Create repository
	repo := NewPostgresAssertionRepository(pool, logger)

	// Create a test source record
	testSourceID := createTestSource(t, ctx, pool, tenantID)
	defer cleanupTestSource(t, ctx, pool, testSourceID)

	// Create test assertions - these represent what would come from reprocessing
	// the same email multiple times
	assertions := []*Assertion{
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
			Subject:      "Budget",
			Predicate:    "is",
			Object:       "approved",
			Confidence:   0.90,
			SourceText:   "The budget has been approved by finance",
			Category:     "decision",
			Attributions: []Attribution{},
		},
	}

	// STEP 1: Insert assertions first time (original processing)
	count1, err := repo.StoreAssertions(ctx, tenantID, testSourceID, assertions, "test-model-v1")
	require.NoError(t, err, "First StoreAssertions call should succeed")
	assert.Equal(t, 2, count1, "Should insert 2 assertions on first call")

	// STEP 2: Reprocess - insert the SAME assertions again (same source_id, same data)
	count2, err := repo.StoreAssertions(ctx, tenantID, testSourceID, assertions, "test-model-v1")
	require.NoError(t, err, "Second StoreAssertions call should succeed")

	// With proper deduplication, count2 should be 0 (all were duplicates)
	assert.Equal(t, 0, count2, "Second insert should create 0 assertions (all duplicates)")

	// STEP 3: Verify total count in database
	var totalCount int
	countQuery := `
		SELECT COUNT(*)
		FROM assertions
		WHERE tenant_id = $1::uuid AND source_id = $2
	`
	err = pool.QueryRow(ctx, countQuery, tenantID, testSourceID).Scan(&totalCount)
	require.NoError(t, err, "Should be able to query assertion count")

	// Should have exactly 2 assertions (no duplicates created on second insert)
	assert.Equal(t, 2, totalCount, "Should have exactly 2 assertions in database (no duplicates)")

	// STEP 4: Verify no duplicate pairs exist
	duplicateQuery := `
		SELECT assertion_type, description, source_quote, COUNT(*) as count
		FROM assertions
		WHERE tenant_id = $1::uuid AND source_id = $2
		GROUP BY assertion_type, description, source_quote
		HAVING COUNT(*) > 1
	`
	rows, err := pool.Query(ctx, duplicateQuery, tenantID, testSourceID)
	require.NoError(t, err, "Should be able to query for duplicates")
	defer rows.Close()

	var duplicateFound bool
	for rows.Next() {
		var assertionType, description, sourceQuote string
		var count int
		err := rows.Scan(&assertionType, &description, &sourceQuote, &count)
		require.NoError(t, err)
		t.Errorf("Found duplicate: type=%s, desc=%s, quote=%s, count=%d",
			assertionType, description, sourceQuote, count)
		duplicateFound = true
	}

	assert.False(t, duplicateFound, "Should have no duplicate assertion tuples")
}

// TestStoreAssertions_DifferentTypeAllowed tests that assertions with the
// same source_id and source_quote but different assertion_types are NOT
// considered duplicates.
//
// This ensures the dedup key includes assertion_type.
//
// Expected behavior:
// - Insert assertion with type "risk"
// - Insert assertion with same source_quote but type "issue"
// - Both should be created (total: 2)
func TestStoreAssertions_DifferentTypeAllowed(t *testing.T) {
	ctx := context.Background()

	// Setup database connection
	pool := testutil.OpenDB(t)
	defer pool.Close()

	tenantID := testRunTenantID
	logger := logging.MustGlobal()

	// Create repository
	repo := NewPostgresAssertionRepository(pool, logger)

	// Create a test source record
	testSourceID := createTestSource(t, ctx, pool, tenantID)
	defer cleanupTestSource(t, ctx, pool, testSourceID)

	// Same source_quote, different categories (which map to different types)
	assertion1 := []*Assertion{
		{
			Subject:      "Timeline",
			Predicate:    "is at",
			Object:       "risk",
			Confidence:   0.85,
			SourceText:   "The timeline is at risk due to resource constraints",
			Category:     "risk", // maps to type "risk"
			Attributions: []Attribution{},
		},
	}

	assertion2 := []*Assertion{
		{
			Subject:      "Timeline",
			Predicate:    "is at",
			Object:       "risk",
			Confidence:   0.85,
			SourceText:   "The timeline is at risk due to resource constraints", // SAME source_quote
			Category:     "issue", // maps to type "issue" - DIFFERENT type
			Attributions: []Attribution{},
		},
	}

	// Insert first assertion (type "risk")
	count1, err := repo.StoreAssertions(ctx, tenantID, testSourceID, assertion1, "test-model-v1")
	require.NoError(t, err, "First insert should succeed")
	assert.Equal(t, 1, count1, "Should insert 1 assertion")

	// Insert second assertion (type "issue", same source_quote)
	count2, err := repo.StoreAssertions(ctx, tenantID, testSourceID, assertion2, "test-model-v1")
	require.NoError(t, err, "Second insert should succeed")
	assert.Equal(t, 1, count2, "Should insert 1 assertion (different type, not a duplicate)")

	// Verify total count
	var totalCount int
	countQuery := `
		SELECT COUNT(*)
		FROM assertions
		WHERE tenant_id = $1::uuid AND source_id = $2
	`
	err = pool.QueryRow(ctx, countQuery, tenantID, testSourceID).Scan(&totalCount)
	require.NoError(t, err)

	assert.Equal(t, 2, totalCount, "Should have 2 assertions (different types allowed)")

	// Verify we have one of each type
	typeQuery := `
		SELECT assertion_type, COUNT(*) as count
		FROM assertions
		WHERE tenant_id = $1::uuid AND source_id = $2
		GROUP BY assertion_type
		ORDER BY assertion_type
	`
	rows, err := pool.Query(ctx, typeQuery, tenantID, testSourceID)
	require.NoError(t, err)
	defer rows.Close()

	typeCounts := make(map[string]int)
	for rows.Next() {
		var assertionType string
		var count int
		err := rows.Scan(&assertionType, &count)
		require.NoError(t, err)
		typeCounts[assertionType] = count
	}

	assert.Equal(t, 1, typeCounts["risk"], "Should have 1 risk assertion")
	assert.Equal(t, 1, typeCounts["issue"], "Should have 1 issue assertion")
}

// TestStoreAssertions_DifferentQuoteAllowed tests that assertions with the
// same source_id and assertion_type but different source_quotes are NOT
// considered duplicates.
//
// This ensures the dedup key includes source_quote.
//
// Expected behavior:
// - Insert assertion with quote A
// - Insert assertion with quote B (same type)
// - Both should be created (total: 2)
func TestStoreAssertions_DifferentQuoteAllowed(t *testing.T) {
	ctx := context.Background()

	// Setup database connection
	pool := testutil.OpenDB(t)
	defer pool.Close()

	tenantID := testRunTenantID
	logger := logging.MustGlobal()

	// Create repository
	repo := NewPostgresAssertionRepository(pool, logger)

	// Create a test source record
	testSourceID := createTestSource(t, ctx, pool, tenantID)
	defer cleanupTestSource(t, ctx, pool, testSourceID)

	// Same type, different source_quotes
	assertion1 := []*Assertion{
		{
			Subject:      "Budget",
			Predicate:    "is",
			Object:       "approved",
			Confidence:   0.90,
			SourceText:   "The budget has been approved by finance", // Quote A
			Category:     "decision",
			Attributions: []Attribution{},
		},
	}

	assertion2 := []*Assertion{
		{
			Subject:      "Timeline",
			Predicate:    "is",
			Object:       "extended",
			Confidence:   0.85,
			SourceText:   "The timeline has been extended by two weeks", // Quote B - DIFFERENT
			Category:     "decision", // Same type
			Attributions: []Attribution{},
		},
	}

	// Insert first assertion
	count1, err := repo.StoreAssertions(ctx, tenantID, testSourceID, assertion1, "test-model-v1")
	require.NoError(t, err, "First insert should succeed")
	assert.Equal(t, 1, count1, "Should insert 1 assertion")

	// Insert second assertion (same type, different source_quote)
	count2, err := repo.StoreAssertions(ctx, tenantID, testSourceID, assertion2, "test-model-v1")
	require.NoError(t, err, "Second insert should succeed")
	assert.Equal(t, 1, count2, "Should insert 1 assertion (different quote, not a duplicate)")

	// Verify total count
	var totalCount int
	countQuery := `
		SELECT COUNT(*)
		FROM assertions
		WHERE tenant_id = $1::uuid AND source_id = $2
	`
	err = pool.QueryRow(ctx, countQuery, tenantID, testSourceID).Scan(&totalCount)
	require.NoError(t, err)

	assert.Equal(t, 2, totalCount, "Should have 2 assertions (different quotes allowed)")

	// Verify we have both distinct source_quotes
	quoteQuery := `
		SELECT DISTINCT source_quote
		FROM assertions
		WHERE tenant_id = $1::uuid AND source_id = $2
		ORDER BY source_quote
	`
	rows, err := pool.Query(ctx, quoteQuery, tenantID, testSourceID)
	require.NoError(t, err)
	defer rows.Close()

	var quotes []string
	for rows.Next() {
		var quote string
		err := rows.Scan(&quote)
		require.NoError(t, err)
		quotes = append(quotes, quote)
	}

	assert.Len(t, quotes, 2, "Should have 2 distinct source_quotes")
	assert.Contains(t, quotes, "The budget has been approved by finance")
	assert.Contains(t, quotes, "The timeline has been extended by two weeks")
}

// TestStoreAssertions_FirstTimeCreates tests that processing an email for
// the first time correctly creates assertions.
//
// This maps to acceptance criterion:
// - [ ] First-time processing still creates assertions normally
//
// Expected behavior:
// - Insert new assertions for a new source_id
// - All assertions should be created
func TestStoreAssertions_FirstTimeCreates(t *testing.T) {
	ctx := context.Background()

	// Setup database connection
	pool := testutil.OpenDB(t)
	defer pool.Close()

	tenantID := testRunTenantID
	logger := logging.MustGlobal()

	// Create repository
	repo := NewPostgresAssertionRepository(pool, logger)

	// Create a test source record
	testSourceID := createTestSource(t, ctx, pool, tenantID)
	defer cleanupTestSource(t, ctx, pool, testSourceID)

	// Create brand new assertions
	assertions := []*Assertion{
		{
			Subject:      "New Feature",
			Predicate:    "requires",
			Object:       "approval",
			Confidence:   0.92,
			SourceText:   "The new feature requires approval from product",
			Category:     "action",
			Attributions: []Attribution{},
		},
		{
			Subject:      "Database",
			Predicate:    "needs",
			Object:       "migration",
			Confidence:   0.88,
			SourceText:   "The database needs migration before deploy",
			Category:     "risk",
			Attributions: []Attribution{},
		},
		{
			Subject:      "Release",
			Predicate:    "is scheduled for",
			Object:       "Friday",
			Confidence:   0.95,
			SourceText:   "The release is scheduled for Friday afternoon",
			Category:     "milestone",
			Attributions: []Attribution{},
		},
	}

	// Insert assertions for the first time
	count, err := repo.StoreAssertions(ctx, tenantID, testSourceID, assertions, "test-model-v1")
	require.NoError(t, err, "StoreAssertions should succeed on first insert")
	assert.Equal(t, 3, count, "Should insert all 3 assertions on first processing")

	// Verify all assertions are in the database
	var totalCount int
	countQuery := `
		SELECT COUNT(*)
		FROM assertions
		WHERE tenant_id = $1::uuid AND source_id = $2
	`
	err = pool.QueryRow(ctx, countQuery, tenantID, testSourceID).Scan(&totalCount)
	require.NoError(t, err)

	assert.Equal(t, 3, totalCount, "Should have exactly 3 assertions in database")

	// Verify all expected types are present
	typeQuery := `
		SELECT assertion_type
		FROM assertions
		WHERE tenant_id = $1::uuid AND source_id = $2
		ORDER BY assertion_type
	`
	rows, err := pool.Query(ctx, typeQuery, tenantID, testSourceID)
	require.NoError(t, err)
	defer rows.Close()

	var types []string
	for rows.Next() {
		var assertionType string
		err := rows.Scan(&assertionType)
		require.NoError(t, err)
		types = append(types, assertionType)
	}

	assert.Contains(t, types, "action", "Should have action assertion")
	assert.Contains(t, types, "risk", "Should have risk assertion")
	assert.Contains(t, types, "milestone", "Should have milestone assertion")
}

// TestStoreAssertions_MultipleReprocessing tests that reprocessing the same
// email multiple times (3+ times) never creates duplicates.
//
// This validates idempotency across multiple reprocessing cycles.
//
// Expected behavior:
// - First insert: N assertions created
// - Second insert: 0 assertions created (all duplicates)
// - Third insert: 0 assertions created (all duplicates)
// - Total in DB: N assertions (no duplicates)
func TestStoreAssertions_MultipleReprocessing(t *testing.T) {
	ctx := context.Background()

	// Setup database connection
	pool := testutil.OpenDB(t)
	defer pool.Close()

	tenantID := testRunTenantID
	logger := logging.MustGlobal()

	// Create repository
	repo := NewPostgresAssertionRepository(pool, logger)

	// Create a test source record
	testSourceID := createTestSource(t, ctx, pool, tenantID)
	defer cleanupTestSource(t, ctx, pool, testSourceID)

	assertions := []*Assertion{
		{
			Subject:      "Server",
			Predicate:    "is",
			Object:       "down",
			Confidence:   0.95,
			SourceText:   "The production server is down",
			Category:     "issue",
			Attributions: []Attribution{},
		},
	}

	// First processing
	count1, err := repo.StoreAssertions(ctx, tenantID, testSourceID, assertions, "test-model-v1")
	require.NoError(t, err)
	assert.Equal(t, 1, count1, "First insert should create 1 assertion")

	// Second processing (reprocess)
	count2, err := repo.StoreAssertions(ctx, tenantID, testSourceID, assertions, "test-model-v1")
	require.NoError(t, err)
	assert.Equal(t, 0, count2, "Second insert should create 0 assertions (duplicate)")

	// Third processing (reprocess again)
	count3, err := repo.StoreAssertions(ctx, tenantID, testSourceID, assertions, "test-model-v1")
	require.NoError(t, err)
	assert.Equal(t, 0, count3, "Third insert should create 0 assertions (duplicate)")

	// Fourth processing (reprocess yet again)
	count4, err := repo.StoreAssertions(ctx, tenantID, testSourceID, assertions, "test-model-v1")
	require.NoError(t, err)
	assert.Equal(t, 0, count4, "Fourth insert should create 0 assertions (duplicate)")

	// Verify total count remains 1
	var totalCount int
	countQuery := `
		SELECT COUNT(*)
		FROM assertions
		WHERE tenant_id = $1::uuid AND source_id = $2
	`
	err = pool.QueryRow(ctx, countQuery, tenantID, testSourceID).Scan(&totalCount)
	require.NoError(t, err)

	assert.Equal(t, 1, totalCount, "Should have exactly 1 assertion despite 4 insert attempts")
}

// TestStoreAssertions_DifferentSourcesAllowed tests that the same assertion
// content from different source_ids should be allowed (not considered duplicates).
//
// This ensures the dedup key includes source_id.
//
// Expected behavior:
// - Insert assertion for source A
// - Insert identical assertion for source B (different source_id)
// - Both should be created (total: 2)
func TestStoreAssertions_DifferentSourcesAllowed(t *testing.T) {
	ctx := context.Background()

	// Setup database connection
	pool := testutil.OpenDB(t)
	defer pool.Close()

	tenantID := testRunTenantID
	logger := logging.MustGlobal()

	// Create repository
	repo := NewPostgresAssertionRepository(pool, logger)

	// Create TWO different test sources
	testSourceID1 := createTestSource(t, ctx, pool, tenantID)
	defer cleanupTestSource(t, ctx, pool, testSourceID1)

	testSourceID2 := createTestSource(t, ctx, pool, tenantID)
	defer cleanupTestSource(t, ctx, pool, testSourceID2)

	// Same assertion content for both sources
	assertion := []*Assertion{
		{
			Subject:      "Budget",
			Predicate:    "is",
			Object:       "approved",
			Confidence:   0.90,
			SourceText:   "The budget has been approved by finance",
			Category:     "decision",
			Attributions: []Attribution{},
		},
	}

	// Insert assertion for first source
	count1, err := repo.StoreAssertions(ctx, tenantID, testSourceID1, assertion, "test-model-v1")
	require.NoError(t, err)
	assert.Equal(t, 1, count1, "Should insert 1 assertion for source 1")

	// Insert identical assertion for second source
	count2, err := repo.StoreAssertions(ctx, tenantID, testSourceID2, assertion, "test-model-v1")
	require.NoError(t, err)
	assert.Equal(t, 1, count2, "Should insert 1 assertion for source 2 (different source_id, not a duplicate)")

	// Verify we have assertions for both sources
	countQuery := `
		SELECT source_id, COUNT(*) as count
		FROM assertions
		WHERE tenant_id = $1::uuid AND source_id = ANY($2::bigint[])
		GROUP BY source_id
		ORDER BY source_id
	`
	rows, err := pool.Query(ctx, countQuery, tenantID, []int64{testSourceID1, testSourceID2})
	require.NoError(t, err)
	defer rows.Close()

	sourceCounts := make(map[int64]int)
	for rows.Next() {
		var sourceID int64
		var count int
		err := rows.Scan(&sourceID, &count)
		require.NoError(t, err)
		sourceCounts[sourceID] = count
	}

	assert.Equal(t, 1, sourceCounts[testSourceID1], "Should have 1 assertion for source 1")
	assert.Equal(t, 1, sourceCounts[testSourceID2], "Should have 1 assertion for source 2")
}

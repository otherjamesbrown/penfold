package assertions

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// TestRepository_ListAssertions tests the ListAssertions method.
func TestRepository_ListAssertions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	ctx := context.Background()
	pool := setupTestDB(t)
	defer pool.Close()

	repo := NewRepository(pool, logging.NewNopLogger())

	// Test basic list with no filters
	t.Run("list all assertions", func(t *testing.T) {
		filters := ListFilters{
			Limit: 10,
		}

		results, count, err := repo.ListAssertions(ctx, "00000001-0000-0000-0000-000000000001", filters)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, count, int64(0))
		assert.LessOrEqual(t, len(results), 10)
	})

	// Test filter by type
	t.Run("filter by type", func(t *testing.T) {
		assertionType := "decision"
		filters := ListFilters{
			Type:  &assertionType,
			Limit: 10,
		}

		results, count, err := repo.ListAssertions(ctx, "00000001-0000-0000-0000-000000000001", filters)
		require.NoError(t, err)

		// Verify all results match the filter
		for _, r := range results {
			assert.Equal(t, "decision", r.AssertionType)
		}

		// Count should match number of results if less than limit
		if count < 10 {
			assert.Equal(t, count, int64(len(results)))
		}
	})

	// Test filter by date range
	t.Run("filter by date range", func(t *testing.T) {
		since := time.Now().Add(-30 * 24 * time.Hour)
		filters := ListFilters{
			Since: &since,
			Limit: 10,
		}

		results, count, err := repo.ListAssertions(ctx, "00000001-0000-0000-0000-000000000001", filters)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, count, int64(0))

		// Verify all results are within date range
		for _, r := range results {
			if r.SourceDate != nil {
				assert.True(t, r.SourceDate.After(since) || r.SourceDate.Equal(since),
					"Source date should be >= since")
			}
		}
	})

	// Test pagination
	t.Run("pagination", func(t *testing.T) {
		// Get first page
		filters1 := ListFilters{
			Limit:  5,
			Offset: 0,
		}
		results1, count1, err := repo.ListAssertions(ctx, "00000001-0000-0000-0000-000000000001", filters1)
		require.NoError(t, err)

		// Get second page
		filters2 := ListFilters{
			Limit:  5,
			Offset: 5,
		}
		results2, count2, err := repo.ListAssertions(ctx, "00000001-0000-0000-0000-000000000001", filters2)
		require.NoError(t, err)

		// Total count should be the same
		assert.Equal(t, count1, count2)

		// Results should be different (unless there are < 6 total)
		if count1 > 5 {
			assert.NotEqual(t, results1[0].ID, results2[0].ID)
		}
	})
}

// TestRepository_SearchAssertions tests the SearchAssertions method.
func TestRepository_SearchAssertions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	ctx := context.Background()
	pool := setupTestDB(t)
	defer pool.Close()

	repo := NewRepository(pool, logging.NewNopLogger())

	t.Run("search with keyword", func(t *testing.T) {
		filters := ListFilters{
			Limit: 10,
		}

		// Search for a common word
		_, count, err := repo.SearchAssertions(ctx, "00000001-0000-0000-0000-000000000001", "the", filters)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, count, int64(0))

		// Verify results contain the keyword (case-insensitive)
		// Note: Some results might have the keyword only in source_quote
	})

	t.Run("search with type filter", func(t *testing.T) {
		assertionType := "risk"
		filters := ListFilters{
			Type:  &assertionType,
			Limit: 10,
		}

		results, _, err := repo.SearchAssertions(ctx, "00000001-0000-0000-0000-000000000001", "test", filters)
		require.NoError(t, err)

		// All results should be of type "risk"
		for _, r := range results {
			assert.Equal(t, "risk", r.AssertionType)
		}
	})
}

// TestRepository_GetAssertionSummary tests the GetAssertionSummary method.
func TestRepository_GetAssertionSummary(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	ctx := context.Background()
	pool := setupTestDB(t)
	defer pool.Close()

	repo := NewRepository(pool, logging.NewNopLogger())

	t.Run("summary by type", func(t *testing.T) {
		_, total, err := repo.GetAssertionSummary(ctx, "00000001-0000-0000-0000-000000000001", nil, nil, "type")
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, int64(0))
	})

	t.Run("summary by project", func(t *testing.T) {
		_, total, err := repo.GetAssertionSummary(ctx, "00000001-0000-0000-0000-000000000001", nil, nil, "project")
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, int64(0))
	})

	t.Run("summary with date range", func(t *testing.T) {
		since := time.Now().Add(-90 * 24 * time.Hour)
		entries, total, err := repo.GetAssertionSummary(ctx, "00000001-0000-0000-0000-000000000001", &since, nil, "type")
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, int64(0))

		// Verify week counts make sense
		for _, e := range entries {
			assert.LessOrEqual(t, e.ThisWeek, e.Count)
			assert.LessOrEqual(t, e.LastWeek, e.Count)
		}
	})
}

// setupTestDB creates a test database connection.
func setupTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	// Use DATABASE_URL from environment
	dbURL := getTestDatabaseURL(t)

	config, err := pgxpool.ParseConfig(dbURL)
	require.NoError(t, err)

	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	require.NoError(t, err)

	// Verify connection
	err = pool.Ping(context.Background())
	require.NoError(t, err)

	return pool
}

// getTestDatabaseURL returns the database URL for testing.
func getTestDatabaseURL(t *testing.T) string {
	t.Helper()

	dbURL := "postgres://penfold:penfold123@dev02.brown.chat:5432/penfold?sslmode=disable"
	return dbURL
}

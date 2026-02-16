//go:build integration

package classify

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// setupTestDB creates a test database connection.
func setupTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Use DATABASE_URL from environment or default to dev02 with SSL cert auth
	dbURL := "host=dev02.brown.chat dbname=penfold user=penfold sslmode=verify-full"

	config, err := pgxpool.ParseConfig(dbURL)
	require.NoError(t, err)

	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	require.NoError(t, err)

	// Verify connection
	err = pool.Ping(context.Background())
	require.NoError(t, err)

	return pool
}

// cleanupSuggestions removes test suggestions to ensure test isolation
func cleanupSuggestions(t *testing.T, pool *pgxpool.Pool, contentIDs ...string) {
	t.Helper()
	ctx := context.Background()

	for _, contentID := range contentIDs {
		_, err := pool.Exec(ctx, "DELETE FROM classification_suggestions WHERE content_id = $1", contentID)
		require.NoError(t, err)
	}
}

// TestCreateSuggestion verifies that a classification suggestion can be created and stored.
func TestCreateSuggestion(t *testing.T) {
	pool := setupTestDB(t)
	defer pool.Close()

	repo := NewClassificationSuggestionRepository(pool, logging.NewNopLogger())
	ctx := context.Background()

	contentID := uuid.New().String()
	defer cleanupSuggestions(t, pool, contentID)

	suggestion := &ClassificationSuggestion{
		ContentID:    contentID,
		SourceSystem: "jira",
		Pattern:      "Contains [PROJECT-123] ticket reference",
		Confidence:   0.95,
		Status:       "pending",
	}

	err := repo.CreateSuggestion(ctx, suggestion)
	require.NoError(t, err)

	// Verify it was stored by retrieving it
	retrieved, err := repo.GetByContentID(ctx, contentID)
	require.NoError(t, err)
	require.Len(t, retrieved, 1)

	assert.Equal(t, contentID, retrieved[0].ContentID)
	assert.Equal(t, "jira", retrieved[0].SourceSystem)
	assert.Equal(t, "Contains [PROJECT-123] ticket reference", retrieved[0].Pattern)
	assert.Equal(t, 0.95, retrieved[0].Confidence)
	assert.Equal(t, "pending", retrieved[0].Status)
	assert.NotZero(t, retrieved[0].ID)
	assert.NotZero(t, retrieved[0].CreatedAt)
}

// TestListPending verifies that only pending suggestions are returned.
func TestListPending(t *testing.T) {
	pool := setupTestDB(t)
	defer pool.Close()

	repo := NewClassificationSuggestionRepository(pool, logging.NewNopLogger())
	ctx := context.Background()

	contentID1 := uuid.New().String()
	contentID2 := uuid.New().String()
	contentID3 := uuid.New().String()
	defer cleanupSuggestions(t, pool, contentID1, contentID2, contentID3)

	// Create suggestions with different statuses
	pending1 := &ClassificationSuggestion{
		ContentID:    contentID1,
		SourceSystem: "jira",
		Pattern:      "Pattern 1",
		Confidence:   0.9,
		Status:       "pending",
	}
	pending2 := &ClassificationSuggestion{
		ContentID:    contentID2,
		SourceSystem: "aha",
		Pattern:      "Pattern 2",
		Confidence:   0.85,
		Status:       "pending",
	}
	approved := &ClassificationSuggestion{
		ContentID:    contentID3,
		SourceSystem: "google_docs",
		Pattern:      "Pattern 3",
		Confidence:   0.95,
		Status:       "approved",
	}

	require.NoError(t, repo.CreateSuggestion(ctx, pending1))
	require.NoError(t, repo.CreateSuggestion(ctx, pending2))
	require.NoError(t, repo.CreateSuggestion(ctx, approved))

	// List pending suggestions
	pendingSuggestions, err := repo.ListPending(ctx)
	require.NoError(t, err)

	// Filter to only our test suggestions
	var ourPending []*ClassificationSuggestion
	testIDs := map[string]bool{contentID1: true, contentID2: true, contentID3: true}
	for _, s := range pendingSuggestions {
		if testIDs[s.ContentID] {
			ourPending = append(ourPending, s)
		}
	}

	require.Len(t, ourPending, 2, "Should return exactly 2 pending suggestions")

	// Verify only pending suggestions are returned
	for _, s := range ourPending {
		assert.Equal(t, "pending", s.Status)
		assert.NotEqual(t, contentID3, s.ContentID, "Approved suggestion should not be in pending list")
	}
}

// TestUpdateStatus verifies that suggestion status can be updated.
func TestUpdateStatus(t *testing.T) {
	pool := setupTestDB(t)
	defer pool.Close()

	repo := NewClassificationSuggestionRepository(pool, logging.NewNopLogger())
	ctx := context.Background()

	contentID := uuid.New().String()
	defer cleanupSuggestions(t, pool, contentID)

	// Create a pending suggestion
	suggestion := &ClassificationSuggestion{
		ContentID:    contentID,
		SourceSystem: "jira",
		Pattern:      "Test pattern",
		Confidence:   0.9,
		Status:       "pending",
	}

	err := repo.CreateSuggestion(ctx, suggestion)
	require.NoError(t, err)

	// Retrieve to get the ID
	retrieved, err := repo.GetByContentID(ctx, contentID)
	require.NoError(t, err)
	require.Len(t, retrieved, 1)
	suggestionID := retrieved[0].ID

	// Update status to approved
	err = repo.UpdateStatus(ctx, suggestionID, "approved")
	require.NoError(t, err)

	// Verify the status was updated
	updated, err := repo.GetByContentID(ctx, contentID)
	require.NoError(t, err)
	require.Len(t, updated, 1)
	assert.Equal(t, "approved", updated[0].Status)

	// Update status to rejected
	err = repo.UpdateStatus(ctx, suggestionID, "rejected")
	require.NoError(t, err)

	// Verify the status was updated again
	updated, err = repo.GetByContentID(ctx, contentID)
	require.NoError(t, err)
	require.Len(t, updated, 1)
	assert.Equal(t, "rejected", updated[0].Status)
}

// TestGetByContentID verifies retrieval of suggestions by content ID.
func TestGetByContentID(t *testing.T) {
	pool := setupTestDB(t)
	defer pool.Close()

	repo := NewClassificationSuggestionRepository(pool, logging.NewNopLogger())
	ctx := context.Background()

	contentID1 := uuid.New().String()
	contentID2 := uuid.New().String()
	defer cleanupSuggestions(t, pool, contentID1, contentID2)

	// Create multiple suggestions for different content IDs
	suggestion1 := &ClassificationSuggestion{
		ContentID:    contentID1,
		SourceSystem: "jira",
		Pattern:      "Pattern 1",
		Confidence:   0.9,
		Status:       "pending",
	}
	suggestion2a := &ClassificationSuggestion{
		ContentID:    contentID2,
		SourceSystem: "aha",
		Pattern:      "Pattern 2a",
		Confidence:   0.85,
		Status:       "pending",
	}
	suggestion2b := &ClassificationSuggestion{
		ContentID:    contentID2,
		SourceSystem: "google_docs",
		Pattern:      "Pattern 2b",
		Confidence:   0.8,
		Status:       "pending",
	}

	require.NoError(t, repo.CreateSuggestion(ctx, suggestion1))
	require.NoError(t, repo.CreateSuggestion(ctx, suggestion2a))
	require.NoError(t, repo.CreateSuggestion(ctx, suggestion2b))

	// Retrieve by contentID1
	results1, err := repo.GetByContentID(ctx, contentID1)
	require.NoError(t, err)
	require.Len(t, results1, 1)
	assert.Equal(t, contentID1, results1[0].ContentID)
	assert.Equal(t, "jira", results1[0].SourceSystem)

	// Retrieve by contentID2 (should return 2 suggestions)
	results2, err := repo.GetByContentID(ctx, contentID2)
	require.NoError(t, err)
	require.Len(t, results2, 2)

	// Verify both suggestions belong to contentID2
	for _, s := range results2 {
		assert.Equal(t, contentID2, s.ContentID)
	}

	// Verify different source systems
	sourceSystems := make(map[string]bool)
	for _, s := range results2 {
		sourceSystems[s.SourceSystem] = true
	}
	assert.True(t, sourceSystems["aha"])
	assert.True(t, sourceSystems["google_docs"])
}

// TestGetByContentID_NoResults verifies empty result for non-existent content ID.
func TestGetByContentID_NoResults(t *testing.T) {
	pool := setupTestDB(t)
	defer pool.Close()

	repo := NewClassificationSuggestionRepository(pool, logging.NewNopLogger())
	ctx := context.Background()

	nonExistentID := uuid.New().String()

	results, err := repo.GetByContentID(ctx, nonExistentID)
	require.NoError(t, err)
	assert.Empty(t, results)
}

// TestDuplicateContentID verifies handling of duplicate suggestions for same content.
func TestDuplicateContentID(t *testing.T) {
	pool := setupTestDB(t)
	defer pool.Close()

	repo := NewClassificationSuggestionRepository(pool, logging.NewNopLogger())
	ctx := context.Background()

	contentID := uuid.New().String()
	defer cleanupSuggestions(t, pool, contentID)

	// Create first suggestion
	suggestion1 := &ClassificationSuggestion{
		ContentID:    contentID,
		SourceSystem: "jira",
		Pattern:      "Pattern 1",
		Confidence:   0.9,
		Status:       "pending",
	}

	err := repo.CreateSuggestion(ctx, suggestion1)
	require.NoError(t, err)

	// Attempt to create a second suggestion with different source system but same content_id
	// This should succeed - multiple suggestions per content are allowed
	suggestion2 := &ClassificationSuggestion{
		ContentID:    contentID,
		SourceSystem: "aha",
		Pattern:      "Pattern 2",
		Confidence:   0.85,
		Status:       "pending",
	}

	err = repo.CreateSuggestion(ctx, suggestion2)
	require.NoError(t, err)

	// Verify both suggestions exist
	results, err := repo.GetByContentID(ctx, contentID)
	require.NoError(t, err)
	require.Len(t, results, 2, "Multiple suggestions per content_id should be allowed")

	// Verify different source systems
	sourceSystems := make(map[string]bool)
	for _, s := range results {
		sourceSystems[s.SourceSystem] = true
	}
	assert.True(t, sourceSystems["jira"])
	assert.True(t, sourceSystems["aha"])
}

// TestConfidenceBoundaries verifies confidence values are stored correctly.
func TestConfidenceBoundaries(t *testing.T) {
	pool := setupTestDB(t)
	defer pool.Close()

	repo := NewClassificationSuggestionRepository(pool, logging.NewNopLogger())
	ctx := context.Background()

	testCases := []struct {
		name       string
		confidence float64
	}{
		{"minimum", 0.0},
		{"low", 0.25},
		{"medium", 0.5},
		{"high", 0.75},
		{"maximum", 1.0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			contentID := uuid.New().String()
			defer cleanupSuggestions(t, pool, contentID)

			suggestion := &ClassificationSuggestion{
				ContentID:    contentID,
				SourceSystem: "jira",
				Pattern:      "Test pattern",
				Confidence:   tc.confidence,
				Status:       "pending",
			}

			err := repo.CreateSuggestion(ctx, suggestion)
			require.NoError(t, err)

			retrieved, err := repo.GetByContentID(ctx, contentID)
			require.NoError(t, err)
			require.Len(t, retrieved, 1)
			assert.Equal(t, tc.confidence, retrieved[0].Confidence)
		})
	}
}

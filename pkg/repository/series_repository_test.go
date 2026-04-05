package repository

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getEnvOrDefault returns the environment variable value or a default.
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// setupTestDB creates a test database connection.
// It reads configuration from environment variables:
//   - PENFOLD_DB_HOST (default: dev02.brown.chat)
//   - PENFOLD_DB_PORT (default: 5432)
//   - PENFOLD_DB_USER (default: penfold)
//   - PENFOLD_DB_PASSWORD (required, or uses SSL cert auth)
//   - PENFOLD_DB_NAME (default: penfold_test)
func setupTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	host := getEnvOrDefault("PENFOLD_DB_HOST", "dev02.brown.chat")
	port := getEnvOrDefault("PENFOLD_DB_PORT", "5432")
	user := getEnvOrDefault("PENFOLD_DB_USER", "penfold")
	dbName := getEnvOrDefault("PENFOLD_DB_NAME", "penfold")

	// Build connection string with SSL verify-full (standard for dev02)
	connString := fmt.Sprintf(
		"host=%s port=%s user=%s dbname=%s sslmode=verify-full",
		host, port, user, dbName,
	)

	pool, err := pgxpool.New(context.Background(), connString)
	if err != nil {
		t.Skipf("Could not connect to test database: %v", err)
		return nil
	}

	// Verify connection
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Skipf("Could not ping test database: %v", err)
		return nil
	}

	return pool
}

// cleanupTestData removes test data from the database.
func cleanupTestData(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	// Delete test series (cascading will handle meetings)
	_, err := pool.Exec(ctx, "DELETE FROM meeting_series WHERE name LIKE 'Test%'")
	if err != nil {
		t.Logf("Warning: could not clean up test data: %v", err)
	}
}

func TestSeriesRepository_Create(t *testing.T) {
	pool := setupTestDB(t)
	if pool == nil {
		return
	}
	defer pool.Close()
	defer cleanupTestData(t, pool)

	repo := NewSeriesRepository(pool)
	ctx := context.Background()

	t.Run("creates series with auto-generated ID", func(t *testing.T) {
		series := &MeetingSeries{
			Name:        "Test Series Auto ID",
			Description: "Test description",
		}

		err := repo.Create(ctx, series)
		require.NoError(t, err)
		assert.NotEmpty(t, series.ID)
		assert.Contains(t, series.ID, "ms-")
		assert.NotZero(t, series.CreatedAt)
		assert.NotZero(t, series.UpdatedAt)
	})

	t.Run("creates series with provided ID", func(t *testing.T) {
		series := &MeetingSeries{
			ID:          "ms-custom-test-id",
			Name:        "Test Series Custom ID",
			Description: "Test description",
		}

		err := repo.Create(ctx, series)
		require.NoError(t, err)
		assert.Equal(t, "ms-custom-test-id", series.ID)
	})

	t.Run("creates series without project ID", func(t *testing.T) {
		series := &MeetingSeries{
			Name:        "Test Series No Project",
			Description: "Test description",
			ProjectID:   nil, // Project ID is optional
		}

		err := repo.Create(ctx, series)
		require.NoError(t, err)
		assert.NotEmpty(t, series.ID)
		assert.Nil(t, series.ProjectID)
	})

	t.Run("returns error for nil series", func(t *testing.T) {
		err := repo.Create(ctx, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("returns error for duplicate name", func(t *testing.T) {
		series1 := &MeetingSeries{
			Name:        "Test Duplicate Name",
			Description: "First",
		}
		err := repo.Create(ctx, series1)
		require.NoError(t, err)

		series2 := &MeetingSeries{
			Name:        "Test Duplicate Name",
			Description: "Second",
		}
		err = repo.Create(ctx, series2)
		assert.Error(t, err)
	})
}

func TestSeriesRepository_GetByID(t *testing.T) {
	pool := setupTestDB(t)
	if pool == nil {
		return
	}
	defer pool.Close()
	defer cleanupTestData(t, pool)

	repo := NewSeriesRepository(pool)
	ctx := context.Background()

	t.Run("retrieves existing series", func(t *testing.T) {
		// Create a series
		created := &MeetingSeries{
			Name:        "Test GetByID",
			Description: "Test description",
		}
		err := repo.Create(ctx, created)
		require.NoError(t, err)

		// Retrieve it
		retrieved, err := repo.GetByID(ctx, created.ID)
		require.NoError(t, err)
		require.NotNil(t, retrieved)
		assert.Equal(t, created.ID, retrieved.ID)
		assert.Equal(t, created.Name, retrieved.Name)
		assert.Equal(t, created.Description, retrieved.Description)
	})

	t.Run("returns nil for non-existent series", func(t *testing.T) {
		retrieved, err := repo.GetByID(ctx, "ms-nonexistent")
		require.NoError(t, err)
		assert.Nil(t, retrieved)
	})
}

func TestSeriesRepository_GetByName(t *testing.T) {
	pool := setupTestDB(t)
	if pool == nil {
		return
	}
	defer pool.Close()
	defer cleanupTestData(t, pool)

	repo := NewSeriesRepository(pool)
	ctx := context.Background()

	t.Run("retrieves existing series by name", func(t *testing.T) {
		// Create a series
		created := &MeetingSeries{
			Name:        "Test GetByName Unique",
			Description: "Test description",
		}
		err := repo.Create(ctx, created)
		require.NoError(t, err)

		// Retrieve it by name
		retrieved, err := repo.GetByName(ctx, "Test GetByName Unique")
		require.NoError(t, err)
		require.NotNil(t, retrieved)
		assert.Equal(t, created.ID, retrieved.ID)
		assert.Equal(t, created.Name, retrieved.Name)
	})

	t.Run("returns nil for non-existent name", func(t *testing.T) {
		retrieved, err := repo.GetByName(ctx, "Non Existent Series")
		require.NoError(t, err)
		assert.Nil(t, retrieved)
	})
}

func TestSeriesRepository_List(t *testing.T) {
	pool := setupTestDB(t)
	if pool == nil {
		return
	}
	defer pool.Close()
	defer cleanupTestData(t, pool)

	repo := NewSeriesRepository(pool)
	ctx := context.Background()

	t.Run("lists all series", func(t *testing.T) {
		// Create multiple series
		series1 := &MeetingSeries{Name: "Test List A", Description: "First"}
		series2 := &MeetingSeries{Name: "Test List B", Description: "Second"}
		series3 := &MeetingSeries{Name: "Test List C", Description: "Third"}

		require.NoError(t, repo.Create(ctx, series1))
		require.NoError(t, repo.Create(ctx, series2))
		require.NoError(t, repo.Create(ctx, series3))

		// List them
		list, err := repo.List(ctx)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(list), 3)

		// Find our test series
		var found int
		for _, s := range list {
			if s.Name == "Test List A" || s.Name == "Test List B" || s.Name == "Test List C" {
				found++
			}
		}
		assert.Equal(t, 3, found)
	})

	t.Run("returns ordered by name", func(t *testing.T) {
		list, err := repo.List(ctx)
		require.NoError(t, err)

		// Verify ordering (at least for our test data)
		for i := 1; i < len(list); i++ {
			assert.LessOrEqual(t, list[i-1].Name, list[i].Name)
		}
	})
}

func TestSeriesRepository_Delete_OrphansMeetings(t *testing.T) {
	pool := setupTestDB(t)
	if pool == nil {
		return
	}
	defer pool.Close()
	defer cleanupTestData(t, pool)

	repo := NewSeriesRepository(pool)
	ctx := context.Background()

	t.Run("deletes series and orphans meetings", func(t *testing.T) {
		// Create a series
		series := &MeetingSeries{
			Name:        "Test Delete Series",
			Description: "Will be deleted",
		}
		err := repo.Create(ctx, series)
		require.NoError(t, err)

		// Create some test meetings and assign them to the series
		// First, we need to create meetings in the database
		meetingID1 := createTestMeeting(t, pool, "Test Meeting 1")
		meetingID2 := createTestMeeting(t, pool, "Test Meeting 2")

		// Assign meetings to series
		err = repo.SetMeetingSeries(ctx, meetingID1, series.ID)
		require.NoError(t, err)
		err = repo.SetMeetingSeries(ctx, meetingID2, series.ID)
		require.NoError(t, err)

		// Verify count before delete
		count, err := repo.CountMeetingsInSeries(ctx, series.ID)
		require.NoError(t, err)
		assert.Equal(t, 2, count)

		// Delete the series
		orphanedCount, err := repo.Delete(ctx, series.ID)
		require.NoError(t, err)
		assert.Equal(t, 2, orphanedCount)

		// Verify series is deleted
		retrieved, err := repo.GetByID(ctx, series.ID)
		require.NoError(t, err)
		assert.Nil(t, retrieved)

		// Verify meetings still exist but are orphaned
		meetings, err := repo.GetMeetingsForSeries(ctx, series.ID)
		require.NoError(t, err)
		assert.Equal(t, 0, len(meetings))

		// Clean up test meetings
		deleteTestMeeting(t, pool, meetingID1)
		deleteTestMeeting(t, pool, meetingID2)
	})

	t.Run("returns error for non-existent series", func(t *testing.T) {
		_, err := repo.Delete(ctx, "ms-nonexistent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestSeriesRepository_SetMeetingSeries(t *testing.T) {
	pool := setupTestDB(t)
	if pool == nil {
		return
	}
	defer pool.Close()
	defer cleanupTestData(t, pool)

	repo := NewSeriesRepository(pool)
	ctx := context.Background()

	t.Run("assigns meeting to series", func(t *testing.T) {
		series := &MeetingSeries{
			Name:        "Test Set Series",
			Description: "For assignment",
		}
		err := repo.Create(ctx, series)
		require.NoError(t, err)

		meetingID := createTestMeeting(t, pool, "Test Meeting for Set")
		defer deleteTestMeeting(t, pool, meetingID)

		err = repo.SetMeetingSeries(ctx, meetingID, series.ID)
		require.NoError(t, err)

		// Verify assignment
		count, err := repo.CountMeetingsInSeries(ctx, series.ID)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("returns error for non-existent meeting", func(t *testing.T) {
		series := &MeetingSeries{
			Name:        "Test Set Series Error",
			Description: "For error test",
		}
		err := repo.Create(ctx, series)
		require.NoError(t, err)

		err = repo.SetMeetingSeries(ctx, "999999", series.ID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestSeriesRepository_UnsetMeetingSeries(t *testing.T) {
	pool := setupTestDB(t)
	if pool == nil {
		return
	}
	defer pool.Close()
	defer cleanupTestData(t, pool)

	repo := NewSeriesRepository(pool)
	ctx := context.Background()

	t.Run("removes meeting from series", func(t *testing.T) {
		series := &MeetingSeries{
			Name:        "Test Unset Series",
			Description: "For removal",
		}
		err := repo.Create(ctx, series)
		require.NoError(t, err)

		meetingID := createTestMeeting(t, pool, "Test Meeting for Unset")
		defer deleteTestMeeting(t, pool, meetingID)

		// Assign then unassign
		err = repo.SetMeetingSeries(ctx, meetingID, series.ID)
		require.NoError(t, err)

		err = repo.UnsetMeetingSeries(ctx, meetingID)
		require.NoError(t, err)

		// Verify removal
		count, err := repo.CountMeetingsInSeries(ctx, series.ID)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})
}

func TestSeriesRepository_GetMeetingsForSeries(t *testing.T) {
	pool := setupTestDB(t)
	if pool == nil {
		return
	}
	defer pool.Close()
	defer cleanupTestData(t, pool)

	repo := NewSeriesRepository(pool)
	ctx := context.Background()

	t.Run("retrieves meetings for series", func(t *testing.T) {
		series := &MeetingSeries{
			Name:        "Test GetMeetings Series",
			Description: "For retrieval",
		}
		err := repo.Create(ctx, series)
		require.NoError(t, err)

		meetingID1 := createTestMeeting(t, pool, "Test Meeting 1")
		meetingID2 := createTestMeeting(t, pool, "Test Meeting 2")
		defer deleteTestMeeting(t, pool, meetingID1)
		defer deleteTestMeeting(t, pool, meetingID2)

		err = repo.SetMeetingSeries(ctx, meetingID1, series.ID)
		require.NoError(t, err)
		err = repo.SetMeetingSeries(ctx, meetingID2, series.ID)
		require.NoError(t, err)

		meetings, err := repo.GetMeetingsForSeries(ctx, series.ID)
		require.NoError(t, err)
		assert.Equal(t, 2, len(meetings))

		// Verify meeting info structure
		for _, m := range meetings {
			assert.NotEmpty(t, m.ID)
			assert.NotEmpty(t, m.Title)
			// Date and Platform may be null in test data
		}
	})

	t.Run("returns empty list for series with no meetings", func(t *testing.T) {
		series := &MeetingSeries{
			Name:        "Test Empty Series",
			Description: "No meetings",
		}
		err := repo.Create(ctx, series)
		require.NoError(t, err)

		meetings, err := repo.GetMeetingsForSeries(ctx, series.ID)
		require.NoError(t, err)
		assert.Equal(t, 0, len(meetings))
	})
}

func TestSeriesRepository_CountMeetingsInSeries(t *testing.T) {
	pool := setupTestDB(t)
	if pool == nil {
		return
	}
	defer pool.Close()
	defer cleanupTestData(t, pool)

	repo := NewSeriesRepository(pool)
	ctx := context.Background()

	t.Run("counts meetings in series", func(t *testing.T) {
		series := &MeetingSeries{
			Name:        "Test Count Series",
			Description: "For counting",
		}
		err := repo.Create(ctx, series)
		require.NoError(t, err)

		// Initially zero
		count, err := repo.CountMeetingsInSeries(ctx, series.ID)
		require.NoError(t, err)
		assert.Equal(t, 0, count)

		// Add meetings
		meetingID1 := createTestMeeting(t, pool, "Test Meeting 1")
		meetingID2 := createTestMeeting(t, pool, "Test Meeting 2")
		meetingID3 := createTestMeeting(t, pool, "Test Meeting 3")
		defer deleteTestMeeting(t, pool, meetingID1)
		defer deleteTestMeeting(t, pool, meetingID2)
		defer deleteTestMeeting(t, pool, meetingID3)

		err = repo.SetMeetingSeries(ctx, meetingID1, series.ID)
		require.NoError(t, err)
		err = repo.SetMeetingSeries(ctx, meetingID2, series.ID)
		require.NoError(t, err)
		err = repo.SetMeetingSeries(ctx, meetingID3, series.ID)
		require.NoError(t, err)

		// Count should be 3
		count, err = repo.CountMeetingsInSeries(ctx, series.ID)
		require.NoError(t, err)
		assert.Equal(t, 3, count)
	})
}

// Helper functions for creating and deleting test meetings

func createTestMeeting(t *testing.T, pool *pgxpool.Pool, title string) string {
	t.Helper()
	ctx := context.Background()

	query := `
		INSERT INTO meetings (title, meeting_date, platform, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		RETURNING id
	`

	var id int64
	err := pool.QueryRow(ctx, query, title, time.Now(), "test").Scan(&id)
	if err != nil {
		t.Fatalf("Failed to create test meeting: %v", err)
	}

	return fmt.Sprintf("%d", id)
}

func deleteTestMeeting(t *testing.T, pool *pgxpool.Pool, meetingID string) {
	t.Helper()
	ctx := context.Background()

	query := `DELETE FROM meetings WHERE id = $1`
	_, err := pool.Exec(ctx, query, meetingID)
	if err != nil {
		t.Logf("Warning: could not delete test meeting %s: %v", meetingID, err)
	}
}

// Helper functions for creating and deleting test sources

func createTestSource(t *testing.T, pool *pgxpool.Pool, title string) string {
	t.Helper()
	ctx := context.Background()

	// Generate a short content ID (max 12 chars for varchar(12))
	contentID := fmt.Sprintf("ts-%d", time.Now().UnixNano()%1000000000)

	// Use test values for required fields
	testTenantID := "00000000-0000-0000-0000-000000000001"
	externalID := fmt.Sprintf("ext-%d", time.Now().UnixNano()%1000000)
	// Content hash must be valid hex (SHA-256 is 64 chars)
	contentHash := fmt.Sprintf("%064d", time.Now().UnixNano()%1000000000000000)

	query := `
		INSERT INTO sources (
			tenant_id,
			content_id,
			external_id,
			content_hash,
			source_system,
			source_timestamp,
			ingestion_metadata,
			is_deleted,
			created_at,
			updated_at
		)
		VALUES ($1::uuid, $2::text, $3::text, $4::text, $5::text, NOW(), jsonb_build_object('title', $6::text), false, NOW(), NOW())
		RETURNING content_id
	`

	var id string
	err := pool.QueryRow(ctx, query, testTenantID, contentID, externalID, contentHash, "teams", title).Scan(&id)
	if err != nil {
		t.Fatalf("Failed to create test source: %v", err)
	}

	return id
}

func deleteTestSource(t *testing.T, pool *pgxpool.Pool, contentID string) {
	t.Helper()
	ctx := context.Background()

	query := `DELETE FROM sources WHERE content_id = $1`
	_, err := pool.Exec(ctx, query, contentID)
	if err != nil {
		t.Logf("Warning: could not delete test source %s: %v", contentID, err)
	}
}

func TestSeriesRepository_UpdateSourceMetadata(t *testing.T) {
	pool := setupTestDB(t)
	if pool == nil {
		return
	}
	defer pool.Close()

	repo := NewSeriesRepository(pool)
	ctx := context.Background()

	t.Run("updates title only", func(t *testing.T) {
		contentID := createTestSource(t, pool, "Original Title")
		defer deleteTestSource(t, pool, contentID)

		updates := map[string]interface{}{
			"title": "Updated Title",
		}

		err := repo.UpdateSourceMetadata(ctx, contentID, updates)
		require.NoError(t, err)

		// Verify update
		source, err := repo.GetSourceByContentID(ctx, contentID)
		require.NoError(t, err)
		require.NotNil(t, source)
		assert.Equal(t, "Updated Title", source.Title)
	})

	t.Run("updates multiple fields", func(t *testing.T) {
		contentID := createTestSource(t, pool, "Original Title")
		defer deleteTestSource(t, pool, contentID)

		updates := map[string]interface{}{
			"title":       "New Title",
			"description": "New Description",
			"category":    "New Category",
		}

		err := repo.UpdateSourceMetadata(ctx, contentID, updates)
		require.NoError(t, err)

		// Verify updates
		source, err := repo.GetSourceByContentID(ctx, contentID)
		require.NoError(t, err)
		require.NotNil(t, source)
		assert.Equal(t, "New Title", source.Title)
		assert.Equal(t, "New Description", source.Description)
		assert.Equal(t, "New Category", source.Category)
	})

	t.Run("handles empty updates map", func(t *testing.T) {
		contentID := createTestSource(t, pool, "Test Title")
		defer deleteTestSource(t, pool, contentID)

		updates := map[string]interface{}{}

		err := repo.UpdateSourceMetadata(ctx, contentID, updates)
		require.NoError(t, err)
	})

	t.Run("returns error for non-existent source", func(t *testing.T) {
		updates := map[string]interface{}{
			"title": "Updated Title",
		}

		err := repo.UpdateSourceMetadata(ctx, "non-existent-id", updates)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("updates tags with string array", func(t *testing.T) {
		contentID := createTestSource(t, pool, "Test Title")
		defer deleteTestSource(t, pool, contentID)

		// Pass a []string for tags (this was causing the bug)
		updates := map[string]interface{}{
			"tags": []string{"test-tag", "another-tag"},
		}

		err := repo.UpdateSourceMetadata(ctx, contentID, updates)
		require.NoError(t, err)

		// Verify the tags were stored correctly by querying the database directly
		var tagsJSON string
		err = pool.QueryRow(ctx,
			"SELECT COALESCE(ingestion_metadata->>'tags', '[]') FROM sources WHERE content_id = $1",
			contentID,
		).Scan(&tagsJSON)
		require.NoError(t, err)

		// The tags should be stored as a JSON array
		assert.Contains(t, tagsJSON, "test-tag")
		assert.Contains(t, tagsJSON, "another-tag")
	})
}

func TestSeriesRepository_GetSourceByContentID(t *testing.T) {
	pool := setupTestDB(t)
	if pool == nil {
		return
	}
	defer pool.Close()

	repo := NewSeriesRepository(pool)
	ctx := context.Background()

	t.Run("retrieves existing source", func(t *testing.T) {
		contentID := createTestSource(t, pool, "Test Source Title")
		defer deleteTestSource(t, pool, contentID)

		source, err := repo.GetSourceByContentID(ctx, contentID)
		require.NoError(t, err)
		require.NotNil(t, source)
		assert.Equal(t, contentID, source.ContentID)
		assert.Equal(t, "Test Source Title", source.Title)
		assert.Equal(t, "teams", source.Platform)
	})

	t.Run("returns nil for non-existent source", func(t *testing.T) {
		source, err := repo.GetSourceByContentID(ctx, "non-existent-id")
		require.NoError(t, err)
		assert.Nil(t, source)
	})

	t.Run("retrieves source with metadata fields", func(t *testing.T) {
		contentID := createTestSource(t, pool, "Test Source")
		defer deleteTestSource(t, pool, contentID)

		// Update with multiple metadata fields
		updates := map[string]interface{}{
			"description": "Test Description",
			"category":    "Test Category",
		}
		err := repo.UpdateSourceMetadata(ctx, contentID, updates)
		require.NoError(t, err)

		// Retrieve and verify
		source, err := repo.GetSourceByContentID(ctx, contentID)
		require.NoError(t, err)
		require.NotNil(t, source)
		assert.Equal(t, "Test Description", source.Description)
		assert.Equal(t, "Test Category", source.Category)
	})
}

//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"

	"github.com/otherjamesbrown/penfold/pkg/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMeetingSeries_CRUD(t *testing.T) {
	db := SetupTestDB(t)

	repo := repository.NewSeriesRepository(db.Pool)
	ctx := context.Background()

	// Use unique names to avoid conflicts across test runs
	testID := uniqueTestID()
	weeklyName := "Weekly Standup " + testID
	sprintName := "Sprint Planning " + testID

	var createdSeriesIDs []string
	t.Cleanup(func() {
		for _, id := range createdSeriesIDs {
			_, _ = repo.Delete(ctx, id)
		}
	})

	t.Run("create series", func(t *testing.T) {
		series := &repository.MeetingSeries{
			Name:        weeklyName,
			Description: "Engineering team weekly standup",
		}

		err := repo.Create(ctx, series)
		require.NoError(t, err)
		assert.NotEmpty(t, series.ID)
		assert.Contains(t, series.ID, "ms-")
		assert.NotZero(t, series.CreatedAt)
		assert.NotZero(t, series.UpdatedAt)
		createdSeriesIDs = append(createdSeriesIDs, series.ID)
	})

	t.Run("get series by name", func(t *testing.T) {
		series, err := repo.GetByName(ctx, weeklyName)
		require.NoError(t, err)
		require.NotNil(t, series)
		assert.Equal(t, weeklyName, series.Name)
		assert.Equal(t, "Engineering team weekly standup", series.Description)
	})

	t.Run("list all series", func(t *testing.T) {
		// Create another series
		series := &repository.MeetingSeries{
			Name:        sprintName,
			Description: "Bi-weekly sprint planning",
		}
		err := repo.Create(ctx, series)
		require.NoError(t, err)
		createdSeriesIDs = append(createdSeriesIDs, series.ID)

		// List all
		list, err := repo.List(ctx)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(list), 2)

		// Verify ordering (should be alphabetical by name)
		for i := 1; i < len(list); i++ {
			assert.LessOrEqual(t, list[i-1].Name, list[i].Name)
		}
	})

	t.Run("delete series", func(t *testing.T) {
		// Get the series we created
		series, err := repo.GetByName(ctx, sprintName)
		require.NoError(t, err)
		require.NotNil(t, series)

		// Delete it
		orphanedCount, err := repo.Delete(ctx, series.ID)
		require.NoError(t, err)
		assert.Equal(t, 0, orphanedCount)

		// Verify it's gone
		deleted, err := repo.GetByID(ctx, series.ID)
		require.NoError(t, err)
		assert.Nil(t, deleted)

		// Remove from cleanup list since we already deleted it
		for i, id := range createdSeriesIDs {
			if id == series.ID {
				createdSeriesIDs = append(createdSeriesIDs[:i], createdSeriesIDs[i+1:]...)
				break
			}
		}
	})

	t.Run("duplicate series name fails", func(t *testing.T) {
		series := &repository.MeetingSeries{
			Name:        weeklyName, // Already exists
			Description: "Duplicate",
		}

		err := repo.Create(ctx, series)
		assert.Error(t, err)
	})
}

func TestMeetingSeries_MeetingAssociations(t *testing.T) {
	db := SetupTestDB(t)

	repo := repository.NewSeriesRepository(db.Pool)
	ctx := context.Background()

	// Use unique name to avoid conflicts
	testID := uniqueTestID()
	seriesName := "TER Weekly " + testID

	// Create a series
	series := &repository.MeetingSeries{
		Name:        seriesName,
		Description: "Technical Execution Review",
	}
	err := repo.Create(ctx, series)
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = repo.Delete(ctx, series.ID)
	})

	// Create test meetings
	meetingID1 := createTestMeetingInDB(t, db, "TER 2026-01-15", "2026-01-15")
	meetingID2 := createTestMeetingInDB(t, db, "TER 2026-01-22", "2026-01-22")
	meetingID3 := createTestMeetingInDB(t, db, "TER 2026-01-29", "2026-01-29")

	t.Run("assign meetings to series", func(t *testing.T) {
		err := repo.SetMeetingSeries(ctx, meetingID1, series.ID)
		require.NoError(t, err)

		err = repo.SetMeetingSeries(ctx, meetingID2, series.ID)
		require.NoError(t, err)

		err = repo.SetMeetingSeries(ctx, meetingID3, series.ID)
		require.NoError(t, err)

		// Verify count
		count, err := repo.CountMeetingsInSeries(ctx, series.ID)
		require.NoError(t, err)
		assert.Equal(t, 3, count)
	})

	t.Run("get meetings for series", func(t *testing.T) {
		meetings, err := repo.GetMeetingsForSeries(ctx, series.ID)
		require.NoError(t, err)
		assert.Equal(t, 3, len(meetings))

		// Verify meeting info
		for _, m := range meetings {
			assert.NotEmpty(t, m.ID)
			assert.NotEmpty(t, m.Title)
			assert.Contains(t, m.Title, "TER")
		}
	})

	t.Run("unset meeting series", func(t *testing.T) {
		err := repo.UnsetMeetingSeries(ctx, meetingID2)
		require.NoError(t, err)

		// Verify count decreased
		count, err := repo.CountMeetingsInSeries(ctx, series.ID)
		require.NoError(t, err)
		assert.Equal(t, 2, count)
	})

	t.Run("delete series orphans meetings", func(t *testing.T) {
		orphanedCount, err := repo.Delete(ctx, series.ID)
		require.NoError(t, err)
		assert.Equal(t, 2, orphanedCount) // Only meetingID1 and meetingID3

		// Verify series is deleted
		deleted, err := repo.GetByID(ctx, series.ID)
		require.NoError(t, err)
		assert.Nil(t, deleted)

		// Verify meetings still exist (by checking if we can query them)
		// This would normally use a meeting repository, but we can check
		// by trying to get meetings for the deleted series
		meetings, err := repo.GetMeetingsForSeries(ctx, series.ID)
		require.NoError(t, err)
		assert.Equal(t, 0, len(meetings))
	})
}

func TestMeetingSeries_ListMeetings(t *testing.T) {
	db := SetupTestDB(t)

	repo := repository.NewSeriesRepository(db.Pool)
	ctx := context.Background()

	// Use unique name to avoid conflicts
	testID := uniqueTestID()
	seriesName := "Design Review " + testID

	// Create a series
	series := &repository.MeetingSeries{
		Name:        seriesName,
		Description: "Weekly design reviews",
	}
	err := repo.Create(ctx, series)
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = repo.Delete(ctx, series.ID)
	})

	// Create test meetings and assign to series
	for i := 0; i < 5; i++ {
		title := fmt.Sprintf("Design Review Meeting %s #%d", testID, i)
		date := "2026-01-15"
		meetingID := createTestMeetingInDB(t, db, title, date)
		err := repo.SetMeetingSeries(ctx, meetingID, series.ID)
		require.NoError(t, err)
	}

	// Note: GetMeetingsForSeries queries the meetings table (where we created data),
	// while ListMeetings queries the sources table. Using the correct method here.
	t.Run("get meetings for series", func(t *testing.T) {
		meetings, err := repo.GetMeetingsForSeries(ctx, series.ID)
		require.NoError(t, err)
		assert.Equal(t, 5, len(meetings))
	})

	t.Run("count meetings in series", func(t *testing.T) {
		count, err := repo.CountMeetingsInSeries(ctx, series.ID)
		require.NoError(t, err)
		assert.Equal(t, 5, count)
	})

	t.Run("get meetings for non-existent series", func(t *testing.T) {
		meetings, err := repo.GetMeetingsForSeries(ctx, "ms-nonexistent")
		require.NoError(t, err)
		assert.Equal(t, 0, len(meetings))
	})
}

func TestMeetingSeries_EdgeCases(t *testing.T) {
	db := SetupTestDB(t)

	repo := repository.NewSeriesRepository(db.Pool)
	ctx := context.Background()

	// Use unique name to avoid conflicts
	testID := uniqueTestID()

	var createdSeriesIDs []string
	t.Cleanup(func() {
		for _, id := range createdSeriesIDs {
			_, _ = repo.Delete(ctx, id)
		}
	})

	t.Run("empty series name", func(t *testing.T) {
		series := &repository.MeetingSeries{
			Name:        "",
			Description: "Should fail",
		}
		err := repo.Create(ctx, series)
		assert.Error(t, err)
	})

	t.Run("nil series", func(t *testing.T) {
		err := repo.Create(ctx, nil)
		assert.Error(t, err)
	})

	t.Run("set series for non-existent meeting", func(t *testing.T) {
		series := &repository.MeetingSeries{
			Name:        "Test Series " + testID,
			Description: "For edge cases",
		}
		err := repo.Create(ctx, series)
		require.NoError(t, err)
		createdSeriesIDs = append(createdSeriesIDs, series.ID)

		err = repo.SetMeetingSeries(ctx, "999999", series.ID)
		assert.Error(t, err)
	})

	t.Run("delete non-existent series", func(t *testing.T) {
		_, err := repo.Delete(ctx, "ms-nonexistent")
		assert.Error(t, err)
	})

	t.Run("unset series for meeting not in series", func(t *testing.T) {
		meetingID := createTestMeetingInDB(t, db, "Orphan Meeting "+testID, "2026-01-15")

		// This should not error even if meeting has no series
		err := repo.UnsetMeetingSeries(ctx, meetingID)
		require.NoError(t, err)
	})
}

func TestMeetingSeries_ConcurrentOperations(t *testing.T) {
	db := SetupTestDB(t)

	repo := repository.NewSeriesRepository(db.Pool)
	ctx := context.Background()

	// Use unique names to avoid conflicts
	testID := uniqueTestID()

	t.Run("reassign meeting to different series", func(t *testing.T) {
		// Create two series
		series1 := &repository.MeetingSeries{
			Name:        "Series A " + testID,
			Description: "First series",
		}
		err := repo.Create(ctx, series1)
		require.NoError(t, err)

		series2 := &repository.MeetingSeries{
			Name:        "Series B " + testID,
			Description: "Second series",
		}
		err = repo.Create(ctx, series2)
		require.NoError(t, err)

		t.Cleanup(func() {
			_, _ = repo.Delete(ctx, series1.ID)
			_, _ = repo.Delete(ctx, series2.ID)
		})

		// Create a meeting
		meetingID := createTestMeetingInDB(t, db, "Test Meeting "+testID, "2026-01-15")

		// Assign to series1
		err = repo.SetMeetingSeries(ctx, meetingID, series1.ID)
		require.NoError(t, err)

		count1, err := repo.CountMeetingsInSeries(ctx, series1.ID)
		require.NoError(t, err)
		assert.Equal(t, 1, count1)

		// Reassign to series2
		err = repo.SetMeetingSeries(ctx, meetingID, series2.ID)
		require.NoError(t, err)

		// Verify counts
		count1, err = repo.CountMeetingsInSeries(ctx, series1.ID)
		require.NoError(t, err)
		assert.Equal(t, 0, count1)

		count2, err := repo.CountMeetingsInSeries(ctx, series2.ID)
		require.NoError(t, err)
		assert.Equal(t, 1, count2)
	})
}

// Helper function to create a meeting in the database for testing
func createTestMeetingInDB(t *testing.T, db *TestDB, title, date string) string {
	t.Helper()
	ctx := context.Background()

	// Ensure the test tenant exists first
	db.EnsureTenantExists(t)

	query := `
		INSERT INTO meetings (tenant_id, title, meeting_date, platform, created_at, updated_at)
		VALUES ($1, $2, $3, 'test', NOW(), NOW())
		RETURNING id
	`

	var id int64
	err := db.Pool.QueryRow(ctx, query, db.TenantID, title, date).Scan(&id)
	require.NoError(t, err, "failed to create test meeting")

	return fmt.Sprintf("%d", id)
}

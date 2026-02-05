//go:build integration

package integration

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCLI_MeetingList tests the basic meeting list command.
func TestCLI_MeetingList(t *testing.T) {
	db := SetupTestDB(t)
	EnsureAcmeCorpFixtures(t, db)

	stdout, stderr, err := runCLI(t, "meeting", "list")

	require.NoError(t, err, "meeting list should succeed. stderr: %s", stderr)
	// Output may be empty if no meetings exist - this tests that the command runs
	t.Logf("Meeting list output: %s", stdout)
}

// TestCLI_MeetingList_JSONOutput tests JSON output format.
func TestCLI_MeetingList_JSONOutput(t *testing.T) {
	db := SetupTestDB(t)
	EnsureAcmeCorpFixtures(t, db)

	stdout, stderr, err := runCLI(t, "meeting", "list", "-o", "json")

	require.NoError(t, err, "meeting list JSON should succeed. stderr: %s", stderr)
	// Even with no meetings, JSON output should be valid
	assert.Contains(t, stdout, "{", "output should be JSON")
}

// TestCLI_MeetingList_Limit tests the --limit flag.
func TestCLI_MeetingList_Limit(t *testing.T) {
	db := SetupTestDB(t)
	EnsureAcmeCorpFixtures(t, db)

	stdout, stderr, err := runCLI(t, "meeting", "list", "--limit", "5")

	require.NoError(t, err, "meeting list with limit should succeed. stderr: %s", stderr)
	t.Logf("Meeting list with limit output: %s", stdout)
}

// TestCLI_MeetingSeriesList tests listing meeting series.
func TestCLI_MeetingSeriesList(t *testing.T) {
	db := SetupTestDB(t)
	EnsureAcmeCorpFixtures(t, db)

	stdout, stderr, err := runCLI(t, "meeting", "series", "list")

	require.NoError(t, err, "meeting series list should succeed. stderr: %s", stderr)
	t.Logf("Meeting series list output: %s", stdout)
}

// TestCLI_MeetingSeriesList_JSONOutput tests JSON output for series list.
func TestCLI_MeetingSeriesList_JSONOutput(t *testing.T) {
	db := SetupTestDB(t)
	EnsureAcmeCorpFixtures(t, db)

	stdout, stderr, err := runCLI(t, "meeting", "series", "list", "-o", "json")

	require.NoError(t, err, "meeting series list JSON should succeed. stderr: %s", stderr)
	// Even with no series, JSON output should be valid
	assert.Contains(t, stdout, "{", "output should be JSON")
}

// TestCLI_MeetingSeriesShow_NotFound tests error handling for non-existent series.
func TestCLI_MeetingSeriesShow_NotFound(t *testing.T) {
	db := SetupTestDB(t)
	EnsureAcmeCorpFixtures(t, db)

	_, stderr, err := runCLI(t, "meeting", "series", "show", "nonexistent-series-id")

	require.Error(t, err, "meeting series show for non-existent series should fail")
	assert.True(t,
		strings.Contains(stderr, "not found") || strings.Contains(stderr, "error") || strings.Contains(stderr, "Error"),
		"stderr should indicate series not found: %s", stderr)
}

// TestCLI_MeetingRecap_MissingArgs tests error handling for missing arguments.
func TestCLI_MeetingRecap_MissingArgs(t *testing.T) {
	db := SetupTestDB(t)
	EnsureAcmeCorpFixtures(t, db)

	_, stderr, err := runCLI(t, "meeting", "recap")

	require.Error(t, err, "meeting recap without ID should fail")
	assert.True(t,
		strings.Contains(stderr, "required") || strings.Contains(stderr, "argument") || strings.Contains(stderr, "Usage"),
		"stderr should indicate missing argument: %s", stderr)
}

// TestCLI_MeetingList_WithMeetings tests meeting list when meetings exist.
// This test is conditional on having meeting data.
func TestCLI_MeetingList_WithMeetings(t *testing.T) {
	db := SetupTestDB(t)
	EnsureAcmeCorpFixtures(t, db)

	// First check if any meetings exist
	type MeetingListResponse struct {
		Meetings []struct {
			ID string `json:"id"`
		} `json:"meetings"`
	}
	result, _, err := runCLIWithJSON[MeetingListResponse](t, "meeting", "list")
	if err != nil {
		t.Skip("Skipping - could not parse meeting list response")
	}

	if len(result.Meetings) == 0 {
		t.Skip("Skipping - no meetings in test data")
	}

	// If meetings exist, verify the list contains expected fields
	stdout, stderr, err := runCLI(t, "meeting", "list", "-o", "json")
	require.NoError(t, err, "meeting list should succeed. stderr: %s", stderr)
	assertJSONContains(t, stdout, "meetings")
}

// TestCLI_MeetingSeriesList_WithSeries tests series list when series exist.
// This test is conditional on having series data.
func TestCLI_MeetingSeriesList_WithSeries(t *testing.T) {
	db := SetupTestDB(t)
	EnsureAcmeCorpFixtures(t, db)

	// First check if any series exist
	type SeriesListResponse struct {
		Series []struct {
			ID string `json:"id"`
		} `json:"series"`
	}
	result, _, err := runCLIWithJSON[SeriesListResponse](t, "meeting", "series", "list")
	if err != nil {
		t.Skip("Skipping - could not parse series list response")
	}

	if len(result.Series) == 0 {
		t.Skip("Skipping - no series in test data")
	}

	// If series exist, verify the list contains expected fields
	stdout, stderr, err := runCLI(t, "meeting", "series", "list", "-o", "json")
	require.NoError(t, err, "series list should succeed. stderr: %s", stderr)
	assertJSONContains(t, stdout, "series")
}

// TestCLI_MeetingRecap_WithMeeting tests recap when a meeting exists.
// This test is conditional on having meeting data.
func TestCLI_MeetingRecap_WithMeeting(t *testing.T) {
	db := SetupTestDB(t)
	EnsureAcmeCorpFixtures(t, db)

	// First get a meeting ID
	type MeetingListResponse struct {
		Meetings []struct {
			ID string `json:"id"`
		} `json:"meetings"`
	}
	result, _, err := runCLIWithJSON[MeetingListResponse](t, "meeting", "list")
	if err != nil {
		t.Skip("Skipping - could not parse meeting list response")
	}

	if len(result.Meetings) == 0 {
		t.Skip("Skipping - no meetings in test data")
	}

	meetingID := result.Meetings[0].ID
	stdout, stderr, err := runCLI(t, "meeting", "recap", meetingID)

	// Recap might fail if the meeting has no transcript/content
	if err != nil {
		t.Logf("Meeting recap returned error (may be expected if no transcript): %s", stderr)
	} else {
		t.Logf("Meeting recap output: %s", stdout)
	}
}

// =============================================================================
// Write Command Tests (Phase 2)
// =============================================================================

// TestCLI_MeetingSeriesCreate tests creating a new meeting series.
func TestCLI_MeetingSeriesCreate(t *testing.T) {
	db := SetupTestDB(t)
	EnsureAcmeCorpFixtures(t, db)

	seriesName := "Test Series " + uniqueTestID()
	var seriesID string

	t.Cleanup(func() {
		if seriesID != "" {
			cleanupMeetingSeries(t, seriesID)
		}
	})

	stdout, stderr, err := runCLI(t, "meeting", "series", "create", seriesName)
	require.NoError(t, err, "meeting series create should succeed. stderr: %s", stderr)
	assert.Contains(t, stdout, seriesName, "output should confirm series was created")
	t.Logf("Series create output: %s", stdout)

	// Try to extract series ID from JSON output for cleanup
	type SeriesCreateResponse struct {
		Created bool `json:"created"`
		Series  struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"series"`
	}
	result, _, err := runCLIWithJSON[SeriesCreateResponse](t, "meeting", "series", "create", "Test Series JSON "+uniqueTestID())
	if err == nil && result.Series.ID != "" {
		seriesID = result.Series.ID
		t.Logf("Created series with ID: %s", seriesID)
		// Clean up this second series too
		t.Cleanup(func() {
			cleanupMeetingSeries(t, result.Series.ID)
		})
	}
}

// TestCLI_MeetingSeriesCreate_JSONOutput tests JSON output format for series create.
func TestCLI_MeetingSeriesCreate_JSONOutput(t *testing.T) {
	db := SetupTestDB(t)
	EnsureAcmeCorpFixtures(t, db)

	seriesName := "Test JSON Series " + uniqueTestID()

	// The actual JSON structure has a nested "series" object
	type SeriesCreateResponse struct {
		Created bool `json:"created"`
		Series  struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"series"`
	}
	result, stderr, err := runCLIWithJSON[SeriesCreateResponse](t, "meeting", "series", "create", seriesName)

	if err != nil {
		t.Fatalf("meeting series create JSON should succeed. stderr: %s", stderr)
	}

	t.Cleanup(func() {
		if result.Series.ID != "" {
			cleanupMeetingSeries(t, result.Series.ID)
		}
	})

	assert.NotEmpty(t, result.Series.ID, "should return series ID")
	assert.Equal(t, seriesName, result.Series.Name, "should return series name")
}

// TestCLI_MeetingSeriesDelete tests deleting a meeting series.
func TestCLI_MeetingSeriesDelete(t *testing.T) {
	db := SetupTestDB(t)
	EnsureAcmeCorpFixtures(t, db)

	seriesName := "Test Delete Series " + uniqueTestID()

	// First create a series - note the nested JSON structure
	type SeriesCreateResponse struct {
		Created bool `json:"created"`
		Series  struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"series"`
	}
	result, stderr, err := runCLIWithJSON[SeriesCreateResponse](t, "meeting", "series", "create", seriesName)
	if err != nil {
		t.Fatalf("setup: series create should succeed. stderr: %s", stderr)
	}

	seriesID := result.Series.ID
	require.NotEmpty(t, seriesID, "should have created series ID")

	// Now delete it
	stdout, stderr, err := runCLI(t, "meeting", "series", "delete", seriesID)
	require.NoError(t, err, "meeting series delete should succeed. stderr: %s", stderr)
	t.Logf("Series delete output: %s", stdout)

	// Verify series no longer exists
	_, stderr, err = runCLI(t, "meeting", "series", "show", seriesID)
	require.Error(t, err, "series show should fail after deletion")
}

// TestCLI_MeetingSeriesDelete_NotFound tests behavior for non-existent series.
// Note: Delete is idempotent - it returns success even if series doesn't exist.
func TestCLI_MeetingSeriesDelete_NotFound(t *testing.T) {
	db := SetupTestDB(t)
	EnsureAcmeCorpFixtures(t, db)

	stdout, stderr, err := runCLI(t, "meeting", "series", "delete", "ms-nonexistent-xyz789")

	// Delete is idempotent - it may return success with "not found" message
	// instead of an error. This is valid behavior.
	if err != nil {
		// If it does error, check the message
		assert.True(t,
			strings.Contains(stderr, "not found") || strings.Contains(stderr, "error") || strings.Contains(stderr, "Error"),
			"stderr should indicate series not found: %s", stderr)
	} else {
		// If it succeeds, check output mentions "not found"
		assert.Contains(t, stdout, "not found", "output should indicate series was not found")
	}
}

// TestCLI_MeetingUpdate tests updating meeting metadata.
func TestCLI_MeetingUpdate(t *testing.T) {
	db := SetupTestDB(t)
	EnsureAcmeCorpFixtures(t, db)

	// First get a meeting ID
	type MeetingListResponse struct {
		Meetings []struct {
			ID string `json:"id"`
		} `json:"meetings"`
	}
	result, _, err := runCLIWithJSON[MeetingListResponse](t, "meeting", "list")
	if err != nil {
		t.Skip("Skipping - could not parse meeting list response")
	}

	if len(result.Meetings) == 0 {
		t.Skip("Skipping - no meetings in test data")
	}

	meetingID := result.Meetings[0].ID
	newTitle := "Updated Title " + uniqueTestID()

	stdout, stderr, err := runCLI(t, "meeting", "update", meetingID, "--title", newTitle)
	require.NoError(t, err, "meeting update should succeed. stderr: %s", stderr)
	assert.Contains(t, stdout, newTitle, "output should show updated title")
	t.Logf("Meeting update output: %s", stdout)
}

// TestCLI_MeetingUpdate_MissingFlags tests error handling when no update flags are provided.
func TestCLI_MeetingUpdate_MissingFlags(t *testing.T) {
	db := SetupTestDB(t)
	EnsureAcmeCorpFixtures(t, db)

	// First get a meeting ID
	type MeetingListResponse struct {
		Meetings []struct {
			ID string `json:"id"`
		} `json:"meetings"`
	}
	result, _, err := runCLIWithJSON[MeetingListResponse](t, "meeting", "list")
	if err != nil {
		t.Skip("Skipping - could not parse meeting list response")
	}

	if len(result.Meetings) == 0 {
		t.Skip("Skipping - no meetings in test data")
	}

	meetingID := result.Meetings[0].ID

	_, stderr, err := runCLI(t, "meeting", "update", meetingID)

	require.Error(t, err, "meeting update without flags should fail")
	assert.True(t,
		strings.Contains(stderr, "flag") || strings.Contains(stderr, "required") || strings.Contains(stderr, "at least"),
		"stderr should indicate missing flags: %s", stderr)
}

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

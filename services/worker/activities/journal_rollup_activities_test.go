package activities

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestJournalRollupGatherInput_JSONRoundtrip verifies the input type uses the correct
// JSON field names to match the workflow package's unexported journalRollupGatherInput.
func TestJournalRollupGatherInput_JSONRoundtrip(t *testing.T) {
	input := JournalRollupGatherInput{
		TenantID:      "tenant-abc",
		ReferenceDate: "2026-03-06",
	}

	data, err := json.Marshal(input)
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))
	require.Contains(t, raw, "tenant_id")
	require.Contains(t, raw, "reference_date")

	var decoded JournalRollupGatherInput
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Equal(t, input.TenantID, decoded.TenantID)
	require.Equal(t, input.ReferenceDate, decoded.ReferenceDate)
}

// TestJournalRollupGatherOutput_JSONRoundtrip verifies the output type field names.
func TestJournalRollupGatherOutput_JSONRoundtrip(t *testing.T) {
	out := JournalRollupGatherOutput{
		LedgerEntries:     json.RawMessage(`[{"id":1,"category":"work"}]`),
		Consolidations:    json.RawMessage(`[]`),
		ExistingSummaries: json.RawMessage(`[]`),
		HasContent:        true,
		DailyCount:        3,
	}

	data, err := json.Marshal(out)
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))
	require.Contains(t, raw, "ledger_entries")
	require.Contains(t, raw, "consolidations")
	require.Contains(t, raw, "existing_summaries")
	require.Contains(t, raw, "has_content")
	require.Contains(t, raw, "daily_count")

	var decoded JournalRollupGatherOutput
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.True(t, decoded.HasContent)
	require.Equal(t, 3, decoded.DailyCount)
}

// TestJournalRollupDailyInput_JSONRoundtrip verifies daily summary input field names.
func TestJournalRollupDailyInput_JSONRoundtrip(t *testing.T) {
	input := JournalRollupDailyInput{
		TenantID:          "tenant-abc",
		ReferenceDate:     "2026-03-06",
		LedgerEntries:     json.RawMessage(`[{"id":1}]`),
		Consolidations:    json.RawMessage(`[]`),
		ExistingSummaries: json.RawMessage(`[]`),
	}

	data, err := json.Marshal(input)
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))
	require.Contains(t, raw, "tenant_id")
	require.Contains(t, raw, "reference_date")
	require.Contains(t, raw, "ledger_entries")
	require.Contains(t, raw, "consolidations")
	require.Contains(t, raw, "existing_summaries")
}

// TestJournalRollupDailyOutput_JSONRoundtrip verifies daily summary output field names.
func TestJournalRollupDailyOutput_JSONRoundtrip(t *testing.T) {
	out := JournalRollupDailyOutput{
		DigestID:         "digest-abc",
		Body:             json.RawMessage(`{"summary":"test"}`),
		ModelUsed:        "claude-3-haiku",
		PromptTemplateID: 42,
		InputTokenCount:  200,
		OutputTokenCount: 80,
	}

	data, err := json.Marshal(out)
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))
	require.Contains(t, raw, "digest_id")
	require.Contains(t, raw, "body")
	require.Contains(t, raw, "model_used")
	require.Contains(t, raw, "prompt_template_id")
	require.Contains(t, raw, "input_token_count")
	require.Contains(t, raw, "output_token_count")
}

// TestJournalRollupWeeklyOutput_SkippedFlag verifies the skipped flag round-trips correctly.
func TestJournalRollupWeeklyOutput_SkippedFlag(t *testing.T) {
	out := JournalRollupWeeklyOutput{
		Skipped: true,
	}

	data, err := json.Marshal(out)
	require.NoError(t, err)

	var decoded JournalRollupWeeklyOutput
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.True(t, decoded.Skipped)
	require.Empty(t, decoded.DigestID)
}

// TestJournalRollupOverviewInput_JSONRoundtrip verifies overview input field names.
func TestJournalRollupOverviewInput_JSONRoundtrip(t *testing.T) {
	input := JournalRollupOverviewInput{
		TenantID:       "tenant-abc",
		ReferenceDate:  "2026-03-06",
		WeeklyDigestID: "weekly-digest-123",
		WeeklyBody:     json.RawMessage(`{"summary":"weekly summary"}`),
	}

	data, err := json.Marshal(input)
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))
	require.Contains(t, raw, "tenant_id")
	require.Contains(t, raw, "reference_date")
	require.Contains(t, raw, "weekly_digest_id")
	require.Contains(t, raw, "weekly_body")

	var decoded JournalRollupOverviewInput
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Equal(t, input.WeeklyDigestID, decoded.WeeklyDigestID)
}

// TestJournalRollupOverviewOutput_JSONRoundtrip verifies overview output field names.
func TestJournalRollupOverviewOutput_JSONRoundtrip(t *testing.T) {
	out := JournalRollupOverviewOutput{
		DigestID:         "overview-abc",
		Body:             json.RawMessage(`{"summary":"overview"}`),
		ModelUsed:        "claude-3-haiku",
		InputTokenCount:  300,
		OutputTokenCount: 100,
	}

	data, err := json.Marshal(out)
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))
	require.Contains(t, raw, "digest_id")
	require.Contains(t, raw, "body")
	require.Contains(t, raw, "model_used")
	require.Contains(t, raw, "input_token_count")
	require.Contains(t, raw, "output_token_count")
}

// TestWeekMonday verifies the weekMonday helper returns the correct Monday for various inputs.
func TestWeekMonday(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"monday", "2026-03-02", "2026-03-02"},     // already Monday
		{"tuesday", "2026-03-03", "2026-03-02"},    // Tuesday → Monday
		{"wednesday", "2026-03-04", "2026-03-02"},  // Wednesday → Monday
		{"thursday", "2026-03-05", "2026-03-02"},   // Thursday → Monday
		{"friday", "2026-03-06", "2026-03-02"},     // Friday → Monday
		{"saturday", "2026-03-07", "2026-03-02"},   // Saturday → Monday
		{"sunday", "2026-03-08", "2026-03-02"},     // Sunday → Monday (same week)
		{"next_monday", "2026-03-09", "2026-03-09"}, // next Monday
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d, err := time.Parse("2006-01-02", tc.input)
			require.NoError(t, err)

			monday := weekMonday(d)
			require.Equal(t, tc.expected, monday.Format("2006-01-02"),
				"input %s should give Monday %s", tc.input, tc.expected)
		})
	}
}

// TestFormatDigestSummaries_Empty verifies empty/nil slice returns "None".
func TestFormatDigestSummaries_Empty(t *testing.T) {
	result := formatDigestSummaries(nil)
	require.Equal(t, "None", result)
}

// TestFormatLedgerEntriesFromJSON_Empty verifies empty/null JSON returns "None".
func TestFormatLedgerEntriesFromJSON_Empty(t *testing.T) {
	require.Equal(t, "None", formatLedgerEntriesFromJSON(nil))
	require.Equal(t, "None", formatLedgerEntriesFromJSON(json.RawMessage(`null`)))
	require.Equal(t, "None", formatLedgerEntriesFromJSON(json.RawMessage(`[]`)))
}

// TestFormatRawJSONAsText_Empty verifies empty/null JSON returns "None".
func TestFormatRawJSONAsText_Empty(t *testing.T) {
	require.Equal(t, "None", formatRawJSONAsText(nil, "test"))
	require.Equal(t, "None", formatRawJSONAsText(json.RawMessage(`null`), "test"))
	require.Equal(t, "None", formatRawJSONAsText(json.RawMessage(`[]`), "test"))
}

// TestFormatRawJSONAsText_ValidJSON verifies valid JSON is pretty-printed.
func TestFormatRawJSONAsText_ValidJSON(t *testing.T) {
	input := json.RawMessage(`{"key":"value","nested":{"a":1}}`)
	result := formatRawJSONAsText(input, "test")
	require.NotEqual(t, "None", result)
	require.Contains(t, result, "key")
}


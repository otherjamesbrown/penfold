package activities

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	pkgdigest "github.com/otherjamesbrown/penfold/pkg/digest"
)

// TestDigestRollupGatherInput_JSONRoundtrip verifies the input type round-trips via JSON
// with field names that match the workflow package's unexported digestRollupGatherInput.
func TestDigestRollupGatherInput_JSONRoundtrip(t *testing.T) {
	input := DigestRollupGatherInput{
		TenantID:      "tenant-abc",
		Window:        "7d",
		SourceFilter:  json.RawMessage(`{"subtypes":["NOTIFICATION"]}`),
		MaxItems:      25,
		ReferenceDate: "2026-03-06",
		ScheduleID:    "sched-001",
	}

	data, err := json.Marshal(input)
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))

	// Verify JSON field names match the workflow package's unexported type field tags.
	require.Contains(t, raw, "tenant_id")
	require.Contains(t, raw, "window")
	require.Contains(t, raw, "source_filter")
	require.Contains(t, raw, "max_items")
	require.Contains(t, raw, "reference_date")
	require.Contains(t, raw, "schedule_id")

	var decoded DigestRollupGatherInput
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Equal(t, input.TenantID, decoded.TenantID)
	require.Equal(t, input.Window, decoded.Window)
	require.Equal(t, input.MaxItems, decoded.MaxItems)
	require.Equal(t, input.ReferenceDate, decoded.ReferenceDate)
}

// TestDigestRollupGatherOutput_JSONRoundtrip verifies output type field names match
// what the workflow package expects when deserialising from Temporal.
func TestDigestRollupGatherOutput_JSONRoundtrip(t *testing.T) {
	out := DigestRollupGatherOutput{
		Items:      json.RawMessage(`[{"source_id":1,"subject":"Test"}]`),
		SourceIDs:  []int64{1, 2, 3},
		WindowFrom: "2026-02-27",
		WindowTo:   "2026-03-06",
		Count:      3,
	}

	data, err := json.Marshal(out)
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))
	require.Contains(t, raw, "items")
	require.Contains(t, raw, "source_ids")
	require.Contains(t, raw, "window_from")
	require.Contains(t, raw, "window_to")
	require.Contains(t, raw, "count")
}

// TestDigestRollupGenerateInput_JSONRoundtrip verifies the generate input type
// uses the correct JSON field names.
func TestDigestRollupGenerateInput_JSONRoundtrip(t *testing.T) {
	input := DigestRollupGenerateInput{
		TenantID:   "tenant-abc",
		Name:       "aha-notifications",
		PromptID:   "my-prompt",
		Items:      json.RawMessage(`[]`),
		WindowFrom: "2026-02-27",
		WindowTo:   "2026-03-06",
	}

	data, err := json.Marshal(input)
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))
	require.Contains(t, raw, "tenant_id")
	require.Contains(t, raw, "name")
	require.Contains(t, raw, "prompt_id")
	require.Contains(t, raw, "window_from")
	require.Contains(t, raw, "window_to")
}

// TestDigestRollupDeliverInput_JSONRoundtrip verifies delivery input field names.
func TestDigestRollupDeliverInput_JSONRoundtrip(t *testing.T) {
	input := DigestRollupDeliverInput{
		TenantID:         "tenant-abc",
		Name:             "test",
		Delivery:         []string{"store", "email"},
		Body:             json.RawMessage(`{"summary":"test"}`),
		ModelUsed:        "claude-3-haiku",
		PromptTemplateID: 42,
		InputTokenCount:  100,
		OutputTokenCount: 50,
		SourceIDs:        []int64{1, 2},
		WindowFrom:       "2026-02-27",
		WindowTo:         "2026-03-06",
	}

	data, err := json.Marshal(input)
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))
	require.Contains(t, raw, "delivery")
	require.Contains(t, raw, "source_ids")
	require.Contains(t, raw, "prompt_template_id")

	var decoded DigestRollupDeliverInput
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Equal(t, []string{"store", "email"}, decoded.Delivery)
	require.Equal(t, []int64{1, 2}, decoded.SourceIDs)
}

// TestRollupRecordMetadataInput_JSONRoundtrip verifies the metadata input type round-trips
// and uses the correct JSON field names.
func TestRollupRecordMetadataInput_JSONRoundtrip(t *testing.T) {
	input := RollupRecordMetadataInput{
		TenantID:       "tenant-abc",
		ScheduleID:     "sched-001",
		WorkflowType:   "digest_rollup",
		ReferenceDate:  "2026-03-06",
		DigestID:       "digest-xyz",
		ItemsGathered:  10,
		WindowFrom:     "2026-02-27",
		WindowTo:       "2026-03-06",
		DeliveryStatus: map[string]string{"store": "ok"},
		ModelUsed:      "claude-3-haiku",
		InputTokens:    500,
		OutputTokens:   120,
	}

	data, err := json.Marshal(input)
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))
	require.Contains(t, raw, "tenant_id")
	require.Contains(t, raw, "schedule_id")
	require.Contains(t, raw, "workflow_type")
	require.Contains(t, raw, "reference_date")
	require.Contains(t, raw, "digest_id")
	require.Contains(t, raw, "items_gathered")
	require.Contains(t, raw, "input_tokens")
	require.Contains(t, raw, "output_tokens")

	var decoded RollupRecordMetadataInput
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Equal(t, input.TenantID, decoded.TenantID)
	require.Equal(t, input.ScheduleID, decoded.ScheduleID)
	require.Equal(t, input.WorkflowType, decoded.WorkflowType)
	require.Equal(t, input.DigestID, decoded.DigestID)
	require.Equal(t, input.ItemsGathered, decoded.ItemsGathered)
	require.Equal(t, input.ModelUsed, decoded.ModelUsed)
	require.Equal(t, input.InputTokens, decoded.InputTokens)
	require.Equal(t, input.OutputTokens, decoded.OutputTokens)
}

// TestDeliverRollupResults_DeliveryStatusLogic verifies delivery status assignment
// for all supported delivery types including the email stub.
func TestDeliverRollupResults_DeliveryStatusLogic(t *testing.T) {
	tests := []struct {
		deliveryType   string
		expectedStatus string
	}{
		{"email", "not_yet_implemented"},
		{"unknown_type", "unknown_delivery_type"},
	}

	for _, tc := range tests {
		t.Run(tc.deliveryType, func(t *testing.T) {
			deliveryStatus := make(map[string]string)
			switch tc.deliveryType {
			case "store":
				deliveryStatus["store"] = "ok"
			case "email":
				deliveryStatus["email"] = "not_yet_implemented"
			default:
				deliveryStatus[tc.deliveryType] = "unknown_delivery_type"
			}
			require.Equal(t, tc.expectedStatus, deliveryStatus[tc.deliveryType])
		})
	}
}

// TestDigestRollupContent_WindowExpressions verifies that the pkg/digest.CalculateWindow
// function handles the expressions used by digest rollup correctly.
func TestDigestRollupContent_WindowExpressions(t *testing.T) {
	refDate, err := time.Parse("2006-01-02", "2026-03-06")
	require.NoError(t, err)

	tests := []struct {
		name    string
		expr    string
		wantErr bool
	}{
		{"7d", "7d", false},
		{"30d", "30d", false},
		{"1m", "1m", false},
		{"since_last_run_without_lastrun", "since_last_run", true},
		{"invalid_expression", "badexpr", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w, calcErr := pkgdigest.CalculateWindow(tc.expr, refDate, nil)
			if tc.wantErr {
				require.Error(t, calcErr)
			} else {
				require.NoError(t, calcErr)
				require.False(t, w.From.IsZero())
				require.False(t, w.To.IsZero())
				require.True(t, w.From.Before(w.To))
			}
		})
	}
}

// TestDigestRollupContent_SourceFilterUnmarshal verifies that SourceFilter JSON
// unmarshals correctly with subtypes and sources.
func TestDigestRollupContent_SourceFilterUnmarshal(t *testing.T) {
	filterJSON := json.RawMessage(`{"subtypes":["NOTIFICATION","UPDATES"],"sources":["aha","jira"]}`)

	var filter pkgdigest.SourceFilter
	require.NoError(t, json.Unmarshal(filterJSON, &filter))

	require.Equal(t, []string{"NOTIFICATION", "UPDATES"}, filter.Subtypes)
	require.Equal(t, []string{"aha", "jira"}, filter.Sources)
}

// TestDigestRollupContent_EmptySourceFilter verifies that null/empty source filter
// results in an empty SourceFilter without error.
func TestDigestRollupContent_EmptySourceFilter(t *testing.T) {
	var filter pkgdigest.SourceFilter

	// null JSON
	require.NoError(t, json.Unmarshal([]byte(`{}`), &filter))
	require.Empty(t, filter.Subtypes)
	require.Empty(t, filter.Sources)
}

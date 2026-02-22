package pipeline

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	contentv1 "github.com/otherjamesbrown/penfold/api/proto/content/v1"
)

// ============ Test Helpers ============

func intPtr(i int) *int {
	return &i
}

func stringPtr(s string) *string {
	return &s
}

func makeRun(sourceID int64, stage, status string) PipelineRun {
	return PipelineRun{
		ID:        1,
		SourceID:  sourceID,
		Stage:     stage,
		Status:    status,
		CreatedAt: time.Now(),
	}
}

func makeRunWithTokens(sourceID int64, stage, status string, inputTokens, outputTokens int) PipelineRun {
	return PipelineRun{
		ID:           1,
		SourceID:     sourceID,
		Stage:        stage,
		Status:       status,
		CreatedAt:    time.Now(),
		InputTokens:  intPtr(inputTokens),
		OutputTokens: intPtr(outputTokens),
	}
}

func makeRunWithTriageData(sourceID int64, contribution, reason string) PipelineRun {
	triageData := map[string]interface{}{
		"content_contribution": contribution,
		"contribution_reason":  reason,
		"category":             "test_category",
		"importance":           "high",
	}
	parsedData, _ := json.Marshal(triageData)
	return PipelineRun{
		ID:         1,
		SourceID:   sourceID,
		Stage:      "triage",
		Status:     "completed",
		CreatedAt:  time.Now(),
		ParsedData: parsedData,
	}
}

// ============ BuildProcessingStatus Tests ============

// TestBuildProcessingStatus_EmptyRuns verifies that an empty slice of runs
// produces an empty result.
func TestBuildProcessingStatus_EmptyRuns(t *testing.T) {
	result := BuildProcessingStatus([]PipelineRun{})

	require.NotNil(t, result)
	assert.Empty(t, result.Stages)
	assert.Empty(t, result.ContentContribution)
	assert.Empty(t, result.ContributionReason)
	assert.Equal(t, int32(0), result.TotalInputTokens)
	assert.Equal(t, int32(0), result.TotalOutputTokens)
	assert.Nil(t, result.StartedAt)
	assert.Nil(t, result.FinishedAt)
}

// TestBuildProcessingStatus_SingleCompletedRun verifies that a single completed
// run produces a StageResult with the correct status.
func TestBuildProcessingStatus_SingleCompletedRun(t *testing.T) {
	runs := []PipelineRun{
		makeRunWithTokens(100, "triage", "completed", 50, 20),
	}

	result := BuildProcessingStatus(runs)

	require.NotNil(t, result)
	require.Len(t, result.Stages, 1)

	sr := result.Stages[0]
	assert.Equal(t, contentv1.ProcessingStage_PROCESSING_STAGE_TRIAGE, sr.Stage)
	assert.Equal(t, contentv1.StageStatus_STAGE_STATUS_COMPLETED, sr.Status)
	require.NotNil(t, sr.InputTokens)
	assert.Equal(t, int32(50), *sr.InputTokens)
	require.NotNil(t, sr.OutputTokens)
	assert.Equal(t, int32(20), *sr.OutputTokens)

	assert.Equal(t, int32(50), result.TotalInputTokens)
	assert.Equal(t, int32(20), result.TotalOutputTokens)
}

// TestBuildProcessingStatus_MultipleRuns verifies that multiple runs for
// different stages are all included, each with correct status.
func TestBuildProcessingStatus_MultipleRuns(t *testing.T) {
	runs := []PipelineRun{
		makeRunWithTokens(100, "parse", "completed", 0, 0),
		makeRunWithTriageData(100, "MEDIUM", "relevant content"),
		makeRunWithTokens(100, "analyze", "completed", 200, 80),
		makeRun(100, "persist", "completed"),
	}

	result := BuildProcessingStatus(runs)

	require.NotNil(t, result)
	assert.Len(t, result.Stages, 4)
	assert.Equal(t, "MEDIUM", result.ContentContribution)
	assert.Equal(t, "relevant content", result.ContributionReason)
	assert.Equal(t, "test_category", result.TriageCategory)
	assert.Equal(t, "high", result.TriageImportance)

	// Token totals are summed across all runs
	assert.Equal(t, int32(200), result.TotalInputTokens)
	assert.Equal(t, int32(80), result.TotalOutputTokens)
}

// TestBuildProcessingStatus_SkippedStage verifies that a skipped stage is
// represented correctly with STAGE_STATUS_SKIPPED.
func TestBuildProcessingStatus_SkippedStage(t *testing.T) {
	skipReason := "contribution_gating:LOW"
	runs := []PipelineRun{
		makeRun(100, "parse", "completed"),
		makeRunWithTriageData(100, "LOW", "not relevant"),
		{
			ID:         1,
			SourceID:   100,
			Stage:      "analyze",
			Status:     "skipped",
			CreatedAt:  time.Now(),
			SkipReason: &skipReason,
		},
		makeRun(100, "persist", "completed"),
	}

	result := BuildProcessingStatus(runs)

	require.NotNil(t, result)

	// Find the analyze stage
	var analyzeStage *contentv1.StageResult
	for _, sr := range result.Stages {
		if sr.Stage == contentv1.ProcessingStage_PROCESSING_STAGE_ANALYZE {
			analyzeStage = sr
			break
		}
	}

	require.NotNil(t, analyzeStage, "analyze stage must be present")
	assert.Equal(t, contentv1.StageStatus_STAGE_STATUS_SKIPPED, analyzeStage.Status)
	require.NotNil(t, analyzeStage.SkipReason)
	assert.Equal(t, skipReason, *analyzeStage.SkipReason)
	assert.Equal(t, "LOW", result.ContentContribution)
}

// TestBuildProcessingStatus_LatestRunWinsPerStage verifies that when there are
// multiple runs for the same stage (re-runs), only the latest one is included
// in the stages list. Runs are ordered DESC (newest first).
func TestBuildProcessingStatus_LatestRunWinsPerStage(t *testing.T) {
	now := time.Now()
	// Runs ordered newest first (DESC order, as from DB)
	runs := []PipelineRun{
		// Latest triage run (first = newest)
		{
			ID:           2,
			SourceID:     100,
			Stage:        "triage",
			Status:       "completed",
			CreatedAt:    now,
			InputTokens:  intPtr(120),
			OutputTokens: intPtr(40),
		},
		// Older triage run (second = older, should be ignored for stage result)
		{
			ID:           1,
			SourceID:     100,
			Stage:        "triage",
			Status:       "failed",
			CreatedAt:    now.Add(-1 * time.Hour),
			InputTokens:  intPtr(80),
			OutputTokens: intPtr(0),
		},
	}

	result := BuildProcessingStatus(runs)

	require.NotNil(t, result)
	// Only 1 stage entry (triage), not 2
	require.Len(t, result.Stages, 1)

	// The stage result should be from the newest run (completed)
	assert.Equal(t, contentv1.StageStatus_STAGE_STATUS_COMPLETED, result.Stages[0].Status)

	// Token totals should include BOTH runs
	assert.Equal(t, int32(200), result.TotalInputTokens, "token totals include all runs, not just latest")
	assert.Equal(t, int32(40), result.TotalOutputTokens, "token totals include all runs, not just latest")
}

// TestBuildProcessingStatus_TokenSummation verifies that token counts are
// accumulated across all runs, not just the latest per stage.
func TestBuildProcessingStatus_TokenSummation(t *testing.T) {
	runs := []PipelineRun{
		makeRunWithTokens(100, "triage", "completed", 50, 20),
		makeRunWithTokens(100, "extract_ner", "completed", 100, 40),
		makeRunWithTokens(100, "analyze", "completed", 200, 80),
	}

	result := BuildProcessingStatus(runs)

	require.NotNil(t, result)
	assert.Equal(t, int32(350), result.TotalInputTokens)
	assert.Equal(t, int32(140), result.TotalOutputTokens)
}

// TestBuildProcessingStatus_UnknownStageSkipped verifies that unknown stage
// names are skipped and don't appear in the stages list.
func TestBuildProcessingStatus_UnknownStageSkipped(t *testing.T) {
	runs := []PipelineRun{
		makeRun(100, "parse", "completed"),
		makeRun(100, "unknown_future_stage", "completed"),
		makeRun(100, "persist", "completed"),
	}

	result := BuildProcessingStatus(runs)

	require.NotNil(t, result)
	// Only 2 known stages; unknown stage is skipped
	assert.Len(t, result.Stages, 2)

	for _, sr := range result.Stages {
		assert.NotEqual(t, contentv1.ProcessingStage_PROCESSING_STAGE_UNSPECIFIED, sr.Stage,
			"no stage result should have UNSPECIFIED stage")
	}
}

// TestDBRunStatusToStageStatus verifies the status string to proto enum mapping.
func TestDBRunStatusToStageStatus(t *testing.T) {
	tests := []struct {
		input    string
		expected contentv1.StageStatus
	}{
		{"pending", contentv1.StageStatus_STAGE_STATUS_PENDING},
		{"running", contentv1.StageStatus_STAGE_STATUS_RUNNING},
		{"completed", contentv1.StageStatus_STAGE_STATUS_COMPLETED},
		{"failed", contentv1.StageStatus_STAGE_STATUS_FAILED},
		{"skipped", contentv1.StageStatus_STAGE_STATUS_SKIPPED},
		{"unknown", contentv1.StageStatus_STAGE_STATUS_UNSPECIFIED},
		{"", contentv1.StageStatus_STAGE_STATUS_UNSPECIFIED},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := DBRunStatusToStageStatus(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestApplyToProtoProcessingStatus_TriageEnums verifies that enum fields are
// populated alongside the deprecated string fields when applying triage data.
func TestApplyToProtoProcessingStatus_TriageEnums(t *testing.T) {
	ps := &ProcessingStatusResult{
		TriageCategory:   "ACTION_REQUEST",
		TriageImportance: "HIGH",
	}
	proto := &contentv1.ProcessingStatus{}

	ApplyToProtoProcessingStatus(ps, proto)

	// Deprecated string fields must still be populated.
	assert.Equal(t, "ACTION_REQUEST", proto.TriageCategory)
	assert.Equal(t, "HIGH", proto.TriageImportance)

	// New enum fields must also be populated.
	assert.Equal(t, contentv1.TriageCategory_TRIAGE_CATEGORY_ACTION_REQUEST, proto.TriageCategoryEnum)
	assert.Equal(t, contentv1.TriageImportance_TRIAGE_IMPORTANCE_HIGH, proto.TriageImportanceEnum)
}

// TestMapTriageCategoryToProto tests all triage category mappings including aliases.
func TestMapTriageCategoryToProto(t *testing.T) {
	cases := []struct {
		input    string
		expected contentv1.TriageCategory
	}{
		{"ACTION_REQUEST", contentv1.TriageCategory_TRIAGE_CATEGORY_ACTION_REQUEST},
		{"action_request", contentv1.TriageCategory_TRIAGE_CATEGORY_ACTION_REQUEST}, // case-insensitive
		{"PROJECT_UPDATE", contentv1.TriageCategory_TRIAGE_CATEGORY_PROJECT_UPDATE},
		{"RISK_ISSUE", contentv1.TriageCategory_TRIAGE_CATEGORY_RISK_ISSUE},
		{"CUSTOMER", contentv1.TriageCategory_TRIAGE_CATEGORY_CUSTOMER},
		{"DECISION", contentv1.TriageCategory_TRIAGE_CATEGORY_DECISION},
		{"INTERNAL_COMMS", contentv1.TriageCategory_TRIAGE_CATEGORY_INTERNAL_COMMS},
		{"PERSONAL", contentv1.TriageCategory_TRIAGE_CATEGORY_PERSONAL},
		{"FYI", contentv1.TriageCategory_TRIAGE_CATEGORY_FYI},
		{"OTHER", contentv1.TriageCategory_TRIAGE_CATEGORY_FYI}, // legacy alias
		{"", contentv1.TriageCategory_TRIAGE_CATEGORY_UNSPECIFIED},
		{"UNKNOWN", contentv1.TriageCategory_TRIAGE_CATEGORY_UNSPECIFIED},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			result := mapTriageCategoryToProto(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestMapTriageImportanceToProto tests all triage importance mappings.
func TestMapTriageImportanceToProto(t *testing.T) {
	cases := []struct {
		input    string
		expected contentv1.TriageImportance
	}{
		{"HIGH", contentv1.TriageImportance_TRIAGE_IMPORTANCE_HIGH},
		{"high", contentv1.TriageImportance_TRIAGE_IMPORTANCE_HIGH}, // case-insensitive
		{"MEDIUM", contentv1.TriageImportance_TRIAGE_IMPORTANCE_MEDIUM},
		{"LOW", contentv1.TriageImportance_TRIAGE_IMPORTANCE_LOW},
		{"", contentv1.TriageImportance_TRIAGE_IMPORTANCE_UNSPECIFIED},
		{"UNKNOWN", contentv1.TriageImportance_TRIAGE_IMPORTANCE_UNSPECIFIED},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			result := mapTriageImportanceToProto(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestApplyToProtoProcessingStatus_TriageEnums_EmptyStrings verifies empty
// triage strings map to UNSPECIFIED enums without error.
func TestApplyToProtoProcessingStatus_TriageEnums_EmptyStrings(t *testing.T) {
	ps := &ProcessingStatusResult{
		TriageCategory:   "",
		TriageImportance: "",
	}
	proto := &contentv1.ProcessingStatus{}

	ApplyToProtoProcessingStatus(ps, proto)

	assert.Equal(t, contentv1.TriageCategory_TRIAGE_CATEGORY_UNSPECIFIED, proto.TriageCategoryEnum)
	assert.Equal(t, contentv1.TriageImportance_TRIAGE_IMPORTANCE_UNSPECIFIED, proto.TriageImportanceEnum)
}

// TestApplyToProtoProcessingStatus_TriageFYIAlias verifies the "OTHER" → FYI alias.
func TestApplyToProtoProcessingStatus_TriageFYIAlias(t *testing.T) {
	ps := &ProcessingStatusResult{
		TriageCategory:   "OTHER",
		TriageImportance: "MEDIUM",
	}
	proto := &contentv1.ProcessingStatus{}

	ApplyToProtoProcessingStatus(ps, proto)

	assert.Equal(t, contentv1.TriageCategory_TRIAGE_CATEGORY_FYI, proto.TriageCategoryEnum)
	assert.Equal(t, contentv1.TriageImportance_TRIAGE_IMPORTANCE_MEDIUM, proto.TriageImportanceEnum)
}

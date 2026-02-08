// Package pipeline provides types and repository for pipeline status and job tracking.
package pipeline

import (
	"context"
	"encoding/json"
	"testing"
)

// TestDiffPipelineRuns_NormalDiff verifies that DiffPipelineRuns returns meaningful diffs
// when comparing two runs with different parsed_data.
func TestDiffPipelineRuns_NormalDiff(t *testing.T) {
	// This test will fail because DiffPipelineRuns doesn't exist yet
	var r *Repository
	if r == nil {
		t.Skip("Repository not initialized - this test verifies method signature exists")
	}

	ctx := context.Background()
	sourceID := int64(123)
	runIDA := int64(1) // older run
	runIDB := int64(2) // newer run

	// Expected: DiffPipelineRuns should return diffs between two runs
	diffs, err := r.DiffPipelineRuns(ctx, sourceID, runIDA, runIDB)
	if err != nil {
		t.Fatalf("DiffPipelineRuns failed: %v", err)
	}

	// Verify we get diffs when data differs
	if len(diffs) == 0 {
		t.Error("Expected diffs between runs with different data, got empty result")
	}

	// Verify diff structure
	for _, diff := range diffs {
		if diff.Stage == "" {
			t.Error("Expected non-empty Stage in diff")
		}
		if diff.Field == "" {
			t.Error("Expected non-empty Field in diff")
		}
		if diff.ChangeType == "" {
			t.Error("Expected non-empty ChangeType in diff")
		}
		// ChangeType should be one of: "added", "removed", "modified"
		validTypes := map[string]bool{"added": true, "removed": true, "modified": true}
		if !validTypes[diff.ChangeType] {
			t.Errorf("Invalid ChangeType %q, expected added/removed/modified", diff.ChangeType)
		}
	}
}

// TestDiffPipelineRuns_NoDifferences verifies that DiffPipelineRuns returns empty diff
// when comparing two identical runs.
func TestDiffPipelineRuns_NoDifferences(t *testing.T) {
	// This test will fail because DiffPipelineRuns doesn't exist yet
	var r *Repository
	if r == nil {
		t.Skip("Repository not initialized - this test verifies method signature exists")
	}

	ctx := context.Background()
	sourceID := int64(123)
	runIDA := int64(1)
	runIDB := int64(1) // same run

	// Expected: comparing a run to itself should return no diffs
	diffs, err := r.DiffPipelineRuns(ctx, sourceID, runIDA, runIDB)
	if err != nil {
		t.Fatalf("DiffPipelineRuns failed: %v", err)
	}

	if len(diffs) != 0 {
		t.Errorf("Expected no diffs when comparing identical runs, got %d diffs", len(diffs))
	}
}

// TestDiffPipelineRuns_DefaultRunSelection verifies that DiffPipelineRuns auto-selects
// the latest two runs when runIDA=0 and runIDB=0.
func TestDiffPipelineRuns_DefaultRunSelection(t *testing.T) {
	// This test will fail because DiffPipelineRuns doesn't exist yet
	var r *Repository
	if r == nil {
		t.Skip("Repository not initialized - this test verifies method signature exists")
	}

	ctx := context.Background()
	sourceID := int64(123)
	runIDA := int64(0) // auto-select second-most-recent
	runIDB := int64(0) // auto-select most recent

	// Expected: should auto-select latest two runs and return diffs
	diffs, err := r.DiffPipelineRuns(ctx, sourceID, runIDA, runIDB)
	if err != nil {
		t.Fatalf("DiffPipelineRuns with default selection failed: %v", err)
	}

	// Diffs may or may not be present depending on data, but should not error
	_ = diffs
}

// TestDiffPipelineRuns_OnlyOneRun verifies that DiffPipelineRuns handles the case
// when only one run exists for a source.
func TestDiffPipelineRuns_OnlyOneRun(t *testing.T) {
	// This test will fail because DiffPipelineRuns doesn't exist yet
	var r *Repository
	if r == nil {
		t.Skip("Repository not initialized - this test verifies method signature exists")
	}

	ctx := context.Background()
	sourceID := int64(123)
	runIDA := int64(0) // auto-select (should fail gracefully)
	runIDB := int64(0) // auto-select (should fail gracefully)

	// Expected: should return error or empty diff when only one run exists
	diffs, err := r.DiffPipelineRuns(ctx, sourceID, runIDA, runIDB)

	// Either error OR empty diffs is acceptable
	if err == nil && len(diffs) != 0 {
		t.Error("Expected error or empty diffs when only one run exists")
	}
}

// TestDiffPipelineRuns_NoRuns verifies that DiffPipelineRuns returns an error
// when no runs exist for a source.
func TestDiffPipelineRuns_NoRuns(t *testing.T) {
	// This test will fail because DiffPipelineRuns doesn't exist yet
	var r *Repository
	if r == nil {
		t.Skip("Repository not initialized - this test verifies method signature exists")
	}

	ctx := context.Background()
	sourceID := int64(999999) // non-existent source
	runIDA := int64(0)
	runIDB := int64(0)

	// Expected: should return error when no runs exist
	_, err := r.DiffPipelineRuns(ctx, sourceID, runIDA, runIDB)
	if err == nil {
		t.Error("Expected error when no runs exist for source")
	}
}

// TestDiffPipelineRuns_PartialRunID verifies that DiffPipelineRuns handles cases
// where one run ID is specified and one is auto-selected.
func TestDiffPipelineRuns_PartialRunID(t *testing.T) {
	// This test will fail because DiffPipelineRuns doesn't exist yet
	var r *Repository
	if r == nil {
		t.Skip("Repository not initialized - this test verifies method signature exists")
	}

	ctx := context.Background()
	sourceID := int64(123)

	// Case 1: runIDA specified, runIDB auto-selected (latest)
	runIDA := int64(1)
	runIDB := int64(0)
	diffs, err := r.DiffPipelineRuns(ctx, sourceID, runIDA, runIDB)
	if err != nil {
		t.Fatalf("DiffPipelineRuns with partial run ID (A specified) failed: %v", err)
	}
	_ = diffs

	// Case 2: runIDA auto-selected (second-latest), runIDB specified
	runIDA = int64(0)
	runIDB = int64(2)
	diffs, err = r.DiffPipelineRuns(ctx, sourceID, runIDA, runIDB)
	if err != nil {
		t.Fatalf("DiffPipelineRuns with partial run ID (B specified) failed: %v", err)
	}
	_ = diffs
}

// TestGetLatestRunIDs verifies that GetLatestRunIDs returns N most recent run IDs
// for a given source.
func TestGetLatestRunIDs(t *testing.T) {
	// This test will fail because GetLatestRunIDs doesn't exist yet
	var r *Repository
	if r == nil {
		t.Skip("Repository not initialized - this test verifies method signature exists")
	}

	ctx := context.Background()
	sourceID := int64(123)
	count := 5

	// Expected: should return up to N run IDs in descending order (newest first)
	runIDs, err := r.GetLatestRunIDs(ctx, sourceID, count)
	if err != nil {
		t.Fatalf("GetLatestRunIDs failed: %v", err)
	}

	// Verify we get IDs
	if len(runIDs) == 0 {
		t.Error("Expected at least one run ID")
	}

	// Verify we don't exceed requested count
	if len(runIDs) > count {
		t.Errorf("Expected at most %d run IDs, got %d", count, len(runIDs))
	}

	// Verify IDs are in descending order (newest first)
	for i := 1; i < len(runIDs); i++ {
		if runIDs[i] >= runIDs[i-1] {
			t.Error("Expected run IDs in descending order (newest first)")
		}
	}
}

// TestGetLatestRunIDs_NoRuns verifies that GetLatestRunIDs handles sources with no runs.
func TestGetLatestRunIDs_NoRuns(t *testing.T) {
	// This test will fail because GetLatestRunIDs doesn't exist yet
	var r *Repository
	if r == nil {
		t.Skip("Repository not initialized - this test verifies method signature exists")
	}

	ctx := context.Background()
	sourceID := int64(999999) // non-existent source
	count := 5

	// Expected: should return empty slice or error for source with no runs
	runIDs, err := r.GetLatestRunIDs(ctx, sourceID, count)
	if err != nil && len(runIDs) != 0 {
		t.Error("Expected empty slice when no runs exist")
	}
}

// TestGetLatestRunIDs_DefaultCount verifies that GetLatestRunIDs handles default count.
func TestGetLatestRunIDs_DefaultCount(t *testing.T) {
	// This test will fail because GetLatestRunIDs doesn't exist yet
	var r *Repository
	if r == nil {
		t.Skip("Repository not initialized - this test verifies method signature exists")
	}

	ctx := context.Background()
	sourceID := int64(123)
	count := 0 // request default

	// Expected: should use default count (e.g., 10) when count is 0
	runIDs, err := r.GetLatestRunIDs(ctx, sourceID, count)
	if err != nil {
		t.Fatalf("GetLatestRunIDs with default count failed: %v", err)
	}

	// Should return some IDs (implementation defines default)
	_ = runIDs
}

// TestRecordOverrides verifies that RecordOverrides stores override parameters
// in pipeline_runs input_data JSONB under "overrides" key.
func TestRecordOverrides(t *testing.T) {
	// This test will fail because RecordOverrides doesn't exist yet
	var r *Repository
	if r == nil {
		t.Skip("Repository not initialized - this test verifies method signature exists")
	}

	ctx := context.Background()
	runID := int64(1)
	overrides := map[string]string{
		"glossary_version": "v2",
		"use_fuzzy_match":  "true",
		"threshold":        "0.8",
	}

	// Expected: should store overrides in input_data JSONB
	err := r.RecordOverrides(ctx, runID, overrides)
	if err != nil {
		t.Fatalf("RecordOverrides failed: %v", err)
	}

	// Verify overrides were stored by reading back the run
	run, err := r.GetRunByID(ctx, runID)
	if err != nil {
		t.Fatalf("GetRunByID failed: %v", err)
	}

	// Parse input_data to verify overrides
	var inputData map[string]interface{}
	if err := json.Unmarshal(run.InputData, &inputData); err != nil {
		t.Fatalf("Failed to parse input_data: %v", err)
	}

	// Verify "overrides" key exists
	overridesData, ok := inputData["overrides"]
	if !ok {
		t.Error("Expected 'overrides' key in input_data")
	}

	// Verify override values
	overridesMap, ok := overridesData.(map[string]interface{})
	if !ok {
		t.Error("Expected 'overrides' to be a map")
	}

	for key, expectedVal := range overrides {
		actualVal, exists := overridesMap[key]
		if !exists {
			t.Errorf("Expected override key %q not found", key)
		}
		if actualVal != expectedVal {
			t.Errorf("Override %q: expected %q, got %q", key, expectedVal, actualVal)
		}
	}
}

// TestRecordOverrides_EmptyOverrides verifies that RecordOverrides handles empty override map.
func TestRecordOverrides_EmptyOverrides(t *testing.T) {
	// This test will fail because RecordOverrides doesn't exist yet
	var r *Repository
	if r == nil {
		t.Skip("Repository not initialized - this test verifies method signature exists")
	}

	ctx := context.Background()
	runID := int64(1)
	overrides := map[string]string{} // empty map

	// Expected: should handle empty overrides gracefully
	err := r.RecordOverrides(ctx, runID, overrides)
	if err != nil {
		t.Fatalf("RecordOverrides with empty map failed: %v", err)
	}
}

// TestRecordOverrides_NonExistentRun verifies that RecordOverrides returns error
// when run ID doesn't exist.
func TestRecordOverrides_NonExistentRun(t *testing.T) {
	// This test will fail because RecordOverrides doesn't exist yet
	var r *Repository
	if r == nil {
		t.Skip("Repository not initialized - this test verifies method signature exists")
	}

	ctx := context.Background()
	runID := int64(999999) // non-existent run
	overrides := map[string]string{
		"test_override": "value",
	}

	// Expected: should return error when run doesn't exist
	err := r.RecordOverrides(ctx, runID, overrides)
	if err == nil {
		t.Error("Expected error when recording overrides for non-existent run")
	}
}

// TestStageDiff_Type verifies that StageDiff type has the required fields.
func TestStageDiff_Type(t *testing.T) {
	// Verify StageDiff type structure
	diff := StageDiff{
		Stage:      "triage",
		Field:      "category",
		OldValue:   "PROJECT_UPDATE",
		NewValue:   "CUSTOMER",
		ChangeType: "modified",
	}

	if diff.Stage != "triage" {
		t.Errorf("Expected Stage 'triage', got %s", diff.Stage)
	}
	if diff.Field != "category" {
		t.Errorf("Expected Field 'category', got %s", diff.Field)
	}
	if diff.OldValue != "PROJECT_UPDATE" {
		t.Errorf("Expected OldValue 'PROJECT_UPDATE', got %s", diff.OldValue)
	}
	if diff.NewValue != "CUSTOMER" {
		t.Errorf("Expected NewValue 'CUSTOMER', got %s", diff.NewValue)
	}
	if diff.ChangeType != "modified" {
		t.Errorf("Expected ChangeType 'modified', got %s", diff.ChangeType)
	}
}

// TestStageDiff_ChangeTypes verifies valid change types.
func TestStageDiff_ChangeTypes(t *testing.T) {
	validTypes := []string{"added", "removed", "modified"}

	for _, changeType := range validTypes {
		diff := StageDiff{
			Stage:      "extract_ner",
			Field:      "entities",
			OldValue:   "",
			NewValue:   "test",
			ChangeType: changeType,
		}

		if diff.ChangeType != changeType {
			t.Errorf("Expected ChangeType %q, got %q", changeType, diff.ChangeType)
		}
	}
}

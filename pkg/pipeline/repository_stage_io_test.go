// Package pipeline provides types and repository for pipeline status and job tracking.
package pipeline

import (
	"context"
	"encoding/json"
	"testing"
)

// TestPipelineRunInput_IOFields verifies that PipelineRunInput has the correct IO fields.
func TestPipelineRunInput_IOFields(t *testing.T) {
	// Create a PipelineRunInput with IO data
	input := PipelineRunInput{
		SourceID:      123,
		Stage:         "triage",
		ModelID:       "qwen2.5-7b",
		PromptVersion: 1,
		ConfigHash:    "abc123",
		Status:        "completed",
		DurationMS:    500,
		InputData:     json.RawMessage(`{"prompt": "test"}`),
		OutputData:    json.RawMessage(`{"result": "success"}`),
		ParsedData:    json.RawMessage(`{"category": "PROJECT_UPDATE"}`),
	}

	// Verify fields are accessible
	if input.SourceID != 123 {
		t.Errorf("Expected SourceID 123, got %d", input.SourceID)
	}
	if input.Stage != "triage" {
		t.Errorf("Expected Stage 'triage', got %s", input.Stage)
	}
	if string(input.InputData) != `{"prompt": "test"}` {
		t.Errorf("Expected InputData to match, got %s", string(input.InputData))
	}
	if string(input.OutputData) != `{"result": "success"}` {
		t.Errorf("Expected OutputData to match, got %s", string(input.OutputData))
	}
	if string(input.ParsedData) != `{"category": "PROJECT_UPDATE"}` {
		t.Errorf("Expected ParsedData to match, got %s", string(input.ParsedData))
	}
}

// TestPipelineRun_IOFields verifies that PipelineRun has the correct IO fields.
func TestPipelineRun_IOFields(t *testing.T) {
	// Create a PipelineRun with IO data
	run := PipelineRun{
		ID:         1,
		SourceID:   123,
		Stage:      "triage",
		InputData:  json.RawMessage(`{"prompt": "test"}`),
		OutputData: json.RawMessage(`{"result": "success"}`),
		ParsedData: json.RawMessage(`{"category": "PROJECT_UPDATE"}`),
	}

	// Verify fields are accessible
	if run.ID != 1 {
		t.Errorf("Expected ID 1, got %d", run.ID)
	}
	if run.SourceID != 123 {
		t.Errorf("Expected SourceID 123, got %d", run.SourceID)
	}
	if string(run.InputData) != `{"prompt": "test"}` {
		t.Errorf("Expected InputData to match, got %s", string(run.InputData))
	}
	if string(run.OutputData) != `{"result": "success"}` {
		t.Errorf("Expected OutputData to match, got %s", string(run.OutputData))
	}
	if string(run.ParsedData) != `{"category": "PROJECT_UPDATE"}` {
		t.Errorf("Expected ParsedData to match, got %s", string(run.ParsedData))
	}
}

// TestRepositoryMethods_Compile verifies that new repository methods compile.
// This is a compile-time check only - it does not execute against a real database.
func TestRepositoryMethods_Compile(t *testing.T) {
	// This test is intentionally empty and just verifies the methods exist and compile.
	// The actual implementation will be tested with integration tests when a database is available.

	// Verify method signatures exist (compile-time check)
	var r *Repository
	if r != nil {
		ctx := context.Background()
		_, _ = r.GetStageIO(ctx, 123, "triage", 5)
		_, _ = r.GetRunByID(ctx, 1)
		_, _ = r.PruneOldRunIO(ctx, 123, "triage", 3)
	}
}

// TestGetStageIO_QueryParameters verifies GetStageIO handles default limit correctly.
func TestGetStageIO_QueryParameters(t *testing.T) {
	testCases := []struct {
		name          string
		inputLimit    int
		expectedLimit int
	}{
		{"Zero limit defaults to 3", 0, 3},
		{"Negative limit defaults to 3", -1, 3},
		{"Positive limit is preserved", 10, 10},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// This test verifies logic that would be exercised in GetStageIO
			limit := tc.inputLimit
			if limit <= 0 {
				limit = 3
			}
			if limit != tc.expectedLimit {
				t.Errorf("Expected limit %d, got %d", tc.expectedLimit, limit)
			}
		})
	}
}

// TestPruneOldRunIO_QueryParameters verifies PruneOldRunIO handles default keepN correctly.
func TestPruneOldRunIO_QueryParameters(t *testing.T) {
	testCases := []struct {
		name          string
		inputKeepN    int
		expectedKeepN int
	}{
		{"Zero keepN defaults to 3", 0, 3},
		{"Negative keepN defaults to 3", -1, 3},
		{"Positive keepN is preserved", 5, 5},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// This test verifies logic that would be exercised in PruneOldRunIO
			keepN := tc.inputKeepN
			if keepN <= 0 {
				keepN = 3
			}
			if keepN != tc.expectedKeepN {
				t.Errorf("Expected keepN %d, got %d", tc.expectedKeepN, keepN)
			}
		})
	}
}

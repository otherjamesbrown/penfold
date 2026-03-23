package temporal

import (
	"encoding/json"
	"testing"
	"time"
)

func TestStageOutputSuccessRoundTrip(t *testing.T) {
	original := StageOutput{
		Success:         true,
		Duration:        150 * time.Millisecond,
		RawData:         json.RawMessage(`{"model_used":"gpt-4","category":"internal"}`),
		LangfusePhaseID: "phase-abc-123",
		StageName:       "triage",
		StageKind:       "llm",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var got StageOutput
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if got.Success != original.Success {
		t.Errorf("Success: got %v, want %v", got.Success, original.Success)
	}
	if got.Duration != original.Duration {
		t.Errorf("Duration: got %v, want %v", got.Duration, original.Duration)
	}
	if got.LangfusePhaseID != original.LangfusePhaseID {
		t.Errorf("LangfusePhaseID: got %q, want %q", got.LangfusePhaseID, original.LangfusePhaseID)
	}
	if got.StageName != original.StageName {
		t.Errorf("StageName: got %q, want %q", got.StageName, original.StageName)
	}
	if got.StageKind != original.StageKind {
		t.Errorf("StageKind: got %q, want %q", got.StageKind, original.StageKind)
	}
	if string(got.RawData) != string(original.RawData) {
		t.Errorf("RawData: got %s, want %s", got.RawData, original.RawData)
	}
	if got.Error != "" {
		t.Errorf("Error: got %q, want empty", got.Error)
	}
}

func TestStageOutputFailedRoundTrip(t *testing.T) {
	original := StageOutput{
		Success:   false,
		Error:     "activity timeout after 30s",
		Duration:  30 * time.Second,
		StageName: "analyze",
		StageKind: "llm",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var got StageOutput
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if got.Success != original.Success {
		t.Errorf("Success: got %v, want %v", got.Success, original.Success)
	}
	if got.Error != original.Error {
		t.Errorf("Error: got %q, want %q", got.Error, original.Error)
	}
	if got.Duration != original.Duration {
		t.Errorf("Duration: got %v, want %v", got.Duration, original.Duration)
	}
	if len(got.RawData) != 0 {
		t.Errorf("RawData: got %s, want empty", got.RawData)
	}
}

func TestStageOutputNilRawDataRoundTrip(t *testing.T) {
	// Stages that produce no payload (e.g. code_only) must not break serialization.
	original := StageOutput{
		Success:   true,
		StageName: "persist",
		StageKind: "code_only",
		Duration:  5 * time.Millisecond,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var got StageOutput
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if got.Success != original.Success {
		t.Errorf("Success: got %v, want %v", got.Success, original.Success)
	}
	if got.StageName != original.StageName {
		t.Errorf("StageName: got %q, want %q", got.StageName, original.StageName)
	}
}

func TestStageInputRoundTrip(t *testing.T) {
	original := StageInput{
		TenantID:  "tenant-1",
		SourceID:  42,
		ContentID: "em-abc123",
		JobID:     "job-xyz",
		Content:   "Hello world",
		PreviousOutputs: map[string]StageOutput{
			"triage": {
				Success:   true,
				StageName: "triage",
				StageKind: "llm",
				RawData:   json.RawMessage(`{"category":"internal"}`),
			},
		},
		LangfuseTraceID: "trace-123",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var got StageInput
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if got.TenantID != original.TenantID {
		t.Errorf("TenantID: got %q, want %q", got.TenantID, original.TenantID)
	}
	if got.SourceID != original.SourceID {
		t.Errorf("SourceID: got %d, want %d", got.SourceID, original.SourceID)
	}
	if got.ContentID != original.ContentID {
		t.Errorf("ContentID: got %q, want %q", got.ContentID, original.ContentID)
	}
	if got.JobID != original.JobID {
		t.Errorf("JobID: got %q, want %q", got.JobID, original.JobID)
	}
	if got.Content != original.Content {
		t.Errorf("Content: got %q, want %q", got.Content, original.Content)
	}
	if got.LangfuseTraceID != original.LangfuseTraceID {
		t.Errorf("LangfuseTraceID: got %q, want %q", got.LangfuseTraceID, original.LangfuseTraceID)
	}
	if len(got.PreviousOutputs) != 1 {
		t.Fatalf("PreviousOutputs: got %d entries, want 1", len(got.PreviousOutputs))
	}
	triage, ok := got.PreviousOutputs["triage"]
	if !ok {
		t.Fatal("PreviousOutputs: missing 'triage' key")
	}
	if triage.StageName != "triage" {
		t.Errorf("PreviousOutputs['triage'].StageName: got %q, want %q", triage.StageName, "triage")
	}
	if string(triage.RawData) != `{"category":"internal"}` {
		t.Errorf("PreviousOutputs['triage'].RawData: got %s, want %s", triage.RawData, `{"category":"internal"}`)
	}
}

func TestStageInputNoPreviousOutputsRoundTrip(t *testing.T) {
	// First stage in the pipeline has no previous outputs.
	original := StageInput{
		TenantID: "tenant-1",
		SourceID: 1,
		JobID:    "job-1",
		Content:  "some content",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var got StageInput
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if len(got.PreviousOutputs) != 0 {
		t.Errorf("PreviousOutputs: got %d entries, want 0", len(got.PreviousOutputs))
	}
}

func TestPipelineStageConfigRoundTrip(t *testing.T) {
	original := PipelineStageConfig{
		Stage:          "triage",
		StageKind:      "llm",
		StageOrder:     1,
		Enabled:        true,
		SkipWhenLow:    false,
		Optional:       false,
		TimeoutSeconds: 60,
		ModelOverride:  "claude-3-haiku",
		PromptOverride: 3,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var got PipelineStageConfig
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if got.Stage != original.Stage {
		t.Errorf("Stage: got %q, want %q", got.Stage, original.Stage)
	}
	if got.StageKind != original.StageKind {
		t.Errorf("StageKind: got %q, want %q", got.StageKind, original.StageKind)
	}
	if got.StageOrder != original.StageOrder {
		t.Errorf("StageOrder: got %d, want %d", got.StageOrder, original.StageOrder)
	}
	if got.Enabled != original.Enabled {
		t.Errorf("Enabled: got %v, want %v", got.Enabled, original.Enabled)
	}
	if got.TimeoutSeconds != original.TimeoutSeconds {
		t.Errorf("TimeoutSeconds: got %d, want %d", got.TimeoutSeconds, original.TimeoutSeconds)
	}
	if got.ModelOverride != original.ModelOverride {
		t.Errorf("ModelOverride: got %q, want %q", got.ModelOverride, original.ModelOverride)
	}
	if got.PromptOverride != original.PromptOverride {
		t.Errorf("PromptOverride: got %d, want %d", got.PromptOverride, original.PromptOverride)
	}
}

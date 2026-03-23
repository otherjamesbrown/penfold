package pipeline

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type mockPipelineLister struct {
	defs []PipelineDefinition
	err  error
}

func (m *mockPipelineLister) ListPipelines(_ context.Context, _ string) ([]PipelineDefinition, error) {
	return m.defs, m.err
}

func TestValidateAllDefinitions_Valid(t *testing.T) {
	lister := &mockPipelineLister{
		defs: []PipelineDefinition{
			{Pipeline: "slm", ContentType: "email", Stages: []StageDefinition{
				{Stage: "parse", Enabled: true},
				{Stage: "triage", Enabled: true},
			}},
			{Pipeline: "newsletter", ContentType: "newsletter", Stages: []StageDefinition{
				{Stage: "parse", Enabled: true},
			}},
		},
	}

	if err := ValidateAllDefinitions(context.Background(), lister, "test-tenant"); err != nil {
		t.Errorf("expected no error for valid definitions, got: %v", err)
	}
}

func TestValidateAllDefinitions_NoEnabledStages(t *testing.T) {
	lister := &mockPipelineLister{
		defs: []PipelineDefinition{
			{Pipeline: "slm", ContentType: "email", Stages: []StageDefinition{
				{Stage: "parse", Enabled: true},
			}},
			{Pipeline: "broken", ContentType: "email", Stages: []StageDefinition{
				{Stage: "parse", Enabled: false},
				{Stage: "triage", Enabled: false},
			}},
		},
	}

	err := ValidateAllDefinitions(context.Background(), lister, "test-tenant")
	if err == nil {
		t.Fatal("expected error for pipeline with no enabled stages, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "broken") {
		t.Errorf("error should name the invalid pipeline, got: %s", msg)
	}
	if !strings.Contains(msg, "test-tenant") {
		t.Errorf("error should include tenant ID, got: %s", msg)
	}
}

func TestValidateAllDefinitions_MultipleInvalid(t *testing.T) {
	lister := &mockPipelineLister{
		defs: []PipelineDefinition{
			{Pipeline: "broken1", Stages: []StageDefinition{{Stage: "parse", Enabled: false}}},
			{Pipeline: "broken2", Stages: []StageDefinition{{Stage: "parse", Enabled: false}}},
		},
	}

	err := ValidateAllDefinitions(context.Background(), lister, "test-tenant")
	if err == nil {
		t.Fatal("expected error for multiple invalid pipelines, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "broken1") || !strings.Contains(msg, "broken2") {
		t.Errorf("error should name all invalid pipelines, got: %s", msg)
	}
}

func TestValidateAllDefinitions_ListError(t *testing.T) {
	lister := &mockPipelineLister{
		err: errors.New("connection refused"),
	}

	err := ValidateAllDefinitions(context.Background(), lister, "test-tenant")
	if err == nil {
		t.Fatal("expected error when ListPipelines fails, got nil")
	}
	if !strings.Contains(err.Error(), "test-tenant") {
		t.Errorf("error should include tenant ID, got: %v", err)
	}
}

func TestValidateAllDefinitions_Empty(t *testing.T) {
	lister := &mockPipelineLister{defs: nil}

	if err := ValidateAllDefinitions(context.Background(), lister, "test-tenant"); err != nil {
		t.Errorf("expected no error for empty definitions, got: %v", err)
	}
}


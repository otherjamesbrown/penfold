package temporal

import (
	"context"
	"errors"
	"testing"
)

// --- test doubles ---

// mockExecutor is a StageExecutor that records calls and returns a preset output.
type mockExecutor struct {
	called    bool
	output    StageOutput
	err       error
}

func (m *mockExecutor) Execute(_ context.Context, _ PipelineStageConfig, _ StageInput) (StageOutput, error) {
	m.called = true
	return m.output, m.err
}

// mockReporter records every ReportPhase call.
type mockReporter struct {
	calls []LangfusePhaseReport
	err   error
}

func (r *mockReporter) ReportPhase(_ context.Context, report LangfusePhaseReport) error {
	r.calls = append(r.calls, report)
	return r.err
}

// --- ExecutorRegistry tests ---

func TestRegistryRegisterAndGet(t *testing.T) {
	reg := NewExecutorRegistry(&mockReporter{})
	inner := &mockExecutor{output: StageOutput{Success: true, StageName: "parse", StageKind: "code_only"}}
	reg.Register("code_only", inner)

	got, err := reg.Get("code_only")
	if err != nil {
		t.Fatalf("Get: unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("Get: returned nil executor")
	}
}

func TestRegistryGetUnknownKind(t *testing.T) {
	reg := NewExecutorRegistry(&mockReporter{})

	_, err := reg.Get("nonexistent")
	if err == nil {
		t.Fatal("Get: expected error for unknown stage_kind, got nil")
	}

	want := `executor registry: unknown stage_kind "nonexistent"`
	if err.Error() != want {
		t.Errorf("Get: error = %q, want %q", err.Error(), want)
	}
}

func TestRegistryOverwritesExistingKind(t *testing.T) {
	reg := NewExecutorRegistry(&mockReporter{})

	first := &mockExecutor{output: StageOutput{Success: true, StageName: "first"}}
	second := &mockExecutor{output: StageOutput{Success: true, StageName: "second"}}

	reg.Register("llm", first)
	reg.Register("llm", second)

	executor, err := reg.Get("llm")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	out, _ := executor.Execute(context.Background(), PipelineStageConfig{Stage: "triage", StageKind: "llm"}, StageInput{})
	if out.StageName != "second" {
		t.Errorf("expected second executor; StageName = %q", out.StageName)
	}
}

// --- LangfuseWrappedExecutor tests ---

func TestWrappingCallsReporter(t *testing.T) {
	reporter := &mockReporter{}
	inner := &mockExecutor{
		output: StageOutput{Success: true, StageName: "triage", StageKind: "llm"},
	}
	wrapped := &LangfuseWrappedExecutor{inner: inner, reporter: reporter}

	stage := PipelineStageConfig{Stage: "triage", StageKind: "llm"}
	input := StageInput{LangfuseTraceID: "trace-xyz"}

	out, err := wrapped.Execute(context.Background(), stage, input)
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}

	// Inner was called.
	if !inner.called {
		t.Error("inner executor was not called")
	}

	// Reporter was called exactly once.
	if len(reporter.calls) != 1 {
		t.Fatalf("ReportPhase called %d times, want 1", len(reporter.calls))
	}

	report := reporter.calls[0]

	if report.PhaseName != "triage" {
		t.Errorf("PhaseName = %q, want %q", report.PhaseName, "triage")
	}
	if report.StageKind != "llm" {
		t.Errorf("StageKind = %q, want %q", report.StageKind, "llm")
	}
	if report.TraceID != "trace-xyz" {
		t.Errorf("TraceID = %q, want %q", report.TraceID, "trace-xyz")
	}
	if !report.Success {
		t.Errorf("Success = false, want true")
	}
	if report.PhaseID == "" {
		t.Error("PhaseID should not be empty")
	}
	if report.StartTime.IsZero() || report.EndTime.IsZero() {
		t.Error("StartTime and EndTime should be set")
	}
	if report.EndTime.Before(report.StartTime) {
		t.Error("EndTime should not be before StartTime")
	}

	// LangfusePhaseID populated on output.
	if out.LangfusePhaseID == "" {
		t.Error("LangfusePhaseID not set on StageOutput")
	}
	if out.LangfusePhaseID != report.PhaseID {
		t.Errorf("LangfusePhaseID %q != report PhaseID %q", out.LangfusePhaseID, report.PhaseID)
	}
}

func TestWrappingCapturesDuration(t *testing.T) {
	reporter := &mockReporter{}
	inner := &mockExecutor{output: StageOutput{Success: true}}
	wrapped := &LangfuseWrappedExecutor{inner: inner, reporter: reporter}

	out, _ := wrapped.Execute(context.Background(), PipelineStageConfig{Stage: "embed", StageKind: "embedding"}, StageInput{})

	if out.Duration <= 0 {
		t.Errorf("Duration should be positive, got %v", out.Duration)
	}
	if len(reporter.calls) == 0 {
		t.Fatal("reporter not called")
	}
	report := reporter.calls[0]
	if report.EndTime.Sub(report.StartTime) <= 0 {
		t.Errorf("report EndTime - StartTime should be positive, got %v", report.EndTime.Sub(report.StartTime))
	}
}

func TestWrappingCapturesFailure(t *testing.T) {
	reporter := &mockReporter{}
	inner := &mockExecutor{
		output: StageOutput{Success: false, Error: "activity timed out"},
		err:    errors.New("activity timed out"),
	}
	wrapped := &LangfuseWrappedExecutor{inner: inner, reporter: reporter}

	stage := PipelineStageConfig{Stage: "analyze", StageKind: "llm"}
	_, _ = wrapped.Execute(context.Background(), PipelineStageConfig{Stage: stage.Stage, StageKind: stage.StageKind}, StageInput{})

	if len(reporter.calls) != 1 {
		t.Fatalf("expected 1 reporter call, got %d", len(reporter.calls))
	}
	report := reporter.calls[0]
	if report.Success {
		t.Error("Success should be false for failed stage")
	}
	if report.StatusMessage == "" {
		t.Error("StatusMessage should be set on failure")
	}
}

func TestWrappingReporterErrorDoesNotPropagateToCallerFailure(t *testing.T) {
	// Reporter errors must not mask the real stage error.
	reporter := &mockReporter{err: errors.New("langfuse unreachable")}
	inner := &mockExecutor{
		output: StageOutput{Success: false, Error: "parse failed"},
		err:    errors.New("parse failed"),
	}
	wrapped := &LangfuseWrappedExecutor{inner: inner, reporter: reporter}

	_, err := wrapped.Execute(context.Background(), PipelineStageConfig{Stage: "parse", StageKind: "code_only"}, StageInput{})
	if err == nil || err.Error() != "parse failed" {
		t.Errorf("expected inner error %q, got %v", "parse failed", err)
	}
}

func TestWrappingReporterErrorDoesNotPropagateToCallerSuccess(t *testing.T) {
	// Reporter errors must not surface when the stage itself succeeded.
	reporter := &mockReporter{err: errors.New("langfuse unreachable")}
	inner := &mockExecutor{output: StageOutput{Success: true}}
	wrapped := &LangfuseWrappedExecutor{inner: inner, reporter: reporter}

	_, err := wrapped.Execute(context.Background(), PipelineStageConfig{Stage: "parse", StageKind: "code_only"}, StageInput{})
	if err != nil {
		t.Errorf("reporter error must not propagate to caller: %v", err)
	}
}

func TestWrappingEachPhaseIDIsUnique(t *testing.T) {
	reporter := &mockReporter{}
	inner := &mockExecutor{output: StageOutput{Success: true}}
	wrapped := &LangfuseWrappedExecutor{inner: inner, reporter: reporter}

	stage := PipelineStageConfig{Stage: "triage", StageKind: "llm"}
	input := StageInput{}

	const n = 5
	seen := make(map[string]bool, n)
	for i := 0; i < n; i++ {
		out, _ := wrapped.Execute(context.Background(), stage, input)
		if seen[out.LangfusePhaseID] {
			t.Errorf("duplicate PhaseID %q on call %d", out.LangfusePhaseID, i)
		}
		seen[out.LangfusePhaseID] = true
	}
}

// TestRegistryExecutorsAreWrapped verifies that executors returned by Get are
// LangfuseWrappedExecutors (reporter is called when Execute is invoked).
func TestRegistryExecutorsAreWrapped(t *testing.T) {
	reporter := &mockReporter{}
	reg := NewExecutorRegistry(reporter)
	inner := &mockExecutor{output: StageOutput{Success: true, StageName: "triage", StageKind: "llm"}}
	reg.Register("llm", inner)

	executor, _ := reg.Get("llm")

	stage := PipelineStageConfig{Stage: "triage", StageKind: "llm"}
	out, err := executor.Execute(context.Background(), stage, StageInput{LangfuseTraceID: "trace-abc"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// Reporter was triggered via the registry-injected wrapper.
	if len(reporter.calls) != 1 {
		t.Fatalf("expected 1 reporter call after Execute via registry, got %d", len(reporter.calls))
	}
	// LangfusePhaseID populated.
	if out.LangfusePhaseID == "" {
		t.Error("LangfusePhaseID not set on output from registry-wrapped executor")
	}
	// Inner was reached.
	if !inner.called {
		t.Error("inner executor not reached through registry wrapper")
	}

}

package temporal

import (
	"context"
	"errors"
	"testing"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// testExecutor is a mock StageExecutor that returns configurable results.
type testExecutor struct {
	output StageOutput
	err    error
	// capturedInputs records StageInput passed to each Execute call.
	capturedInputs []StageInput
}

func (e *testExecutor) Execute(_ context.Context, stage PipelineStageConfig, input StageInput) (StageOutput, error) {
	e.capturedInputs = append(e.capturedInputs, input)
	out := e.output
	// Always populate StageName/StageKind from the stage config so tests can assert
	// which stage was executed.
	if out.StageName == "" {
		out.StageName = stage.Stage
	}
	if out.StageKind == "" {
		out.StageKind = stage.StageKind
	}
	return out, e.err
}

// nopReporter is a no-op LangfuseReporter for tests.
type nopReporter struct{}

func (n *nopReporter) ReportPhase(_ context.Context, _ LangfusePhaseReport) error { return nil }

// buildRegistry creates a registry with a single "test" kind executor.
func buildRegistry(exec StageExecutor) *ExecutorRegistry {
	reg := NewExecutorRegistry(&nopReporter{})
	reg.Register("test", exec)
	return reg
}

func newNopLogger() logging.Logger {
	return logging.NewNopLogger()
}

// TestExecutePipeline_AllSucceed verifies all stages succeed and outputs are accumulated.
func TestExecutePipeline_AllSucceed(t *testing.T) {
	exec := &testExecutor{output: StageOutput{Success: true}}
	reg := buildRegistry(exec)

	stages := []PipelineStageConfig{
		{Stage: "a", StageKind: "test", StageOrder: 1, Enabled: true},
		{Stage: "b", StageKind: "test", StageOrder: 2, Enabled: true},
	}

	input := StageInput{TenantID: "tenant1"}
	outputs, err := ExecutePipeline(context.Background(), reg, stages, input, newNopLogger())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(outputs) != 2 {
		t.Fatalf("expected 2 outputs, got %d", len(outputs))
	}
	if !outputs["a"].Success {
		t.Error("stage a should succeed")
	}
	if !outputs["b"].Success {
		t.Error("stage b should succeed")
	}
}

// TestExecutePipeline_DisabledStage verifies disabled stages are not added to outputs.
func TestExecutePipeline_DisabledStage(t *testing.T) {
	exec := &testExecutor{output: StageOutput{Success: true}}
	reg := buildRegistry(exec)

	stages := []PipelineStageConfig{
		{Stage: "enabled_stage", StageKind: "test", StageOrder: 1, Enabled: true},
		{Stage: "disabled_stage", StageKind: "test", StageOrder: 2, Enabled: false},
	}

	outputs, err := ExecutePipeline(context.Background(), reg, stages, StageInput{}, newNopLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := outputs["disabled_stage"]; ok {
		t.Error("disabled stage should not appear in outputs")
	}
	if _, ok := outputs["enabled_stage"]; !ok {
		t.Error("enabled stage should appear in outputs")
	}
}

// TestExecutePipeline_SkipWhenLow verifies SkipWhenLow=true stages are skipped when SkipDeep=true.
func TestExecutePipeline_SkipWhenLow(t *testing.T) {
	exec := &testExecutor{output: StageOutput{Success: true}}
	reg := buildRegistry(exec)

	stages := []PipelineStageConfig{
		{Stage: "low_value", StageKind: "test", StageOrder: 1, Enabled: true, SkipWhenLow: true},
	}

	input := StageInput{SkipDeep: true}
	outputs, err := ExecutePipeline(context.Background(), reg, stages, input, newNopLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out, ok := outputs["low_value"]
	if !ok {
		t.Fatal("low_value stage should be in outputs (as skipped)")
	}
	if !out.Skipped {
		t.Error("output should be marked Skipped")
	}
	if out.SkipReason != "skip_when_low" {
		t.Errorf("SkipReason = %q, want %q", out.SkipReason, "skip_when_low")
	}
	if !out.Success {
		t.Error("skipped output should still have Success=true")
	}
}

// TestExecutePipeline_SkipWhenLow_NotTriggered verifies SkipWhenLow is ignored when SkipDeep=false.
func TestExecutePipeline_SkipWhenLow_NotTriggered(t *testing.T) {
	exec := &testExecutor{output: StageOutput{Success: true}}
	reg := buildRegistry(exec)

	stages := []PipelineStageConfig{
		{Stage: "low_value", StageKind: "test", StageOrder: 1, Enabled: true, SkipWhenLow: true},
	}

	input := StageInput{SkipDeep: false}
	outputs, err := ExecutePipeline(context.Background(), reg, stages, input, newNopLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out, ok := outputs["low_value"]
	if !ok {
		t.Fatal("low_value stage should be in outputs")
	}
	if out.Skipped {
		t.Error("stage should not be marked Skipped when SkipDeep=false")
	}
	if !out.Success {
		t.Error("stage should succeed when executed")
	}
}

// TestExecutePipeline_UnmetDependency verifies stages with unmet dependencies are skipped.
func TestExecutePipeline_UnmetDependency(t *testing.T) {
	exec := &testExecutor{output: StageOutput{Success: true}}
	reg := buildRegistry(exec)

	stages := []PipelineStageConfig{
		{Stage: "dependent", StageKind: "test", StageOrder: 1, Enabled: true, DependsOn: []string{"missing_dep"}},
	}

	outputs, err := ExecutePipeline(context.Background(), reg, stages, StageInput{}, newNopLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out, ok := outputs["dependent"]
	if !ok {
		t.Fatal("dependent stage should be in outputs (as skipped)")
	}
	if !out.Skipped {
		t.Error("output should be Skipped")
	}
	if out.Success {
		t.Error("skipped-due-to-unmet-deps should have Success=false")
	}
	if out.SkipReason == "" {
		t.Error("SkipReason should be set")
	}
}

// TestExecutePipeline_UnknownStageKind verifies unknown stage kinds are skipped without error.
func TestExecutePipeline_UnknownStageKind(t *testing.T) {
	reg := NewExecutorRegistry(&nopReporter{}) // no executors registered

	stages := []PipelineStageConfig{
		{Stage: "mystery", StageKind: "unknown_kind", StageOrder: 1, Enabled: true},
	}

	outputs, err := ExecutePipeline(context.Background(), reg, stages, StageInput{}, newNopLogger())
	if err != nil {
		t.Fatalf("unknown stage kind should not cause pipeline error, got: %v", err)
	}
	if _, ok := outputs["mystery"]; ok {
		t.Error("unknown kind stage should not appear in outputs (silently skipped)")
	}
}

// TestExecutePipeline_NonOptionalFailure verifies non-optional stage failure stops the pipeline.
func TestExecutePipeline_NonOptionalFailure(t *testing.T) {
	execFail := &testExecutor{
		output: StageOutput{Success: false, Error: "activity failed"},
		err:    errors.New("activity failed"),
	}
	execOk := &testExecutor{output: StageOutput{Success: true}}

	reg := NewExecutorRegistry(&nopReporter{})
	reg.Register("fail_kind", execFail)
	reg.Register("ok_kind", execOk)

	stages := []PipelineStageConfig{
		{Stage: "critical", StageKind: "fail_kind", StageOrder: 1, Enabled: true, Optional: false},
		{Stage: "next", StageKind: "ok_kind", StageOrder: 2, Enabled: true},
	}

	outputs, err := ExecutePipeline(context.Background(), reg, stages, StageInput{}, newNopLogger())
	if err == nil {
		t.Fatal("expected error from non-optional stage failure")
	}
	// Should have the failing stage in outputs but not the next one.
	if _, ok := outputs["critical"]; !ok {
		t.Error("failing stage should be in outputs")
	}
	if _, ok := outputs["next"]; ok {
		t.Error("stage after non-optional failure should not be in outputs")
	}
}

// TestExecutePipeline_OptionalFailure verifies optional stage failure continues pipeline.
func TestExecutePipeline_OptionalFailure(t *testing.T) {
	execFail := &testExecutor{
		output: StageOutput{Success: false, Error: "optional activity failed"},
	}
	execOk := &testExecutor{output: StageOutput{Success: true}}

	reg := NewExecutorRegistry(&nopReporter{})
	reg.Register("fail_kind", execFail)
	reg.Register("ok_kind", execOk)

	stages := []PipelineStageConfig{
		{Stage: "optional_stage", StageKind: "fail_kind", StageOrder: 1, Enabled: true, Optional: true},
		{Stage: "next_stage", StageKind: "ok_kind", StageOrder: 2, Enabled: true},
	}

	outputs, err := ExecutePipeline(context.Background(), reg, stages, StageInput{}, newNopLogger())
	if err != nil {
		t.Fatalf("optional failure should not stop pipeline: %v", err)
	}
	if _, ok := outputs["optional_stage"]; !ok {
		t.Error("optional failing stage should still be in outputs")
	}
	if !outputs["next_stage"].Success {
		t.Error("next stage should have run and succeeded")
	}
}

// TestExecutePipeline_DependencyChain verifies stage B gets A's output in PreviousOutputs.
func TestExecutePipeline_DependencyChain(t *testing.T) {
	execA := &testExecutor{output: StageOutput{Success: true, StageName: "stage_a"}}
	execB := &testExecutor{output: StageOutput{Success: true, StageName: "stage_b"}}

	reg := NewExecutorRegistry(&nopReporter{})
	reg.Register("kind_a", execA)
	reg.Register("kind_b", execB)

	stages := []PipelineStageConfig{
		{Stage: "stage_a", StageKind: "kind_a", StageOrder: 1, Enabled: true},
		{Stage: "stage_b", StageKind: "kind_b", StageOrder: 2, Enabled: true, DependsOn: []string{"stage_a"}},
	}

	outputs, err := ExecutePipeline(context.Background(), reg, stages, StageInput{}, newNopLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(outputs) != 2 {
		t.Fatalf("expected 2 outputs, got %d", len(outputs))
	}

	// Verify that stage_b received stage_a's output in PreviousOutputs.
	if len(execB.capturedInputs) == 0 {
		t.Fatal("stage_b executor was not called")
	}
	prevOutputs := execB.capturedInputs[0].PreviousOutputs
	if prevOutputs == nil {
		t.Fatal("stage_b did not receive PreviousOutputs")
	}
	aOut, ok := prevOutputs["stage_a"]
	if !ok {
		t.Error("stage_a output not present in stage_b's PreviousOutputs")
	}
	if !aOut.Success {
		t.Error("stage_a output in PreviousOutputs should be successful")
	}
}

// TestExecutePipeline_OrderedStages verifies stages execute in stage_order order regardless
// of input slice order.
func TestExecutePipeline_OrderedStages(t *testing.T) {
	var executionOrder []string
	makeExec := func(name string) *testExecutor {
		return &testExecutor{
			output: StageOutput{Success: true, StageName: name},
		}
	}

	// Create executors that record execution order.
	type orderRecorder struct {
		name  string
		order *[]string
	}
	recorders := []orderRecorder{
		{"first", &executionOrder},
		{"second", &executionOrder},
		{"third", &executionOrder},
	}

	reg := NewExecutorRegistry(&nopReporter{})
	for _, r := range recorders {
		name := r.name
		orderPtr := r.order
		e := makeExec(name)
		_ = e
		// Use a custom executor to record order.
		reg.Register(name, &orderTrackingExecutor{name: name, order: orderPtr})
	}

	// Provide stages in reverse order.
	stages := []PipelineStageConfig{
		{Stage: "s3", StageKind: "third", StageOrder: 3, Enabled: true},
		{Stage: "s1", StageKind: "first", StageOrder: 1, Enabled: true},
		{Stage: "s2", StageKind: "second", StageOrder: 2, Enabled: true},
	}
	// Sort them first (caller's responsibility per the function contract).
	sortedStages := OrderedPipelineStages(stages)

	_, err := ExecutePipeline(context.Background(), reg, sortedStages, StageInput{}, newNopLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(executionOrder) != 3 {
		t.Fatalf("expected 3 executions, got %d", len(executionOrder))
	}
	if executionOrder[0] != "first" || executionOrder[1] != "second" || executionOrder[2] != "third" {
		t.Errorf("wrong execution order: %v", executionOrder)
	}
}

// orderTrackingExecutor records its name into a shared order slice on Execute.
type orderTrackingExecutor struct {
	name  string
	order *[]string
}

func (e *orderTrackingExecutor) Execute(_ context.Context, _ PipelineStageConfig, _ StageInput) (StageOutput, error) {
	*e.order = append(*e.order, e.name)
	return StageOutput{Success: true, StageName: e.name}, nil
}

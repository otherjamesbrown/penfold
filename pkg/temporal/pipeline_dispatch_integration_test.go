package temporal

// Integration test: stage-kind-driven dispatch end-to-end (pf-4df4cd)
//
// Verifies the complete dispatch mechanism across all acceptance criteria:
//   - Design: execution order, extensibility, stage_kind-driven dispatch, pipeline variants
//   - Stage dispatch: all four stage_kinds route to registered executors
//   - Legacy removal: DispatchLoopEnabled flag gone, no stage-name branching in dispatch
//   - Observability: Langfuse auto-wrapping fires per-stage with required metadata
//
// These are package-level unit tests (no Temporal test environment needed).
// Executors that require workflow.Context (CodeOnlyExecutor) are represented by
// dispatchTracker stubs — the dispatch mechanism is what is under test here, not
// individual executor implementations.

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

// dispatchTracker is a StageExecutor stub that records which stage names it handled.
// Used to verify that stage_kind routing dispatches to the expected executor instance
// without branching on stage name inside the executor.
type dispatchTracker struct {
	kind          string
	stagesHandled []string
	output        StageOutput
}

func (d *dispatchTracker) Execute(_ context.Context, stage PipelineStageConfig, _ StageInput) (StageOutput, error) {
	d.stagesHandled = append(d.stagesHandled, stage.Stage)
	out := d.output
	out.StageName = stage.Stage
	out.StageKind = stage.StageKind
	return out, nil
}

// ============================================================
// Design acceptance criteria
// ============================================================

// TestDispatch_StagesExecuteInPipelineDefinitionOrder verifies the dispatch loop
// executes stages in the order declared by stage_order, not the order of the input
// slice. This is the primary design criterion: pipeline_definitions.stage_order drives
// execution, not hardcoded sequencing.
func TestDispatch_StagesExecuteInPipelineDefinitionOrder(t *testing.T) {
	var executionOrder []string

	reg := NewExecutorRegistry(&nopReporter{})
	for _, name := range []string{"first", "second", "third"} {
		n := name // capture
		reg.Register(n, &orderTrackingExecutor{name: n, order: &executionOrder})
	}

	// Stages declared in reverse order; OrderedPipelineStages must reorder them.
	unsorted := []PipelineStageConfig{
		{Stage: "s3", StageKind: "third", StageOrder: 30, Enabled: true},
		{Stage: "s1", StageKind: "first", StageOrder: 10, Enabled: true},
		{Stage: "s2", StageKind: "second", StageOrder: 20, Enabled: true},
	}
	stages := OrderedPipelineStages(unsorted)

	_, err := ExecutePipeline(context.Background(), reg, stages, StageInput{}, newNopLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"first", "second", "third"}
	if len(executionOrder) != len(want) {
		t.Fatalf("expected %d executions, got %d: %v", len(want), len(executionOrder), executionOrder)
	}
	for i := range want {
		if executionOrder[i] != want[i] {
			t.Errorf("execution[%d] = %q, want %q (order: %v)", i, executionOrder[i], want[i], executionOrder)
		}
	}
}

// TestDispatch_NewStageKindRequiresOnlyRegistrationAndConfig demonstrates extensibility:
// adding a new stage_kind requires only (1) registering an executor and (2) adding a
// PipelineStageConfig row. No changes to ExecutePipeline, ExecutorRegistry, or any
// existing executor are needed.
func TestDispatch_NewStageKindRequiresOnlyRegistrationAndConfig(t *testing.T) {
	customExec := &dispatchTracker{kind: "custom_analysis", output: StageOutput{Success: true}}

	reg := NewExecutorRegistry(&nopReporter{})
	reg.Register("custom_analysis", customExec) // only registration needed

	// Single PipelineStageConfig row — no code changes anywhere else.
	stages := []PipelineStageConfig{
		{Stage: "my_custom_stage", StageKind: "custom_analysis", StageOrder: 1, Enabled: true},
	}

	outputs, err := ExecutePipeline(context.Background(), reg, stages, StageInput{TenantID: "t1"}, newNopLogger())
	if err != nil {
		t.Fatalf("custom stage should execute without error: %v", err)
	}
	out, ok := outputs["my_custom_stage"]
	if !ok {
		t.Fatal("custom stage output not found")
	}
	if !out.Success {
		t.Error("custom stage should succeed")
	}
	if len(customExec.stagesHandled) == 0 {
		t.Error("custom executor was not called — stage_kind did not dispatch correctly")
	}
}

// TestDispatch_StageKindDeterminesDispatch_NotStageName verifies that the same executor
// handles multiple stage names when all share a stage_kind. There is no branching on
// stage name inside ExecutePipeline — dispatch is purely stage_kind-driven.
func TestDispatch_StageKindDeterminesDispatch_NotStageName(t *testing.T) {
	llmExec := &dispatchTracker{kind: "llm", output: StageOutput{Success: true}}

	reg := NewExecutorRegistry(&nopReporter{})
	reg.Register("llm", llmExec)

	// Five different stage names, all with stage_kind="llm" — one executor handles all.
	stages := OrderedPipelineStages([]PipelineStageConfig{
		{Stage: "triage",            StageKind: "llm", StageOrder: 1, Enabled: true},
		{Stage: "extract_semantic",  StageKind: "llm", StageOrder: 2, Enabled: true},
		{Stage: "extract_ner",       StageKind: "llm", StageOrder: 3, Enabled: true},
		{Stage: "analyze",           StageKind: "llm", StageOrder: 4, Enabled: true},
		{Stage: "summarize",         StageKind: "llm", StageOrder: 5, Enabled: true},
	})

	_, err := ExecutePipeline(context.Background(), reg, stages, StageInput{}, newNopLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(llmExec.stagesHandled) != 5 {
		t.Fatalf("llm executor called %d times, want 5; stages handled: %v",
			len(llmExec.stagesHandled), llmExec.stagesHandled)
	}
	wantNames := []string{"triage", "extract_semantic", "extract_ner", "analyze", "summarize"}
	for i, got := range llmExec.stagesHandled {
		if got != wantNames[i] {
			t.Errorf("stagesHandled[%d] = %q, want %q", i, got, wantNames[i])
		}
	}
}

// ============================================================
// Stage dispatch verification — all four stage_kinds
// ============================================================

// TestDispatch_AllFourStageKindsRouteToRegisteredExecutors verifies that each
// stage_kind (code_only, llm, embedding, structured_extract) dispatches to its
// registered executor instance with no cross-kind confusion.
func TestDispatch_AllFourStageKindsRouteToRegisteredExecutors(t *testing.T) {
	codeOnlyExec := &dispatchTracker{kind: "code_only", output: StageOutput{Success: true}}
	llmExec := &dispatchTracker{kind: "llm", output: StageOutput{Success: true}}
	embeddingExec := &dispatchTracker{kind: "embedding", output: StageOutput{Success: true}}
	structuredExtExec := &dispatchTracker{kind: "structured_extract", output: StageOutput{Success: true}}

	reg := NewExecutorRegistry(&nopReporter{})
	reg.Register("code_only", codeOnlyExec)
	reg.Register("llm", llmExec)
	reg.Register("embedding", embeddingExec)
	reg.Register("structured_extract", structuredExtExec)

	stages := OrderedPipelineStages([]PipelineStageConfig{
		{Stage: "parse",              StageKind: "code_only",          StageOrder: 1, Enabled: true},
		{Stage: "triage",             StageKind: "llm",                StageOrder: 2, Enabled: true},
		{Stage: "embed",              StageKind: "embedding",          StageOrder: 3, Enabled: true},
		{Stage: "newsletter_extract", StageKind: "structured_extract", StageOrder: 4, Enabled: true},
	})

	outputs, err := ExecutePipeline(context.Background(), reg, stages, StageInput{TenantID: "t1"}, newNopLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(outputs) != 4 {
		t.Fatalf("expected 4 outputs, got %d", len(outputs))
	}

	cases := []struct {
		stageName string
		tracker   *dispatchTracker
	}{
		{"parse", codeOnlyExec},
		{"triage", llmExec},
		{"embed", embeddingExec},
		{"newsletter_extract", structuredExtExec},
	}
	for _, c := range cases {
		if len(c.tracker.stagesHandled) == 0 {
			t.Errorf("executor for %q was not called", c.tracker.kind)
			continue
		}
		if c.tracker.stagesHandled[0] != c.stageName {
			t.Errorf("executor %q handled stage %q, want %q", c.tracker.kind, c.tracker.stagesHandled[0], c.stageName)
		}
	}
}

// ============================================================
// Pipeline variant coverage
// ============================================================

// TestDispatch_StandardPipelineVariant verifies that a representative standard
// pipeline (parse → resolve → triage → extract → analyze → embed → persist)
// executes all stages in stage_order and emits one Langfuse trace per stage.
func TestDispatch_StandardPipelineVariant(t *testing.T) {
	reporter := &mockReporter{}
	reg := NewExecutorRegistry(reporter)
	for _, kind := range []string{"code_only", "llm", "embedding"} {
		reg.Register(kind, &testExecutor{output: StageOutput{Success: true}})
	}

	stages := OrderedPipelineStages([]PipelineStageConfig{
		{Stage: "parse",            StageKind: "code_only", StageOrder: 100, Enabled: true},
		{Stage: "resolve",          StageKind: "code_only", StageOrder: 200, Enabled: true},
		{Stage: "triage",           StageKind: "llm",       StageOrder: 300, Enabled: true},
		{Stage: "extract_semantic", StageKind: "llm",       StageOrder: 400, Enabled: true},
		{Stage: "extract_ner",      StageKind: "llm",       StageOrder: 500, Enabled: true},
		{Stage: "analyze",          StageKind: "llm",       StageOrder: 600, Enabled: true},
		{Stage: "embed",            StageKind: "embedding", StageOrder: 700, Enabled: true},
		{Stage: "persist",          StageKind: "code_only", StageOrder: 800, Enabled: true},
	})

	outputs, err := ExecutePipeline(context.Background(), reg, stages, StageInput{TenantID: "t1"}, newNopLogger())
	if err != nil {
		t.Fatalf("standard pipeline: unexpected error: %v", err)
	}
	if len(outputs) != 8 {
		t.Errorf("expected 8 outputs, got %d", len(outputs))
	}
	for name, out := range outputs {
		if !out.Success {
			t.Errorf("stage %q should succeed", name)
		}
	}
	// One Langfuse report per stage, auto-wrapped by registry.
	if len(reporter.calls) != 8 {
		t.Errorf("expected 8 Langfuse traces (one per stage), got %d", len(reporter.calls))
	}
}

// TestDispatch_NewsletterVariantPipelineStages verifies that a newsletter pipeline
// variant with a structured_extract stage executes correctly alongside standard stages.
func TestDispatch_NewsletterVariantPipelineStages(t *testing.T) {
	reg := NewExecutorRegistry(&nopReporter{})
	for _, kind := range []string{"code_only", "llm", "embedding", "structured_extract"} {
		reg.Register(kind, &testExecutor{output: StageOutput{Success: true}})
	}

	// Newsletter pipeline inserts newsletter_extract (structured_extract) after analyze.
	stages := OrderedPipelineStages([]PipelineStageConfig{
		{Stage: "parse",              StageKind: "code_only",          StageOrder: 100, Enabled: true},
		{Stage: "triage",             StageKind: "llm",                StageOrder: 300, Enabled: true},
		{Stage: "analyze",            StageKind: "llm",                StageOrder: 600, Enabled: true},
		{Stage: "newsletter_extract", StageKind: "structured_extract", StageOrder: 650, Enabled: true},
		{Stage: "embed",              StageKind: "embedding",          StageOrder: 700, Enabled: true},
		{Stage: "persist",            StageKind: "code_only",          StageOrder: 800, Enabled: true},
	})

	outputs, err := ExecutePipeline(context.Background(), reg, stages, StageInput{TenantID: "t1"}, newNopLogger())
	if err != nil {
		t.Fatalf("newsletter pipeline: unexpected error: %v", err)
	}
	if out, ok := outputs["newsletter_extract"]; !ok || !out.Success {
		t.Error("newsletter_extract stage should be present and succeed")
	}
}

// TestDispatch_NotificationVariantPipelineStages verifies that a notification pipeline
// variant (lighter set of stages, no deep analysis) executes correctly.
func TestDispatch_NotificationVariantPipelineStages(t *testing.T) {
	reg := NewExecutorRegistry(&nopReporter{})
	for _, kind := range []string{"code_only", "llm", "structured_extract"} {
		reg.Register(kind, &testExecutor{output: StageOutput{Success: true}})
	}

	// Notification pipeline: parse → triage → notification_extract → persist.
	stages := OrderedPipelineStages([]PipelineStageConfig{
		{Stage: "parse",                 StageKind: "code_only",          StageOrder: 100, Enabled: true},
		{Stage: "triage",                StageKind: "llm",                StageOrder: 300, Enabled: true},
		{Stage: "notification_extract",  StageKind: "structured_extract", StageOrder: 400, Enabled: true},
		{Stage: "persist",               StageKind: "code_only",          StageOrder: 800, Enabled: true},
	})

	outputs, err := ExecutePipeline(context.Background(), reg, stages, StageInput{TenantID: "t1"}, newNopLogger())
	if err != nil {
		t.Fatalf("notification pipeline: unexpected error: %v", err)
	}
	if len(outputs) != 4 {
		t.Errorf("expected 4 outputs, got %d", len(outputs))
	}
}

// ============================================================
// Langfuse observability verification
// ============================================================

// TestObservability_LangfuseAutoWrappingFiresPerStage verifies that the reporter is
// called exactly once per executed stage via the registry's LangfuseWrappedExecutor,
// without any executor manually calling the reporter.
func TestObservability_LangfuseAutoWrappingFiresPerStage(t *testing.T) {
	reporter := &mockReporter{}
	reg := NewExecutorRegistry(reporter)

	// Inner executors are plain stubs — no tracing logic inside them.
	inner := &testExecutor{output: StageOutput{Success: true}}
	reg.Register("llm", inner)
	reg.Register("embedding", inner)
	reg.Register("structured_extract", inner)

	stages := OrderedPipelineStages([]PipelineStageConfig{
		{Stage: "triage",             StageKind: "llm",                StageOrder: 1, Enabled: true},
		{Stage: "embed",              StageKind: "embedding",          StageOrder: 2, Enabled: true},
		{Stage: "newsletter_extract", StageKind: "structured_extract", StageOrder: 3, Enabled: true},
	})

	_, err := ExecutePipeline(
		context.Background(), reg, stages,
		StageInput{TenantID: "t1", LangfuseTraceID: "trace-e2e"},
		newNopLogger(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(reporter.calls) != 3 {
		t.Fatalf("expected 3 Langfuse reports (one per stage), got %d", len(reporter.calls))
	}
}

// TestObservability_TraceIncludesDurationSuccessAndStageKind verifies that each
// Langfuse trace includes the required metadata: duration (start/end times), success
// status, and stage_kind — sufficient for observability dashboards and alerting.
func TestObservability_TraceIncludesDurationSuccessAndStageKind(t *testing.T) {
	reporter := &mockReporter{}
	reg := NewExecutorRegistry(reporter)
	reg.Register("llm", &testExecutor{output: StageOutput{Success: true}})

	stages := []PipelineStageConfig{
		{Stage: "triage", StageKind: "llm", StageOrder: 1, Enabled: true},
	}
	_, err := ExecutePipeline(
		context.Background(), reg, stages,
		StageInput{TenantID: "t1", LangfuseTraceID: "trace-meta"},
		newNopLogger(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(reporter.calls) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reporter.calls))
	}
	r := reporter.calls[0]

	// Required metadata fields.
	if r.PhaseID == "" {
		t.Error("PhaseID must be set")
	}
	if r.TraceID != "trace-meta" {
		t.Errorf("TraceID = %q, want %q", r.TraceID, "trace-meta")
	}
	if r.StageKind != "llm" {
		t.Errorf("StageKind = %q, want %q", r.StageKind, "llm")
	}
	if r.PhaseName != "triage" {
		t.Errorf("PhaseName = %q, want %q", r.PhaseName, "triage")
	}
	if !r.Success {
		t.Error("Success should be true for successful stage")
	}
	// Duration tracking.
	if r.StartTime.IsZero() || r.EndTime.IsZero() {
		t.Error("StartTime and EndTime must be non-zero (duration tracking)")
	}
	if r.EndTime.Before(r.StartTime) {
		t.Error("EndTime must be >= StartTime")
	}
}

// TestObservability_FailedStageTraceReflectsFailure verifies that Langfuse reports
// for failed stages have Success=false and a non-empty StatusMessage.
func TestObservability_FailedStageTraceReflectsFailure(t *testing.T) {
	reporter := &mockReporter{}
	reg := NewExecutorRegistry(reporter)
	reg.Register("llm", &testExecutor{
		output: StageOutput{Success: false, Error: "LLM timeout"},
		err:    errors.New("LLM timeout"),
	})

	stages := []PipelineStageConfig{
		{Stage: "triage", StageKind: "llm", StageOrder: 1, Enabled: true, Optional: false},
	}
	_, pipelineErr := ExecutePipeline(context.Background(), reg, stages, StageInput{}, newNopLogger())
	if pipelineErr == nil {
		t.Fatal("expected pipeline error for non-optional stage failure")
	}

	// Report must still be emitted even when the stage failed.
	if len(reporter.calls) != 1 {
		t.Fatalf("expected 1 Langfuse report on failure, got %d", len(reporter.calls))
	}
	r := reporter.calls[0]
	if r.Success {
		t.Error("Langfuse report Success must be false for failed stage")
	}
	if r.StatusMessage == "" {
		t.Error("StatusMessage must be non-empty for failed stage trace")
	}
	if r.StageKind != "llm" {
		t.Errorf("StageKind = %q, want %q", r.StageKind, "llm")
	}
}

// TestObservability_LangfusePhaseIDPopulatedOnOutput verifies that StageOutput carries
// a non-empty LangfusePhaseID after execution via the registry wrapper.
func TestObservability_LangfusePhaseIDPopulatedOnOutput(t *testing.T) {
	reg := NewExecutorRegistry(&mockReporter{})
	reg.Register("llm", &testExecutor{output: StageOutput{Success: true}})

	stages := []PipelineStageConfig{
		{Stage: "triage",  StageKind: "llm", StageOrder: 1, Enabled: true},
		{Stage: "analyze", StageKind: "llm", StageOrder: 2, Enabled: true},
	}

	outputs, err := ExecutePipeline(context.Background(), reg, stages, StageInput{}, newNopLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for name, out := range outputs {
		if out.LangfusePhaseID == "" {
			t.Errorf("stage %q: LangfusePhaseID should be populated by auto-wrapping", name)
		}
	}
}

// ============================================================
// Legacy removal verification
// ============================================================

// TestLegacy_DispatchLoopEnabledFlagRemoved verifies that the DispatchLoopEnabled
// feature flag removed in pf-bfa8ae no longer exists in pipeline.go.
// The flag previously gated the embed dispatch path: "if DispatchLoopEnabled { new } else { old }".
func TestLegacy_DispatchLoopEnabledFlagRemoved(t *testing.T) {
	content, err := os.ReadFile("../../services/worker/workflows/pipeline.go")
	if err != nil {
		t.Fatalf("could not read pipeline.go: %v", err)
	}
	if strings.Contains(string(content), "DispatchLoopEnabled") {
		t.Error("DispatchLoopEnabled feature flag must be removed from pipeline.go (pf-bfa8ae)")
	}
}

// TestLegacy_EmbedStageUsesDispatchLoop verifies that the embed stage in pipeline.go
// calls ExecutePipeline — i.e. the dispatch loop is active, not a legacy path.
func TestLegacy_EmbedStageUsesDispatchLoop(t *testing.T) {
	content, err := os.ReadFile("../../services/worker/workflows/pipeline.go")
	if err != nil {
		t.Fatalf("could not read pipeline.go: %v", err)
	}
	if !strings.Contains(string(content), "ExecutePipeline") {
		t.Error("pipeline.go must call ExecutePipeline for the embed stage (dispatch loop active)")
	}
}

// TestLegacy_DispatchLoopHasNoStageNameBranching verifies that dispatch_loop.go
// contains no hard-coded stage-name conditionals. The loop is generic — it dispatches
// by stage_kind only, never branching on stage identity.
func TestLegacy_DispatchLoopHasNoStageNameBranching(t *testing.T) {
	content, err := os.ReadFile("dispatch_loop.go")
	if err != nil {
		t.Fatalf("could not read dispatch_loop.go: %v", err)
	}
	src := string(content)

	// None of the stage names that were hard-coded in the old pipeline.go monolith
	// should appear as string literals in the new dispatch loop.
	hardcodedNames := []string{
		`"triage"`, `"parse"`, `"resolve"`, `"persist"`,
		`"extract_semantic"`, `"extract_ner"`, `"analyze"`, `"summarize"`,
		`"enrich_entities"`, `"newsletter_extract"`, `"notification_extract"`,
		`"embed"`,
	}
	for _, name := range hardcodedNames {
		if strings.Contains(src, name) {
			t.Errorf("dispatch_loop.go contains hard-coded stage name %s — dispatch must be stage_kind-driven only", name)
		}
	}
}

// TestLegacy_ExecutorRegistryHasNoStageNameBranching verifies that executor_registry.go
// has no stage-name conditionals — it routes by stage_kind key only.
func TestLegacy_ExecutorRegistryHasNoStageNameBranching(t *testing.T) {
	content, err := os.ReadFile("executor_registry.go")
	if err != nil {
		t.Fatalf("could not read executor_registry.go: %v", err)
	}
	src := string(content)

	hardcodedNames := []string{
		`"triage"`, `"parse"`, `"resolve"`, `"persist"`, `"embed"`,
		`"extract_semantic"`, `"extract_ner"`, `"analyze"`, `"summarize"`,
	}
	for _, name := range hardcodedNames {
		if strings.Contains(src, name) {
			t.Errorf("executor_registry.go contains hard-coded stage name %s — must be stage_kind-driven only", name)
		}
	}
}

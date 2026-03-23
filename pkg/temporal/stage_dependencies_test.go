package temporal

import (
	"strings"
	"testing"
)

// --- ValidatePipelineDependencies tests ---

func TestValidatePipelineDependencies_ValidLinearChain(t *testing.T) {
	stages := []PipelineStageConfig{
		{Stage: "parse"},
		{Stage: "triage", DependsOn: []string{"parse"}},
		{Stage: "extract", DependsOn: []string{"triage"}},
		{Stage: "summarize", DependsOn: []string{"extract"}},
	}
	if err := ValidatePipelineDependencies(stages); err != nil {
		t.Errorf("expected valid DAG to pass, got: %v", err)
	}
}

func TestValidatePipelineDependencies_ValidDiamond(t *testing.T) {
	// parse → triage, extract both depend on parse; summarize depends on both.
	stages := []PipelineStageConfig{
		{Stage: "parse"},
		{Stage: "triage", DependsOn: []string{"parse"}},
		{Stage: "extract", DependsOn: []string{"parse"}},
		{Stage: "summarize", DependsOn: []string{"triage", "extract"}},
	}
	if err := ValidatePipelineDependencies(stages); err != nil {
		t.Errorf("expected valid diamond DAG to pass, got: %v", err)
	}
}

func TestValidatePipelineDependencies_NoDependencies(t *testing.T) {
	stages := []PipelineStageConfig{
		{Stage: "parse"},
		{Stage: "triage"},
		{Stage: "extract"},
	}
	if err := ValidatePipelineDependencies(stages); err != nil {
		t.Errorf("expected stages with no depends_on to pass, got: %v", err)
	}
}

func TestValidatePipelineDependencies_EmptyStages(t *testing.T) {
	if err := ValidatePipelineDependencies(nil); err != nil {
		t.Errorf("expected nil stages to pass, got: %v", err)
	}
	if err := ValidatePipelineDependencies([]PipelineStageConfig{}); err != nil {
		t.Errorf("expected empty stages to pass, got: %v", err)
	}
}

func TestValidatePipelineDependencies_DirectCycle(t *testing.T) {
	stages := []PipelineStageConfig{
		{Stage: "a", DependsOn: []string{"b"}},
		{Stage: "b", DependsOn: []string{"a"}},
	}
	err := ValidatePipelineDependencies(stages)
	if err == nil {
		t.Error("expected cycle to be detected, got nil")
	}
	if !strings.Contains(err.Error(), "circular dependency") {
		t.Errorf("expected 'circular dependency' in error, got: %v", err)
	}
}

func TestValidatePipelineDependencies_ThreeNodeCycle(t *testing.T) {
	stages := []PipelineStageConfig{
		{Stage: "a", DependsOn: []string{"c"}},
		{Stage: "b", DependsOn: []string{"a"}},
		{Stage: "c", DependsOn: []string{"b"}},
	}
	err := ValidatePipelineDependencies(stages)
	if err == nil {
		t.Error("expected 3-node cycle to be detected, got nil")
	}
	if !strings.Contains(err.Error(), "circular dependency") {
		t.Errorf("expected 'circular dependency' in error, got: %v", err)
	}
}

func TestValidatePipelineDependencies_SelfDependency(t *testing.T) {
	stages := []PipelineStageConfig{
		{Stage: "parse", DependsOn: []string{"parse"}},
	}
	err := ValidatePipelineDependencies(stages)
	if err == nil {
		t.Error("expected self-dependency cycle to be detected, got nil")
	}
}

func TestValidatePipelineDependencies_MissingDependency(t *testing.T) {
	stages := []PipelineStageConfig{
		{Stage: "triage", DependsOn: []string{"nonexistent"}},
	}
	err := ValidatePipelineDependencies(stages)
	if err == nil {
		t.Error("expected missing dependency to be detected, got nil")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("expected missing stage name in error, got: %v", err)
	}
}

func TestValidatePipelineDependencies_PartialCycle(t *testing.T) {
	// parse has no deps (valid), but triage <-> extract form a cycle.
	stages := []PipelineStageConfig{
		{Stage: "parse"},
		{Stage: "triage", DependsOn: []string{"extract"}},
		{Stage: "extract", DependsOn: []string{"triage"}},
	}
	err := ValidatePipelineDependencies(stages)
	if err == nil {
		t.Error("expected partial cycle to be detected, got nil")
	}
}

// --- ResolveDependencies tests ---

func TestResolveDependencies_NoDependencies(t *testing.T) {
	stage := PipelineStageConfig{Stage: "parse", DependsOn: nil}
	outputs := map[string]StageOutput{}

	ready, unmet := ResolveDependencies(stage, outputs)
	if !ready {
		t.Error("expected ready=true when no dependencies")
	}
	if len(unmet) != 0 {
		t.Errorf("expected no unmet deps, got: %v", unmet)
	}
}

func TestResolveDependencies_AllDependenciesMet(t *testing.T) {
	stage := PipelineStageConfig{
		Stage:     "extract",
		DependsOn: []string{"parse", "triage"},
	}
	outputs := map[string]StageOutput{
		"parse":  {Success: true, StageName: "parse"},
		"triage": {Success: true, StageName: "triage"},
	}

	ready, unmet := ResolveDependencies(stage, outputs)
	if !ready {
		t.Errorf("expected ready=true, got unmet: %v", unmet)
	}
	if len(unmet) != 0 {
		t.Errorf("expected no unmet deps, got: %v", unmet)
	}
}

func TestResolveDependencies_DependencyNotYetRun(t *testing.T) {
	stage := PipelineStageConfig{
		Stage:     "extract",
		DependsOn: []string{"parse", "triage"},
	}
	// triage not in outputs yet
	outputs := map[string]StageOutput{
		"parse": {Success: true, StageName: "parse"},
	}

	ready, unmet := ResolveDependencies(stage, outputs)
	if ready {
		t.Error("expected ready=false when dependency missing from outputs")
	}
	if len(unmet) != 1 || unmet[0] != "triage" {
		t.Errorf("expected [triage] as unmet, got: %v", unmet)
	}
}

func TestResolveDependencies_DependencyFailed(t *testing.T) {
	stage := PipelineStageConfig{
		Stage:     "extract",
		DependsOn: []string{"triage"},
	}
	outputs := map[string]StageOutput{
		"triage": {Success: false, Error: "LLM timeout", StageName: "triage"},
	}

	ready, unmet := ResolveDependencies(stage, outputs)
	if ready {
		t.Error("expected ready=false when dependency failed")
	}
	if len(unmet) != 1 || unmet[0] != "triage" {
		t.Errorf("expected [triage] as unmet, got: %v", unmet)
	}
}

func TestResolveDependencies_MultipleDependenciesFailed(t *testing.T) {
	stage := PipelineStageConfig{
		Stage:     "summarize",
		DependsOn: []string{"triage", "extract", "analyze"},
	}
	outputs := map[string]StageOutput{
		"triage":  {Success: true, StageName: "triage"},
		"extract": {Success: false, Error: "timeout", StageName: "extract"},
		// analyze not in outputs
	}

	ready, unmet := ResolveDependencies(stage, outputs)
	if ready {
		t.Error("expected ready=false when multiple dependencies unmet")
	}
	if len(unmet) != 2 {
		t.Errorf("expected 2 unmet deps, got %d: %v", len(unmet), unmet)
	}
	// Verify both unmet deps are reported.
	unmetSet := make(map[string]bool)
	for _, u := range unmet {
		unmetSet[u] = true
	}
	if !unmetSet["extract"] {
		t.Error("expected 'extract' in unmet deps")
	}
	if !unmetSet["analyze"] {
		t.Error("expected 'analyze' in unmet deps")
	}
}

func TestResolveDependencies_EmptyOutputMap(t *testing.T) {
	stage := PipelineStageConfig{
		Stage:     "triage",
		DependsOn: []string{"parse"},
	}

	ready, unmet := ResolveDependencies(stage, nil)
	if ready {
		t.Error("expected ready=false when outputs map is nil and deps exist")
	}
	if len(unmet) != 1 || unmet[0] != "parse" {
		t.Errorf("expected [parse] as unmet, got: %v", unmet)
	}
}

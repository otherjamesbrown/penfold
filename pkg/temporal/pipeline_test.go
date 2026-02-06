package temporal

import (
	"testing"
)

func TestFullPipelineTotalSteps(t *testing.T) {
	got := FullPipelineTotalSteps()
	if got != 7 {
		t.Errorf("FullPipelineTotalSteps() = %d, want 7", got)
	}
}

func TestSkipDeepTotalSteps(t *testing.T) {
	got := SkipDeepTotalSteps()
	// Parse, Triage, Embed are the non-skippable stages
	if got != 3 {
		t.Errorf("SkipDeepTotalSteps() = %d, want 3", got)
	}
}

func TestStageNames(t *testing.T) {
	expected := []string{"Parse", "Triage", "Extract", "Context", "Analyze", "Persist", "Embed"}
	got := StageNames()

	if len(got) != len(expected) {
		t.Fatalf("StageNames() returned %d names, want %d", len(got), len(expected))
	}
	for i, name := range got {
		if name != expected[i] {
			t.Errorf("StageNames()[%d] = %q, want %q", i, name, expected[i])
		}
	}
}

func TestRequiredActivities(t *testing.T) {
	got := RequiredActivities()
	// Required = non-optional AND non-skippable: Parse (ParseEmail), Triage, Embed (GenerateContentEmbedding)
	expected := map[string]bool{
		ActivityParseEmail:              true,
		ActivityTriage:                  true,
		ActivityGenerateContentEmbedding: true,
	}

	if len(got) != len(expected) {
		t.Fatalf("RequiredActivities() returned %d activities, want %d: %v", len(got), len(expected), got)
	}
	for _, activity := range got {
		if !expected[activity] {
			t.Errorf("RequiredActivities() includes unexpected activity %q", activity)
		}
	}
}

func TestDeepProcessingActivities(t *testing.T) {
	got := DeepProcessingActivities()
	// Deep = SkipWhenLow stages: Extract, Context, Analyze, Persist
	expected := map[string]bool{
		ActivityExtractEntitiesActivity: true,
		ActivityExtractAssertions:       true,
		ActivityBuildContextPackage:     true,
		ActivityDeepAnalyze:             true,
		ActivityPersistFindings:         true,
	}

	if len(got) != len(expected) {
		t.Fatalf("DeepProcessingActivities() returned %d activities, want %d: %v", len(got), len(expected), got)
	}
	for _, activity := range got {
		if !expected[activity] {
			t.Errorf("DeepProcessingActivities() includes unexpected activity %q", activity)
		}
	}
}

func TestSLMPipelineStages_NoDuplicateNumbers(t *testing.T) {
	seen := make(map[int]string)
	for _, s := range SLMPipelineStages {
		if prev, ok := seen[s.Number]; ok {
			t.Errorf("Stage number %d used by both %q and %q", s.Number, prev, s.Name)
		}
		seen[s.Number] = s.Name
	}
}

func TestSLMPipelineStages_ConsecutiveNumbers(t *testing.T) {
	for i, s := range SLMPipelineStages {
		if s.Number != i {
			t.Errorf("Stage %q has Number %d, want %d (consecutive from 0)", s.Name, s.Number, i)
		}
	}
}

func TestSLMPipelineStages_AllActivitiesExist(t *testing.T) {
	mainActivities := make(map[string]bool)
	for _, a := range AllMainQueueActivities() {
		mainActivities[a] = true
	}

	for _, s := range SLMPipelineStages {
		for _, activity := range s.Activities {
			if !mainActivities[activity] {
				t.Errorf("Stage %q references activity %q not found in AllMainQueueActivities()", s.Name, activity)
			}
		}
	}
}

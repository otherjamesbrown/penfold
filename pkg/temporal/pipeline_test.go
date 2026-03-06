package temporal

import (
	"context"
	"testing"

	"github.com/rs/zerolog"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

func TestFullPipelineTotalSteps(t *testing.T) {
	got := FullPipelineTotalSteps()
	// Parse, Triage, Extract, Context, EnrichEntities, Analyze, Persist, Embed
	if got != 8 {
		t.Errorf("FullPipelineTotalSteps() = %d, want 8", got)
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
	expected := []string{"Parse", "Triage", "Extract", "Context", "EnrichEntities", "Analyze", "Persist", "Embed"}
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
	// Deep = SkipWhenLow stages: Extract, Context, EnrichEntities, Analyze, Persist
	expected := map[string]bool{
		ActivityExtractEntitiesActivity: true,
		ActivityExtractAssertions:       true,
		ActivityBuildContextPackage:     true,
		ActivityEnrichEntities:          true,
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

// captureLogger captures log messages for test assertions.
type captureLogger struct {
	warns []string
	infos []string
}

func (l *captureLogger) Debug(msg string, fields ...logging.Field) {}
func (l *captureLogger) Info(msg string, fields ...logging.Field)  { l.infos = append(l.infos, msg) }
func (l *captureLogger) Warn(msg string, fields ...logging.Field)  { l.warns = append(l.warns, msg) }
func (l *captureLogger) Error(msg string, fields ...logging.Field) {}
func (l *captureLogger) With(fields ...logging.Field) logging.Logger {
	return l
}
func (l *captureLogger) WithContext(ctx context.Context) logging.Logger { return l }
func (l *captureLogger) Zerolog() zerolog.Logger                       { return zerolog.Nop() }

func TestStageActivityMap_AllActivitiesExist(t *testing.T) {
	mainActivities := make(map[string]bool)
	for _, a := range AllMainQueueActivities() {
		mainActivities[a] = true
	}

	for stage, activities := range StageActivityMap {
		for _, act := range activities {
			if !mainActivities[act] {
				t.Errorf("StageActivityMap[%q] references activity %q not in AllMainQueueActivities()", stage, act)
			}
		}
	}
}

func TestValidateStageRegistry_AllMatch(t *testing.T) {
	logger := &captureLogger{}

	// All mapped stages are defined and all activities registered
	var defined []string
	for stage := range StageActivityMap {
		defined = append(defined, stage)
	}

	var registered []string
	for _, acts := range StageActivityMap {
		registered = append(registered, acts...)
	}

	ValidateStageRegistry(logger, defined, registered)

	if len(logger.warns) != 0 {
		t.Errorf("expected no warnings, got %d: %v", len(logger.warns), logger.warns)
	}
}

func TestValidateStageRegistry_UnknownStage(t *testing.T) {
	logger := &captureLogger{}

	ValidateStageRegistry(logger, []string{"fake_stage"}, AllMainQueueActivities())

	found := false
	for _, w := range logger.warns {
		if w == "pipeline definition references unknown stage (no activity mapping)" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning about unknown stage, got: %v", logger.warns)
	}
}

func TestValidateStageRegistry_UnregisteredActivity(t *testing.T) {
	logger := &captureLogger{}

	// Define a stage but don't register its activity
	ValidateStageRegistry(logger, []string{"parse"}, []string{})

	found := false
	for _, w := range logger.warns {
		if w == "pipeline stage references unregistered activity" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning about unregistered activity, got: %v", logger.warns)
	}
}

func TestValidateStageRegistry_OrphanActivity(t *testing.T) {
	logger := &captureLogger{}

	// Register parse activity but don't define the parse stage
	ValidateStageRegistry(logger, []string{}, []string{ActivityParseEmail})

	found := false
	for _, w := range logger.warns {
		if w == "registered activity not referenced by any pipeline definition" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning about orphan activity, got: %v", logger.warns)
	}
}

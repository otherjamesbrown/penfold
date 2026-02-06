package temporal

import "time"

// Stage describes a single step in the SLM pipeline.
type Stage struct {
	Name       string        // Human-readable: "Parse", "Triage", etc.
	StatusName string        // Status string: "parsing", "triaging", etc.
	Number     int           // Stage number (0-based)
	Activities []string      // Activity name constants (usually 1, Extract has 2)
	SkipWhenLow bool         // Skip when triage says SkipDeep
	Optional   bool          // If true, failure doesn't block pipeline
	Timeout    time.Duration // Default activity timeout for this stage
}

// SLMPipelineStages defines all stages in execution order.
//
// Note: Parse stage lists ActivityParseEmail as the primary activity but the
// workflow dispatches to ParseEmail or ParseTranscript based on ContentType.
// The metadata captures the primary activity; the workflow handles the dispatch.
var SLMPipelineStages = []Stage{
	{Name: "Parse", StatusName: "parsing", Number: 0, Activities: []string{ActivityParseEmail}, SkipWhenLow: false, Optional: false, Timeout: 30 * time.Second},
	{Name: "Triage", StatusName: "triaging", Number: 1, Activities: []string{ActivityTriage}, SkipWhenLow: false, Optional: false, Timeout: 60 * time.Second},
	{Name: "Extract", StatusName: "extracting", Number: 2, Activities: []string{ActivityExtractEntitiesActivity, ActivityExtractAssertions}, SkipWhenLow: true, Optional: false, Timeout: 120 * time.Second},
	{Name: "Context", StatusName: "building_context", Number: 3, Activities: []string{ActivityBuildContextPackage}, SkipWhenLow: true, Optional: false, Timeout: 60 * time.Second},
	{Name: "Analyze", StatusName: "analyzing", Number: 4, Activities: []string{ActivityDeepAnalyze}, SkipWhenLow: true, Optional: true, Timeout: 180 * time.Second},
	{Name: "Persist", StatusName: "persisting", Number: 5, Activities: []string{ActivityPersistFindings}, SkipWhenLow: true, Optional: false, Timeout: 60 * time.Second},
	{Name: "Embed", StatusName: "embedding", Number: 6, Activities: []string{ActivityGenerateContentEmbedding}, SkipWhenLow: false, Optional: false, Timeout: 60 * time.Second},
}

// FullPipelineTotalSteps returns the step count for the full pipeline.
func FullPipelineTotalSteps() int { return len(SLMPipelineStages) }

// SkipDeepTotalSteps returns the step count when deep processing is skipped.
func SkipDeepTotalSteps() int {
	count := 0
	for _, s := range SLMPipelineStages {
		if !s.SkipWhenLow {
			count++
		}
	}
	return count
}

// StageNames returns all stage names in order.
func StageNames() []string {
	names := make([]string, len(SLMPipelineStages))
	for i, s := range SLMPipelineStages {
		names[i] = s.Name
	}
	return names
}

// RequiredActivities returns activity names for non-optional, non-skippable stages.
func RequiredActivities() []string {
	var activities []string
	for _, s := range SLMPipelineStages {
		if !s.Optional && !s.SkipWhenLow {
			activities = append(activities, s.Activities...)
		}
	}
	return activities
}

// DeepProcessingActivities returns activity names for stages skipped when SkipDeep.
func DeepProcessingActivities() []string {
	var activities []string
	for _, s := range SLMPipelineStages {
		if s.SkipWhenLow {
			activities = append(activities, s.Activities...)
		}
	}
	return activities
}

package temporal

import (
	"strings"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

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
	{Name: "EnrichEntities", StatusName: "enriching_entities", Number: 4, Activities: []string{ActivityEnrichEntities}, SkipWhenLow: true, Optional: true, Timeout: 120 * time.Second},
	{Name: "Analyze", StatusName: "analyzing", Number: 5, Activities: []string{ActivityDeepAnalyze}, SkipWhenLow: true, Optional: true, Timeout: 180 * time.Second},
	{Name: "Persist", StatusName: "persisting", Number: 6, Activities: []string{ActivityPersistFindings}, SkipWhenLow: true, Optional: false, Timeout: 60 * time.Second},
	{Name: "Embed", StatusName: "embedding", Number: 7, Activities: []string{ActivityGenerateContentEmbedding}, SkipWhenLow: false, Optional: false, Timeout: 60 * time.Second},
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

// StageActivityMap maps pipeline_definitions stage names to the activity constants
// that implement them. This is the authoritative mapping between DB-defined stages
// and registered Temporal activities.
var StageActivityMap = map[string][]string{
	"parse":               {ActivityParseEmail},
	"parse_transcript":    {ActivityParseTranscript},
	"triage":              {ActivityTriage},
	"extract_ner":         {ActivityExtractEntitiesActivity},
	"extract_assertions":  {ActivityExtractAssertions},
	"extract_semantic":    {ActivityExtractMentions},
	"resolve":             {ActivityBuildContextPackage},
	"enrich_entities":     {ActivityEnrichEntities},
	"analyze":             {ActivityDeepAnalyze},
	"summary":             {ActivityGenerateContentSummary},
	"persist":             {ActivityPersistFindings},
	"embed":               {ActivityGenerateContentEmbedding},
	"attribute_project":      {ActivityAttributeProject},
	"instruction_evaluate":   {ActivityInstructionEvaluate},
	// newsletter_extract: structured JSON extraction stored in extracted_data['newsletter'].
	// Uses two activities: LLM call + JSONB persist. No assertions or entity mentions created.
	"newsletter_extract": {ActivityNewsletterExtract, ActivityPersistExtractedData},
}

// ValidateStageRegistry compares pipeline_definitions stage names against registered
// activities and logs warnings for mismatches. Does not block startup.
//
// definedStages: unique stage names from the pipeline_definitions table.
// registeredActivities: activity names registered with the Temporal worker.
func ValidateStageRegistry(logger logging.Logger, definedStages []string, registeredActivities []string) {
	registeredSet := make(map[string]bool, len(registeredActivities))
	for _, a := range registeredActivities {
		registeredSet[a] = true
	}

	definedSet := make(map[string]bool, len(definedStages))
	for _, s := range definedStages {
		definedSet[strings.ToLower(s)] = true
	}

	// Check 1: stages in definitions that reference unregistered activities
	for _, stage := range definedStages {
		lower := strings.ToLower(stage)
		activities, known := StageActivityMap[lower]
		if !known {
			logger.Warn("pipeline definition references unknown stage (no activity mapping)",
				logging.F("stage", stage),
			)
			continue
		}
		for _, act := range activities {
			if !registeredSet[act] {
				logger.Warn("pipeline stage references unregistered activity",
					logging.F("stage", stage),
					logging.F("activity", act),
				)
			}
		}
	}

	// Check 2: mapped stages with registered activities but not in any definition
	for stage, activities := range StageActivityMap {
		if definedSet[stage] {
			continue
		}
		// Only warn if the activities are actually registered (orphan = registered but unused)
		for _, act := range activities {
			if registeredSet[act] {
				logger.Warn("registered activity not referenced by any pipeline definition",
					logging.F("stage", stage),
					logging.F("activity", act),
				)
			}
		}
	}

	logger.Info("pipeline stage registry validation complete",
		logging.F("defined_stages", len(definedStages)),
		logging.F("registered_activities", len(registeredActivities)),
		logging.F("mapped_stages", len(StageActivityMap)),
	)
}

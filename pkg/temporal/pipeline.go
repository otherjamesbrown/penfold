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
	"classify_project":       {ActivityClassifyProject},
	"instruction_evaluate":   {ActivityInstructionEvaluate},
	// structured extract stages: generic StructuredExtract + shared PersistExtractedData
	"newsletter_extract":   {ActivityStructuredExtract, ActivityPersistExtractedData},
	"notification_extract": {ActivityStructuredExtract, ActivityPersistExtractedData},
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

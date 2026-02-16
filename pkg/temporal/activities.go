package temporal

// Activity name constants for all Temporal activities across task queues.
// These constants provide a single source of truth for activity names,
// preventing typos and mismatches between registration and invocation.

// Main Task Queue activity names (21 activities).
const (
	ActivityValidateContent          = "ValidateContent"
	ActivityFetchContent             = "FetchContent"
	ActivityUpdateContentStatus      = "UpdateContentStatus"
	ActivityGenerateContentEmbedding = "GenerateContentEmbedding"
	ActivityDeleteEmbedding          = "DeleteEmbedding"
	ActivityGenerateContentSummary   = "GenerateContentSummary"
	ActivityDeleteSummary            = "DeleteSummary"
	ActivityExtractEntitiesActivity  = "ExtractEntitiesActivity"
	ActivityExtractAssertions        = "ExtractAssertions"
	ActivityExtractMentions          = "ExtractMentions"
	ActivityTagProjects              = "TagProjects"
	ActivityParseEmail               = "ParseEmail"
	ActivityParseTranscript          = "ParseTranscript"
	ActivityPersistFindings          = "PersistFindings"
	ActivityTriage                   = "Triage"
	ActivityBuildContextPackage      = "BuildContextPackage"
	ActivityEnrichPersonMetadata     = "EnrichPersonMetadata"
	ActivityDeepAnalyze              = "DeepAnalyze"
	ActivityRecordOverrides          = "RecordOverrides"
	ActivityStartPipelineTracing     = "StartPipelineTracing"
	ActivityGroupEmailThread         = "GroupEmailThread"
	ActivityCreateEnrichmentRecord   = "CreateEnrichmentRecord"
)

// AI Task Queue activity names (4 unique activities).
// Note: ExtractAssertions, ExtractEntitiesActivity, and ExtractMentions are shared with main queue.
const (
	ActivityGenerateEmbedding          = "GenerateEmbedding"
	ActivityGenerateEmbeddingBatch     = "GenerateEmbeddingBatch"
	ActivityGenerateSummary            = "GenerateSummary"
	ActivityGenerateSummaryWithOptions = "GenerateSummaryWithOptions"
)

// Email Task Queue activity names (2 unique activities).
// Note: ParseEmail is shared with main queue.
const (
	ActivityFetchSource       = "FetchSource"
	ActivityUpdateSourceStatus = "UpdateSourceStatus"
)

// AllMainQueueActivities returns all activity names for the main task queue.
func AllMainQueueActivities() []string {
	return []string{
		ActivityValidateContent,
		ActivityFetchContent,
		ActivityUpdateContentStatus,
		ActivityGenerateContentEmbedding,
		ActivityDeleteEmbedding,
		ActivityGenerateContentSummary,
		ActivityDeleteSummary,
		ActivityExtractEntitiesActivity,
		ActivityExtractAssertions,
		ActivityExtractMentions,
		ActivityTagProjects,
		ActivityParseEmail,
		ActivityParseTranscript,
		ActivityPersistFindings,
		ActivityTriage,
		ActivityBuildContextPackage,
		ActivityEnrichPersonMetadata,
		ActivityDeepAnalyze,
		ActivityRecordOverrides,
		ActivityStartPipelineTracing,
		ActivityGroupEmailThread,
		ActivityCreateEnrichmentRecord,
	}
}

// AllAIQueueActivities returns all activity names for the AI task queue.
func AllAIQueueActivities() []string {
	return []string{
		ActivityGenerateEmbedding,
		ActivityGenerateEmbeddingBatch,
		ActivityGenerateSummary,
		ActivityGenerateSummaryWithOptions,
		ActivityExtractAssertions,
		ActivityExtractEntitiesActivity,
		ActivityExtractMentions,
	}
}

// AllEmailQueueActivities returns all activity names for the email task queue.
func AllEmailQueueActivities() []string {
	return []string{
		ActivityFetchSource,
		ActivityUpdateSourceStatus,
		ActivityParseEmail,
	}
}

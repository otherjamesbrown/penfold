package temporal

// Activity name constants for all Temporal activities across task queues.
// These constants provide a single source of truth for activity names,
// preventing typos and mismatches between registration and invocation.

// Main Task Queue activity names (22 activities).
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
	ActivityGroupEmailThread         = "GroupEmailThread"
	ActivityCreateEnrichmentRecord   = "CreateEnrichmentRecord"
	ActivityLinkConversation         = "LinkConversation"
	ActivityKickNextPending          = "KickNextPending"
	ActivityRecordSkippedStage       = "RecordSkippedStage"
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

// Langfuse activity name constants (registered on the main task queue).
const (
	ActivityCreateLangfuseTrace      = "CreateLangfuseTrace"
	ActivityReportLangfusePhase      = "ReportLangfusePhase"
	ActivityFinishLangfuseTrace      = "FinishLangfuseTrace"
	ActivityPersistLangfuseTraceID   = "PersistLangfuseTraceID"
	ActivityUpdateLangfuseTraceTags  = "UpdateLangfuseTraceTags"
)

// Deprecated Langfuse activity name constants — removed from the main queue in pf-37ebe8.
// Kept here for reference in tests that verify the constant is no longer registered.
const (
	// ActivityReportLangfuseGeneration was removed in pf-37ebe8.
	// Generation reporting is now handled by the AI coordinator via gRPC metadata.
	// This constant is retained only so test code that asserts its absence from
	// AllMainQueueActivities() can still compile.
	ActivityReportLangfuseGeneration = "ReportLangfuseGeneration"
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
		ActivityGroupEmailThread,
		ActivityCreateEnrichmentRecord,
		ActivityLinkConversation,
		ActivityCreateLangfuseTrace,
		ActivityReportLangfusePhase,
		ActivityFinishLangfuseTrace,
		ActivityPersistLangfuseTraceID,
		ActivityUpdateLangfuseTraceTags,
		ActivityKickNextPending,
		ActivityRecordSkippedStage,
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

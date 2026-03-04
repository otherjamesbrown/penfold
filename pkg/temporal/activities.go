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
	ActivityBuildExtractionContext   = "BuildExtractionContext"
	ActivityBuildContextPackage      = "BuildContextPackage"
	ActivityEnrichPersonMetadata     = "EnrichPersonMetadata"
	ActivityDeepAnalyze              = "DeepAnalyze"
	ActivityRecordOverrides          = "RecordOverrides"
	ActivityGroupEmailThread         = "GroupEmailThread"
	ActivityCreateEnrichmentRecord   = "CreateEnrichmentRecord"
	ActivityLinkConversation                = "LinkConversation"
	ActivityBackfillConversationSummaries  = "BackfillConversationSummaries"
	ActivityRegenerateConversationSummary  = "RegenerateConversationSummary"
	ActivityCheckStaleConversations        = "CheckStaleConversations"
	ActivityKickNextPending                = "KickNextPending"
	ActivityRecordSkippedStage             = "RecordSkippedStage"
	ActivityDeleteAssertions               = "DeleteAssertions"
	ActivityFetchPipelineDefinition        = "FetchPipelineDefinition"
	ActivityConsolidateEntries             = "ConsolidateEntries"
	ActivityExtractHeaderMentions          = "ExtractHeaderMentions"
	ActivityEnrichEntities                 = "EnrichEntities"
	ActivityHeartbeatCheckReviewQueue      = "HeartbeatCheckReviewQueue"
	ActivityHeartbeatCheckWatchMatches     = "HeartbeatCheckWatchMatches"
	ActivityHeartbeatCheckStaleContent     = "HeartbeatCheckStaleContent"
	ActivityHeartbeatUpdateStatus          = "HeartbeatUpdateStatus"
	ActivityAttributeProject               = "AttributeProject"
	ActivityInstructionEvaluate            = "InstructionEvaluate"

	// Digest activities for daily/weekly digest generation
	ActivityGatherDigestData        = "GatherDigestData"
	ActivityGenerateDigestNarrative = "GenerateDigestNarrative"
	ActivitySaveDigest              = "SaveDigest"
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
	ActivityUpdateLangfuseTraceTags     = "UpdateLangfuseTraceTags"
	ActivityUpdateLangfuseTraceMetadata = "UpdateLangfuseTraceMetadata"
)

// Graph API (Microsoft Outlook + Teams) activity name constants.
// These are registered on the main task queue and shared by OutlookSyncWorkflow and TeamsSyncWorkflow.
const (
	// Shared
	ActivityCheckGraphAuth = "CheckGraphAuth"

	// Outlook sync activities
	ActivityFetchOutlookMessages  = "FetchOutlookMessages"
	ActivityProcessOutlookMessage = "ProcessOutlookMessage"
	ActivityUpdateOutlookSyncState = "UpdateOutlookSyncState"
	ActivityRollbackOutlookSync   = "RollbackOutlookSync"

	// Teams sync activities
	ActivityFetchTeamChannels    = "FetchTeamChannels"
	ActivityFetchChannelMessages = "FetchChannelMessages"
	ActivityProcessTeamsThread   = "ProcessTeamsThread"
	ActivityUpdateTeamsSyncState = "UpdateTeamsSyncState"
	ActivityRollbackTeamsSync    = "RollbackTeamsSync"

	// Transcript sync activities
	ActivityFetchMeetingTranscripts   = "FetchMeetingTranscripts"
	ActivityProcessTranscriptContent  = "ProcessTranscriptContent"
	ActivityUpdateTranscriptSyncState = "UpdateTranscriptSyncState"
	ActivityRollbackTranscriptSync    = "RollbackTranscriptSync"
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
		ActivityBuildExtractionContext,
		ActivityBuildContextPackage,
		ActivityEnrichPersonMetadata,
		ActivityDeepAnalyze,
		ActivityRecordOverrides,
		ActivityGroupEmailThread,
		ActivityCreateEnrichmentRecord,
		ActivityLinkConversation,
		ActivityBackfillConversationSummaries,
		ActivityRegenerateConversationSummary,
		ActivityCheckStaleConversations,
		ActivityCreateLangfuseTrace,
		ActivityReportLangfusePhase,
		ActivityFinishLangfuseTrace,
		ActivityPersistLangfuseTraceID,
		ActivityUpdateLangfuseTraceTags,
		ActivityUpdateLangfuseTraceMetadata,
		ActivityKickNextPending,
		ActivityRecordSkippedStage,
		ActivityDeleteAssertions,
		ActivityFetchPipelineDefinition,
		ActivityConsolidateEntries,
		ActivityExtractHeaderMentions,
		ActivityEnrichEntities,
		ActivityHeartbeatCheckReviewQueue,
		ActivityHeartbeatCheckWatchMatches,
		ActivityHeartbeatCheckStaleContent,
		ActivityHeartbeatUpdateStatus,
		ActivityAttributeProject,
		ActivityInstructionEvaluate,
		// Digest activities
		ActivityGatherDigestData,
		ActivityGenerateDigestNarrative,
		ActivitySaveDigest,
		// Graph API activities (Outlook + Teams sync + Transcript sync)
		ActivityCheckGraphAuth,
		ActivityFetchOutlookMessages,
		ActivityProcessOutlookMessage,
		ActivityUpdateOutlookSyncState,
		ActivityRollbackOutlookSync,
		ActivityFetchTeamChannels,
		ActivityFetchChannelMessages,
		ActivityProcessTeamsThread,
		ActivityUpdateTeamsSyncState,
		ActivityRollbackTeamsSync,
		ActivityFetchMeetingTranscripts,
		ActivityProcessTranscriptContent,
		ActivityUpdateTranscriptSyncState,
		ActivityRollbackTranscriptSync,
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
// Includes Graph API activities since Outlook/Teams sync workflows run on this queue.
func AllEmailQueueActivities() []string {
	return []string{
		ActivityFetchSource,
		ActivityUpdateSourceStatus,
		ActivityParseEmail,
		// Graph API activities (Outlook + Teams sync + Transcript sync workflows run on email queue)
		ActivityCheckGraphAuth,
		ActivityFetchOutlookMessages,
		ActivityProcessOutlookMessage,
		ActivityUpdateOutlookSyncState,
		ActivityRollbackOutlookSync,
		ActivityFetchTeamChannels,
		ActivityFetchChannelMessages,
		ActivityProcessTeamsThread,
		ActivityUpdateTeamsSyncState,
		ActivityRollbackTeamsSync,
		ActivityFetchMeetingTranscripts,
		ActivityProcessTranscriptContent,
		ActivityUpdateTranscriptSyncState,
		ActivityRollbackTranscriptSync,
	}
}

package temporal

// Workflow name constants for all Temporal workflows across task queues.
// These constants provide a single source of truth for workflow names,
// preventing typos and mismatches between registration and invocation.

// Main Task Queue workflow names.
const (
	WorkflowSLMPipeline               = "SLMPipelineWorkflow"
	WorkflowContentIngestion          = "ContentIngestionWorkflow"
	WorkflowRelationshipDiscovery     = "RelationshipDiscoveryWorkflow"
	WorkflowDailyReview               = "DailyReviewWorkflow"
	WorkflowBatchPipeline             = "BatchPipelineWorkflow"
	WorkflowConversationMaintenance        = "ConversationMaintenanceWorkflow"
	WorkflowConversationBackfill           = "ConversationBackfillWorkflow"
	WorkflowSessionLedgerConsolidation     = "SessionLedgerConsolidationWorkflow"
	WorkflowHeartbeat                      = "HeartbeatWorkflow"
	WorkflowDigest                         = "DigestWorkflow"
	WorkflowWeeklyDigest                   = "WeeklyDigestWorkflow"
)

// AI Task Queue workflow names.
const (
	WorkflowAnalysis = "AnalysisWorkflow"
)

// Email Task Queue workflow names.
const (
	WorkflowEmailProcessing = "EmailProcessingWorkflow"
	WorkflowGmailSync       = "GmailSyncWorkflow"
)

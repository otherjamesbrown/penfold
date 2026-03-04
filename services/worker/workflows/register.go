// Package workflows provides workflow registration for the Temporal worker.
package workflows

import (
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"

	"github.com/otherjamesbrown/penfold/services/worker/config"
)

// Registrar handles workflow registration with Temporal workers.
type Registrar struct {
	// Add dependencies here as needed (e.g., for workflow initialization).
}

// NewRegistrar creates a new workflow registrar.
func NewRegistrar() *Registrar {
	return &Registrar{}
}

// RegisterAll registers all workflows with the given worker based on task queue.
func (r *Registrar) RegisterAll(w worker.Worker, taskQueue string) {
	switch taskQueue {
	case config.MainTaskQueue:
		r.registerMainQueueWorkflows(w)
	case config.AITaskQueue:
		r.registerAIQueueWorkflows(w)
	case config.EmailTaskQueue:
		r.registerEmailQueueWorkflows(w)
	default:
		// Register common workflows for unknown queues
		r.registerCommonWorkflows(w)
	}
}

// registerMainQueueWorkflows registers workflows for the main task queue.
func (r *Registrar) registerMainQueueWorkflows(w worker.Worker) {
	// Register common workflows
	r.registerCommonWorkflows(w)

	// SLM pipeline workflow (primary ingestion path)
	w.RegisterWorkflowWithOptions(SLMPipelineWorkflow, workflow.RegisterOptions{
		Name: "SLMPipelineWorkflow",
	})

	// Content ingestion workflow (legacy)
	w.RegisterWorkflowWithOptions(ContentIngestionWorkflow, workflow.RegisterOptions{
		Name: "ContentIngestionWorkflow",
	})

	// Relationship discovery workflow
	w.RegisterWorkflowWithOptions(RelationshipDiscoveryWorkflow, workflow.RegisterOptions{
		Name: "RelationshipDiscoveryWorkflow",
	})

	// Daily review workflow
	w.RegisterWorkflowWithOptions(DailyReviewWorkflow, workflow.RegisterOptions{
		Name: "DailyReviewWorkflow",
	})

	// Batch pipeline workflow (rate-limited batch processing)
	w.RegisterWorkflowWithOptions(BatchPipelineWorkflow, workflow.RegisterOptions{
		Name: "BatchPipelineWorkflow",
	})

	// Conversation maintenance workflow (stale detection, scheduled daily)
	w.RegisterWorkflowWithOptions(ConversationMaintenanceWorkflow, workflow.RegisterOptions{
		Name: "ConversationMaintenanceWorkflow",
	})

	// Conversation backfill workflow (one-shot summary backfill)
	w.RegisterWorkflowWithOptions(ConversationBackfillWorkflow, workflow.RegisterOptions{
		Name: "ConversationBackfillWorkflow",
	})

	// Session ledger consolidation workflow (daily consolidation of ledger entries)
	w.RegisterWorkflowWithOptions(SessionLedgerConsolidationWorkflow, workflow.RegisterOptions{
		Name: "SessionLedgerConsolidationWorkflow",
	})

	// Heartbeat workflow (scheduled health check aggregation)
	w.RegisterWorkflowWithOptions(HeartbeatWorkflow, workflow.RegisterOptions{
		Name: "HeartbeatWorkflow",
	})

	// Digest workflow (daily digest generation)
	w.RegisterWorkflowWithOptions(DigestWorkflow, workflow.RegisterOptions{
		Name: "DigestWorkflow",
	})

	// Weekly digest workflow (weekly rollup generation)
	w.RegisterWorkflowWithOptions(WeeklyDigestWorkflow, workflow.RegisterOptions{
		Name: "WeeklyDigestWorkflow",
	})
}

// registerAIQueueWorkflows registers workflows for the AI task queue.
func (r *Registrar) registerAIQueueWorkflows(w worker.Worker) {
	// Analysis workflow - runs AI-intensive content analysis
	w.RegisterWorkflowWithOptions(AnalysisWorkflow, workflow.RegisterOptions{
		Name: "AnalysisWorkflow",
	})
}

// registerEmailQueueWorkflows registers workflows for the email task queue.
func (r *Registrar) registerEmailQueueWorkflows(w worker.Worker) {
	// Email processing workflow
	w.RegisterWorkflowWithOptions(EmailProcessingWorkflow, workflow.RegisterOptions{
		Name: "EmailProcessingWorkflow",
	})

	// Gmail sync workflow
	w.RegisterWorkflowWithOptions(GmailSyncWorkflow, workflow.RegisterOptions{
		Name: "GmailSyncWorkflow",
	})

	// Teams channel message sync workflow
	w.RegisterWorkflowWithOptions(TeamsSyncWorkflow, workflow.RegisterOptions{
		Name: "TeamsSyncWorkflow",
	})

	// Outlook email sync workflow
	w.RegisterWorkflowWithOptions(OutlookSyncWorkflow, workflow.RegisterOptions{
		Name: "OutlookSyncWorkflow",
	})

	// Azure AD directory sync workflow
	w.RegisterWorkflowWithOptions(ADSyncWorkflow, workflow.RegisterOptions{
		Name: "ADSyncWorkflow",
	})

	// Teams meeting transcript sync workflow
	w.RegisterWorkflowWithOptions(TranscriptSyncWorkflow, workflow.RegisterOptions{
		Name: "TranscriptSyncWorkflow",
	})
}

// registerCommonWorkflows registers workflows shared across all task queues.
func (r *Registrar) registerCommonWorkflows(w worker.Worker) {
	// Common workflows that may be needed by multiple queues
	// Add as they are implemented:
	// w.RegisterWorkflow(HealthCheckWorkflow)
}

// WorkflowCount returns the count of registered workflows for a given task queue.
func (r *Registrar) WorkflowCount(taskQueue string) int {
	switch taskQueue {
	case config.MainTaskQueue:
		return 11 // SLMPipelineWorkflow, ContentIngestionWorkflow, RelationshipDiscoveryWorkflow, DailyReviewWorkflow, BatchPipelineWorkflow, ConversationMaintenanceWorkflow, ConversationBackfillWorkflow, SessionLedgerConsolidationWorkflow, HeartbeatWorkflow, DigestWorkflow, WeeklyDigestWorkflow
	case config.AITaskQueue:
		return 1 // AnalysisWorkflow
	case config.EmailTaskQueue:
		return 6 // EmailProcessingWorkflow, GmailSyncWorkflow, TeamsSyncWorkflow, OutlookSyncWorkflow, ADSyncWorkflow, TranscriptSyncWorkflow
	default:
		return 0
	}
}

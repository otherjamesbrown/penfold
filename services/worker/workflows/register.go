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

	// Content ingestion workflow
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
		return 3 // ContentIngestionWorkflow, RelationshipDiscoveryWorkflow, DailyReviewWorkflow
	case config.AITaskQueue:
		return 1 // AnalysisWorkflow
	case config.EmailTaskQueue:
		return 2 // EmailProcessingWorkflow, GmailSyncWorkflow
	default:
		return 0
	}
}

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

	// Main queue specific workflows
	// Add as they are implemented:
	// w.RegisterWorkflow(ContentProcessingWorkflow)
	// w.RegisterWorkflow(RelationshipDiscoveryWorkflow)
}

// registerAIQueueWorkflows registers workflows for the AI task queue.
func (r *Registrar) registerAIQueueWorkflows(w worker.Worker) {
	// AI-intensive workflows
	// Add as they are implemented:
	// w.RegisterWorkflow(EmbeddingBatchWorkflow)
	// w.RegisterWorkflow(SummarizationWorkflow)
}

// registerEmailQueueWorkflows registers workflows for the email task queue.
func (r *Registrar) registerEmailQueueWorkflows(w worker.Worker) {
	// Email processing workflows
	w.RegisterWorkflowWithOptions(EmailProcessingWorkflow, workflow.RegisterOptions{
		Name: "EmailProcessingWorkflow",
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
		return 0 // Will increase as workflows are added
	case config.AITaskQueue:
		return 0
	case config.EmailTaskQueue:
		return 1 // EmailProcessingWorkflow
	default:
		return 0
	}
}

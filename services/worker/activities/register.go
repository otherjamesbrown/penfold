// Package activities provides activity registration for the Temporal worker.
package activities

import (
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/worker"

	"github.com/otherjamesbrown/penfold/services/worker/config"
)

// Registrar handles activity registration with Temporal workers.
type Registrar struct {
	// Activities holds the activity implementations.
	// This will be populated with actual implementations when they are ready.
	activities *Activities
}

// NewRegistrar creates a new activity registrar.
func NewRegistrar(activities *Activities) *Registrar {
	return &Registrar{
		activities: activities,
	}
}

// RegisterAll registers all activities with the given worker based on task queue.
func (r *Registrar) RegisterAll(w worker.Worker, taskQueue string) {
	switch taskQueue {
	case config.MainTaskQueue:
		r.registerMainQueueActivities(w)
	case config.AITaskQueue:
		r.registerAIQueueActivities(w)
	case config.EmailTaskQueue:
		r.registerEmailQueueActivities(w)
	default:
		// Register common activities for unknown queues
		r.registerCommonActivities(w)
	}
}

// registerMainQueueActivities registers activities for the main task queue.
func (r *Registrar) registerMainQueueActivities(w worker.Worker) {
	// Register common activities
	r.registerCommonActivities(w)

	// Main queue specific activities
	// Add as they are implemented:
	// w.RegisterActivity(r.activities.ProcessContent)
	// w.RegisterActivity(r.activities.DiscoverRelationships)
}

// registerAIQueueActivities registers activities for the AI task queue.
func (r *Registrar) registerAIQueueActivities(w worker.Worker) {
	// AI-intensive activities
	if r.activities != nil {
		w.RegisterActivityWithOptions(r.activities.GenerateEmbedding, activity.RegisterOptions{
			Name: "GenerateEmbedding",
		})
		w.RegisterActivityWithOptions(r.activities.GenerateSummary, activity.RegisterOptions{
			Name: "GenerateSummary",
		})
		w.RegisterActivityWithOptions(r.activities.ExtractAssertions, activity.RegisterOptions{
			Name: "ExtractAssertions",
		})
	}
}

// registerEmailQueueActivities registers activities for the email task queue.
func (r *Registrar) registerEmailQueueActivities(w worker.Worker) {
	// Email processing activities
	if r.activities != nil {
		w.RegisterActivityWithOptions(r.activities.FetchSource, activity.RegisterOptions{
			Name: "FetchSource",
		})
		w.RegisterActivityWithOptions(r.activities.UpdateSourceStatus, activity.RegisterOptions{
			Name: "UpdateSourceStatus",
		})
	}

	// Also register AI activities since email processing needs them
	r.registerAIQueueActivities(w)
}

// registerCommonActivities registers activities shared across all task queues.
func (r *Registrar) registerCommonActivities(w worker.Worker) {
	// Common activities that may be needed by multiple queues
	// Add as they are implemented:
	// w.RegisterActivity(r.activities.HealthCheck)
}

// ActivityCount returns the count of registered activities for a given task queue.
func (r *Registrar) ActivityCount(taskQueue string) int {
	switch taskQueue {
	case config.MainTaskQueue:
		return 0 // Will increase as activities are added
	case config.AITaskQueue:
		return 3 // GenerateEmbedding, GenerateSummary, ExtractAssertions
	case config.EmailTaskQueue:
		return 5 // FetchSource, UpdateSourceStatus + AI activities
	default:
		return 0
	}
}

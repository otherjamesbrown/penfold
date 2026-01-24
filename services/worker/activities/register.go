// Package activities provides activity registration for the Temporal worker.
package activities

import (
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/worker"

	"github.com/otherjamesbrown/penfold/services/worker/config"
)

// Registrar handles activity registration with Temporal workers.
type Registrar struct {
	// Activities holds the legacy activity implementations.
	// This will be deprecated as activities migrate to use AIClient.
	activities *Activities

	// Specialized activity implementations using AIClient
	embeddingActivities      *EmbeddingActivities
	summarizationActivities  *SummarizationActivities
	extractionActivities     *ExtractionActivities
	mentionsActivities       *MentionsActivities
}

// NewRegistrar creates a new activity registrar.
func NewRegistrar(activities *Activities) *Registrar {
	return &Registrar{
		activities: activities,
	}
}

// WithMentionsActivities adds mentions activities to the registrar.
func (r *Registrar) WithMentionsActivities(ma *MentionsActivities) *Registrar {
	r.mentionsActivities = ma
	return r
}

// WithEmbeddingActivities adds embedding activities to the registrar.
func (r *Registrar) WithEmbeddingActivities(ea *EmbeddingActivities) *Registrar {
	r.embeddingActivities = ea
	return r
}

// WithSummarizationActivities adds summarization activities to the registrar.
func (r *Registrar) WithSummarizationActivities(sa *SummarizationActivities) *Registrar {
	r.summarizationActivities = sa
	return r
}

// WithExtractionActivities adds extraction activities to the registrar.
func (r *Registrar) WithExtractionActivities(ea *ExtractionActivities) *Registrar {
	r.extractionActivities = ea
	return r
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
	// AI-intensive activities using AIClient (preferred)
	if r.embeddingActivities != nil {
		w.RegisterActivityWithOptions(r.embeddingActivities.GenerateEmbedding, activity.RegisterOptions{
			Name: "GenerateEmbedding",
		})
		w.RegisterActivityWithOptions(r.embeddingActivities.GenerateEmbeddingBatch, activity.RegisterOptions{
			Name: "GenerateEmbeddingBatch",
		})
	} else if r.activities != nil {
		// Fallback to legacy activities if AIClient-based activities not configured
		w.RegisterActivityWithOptions(r.activities.GenerateEmbedding, activity.RegisterOptions{
			Name: "GenerateEmbedding",
		})
	}

	if r.summarizationActivities != nil {
		w.RegisterActivityWithOptions(r.summarizationActivities.GenerateSummary, activity.RegisterOptions{
			Name: "GenerateSummary",
		})
		w.RegisterActivityWithOptions(r.summarizationActivities.GenerateSummaryWithOptions, activity.RegisterOptions{
			Name: "GenerateSummaryWithOptions",
		})
	} else if r.activities != nil {
		// Fallback to legacy activities
		w.RegisterActivityWithOptions(r.activities.GenerateSummary, activity.RegisterOptions{
			Name: "GenerateSummary",
		})
	}

	if r.extractionActivities != nil {
		w.RegisterActivityWithOptions(r.extractionActivities.ExtractAssertions, activity.RegisterOptions{
			Name: "ExtractAssertions",
		})
		w.RegisterActivityWithOptions(r.extractionActivities.ExtractEntities, activity.RegisterOptions{
			Name: "ExtractEntities",
		})
	} else if r.activities != nil {
		// Fallback to legacy activities
		w.RegisterActivityWithOptions(r.activities.ExtractAssertions, activity.RegisterOptions{
			Name: "ExtractAssertions",
		})
	}

	// Mentions extraction activity (LLM-driven, so registered with AI queue)
	if r.mentionsActivities != nil {
		w.RegisterActivityWithOptions(r.mentionsActivities.ExtractMentions, activity.RegisterOptions{
			Name: "ExtractMentions",
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
		count := 0
		// Count embedding activities
		if r.embeddingActivities != nil {
			count += 2 // GenerateEmbedding, GenerateEmbeddingBatch
		} else if r.activities != nil {
			count += 1 // GenerateEmbedding (legacy)
		}
		// Count summarization activities
		if r.summarizationActivities != nil {
			count += 2 // GenerateSummary, GenerateSummaryWithOptions
		} else if r.activities != nil {
			count += 1 // GenerateSummary (legacy)
		}
		// Count extraction activities
		if r.extractionActivities != nil {
			count += 2 // ExtractAssertions, ExtractEntities
		} else if r.activities != nil {
			count += 1 // ExtractAssertions (legacy)
		}
		if r.mentionsActivities != nil {
			count++ // ExtractMentions
		}
		return count
	case config.EmailTaskQueue:
		count := 2 // FetchSource, UpdateSourceStatus
		// Add AI activities count
		count += r.ActivityCount(config.AITaskQueue)
		return count
	default:
		return 0
	}
}

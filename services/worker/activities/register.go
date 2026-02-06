// Package activities provides activity registration for the Temporal worker.
package activities

import (
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/worker"

	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
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
	parseActivities          *ParseActivities
	persistActivities        *PersistActivities
	triageActivities         *TriageActivities
	contextBuilderActivities *ContextBuilderActivities
	analysisActivities       *AnalysisActivities
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

// WithParseActivities adds parse activities to the registrar.
func (r *Registrar) WithParseActivities(pa *ParseActivities) *Registrar {
	r.parseActivities = pa
	return r
}

// WithPersistActivities adds persist activities to the registrar.
func (r *Registrar) WithPersistActivities(pa *PersistActivities) *Registrar {
	r.persistActivities = pa
	return r
}

// WithTriageActivities adds triage activities to the registrar.
func (r *Registrar) WithTriageActivities(ta *TriageActivities) *Registrar {
	r.triageActivities = ta
	return r
}

// WithContextBuilderActivities adds context builder activities to the registrar.
func (r *Registrar) WithContextBuilderActivities(cba *ContextBuilderActivities) *Registrar {
	r.contextBuilderActivities = cba
	return r
}

// WithAnalysisActivities adds analysis activities to the registrar.
func (r *Registrar) WithAnalysisActivities(aa *AnalysisActivities) *Registrar {
	r.analysisActivities = aa
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

	// ContentIngestionWorkflow needs these activities
	if r.activities != nil {
		// ValidateContent - validates content before processing
		w.RegisterActivityWithOptions(r.activities.ValidateContent, activity.RegisterOptions{
			Name: pkgtemporal.ActivityValidateContent,
		})
		// FetchContent - fetches source content from database
		w.RegisterActivityWithOptions(r.activities.FetchSource, activity.RegisterOptions{
			Name: pkgtemporal.ActivityFetchContent,
		})
		// UpdateContentStatus - updates source processing status
		w.RegisterActivityWithOptions(r.activities.UpdateSourceStatus, activity.RegisterOptions{
			Name: pkgtemporal.ActivityUpdateContentStatus,
		})
	}

	// Embedding activities for content processing
	if r.embeddingActivities != nil {
		w.RegisterActivityWithOptions(r.embeddingActivities.GenerateEmbedding, activity.RegisterOptions{
			Name: pkgtemporal.ActivityGenerateContentEmbedding,
		})
		w.RegisterActivityWithOptions(r.embeddingActivities.DeleteEmbedding, activity.RegisterOptions{
			Name: pkgtemporal.ActivityDeleteEmbedding,
		})
	} else if r.activities != nil {
		w.RegisterActivityWithOptions(r.activities.GenerateEmbedding, activity.RegisterOptions{
			Name: pkgtemporal.ActivityGenerateContentEmbedding,
		})
	}

	// Summary activities for content processing
	if r.summarizationActivities != nil {
		w.RegisterActivityWithOptions(r.summarizationActivities.GenerateSummary, activity.RegisterOptions{
			Name: pkgtemporal.ActivityGenerateContentSummary,
		})
		w.RegisterActivityWithOptions(r.summarizationActivities.DeleteSummary, activity.RegisterOptions{
			Name: pkgtemporal.ActivityDeleteSummary,
		})
	} else if r.activities != nil {
		w.RegisterActivityWithOptions(r.activities.GenerateSummary, activity.RegisterOptions{
			Name: pkgtemporal.ActivityGenerateContentSummary,
		})
	}

	// Extraction activities for content processing
	if r.extractionActivities != nil {
		w.RegisterActivityWithOptions(r.extractionActivities.ExtractEntities, activity.RegisterOptions{
			Name: pkgtemporal.ActivityExtractEntitiesActivity,
		})
		w.RegisterActivityWithOptions(r.extractionActivities.ExtractAssertions, activity.RegisterOptions{
			Name: pkgtemporal.ActivityExtractAssertions,
		})
	} else if r.activities != nil {
		w.RegisterActivityWithOptions(r.activities.ExtractAssertions, activity.RegisterOptions{
			Name: pkgtemporal.ActivityExtractAssertions,
		})
	}

	// Mentions extraction for content processing
	if r.mentionsActivities != nil {
		w.RegisterActivityWithOptions(r.mentionsActivities.ExtractMentions, activity.RegisterOptions{
			Name: pkgtemporal.ActivityExtractMentions,
		})
	}

	// Parse activities for content processing (Stage 0, deterministic)
	if r.parseActivities != nil {
		w.RegisterActivityWithOptions(r.parseActivities.ParseEmail, activity.RegisterOptions{
			Name: pkgtemporal.ActivityParseEmail,
		})
		w.RegisterActivityWithOptions(r.parseActivities.ParseTranscript, activity.RegisterOptions{
			Name: pkgtemporal.ActivityParseTranscript,
		})
	}

	// Persist activities for Stage 4.5 (persistence)
	if r.persistActivities != nil {
		w.RegisterActivityWithOptions(r.persistActivities.PersistFindings, activity.RegisterOptions{
			Name: pkgtemporal.ActivityPersistFindings,
		})
	}

	// Triage activities for Stage 1 (triage)
	if r.triageActivities != nil {
		w.RegisterActivityWithOptions(r.triageActivities.Triage, activity.RegisterOptions{
			Name: pkgtemporal.ActivityTriage,
		})
	}

	// Context builder activities for Stage 3 (context assembly)
	if r.contextBuilderActivities != nil {
		w.RegisterActivityWithOptions(r.contextBuilderActivities.BuildContextPackage, activity.RegisterOptions{
			Name: pkgtemporal.ActivityBuildContextPackage,
		})
	}

	// Analysis activities for Stage 4 (deep analysis)
	if r.analysisActivities != nil {
		w.RegisterActivityWithOptions(r.analysisActivities.DeepAnalyze, activity.RegisterOptions{
			Name: pkgtemporal.ActivityDeepAnalyze,
		})
	}
}

// registerAIQueueActivities registers activities for the AI task queue.
func (r *Registrar) registerAIQueueActivities(w worker.Worker) {
	// AI-intensive activities using AIClient (preferred)
	if r.embeddingActivities != nil {
		w.RegisterActivityWithOptions(r.embeddingActivities.GenerateEmbedding, activity.RegisterOptions{
			Name: pkgtemporal.ActivityGenerateEmbedding,
		})
		w.RegisterActivityWithOptions(r.embeddingActivities.GenerateEmbeddingBatch, activity.RegisterOptions{
			Name: pkgtemporal.ActivityGenerateEmbeddingBatch,
		})
	} else if r.activities != nil {
		// Fallback to legacy activities if AIClient-based activities not configured
		w.RegisterActivityWithOptions(r.activities.GenerateEmbedding, activity.RegisterOptions{
			Name: pkgtemporal.ActivityGenerateEmbedding,
		})
	}

	if r.summarizationActivities != nil {
		w.RegisterActivityWithOptions(r.summarizationActivities.GenerateSummary, activity.RegisterOptions{
			Name: pkgtemporal.ActivityGenerateSummary,
		})
		w.RegisterActivityWithOptions(r.summarizationActivities.GenerateSummaryWithOptions, activity.RegisterOptions{
			Name: pkgtemporal.ActivityGenerateSummaryWithOptions,
		})
	} else if r.activities != nil {
		// Fallback to legacy activities
		w.RegisterActivityWithOptions(r.activities.GenerateSummary, activity.RegisterOptions{
			Name: pkgtemporal.ActivityGenerateSummary,
		})
	}

	if r.extractionActivities != nil {
		w.RegisterActivityWithOptions(r.extractionActivities.ExtractAssertions, activity.RegisterOptions{
			Name: pkgtemporal.ActivityExtractAssertions,
		})
		w.RegisterActivityWithOptions(r.extractionActivities.ExtractEntities, activity.RegisterOptions{
			Name: pkgtemporal.ActivityExtractEntitiesActivity,
		})
	} else if r.activities != nil {
		// Fallback to legacy activities
		w.RegisterActivityWithOptions(r.activities.ExtractAssertions, activity.RegisterOptions{
			Name: pkgtemporal.ActivityExtractAssertions,
		})
	}

	// Mentions extraction activity (LLM-driven, so registered with AI queue)
	if r.mentionsActivities != nil {
		w.RegisterActivityWithOptions(r.mentionsActivities.ExtractMentions, activity.RegisterOptions{
			Name: pkgtemporal.ActivityExtractMentions,
		})
	}
}

// registerEmailQueueActivities registers activities for the email task queue.
func (r *Registrar) registerEmailQueueActivities(w worker.Worker) {
	// Email processing activities
	if r.activities != nil {
		w.RegisterActivityWithOptions(r.activities.FetchSource, activity.RegisterOptions{
			Name: pkgtemporal.ActivityFetchSource,
		})
		w.RegisterActivityWithOptions(r.activities.UpdateSourceStatus, activity.RegisterOptions{
			Name: pkgtemporal.ActivityUpdateSourceStatus,
		})
	}

	// Parse activities for email processing (Stage 0, deterministic)
	if r.parseActivities != nil {
		w.RegisterActivityWithOptions(r.parseActivities.ParseEmail, activity.RegisterOptions{
			Name: pkgtemporal.ActivityParseEmail,
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
		count := 0
		// ValidateContent, FetchContent, UpdateContentStatus
		if r.activities != nil {
			count += 3
		}
		// GenerateContentEmbedding, DeleteEmbedding
		if r.embeddingActivities != nil {
			count += 2
		} else if r.activities != nil {
			count += 1
		}
		// GenerateContentSummary, DeleteSummary
		if r.summarizationActivities != nil {
			count += 2
		} else if r.activities != nil {
			count += 1
		}
		// ExtractEntities, ExtractAssertions
		if r.extractionActivities != nil {
			count += 2
		} else if r.activities != nil {
			count += 1 // ExtractAssertions (legacy)
		}
		// ExtractMentions
		if r.mentionsActivities != nil {
			count += 1
		}
		// ParseEmail, ParseTranscript
		if r.parseActivities != nil {
			count += 2
		}
		// PersistFindings
		if r.persistActivities != nil {
			count += 1
		}
		// Triage
		if r.triageActivities != nil {
			count += 1
		}
		// BuildContextPackage
		if r.contextBuilderActivities != nil {
			count += 1
		}
		// DeepAnalyze
		if r.analysisActivities != nil {
			count += 1
		}
		return count
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
		// ParseEmail
		if r.parseActivities != nil {
			count += 1
		}
		// Add AI activities count
		count += r.ActivityCount(config.AITaskQueue)
		return count
	default:
		return 0
	}
}

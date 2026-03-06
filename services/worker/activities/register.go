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
	embeddingActivities        *EmbeddingActivities
	summarizationActivities    *SummarizationActivities
	extractionActivities       *ExtractionActivities
	mentionsActivities         *MentionsActivities
	parseActivities            *ParseActivities
	persistActivities          *PersistActivities
	triageActivities           *TriageActivities
	contextBuilderActivities   *ContextBuilderActivities
	analysisActivities         *AnalysisActivities
	pipelineActivities         *PipelineActivities
	personEnrichmentActivities *PersonEnrichmentActivities
	projectTaggingActivities   *ProjectTaggingActivities
	attributionActivities      *AttributionActivities
	threadActivities           *ThreadActivities
	enrichmentActivities       *EnrichmentActivities
	conversationActivities     *ConversationActivities
	langfuseActivities         *LangfuseActivities
	consolidationActivities    *ConsolidationActivities
	headerMentionsActivities   *HeaderMentionsActivities
	entityEnrichmentActivities *EntityEnrichmentActivities
	heartbeatActivities               *HeartbeatActivities
	graphActivities                   *GraphActivities
	instructionEvaluationActivities   *InstructionEvaluationActivities
	digestActivities                  *DigestActivities
	newsletterExtractActivities       *NewsletterExtractActivities
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

// WithPipelineActivities adds pipeline activities to the registrar.
func (r *Registrar) WithPipelineActivities(pa *PipelineActivities) *Registrar {
	r.pipelineActivities = pa
	return r
}

// WithPersonEnrichmentActivities adds person enrichment activities to the registrar.
func (r *Registrar) WithPersonEnrichmentActivities(pea *PersonEnrichmentActivities) *Registrar {
	r.personEnrichmentActivities = pea
	return r
}

// WithProjectTaggingActivities adds project tagging activities to the registrar.
func (r *Registrar) WithProjectTaggingActivities(pta *ProjectTaggingActivities) *Registrar {
	r.projectTaggingActivities = pta
	return r
}

// WithAttributionActivities adds project attribution activities to the registrar.
func (r *Registrar) WithAttributionActivities(aa *AttributionActivities) *Registrar {
	r.attributionActivities = aa
	return r
}

// WithThreadActivities adds thread activities to the registrar.
func (r *Registrar) WithThreadActivities(ta *ThreadActivities) *Registrar {
	r.threadActivities = ta
	return r
}

// WithEnrichmentActivities adds enrichment activities to the registrar.
func (r *Registrar) WithEnrichmentActivities(ea *EnrichmentActivities) *Registrar {
	r.enrichmentActivities = ea
	return r
}

// WithConversationActivities adds conversation activities to the registrar.
func (r *Registrar) WithConversationActivities(ca *ConversationActivities) *Registrar {
	r.conversationActivities = ca
	return r
}

// WithLangfuseActivities adds Langfuse ingestion activities to the registrar.
func (r *Registrar) WithLangfuseActivities(la *LangfuseActivities) *Registrar {
	r.langfuseActivities = la
	return r
}

// WithConsolidationActivities adds session ledger consolidation activities to the registrar.
func (r *Registrar) WithConsolidationActivities(ca *ConsolidationActivities) *Registrar {
	r.consolidationActivities = ca
	return r
}

// WithHeaderMentionsActivities adds header-based mention extraction activities to the registrar.
func (r *Registrar) WithHeaderMentionsActivities(hma *HeaderMentionsActivities) *Registrar {
	r.headerMentionsActivities = hma
	return r
}

// WithEntityEnrichmentActivities adds entity enrichment activities to the registrar.
func (r *Registrar) WithEntityEnrichmentActivities(ea *EntityEnrichmentActivities) *Registrar {
	r.entityEnrichmentActivities = ea
	return r
}

// WithHeartbeatActivities adds heartbeat activities to the registrar.
func (r *Registrar) WithHeartbeatActivities(ha *HeartbeatActivities) *Registrar {
	r.heartbeatActivities = ha
	return r
}

// WithGraphActivities adds Microsoft Graph API activities (Outlook + Teams) to the registrar.
func (r *Registrar) WithGraphActivities(ga *GraphActivities) *Registrar {
	r.graphActivities = ga
	return r
}

// WithInstructionEvaluationActivities adds instruction evaluation activities to the registrar.
func (r *Registrar) WithInstructionEvaluationActivities(iea *InstructionEvaluationActivities) *Registrar {
	r.instructionEvaluationActivities = iea
	return r
}

// WithDigestActivities adds digest generation activities to the registrar.
func (r *Registrar) WithDigestActivities(da *DigestActivities) *Registrar {
	r.digestActivities = da
	return r
}

// WithNewsletterExtractActivities adds newsletter structured extraction activities to the registrar.
func (r *Registrar) WithNewsletterExtractActivities(nea *NewsletterExtractActivities) *Registrar {
	r.newsletterExtractActivities = nea
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
		w.RegisterActivityWithOptions(r.extractionActivities.DeleteAssertions, activity.RegisterOptions{
			Name: pkgtemporal.ActivityDeleteAssertions,
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

	// Header-based mention extraction (deterministic, from email From/To/CC headers)
	if r.headerMentionsActivities != nil {
		w.RegisterActivityWithOptions(r.headerMentionsActivities.ExtractHeaderMentions, activity.RegisterOptions{
			Name: pkgtemporal.ActivityExtractHeaderMentions,
		})
	}

	// Project tagging for content processing
	if r.projectTaggingActivities != nil {
		w.RegisterActivityWithOptions(r.projectTaggingActivities.TagProjects, activity.RegisterOptions{
			Name: pkgtemporal.ActivityTagProjects,
		})
	}

	// Project attribution for assertion-level attribution
	if r.attributionActivities != nil {
		w.RegisterActivityWithOptions(r.attributionActivities.AttributeProject, activity.RegisterOptions{
			Name: pkgtemporal.ActivityAttributeProject,
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
		w.RegisterActivityWithOptions(r.contextBuilderActivities.BuildExtractionContext, activity.RegisterOptions{
			Name: pkgtemporal.ActivityBuildExtractionContext,
		})
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

	// Pipeline activities for pipeline metadata recording
	if r.pipelineActivities != nil {
		w.RegisterActivityWithOptions(r.pipelineActivities.RecordOverrides, activity.RegisterOptions{
			Name: pkgtemporal.ActivityRecordOverrides,
		})
		// KickNextPending - auto-drain: kicks next pending item after workflow completion
		w.RegisterActivityWithOptions(r.pipelineActivities.KickNextPending, activity.RegisterOptions{
			Name: pkgtemporal.ActivityKickNextPending,
		})
		// RecordSkippedStage - records pipeline_runs rows for stages skipped by gating logic
		w.RegisterActivityWithOptions(r.pipelineActivities.RecordSkippedStage, activity.RegisterOptions{
			Name: pkgtemporal.ActivityRecordSkippedStage,
		})
		// FetchPipelineDefinition - reads pipeline_definitions from DB for workflow stage config
		w.RegisterActivityWithOptions(r.pipelineActivities.FetchPipelineDefinition, activity.RegisterOptions{
			Name: pkgtemporal.ActivityFetchPipelineDefinition,
		})
	}

	// Person enrichment activities for Stage 3.5 (entity enrichment)
	if r.personEnrichmentActivities != nil {
		w.RegisterActivityWithOptions(r.personEnrichmentActivities.EnrichPersonMetadata, activity.RegisterOptions{
			Name: pkgtemporal.ActivityEnrichPersonMetadata,
		})
	}

	// Entity enrichment activities for enrich_entities stage
	if r.entityEnrichmentActivities != nil {
		w.RegisterActivityWithOptions(r.entityEnrichmentActivities.EnrichEntities, activity.RegisterOptions{
			Name: pkgtemporal.ActivityEnrichEntities,
		})
	}

	// Thread activities for Stage 2.5 (email threading)
	if r.threadActivities != nil {
		w.RegisterActivityWithOptions(r.threadActivities.GroupEmailThread, activity.RegisterOptions{
			Name: pkgtemporal.ActivityGroupEmailThread,
		})
	}

	// Enrichment activities for content_enrichment record creation
	if r.enrichmentActivities != nil {
		w.RegisterActivityWithOptions(r.enrichmentActivities.CreateEnrichmentRecord, activity.RegisterOptions{
			Name: pkgtemporal.ActivityCreateEnrichmentRecord,
		})
	}

	// Conversation activities for conversation auto-linking and management
	if r.conversationActivities != nil {
		w.RegisterActivityWithOptions(r.conversationActivities.LinkConversation, activity.RegisterOptions{
			Name: pkgtemporal.ActivityLinkConversation,
		})
		w.RegisterActivityWithOptions(r.conversationActivities.BackfillConversationSummaries, activity.RegisterOptions{
			Name: pkgtemporal.ActivityBackfillConversationSummaries,
		})
		w.RegisterActivityWithOptions(r.conversationActivities.RegenerateConversationSummary, activity.RegisterOptions{
			Name: pkgtemporal.ActivityRegenerateConversationSummary,
		})
		w.RegisterActivityWithOptions(r.conversationActivities.CheckStaleConversations, activity.RegisterOptions{
			Name: pkgtemporal.ActivityCheckStaleConversations,
		})
	}

	// Consolidation activities for session ledger daily consolidation
	if r.consolidationActivities != nil {
		w.RegisterActivityWithOptions(r.consolidationActivities.ConsolidateEntries, activity.RegisterOptions{
			Name: pkgtemporal.ActivityConsolidateEntries,
		})
	}

	// Heartbeat activities for scheduled health checks
	if r.heartbeatActivities != nil {
		w.RegisterActivityWithOptions(
			r.heartbeatActivities.CheckReviewQueue,
			activity.RegisterOptions{Name: pkgtemporal.ActivityHeartbeatCheckReviewQueue},
		)
		w.RegisterActivityWithOptions(
			r.heartbeatActivities.CheckWatchMatches,
			activity.RegisterOptions{Name: pkgtemporal.ActivityHeartbeatCheckWatchMatches},
		)
		w.RegisterActivityWithOptions(
			r.heartbeatActivities.CheckStaleContent,
			activity.RegisterOptions{Name: pkgtemporal.ActivityHeartbeatCheckStaleContent},
		)
		w.RegisterActivityWithOptions(
			r.heartbeatActivities.UpdateScheduleStatus,
			activity.RegisterOptions{Name: pkgtemporal.ActivityHeartbeatUpdateStatus},
		)
	}

	// Langfuse ingestion activities for pipeline trace reporting
	if r.langfuseActivities != nil {
		w.RegisterActivityWithOptions(r.langfuseActivities.CreateLangfuseTrace, activity.RegisterOptions{
			Name: pkgtemporal.ActivityCreateLangfuseTrace,
		})
		w.RegisterActivityWithOptions(r.langfuseActivities.ReportLangfusePhase, activity.RegisterOptions{
			Name: pkgtemporal.ActivityReportLangfusePhase,
		})
		w.RegisterActivityWithOptions(r.langfuseActivities.FinishLangfuseTrace, activity.RegisterOptions{
			Name: pkgtemporal.ActivityFinishLangfuseTrace,
		})
		w.RegisterActivityWithOptions(r.langfuseActivities.PersistLangfuseTraceID, activity.RegisterOptions{
			Name: pkgtemporal.ActivityPersistLangfuseTraceID,
		})
		w.RegisterActivityWithOptions(r.langfuseActivities.UpdateLangfuseTraceTags, activity.RegisterOptions{
			Name: pkgtemporal.ActivityUpdateLangfuseTraceTags,
		})
		w.RegisterActivityWithOptions(r.langfuseActivities.UpdateLangfuseTraceMetadata, activity.RegisterOptions{
			Name: pkgtemporal.ActivityUpdateLangfuseTraceMetadata,
		})
	}

	// Instruction evaluation activities for watch instructions
	if r.instructionEvaluationActivities != nil {
		w.RegisterActivityWithOptions(r.instructionEvaluationActivities.InstructionEvaluate, activity.RegisterOptions{
			Name: pkgtemporal.ActivityInstructionEvaluate,
		})
	}

	// Digest activities for daily/weekly digest generation
	if r.digestActivities != nil {
		w.RegisterActivityWithOptions(r.digestActivities.GatherDigestData, activity.RegisterOptions{
			Name: pkgtemporal.ActivityGatherDigestData,
		})
		w.RegisterActivityWithOptions(r.digestActivities.GenerateDigestNarrative, activity.RegisterOptions{
			Name: pkgtemporal.ActivityGenerateDigestNarrative,
		})
		w.RegisterActivityWithOptions(r.digestActivities.SaveDigest, activity.RegisterOptions{
			Name: pkgtemporal.ActivitySaveDigest,
		})
		// Weekly digest activities
		w.RegisterActivityWithOptions(r.digestActivities.GatherWeeklyDigestData, activity.RegisterOptions{
			Name: pkgtemporal.ActivityGatherWeeklyDigestData,
		})
		w.RegisterActivityWithOptions(r.digestActivities.GenerateWeeklyNarrative, activity.RegisterOptions{
			Name: pkgtemporal.ActivityGenerateWeeklyNarrative,
		})
		w.RegisterActivityWithOptions(r.digestActivities.UpdateThemeContexts, activity.RegisterOptions{
			Name: pkgtemporal.ActivityUpdateThemeContexts,
		})
		// Journal digest activities
		w.RegisterActivityWithOptions(r.digestActivities.GatherJournalData, activity.RegisterOptions{
			Name: pkgtemporal.ActivityGatherJournalData,
		})
		w.RegisterActivityWithOptions(r.digestActivities.GenerateJournalNarrative, activity.RegisterOptions{
			Name: pkgtemporal.ActivityGenerateJournalNarrative,
		})
	}

	// Newsletter extraction activities for newsletter_extract pipeline stage
	if r.newsletterExtractActivities != nil {
		w.RegisterActivityWithOptions(r.newsletterExtractActivities.NewsletterExtract, activity.RegisterOptions{
			Name: pkgtemporal.ActivityNewsletterExtract,
		})
		w.RegisterActivityWithOptions(r.newsletterExtractActivities.PersistExtractedData, activity.RegisterOptions{
			Name: pkgtemporal.ActivityPersistExtractedData,
		})
	}

	// Graph activities for Outlook and Teams sync workflows
	r.registerGraphActivities(w)
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

	// Graph activities for Outlook and Teams sync workflows (both run on the email queue)
	r.registerGraphActivities(w)
}

// registerGraphActivities registers Microsoft Graph API activities.
// Called from both main and email queues since Outlook/Teams sync workflows
// are registered on the email task queue.
func (r *Registrar) registerGraphActivities(w worker.Worker) {
	if r.graphActivities == nil {
		return
	}
	// Shared
	w.RegisterActivityWithOptions(r.graphActivities.CheckGraphAuth, activity.RegisterOptions{
		Name: pkgtemporal.ActivityCheckGraphAuth,
	})
	// Outlook
	w.RegisterActivityWithOptions(r.graphActivities.FetchOutlookMessages, activity.RegisterOptions{
		Name: pkgtemporal.ActivityFetchOutlookMessages,
	})
	w.RegisterActivityWithOptions(r.graphActivities.ProcessOutlookMessage, activity.RegisterOptions{
		Name: pkgtemporal.ActivityProcessOutlookMessage,
	})
	w.RegisterActivityWithOptions(r.graphActivities.UpdateOutlookSyncState, activity.RegisterOptions{
		Name: pkgtemporal.ActivityUpdateOutlookSyncState,
	})
	w.RegisterActivityWithOptions(r.graphActivities.RollbackOutlookSync, activity.RegisterOptions{
		Name: pkgtemporal.ActivityRollbackOutlookSync,
	})
	// Teams
	w.RegisterActivityWithOptions(r.graphActivities.FetchTeamChannels, activity.RegisterOptions{
		Name: pkgtemporal.ActivityFetchTeamChannels,
	})
	w.RegisterActivityWithOptions(r.graphActivities.FetchChannelMessages, activity.RegisterOptions{
		Name: pkgtemporal.ActivityFetchChannelMessages,
	})
	w.RegisterActivityWithOptions(r.graphActivities.ProcessTeamsThread, activity.RegisterOptions{
		Name: pkgtemporal.ActivityProcessTeamsThread,
	})
	w.RegisterActivityWithOptions(r.graphActivities.UpdateTeamsSyncState, activity.RegisterOptions{
		Name: pkgtemporal.ActivityUpdateTeamsSyncState,
	})
	w.RegisterActivityWithOptions(r.graphActivities.RollbackTeamsSync, activity.RegisterOptions{
		Name: pkgtemporal.ActivityRollbackTeamsSync,
	})
	// Transcript sync
	w.RegisterActivityWithOptions(r.graphActivities.FetchMeetingTranscripts, activity.RegisterOptions{
		Name: pkgtemporal.ActivityFetchMeetingTranscripts,
	})
	w.RegisterActivityWithOptions(r.graphActivities.ProcessTranscriptContent, activity.RegisterOptions{
		Name: pkgtemporal.ActivityProcessTranscriptContent,
	})
	w.RegisterActivityWithOptions(r.graphActivities.UpdateTranscriptSyncState, activity.RegisterOptions{
		Name: pkgtemporal.ActivityUpdateTranscriptSyncState,
	})
	w.RegisterActivityWithOptions(r.graphActivities.RollbackTranscriptSync, activity.RegisterOptions{
		Name: pkgtemporal.ActivityRollbackTranscriptSync,
	})
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
		// ExtractEntities, ExtractAssertions, DeleteAssertions
		if r.extractionActivities != nil {
			count += 3
		} else if r.activities != nil {
			count += 1 // ExtractAssertions (legacy)
		}
		// ExtractMentions
		if r.mentionsActivities != nil {
			count += 1
		}
		// ExtractHeaderMentions
		if r.headerMentionsActivities != nil {
			count += 1
		}
		// TagProjects
		if r.projectTaggingActivities != nil {
			count += 1
		}
		// AttributeProject
		if r.attributionActivities != nil {
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
		// BuildExtractionContext, BuildContextPackage
		if r.contextBuilderActivities != nil {
			count += 2
		}
		// DeepAnalyze
		if r.analysisActivities != nil {
			count += 1
		}
		// RecordOverrides, KickNextPending, RecordSkippedStage, FetchPipelineDefinition
		if r.pipelineActivities != nil {
			count += 4
		}
		// EnrichPersonMetadata
		if r.personEnrichmentActivities != nil {
			count += 1
		}
		// EnrichEntities
		if r.entityEnrichmentActivities != nil {
			count += 1
		}
		// GroupEmailThread
		if r.threadActivities != nil {
			count += 1
		}
		// CreateEnrichmentRecord
		if r.enrichmentActivities != nil {
			count += 1
		}
		// LinkConversation, BackfillConversationSummaries, RegenerateConversationSummary, CheckStaleConversations
		if r.conversationActivities != nil {
			count += 4
		}
		// ConsolidateEntries
		if r.consolidationActivities != nil {
			count += 1
		}
		// HeartbeatCheckReviewQueue, HeartbeatCheckWatchMatches, HeartbeatCheckStaleContent, HeartbeatUpdateStatus
		if r.heartbeatActivities != nil {
			count += 4
		}
		// CreateLangfuseTrace, ReportLangfusePhase, FinishLangfuseTrace, PersistLangfuseTraceID, UpdateLangfuseTraceTags, UpdateLangfuseTraceMetadata
		if r.langfuseActivities != nil {
			count += 6
		}
		// InstructionEvaluate
		if r.instructionEvaluationActivities != nil {
			count += 1
		}
		// GatherDigestData, GenerateDigestNarrative, SaveDigest,
		// GatherWeeklyDigestData, GenerateWeeklyNarrative, UpdateThemeContexts,
		// GatherJournalData, GenerateJournalNarrative
		if r.digestActivities != nil {
			count += 8
		}
		// NewsletterExtract, PersistExtractedData
		if r.newsletterExtractActivities != nil {
			count += 2
		}
		// CheckGraphAuth, FetchOutlookMessages, ProcessOutlookMessage, UpdateOutlookSyncState, RollbackOutlookSync,
		// FetchTeamChannels, FetchChannelMessages, ProcessTeamsThread, UpdateTeamsSyncState, RollbackTeamsSync,
		// FetchMeetingTranscripts, ProcessTranscriptContent, UpdateTranscriptSyncState, RollbackTranscriptSync
		if r.graphActivities != nil {
			count += 14
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
		// Graph activities (Outlook + Teams + Transcript sync workflows run on email queue)
		if r.graphActivities != nil {
			count += 14
		}
		return count
	default:
		return 0
	}
}

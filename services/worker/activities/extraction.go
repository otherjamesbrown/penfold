// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"google.golang.org/grpc/metadata"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/pkg/enrichment/entities"
	perrors "github.com/otherjamesbrown/penfold/pkg/errors"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/tracing"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// ModelInfoProvider resolves model metadata from the database.
// Implemented by registry.PostgresRepository / registry.DBRegistry.
type ModelInfoProvider interface {
	GetModelContextWindowByName(ctx context.Context, modelName string) (int, error)
}

// ExtractionActivities holds dependencies for extraction-related activities.
type ExtractionActivities struct {
	logger         logging.Logger
	aiClient       AIClient
	assertionRepo  AssertionRepository
	entityRepo     EntityRepository
	pipelineRepo   PipelineRepository
	personLookup   PersonLookup
	modelInfo      ModelInfoProvider
}

// NewExtractionActivities creates a new ExtractionActivities instance.
func NewExtractionActivities(
	logger logging.Logger,
	aiClient AIClient,
	assertionRepo AssertionRepository,
	entityRepo EntityRepository,
	pipelineRepo PipelineRepository,
) *ExtractionActivities {
	if logger == nil {
		panic("NewExtractionActivities: logger is required")
	}
	if aiClient == nil {
		panic("NewExtractionActivities: aiClient is required")
	}
	if assertionRepo == nil {
		panic("NewExtractionActivities: assertionRepo is required")
	}
	if entityRepo == nil {
		panic("NewExtractionActivities: entityRepo is required")
	}
	// pipelineRepo is optional (provenance recording)
	return &ExtractionActivities{
		logger:        logger.With(logging.F("component", "extraction_activities")),
		aiClient:      aiClient,
		assertionRepo: assertionRepo,
		entityRepo:    entityRepo,
		pipelineRepo:  pipelineRepo,
	}
}

// WithPersonLookup sets the optional PersonLookup dependency for NER header enrichment (pf-2059f5).
func (a *ExtractionActivities) WithPersonLookup(pl PersonLookup) {
	a.personLookup = pl
}

// WithModelInfo sets the optional ModelInfoProvider for DB-backed context window resolution (pf-b158ef).
func (a *ExtractionActivities) WithModelInfo(mp ModelInfoProvider) {
	a.modelInfo = mp
}

// ExtractAssertions extracts assertions from the given content using an LLM.
// Assertions are subject-predicate-object triples representing facts and claims.
func (a *ExtractionActivities) ExtractAssertions(ctx context.Context, input workflows.ExtractAssertionsInput) (int, error) {
	// Set trace_id in context for log correlation
	if input.ContentID != "" {
		ctx = context.WithValue(ctx, logging.TraceIDKey, input.ContentID)
	}
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "ExtractAssertions"),
		logging.F("tenant_id", input.TenantID),
		logging.F("source_id", input.SourceID),
		logging.F("job_id", input.JobID),
		logging.F("content_length", len(input.Content)),
	)

	// Record initial heartbeat (safe - checks if in activity context)
	recordHeartbeat(ctx, "starting assertion extraction")

	logger.Info("Extracting assertions from content")

	// Check for cancellation
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}

	// Skip assertion extraction for meeting transcripts — the extraction prompt is designed
	// for email format and returns 0 assertions for conversational transcript structure.
	// Meeting assertions are created by Stage 4.5 (PersistFindings) from the analysis output,
	// which produces higher-quality verified actions, decisions, and risks.
	if strings.EqualFold(input.ContentType, "meeting") {
		logger.Info("Skipping assertion extraction for meeting transcript (handled by Stage 4.5 PersistFindings)")
		return 0, nil
	}

	// Validate input
	if input.Content == "" {
		return 0, temporal.NewApplicationError(
			"content is empty",
			"ValidationError",
		)
	}

	// Check if AI client is available
	if a.aiClient == nil {
		logger.Warn("AI client not configured")
		return 0, temporal.NewApplicationErrorWithCause(
			"AI client not configured",
			"ConfigurationError",
			nil,
		)
	}

	// Call AI service to extract assertions
	startTime := time.Now()
	recordHeartbeat(ctx, "calling AI service for assertion extraction")

	// Create stage span wrapping the gRPC call
	stageCtx, stageSpan := tracing.StartStageSpan(ctx, "stage.extract_assertions", tracing.StageSpanOptions{
		ContentID: input.ContentID,
		TenantID:  input.TenantID,
	})
	defer stageSpan.End()

	// Default parameters for assertion extraction
	minConfidence := float32(0.5)
	maxAssertions := int32(20)

	// Assemble content for assertion extraction. Order matters for LLM reading:
	// 1. BackgroundContext (glossary/topics) — grounds the model before it reads the email
	// 2. Subject line — topic framing (pf-e219c1)
	// 3. New email body — the primary content to extract assertions from
	// 4. QuotedContent (parent email) — for resolving forward-references like "#2" (pf-90b749)
	content := input.Content
	if input.Subject != "" {
		content = "Subject: " + input.Subject + "\n\n" + content
	}
	if input.BackgroundContext != "" {
		content = input.BackgroundContext + "\n\n" + content
	}
	if input.QuotedContent != "" {
		quoted := truncateContent(input.QuotedContent, 3000)
		content = content + "\n\n--- Parent message (for reference resolution) ---\n" + quoted
	}

	assertionReq := &aiv1.AssertionRequest{
		Content:       content,
		MinConfidence: &minConfidence,
		MaxAssertions: &maxAssertions,
		TenantId:      &input.TenantID,
	}
	if input.ContentID != "" {
		assertionReq.ContentId = &input.ContentID
	}
	// Pipeline span context is injected above; gRPC OTel interceptors propagate traceparent automatically.

	// Attach Langfuse tracing metadata for AI coordinator to use when creating generation spans.
	assertionCallCtx := stageCtx
	if input.LangfuseTraceID != "" {
		md := metadata.Pairs(
			"x-langfuse-trace-id", input.LangfuseTraceID,
			"x-langfuse-phase-id", input.LangfusePhaseID,
		)
		assertionCallCtx = metadata.NewOutgoingContext(stageCtx, md)
	}

	// Call AI service with stage span context
	resp, err := a.aiClient.ExtractAssertions(assertionCallCtx, assertionReq)
	if err != nil {
		pe := perrors.ClassifyError(err, "extract_assertions")
		logger.Error("Failed to extract assertions from AI service", logging.Err(pe))
		return 0, WrapForTemporal(pe)
	}

	// Record heartbeat after AI call
	recordHeartbeat(ctx, "assertions extracted, processing results")

	logger.Info("Assertions extracted successfully",
		logging.F("ai_duration", time.Since(startTime)),
		logging.F("assertions_found", len(resp.Assertions)),
		logging.F("total_found", resp.TotalFound),
		logging.F("filtered_count", resp.FilteredCount),
		logging.F("model", resp.ModelUsed),
	)

	// If no assertions found, record pipeline run and return early
	if len(resp.Assertions) == 0 {
		logger.Info("No assertions found in content")
		if a.pipelineRepo != nil {
			durationMS := int(time.Since(startTime).Milliseconds())
			inputJSON, _ := json.Marshal(map[string]interface{}{
				"content_length": len(input.Content),
				"tenant_id":      input.TenantID,
			})
			outputJSON, _ := json.Marshal(map[string]interface{}{
				"assertions_found": 0,
				"total_found":      resp.TotalFound,
				"filtered_count":   resp.FilteredCount,
				"model_used":       resp.ModelUsed,
			})
			runErr := a.pipelineRepo.CreateRun(ctx, PipelineRunInput{
				SourceID:        input.SourceID,
				Stage:           "extract_assertions",
				ModelID:         resp.ModelUsed,
				PromptVersion:   int(resp.GetPromptVersion()),
				Status:          "completed",
				DurationMS:      durationMS,
				InputData:       inputJSON,
				OutputData:      outputJSON,
				InputTokens:     int(resp.GetInputTokens()),
				OutputTokens:    int(resp.GetOutputTokens()),
				LangfuseTraceID: input.LangfuseTraceID,
			})
			if runErr != nil {
				logger.Warn("Failed to record pipeline run for extract_assertions", logging.Err(runErr))
			}
		}
		return 0, nil
	}

	// Check if repository is available for storage
	if a.assertionRepo == nil {
		logger.Warn("Assertion repository not configured, skipping storage")
		return len(resp.Assertions), nil
	}

	// Convert proto assertions to domain model
	assertions := make([]*Assertion, len(resp.Assertions))
	for i, pa := range resp.Assertions {
		assertion := &Assertion{
			Subject:    pa.Subject,
			Predicate:  pa.Predicate,
			Object:     pa.Object,
			Confidence: pa.Confidence,
			SourceText: pa.GetSourceText(),
			Category:   pa.GetCategory(),
		}

		// Add sender as "owner" attribution if present
		if input.SenderEmail != "" {
			assertion.Attributions = append(assertion.Attributions, Attribution{
				EntityID: input.SenderEmail,
				Role:     "owner",
			})
		}

		assertions[i] = assertion
	}

	// Store the assertions
	storeStart := time.Now()
	count, err := a.assertionRepo.StoreAssertions(ctx, input.TenantID, input.SourceID, assertions, resp.ModelUsed)
	if err != nil {
		pe := perrors.ClassifyError(err, "extract_assertions")
		logger.Error("Failed to store assertions", logging.Err(pe))
		return 0, WrapForTemporal(pe)
	}

	logger.Info("Assertions stored successfully",
		logging.F("store_duration", time.Since(storeStart)),
		logging.F("stored_count", count),
	)

	// Record pipeline run for provenance tracking
	if a.pipelineRepo != nil {
		durationMS := int(time.Since(startTime).Milliseconds())

		inputJSON, _ := json.Marshal(map[string]interface{}{
			"content_length": len(input.Content),
			"tenant_id":      input.TenantID,
		})
		outputJSON, _ := json.Marshal(map[string]interface{}{
			"assertions_found": len(resp.Assertions),
			"total_found":      resp.TotalFound,
			"filtered_count":   resp.FilteredCount,
			"model_used":       resp.ModelUsed,
		})

		runErr := a.pipelineRepo.CreateRun(ctx, PipelineRunInput{
			SourceID:        input.SourceID,
			Stage:           "extract_assertions",
			ModelID:         resp.ModelUsed,
			PromptVersion:   int(resp.GetPromptVersion()),
			Status:          "completed",
			DurationMS:      durationMS,
			InputData:       inputJSON,
			OutputData:      outputJSON,
			InputTokens:     int(resp.GetInputTokens()),
			OutputTokens:    int(resp.GetOutputTokens()),
			LangfuseTraceID: input.LangfuseTraceID,
		})
		if runErr != nil {
			logger.Warn("Failed to record pipeline run for extract_assertions", logging.Err(runErr))
		}
	}

	return count, nil
}

// ExtractedEntity represents an entity extracted from content.
// DEPRECATED: Use the structured fields in ExtractEntitiesOutput instead.
type ExtractedEntity struct {
	Name       string  `json:"name"`
	Type       string  `json:"type"`
	Confidence float32 `json:"confidence"`
}

// Type aliases for backward compatibility with code not yet migrated to workflows package.
type (
	DeepAnalyzeInput                 = workflows.DeepAnalyzeInput
	DeepAnalyzeOutput                = workflows.DeepAnalyzeOutput
	ExtractEntitiesInput             = workflows.SLMPipelineExtractEntitiesInput
	ExtractEntitiesOutput            = workflows.SLMPipelineExtractEntitiesOutput
	PersonResult                     = workflows.PersonResult
	DateResult                       = workflows.DateResult
	ActionItemResult                 = workflows.ActionItemResult
	DetailedRisk                     = workflows.DetailedRisk
	VerifiedActionOutput             = workflows.VerifiedActionOutput
	VerifiedDecisionOutput           = workflows.VerifiedDecisionOutput
	RiskReferenceOutput              = workflows.RiskReferenceOutput
	ImplicitActionOutput             = workflows.ImplicitActionOutput
	SentimentOutput                  = workflows.SentimentOutput
	TopicMappingOutput               = workflows.TopicMappingOutput
	ParseEmailInput                  = workflows.ParseEmailInput
	ParseEmailOutput                 = workflows.ParseEmailOutput
	ParseTranscriptInput             = workflows.ParseTranscriptInput
	ParseTranscriptOutput            = workflows.ParseTranscriptOutput
	TriageInput                      = workflows.TriageInput
	TriageOutput                     = workflows.TriageOutput
	BuildContextInput                = workflows.BuildContextInput
	BuildContextOutput               = workflows.BuildContextOutput
	ResolvedPerson                   = workflows.ResolvedPerson
	ResolvedProject                  = workflows.ResolvedProject
	PersistFindingsActivityInput     = workflows.PersistFindingsInput
	PersistFindingsActivityOutput    = workflows.PersistFindingsOutput
)

// fallbackContextWindows is a fallback map of model context window sizes (in tokens).
// Used only when the DB lookup via ModelInfoProvider is unavailable.
var fallbackContextWindows = map[string]int{
	"gemini-2.5-flash": 1048576,
	"gemini-2.5-pro":   1048576,
	"gemini-2.0-flash": 1048576,
	"qwen2.5:7b":       32768,
	"qwen3:8b":         32768,
}

// defaultExtractModel is the default model used for extraction stages
// when no ModelOverride is specified.
const defaultExtractModel = "gemini-2.5-flash"

// maxInputChars returns the maximum input size in characters for a given model.
// Uses 80% of context window (tokens * ~4 chars/token) to leave room for prompt + output.
// Tries DB-backed ModelInfoProvider first, falls back to hardcoded map.
// Returns a conservative 6000-char default for unknown models.
func maxInputChars(ctx context.Context, modelInfo ModelInfoProvider, model string) int {
	// Try DB first via ModelInfoProvider (pf-b158ef)
	if modelInfo != nil {
		ctxWindow, err := modelInfo.GetModelContextWindowByName(ctx, model)
		if err == nil && ctxWindow > 0 {
			return (ctxWindow * 4) * 80 / 100
		}
	}
	// Fallback to hardcoded map
	ctxWindow, ok := fallbackContextWindows[model]
	if !ok {
		return 6000
	}
	return (ctxWindow * 4) * 80 / 100
}

// extractChunkSize returns model-appropriate chunk size for chunked extraction.
// Caps at 50K chars to keep individual LLM calls manageable.
func extractChunkSize(ctx context.Context, modelInfo ModelInfoProvider, model string) int {
	threshold := maxInputChars(ctx, modelInfo, model)
	size := threshold / 2
	if size > 50000 {
		return 50000
	}
	return size
}

// ExtractEntities performs two-pass entity extraction with chunking support.
// For content within the model's context window, makes a single RPC call.
// For content exceeding the model's capacity, splits into chunks, calls RPC for each, and merges results.
func (a *ExtractionActivities) ExtractEntities(ctx context.Context, input workflows.SLMPipelineExtractEntitiesInput) (*workflows.SLMPipelineExtractEntitiesOutput, error) {
	// Set trace_id in context for log correlation
	if input.ContentID != "" {
		ctx = context.WithValue(ctx, logging.TraceIDKey, input.ContentID)
	}
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "ExtractEntities"),
		logging.F("tenant_id", input.TenantID),
		logging.F("source_id", input.SourceID),
		logging.F("job_id", input.JobID),
		logging.F("content_length", len(input.Content)),
	)

	// Record initial heartbeat (safe - checks if in activity context)
	recordHeartbeat(ctx, "starting entity extraction")

	logger.Info("Extracting entities from content")

	// Check for cancellation
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Handle metadata-only content (pf-479452)
	// For calendar invites with empty body, return empty extraction result
	// Metadata extraction (organizer, attendees) happens in Stage 3 via CalendarExtractor
	if input.Content == "" {
		logger.Info("Empty content detected, returning empty extraction result")
		return &workflows.SLMPipelineExtractEntitiesOutput{
			People:               []workflows.PersonResult{},
			Dates:                []workflows.DateResult{},
			Projects:             []string{},
			Organisations:        []string{},
			ActionItems:          []workflows.ActionItemResult{},
			Decisions:            []string{},
			Risks:                []string{},
			DetailedRisks:        []workflows.DetailedRisk{},
			QualityGateTriggered: false,
			ModelUsed:            "metadata-only",
		}, nil
	}

	// Background context (glossary + topics) is injected by the AI coordinator's
	// buildSemanticPrompt, not here. The worker sends content and BackgroundContext
	// as separate proto fields — the AI coordinator handles the merge (pf-73ac20).
	content := input.Content

	// Prepend email headers to content for NER prompt enrichment (pf-de2b09).
	// The NER prompt instructs the model to use full names from email headers,
	// but previously only body text was sent. This injects structured header
	// metadata so the model can see From/To/CC/Subject.
	if input.ContentType == "email" {
		// Enrich participants with person DB data (pf-2059f5)
		if a.personLookup != nil {
			enrichedSenderName, enrichedParticipants := a.enrichHeaderParticipants(
				ctx, input.TenantID, input.SenderEmail, input.SenderName, input.Participants, logger,
			)
			input.SenderName = enrichedSenderName
			input.Participants = enrichedParticipants
		}
		if headerBlock := buildEmailHeaderBlock(input); headerBlock != "" {
			content = headerBlock + content
			logger.Info("Prepended email headers to extraction content",
				logging.F("header_length", len(headerBlock)),
				logging.F("total_length", len(content)),
			)
		}
	}

	// Check if AI client is available
	if a.aiClient == nil {
		logger.Warn("AI client not configured")
		return nil, temporal.NewApplicationErrorWithCause(
			"AI client not configured",
			"ConfigurationError",
			nil,
		)
	}

	startTime := time.Now()
	contentRunes := []rune(content)

	// Determine extraction model for chunking threshold (pf-6992f6).
	// Uses ModelOverride if set, otherwise defaults to gemini-2.5-flash.
	extractModel := defaultExtractModel
	if input.ModelOverride != "" {
		extractModel = input.ModelOverride
	}
	chunkThreshold := maxInputChars(ctx, a.modelInfo, extractModel)

	// pf-edbda4: Record failed pipeline runs for provenance.
	// When extraction fails, record the failure so `penf pipeline inspect` shows
	// the attempt rather than appearing as if the stage was skipped entirely.
	// pf-04a2de: Use a detached context (context.Background) so the write succeeds
	// even after Temporal cancels the activity context due to HeartbeatTimeout.
	var extractionFailed bool
	var failureError string
	defer func() {
		if extractionFailed && a.pipelineRepo != nil {
			writeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			durationMS := int(time.Since(startTime).Milliseconds())
			for _, stage := range []string{"extract_ner", "extract_semantic"} {
				if err := a.pipelineRepo.CreateRun(writeCtx, PipelineRunInput{
					SourceID:        input.SourceID,
					Stage:           stage,
					Status:          "failed",
					DurationMS:      durationMS,
					SkipReason:      failureError,
					LangfuseTraceID: input.LangfuseTraceID,
				}); err != nil {
					logger.Warn("Failed to record failed pipeline run",
						logging.F("stage", stage),
						logging.Err(err),
					)
				}
			}
		}
	}()

	// Create stage span wrapping ALL gRPC calls (single or chunked)
	stageCtx, stageSpan := tracing.StartStageSpan(ctx, "stage.extract_entities", tracing.StageSpanOptions{
		ContentID: input.ContentID,
		TenantID:  input.TenantID,
	})
	defer stageSpan.End()

	// Attach Langfuse tracing metadata for AI coordinator to use when creating generation spans.
	entityCallCtx := stageCtx
	if input.LangfuseTraceID != "" {
		md := metadata.Pairs(
			"x-langfuse-trace-id", input.LangfuseTraceID,
			"x-langfuse-phase-id", input.LangfusePhaseID,
		)
		entityCallCtx = metadata.NewOutgoingContext(stageCtx, md)
	}

	var results []*aiv1.ExtractEntitiesResponse

	if len(contentRunes) <= chunkThreshold {
		// Single call — content fits within model context window
		recordHeartbeat(entityCallCtx, "calling AI service for entity extraction (single call)")
		logger.Info("Content within model context window, making single RPC call",
			logging.F("model", extractModel),
			logging.F("threshold_chars", chunkThreshold),
		)

		req := &aiv1.ExtractEntitiesRequest{
			Content:           content,
			TenantId:          optString(input.TenantID),
			BackgroundContext: input.BackgroundContext,
			Stages:            input.PipelineStages,
		}
		if input.TriageCategory != "" {
			req.TriageCategory = optString(input.TriageCategory)
		}
		if input.ContentID != "" {
			req.ContentId = optString(input.ContentID)
		}
		if input.NERPromptOverride > 0 {
			req.NerPromptVersion = &input.NERPromptOverride
		}
		if input.SemanticPromptOverride > 0 {
			req.SemanticPromptVersion = &input.SemanticPromptOverride
		}
		// Pipeline span context is injected above; gRPC OTel interceptors propagate traceparent automatically.

		// Heartbeat goroutine keeps activity alive during long API calls (pf-04a2de).
		// Without this, single-call extraction of large content (e.g. 50K-char meeting transcripts)
		// can exceed HeartbeatTimeout and get cancelled by Temporal.
		heartbeatDone := make(chan struct{})
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					recordHeartbeat(ctx, "waiting for AI service response (single call)")
				case <-heartbeatDone:
					return
				}
			}
		}()

		// Call AI service with stage span context (and Langfuse metadata if set)
		resp, err := a.aiClient.ExtractEntities(entityCallCtx, req)
		close(heartbeatDone)
		if err != nil {
			pe := perrors.ClassifyError(err, "extract_ner")
			logger.Error("Failed to extract entities from AI service", logging.Err(pe))
			extractionFailed = true
			failureError = pe.Error()
			return nil, WrapForTemporal(pe)
		}
		results = append(results, resp)
	} else {
		// Chunked extraction — content exceeds model context window
		recordHeartbeat(entityCallCtx, "calling AI service for entity extraction (chunked)")
		chunkSize := extractChunkSize(ctx, a.modelInfo, extractModel)
		logger.Info("Content exceeds model context window, splitting into chunks",
			logging.F("content_runes", len(contentRunes)),
			logging.F("model", extractModel),
			logging.F("threshold_chars", chunkThreshold),
			logging.F("chunk_size", chunkSize),
		)

		chunks := splitIntoChunks(content, chunkSize, chunkSize/10)
		logger.Info("Content split into chunks", logging.F("chunk_count", len(chunks)))

		for i, chunk := range chunks {
			// Check for cancellation between chunks (e.g. Temporal timeout)
			if entityCallCtx.Err() != nil {
				extractionFailed = true
				failureError = fmt.Sprintf("context cancelled during chunk %d/%d: %v", i+1, len(chunks), entityCallCtx.Err())
				return nil, entityCallCtx.Err()
			}

			recordHeartbeat(entityCallCtx, fmt.Sprintf("extracting entities from chunk %d/%d", i+1, len(chunks)))

			req := &aiv1.ExtractEntitiesRequest{
				Content:           chunk.Content,
				TenantId:          optString(input.TenantID),
				BackgroundContext: input.BackgroundContext,
				Stages:            input.PipelineStages,
			}
			// Only pass triage_category on first chunk for quality gate
			if i == 0 && input.TriageCategory != "" {
				req.TriageCategory = optString(input.TriageCategory)
			}
			if input.ContentID != "" {
				req.ContentId = optString(input.ContentID)
			}
			if input.NERPromptOverride > 0 {
				req.NerPromptVersion = &input.NERPromptOverride
			}
			if input.SemanticPromptOverride > 0 {
				req.SemanticPromptVersion = &input.SemanticPromptOverride
			}
			// Pipeline span context is injected above; gRPC OTel interceptors propagate traceparent automatically.

			// Heartbeat goroutine keeps activity alive during long per-chunk API calls (pf-04a2de).
			heartbeatDone := make(chan struct{})
			go func() {
				ticker := time.NewTicker(30 * time.Second)
				defer ticker.Stop()
				for {
					select {
					case <-ticker.C:
						recordHeartbeat(ctx, fmt.Sprintf("waiting for AI service response (chunk %d/%d)", i+1, len(chunks)))
					case <-heartbeatDone:
						return
					}
				}
			}()

			resp, err := a.aiClient.ExtractEntities(entityCallCtx, req)
			close(heartbeatDone)
			if err != nil {
				pe := perrors.ClassifyError(err, "extract_ner")
				logger.Error("Failed to extract entities from chunk",
					logging.Err(pe),
					logging.F("chunk_index", i),
					logging.F("chunk_length", len(chunk.Content)),
				)
				extractionFailed = true
				failureError = pe.Error()
				return nil, WrapForTemporal(pe)
			}
			results = append(results, resp)
		}
	}

	// Record heartbeat after AI calls
	recordHeartbeat(ctx, "merging extraction results")

	// Merge results from all chunks
	output := mergeExtractionResults(results)

	logger.Info("Entities extracted successfully",
		logging.F("ai_duration", time.Since(startTime)),
		logging.F("people", len(output.People)),
		logging.F("dates", len(output.Dates)),
		logging.F("projects", len(output.Projects)),
		logging.F("organisations", len(output.Organisations)),
		logging.F("action_items", len(output.ActionItems)),
		logging.F("decisions", len(output.Decisions)),
		logging.F("risks", len(output.Risks)),
		logging.F("quality_gate_triggered", output.QualityGateTriggered),
		logging.F("model", output.ModelUsed),
	)

	// Record pipeline run for provenance tracking
	// Note: This covers both extract_ner and extract_semantic stages in one call
	// The actual pipeline has separate stages, but they run together in this activity
	if a.pipelineRepo != nil {
		durationMS := int(time.Since(startTime).Milliseconds())

		// Accumulate token counts across all chunk responses
		// Capture prompt_version from the first result (all chunks use the same prompt template)
		var totalInputTokens, totalOutputTokens int
		var extractPromptVersion int32
		var semanticPromptVersion int32
		for i, r := range results {
			totalInputTokens += int(r.GetInputTokens())
			totalOutputTokens += int(r.GetOutputTokens())
			if i == 0 {
				extractPromptVersion = r.GetPromptVersion()
				semanticPromptVersion = r.GetSemanticPromptVersion()
			}
		}

		// Capture IO data
		inputJSON, _ := json.Marshal(map[string]interface{}{
			"content_length":  len(content),
			"triage_category": input.TriageCategory,
			"tenant_id":       input.TenantID,
			"has_headers":     input.ContentType == "email" && input.SenderEmail != "",
		})
		outputJSON, _ := json.Marshal(map[string]interface{}{
			"response_count": len(results),
			"model_used":     output.ModelUsed,
		})
		// Split parsed data: NER gets entities, semantic gets higher-level extractions
		nerParsed, _ := json.Marshal(map[string]interface{}{
			"people":        output.People,
			"dates":         output.Dates,
			"projects":      output.Projects,
			"organisations": output.Organisations,
			"model_used":    output.ModelUsed,
		})
		semParsed, _ := json.Marshal(map[string]interface{}{
			"action_items":          output.ActionItems,
			"decisions":             output.Decisions,
			"risks":                 output.Risks,
			"detailed_risks":        output.DetailedRisks,
			"quality_gate_triggered": output.QualityGateTriggered,
			"model_used":            output.ModelUsed,
		})

		// Record extract_ner stage only if it's in the pipeline definition.
		// Nil/empty PipelineStages means no pipeline definition was loaded — record all stages (backward compat).
		if stageInPipelineList(input.PipelineStages, "extract_ner") {
			runErr := a.pipelineRepo.CreateRun(ctx, PipelineRunInput{
				SourceID:        input.SourceID,
				Stage:           "extract_ner",
				ModelID:         output.ModelUsed,
				PromptVersion:   int(extractPromptVersion),
				Status:          "completed",
				DurationMS:      durationMS,
				InputData:       inputJSON,
				OutputData:      outputJSON,
				ParsedData:      nerParsed,
				InputTokens:     totalInputTokens,
				OutputTokens:    totalOutputTokens,
				LangfuseTraceID: input.LangfuseTraceID,
			})
			if runErr != nil {
				logger.Warn("Failed to record pipeline run for extract_ner", logging.Err(runErr))
			}
		}
		// Record extract_semantic stage only if it's in the pipeline definition.
		// Lightweight pipelines (notification, attendees_only) don't include extract_semantic.
		if stageInPipelineList(input.PipelineStages, "extract_semantic") {
			runErr := a.pipelineRepo.CreateRun(ctx, PipelineRunInput{
				SourceID:        input.SourceID,
				Stage:           "extract_semantic",
				ModelID:         output.ModelUsed,
				PromptVersion:   int(semanticPromptVersion),
				Status:          "completed",
				DurationMS:      durationMS,
				InputData:       inputJSON,
				OutputData:      outputJSON,
				ParsedData:      semParsed,
				InputTokens:     0,
				OutputTokens:    0,
				LangfuseTraceID: input.LangfuseTraceID,
			})
			if runErr != nil {
				logger.Warn("Failed to record pipeline run for extract_semantic", logging.Err(runErr))
			}
		}
	}

	return output, nil
}

// stageInPipelineList reports whether stage is present in the pipeline stages list.
// Returns true when stages is nil or empty (no pipeline definition loaded), preserving
// backward compatibility: existing callers that don't pass PipelineStages record all stages.
func stageInPipelineList(stages []string, stage string) bool {
	if len(stages) == 0 {
		return true
	}
	for _, s := range stages {
		if s == stage {
			return true
		}
	}
	return false
}

// mergeExtractionResults merges extraction results from multiple chunks.
// Deduplicates entities using case-insensitive matching where appropriate.
func mergeExtractionResults(results []*aiv1.ExtractEntitiesResponse) *workflows.SLMPipelineExtractEntitiesOutput {
	if len(results) == 0 {
		return &workflows.SLMPipelineExtractEntitiesOutput{
			People:               []workflows.PersonResult{},
			Dates:                []workflows.DateResult{},
			Projects:             []string{},
			Organisations:        []string{},
			ActionItems:          []workflows.ActionItemResult{},
			Decisions:            []string{},
			Risks:                []string{},
			DetailedRisks:        []workflows.DetailedRisk{},
			QualityGateTriggered: false,
			ModelUsed:            "",
		}
	}

	// Use first result's model and quality gate status
	modelUsed := results[0].ModelUsed
	qualityGateTriggered := false

	// People: deduplicate by canonical name.
	// NormalizeDisplayName handles "Last, First" → "First Last" + whitespace/quote cleanup.
	// Lowercasing ensures different representations produce the same dedup key.
	// isGarbageName filters single-character NER artifacts before they enter the map.
	peopleMap := make(map[string]workflows.PersonResult)
	for _, result := range results {
		if result.QualityGateTriggered {
			qualityGateTriggered = true
		}
		for _, p := range result.People {
			if isGarbageName(p.Name) {
				continue
			}
			key := strings.ToLower(entities.NormalizeDisplayName(p.Name))
			if existing, ok := peopleMap[key]; !ok || len(p.Role) > len(existing.Role) {
				// Keep the one with more role info
				peopleMap[key] = workflows.PersonResult{Name: p.Name, Role: p.Role, Source: p.Source}
			}
		}
	}

	// Subset/initial matching: merge "F Last" into "FirstName Last" and
	// single-word "First" into "First Last" when an unambiguous match exists.
	//
	// Pass 1: for each key of the form "X word" where X is a single character,
	// find a key "word1 word2" where word1 starts with X and word2 == word.
	// Pass 2: for each single-word key, find a multi-word key that starts with it.
	for key := range peopleMap {
		parts := strings.Fields(key)
		switch {
		case len(parts) == 2 && len(parts[0]) == 1:
			// "f pandya" — look for "* pandya" where * starts with "f"
			initial := parts[0]
			lastName := parts[1]
			var matchKey string
			for candidate := range peopleMap {
				if candidate == key {
					continue
				}
				cParts := strings.Fields(candidate)
				if len(cParts) >= 2 && strings.HasPrefix(cParts[0], initial) && cParts[len(cParts)-1] == lastName {
					if matchKey == "" {
						matchKey = candidate
					} else {
						matchKey = "" // ambiguous — two candidates, skip
						break
					}
				}
			}
			if matchKey != "" {
				existing := peopleMap[matchKey]
				entry := peopleMap[key]
				if len(entry.Role) > len(existing.Role) {
					existing.Role = entry.Role
				}
				peopleMap[matchKey] = existing
				delete(peopleMap, key)
			}

		case len(parts) == 1:
			// "parimal" — look for "parimal *" (any multi-word key starting with this)
			word := parts[0]
			var matchKey string
			for candidate := range peopleMap {
				if candidate == key {
					continue
				}
				cParts := strings.Fields(candidate)
				if len(cParts) >= 2 && cParts[0] == word {
					if matchKey == "" {
						matchKey = candidate
					} else {
						matchKey = "" // ambiguous
						break
					}
				}
			}
			if matchKey != "" {
				existing := peopleMap[matchKey]
				entry := peopleMap[key]
				if len(entry.Role) > len(existing.Role) {
					existing.Role = entry.Role
				}
				peopleMap[matchKey] = existing
				delete(peopleMap, key)
			}
		}
	}

	people := make([]workflows.PersonResult, 0, len(peopleMap))
	for _, p := range peopleMap {
		people = append(people, p)
	}

	// Dates: deduplicate by date string (case-insensitive)
	datesMap := make(map[string]workflows.DateResult)
	for _, result := range results {
		for _, d := range result.Dates {
			key := normalizeString(d.Date)
			if existing, ok := datesMap[key]; !ok || len(d.Context) > len(existing.Context) {
				// Keep the one with more context info
				datesMap[key] = workflows.DateResult{Date: d.Date, Context: d.Context}
			}
		}
	}
	dates := make([]workflows.DateResult, 0, len(datesMap))
	for _, d := range datesMap {
		dates = append(dates, d)
	}

	// Projects: deduplicate by case-insensitive match
	projectsSet := make(map[string]string)
	for _, result := range results {
		for _, proj := range result.Projects {
			key := normalizeString(proj)
			if _, ok := projectsSet[key]; !ok {
				projectsSet[key] = proj
			}
		}
	}
	projects := make([]string, 0, len(projectsSet))
	for _, proj := range projectsSet {
		projects = append(projects, proj)
	}

	// Organisations: deduplicate by case-insensitive match
	orgsSet := make(map[string]string)
	for _, result := range results {
		for _, org := range result.Organisations {
			key := normalizeString(org)
			if _, ok := orgsSet[key]; !ok {
				orgsSet[key] = org
			}
		}
	}
	organisations := make([]string, 0, len(orgsSet))
	for _, org := range orgsSet {
		organisations = append(organisations, org)
	}

	// ActionItems: deduplicate by composite key (action + assignee, case-insensitive)
	actionItemsMap := make(map[string]workflows.ActionItemResult)
	for _, result := range results {
		for _, ai := range result.ActionItems {
			// Use composite key: action + assignee
			key := normalizeString(ai.Action) + "|" + normalizeString(ai.Assignee)
			if existing, ok := actionItemsMap[key]; !ok || len(ai.Due) > len(existing.Due) {
				// Keep the one with more due info
				actionItemsMap[key] = workflows.ActionItemResult{
					Assignee: ai.Assignee,
					Action:   ai.Action,
					Due:      ai.Due,
				}
			}
		}
	}
	actionItems := make([]workflows.ActionItemResult, 0, len(actionItemsMap))
	for _, ai := range actionItemsMap {
		actionItems = append(actionItems, ai)
	}

	// Decisions: deduplicate by exact string (case-insensitive)
	decisionsSet := make(map[string]string)
	for _, result := range results {
		for _, dec := range result.Decisions {
			key := normalizeString(dec)
			if _, ok := decisionsSet[key]; !ok {
				decisionsSet[key] = dec
			}
		}
	}
	decisions := make([]string, 0, len(decisionsSet))
	for _, dec := range decisionsSet {
		decisions = append(decisions, dec)
	}

	// Risks: deduplicate by exact string (case-insensitive)
	risksSet := make(map[string]string)
	for _, result := range results {
		for _, risk := range result.Risks {
			key := normalizeString(risk)
			if _, ok := risksSet[key]; !ok {
				risksSet[key] = risk
			}
		}
	}
	risks := make([]string, 0, len(risksSet))
	for _, risk := range risksSet {
		risks = append(risks, risk)
	}

	// DetailedRisks: deduplicate by description (case-insensitive)
	detailedRisksMap := make(map[string]workflows.DetailedRisk)
	for _, result := range results {
		for _, dr := range result.DetailedRisks {
			key := normalizeString(dr.Description)
			if existing, ok := detailedRisksMap[key]; !ok || len(dr.SeverityHint) > len(existing.SeverityHint) {
				// Keep the one with more detail
				detailedRisksMap[key] = workflows.DetailedRisk{
					Description:  dr.Description,
					SeverityHint: dr.SeverityHint,
					OwnerHint:    dr.OwnerHint,
				}
			}
		}
	}
	detailedRisks := make([]workflows.DetailedRisk, 0, len(detailedRisksMap))
	for _, dr := range detailedRisksMap {
		detailedRisks = append(detailedRisks, dr)
	}

	return &workflows.SLMPipelineExtractEntitiesOutput{
		People:               people,
		Dates:                dates,
		Projects:             projects,
		Organisations:        organisations,
		ActionItems:          actionItems,
		Decisions:            decisions,
		Risks:                risks,
		DetailedRisks:        detailedRisks,
		QualityGateTriggered: qualityGateTriggered,
		ModelUsed:            modelUsed,
	}
}

// enrichHeaderParticipants enriches email participants with canonical names and aliases from the person DB (pf-2059f5).
// For each email address, it looks up the person record and replaces the raw header display name
// with the canonical name. If the person has name-type aliases, it appends them.
func (a *ExtractionActivities) enrichHeaderParticipants(
	ctx context.Context,
	tenantID, senderEmail, senderName string,
	participants []workflows.Participant,
	logger logging.Logger,
) (string, []workflows.Participant) {
	// Collect all unique emails
	emails := make([]string, 0, len(participants)+1)
	if senderEmail != "" {
		emails = append(emails, senderEmail)
	}
	for _, p := range participants {
		if p.Email != "" {
			emails = append(emails, p.Email)
		}
	}
	if len(emails) == 0 {
		return senderName, participants
	}

	// Batch lookup people by email
	people, err := a.personLookup.GetPeopleByEmails(ctx, tenantID, emails)
	if err != nil {
		logger.Warn("Failed to lookup people for header enrichment", logging.Err(err))
		return senderName, participants
	}
	if len(people) == 0 {
		return senderName, participants
	}

	// Collect person IDs for metadata lookup
	var personIDs []int64
	for _, p := range people {
		personIDs = append(personIDs, p.ID)
	}

	// Batch lookup person metadata (pf-2d3ec8)
	personMetadata, err := a.personLookup.GetMetadataByPersonIDs(ctx, personIDs)
	if err != nil {
		logger.Warn("Failed to lookup metadata for header enrichment", logging.Err(err))
		// Continue without metadata — we still have canonical names
		personMetadata = nil
	}

	// Attach metadata to PersonInfo objects
	if personMetadata != nil {
		for _, p := range people {
			if md, ok := personMetadata[p.ID]; ok {
				p.Metadata = md
			}
		}
	}

	// Enrich sender (map keys are lowercase from GetPeopleByEmails)
	enrichedSenderName := senderName
	if person, ok := people[strings.ToLower(senderEmail)]; ok {
		enrichedSenderName = formatEnrichedName(person)
		logger.Info("Enriched sender from person DB",
			logging.F("raw_name", senderName),
			logging.F("canonical_name", person.CanonicalName),
		)
	}

	// Enrich participants (make a copy to avoid mutating input)
	enrichedParticipants := make([]workflows.Participant, len(participants))
	copy(enrichedParticipants, participants)
	for i, p := range enrichedParticipants {
		if person, ok := people[strings.ToLower(p.Email)]; ok {
			enrichedParticipants[i].DisplayName = formatEnrichedName(person)
		}
	}

	logger.Info("Enriched email header participants from person DB",
		logging.F("total_emails", len(emails)),
		logging.F("matched_people", len(people)),
		logging.F("metadata_found", len(personMetadata)),
	)

	return enrichedSenderName, enrichedParticipants
}

// nerMetadataKeys defines which metadata keys to include in NER header enrichment.
// Not all metadata is useful for NER — this whitelist controls what's shown.
var nerMetadataKeys = []string{"reports_to", "notes", "team"}

// formatEnrichedName builds a display string from PersonInfo for use in NER input.
// Alias annotations are intentionally excluded — they caused the NER model to extract
// aliases as separate person entities (pf-0e4d69). Alias data lives in person_aliases
// for downstream entity resolution, not for NER input enrichment.
// Example outputs:
//   "Tim Dunn" (canonical name only)
//   "Tim Dunn [Senior Director, Hardware Engineering]" (with title)
//   "Sara Weisman [MTC Program Solution Lead, reports to James Brown]" (with metadata)
func formatEnrichedName(person *PersonInfo) string {
	name := person.CanonicalName

	// Build bracket annotation: title + whitelisted metadata
	var annotations []string
	if person.Title != "" {
		annotations = append(annotations, person.Title)
	}
	if person.Metadata != nil {
		for _, key := range nerMetadataKeys {
			if val, ok := person.Metadata[key]; ok && val != "" {
				// Format as "key value" with underscores replaced by spaces
				label := strings.ReplaceAll(key, "_", " ")
				annotations = append(annotations, label+" "+val)
			}
		}
	}
	if len(annotations) > 0 {
		name += " [" + strings.Join(annotations, ", ") + "]"
	}

	return name
}

// buildEmailHeaderBlock constructs a structured email header block for NER prompt enrichment (pf-de2b09).
// Returns empty string if no meaningful header data is available.
//
// To/CC participant names are intentionally excluded from the NER prompt input (pf-c9077c).
// Header recipients are already captured deterministically by ExtractHeaderMentions (Stage 1.5).
// Including them here caused the NER model to extract every header recipient as a "person
// mentioned", flooding output with header-only names and obscuring genuine body mentions.
// Only the sender (From) is retained for context, as it is meaningful for disambiguation.
func buildEmailHeaderBlock(input workflows.SLMPipelineExtractEntitiesInput) string {
	var lines []string

	// From line — sender context is useful for NER disambiguation.
	if input.SenderEmail != "" {
		if input.SenderName != "" {
			lines = append(lines, fmt.Sprintf("From: %s <%s>", input.SenderName, input.SenderEmail))
		} else {
			lines = append(lines, fmt.Sprintf("From: %s", input.SenderEmail))
		}
	}

	// To/CC lines are intentionally omitted. Header recipients are captured by
	// ExtractHeaderMentions; emitting them here causes NER to re-extract them as
	// body mentions and dominates extraction output (pf-c9077c).

	// Subject line
	if input.Subject != "" {
		lines = append(lines, fmt.Sprintf("Subject: %s", input.Subject))
	}

	if len(lines) == 0 {
		return ""
	}

	return "EMAIL METADATA:\n" + strings.Join(lines, "\n") + "\n---\nBODY:\n"
}

// normalizeString converts a string to lowercase and trims whitespace for deduplication.
func normalizeString(s string) string {
	return strings.TrimSpace(strings.ToLower(s))
}

// isGarbageName returns true if name is too short to be a real person's name.
// Single-character names ("P", "K") are NER artifacts and should be filtered.
func isGarbageName(name string) bool {
	return len(strings.TrimSpace(name)) < 2
}

// optString returns a pointer to the given string (for proto optional fields).
func optString(s string) *string {
	return &s
}

// inferEntityType attempts to infer the entity type from the name and category.
// DEPRECATED: Used by old assertion-based extraction logic.
func inferEntityType(name, category string) string {
	// Use category as a hint if available
	switch category {
	case "organizational":
		return "organization"
	case "temporal":
		return "date"
	case "location", "geographical":
		return "location"
	case "person", "personnel":
		return "person"
	}

	// Basic heuristics based on name patterns
	// In a real implementation, this would use NER models
	if len(name) == 0 {
		return "unknown"
	}

	// Default to unknown - a proper NER model would classify this
	return "unknown"
}

// recordHeartbeat safely records an activity heartbeat, only if in an activity context.
// This allows the function to be called in tests without panicking.
func recordHeartbeat(ctx context.Context, details ...interface{}) {
	defer func() {
		// Recover from panic if not in activity context
		_ = recover()
	}()
	activity.RecordHeartbeat(ctx, details...)
}

// DeleteAssertions removes all assertions for a source during reprocessing when
// extraction is being skipped (e.g. auto-reply reclassified to contribution=NONE).
// This prevents stale assertions from a prior run from persisting after reclassification.
// It is best-effort: the activity logs a warning but does not fail the workflow.
func (a *ExtractionActivities) DeleteAssertions(ctx context.Context, input workflows.DeleteAssertionsInput) (*workflows.DeleteAssertionsOutput, error) {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "DeleteAssertions"),
		logging.F("tenant_id", input.TenantID),
		logging.F("source_id", input.SourceID),
	)

	recordHeartbeat(ctx, "starting delete assertions")

	logger.Info("Deleting stale assertions for source (reprocess with skipExtract)")

	if input.TenantID == "" || input.SourceID <= 0 {
		logger.Warn("DeleteAssertions called with invalid input, skipping",
			logging.F("tenant_id", input.TenantID),
			logging.F("source_id", input.SourceID),
		)
		return &workflows.DeleteAssertionsOutput{Deleted: 0}, nil
	}

	deleted, err := a.assertionRepo.DeleteAssertions(ctx, input.TenantID, input.SourceID)
	if err != nil {
		logger.Warn("Failed to delete assertions, continuing", logging.Err(err))
		// Best-effort: do not fail the workflow
		return &workflows.DeleteAssertionsOutput{Deleted: 0}, nil
	}

	recordHeartbeat(ctx, "delete assertions complete")

	logger.Info("Deleted stale assertions",
		logging.F("deleted_count", deleted),
	)

	return &workflows.DeleteAssertionsOutput{Deleted: deleted}, nil
}

// Ensure ExtractionActivities implements required interfaces at compile time.
var _ interface {
	ExtractAssertions(ctx context.Context, input workflows.ExtractAssertionsInput) (int, error)
	ExtractEntities(ctx context.Context, input workflows.SLMPipelineExtractEntitiesInput) (*workflows.SLMPipelineExtractEntitiesOutput, error)
	DeleteAssertions(ctx context.Context, input workflows.DeleteAssertionsInput) (*workflows.DeleteAssertionsOutput, error)
} = (*ExtractionActivities)(nil)

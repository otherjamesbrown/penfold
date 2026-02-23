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
	perrors "github.com/otherjamesbrown/penfold/pkg/errors"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/tracing"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// ExtractionActivities holds dependencies for extraction-related activities.
type ExtractionActivities struct {
	logger         logging.Logger
	aiClient       AIClient
	assertionRepo  AssertionRepository
	entityRepo     EntityRepository
	pipelineRepo   PipelineRepository
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

	// Inject pipeline OTel span context so stage spans nest under the pipeline trace.
	if input.PipelineSpanID != "" && input.LangfuseTraceID != "" {
		otelTraceID := strings.ReplaceAll(input.LangfuseTraceID, "-", "")
		ctx = tracing.ContextWithPipelineSpan(ctx, otelTraceID, input.PipelineSpanID)
	}

	// Create stage span wrapping the gRPC call
	stageCtx, stageSpan := tracing.StartStageSpan(ctx, "stage.extract_assertions", tracing.StageSpanOptions{
		ContentID: input.ContentID,
		TenantID:  input.TenantID,
	})
	defer stageSpan.End()

	// Default parameters for assertion extraction
	minConfidence := float32(0.5)
	maxAssertions := int32(20)

	assertionReq := &aiv1.AssertionRequest{
		Content:       input.Content,
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

// ExtractEntities performs two-pass entity extraction with chunking support.
// For content under 6K chars, makes a single RPC call.
// For content over 6K chars, splits into chunks, calls RPC for each, and merges results.
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
	contentRunes := []rune(input.Content)

	// Inject pipeline OTel span context so stage spans nest under the pipeline trace.
	if input.PipelineSpanID != "" && input.LangfuseTraceID != "" {
		otelTraceID := strings.ReplaceAll(input.LangfuseTraceID, "-", "")
		ctx = tracing.ContextWithPipelineSpan(ctx, otelTraceID, input.PipelineSpanID)
	}

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

	if len(contentRunes) <= 6000 {
		// Single call for short content
		recordHeartbeat(entityCallCtx, "calling AI service for entity extraction (single call)")
		logger.Info("Content under 6K chars, making single RPC call")

		req := &aiv1.ExtractEntitiesRequest{
			Content:  input.Content,
			TenantId: optString(input.TenantID),
		}
		if input.TriageCategory != "" {
			req.TriageCategory = optString(input.TriageCategory)
		}
		if input.ContentID != "" {
			req.ContentId = optString(input.ContentID)
		}
		// Pipeline span context is injected above; gRPC OTel interceptors propagate traceparent automatically.

		// Call AI service with stage span context (and Langfuse metadata if set)
		resp, err := a.aiClient.ExtractEntities(entityCallCtx, req)
		if err != nil {
			pe := perrors.ClassifyError(err, "extract_ner")
			logger.Error("Failed to extract entities from AI service", logging.Err(pe))
			return nil, WrapForTemporal(pe)
		}
		results = append(results, resp)
	} else {
		// Chunked extraction for long content
		recordHeartbeat(entityCallCtx, "calling AI service for entity extraction (chunked)")
		logger.Info("Content over 6K chars, splitting into chunks",
			logging.F("content_runes", len(contentRunes)),
		)

		chunks := splitIntoChunks(input.Content, 1500, 200)
		logger.Info("Content split into chunks", logging.F("chunk_count", len(chunks)))

		for i, chunk := range chunks {
			// Check for cancellation between chunks
			if entityCallCtx.Err() != nil {
				return nil, entityCallCtx.Err()
			}

			recordHeartbeat(entityCallCtx, fmt.Sprintf("extracting entities from chunk %d/%d", i+1, len(chunks)))

			req := &aiv1.ExtractEntitiesRequest{
				Content:  chunk.Content,
				TenantId: optString(input.TenantID),
			}
			// Only pass triage_category on first chunk for quality gate
			if i == 0 && input.TriageCategory != "" {
				req.TriageCategory = optString(input.TriageCategory)
			}
			if input.ContentID != "" {
				req.ContentId = optString(input.ContentID)
			}
			// Pipeline span context is injected above; gRPC OTel interceptors propagate traceparent automatically.

			resp, err := a.aiClient.ExtractEntities(entityCallCtx, req)
			if err != nil {
				pe := perrors.ClassifyError(err, "extract_ner")
				logger.Error("Failed to extract entities from chunk",
					logging.Err(pe),
					logging.F("chunk_index", i),
					logging.F("chunk_length", len(chunk.Content)),
				)
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
		for i, r := range results {
			totalInputTokens += int(r.GetInputTokens())
			totalOutputTokens += int(r.GetOutputTokens())
			if i == 0 {
				extractPromptVersion = r.GetPromptVersion()
			}
		}

		// Capture IO data
		inputJSON, _ := json.Marshal(map[string]interface{}{
			"content_length":  len(input.Content),
			"triage_category": input.TriageCategory,
			"tenant_id":       input.TenantID,
		})
		outputJSON, _ := json.Marshal(map[string]interface{}{
			"response_count": len(results),
			"model_used":     output.ModelUsed,
		})
		parsedJSON, _ := json.Marshal(output)

		// Record as extract_ner stage (primary extraction stage; attribute all tokens here)
		runErr := a.pipelineRepo.CreateRun(ctx, PipelineRunInput{
			SourceID:        input.SourceID,
			Stage:           "extract_ner",
			ModelID:         output.ModelUsed,
			PromptVersion:   int(extractPromptVersion),
			Status:          "completed",
			DurationMS:      durationMS,
			InputData:       inputJSON,
			OutputData:      outputJSON,
			ParsedData:      parsedJSON,
			InputTokens:     totalInputTokens,
			OutputTokens:    totalOutputTokens,
			LangfuseTraceID: input.LangfuseTraceID,
		})
		if runErr != nil {
			logger.Warn("Failed to record pipeline run for extract_ner", logging.Err(runErr))
		}
		// Also record extract_semantic since this activity does both
		runErr = a.pipelineRepo.CreateRun(ctx, PipelineRunInput{
			SourceID:        input.SourceID,
			Stage:           "extract_semantic",
			ModelID:         output.ModelUsed,
			PromptVersion:   int(extractPromptVersion),
			Status:          "completed",
			DurationMS:      durationMS,
			InputData:       inputJSON,
			OutputData:      outputJSON,
			ParsedData:      parsedJSON,
			InputTokens:     totalInputTokens,
			OutputTokens:    totalOutputTokens,
			LangfuseTraceID: input.LangfuseTraceID,
		})
		if runErr != nil {
			logger.Warn("Failed to record pipeline run for extract_semantic", logging.Err(runErr))
		}
	}

	return output, nil
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

	// People: deduplicate by case-insensitive name
	peopleMap := make(map[string]workflows.PersonResult)
	for _, result := range results {
		if result.QualityGateTriggered {
			qualityGateTriggered = true
		}
		for _, p := range result.People {
			key := normalizeString(p.Name)
			if existing, ok := peopleMap[key]; !ok || len(p.Role) > len(existing.Role) {
				// Keep the one with more role info
				peopleMap[key] = workflows.PersonResult{Name: p.Name, Role: p.Role}
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

// normalizeString converts a string to lowercase and trims whitespace for deduplication.
func normalizeString(s string) string {
	return strings.TrimSpace(strings.ToLower(s))
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

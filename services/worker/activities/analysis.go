// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"context"
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/tracing"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// AnalysisActivities holds dependencies for analysis-related activities.
type AnalysisActivities struct {
	logger       logging.Logger
	aiClient     AIClient
	pipelineRepo PipelineRepository
}

// NewAnalysisActivities creates a new AnalysisActivities instance.
func NewAnalysisActivities(
	logger logging.Logger,
	aiClient AIClient,
	pipelineRepo PipelineRepository,
) *AnalysisActivities {
	if logger == nil {
		panic("NewAnalysisActivities: logger is required")
	}
	if aiClient == nil {
		panic("NewAnalysisActivities: aiClient is required")
	}
	// pipelineRepo is optional (provenance recording)
	return &AnalysisActivities{
		logger:       logger.With(logging.F("component", "analysis_activities")),
		aiClient:     aiClient,
		pipelineRepo: pipelineRepo,
	}
}

// DeepAnalyze performs Stage 4 deep analysis using a remote LLM.
// Takes pre-processed input from pipeline stages and calls the DeepAnalyze RPC.
func (a *AnalysisActivities) DeepAnalyze(ctx context.Context, input workflows.DeepAnalyzeInput) (*workflows.DeepAnalyzeOutput, error) {
	// Set trace_id in context for log correlation
	if input.ContentID != "" {
		ctx = context.WithValue(ctx, logging.TraceIDKey, input.ContentID)
	}
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "DeepAnalyze"),
		logging.F("tenant_id", input.TenantID),
		logging.F("source_id", input.SourceID),
		logging.F("job_id", input.JobID),
		logging.F("content_length", len(input.Content)),
		logging.F("content_type", input.ContentType),
	)

	// Record initial heartbeat
	recordHeartbeat(ctx, "starting deep analysis")

	logger.Info("Starting deep analysis")

	// Check for cancellation
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Validate input
	if input.Content == "" {
		return nil, temporal.NewApplicationError(
			"content is empty",
			"ValidationError",
		)
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

	// Start LLM call trace
	contentID := input.ContentID
	if contentID == "" {
		contentID = fmt.Sprintf("%d", input.SourceID)
	}
	startTime := time.Now()
	ctx, llmSpan := tracing.StartLLMCall(ctx, "ai.deep_analyze", tracing.LLMCallOptions{
		TenantID:  input.TenantID,
		ContentID: contentID,
		TaskType:  "analyze",
	})
	defer llmSpan.End()

	// Record heartbeat before calling AI service
	recordHeartbeat(ctx, "calling AI service for deep analysis")

	// Build DeepAnalyzeRequest from input
	req := buildDeepAnalyzeRequest(input)

	// Call AI service
	resp, err := a.aiClient.DeepAnalyze(ctx, req)
	if err != nil {
		tracing.SetLLMResult(llmSpan, tracing.LLMResult{
			LatencyMs: time.Since(startTime).Milliseconds(),
			Error:     err,
		})
		logger.Error("Failed to perform deep analysis", logging.Err(err))
		return nil, fmt.Errorf("failed to perform deep analysis: %w", err)
	}

	// Record LLM result
	tracing.SetLLMResult(llmSpan, tracing.LLMResult{
		Model:     resp.ModelUsed,
		LatencyMs: time.Since(startTime).Milliseconds(),
	})

	// Convert proto response to domain output types
	output := convertDeepAnalyzeResponse(resp)

	// Add tracing attributes
	tracing.SetAttributes(llmSpan,
		tracing.AttrInt("analysis.topic_mappings", len(output.TopicMappings)),
		tracing.AttrInt("analysis.verified_actions", len(output.VerifiedActions)),
		tracing.AttrInt("analysis.verified_decisions", len(output.VerifiedDecisions)),
		tracing.AttrInt("analysis.risk_references", len(output.RiskReferences)),
		tracing.AttrInt("analysis.insights", len(output.Insights)),
		tracing.AttrInt("analysis.implicit_actions", len(output.ImplicitActions)),
	)

	// Record heartbeat after processing
	recordHeartbeat(ctx, "deep analysis complete")

	logger.Info("Deep analysis completed successfully",
		logging.F("ai_duration", time.Since(startTime)),
		logging.F("topic_mappings", len(output.TopicMappings)),
		logging.F("verified_actions", len(output.VerifiedActions)),
		logging.F("verified_decisions", len(output.VerifiedDecisions)),
		logging.F("risk_references", len(output.RiskReferences)),
		logging.F("insights", len(output.Insights)),
		logging.F("implicit_actions", len(output.ImplicitActions)),
		logging.F("model", output.ModelUsed),
	)

	// Record pipeline run for provenance tracking (Stage 4: analyze)
	if a.pipelineRepo != nil {
		durationMS := int(time.Since(startTime).Milliseconds())
		runErr := a.pipelineRepo.CreateRun(ctx, PipelineRunInput{
			SourceID:   input.SourceID,
			Stage:      "analyze",
			ModelID:    output.ModelUsed,
			Status:     "completed",
			DurationMS: durationMS,
		})
		if runErr != nil {
			logger.Warn("Failed to record pipeline run", logging.Err(runErr))
		}
	}

	return output, nil
}

// buildDeepAnalyzeRequest converts workflows.DeepAnalyzeInput to proto request.
func buildDeepAnalyzeRequest(input workflows.DeepAnalyzeInput) *aiv1.DeepAnalyzeRequest {
	req := &aiv1.DeepAnalyzeRequest{
		Content:          input.Content,
		TriageCategory:   input.TriageCategory,
		TriageImportance: input.TriageImportance,
		BackgroundContext: input.BackgroundContext,
	}

	// Set optional fields
	if input.TenantID != "" {
		req.TenantId = &input.TenantID
	}
	if input.SourceID > 0 {
		req.SourceId = &input.SourceID
	}
	if input.ContentID != "" {
		req.ContentId = &input.ContentID
	}

	// Convert workflows.SLMPipelineExtractEntitiesOutput to proto types
	if input.ExtractionResult != nil {
		// Convert people
		for _, p := range input.ExtractionResult.People {
			req.VerifiedPeople = append(req.VerifiedPeople, &aiv1.PersonEntity{
				Name: p.Name,
				Role: p.Role,
			})
		}

		// Convert dates
		for _, d := range input.ExtractionResult.Dates {
			req.VerifiedDates = append(req.VerifiedDates, &aiv1.DateEntity{
				Date:    d.Date,
				Context: d.Context,
			})
		}

		// Convert projects
		req.VerifiedProjects = input.ExtractionResult.Projects

		// Convert organisations
		req.VerifiedOrganisations = input.ExtractionResult.Organisations

		// Convert preliminary action items
		for _, ai := range input.ExtractionResult.ActionItems {
			req.PreliminaryActionItems = append(req.PreliminaryActionItems, &aiv1.ActionItemEntity{
				Assignee: ai.Assignee,
				Action:   ai.Action,
				Due:      ai.Due,
			})
		}

		// Convert preliminary decisions
		req.PreliminaryDecisions = input.ExtractionResult.Decisions

		// Convert preliminary risks
		req.PreliminaryRisks = input.ExtractionResult.Risks
	}

	return req
}

// convertDeepAnalyzeResponse converts proto response to domain output types.
func convertDeepAnalyzeResponse(resp *aiv1.DeepAnalyzeResponse) *workflows.DeepAnalyzeOutput {
	output := &workflows.DeepAnalyzeOutput{
		Summary:   resp.Summary,
		ModelUsed: resp.ModelUsed,
	}

	// Convert sentiment
	if resp.Sentiment != nil {
		output.Sentiment = &workflows.SentimentOutput{
			Score:       resp.Sentiment.Score,
			Label:       resp.Sentiment.Label,
			Confidence:  resp.Sentiment.Confidence,
			Indicators:  resp.Sentiment.Indicators,
			Explanation: resp.Sentiment.Explanation,
		}
	}

	// Convert topic mappings
	for _, tm := range resp.TopicMappings {
		output.TopicMappings = append(output.TopicMappings, workflows.TopicMappingOutput{
			Topic:          tm.Topic,
			RelatedProject: tm.RelatedProject,
			Relationship:   tm.Relationship,
			Confidence:     tm.Confidence,
		})
	}

	// Convert verified action items
	for _, va := range resp.VerifiedActionItems {
		output.VerifiedActions = append(output.VerifiedActions, workflows.VerifiedActionOutput{
			Description:    va.Description,
			Assignee:       va.Assignee,
			Due:            va.Due,
			Priority:       va.Priority,
			ContextExcerpt: va.ContextExcerpt,
			Status:         va.Status,
		})
	}

	// Convert verified decisions
	for _, vd := range resp.VerifiedDecisions {
		output.VerifiedDecisions = append(output.VerifiedDecisions, workflows.VerifiedDecisionOutput{
			Description:    vd.Description,
			ContextExcerpt: vd.ContextExcerpt,
			Status:         vd.Status,
		})
	}

	// Convert risk references
	for _, rr := range resp.RiskReferences {
		riskRef := workflows.RiskReferenceOutput{
			Description:    rr.Description,
			Significance:   rr.Significance,
			ContextExcerpt: rr.ContextExcerpt,
			IsNew:          rr.IsNew,
		}
		if rr.RootId != nil {
			riskRef.RootID = rr.RootId
		}
		if rr.LifecycleChange != nil {
			riskRef.LifecycleChange = rr.LifecycleChange
		}
		if rr.SeverityChange != nil {
			riskRef.SeverityChange = rr.SeverityChange
		}
		if rr.OwnerChange != nil {
			riskRef.OwnerChange = rr.OwnerChange
		}
		output.RiskReferences = append(output.RiskReferences, riskRef)
	}

	// Convert strategic insights
	output.Insights = resp.StrategicInsights

	// Convert implicit action items
	for _, ia := range resp.ImplicitActionItems {
		output.ImplicitActions = append(output.ImplicitActions, workflows.ImplicitActionOutput{
			Description:    ia.Description,
			Reasoning:      ia.Reasoning,
			ContextExcerpt: ia.ContextExcerpt,
		})
	}

	return output
}

// Ensure AnalysisActivities implements required interfaces at compile time.
var _ interface {
	DeepAnalyze(ctx context.Context, input workflows.DeepAnalyzeInput) (*workflows.DeepAnalyzeOutput, error)
} = (*AnalysisActivities)(nil)

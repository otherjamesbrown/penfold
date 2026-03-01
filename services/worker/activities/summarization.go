// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"context"
	"fmt"
	"time"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"google.golang.org/grpc/metadata"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/tracing"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// SummarizationActivities holds dependencies for summarization-related activities.
type SummarizationActivities struct {
	logger      logging.Logger
	aiClient    AIClient
	summaryRepo SummaryRepository
}

// NewSummarizationActivities creates a new SummarizationActivities instance.
func NewSummarizationActivities(
	logger logging.Logger,
	aiClient AIClient,
	summaryRepo SummaryRepository,
) *SummarizationActivities {
	if logger == nil {
		panic("NewSummarizationActivities: logger is required")
	}
	if aiClient == nil {
		panic("NewSummarizationActivities: aiClient is required")
	}
	if summaryRepo == nil {
		panic("NewSummarizationActivities: summaryRepo is required")
	}
	return &SummarizationActivities{
		logger:      logger.With(logging.F("component", "summarization_activities")),
		aiClient:    aiClient,
		summaryRepo: summaryRepo,
	}
}

// GenerateSummary generates a summary for the given content using an LLM.
// The summary includes both a text summary and extracted key points.
func (a *SummarizationActivities) GenerateSummary(ctx context.Context, input workflows.GenerateSummaryInput) (int64, error) {
	logger := a.logger.With(
		logging.F("activity", "GenerateSummary"),
		logging.F("tenant_id", input.TenantID),
		logging.F("source_id", input.SourceID),
		logging.F("job_id", input.JobID),
		logging.F("content_length", len(input.Content)),
	)

	// Record initial heartbeat
	activity.RecordHeartbeat(ctx, "starting summary generation")

	logger.Info("Generating summary for content")

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

	// Call AI service to generate summary
	startTime := time.Now()
	activity.RecordHeartbeat(ctx, "calling AI service for summary generation")

	// Create stage span wrapping the gRPC call
	stageCtx, stageSpan := tracing.StartStageSpan(ctx, "stage.summarize", tracing.StageSpanOptions{
		ContentID: input.ContentID,
		TenantID:  input.TenantID,
	})
	defer stageSpan.End()

	// Default parameters for summary generation
	maxLength := int32(150) // tokens

	summaryReq := &aiv1.SummaryRequest{
		Content:   input.Content,
		MaxLength: &maxLength,
		Style:     aiv1.SummaryStyle_SUMMARY_STYLE_BRIEF,
		TenantId:  &input.TenantID,
	}
	if input.PromptOverride > 0 {
		summaryReq.PromptVersion = &input.PromptOverride
	}

	// Attach Langfuse tracing metadata for AI coordinator to use when creating generation spans.
	summaryCallCtx := stageCtx
	if input.LangfuseTraceID != "" {
		md := metadata.Pairs(
			"x-langfuse-trace-id", input.LangfuseTraceID,
			"x-langfuse-phase-id", input.LangfusePhaseID,
		)
		summaryCallCtx = metadata.NewOutgoingContext(stageCtx, md)
	}

	resp, err := a.aiClient.GenerateSummary(summaryCallCtx, summaryReq)
	if err != nil {
		logger.Error("Failed to generate summary from AI service", logging.Err(err))
		return 0, fmt.Errorf("failed to generate summary: %w", err)
	}

	// Record heartbeat after AI call
	activity.RecordHeartbeat(ctx, "summary generated, storing")

	logger.Info("Summary generated successfully",
		logging.F("ai_duration", time.Since(startTime)),
		logging.F("summary_length", len(resp.Summary)),
		logging.F("key_points", len(resp.KeyPoints)),
		logging.F("model", resp.ModelUsed),
	)

	// Check if repository is available for storage
	if a.summaryRepo == nil {
		logger.Warn("Summary repository not configured, skipping storage")
		return 0, nil
	}

	// Store the summary
	storeStart := time.Now()
	summaryID, err := a.summaryRepo.StoreSummary(
		ctx,
		input.TenantID,
		input.SourceID,
		resp.Summary,
		resp.KeyPoints,
		resp.ModelUsed,
	)
	if err != nil {
		logger.Error("Failed to store summary", logging.Err(err))
		return 0, fmt.Errorf("failed to store summary: %w", err)
	}

	logger.Info("Summary stored successfully",
		logging.F("store_duration", time.Since(storeStart)),
		logging.F("summary_id", summaryID),
	)

	return summaryID, nil
}

// GenerateSummaryWithOptions generates a summary with custom options.
type GenerateSummaryWithOptionsInput struct {
	TenantID  string            `json:"tenant_id"`
	SourceID  int64             `json:"source_id"`
	// ContentID is the unique content identifier for tracing (format: <type:2>-<base62:8>)
	ContentID       string            `json:"content_id,omitempty"`
	JobID           string            `json:"job_id"`
	Content         string            `json:"content"`
	MaxLength       int32             `json:"max_length,omitempty"`
	Style           aiv1.SummaryStyle `json:"style,omitempty"`
	Model           string            `json:"model,omitempty"`
}

// GenerateSummaryOutput contains the result of summary generation.
type GenerateSummaryOutput struct {
	SummaryID    int64    `json:"summary_id"`
	Summary      string   `json:"summary"`
	KeyPoints    []string `json:"key_points"`
	ModelUsed    string   `json:"model_used"`
	InputTokens  int32    `json:"input_tokens,omitempty"`
	OutputTokens int32    `json:"output_tokens,omitempty"`
}

// GenerateSummaryWithOptions generates a summary with custom options and returns detailed output.
func (a *SummarizationActivities) GenerateSummaryWithOptions(ctx context.Context, input GenerateSummaryWithOptionsInput) (*GenerateSummaryOutput, error) {
	logger := a.logger.With(
		logging.F("activity", "GenerateSummaryWithOptions"),
		logging.F("tenant_id", input.TenantID),
		logging.F("source_id", input.SourceID),
		logging.F("job_id", input.JobID),
		logging.F("content_length", len(input.Content)),
		logging.F("max_length", input.MaxLength),
		logging.F("style", input.Style.String()),
	)

	// Record initial heartbeat
	activity.RecordHeartbeat(ctx, "starting summary generation with options")

	logger.Info("Generating summary with custom options")

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

	// Set defaults
	maxLength := input.MaxLength
	if maxLength == 0 {
		maxLength = 150
	}

	style := input.Style
	if style == aiv1.SummaryStyle_SUMMARY_STYLE_UNSPECIFIED {
		style = aiv1.SummaryStyle_SUMMARY_STYLE_BRIEF
	}

	// Call AI service
	startTime := time.Now()
	activity.RecordHeartbeat(ctx, "calling AI service")

	summaryReq := &aiv1.SummaryRequest{
		Content:   input.Content,
		MaxLength: &maxLength,
		Style:     style,
		TenantId:  &input.TenantID,
	}
	if input.Model != "" {
		summaryReq.Model = &input.Model
	}
	// Pipeline span context is injected at the activity level; gRPC OTel interceptors propagate traceparent.

	// Tracing is handled by the AI server, not duplicated here
	resp, err := a.aiClient.GenerateSummary(ctx, summaryReq)
	if err != nil {
		logger.Error("Failed to generate summary from AI service", logging.Err(err))
		return nil, fmt.Errorf("failed to generate summary: %w", err)
	}

	activity.RecordHeartbeat(ctx, "summary generated, storing")

	logger.Info("Summary generated successfully",
		logging.F("ai_duration", time.Since(startTime)),
		logging.F("summary_length", len(resp.Summary)),
		logging.F("key_points", len(resp.KeyPoints)),
		logging.F("model", resp.ModelUsed),
	)

	output := &GenerateSummaryOutput{
		Summary:      resp.Summary,
		KeyPoints:    resp.KeyPoints,
		ModelUsed:    resp.ModelUsed,
		InputTokens:  resp.GetInputTokens(),
		OutputTokens: resp.GetOutputTokens(),
	}

	// Store if repository is available
	if a.summaryRepo != nil {
		storeStart := time.Now()
		summaryID, err := a.summaryRepo.StoreSummary(
			ctx,
			input.TenantID,
			input.SourceID,
			resp.Summary,
			resp.KeyPoints,
			resp.ModelUsed,
		)
		if err != nil {
			logger.Error("Failed to store summary", logging.Err(err))
			return nil, fmt.Errorf("failed to store summary: %w", err)
		}
		output.SummaryID = summaryID
		logger.Info("Summary stored successfully",
			logging.F("store_duration", time.Since(storeStart)),
			logging.F("summary_id", summaryID),
		)
	}

	return output, nil
}

// DeleteSummary deletes a summary by source ID.
// This is a compensation activity used by the saga pattern when downstream stages fail.
func (a *SummarizationActivities) DeleteSummary(ctx context.Context, summaryID int64) error {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "DeleteSummary"),
		logging.F("summary_id", summaryID),
	)

	activity.RecordHeartbeat(ctx, "deleting summary")

	logger.Info("Deleting summary for saga compensation")

	if ctx.Err() != nil {
		return ctx.Err()
	}

	if summaryID <= 0 {
		return temporal.NewApplicationError(
			fmt.Sprintf("invalid summary_id: %d", summaryID),
			"ValidationError",
		)
	}

	if a.summaryRepo == nil {
		logger.Warn("Summary repository not configured")
		return temporal.NewApplicationErrorWithCause(
			"summary repository not configured",
			"ConfigurationError",
			nil,
		)
	}

	if err := a.summaryRepo.DeleteSummary(ctx, summaryID); err != nil {
		logger.Error("Failed to delete summary", logging.Err(err))
		return fmt.Errorf("failed to delete summary: %w", err)
	}

	logger.Info("Summary deleted successfully")
	return nil
}

// Ensure SummarizationActivities implements required interfaces at compile time.
var _ interface {
	GenerateSummary(ctx context.Context, input workflows.GenerateSummaryInput) (int64, error)
	DeleteSummary(ctx context.Context, summaryID int64) error
} = (*SummarizationActivities)(nil)

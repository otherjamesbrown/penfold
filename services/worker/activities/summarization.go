// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"context"
	"fmt"
	"time"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"

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

	// Start LLM call trace
	// Note: ContentID uses source_id for now; will be updated when content ID propagation is complete
	ctx, llmSpan := tracing.StartLLMCall(ctx, "ai.summarize", tracing.LLMCallOptions{
		TenantID:  input.TenantID,
		ContentID: fmt.Sprintf("%d", input.SourceID),
		TaskType:  "summarize",
	})
	defer llmSpan.End()

	// Default parameters for summary generation
	maxLength := int32(150) // tokens

	summaryReq := &aiv1.SummaryRequest{
		Content:   input.Content,
		MaxLength: &maxLength,
		Style:     aiv1.SummaryStyle_SUMMARY_STYLE_BRIEF,
		TenantId:  &input.TenantID,
	}

	resp, err := a.aiClient.GenerateSummary(ctx, summaryReq)
	if err != nil {
		tracing.SetLLMResult(llmSpan, tracing.LLMResult{
			LatencyMs: time.Since(startTime).Milliseconds(),
			Error:     err,
		})
		logger.Error("Failed to generate summary from AI service", logging.Err(err))
		return 0, fmt.Errorf("failed to generate summary: %w", err)
	}

	// Record LLM result
	tracing.SetLLMResult(llmSpan, tracing.LLMResult{
		InputTokens:  int(resp.GetInputTokens()),
		OutputTokens: int(resp.GetOutputTokens()),
		Model:        resp.ModelUsed,
		LatencyMs:    time.Since(startTime).Milliseconds(),
	})
	tracing.SetAttributes(llmSpan,
		tracing.AttrInt("summary.length", len(resp.Summary)),
		tracing.AttrInt("summary.key_points_count", len(resp.KeyPoints)),
	)

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
	JobID     string            `json:"job_id"`
	Content   string            `json:"content"`
	MaxLength int32             `json:"max_length,omitempty"`
	Style     aiv1.SummaryStyle `json:"style,omitempty"`
	Model     string            `json:"model,omitempty"`
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

	// Start LLM call trace
	// Note: ContentID uses source_id for now; will be updated when content ID propagation is complete
	ctx, llmSpan := tracing.StartLLMCall(ctx, "ai.summarize", tracing.LLMCallOptions{
		Model:     input.Model,
		TenantID:  input.TenantID,
		ContentID: fmt.Sprintf("%d", input.SourceID),
		TaskType:  "summarize",
	})
	defer llmSpan.End()
	tracing.SetAttributes(llmSpan,
		tracing.AttrInt("summary.max_length", int(maxLength)),
		tracing.Attr("summary.style", style.String()),
	)

	summaryReq := &aiv1.SummaryRequest{
		Content:   input.Content,
		MaxLength: &maxLength,
		Style:     style,
		TenantId:  &input.TenantID,
	}
	if input.Model != "" {
		summaryReq.Model = &input.Model
	}

	resp, err := a.aiClient.GenerateSummary(ctx, summaryReq)
	if err != nil {
		tracing.SetLLMResult(llmSpan, tracing.LLMResult{
			LatencyMs: time.Since(startTime).Milliseconds(),
			Error:     err,
		})
		logger.Error("Failed to generate summary from AI service", logging.Err(err))
		return nil, fmt.Errorf("failed to generate summary: %w", err)
	}

	// Record LLM result
	tracing.SetLLMResult(llmSpan, tracing.LLMResult{
		InputTokens:  int(resp.GetInputTokens()),
		OutputTokens: int(resp.GetOutputTokens()),
		Model:        resp.ModelUsed,
		LatencyMs:    time.Since(startTime).Milliseconds(),
	})
	tracing.SetAttributes(llmSpan,
		tracing.AttrInt("summary.length", len(resp.Summary)),
		tracing.AttrInt("summary.key_points_count", len(resp.KeyPoints)),
	)

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

// Ensure SummarizationActivities implements required interfaces at compile time.
var _ interface {
	GenerateSummary(ctx context.Context, input workflows.GenerateSummaryInput) (int64, error)
} = (*SummarizationActivities)(nil)

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
)

// TriageActivities holds dependencies for triage-related activities.
type TriageActivities struct {
	logger       logging.Logger
	aiClient     AIClient
	pipelineRepo PipelineRepository
}

// NewTriageActivities creates a new TriageActivities instance.
func NewTriageActivities(
	logger logging.Logger,
	aiClient AIClient,
	pipelineRepo PipelineRepository,
) *TriageActivities {
	return &TriageActivities{
		logger:       logger.With(logging.F("component", "triage_activities")),
		aiClient:     aiClient,
		pipelineRepo: pipelineRepo,
	}
}

// TriageInput is the input for the Triage activity.
type TriageInput struct {
	TenantID    string `json:"tenant_id"`
	SourceID    int64  `json:"source_id"`
	ContentID   string `json:"content_id,omitempty"`
	JobID       string `json:"job_id"`
	Content     string `json:"content"`
	Subject     string `json:"subject,omitempty"`
	SenderEmail string `json:"sender_email,omitempty"`
	ContentType string `json:"content_type"` // email, meeting, slack
}

// TriageOutput is the output from the Triage activity.
type TriageOutput struct {
	Category   string  `json:"category"`   // PROJECT_UPDATE, CUSTOMER, RISK_ISSUE, etc.
	Importance string  `json:"importance"` // HIGH, MEDIUM, LOW
	Reason     string  `json:"reason"`
	Confidence float32 `json:"confidence"` // Derived from model confidence (0.0-1.0)
	ModelUsed  string  `json:"model_used"`
	SkipDeep   bool    `json:"skip_deep"` // True if LOW/PERSONAL - skip stages 2-4
}

// Triage performs Stage 1 content triage using an SLM.
// Classifies content into categories and importance levels.
func (a *TriageActivities) Triage(ctx context.Context, input TriageInput) (*TriageOutput, error) {
	// Set trace_id in context for log correlation
	if input.ContentID != "" {
		ctx = context.WithValue(ctx, logging.TraceIDKey, input.ContentID)
	}
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "Triage"),
		logging.F("tenant_id", input.TenantID),
		logging.F("source_id", input.SourceID),
		logging.F("job_id", input.JobID),
		logging.F("content_length", len(input.Content)),
		logging.F("content_type", input.ContentType),
	)

	// Record initial heartbeat
	recordHeartbeat(ctx, "starting triage")

	logger.Info("Starting content triage")

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
	ctx, llmSpan := tracing.StartLLMCall(ctx, "ai.triage_content", tracing.LLMCallOptions{
		TenantID:  input.TenantID,
		ContentID: contentID,
		TaskType:  "triage",
	})
	defer llmSpan.End()

	// Record heartbeat before calling AI service
	recordHeartbeat(ctx, "calling AI service for triage")

	// Build TriageContentRequest
	req := &aiv1.TriageContentRequest{
		Content: input.Content,
	}
	if input.Subject != "" {
		req.Subject = &input.Subject
	}
	if input.SenderEmail != "" {
		req.Sender = &input.SenderEmail
	}
	if input.TenantID != "" {
		req.TenantId = &input.TenantID
	}
	if input.SourceID > 0 {
		req.SourceId = &input.SourceID
	}

	// Call AI service
	resp, err := a.aiClient.TriageContent(ctx, req)
	if err != nil {
		tracing.SetLLMResult(llmSpan, tracing.LLMResult{
			LatencyMs: time.Since(startTime).Milliseconds(),
			Error:     err,
		})
		logger.Error("Failed to perform triage", logging.Err(err))
		return nil, fmt.Errorf("failed to perform triage: %w", err)
	}

	// Record LLM result
	tracing.SetLLMResult(llmSpan, tracing.LLMResult{
		Model:     resp.ModelUsed,
		LatencyMs: time.Since(startTime).Milliseconds(),
	})

	// Determine if we should skip deep processing (Stage 2-4)
	skipDeep := shouldSkipDeep(resp.Category, resp.Importance)

	// Convert proto response to domain output
	output := &TriageOutput{
		Category:   resp.Category,
		Importance: resp.Importance,
		Reason:     resp.Reason,
		Confidence: 0.85, // Default confidence - can be enhanced later
		ModelUsed:  resp.ModelUsed,
		SkipDeep:   skipDeep,
	}

	// Add tracing attributes
	tracing.SetAttributes(llmSpan,
		tracing.Attr("triage.category", output.Category),
		tracing.Attr("triage.importance", output.Importance),
		tracing.AttrBool("triage.skip_deep", output.SkipDeep),
	)

	// Record heartbeat after processing
	recordHeartbeat(ctx, "triage complete")

	logger.Info("Triage completed successfully",
		logging.F("ai_duration", time.Since(startTime)),
		logging.F("category", output.Category),
		logging.F("importance", output.Importance),
		logging.F("skip_deep", output.SkipDeep),
		logging.F("model", output.ModelUsed),
	)

	// Record pipeline run for provenance tracking
	if a.pipelineRepo != nil {
		durationMS := int(time.Since(startTime).Milliseconds())
		runErr := a.pipelineRepo.CreateRun(ctx, PipelineRunInput{
			SourceID:   input.SourceID,
			Stage:      "triage",
			ModelID:    output.ModelUsed,
			Status:     "completed",
			DurationMS: durationMS,
		})
		if runErr != nil {
			logger.Warn("Failed to record pipeline run", logging.Err(runErr))
			// Don't fail the activity if run recording fails
		}
	}

	return output, nil
}

// shouldSkipDeep determines if deep processing (stages 2-4) should be skipped
// based on the triage results.
// Skip when:
// - Category is PERSONAL (any importance)
// - Category is INTERNAL_COMMS and Importance is LOW
func shouldSkipDeep(category, importance string) bool {
	if category == "PERSONAL" {
		return true
	}
	if category == "INTERNAL_COMMS" && importance == "LOW" {
		return true
	}
	return false
}

// Ensure TriageActivities implements required interfaces at compile time.
var _ interface {
	Triage(ctx context.Context, input TriageInput) (*TriageOutput, error)
} = (*TriageActivities)(nil)

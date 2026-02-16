// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"context"
	"encoding/json"
	"time"

	"go.temporal.io/sdk/temporal"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/pkg/enrichment"
	perrors "github.com/otherjamesbrown/penfold/pkg/errors"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// TriageActivities holds dependencies for triage-related activities.
type TriageActivities struct {
	logger         logging.Logger
	aiClient       AIClient
	pipelineRepo   PipelineRepository
	enrichmentRepo EnrichmentRepository
}

// NewTriageActivities creates a new TriageActivities instance.
func NewTriageActivities(
	logger logging.Logger,
	aiClient AIClient,
	pipelineRepo PipelineRepository,
	enrichmentRepo EnrichmentRepository,
) *TriageActivities {
	if logger == nil {
		panic("NewTriageActivities: logger is required")
	}
	if aiClient == nil {
		panic("NewTriageActivities: aiClient is required")
	}
	// pipelineRepo is optional (provenance recording)
	// enrichmentRepo is optional (for content subtype classification)
	return &TriageActivities{
		logger:         logger.With(logging.F("component", "triage_activities")),
		aiClient:       aiClient,
		pipelineRepo:   pipelineRepo,
		enrichmentRepo: enrichmentRepo,
	}
}

// Triage performs Stage 1 content triage using an SLM.
// Classifies content into categories and importance levels.
func (a *TriageActivities) Triage(ctx context.Context, input workflows.TriageInput) (*workflows.TriageOutput, error) {
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

	// Classify content subtype BEFORE validating content (Stage 0.5 - before AI triage)
	// This allows us to handle metadata-only calendar invites with empty body
	// This runs for all content types but is most useful for emails
	var subtype enrichment.ContentSubtype
	if a.enrichmentRepo != nil && input.SourceID > 0 {
		subtype = enrichment.ClassifyContentSubtype(
			input.Headers,
			input.SenderEmail,
			input.Subject,
			nil, // TODO: Load tenant patterns from config if needed
		)

		logger.Info("Content subtype classified",
			logging.F("subtype", string(subtype)),
			logging.F("source_id", input.SourceID),
		)

		// Update the enrichment record with the classified subtype
		// This is a best-effort update; don't fail the activity if it fails
		if enrichmentRec, err := a.enrichmentRepo.GetBySourceID(ctx, input.SourceID); err == nil && enrichmentRec != nil {
			enrichmentRec.ContentSubtype = string(subtype)
			if updateErr := a.enrichmentRepo.Update(ctx, enrichmentRec); updateErr != nil {
				logger.Warn("Failed to update enrichment subtype",
					logging.Err(updateErr),
					logging.F("source_id", input.SourceID),
				)
			} else {
				logger.Info("Updated enrichment subtype",
					logging.F("source_id", input.SourceID),
					logging.F("subtype", string(subtype)),
				)
			}
		} else if err != nil {
			logger.Warn("Failed to fetch enrichment for subtype update",
				logging.Err(err),
				logging.F("source_id", input.SourceID),
			)
		}
	}

	// Handle calendar invites with empty body (pf-479452)
	// Calendar metadata (organizer, attendees, title, date) is extracted later in Stage 3,
	// so we can skip AI triage and return a metadata-only result
	if input.Content == "" && subtype.IsCalendar() {
		logger.Info("Calendar invite with empty body detected, skipping AI triage",
			logging.F("subtype", string(subtype)),
		)

		// Return metadata-only result without calling AI
		output := &workflows.TriageOutput{
			Category:       "MEETING",
			Importance:     "MEDIUM",
			Reason:         "Calendar invite with no body text (metadata-only)",
			Confidence:     1.0, // High confidence - deterministic classification
			ModelUsed:      "metadata-classifier",
			SkipDeep:       true, // Skip deep processing (Stages 2-4)
			ContentSubtype: string(subtype),
		}

		logger.Info("Triage completed (metadata-only)",
			logging.F("category", output.Category),
			logging.F("importance", output.Importance),
			logging.F("skip_deep", output.SkipDeep),
		)

		return output, nil
	}

	// Validate input - only fail if content is empty and it's NOT a calendar
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

	startTime := time.Now()

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
	if input.PipelineTraceID != "" {
		req.PipelineTraceId = &input.PipelineTraceID
	}
	if input.ContentID != "" {
		req.ContentId = &input.ContentID
	}

	// Call AI service (tracing is handled by the AI server, not duplicated here)
	resp, err := a.aiClient.TriageContent(ctx, req)
	if err != nil {
		pe := perrors.ClassifyError(err, "triage")
		logger.Error("Failed to perform triage", logging.Err(pe))
		return nil, WrapForTemporal(pe)
	}

	// Extract content_contribution from proto response (with default)
	contentContribution := ""
	if resp.ContentContribution != nil {
		contentContribution = *resp.ContentContribution
	}
	// Default to HIGH if not provided (never skip by accident)
	if contentContribution == "" {
		contentContribution = "HIGH"
	}

	contributionReason := ""
	if resp.ContributionReason != nil {
		contributionReason = *resp.ContributionReason
	}

	// Determine if we should skip deep processing (Stage 2-4)
	skipDeep := shouldSkipDeep(resp.Category, resp.Importance)

	// Convert proto response to domain output
	output := &workflows.TriageOutput{
		Category:            resp.Category,
		Importance:          resp.Importance,
		Reason:              resp.Reason,
		Confidence:          0.85, // Default confidence - can be enhanced later
		ModelUsed:           resp.ModelUsed,
		SkipDeep:            skipDeep,
		ContentSubtype:      string(subtype),
		ContentContribution: contentContribution,
		ContributionReason:  contributionReason,
	}

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

		// Capture IO data
		inputJSON, _ := json.Marshal(map[string]interface{}{
			"content_length": len(input.Content),
			"content_type": input.ContentType,
			"has_subject": input.Subject != "",
			"has_sender": input.SenderEmail != "",
			"tenant_id": input.TenantID,
		})
		outputJSON, _ := json.Marshal(map[string]interface{}{
			"category": resp.Category,
			"importance": resp.Importance,
			"model_used": resp.ModelUsed,
		})
		parsedJSON, _ := json.Marshal(output)

		runErr := a.pipelineRepo.CreateRun(ctx, PipelineRunInput{
			SourceID:   input.SourceID,
			Stage:      "triage",
			ModelID:    output.ModelUsed,
			Status:     "completed",
			DurationMS: durationMS,
			InputData:  inputJSON,
			OutputData: outputJSON,
			ParsedData: parsedJSON,
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
	Triage(ctx context.Context, input workflows.TriageInput) (*workflows.TriageOutput, error)
} = (*TriageActivities)(nil)

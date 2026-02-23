// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"

	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// SourceActivities holds dependencies for source-related activities.
type SourceActivities struct {
	logger     logging.Logger
	sourceRepo SourceRepository
}

// NewSourceActivities creates a new SourceActivities instance.
func NewSourceActivities(logger logging.Logger, sourceRepo SourceRepository) *SourceActivities {
	return &SourceActivities{
		logger:     logger.With(logging.F("component", "source_activities")),
		sourceRepo: sourceRepo,
	}
}

// FetchSource fetches the source content from the database.
// This activity retrieves the raw content for processing by subsequent activities.
func (a *SourceActivities) FetchSource(ctx context.Context, input workflows.FetchSourceInput) (*workflows.FetchSourceOutput, error) {
	logger := a.logger.With(
		logging.F("activity", "FetchSource"),
		logging.F("tenant_id", input.TenantID),
		logging.F("source_id", input.SourceID),
	)

	// Record heartbeat for monitoring
	activity.RecordHeartbeat(ctx, "starting fetch")

	logger.Info("Fetching source content from database")

	// Check for cancellation before expensive operations
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Check if repository is available
	if a.sourceRepo == nil {
		logger.Warn("Source repository not configured, returning placeholder")
		return nil, temporal.NewApplicationErrorWithCause(
			"source repository not configured",
			"ConfigurationError",
			nil,
		)
	}

	// Fetch the source from the repository
	startTime := time.Now()
	source, err := a.sourceRepo.GetSource(ctx, input.TenantID, input.SourceID)
	if err != nil {
		logger.Error("Failed to fetch source from database", logging.Err(err))
		return nil, fmt.Errorf("failed to fetch source %d: %w", input.SourceID, err)
	}

	// Record heartbeat after successful fetch
	activity.RecordHeartbeat(ctx, "fetch complete")

	logger.Info("Source content fetched successfully",
		logging.F("duration", time.Since(startTime)),
		logging.F("content_length", len(source.ContentText)),
		logging.F("content_type", source.ContentType),
	)

	// Parse participant emails from "to" and "cc" metadata keys.
	// These are stored as JSON arrays of {name, address} objects.
	participants := parseParticipants(source.Metadata)

	return &workflows.FetchSourceOutput{
		ContentText:       source.ContentText,
		ContentType:       source.ContentType,
		ContentID:         source.ContentID,
		SourceSystem:      source.SourceSystem,
		Subject:           source.Metadata["subject"],
		SenderEmail:       source.Metadata["from_address"],
		SenderName:        source.Metadata["from_name"],
		BodyHTML:          source.Metadata["body_html"],
		ParticipantEmails: participants,
		Headers:           extractHeaders(source.Metadata),
	}, nil
}

// participantEntry mirrors the {name, address} shape stored in metadata JSON arrays.
type participantEntry struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

// parseParticipants extracts Participant values from the "to" and "cc" metadata keys.
// Each key holds a JSON-encoded array of {name, address} objects.
func parseParticipants(metadata map[string]string) []workflows.Participant {
	if len(metadata) == 0 {
		return nil
	}

	var participants []workflows.Participant

	for _, key := range []string{"to", "cc"} {
		raw, ok := metadata[key]
		if !ok || raw == "" {
			continue
		}

		var entries []participantEntry
		if err := json.Unmarshal([]byte(raw), &entries); err != nil {
			// Malformed JSON is silently skipped; the field is best-effort.
			continue
		}

		for _, e := range entries {
			if e.Address == "" {
				continue
			}
			participants = append(participants, workflows.Participant{
				Email:       e.Address,
				DisplayName: e.Name,
				HeaderRole:  key,
			})
		}
	}

	return participants
}

// extractHeaders parses the "headers" key from source metadata.
// The gateway stores email MIME headers as a JSON-encoded map[string]string
// under metadata["headers"]. Returns nil if absent or unparseable.
func extractHeaders(metadata map[string]string) map[string]string {
	raw, ok := metadata["headers"]
	if !ok || raw == "" {
		return nil
	}
	var headers map[string]string
	if err := json.Unmarshal([]byte(raw), &headers); err != nil {
		return nil
	}
	return headers
}

// UpdateSourceStatus updates the processing status of a source.
// This activity is used to mark sources as processing, completed, or failed.
func (a *SourceActivities) UpdateSourceStatus(ctx context.Context, input workflows.UpdateSourceStatusInput) error {
	logger := a.logger.With(
		logging.F("activity", "UpdateSourceStatus"),
		logging.F("tenant_id", input.TenantID),
		logging.F("source_id", input.SourceID),
		logging.F("status", input.Status),
	)

	// Record heartbeat
	activity.RecordHeartbeat(ctx, fmt.Sprintf("updating status to %s", input.Status))

	logger.Info("Updating source status")

	// Check for cancellation
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Validate status
	validStatuses := map[string]bool{
		"pending":    true,
		"processing": true,
		"parsed":     true,
		"extracted":  true,
		"completed":  true,
		"failed":     true,
		"retrying":   true,
		"rejected":   true,
		"skipped":    true,
		"cancelled":  true,
	}
	if !validStatuses[input.Status] {
		return temporal.NewApplicationError(
			fmt.Sprintf("invalid status: %s", input.Status),
			"ValidationError",
		)
	}

	// Check if repository is available
	if a.sourceRepo == nil {
		logger.Warn("Source repository not configured")
		return temporal.NewApplicationErrorWithCause(
			"source repository not configured",
			"ConfigurationError",
			nil,
		)
	}

	// Update the status with failure fields and triage metadata (if present)
	startTime := time.Now()

	// Build triage metadata map — only skip_deep is written to JSONB
	// Classification fields (triage_category, content_subtype, source_system, etc.)
	// are stored in proper DB columns, not in ingestion_metadata JSONB.
	var triageMetadata map[string]interface{}
	if input.SkipDeep != nil {
		triageMetadata = map[string]interface{}{
			"skip_deep": *input.SkipDeep,
		}
	}

	// Call repository with or without triage metadata
	var err error
	if triageMetadata != nil {
		err = a.sourceRepo.UpdateSourceStatusWithFailure(ctx, input.TenantID, input.SourceID, input.Status, input.FailureCategory, input.FailureReason, triageMetadata)
	} else {
		err = a.sourceRepo.UpdateSourceStatusWithFailure(ctx, input.TenantID, input.SourceID, input.Status, input.FailureCategory, input.FailureReason)
	}

	if err != nil {
		logger.Error("Failed to update source status", logging.Err(err))
		return fmt.Errorf("failed to update source %d status to %s: %w", input.SourceID, input.Status, err)
	}

	logger.Info("Source status updated successfully",
		logging.F("duration", time.Since(startTime)),
		logging.F("failure_category", input.FailureCategory),
		logging.F("failure_reason", input.FailureReason),
		logging.F("triage_category", input.TriageCategory),
		logging.F("triage_importance", input.TriageImportance),
	)

	return nil
}

// Ensure SourceActivities implements required interfaces at compile time.
var _ interface {
	FetchSource(ctx context.Context, input workflows.FetchSourceInput) (*workflows.FetchSourceOutput, error)
	UpdateSourceStatus(ctx context.Context, input workflows.UpdateSourceStatusInput) error
} = (*SourceActivities)(nil)

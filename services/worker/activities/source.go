// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"

	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// SourceActivities holds dependencies for source-related activities.
type SourceActivities struct {
	logger     zerolog.Logger
	sourceRepo SourceRepository
}

// NewSourceActivities creates a new SourceActivities instance.
func NewSourceActivities(logger zerolog.Logger, sourceRepo SourceRepository) *SourceActivities {
	return &SourceActivities{
		logger:     logger.With().Str("component", "source_activities").Logger(),
		sourceRepo: sourceRepo,
	}
}

// FetchSource fetches the source content from the database.
// This activity retrieves the raw content for processing by subsequent activities.
func (a *SourceActivities) FetchSource(ctx context.Context, input workflows.FetchSourceInput) (*workflows.FetchSourceOutput, error) {
	logger := a.logger.With().
		Str("activity", "FetchSource").
		Str("tenant_id", input.TenantID).
		Int64("source_id", input.SourceID).
		Logger()

	// Record heartbeat for monitoring
	activity.RecordHeartbeat(ctx, "starting fetch")

	logger.Info().Msg("Fetching source content from database")

	// Check for cancellation before expensive operations
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Check if repository is available
	if a.sourceRepo == nil {
		logger.Warn().Msg("Source repository not configured, returning placeholder")
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
		logger.Error().Err(err).Msg("Failed to fetch source from database")
		return nil, fmt.Errorf("failed to fetch source %d: %w", input.SourceID, err)
	}

	// Record heartbeat after successful fetch
	activity.RecordHeartbeat(ctx, "fetch complete")

	logger.Info().
		Dur("duration", time.Since(startTime)).
		Int("content_length", len(source.ContentText)).
		Str("content_type", source.ContentType).
		Msg("Source content fetched successfully")

	return &workflows.FetchSourceOutput{
		ContentText: source.ContentText,
		ContentType: source.ContentType,
	}, nil
}

// UpdateSourceStatus updates the processing status of a source.
// This activity is used to mark sources as processing, completed, or failed.
func (a *SourceActivities) UpdateSourceStatus(ctx context.Context, input workflows.UpdateSourceStatusInput) error {
	logger := a.logger.With().
		Str("activity", "UpdateSourceStatus").
		Str("tenant_id", input.TenantID).
		Int64("source_id", input.SourceID).
		Str("status", input.Status).
		Logger()

	// Record heartbeat
	activity.RecordHeartbeat(ctx, fmt.Sprintf("updating status to %s", input.Status))

	logger.Info().Msg("Updating source status")

	// Check for cancellation
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Validate status
	validStatuses := map[string]bool{
		"pending":    true,
		"processing": true,
		"completed":  true,
		"failed":     true,
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
		logger.Warn().Msg("Source repository not configured")
		return temporal.NewApplicationErrorWithCause(
			"source repository not configured",
			"ConfigurationError",
			nil,
		)
	}

	// Update the status
	startTime := time.Now()
	if err := a.sourceRepo.UpdateSourceStatus(ctx, input.TenantID, input.SourceID, input.Status); err != nil {
		logger.Error().Err(err).Msg("Failed to update source status")
		return fmt.Errorf("failed to update source %d status to %s: %w", input.SourceID, input.Status, err)
	}

	logger.Info().
		Dur("duration", time.Since(startTime)).
		Msg("Source status updated successfully")

	return nil
}

// Ensure SourceActivities implements required interfaces at compile time.
var _ interface {
	FetchSource(ctx context.Context, input workflows.FetchSourceInput) (*workflows.FetchSourceOutput, error)
	UpdateSourceStatus(ctx context.Context, input workflows.UpdateSourceStatusInput) error
} = (*SourceActivities)(nil)

package activities

import (
	"context"
	"fmt"

	"go.temporal.io/sdk/activity"
)

// UpdateSourceStatusInput contains the input for the UpdateSourceStatus activity.
type UpdateSourceStatusInput struct {
	TenantID string `json:"tenant_id"`
	SourceID int64  `json:"source_id"`
	Status   string `json:"status"`
}

// UpdateSourceStatus updates the processing status of a source in the database.
func (a *Activities) UpdateSourceStatus(ctx context.Context, input UpdateSourceStatusInput) error {
	logger := activity.GetLogger(ctx)
	logger.Info("Updating source status", "source_id", input.SourceID, "status", input.Status)

	if err := a.sourceRepo.UpdateProcessingStatus(ctx, input.TenantID, input.SourceID, input.Status); err != nil {
		a.logger.Error().
			Err(err).
			Int64("source_id", input.SourceID).
			Str("status", input.Status).
			Msg("Failed to update processing status")
		return fmt.Errorf("failed to update source status: %w", err)
	}

	a.logger.Debug().
		Int64("source_id", input.SourceID).
		Str("status", input.Status).
		Msg("Source status updated successfully")

	return nil
}

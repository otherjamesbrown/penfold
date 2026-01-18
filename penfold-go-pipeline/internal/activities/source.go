package activities

import (
	"context"
	"fmt"

	"go.temporal.io/sdk/activity"
)

// FetchSourceInput contains the input for the FetchSource activity.
type FetchSourceInput struct {
	TenantID string `json:"tenant_id"`
	SourceID int64  `json:"source_id"`
}

// FetchSourceOutput contains the output from the FetchSource activity.
type FetchSourceOutput struct {
	SourceID    int64   `json:"source_id"`
	ContentText string  `json:"content_text"`
	ContentType *string `json:"content_type,omitempty"`
}

// FetchSource fetches source content from the database.
func (a *Activities) FetchSource(ctx context.Context, input FetchSourceInput) (*FetchSourceOutput, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Fetching source", "source_id", input.SourceID, "tenant_id", input.TenantID)

	source, err := a.sourceRepo.GetByID(ctx, input.TenantID, input.SourceID)
	if err != nil {
		a.logger.Error().Err(err).Int64("source_id", input.SourceID).Msg("Failed to fetch source")
		return nil, fmt.Errorf("failed to fetch source: %w", err)
	}

	if source == nil {
		a.logger.Warn().Int64("source_id", input.SourceID).Msg("Source not found")
		return nil, fmt.Errorf("source not found: id=%d", input.SourceID)
	}

	a.logger.Debug().
		Int64("source_id", input.SourceID).
		Int("content_length", len(source.ContentText)).
		Msg("Source fetched successfully")

	return &FetchSourceOutput{
		SourceID:    source.ID,
		ContentText: source.ContentText,
		ContentType: source.ContentType,
	}, nil
}

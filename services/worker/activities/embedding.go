// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// EmbeddingActivities holds dependencies for embedding-related activities.
type EmbeddingActivities struct {
	logger        zerolog.Logger
	aiClient      AIClient
	embeddingRepo EmbeddingRepository
}

// NewEmbeddingActivities creates a new EmbeddingActivities instance.
func NewEmbeddingActivities(logger zerolog.Logger, aiClient AIClient, embeddingRepo EmbeddingRepository) *EmbeddingActivities {
	return &EmbeddingActivities{
		logger:        logger.With().Str("component", "embedding_activities").Logger(),
		aiClient:      aiClient,
		embeddingRepo: embeddingRepo,
	}
}

// GenerateEmbedding generates a vector embedding for the given content.
// This activity calls the AI service to create embeddings and stores them in the database.
func (a *EmbeddingActivities) GenerateEmbedding(ctx context.Context, input workflows.GenerateEmbeddingInput) (int64, error) {
	logger := a.logger.With().
		Str("activity", "GenerateEmbedding").
		Str("tenant_id", input.TenantID).
		Int64("source_id", input.SourceID).
		Int("content_length", len(input.Content)).
		Logger()

	// Record initial heartbeat
	activity.RecordHeartbeat(ctx, "starting embedding generation")

	logger.Info().Msg("Generating embedding for content")

	// Check for cancellation before expensive operations
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
		logger.Warn().Msg("AI client not configured")
		return 0, temporal.NewApplicationErrorWithCause(
			"AI client not configured",
			"ConfigurationError",
			nil,
		)
	}

	// Call AI service to generate embedding
	startTime := time.Now()
	activity.RecordHeartbeat(ctx, "calling AI service")

	embeddingReq := &aiv1.EmbeddingRequest{
		Text:     input.Content,
		TenantId: &input.TenantID,
	}

	resp, err := a.aiClient.GenerateEmbedding(ctx, embeddingReq)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to generate embedding from AI service")
		return 0, fmt.Errorf("failed to generate embedding: %w", err)
	}

	// Record heartbeat after AI call
	activity.RecordHeartbeat(ctx, "embedding generated, storing")

	logger.Info().
		Dur("ai_duration", time.Since(startTime)).
		Int("dimensions", int(resp.Dimensions)).
		Str("model", resp.ModelUsed).
		Msg("Embedding generated successfully")

	// Check if repository is available for storage
	if a.embeddingRepo == nil {
		logger.Warn().Msg("Embedding repository not configured, skipping storage")
		// Return 0 to indicate no stored embedding, but operation was successful
		return 0, nil
	}

	// Store the embedding
	storeStart := time.Now()
	embeddingID, err := a.embeddingRepo.StoreEmbedding(
		ctx,
		input.TenantID,
		input.SourceID,
		resp.Vector,
		resp.ModelUsed,
		resp.Dimensions,
	)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to store embedding")
		return 0, fmt.Errorf("failed to store embedding: %w", err)
	}

	logger.Info().
		Dur("store_duration", time.Since(storeStart)).
		Int64("embedding_id", embeddingID).
		Msg("Embedding stored successfully")

	return embeddingID, nil
}

// GenerateEmbeddingBatch generates embeddings for multiple content items.
// This is an optimization for batch processing scenarios.
type GenerateEmbeddingBatchInput struct {
	TenantID string           `json:"tenant_id"`
	Items    []EmbeddingInput `json:"items"`
}

// EmbeddingInput represents a single item for embedding generation.
type EmbeddingInput struct {
	SourceID    int64  `json:"source_id"`
	Content     string `json:"content"`
	ContentHash string `json:"content_hash"`
}

// GenerateEmbeddingBatchOutput contains the results of batch embedding generation.
type GenerateEmbeddingBatchOutput struct {
	Results []EmbeddingResult `json:"results"`
}

// EmbeddingResult represents the result of a single embedding generation.
type EmbeddingResult struct {
	SourceID    int64  `json:"source_id"`
	EmbeddingID int64  `json:"embedding_id"`
	Success     bool   `json:"success"`
	Error       string `json:"error,omitempty"`
}

// GenerateEmbeddingBatch generates embeddings for multiple content items in a single activity.
// This reduces overhead for batch processing workflows.
func (a *EmbeddingActivities) GenerateEmbeddingBatch(ctx context.Context, input GenerateEmbeddingBatchInput) (*GenerateEmbeddingBatchOutput, error) {
	logger := a.logger.With().
		Str("activity", "GenerateEmbeddingBatch").
		Str("tenant_id", input.TenantID).
		Int("batch_size", len(input.Items)).
		Logger()

	logger.Info().Msg("Starting batch embedding generation")

	// Check for cancellation
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	if len(input.Items) == 0 {
		return &GenerateEmbeddingBatchOutput{Results: []EmbeddingResult{}}, nil
	}

	results := make([]EmbeddingResult, len(input.Items))

	for i, item := range input.Items {
		// Record heartbeat for each item
		activity.RecordHeartbeat(ctx, fmt.Sprintf("processing item %d/%d", i+1, len(input.Items)))

		// Check for cancellation between items
		if ctx.Err() != nil {
			// Mark remaining items as failed
			for j := i; j < len(input.Items); j++ {
				results[j] = EmbeddingResult{
					SourceID: input.Items[j].SourceID,
					Success:  false,
					Error:    "context cancelled",
				}
			}
			return &GenerateEmbeddingBatchOutput{Results: results}, ctx.Err()
		}

		// Generate embedding for this item
		embeddingID, err := a.GenerateEmbedding(ctx, workflows.GenerateEmbeddingInput{
			TenantID:    input.TenantID,
			SourceID:    item.SourceID,
			Content:     item.Content,
			ContentHash: item.ContentHash,
		})

		if err != nil {
			logger.Warn().
				Int64("source_id", item.SourceID).
				Err(err).
				Msg("Failed to generate embedding for item in batch")

			results[i] = EmbeddingResult{
				SourceID: item.SourceID,
				Success:  false,
				Error:    err.Error(),
			}
		} else {
			results[i] = EmbeddingResult{
				SourceID:    item.SourceID,
				EmbeddingID: embeddingID,
				Success:     true,
			}
		}
	}

	successCount := 0
	for _, r := range results {
		if r.Success {
			successCount++
		}
	}

	logger.Info().
		Int("success_count", successCount).
		Int("failure_count", len(input.Items)-successCount).
		Msg("Batch embedding generation completed")

	return &GenerateEmbeddingBatchOutput{Results: results}, nil
}

// Ensure EmbeddingActivities implements required interfaces at compile time.
var _ interface {
	GenerateEmbedding(ctx context.Context, input workflows.GenerateEmbeddingInput) (int64, error)
} = (*EmbeddingActivities)(nil)

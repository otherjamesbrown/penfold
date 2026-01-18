package activities

import (
	"context"
	"fmt"

	"go.temporal.io/sdk/activity"

	"github.com/otherjamesbrown/penfold-go-pipeline/internal/storage"
)

// GenerateEmbeddingInput contains the input for the GenerateEmbedding activity.
type GenerateEmbeddingInput struct {
	TenantID    string `json:"tenant_id"`
	SourceID    int64  `json:"source_id"`
	Content     string `json:"content"`
	ContentHash string `json:"content_hash"`
}

// GenerateEmbedding generates and stores an embedding for the given content.
// Returns the embedding ID on success.
func (a *Activities) GenerateEmbedding(ctx context.Context, input GenerateEmbeddingInput) (int64, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Generating embedding", "source_id", input.SourceID)

	// Record heartbeat before the potentially slow operation
	activity.RecordHeartbeat(ctx, "starting_embedding_generation")

	// Generate embedding using MLX sidecar
	embeddings, err := a.embedClient.EmbedBatch(ctx, []string{input.Content})
	if err != nil {
		a.logger.Error().Err(err).Int64("source_id", input.SourceID).Msg("Failed to generate embedding")
		return 0, fmt.Errorf("failed to generate embedding: %w", err)
	}

	if len(embeddings) == 0 {
		return 0, fmt.Errorf("no embeddings returned")
	}

	activity.RecordHeartbeat(ctx, "embedding_generated_storing")

	// Store embedding in database
	embedding := &storage.Embedding{
		TenantID:       input.TenantID,
		EntityType:     "source",
		EntityID:       input.SourceID,
		SourceID:       &input.SourceID,
		Embedding:      embeddings[0],
		EmbeddingModel: a.config.EmbeddingsModel,
		ContentHash:    &input.ContentHash,
	}

	if err := a.embeddingRepo.Store(ctx, embedding); err != nil {
		a.logger.Error().Err(err).Int64("source_id", input.SourceID).Msg("Failed to store embedding")
		return 0, fmt.Errorf("failed to store embedding: %w", err)
	}

	a.logger.Debug().
		Int64("source_id", input.SourceID).
		Int64("embedding_id", embedding.ID).
		Int("dimensions", len(embeddings[0])).
		Msg("Embedding stored successfully")

	return embedding.ID, nil
}
